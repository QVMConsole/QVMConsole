# 节点管理页（/nodes）

> 新前端（React + Semi Design）实现，迁移自旧前端 `web-backup/src/views/node/index.vue`。仅管理员可见。

## 功能概述

节点管理用于维护**跨节点迁移**的目标节点连接信息，每个节点包含两条通道：

- **面板 API 通道**：目标面板地址 + 管理员 API ID/Key，用于目标面板接管虚拟机、查询迁移选项等；
- **SSH 通道**：SSH 地址/端口 + root 用户，用于磁盘数据传输、libvirt/OVS 检查等底层操作。后端强制 SSH 用户必须为 `root`，非 root 用户会在保存、探测和迁移预检阶段被阻断。

SSH 认证支持两种方式（新增字段 `ssh_key_auth`）：

- **密码认证**（默认）：root 密码由后端加密存储（`SSHPasswordEnc`），接口不回传明文；
- **SSH 密钥认证**：面板**不保存任何密钥/私钥内容**，用户自行在系统中配置免密登录（将面板所在系统的公钥追加到目标节点 root 的 `~/.ssh/authorized_keys`），面板仅做免密连通性检测。可选的 `ssh_key_path` 指定面板所在系统的本机私钥路径，留空使用默认迁移密钥 `/root/.ssh/id_ed25519`。

## 页面功能

| 功能 | 说明 |
| --- | --- |
| 节点列表 | 名称 / 面板 API 地址 / SSH（user@host:port）/ 状态标签（在线-绿 / 异常-红 / 未知-灰）/ 最近探测消息与时间 / 启用状态 |
| 筛选 | 名称搜索、状态筛选（online/error/unknown）、启用状态筛选；筛选变化自动重置分页 |
| 添加/编辑节点 | 弹窗表单：名称、面板 API 地址、API ID、API Key、SSH 地址/端口、固定 root 用户、**SSH 认证方式（密码 / SSH 密钥，密钥方式可填可选私钥路径）**、root 密码（仅密码认证显示）、启用开关（TextSwitch）；**编辑时 API Key 与 root 密码留空表示不修改**，但历史非 root 节点改回 root 时需要重新输入 root 密码，密钥认证切换为密码认证时必须输入密码 |
| **保存即探测（先探测后保存）** | 点击「保存」按钮后按钮进入转圈等待状态，后端在写入数据库**之前**先执行完整连接探测：SSH + 面板 API 双通道校验全部通过才真正保存入库并关闭弹窗；探测失败（如 SSH 密码错误、密钥未配置、面板 API 不可达）则**不保存**，弹出错误弹窗展示失败原因，表单保留可修改，**必须解决连接问题后再次点击保存，探测通过才能成功**，避免节点入库后到实际迁移任务时才暴露连接问题（如热迁移线路测速报 Permission denied） |
| 探测节点 | 行内「探测」图标（IconPulse），行级 loading（IconRefresh spin），120s 超时；成功 Toast 探测消息，无论成败均刷新列表同步状态 |
| 删除节点 | ⋯ 下拉菜单内危险项，二次确认后删除 |
| 移动端适配 | ≤768px 隐藏表格，切换卡片视图（复用同一套行内操作区） |

## 项目规范落实

- **密码泄露检测（规范 22）**：保存时对 root 密码执行本地弱密码检测（`validatePassword`）+ 后端 HIBP k-匿名检测（`checkPasswordBreachAsync`）。由于 root 密码属于既有服务器凭据，检测到泄露时**不强制阻断**，弹出危险确认（建议尽快更换）后仍可继续保存；SSH 密钥认证不涉及密码，跳过检测。
- **行内操作图标化（规范 28）**：高频「探测」图标外露 + Tooltip，编辑/删除收进 ⋯ 下拉菜单（Dropdown trigger="click" position="bottomRight"，删除为 danger 项）。
- **Switch 内嵌单字符状态（规范 29）**：启用开关使用共享组件 `features/vm-form/sections/TextSwitch.tsx`。
- **深色模式（规范 18/27）**：页头标题与节点名称在 `body[theme-mode='dark']` 下降对比为柔和灰。

## 代码位置

| 文件 | 说明 |
| --- | --- |
| `web/src/api/node.ts` | 节点管理 API 封装（列表/创建/更新/删除/探测），类型对应后端 `HostNodeView` / `HostNodeRequest` |
| `web/src/views/node/index.tsx` | 页面主体（列表、筛选、分页、移动卡片、行内操作） |
| `web/src/views/node/dialogs/NodeDialog.tsx` | 添加/编辑弹窗（含密码泄露检测） |
| `web/src/views/node/node.css` | 页面样式（--qvm- 设计令牌，浅色优先 + 深色适配） |
| `web/src/router/index.tsx` + `pages.tsx` | 路由 `/nodes`（懒加载） |
| `web/src/config/nav.tsx` | 侧边栏「系统」分组新增「节点管理」（IconServerStroked，仅管理员导航） |

## 后端接口

均为既有接口（`server/handler/node_migration.go`，管理员组 `/api/nodes`；本轮扩展了节点 SSH 密钥认证字段，并将创建/更新改为**先探测后保存**）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/nodes` | 节点列表 |
| POST | `/api/nodes` | 创建节点（新增 `ssh_key_auth` / `ssh_key_path` 字段；密钥认证时无需 `ssh_password`；**先探测连接，探测通过才创建，失败返回 400 且不落库**） |
| PUT | `/api/nodes/:id` | 更新节点（api_key / ssh_password 缺省表示不修改；密钥认证切换回密码认证时必须提供密码；**先探测连接，探测通过才更新，失败返回 400 且不改动原节点**） |
| DELETE | `/api/nodes/:id` | 删除节点 |
| POST | `/api/nodes/:id/probe` | 探测节点（API + root SSH 双通道校验，前端 120s 超时；SSH 密钥认证节点探测免密连通性，失败时会附带公钥配置提示） |

后端实现要点：

- `server/model/host_node.go`：`HostNode` 新增 `SSHKeyAuth` / `SSHKeyPath` 字段（AutoMigrate 自动建列，无需手工迁移）；
- `server/service/host/node.go`：探测核心提取为 `probeNode`（`persist` 控制是否落库）；`CreateHostNode` / `UpdateHostNode` 先调用 `probeNode(node, false)` 预检，失败直接返回错误不写库，成功才 `Create`/`Save`（节点带上预检得到的 online 状态）；
- `server/service/remote_exec.go`：`remoteSSHExec` / `RemoteRsyncFile` 在密钥认证时改用 `ssh -i <keyPath> -o BatchMode=yes` 免密执行（不再使用 `sshpass`），密钥文件不存在时返回中文错误提示；默认密钥路径 `/root/.ssh/id_ed25519` 与迁移时 `EnsureDefaultSSHKeyTrusted` 保持一致；
- 创建/更新失败响应携带 `data`（节点视图，含 `last_probe_message`），前端弹窗可直接展示失败原因。

跨节点迁移选项接口（`GET /api/nodes/:id/migration-options`）由 `web/src/api/migration.ts` 维护；若目标节点 SSH 用户不是 `root`，接口直接返回错误，避免迁移任务进入 rsync 后再因目标存储目录权限不足失败。
