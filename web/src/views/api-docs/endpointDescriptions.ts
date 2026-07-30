/**
 * API 文档补充描述
 * 接口清单本体由 scripts/generate-api-endpoints.mjs 从后端源码自动生成（endpoints.json），
 * 本文件仅维护自动解析拿不到的中文摘要、请求体/返回说明与备注，按「METHOD 路径」索引。
 * 后端新增接口时无需改动本文件也会出现在文档页（显示自动推断摘要），有空再补充文案即可。
 * 权限类信息（管理员/轻量云限制/VM 归属/二次验证）由生成数据自动提供，请勿在 notes 中重复。
 */

export interface EndpointDescription {
  /** 接口中文摘要 */
  summary?: string
  /** 请求体说明 */
  body?: string
  /** 返回说明 */
  response?: string
  /** 查询参数 */
  query?: string[]
  /** 备注 */
  notes?: string[]
  /** 必填字段（用于请求参数表标注） */
  requiredFields?: string[]
  /** 高风险触发条件的补充说明（触发标识由后端源码自动解析） */
  highRiskNote?: string
}

/** 模块分组：按路径前缀归类（新增后端路由组时在此补充，否则归入「其他」） */
export const moduleGroups: { name: string; description: string; prefixes: string[] }[] = [
  { name: '公开接口', description: '无需登录，通常用于登录页初始化展示。', prefixes: ['/public'] },
  { name: '认证与账户安全', description: '登录、邀请、找回密码、安全验证和 API Key 管理。', prefixes: ['/auth'] },
  { name: '系统设置', description: '系统级配置、密码泄露扫描与处置，管理员使用；SMTP 设置在安全初始化阶段可使用 bootstrap token。', prefixes: ['/settings', '/security'] },
  { name: '虚拟机', description: 'VM 生命周期、详情、监控、网络绑定、调度、磁盘、VNC/SPICE、快照和救援。', prefixes: ['/vm'] },
  { name: '模板', description: '模板制作、导入导出、发布和删除。', prefixes: ['/template'] },
  { name: '网络', description: '静态 IP、端口转发、宿主机网桥、公网 IP 和抓包。', prefixes: ['/network'] },
  { name: 'VPC', description: 'VPC 交换机、安全组和 ACL。', prefixes: ['/vpc'] },
  { name: '防火墙', description: 'KVM 全局防火墙和宿主机防火墙，均为管理员接口。', prefixes: ['/firewall'] },
  { name: 'OVS', description: 'OVS 网络诊断，管理员接口。', prefixes: ['/ovs'] },
  { name: '存储池', description: '宿主机存储池、ISO 聚合和 VM 存储目标。', prefixes: ['/storage-pool'] },
  { name: '节点管理', description: '管理员维护跨节点迁移目标节点，并由目标面板接管迁移后的 VM。', prefixes: ['/nodes', '/migration'] },
  { name: '用户管理', description: '管理员管理用户、配额、轻量云登记和 SSH。', prefixes: ['/user'] },
  { name: '用户自助与我的存储', description: '普通用户查询配额、管理自己的 VM 和存储。', prefixes: ['/self'] },
  { name: '宿主机', description: '宿主机监控和宿主机级 KVM/KSM/zRAM/硬件直通参数。', prefixes: ['/host'] },
  { name: '任务与调度', description: '任务队列、任务 SSE 和调度器事件。', prefixes: ['/task', '/scheduler'] },
]

/** 兜底分组名 */
export const fallbackGroupName = '其他'
export const fallbackGroupDescription = '未归入以上模块的通用接口。'

// 复用的请求体描述
const vmCreateBody =
  'JSON: name, remark, vcpu, ram, disk_size, disk_format, disk_bus, os_variant, iso_path, iso_paths[], nic_model, autostart, freeze, apic, pae, rtc_offset, rtc_startdate, guest_agent{enabled}, smbios1{base64,family,manufacturer,product,serial,sku,uuid,version}, os_type, machine_type, boot_type, watchdog, boot_order[], video_model(virtio/vga/vmvga/cirrus/ramfb/none，none=禁用虚拟显示), spice_enabled(bool,是否启用SPICE显示协议,不传=回退全局默认), cpu_topology_mode(auto/single_socket/host_default), cpu_limit_percent(仅管理员, 0-100), virt_type(kvm/qemu), arch(x86_64/aarch64/riscv64), memory_dynamic{dynamic_enabled,memory_backend,memory_initial,memory_min,memory_max,memory_auto_balloon,memory_current}, switch_id, security_group_id, storage_pool_id, extra_disks[{size,format,bus,storage_pool_id}], host_devices[{pci_address}](仅管理员)'
const selfVmCreateBody =
  'JSON: name, remark, vcpu, ram, disk_size, disk_format, disk_bus, os_variant, iso_path, iso_paths[], nic_model, autostart, freeze, apic, pae, rtc_offset, rtc_startdate, guest_agent{enabled}, smbios1{base64,family,manufacturer,product,serial,sku,uuid,version}, os_type, machine_type, boot_type, boot_order[], video_model(virtio/vga/vmvga/cirrus/ramfb/none，none=禁用虚拟显示), spice_enabled(bool,是否启用SPICE显示协议,不传=回退全局默认), cpu_topology_mode(auto/single_socket/host_default), memory_dynamic{dynamic_enabled,memory_backend,memory_initial,memory_min,memory_max,memory_auto_balloon,memory_current}, switch_id, security_group_id, storage_pool_id, extra_disks[{size,format,bus,storage_pool_id}]'
const cloneBody =
  'JSON: template/name, new_name/name, remark, vcpu, ram, disk_size, disk_bus, switch_id, security_group_id, storage_pool_id, extra_disks[{size,format,bus,storage_pool_id}], host_devices[{pci_address}](仅管理员), nic_model, video_model(支持none禁用虚拟显示), spice_enabled(bool,是否启用SPICE显示协议,不传=回退全局默认), cpu_topology_mode, cpu_limit_percent(仅管理员, 0-100), first_boot_reboot_mode(normal/cold), preserve_fnos_device_id/fnos_device_id(FnOS 可选), autostart, freeze, apic, pae, rtc_offset, credentials, kvm_hidden(bool), vendor_id(str), nested_virt(bool,默认true) 等克隆表单字段'
const reinstallBody = 'JSON: template, disk_size, hostname, user, password, preserve_fnos_device_id, fnos_device_id'
const scheduleBody = 'JSON: name, action(start/shutdown/destroy/reboot/delete), cron/execute_at, enabled, timezone, params'
const portForwardBody =
  'JSON: vm_name, guest_ip, guest_port, host_port, protocol(tcp/udp), description, target_type, public_ip_id'
const publicIPBody = 'JSON: address, cidr, gateway, iface, mac, vm_name, mode, remark, enabled 等公网 IP 配置字段'
const firewallPolicyBody =
  'JSON: policy 或完整防火墙策略对象，包含 default_action, rules, region_rules, port_forward_policy 等'
const vpcSwitchBody =
  'JSON: name, cidr, gateway, dhcp_start, dhcp_end, vlan_id, dns, remark, username, bridge_mode, host_interface'
const securityGroupBody = 'JSON: name, remark, username'
const securityRuleBody =
  'JSON: direction, protocol, port_start, port_end, target_type(cidr/switch/security_group), target_value, action, remark'
