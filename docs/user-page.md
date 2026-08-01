# 用户管理页（/user）

> 新前端（React + Semi Design）重构版本，迁移自旧前端 `web-backup/src/views/user/index.vue`（Vue + Element Plus）。
> 仅管理员可访问，非管理员进入时显示无权限提示。

## 文件结构

```
web/src/views/user/
├── index.tsx                          # 页面主体：筛选、表格、行内操作、弹窗调度
├── user.css                           # 页面样式（--qvm- 设计令牌，含深色模式降对比）
├── utils.ts                           # 状态/云类型标签映射、配额百分比、单 VM 配额默认值等
├── components/
│   ├── QuotaOverviewCell.tsx          # 紧凑配额单元格（迷你进度条 + 悬浮完整明细）
│   ├── QuotaFormFields.tsx            # 用户级配额表单（创建/编辑共用，支持使用量展示）
│   └── LightweightQuotaTable.tsx      # 轻量云单 VM 配额编辑表格（三处弹窗复用）
└── dialogs/
    ├── CreateUserDialog.tsx           # 新增用户
    ├── EditQuotaDialog.tsx            # 编辑账户资料与用户配置
    ├── AssignVmDialog.tsx             # 分配 VM（弹性云）
    ├── RegistrationDialog.tsx         # 轻量云注册 VM / 分配已有 VM 管理
    ├── RegistrationQuotaDialog.tsx    # 编辑轻量云单 VM 配额
    └── vpcOption.ts                   # NAT VPC 过滤与下拉选项文案（共用）
```

相关扩展：

- `web/src/api/user.ts`：补齐用户管理接口（列表完整类型 `UserListItem`、`UserQuotaUsage`、创建/删除/配额/状态/分配/SSH/流量重置/重发邀请/轻量云注册系列接口）。
- `web/src/router/index.tsx`：新增 `/user` 路由；`web/src/config/nav.tsx`：「用户管理」菜单取消 `coming` 占位。

## 页面设计

### 列表（紧凑配额方案）

旧版 17 个独立配额列改为紧凑展示（经确认采用「紧凑配额单元格」方案）：

| 列 | 说明 |
| --- | --- |
| 用户 | 用户名 + 邮箱（次要文字） |
| 角色/类型 | 管理员/普通用户 + 弹性云/轻量云 Tag |
| 状态 | 正常 / 待激活 / 已封禁 |
| 配额 | 弹性云：CPU/内存/磁盘/VM 四条迷你进度条 + 限速/时长耗尽 Tag，悬浮 Popover 查看快照、端口转发、公网 IP、存储、运行时长、上下行流量、带宽完整明细；轻量云：`单 VM 配额 × N`，悬浮查看每台 VM 配额摘要；管理员：仅存储条 |
| 虚拟机 | 前 5 台 Tag + `+N`；轻量云用户追加注册状态 Tag（待确认/开通中/已开通/失败） |
| SSH | Switch 开关（仅普通用户、状态正常时可操作，失败自动回滚） |
| 操作 | 高频「配置」图标外露 + ⋯ 下拉（分配/注册 VM、重发邀请、重置流量、封禁/解封、删除），遵循「纯图标 + Tooltip」规范 |

筛选：用户名、邮箱、角色、状态、用户类型；前端分页每页 100 条。

### 新增用户

