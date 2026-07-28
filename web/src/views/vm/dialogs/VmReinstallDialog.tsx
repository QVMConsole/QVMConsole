/**
 * 重装系统弹窗
 * 迁移自旧前端 VmReinstallDialog.vue
 */
import { useEffect, useMemo, useState } from 'react'
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Toast,
} from '@douyinfe/semi-ui'
import type { VmListItem } from '@/api/vm'
import { reinstallVm } from '@/api/vm'
import { getTemplateList, type TemplateItem } from '@/api/template'
import {
  checkPasswordBreachAsync,
  generatePassword,
  validatePassword,
  STRONG_PASSWORD_MIN_LENGTH,
  PASSWORD_ALLOWED_PATTERN,
} from '@/utils/validate'
import { parseDiskSizeGB, resolveTemplateMinDiskSize } from '../utils'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface VmReinstallDialogProps {
  vm: VmListItem
  onClose: () => void
  onSuccess: () => void
}

const WINDOWS_USERNAME = 'administrator'
const USERNAME_PATTERN = /^[a-z_][a-z0-9_-]{0,31}$/
const FNOS_DEVICE_ID_PATTERN = /^[0-9a-fA-F]{32}([0-9a-fA-F]{8})?$/

export default function VmReinstallDialog({ vm, onClose, onSuccess }: VmReinstallDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [templates, setTemplates] = useState<TemplateItem[]>([])
  const [templateLoading, setTemplateLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const [template, setTemplate] = useState('')
  const [diskSize, setDiskSize] = useState<number>(parseDiskSizeGB(vm.disk_size) || 0)
  const [hostname, setHostname] = useState(vm.name || '')
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [fnosDeviceIdMode, setFnosDeviceIdMode] = useState<'regenerate' | 'preserve' | 'custom'>(
    'regenerate',
  )
  const [fnosDeviceId, setFnosDeviceId] = useState('')

  const currentDiskSize = useMemo(() => parseDiskSizeGB(vm.disk_size), [vm.disk_size])
  const selectedTemplate = useMemo(
    () => templates.find((item) => item.name === template) || null,
    [templates, template],
  )
  const templateType = (selectedTemplate?.type || '').trim().toLowerCase()
  const templateMinDiskSize = resolveTemplateMinDiskSize(selectedTemplate)
  const showCredentialFields = ['linux', 'windows', 'fnos'].includes(templateType)
  const isFnos = templateType === 'fnos'
  const isWindows = templateType === 'windows'

  // 加载模板列表并预选当前模板
  useEffect(() => {
    let cancelled = false
    setTemplateLoading(true)
    getTemplateList()
      .then((res) => {
        if (cancelled) return
        const items = Array.isArray(res.data) ? res.data : []
        setTemplates(items)
        const currentTemplate = (vm.template || '').trim()
        if (currentTemplate && items.some((item) => item.name === currentTemplate)) {
          setTemplate(currentTemplate)
        }
      })
      .catch((err) => console.error('获取模板列表失败', err))
      .finally(() => {
        if (!cancelled) setTemplateLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [vm.template])

  // 切换模板时应用默认值
  useEffect(() => {
    if (isWindows) {
      setUser(WINDOWS_USERNAME)
    } else if (selectedTemplate?.template_user) {
      setUser(selectedTemplate.template_user)
    }
    if (templateMinDiskSize > 0 && diskSize < templateMinDiskSize) {
      setDiskSize(templateMinDiskSize)
    }
    if (!isFnos) {
      setFnosDeviceIdMode('regenerate')
      setFnosDeviceId('')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [template, templates])

  const validate = (): boolean => {
    if (!template) {
      Toast.warning('请选择要重装的模板')
      return false
    }
    if (!diskSize || diskSize <= 0) {
      Toast.warning('请设置系统盘大小')
      return false
    }
    if (templateMinDiskSize > 0 && diskSize < templateMinDiskSize) {
      Toast.warning(`系统盘大小不能小于 ${templateMinDiskSize} GB`)
      return false
    }
    if (showCredentialFields) {
      if (!hostname.trim()) {
        Toast.warning('请输入主机名')
        return false
      }
      if (!isWindows && !USERNAME_PATTERN.test(user.trim())) {
        Toast.warning('用户名仅支持小写字母、数字、下划线和短横线，且需以字母或下划线开头')
        return false
      }
      if (!password) {
        Toast.warning('请输入登录密码')
        return false
      }
      if (password.length < STRONG_PASSWORD_MIN_LENGTH || !PASSWORD_ALLOWED_PATTERN.test(password)) {
        Toast.warning(`密码至少 ${STRONG_PASSWORD_MIN_LENGTH} 位，只支持字母、数字和 !@#$%^&*_-+=? 符号`)
        return false
      }
      if (!validatePassword(password).valid) {
        Toast.warning('该密码过于常见，请更换为更安全的密码')
        return false
      }
    }
    if (isFnos && fnosDeviceIdMode === 'custom') {
      if (!FNOS_DEVICE_ID_PATTERN.test(fnosDeviceId.trim())) {
        Toast.warning('设备 ID 只能为 32 位或 40 位十六进制字符串')
        return false
      }
    }
    return true
  }

  const handleSubmit = async () => {
    if (!validate()) return
    // 异步泄露密码检测（HIBP）
    if (password) {
      const breach = await checkPasswordBreachAsync(password)
      if (breach.enabled && breach.breached) {
        Toast.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
        return
      }
    }
    setSubmitting(true)
    try {
      await reinstallVm(vm.name, {
        template,
        disk_size: Number(diskSize || 0),
        hostname: hostname.trim(),
        user: isWindows ? WINDOWS_USERNAME : user.trim(),
        password,
        preserve_fnos_device_id: isFnos && fnosDeviceIdMode !== 'regenerate',
        fnos_device_id: isFnos && fnosDeviceIdMode === 'custom' ? fnosDeviceId.trim() : '',
      })
      Toast.success('重装任务已提交，请在任务中心查看进度')
      requestClose()
      onSuccess()
    } catch {
      // 错误提示由请求层统一处理
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="重装系统"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="提交重装任务"
      cancelText="取消"
      okButtonProps={{ type: 'danger' }}
      confirmLoading={submitting}
      width={640}
      closeOnEsc
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 16 }}
        description="重装会替换当前系统盘并自动删除全部快照，CPU、内存、网络和额外数据盘会保留，虚拟机会在任务开始时自动关机。"
      />

      <div className="qvm-form-item">
        <div className="qvm-form-label">虚拟机名称</div>
        <Input value={vm.name} disabled />
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">模板</div>
        <Select
          style={{ width: '100%' }}
          value={template || undefined}
          onChange={(value) => setTemplate(value as string)}
          filter
          showClear
          loading={templateLoading}
          placeholder="请选择要重装的模板"
          optionList={templates.map((item) => ({
            label: item.display_name || item.admin_name || item.name,
            value: item.name,
          }))}
        />
        {selectedTemplate && (
          <div className="qvm-form-tip">
            {selectedTemplate.type || 'linux'} / 最低 {templateMinDiskSize || '-'} GB
          </div>
        )}
      </div>
      <div className="qvm-form-item">
        <div className="qvm-form-label required">系统盘大小</div>
        <InputNumber
          style={{ width: '100%' }}
          value={diskSize}
          onChange={(value) => setDiskSize(Number(value || 0))}
          min={templateMinDiskSize || 1}
          max={8192}
          step={10}
          suffix="GB"
        />
        <div className="qvm-form-tip">
          默认值为当前系统盘 {currentDiskSize || '-'} GB；最低不能小于模板原始磁盘{' '}
          {templateMinDiskSize || '-'} GB。
        </div>
      </div>

      {showCredentialFields && (
        <>
          <div className="qvm-form-item">
            <div className="qvm-form-label required">主机名</div>
            <Input value={hostname} onChange={setHostname} placeholder="请输入重装后的主机名" />
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label required">登录用户名</div>
            <Input
              value={user}
              onChange={setUser}
              disabled={isWindows}
              placeholder={isWindows ? WINDOWS_USERNAME : '请输入登录用户名'}
            />
            <div className="qvm-form-tip">
              {isWindows
                ? 'Windows 模板固定使用 administrator。'
                : isFnos
                  ? 'FnOS 会把该账号写入为首次管理员账号。'
                  : '仅支持小写字母、数字、下划线和短横线，且需以字母或下划线开头。'}
            </div>
          </div>
          <div className="qvm-form-item">
            <div className="qvm-form-label required">登录密码</div>
            <Input
              mode="password"
              value={password}
              onChange={setPassword}
              placeholder="请输入强密码"
            />
            <div className="qvm-form-tip-row">
              <span className="qvm-form-tip">至少 {STRONG_PASSWORD_MIN_LENGTH} 位（支持 !@#$%^&*_-+=?）</span>
              <Button
                size="small"
                theme="borderless"
                type="primary"
                onClick={() => setPassword(generatePassword())}
              >
                随机强密码
              </Button>
            </div>
          </div>
        </>
      )}

      {isFnos && (
        <div className="qvm-form-item">
          <div className="qvm-form-label">FnOS 设备 ID</div>
          <Radio.Group
            value={fnosDeviceIdMode}
            onChange={(e) => setFnosDeviceIdMode(e.target.value as typeof fnosDeviceIdMode)}
          >
            <Radio value="regenerate">重新生成</Radio>
            <Radio value="preserve">保留模板设备 ID</Radio>
            <Radio value="custom">自定义</Radio>
          </Radio.Group>
        </div>
      )}
      {isFnos && fnosDeviceIdMode === 'custom' && (
        <div className="qvm-form-item">
          <div className="qvm-form-label required">自定义设备 ID</div>
          <Input
            value={fnosDeviceId}
            onChange={setFnosDeviceId}
            placeholder="请输入 32 位或 40 位十六进制设备 ID"
          />
        </div>
      )}
    </Modal>
  )
}
