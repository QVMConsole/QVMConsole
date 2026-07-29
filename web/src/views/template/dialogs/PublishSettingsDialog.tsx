/**
 * 模板发布设置弹窗：名称/分类/可见性/默认创建配置/启动后命令
 * 迁移自旧前端 views/template/index.vue 的发布设置对话框
 */
import { useMemo, useState } from 'react'
import {
  Button,
  Checkbox,
  Input,
  InputNumber,
  Modal,
  Select,
  Switch,
  TextArea,
  Toast,
} from '@douyinfe/semi-ui'
import { updateTemplatePublish, type TemplateItem } from '@/api/template'
import {
  LINUX_TEMPLATE_CATEGORY_OPTIONS,
  WINDOWS_TEMPLATE_CATEGORY_OPTIONS,
  normalizeTemplateCategory,
} from '@/utils/templateCategory'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface PublishSettingsDialogProps {
  node: TemplateItem
  onClose: () => void
  /** 保存成功后回调（用于刷新列表） */
  onSaved: () => void
}

const DISK_BUS_OPTIONS = [
  { label: 'VirtIO', value: 'virtio' },
  { label: 'SCSI', value: 'scsi' },
  { label: 'SATA', value: 'sata' },
  { label: 'IDE', value: 'ide' },
]

const NIC_MODEL_OPTIONS = [
  { label: 'VirtIO', value: 'virtio' },
  { label: 'e1000e (Intel)', value: 'e1000e' },
  { label: 'rtl8139', value: 'rtl8139' },
]

const VIDEO_MODEL_OPTIONS = [
  { label: 'VirtIO（高性能）', value: 'virtio' },
  { label: 'VGA（兼容模式）', value: 'vga' },
  { label: 'VMVGA（VMware 嵌套）', value: 'vmvga' },
  { label: 'Cirrus（保守排障）', value: 'cirrus' },
  { label: 'None（禁用虚拟显示）', value: 'none' },
]

const CPU_TOPOLOGY_OPTIONS = [
  { label: '自动（Windows 使用单插槽多核心）', value: 'auto' },
  { label: '单插槽多核心', value: 'single_socket' },
  { label: '宿主默认拓扑', value: 'host_default' },
]

const FIRST_BOOT_REBOOT_OPTIONS = [
  { label: '普通重启', value: 'normal' },
  { label: '宿主冷启动', value: 'cold' },
]

