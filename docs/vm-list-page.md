# 虚拟机列表页（新前端）

> 对应路由：`/vm`
> 源码目录：`web/src/views/vm/`
> 设计稿：`plan/vm-list-concept.html`（布局参考，实际遵循 Semi Design 风格）

## 功能总览

| 功能 | 说明 |
|------|------|
| 实时刷新 | 进入页面即建立 SSE 长连接（管理员 `/vm/sse`，用户 `/self/vms/sse`），列表实时推送更新；断线 5 秒自动重连。无独立「自动刷新」开关 |
| 缓存优先 | 从其他页面返回时先渲染 Zustand 缓存，SSE/HTTP 静默更新，避免转圈 |
| 双视图 | 表格 / 卡片视图切换，选择持久化到 localStorage（`vmListViewMode`），默认表格 |
| 标签 | 表格与卡片视图均提供标签字段；可直接输入并按回车或点击加号添加，点击标签关闭图标可移除。标签持久化在 libvirt 虚拟机元数据中，缓存仅作列表投影；每台最多 20 个标签、单个最长 32 个字符。 |
| 搜索与标签筛选 | 工具栏搜索框按 名称 / 备注 / 模板 / 标签 模糊过滤（客户端），可一键清空；标签多选框按所选标签交集筛选，支持在候选标签中搜索；统计行显示「匹配 N 台」。 |
| 分组视图 | 列表头右侧「全部 / 按状态 / 按模板 / 自定义」分组切换（localStorage `vmListGroupBy` 持久化，默认全部平铺）。分组模式下按组分块渲染（表格/卡片均支持），组头可折叠、显示数量标签，支持「全选本组」与跨组多选；组内顺序沿用当前排序，不分页 |
| 列头排序 | 「名称」「配置 (资源使用)」「IP 地址」三列点击表头切换升/降序，工具栏右侧显示当前排序依据 |
| 详情入口 | 表格视图点击任意非交互区域的虚拟机行、卡片视图点击任意非交互区域的虚拟机卡片，均进入详情页；勾选框和行内操作保持原行为。 |
| 资源进度条 | 配置列下方实时显示 CPU（青色渐变）/ MEM（紫色渐变）百分比；非运行状态显示灰色空条与状态标注 |
| 状态/操作图标化 | 状态列与操作列均为纯图标，悬停 Tooltip 显示文案 |
| IP 地址直显 | 列表随 SSE 直接下发 IP（`include_ip=1`），未分配时显示「未分配」 |
| 批量电源 | 勾选后工具栏「批量电源」可用：开机 / 重启 / 关机 / 强制断电 / 删除（危险操作带二次确认，含锁定机批量提醒） |
| 新建虚拟机 | 全屏弹窗向导支持 ISO 安装 / 模板克隆（含批量）/ 导入已有磁盘 / 导入 OVF-OVA 虚拟机包四种方式，与详情页编辑表单共用 `features/vm-form` 同一套模型与规则 |
| 单机操作 | 控制台（占位）、电源（按状态自动切换 开机/关机/继续启动）、更多菜单 |
| 维护模式 | 系统维护模式下页面内容虚化并弹出维护提示 |

## 更多菜单（⋯）

按角色与云类型裁剪显示：

- 重置（仅暂停态）/ 重启 / 强制断电（仅运行态）
- 编辑备注、编辑分组（轻量云隐藏）
- 制作模板（仅管理员）
- 导出虚拟机（轻量云隐藏）：可选兼容 QCOW2 系统盘或标准 OVA；OVA 要求关机，系统盘固定、数据盘可选，结果计入我的存储配额
- 重装系统（轻量云隐藏，含模板选择 / 系统盘大小 / 主机名 / 凭据 / FnOS 设备 ID）
- 迁移（仅管理员，含「迁移虚拟机」跨节点预检与「迁移硬盘」本机换存储）
- 转为独立虚拟机（仅管理员 + 链式克隆 + 关机态）
- 启动 / 关闭救援系统
- 锁定 / 解除锁定（轻量云隐藏；解锁走 428 高风险二次验证，由请求层自动处理）
- 删除（轻量云隐藏；单台可勾选删除磁盘、未勾选磁盘转移到「我的存储」；支持批量）

