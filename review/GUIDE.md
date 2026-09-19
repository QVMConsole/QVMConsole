# QVMConsole 代码审查指导文档

> 本文用于指导后续审查人员在独立的「代码审查」预设会话中开展静态审查、编译验证和专属 Linux 审查机上的后端接口测试。本文不是审查结论，也不授权直接修改业务代码或直接执行高风险操作。

## 一、项目概述

### 1.1 项目定位

QVMConsole 是面向小型企业和个人私有云场景的 KVM/QEMU 虚拟机管理平台，提供虚拟机生命周期、模板与克隆、快照、存储池、Open vSwitch 网络、VPC/安全组、防火墙、公网 IP、任务队列、监控、Web VNC/SPICE、用户与配额、安全认证及 REST API。

项目规则要求：

- 虚拟机运行态尽量以 libvirt、宿主机命令和配置文件为事实来源，不应只依赖数据库缓存；
- 耗时操作必须进入任务队列；涉及文件复制、镜像转换、网络传输等 I/O 操作时，不应设置固定超时，但仍需支持上下文取消与清理；
- 敏感操作在 JWT 会话下保留高风险二次验证；允许 API Key 的业务接口由 API Key 自身认证，不触发交互式 HTTP 428；
- 后端新增业务接口原则上兼容 API Key，但账户安全类接口应保持 JWT-only；
- 虚拟机创建、克隆、批量克隆、导入等链路彼此独立，新增字段时必须分别核对；
- 影响正式虚拟机创建、KVM/QEMU/libvirt/OVS 基础能力、基础网络或兼容性测试流程的改动，需按影响范围同步检查兼容性测试与安装脚本；
- 前端遵循 React + Semi Design 的项目级交互、深色模式、Switch、行内操作和 Modal 离场动画规范。

### 1.2 技术栈

以后端与前端实际依赖文件为准；README 中的部分小版本可能滞后。

| 层级 | 技术 |
| --- | --- |
| 后端 | Go `1.26.0`、Gin `v1.12.0`、GORM `v1.31.2`、SQLite 驱动 `v1.6.0`、go-libvirt RPC、JWT v5、TOTP、gorilla/websocket、lumberjack |
| 前端 | React `19.2.8`、TypeScript `7.0.2`、Semi Design `2.103.0`、Vite `8.2.2`、React Router `8.3.1`、Zustand `5.0.15`、Axios `1.20.0`、ECharts、noVNC、xterm |
| 虚拟化与系统 | KVM/QEMU、libvirt、Open vSwitch、dnsmasq、iptables/nftables、QEMU Guest Agent、libguestfs、qemu-img |
| 数据与状态 | SQLite（账户、安全、设置、配额、缓存等）、libvirt/宿主机运行态、面板管理配置文件、模板元数据、内存任务队列 |
| 构建与质量 | npm、TypeScript、Vite、Oxlint、Go build/vet、CGO、Zig 兼容构建、GitHub Actions |

### 1.3 目录与模块划分

| 路径 | 职责 | 审查重点 |
| --- | --- | --- |
| `server/main.go` | 启动顺序、后台调度器、任务处理器注册、运行态恢复 | 初始化失败边界、后台 goroutine、任务补偿、启动副作用 |
| `server/router/router.go` | `/api` 路由、中间件链、静态资源与 SPA 回退 | 公开/JWT/API Key 边界、管理员与 VM 归属权限、路由遗漏 |
| `server/middleware/` | 认证、CORS、公网门禁、凭据格式、限频、请求过滤、日志、安全头 | 绕过、代理信任、重复凭据、敏感信息、状态码一致性 |
| `server/handler/` | HTTP 参数绑定、权限后的业务入口、响应封装 | 输入校验、错误映射、高风险验证、任务提交而非阻塞执行 |
| `server/service/` | 虚拟机、网络、存储、模板、安全、迁移等核心业务 | 命令安全、回滚、幂等、真实运行态、配额与资源清理 |
| `server/taskqueue/queue.go` | 三 Worker 的内存任务队列、取消、SSE、24 小时清理 | 并发安全、阻塞、重启丢失、取消传播、用户隔离 |
| `server/model/` | SQLite 模型、GORM AutoMigrate、账户与设置等持久化 | 事务、并发写、迁移兼容、敏感字段与索引 |
| `server/config/` | `.env`、环境变量、数据库设置与默认值 | 安全默认值、配置优先级、外置参数、文件权限 |
| `server/utils/`、`server/logger/` | 命令执行、文件工具、日志与轮转 | shell 注入、敏感参数、超时策略、文件关闭与日志脱敏 |
| `web/src/api/` | Axios 请求封装和各业务 API | JWT 注入、401/428 重试、错误处理、请求类型 |
| `web/src/features/vm-form/` | 创建与编辑 VM 的共享表单及多链路载荷 | ISO/克隆/批量/导入/编辑字段同步、密码泄露检测 |
| `web/src/views/` | 各业务页面 | 权限显隐、Semi 规范、暗色模式、异步状态与危险确认 |
| `web/src/stores/`、`web/src/hooks/` | Zustand 状态、SSE、Modal 生命周期等 | 订阅清理、闭包过期、重复连接、未卸载更新 |
| `web/scripts/generate-api-endpoints.mjs` | 从后端路由/handler 生成接口清单 | 解析准确性、生成文件漂移、权限与高风险元数据 |
| `scripts/`、`install.sh`、`build.sh` | 兼容性实测、系统脚本、安装与发行构建 | 幂等、发行版兼容、危险命令、回滚与依赖同步 |
| `docs/` | 功能、风险与运维约定 | 代码行为与文档一致性 |
| `security/` | 已知安全问题与修复脚本 | 修复边界、回滚、版本适用性 |

`web-backup/` 是本地忽略的旧前端参考备份，不属于本次审查范围。

### 1.4 核心数据流

