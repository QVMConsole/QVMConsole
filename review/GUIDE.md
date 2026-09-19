# QVMConsole 代码审查指导文档

> 本文用于指导后续审查人员在独立的「代码审查」预设会话中开展静态审查、编译验证和**按需**的后端接口测试。本文不是审查结论，也不授权直接修改业务代码或直接执行高风险操作。
>
> **修订说明（2026-09-19）**：本版将审查主线由「固定全量清单 + 全量安全测试」调整为**变动审计（diff-driven）**。原因：原第五章是一份固定不变的接口与安全测试清单，任何一次审查都会把同一批端点、同一批安全用例重新执行一遍，既不产生新信息，又会拉长会话、放大环境噪声、掩盖本次变更真正引入的问题。新规则是：**只审计本次提交范围内变更的代码及其真实影响面；未变更的代码不重复审计，与本次变更无关的接口/安全测试不重复执行，上一份报告的结论不自动继承。**

## 一、核心原则：变动审计

### 1.1 三条硬约束

1. **只审变更**：审查对象 = 本次 `git diff` 的变更行 + 其真实影响面。未出现在变更清单中的文件，除被变更代码直接引用（新增调用、新增依赖、契约变化波及）外，一律不审。
2. **不重复测试**：动态验证只覆盖本次变更影响的端点与链路。禁止每次会话重跑同一份固定全量接口清单；禁止把「上次测过」当成「这次不用测」，也禁止把「上次的通过结论」当成「这次的通过依据」。
3. **不继承结论**：上一份报告中的「未覆盖 / 环境受限」判定不自动延续到本次报告。本次报告只对本次变更范围下结论；范围外的能力统一写「未审查（不在本次变更范围）」，不复述历史判定、不重复罗列历史排除项。

### 1.2 变更影响面三环模型

| 环 | 范围 | 是否必审 |
| --- | --- | --- |
| 第 1 环 | 变更行本身（新增/修改/删除的代码、配置、脚本、文档） | 必审，逐行 |
| 第 2 环 | 真实影响面：变更符号的直接调用方与被调用方、类型/常量/接口契约变化波及的编译单元、同源前端字段与类型 | 必审，但只审受影响的那段逻辑，不整文件重审 |
| 第 3 环 | 项目规则要求同步的位置（见 1.3 判定表） | 按表逐项判定，判定结果必须有证据 |
| 环外 | 与变更无调用、契约、配置关联的一切既有代码 | **不审**。顺带发现的问题最多记为 `suggestion`，并注明「非本次变更引入」 |

判定影响面时以证据为准（`grep` 调用点、路由表、类型定义、任务处理器注册点），不得凭文件名相似臆测关联，也不得因为「顺手看一眼」扩大审查范围。

### 1.3 第 3 环同步项判定表

| 变更类型 | 必须核对的同步位置 | 判定要求 |
| --- | --- | --- |
| VM 新增/修改字段 | ISO 创建、模板单克隆、批量克隆、链式克隆、OVF/OVA/磁盘导入、编辑载荷；前端创建向导与编辑选项卡两处表单 | 要么同步补齐，要么给出「哪条链路不涉及该字段」的具体证据 |
| 触及 VM 创建参数语义、KVM/QEMU/libvirt/OVS 基础能力、基础网络、兼容性测试流程 | `scripts/check-system-compatibility.sh`、`server/compatibility_command.go`、`server/service/compatibility/`、`install.sh` 及相关文档 | 按 AGENTS.md 第 33 条「按影响范围同步」判定；纯前端样式、文案、账户认证、监控统计、无关存储网络功能不触发 |
| 新增系统依赖 | `install.sh`、`docs/dependencies.md` | 发行版差异与降级行为明确 |
| 新增/修改路由或权限元数据 | `web/scripts/generate-api-endpoints.mjs` 生成的清单、`endpointDescriptions.ts`、模块分组映射 | 生成清单与源码一致，认证/管理员/高风险标签无遗漏 |
| 新增后端业务接口 | API Key 兼容性（账户安全类除外）、`router.go` 行尾中文注释 | 默认兼容 API Key，敏感操作保留二次验证 |
| 前端交互改动 | `semi-design-guide` 规范、深色模式、Switch 单字符、图标 + Tooltip、Modal `useMountModalLifecycle` | 见第五章 5.9 清单 |
| 用户可见行为变化 | `docs/` 对应功能文档 | 文档与代码行为一致 |

### 1.4 允许重复执行的情形（唯一例外）

只有以下情况允许脱离本次变更范围做重复或全量动作，且必须在报告中注明理由：

1. 首次审查或基准提交未知，需要先建立基线；
2. 变更本身触及认证、中间件、响应封装、任务队列等**横切模块**，需要对该横切面做一次针对性边界抽样（见 6.3）；
3. 用户本次明确要求全量回归（报告中标注为用户指定偏离）。

## 二、项目概述

### 2.1 项目定位

QVMConsole 是面向小型企业和个人私有云场景的 KVM/QEMU 虚拟机管理平台，提供虚拟机生命周期、模板与克隆、快照、存储池、Open vSwitch 网络、VPC/安全组、防火墙、公网 IP、任务队列、监控、Web VNC/SPICE、用户与配额、安全认证及 REST API。

项目规则要求：

- 虚拟机运行态尽量以 libvirt、宿主机命令和配置文件为事实来源，不应只依赖数据库缓存；
- 耗时操作必须进入任务队列；涉及文件复制、镜像转换、网络传输等 I/O 操作时，不应设置固定超时，但仍需支持上下文取消与清理；
- 敏感操作在 JWT 会话下保留高风险二次验证；允许 API Key 的业务接口由 API Key 自身认证，不触发交互式 HTTP 428；
- 后端新增业务接口原则上兼容 API Key，但账户安全类接口应保持 JWT-only；
- 虚拟机创建、克隆、批量克隆、导入等链路彼此独立，新增字段时必须分别核对；
- 影响正式虚拟机创建、KVM/QEMU/libvirt/OVS 基础能力、基础网络或兼容性测试流程的改动，需按影响范围同步检查兼容性测试与安装脚本；
- 前端遵循 React + Semi Design 的项目级交互、深色模式、Switch、行内操作和 Modal 离场动画规范。

