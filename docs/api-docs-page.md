# API 文档页（自动生成接口清单）

> 路由：`/api-docs`　源码：`web/src/views/api-docs/`　所有登录角色可访问（含轻量云用户）

## 功能概述

API 文档页展示面板全部 HTTP 接口的调用说明，供外部程序使用 API Key 集成时查阅。与旧前端**手工维护接口清单**不同，新版接口清单在**构建时从后端源码自动解析生成**，后端新增接口后前端重新构建即可自动同步，不再需要手动添加。

页面功能：

- 认证与响应说明卡片（API Key 请求头示例、统一 JSON 响应结构、一键复制认证示例）
- 接口检索：关键字搜索（路径/说明/字段）、模块筛选、只看二次验证接口
- 按模块分组的接口折叠列表：方法标签、路径、中文摘要、权限标签（二次验证/管理员/轻量云不可用）
- 展开详情：认证方式、请求头、路径/查询参数、请求体与返回说明、请求/返回字段解释表、可复制的 curl 示例
- 高风险操作说明卡片（428 → `/auth/high-risk/verify` → `X-High-Risk-Token` 流程）

## 自动同步机制

### 生成脚本

`web/scripts/generate-api-endpoints.mjs`，通过 `predev` / `prebuild` 钩子在 `npm run dev`、`npm run build` 前自动执行，也可手动运行 `npm run gen:api`。

脚本静态解析后端源码（**不修改任何后端代码**）：

| 解析目标 | 提取内容 |
|---------|---------|
| `server/router/router.go` | 路由分组前缀、请求方法、路径、分组/行内中间件链、handler 名、行尾注释 |
| `server/handler/*.go` | 各 handler 函数体内 `requireHighRiskVerification(c, "op")` 调用 → 高风险操作标识 |

中间件链推导出的元数据：

- `AdminMiddleware` → 管理员接口标签
- `ElasticCloudOnlyMiddleware` → 轻量云不可用标签
- `VMAccessMiddleware` → VM 归属校验
- `AuthMiddleware` / `TokenTypeMiddleware` / `JWTTokenTypeMiddleware` → 认证方式（公开 / JWT+API Key / 仅 JWT / 仅登录阶段 token）

输出文件：`web/src/views/api-docs/generated/endpoints.json`（按方法+路径排序，diff 稳定，已入库）。

容错：若构建环境没有 `server/` 目录，脚本会沿用已入库的历史 `endpoints.json` 并告警；两者都没有时构建报错。

### 中文文案补充

自动解析拿不到中文摘要与请求体说明，这部分维护在：

- `web/src/views/api-docs/endpointDescriptions.ts` —— 按 `"METHOD /path"` 索引的补充描述（摘要/请求体/返回/查询参数/备注/必填字段），并定义模块分组（路径前缀 → 中文模块名）
- `web/src/views/api-docs/fieldDictionary.ts` —— 请求/返回字段的中文解释字典

**新接口即使没写文案也会自动出现在文档页**：摘要回退为 router.go 行尾注释或 handler 函数名，并带「待补充文案」标签，方便后续认领补齐。

OVF/OVA 功能新增的检查、导入和导出选项接口已由生成脚本纳入清单，并在 `endpointDescriptions.ts` 中补充 `ApplianceMetadata`、`config_mode` 导入策略与 `format/disk_devices` 字段说明。创建向导直接提交异步导入任务，只读检查接口保留给 API 调试和独立预览。

### 后端新增接口时的工作流

1. 后端在 `router.go` 正常注册路由（建议行尾写中文注释，会被自动用作摘要兜底）
2. 前端重新 `npm run dev` / `npm run build`，接口自动进入文档页
3. （可选）在 `endpointDescriptions.ts` 中补充该接口的中文摘要与请求体说明
4. （可选）新增路由组时在 `endpointDescriptions.ts` 的 `moduleGroups` 中补充分组映射，否则归入「其他」

## 文件结构

```
web/scripts/generate-api-endpoints.mjs   # 构建时生成脚本
web/src/views/api-docs/
├── generated/endpoints.json             # 自动生成的接口清单（勿手工编辑）
├── endpointDescriptions.ts              # 人工补充中文文案 + 模块分组
├── fieldDictionary.ts                   # 字段中文解释字典
├── docUtils.ts                          # 合并/字段提取/curl 构建工具
├── EndpointDetail.tsx                   # 接口展开详情组件
├── index.tsx                            # 页面主入口（检索/分组渲染）
└── api-docs.css                         # 样式（含深色模式柔和灰覆盖）
```

## 关于项目页

> 路由：`/about`　源码：`web/src/views/about/`

迁移自旧前端关于页，四个折叠区块：

- **技术栈**：React 19 / Semi Design / Vite / Zustand / Go / Gin / SQLite / libvirt / QEMU-KVM / noVNC（默认折叠，点击展开后可跳转官网）
- **项目信息**：开源地址、开发者、官网、文档链接
- **面板信息**：版本 / 构建时间 / 站点名称（`GET /public/version`）、运行模式
- **系统信息**：操作系统 / 内核 / 架构 / 主机名 / CPU 核数 / Go / QEMU / libvirt 版本 / 运行时间（`GET /system-info`）

配套在 `web/src/api/settings.ts` 补充了 `getPublicVersion()`。

## 其他说明

- 两个页面均已注册路由（`web/src/router/index.tsx`）与侧边栏「支持」分组入口（`web/src/config/nav.tsx`），轻量云用户路由白名单原本就包含 `/api-docs` 与 `/about`
- 复制功能使用 `copyTextWithFallback`，HTTP 场景自动降级 `execCommand`
- 深色模式下大面积标题/正文通过 `body[theme-mode='dark']` 覆盖为柔和灰（#b8c1cf），避免高对比刺眼
- 小屏端筛选栏、信息网格、curl 示例区均转纵向布局
