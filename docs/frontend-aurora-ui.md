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
- 交互：勾选《用户协议》《公测协议》后登录按钮才可点击；密码可见性切换；多阶段登录（强制改密 / 安全初始化 / 登录二次验证）已全部接入。
- **登录二次验证**（`web/src/views/login/LoginVerifyPanel.tsx`）：登录返回 `stage=login_verify` 时右侧卡片切换为验证面板，持 login 令牌（15 分钟）调 `/auth/login/verify`；多方式时 RadioGroup 按钮切换（管理员仅 2FA 动态码 / 恢复码，普通用户为邮箱验证码，绑定 2FA 后可选动态码 / 恢复码），邮箱方式需先点「发送邮箱验证码」（`/auth/login/email/send`，Banner 显示掩码邮箱），恢复码方式为 16 位输入并提示一次性失效；验证通过返回完整登录态直接进入系统，「返回登录」放弃验证。后端配套修复：已绑定 2FA 的管理员即使曾跳过安全初始化（`bootstrap_skipped`）也强制登录验证（`service/security/account.go` 的 `NeedsLoginVerification`）。
- **强制修改默认密码**（`web/src/views/login/ForcePasswordModal.tsx`）：登录返回 `force_password_change` 时仅写入临时 token（后端中间件只放行改密/登出等白名单接口）并弹出改密弹窗；当前密码预填登录密码，新密码经本地弱密码快速检测 + HIBP 泄露检测（后端 k-匿名）后提交 `PUT /auth/password`（此场景后端跳过 428 高风险验证）；改密成功后旧 token 随 `security_updated_at` 更新立即失效，前端用新密码自动重新登录并进入系统；点击「退出登录」则清除临时会话回到登录表单。
- **安全初始化**（`web/src/views/login/BootstrapSecurityPanel.tsx`）：登录返回 `stage=bootstrap_security` 时右侧卡片切换为引导面板（520px、内部滚动），全程持登录返回的 bootstrap 令牌（30 分钟）调用接口，不写入用户 Store：
  - 管理员依次呈现三个区块：SMTP 配置（未配置时显示，先「发送测试邮件」验证通过后才出现「保存 SMTP」，配置以 `getSettings/updateSettings/testSMTP` 携带 stage 令牌读写）→ 绑定邮箱（发送验证码 + 绑定，SMTP 未配置时禁用并提示）→ 绑定 2FA（生成配置渲染二维码 → 输入动态码启用）；普通用户仅显示绑定邮箱。
  - 后端判定全部安全要求完成时（`/auth/email/bind`、`/auth/2fa/enable` 在 bootstrap 令牌下）直接返回完整登录态（stage=success + access token），前端应用会话进入系统；启用 2FA 返回的一次性恢复码复用安全中心的 `RecoveryCodesModal`（样式已抽出为组件自带 `recovery-codes.css`），确认「我已安全保存」后才应用会话。
  - 管理员可「跳过安全设置」：风险确认弹窗（Modal.confirm 危险色）后调 `POST /auth/skip-bootstrap`，同样返回完整登录态；「返回登录」放弃本次初始化回到登录表单。
  - 相关 API（`api/auth.ts` 的 `sendEmailCode/bindEmail/setup2FA/enable2FA` 与 `api/settings.ts` 的 `getSettings/updateSettings/testSMTP`）均支持可选 `stageToken` 参数，未传时仍走请求层默认注入，安全中心等既有调用不受影响。

## 3. 主布局

- 路径：`web/src/layout/index.tsx`，样式 `layout.css`。
- **贴边侧边栏**（`components/Sidebar.tsx`）
  - 满高贴左（经典贴边布局），与顶部导航栏通过描边线无缝衔接；主内容区根据侧边栏宽度自动预留间距。
  - 顶部 Logo 区：`web/public/favicon.png` 图标 + 站点名 + 副标题；折叠时仅保留图标。
  - **手动折叠**：右缘悬浮圆形开关（chevron 随状态旋转），折叠后为纯图标模式（74px），菜单项悬停显示 Semi Tooltip 菜单名；折叠偏好持久化到 localStorage（`sidebar_collapsed`），未记录偏好时 ≤1180px 首次进入默认折叠。
  - **图标彩色悬停**：每个菜单项在 `web/src/config/nav.tsx` 的 `NAV_COLORS` 配置专属颜色，悬停时图标点亮为彩色并带微光（CSS 变量 `--nav-ic` 注入）。
  - 菜单按角色区分（`web/src/config/nav.tsx`），轻量云用户自动隐藏「网络」分组。
  - 用户入口已移至顶部导航栏最右侧，显示头像、账号和角色（系统管理员 / 弹性云用户 / 轻量云用户）；下拉菜单提供「安全中心」及经 `Modal.confirm` 二次确认的「退出登录」。
  - 未迁移模块点击时 Toast 提示「将在后续迭代提供」，不产生死链。