- 邮箱为选填；初始密码留空时必须填写邮箱，并在 SMTP 已配置时发送邀请邮件，由用户自行完成注册。
- 填写初始密码：不论 SMTP 是否配置、是否填写邮箱，均直接创建已激活、可登录的用户，不再发送注册邀请。
- 普通用户未设置邮箱时跳过邮箱二段登录；邮件通知、找回密码及基于邮箱的高风险验证需在后续绑定邮箱后使用。
- 初始密码旁提供「随机」按钮，会同时生成密码和确认密码；手工输入与随机密码都接入本地弱密码检测和后端泄露检测（`checkPasswordBreachAsync`）。
- 角色为管理员：仅存储配额（默认 0 不限）；普通用户默认存储配额 10GB。
- 弹性云用户：完整用户级配额表单（计算/存储/运行时长/端口转发/公网 IP/快照/带宽/流量）。
- 轻量云用户：
  - `选择已有 VM`：多选未被占用的 VM，并为每台设置单 VM 配额（LightweightQuotaTable）；
  - `注册新 VM`：先选专用 NAT VPC，再通过 **创建虚拟机向导的轻量云登记模式**（`CreateVmWizard` 的 `registration` + `onDraft`）添加注册草稿，提交后随邀请流程开通。
  - 分配专用 VPC 时允许复用并自动修复该 VM 既有的 VM 级安全组归属，即使历史安全组用户名为空；管理员 NAT VPC 归属到交换机用户，系统基础网络归属到 VM 实际用户，仅限当前轻量云用户绑定的专用 NAT VPC。

### 编辑用户配置

- 邮箱为选填，可直接修改或清空；新密码同样为选填，留空时保持原密码，填写时需二次确认并通过密码泄露检测。
- 为「待激活」用户设置新密码后，该账户会直接激活并完成系统用户资源初始化，原邀请链接随即失效。
- 管理员：仅存储配额（附当前使用量）。
- 普通用户：可切换弹性云/轻量云；弹性云展示完整配额表单及使用量进度；轻量云需选择专用 VPC，计算配额改由单 VM 配额管理。

### 轻量云注册管理（注册 VM 入口）

- 列表合并三类行：已保存注册项、仅有单 VM 配额的已开通 VM（`quota_only`）、本地草稿（未保存）。
- 支持：添加注册草稿（复用创建向导）、编辑单 VM 配额（草稿本地改，已保存走 `PUT /user/:name/lightweight-vm-quota` 即时生效）、删除待注册项；已开通 VM（包括仅配额行）点击「删除」后可选择仅移除分配，或同时删除虚拟机及其磁盘。后者走删除任务和 `delete_vm` 二次验证，只有物理删除成功后才清理注册记录、单 VM 配额和访问授权。
- 「分配已有 VM」模式与创建用户时一致，提交 `PUT /user/:name/vms` 附带 `lightweight_quotas`。

### 行为约束（与旧版一致）

- 管理员不能编辑自己的配置、不能被分配 VM；内置 `admin` 账号与当前登录账号不可删除/封禁。
- 重置流量仅在弹性云用户已限速（`is_limited_down/up`）时可用。
- 删除用户、封禁、创建用户等敏感操作由请求层统一处理 428 高危二次验证。
- 删除/封禁为任务队列异步操作，提交后延迟约 2 秒刷新列表。

## 后端接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | /api/user/list | 用户列表（含配额使用、轻量云注册/配额） |
| POST | /api/user | 创建用户 |
| PUT | /api/user/:username/account | 更新用户邮箱和密码；为待邀请用户设置密码时直接激活 |
| DELETE | /api/user/:username | 删除用户（任务） |
| PUT | /api/user/:username/quota | 更新配额 |
| GET | /api/user/:username/quota | 配额使用情况 |
| PUT | /api/user/:username/status | 封禁/解封（任务） |
| PUT | /api/user/:username/vms | 分配 VM |
| PUT | /api/user/:username/ssh | SSH 开关 |
| POST | /api/user/:username/traffic/reset | 重置流量 |
| POST | /api/user/:username/resend-invite | 重发邀请 |
| POST | /api/user/:username/lightweight-registrations | 登记待开通 VM |
| DELETE | /api/user/:username/lightweight-registrations/:id | 删除注册项 |
| DELETE | /api/user/:username/lightweight-vm/:vmName | 移除已开通 VM 记录 |
| POST | /api/user/:username/lightweight-vm/:vmName/delete | 删除已开通轻量云 VM（任务） |
| PUT | /api/user/:username/lightweight-vm-quota | 更新单 VM 配额 |

## 深色模式

页面标题、用户名、配额明细标题、面板标题等大面积文字在 `body[theme-mode='dark']` 下降低对比为柔和灰 `#b8c1cf`，浅色模式仍使用设计令牌。
