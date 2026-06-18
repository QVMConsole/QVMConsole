const json = 'Content-Type: application/json'
const form = 'Content-Type: multipart/form-data'
const apiHeaders = ['X-API-Key-ID: <API_ID>', 'X-API-Key: <API_KEY>', json]
const jwtHeaders = ['Authorization: Bearer <access_token>', json]
const jwtOnlyHeaders = ['Authorization: Bearer <access/login/bootstrap token>', json]
const admin = '需要管理员权限'
const elastic = '轻量云用户不可用'
const vmAccess = '普通用户只能访问归属自己的 VM'
const highRisk = '需要二次验证，428 后携带 X-High-Risk-Token 重试'
const apiCompatible = 'JWT / API Key'
const jwtOnly = '仅 JWT'
const publicAuth = '公开'

function ep(method, path, summary, options = {}) {
  const auth = options.auth || apiCompatible
  return {
    method,
    path,
    summary,
    auth,
    headers: options.headers || (auth === publicAuth ? ['无'] : auth === jwtOnly ? jwtOnlyHeaders : apiHeaders),
    pathParams: options.pathParams || [],
    query: options.query || [],
    body: options.body || '无',
    response: options.response || '统一返回 { code, message, data }，data 为对应资源或任务信息。',
    highRisk: options.highRisk || '',
    notes: options.notes || [],
    requiredFields: options.requiredFields || []
  }
}

const vmCreateBody = 'JSON: name, remark, vcpu, ram, disk_size, disk_format, disk_bus, os_variant, iso_path, iso_paths[], nic_model, autostart, freeze, apic, pae, rtc_offset, rtc_startdate, guest_agent{enabled}, smbios1{base64,family,manufacturer,product,serial,sku,uuid,version}, os_type, machine_type, boot_type, watchdog, boot_order[], video_model, cpu_topology_mode(auto/single_socket/host_default), cpu_limit_percent(仅管理员, 0-100), virt_type(kvm/qemu), arch(x86_64/aarch64/riscv64), memory_dynamic{dynamic_enabled,memory_backend,memory_initial,memory_min,memory_max,memory_auto_balloon,memory_current}, switch_id, security_group_id, storage_pool_id, extra_disks[{size,format,bus,storage_pool_id}]'
const selfVmCreateBody = 'JSON: name, remark, vcpu, ram, disk_size, disk_format, disk_bus, os_variant, iso_path, iso_paths[], nic_model, autostart, freeze, apic, pae, rtc_offset, rtc_startdate, guest_agent{enabled}, smbios1{base64,family,manufacturer,product,serial,sku,uuid,version}, os_type, machine_type, boot_type, boot_order[], video_model, cpu_topology_mode(auto/single_socket/host_default), memory_dynamic{dynamic_enabled,memory_backend,memory_initial,memory_min,memory_max,memory_auto_balloon,memory_current}, switch_id, security_group_id, storage_pool_id, extra_disks[{size,format,bus,storage_pool_id}]'
const cloneBody = 'JSON: template/name, new_name/name, remark, vcpu, ram, disk_size, disk_bus, switch_id, security_group_id, storage_pool_id, extra_disks[{size,format,bus,storage_pool_id}], nic_model, video_model, cpu_topology_mode, cpu_limit_percent(仅管理员, 0-100), first_boot_reboot_mode(normal/cold), preserve_fnos_device_id/fnos_device_id(FnOS 可选), autostart, freeze, apic, pae, rtc_offset, credentials 等克隆表单字段'
const reinstallBody = 'JSON: template, disk_size, hostname, user, password, preserve_fnos_device_id, fnos_device_id'
const scheduleBody = 'JSON: name, action(start/shutdown/destroy/reboot/delete), cron/execute_at, enabled, timezone, params'
const portForwardBody = 'JSON: vm_name, guest_ip, guest_port, host_port, protocol(tcp/udp), description, target_type, public_ip_id'
const publicIPBody = 'JSON: address, cidr, gateway, iface, mac, vm_name, mode, remark, enabled 等公网 IP 配置字段'
const firewallPolicyBody = 'JSON: policy 或完整防火墙策略对象，包含 default_action, rules, region_rules, port_forward_policy 等'
const vpcSwitchBody = 'JSON: name, cidr, gateway, dhcp_start, dhcp_end, vlan_id, dns, remark, username, bridge_mode, host_interface'
const securityGroupBody = 'JSON: name, remark, username'
const securityRuleBody = 'JSON: direction, protocol, port_start, port_end, target_type(cidr/switch/security_group), target_value, action, remark'