1. 浏览器通过 `web/src/api/client.ts` 请求 `/api`。开发环境由 Vite 代理到 `http://localhost:8080`，生产环境由后端同进程提供 `web-dist/`。
2. Gin 依次执行请求日志/恢复、公网访问门禁、CORS、安全响应头、凭据格式检查、请求过滤/防护和全局限频。
3. 路由组再执行 JWT/API Key、强制改密、管理员、云类型和 VM 归属等中间件。
4. `handler` 绑定并校验输入，执行高风险验证门禁；耗时操作只提交到 `taskqueue`，同步只读或轻量操作调用 `service`。
5. `service` 优先通过 go-libvirt RPC、`virsh`、`qemu-img`、OVS、网络及文件系统命令读取或修改真实运行态；SQLite 主要保存账户、安全状态、设置、配额、业务元数据和缓存。
6. 任务队列由 3 个 Worker 执行，进度通过内存事件中心和 `/api/task/sse` 推送；VM、宿主机和调度事件另有各自 SSE。
7. 启动顺序为：加载环境配置 → 初始化日志 → SQLite/AutoMigrate → 数据库设置覆盖 → libvirt RPC → VM 缓存同步 → 安全检查 → 注册并启动任务队列/调度器 → 恢复网络、端口转发、端口镜像、公网 IP 与端口安全运行态 → 注册路由并监听端口。

### 1.5 接口清单入口

- 后端唯一权威路由入口：`server/router/router.go`，统一前缀为 `/api`；
- 构建时生成器：`web/scripts/generate-api-endpoints.mjs`；
- 已生成清单：`web/src/views/api-docs/generated/endpoints.json`，本文生成时记录为 **342 个端点**；
- 人工描述：`web/src/views/api-docs/endpointDescriptions.ts`；
- 字段字典：`web/src/views/api-docs/fieldDictionary.ts`；
- 登录后的前端接口页：`/api-docs`；
- 相关说明：`docs/api-docs-page.md`。

新增或变更路由时，应同时检查生成清单、中文摘要、模块分组、认证方式、管理员/云类型/VM 归属标签和高风险操作标识。

## 二、构建与验证

### 2.1 环境前提

- Go：`server/go.mod` 声明 `go 1.26.0`；审查机工具链必须满足该要求。
- Node.js：`DEPENDENCIES.md` 与 `docs/react-router-security-update.md` 要求 `22.22+`；npm 建议 `9+`。
- 后端使用 `go-sqlite3`，需要 `CGO_ENABLED=1` 和可用 C 编译器。Windows 可使用 MinGW-w64，Linux 使用 GCC/等价工具链。
- 前端必须使用已提交的 `web/package-lock.json` 和 `npm ci` 验证可复现安装。
- 完整 Linux 兼容包需要 Zig；兼容版还会用 `readelf` 校验 GLIBC 上限。
- 后端实际启动依赖 libvirt/KVM/OVS 等 Linux 运行环境；Windows 编译通过不等于宿主功能通过。

> 基线风险：`.github/workflows/build.yml` 当前配置 Node.js `20`，而项目文档和 React Router 8 要求 Node.js `22.22+`。审查时必须核对 CI 是否能够真实完成当前前端构建；若 CI 因版本不满足而无法构建，按 blocker 处理。

### 2.2 建议验证顺序

先确认工作区和提交基线：

```powershell
git status --short --branch
git rev-parse --short HEAD
git rev-parse HEAD
```

#### A. 前端依赖、接口清单、静态检查与编译

```powershell
Set-Location web
node --version
npm --version
npm ci
npm run gen:api
git diff -- src/views/api-docs/generated/endpoints.json
npm run lint
npm run build
```

通过标准：

- `npm ci` 退出码为 0，未隐式改写锁文件；
- `npm run gen:api` 成功生成接口清单，端点数量与当前路由源码一致；
- 若本次没有路由/handler 权限元数据变化，生成文件不应出现无法解释的端点增删；`generated_at` 时间变化需单独识别，禁止把时间戳噪音误判为业务变更；
- `npm run lint` 无 error；
- `npm run build` 中 `tsc -b` 与 Vite 均成功，生成 `web/dist/`；
- 构建日志不得包含明文凭据、私有地址或未解释的动态依赖下载。

#### B. 后端静态检查与 Windows 编译

```powershell
Set-Location ../server
go version
go env CGO_ENABLED
go mod download
gofmt -l .
go vet ./...
go build ./...
```

通过标准：

- Go 版本满足 `go 1.26.0`；
- `CGO_ENABLED=1`，C 编译器可用；
- `gofmt -l .` 无输出；
- `go vet ./...`、`go build ./...` 退出码均为 0；
- 不产生未跟踪的二进制、数据库或临时文件；若产生，先判断是否由构建命令造成，再清理构建产物，禁止误删审查前已有文件。

项目规则明确说明当前仓库没有测试代码。审查人员不得以“存在自动化测试覆盖”作为通过依据，也不应为了本次审查擅自新增测试框架。可用 `git ls-files` 核对是否仍无 `_test.go`、`*.test.*`、`*.spec.*` 等测试源文件。

#### C. Linux 发行包验证（仅构建/安装链路相关改动必须执行）

原生版：

```bash
bash build.sh -v review --variant native
```

兼容版：

```bash
bash build.sh -v review --variant compat
```

通过标准：

- 生成 `release/kvm-console-linux-{amd64|arm64}.tar.gz`；
- 包内至少包含后端二进制、`web-dist/`、`install.sh`、`check-system-compatibility.sh`；
- 兼容版实际最高 GLIBC 依赖不超过目标值（amd64 默认 `2.2.5`，arm64 默认 `2.17`）；
- `build.sh` 的 RPM 下载失败当前是警告而非核心构建失败，需记录网络与可选功能影响；
- 不应无解释地接受 `build.sh` 在 `npm ci` 失败后执行 `npm install` 并改写锁文件的结果，必须审查锁文件差异。

### 2.3 启动方式

仓库开发启动命令：

```bash
bash start-dev.sh
```

该脚本启动：

- 后端：`http://localhost:8080`（Air 热重载）；
- 前端：`http://0.0.0.0:5173`（Vite）；
- Vite 将 `/api` 代理到 `http://localhost:8080`。

