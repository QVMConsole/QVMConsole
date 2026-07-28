# 找回密码 / 重置密码（忘记密码流程 + /reset-password）

## 功能概述

用户忘记密码时，在登录页点击「忘记密码」打开找回密码弹窗，通过邮箱验证码确认身份并选择账号后，跳转 `/reset-password` 设置新密码。

- 找回密码弹窗：`web/src/views/login/ForgotPasswordModal.tsx`（登录页 `index.tsx` 内接入）
- 重置密码页：`web/src/views/reset-password/index.tsx`（公开路由，注册于 `web/src/router/index.tsx`）
- API 封装：`web/src/api/auth.ts` 中的 `sendForgotPasswordCode` / `verifyForgotPasswordCode` / `selectForgotPasswordAccount` / `resetPasswordByEmail`

## 流程与后端接口（复用已有接口，未修改后端）

| 步骤 | 方法 | 路径 | 说明 |
| ---- | ---- | ---- | ---- |
| 1. 发送验证码 | POST | `/api/auth/password/forgot/send-code` | body：`email`；返回 `challenge_id / masked_email / expires_in`；SMTP 未配置时报错 |
| 2. 校验验证码 | POST | `/api/auth/password/forgot/verify-code` | body：`email / code / challenge_id`；返回 `selection_token`（10 分钟有效）与该邮箱下可重置的已激活账号列表 `accounts` |
| 3. 选择账号 | POST | `/api/auth/password/forgot/select-account` | body：`selection_token / username`；返回 `reset_token`（15 分钟有效） |
| 4. 重置密码 | POST | `/api/auth/password/reset` | body：`token / password / confirm_password`；成功后回登录页 |

## 找回密码弹窗（三步）

1. **email**：输入绑定邮箱，主按钮「发送验证码」；
2. **verify**：Banner 显示脱敏邮箱与有效期，输入 6 位验证码，主按钮「验证邮箱」；次按钮「返回」回到上一步；
3. **select**：账号下拉（仅一个账号时自动选中），显示用户名与角色，主按钮「继续重置」，成功后关闭弹窗并携带 `reset_token` 跳转 `/reset-password?token=xxx`。

邮箱下无可重置账号时提示并重置弹窗状态；关闭弹窗时清空全部中间状态。

## 重置密码页（/reset-password）

- 布局复用登录页背景与卡片样式（420px 卡片），新密码 + 确认密码两个输入框，支持回车提交，底部「返回登录」链接；
- URL 无 `token` 时 Toast 提示并跳回 `/login`；
- 重置成功 Toast「密码已重置，请重新登录」并跳转 `/login`。

## 密码校验流程（与邀请注册/登录一致）

1. 非空 + 两次一致 + 长度 ≥ `STRONG_PASSWORD_MIN_LENGTH`（12 位）；
2. 本地常见弱密码检测：`validatePassword`（`web/src/utils/validate.ts`）；
3. 异步泄露密码检测：`checkPasswordBreachAsync`（后端 HIBP k-匿名接口），命中则阻断提交；
4. 最终校验以后端 `ResetPasswordByToken / ResetPasswordByUserID` 为准。

## 相关说明

- `/api/auth/password/forgot`（一步式发送重置邮件）为旧接口，新前端未使用，保留兼容邮件链接直接进入 `/reset-password` 的场景；
- 深色模式、响应式均复用登录页 / 邀请页样式（`login.css` + `invite.css` 公共类）。
