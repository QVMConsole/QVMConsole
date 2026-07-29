# 防火墙页（新前端）

> 对应路由：`/firewall`（仅管理员）
> 源码目录：`web/src/views/firewall/`
> 旧版对照：`web-backup/src/views/firewall/index.vue`

## 功能总览

| 功能 | 说明 |
|------|------|
| 角色限制 | 仅管理员可访问（页面内非管理员显示无权限提示，与公网 IP 页一致；后端接口由 AdminMiddleware 保护） |
| 页头操作 | 刷新（按当前 Tab 加载对应数据） |
| 标签页 | 宿主机防火墙 / KVM 网络防火墙 / 连接管理；首次切到 KVM Tab 才加载其状态与策略 |

### 宿主机防火墙（UFW）

| 功能 | 说明 |
|------|------|
| 状态横幅 | 已启用（青）/已关闭（橙）；开启（先预览推荐规则再确认）/关闭（高风险确认，任务队列） |
| 运行状态卡 | UFW 可用性、入站/出站/转发默认策略、SSH 端口、面板端口、Docker 兼容说明 |
| 规则表 | 动作/协议/端口区间/来源 CIDR/备注；保护规则（SSH、面板端口）标红且禁止编辑删除；面板管理规则置灰标签 |
| 筛选 | 端口搜索、协议筛选（TCP/UDP）、动作筛选（允许/拒绝）、备注搜索 |
| 规则操作 | 行内「编辑/删除」纯图标 + Tooltip（保护行禁用）；添加规则弹窗支持 TCP/UDP/TCP+UDP 与端口区间；添加 VNC 5900-5999 默认放通 |
| 启用确认弹窗 | 展示推荐规则（SSH/面板保护 + 端口转发放通），保护行禁止编辑，确认后提交任务队列启用 |

### KVM 网络防火墙（nftables）

| 功能 | 说明 |
|------|------|
| 操作条 | 预览规则 / 保存策略 / 应用规则 / 禁用 / 回滚（后三者为高风险确认，走任务队列） |
| 状态横幅 | 规则已生效（青）/规则未应用（蓝） |
| 全局策略 | 虚拟网桥、虚拟机网段、出站/入站区域限制（内嵌单字符状态的 TextSwitch + 区域多选）、禁用 VM IPv6、拦截动作（reject/drop）、白名单 CIDR（每行一个） |
| 区域数据 | 下载源地址、更新区域代码（逗号分隔，在线更新走任务队列）、本地导入（代码/名称/来源/CIDR 列表）；区域表格（代码/名称/CIDR 数/更新时间）；GeoIP 版权说明 |
| VM 覆盖策略 | 每台 VM 独立管控模式（继承全局/关闭管控/仅允许入站/仅允许区域/阻断区域）；仅 allow/block 模式可选区域；已删除 VM 的残留覆盖条目在加载时自动剔除 |

### 连接管理

| 功能 | 说明 |
|------|------|
| 非防火墙端口 | 预览/关闭本地端口不在 UFW 允许规则内的 TCP 已建立连接 |
| 全部连接 | 预览/关闭所有 TCP 已建立连接（含 SSH 与面板，二次确认中展示将关闭的连接数） |
| 连接预览 | 协议、本地地址、对端地址、是否命中防火墙放行端口；后端返回的 warning 单独展示 |

## 目录结构

```
web/src/views/firewall/
├── index.tsx                        # 主入口：页头/Tab 容器/数据加载/全部操作与弹窗分发
├── firewall.css                     # 页面样式（深空极光，浅色优先 + 深色适配）
├── utils.ts                         # 默认策略/规则工厂、端口格式化、区域选项、VM 覆盖归一化
├── components/
│   ├── HostFirewallTab.tsx          # 宿主机防火墙（状态横幅 + 运行状态卡 + 规则表）
│   ├── KvmFirewallTab.tsx           # KVM 网络防火墙（操作条 + 全局策略 + 区域数据 + VM 覆盖）
│   └── ConnectionsTab.tsx           # 连接管理（预览/关闭连接）
└── dialogs/
    ├── EnableHostFirewallDialog.tsx # 启用宿主机防火墙确认（可编辑推荐规则表）
    ├── HostRuleDialog.tsx           # 添加/编辑宿主机规则
    └── ImportRegionDialog.tsx       # 导入区域 CIDR
```

相关共享模块：

- `web/src/api/firewall.ts`：KVM 策略/状态、GeoIP 导入更新、宿主机状态/启用/规则 CRUD、连接预览关闭接口（新建）
- `web/src/features/vm-form/sections/TextSwitch.tsx`：带内部单字符状态文字的共享开关

## 涉及接口（均为管理员）

- `GET /firewall/status`、`GET/PUT /firewall/policy`：KVM 网络防火墙状态与策略
- `POST /firewall/preview`：预览策略生成的 nftables 规则文本
- `POST /firewall/apply`、`/disable`、`/rollback`：应用/禁用/回滚（高风险 428 二次验证，任务队列）
- `POST /firewall/geoip/import`、`/geoip/update`：区域 CIDR 本地导入 / 在线更新（后者任务队列）
- `GET /firewall/host/status`、`POST /firewall/host/enable/preview`、`/enable`、`/disable`：宿主机防火墙状态与启停（启停高风险，任务队列）
- `POST/PUT/DELETE /firewall/host/rules[/:id]`、`POST /firewall/host/rules/vnc-default`：宿主机规则 CRUD 与 VNC 默认放通（高风险）
- `GET /firewall/host/connections/preview`、`POST /firewall/host/connections/close`：连接预览与关闭（关闭高风险）

## 与旧版差异

1. **启用弹窗数据源修正**：旧版读取预览接口的 `rules` 字段（当前 UFW 已持久化规则），在全新系统上弹窗为空，无法完成"确认 SSH 和面板端口"步骤；新版改用预览接口专门计算的 `recommended_rules`（SSH/面板保护 + 端口转发放通），`rules` 仅作兜底。
2. **行内操作收折**：旧版规则表"编辑/删除"为文字按钮，新版按「纯图标 + Tooltip」规范实现，保护行禁用态置灰并附原因提示。
3. **开关组件统一**：所有开关均使用内部单字符状态文字，通用状态为“开/关”。
4. **VM 覆盖脏数据清理**：加载策略时自动剔除已不存在 VM 的覆盖条目，避免残留数据随保存提交（旧版仅追加不清理）。
5. **任务后延迟刷新**：应用/禁用/回滚/启停/GeoIP 更新等任务队列操作提交后延迟 1.2s 刷新状态，与公网 IP 页策略一致（旧版不刷新）。
6. **布局响应式**：旧版 `el-row :span` 固定分栏，新版 `Row/Col` 响应式断点（xs 24 / md 9-15 / lg 8-16、14-10），小屏自动堆叠；连接管理双操作组小屏转为纵向排列。