**安全审查限制：** `start-dev.sh` 明确设置 `KVM_DEVELOPMENT_MODE=true`，会绕过部分安全验证，因此不能用它证明 JWT 二段登录、428 高风险验证、公网门禁等安全控制有效。本文第五章的接口测试必须在专属 Linux 审查机上以 `development_mode=false` 的安装/运行方式执行。

生产安装入口为交互式 `install.sh`，安装服务名为 `kvm-console.service`。安装、更新、兼容性测试会修改宿主机依赖、网络、systemd 与 `/opt/kvm-console`，不得仅为普通代码审查在已有环境直接重跑。

### 2.4 常见构建失败及处理

| 现象 | 判定与处理 |
| --- | --- |
| Go 提示 `go.mod requires go >= 1.26.0` | 升级到 `go.mod` 指定工具链；不得降低 `go` 指令规避 |
| `go-sqlite3` 报 CGO stub 或找不到 C 编译器 | 确认 `CGO_ENABLED=1` 并安装对应平台 C 编译器；重新编译 |
| Node engine/React Router 构建失败 | 使用 Node.js `22.22+`；同时检查 CI 的 Node 20 配置 |
| `npm ci` 报 package/lock 不同步 | 视为依赖一致性问题；先检查 `package.json` 与 `package-lock.json` 差异，不可直接以 `npm install` 掩盖 |
| `gen:api` 找不到后端源码 | 必须从完整仓库执行；只有发布前端独立构建且已有历史清单时才允许生成器降级沿用旧文件 |
| TypeScript 构建失败但 Vite 能启动 | 仍判定编译失败；以 `npm run build` 的 `tsc -b` 为准 |
| 完整构建提示缺少 Zig | 原生版可单独验证；涉及兼容发行包时必须安装 Zig，不得宣称兼容版通过 |
| 交叉编译缺少 `gcc-*-linux-gnu` | 安装目标架构交叉编译器，或在目标架构主机原生构建 |
| `readelf` 缺失 | 构建脚本会跳过 GLIBC 校验；涉及发布兼容性时此结果不能验收，应补齐工具后重跑 |
| 后端可编译但启动失败 | 检查 libvirt RPC、`/dev/kvm`、OVS、数据库路径、目录权限和环境配置；Windows 不承担运行态验证 |
| 安全接口未返回 428 | 先确认 `development_mode=false`、账户是否仍在高风险信任窗口、SMTP/TOTP 是否可用；不得直接认定验证逻辑通过 |

## 三、审查范围声明

### 3.1 已选范围

用户选择：**包含生产环境测试（后端接口）**。

本次指导范围包括：

1. Windows 或等价开发环境上的前后端编译与静态检查；
2. 对 Go、React/TypeScript、Shell、安装与 CI 配置进行静态审查；
3. 在后续专属 Linux 审查机上，通过真实 HTTP 请求验证后端接口；
4. 接口测试全部使用 JWT（包括 access/login/bootstrap/high-risk 等实际流程产生的 JWT），不使用 API Key 作为动态测试凭据；
5. 重点审查安全与认证；其余 VM、网络、存储、任务队列等按风险抽样并做静态全覆盖；
6. 高风险接口确有必要测试时，必须先通知用户，由用户为审查机创建快照并明确回复可继续；未获得确认时只做静态审查和无副作用接口测试。

不包含：

- 浏览器 MCP 端到端测试；
- 在真实业务生产主机上直接执行危险测试；
- 未经逐项说明的虚拟机创建/删除、宿主网络切换、防火墙重写、磁盘格式化、JWT 密钥轮换、API Key 轮换等操作；
- 审查 `web/node_modules/`、`web/dist/`、`release/`、`server/tmp/`、`tmp/` 等依赖、生成物和临时目录；
- 审查本地忽略的旧版 `web-backup/`。

### 3.2 待补充项

以下信息未提供，实际接口测试前必须补充，不得猜测：

- 待补充：专属 Linux 审查机的面板 `BASE_URL`（HTTP/HTTPS、端口、是否经反向代理）；
- 待补充：审查机启动/重启服务的标准命令及允许的测试时间窗口；
- 待补充：管理员测试账号与密码的安全交付方式；明文不得写入仓库、审查报告、命令历史或日志；
- 待补充：普通用户 JWT 测试账号；若要验证弹性云、轻量云差异，应分别提供账号；
- 待补充：管理员账户的登录二段验证方式（TOTP、恢复码或邮箱）及验证码人工协作方式；
- 待补充：用于 VM 归属隔离测试的现有只读样本 VM 名称及归属关系；
- 待补充：反向代理与 `KVM_TRUSTED_PROXIES` 的实际配置，用于公网来源和会话指纹测试；
- 待补充：高风险测试前用户创建的审查机快照标识、创建时间、恢复负责人和确认语句；
- 待补充：高风险测试允许触及的具体模块、测试数据名称及回滚时限。

### 3.3 高风险测试闸门

满足以下全部条件前，禁止请求任何可能实际落地变更的高风险接口，也不要用“只想观察 428”为理由试探，因为账户可能处于信任窗口或开发模式，接口可能直接执行：

1. 向用户列明拟测端点、请求体、预计副作用、资源名称和清理方式；
2. 用户已为专属 Linux 审查机创建快照；
3. 用户回复确认快照完成并允许继续；
4. 记录快照标识、Git HEAD、服务版本和测试前资源状态；
5. 确认 `development_mode=false`；
6. 准备回滚命令或界面路径，并约定失败后的停止条件。

## 四、静态审查清单

以下每项都应给出“通过/不通过/不适用”及证据文件、行号或命令输出。不能只写“看起来没问题”。

### 4.1 变更边界与影响面

