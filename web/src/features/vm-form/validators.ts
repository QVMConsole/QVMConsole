/**
 * 虚拟机表单校验规则（创建 / 编辑共用）
 * 所有函数返回错误消息字符串，合法返回空字符串。
 */
import {
  FNOS_DEVICE_ID_PATTERN,
  HOSTNAME_PATTERN,
  TEMPLATE_USERNAME_PATTERN,
  VM_NAME_PATTERN,
  WINDOWS_TEMPLATE_USERNAME,
} from './constants'
import {
  PASSWORD_ALLOWED_PATTERN,
  STRONG_PASSWORD_MIN_LENGTH,
  validatePassword,
} from '@/utils/validate'
import type { VmFormModel } from './types'

/** 校验上下文（创建模式分支判断） */
export interface ValidateContext {
  isAdmin: boolean
  hostArch: string
  isTemplateMode: boolean
  isWindowsTemplate: boolean
  isFnOSTemplate: boolean
  isOpenWrtTemplate: boolean
  isNoInitTemplate: boolean
  disableSystemInit: boolean
  registrationMode: boolean
  templateMinDiskSize: number
}

/** 虚拟机名称 */
export function validateVmName(name: string): string {
  if (!name) return '请输入虚拟机名称'
  if (!VM_NAME_PATTERN.test(name)) {
    return '虚拟机名称只能包含字母、数字和短横线，且不能以短横线开头或结尾'
  }
  return ''
}

/** 主机名（模板/导入初始化，可空） */
export function validateHostname(hostname: string): string {
  if (!hostname) return ''
  if (!HOSTNAME_PATTERN.test(hostname)) {
    return '主机名只能包含字母、数字和短横线，且不能以短横线开头或结尾'
  }
  return ''
}

/** 登录用户名（模板初始化） */
export function validateTemplateUsername(user: string, isWindowsTemplate: boolean): string {
  const normalized = String(user || '').trim()
  if (isWindowsTemplate) {
    if (normalized !== WINDOWS_TEMPLATE_USERNAME) {
      return 'Windows 模板用户名固定为 administrator，不支持修改'
    }
    return ''
  }
  if (!normalized) return '请输入用户名'
  if (!TEMPLATE_USERNAME_PATTERN.test(normalized)) {
    return '用户名只能以小写字母或下划线开头，且只能包含小写字母、数字、下划线和短横线'
  }
  return ''
}

/** 登录密码（模板初始化；批量创建可留空自动生成） */
export function validateTemplatePassword(password: string, batchCount: number): string {
  if (!password) {
    if (batchCount > 1) return ''
    return '请输入密码'
  }
  if (
    password.length < STRONG_PASSWORD_MIN_LENGTH ||
    !PASSWORD_ALLOWED_PATTERN.test(password)
  ) {
    return `密码至少 ${STRONG_PASSWORD_MIN_LENGTH} 位，只支持字母、数字和 !@#$%^&*_-+=? 符号`
  }
  const local = validatePassword(password)
  if (!local.valid) return local.message
  return ''
}

/** FnOS 设备 ID（custom 模式） */
export function validateFnosDeviceId(deviceId: string): string {
  if (!FNOS_DEVICE_ID_PATTERN.test(deviceId || '')) {
    return '请输入 32 位或 40 位十六进制设备 ID'
  }
  return ''
}

/** 磁盘大小（模板模式不能小于模板磁盘） */
export function validateTemplateDiskSize(
  diskSize: number,
  templateSelected: boolean,
  templateMinDiskSize: number,
): string {
  if (!templateSelected) return ''
  if (!diskSize || diskSize <= 0) {
    return '请选择模板后使用默认磁盘大小，或填写更大的磁盘'
  }
  if (templateMinDiskSize > 0 && diskSize < templateMinDiskSize) {
    return `磁盘大小不能小于模板磁盘大小 ${templateMinDiskSize} GB`
  }
  return ''
}

/** OpenWrt 静态 IP（CIDR 格式） */
export function validateStaticIp(ip: string): string {
  if (!ip) return '请输入静态 IP'
  if (!/^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/.test(ip.trim())) {
    return '静态 IP 格式应为 IP/子网掩码，如 192.168.1.100/24'
  }
  return ''
}

/**
 * 创建向导按步骤校验，返回首个错误消息（空串表示通过）。
 * step 与向导步骤名一一对应。
 */