### 2.2 技术栈

以后端与前端实际依赖文件为准；README 中的部分小版本可能滞后。

| 层级 | 技术 |
| --- | --- |
| 后端 | Go `1.26.0`、Gin `v1.12.0`、GORM `v1.31.2`、SQLite 驱动 `v1.6.0`、go-libvirt RPC、JWT v5、TOTP、gorilla/websocket、lumberjack |
| 前端 | React `19.2.8`、TypeScript `7.0.2`、Semi Design `2.103.0`、Vite `8.2.2`、React Router `8.3.1`、Zustand `5.0.15`、Axios `1.20.0`、ECharts、noVNC、xterm |
| 虚拟化与系统 | KVM/QEMU、libvirt、Open vSwitch、dnsmasq、iptables/nftables、QEMU Guest Agent、libguestfs、qemu-img |
| 数据与状态 | SQLite（账户、安全、设置、配额、缓存等）、libvirt/宿主机运行态、面板管理配置文件、模板元数据、内存任务队列 |
| 构建与质量 | npm、TypeScript、Vite、Oxlint、Go build/vet、CGO、Zig 兼容构建、GitHub Actions |

### 2.3 目录与模块划分

| 路径 | 职责 | 变更时的审查重点 |
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

`web-backup/` 是本地忽略的旧前端参考备份，不属于审查范围。

### 2.4 核心数据流

1. 浏览器通过 `web/src/api/client.ts` 请求 `/api`。开发环境由 Vite 代理到 `http://localhost:8080`，生产环境由后端同进程提供 `web-dist/`。
2. Gin 依次执行请求日志/恢复、公网访问门禁、CORS、安全响应头、凭据格式检查、请求过滤/防护和全局限频。
3. 路由组再执行 JWT/API Key、强制改密、管理员、云类型和 VM 归属等中间件。
4. `handler` 绑定并校验输入，执行高风险验证门禁；耗时操作只提交到 `taskqueue`，同步只读或轻量操作调用 `service`。
5. `service` 优先通过 go-libvirt RPC、`virsh`、`qemu-img`、OVS、网络及文件系统命令读取或修改真实运行态；SQLite 主要保存账户、安全状态、设置、配额、业务元数据和缓存。
6. 任务队列由 3 个 Worker 执行，进度通过内存事件中心和 `/api/task/sse` 推送；VM、宿主机和调度事件另有各自 SSE。
7. 启动顺序为：加载环境配置 → 初始化日志 → SQLite/AutoMigrate → 数据库设置覆盖 → libvirt RPC → VM 缓存同步 → 安全检查 → 注册并启动任务队列/调度器 → 恢复网络、端口转发、端口镜像、公网 IP 与端口安全运行态 → 注册路由并监听端口。

### 2.5 接口清单入口

- 后端唯一权威路由入口：`server/router/router.go`，统一前缀为 `/api`；
- 构建时生成器：`web/scripts/generate-api-endpoints.mjs`；
- 已生成清单：`web/src/views/api-docs/generated/endpoints.json`；
- 人工描述：`web/src/views/api-docs/endpointDescriptions.ts`；
- 字段字典：`web/src/views/api-docs/fieldDictionary.ts`；
- 登录后的前端接口页：`/api-docs`；
- 相关说明：`docs/api-docs-page.md`。

新增或变更路由时，应同时检查生成清单、中文摘要、模块分组、认证方式、管理员/云类型/VM 归属标签和高风险操作标识。

## 三、审查范围声明与执行流程

### 3.1 默认范围：变动审计（范围 1）

| 范围 | 内容 | 触发条件 |
| --- | --- | --- |
| 范围 1（默认） | 按变更语言侧执行编译/静态检查 + 变更文件及影响面静态审查 | 任何审查都执行 |
| 范围 2（按需追加） | 后端接口动态验证，**只覆盖变更影响的端点** | 变更触及 `server/router`、`server/handler`、`server/middleware`、`server/service` 的 HTTP 行为、请求/响应契约、认证与权限时 |
| 范围 3（按需追加） | 前端浏览器端到端 | 变更触及前端交互，且用户本次明确要求时；优先使用会话已配置的浏览器 MCP，无可用工具时降级 `playwright` skill 并在报告中注明降级 |

默认不包含（除非用户本次明确指定）：

- 与本次变更无关的历史接口回归、固定全量安全测试清单；
- 浏览器全量端到端测试；
- 在真实业务生产主机上直接执行危险测试；
- 未经逐项说明的虚拟机创建/删除、宿主网络切换、防火墙重写、磁盘格式化、JWT 密钥轮换、API Key 轮换等操作；
- 审查 `web/node_modules/`、`web/dist/`、`release/`、`server/tmp/`、`tmp/` 等依赖、生成物和临时目录；
- 审查本地忽略的旧版 `web-backup/`。

范围 2 一旦触发，其测试集由**本次变更映射生成**（见第六章），不是照抄固定清单。

### 3.2 基线确定与变更清单生成

1. 按项目实际使用的版本控制执行 `git pull`；失败（无远端、网络问题、冲突）时如实报告并询问用户，不自行解决冲突或强制覆盖。
2. 确定基准提交，优先级：用户本次明确指定 > `review/.last-review.json` 的 `lastEndCommit` > 用 `ask_user_question` 询问（可给常用选项：仓库首个提交、指定 tag、最近 N 次提交）。
3. 生成变更清单：

```bash
git log --oneline <base>..HEAD
git diff --name-status <base>..HEAD
git diff --stat <base>..HEAD
```

4. 给每个变更文件打标签并据此决定执行范围：