- [ ] 使用 `git status`、`git diff --stat`、`git diff --name-only` 确认改动边界；判定标准：无无关格式化、依赖目录、构建产物或敏感文件。
- [ ] 从变更入口反查全部调用链；判定标准：handler、service、model/config、任务处理器、前端 API/类型/页面和文档均有结论。
- [ ] VM 新增字段逐链核对 ISO 创建、模板单克隆、批量克隆、链式克隆、OVF/OVA/磁盘导入和编辑载荷；判定标准：不存在只补一条链路导致字段静默丢失。
- [ ] 修改 VM 创建、架构、KVM/QEMU/libvirt、系统基础 OVS 网络或兼容性流程时，检查 `scripts/check-system-compatibility.sh`、`server/compatibility_command.go`、`server/service/compatibility/`、`install.sh` 与文档；判定标准：按实际影响同步，或有明确“不受影响”证据。
- [ ] 新增系统依赖时同步 `install.sh`、`docs/dependencies.md`；判定标准：支持的 Debian/RPM 架构与降级行为均明确。
- [ ] 新增/修改路由后运行接口生成器；判定标准：生成清单、模块分组、中文描述、认证/管理员/高风险元数据与源码一致。

### 4.2 安全与认证（本次重点）

- [ ] 路由认证边界：逐个新改端点核对公开、`AuthMiddleware`、`JWTTokenTypeMiddleware`、`AdminMiddleware`、`ElasticCloudOnlyMiddleware`、`VMAccessMiddleware`；判定标准：最低权限原则成立，无仅靠前端隐藏的授权。
- [ ] JWT 类型限制：access、login、bootstrap、high-risk 令牌不可跨阶段使用；判定标准：账户安全入口 JWT-only，普通业务不接受 login/bootstrap 令牌。
- [ ] API Key 静态兼容性：除账户安全流程外，新增业务接口应允许 API Key；判定标准：路由使用允许 API Key 的认证中间件，高风险业务的 API Key 行为符合项目规则。动态测试仍只使用 JWT。
- [ ] 高风险验证：所有创建/删除/重装/迁移/网络/存储/凭据等敏感操作调用 `requireHighRiskVerification` 或更强门禁；判定标准：验证 token 绑定正确 `operation`、有效期与用户，不能跨操作复用。
- [ ] 公网开关：核对开发模式互斥、管理员 2FA、API Key 撤销、可信代理和 LAN 判定；判定标准：公网关闭时 API/静态资源/OPTIONS/SSE/WebSocket 均被门禁覆盖。
- [ ] 凭据入口唯一性：核对重复 Authorization、API Key 别名、Bearer/API Key 混用、查询 token 冲突；判定标准：格式异常在数据库查询前统一拒绝，日志不回显凭据。
- [ ] 会话失效：检查密码/用户名/安全状态变更、禁用账户、登出、公网 30 分钟空闲和 SSE/WebSocket 会话校验；判定标准：旧会话按设计失效，前端正确清理状态。特别核对“登出撤销”在 LAN 与公网请求中的语义是否与文档一致。
- [ ] 会话指纹：判定标准：仅信任配置过的代理头，IP/User-Agent 变化返回 401，不可由任意客户端伪造转发头绕过。
- [ ] 密码输入：登录、邀请、找回/重置、创建/编辑用户、VM 凭据、SSH 密码等所有密码入口均调用项目的强度/泄露检测流程；判定标准：不存在新增输入路径绕过检测，且不记录明文。
- [ ] 密钥与默认值：核对 `KVM_JWT_SECRET`、`KVM_SECURITY_SECRET`、VM 凭据密钥和默认管理员密码；判定标准：生产启动安全校验能阻止不安全默认值，密钥文件/`.env` 权限合理。
- [ ] 命令注入：用户输入不得直接拼入 `bash -c`；判定标准：优先 `ExecCommand(name, args...)`，确需 shell 时每个外部值经过 `ShellSingleQuote` 或等价严格白名单。
- [ ] 路径与归档安全：上传、下载、日志、模板、OVA/OVF、磁盘路径必须阻断 `..`、绝对路径越权、空字节、符号链接/特殊文件和目录逃逸；判定标准：规范化后做根目录边界校验，不只检查扩展名。
- [ ] SSRF/远程连接：节点面板地址、SSH 主机、下载源等外部目标需限制协议、凭据和错误信息；判定标准：不能访问未授权本机/元数据地址，敏感参数不出现在进程列表和日志。
- [ ] 请求与响应日志：判定标准：请求体不记录；token/password/secret/api_key/验证码等查询参数和响应字段递归脱敏；SSE/二进制不缓存；读取日志仅管理员且防路径穿越。
- [ ] 安全响应头与 CORS：判定标准：API `no-store`，静态页面有 CSP；生产 CORS 不应在无必要时为 `*`，允许凭据时返回具体 Origin 并设置 `Vary: Origin`。
- [ ] 错误详情：判定标准：客户端不接收命令 stderr、SQL、绝对敏感路径、密钥或堆栈；诊断信息进入受控日志，`KVM_ERROR_DETAIL_IN_RESPONSE` 的生产值受控。

### 4.3 错误处理与回滚

- [ ] 每个返回的 `error`、命令 `ExitCode/Error/Stderr`、GORM 操作均被处理；判定标准：失败后不继续返回成功或写入后续状态。
- [ ] HTTP 状态与 JSON `code` 对齐；判定标准：400/401/403/404/409/428/429/500 语义一致，前端拦截器不会误判成功。
- [ ] 多阶段操作先验证后落地；判定标准：预检失败不写数据库、不修改 XML/网络/磁盘。
- [ ] 虚拟机、模板、存储、网络重配置存在反向补偿；判定标准：每个已成功步骤都有对应清理，失败时保留原定义或明确报告部分成功。
- [ ] 任务取消传播到 `context.Context`；判定标准：停止新增步骤、终止子进程、清理临时文件和部分资源，并返回 canceled 而非 success。
- [ ] 任务部分成功结果可观测；判定标准：宿主阶段成功、来宾阶段失败等情况在结构化结果和消息中明确，不静默吞错。
- [ ] 重试幂等；判定标准：重复请求不会重复绑定、重复写规则、重复扣配额或误删其他资源。
- [ ] 数据库多步更新使用事务或条件更新；判定标准：并发下不超配、不覆盖新状态，失败不会留下半套关系。

