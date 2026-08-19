# 请求日志安全

`server/log/request.log` 用于记录 HTTP 请求的路径、状态码、耗时、来源地址和已认证用户，便于排查接口问题。开启「请求日志详情」后（默认开启，见 `docs/request-log-viewer.md`），还会记录脱敏后的响应体 JSON。

## 查询参数脱敏

请求路径包含查询参数时，日志会保留普通参数，但会将下列敏感参数的值替换为 `[REDACTED]`：

- 访问令牌与刷新令牌（名称包含 `token`）
- 密码（名称包含 `password` 或为 `passwd`）
- 密钥与 API Key（`key`、`api_key`、`apikey` 或以 `_key` 结尾）
- 密钥材料（名称包含 `secret`）
- 授权头参数与一次性验证码（`authorization`、`code`）

无法解析的查询字符串不会写入请求日志，避免通过非法编码绕过脱敏。已有日志中的敏感内容不会自动改写；发现历史日志记录过令牌时，应及时轮换对应令牌或签名密钥，并按现有日志保留策略清理历史文件。

## 响应体脱敏

「请求日志详情」开启时，响应体 JSON 在写入日志前会递归脱敏：名称包含 `token` / `secret` / `password` / `passwd` / `api_key` / `apikey`，或精确命中 `totp`、`otp`、`recovery_code`、`verify_code`、`auth_code`、`captcha`、`authorization`、`credential(s)` 的字段，其字符串值被替换为 `[REDACTED]`。数字与布尔值（如 `has_password`）不替换，`code`、`key`、`name` 等业务字段刻意保留。典型覆盖：登录 `token`、`verification_token`、API Key 明文 `api_key`、虚拟机保存凭据 `password`。

脱敏规则的修改集中在 `server/middleware/redact.go`（响应体）与 `server/middleware/request_logger.go`（查询参数），新增接口返回敏感字段时请先确认能被规则覆盖。

## 其它安全设计

- 「请求日志详情」开关（默认开启）控制整个请求日志功能：关闭后 request.log 完全停止记录（含基础请求信息），仅保留历史文件；
- SSE 流式路径（`/sse`）与二进制响应（ZIP、镜像等）不记录响应内容；
- 响应体超过配置上限（默认 8 KB）时截断并打标记，避免日志文件无限膨胀；
- 在线查看接口（`GET /settings/log/read`）仅管理员可用，且拒绝路径穿越与非 `.log` 文件；
- 请求体（Body）一律不记录，避免明文密码等敏感输入落盘。