| 标签 | 典型路径 | 触发 |
| --- | --- | --- |
| 后端接口类 | `server/router|handler|middleware` | 范围 2 |
| 后端业务/基础设施类 | `server/service|taskqueue|model|config|utils` | 范围 2 仅当行为可从 HTTP 观察 |
| 前端类 | `web/src/**`、`web/scripts/**` | 范围 1；用户要求时范围 3 |
| 系统/安装类 | `scripts/**`、`install.sh`、`build.sh`、`.github/**` | 范围 1 的编译 C 路径 |
| 文档/配置类 | `docs/**`、`*.md`、`.gitignore` | 一致性核对 |

5. 变更清单之外的既有代码不进入审查对象清单。

### 3.3 高风险测试闸门

满足以下全部条件前，禁止请求任何可能实际落地变更的高风险接口，也不要用「只想观察 428」为理由试探，因为账户可能处于信任窗口或开发模式，接口可能直接执行：

1. 向用户列明拟测端点、请求体、预计副作用、资源名称和清理方式；
2. 用户已为专属 Linux 审查机创建快照；
3. 用户回复确认快照完成并允许继续；
4. 记录快照标识、Git HEAD、服务版本和测试前资源状态；
5. 确认 `development_mode=false`；
6. 准备回滚命令或界面路径，并约定失败后的停止条件。

## 四、构建与验证

### 4.1 环境前提

- Go：`server/go.mod` 声明 `go 1.26.0`；审查机工具链必须满足该要求。
- Node.js：`DEPENDENCIES.md` 与 `docs/react-router-security-update.md` 要求 `22.22+`；npm 建议 `9+`。
- 后端使用 `go-sqlite3`，需要 `CGO_ENABLED=1` 和可用 C 编译器。Windows 可使用 MinGW-w64，Linux 使用 GCC/等价工具链。
- 前端必须使用已提交的 `web/package-lock.json` 和 `npm ci` 验证可复现安装。
- 完整 Linux 兼容包需要 Zig；兼容版还会用 `readelf` 校验 GLIBC 上限。
- 后端实际启动依赖 libvirt/KVM/OVS 等 Linux 运行环境；Windows 编译通过不等于宿主功能通过。

> 基线风险（仅在 CI 配置未变时适用）：`.github/workflows/build.yml` 当前配置 Node.js `20`，而项目文档和 React Router 8 要求 Node.js `22.22+`。若本次变更触及 CI 或前端构建链路，必须核对 CI 是否能够真实完成前端构建；若 CI 因版本不满足而无法构建，按 blocker 处理。

### 4.2 最小验证集（按变更语言侧选择，不做无关的全量构建）

先确认工作区和提交基线：

```powershell
git status --short --branch
git rev-parse --short HEAD
git rev-parse HEAD
```

#### A. 前端侧（变更涉及 `web/**` 时必跑）

```powershell
Set-Location web
node --version
npm --version
# 仅在 package.json / package-lock.json 变更时重装依赖
npm ci
npm run gen:api      # 仅当变更涉及路由/handler/权限元数据
git diff -- src/views/api-docs/generated/endpoints.json
npm run lint
npm run build
```

通过标准：

- `npm ci`（若执行）退出码为 0，未隐式改写锁文件；
- `npm run gen:api`（若执行）成功生成接口清单，端点数量与当前路由源码一致；
- 若本次没有路由/handler 权限元数据变化，生成文件不应出现无法解释的端点增删；`generated_at` 时间变化需单独识别，禁止把时间戳噪音误判为业务变更；
- `npm run lint` 无 error；
- `npm run build` 中 `tsc -b` 与 Vite 均成功，生成 `web/dist/`；
- 构建日志不得包含明文凭据、私有地址或未解释的动态依赖下载。

#### B. 后端侧（变更涉及 `server/**` 时必跑）

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

项目规则明确说明当前仓库没有测试代码。审查人员不得以「存在自动化测试覆盖」作为通过依据，也不应为了本次审查擅自新增测试框架。可用 `git ls-files` 核对是否仍无 `_test.go`、`*.test.*`、`*.spec.*` 等测试源文件。

#### C. Linux 发行包验证（仅在构建/安装/依赖/兼容性链路变更时执行）

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

### 4.3 启动方式

仓库开发启动命令：

```bash
bash start-dev.sh
```

该脚本启动：

- 后端：`http://localhost:8080`（Air 热重载）；
- 前端：`http://0.0.0.0:5173`（Vite）；
- Vite 将 `/api` 代理到 `http://localhost:8080`。

**安全测试限制：** `start-dev.sh` 明确设置 `KVM_DEVELOPMENT_MODE=true`，会绕过部分安全验证，因此不能用它证明 JWT 二段登录、428 高风险验证、公网门禁等安全控制有效。凡要验证安全控制有效性，必须在专属 Linux 审查机上以 `development_mode=false` 的安装/运行方式执行。

生产安装入口为交互式 `install.sh`，安装服务名为 `kvm-console.service`。安装、更新、兼容性测试会修改宿主机依赖、网络、systemd 与 `/opt/kvm-console`，不得仅为普通代码审查在已有环境直接重跑。

### 4.4 环境能力探测（每次动态测试前必做，禁止沿用历史结论）

执行范围 2/3 前，必须实测下表并在报告中记录**本次实际值**。任何一项未探测或探测不到，对应能力一律标注「未覆盖」，不得引用上一份报告的判定。

| 探测项 | 探测位置/命令 | 记录内容 |
| --- | --- | --- |
| 开发模式 | `KVM_DEVELOPMENT_MODE` 实际值、`server/.env`、启动日志 | 是否 `true`；若为 true，说明二段登录、428、公网门禁均不可验证 |
| JWT/安全密钥 | `KVM_JWT_SECRET`、`KVM_SECURITY_SECRET` 是否显式设置 | 是否触发默认密钥拒绝启动校验 |
| 二段验证可用性 | SMTP 配置、账户 TOTP 绑定状态 | `login_verify`/高风险验证是否可达 |
| 公网开关与代理 | 公网访问开关、`KVM_TRUSTED_PROXIES`、反代/`ufw` 状态 | 公网门禁、会话指纹、限频是否可验证 |
| 服务地址与启动方式 | 后端监听端口、进程管理方式（air/systemd）、前端端口 | 本次 `BASE_URL` 与热重载等待时间 |
| 服务日志路径 | `server/log/` 实际目录 | `app.log`、`cmd.log`、`request.log`、`libvirt.log` |
| 账号状态 | 登录接口返回的 `stage`、`force_password_change`；管理员凭据见 §4.4.1 | 是否需要先解除强制改密；是否需要临时账号 |
| 样本资源 | `virsh list --all`、任务列表、既有模板/网络 | VM 归属隔离、任务隔离用例是否可执行 |