### 4.4 并发与任务队列

- [ ] 所有共享 map、缓存、客户端集合有 mutex/atomic 或单线程所有权；判定标准：读写均在同一锁纪律下。
- [ ] goroutine 有退出条件和 panic recovery；判定标准：长期后台任务可停止，临时 goroutine 不泄漏，关键异步任务使用 `utils.SafeGo` 或显式 `defer RecoverAndLog`。
- [ ] channel 发送不会永久阻塞请求；判定标准：任务队列满、SSE 慢客户端和关闭竞态有明确策略。
- [ ] 任务归属隔离：判定标准：普通用户只能列出、查看、取消自己的任务；SSE 事件在发送前再次做访问检查。
- [ ] 同一 VM/磁盘/网络对象的冲突操作串行化；判定标准：锁粒度能防止并行修改 XML、分区、快照、凭据或网络规则。
- [ ] SQLite 并发：判定标准：WAL、busy timeout 与立即事务配置未被破坏；热点写操作具备条件更新或重试语义。
- [ ] 前端 SSE/定时器/订阅在卸载、失焦和登出时清理；判定标准：无重复连接、陈旧闭包、重复通知或卸载后 setState。
- [ ] 批量克隆/批量操作遵守并发上限；判定标准：单项错误能归属到具体资源，结果汇总不把部分失败写成全成功。

### 4.5 资源释放与 I/O

- [ ] `os.File`、HTTP response body、WebSocket、TCP listener/conn、zip/tar writer、ticker/timer 均在成功创建后及时 `defer Close/Stop`；判定标准：每个获取点有释放路径。
- [ ] 子进程支持进程树终止；判定标准：取消/普通超时不会留下 `qemu-img`、`rsync`、`tcpdump`、guestfs 等孤儿进程。
- [ ] 大文件复制、镜像转换和网络传输不使用固定自动超时；判定标准：使用 no-timeout/Context 变体，仍可由用户取消。
- [ ] 普通探测命令设置合理上限；判定标准：不会因 `virsh`、OVS、网络探测永久挂住 Worker。
- [ ] 临时文件采用唯一目录、安全权限和原子替换；判定标准：成功、失败、取消与进程重启后均有清理策略。
- [ ] 上传/解包限制文件数量、展开大小、磁盘空间和用户配额；判定标准：校验发生在大量写入前，最终写入再次受 project quota 约束。
- [ ] 日志、任务和缓存有清理周期；判定标准：清理只处理模块拥有的对象，不按宽泛名称扫描误删。

### 4.6 硬编码、配置与跨平台

- [ ] 端口、路径、网段、网卡、用户名、服务名、架构和固件路径优先来自配置或运行态探测；判定标准：没有只适用于单台机器的新增常量。
- [ ] 必要默认值安全且可覆盖；判定标准：环境变量、数据库设置和表单之间优先级明确，保存后 `.env` 权限为 `0600`。
- [ ] 架构专属功能只在对应架构展示并由后端复检；判定标准：x86_64/aarch64 的机型、固件、QEMU 命令和依赖不会串用。
- [ ] Debian/Ubuntu 与 RPM 系包名、服务名和命令差异均处理；判定标准：安装脚本和运行时探测一致。
- [ ] 虚拟机运行态不以陈旧 DB 记录为唯一依据；判定标准：关键状态从 libvirt/OVS/文件系统回读，数据库只作元数据或缓存。

### 4.7 日志与可观测性

- [ ] 日志包含模块、资源名、任务 ID、阶段和错误，但不含密码/token/API Key/TOTP/恢复码/私钥；判定标准：可定位且不泄密。
- [ ] 敏感命令使用 `ExecCommandSensitive*`；判定标准：参数正文不会进入 `cmd.log`，quiet 变体只用于预期非零的探测/清理。
- [ ] 失败级别正确；判定标准：预期“未找到/无匹配”不刷 error，真实资源修改失败不能只记 debug。
- [ ] SSE/任务状态足以追踪耗时操作；判定标准：提交、开始、进度、成功/失败/取消都有事件，断线后可由详情接口恢复。
- [ ] 日志轮转与权限合理；判定标准：大响应截断、压缩归档、保留天数/份数可配置，诊断包不意外打包凭据。

### 4.8 依赖与构建配置

- [ ] Go/Node 依赖变更同时更新锁文件并说明理由；判定标准：无未使用依赖、无直接编辑 `node_modules`。
- [ ] 依赖升级检查运行时要求和破坏性变更；判定标准：React Router、React、Semi、Vite、Go/CGO 与 CI 工具链一致。
- [ ] 执行 `npm audit`/适当依赖审计时记录结果和误报判断；判定标准：高危漏洞有处置结论，不因自动升级引入兼容问题。
- [ ] GitHub Actions 与本地命令一致；判定标准：CI 使用满足依赖要求的 Node/Go，能构建两个目标架构或明确矩阵限制。
- [ ] 生成文件可重现；判定标准：API 端点清单除了可解释的路由/权限/时间字段外无随机漂移。

### 4.9 前端项目规范

- [ ] 修改 Semi 组件前阅读 `semi-design-guide` skill；判定标准：组件 API 与当前 Semi 版本匹配。
- [ ] 行内操作采用纯图标 + Tooltip，超过 2~3 个时收进 `⋯`；判定标准：危险项标红、加载态用旋转图标。
- [ ] Switch 使用内部单字符 `checkedText/uncheckedText`；判定标准：无外置重复状态文字。
- [ ] 条件挂载的 Semi Modal 使用 `useMountModalLifecycle.ts`；判定标准：先 `visible=false`，`afterClose` 后卸载。
- [ ] 深色模式大面积文字使用柔和灰而非近白 `--qvm-text-0`；判定标准：浅色优先、暗色对比不刺眼。
- [ ] 复制功能使用 `copyTextWithFallback`；判定标准：HTTP 非安全上下文仍可降级复制。
- [ ] 所有密码输入接入本地强度与后端泄露检测；判定标准：创建、编辑和弹窗入口一致。
- [ ] 角色/云类型/架构只在前端隐藏还不够；判定标准：后端存在同等或更严格校验。