const templateMetaBody =
  'JSON: admin_name, display_name, clone_visible, disabled, category, vcpu, ram, disk_size, disk_bus, nic_model, video_model, cpu_topology_mode, first_boot_reboot_mode'
const hostRuleBody = 'JSON: name, direction, protocol, port, source, action, enabled'
const migrationBody =
  'JSON: node_id, preview_id(可选), skip_precheck, target_storage_pool_id, disk_storage_targets[{target,target_storage_pool_id}], target_switch_id, target_security_group_id, enable_cpu_throttle, cpu_throttle_percent'

export const endpointDescriptions: Record<string, EndpointDescription> = {
  // ==================== 公开接口 ====================
  'GET /public/settings': {
    summary: '读取公开站点设置',
    response: 'data: site_title, login_background, development_mode 等公开配置。',
  },
  'GET /public/version': {
    summary: '读取面板版本信息',
    response: 'data: version, build_time, site_title。',
  },

  // ==================== 认证与账户安全 ====================
  'POST /auth/login': {
    summary: '登录并进入 success/login_verify/bootstrap_security 阶段',
    body: 'JSON: username, password',
    response: 'data: stage, token, username, role, cloud_type, security, allowed_methods。',
    requiredFields: ['username', 'password'],
  },
  'GET /auth/invite': {
    summary: '读取邀请注册信息',
    query: ['token'],
    response: 'data: 邀请账号、邮箱、角色、过期状态。',
  },
  'POST /auth/invite/complete': {
    summary: '完成邀请注册',
    body: 'JSON: token, password, confirm_password',
    response: 'data: stage, token, username, role, cloud_type, security。',
  },
  'POST /auth/password/forgot': {
    summary: '发送旧版找回密码邮件链接',
    body: 'JSON: email',
    response: '统一成功提示，避免枚举账号。',
  },
  'POST /auth/password/forgot/send-code': {
    summary: '发送忘记密码邮箱验证码',
    body: 'JSON: email',
    response: 'data: challenge_id, masked_email, expires_in。',
  },
  'POST /auth/password/forgot/verify-code': {
    summary: '校验忘记密码验证码并列出账号',
    body: 'JSON: email, code, challenge_id',
    response: 'data: selection_token, accounts, email, masked_email。',
  },
  'POST /auth/password/forgot/select-account': {
    summary: '选择要重置密码的账号',
    body: 'JSON: selection_token, username',
    response: 'data: reset_token, username。',
  },
  'POST /auth/password/reset': {
    summary: '使用重置令牌修改密码',
    body: 'JSON: token, password, confirm_password',
    response: '密码重置成功提示。',
  },
  'POST /auth/check-password': {
    summary: '检查密码是否在泄露数据库中',
    body: 'JSON: password',
    response:
      'data: enabled(泄露检测是否开启), breached(是否泄露), warning(可选,检测服务不可用时的提示)。采用 HIBP k-匿名性模型，密码哈希不离开本机。',
  },
  'GET /security/password-breach/status': {
    summary: '读取密码泄露扫描状态',
    response:
      'data.status: scheduled_enabled, last_checked_at, running, breached_total, breached_admins, breached_users, affected[]；运行中时另含 active_task。',
  },
  'POST /security/password-breach/scan': {
    summary: '立即提交完整密码泄露扫描',
    response: 'data: task, reused；已有扫描运行时复用现有任务，不重复创建。',
    notes: ['不受实时泄露检测开关与定时泄露检测开关限制，会执行正式账户处置和首次邮件通知。'],
    highRiskNote: '扫描可能撤销管理员会话并触发邮件通知，必须完成高风险二次验证。',
  },
  'POST /auth/login/email/send': {
    summary: '登录阶段发送邮箱验证码',
    response: 'data: challenge_id, masked_email, expires_in。',
    notes: ['请求头必须使用 login token，不支持 API Key。'],
  },
  'POST /auth/login/verify': {
    summary: '完成登录阶段 TOTP/邮箱/恢复码验证',
    body: 'JSON: method(totp/recovery/email), code, challenge_id',
    response: 'data: stage, token, username, role, cloud_type, security。',
    notes: ['请求头必须使用 login token，不支持 API Key。', 'recovery 方法传入 16 位恢复码，每个恢复码只能使用一次。'],
  },
  'POST /auth/email/code/send': {
    summary: '发送邮箱绑定验证码',
    body: 'JSON: email',
    response: 'data: challenge_id, masked_email, expires_in。',
    notes: ['支持 access/bootstrap token，不支持 API Key。'],
  },
  'POST /auth/email/bind': {
    summary: '绑定或换绑邮箱',
    body: 'JSON: email, code, challenge_id',
    response: 'data: security；引导完成时返回新的 access token。',
    notes: ['支持 access/bootstrap token，不支持 API Key。'],
  },
  'POST /auth/2fa/setup': {
    summary: '生成 TOTP 配置',
    response: 'data: secret, otpauth_url。',
    notes: ['支持 access/bootstrap token，不支持 API Key。'],
  },
  'POST /auth/2fa/enable': {
    summary: '启用 TOTP 2FA',
    body: 'JSON: secret, code',
    response:
      'data: security；管理员引导完成时返回新的 access token。response 中还包含 recovery: { recovery_codes: [...] }，为 10 组一次性恢复码（仅此一次可获取）。',
    notes: ['支持 access/bootstrap token，不支持 API Key。'],
  },
  'POST /auth/2fa/disable': {
    summary: '关闭 TOTP 2FA',
    body: 'JSON: password, code',
    response: 'data: security。关闭 2FA 的同时会清除所有恢复码。',
    notes: ['不支持 API Key。'],
  },
  'POST /auth/2fa/recovery/regen': {
    summary: '重新生成恢复码',
    body: 'JSON: password, code',
    response: 'recovery: { recovery_codes: [...] }，旧恢复码立即失效。',
    notes: ['不支持 API Key。', '需要验证当前密码和 2FA 验证码。'],
  },
  'POST /auth/skip-bootstrap': {
    summary: '管理员跳过安全初始化',
    body: 'JSON: confirm(true)',
    response: 'data: stage=success 时返回正式 access token。',
    notes: ['仅安全初始化引导阶段可用，不支持 API Key。'],
  },
  'GET /auth/info': {
    summary: '读取当前用户信息',
    response: 'data: id, username, role, cloud_type, security。',
  },
  'GET /auth/api-key': {
    summary: '读取当前用户 API Key 状态',
    response: 'data: api_key_id, key_prefix, created_at, last_used_at, enabled。',
  },
  'POST /auth/api-key': {
    summary: '生成或重新生成 API Key',
    response: 'data: api_key_id, api_key, key_prefix, created_at, enabled；api_key 仅返回一次。',
  },
  'DELETE /auth/api-key': { summary: '撤销当前 API Key' },
  'PUT /auth/password': {
    summary: '修改当前账户密码',
    body: 'JSON: old_password, new_password',
    response: '密码修改成功后需要重新登录。',
  },
  'PUT /auth/username': {
    summary: '修改当前账户用户名',
    body: 'JSON: new_username, password',
    response: 'data: token, username。',
  },
  'POST /auth/high-risk/verify': {
    summary: '完成高风险操作二次验证',
    body: 'JSON: method(totp/recovery/email), code, challenge_id, operation',
    response: 'data: verification_token, trusted_until。recovery 方式额外返回 recovery_codes_remaining。',
    notes: ['使用 API Key 调用敏感接口时，也需要先调用本接口。', 'recovery 方法传入 16 位恢复码。'],
  },

  // ==================== 系统设置 ====================
  'GET /settings': {
    summary: '读取系统设置',
    notes: ['支持 access/bootstrap token；API Key 仅适用于 access 模式。'],
  },
  'PUT /settings': {
    summary: '保存系统设置',
    body: 'JSON: template_dir, clone_dir, iso_dir, network_backend, ovs_bridge, host_ip, public_base_url, site_title, development_mode, maintenance_mode, smtp_* 等可持久化配置',
    highRiskNote: '修改 development_mode、maintenance_mode、SMTP 密码等敏感项时需要二次验证',
  },
  'GET /settings/user-storage-iso-path': {
    summary: '获取用户存储 ISO 目录路径',
    response: 'data: iso_path。用于一键替换系统 ISO 存放位置。',
  },
  'POST /settings/smtp/test': { summary: '发送 SMTP 测试邮件', body: 'JSON: email' },
  'PUT /settings/cpu-affinity-presets': {
    summary: '保存 CPU 亲和性预设列表',
    body: 'JSON: presets[{name,value}]',
  },
  'POST /settings/jwt-secret/rotate': {
    summary: '手动轮换 JWT 密钥',
    response: '轮换后所有已签发 token 立即失效，需要重新登录。',
  },
  'GET /settings/log/status': {
    summary: '获取日志状态',
    response: 'data: total_size, total_size_human, files[{name,size,mod_time,is_today,category}], categories',
    notes: ['返回日志目录下所有日志文件列表及磁盘总占用大小'],
  },
  'POST /settings/log/delete': {
    summary: '删除日志文件',
    body: 'JSON: files[] 文件名列表',
    response: 'data: deleted[], failed[]',
    notes: ['仅允许删除 .log 和 .log.gz 文件，自动校验路径安全'],
  },
  'POST /settings/log/export': {
    summary: '导出日志文件',
    body: 'JSON: files[] 文件名列表',
    response: 'application/zip 二进制流',
    notes: ['将选中的日志文件打包为 ZIP 压缩包下载'],
  },
  'GET /settings/diagnostics/categories': {
    summary: '获取诊断类别列表',
    response: 'data: categories[{id,label,description}]',
    notes: ['返回可用的诊断信息收集类别'],
  },
  'POST /settings/diagnostics/export': {
    summary: '导出诊断信息',
    body: 'JSON: categories[] 类别ID列表',
    response: 'application/zip 二进制流',
    notes: ['收集选中类别的系统及面板诊断信息，打包为ZIP下载'],
  },
  'POST /settings/storage/trim': {
    summary: '执行用户存储空间回收',
    response: 'data: before_blocks, after_blocks, trimmed_bytes, trimmed_human。',
    notes: ['执行 fstrim 与稀疏化回收，耗时较长。'],
  },

  // ==================== 虚拟机 ====================
  'GET /vm/list': { summary: '获取虚拟机列表', query: ['keyword', 'status', 'owner 等筛选字段'] },
  'GET /vm/sse': {
    summary: '虚拟机列表 SSE 推送',
    query: ['token'],
    response: 'text/event-stream，推送 VM 列表变化。',
    notes: ['浏览器 EventSource 通常使用 token 查询参数；外部客户端可使用请求头。'],
  },
  'GET /vm/:name': {
    summary: '获取虚拟机详情',
    notes: ['guest_agent_status: QEMU Guest Agent 状态（configured/connected/version）'],
  },
  'GET /vm/:name/xml': { summary: '读取虚拟机持久化 XML', response: 'data: xml 字符串。' },
  'GET /vm/:name/ip': { summary: '获取虚拟机 IP' },
  'GET /vm/:name/sse': {
    summary: '虚拟机详情 SSE 推送',
    query: ['token'],
    response: 'text/event-stream，推送 VM 详情。',
  },
  'GET /vm/:name/pcie-info': {
    summary: '获取 PCIe 根端口用量',
    response: 'data: total_ports, used_ports, free_ports。',
  },
  'POST /vm/:name/operate': {
    summary: '执行开机/关机/重启等操作',
    body: 'JSON: action(start/shutdown/destroy/reboot/reset)',
    requiredFields: ['action'],
  },
  'PUT /vm/:name': {
    summary: '编辑虚拟机配置',
    body: 'JSON: vcpu, ram, remark, tags[], boot_type, boot_order, bandwidth, display, apic, pae, rtc, cpu_limit_percent(仅管理员, 0-100) 等可编辑字段',
    notes: ['remark 支持单独提交，用于独立更新虚拟机备注。', 'tags[] 支持单独提交，最多 20 个标签，单个标签最多 32 个字符。', '修改 boot_type 需要虚拟机关机后执行。'],
  },
  'PUT /vm/:name/xml': { summary: '保存虚拟机 XML', body: 'JSON: xml' },
  'GET /vm/:name/stats': { summary: '读取虚拟机实时资源统计', query: ['refresh'] },
  'GET /vm/:name/stats/history': { summary: '读取虚拟机历史资源统计', query: ['start', 'end'] },
  'GET /vm/:name/schedules': { summary: '获取虚拟机定时任务' },
  'POST /vm/:name/schedules': {
    summary: '创建虚拟机定时任务',
    body: scheduleBody,
    highRiskNote: '定时删除 VM 任务需要二次验证',
  },
  'PUT /vm/:name/schedules/:id': {
    summary: '更新虚拟机定时任务',
    body: scheduleBody,
    highRiskNote: '启用/修改定时删除 VM 任务需要二次验证',
  },
  'DELETE /vm/:name/schedules/:id': { summary: '删除虚拟机定时任务' },
  'GET /vm/:name/network/status': {
    summary: '读取 VM OVS 网络运行状态',
    notes: ['每个接口含 ip / ip_source，优先 QEMU Guest Agent'],
  },
  'GET /vm/:name/network/diagnostics': { summary: '读取 VM 网络诊断信息' },
  'POST /vm/:name/network/capture': {
    summary: '启动 VM 网络抓包任务',
    body: 'JSON: interface, seconds, max_mb, max_packets, filter',
  },
  'GET /vm/:name/vpc': { summary: '读取 VM VPC 绑定' },
  'PUT /vm/:name/vpc': { summary: '绑定 VM 到 VPC 交换机/安全组', body: 'JSON: switch_id, security_group_id' },
  'POST /vm/:name/migration/preview': {
    summary: '预检跨节点迁移',
    body: migrationBody,
    notes: ['该接口可选；源 VM 运行中自动热迁移，关机自动冷迁移；热迁移返回 live_assessment 与 preview_id。'],
  },
  'POST /vm/:name/migrate': {
    summary: '提交跨节点迁移任务',
    body: migrationBody,
    notes: ['有 preview_id 时复用预检；没有 preview_id 时任务开始后生成执行计划；迁移中 VM 状态为 migrating 并阻止用户侧操作。'],
  },
  'GET /vm/:name/disk-migration/options': {
    summary: '获取本机硬盘迁移选项',
    notes: ['返回当前冷热迁移模式、可迁移硬盘和本机目标存储。'],
  },
  'POST /vm/:name/disk/:dev/migrate': {
    summary: '提交本机硬盘迁移任务',
    body: 'JSON: target_storage_pool_id',
    notes: ['运行中 VM 自动执行硬盘热迁移，关机 VM 自动执行冷迁移；成功后删除源硬盘文件。'],
  },
  'PUT /vm/:name/security-group': { summary: '切换 VM 安全组', body: 'JSON: security_group_id' },
  'GET /vm/:name/interfaces': { summary: '列出 VM 所有网口' },
  'POST /vm/:name/interfaces': { summary: '新增 VM 网口', body: 'JSON: switch_id, security_group_id, nic_model' },
  'PUT /vm/:name/interfaces/:order': {
    summary: '更新 VM 指定网口',
    body: 'JSON: switch_id, security_group_id, nic_model',
  },
  'DELETE /vm/:name/interfaces/:order': { summary: '删除 VM 指定网口' },
  'DELETE /vm/:name': { summary: '删除虚拟机', body: 'JSON: delete_disks, transfer_disks, transfer_user' },
  'POST /vm/:name/force-delete': {
    summary: '强制删除虚拟机',
    notes: ['跳过常规校验直接清理 VM 定义与磁盘，仅用于常规删除失败的异常兜底。'],
  },
  'GET /vm/:name/qcow2-disks': { summary: '获取 VM qcow2 磁盘列表' },
  'POST /vm/:name/lock': { summary: '锁定虚拟机', notes: ['锁定后虚拟机无法关机或删除。'] },
  'POST /vm/:name/unlock': { summary: '解锁虚拟机', notes: ['解锁需要二次验证。'] },
  'GET /vm/:name/lock': {
    summary: '获取虚拟机锁定状态',
    response: 'data: { vm_name, locked, locked_at, locked_by }',
  },
  'GET /vm/:name/passthrough': { summary: '获取 VM 已直通的 PCI 设备列表' },
  'POST /vm/:name/passthrough': {
    summary: '为 VM 附加 PCI 直通设备',
    body: 'JSON: pci_address, primary_display 等直通选项',
  },
  'DELETE /vm/:name/passthrough': { summary: '从 VM 移除 PCI 直通设备', body: 'JSON: pci_address' },
  'POST /vm/:name/make-independent': {
    summary: '将链式克隆虚拟机转为独立虚拟机',
    notes: [
      '仅链式克隆（有 backing file）的关机 VM 可用。将 backing chain 合并为独立磁盘镜像，脱离对模板的依赖。',
      '异步任务，返回 task_id 请在任务中心查看进度。',
    ],
  },
  'POST /vm/create': {
    summary: '创建虚拟机',
    body: vmCreateBody,
    response: 'data: task_id。创建操作为异步任务，请继续查询任务详情。',
    notes: [
      'name/vcpu/ram/disk_size 为必填；name 只能包含字母和数字。',
      'remark 为可选备注，会写入虚拟机元数据。',
      'iso_paths 支持一次挂载多个安装 ISO，首个 ISO 会作为主安装盘。',
      'extra_disks 支持为每块额外硬盘指定 storage_pool_id；普通用户会计入硬盘配额。',
      'virt_type、arch、watchdog 为管理员普通创建接口支持字段。',
    ],
    requiredFields: ['name', 'vcpu', 'ram', 'disk_size'],
  },
  'POST /vm/import-disk': {
    summary: '管理员按磁盘文件导入创建虚拟机',
    body: 'JSON: name, disk_path/disk_file, disk_source_type, storage_pool_id, vcpu, ram, copy_disk, init_type, hostname, user, password 等导入表单字段',
    response: 'data: task_id。导入为异步任务。',
  },
  'POST /vm/import-appliance/inspect': {
    summary: '管理员检查 OVF/OVA 虚拟机包',
    body: 'JSON: appliance_file/appliance_path, source_type(storage/path)',
    response: 'data: source_format, name, architecture, vcpu, ram, boot_type, machine_type, os_type, disks, networks, warnings。',
  },
  'POST /vm/import-appliance': {
    summary: '管理员导入 OVF/OVA 虚拟机包',
    body: 'JSON: appliance_file/appliance_path, source_type, config_mode(ovf/custom), copy_source，以及 name, vcpu, ram, storage_pool_id, 网络映射、开机自启和导入后启动配置',
    response: 'data: task_id。接口立即创建 import_appliance 任务，完整包校验、架构检查、配额复核和解包在任务中执行。',
  },
  'GET /vm/os-variants': { summary: '获取 libosinfo 系统变体列表', response: 'data: OS variant 列表。' },
  'GET /vm/iso-list': { summary: '获取全局 ISO 列表', response: 'data: ISO 文件列表。' },
  'POST /vm/clone': {
    summary: '从模板克隆虚拟机',
    body: cloneBody,
    highRiskNote: '创建 VM 类高风险验证按现有策略触发',
  },
  'POST /vm/linked-clone': {
    summary: '原生链式克隆虚拟机',
    body: cloneBody,
    highRiskNote: '创建 VM 类高风险验证按现有策略触发',
  },
  'POST /vm/batch-clone': {
    summary: '批量克隆虚拟机',
    body: 'JSON: prefix(名称前缀), start_num(起始编号), count(创建数量), template, template_type, clone_mode(linked/full), vcpu, ram, disk_size, hostname(可选), user(新建用户), password, freeze, template_root_pass, template_user, video_model(支持none禁用虚拟显示), spice_enabled(bool,是否启用SPICE显示协议,不传=回退全局默认), disk_bus, cpu_topology_mode, first_boot_reboot_mode, uefi；count>1 时不允许 host_devices',
  },
  'POST /vm/:name/reinstall': {
    summary: '重装虚拟机',
    body: reinstallBody,
    notes: [
      '仅支持弹性云 VM，保留现有 CPU、内存、网络、VPC、安全组与额外数据盘，只替换第一块系统盘。',
      '未传 disk_size 时默认使用当前系统盘大小；若小于模板原始磁盘大小，后端会自动提升到模板最小值。',
      '提交后会先自动删除该 VM 的全部快照，再强制关机并按模板类型重新执行系统初始化。',
      '若模板启动族与当前 VM 不一致（BIOS/UEFI），接口会直接拒绝。',
    ],
  },
  'GET /vm/:name/snapshots': { summary: '获取快照列表' },
  'DELETE /vm/:name/snapshots': {
    summary: '删除全部快照',
    notes: [
      '按快照树从叶子节点开始删除；外部快照会尽量合并并保留当前状态；历史内部快照若已不在当前活动磁盘链，会仅清理 libvirt 元数据；完成后会清理不再被引用的 .snap_* / .snap_restore_* 残留文件。',
    ],
  },
  'POST /vm/:name/snapshot': {
    summary: '创建快照',
    body: 'JSON: description, include_memory, pause_for_memory_snapshot, auto_fix_nvram, name(可选)',
    notes: [
      '未传 name 时系统自动生成快照名称；显式名称仅支持英文、数字、下划线、点和短横线，最长 64 个字符。',
      '运行中创建包含内存的快照时，pause_for_memory_snapshot 默认为 true，面板会先暂停虚拟机并在快照完成后恢复运行；传 false 则不主动暂停，但 QEMU 保存内存时 VM 仍会进入 paused (saving) 状态（非面板行为，是虚拟化层固有机制）。',
      '内存快照耗时取决于虚拟机内存大小，大内存虚拟机可能需要数分钟。',
      '运行中 VM 挂载 9p/VirtFS 时不支持包含内存状态的内部快照。',
    ],
  },
  'POST /vm/:name/snapshot/:snap/revert': { summary: '恢复快照' },
  'DELETE /vm/:name/snapshot/:snap': { summary: '删除快照' },
  'GET /vm/:name/vnc/status': { summary: '读取 VNC 状态' },
  'POST /vm/:name/vnc/enable': { summary: '启用 VNC', body: 'JSON: password(可选)' },
  'POST /vm/:name/vnc/disable': { summary: '关闭 VNC' },
  'POST /vm/:name/vnc/passwd': { summary: '修改 VNC 密码', body: 'JSON: password' },
  'POST /vm/:name/vnc/expose': { summary: '切换 VNC 对外暴露', body: 'JSON: expose' },
  'GET /vm/:name/vnc/ws': {
    summary: 'VNC WebSocket 连接',
    query: ['token'],
    response: 'WebSocket 数据流。',
    notes: ['浏览器 WebSocket 不便自定义 API Key 请求头，建议使用 JWT token 查询参数。'],
  },
  'GET /vm/:name/spice/status': {
    summary: '读取 SPICE 状态',
    response: 'data: enabled, port, auth, exposed。',
  },
  'GET /vm/:name/spice/info': { summary: '获取 SPICE 连接信息', response: 'data: 主机、端口、密码提示等连接参数。' },
  'POST /vm/:name/spice/enable': { summary: '启用 SPICE', body: 'JSON: password(可选)' },
  'POST /vm/:name/spice/disable': { summary: '关闭 SPICE' },
  'POST /vm/:name/spice/passwd': { summary: '修改 SPICE 密码', body: 'JSON: password' },
  'POST /vm/:name/spice/expose': { summary: '切换 SPICE 对外暴露', body: 'JSON: expose' },
  'GET /vm/:name/spice/vv': {
    summary: '下载 SPICE .vv 连接文件',
    query: ['token'],
    response: '.vv 文件流，供 virt-viewer/spicy 等外部客户端使用。',
  },
  'GET /vm/:name/monitor/status': { summary: '获取 QEMU Monitor 状态' },
  'POST /vm/:name/monitor/command': {
    summary: '执行 QEMU Monitor 命令',
    body: 'JSON: command',
    notes: ['普通用户只开放安全命令子集。'],
  },
  'GET /vm/:name/disks': { summary: '获取磁盘列表' },
  'POST /vm/:name/disk': { summary: '新增磁盘', body: 'JSON: size_gb, format, bus, storage_pool_id, guest_mount{enabled,filesystem,mount_point,drive_letter}' },
  'POST /vm/:name/disk/:dev/resize': { summary: '扩容磁盘', body: 'JSON: size_gb, auto_grow_partition（Linux 运行态可选）' },
  'GET /vm/:name/disk/:dev/guest-status': { summary: '读取磁盘来宾映射和文件系统状态' },
  'POST /vm/:name/disk/:dev/guest-mount': {
    summary: '配置或重试来宾数据盘挂载',
    body: 'JSON: guest_mount{enabled,filesystem,mount_point,drive_letter}, existing_disk',
  },
  'POST /vm/:name/disk/:dev/guest-grow': { summary: '重试 Linux 系统分区扩容' },
  'PUT /vm/:name/disk/:dev/bus': { summary: '修改磁盘总线类型', body: 'JSON: bus(virtio/scsi/sata/ide)' },
  'POST /vm/:name/disk/attach': { summary: '挂载已有磁盘文件', body: 'JSON: path, bus, guest_mount{enabled,mount_point,drive_letter}' },
  'POST /vm/:name/disk/import': {
    summary: '为已有 VM 导入磁盘文件',
    body: 'JSON: disk_path/disk_file, disk_source_type, copy_disk, bus, storage_pool_id, guest_mount{enabled,mount_point,drive_letter}',
  },
  'DELETE /vm/:name/disk/:dev': { summary: '删除或转移磁盘', body: 'JSON: delete_file, transfer' },
  'GET /vm/:name/disk/:dev/iops': { summary: '读取磁盘 IOPS/吞吐限制' },
  'PUT /vm/:name/disk/:dev/iops': {
    summary: '设置磁盘 IOPS/吞吐限制',
    body: 'JSON: iops_total, iops_read, iops_write, mbps_* 等限速字段',
  },
  'POST /vm/:name/cdrom': { summary: '插入或更换 CD/DVD', body: 'JSON: iso_path, device, bus' },
  'POST /vm/:name/cdrom/eject': { summary: '弹出 CD/DVD', query: ['device'] },
  'DELETE /vm/:name/cdrom': { summary: '移除 CD/DVD 光驱', query: ['device'] },
  'POST /vm/:name/floppy': { summary: '插入或更换软盘镜像', body: 'JSON: vfd_path' },
  'POST /vm/:name/floppy/eject': { summary: '弹出软盘' },
  'DELETE /vm/:name/floppy': { summary: '移除软盘驱动器' },
  'POST /vm/:name/rescue': { summary: '启动或关闭救援系统', body: 'JSON: action(enable/disable/start/stop)' },
  'POST /vm/:name/password/reset': {
    summary: '在线或离线重置虚拟机系统密码',
    body: 'JSON: username, password, mode(auto/online/offline，默认auto)',
  },
  'GET /vm/:name/shares': { summary: '获取共享目录列表' },
  'POST /vm/:name/share': { summary: '挂载共享目录', body: 'JSON: host_path, tag, security_model, readonly' },
  'DELETE /vm/:name/share/:tag': { summary: '移除共享目录' },

  // ==================== 模板 ====================
  'GET /template/list': { summary: '获取模板列表' },
  'POST /template/prepare': {
    summary: '制作模板',
    body: 'JSON: vm_name, template_name, display_name, type, category, root_password, template_user',
  },
  'POST /template/:name/prepare-linux': {
    summary: '预处理已导入 Linux 模板',
    response: 'data: task_id。任务会在模板阶段安装并校验 cloud-init 与磁盘扩容依赖，克隆阶段保持离线。',
  },
  'POST /template/upload/init': {
    summary: '模板包分片上传-初始化/秒传',
    body: 'JSON: file_name, total_size, file_hash(抽样哈希)',
    response: 'data: session_key, total_chunks, chunk_size, received[], instant, completed。',
  },
  'POST /template/upload/chunk': {
    summary: '模板包分片上传-单片(1MB)',
    body: 'FormData: file, session_key, index',
  },
  'POST /template/upload/complete': {
    summary: '模板包分片上传-完成校验',
    body: 'JSON: session_key, file_hash(抽样哈希)',
    response: 'data: session_key(导入临时路径，作为 preview 的 source_path)。',
  },
  'DELETE /template/upload': { summary: '清理已上传的模板临时包', query: ['path(session_key)'] },
  'POST /template/import': { summary: '兼容旧格式导入模板', body: 'FormData: file/source_path, name, description' },
  'POST /template/import/preview': {
    summary: '预览模板包导入',
    body: 'FormData: source_path(分片上传返回的 session_key 或宿主机绝对路径)',
    response: 'data: token, manifest, conflicts, warnings。',
  },
  'POST /template/import/confirm': { summary: '确认模板包导入', body: 'JSON: token' },
  'GET /template/download/:filename': {
    summary: '下载模板导出文件',
    query: ['token'],
    response: '文件流。',
    notes: ['浏览器下载通常使用 token 查询参数。'],
  },
  'GET /template/:name/delete-preview': {
    summary: '获取模板删除影响预览',
    response:
      'data: templates, related_vms, parent_template, promoted_templates, rebased_vms, can_promote, promote_blockers, can_promote_hot, promote_hot_blockers。',
  },
  'GET /template/:name/vms': { summary: '获取使用模板创建的 VM' },
  'POST /template/:name/export': {
    summary: '导出模板包',
    query: ['scope(node/all)'],
    response: 'data: task_id 或导出任务。',
  },
  'DELETE /template/:name/export': { summary: '删除模板导出文件' },
  'PUT /template/:name/publish': { summary: '更新模板发布展示状态', body: templateMetaBody },
  'PUT /template/:name/meta': { summary: '更新模板元数据', body: templateMetaBody },
  'DELETE /template/:name': {
    summary: '删除模板',
    body: 'JSON: delete_mode(cascade/promote_children/promote_children_hot), delete_vms, expected_vms',
  },

  // ==================== 网络 ====================
  'GET /network/static-ip/list': { summary: '获取静态 IP 列表' },
  'POST /network/static-ip/bind': { summary: '绑定静态 IP', body: 'JSON: vm_name, ip, mac, network' },
  'POST /network/static-ip/unbind': { summary: '解绑静态 IP', body: 'JSON: vm_name, ip' },
  'GET /network/port-forward/list': { summary: '获取端口转发列表' },
  'POST /network/port-forward/add': { summary: '新增端口转发', body: portForwardBody },
  'PUT /network/port-forward/:id': { summary: '更新端口转发', body: portForwardBody },
  'DELETE /network/port-forward/:id': { summary: '删除端口转发' },
  'POST /network/port-forward/batch-delete': { summary: '批量删除端口转发', body: 'JSON: ids 或 rule_keys' },
  'POST /network/port-forward/save': { summary: '手动保存端口转发规则到系统', response: '保存结果。' },
  'GET /network/port-forward/ip-mapping': { summary: '获取端口转发手动 IP 映射', query: ['vm_name'] },
  'POST /network/port-forward/ip-mapping': { summary: '新增端口转发手动 IP 映射', body: 'JSON: vm_name, ip, remark' },
  'DELETE /network/port-forward/ip-mapping/:id': { summary: '删除端口转发手动 IP 映射' },
  'GET /network/ufw/status': { summary: '读取 UFW 状态' },
  'POST /network/ufw/rule': { summary: '管理 UFW 规则', body: 'JSON: action, port, protocol, source, comment' },
  'GET /network/host/interfaces': { summary: '列出宿主机网卡' },
  'GET /network/bridges': { summary: '列出宿主机网桥' },
  'POST /network/bridges': { summary: '创建宿主机网桥', body: 'JSON: name, interface, address, gateway, dns, mode' },
  'DELETE /network/bridges/:id': { summary: '删除宿主机网桥' },
  'GET /network/interfaces/:name/config': { summary: '获取接口 IP/DNS 配置' },
  'PUT /network/interfaces/:name/config': {
    summary: '设置接口 IP/DNS 配置',
    body: 'JSON: addrs(CIDR换行分隔), gateway, dns(空格分隔), clear(bool)',
  },
  'GET /network/public-ips': { summary: '列出公网 IP' },
  'POST /network/public-ips': { summary: '新增公网 IP', body: publicIPBody },
  'PUT /network/public-ips/:id': { summary: '更新公网 IP', body: publicIPBody },
  'DELETE /network/public-ips/:id': { summary: '删除公网 IP' },
  'POST /network/public-ips/:id/preview': { summary: '预览公网 IP 规则', body: publicIPBody },
  'POST /network/public-ips/:id/bind': { summary: '绑定公网 IP', body: 'JSON: vm_name, guest_ip, mac, mode' },
  'POST /network/public-ips/:id/unbind': { summary: '解绑公网 IP' },
  'POST /network/public-ips/:id/migrate': { summary: '迁移公网 IP 绑定', body: 'JSON: vm_name, guest_ip, mode' },
  'POST /network/public-ips/apply': { summary: '应用公网 IP 规则' },
  'GET /network/captures/:task_id': { summary: '获取抓包会话' },
  'GET /network/captures/:task_id/download': { summary: '下载抓包文件', query: ['token'], response: 'pcap 文件流。' },
  'DELETE /network/captures/:task_id': { summary: '删除抓包会话文件' },

  // ==================== VPC ====================
  'GET /vpc/quota': { summary: '读取 VPC 配额', query: ['username(管理员可选)'] },
  'GET /vpc/switches': { summary: '列出 VPC 交换机', query: ['username(管理员可选)'] },
  'POST /vpc/switches': { summary: '创建 VPC 交换机', body: vpcSwitchBody },
  'PUT /vpc/switches/:id': { summary: '更新 VPC 交换机', body: vpcSwitchBody },
  'POST /vpc/switches/:id/traffic/reset': { summary: '重置交换机流量统计' },
  'DELETE /vpc/switches/:id': { summary: '删除 VPC 交换机' },
  'GET /vpc/switches/:id/vms': { summary: '获取交换机下的 VM 列表' },
  'GET /vpc/security-groups': { summary: '列出安全组', query: ['username(管理员可选)'] },
  'POST /vpc/security-groups': { summary: '创建安全组', body: securityGroupBody },
  'PUT /vpc/security-groups/:id': { summary: '更新安全组', body: securityGroupBody },
  'DELETE /vpc/security-groups/:id': { summary: '删除安全组' },
  'POST /vpc/security-groups/:id/rules': { summary: '新增安全组规则', body: securityRuleBody },
  'DELETE /vpc/security-groups/rules/:id': { summary: '删除安全组规则' },
  'GET /vpc/acl/preview': { summary: '预览 VPC ACL 规则', response: 'data: ACL 预览文本或结构。' },
  'POST /vpc/acl/apply': { summary: '应用 VPC ACL 规则' },

  // ==================== 防火墙 ====================
  'GET /firewall/status': { summary: '读取防火墙状态' },
  'GET /firewall/policy': { summary: '读取防火墙策略' },
  'PUT /firewall/policy': { summary: '保存防火墙策略', body: firewallPolicyBody },
  'POST /firewall/preview': { summary: '预览防火墙策略', body: firewallPolicyBody },
  'POST /firewall/apply': { summary: '应用防火墙策略', body: 'JSON: policy' },
  'POST /firewall/disable': { summary: '禁用防火墙' },
  'POST /firewall/rollback': { summary: '回滚防火墙策略' },
  'POST /firewall/geoip/import': { summary: '导入地域库', body: 'JSON 或 FormData: region 数据文件/内容' },
  'POST /firewall/geoip/update': { summary: '更新 GeoIP 数据', body: 'JSON: source/url/path' },
  'PUT /firewall/port-forward': {
    summary: '设置端口转发防火墙策略',
    body: 'JSON: enabled, mode, allowed_regions, blocked_regions',
  },
  'GET /firewall/host/status': { summary: '读取宿主机防火墙状态' },
  'POST /firewall/host/enable/preview': {
    summary: '预览启用宿主机防火墙',
    body: 'JSON: mode, allow_ssh, allow_panel, extra_rules',
  },
  'POST /firewall/host/enable': { summary: '启用宿主机防火墙', body: 'JSON: mode, allow_ssh, allow_panel, extra_rules' },
  'POST /firewall/host/disable': { summary: '禁用宿主机防火墙' },
  'GET /firewall/host/rules': { summary: '列出宿主机防火墙规则' },
  'POST /firewall/host/rules': { summary: '创建宿主机防火墙规则', body: hostRuleBody },
  'PUT /firewall/host/rules/:id': { summary: '更新宿主机防火墙规则', body: hostRuleBody },
  'DELETE /firewall/host/rules/:id': { summary: '删除宿主机防火墙规则' },
  'POST /firewall/host/rules/vnc-default': { summary: '添加 VNC 默认防火墙规则' },
  'GET /firewall/host/connections/preview': { summary: '预览宿主机连接关闭影响', query: ['mode'] },
  'POST /firewall/host/connections/close': {
    summary: '关闭宿主机连接',
    body: 'JSON: mode, ports, exclude_current_session',
  },

  // ==================== OVS ====================
  'GET /ovs/status': { summary: '读取 OVS 状态' },
  'GET /ovs/ports': { summary: '读取 OVS 端口' },
  'GET /ovs/leases': { summary: '读取 DHCP 租约' },
  'POST /ovs/check': { summary: '检查 OVS 网络' },
  'POST /ovs/repair': { summary: '修复 OVS 网络' },

  // ==================== 存储池 ====================
  'GET /storage-pool/list': { summary: '获取存储池列表' },
  'GET /storage-pool/all-isos': { summary: '获取所有存储池 ISO' },
  'GET /storage-pool/vm-targets': { summary: '获取创建 VM 可选存储目标' },
  'GET /storage-pool/:id': { summary: '获取存储池详情' },
  'PUT /storage-pool/:id/config': {
    summary: '更新存储池配置',
    body: 'JSON: name, path, type, enabled, allow_template, allow_vm, remark',
  },
  'POST /storage-pool/:id/default': { summary: '设置默认存储池' },
  'POST /storage-pool/:id/format-mount': { summary: '格式化并挂载存储池', body: 'JSON: fstype(默认 ext4)' },
  'POST /storage-pool/:id/create-partition': { summary: '创建磁盘分区' },
  'POST /storage-pool/:id/delete-partitions': { summary: '删除磁盘分区' },
  'GET /storage-pool/pv-targets': { summary: '获取可用 LVM PV 目标' },
  'POST /storage-pool/create-volume': { summary: '创建 LVM 逻辑卷' },
  'POST /storage-pool/delete-volume': { summary: '删除 LVM 逻辑卷' },

  // ==================== 节点管理 ====================
  'GET /nodes': { summary: '获取节点列表' },
  'POST /nodes': {
    summary: '添加节点',
    body: 'JSON: name, api_base_url, api_key_id, api_key, ssh_host, ssh_port, ssh_user, ssh_password, enabled',
  },
  'PUT /nodes/:id': {
    summary: '更新节点',
    body: 'JSON: name, api_base_url, api_key_id, api_key, ssh_host, ssh_port, ssh_user, ssh_password, enabled；密钥留空表示不修改',
  },
  'DELETE /nodes/:id': { summary: '删除节点' },
  'POST /nodes/:id/probe': { summary: '探测节点能力' },
  'GET /nodes/:id/migration-options': {
    summary: '加载 VM 迁移表单选项',
    query: ['vm_name'],
    notes: ['返回自动迁移模式、目标存储、目标用户处理方式；目标已有同名用户时才返回该用户下的 VPC/安全组。'],
  },
  'POST /migration/adopt-vm': {
    summary: '目标面板接管迁移 VM',
    body: 'JSON: vm_name, owner, cloud_type, target_switch_id, credential, port_forwards 等迁移接管数据',
    notes: ['通常由源节点迁移任务调用'],
  },

  // ==================== 用户管理 ====================
  'GET /user/list': { summary: '获取用户列表', query: ['page', 'page_size', 'keyword', 'status', 'role'] },
  'POST /user': {
    summary: '创建用户或邀请用户',
    body: 'JSON: username, email, password, role, cloud_type, quota 字段, enable_port_forward, dedicated_vpc_switch_id',
  },
  'PUT /user/:username/vms': { summary: '分配 VM 给用户', body: 'JSON: vms, lightweight_quotas' },
  'POST /user/:username/lightweight-registrations': {
    summary: '登记轻量云待开通 VM',
    body: 'JSON: registrations[]，每项包含 vm_name, quota, template, network, preserve_fnos_device_id/fnos_device_id(FnOS 可选) 等',
  },
  'PUT /user/:username/lightweight-vm-quota': {
    summary: '更新轻量云单 VM 配额',
    body: 'JSON: vm_name, max_cpu, max_memory, max_disk, max_bandwidth_*, max_traffic_*, max_snapshots, max_runtime_hours',
  },
  'DELETE /user/:username/lightweight-vm/:vmName': { summary: '移除已开通轻量云 VM 注册记录' },
  'DELETE /user/:username/lightweight-registrations/:id': { summary: '删除轻量云待开通登记' },
  'PUT /user/:username/quota': {
    summary: '更新用户配额',
    body: 'JSON: max_cpu, max_memory, max_disk, max_vm, max_storage, max_runtime_hours, max_port_forwards, max_snapshots, bandwidth/traffic/public_ip 配额等',
  },
  'PUT /user/:username/status': { summary: '封禁或解封用户', body: 'JSON: status(active/disabled)' },
  'GET /user/:username/quota': { summary: '获取用户配额使用情况' },
  'PUT /user/:username/ssh': { summary: '切换用户 SSH 权限', body: 'JSON: enabled' },
  'POST /user/:username/resend-invite': { summary: '重发邀请邮件' },
  'POST /user/:username/traffic/reset': { summary: '重置用户流量配额' },
  'DELETE /user/:username': { summary: '删除用户及其资产' },

  // ==================== 用户自助与我的存储 ====================
  'GET /self/quota': { summary: '查看自己的配额' },
  'GET /self/vms': { summary: '查看自己的 VM 列表', query: ['keyword', 'status'] },
  'GET /self/vms/sse': { summary: '自己的 VM 列表 SSE 推送', query: ['token'], response: 'text/event-stream。' },
  'GET /self/lightweight-registrations': { summary: '查看轻量云待确认服务器' },
  'POST /self/lightweight-registrations/:id/confirm': {
    summary: '确认开通轻量云服务器',
    body: 'JSON: password, confirm_options, network/VPC 选择等',
  },
  'POST /self/vm/clone': { summary: '用户自助从模板克隆 VM', body: cloneBody },
  'POST /self/vm/create': {
    summary: '用户自助创建 VM',
    body: selfVmCreateBody,
    response: 'data: task_id。创建操作为异步任务，请继续查询任务详情。',
    notes: [
      'name/vcpu/ram/disk_size 为必填；name 只能包含字母和数字。',
      'remark 为可选备注，会写入虚拟机元数据。',
      'iso_paths 支持一次挂载多个安装 ISO，首个 ISO 会作为主安装盘。',
      '普通用户的 switch_id/security_group_id 会按当前用户 VPC 权限解析，并受配额限制。',
    ],
    requiredFields: ['name', 'vcpu', 'ram', 'disk_size'],
  },
  'DELETE /self/vm/:name': { summary: '用户自助删除自己的 VM', body: 'JSON: delete_disks, transfer_disks' },
  'GET /self/vm/:name/qcow2-disks': { summary: '获取自己的 VM qcow2 磁盘列表' },
  'POST /self/vm/export': {
    summary: '导出自己的 VM',
    body: 'JSON: vm_name, format(qcow2/ova，省略时为 qcow2), disk_devices。OVA 的系统盘固定导出，数据盘可选。',
  },
  'GET /self/vm/:name/export-options': {
    summary: '获取虚拟机导出选项',
    response: 'data: vm_name, status, disks（设备名、格式、总线、容量、系统盘标记和兼容状态）。',
  },
  'POST /self/vm/import': {
    summary: '导入 VM 到自己账号',
    body: 'JSON: file/category/path, name, remark, vcpu, ram, switch_id, security_group_id, credentials 等',
  },
  'POST /self/vm/import-appliance/inspect': {
    summary: '检查我的存储中的 OVF/OVA 虚拟机包',
    body: 'JSON: appliance_file, source_type(storage)',
    response: 'data: ApplianceMetadata，包含硬件、全部磁盘、网卡与兼容性提示。',
  },
  'POST /self/vm/import-appliance': {
    summary: '从我的存储导入 OVF/OVA 虚拟机包',
    body: 'JSON: appliance_file, source_type(storage), config_mode(ovf/custom), copy_source，以及最终硬件、目标存储、网络映射和导入后启动配置',
    response: 'data: task_id。创建操作保留二次验证；完整包校验与配额复核在异步任务中执行。',
  },
  'GET /self/storage/info': { summary: '获取我的存储信息' },
  'POST /self/storage/init': { summary: '初始化我的存储' },
  'GET /self/storage/files/:category': { summary: '列出我的存储文件' },
  'POST /self/storage/upload/init': {
    summary: '分片上传-初始化/秒传',
    body: 'JSON: category(iso/share/disk), file_name, total_size, file_hash(抽样哈希)',
    response:
      'data: session_key, total_chunks, chunk_size, received[], uploaded_bytes, instant, completed。completed/instant=true 表示秒传成功。',
  },
  'POST /self/storage/upload/chunk': { summary: '分片上传-单片(1MB)', body: 'FormData: file, session_key, index' },
  'POST /self/storage/upload/complete': { summary: '分片上传-完成校验', body: 'JSON: session_key, file_hash(抽样哈希)' },
  'GET /self/storage/upload/status': {
    summary: '查询上传进度(断点续传)',
    query: ['path(session_key)'],
    response: 'data: exists, status, total_chunks, chunk_size, received[], uploaded_bytes。',
  },
  'GET /self/storage/upload/pending': {
    summary: '列出未完成的上传会话(主动恢复)',
    response:
      'data: [{session_key, category, file_name, total_size, uploaded_bytes, total_chunks, progress, file_hash}]。',
  },
  'DELETE /self/storage/upload': { summary: '取消上传并清理', query: ['path(session_key)'] },
  'DELETE /self/storage/file/:category/:filename': { summary: '删除我的存储文件' },
  'GET /self/storage/download/:category/:filename': {
    summary: '下载我的存储文件',
    query: ['token'],
    response: '文件流。',
  },
  'GET /self/storage/isos': { summary: '获取我的 ISO 列表' },
  'GET /self/storage/mounts': { summary: '获取我的存储挂载列表' },
  'POST /self/storage/mount': {
    summary: '挂载我的存储到 VM',
    body: 'JSON: vm_name, category(iso/share/disk), filename/tag, readonly',
  },
  'DELETE /self/storage/mount/:vmName/:tag': { summary: '卸载我的存储挂载' },

  // ==================== 宿主机 ====================
  'GET /host/stats': { summary: '读取宿主机实时统计' },
  'GET /host/stats/sse': { summary: '宿主机实时统计 SSE 推送', query: ['token'], response: 'text/event-stream。' },
  'GET /host/stats/history': { summary: '读取宿主机历史统计', query: ['start', 'end'] },
  'GET /host/cpus': { summary: '获取宿主机 CPU 核心数', response: 'data: cores。' },
  'GET /host/cpu/hardware': {
    summary: '获取宿主机 CPU 硬件信息与每核使用率',
    response: 'data: model, sockets, cores, threads, per_core_usage。',
  },
  'GET /host/memory/modules': {
    summary: '获取宿主机内存条（DIMM）信息',
    response: 'data: total_slots, installed, modules, message。',
  },
  'GET /host/disks': { summary: '获取宿主机磁盘挂载列表' },
  'GET /host/kvm-intel-unrestricted-guest': { summary: '读取 Intel KVM unrestricted_guest 状态' },
  'PUT /host/kvm-intel-unrestricted-guest': { summary: '设置 Intel KVM unrestricted_guest', body: 'JSON: enabled' },
  'GET /host/ksm': { summary: '读取 KSM 状态' },
  'PUT /host/ksm': { summary: '设置 KSM 挡位', body: 'JSON: profile(off/conservative/balanced/aggressive)' },
  'GET /host/zram': { summary: '读取 zRAM 状态' },
  'PUT /host/zram': { summary: '设置 zRAM 挡位', body: 'JSON: profile(off/conservative/balanced/aggressive)' },
  'GET /host/hardware-passthrough/status': { summary: '获取硬件直通环境状态' },
  'POST /host/hardware-passthrough/enable-iommu': { summary: '一键开启 IOMMU', notes: ['写入 grub 并执行 update-grub，需要重启宿主机生效。'] },
  'POST /host/hardware-passthrough/load-vfio': { summary: '一键加载 vfio-pci 模块' },
  'GET /host/passthrough': { summary: '获取可直通 PCI 设备列表' },
  'POST /host/passthrough/bind': { summary: '绑定 PCI 设备到 vfio-pci', body: 'JSON: pci_address' },
  'POST /host/passthrough/unbind': { summary: '从 vfio-pci 解绑 PCI 设备', body: 'JSON: pci_address' },

  // ==================== 任务与调度 ====================
  'GET /task/list': { summary: '获取任务列表', query: ['page', 'page_size', 'status', 'type'] },
  'GET /task/sse': { summary: '任务进度 SSE 推送', query: ['token'], response: 'text/event-stream。' },
  'GET /task/:id': { summary: '获取任务详情' },
  'POST /task/:id/cancel': { summary: '取消任务' },
  'DELETE /task/clear': { summary: '清理已完成任务' },
  'GET /scheduler/list': { summary: '获取调度器概览' },
  'GET /scheduler/events': { summary: '获取调度事件列表', query: ['page', 'page_size', 'type', 'status', 'start', 'end'] },
  'GET /scheduler/events/sse': { summary: '调度事件 SSE 推送', query: ['token'], response: 'text/event-stream。' },

  // ==================== 其他 ====================
  'GET /cpu-affinity-presets': { summary: '获取 CPU 亲和性预设列表', response: 'data: presets[{name,value}]。' },
  'GET /system-info': {
    summary: '获取系统运行环境信息',
    response: 'data: os, distro, kernel, arch, hostname, num_cpu, go_version, qemu, libvirt, uptime 等。',
  },
}