参考值（**历史实测，非承诺值，执行前必须复核**）：审查实例运行在 `http://192.168.11.33:8080`（后端直连，`http://127.0.0.1:8080` 同机可用）与 `http://192.168.11.33:5173`（Vite）；无反向代理，`KVM_TRUSTED_PROXIES` 未配置；后端进程工作目录为仓库 `server/`，数据库为 `server/data/kvm_console.db`。

#### 4.4.1 本开发审查实例的管理员凭据（用户 2026-09-19 指定）

- 管理员用户名 **`admin`**，密码 **`admin123`**。该口令即 `server/config/config.go` 中 `KVM_ADMIN_PASS` 的默认值，也是面板自带管理脚本 `qvmc-manage.sh` 功能 1「重置默认管理员密码」的默认口令（脚本第 140 行 `ADMIN_PASS="${KVM_ADMIN_PASS:-admin123}"`）。
- 需要把口令重置回该值时，优先使用 `qvmc-manage.sh` 功能 1（交互式，需 `sqlite3` CLI）；或在不影响服务的前提下执行其等价 SQL：

```sql
UPDATE users SET password_hash='<bcrypt cost=10 $2a$ 哈希>', totp_enabled=0, totp_secret_enc='',
       totp_recovery_codes_enc='', totp_bound_at=NULL, email='', email_verified_at=NULL,
       updated_at=datetime('now')
WHERE username='admin' AND deleted_at IS NULL;
```

- **适用范围仅限本机开发审查实例，禁止用于生产或任何对外环境。** `admin123` 属于典型弱口令，登录时泄露检测会标记 `password_breached=true`（实测 `password_breach_count=1`）；该账户未绑定 TOTP，按 `server/service/security/password_scan.go:197-203` 不会触发强制改密。
- **首次登录后当次 JWT 会立即失效**：泄露检测对管理员账户首次命中时会刷新 `security_updated_at`，而 `server/middleware/auth.go:274` 以 `IssuedAt < SecurityUpdatedAt` 判定会话失效，接口返回 401「登录状态已失效，请重新登录」。这是预期行为，**重新登录一次**即可正常调用，不得记为缺陷。
- 除本节明文登记的开发实例口令外，其他凭据一律以用户当场提供为准，不得写入仓库、报告、命令历史或日志。

### 4.5 常见构建失败及处理

| 现象 | 判定与处理 |
| --- | --- |
| Go 提示 `go.mod requires go >= 1.26.0` | 升级到 `go.mod` 指定工具链；不得降低 `go` 指令规避 |
| `go-sqlite3` 报 CGO stub 或找不到 C 编译器 | 确认 `CGO_ENABLED=1` 并安装对应平台 C 编译器；重新编译 |
| Node engine/React Router 构建失败 | 使用 Node.js `22.22+`；同时检查 CI 的 Node 配置 |
| `npm ci` 报 package/lock 不同步 | 视为依赖一致性问题；先检查 `package.json` 与 `package-lock.json` 差异，不可直接以 `npm install` 掩盖 |
| `gen:api` 找不到后端源码 | 必须从完整仓库执行；只有发布前端独立构建且已有历史清单时才允许生成器降级沿用旧文件 |
| TypeScript 构建失败但 Vite 能启动 | 仍判定编译失败；以 `npm run build` 的 `tsc -b` 为准 |
| 完整构建提示缺少 Zig | 原生版可单独验证；涉及兼容发行包时必须安装 Zig，不得宣称兼容版通过 |
| 交叉编译缺少 `gcc-*-linux-gnu` | 安装目标架构交叉编译器，或在目标架构主机原生构建 |
| `readelf` 缺失 | 构建脚本会跳过 GLIBC 校验；涉及发布兼容性时此结果不能验收，应补齐工具后重跑 |
| 后端可编译但启动失败 | 检查 libvirt RPC、`/dev/kvm`、OVS、数据库路径、目录权限和环境配置；Windows 不承担运行态验证 |
| 安全接口未返回 428 | 先确认 `development_mode=false`、账户是否仍在高风险信任窗口、SMTP/TOTP 是否可用；不得直接认定验证逻辑通过 |

## 五、静态审查清单（只对适用项出结论）

**适用性规则：** 下列清单均为「变更驱动」条目。审查时先判断本次变更是否触及该条；**未触及的条目一律写 `不适用（本次变更未涉及）` 一行即可，不写论证、不补做检查**。触及的条目必须给出「通过/不通过」及证据（文件:行号、`git diff` 片段或命令输出），不能只写「看起来没问题」。

### 5.1 变更边界与影响面

- [ ] 使用 `git status`、`git diff --stat`、`git diff --name-only` 确认改动边界；判定标准：无无关格式化、依赖目录、构建产物或敏感文件。
- [ ] 从变更入口反查第 2 环影响面；判定标准：变更符号的调用方、被调用方、类型/契约波及点均有结论，且不越界重审环外代码。
- [ ] 按 1.3 判定表逐项核对第 3 环同步项；判定标准：每项要么已同步，要么有「不受影响」的具体证据。
- [ ] VM 新增字段逐链核对 ISO 创建、模板单克隆、批量克隆、链式克隆、OVF/OVA/磁盘导入和编辑载荷；判定标准：不存在只补一条链路导致字段静默丢失。
- [ ] 新增/修改路由后运行接口生成器；判定标准：生成清单、模块分组、中文描述、认证/管理员/高风险元数据与源码一致。