## 五、接口测试清单

### 5.1 测试约定与凭据

本章在后续专属 Linux 审查机执行，不在本文生成会话执行。

```bash
export BASE_URL='<待补充，例如 https://review.example.test>'
export JWT='<登录响应 data.token；仅放入当前临时 shell，不写入脚本或仓库>'
```

凭据要求：

- 管理员 access JWT：必需；
- 普通用户 access JWT：待补充，用于角色与资源隔离；
- 轻量云用户 access JWT：待补充，仅在需要验证云类型边界时使用；
- login/bootstrap JWT：仅当登录响应进入对应阶段时临时使用；
- TOTP/邮箱验证码：人工即时输入；恢复码为一次性消耗品，除非用户明确同意，不用于常规审查。

统一通过标准：

- 常规成功 JSON 符合 `{ "code": 200|202, "message": "...", "data": ... }`；`GET /api/public/version` 当前实现仅返回 `code/data`，审查时需确认其是否作为兼容例外，否则记录统一响应结构缺口；
- HTTP 状态与 `code` 一致；
- 响应不包含密码、完整密钥、JWT、TOTP secret、恢复码或内部堆栈；
- 所有测试记录请求 ID/时间、端点、角色、HTTP 状态和脱敏摘要，不记录 Authorization 值；
- 测试后检查请求日志脱敏与临时资源清理。

### 5.2 阶段 A：公开与基础协议（无副作用）

| 端点 | 请求样例 | 预期 |
| --- | --- | --- |
| `GET /api/public/version` | `curl -sS -i "$BASE_URL/api/public/version"` | HTTP 200；`data` 含 `version/build_time/site_title`；无敏感配置 |
| `GET /api/public/settings` | `curl -sS -i "$BASE_URL/api/public/settings"` | HTTP 200；仅返回 `site_title/password_breach_check_enabled/spice_enabled_by_default` |
| `GET /api/not-exist` | `curl -sS -i "$BASE_URL/api/not-exist"` | HTTP 404；JSON `code=404`，不回退到 SPA |
| 安全响应头 | `curl -sS -D - -o /dev/null "$BASE_URL/api/public/version"` | `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、API `Cache-Control: no-store...` |
| 密码泄露查询 | `curl -sS -i -H 'Content-Type: application/json' -d '{"password":"QvmReview-Unique-DoNotUse-2026!"}' "$BASE_URL/api/auth/check-password"` | HTTP 200；`enabled/breached` 为布尔值；外部服务异常时只能返回明确 warning，不泄露输入密码 |

密码泄露查询只能使用专为审查构造、从未作为真实凭据的字符串。

### 5.3 阶段 B：认证与凭据格式（无业务写入）

#### 1. 有效登录

```bash
curl -sS -i \
  -H 'Content-Type: application/json' \
  --data-binary @- \
  "$BASE_URL/api/auth/login"
# 随后从标准输入提供：{"username":"<待补充>","password":"<仅运行时输入>"}
# 输入完成后按 Ctrl+D；不要把真实密码写进命令参数、脚本、仓库或审查报告。
```

预期为以下合法分支之一：

- `stage=success`：返回 access JWT；
- `stage=bootstrap_security`：返回 bootstrap JWT，只能访问安全初始化白名单；
- `stage=login_verify`：返回 login JWT 和 `allowed_methods`，完成 `/api/auth/login/verify` 后才返回 access JWT；
- 首次默认密码场景可能返回 `force_password_change=true`，其余业务接口应被强制改密中间件阻断。

不得为了测试爆破保护连续提交错误密码；登录锁定测试仅可在专用一次性账号和单独批准的窗口内执行。

#### 2. 未认证与格式异常

```bash
curl -sS -i "$BASE_URL/api/auth/info"

curl -sS -i \
  -H 'Authorization: Bearer not-a-jwt' \
  "$BASE_URL/api/auth/info"

curl -sS -i \
  -H "Authorization: Bearer $JWT" \
  -H 'Authorization: Bearer another.invalid.token' \
  "$BASE_URL/api/auth/info"

curl -sS -i \
  -H "Authorization: Bearer $JWT" \
  -H 'X-API-Key-ID: fake' \
  -H 'X-API-Key: fake' \
  "$BASE_URL/api/auth/info"
```

预期：

- 无凭据为 HTTP 401；
- 非法 JWT、重复 Authorization、JWT/API Key 混用在凭据守卫处拒绝，格式类错误为 HTTP 403 且不回显具体凭据；
- 服务日志只记录拒绝原因，不记录 token/key 正文。

#### 3. 有效 JWT 身份

```bash
curl -sS -i \
  -H "Authorization: Bearer $JWT" \
  "$BASE_URL/api/auth/info"
```

预期 HTTP 200，`data.username/role/cloud_type/security` 与测试账号一致；不得返回密码哈希、TOTP secret、恢复码或 API Key 明文。

#### 4. 查询 token 与 Header 冲突

```bash
curl -sS -i --get \
  -H "Authorization: Bearer $JWT" \
  --data-urlencode "token=$JWT" \
  "$BASE_URL/api/task/list"
