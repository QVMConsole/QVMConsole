# 网络中心页（新前端）

> 对应路由：`/network`（管理员标题「网络中心」，普通用户标题「VPC 网络」）
> 源码目录：`web/src/views/network/`
> 旧版对照：`web-backup/src/views/network/index.vue`

## 功能总览

| 功能 | 说明 |
|------|------|
| 角色差异 | 管理员 4 个 Tab（网络概览/交换机/安全组策略/ACL）；普通用户（弹性云）2 个 Tab（交换机/安全组策略）；轻量云用户由路由守卫拦截，不可访问 |
| 按用户筛选 | 管理员页头输入框，影响交换机/安全组/配额的查询参数（`username`） |
| 网络概览（管理员） | OVS 状态/网桥/端口数/内网 CIDR 统计卡；检测（同步刷新状态）、修复（高风险确认，异步任务）；端口镜像多来源/多目标配置、分源与分目标计数、故障状态，状态刷新期间显示加载提示并锁定操作；端口安全总开关、预检结果、默认折叠的逐端口状态与协调/隔离操作；基础状态 + 服务状态信息卡；宿主机网桥表、物理网卡表、OVS 端口表 |
| 宿主机网桥 | 网络概览保留历史网桥展示、删除与接口 IP 配置；新的物理上行统一在创建交换机时选择，不再提供独立“创建桥接网桥”入口 |
| OVS NAT 出口协调 | `KVM_OVS_UPLINK` 可继续保存物理上联口；若该物理口已加入 OVS 网桥且默认路由已迁移到网桥，运行态 NAT/FORWARD 会自动改用实际三层网桥，并清理指向旧出口的同网段规则 |
| 接口 IP/DNS 配置 | 展示当前 IPv4/IPv6/网关/DNS；编辑保存或一键清除；物理网卡已加入网桥时提示改为在网桥上配置；不可配置接口禁用表单 |
| 交换机 | 直接管理零或一个上行链路，明确显示“空交换机 / 物理直通 / 内置 DHCP/NAT / 系统基础网络”；普通用户自行创建时固定为空交换机；历史用户 NAT 交换机显示待迁移状态并由管理员逐台迁移 |
| 交换机表单 | 一步选择物理上行和内置 DHCP。无上行时为独立纯二层；物理上行且 DHCP 关闭时显示 VLAN、桥接安全与宿主机 IP 迁移；DHCP 开启时显示上行网关、CIDR、内部网关和地址池。自动检测不到出口默认路由时可填写上行网关；已有直通网桥的物理口既可作为托管 NAT 出口，也可由使用不同非零 VLAN ID 的直通交换机共享。关闭 DHCP 会保留最近一次托管网段，后续可复用 |
| 在线重配置 | 拓扑变化提交 `vpc_switch_reconfigure` 异步任务；任务按网口保留 MAC、型号、interface ID 与带宽 XML，热插失败时恢复已经处理的网口和旧运行态；行级任务期间更多按钮显示旋转图标 |
| 交换机操作 | 行内只保留“查看虚拟机”图标；编辑、历史迁移、重置流量和删除收入 `⋯` 菜单。系统基础网络只读；物理上行、在线切换和宿主机网络变化保留二次验证 |
| 安全组策略 | 名称/类型搜索；行展开内联管理规则（方向/IP 版本/协议/端口范围/目标/备注/删除）；创建/编辑（默认组名称不可改）；删除（默认组受保护禁用） |
| 安全组规则 | IP 版本（IPv4/IPv6）；方向（入站/出站）；协议（IPv4：TCP/UDP/ICMP/全部，IPv6：TCP/UDP/ICMPv6/全部）；端口（单端口/范围/全端口，ICMP、ICMPv6 与全部协议固定 0-0）；目标类型（CIDR/IP、指定交换机、指定安全组，仅允许选择当前用户可见资源） |
| ACL（管理员） | nftables 规则预览（代码块 + 复制，HTTP 场景剪贴板降级）；应用 ACL（高风险确认后重建防火墙规则，428 二次验证由请求层自动处理） |