### 5.2 安全与认证（仅当变更触及认证、权限、凭据、中间件或敏感操作时逐条核对）

- [ ] 路由认证边界：逐个**新改**端点核对公开、`AuthMiddleware`、`JWTTokenTypeMiddleware`、`AdminMiddleware`、`ElasticCloudOnlyMiddleware`、`VMAccessMiddleware`；判定标准：最低权限原则成立，无仅靠前端隐藏的授权。
- [ ] JWT 类型限制：access、login、bootstrap、high-risk 令牌不可跨阶段使用；判定标准：账户安全入口 JWT-only，普通业务不接受 login/bootstrap 令牌。
- [ ] API Key 静态兼容性：除账户安全流程外，新增业务接口应允许 API Key；判定标准：路由使用允许 API Key 的认证中间件，高风险业务的 API Key 行为符合项目规则。
- [ ] 高风险验证：所有创建/删除/重装/迁移/网络/存储/凭据等敏感操作调用 `requireHighRiskVerification` 或更强门禁；判定标准：验证 token 绑定正确 `operation`、有效期与用户，不能跨操作复用。
- [ ] 公网开关：核对开发模式互斥、管理员 2FA、API Key 撤销、可信代理和 LAN 判定；判定标准：公网关闭时 API/静态资源/OPTIONS/SSE/WebSocket 均被门禁覆盖。
- [ ] 凭据入口唯一性：核对重复 Authorization、API Key 别名、Bearer/API Key 混用、查询 token 冲突；判定标准：格式异常在数据库查询前统一拒绝，日志不回显凭据。
- [ ] 会话失效：检查密码/用户名/安全状态变更、禁用账户、登出、公网 30 分钟空闲和 SSE/WebSocket 会话校验；判定标准：旧会话按设计失效，前端正确清理状态。
- [ ] 会话指纹：判定标准：仅信任配置过的代理头，IP/User-Agent 变化返回 401，不可由任意客户端伪造转发头绕过。
- [ ] 密码输入：新增或修改的密码入口（登录、邀请、找回/重置、创建/编辑用户、VM 凭据、SSH 密码）均调用项目的强度/泄露检测流程；判定标准：不存在新增输入路径绕过检测，且不记录明文。
- [ ] 密钥与默认值：判定标准：生产启动安全校验能阻止不安全默认值，密钥文件/`.env` 权限合理，新增配置项有安全默认值。
- [ ] 命令注入：变更中新增的外部输入不得直接拼入 `bash -c`；判定标准：优先 `ExecCommand(name, args...)`，确需 shell 时每个外部值经过 `ShellSingleQuote` 或等价严格白名单。
- [ ] 路径与归档安全：变更涉及的路径拼接必须阻断 `..`、绝对路径越权、空字节、符号链接/特殊文件和目录逃逸；判定标准：规范化后做根目录边界校验。
- [ ] SSRF/远程连接：新增的外部目标（节点面板地址、SSH 主机、下载源）需限制协议、凭据和错误信息；判定标准：不能访问未授权本机/元数据地址。
- [ ] 请求与响应日志：判定标准：请求体不记录；新增的敏感字段被纳入递归脱敏；读取日志仅管理员且防路径穿越。
- [ ] 安全响应头与 CORS：判定标准：API `no-store`，静态页面有 CSP；生产 CORS 不应在无必要时为 `*`。
- [ ] 错误详情：判定标准：客户端不接收命令 stderr、SQL、绝对敏感路径、密钥或堆栈。

### 5.3 错误处理与回滚

- [ ] 变更涉及的每个返回 `error`、命令 `ExitCode/Error/Stderr`、GORM 操作均被处理；判定标准：失败后不继续返回成功或写入后续状态。
- [ ] HTTP 状态与 JSON `code` 对齐；判定标准：400/401/403/404/409/428/429/500 语义一致，前端拦截器不会误判成功。
- [ ] 多阶段操作先验证后落地；判定标准：预检失败不写数据库、不修改 XML/网络/磁盘。
- [ ] 新增的虚拟机、模板、存储、网络重配置存在反向补偿；判定标准：每个已成功步骤都有对应清理。
- [ ] 任务取消传播到 `context.Context`；判定标准：停止新增步骤、终止子进程、清理临时文件和部分资源，并返回 canceled 而非 success。
- [ ] 任务部分成功结果可观测；判定标准：宿主阶段成功、来宾阶段失败等情况在结构化结果和消息中明确。
- [ ] 重试幂等；判定标准：重复请求不会重复绑定、重复写规则、重复扣配额或误删其他资源。
- [ ] 数据库多步更新使用事务或条件更新；判定标准：并发下不超配、不覆盖新状态。

### 5.4 并发与任务队列

- [ ] 变更新增的共享 map、缓存、客户端集合有 mutex/atomic 或单线程所有权。
- [ ] 新增 goroutine 有退出条件和 panic recovery；判定标准：关键异步任务使用 `utils.SafeGo` 或显式 `defer RecoverAndLog`。
- [ ] 新增 channel 发送不会永久阻塞请求。
- [ ] 任务归属隔离：判定标准：普通用户只能列出、查看、取消自己的任务；SSE 事件在发送前再次做访问检查。
- [ ] 同一 VM/磁盘/网络对象的冲突操作串行化。
- [ ] SQLite 并发：判定标准：WAL、busy timeout 与立即事务配置未被破坏。
- [ ] 变更涉及的前端 SSE/定时器/订阅在卸载、失焦和登出时清理。
- [ ] 批量克隆/批量操作遵守并发上限；判定标准：结果汇总不把部分失败写成全成功。

### 5.5 资源释放与 I/O