export function validateCreateStep(
  step: string,
  form: VmFormModel,
  ctx: ValidateContext,
): string {
  if (step === 'basic') {
    const nameErr = validateVmName(form.name)
    if (nameErr) return nameErr
    if (ctx.isTemplateMode) {
      if (!form.template) return '请选择模板'
      const diskErr = validateTemplateDiskSize(form.disk_size, true, ctx.templateMinDiskSize)
      if (diskErr) return diskErr
      if (!ctx.disableSystemInit || ctx.registrationMode) {
        const hostErr = validateHostname(form.hostname)
        if (hostErr) return hostErr
      }
      if (!ctx.registrationMode && !ctx.disableSystemInit && !ctx.isNoInitTemplate && !ctx.isOpenWrtTemplate) {
        const userErr = validateTemplateUsername(form.import_user, ctx.isWindowsTemplate)
        if (userErr) return userErr
        const passErr = validateTemplatePassword(form.import_password, form.batch_count)
        if (passErr) return passErr
      }
      if (ctx.isOpenWrtTemplate && !ctx.disableSystemInit) {
        const ipErr = validateStaticIp(form.static_ip)
        if (ipErr) return ipErr
      }
      if (ctx.isFnOSTemplate && !ctx.disableSystemInit && form.fnos_device_id_mode === 'custom') {
        const idErr = validateFnosDeviceId(form.fnos_device_id)
        if (idErr) return idErr
      }
    }
    return ''
  }
  if (step === 'appliance') {
    if (ctx.isAdmin && form.appliance_source_type === 'path') {
      if (!form.appliance_path) return '请输入 OVA/OVF 文件绝对路径'
    } else if (!form.appliance_file) {
      return '请选择 OVA/OVF 虚拟机包'
    }
    return ''
  }
  if (step === 'hardware') {
    if (!form.vcpu || form.vcpu <= 0) return '请设置 CPU 核心数'
    if (!form.ram || form.ram <= 0) return '请设置内存大小'
    return ''
  }
  if (step === 'storage') {
    if (form.create_mode === 'iso') {
      if (!form.disk_size || form.disk_size <= 0) return '请设置磁盘大小'
    } else if (form.create_mode === 'import') {
      if (ctx.isAdmin && form.disk_source_type === 'path') {
        if (!form.disk_path) return '请输入磁盘文件绝对路径'
      } else if (!form.disk_file) {
        return '请选择磁盘文件'
      }
    }
    return ''
  }
  return ''
}

/**
 * 提交按钮可用性汇总（未满足时返回缺失项列表）。
 */
export function collectMissingRequired(form: VmFormModel, ctx: ValidateContext): string[] {
  const missing: string[] = []
  if (!VM_NAME_PATTERN.test(form.name)) missing.push('虚拟机名称')
  if (!form.vcpu || form.vcpu <= 0) missing.push('CPU 核心')
  if (!form.ram || form.ram <= 0) missing.push('内存')
  const taskMapsApplianceNetwork =
    form.create_mode === 'appliance' && form.appliance_config_mode === 'ovf'
  if (
    !ctx.isAdmin &&
    !form.security_group_id &&
    !ctx.registrationMode &&
    !taskMapsApplianceNetwork
  ) {
    missing.push('安全组')
  }
  if (form.create_mode === 'iso') {
    if (!form.disk_size || form.disk_size <= 0) missing.push('系统盘大小')
  } else if (form.create_mode === 'import') {
    if (ctx.isAdmin && form.disk_source_type === 'path') {
      if (!form.disk_path) missing.push('磁盘路径')
    } else if (!form.disk_file) {
      missing.push('磁盘文件')
    }
  } else if (form.create_mode === 'appliance') {
    if (ctx.isAdmin && form.appliance_source_type === 'path') {
      if (!form.appliance_path) missing.push('虚拟机包路径')
    } else if (!form.appliance_file) {
      missing.push('虚拟机包')
    }
  } else if (ctx.isTemplateMode) {
    if (!form.template) missing.push('模板')
    if (!form.disk_size || form.disk_size <= 0) missing.push('磁盘大小')
    if (ctx.templateMinDiskSize > 0 && form.disk_size < ctx.templateMinDiskSize) {
      missing.push(`磁盘大小不能小于 ${ctx.templateMinDiskSize} GB`)
    }
    if (!ctx.registrationMode && !ctx.disableSystemInit && !ctx.isNoInitTemplate && !ctx.isOpenWrtTemplate) {
      if (!form.import_user) missing.push('用户名')
      if (!form.import_password && form.batch_count <= 1) missing.push('密码')
    }
    if (ctx.isOpenWrtTemplate && !ctx.disableSystemInit && !form.static_ip) {
      missing.push('静态 IP')
    }
    if (
      ctx.isFnOSTemplate &&
      !ctx.disableSystemInit &&
      form.fnos_device_id_mode === 'custom' &&
      !FNOS_DEVICE_ID_PATTERN.test(form.fnos_device_id || '')
    ) {
      missing.push('FnOS 设备 ID')
    }
  }
  return missing
}
