# 请求日志详情记录与在线查看

## 功能概述

开启「请求日志详情」后，`log/request.log` 会在原有基础请求日志（方法、路径、状态码、耗时、来源 IP、用户）基础上，**追加记录每个请求脱敏后的响应体 JSON**，用于排查接口返回异常、误删数据等场景。

- 记录位置：与服务端其它日志（`app.log`、`cmd.log`、`libvirt.log`）同目录 `log/request.log`；
- 归档轮询：与其它日志完全一致，跟随 lumberjack 的大小轮转（`KVM_LOG_MAX_SIZE_MB`）、保留天数（`KVM_LOG_MAX_DAYS`）、保留份数（`KVM_LOG_MAX_BACKUPS`）以及每日 00:00 定时轮转，压缩归档为 `request-xxxx.log.gz`；
- 页面查看：系统设置 → 日志管理 → 文件列表每行的「查看内容」按钮，直接在线预览 `.log` 文本；压缩归档（`.log.gz`）不支持在线预览，请「一键导出」后本地解压查看。

> 说明：本功能只记录**响应体**，不记录请求体（请求体可能包含明文密码，风险高收益低）。

## 开关配置

默认**开启**，运行时生效（无需重启），并持久化到数据库与 `.env`。

| 配置项 | 说明 | 默认值 | 环境变量 |
| --- | --- | --- | --- |
| `request_detail_log_enabled` | 请求日志详情记录（响应体 JSON）开关 | `true` | `KVM_REQUEST_DETAIL_LOG_ENABLED` |
| `request_log_max_body_bytes` | 单条响应体捕获上限（字节，超出截断并打标记） | `8192` | `KVM_REQUEST_LOG_MAX_BODY_BYTES` |

- 页面开关：系统设置 → 日志管理 → 「请求日志详情 → 记录请求日志」；
- 关闭开关后**完全停止请求日志记录**（request.log 不再产生任何新记录，包括基础请求信息）；已写入的历史记录（含响应体）仍保留在文件中，直至按归档策略轮转清理；
- 查看器在开关关闭时会显示提示，并区分文件中的历史响应体记录（body=）。

## 响应体脱敏

记录响应体前会递归扫描 JSON，以下字段的**字符串值**会被替换为 `[REDACTED]`（数字、布尔值不受影响，如 `has_password: true` 仍正常显示）：

- 名称包含 `token`、`secret`、`password`、`passwd`、`api_key`、`apikey`；
- 精确匹配：`totp`、`otp`、`recovery_code`、`verify_code`、`auth_code`、`captcha`、`authorization`、`credential`、`credentials`；
- 刻意保留 `code`、`key`、`name` 等业务字段，避免破坏日志可读性。

典型示例：登录接口返回的 `token`、`verification_token`，API Key 接口的 `api_key`，虚拟机凭据的 `password`、`credential` 等均不会以明文落盘。

## 非 JSON / 流式 / 超大响应

- SSE 流式推送（`/sse` 路径）不做响应体捕获，避免缓冲阻塞推送；
- 二进制响应（ZIP 导出、镜像、图片等）记录为 `[非JSON响应 N 字节]` 占位，不落二进制内容；
- 响应体超过 `request_log_max_body_bytes`（默认 8 KB）时截断并追加 `...[响应体已截断]` 标记。

## 在线查看（读取文本）

系统设置 → 日志管理 → 文件列表 → 「查看内容」（眼睛图标）打开查看对话框：

- 默认展示该文件**末尾** 200 行；「加载更早的记录」向前分页（每次 200 行，可 1000 行/次），data.eof 为 true 时按钮置灰；
- 「刷新」重新读取文件末尾；
- 查看内容仅是请求日志文本的只读展示，不落库、不经解析渲染，深色模式下自动适配低对比文本。

## 后端接口

`GET /api/settings/log/read?file=<文件名>&lines=<行数>&offset=<字节偏移>`

- 仅管理员可调用（同时兼容 API Key 调用与 bootstrap 令牌）；
- `file` 必须为 `log` 目录下的 `.log` 文件（拒绝路径穿越）；`.log.gz` 返回 400；
- `lines` 默认 200、上限 1000；`offset` 缺省/<=0 表示从文件末尾读取；
- 返回：`data: { name, content, lines, prev_offset, eof }`，将 `prev_offset` 回传即可加载更早记录，`eof=true` 表示已到文件头；
- 读取采用从文件尾部反向扫描的方式，不会将整个日志文件读入内存。

## 排查指引

1. 确认开关状态：系统设置 → 日志管理 → 记录请求日志为「开」；
2. 新产生的请求会包含 `body={...}` 字段，历史日志（开启前的记录）无该字段；关闭开关后文件不再增长；
3. 若单条响应体超大（如列表导出接口），会看到截断标记，可调大 `KVM_REQUEST_LOG_MAX_BODY_BYTES` 后重启生效；
4. 若页面提示「压缩归档日志不支持在线预览」，使用「一键导出」获取对应 `.log.gz` 本地解压查看；
5. 关闭开关后查看器会提示当前为历史记录，如仍希望清除，可在文件列表中勾选后「一键删除」（响应体日志删除后不可恢复）。