/**
 * 制作模板弹窗（从虚拟机制作模板，仅管理员）
 * 迁移自旧前端 TemplateForm.vue
 */
import { useMemo, useState } from 'react'
import { Banner, Checkbox, Input, Modal, Radio, Select, TextArea, Toast } from '@douyinfe/semi-ui'
import { prepareTemplate } from '@/api/template'
import { confirmModal } from '@/utils/confirm'
import {
  DEFAULT_LINUX_TEMPLATE_CATEGORY,
  DEFAULT_WINDOWS_TEMPLATE_CATEGORY,
  normalizeTemplateCategory,
  templateCategoryOptions,
} from '@/utils/templateCategory'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface MakeTemplateDialogProps {
  vmName: string
  onClose: () => void
}

type TemplateType = 'linux' | 'windows' | 'fnos' | 'openwrt' | 'other'
type TransferMode = 'copy' | 'move'

/** 各系统类型的初始化方式选项 */
const INIT_MODE_OPTIONS: Record<TemplateType, Array<{ value: string; label: string; tip: string }>> = {
  linux: [
    { value: 'nocloud', label: '☁️ cloud-init（推荐）', tip: '模板内需预装 cloud-init，克隆时自动扩容磁盘、设置 hostname，无需 SSH 连接' },
    { value: 'none', label: '🚫 不初始化', tip: '克隆时将直接完整复制模板磁盘，不做任何初始化操作' },
  ],
  windows: [
    { value: 'configdrive', label: '🪟 ConfigDrive cloudbase-init（推荐）', tip: '克隆时通过 ConfigDrive 注入 cloudbase-init 配置，自动设置密码和网络' },
    { value: 'none', label: '🚫 不初始化', tip: '克隆时将直接完整复制模板磁盘，不做任何初始化操作' },
  ],
  fnos: [
    { value: 'fnos', label: '🛠️ virt-customize 离线初始化（推荐）', tip: '克隆时通过 virt-customize 注入用户名、密码、hostname、设备 ID 等 FnOS 首次启动配置' },
    { value: 'none', label: '🚫 不初始化', tip: '克隆时将直接完整复制模板磁盘，不做任何初始化操作' },
  ],
  openwrt: [
    { value: 'openwrt', label: '🌐 UCI 配置注入（推荐）', tip: '克隆时通过 virt-customize 注入静态 IP、网关、DNS 和主机名等 OpenWrt UCI 配置' },
    { value: 'none', label: '🚫 不初始化', tip: '克隆时将直接完整复制模板磁盘，不做任何初始化操作' },
  ],
  other: [
    { value: 'none', label: '🚫 不执行系统初始化', tip: '克隆时可选择链式或完整克隆，但不会执行任何系统初始化操作' },
  ],
}

const TYPE_OPTIONS: Array<{ value: TemplateType; label: string }> = [
  { value: 'linux', label: '🐧 Linux' },
  { value: 'windows', label: '🪟 Windows' },
  { value: 'fnos', label: '📦 FnOS' },
  { value: 'openwrt', label: '🌐 OpenWrt' },
  { value: 'other', label: '💾 其它' },
]

/** 选择「不初始化」时的风险确认文案 */
const NONE_INIT_CONFIRM: Record<TemplateType, { title: string; content: string; okText: string }> = {
  linux: {
    title: '风险确认：不初始化模板',
    content:
      '选择「不初始化」意味着克隆此模板时不会进行任何系统初始化操作（不会设置 hostname、不会扩容磁盘、不会注入密码），克隆出的虚拟机将完全保留模板的原始状态。\n\n请确保：\n1. 模板内已自行完成通用化处理（如删除 SSH 主机密钥、清理 machine-id 等）\n2. 模板磁盘大小已满足最终需求，后续不会自动扩容\n3. 你清楚克隆后需自行登录虚拟机进行个性化配置',
    okText: '我已知晓风险，继续',
  },
  windows: {
    title: '风险确认：不初始化模板',
    content:
      '选择「不初始化」意味着克隆此模板时不会进行任何系统初始化操作（不会注入 ConfigDrive、不会设置密码、不会执行 cloudbase-init）。克隆出的虚拟机将完全保留模板的原始状态。\n\n请在制作模板前务必对源虚拟机执行 sysprep 通用化：\n1. 运行 sysprep.exe 并勾选「通用」选项（/generalize）\n2. 关机后制作模板，确保 SID 和其他唯一标识已被清除\n3. 克隆后的 Windows 将在首次启动时重新进入 OOBE 初始化流程\n\n未通用化的 Windows 模板将导致克隆虚拟机出现 SID 冲突、域加入失败等问题。',
    okText: '已通用化，继续',
  },
  fnos: {
    title: '风险确认：不初始化模板',
    content:
      '选择「不初始化」意味着克隆此模板时不会进行任何系统初始化操作。克隆出的虚拟机将完全保留模板的原始状态。\n\n请确保模板已完成必要的通用化处理。',
    okText: '我已知晓风险，继续',
  },
  openwrt: {
    title: '风险确认：不初始化模板',
    content:
      '选择「不初始化」意味着克隆此模板时不会注入任何网络配置。克隆出的 OpenWrt 将保留模板原始 IP 配置。\n\n请确保模板已完成必要的通用化处理。',
    okText: '我已知晓风险，继续',
  },
  other: {
    title: '风险确认：其它模板',
    content: '其它模板克隆时不会执行任何系统初始化操作。',
    okText: '我已知晓风险，继续',
  },
}