- [ ] 变更新增的 `os.File`、HTTP response body、WebSocket、TCP listener/conn、zip/tar writer、ticker/timer 均在成功创建后及时 `defer Close/Stop`。
- [ ] 新增子进程支持进程树终止；判定标准：取消/普通超时不会留下 `qemu-img`、`rsync`、`tcpdump`、guestfs 等孤儿进程。
- [ ] 大文件复制、镜像转换和网络传输不使用固定自动超时；判定标准：使用 no-timeout/Context 变体，仍可由用户取消。
- [ ] 普通探测命令设置合理上限。
- [ ] 临时文件采用唯一目录、安全权限和原子替换；判定标准：成功、失败、取消与进程重启后均有清理策略。
- [ ] 上传/解包限制文件数量、展开大小、磁盘空间和用户配额；判定标准：校验发生在大量写入前。

### 5.6 硬编码、配置与跨平台

- [ ] 变更新增的端口、路径、网段、网卡、用户名、服务名、架构和固件路径优先来自配置或运行态探测；判定标准：没有只适用于单台机器的新增常量。
- [ ] 变更新增的必要默认值安全且可覆盖；判定标准：环境变量、数据库设置和表单之间优先级明确，保存后 `.env` 权限为 `0600`。
- [ ] 架构专属功能只在对应架构展示并由后端复检；判定标准：x86_64/aarch64 的机型、固件、QEMU 命令和依赖不会串用。
- [ ] Debian/Ubuntu 与 RPM 系包名、服务名和命令差异均处理。
- [ ] 虚拟机运行态不以陈旧 DB 记录为唯一依据；判定标准：关键状态从 libvirt/OVS/文件系统回读。

### 5.7 日志与可观测性

- [ ] 变更新增日志包含模块、资源名、任务 ID、阶段和错误，但不含密码/token/API Key/TOTP/恢复码/私钥。
- [ ] 敏感命令使用 `ExecCommandSensitive*`；判定标准：参数正文不会进入 `cmd.log`。
- [ ] 失败级别正确；判定标准：预期「未找到/无匹配」不刷 error，真实资源修改失败不能只记 debug。
- [ ] 新增耗时操作有 SSE/任务状态可追踪；判定标准：提交、开始、进度、成功/失败/取消都有事件。
- [ ] 日志轮转与权限未被破坏；判定标准：诊断包不意外打包凭据。

### 5.8 依赖与构建配置

- [ ] 依赖变更同时更新锁文件并说明理由；判定标准：无未使用依赖、无直接编辑 `node_modules`。
- [ ] 依赖升级检查运行时要求和破坏性变更；判定标准：React Router、React、Semi、Vite、Go/CGO 与 CI 工具链一致。
- [ ] 执行 `npm audit`/适当依赖审计时记录结果和误报判断；判定标准：高危漏洞有处置结论。
- [ ] GitHub Actions 变更与本地命令一致；判定标准：CI 使用满足依赖要求的 Node/Go。
- [ ] 生成文件可重现；判定标准：API 端点清单除可解释的路由/权限/时间字段外无随机漂移。

### 5.9 前端项目规范

- [ ] 修改 Semi 组件前阅读 `semi-design-guide` skill；判定标准：组件 API 与当前 Semi 版本匹配。
- [ ] 行内操作采用纯图标 + Tooltip，超过 2~3 个时收进 `⋯`；判定标准：危险项标红、加载态用旋转图标。
- [ ] Switch 使用内部单字符 `checkedText/uncheckedText`；判定标准：无外置重复状态文字。
- [ ] 条件挂载的 Semi Modal 使用 `useMountModalLifecycle.ts`；判定标准：先 `visible=false`，`afterClose` 后卸载。
- [ ] 深色模式大面积文字使用柔和灰而非近白 `--qvm-text-0`；判定标准：浅色优先、暗色对比不刺眼。
- [ ] 复制功能使用 `copyTextWithFallback`；判定标准：HTTP 非安全上下文仍可降级复制。
- [ ] 所有密码输入接入本地强度与后端泄露检测；判定标准：创建、编辑和弹窗入口一致。
- [ ] 角色/云类型/架构只在前端隐藏还不够；判定标准：后端存在同等或更严格校验。

## 六、接口验证（按变更映射，不做全量回归）

> 本章仅在范围 2/3 触发时执行（见 3.1、3.2）。**禁止**每次审查重跑同一批固定端点；测试集必须能追溯到本次 diff。

### 6.1 变更 → 端点映射

1. 从 diff 提取变更涉及的 `router.go` 行、handler 函数、中间件挂载点；
2. 用 `web/src/views/api-docs/generated/endpoints.json`（或变更后重新生成）与 `server/router/router.go` 确认端点全路径、方法、认证方式与高风险标识；
3. 产出**本次测试矩阵**，只包含：变更端点本身 + 其认证/权限边界的必要对照项。

请求体字段一律以 `endpoints.json`、`endpointDescriptions.ts` 和对应 handler/结构体为准，禁止凭经验臆造。

### 6.2 最小测试集（默认）

对每个受影响端点：

- 1 条合法凭据成功路径（含返回值结构、状态码与脱敏检查）；
- 1 条失败或边界路径（缺参、越权、非法状态）；
- 若该端点为高风险操作，默认**只做静态审查**，动态测试按 3.3 闸门执行；
- 变更涉及写操作时，测试后必须核对资源与任务状态清理（见 6.8）。

### 6.3 横切变更的边界抽样（仅当变更触及中间件/认证/响应封装/任务队列时）

此时才允许对该横切面做一次针对性抽样，抽样项按变更内容选择，例如：

```bash
# 凭据格式与来源冲突（仅在变更触及 auth/credential_guard 时执行）
curl -sS -i "$BASE_URL/api/auth/info"
curl -sS -i -H 'Authorization: Bearer not-a-jwt' "$BASE_URL/api/auth/info"
curl -sS -i -H "Authorization: Bearer $JWT" -H 'Authorization: Bearer another.invalid.token' "$BASE_URL/api/auth/info"
curl -sS -i -H "Authorization: Bearer $JWT" --get --data-urlencode "token=$JWT" "$BASE_URL/api/task/list"

# 权限分隔（仅在变更触及 AdminMiddleware/VMAccessMiddleware 时执行）
curl -sS -i -H "Authorization: Bearer $USER_JWT" "$BASE_URL/api/security/password-breach/status"
```

