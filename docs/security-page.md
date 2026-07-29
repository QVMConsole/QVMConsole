# 个人安全中心页（新前端 React + Semi Design）

> 迁移自旧前端 `web-backup/src/layout/index.vue` 中的「安全设置」对话框（用户头像下拉入口）。
> 页面路由：`/security`（管理员与普通用户均可用），支持 `?tab=xxx` 直接定位标签页。

## 功能总览

共 5 个标签页，与旧前端安全设置对话框一一对应：

| Tab 键位 | 名称 | 内容 |
|----------|------|------|
| `email` | 邮箱绑定 | 当前邮箱 / 验证状态展示，新邮箱 + 验证码换绑流程 |
| `totp` | 两步验证 | 2FA 生成配置（二维码 + 密钥）/ 启用 / 关闭，恢复码状态展示与重新生成 |
| `api` | API 凭证 | API ID 与 Key 标识展示，生成 / 重新生成 / 撤销 API Key，明文 Key 仅在生成响应后显示一次 |
| `password` | 修改密码 | 当前密码 + 新密码（强度校验 + HIBP 泄露检测）+ 确认密码，成功后重新登录 |
| `username` | 修改用户名 | 新用户名（3-32 字符）+ 密码确认身份，成功后本地同步新 Token |

## 文件结构

```
web/src/views/security/
├── index.tsx                       # 主入口：刷新安全状态 / Tabs / ?tab= 定位
├── security.css                    # 页面样式（--qvm- 设计令牌，深色适配）
└── components/
    ├── EmailSection.tsx            # 邮箱绑定区块
    ├── TotpSection.tsx             # 两步验证区块（启用/关闭/恢复码管理）
    ├── ApiKeySection.tsx           # API 凭证区块（生成/轮换/撤销）
    ├── PasswordSection.tsx         # 修改密码区块
    ├── UsernameSection.tsx         # 修改用户名区块
    └── RecoveryCodesModal.tsx      # 恢复码展示弹窗（编号网格 + 复制/下载）
```

相关公共代码：

- `web/src/api/auth.ts`：安全中心的邮箱、2FA、密码、用户名接口与类型
- `web/src/api/apiKey.ts`：API 凭证的读取、生成/轮换与撤销接口及类型
- `web/src/utils/validate.ts`：`validatePassword` 本地弱密码快检 + `checkPasswordBreachAsync` HIBP 泄露检测
- `web/src/utils/clipboard.ts`：`copyTextWithFallback`（HTTP 场景复制降级）
- `web/src/utils/confirm.tsx`：`confirmModal`（关闭 2FA 二次确认）

## 入口

- 侧边栏「系统」分组新增「安全中心」导航项（管理员与普通用户导航均有，`IconSafeStroked` 图标）
- 侧边栏底部用户卡片（头像 + 用户名区域）可点击直达 `/security`，对齐旧版「点击头像 → 安全设置」的交互习惯

## 交互与机制

- **安全状态刷新**：进入页面及各操作成功后调用 `GET /auth/info` 刷新全局 `security` 状态（`useUserStore.setUserInfo`），邮箱验证状态、2FA、恢复码及当前密码泄露状态即时同步。
- **邮箱绑定**：`must_bind_email=true` 时显示警示 Banner；SMTP 未配置时显示提示 Banner。流程为「发送验证码（`challenge_id` 10 分钟有效）→ 输入验证码 → 保存」，绑定成功后清空表单。
- **2FA 启用**：`POST /auth/2fa/setup` 返回 `secret` + `otpauth_url`，前端用 `qrcode` 包本地渲染二维码（深色模式下二维码保持白底确保可扫描）；输入 6 位动态验证码启用，响应顶层 `recovery.recovery_codes` 仅展示一次，弹出恢复码弹窗。
- **2FA 关闭**：危险操作，需密码 + 动态验证码，提交前 `confirmModal` 二次确认。
- **恢复码重新生成**：需密码 + 动态验证码；旧码立即失效，新码同样仅展示一次。恢复码弹窗支持一键复制全部与下载为 `qvmconsole-recovery-codes.txt`。
- **API 凭证**：迁移自旧前端安全设置的 API 标签。可显示 API ID、脱敏 Key 标识、创建/最后使用时间；生成、重新生成与撤销均先经确认，随后由请求层处理 428 高风险二次验证。生成响应中的明文 API Key 使用密码框展示并支持 HTTP 降级复制，仅保留至页面刷新或离开安全中心。
- **修改密码**：新密码依次经过「长度 ≥ 12 → 本地弱密码快检 → 两次输入一致性 → HIBP 泄露检测（后端 k-匿名）」，全部通过后提交；该接口为高风险操作，428 二次验证由请求层（`api/client.ts`）自动弹窗处理；成功后清空登录态并跳转登录页。
- **泄露登录处置**：普通用户登录后收到可关闭警告并可直接进入密码标签页；已绑定 2FA 的管理员完成登录验证后进入现有强制改密窗口；未绑定 2FA 的管理员须通过 `sudo bash qvmc-manage.sh` 修改密码。
- **修改用户名**：后端校验唯一性后重新签发访问 Token，前端同步 `setToken` + `setUserInfo`，无需重新登录。
- **?tab= 定位**：`VALID_TABS` 白名单校验（email / totp / api / password / username），切换 Tab 时 `replace` 回写 URL。

## 后端接口清单

均为旧前端已存在的接口，本次未修改后端：

| 接口 | 方法 | 说明 |
|------|------|------|
| `/auth/info` | GET | 获取当前用户信息（含 security 安全状态） |
| `/auth/email/code/send` | POST | 发送邮箱绑定验证码 |
| `/auth/email/bind` | POST | 绑定 / 换绑邮箱 |
| `/auth/2fa/setup` | POST | 生成 2FA 配置（secret + otpauth_url） |
| `/auth/2fa/enable` | POST | 启用 2FA（返回一次性恢复码） |
| `/auth/2fa/disable` | POST | 关闭 2FA |
| `/auth/2fa/recovery/regen` | POST | 重新生成恢复码 |
| `/auth/api-key` | GET / POST / DELETE | 读取 API Key 状态 / 生成或重新生成 / 撤销（POST、DELETE 为高风险操作） |
| `/auth/password` | PUT | 修改密码（高风险，428 二次验证） |
| `/auth/username` | PUT | 修改用户名（返回新 Token） |
| `/auth/check-password` | POST | HIBP 泄露密码检测 |

## 设计规范落实

- 全部使用 Semi Design 组件（Banner / Tabs / Input / Button / Tag / Modal），提示统一走 Toast API
- 危险操作（关闭 2FA）经 `confirmModal` 二次确认，按钮 `type="danger" theme="light"`
- 恢复码弹窗禁止遮罩关闭（`maskClosable={false}`），必须点击「我已安全保存」确认
- API ID 与本次生成的 API Key 复制使用纯图标 + Tooltip，并通过 `copyTextWithFallback` 兼容 HTTP
- 深色模式下页头 h2 / 分区标题 / 密钥与恢复码等大面积文字降对比为 `#b8c1cf`，二维码图片保持白底
- 720px 以下表单行改纵向堆叠，恢复码网格退化为单列
