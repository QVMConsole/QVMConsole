# 调度事件页（新前端）

> 对应路由：`/scheduler`（仅管理员）
> 源码目录：`web/src/views/scheduler/`
> 旧版对照：`web-backup/src/views/scheduler/index.vue`

## 功能总览

| 功能 | 说明 |
|------|------|
| 角色限制 | 仅管理员可访问（页面内非管理员显示无权限提示；后端 `/scheduler` 路由组由 AdminMiddleware 保护） |
| 调度器概览 | 卡片网格展示已注册调度器：名称、key、启用状态标签、分组、说明、最近事件时间；SSE 收到新事件时自动刷新对应调度器的最近事件时间 |
| 事件筛选 | 调度器（下拉，选项来自概览列表）、状态（正在执行/执行完毕/执行失败）、虚拟机名称（模糊）、时间范围（Semi DatePicker dateTimeRange）；查询/重置按钮 |
| 事件列表 | 触发时间、虚拟机、调度器类型、调度状态标签、调度原因、执行结果/失败原因（省略号 + 悬停完整）、完成时间；服务端分页（10/20/50） |
| SSE 实时推送 | 页面独立连接 `/scheduler/events/sse`：新事件在第一页且命中当前筛选时头部插入；已存在的事件行原位更新（running → success/failed）；断线 5s 自动重连；页头显示连接状态徽标 |
| 密码泄露日检 | 后端调度器 `password_breach_daily` 在宿主机本地时间每天 `00:00` 提交扫描；关闭定时开关或错过午夜时不补跑 |

## 目录结构

```
web/src/views/scheduler/
├── index.tsx        # 主入口：概览卡片 + 筛选 + 事件表格 + SSE 实时合并
└── scheduler.css    # 页面样式（深空极光，浅色优先 + 深色适配）
```

相关共享模块：

- `web/src/api/scheduler.ts`：调度器概览、事件列表、SSE 连接（新建）
- `web/src/utils/format.ts`：新增 `formatDateTime` 通用日期时间格式化

## 涉及接口（均为管理员）

- `GET /scheduler/list`：调度器概览列表
- `GET /scheduler/events`：调度事件分页列表（`page/page_size/scheduler_key/status/vm_name/start/end`，时间参数为 `YYYY-MM-DD HH:mm:ss`）
- `GET /scheduler/events/sse?token=`：调度事件实时推送（`connected` / `scheduler_event` 事件，消息体 `{ action, event }`）

## 与旧版差异

1. **SSE 回调防过期**：筛选/分页状态通过 ref 提供给 SSE 回调读取，避免 React 闭包过期导致的插入判断失效（旧版 Vue 响应式天然无此问题）。
2. **概览与事件同卡片体系**：改用 `--qvm-` 设计令牌卡片，深色模式下标题降对比为柔和灰。
3. **交互语义保持**：仅第一页且命中筛选时插入新事件、事件原位更新、概览最近事件时间同步、5s 断线重连均与旧版一致。