export const endpointGroups = [
  {
    name: '公开接口',
    description: '无需登录，通常用于登录页初始化展示。',
    endpoints: [
      ep('GET', '/public/settings', '读取公开站点设置', {
        auth: publicAuth,
        response: 'data: site_title, login_background, development_mode 等公开配置。'
      })
    ]
  },
  {
    name: '认证与账户安全',
    description: '登录、邀请、找回密码、安全验证和 API Key 管理。',
    endpoints: [
      ep('POST', '/auth/login', '登录并进入 success/login_verify/bootstrap_security 阶段', {
        auth: publicAuth,
        body: 'JSON: username, password',
        response: 'data: stage, token, username, role, cloud_type, security, allowed_methods。'
      }),
      ep('GET', '/auth/invite', '读取邀请注册信息', {
        auth: publicAuth,
        query: ['token'],
        response: 'data: 邀请账号、邮箱、角色、过期状态。'
      }),
      ep('POST', '/auth/invite/complete', '完成邀请注册', {
        auth: publicAuth,
        body: 'JSON: token, password, confirm_password',
        response: 'data: stage, token, username, role, cloud_type, security。'
      }),
      ep('POST', '/auth/password/forgot', '发送旧版找回密码邮件链接', {
        auth: publicAuth,
        body: 'JSON: email',
        response: '统一成功提示，避免枚举账号。'
      }),
      ep('POST', '/auth/password/forgot/send-code', '发送忘记密码邮箱验证码', {
        auth: publicAuth,
        body: 'JSON: email',
        response: 'data: challenge_id, masked_email, expires_in。'
      }),
      ep('POST', '/auth/password/forgot/verify-code', '校验忘记密码验证码并列出账号', {
        auth: publicAuth,
        body: 'JSON: email, code, challenge_id',
        response: 'data: selection_token, accounts, email, masked_email。'
      }),
      ep('POST', '/auth/password/forgot/select-account', '选择要重置密码的账号', {
        auth: publicAuth,
        body: 'JSON: selection_token, username',
        response: 'data: reset_token, username。'
      }),
      ep('POST', '/auth/password/reset', '使用重置令牌修改密码', {
        auth: publicAuth,
        body: 'JSON: token, password, confirm_password',
        response: '密码重置成功提示。'
      }),
      ep('POST', '/auth/login/email/send', '登录阶段发送邮箱验证码', {
        auth: jwtOnly,
        body: '无',
        response: 'data: challenge_id, masked_email, expires_in。',
        notes: ['请求头必须使用 login token，不支持 API Key。']
      }),
      ep('POST', '/auth/login/verify', '完成登录阶段 TOTP/邮箱/恢复码验证', {
        auth: jwtOnly,
        body: 'JSON: method(totp/recovery/email), code, challenge_id',
        response: 'data: stage, token, username, role, cloud_type, security。',
        notes: ['请求头必须使用 login token，不支持 API Key。', 'recovery 方法传入 16 位恢复码，每个恢复码只能使用一次。']
      }),
      ep('POST', '/auth/email/code/send', '发送邮箱绑定验证码', {
        auth: jwtOnly,
        body: 'JSON: email',
        response: 'data: challenge_id, masked_email, expires_in。',
        notes: ['支持 access/bootstrap token，不支持 API Key。']
      }),
      ep('POST', '/auth/email/bind', '绑定或换绑邮箱', {
        auth: jwtOnly,
        body: 'JSON: email, code, challenge_id',
        response: 'data: security；引导完成时返回新的 access token。',
        notes: ['支持 access/bootstrap token，不支持 API Key。']
      }),
      ep('POST', '/auth/2fa/setup', '生成 TOTP 配置', {
        auth: jwtOnly,
        response: 'data: secret, otpauth_url。',
        notes: ['支持 access/bootstrap token，不支持 API Key。']
      }),
      ep('POST', '/auth/2fa/enable', '启用 TOTP 2FA', {
        auth: jwtOnly,
        body: 'JSON: secret, code',
        response: 'data: security；管理员引导完成时返回新的 access token。response 中还包含 recovery: { recovery_codes: [...] }，为 10 组一次性恢复码（仅此一次可获取）。',
        notes: ['支持 access/bootstrap token，不支持 API Key。']
      }),
      ep('POST', '/auth/2fa/disable', '关闭 TOTP 2FA', {
        auth: jwtOnly,
        body: 'JSON: password, code',
        response: 'data: security。关闭 2FA 的同时会清除所有恢复码。',
        notes: ['不支持 API Key。']
      }),
      ep('POST', '/auth/2fa/recovery/regen', '重新生成恢复码', {
        auth: jwtOnly,
        body: 'JSON: password, code',
        response: 'recovery: { recovery_codes: [...] }，旧恢复码立即失效。',
        notes: ['不支持 API Key。', '需要验证当前密码和 2FA 验证码。']
      }),
      ep('GET', '/auth/info', '读取当前用户信息', {
        response: 'data: id, username, role, cloud_type, security。'
      }),
      ep('GET', '/auth/api-key', '读取当前用户 API Key 状态', {
        response: 'data: api_key_id, key_prefix, created_at, last_used_at, enabled。'
      }),
      ep('POST', '/auth/api-key', '生成或重新生成 API Key', {
        response: 'data: api_key_id, api_key, key_prefix, created_at, enabled；api_key 仅返回一次。',
        highRisk
      }),
      ep('DELETE', '/auth/api-key', '撤销当前 API Key', { highRisk }),
      ep('PUT', '/auth/password', '修改当前账户密码', {
        body: 'JSON: old_password, new_password',
        response: '密码修改成功后需要重新登录。',
        highRisk
      }),
      ep('PUT', '/auth/username', '修改当前账户用户名', {
        body: 'JSON: new_username, password',
        response: 'data: token, username。'
      }),
      ep('POST', '/auth/high-risk/verify', '完成高风险操作二次验证', {
        body: 'JSON: method(totp/recovery/email), code, challenge_id, operation',
        response: 'data: verification_token, trusted_until。recovery 方式额外返回 recovery_codes_remaining。',
        notes: ['使用 API Key 调用敏感接口时，也需要先调用本接口。', 'recovery 方法传入 16 位恢复码。']
      })
    ]
  },
  {
    name: '系统设置',
    description: '系统级配置，管理员使用；SMTP 设置在安全初始化阶段可使用 bootstrap token。',
    endpoints: [
      ep('GET', '/settings', '读取系统设置', { notes: [admin, '支持 access/bootstrap token；API Key 仅适用于 access 模式。'] }),
      ep('PUT', '/settings', '保存系统设置', {
        body: 'JSON: template_dir, clone_dir, iso_dir, network_backend, ovs_bridge, host_ip, public_base_url, site_title, development_mode, maintenance_mode, smtp_* 等可持久化配置',
        notes: [admin],
        highRisk: '修改 development_mode、maintenance_mode、SMTP 密码等敏感项时需要二次验证'
      }),
      ep('POST', '/settings/smtp/test', '发送 SMTP 测试邮件', {
        body: 'JSON: email',
        notes: [admin]
      }),
      ep('GET', '/settings/log/status', '获取日志状态', {
        response: 'data: total_size, total_size_human, files[{name,size,mod_time,is_today,category}], categories',
        notes: [admin, '返回日志目录下所有日志文件列表及磁盘总占用大小']
      }),
      ep('POST', '/settings/log/delete', '删除日志文件', {
        body: 'JSON: files[] 文件名列表',
        response: 'data: deleted[], failed[]',
        notes: [admin, '仅允许删除 .log 和 .log.gz 文件，自动校验路径安全']
      }),
      ep('POST', '/settings/log/export', '导出日志文件', {
        body: 'JSON: files[] 文件名列表',
        response: 'application/zip 二进制流',
        notes: [admin, '将选中的日志文件打包为 ZIP 压缩包下载']
      })
    ]
  },
  {
    name: '虚拟机',
    description: 'VM 生命周期、详情、监控、网络绑定、调度、磁盘、VNC、快照和救援。',
    endpoints: [
      ep('GET', '/vm/list', '获取虚拟机列表', { query: ['keyword', 'status', 'owner 等筛选字段'], notes: [vmAccess] }),
      ep('GET', '/vm/sse', '虚拟机列表 SSE 推送', { query: ['token'], response: 'text/event-stream，推送 VM 列表变化。', notes: ['浏览器 EventSource 通常使用 token 查询参数；外部客户端可使用请求头。'] }),
      ep('GET', '/vm/:name', '获取虚拟机详情', { pathParams: ['name'], notes: [vmAccess, 'guest_agent_status: QEMU Guest Agent 状态（configured/connected/version）'] }),
      ep('GET', '/vm/:name/xml', '读取虚拟机持久化 XML', { pathParams: ['name'], response: 'data: xml 字符串。', notes: [admin, elastic, vmAccess] }),
      ep('GET', '/vm/:name/ip', '获取虚拟机 IP', { pathParams: ['name'], notes: [vmAccess] }),
      ep('GET', '/vm/:name/sse', '虚拟机详情 SSE 推送', { pathParams: ['name'], query: ['token'], response: 'text/event-stream，推送 VM 详情。', notes: [vmAccess] }),
      ep('POST', '/vm/:name/operate', '执行开机/关机/重启等操作', { pathParams: ['name'], body: 'JSON: action(start/shutdown/destroy/reboot/reset)', notes: [vmAccess] }),
      ep('PUT', '/vm/:name', '编辑虚拟机配置', { pathParams: ['name'], body: 'JSON: vcpu, ram, remark, boot_type, boot_order, bandwidth, display, apic, pae, rtc, cpu_limit_percent(仅管理员, 0-100) 等可编辑字段', notes: [elastic, vmAccess, 'remark 支持单独提交，用于独立更新虚拟机备注。', '修改 boot_type 需要虚拟机关机后执行。'] }),
      ep('PUT', '/vm/:name/xml', '保存虚拟机 XML', { pathParams: ['name'], body: 'JSON: xml', notes: [admin, elastic, vmAccess], highRisk: 'edit_vm_xml' }),
      ep('GET', '/vm/:name/stats', '读取虚拟机实时资源统计', { pathParams: ['name'], query: ['refresh'], notes: [vmAccess] }),
      ep('GET', '/vm/:name/stats/history', '读取虚拟机历史资源统计', { pathParams: ['name'], query: ['start', 'end'], notes: [vmAccess] }),
      ep('GET', '/vm/:name/schedules', '获取虚拟机定时任务', { pathParams: ['name'], notes: [elastic, vmAccess] }),
      ep('POST', '/vm/:name/schedules', '创建虚拟机定时任务', { pathParams: ['name'], body: scheduleBody, notes: [elastic, vmAccess], highRisk: '定时删除 VM 任务需要二次验证' }),
      ep('PUT', '/vm/:name/schedules/:id', '更新虚拟机定时任务', { pathParams: ['name', 'id'], body: scheduleBody, notes: [elastic, vmAccess], highRisk: '启用/修改定时删除 VM 任务需要二次验证' }),
      ep('DELETE', '/vm/:name/schedules/:id', '删除虚拟机定时任务', { pathParams: ['name', 'id'], notes: [elastic, vmAccess] }),
      ep('GET', '/vm/:name/network/status', '读取 VM OVS 网络运行状态', { pathParams: ['name'], notes: [vmAccess, '每个接口含 ip / ip_source，优先 QEMU Guest Agent'] }),
      ep('GET', '/vm/:name/network/diagnostics', '读取 VM 网络诊断信息', { pathParams: ['name'], notes: [admin, vmAccess] }),
      ep('POST', '/vm/:name/network/capture', '启动 VM 网络抓包任务', { pathParams: ['name'], body: 'JSON: interface, seconds, max_mb, max_packets, filter', notes: [admin, vmAccess], highRisk: 'network_capture' }),
      ep('GET', '/vm/:name/vpc', '读取 VM VPC 绑定', { pathParams: ['name'], notes: [vmAccess] }),
      ep('PUT', '/vm/:name/vpc', '绑定 VM 到 VPC 交换机/安全组', { pathParams: ['name'], body: 'JSON: switch_id, security_group_id', notes: [vmAccess] }),
      ep('POST', '/vm/:name/migration/preview', '预检跨节点迁移', { pathParams: ['name'], body: 'JSON: node_id, skip_precheck, target_storage_pool_id, disk_storage_targets[{target,target_storage_pool_id}], target_switch_id, target_security_group_id, enable_cpu_throttle, cpu_throttle_percent', notes: [admin, '该接口可选；源 VM 运行中自动热迁移，关机自动冷迁移；热迁移返回 live_assessment 与 preview_id。'] }),
      ep('POST', '/vm/:name/migrate', '提交跨节点迁移任务', { pathParams: ['name'], body: 'JSON: node_id, preview_id(可选), skip_precheck, target_storage_pool_id, disk_storage_targets[{target,target_storage_pool_id}], target_switch_id, target_security_group_id, enable_cpu_throttle, cpu_throttle_percent', notes: [admin, '有 preview_id 时复用预检；没有 preview_id 时任务开始后生成执行计划；迁移中 VM 状态为 migrating 并阻止用户侧操作。'], highRisk: 'migrate_vm' }),
      ep('GET', '/vm/:name/disk-migration/options', '获取本机硬盘迁移选项', { pathParams: ['name'], notes: [admin, '返回当前冷热迁移模式、可迁移硬盘和本机目标存储。'] }),
      ep('POST', '/vm/:name/disk/:dev/migrate', '提交本机硬盘迁移任务', { pathParams: ['name', 'dev'], body: 'JSON: target_storage_pool_id', notes: [admin, '运行中 VM 自动执行硬盘热迁移，关机 VM 自动执行冷迁移；成功后删除源硬盘文件。'], highRisk: 'migrate_vm_disk' }),
      ep('PUT', '/vm/:name/security-group', '切换 VM 安全组', { pathParams: ['name'], body: 'JSON: security_group_id', notes: [vmAccess] }),

      // 多网口管理
      ep('GET', '/vm/:name/interfaces', '列出 VM 所有网口', { pathParams: ['name'], notes: [admin] }),
      ep('POST', '/vm/:name/interfaces', '新增 VM 网口', { pathParams: ['name'], body: 'JSON: switch_id, security_group_id, nic_model', notes: [admin] }),
      ep('PUT', '/vm/:name/interfaces/:order', '更新 VM 指定网口', { pathParams: ['name', 'order'], body: 'JSON: switch_id, security_group_id, nic_model', notes: [admin] }),
      ep('DELETE', '/vm/:name/interfaces/:order', '删除 VM 指定网口', { pathParams: ['name', 'order'], notes: [admin] }),

      ep('DELETE', '/vm/:name', '删除虚拟机', { pathParams: ['name'], body: 'JSON: delete_disks, transfer_disks, transfer_user', notes: [elastic, vmAccess], highRisk: 'delete_vm' }),
      ep('GET', '/vm/:name/qcow2-disks', '获取 VM qcow2 磁盘列表', { pathParams: ['name'], notes: [vmAccess] }),

      // 虚拟机锁定管理
      ep('POST', '/vm/:name/lock', '锁定虚拟机', { pathParams: ['name'], notes: [elastic, vmAccess, '锁定后虚拟机无法关机或删除。'] }),
      ep('POST', '/vm/:name/unlock', '解锁虚拟机', { pathParams: ['name'], notes: [elastic, vmAccess, '解锁需要二次验证。'], highRisk: 'unlock_vm' }),
      ep('GET', '/vm/:name/lock', '获取虚拟机锁定状态', { pathParams: ['name'], notes: [vmAccess], response: 'data: { vm_name, locked, locked_at, locked_by }' }),
      ep('POST', '/vm/:name/make-independent', '将链式克隆虚拟机转为独立虚拟机', { pathParams: ['name'], notes: [admin, elastic, '仅链式克隆（有 backing file）的关机 VM 可用。将 backing chain 合并为独立磁盘镜像，脱离对模板的依赖。', '异步任务，返回 task_id 请在任务中心查看进度。'], highRisk: 'make_vm_independent' }),
      ep('POST', '/vm/create', '创建虚拟机', {
        body: vmCreateBody,
        response: 'data: task_id。创建操作为异步任务，请继续查询任务详情。',
        notes: [elastic, 'name/vcpu/ram/disk_size 为必填；name 只能包含字母和数字。', 'remark 为可选备注，会写入虚拟机元数据。', 'iso_paths 支持一次挂载多个安装 ISO，首个 ISO 会作为主安装盘。', 'extra_disks 支持为每块额外硬盘指定 storage_pool_id；普通用户会计入硬盘配额。', 'virt_type、arch、watchdog 为管理员普通创建接口支持字段。'],
        highRisk: 'create_vm',
        requiredFields: ['name', 'vcpu', 'ram', 'disk_size']
      }),
      ep('GET', '/vm/os-variants', '获取 libosinfo 系统变体列表', { response: 'data: OS variant 列表。' }),
      ep('GET', '/vm/iso-list', '获取全局 ISO 列表', { response: 'data: ISO 文件列表。' }),
      ep('POST', '/vm/clone', '从模板克隆虚拟机', { body: cloneBody, notes: [elastic], highRisk: '创建 VM 类高风险验证按现有策略触发' }),
      ep('POST', '/vm/linked-clone', '原生链式克隆虚拟机', { body: cloneBody, notes: [admin, elastic], highRisk: '创建 VM 类高风险验证按现有策略触发' }),
      ep('POST', '/vm/batch-clone', '批量克隆虚拟机', { body: 'JSON: prefix(名称前缀), start_num(起始编号), count(创建数量), template, template_type, clone_mode(linked/full), vcpu, ram, disk_size, hostname(可选), user(新建用户), password, freeze, template_root_pass, template_user, video_model, disk_bus, cpu_topology_mode, first_boot_reboot_mode, uefi', notes: [elastic] }),
      ep('POST', '/vm/:name/reinstall', '重装虚拟机', {
        pathParams: ['name'],
        body: reinstallBody,
        notes: [
          elastic,
          vmAccess,
          '仅支持弹性云 VM，保留现有 CPU、内存、网络、VPC、安全组与额外数据盘，只替换第一块系统盘。',
          '未传 disk_size 时默认使用当前系统盘大小；若小于模板原始磁盘大小，后端会自动提升到模板最小值。',
          '提交后会先自动删除该 VM 的全部快照，再强制关机并按模板类型重新执行系统初始化。',
          '若模板启动族与当前 VM 不一致（BIOS/UEFI），接口会直接拒绝。'
        ],
        highRisk: 'reinstall_vm'
      }),
      ep('GET', '/vm/:name/snapshots', '获取快照列表', { pathParams: ['name'], notes: [vmAccess] }),
      ep('DELETE', '/vm/:name/snapshots', '删除全部快照', { pathParams: ['name'], notes: [vmAccess, '按快照树从叶子节点开始删除；外部快照会尽量合并并保留当前状态；历史内部快照若已不在当前活动磁盘链，会仅清理 libvirt 元数据；完成后会清理不再被引用的 .snap_* / .snap_restore_* 残留文件。'], highRisk: 'delete_snapshot' }),
      ep('POST', '/vm/:name/snapshot', '创建快照', { pathParams: ['name'], body: 'JSON: description, include_memory, pause_for_memory_snapshot, auto_fix_nvram, name(可选)', notes: [vmAccess, '未传 name 时系统自动生成快照名称；显式名称仅支持英文、数字、下划线、点和短横线，最长 64 个字符。', '运行中创建包含内存的快照时，pause_for_memory_snapshot 默认为 true，面板会先暂停虚拟机并在快照完成后恢复运行；传 false 则不主动暂停，但 QEMU 保存内存时 VM 仍会进入 paused (saving) 状态（非面板行为，是虚拟化层固有机制）。', '内存快照耗时取决于虚拟机内存大小，大内存虚拟机可能需要数分钟。', '运行中 VM 挂载 9p/VirtFS 时不支持包含内存状态的内部快照。'] }),
      ep('POST', '/vm/:name/snapshot/:snap/revert', '恢复快照', { pathParams: ['name', 'snap'], notes: [vmAccess] }),
      ep('DELETE', '/vm/:name/snapshot/:snap', '删除快照', { pathParams: ['name', 'snap'], notes: [vmAccess], highRisk: 'delete_snapshot' }),
      ep('GET', '/vm/:name/vnc/status', '读取 VNC 状态', { pathParams: ['name'], notes: [vmAccess] }),
      ep('POST', '/vm/:name/vnc/enable', '启用 VNC', { pathParams: ['name'], body: 'JSON: password(可选)', notes: [vmAccess] }),
      ep('POST', '/vm/:name/vnc/disable', '关闭 VNC', { pathParams: ['name'], notes: [vmAccess] }),
      ep('POST', '/vm/:name/vnc/passwd', '修改 VNC 密码', { pathParams: ['name'], body: 'JSON: password', notes: [vmAccess] }),
      ep('POST', '/vm/:name/vnc/expose', '切换 VNC 对外暴露', { pathParams: ['name'], body: 'JSON: expose', notes: [vmAccess] }),
      ep('GET', '/vm/:name/vnc/ws', 'VNC WebSocket 连接', { pathParams: ['name'], query: ['token'], response: 'WebSocket 数据流。', notes: ['浏览器 WebSocket 不便自定义 API Key 请求头，建议使用 JWT token 查询参数。', vmAccess] }),
      ep('GET', '/vm/:name/monitor/status', '获取 QEMU Monitor 状态', { pathParams: ['name'], notes: [vmAccess] }),
      ep('POST', '/vm/:name/monitor/command', '执行 QEMU Monitor 命令', { pathParams: ['name'], body: 'JSON: command', notes: [vmAccess, '普通用户只开放安全命令子集。'] }),
      ep('GET', '/vm/:name/disks', '获取磁盘列表', { pathParams: ['name'], notes: [vmAccess] }),
      ep('POST', '/vm/:name/disk', '新增磁盘', { pathParams: ['name'], body: 'JSON: size_gb, bus, storage_pool_id, path', notes: [elastic, vmAccess] }),
      ep('POST', '/vm/:name/disk/:dev/resize', '扩容磁盘', { pathParams: ['name', 'dev'], body: 'JSON: size_gb', notes: [elastic, vmAccess] }),
      ep('PUT', '/vm/:name/disk/:dev/bus', '修改磁盘总线类型', { pathParams: ['name', 'dev'], body: 'JSON: bus(virtio/scsi/sata/ide)', notes: [elastic, vmAccess] }),
      ep('POST', '/vm/:name/disk/attach', '挂载已有磁盘文件', { pathParams: ['name'], body: 'JSON: path, bus', notes: [elastic, vmAccess] }),
      ep('DELETE', '/vm/:name/disk/:dev', '删除或转移磁盘', { pathParams: ['name', 'dev'], body: 'JSON: delete_file, transfer', notes: [elastic, vmAccess], highRisk: 'delete_disk_file' }),
      ep('POST', '/vm/:name/cdrom', '插入或更换 CD/DVD', { pathParams: ['name'], body: 'JSON: iso_path, device, bus', notes: [elastic, vmAccess] }),
      ep('POST', '/vm/:name/cdrom/eject', '弹出 CD/DVD', { pathParams: ['name'], query: ['device'], notes: [elastic, vmAccess] }),
      ep('DELETE', '/vm/:name/cdrom', '移除 CD/DVD 光驱', { pathParams: ['name'], query: ['device'], notes: [elastic, vmAccess] }),
      ep('POST', '/vm/:name/rescue', '启动或关闭救援系统', { pathParams: ['name'], body: 'JSON: action(enable/disable/start/stop)', notes: [vmAccess] }),
      ep('POST', '/vm/:name/password/reset', '离线重置虚拟机系统密码', { pathParams: ['name'], body: 'JSON: username, password, guest_type(linux/windows/fnos)', notes: [vmAccess], highRisk: 'reset_vm_password' }),
      ep('GET', '/vm/:name/shares', '获取共享目录列表', { pathParams: ['name'], notes: [elastic, vmAccess] }),
      ep('POST', '/vm/:name/share', '挂载共享目录', { pathParams: ['name'], body: 'JSON: host_path, tag, security_model, readonly', notes: [elastic, vmAccess] }),
      ep('DELETE', '/vm/:name/share/:tag', '移除共享目录', { pathParams: ['name', 'tag'], notes: [elastic, vmAccess] })
    ]
  },
  {
    name: '模板',
    description: '模板制作、导入导出、发布和删除。',
    endpoints: [
      ep('GET', '/template/list', '获取模板列表', { notes: [elastic] }),
      ep('POST', '/template/prepare', '制作模板', { body: 'JSON: vm_name, template_name, display_name, type, category, root_password, template_user', notes: [elastic] }),
      ep('POST', '/template/import', '兼容旧格式导入模板', { headers: [...apiHeaders.slice(0, 2), form], body: 'FormData: file, name, description', notes: [elastic] }),
      ep('POST', '/template/import/preview', '预览模板包导入', { headers: [...apiHeaders.slice(0, 2), form], body: 'FormData: file', response: 'data: token, manifest, conflicts, warnings。', notes: [elastic] }),
      ep('POST', '/template/import/confirm', '确认模板包导入', { body: 'JSON: token', notes: [elastic] }),
      ep('GET', '/template/download/:filename', '下载模板导出文件', { pathParams: ['filename'], query: ['token'], response: '文件流。', notes: [elastic, '浏览器下载通常使用 token 查询参数。'] }),
      ep('GET', '/template/:name/delete-preview', '获取模板删除影响预览', { pathParams: ['name'], response: 'data: templates, related_vms, parent_template, promoted_templates, rebased_vms, can_promote, promote_blockers, can_promote_hot, promote_hot_blockers。', notes: [elastic] }),
      ep('GET', '/template/:name/vms', '获取使用模板创建的 VM', { pathParams: ['name'], notes: [elastic] }),
      ep('POST', '/template/:name/export', '导出模板包', { pathParams: ['name'], query: ['scope(node/all)'], response: 'data: task_id 或导出任务。', notes: [elastic] }),
      ep('DELETE', '/template/:name/export', '删除模板导出文件', { pathParams: ['name'], notes: [elastic] }),
      ep('PUT', '/template/:name/publish', '更新模板发布展示状态', { pathParams: ['name'], body: 'JSON: admin_name, display_name, clone_visible, disabled, category, vcpu, ram, disk_size, disk_bus, nic_model, video_model, cpu_topology_mode, first_boot_reboot_mode', notes: [elastic] }),
      ep('PUT', '/template/:name/meta', '更新模板元数据', { pathParams: ['name'], body: 'JSON: admin_name, display_name, clone_visible, disabled, category, vcpu, ram, disk_size, disk_bus, nic_model, video_model, cpu_topology_mode, first_boot_reboot_mode', notes: [elastic] }),
      ep('DELETE', '/template/:name', '删除模板', { pathParams: ['name'], body: 'JSON: delete_mode(cascade/promote_children/promote_children_hot), delete_vms, expected_vms', notes: [elastic], highRisk: 'delete_template' })
    ]
  },
  {
    name: '网络',
    description: '静态 IP、端口转发、宿主机网桥、公网 IP 和抓包。',
    endpoints: [
      ep('GET', '/network/static-ip/list', '获取静态 IP 列表'),
      ep('POST', '/network/static-ip/bind', '绑定静态 IP', { body: 'JSON: vm_name, ip, mac, network' }),
      ep('POST', '/network/static-ip/unbind', '解绑静态 IP', { body: 'JSON: vm_name, ip', notes: [elastic] }),
      ep('GET', '/network/port-forward/list', '获取端口转发列表'),
      ep('POST', '/network/port-forward/add', '新增端口转发', { body: portForwardBody }),
      ep('PUT', '/network/port-forward/:id', '更新端口转发', { pathParams: ['id'], body: portForwardBody }),
      ep('DELETE', '/network/port-forward/:id', '删除端口转发', { pathParams: ['id'], highRisk: 'delete_port_forward' }),
      ep('DELETE', '/network/port-forward/by-key/:rule_key', '按规则 key 删除端口转发', { pathParams: ['rule_key'], highRisk: 'delete_port_forward' }),
      ep('POST', '/network/port-forward/batch-delete', '批量删除端口转发', { body: 'JSON: ids 或 rule_keys', highRisk: 'delete_port_forward' }),
      ep('POST', '/network/port-forward/save', '手动保存端口转发规则到系统', { response: '保存结果。' }),
      ep('POST', '/network/port-forward/probe/run', '立即运行端口转发 HTTP 探测', { body: 'JSON: rule_id/rule_key/vm_name 可选过滤条件' }),
      ep('GET', '/network/port-forward/whitelist/summary', '获取端口转发白名单概要', { query: ['vm_name'] }),
      ep('GET', '/network/port-forward/whitelist', '获取端口转发白名单列表', { notes: [admin] }),
      ep('POST', '/network/port-forward/whitelist/user', '新增用户白名单', { body: 'JSON: username, reason', notes: [admin] }),
      ep('DELETE', '/network/port-forward/whitelist/user/:username', '删除用户白名单', { pathParams: ['username'], notes: [admin] }),
      ep('POST', '/network/port-forward/whitelist/vm', '新增 VM 白名单', { body: 'JSON: vm_name, reason', notes: [admin] }),
      ep('DELETE', '/network/port-forward/whitelist/vm/:vm_name', '删除 VM 白名单', { pathParams: ['vm_name'], notes: [admin] }),
      ep('GET', '/network/port-forward/ip-mapping', '获取端口转发手动 IP 映射', { query: ['vm_name'] }),
      ep('POST', '/network/port-forward/ip-mapping', '新增端口转发手动 IP 映射', { body: 'JSON: vm_name, ip, remark', notes: [elastic] }),
      ep('DELETE', '/network/port-forward/ip-mapping/:id', '删除端口转发手动 IP 映射', { pathParams: ['id'], notes: [elastic], highRisk: 'delete_port_forward_ip' }),
      ep('GET', '/network/ufw/status', '读取 UFW 状态', { notes: [admin] }),
      ep('POST', '/network/ufw/rule', '管理 UFW 规则', { body: 'JSON: action, port, protocol, source, comment', notes: [admin] }),
      ep('GET', '/network/host/interfaces', '列出宿主机网卡', { notes: [admin] }),
      ep('GET', '/network/bridges', '列出宿主机网桥', { notes: [admin] }),
      ep('POST', '/network/bridges', '创建宿主机网桥', { body: 'JSON: name, interface, address, gateway, dns, mode', notes: [admin], highRisk: 'create_network_bridge' }),
      ep('DELETE', '/network/bridges/:id', '删除宿主机网桥', { pathParams: ['id'], notes: [admin], highRisk: 'delete_network_bridge' }),
      ep('GET', '/network/interfaces/:name/config', '获取接口 IP/DNS 配置', { pathParams: ['name'], notes: [admin] }),
      ep('PUT', '/network/interfaces/:name/config', '设置接口 IP/DNS 配置', { pathParams: ['name'], body: 'JSON: addrs(CIDR换行分隔), gateway, dns(空格分隔), clear(bool)', notes: [admin], highRisk: 'set_interface_config' }),
      ep('GET', '/network/public-ips', '列出公网 IP', { notes: [admin] }),
      ep('POST', '/network/public-ips', '新增公网 IP', { body: publicIPBody, notes: [admin] }),
      ep('PUT', '/network/public-ips/:id', '更新公网 IP', { pathParams: ['id'], body: publicIPBody, notes: [admin] }),
      ep('DELETE', '/network/public-ips/:id', '删除公网 IP', { pathParams: ['id'], notes: [admin], highRisk: 'delete_public_ip' }),
      ep('POST', '/network/public-ips/:id/preview', '预览公网 IP 规则', { pathParams: ['id'], body: publicIPBody, notes: [admin] }),
      ep('POST', '/network/public-ips/:id/bind', '绑定公网 IP', { pathParams: ['id'], body: 'JSON: vm_name, guest_ip, mac, mode', notes: [admin], highRisk: 'bind_public_ip' }),
      ep('POST', '/network/public-ips/:id/unbind', '解绑公网 IP', { pathParams: ['id'], notes: [admin], highRisk: 'unbind_public_ip' }),
      ep('POST', '/network/public-ips/:id/migrate', '迁移公网 IP 绑定', { pathParams: ['id'], body: 'JSON: vm_name, guest_ip, mode', notes: [admin], highRisk: 'migrate_public_ip' }),
      ep('POST', '/network/public-ips/apply', '应用公网 IP 规则', { notes: [admin], highRisk: 'apply_public_ip' }),
      ep('GET', '/network/captures/:task_id', '获取抓包会话', { pathParams: ['task_id'], notes: [admin] }),
      ep('GET', '/network/captures/:task_id/download', '下载抓包文件', { pathParams: ['task_id'], query: ['token'], response: 'pcap 文件流。', notes: [admin] }),
      ep('DELETE', '/network/captures/:task_id', '删除抓包会话文件', { pathParams: ['task_id'], notes: [admin] })
    ]
  },
  {
    name: 'VPC',
    description: 'VPC 交换机、安全组和 ACL。',
    endpoints: [
      ep('GET', '/vpc/quota', '读取 VPC 配额', { query: ['username(管理员可选)'], notes: [elastic] }),
      ep('GET', '/vpc/switches', '列出 VPC 交换机', { query: ['username(管理员可选)'] }),
      ep('POST', '/vpc/switches', '创建 VPC 交换机', { body: vpcSwitchBody, notes: [elastic] }),
      ep('PUT', '/vpc/switches/:id', '更新 VPC 交换机', { pathParams: ['id'], body: vpcSwitchBody, notes: [elastic] }),
      ep('POST', '/vpc/switches/:id/traffic/reset', '重置交换机流量统计', { pathParams: ['id'], notes: [elastic] }),
      ep('DELETE', '/vpc/switches/:id', '删除 VPC 交换机', { pathParams: ['id'], notes: [elastic] }),
      ep('GET', '/vpc/security-groups', '列出安全组', { query: ['username(管理员可选)'] }),
      ep('POST', '/vpc/security-groups', '创建安全组', { body: securityGroupBody, notes: [elastic] }),
      ep('PUT', '/vpc/security-groups/:id', '更新安全组', { pathParams: ['id'], body: securityGroupBody, notes: [elastic] }),
      ep('DELETE', '/vpc/security-groups/:id', '删除安全组', { pathParams: ['id'], notes: [elastic] }),
      ep('POST', '/vpc/security-groups/:id/rules', '新增安全组规则', { pathParams: ['id'], body: securityRuleBody }),
      ep('DELETE', '/vpc/security-groups/rules/:id', '删除安全组规则', { pathParams: ['id'] }),
      ep('GET', '/vpc/acl/preview', '预览 VPC ACL 规则', { response: 'data: ACL 预览文本或结构。' }),
      ep('POST', '/vpc/acl/apply', '应用 VPC ACL 规则', { highRisk: 'apply_vpc_acl' })
    ]
  },
  {
    name: '防火墙',
    description: 'KVM 全局防火墙和宿主机防火墙，均为管理员接口。',
    endpoints: [
      ep('GET', '/firewall/status', '读取防火墙状态', { notes: [admin] }),
      ep('GET', '/firewall/policy', '读取防火墙策略', { notes: [admin] }),
      ep('PUT', '/firewall/policy', '保存防火墙策略', { body: firewallPolicyBody, notes: [admin] }),
      ep('POST', '/firewall/preview', '预览防火墙策略', { body: firewallPolicyBody, notes: [admin] }),
      ep('POST', '/firewall/apply', '应用防火墙策略', { body: 'JSON: policy', notes: [admin], highRisk: 'apply_firewall' }),
      ep('POST', '/firewall/disable', '禁用防火墙', { notes: [admin], highRisk: 'disable_firewall' }),
      ep('POST', '/firewall/rollback', '回滚防火墙策略', { notes: [admin], highRisk: 'rollback_firewall' }),
      ep('POST', '/firewall/geoip/import', '导入地域库', { body: 'JSON 或 FormData: region 数据文件/内容', notes: [admin] }),
      ep('POST', '/firewall/geoip/update', '更新 GeoIP 数据', { body: 'JSON: source/url/path', notes: [admin] }),
      ep('PUT', '/firewall/port-forward', '设置端口转发防火墙策略', { body: 'JSON: enabled, mode, allowed_regions, blocked_regions', notes: [admin] }),
      ep('GET', '/firewall/host/status', '读取宿主机防火墙状态', { notes: [admin] }),
      ep('POST', '/firewall/host/enable/preview', '预览启用宿主机防火墙', { body: 'JSON: mode, allow_ssh, allow_panel, extra_rules', notes: [admin] }),
      ep('POST', '/firewall/host/enable', '启用宿主机防火墙', { body: 'JSON: mode, allow_ssh, allow_panel, extra_rules', notes: [admin], highRisk: 'enable_host_firewall' }),
      ep('POST', '/firewall/host/disable', '禁用宿主机防火墙', { notes: [admin], highRisk: 'disable_host_firewall' }),
      ep('GET', '/firewall/host/rules', '列出宿主机防火墙规则', { notes: [admin] }),
      ep('POST', '/firewall/host/rules', '创建宿主机防火墙规则', { body: 'JSON: name, direction, protocol, port, source, action, enabled', notes: [admin], highRisk: 'create_host_firewall_rule' }),
      ep('PUT', '/firewall/host/rules/:id', '更新宿主机防火墙规则', { pathParams: ['id'], body: 'JSON: name, direction, protocol, port, source, action, enabled', notes: [admin], highRisk: 'update_host_firewall_rule' }),
      ep('DELETE', '/firewall/host/rules/:id', '删除宿主机防火墙规则', { pathParams: ['id'], notes: [admin], highRisk: 'delete_host_firewall_rule' }),
      ep('POST', '/firewall/host/rules/vnc-default', '添加 VNC 默认防火墙规则', { notes: [admin], highRisk: 'add_host_firewall_vnc_default' }),
      ep('GET', '/firewall/host/connections/preview', '预览宿主机连接关闭影响', { query: ['mode'], notes: [admin] }),
      ep('POST', '/firewall/host/connections/close', '关闭宿主机连接', { body: 'JSON: mode, ports, exclude_current_session', notes: [admin], highRisk: 'close_host_firewall_connections' })
    ]
  },
  {
    name: 'OVS',
    description: 'OVS 网络诊断，管理员接口。',
    endpoints: [
      ep('GET', '/ovs/status', '读取 OVS 状态', { notes: [admin] }),
      ep('GET', '/ovs/ports', '读取 OVS 端口', { notes: [admin] }),
      ep('GET', '/ovs/leases', '读取 DHCP 租约', { notes: [admin] }),
      ep('POST', '/ovs/check', '检查 OVS 网络', { notes: [admin] }),
      ep('POST', '/ovs/repair', '修复 OVS 网络', { notes: [admin], highRisk: 'repair_ovs_network' })
    ]
  },
  {
    name: '存储池',
    description: '宿主机存储池、ISO 聚合和 VM 存储目标。',
    endpoints: [
      ep('GET', '/storage-pool/list', '获取存储池列表', { notes: [admin, elastic] }),
      ep('GET', '/storage-pool/all-isos', '获取所有存储池 ISO', { notes: [elastic] }),
      ep('GET', '/storage-pool/vm-targets', '获取创建 VM 可选存储目标', { notes: [elastic] }),
      ep('GET', '/storage-pool/:id', '获取存储池详情', { pathParams: ['id'], notes: [admin, elastic] }),
      ep('PUT', '/storage-pool/:id/config', '更新存储池配置', { pathParams: ['id'], body: 'JSON: name, path, type, enabled, allow_template, allow_vm, remark', notes: [admin, elastic] }),
      ep('POST', '/storage-pool/:id/default', '设置默认存储池', { pathParams: ['id'], notes: [admin, elastic] }),
      ep('POST', '/storage-pool/:id/format-mount', '格式化并挂载存储池', { pathParams: ['id'], notes: [admin, elastic], highRisk: 'format_storage_pool' })
    ]
  },
  {
    name: '节点管理',
    description: '管理员维护跨节点迁移目标节点，并由目标面板接管迁移后的 VM。',
    endpoints: [
      ep('GET', '/nodes', '获取节点列表', { notes: [admin] }),
      ep('POST', '/nodes', '添加节点', { body: 'JSON: name, api_base_url, api_key_id, api_key, ssh_host, ssh_port, ssh_user, ssh_password, enabled', notes: [admin] }),
      ep('PUT', '/nodes/:id', '更新节点', { pathParams: ['id'], body: 'JSON: name, api_base_url, api_key_id, api_key, ssh_host, ssh_port, ssh_user, ssh_password, enabled；密钥留空表示不修改', notes: [admin] }),
      ep('DELETE', '/nodes/:id', '删除节点', { pathParams: ['id'], notes: [admin] }),
      ep('POST', '/nodes/:id/probe', '探测节点能力', { pathParams: ['id'], notes: [admin] }),
      ep('GET', '/nodes/:id/migration-options', '加载 VM 迁移表单选项', { pathParams: ['id'], query: ['vm_name'], notes: [admin, '返回自动迁移模式、目标存储、目标用户处理方式；目标已有同名用户时才返回该用户下的 VPC/安全组。'] }),
      ep('POST', '/migration/adopt-vm', '目标面板接管迁移 VM', { body: 'JSON: vm_name, owner, cloud_type, target_switch_id, credential, port_forwards 等迁移接管数据', notes: [admin, '通常由源节点迁移任务调用'] })
    ]
  },
  {
    name: '用户管理',
    description: '管理员管理用户、配额、轻量云登记和 SSH。',
    endpoints: [
      ep('GET', '/user/list', '获取用户列表', { query: ['page', 'page_size', 'keyword', 'status', 'role'], notes: [admin] }),
      ep('POST', '/user', '创建用户或邀请用户', { body: 'JSON: username, email, password, role, cloud_type, quota 字段, enable_port_forward, dedicated_vpc_switch_id', notes: [admin] }),
      ep('PUT', '/user/:username/vms', '分配 VM 给用户', { pathParams: ['username'], body: 'JSON: vms, lightweight_quotas', notes: [admin] }),
      ep('POST', '/user/:username/lightweight-registrations', '登记轻量云待开通 VM', { pathParams: ['username'], body: 'JSON: registrations[]，每项包含 vm_name, quota, template, network, preserve_fnos_device_id/fnos_device_id(FnOS 可选) 等', notes: [admin] }),
      ep('PUT', '/user/:username/lightweight-vm-quota', '更新轻量云单 VM 配额', { pathParams: ['username'], body: 'JSON: vm_name, max_cpu, max_memory, max_disk, max_bandwidth_*, max_traffic_*, max_snapshots, max_runtime_hours', notes: [admin] }),
      ep('DELETE', '/user/:username/lightweight-vm/:vmName', '移除已开通轻量云 VM 注册记录', { pathParams: ['username', 'vmName'], notes: [admin] }),
      ep('DELETE', '/user/:username/lightweight-registrations/:id', '删除轻量云待开通登记', { pathParams: ['username', 'id'], notes: [admin] }),
      ep('PUT', '/user/:username/quota', '更新用户配额', { pathParams: ['username'], body: 'JSON: max_cpu, max_memory, max_disk, max_vm, max_storage, max_runtime_hours, max_port_forwards, max_snapshots, bandwidth/traffic/public_ip 配额等', notes: [admin] }),
      ep('PUT', '/user/:username/status', '封禁或解封用户', { pathParams: ['username'], body: 'JSON: status(active/disabled)', notes: [admin], highRisk: 'change_user_status' }),
      ep('GET', '/user/:username/quota', '获取用户配额使用情况', { pathParams: ['username'], notes: [admin] }),
      ep('PUT', '/user/:username/ssh', '切换用户 SSH 权限', { pathParams: ['username'], body: 'JSON: enabled', notes: [admin] }),
      ep('POST', '/user/:username/resend-invite', '重发邀请邮件', { pathParams: ['username'], notes: [admin] }),
      ep('POST', '/user/:username/traffic/reset', '重置用户流量配额', { pathParams: ['username'], notes: [admin] }),
      ep('DELETE', '/user/:username', '删除用户及其资产', { pathParams: ['username'], notes: [admin], highRisk: 'delete_user' })
    ]
  },
  {
    name: '用户自助与我的存储',
    description: '普通用户查询配额、管理自己的 VM 和存储。',
    endpoints: [
      ep('GET', '/self/quota', '查看自己的配额'),
      ep('GET', '/self/vms', '查看自己的 VM 列表', { query: ['keyword', 'status'] }),
      ep('GET', '/self/vms/sse', '自己的 VM 列表 SSE 推送', { query: ['token'], response: 'text/event-stream。' }),
      ep('GET', '/self/lightweight-registrations', '查看轻量云待确认服务器'),
      ep('POST', '/self/lightweight-registrations/:id/confirm', '确认开通轻量云服务器', { pathParams: ['id'], body: 'JSON: password, confirm_options, network/VPC 选择等' }),
      ep('POST', '/self/vm/clone', '用户自助从模板克隆 VM', { body: cloneBody, notes: [elastic] }),
      ep('POST', '/self/vm/create', '用户自助创建 VM', {
        body: selfVmCreateBody,
        response: 'data: task_id。创建操作为异步任务，请继续查询任务详情。',
        notes: [elastic, 'name/vcpu/ram/disk_size 为必填；name 只能包含字母和数字。', 'remark 为可选备注，会写入虚拟机元数据。', 'iso_paths 支持一次挂载多个安装 ISO，首个 ISO 会作为主安装盘。', '普通用户的 switch_id/security_group_id 会按当前用户 VPC 权限解析，并受配额限制。'],
        highRisk: 'create_vm',
        requiredFields: ['name', 'vcpu', 'ram', 'disk_size']
      }),
      ep('DELETE', '/self/vm/:name', '用户自助删除自己的 VM', { pathParams: ['name'], body: 'JSON: delete_disks, transfer_disks', notes: [elastic], highRisk: 'delete_vm' }),
      ep('GET', '/self/vm/:name/qcow2-disks', '获取自己的 VM qcow2 磁盘列表', { pathParams: ['name'] }),
      ep('POST', '/self/vm/export', '导出自己的 VM', { body: 'JSON: vm_name, export_name, include_snapshots, target_storage/category', notes: [elastic] }),
      ep('POST', '/self/vm/import', '导入 VM 到自己账号', { body: 'JSON: file/category/path, name, remark, vcpu, ram, switch_id, security_group_id, credentials 等', notes: [elastic] }),
      ep('GET', '/self/storage/info', '获取我的存储信息', { notes: [elastic] }),
      ep('POST', '/self/storage/init', '初始化我的存储', { notes: [elastic] }),
      ep('GET', '/self/storage/files/:category', '列出我的存储文件', { pathParams: ['category(iso/share/disk)'], notes: [elastic] }),
      ep('POST', '/self/storage/upload/:category', '上传我的存储文件', { pathParams: ['category(iso/share/disk)'], headers: [...apiHeaders.slice(0, 2), form], body: 'FormData: file', notes: [elastic] }),
      ep('DELETE', '/self/storage/file/:category/:filename', '删除我的存储文件', { pathParams: ['category', 'filename'], notes: [elastic], highRisk: 'delete_user_storage_file' }),
      ep('GET', '/self/storage/download/:category/:filename', '下载我的存储文件', { pathParams: ['category', 'filename'], query: ['token'], response: '文件流。', notes: [elastic] }),
      ep('GET', '/self/storage/isos', '获取我的 ISO 列表', { notes: [elastic] }),
      ep('GET', '/self/storage/mounts', '获取我的存储挂载列表', { notes: [elastic] }),
      ep('POST', '/self/storage/mount', '挂载我的存储到 VM', { body: 'JSON: vm_name, category(iso/share/disk), filename/tag, readonly', notes: [elastic] }),
      ep('DELETE', '/self/storage/mount/:vmName/:tag', '卸载我的存储挂载', { pathParams: ['vmName', 'tag'], notes: [elastic] })
    ]
  },
  {
    name: '宿主机',
    description: '宿主机监控和宿主机级 KVM/KSM/zRAM 参数。',
    endpoints: [
      ep('GET', '/host/stats', '读取宿主机实时统计'),
      ep('GET', '/host/stats/history', '读取宿主机历史统计', { query: ['start', 'end'] }),
      ep('GET', '/host/kvm-intel-unrestricted-guest', '读取 Intel KVM unrestricted_guest 状态', { notes: [admin] }),
      ep('PUT', '/host/kvm-intel-unrestricted-guest', '设置 Intel KVM unrestricted_guest', { body: 'JSON: enabled', notes: [admin], highRisk: 'update_kvm_unrestricted_guest' }),
      ep('GET', '/host/ksm', '读取 KSM 状态', { notes: [admin] }),
      ep('PUT', '/host/ksm', '设置 KSM 挡位', { body: 'JSON: profile(off/conservative/balanced/aggressive)', notes: [admin], highRisk: 'update_host_ksm' }),
      ep('GET', '/host/zram', '读取 zRAM 状态', { notes: [admin] }),
      ep('PUT', '/host/zram', '设置 zRAM 挡位', { body: 'JSON: profile(off/conservative/balanced/aggressive)', notes: [admin], highRisk: 'update_host_zram' })
    ]
  },
  {
    name: '任务与调度',
    description: '任务队列、任务 SSE 和调度器事件。',
    endpoints: [
      ep('GET', '/task/list', '获取任务列表', { query: ['page', 'page_size', 'status', 'type'] }),
      ep('GET', '/task/sse', '任务进度 SSE 推送', { query: ['token'], response: 'text/event-stream。' }),
      ep('GET', '/task/:id', '获取任务详情', { pathParams: ['id'] }),
      ep('POST', '/task/:id/cancel', '取消任务', { pathParams: ['id'] }),
      ep('DELETE', '/task/clear', '清理已完成任务', { highRisk: 'clear_finished_tasks' }),
      ep('GET', '/scheduler/list', '获取调度器概览', { notes: [admin] }),
      ep('GET', '/scheduler/events', '获取调度事件列表', { query: ['page', 'page_size', 'type', 'status', 'start', 'end'], notes: [admin] }),
      ep('GET', '/scheduler/events/sse', '调度事件 SSE 推送', { query: ['token'], response: 'text/event-stream。', notes: [admin] })
    ]
  }
]

export const authHeaderExamples = {
  api: apiHeaders,
  jwt: jwtHeaders,
  jwtOnly: jwtOnlyHeaders
}