预期：未认证 401；格式/来源冲突在凭据守卫处拒绝且不回显凭据；越权 403；查询参数 `token` 在 `request.log` 中被脱敏。**这些用例只在上述条件成立时执行，不作为每次审查的固定动作。**

### 6.4 凭据准备（按需，绝不复用历史值）

- 凭据来源：§4.4.1 明文登记的开发实例口令（`admin` / `admin123`），或用户当场提供；除该登记口令外，其他密码一律通过 `read -s` 交互输入，不写入命令行参数、脚本、仓库或报告。
- 本开发实例的标准取凭据方式（无需交互输入）：

```bash
export BASE_URL='http://127.0.0.1:8080'
export JWT=$(curl -sS -X POST -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' \
  "$BASE_URL/api/auth/login" | jq -r '.data.token')
```

- **若上一步之后首次调用业务接口返回 401「登录状态已失效，请重新登录」，属预期行为**（首次登录触发泄露检测刷新 `security_updated_at`，见 §4.4.1），再执行一次上述登录命令取得新 token 即可，不要记为缺陷。
- 若探测（4.4）显示账户处于 `force_password_change=true`，除 `/api/auth/info`、`PUT /api/auth/password`、`/api/auth/logout`、`/api/public/*` 外所有接口返回 403，此时必须先完成改密再取测试凭据：

```bash
export BASE_URL='<本次探测得到的后端地址>'
read -r -s -p '当前密码: ' CUR_PWD; echo
export LOGIN_RESP=$(curl -sS -X POST -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$CUR_PWD\"}" "$BASE_URL/api/auth/login")
echo "$LOGIN_RESP" | jq '{stage:.data.stage, force_password_change:.data.force_password_change}'
export BOOT_JWT=$(echo "$LOGIN_RESP" | jq -r '.data.token'); unset LOGIN_RESP
read -r -s -p '新密码: ' NEW_PWD; echo
curl -sS -X PUT -H "Authorization: Bearer $BOOT_JWT" -H 'Content-Type: application/json' \
  -d "{\"old_password\":\"$CUR_PWD\",\"new_password\":\"$NEW_PWD\"}" \
  "$BASE_URL/api/auth/password" | jq '{code,message}'
export JWT=$(curl -sS -X POST -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$NEW_PWD\"}" "$BASE_URL/api/auth/login" | jq -r '.data.token')
unset CUR_PWD NEW_PWD BOOT_JWT
```

- 新密码一经设置无法回退为默认值，提醒用户妥善保存；
- 需要普通用户对照时，按 `endpoints.json` 与 `server/handler/user.go` 的真实字段创建一次性账号，测试结束后删除；
- 账号安全流程（login/bootstrap/high-risk）的动态测试仅在 4.4 探测确认可达时执行，不可达时只做静态审查并标注「未覆盖」。

### 6.5 高风险验证与受控写入

仅在本次变更直接涉及高风险模块（VM 创建/删除/重装/迁移、模板与导入、OVS/VPC/端口安全、公网 IP、防火墙、存储格式化与迁移、IOMMU/VFIO、公网访问切换、密钥轮换）时，按 3.3 闸门在用户确认快照后执行，并优先选择副作用可控、可清理的用例。

若开发模式导致接口直接执行而不返回 428，这属于环境所致，**不得记为缺陷**；应记录为「428 链路未验证（环境不支持）」，并以静态审查结论为准。

### 6.6 SSE 与连接释放（仅当变更触及 SSE/WebSocket/任务推送时）

```bash
curl -N --max-time 15 -H "Authorization: Bearer $JWT" "$BASE_URL/api/task/sse"
```

预期：HTTP 200、`Content-Type: text/event-stream`、首个事件为 `connected`、15 秒后客户端退出且服务端连接计数回落、事件不跨用户泄漏。并发/慢客户端压测需单独规定连接数，不得在共享审查机无限加压。

### 6.7 限频、登录锁定与公网专项（仅当变更触及对应中间件/配置时）

- 公开/认证接口限频及 `Retry-After`；
- 登录失败计数与 429；
- 公网关闭时非 LAN 请求统一 403；
- 公网 JWT 30 分钟无真实操作失效；
- 可信代理下 `X-Forwarded-For` 解析；
- 会话指纹的 IP/User-Agent 变化。

这些测试可能影响同源 IP 或账号，禁止为测试登录锁定连续提交错误密码以免锁定唯一管理员；执行前需补充代理拓扑、来源 IP、限频配置和一次性账号。条件不满足时标注「未覆盖」，不重复尝试。

### 6.8 接口测试结束检查

- [ ] 删除所有明确标记为审查用途的临时数据；
- [ ] 所有异步任务已终态，无 pending/running 残留；
- [ ] 无残留 VM、磁盘、NVRAM、OVS 端口、流表、meter、iptables/nftables 规则、上传分片或临时目录；
- [ ] `request.log`、`cmd.log`、`app.log` 不含本次 JWT、密码、验证码或高风险 token；
- [ ] 服务、libvirt、OVS、基础网络和原有 VM 状态与测试前一致；
- [ ] 若状态无法恢复，停止测试并使用用户确认的审查机快照回滚；
- [ ] 审查报告仅保留脱敏请求和响应摘要。

## 七、验收标准与严重级别定义

### 7.1 总体验收标准（变动审计口径）

只有同时满足以下条件，才能给出「可通过」结论：