## 目录结构

```
web/src/views/network/
├── index.tsx                        # 主入口：角色分支/Tab 容器/数据加载/删除与确认操作/弹窗分发
├── network.css                      # 页面样式（深空极光，浅色优先 + 深色适配）
├── utils.ts                         # 格式化函数（流量/带宽/桥接模式/规则端口与目标文案）
├── components/
│   ├── OverviewTab.tsx              # 网络概览（统计卡/状态卡/网桥表/物理网卡表/OVS 端口表）
│   ├── SwitchesTab.tsx              # 交换机（配额摘要 + 表格 + 搜索分页）
│   ├── SecurityGroupsTab.tsx        # 安全组（表格 + 展开规则管理）
│   └── AclTab.tsx                   # ACL（预览/应用/复制）
└── dialogs/
    ├── SwitchDialog.tsx             # 创建/编辑交换机
    ├── SwitchVMsDialog.tsx          # 交换机虚拟机列表
    ├── BridgeDialog.tsx             # 历史桥接接口兼容组件（网络概览不再提供创建入口）
    ├── InterfaceConfigDialog.tsx    # 接口 IP/DNS 配置
    ├── SecurityGroupDialog.tsx      # 创建/编辑安全组
    └── RuleDialog.tsx               # 添加安全组规则
```

相关共享模块：

- `web/src/api/ovs.ts`：OVS 检测/修复（新建）
- `web/src/api/network.ts`：新增宿主机网桥、物理网卡、接口 IP/DNS 配置接口
- `web/src/api/vpc.ts`：新增 VPC 配额、交换机 CRUD/流量重置/VM 查询、安全组 CRUD、ACL 预览/应用接口
- `web/src/api/user.ts`：新增管理员用户列表接口（`GET /user/list`）

## 涉及接口

- `POST /ovs/check`、`POST /ovs/repair`：OVS 网络检测与修复（管理员）
- `GET /ovs/port-security/status`、`POST /ovs/port-security/preflight|enable|disable|reconcile`：端口安全状态、预检与异步启停/协调
- `POST /ovs/port-security/ports/:port/isolate|release`：异步隔离或释放端口（高风险操作保留二次验证）
- `GET/POST /network/bridges`、`DELETE /network/bridges/:id`：历史宿主机网桥兼容接口（管理员）
- `GET /ovs/port-mirror/options|status`、`POST /ovs/port-mirror/enable|disable`：端口镜像选项、运行态与异步启停；启停保留二次验证
- `GET /network/host/interfaces`：物理网卡列表，包含直通占用、NAT 使用数量、有效三层接口、网关及可选状态（管理员）
- `GET/PUT /network/interfaces/:name/config`：接口 IP/DNS 配置（管理员，支持 IPv4 + IPv6 双栈）
- `GET /vpc/quota`：流量/带宽配额
- `GET/POST /vpc/switches`、`PUT/DELETE /vpc/switches/:id`、`POST /vpc/switches/:id/traffic/reset`、`GET /vpc/switches/:id/vms`：交换机管理
- `POST /vpc/switches/:id/reconfigure`：异步重配置完整目标拓扑，返回 `task_id/status`；支持 API Key 并保留二次验证
- `GET/POST /vpc/security-groups`、`PUT/DELETE /vpc/security-groups/:id`、`POST /vpc/security-groups/:id/rules`、`DELETE /vpc/security-groups/rules/:id`：安全组与规则管理
- `GET /vpc/acl/preview`、`POST /vpc/acl/apply`：ACL 预览与应用（应用为高风险操作，428 二次验证）
- `GET /user/list`：用户选项（管理员）

## 接口 IP/DNS 配置 IPv6 支持

