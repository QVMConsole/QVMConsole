# 任务中心页（新前端）

> OVF/OVA 虚拟机包导入使用独立任务类型 `import_appliance`，在筛选项和任务详情中显示为“导入虚拟机包”；普通单磁盘导入仍使用原任务类型。

> 对应路由：`/task`（所有登录用户；普通用户仅能看到自己的任务，由后端过滤）
> 源码目录：`web/src/views/task/`
> 旧版对照：`web-backup/src/views/task/index.vue`

## 功能总览

| 功能 | 说明 |
|------|------|
| 页头操作 | 刷新 / 清理已完成（存在终态任务时可用，confirmModal 确认）；SSE 连接状态徽标 |
| 筛选 | 任务状态（等待中/执行中/成功/失败/已取消）、任务类型（下拉可搜索，选项来自 `TASK_TYPE_TEXT` 映射表）；变更即重置到第一页拉取 |
| 任务列表 | ID、类型标签（`taskTypeColor` 配色）、状态标签、进度条（按状态配色 + 百分比）、状态消息（省略号 + 悬停完整）；核显活动帧缓冲导致的直通拒绝会显示排障文档链接；创建人、创建时间；服务端分页（10/20/50） |
| 行内操作 | 「纯图标 + Tooltip」模式：详情（全部任务）、取消（仅 pending/running，confirmModal 确认，运行中任务提示资源自动清理） |
| 任务详情 | 共享组件 `TaskDetailSheet` 抽屉：基础信息、任务参数/执行结果 JSON 美化、结果下载按钮（`download_path` / `extra_downloads`，经 `getTemplateExportDownloadUrl` 附带 token 打开） |
| 实时进度 | **不重复建 SSE**：复用全局任务 Store（`stores/task.ts`，主布局登录后启动）的 SSE；订阅 Store 任务变化后按 id 合并进本页列表；第一页且无筛选时发现新任务自动刷新；详情抽屉打开时同步进度，任务到达终态自动补拉完整详情 |
| 执行结果通知 | 全局 SSE 收到任务成功或失败终态事件时，通过 Semi `Notification` 弹出任务类型和任务 ID；成功展示 5 秒，失败展示 8 秒并携带后端状态消息。核显活动帧缓冲导致的直通拒绝在消息后附带排障文档链接。同一登录会话以「任务 ID + 终态」去重，SSE 断线重连不会重复提示，登出重置后清空去重记录。 |
| 密码泄露任务 | `password_breach_scan` 展示完整扫描，`password_breach_notify` 展示登录后首次通知重试；扫描失败可在详情中查看中文错误 |

## 目录结构

```
web/src/views/task/
├── index.tsx    # 主入口：筛选 + 表格 + 分页 + Store 增量合并
└── task.css     # 页面样式（深空极光，浅色优先 + 深色适配）
```

相关共享模块：

- `web/src/components/business/TaskDetailSheet.tsx`：任务详情抽屉（新建，底部任务栏 `TaskBar` 与任务中心页共用；含结果下载能力）
- `web/src/stores/task.ts`：任务类型文案改为导出 `TASK_TYPE_TEXT` 映射表（筛选选项与展示共用）
- `web/src/layout/components/TaskBar.tsx`：详情抽屉替换为共享组件；「完整任务中心」按钮改为跳转 `/task`

## 涉及接口

- `GET /task/list`：任务分页列表（`page/page_size/status/type`）
- `GET /task/:id`：任务详情（含 params/result）
- `POST /task/:id/cancel`：取消任务
- `DELETE /task/clear`：清理已完成/失败/已取消任务
- `GET /task/sse?token=`：任务进度实时推送（全局 Store 统一连接，页面不直连）

## 与旧版差异

1. **单一 SSE 连接**：旧版任务页自建 EventSource 与全局任务栏重复连接；新版页面订阅全局任务 Store 增量合并，全站仅一条任务 SSE。
2. **详情组件复用**：任务详情抽屉抽取为 `TaskDetailSheet`，任务栏与任务中心共用，并统一补上了旧版仅任务页才有的结果下载按钮。
3. **行内操作图标化**：旧版「详情/取消」文字按钮改为纯图标 + Tooltip，符合项目行内操作规范。
4. **交互语义保持**：取消确认文案、清理确认、终态补拉详情、新任务刷新列表等行为与旧版一致。

## 开发调试

Vite 开发模式下，可在浏览器控制台调用 `window.__qvmDebugShowError(message)` 模拟全局请求失败 Toast。该函数仅在开发构建中存在，不会请求后端或修改任务数据。
