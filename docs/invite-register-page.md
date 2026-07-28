# 邀请注册页（/invite）

## 功能概述

用户通过管理员发送的邀请邮件链接（`/invite?token=xxx`）进入本页面，确认账号信息与资源配额后设置登录密码，完成注册并自动登录进入面板。

- 前端页面：`web/src/views/invite/index.tsx` + `invite.css`
- 路由：`/invite`（公开路由，无需登录，注册于 `web/src/router/index.tsx`）
- API 封装：`web/src/api/auth.ts` 中的 `getInviteInfo` / `completeInvite`

## 后端接口（复用已有接口，未修改后端）

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/api/auth/invite?token=xxx` | 读取邀请详情（`service.BuildInviteDetail`） |
| POST | `/api/auth/invite/complete` | 完成注册，body：`token / password / confirm_password`，成功返回 `LoginStageResponse`（含 JWT，自动登录） |

## 页面结构

1. **账号信息**：用户名、邮箱、角色、用户类型（弹性云/轻量云）、邀请有效期；轻量云额外展示资源模式、配额模式、专用 VPC。
2. **资源配额**（仅弹性云）：CPU / 内存 / 磁盘 / 虚拟机数量 / 存储 / 运行时长 / 端口转发开关及配额，`0` 显示为「不限」，网格卡片展示。
3. **轻量云说明**：Banner 提示配额按单台服务器配置；若存在待确认开通服务器（`lightweight_vm_registrations`），以表格展示名称、模板、规格、网络配额摘要。
4. **设置密码**：密码 + 确认密码（Semi Input `mode="password"`），提交按钮为渐变主按钮，支持回车提交；底部提供「返回登录」链接。

## 密码校验流程（与登录/改密一致）

1. 非空 + 两次一致 + 长度 ≥ `STRONG_PASSWORD_MIN_LENGTH`（12 位）；
2. 本地常见弱密码检测：`validatePassword`（`web/src/utils/validate.ts`）；
3. 异步泄露密码检测：`checkPasswordBreachAsync`（后端 HIBP k-匿名接口），命中则阻断提交；
4. 最终校验以后端 `CompleteInviteRegistration` 为准。

## 注册成功后的处理

写入 `useUserStore`（token / username / role / security / cloud_type）后跳转 `/`；轻量云用户由路由守卫自动重定向到其首页。

## 异常场景

- URL 无 `token`：Toast 提示后跳回 `/login`；
- 令牌无效/过期：接口报错由请求层统一 Toast，页面展示失效态 Banner + 「返回登录」按钮。

## 样式说明

- 复用登录页背景与卡片基础样式（`../login/login.css`：`qvm-login` / `qvm-login-card` / `qvm-btn-grad` 等），卡片加宽至 720px，页面纵向可滚动；
- 深色模式下 Descriptions 值与配额数字降对比为柔和灰 `#b8c1cf`；
- 小屏（≤768px）：配额网格降为两列，卡片全宽。