1. 变更语言侧的编译/静态检查全部通过（后端 `go vet ./...`、`go build ./...`；前端 `npm run lint`、`npm run build`）；未涉及的侧不做、也不计入结论；若受环境阻断，必须列明阻断原因，不能写成通过；
2. 变更清单中每个文件都有明确结论（通过/不通过/不适用），第 3 环同步项逐项有判定证据；
3. 若触发范围 2/3，变更映射出的接口/浏览器用例已执行并记录实际结果；未执行的项写明「未覆盖 + 原因」，不得默认通过；
4. 无未解决 blocker；major 必须修复或由用户书面接受风险并给出补救计划；
5. 变更触及权限/隔离时，用户与管理员、弹性云与轻量云、VM 归属与任务归属隔离有实际证据；
6. 变更涉及的代码、日志与本次报告本身均无明文密钥、密码、JWT、验证码、恢复码、私钥泄露；
7. 变更涉及的耗时操作进入任务队列，支持取消、进度、失败清理和幂等重试；
8. 变更涉及的 VM、存储和网络操作具备可验证的回滚路径，不留下孤儿资源；
9. 依赖、安装脚本、兼容性测试和 docs 按变更影响同步；
10. 审查结束后 `git status` 仅包含审查前已有变更和预期审查产物，未混入构建产物或凭据。

### 7.2 严重级别

| 级别 | 判定界限 | 本项目示例 |
| --- | --- | --- |
| `blocker` | 无法编译/启动/发布；可导致认证绕过、跨租户控制、宿主机失联、不可恢复数据丢失、密钥明文泄露或大范围资源破坏；没有安全回滚 | 非管理员可操作他人 VM；格式化错误磁盘；OVS 重配置切断管理网络且无回滚；JWT 密钥暴露；CI 工具链必然无法构建发布 |
| `major` | 核心功能错误或高概率产生错误状态；权限、高风险验证、配额、并发、任务取消、清理、兼容性存在实质缺陷，但影响范围可控或可恢复 | 高风险路由漏 428；任务显示成功但资源未完成；批量链路漏字段；取消后残留磁盘/进程；普通用户看到他人任务；日志泄露 token |
| `minor` | 不阻断主流程、影响局部可用性/可维护性/诊断质量，存在明确绕行方式且不会造成权限或数据风险 | 错误文案不精确、非关键状态刷新滞后、部分异常缺少上下文、文档小范围落后、暗色模式局部对比不佳 |
| `suggestion` | 当前行为正确，仅为一致性、性能余量、可读性或未来扩展建议 | 提取重复函数、改善命名、减少无害重复请求、补充注释或更细指标 |

严重级别以「实际最坏影响 + 可利用性/发生概率 + 可恢复性」为准，不能因修改行数少而降级。安全与租户隔离问题至少为 major；可直接利用或影响宿主机/全体用户时为 blocker。

**范围归属：** 非本次变更引入的历史问题不计入本次结论的验收门槛，应单独标注「历史遗留（非本次变更引入）」并给出所在位置，供用户决定是否另行处理。

## 八、报告要求

报告写入 `review/reports/REVIEW-<yyyyMMdd>-<endCommit前8位>.md`，必须包含：

1. **审查时间**：ISO 8601（含时区），为报告实际生成时间；
2. **git 提交范围**：`from..to` 完整 sha，附范围内提交简要列表（短 sha + 标题）；
3. **变更清单**：按变更文件列出增删行数统计，并标注每个文件所属标签（后端接口类/业务类/前端类/系统类/文档类）；
4. **逐文件结论**：每个变更文件的审查结论与证据（文件:行号 + 代码摘录）；
5. **第 3 环同步项判定表**：本次涉及的同步项及「已同步/不受影响 + 证据」；
6. **发现列表**：按严重级别排序，每条包含 `文件:行号`、问题描述、证据（代码摘录）、修复建议；非本次变更引入的问题单独分组标注；
7. **编译结果**：实际执行的命令与输出结论；未执行的语言侧写明原因；
8. **接口/浏览器测试结果表**：如触发范围 2/3，逐条记录端点的请求摘要、角色、HTTP 状态、判定；未执行项写明原因；
9. **未审查范围声明**：本次未覆盖的文件/能力及原因（环境受限、不在变更范围等），禁止复述上一份报告的历史结论；
10. 报告语言为中文；代码与路径保持原样；报告不记录任何凭据值。

## 九、附录

### 9.1 文档修订信息

- 本版修订时间：`2026-09-19T15:56:00+08:00`
- Git 分支：`main`
- Git HEAD（短）：`5ae09fe`
- Git HEAD（完整）：`5ae09fed6417a475b57d96de9d38143b594da97e`
- 上一版基线：`c5583abdcab9ff92dea62283ce239e67836c496c`（2026-09-19 13:30:32 +08:00）
- 本版主要变更：
  1. 新增第一章「核心原则：变动审计」，确立「只审变更 / 不重复测试 / 不继承结论」三条硬约束与三环影响面模型；
  2. 审查范围改为自适应（默认范围 1，范围 2/3 按变更触发），删除固定「包含生产环境测试」的声明；
  3. 原第三章的一次性环境实测结论改为第四章 4.4「环境能力探测」，每次审查必须重新实测，禁止沿用历史判定；
  4. 原第五章固定全量接口测试清单改为第六章「按变更映射的接口验证」，横切边界抽样仅在变更触及中间件/认证/任务队列时执行；
  5. 静态审查清单改为适用性判定驱动，未触及条目一律记「不适用」；
  6. 验收标准与报告要求改为变动审计口径，并明确「非本次变更引入」问题的归属方式；
  7. 补充第八章报告要求，第九章附录并重新编号（原文档缺「六」）；
  8. 按用户 2026-09-19 指示，新增 §4.4.1「本开发审查实例的管理员凭据」（`admin` / `admin123`）、口令重置方式与首次登录 JWT 失效说明，并在 §6.4 同步标准取凭据命令。
- CHANGELOG：项目根目录未发现项目级 `CHANGELOG*`；依赖目录中的 CHANGELOG 不作为项目变更记录
- 测试代码：跟踪文件中未发现项目测试源文件，与 `AGENTS.md` 的「本项目没有测试代码」一致

### 9.2 主要参考文件

规则与说明：

- `~/.dsh/AGENTS.md`（全局规则）
- `AGENTS.md`
- `README.md`
- `DEPENDENCIES.md`
- `docs/*.md`

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