export default function PublishSettingsDialog({ node, onClose, onSaved }: PublishSettingsDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const defaults = node.default_config || {}
  const nodeType = node.type || ''

  const [adminName, setAdminName] = useState(node.admin_name || node.name)
  const [displayName, setDisplayName] = useState(
    node.display_name || node.admin_name || node.name,
  )
  const [category, setCategory] = useState(normalizeTemplateCategory(nodeType, node.category))
  const [cloneVisible, setCloneVisible] = useState(!!node.clone_visible)
  const [disabled, setDisabled] = useState(!!node.disabled)
  const [vcpu, setVcpu] = useState(Number(defaults.vcpu || 0))
  const [ram, setRam] = useState(Number(defaults.ram || 0))
  const [diskSize, setDiskSize] = useState(Number(defaults.disk_size || 0))
  const [diskBus, setDiskBus] = useState(defaults.disk_bus || '')
  const [nicModel, setNicModel] = useState(defaults.nic_model || '')
  const [videoModel, setVideoModel] = useState(defaults.video_model || '')
  const [cpuTopologyMode, setCpuTopologyMode] = useState(defaults.cpu_topology_mode || '')
  const [firstBootRebootMode, setFirstBootRebootMode] = useState(
    defaults.first_boot_reboot_mode || '',
  )
  const [postBootCommand, setPostBootCommand] = useState(node.post_boot_command || '')
  const [postBootBlocking, setPostBootBlocking] = useState(!!node.post_boot_blocking)
  const [saving, setSaving] = useState(false)

  const showCategory = nodeType === 'linux' || nodeType === 'windows'
  const categoryOptions = useMemo(
    () => (nodeType === 'windows' ? WINDOWS_TEMPLATE_CATEGORY_OPTIONS : LINUX_TEMPLATE_CATEGORY_OPTIONS),
    [nodeType],
  )
  const categoryTip =
    nodeType === 'windows'
      ? 'Windows 模板按版本分类展示，2012 R2 会保留模板默认硬件配置用于克隆'
      : 'Linux 模板按发行版分类展示'

  const handleSave = async () => {
    if (!adminName.trim() || !displayName.trim()) {
      Toast.warning('请填写管理员名称和用户侧显示')
      return
    }
    setSaving(true)
    try {
      await updateTemplatePublish(node.name, {
        admin_name: adminName.trim(),
        display_name: displayName.trim(),
        clone_visible: cloneVisible,
        disabled,
        category: showCategory ? normalizeTemplateCategory(nodeType, category) : '',
        vcpu: Number(vcpu || 0),
        ram: Number(ram || 0),
        disk_size: Number(diskSize || 0),
        disk_bus: diskBus || '',
        nic_model: nicModel || '',
        video_model: videoModel || '',
        cpu_topology_mode: cpuTopologyMode || '',
        first_boot_reboot_mode: firstBootRebootMode || '',
        post_boot_command: postBootCommand || '',
        post_boot_blocking: postBootBlocking || false,
      })
      Toast.success('模板发布设置已保存')
      onSaved()
      requestClose()
    } catch (err) {
      console.error('保存模板发布设置失败', err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title="发布设置"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={560}
      maskClosable={false}
      footer={
        <>
          <Button onClick={requestClose} disabled={saving}>
            取消
          </Button>
          <Button type="primary" loading={saving} onClick={() => void handleSave()}>
            保存
          </Button>
        </>
      }
    >
      <div className="qvm-form-item">
        <div className="qvm-form-label">模板文件</div>
        <Input value={node.name} disabled />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">管理员名称</div>
        <Input value={adminName} onChange={setAdminName} placeholder="管理员侧名称" />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">用户侧显示</div>
        <Input
          value={displayName}
          onChange={setDisplayName}
          placeholder="从模板克隆下拉框中显示的标题"
        />
      </div>

      {showCategory && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">
            {nodeType === 'windows' ? 'Windows 分类' : 'Linux 分类'}
          </div>
          <Select
            style={{ width: '100%' }}
            value={category}
            onChange={(v) => setCategory(v as string)}
            filter
            optionList={categoryOptions.map((item) => ({ label: item, value: item }))}
          />
          <div className="qvm-form-tip">{categoryTip}</div>
        </div>
      )}

      <div className="qvm-form-item tpl-switch-row">
        <div>
          <div className="qvm-form-label">启用克隆</div>
          <div className="qvm-form-tip">用户可见时普通用户可从此模板克隆</div>
        </div>
        <div className="tpl-switch-control">
          <Switch
            checked={cloneVisible}
            onChange={(v) => setCloneVisible(v)}
            disabled={disabled}
            checkedText="公"
            uncheckedText="私"
          />
        </div>
      </div>
      <div className="qvm-form-item tpl-switch-row">
        <div>
          <div className="qvm-form-label">禁用模板</div>
          <div className="qvm-form-tip">禁用后管理员新建虚拟机下拉框也不会显示该模板</div>
        </div>
        <div className="tpl-switch-control">
          <Switch checked={disabled} onChange={(v) => setDisabled(v)} checkedText="禁" uncheckedText="用" />
        </div>
      </div>

      <div className="qvm-form-divider">默认创建配置</div>

      <div className="tpl-form-grid">
        <div className="qvm-form-item">
          <div className="qvm-form-label">CPU 核心</div>
          <InputNumber
            style={{ width: '100%' }}
            value={vcpu}
            onChange={(v) => setVcpu(Number(v || 0))}
            min={0}
            max={128}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">内存(GB)</div>
          <InputNumber
            style={{ width: '100%' }}
            value={ram}
            onChange={(v) => setRam(Number(v || 0))}
            min={0}
            max={1024}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">磁盘(GB)</div>
          <InputNumber
            style={{ width: '100%' }}
            value={diskSize}
            onChange={(v) => setDiskSize(Number(v || 0))}
            min={0}
            max={8192}
          />
        </div>
        <div className="qvm-form-item">
          <div className="qvm-form-label">磁盘驱动</div>
          <Select
            style={{ width: '100%' }}
            value={diskBus || undefined}
            onChange={(v) => setDiskBus((v as string) || '')}
            optionList={DISK_BUS_OPTIONS}
            placeholder="未设置"
            showClear
          />
        </div>
      </div>

      <div className="qvm-form-item">
        <div className="qvm-form-label">网卡类型</div>
        <Select
          style={{ width: '100%' }}
          value={nicModel || undefined}
          onChange={(v) => setNicModel((v as string) || '')}
          optionList={NIC_MODEL_OPTIONS}
          placeholder="未设置"
          showClear
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">显示设备</div>
        <Select
          style={{ width: '100%' }}
          value={videoModel || undefined}
          onChange={(v) => setVideoModel((v as string) || '')}
          optionList={VIDEO_MODEL_OPTIONS}
          placeholder="未设置"
          showClear
        />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">CPU 拓扑</div>
        <Select
          style={{ width: '100%' }}
          value={cpuTopologyMode || undefined}
          onChange={(v) => setCpuTopologyMode((v as string) || '')}
          optionList={CPU_TOPOLOGY_OPTIONS}
          placeholder="未设置"
          showClear
        />
        <div className="qvm-form-tip">
          填写后，新建 VM 选择该模板时会自动带出这些默认值；填 0 或留空表示不指定
        </div>
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label">首次重启</div>
        <Select
          style={{ width: '100%' }}
          value={firstBootRebootMode || undefined}
          onChange={(v) => setFirstBootRebootMode((v as string) || '')}
          optionList={FIRST_BOOT_REBOOT_OPTIONS}
          placeholder="未设置"
          showClear
        />
        <div className="qvm-form-tip">Windows 模板 OOBE 自动重启黑屏时可设置为宿主冷启动</div>
      </div>

      {nodeType === 'linux' && (
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
      )}
    </Modal>
  )
}