- **顶部导航栏**（`components/TopBar.tsx`）
  - 固定贴顶（54px），左起于侧边栏右缘并随折叠联动平移，与侧边栏无缝衔接；右上角固定保留用户入口。
  - 承载历史页面标签栏（`components/PageTabsBar.tsx`）；左侧为小屏菜单按钮（≤820px 显示）；右侧依次为「开源版」GitHub 链接（小屏仅留图标）、赞助支持入口、深色/浅色主题切换按钮（全站可用，`useTheme`），并保留 `extra` 扩展插槽（后续可放搜索、通知等）。
  - **赞助支持入口**（`components/SponsorWidget.tsx`）：纯图标 + Tooltip 按钮，点击弹出下拉菜单（前往赞助 / 查看权益内容，外链集中维护在 `config/constants.ts` 的 `EXTERNAL_LINKS`）；赞助支持弹窗首次访问当天不弹、次日起自动弹出，关闭后 7 天冷却（localStorage：`sponsor_first_visit` / `sponsor_last_closed`，与旧版前端键位一致），弹出后 5 秒倒计时结束才可关闭；弹窗内含项目介绍、赞助者权益列表与两个跳转按钮，深色模式下标题文字柔化为 #b8c1cf。
  - 工作台为固定标签（不可关闭）；其余页面标签随路由自动注册（`stores/pageTabs.ts`）；支持关闭单个标签，关闭当前页自动回退（「标签操作」批量关闭下拉已移除）。
- **底部任务栏**（`components/TaskBar.tsx`）
  - 点击头部展开/收起、顶部拖拽调高（46px ~ 70% 视高，状态持久化到 localStorage）。
  - 头部展示当前活动任务与进度；展开后为任务表格（类型/描述/状态/进度/消息/创建人/时间/操作）。
  - 任务详情走 `SideSheet` 抽屉（参数/结果 JSON 展示）；取消任务 `Modal.confirm` 二次确认。
  - 数据来自全局任务 Store（`stores/task.ts`）：登录后由布局启动 `/task/sse`，断线 5s 自动重连；任务类型/状态文案与配色集中维护。

## 4. 管理员工作台

- 路径：`web/src/views/dashboard/AdminDashboard.tsx`。
- 数据：`/host/stats`（首屏）+ `/host/stats/sse`（5s 推送，`hooks/useHostStatsSSE.ts`）、`/vm/list`、`/host/stats/history`（历史查询按需）。
- 区块：状态横幅 → 4 统计卡 → 宿主机资源监控四图 → 最近虚拟机（5 台）。
- **宿主机状态横幅**（`components/HostStatusBanner.tsx`）
  - 两种状态：正常（青色，「宿主机运行正常，各项指标处于健康区间」）/ 警告（琥珀色，列出具体原因）。
  - 警告触发条件：CPU 使用率 ≥ 90%、内存使用率 ≥ 90%、存储剩余 < 10 GB，或任一可写挂载点可用空间 < 10 GiB；单盘检测排除只读文件系统、`/boot`、`/boot/efi`，多个原因用顿号并列展示；此外系统未配置 SMTP 时同样触发警告（「您当前没有设置 SMTP，请尽快前往系统设置进行设置」，与负载类原因用分号分句并列）。
  - 负载数据来自宿主机 SSE 实时推送（5s）；挂载盘空间通过 `/host/disks` 每 5 秒刷新，状态自动切换。SMTP 配置状态来自安全状态（进入概览页时调 `/auth/info` 刷新用户 Store 的 security，避免登录后配置变更不同步）；首屏数据未到达时不渲染避免闪烁。
- 顶部问候行（`components/TopLine.tsx`）仅保留问候语 + 状态摘要；原「全局搜索 / 新建虚拟机 / 通知中心 / 任务中心」入口已移除，主题切换上移至顶部导航栏。
- **内存使用率卡片 KSM / zRAM 体现**（`hooks/useHostMemOptimize.ts`）
  - 系统设置开启 KSM 且宿主机支持时，内存卡片进度条下方显示文本行：`KSM 节省 xx`（青色高亮），悬停 Tooltip 同步展示 KSM / zRAM 明细（zRAM 含已用/容量与压缩算法）。
  - zRAM 开启时进度条增加第三条区段（琥珀斜纹）：右端对齐物理内存 100% 位置从后往前，表示物理内存使用进入 zRAM 压缩交换的阶段区间；图例随区段显隐。
  - KSM 节省 = 被共享页 × 4KB（与旧前端口径一致），随宿主机 SSE 实时刷新；zRAM 状态走 `/host/ksm`、`/host/zram`（管理员接口），60s 轮询刷新。
  - 未开启或不支持时区段与文本行自动隐藏，不占卡片空间。