```

预期 HTTP 403，原因属于认证来源冲突。命令执行后检查 `request.log` 中查询参数 `token` 已被替换为 `[REDACTED]`。

### 5.4 阶段 C：JWT 只读业务与权限隔离

| 端点 | 方法/样例 | 凭据 | 预期 |
| --- | --- | --- | --- |
| `/api/system-info` | `GET`，Header `Authorization: Bearer $JWT` | 任意有效 access JWT | 200；返回宿主机摘要，不泄露环境变量/密钥 |
| `/api/task/list?page=1&page_size=20` | `GET` | access JWT | 200；分页字段合法；普通用户只看到本人任务，`params` 中敏感值已脱敏 |
| `/api/security/password-breach/status` | `GET` | 管理员 JWT | 200；返回状态和可能的 active task |
| `/api/security/password-breach/status` | `GET` | 普通用户 JWT（待补充） | 403，需要管理员权限 |
| `/api/settings/log/status` | `GET` | 管理员 JWT | 200；只返回日志元数据，不直接返回任意文件内容 |
| `/api/settings/log/read?file=request.log&lines=200` | `GET` | 管理员 JWT | 200 或文件不存在时 404；内容中的 token/password/secret/API Key 已脱敏 |
| `/api/settings/log/read?file=../app.log` | `GET`，建议 `--get --data-urlencode 'file=../app.log'` | 管理员 JWT | 400，拒绝路径穿越 |
| `/api/vm/<不属于该用户的样本名>` | `GET` | 普通用户 JWT（待补充） | 403，不泄露 VM 详情；样本关系必须事先确认 |

样例命令：

```bash
curl -sS -i \
  -H "Authorization: Bearer $JWT" \
  "$BASE_URL/api/task/list?page=1&page_size=20"

curl -sS -i \
  -H "Authorization: Bearer $JWT" \
  "$BASE_URL/api/security/password-breach/status"

curl -sS -i --get \
  -H "Authorization: Bearer $JWT" \
  --data-urlencode 'file=../app.log' \
  --data-urlencode 'lines=200' \
  "$BASE_URL/api/settings/log/read"
```

### 5.5 阶段 D：SSE 与连接释放

```bash
curl -N --max-time 15 \
  -H "Authorization: Bearer $JWT" \
  "$BASE_URL/api/task/sse"
```

预期：

- HTTP 200、`Content-Type: text/event-stream`；
- 首个事件为 `connected`；
- 15 秒后客户端退出，服务端连接计数回落，无 goroutine/channel 泄漏；
- 普通用户不会收到或读取其他用户任务事件；
- 公网会话失效时流应发送 `session_expired` 并断开；该项仅在公网测试条件已补充时执行。

并发连接、断线重连和慢客户端压测需单独规定连接数，不得直接在共享审查机无限加压。

### 5.6 阶段 E：高风险验证与受控写入（必须先通过快照闸门）

推荐只使用相对受控的密码泄露扫描任务验证完整 428 链路，除非代码变更明确涉及其他高风险模块。

#### 1. 触发验证挑战

```bash
curl -sS -i \
  -X POST \
  -H "Authorization: Bearer $JWT" \
  -H 'Content-Type: application/json' \
  -d '{}' \
  "$BASE_URL/api/security/password-breach/scan"
```

预期在无有效信任窗口时返回 HTTP 428，`data` 含 `method`、`operation=run_password_breach_scan`，邮箱方式还含 `challenge_id/masked_email/expires_in`。如果直接返回 202，先停止后续写测试并核查开发模式、验证可用性及高风险信任窗口，不能把它当成 428 流程通过。

#### 2. 完成高风险验证

TOTP 示例：

```bash
curl -sS -i \
  -X POST \
  -H "Authorization: Bearer $JWT" \
  -H 'Content-Type: application/json' \
  -d '{"method":"totp","code":"<即时验证码>","operation":"run_password_breach_scan"}' \
  "$BASE_URL/api/auth/high-risk/verify"
```

邮箱方式需按 428 响应补 `challenge_id`；双重方式需同时提供 `code` 与 `email_code`。预期 HTTP 200，返回短期 `verification_token`。不得把 token 写入文件或报告。

#### 3. 携带操作绑定 token 重试

```bash
export VERIFY_TOKEN='<上一步 data.verification_token>'

curl -sS -i \
  -X POST \
  -H "Authorization: Bearer $JWT" \
  -H "X-High-Risk-Token: $VERIFY_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{}' \
  "$BASE_URL/api/security/password-breach/scan"
```

预期 HTTP 202、JSON `code=202`，返回 `data.task` 和 `data.reused`。随后仅使用任务只读接口跟踪：

```bash
curl -sS -i \
  -H "Authorization: Bearer $JWT" \
  "$BASE_URL/api/task/<task_id>"