1. 接口配置弹窗同时支持 IPv4 和 IPv6 双栈编辑，分两个区块填写；DNS 服务器可混合 IPv4/IPv6 地址。
2. IPv4 和 IPv6 至少填写一个地址才可保存；某地址族留空时保留该族现有地址，不会误清除。
3. 独立物理网卡的静态配置通过 systemd-networkd `.network` 文件持久化；有 IPv6 地址时保留链路本地地址（IPv6 NDP 依赖），仅禁用 RA 自动配置。
4. 面板管理网桥（已启用宿主机 IP 迁移）的 IPv6 配置持久化到数据库 `host_addrs6`/`host_gateway6`/`host_metric6` 字段，并在网桥恢复脚本中以静态变量恢复。
5. 网桥创建/删除时的 IP 迁移同步处理 IPv4 和 IPv6：创建时从物理口捕获双栈地址并迁移到网桥；删除时将双栈地址回迁至物理口。
6. 清除配置会同时移除 IPv4 和 IPv6 的所有静态地址、路由和 DNS。

## IPv6 安全组规则

1. 添加规则时先选择 `IPv6`，CIDR 默认切换为 `::/0`，协议列表提供 TCP、UDP、ICMPv6 和全部协议。
2. CIDR/IP 目标必须与所选 IP 版本一致；后端也会校验地址族，避免把 IPv4 来源写入 IPv6 规则。
3. 指定交换机或指定安全组作为来源时，ACL 只提取所选地址族的成员地址。IPv6 成员使用精确 `/128`，IPv4 成员使用精确 `/32`。
4. IPv6 ICMP 规则编译为 nftables `ipv6-icmp`，覆盖回显及 IPv6 必需的控制报文；TCP/UDP 继续按单端口或端口范围下发。
5. 历史规则未保存 `address_family` 时保持兼容：IPv6 CIDR 和 ICMPv6 会自动识别为 IPv6，其余旧规则沿用 IPv4 语义。

新增安全组规则的请求示例：

```json
{
  "direction": "ingress",
  "address_family": "ipv6",
  "protocol": "tcp",
  "port_start": 443,
  "port_end": 443,
  "target_type": "cidr",
  "target_value": "::/0",
  "remark": "允许 IPv6 HTTPS"
}
```

## 与旧版差异

1. **端口转发 Tab 未迁移**：旧版网络中心的端口转发（建站扫描/封禁管理/白名单）功能已从产品中移除，新版不再包含该 Tab；用户自助端口转发仍在虚拟机详情页网络 Tab 中。
2. **移动端适配简化**：旧版每个表格配一套移动端卡片，新版统一为响应式表格（小屏横向滚动），减少重复代码。
3. **Tab 组件化**：旧版 3199 行单文件拆分为 4 个 Tab 组件 + 6 个对话框，单文件均控制在 300 行左右。
4. **数据加载策略保持旧版语义**：进入页面加载交换机 + 安全组 + 当前 Tab 数据；切换「概览」「ACL」时按需加载；管理员用户筛选变化触发全量重载。

## 建站扫描功能移除说明（后端）

端口转发 HTTP 探测（建站扫描）功能已整体从后端移除：

- **删除代码**：`server/service/network/probe/` 包（扫描/定时调度/白名单/状态同步）、`server/service/port_forward_probe_wire.go`、`server/model/port_forward_probe_state.go`、`server/model/port_forward_whitelist.go`
- **移除接口**：`POST /network/port-forward/probe/run`、`DELETE /network/port-forward/by-key/:rule_key`、`GET/POST/DELETE /network/port-forward/whitelist*` 系列
- **移除启动项**：`main.go` 中的探测定时调度启动与 `port_forward_http_probe_manual` 任务类型注册
- **移除配置**：`port_forward_http_probe_enabled/interval_minutes/timeout_seconds` 配置项与 `install.sh` 中的对应环境变量
- **数据兼容**：`port_forward_whitelist`、`port_forward_probe_state` 两张历史表保留不删（仅从 AutoMigrate 移除）；端口转发规则结构体同步移除 `live/banned/probe_*` 字段，列表接口仅返回 iptables 实时规则
- **前端同步**：虚拟机详情页端口转发面板移除「探测」按钮、「封禁」状态列与白名单横幅；`web/src/api/network.ts` 移除探测/白名单/按 rule_key 删除接口