## 轻量云待开通面板

轻量云用户列表上方展示「待开通服务器」卡片（管理员登记的配置），点击「确认开通」补全登录用户名/密码（内置随机强密码 + HIBP 泄露检测）后提交开通任务。

## 响应式

| 断点 | 行为 |
|------|------|
| ≤1180px | 隐藏「模板」列 |
| ≤960px | 隐藏「IP 地址」「运行时长」列，工具栏换行，搜索框独占一行 |
| ≤820px | 隐藏勾选列、排序指示、已选统计；卡片视图单列 |

## 目录结构

```
web/src/views/vm/
├── index.tsx                    # 主入口：SSE/搜索/排序/分组/分页/选择/电源与批量/菜单分发/维护模式
├── vm.css                       # 页面样式（深空极光，浅色优先 + 深色适配）
├── utils.ts                     # 状态文案/容量解析/电源操作文案/分组构建 buildVmGroups（模板分类工具已上移至 web/src/utils/templateCategory.ts）
├── components/
│   ├── VmToolbar.tsx            # 批量电源下拉 + 新建 + 搜索框 + 排序指示
│   ├── VmTableView.tsx          # 表格视图（Semi Table，受控排序/选择）
│   ├── VmCardView.tsx           # 卡片视图
│   ├── VmGroupedView.tsx        # 分组视图（组折叠/组内全选，内部复用表格/卡片视图）
│   ├── VmStatusIcon.tsx         # 状态图标 + Tooltip
│   ├── VmResourceBars.tsx       # 配置 + CPU/MEM 双进度条
│   ├── VmIpCell.tsx             # IP 地址单元格（直显）
│   ├── VmActionsCell.tsx        # 行操作图标 + 更多下拉
│   ├── VmIcons.tsx              # 自定义电源图标（Semi 图标集无电源图标）
│   └── PendingRegistrations.tsx # 轻量云待开通面板
└── dialogs/
    ├── VmDeleteDialog.tsx       # 删除（单台磁盘选择/批量）
    ├── VmRemarkDialog.tsx       # 编辑备注
    ├── VmGroupDialog.tsx        # 编辑分组（可创建新分组）
    ├── MakeTemplateDialog.tsx   # 制作模板（含「不初始化」风险确认）
    ├── VmReinstallDialog.tsx    # 重装系统
    ├── VmMigrationDialog.tsx    # 迁移虚拟机（节点/存储/网络/预检/提交）
    ├── VmExportDialog.tsx       # QCOW2 / 标准 OVA 导出与磁盘选择
    └── DiskMigrationPanel.tsx   # 迁移硬盘（迁移弹窗子面板）
```

## 涉及接口

- `GET /vm/list`、`GET /self/vms`、`GET /vm/sse`、`GET /self/vms/sse`
- `POST /vm/:name/operate`（start/shutdown/reboot/destroy/reset）
- `PUT /vm/:name`（备注/分组/标签）、`GET /vm/:name/ip`、`GET /vm/:name/qcow2-disks`
- `DELETE /vm/:name`、`DELETE /self/vm/:name`
- `POST /vm/:name/lock|unlock|rescue|make-independent|reinstall`
- `GET /self/vm/:name/export-options`、`POST /self/vm/export`、`GET /self/storage/info`
- `GET /template/list`、`POST /template/prepare`
- `GET /nodes`、`GET /nodes/:id/migration-options`、`POST /vm/:name/migration/preview`、`POST /vm/:name/migrate`
- `GET /vm/:name/disk-migration/options`、`POST /vm/:name/disk/:dev/migrate`
- `GET /self/lightweight-registrations`、`POST /self/lightweight-registrations/:id/confirm`

## 本轮未迁移（入口占位）

- 控制台（VNC）：P2 模块，点击占位提示
- 快照管理、网络管理：归属详情页迭代，更多菜单不再包含

> 新建虚拟机已由 `features/vm-form/CreateVmWizard` 提供（详见 `docs/vm-create-edit-form.md`）；
> 详情页与编辑表单已分别由 `views/vm/detail/` 与 `features/vm-form/EditVmForm` 提供。