- **CPU / 内存卡片硬件详情折叠区**（`components/StatExpandToggle.tsx` + `CpuDetailPanel.tsx` + `MemModulesPanel.tsx`）
  - 两张卡片底部均有「硬件详情」折叠开关（文本 + 旋转 chevron），**默认收起**；收起即卸载面板、停止请求。
  - CPU 展开：型号 + 插槽/物理核心/线程数 + 每核使用率色块网格（每核一个 12px 色块按使用率着色：<10% 淡青 / <40% 青 / <70% 蓝 / <90% 琥珀 / ≥90% 红，悬停 Semi Tooltip 显示「核心 N：xx%」，核数多时自动换行保持紧凑）；展开期间每 3s 轮询 `GET /host/cpu/hardware`（管理员接口，后端基于 /proc/stat 与上次调用样本差分计算每核使用率，静态型号信息进程内缓存）。
  - 内存展开：已插 x / 总插槽 y + 每根内存条行（插槽位置 / 容量 / 类型·频率·厂商）；静态硬件信息展开时加载一次 `GET /host/memory/modules`（后端 dmidecode 解析 + 进程内缓存）；dmidecode 不可用或 SMBIOS 无数据（常见于虚拟机/部分 ARM 设备）时展示后端返回的中文说明。
- **宿主机资源监控**（`components/HostMonitorCharts.tsx`）
  - 四图：CPU 使用率 / 内存使用率 / 网络流量（接收/发送）/ 磁盘 I/O（读取/写入），ECharts 渲染，配色跟随明暗主题。
  - 实时监控：由宿主机 SSE（5s）驱动逐点追加，最多保留 60 个点；网络与磁盘速率基于累计字节数增量计算（KB/s，首点跳过）。
  - 历史查询：日期范围选择 + 「近 24 小时」快捷查询，走 `/host/stats/history`（后端每 60s 持久化一条记录），速率按相邻记录差分计算并做异常尖峰防抖。
  - 缩放：滚轮/触控板 inside dataZoom + 底部 14px 滑块框选，滑块配色适配明暗主题。
- **理论最大量双进度条**（`components/DualUsageBar.tsx`）
  - CPU 的含义：运行中虚拟机同时 100% 满载时的资源占用合计。内存理论值还会加入系统当前占用：先以宿主机已用内存扣除运行中虚拟机当前分配内存得到系统基线，再加上运行中虚拟机的最大内存；虚拟机统计缓存尚未就绪时，以运行中虚拟机配置内存估算当前分配量，避免首次加载重复计入。已停止、暂停和迁移中的虚拟机不计入 CPU、内存理论值。磁盘容量不受运行状态影响，仍统计全部虚拟机。
    - CPU：Σ(运行中虚拟机 vCPU) / 宿主机核数
    - 内存：(系统当前占用 + Σ(运行中虚拟机最大内存)) / 宿主机内存
    - 磁盘：Σ(全部虚拟机磁盘容量) / 宿主机磁盘
  - 展示：同一轨道内当前使用量进度条在上（遮罩层），理论最大量进度条在下（斜纹半透明）；理论量可超过 100%，轨道按 max(当前, 理论, 100%) 归一化并显示 100% 刻度线。
  - 鼠标悬停（Semi Tooltip）显示当前与理论的具体数值。

## 5. 普通用户工作台

- 路径：`web/src/views/dashboard/UserDashboard.tsx`。
- 数据：`/self/quota`、`/self/vms`、`/vm/:name/stats/history`（展开时按需加载）。
- 区块：资源总览 5 卡（CPU/内存/实例/磁盘/本月时长，配额为 0 显示「不限」）→ 配额详情 3 个折叠分类（计算与实例 / 存储 / 网络资源，按云类型区分弹性云 VPC 与轻量云网桥口径）→ 我的虚拟机资源追踪（展开面板懒加载 24h 历史，渲染 CPU/内存/网络/磁盘 IO 迷你折线图，差分计算速率与 IOPS）。

## 6. 响应式与动画

- 侧边栏收缩不再依赖断点，改为手动折叠开关（≤1180px 首次进入且无偏好记录时默认折叠）；统计卡 ≤1180px 4→2 列；配额卡 5→3 列。
- ≤960px：资源监控四图与底部区单列堆叠。
- ≤820px：侧边栏转抽屉（顶部导航栏左侧菜单按钮 + 遮罩，折叠开关隐藏且抽屉恢复完整菜单）；顶栏铺满全宽；任务栏全宽；虚拟机 meta、配额摘要隐藏；配额卡 3→2 列；VM 迷你图单列。
- 动画：入场上浮（级联 delay）、状态点脉冲、侧边栏宽度/顶栏位置缓动、任务栏高度缓动、进度条宽度过渡、运行中任务闪烁、骨架屏 shimmer、登录页浮动卡片漂浮。

## 7. 后续迭代约定

- 新页面路由在 `web/src/router/index.tsx` 的 `mainChildren` 追加（含 `handle.title`），页面标签会自动注册。
- 侧边栏菜单在 `web/src/config/nav.tsx` 将对应项 `coming` 去掉并补齐 `path` 即可启用。
- 页面复用样式统一走 `aurora.css` 令牌与布局类，避免再写死颜色，保证浅色/深色双主题一致。
