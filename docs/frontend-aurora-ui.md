# 深空极光前端（登录页 / 主布局 / 工作台）使用文档

> 适用范围：`web/`（React 19 + Semi Design + Zustand + Vite）
> 本轮交付：登录页、深空极光主布局、管理员工作台、普通用户工作台
> 设计来源：`plan/login-concept.html`、`plan/frontend-layout-concept.html`、`plan/user-dashboard-concept.html`（仅作布局参考，组件遵循 Semi Design 风格）

---

## 1. 设计令牌与主题

- 全局设计令牌位于 `web/src/assets/styles/aurora.css`，统一 `--qvm-` 前缀，**浅色为默认主题**，深色通过 `body[theme-mode="dark"]` 自动适配（Semi 官方方案，沿用既有 `useTheme` / 应用 Store）。
- 品牌色：霓虹青 `#2DD4BF`、电紫 `#8B5CF6`、冰蓝 `#38BDF8`；渐变 `--qvm-grad` / `--qvm-grad2`。
- 通用工具类：
  - `qvm-g-border`：渐变描边（玻璃卡片通用）
  - `qvm-panel` / `qvm-panel-card`：玻璃拟态面板
  - `qvm-num`：数字等宽字体（表格数值）
  - `qvm-fade-up` + `--qvm-delay`：入场上浮动画（支持级联）
  - `qvm-dot run/warn/off`：状态点
- 全局极光氛围背景（`qvm-aurora` + `qvm-grid-tex`）由主布局与登录页各自渲染一份。

## 2. 登录页

- 路径：`web/src/views/login/index.tsx`，样式 `login.css`。
- 背景图为 AI 生成的纯色渐变图，按主题自动切换：
  - 浅色：`web/src/assets/img/login-bg-light.png`
  - 深色：`web/src/assets/img/login-bg.png`
- 布局：左侧品牌区（logo / 标语 / 特性 / 浮动 VM 装饰卡片，≤960px 隐藏）+ 右侧登录卡片。
- 交互：勾选《用户协议》《公测协议》后登录按钮才可点击；密码可见性切换；多阶段登录（login_verify / bootstrap_security / 强制改密）保留 stage 占位提示，后续迭代接入完整流程。

## 3. 主布局

- 路径：`web/src/layout/index.tsx`，样式 `layout.css`。
- **悬浮侧边栏**（`components/Sidebar.tsx`）
  - 桌面端使用固定定位，页面内容纵向滚动时始终停留在视口左侧；主内容区根据侧边栏宽度自动预留间距。
  - 菜单按角色区分（`web/src/config/nav.tsx`），轻量云用户自动隐藏「网络」分组。
  - 「虚拟机」菜单带数量徽标与快速切换子列表（最多 4 台，状态点映射 运行/暂停/停止）。
  - 底部用户卡显示头像、角色（系统管理员 / 弹性云用户 / 轻量云用户），点击退出走 `Modal.confirm` 二次确认。
  - 未迁移模块点击时 Toast 提示「将在后续迭代提供」，不产生死链。
- **顶部标签页栏**（`components/PageTabsBar.tsx`）
  - 工作台为固定标签（不可关闭）；其余页面标签随路由自动注册（`stores/pageTabs.ts`）。
  - 支持关闭单个 / 关闭其他 / 关闭全部；关闭当前页自动回退。
- **底部任务栏**（`components/TaskBar.tsx`）
  - 点击头部展开/收起、顶部拖拽调高（46px ~ 70% 视高，状态持久化到 localStorage）。
  - 头部展示当前活动任务与进度；展开后为任务表格（类型/描述/状态/进度/消息/创建人/时间/操作）。
  - 任务详情走 `SideSheet` 抽屉（参数/结果 JSON 展示）；取消任务 `Modal.confirm` 二次确认。
  - 数据来自全局任务 Store（`stores/task.ts`）：登录后由布局启动 `/task/sse`，断线 5s 自动重连；任务类型/状态文案与配色集中维护。

## 4. 管理员工作台

- 路径：`web/src/views/dashboard/AdminDashboard.tsx`。
- 数据：`/host/stats`（首屏）+ `/host/stats/sse`（5s 推送，`hooks/useHostStatsSSE.ts`）、`/vm/list`、`/host/stats/history`（24h）、`/storage-pool/list`。
- 区块：4 统计卡 → 资源监控（ECharts 24h CPU/内存）+ 实时资源环 → 最近虚拟机（5 台）+ 存储池用量。
- **理论最大量双进度条**（`components/DualUsageBar.tsx`）
  - 含义：全部虚拟机同时 100% 满载时的资源占用合计。
    - CPU：Σ(vCPU) / 宿主机核数
    - 内存：Σ(虚拟机最大内存) / 宿主机内存
    - 磁盘：Σ(虚拟机磁盘容量) / 宿主机磁盘
  - 展示：同一轨道内当前使用量进度条在上（遮罩层），理论最大量进度条在下（斜纹半透明）；理论量可超过 100%，轨道按 max(当前, 理论, 100%) 归一化并显示 100% 刻度线。
  - 鼠标悬停（Semi Tooltip）显示当前与理论的具体数值。

## 5. 普通用户工作台

- 路径：`web/src/views/dashboard/UserDashboard.tsx`。
- 数据：`/self/quota`、`/self/vms`、`/vm/:name/stats/history`（展开时按需加载）。
- 区块：资源总览 5 卡（CPU/内存/实例/磁盘/本月时长，配额为 0 显示「不限」）→ 配额详情 3 个折叠分类（计算与实例 / 存储 / 网络资源，按云类型区分弹性云 VPC 与轻量云网桥口径）→ 我的虚拟机资源追踪（展开面板懒加载 24h 历史，渲染 CPU/内存/网络/磁盘 IO 迷你折线图，差分计算速率与 IOPS）。

## 6. 响应式与动画

- ≤1180px：侧边栏收缩为纯图标；统计卡 4→2 列；配额卡 5→3 列。
- ≤960px：图表区与底部区单列堆叠。
- ≤820px：侧边栏转抽屉（左上角菜单按钮 + 遮罩）；任务栏全宽；虚拟机 meta、配额摘要隐藏；配额卡 3→2 列；VM 迷你图单列。
- 动画：入场上浮（级联 delay）、状态点脉冲、任务栏高度缓动、进度条宽度过渡、运行中任务闪烁、骨架屏 shimmer、登录页浮动卡片漂浮。

## 7. 后续迭代约定

- 新页面路由在 `web/src/router/index.tsx` 的 `mainChildren` 追加（含 `handle.title`），页面标签会自动注册。
- 侧边栏菜单在 `web/src/config/nav.tsx` 将对应项 `coming` 去掉并补齐 `path` 即可启用。
- 页面复用样式统一走 `aurora.css` 令牌与布局类，避免再写死颜色，保证浅色/深色双主题一致。