```

通过标准：任务进入终态，重复提交复用活动任务，失败信息不含凭据，任务结果与调度/日志一致。

#### 4. 其他高风险模块

仅当被审查变更直接涉及对应模块，且用户在快照后逐项批准时测试：

- VM 创建、删除、重装、迁移、强制电源操作；
- 模板移动/删除、OVF/OVA 导入；
- OVS 修复、VPC 重配置、端口安全、端口镜像、ACL；
- 公网 IP 绑定/解绑/迁移；
- 防火墙启停、规则修改、关闭连接；
- 存储格式化、分区、LVM、磁盘迁移；
- IOMMU/VFIO、主显卡直通；
- 公网访问切换、JWT 密钥轮换、API Key 轮换/撤销。

每项必须使用 `web/src/views/api-docs/generated/endpoints.json`、`endpointDescriptions.ts` 和对应 handler 请求结构确定真实请求体，禁止凭经验臆造字段。执行前后保存只读状态快照；成功后清理临时资源，失败则立即停止同模块后续测试并评估整机快照恢复。

### 5.7 限频、登录锁定与公网专项（条件满足时）

以下测试可能影响同源 IP 或账号，不进入默认冒烟：

- 公开/认证接口每分钟限频及 `Retry-After`；
- 登录失败计数与 429；
- 公网关闭时非 LAN 请求统一 403；
- 公网 JWT 30 分钟无真实操作失效；
- 可信代理下 `X-Forwarded-For` 解析；
- 会话指纹的 IP/User-Agent 变化；
- 登出后旧 JWT 在 LAN 与公网来源的行为。

执行前需补充代理拓扑、来源 IP、限频配置和一次性账号，并确保不会锁定唯一管理员或影响其他审查流量。

### 5.8 接口测试结束检查

- [ ] 删除所有明确标记为审查用途的临时数据；
- [ ] 所有异步任务已终态，无 pending/running 残留；
- [ ] 无残留 VM、磁盘、NVRAM、OVS 端口、流表、meter、iptables/nftables 规则、上传分片或临时目录；
- [ ] `request.log`、`cmd.log`、`app.log` 不含本次 JWT、密码、验证码或高风险 token；
- [ ] 服务、libvirt、OVS、基础网络和原有 VM 状态与测试前一致；
- [ ] 若状态无法恢复，停止测试并使用用户确认的审查机快照回滚；
- [ ] 审查报告仅保留脱敏请求和响应摘要。

## 七、验收标准与严重级别定义

### 7.1 总体验收标准

只有同时满足以下条件，才能给出“可通过”结论：

1. 前端 `npm run lint`、`npm run build` 和后端 `go vet ./...`、`go build ./...` 全部通过；若受环境阻断，必须列明阻断原因，不能写成通过；
2. 路由生成清单与源码一致，所有接口权限和高风险标识无遗漏；
3. 本文第五章无副作用接口测试通过；需高风险验证的变更已在用户确认快照后完成受控测试，或明确标为未验证阻断项；
4. 无未解决 blocker；major 必须修复或由用户书面接受风险并给出补救计划；
5. 用户/管理员、弹性云/轻量云、VM 归属和任务归属隔离成立；
6. 无明文密钥、密码、JWT、验证码、恢复码、私钥或敏感日志泄露；
7. 耗时操作进入任务队列，支持取消、进度、失败清理和幂等重试；
8. VM、存储和网络变更具备可验证的回滚路径，不留下孤儿资源；
9. 依赖、安装脚本、兼容性测试和 docs 按实际影响同步；
10. 审查结束后 `git status` 仅包含审查前已有变更和预期审查产物，未混入构建产物或凭据。

### 7.2 严重级别

| 级别 | 判定界限 | 本项目示例 |
| --- | --- | --- |
| `blocker` | 无法编译/启动/发布；可导致认证绕过、跨租户控制、宿主机失联、不可恢复数据丢失、密钥明文泄露或大范围资源破坏；没有安全回滚 | 非管理员可操作他人 VM；格式化错误磁盘；OVS 重配置切断管理网络且无回滚；JWT 密钥暴露；CI 工具链必然无法构建发布 |
| `major` | 核心功能错误或高概率产生错误状态；权限、高风险验证、配额、并发、任务取消、清理、兼容性存在实质缺陷，但影响范围可控或可恢复 | 高风险路由漏 428；任务显示成功但资源未完成；批量链路漏字段；取消后残留磁盘/进程；普通用户看到他人任务；日志泄露 token |
| `minor` | 不阻断主流程、影响局部可用性/可维护性/诊断质量，存在明确绕行方式且不会造成权限或数据风险 | 错误文案不精确、非关键状态刷新滞后、部分异常缺少上下文、文档小范围落后、暗色模式局部对比不佳 |
| `suggestion` | 当前行为正确，仅为一致性、性能余量、可读性或未来扩展建议 | 提取重复函数、改善命名、减少无害重复请求、补充注释或更细指标 |

严重级别以“实际最坏影响 + 可利用性/发生概率 + 可恢复性”为准，不能因修改行数少而降级。安全与租户隔离问题至少为 major；可直接利用或影响宿主机/全体用户时为 blocker。

## 八、附录

### 8.1 文档生成信息

- 生成时间：`2026-09-19 13:30:32 +08:00`
- Git 分支：`main`
- Git HEAD（短）：`c5583ab`
- Git HEAD（完整）：`c5583abdcab9ff92dea62283ce239e67836c496c`
- 仓库状态（生成前）：工作区无已跟踪/未跟踪变更输出
- CHANGELOG：项目根目录未发现项目级 `CHANGELOG*`；依赖目录中的 CHANGELOG 不作为项目变更记录
- 测试代码：跟踪文件中未发现项目测试源文件，与 `AGENTS.md` 的“本项目没有测试代码”一致

### 8.2 主要参考文件

规则与说明：

- `C:/Users/17737/.dsh/AGENTS.md`
- `AGENTS.md`
- `README.md`
- `DEPENDENCIES.md`
- `docs/*.md`（本文生成时已阅读项目 `docs/` 下现有 59 份文档）

构建与运行：

- `server/go.mod`
- `web/package.json`
- `web/package-lock.json`
- `server/.air.toml`
- `web/vite.config.ts`
- `build.sh`
- `start-dev.sh`
- `install.sh`
- `.github/workflows/build.yml`
- `.github/workflows/opencode.yml`

后端架构与安全：

- `server/main.go`
- `server/config/config.go`
- `server/router/router.go`
- `server/middleware/auth.go`
- `server/middleware/credential_guard.go`
- `server/middleware/public_access.go`
- `server/middleware/ratelimit.go`
- `server/middleware/cors.go`
- `server/middleware/security_headers.go`
- `server/handler/auth.go`
- `server/handler/security_helper.go`
- `server/handler/session.go`
- `server/handler/password_breach.go`
- `server/handler/task.go`
- `server/handler/log_read.go`
- `server/model/db.go`
- `server/taskqueue/queue.go`
- `server/utils/cmd.go`

前端与接口文档：

- `web/src/api/client.ts`
- `web/src/api/auth.ts`
- `web/src/api/settings.ts`
- `web/src/config/constants.ts`
- `web/src/router/index.tsx`
- `web/src/types/api.ts`
- `web/scripts/generate-api-endpoints.mjs`
- `web/src/views/api-docs/generated/endpoints.json`
- `web/src/views/api-docs/endpointDescriptions.ts`
- `web/src/views/api-docs/fieldDictionary.ts`

系统与兼容性：

- `scripts/check-system-compatibility.sh`
- `server/compatibility_command.go`
- `server/service/compatibility/`
- `docs/install-system-compatibility-check.md`
- `docs/build-compatibility.md`
- `docs/public-access-security.md`
- `docs/request-log-security.md`
- `docs/api-docs-page.md`