function defaultInitMode(type: TemplateType): string {
  return INIT_MODE_OPTIONS[type][0].value
}

export default function MakeTemplateDialog({ vmName, onClose }: MakeTemplateDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [name, setName] = useState(`${vmName}-tpl`)
  const [displayName, setDisplayName] = useState(`${vmName}-tpl`)
  const [type, setType] = useState<TemplateType>('linux')
  const [category, setCategory] = useState(DEFAULT_LINUX_TEMPLATE_CATEGORY)
  const [initMode, setInitMode] = useState('nocloud')
  const [templateUser, setTemplateUser] = useState('')
  const [postBootCommand, setPostBootCommand] = useState('')
  const [postBootBlocking, setPostBootBlocking] = useState(false)
  const [compress, setCompress] = useState(false)
  const [transferMode, setTransferMode] = useState<TransferMode>('copy')
  const [loading, setLoading] = useState(false)

  const showCategory = ['linux', 'windows', 'openwrt'].includes(type)
  const isOtherType = type === 'other'
  const categoryOptions = useMemo(() => templateCategoryOptions(type), [type])
  const initOptions = INIT_MODE_OPTIONS[type]
  const initTip = initOptions.find((o) => o.value === initMode)?.tip || ''

  const handleTypeChange = (next: TemplateType) => {
    setType(next)
    setCategory(
      normalizeTemplateCategory(
        next,
        next === 'windows' ? DEFAULT_WINDOWS_TEMPLATE_CATEGORY : DEFAULT_LINUX_TEMPLATE_CATEGORY,
      ),
    )
    setInitMode(defaultInitMode(next))
  }

  const handleInitModeChange = async (value: string) => {
    if (value === 'none') {
      const confirmConfig = NONE_INIT_CONFIRM[type]
      const ok = await confirmModal({
        title: confirmConfig.title,
        content: confirmConfig.content,
        okText: confirmConfig.okText,
      })
      if (!ok) return
    }
    setInitMode(value)
  }

  const handleSubmit = async () => {
    if (name.includes('..')) {
      Toast.warning('模板名称不能包含连续的点')
      return
    }
    if (!/^[a-zA-Z0-9._-]+$/.test(name)) {
      Toast.warning('模板名称只能包含字母、数字、点、下划线和短横线')
      return
    }
    if (transferMode === 'move') {
      const confirmed = await confirmModal({
        title: '确认移动磁盘并删除源虚拟机',
        content:
          `系统盘将直接移动到模板目录，模板制作成功后会立即删除虚拟机“${vmName}”。` +
          '\n\n删除范围包含该虚拟机定义、快照及其它附加磁盘，此操作提交后需要二次验证，请确认已备份所需数据。',
        okText: '确认移动并删除',
        danger: true,
      })
      if (!confirmed) return
    }
    setLoading(true)
    try {
      await prepareTemplate({
        vm_name: vmName,
        template_name: name,
        display_name: displayName || name,
        type,
        compress,
        transfer_mode: compress ? 'copy' : transferMode,
        category: showCategory ? normalizeTemplateCategory(type, category) : undefined,
        cloud_init_mode: initMode === 'none' ? 'none' : initMode || undefined,
        template_user: templateUser || undefined,
        post_boot_command: postBootCommand || undefined,
        post_boot_blocking: postBootBlocking || undefined,
      })
      Toast.success('制作模板任务已提交，请在任务中心查看进度')
      requestClose()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title="制作模板"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="确定"
      cancelText="取消"
      confirmLoading={loading}
      width={520}
      closeOnEsc
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label">源虚拟机</div>
        <Input value={vmName} disabled />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">模板压缩</div>
        <Radio.Group
          type="button"
          value={compress ? 'compressed' : 'uncompressed'}
          onChange={(event) => {
            const nextCompressed = event.target.value === 'compressed'
            setCompress(nextCompressed)
            if (nextCompressed) setTransferMode('copy')
          }}
          options={[
            { value: 'uncompressed', label: '不压缩' },
            { value: 'compressed', label: '压缩' },
          ]}
        />
        <div className="qvm-form-tip">
          {compress
            ? '使用 qemu-img 压缩输出，耗时更长；链式克隆来源会继续保留父模板 backing 与子模板层级。'
            : '不重新编码磁盘，制作速度更快，并保留稀疏文件与现有模板链。'}
        </div>
      </div>
      {!compress && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">磁盘处理</div>
          <Radio.Group
            type="button"
            value={transferMode}
            onChange={(event) => setTransferMode(event.target.value as TransferMode)}
            options={[
              { value: 'copy', label: '复制' },
              { value: 'move', label: '移动' },
            ]}
          />
          <div className="qvm-form-tip">
            {transferMode === 'move'
              ? '直接移动系统盘；模板成功后删除源虚拟机，适合无需保留源虚拟机的场景。'
              : '保留源虚拟机及其磁盘，复制系统盘制作模板。'}
          </div>
        </div>
      )}
      {transferMode === 'move' && !compress && (
        <Banner
          type="warning"
          closeIcon={null}
          description="移动固定为不压缩。模板保存并校验成功后，源虚拟机及其快照、其它附加磁盘会被删除。"
        />
      )}
      <div className="qvm-form-item">
        <div className="qvm-form-label required">模板名称</div>
        <Input
          value={name}
          onChange={setName}
          placeholder="管理员侧名称（字母、数字、点、下划线、短横线）"
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">用户侧显示</div>
        <Input value={displayName} onChange={setDisplayName} placeholder="从模板克隆下拉框中显示的标题" />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">模板类型</div>
        <Radio.Group
          type="button"
          value={type}
          onChange={(e) => handleTypeChange(e.target.value as TemplateType)}
          options={TYPE_OPTIONS}
        />
      </div>

      {showCategory && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">
            {type === 'windows' ? 'Windows 分类' : type === 'openwrt' ? 'OpenWrt 分类' : 'Linux 分类'}
          </div>
          <Select
            style={{ width: '100%' }}
            value={category}
            onChange={(value) => setCategory(value as string)}
            filter
            optionList={categoryOptions.map((item) => ({ label: item, value: item }))}
          />
          <div className="qvm-form-tip">
            {type === 'windows'
              ? 'Windows 模板按版本分类展示，2012 R2 会保留 BIOS/SATA 等默认配置用于克隆'
              : type === 'openwrt'
                ? 'OpenWrt 模板克隆时支持配置静态 IP、网关和密码'
                : 'Linux 模板按发行版分类展示'}
          </div>
        </div>
      )}

      <div className="qvm-form-divider">{`${type === 'linux' ? 'Linux' : type === 'windows' ? 'Windows' : type === 'fnos' ? 'FnOS' : type === 'openwrt' ? 'OpenWrt' : '其它'} 模板配置`}</div>

      {isOtherType ? (
        <Banner
          type="warning"
          closeIcon={null}
          description="其它模板克隆时可选择链式或完整克隆，但系统不会初始化，也不会修改模板内的主机名、用户、密码或网络配置。"
        />
      ) : (
        <div className="qvm-form-item">
          <div className="qvm-form-label">初始化方式</div>
          <Radio.Group
            value={initMode}
            onChange={(e) => void handleInitModeChange(e.target.value as string)}
          >
            {initOptions.map((option) => (
              <Radio key={option.value} value={option.value}>
                {option.label}
              </Radio>
            ))}
          </Radio.Group>
          <div className="qvm-form-tip">{initTip}</div>
        </div>
      )}

      {type === 'linux' && initMode !== 'none' && (
        <>
          <div className="qvm-form-item">
            <div className="qvm-form-label">模板用户名</div>
            <Input value={templateUser} onChange={setTemplateUser} placeholder="模板中已有的普通用户名" />
            <div className="qvm-form-tip">克隆时若目标用户名与模板用户名不同，自动离线重命名</div>
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label">启动后命令</div>
            <TextArea
              value={postBootCommand}
              onChange={setPostBootCommand}
              rows={3}
              placeholder="克隆后首次启动时执行的自定义 Shell 命令（可多行）"
            />
            <div className="qvm-form-tip">命令将以 root 权限执行，仅首次启动时运行</div>
            <Checkbox
              checked={postBootBlocking}
              disabled={!postBootCommand}
              onChange={(e) => setPostBootBlocking(!!e.target.checked)}
              style={{ marginTop: 6 }}
            >
              等待命令执行完毕后再启动 SSH
            </Checkbox>
            {postBootBlocking && (
              <div className="qvm-form-tip warn">
                启用后系统启动期间将显示「正在启动 QVM 初始化服务」，用户在此期间无法通过 SSH 登录
              </div>
            )}
          </div>
        </>
      )}
    </Modal>
  )
}
