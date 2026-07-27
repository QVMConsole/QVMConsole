# QVMConsole 新前端（React + Semi Design）重构迁移细则

> **版本**: v1.0  
> **更新日期**: 2026-07-26  
> **维护者**: QVMConsole Core Team  

---

## 1. 项目概述与目标

### 1.1 项目背景
- **原项目**：Vue 3.5 + Element Plus + Pinia + Vite
- **新架构**：React 19 + Semi Design + Zustand + Vite
- **核心目标**：
  - ✅ 完全保留现有所有功能（后台管理/虚拟机操作/API 集成）
  - ✅ 提升代码可维护性与扩展性
  - ✅ 适配深色模式（Semi 官方方案）
  - ✅ 模块化拆分巨型组件（VmForm/VmDetail/NetworkList → 多个独立子组件）
  - ✅ 支持多人并行开发（清晰的任务拆分与依赖隔离）

### 1.2 已完成地基工作
| 类别 | 文件路径 | 状态 | 说明 |
|------|---------|------|------|
| **配置** | `web/src/config/constants.ts` | ✅ | 全局常量（存储键位、云类型、角色、主题模式） |
| | `web/src/config/site.ts` | ✅ | 站点信息管理（标题同步、公开设置加载） |
| **API 层** | `web/src/api/client.ts` | ✅ | Axios 实例封装（428 高风险验证自动处理、错误提示、Token 注入） |
| | `web/src/api/auth.ts` | ✅ | 认证接口（login/userInfo/checkPasswordBreach） |
| | `web/src/api/settings.ts` | ✅ | 系统设置接口（getPublicSettings） |
| **状态管理** | `web/src/stores/user.ts` | ✅ | 用户 Store（token/username/role/cloud_type/security 持久化） |
| | `web/src/stores/app.ts` | ✅ | 应用 Store（主题/侧边栏状态/站点信息） |
| | `web/src/stores/highRisk.ts` | ✅ | 高风险验证 Store（428 弹窗流程、2FA/邮箱码） |
| **页面** | `web/src/views/login/index.tsx` | ✅ (设计版) | 深空极光登录页（渐变背景图 + 品牌区），stage=success 分支已实现 |
| | `web/src/views/dashboard/index.tsx` | ✅ | 角色双视图：管理员（理论最大量双条/监控/存储池）+ 用户（配额/VM 追踪） |
| | `web/src/views/error/NotFound.tsx` | ✅ | 404 页面 |
| **布局** | `web/src/layout/index.tsx` | ✅ | 深空极光主布局：悬浮侧边栏 / 顶部标签页栏 / 底部任务栏（SSE） |
| **路由** | `web/src/router/index.tsx` | ✅ | React Router 路由（懒加载、RequireAuth 守卫） |
| **工具** | `web/src/utils/clipboard.ts` | ✅ | 剪贴板兼容降级（HTTP 场景） |
| | `web/src/utils/validate.ts` | ✅ | 密码校验（弱密码库+HIBP 检测 + 生成强密码） |
| **组件** | `web/src/components/business/HighRiskChallengeModal.tsx` | ✅ | 428 高风险二次验证弹窗 |
| **Hook** | `web/src/hooks/useTheme.ts` | ✅ | 主题 Hook（system/light/dark） |
| **样式** | `web/src/assets/styles/index.css` | ✅ | 全局样式（NProgress/Semi 颜色对齐、滚动条） |

> **验证通过**：`npm run build` 成功，分包清晰（react-vendor/semi-vendor），开发服务器已启动于 http://localhost:5173。

---

## 2. 迁移优先级与任务拆分

> **说明**：按 P0→P1→P2→P3 顺序执行，每阶段完成后需自我测试并更新本 README 的"完成状态"列。

| 优先级 | 模块 | 后端 API 范围 | 预计工作量 | 负责人 | 状态 | 备注 |
|--------|------|--------------|------------|--------|------|------|
| P0 | **认证完整流程** | `/auth/*` | 3 天 | [待分配] | 🟡 地基已完 | 多阶段登录 stage 状态机 |
| P0 | **仪表盘（Dashboard）** | `/host/stats`(admin)/`/self/quota`(user) | 2 天 | [待分配] | ✅ 已完成 | SSE 实时推送；管理员含理论最大量双进度条 |
| P1 | **虚拟机列表** | `/vm/list` / `/self/vms` + SSE | 4 天 | [待分配] | ⬜ | 缓存优先机制/分组逻辑/批量保护 |
| P1 | **虚拟机详情页** | `/vm/:id` + createVmDetailSSE | 5 天 | [待分配] | ⬜ | SSE 详情通道/懒加载策略/Tab 惰性加载 |
| P2 | **VNC 控制台** | `/vm/:id/vnc/ws` + @novnc/novnc | 3 天 | [待分配] | ⬜ | noVNC 连接逻辑/快捷键发送 |
| P2 | **SPICE 控制台** | `/vm/:id/spice/*` | 2 天 | [待分配] | ⬜ | 状态标签/对外暴露/.vv下载 |
| P2 | **任务中心** | `/task/sse` + `/task/list` | 2 天 | [待分配] | ⬜ | SSE 进度更新/最近任务面板 |
| P3 | **资源图表** | `/vm/:id/stats` / `/host/stats` | 2 天 | [待分配] | ⬜ | ECharts 四图联动/增量计算 |
| P3 | **模板管理** | `/template/*` | 4 天 | [待分配] | ⬜ | 模板族树形渲染/删除三种模式/分片上传 |
| P3 | **网络中心/VPC** | `/network/*`, `/vpc/*` | 6 天 | [待分配] | ⬜ | 桥接直通/NAT 双模式/端口转发封禁 |
| P4 | **存储池管理** | `/storage-pool/*` | 3 天 | [待分配] | ⬜ | LVM/分区格式化/ISO 聚合查询 |
| P4 | **用户管理** | `/user/*` | 4 天 | [待分配] | ⬜ | 配额表单/轻量云注册状态机 |
| P4 | **系统设置** | `/settings/*` | 5 天 | [待分配] | ⬜ | 8 个 Tab 深度配置 |
| P5 | **防火墙/OVS** | `/firewall/*` / `/ovs/*` | 4 天 | [待分配] | ⬜ | nftables 规则生成/GeoIP 区域导入 |
| P5 | **API 文档页** | 静态展示 | 1 天 | [待分配] | ⬜ | 从旧前端 docs.js 转换 |

> **里程碑建议**：
> - Day 3：P0 认证 + Dashboard 完成并自测
> - Day 10：P1 虚拟机列表 + 详情页完成
> - Day 15：P2 VNC/SPICE/任务中心完成
> - Day 30：全部 P3/P4 完成
> - Day 35：全部功能上线准备

---

## 3. 各模块实现指南

### 3.1 P0 - 认证完整流程（/auth/*）

#### 3.1.1 需求概述
- 实现**多阶段登录流程**（stage 状态机）：`login` → (`login_verify` or `bootstrap_security`) → `success`
- 支持分支：强制改密、安全初始化引导、恢复码一次性展示、找回密码三步链
- HIBP 泄露密码检测（前后端异步校验）

#### 3.1.2 后端接口映射
| 接口 | 方法 | 请求参数 | 响应结构 | 注意事项 |
|------|------|----------|----------|----------|
| `/auth/login` | POST | `{ username, password }` | `{ stage, token?, username, role, cloud_type, security, force_password_change? }` | stage=success/login_verify/bootstrap_security |
| `/auth/login/email/send` | POST | `{}` (headers: Authorization: Bearer ${stageToken}) | `{ challenge_id?, masked_email? }` | 仅 login_verify 阶段可用 |
| `/auth/login/verify` | POST | `{ method, code, challenge_id? }` (headers: stageToken) | `{ stage==='success'? token:undefined,... }` | 返回 stage='success' 意味着最后一项必做项 |
| `/auth/info` | GET | headers: Bearer ${token} | `{ id, username, role, cloud_type, security }` | 同步安全初始化状态 |
| `/auth/email/code/send` | POST | `{ email }` (headers: bootstrapToken) | `{ challenge_id? }` | 安全初始化阶段用 |
| `/auth/email/bind` | POST | `{ email, code }` (headers: bootstrapToken) | `{ stage?: 'success' }` | stage='success' 可直接进入系统 |
| `/auth/2fa/setup` | POST | (headers: bootstrapToken) | `{ qr_code_base64, secret }` | 生成 TOTP 配置 |
| `/auth/2fa/enable` | POST | `{ totp_code, recovery_codes? }` (headers: bootstrapToken) | `{ stage?: 'success', recovery_codes? }` | 返回 recovery_codes 仅显示一次 |
| `/auth/skip-bootstrap` | POST | `{ confirm: true }` (headers: bootstrapToken) | `{ stage?: 'success' }` | 管理员跳过安全初始化 |
| `/password/forgot/send-code` | POST | `{ email }` | `{ challenge_id?, masked_email? }` | 找回密码第一步 |
| `/password/forgot/verify-code` | POST | `{ email, code }` | `{ accounts[], selection_token? }` | 第二步：选择账号 |
| `/password/forgot/select-account` | POST | `{ selection_token, account_username }` | `{ reset_token }` | 第三步：跳转重置页 |

#### 3.1.3 前端组件分解
```
src/views/login/
├── index.tsx                 # 主入口：stage 状态机控制器
├── LoginForm.tsx             # Stage 1: 账号密码登录表单
├── LoginVerifyForm.tsx       # Stage 2: 二次验证表单（TOTP/恢复码/邮箱码切换）
├── BootstrapSecurityForm.tsx # Stage 3: 安全初始化表单（SMTP/绑定邮箱/绑定 2FA）
├── ForgotPasswordDialog.tsx  # 对话框：找回密码三步流程
├── ForceChangePasswordDialog.tsx # 对话框：首次登录强制改密
└── RecoveryCodesDisplay.tsx  # 对话框：恢复码展示（编号网格 + 复制/下载）
```

#### 3.1.4 关键业务逻辑伪代码

```typescript
// src/stores/auth.ts (新增)
interface AuthState {
  stage: 'login' | 'login_verify' | 'bootstrap_security' | 'success'
  stageToken: string | null      // 临时阶段令牌（login_verify/bootstrap_security 用）
  accessToken: string | null     // 最终访问令牌
  userInfo: UserInfo | null
  pendingForcePwdSession: boolean   // 强制改密暂存 flag
  holdRecovery: RecoveryCodesData | null // 恢复码 hold 机制
  
  setStage(stage): void
  setStageToken(token): void
  applySuccessSession(token, userInfo): void   // 正式进入系统
  holdForRecovery(codes): void                 // 暂停会话直到恢复码确认
  confirmRecovery(): void                      // 确认恢复码后 applySuccessSession
}

// src/views/login/LoginForm.tsx
async function handleLogin(username, password) {
  const res = await api.login({ username, password })
  const { stage, token, username, role, cloud_type, security, force_password_change } = res.data

  if (stage === 'success') {
    if (force_password_change) {
      // 暂存 session，等待改密成功
      store.pendingForcePwdSession = true
      openForceChangePasswordDialog()
    } else {
      store.applySuccessSession(token, { username, role, cloud_type, security })
      navigate('/')
    }
  } else if (stage === 'login_verify') {
    store.setStage('login_verify')
    store.setStageToken(token)
  } else if (stage === 'bootstrap_security') {
    store.setStage('bootstrap_security')
    store.setStageToken(token)
  }
}

// src/views/login/LoginVerifyForm.tsx
async function handleSubmit(method, code) {
  const res = await api.verifyLoginStage({ method, code }, stageToken)
  if (res.data.stage === 'success' && res.data.token) {
    store.applySuccessSession(res.data.token, ...)
    navigate('/')
  } else {
    // 继续留在当前 stage
  }
}

// src/views/login/RecoveryCodesDisplay.tsx
const handleDownload = () => {
  const blob = new Blob([codes.join('\n')], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'qvmconsole-recovery-codes.txt'
  a.click()
}

// src/views/login/ForgotPasswordDialog.tsx
const handleSelectAccount = async (username) => {
  const res = await api.selectForgotPasswordAccount(selection_token, username)
  const { reset_token } = res.data
  navigate('/reset-password', { state: { token: reset_token } })
}
```

#### 3.1.5 测试用例清单
- [ ] 正常用户登录（直接 success）→ 跳转首页
- [ ] 首次登录强制改密 → 改密成功后跳转首页
- [ ] 开启 2FA 的用户登录 → login_verify 页面 → 输入 TOTP 码 → success
- [ ] 开启 2FA 的用户登录 → login_verify 页面 → 选择恢复码（16 位）→ success
- [ ] 开启 2FA 的用户登录 → login_verify 页面 → 取消验证 → 返回 login 页面
- [ ] SMTP 未配置的管理员登录 → bootstrap_security 页面 → 填写 SMTP 配置 → 保存
- [ ] must_bind_email=true → 绑定邮箱（6 位验证码）→ success
- [ ] must_bind_2fa=true → 生成 2FA → 扫码 → 输入 6 位验证码 → 恢复码展示对话框 → 点击下载 → 点击"我已安全保存" → success
- [ ] 管理员跳过安全初始化 → 风险确认对话框 → skipBootstrap → success
- [ ] 找回密码三步链：忘记密码 → 输入邮箱 → 接收验证码 → 选择账号 → 跳转到/reset-password?token=xxx → 设置新密码 → 自动登录

---

### 3.2 P0 - 仪表盘（Dashboard）

#### 3.2.1 需求概述
- **管理员视图**：宿主机实时监控（SSE 推送）、物理机概览、磁盘空间、24h 图表
- **普通用户视图**：个人配额总览、分类配额详情、自己的 VM 资源追踪
- **公共资源图表**：ECharts 环形图（CPU/内存/Swap）、历史查询

#### 3.2.2 后端接口映射
| 接口 | 方法 | 权限 | 返回字段 | 频率 |
|------|------|------|----------|------|
| `/host/stats` | GET | admin only | `cpu_percent, mem_used, mem_total, net_rx_bytes, net_tx_bytes, disk_rd_bytes, disk_wr_bytes, disk_rd_ops, disk_wr_ops, swap_used, hostname, arch` | SSE 推送每 3s |
| `/host/disks` | GET | admin only | `mount_point, device, fstype, total_kb, used_kb` | 页面加载时 |
| `/self/quota` | GET | user only | `quota_cpu, quota_memory_gb, quota_vm_count, quota_disk_gb, quota_snapshot, quota_runtime_hours, ...` | 页面加载时 |
| `/self/vms` | GET | user only | `[{ id, name, status, vcpu, memory_gb, ip_address, template_name }]` | 页面加载时 |
| `/public/settings` | GET | public | `site_title, smtp_configured, maintenance_mode` | 页面加载时 |

#### 3.2.3 前端组件分解
```
src/views/dashboard/
├── index.tsx                       # 主入口：角色分支渲染器
├── AdminView.tsx                   # 管理员视图
│   ├── RealTimeStats.tsx           # 4 张环形卡（CPU/Mem/Swap/DiskIO）
│   ├── HostOverviewCards.tsx       # 物理机概览卡
│   ├── DiskSpaceTable.tsx          # 磁盘挂载表
│   └── HistoryCharts.tsx           # 24h 图表（复用 ResourceCharts 组件）
├── UserView.tsx                    # 普通用户视图
│   ├── QuotaSummaryCards.tsx       # 资源总览 5 张紧凑卡
│   ├── QuotaDetailsAccordion.tsx   # 配额详情 3 个折叠分类
│   └── MyVmResourceTracker.tsx     # VM 资源追踪折叠面板
└── SharedComponents.tsx            # Shared: SMTPWarningBanner, EmptyState
```

#### 3.2.4 SSE 实时推送实现指南

```typescript
// hooks/useHostStatsSSE.ts (新增)
export function useHostStatsSSE(token: string) {
  const [stats, setStats] = useState<HostStats | null>(null)
  const [sseStatus, setSseStatus] = useState<'connecting'|'connected'|'disconnected'>('connecting')
  let eventSource: EventSource | null = null

  useEffect(() => {
    if (!token) return

    const url = `/api/host/stats/sse?token=${encodeURIComponent(token)}`
    eventSource = new EventSource(url)

    eventSource.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data) as HostStats
        setStats(data)
      } catch (err) {
        console.error('解析 SSE 事件失败', err)
      }
    }

    eventSource.onerror = () => {
      setSseStatus('disconnected')
      eventSource?.close()
      setTimeout(() => {
        setSseStatus('connecting')
      }, 5000)
    }

    return () => {
      eventSource?.close()
    }
  }, [token])

  return { stats, sseStatus }
}
```

#### 3.2.5 ECharts 环形图懒加载实现

```typescript
// components/charts/RingChart.tsx (新增)
export function RingChart({ title, data, max, color }: { title, data, max, color }) {
  const chartRef = useRef<HTMLDivElement>(null)
  const [inited, setInited] = useState(false)

  useEffect(() => {
    if (!inited || !chartRef.current) return
    const chart = echarts.init(chartRef.current)
    chart.setOption({
      series: [{ 
        type: 'gauge', 
        progress: { show: true, width: 18, roundCap: true, itemStyle: { color } },
        axisLine: { lineStyle: { width: 18 } },
        data: [{ value: data, name: `${data.toFixed(1)}%`, nameGap: 10, offsetCenter: ['0%', '30%'] }]
      }]
    })
    setInited(true)
    return () => chart.dispose()
  }, [data, inited])

  return <div ref={chartRef} style={{ height: '200px' }} />
}
```

---

### 3.3 P1 - 虚拟机列表

#### 3.3.1 需求概述
- **核心功能**：列表/卡片双视图、分组（status/template/custom）、搜索、分页、单机/批量电源操作
- **特殊逻辑**：缓存优先 + SSE 静默更新、锁定/迁移中保护、IP/磁盘按需加载、轻量云受限
- **子组件**：VmDeleteDialog、VmMigrationDialog、VmReinstallDialog、VmRemarkDialog、VmGroupDialog、SnapshotManage

#### 3.3.2 后端接口映射
| 接口 | 方法 | 权限 | 返回字段 | 备注 |
|------|------|------|----------|------|
| `/vm/list` | GET | admin | `[{ id, name, status, vcpu, memory_gb, ip_address, disk_gb, group, remark, created_at, ... }]` | 含 vm_list SSE 增量更新 |
| `/self/vms` | GET | user | 同左 | 用户视角过滤 |
| `/vm/:name/ip` | GET | 通用 | `{ ip_address }` | 点击加载 IP |
| `/vm/:name/disks` | GET | 通用 | `[{ dev, size_gb, used_gb, format, bus, path }]` | 查看占用 |
| `/vm/:name/operate` | POST | 通用 | `{ task_id }` | action=start/shutdown/reboot/destroy/reset |
| `/vm/:name/rescue` | POST | 通用 | `{ task_id }` | action=enable/disable |
| `/vm/:name/lock` | GET/POST | 通用 | `is_locked` / `{ task_id }` | 锁定/解锁 |
| `/vm/:name/migration/preview` | POST | admin | `nodes[], migration_options` | 跨节点迁移预览 |
| `/vm/:name/reinstall` | POST | 通用 | `{ task_id }` | 重装系统 |

#### 3.3.3 前端组件分解
```
src/views/vm/
├── index.tsx                       # 主入口：角色分支 + 分组控制器
├── VmListView.tsx                  # 表格视图（双表格：分组表格/大表格）
├── VmCardView.tsx                  # 卡片视图（PC 卡片网格/移动端移动卡片）
├── VmFilters.tsx                   # 筛选栏（用户名搜索/邮箱搜索/角色/状态/类型）
├── VmPagination.tsx                # 分页组件（仅 filteredTableData > pageSize 显示）
├── VmActionToolbar.tsx             # 批量操作条（开机/关机/删除按钮组）
├── VmMaintenanceMask.tsx           # 维护模式遮罩（content 模糊 + 居中提示）
├── SubComponents/
│   ├── VmDeleteDialog.tsx          # 删除确认（单机/批量、三分支选择）
│   ├── VmMigrationDialog.tsx       # 跨节点迁移（预览 + 提交）
│   ├── VmReinstallDialog.tsx       # 重装系统（选择镜像/配额警告）
│   ├── VmRemarkDialog.tsx          # 编辑备注
│   ├── VmGroupDialog.tsx           # 编辑分组（候选取自现有所有分组）
│   └── SnapshotManage.tsx          # 快照管理（配额徽标 + CRUD）
└── SharedComponents/
    ├── VmStatusTag.tsx             # 状态 tag（运行中/已关机/迁移中...）
    ├── VmResourceBar.tsx           # CPU/MEM 双进度条（70%/90% 变色）
    └── VmContinuousRunTime.tsx     # 连续运行时长格式化（x 天 x 小时...）
```

#### 3.3.4 关键业务逻辑伪代码

```typescript
// stores/vm.ts (补充 visitedVms 逻辑)
export interface VisitedVm { id: string; name: string }
export const useVmStore = create<VmStore>((set) => ({
  vmList: [],
  lastFetchTime: 0,
  visitedVms: JSON.parse(localStorage.getItem('visitedVms') || '[]') as VisitedVm[],
  
  setVmList(data) {
    set({ vmList: data, lastFetchTime: Date.now() })
    // 记录最近访问
    data.forEach(vm => {
      const idx = state.visitedVms.findIndex(v => v.id === String(vm.id))
      if (idx === -1) set(state => ({ visitedVms: [...state.visitedVms, { id: String(vm.id), name: vm.name }] }))
      else set(state => ({ visitedVms: state.visitedVms.map((v, i) => i===idx ? {id:String(vm.id),name:vm.name}:v) }))
    })
  },
  addVisitedVm(vm) { /* LIFO 算法 */ },
  removeVisitedVm(id) { set(state => ({ visitedVms: state.visitedVms.filter(v => v.id !== id) })) },
}))

// views/vm/index.tsx
const isAdmin = useUserStore(s => s.role === ROLES.admin)
const isLightweight = useUserStore(s => !isAdmin && s.cloudType === CLOUD_TYPES.lightweight)
const groupBy = useLocalStorage<GroupType>('groupBy', 'status')
const listViewMode = useLocalStorage<ViewMode>('listViewMode', 'card')

const fetchData = async () => {
  const url = isAdmin ? '/api/vm/list' : '/api/self/vms'
  const res = await fetch(url)
  const list = res.data
  vmStore.setVmList(list)
}

// 缓存优先 + SSE 静默更新
useEffect(() => {
  if (vmStore.hasCachedData) {
    // 先渲染缓存，后台刷新
    fetchDataSilently()
  } else {
    fetchData()
  }
  
  // SSE 监听
  const es = new EventSource(`/api/vm/sse?token=${userStore.token}&include_resource_usage=1&include_ip=0`)
  es.addEventListener('vm_list', (e) => {
    const event = JSON.parse(e.data)
    updateTableData(event) // 局部更新表格行
  })
  
  return () => es.close()
}, [])
```

---

### 3.4 P1 - 虚拟机详情页

#### 3.4.1 需求概述
- **SSE 实时更新**：详情数据全部来自 SSE 推送，非轮询
- **懒加载策略**：首屏 Hero 区立即渲染，下方监控/信息片区 IntersectionObserver 触发加载
- **Tab 惰性加载**：快照/网络/定时/VNC/SPICE/监控 六个 Tab，默认激活 snapshot，其他切到才加载
- **角色差异化**：轻量云隐藏：编辑按钮/锁定/定时任务/重装/编辑备注/磁盘 IOPS 卡片

#### 3.4.2 后端接口映射
| 接口 | 方法 | 权限 | 返回字段 | 频率 |
|------|------|------|----------|------|
| `/vm/:id/sse` | GET | 通用 | `EventSource` 推送全量 VM 数据 | 持续推送 |
| `/vm/:id/operate` | POST | 通用 | `{ task_id }` | 电源操作 |
| `/vm/:id/pcie-info` | GET | 通用 | `total_ports, used_ports, free_ports` | 详情加载 |
| `/vm/:id/disks` | GET | 通用 | `disk_iops_limit_total/read/write` | 信息片区可见时才请求 |
| `/vm/:id/network/status` | GET | 通用 | `[{ mac, ip, bridge, switch, sec_group_id }]` | 信息片区可见时才请求 |
| `/vm/:id/spice/status` | GET | 通用 | `enabled, port, auth, exposed` | SPICE Tab 切换时才请求 |
| `/vm/:id/spice/enable` | POST | 通用 | `{ task_id }` | 开启 SPICE |
| `/vm/:id/spice/disable` | POST | 通用 | `{ task_id }` | 关闭 SPICE |
| `/vm/:id/spice/passwd` | PUT | 通用 | `{}` | 修改密码 |
| `/vm/:id/spice/expose` | PUT | 通用 | `{ task_id }` | 对外暴露开关 |
| `/vm/:id/spice/vv` | GET | 通用 | `Blob (.vv 连接文件)` | 下载 |

#### 3.4.3 前端组件分解
```
src/views/vm/
├── detail.tsx                          # 主入口：Hero + Tab 容器
├── components/
│   ├── HeroBanner.tsx                  # Hero 状态横幅（状态图标/名称/电源操作/锁定按钮）
│   ├── RealTimeStatsBar.tsx            # 实时资源概览条（CPU/内存/网络/磁盘 IO 单位切换）
│   ├── TabManager.tsx                  # 功能 Tab 管理器（snapshot/network/schedule/vnc/spice/monitor）
│   ├── ResourceChartsSection.tsx       # 监控图表区（lazy 加载，IntersectionObserver）
│   ├── InfoCardsSection.tsx            # 信息卡片区（basic/login/network/advanced/iops，lazy 加载）
│   └── BackToTopButton.tsx             # 返回顶部悬浮按钮（scroll > 400px 出现）
│
│   // Tab 内容组件
│   ├── tabs/
│   │   ├── SnapshotTab.tsx             # 快照列表（配额徽标 + CRUD）
│   │   ├── NetworkTab.tsx              # 网络管理（复用 NetworkList 组件）
│   │   ├── ScheduleTab.tsx             # 定时任务（新建/编辑/删除）
│   │   ├── VncConsoleTab.tsx           # VNC 控制台（内嵌 VncConsole 组件）
│   │   ├── SpiceConsoleTab.tsx         # SPICE 控制台（自制 SPICE 面板）
│   │   └── MonitorTab.tsx              # 开发者监视器（QEMU Monitor 命令执行）
│
│   // 子对话框
│   ├── dialogs/
│   │   ├── EditVmDialog.tsx            # 编辑虚拟机（复用 EditVmForm 组件）
│   │   ├── ResetPasswordDialog.tsx     # 重置密码（Windows/Linux 区别文案）
│   │   ├── ReinstallDialog.tsx         # 重装系统
│   │   └── VmRemarkDialog.tsx          # 编辑备注
│
│   // 辅助组件
│   ├── PowerOperationButtons.tsx       # 电源操作按钮组（带 popconfirm）
│   ├── LockButtons.tsx                 # 锁定/解锁按钮（锁定态变色 + 二次确认）
│   ├── PCIEHotplugInfo.tsx             # PCIe 热插槽用量（紧张红色警告）
│   └── DiskIOUnitSwitcher.tsx          # 磁盘 IO 单位切换（IOPS↔吞吐量）
```

#### 3.4.4 SSE 详情通道实现

```typescript
// hooks/useVmDetailSSE.ts (新增)
export function useVmDetailSSE(vmName: string, token: string) {
  const [vmData, setVmData] = useState<VMDetail | null>(null)
  const [sseStatus, setSseStatus] = useState<'connecting'|'connected'|'disconnected'>('connecting')
  let eventSource: EventSource | null = null

  useEffect(() => {
    if (!vmName || !token) return

    const url = `/api/vm/${vmName}/sse?token=${encodeURIComponent(token)}`
    eventSource = new EventSource(url)

    eventSource.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data) as VMDetail
        setVmData(data)
        
        // 如果 video_model 变 none 且当前在 vnc/spice tab，自动切回 snapshot
        if (video_model === 'none' && activeTab === 'vnc') setActiveTab('snapshot')
      } catch (err) {
        console.error('解析 SSE 事件失败', err)
      }
    }

    eventSource.onerror = () => {
      setSseStatus('disconnected')
      eventSource?.close()
      setTimeout(() => {
        setSseStatus('connecting')
      }, 5000)
    }

    return () => {
      eventSource?.close()
    }
  }, [vmName, token])

  return { vmData, sseStatus }
}
```

---

### 3.5 P2 - VNC 控制台

#### 3.5.1 需求概述
- **noVNC 连接**：WebSocket 方式连接 QEMU VNC 后端
- **快捷键发送**：Ctrl+Alt+Del / Ctrl+Shift+Esc / Alt+Tab 等常用组合键
- **文本输入**：逐字符模拟输入（支持大写/符号 SHIFT 修饰）
- **粘贴密码**：已保存 guestPassword → 自动输入到焦点位置
- **全屏/新窗口**：支持全屏模式和移动端新窗口打开

#### 3.5.2 后端接口映射
| 接口 | 方法 | 权限 | 返回字段 | 备注 |
|------|------|------|----------|------|
| `/vm/:id/vnc/status` | GET | 通用 | `enabled, port, auth, has_password, exposed` | 获取 VNC 状态 |
| `/vm/:id/vnc/enable` | POST | 通用 | `{ task_id }` | 开启 VNC（传密码） |
| `/vm/:id/vnc/disable` | POST | 通用 | `{ task_id }` | 关闭 VNC |
| `/vm/:id/vnc/passwd` | PUT | 通用 | `{}` | 修改密码（即时生效） |
| `/vm/:id/vnc/expose` | PUT | 通用 | `{ task_id }` | 对外暴露开关 |

#### 3.5.3 前端组件分解
```
src/components/
├── VncConsole.tsx                    # VNC 控制台主组件（toolbar + canvas）
├── VncToolbar.tsx                    # VNC 工具栏（连接/断开/快捷键/全屏/管理下拉）
├── VncEnableDialog.tsx               # 开启 VNC 对话框（密码输入框 + 警告 Alert）
├── VncPasswordDialog.tsx             # 修改密码对话框
├── VncSendTextDialog.tsx             # 发送文本对话框
└── utils/
    ├── vncShortcuts.ts               # 内置快捷键定义（PRIMARY_VNC_SHORTCUT/COMMON_VNC_SHORTCUTS）
    └── sendTextToVnc.ts              # 文本逐字符输入逻辑
```

#### 3.5.4 noVNC 连接核心代码

```typescript
// components/VncConsole.tsx
import RFB from '@novnc/novnc'
import { buildVncWsUrl, sendVncShortcut, sendTextToVnc } from '@/utils/vnc'

const connect = async () => {
  connecting.value = true
  errorMsg.value = ''
  try {
    const url = buildVncWsUrl(vmName, token)
    
    // 清除旧的画布
    const existingCanvas = vncContainer.value.querySelector('canvas')
    if (existingCanvas) vncContainer.value.removeChild(existingCanvas)

    rfb = new RFB(vncContainer.value, url)
    rfb.viewOnly = false
    rfb.scaleViewport = true
    rfb.resizeSession = false

    rfb.addEventListener('connect', () => {
      connected.value = true
      connecting.value = false
      nextTick(() => {
        setTimeout(() => refreshVncViewport(rfb), 200)
      })
    })

    rfb.addEventListener('disconnect', (e) => {
      connected.value = false
      connecting.value = false
      if (!e.detail.clean) errorMsg.value = '连接已断开（异常）'
      rfb = null
    })

    rfb.addEventListener('credentialsrequired', () => {
      ElMessageBox.prompt('请输入 VNC 密码', 'VNC 认证').then(({ value }) => {
        if (rfb) rfb.sendCredentials({ password: value })
      })
    })

  } catch (err) {
    errorMsg.value = `连接失败：${err.message}`
    connecting.value = false
  }
}
```

#### 3.5.5 快捷键发送逻辑

```typescript
// utils/vncShortcuts.ts
export const PRIMARY_VNC_SHORTCUT = {
  id: 'ctrlAltDel',
  label: 'Ctrl+Alt+Del',
  sequence: [
    { keysym: 0xffe3, code: 'ControlLeft' },  // Control_L
    { keysym: 0xffe9, code: 'AltLeft' },       // Alt_L
    { keysym: 0xffff, code: 'Delete' }         // XK_Delete
  ]
}

export const COMMON_VNC_SHORTCUTS = [
  { id: 'ctrlShiftEsc', label: 'Ctrl+Shift+Esc', sequence: [...] },
  { id: 'altTab', label: 'Alt+Tab', sequence: [...] },
  // ... 更多快捷键
]
```

---

### 3.6 P2 - SPICE 控制台

#### 3.6.1 需求概述
- **外部客户端直连**：提供 .vv 连接文件供 virt-viewer/spicy 使用
- **状态标签**：未开启/仅本地/已对外暴露
- **对外暴露开关**：联动宿主防火墙端口（需二次确认）
- **密码管理**：开启时设置密码、修改密码、不支持无密码对外暴露

#### 3.6.2 前端组件分解
```
src/components/
├── SpiceConsole.tsx                    # SPICE 控制台主组件
├── SpiceStatusBadge.tsx                # 状态标签（未开启/仅本地/已对外暴露）
├── SpiceExposeConfirmDialog.tsx        # 对外暴露风险提示对话框（严重警告）
└── SpiceVvDownloadMenu.tsx             # .vv 下载菜单（一次性/可重复）
```

---

### 3.7 P2 - 任务中心

#### 3.7.1 需求概述
- **近期任务面板**：底部抽屉式面板（展开/收起/拖拽调整高度）
- **SSE 实时进度**：任务进度实时更新、终端状态标记
- **详情抽屉**：展示 params/result/download links
- **取消任务**：二次确认后 cancelTask

#### 3.7.2 前端组件分解
```
src/components/
├── RecentTaskPanel.tsx                 # 近期任务面板（header/body/resize-handle）
├── TaskDetailDrawer.tsx                # 任务详情抽屉（descriptions/json 展示）
└── TaskTypeTag.tsx                     # 任务类型标签（clone/batch/delete 等）
```

---

### 3.8 P3 - 资源图表

#### 3.8.1 需求概述
- **四种图表**：CPU 使用率/内存使用率/网络流量/磁盘 IO
- **两种模式**：实时监控（增量计算）/历史查询（daterange picker）
- **磁盘双单位**：IOPS/Throughput（KB/s→MB/s/GB/s 自动换算）
- **Resize 自适应**：ResizeObserver + el-collapse 动画重绘

#### 3.8.2 前端组件分解
```
src/components/
└── ResourceCharts.tsx                  # 统一资源图表组件（type=vm|host, externalStats 驱动或自行轮询）
```

---

### 3.9 P3 - 模板管理

#### 3.9.1 需求要点
- **模板族树形渲染**：族名 + 节点列表（层级色条/展开箭头/链状摘要）
- **三种删除模式**：级联删除/提升子节点/热删除（promote_children/promote_children_hot）
- **分片上传导入**：ChunkUploader 三段式（init/chunk/complete）
- **发布设置**：Linux/Windows 分类、默认创建配置、启动后命令

#### 3.9.2 前端组件分解
```
src/views/template/
├── index.tsx                           # 主入口：族卡片列表
├── TemplateFamilyCard.tsx              # 单家族卡片（族头 + 节点列表）
├── TemplateNodeRow.tsx                 # 单个模板节点行（树线/展开箭头/OS 标签）
├── ImportTemplateDialog.tsx            # 导入模板包对话框（文件上传/预览/确认）
├── PublishSettingsDialog.tsx           # 发布设置对话框（分类/默认配置）
└── DeleteTemplateChainDialog.tsx       # 删除模板链路对话框（三级模式）
```

---

### 3.10 P3 - 网络中心/VPC

#### 3.10.1 需求要点
- **角色差异**：admin 5 个 Tab（概览/交换机/安全组/端口转发/ACL）/ 普通用户 2 个 Tab（交换机/安全组）
- **桥接直通 vs NAT**：DHCP/NAT/安全组/端口转发的可用性差异
- **端口转发封禁机制**：live=false+banned=true → "封禁"tag + tooltip
- **白名单**：用户/VM 双白名单
- **抓包任务**：Admin 专属、BPF 表达式构建、pcap 下载/删除

#### 3.10.2 前端组件分解
```
src/views/network/
├── index.tsx                           # 主入口：角色分支渲染器
├── OverviewTab.tsx                     # 概览 Tab（OVS 健康度/网桥表/物理网卡表）
├── SwitchesTab.tsx                     # 交换机 Tab（配额摘要/交换机表）
├── SecurityGroupsTab.tsx               # 安全组 Tab
├── PortForwardsTab.tsx                 # 端口转发 Tab（探测/白名单/批量删除）
├── AclTab.tsx                          # ACL Tab（nftables 规则预览/应用）
└── SharedComponents/
    ├── BridgeModeAlert.tsx             # 桥接模式提示 Alert
    ├── PortForwardProbeResultTable.tsx # 探测结果表格
    └── NetworkCaptureForm.tsx          # 抓包表单（BPF 表达式构建器）
```

---

## 4. 代码规范与最佳实践

### 4.1 目录结构约定
```
web/src/
├── api/                              # API 层（每个模块一个文件，函数命名驼峰）
│   ├── client.ts                     # 通用 axios 实例（已有）
│   ├── auth.ts                       # 认证接口（已有）
│   ├── settings.ts                   # 系统设置接口（已有）
│   ├── vm.ts                         # [待创建] 虚拟机相关接口
│   └── ...
├── assets/                           # 静态资源
│   └── styles/
│       └── index.css                 # 全局样式（已有）
├── components/                       # 可复用组件（越细越好）
│   ├── business/                     # 业务组件（高耦合组件）
│   │   ├── HighRiskChallengeModal.tsx  # 已有
│   │   └── ...
│   └── shared/                       # 共享组件（低耦合，如 Button/Card）
│       └── ...
├── config/                           # 配置文件（已有 constants.ts/site.ts）
├── hooks/                            # 自定义 Hooks（useXxx）
│   ├── useTheme.ts                   # 已有
│   ├── useVmDetailSSE.ts             # [待创建]
│   └── ...
├── layout/                           # 布局组件（极简占位，待确认设计）
│   └── index.tsx                     # 已有
├── router/                           # 路由配置（已有）
│   └── index.tsx                     # 已有
├── stores/                           # Zustand Stores（已有 user/app/highRisk）
│   └── ...
├── types/                            # TypeScript 类型定义
│   ├── api.ts                        # 已有（ApiResponse/PageParams 等）
│   ├── vm.ts                         # [待创建] 虚拟机类型
│   └── ...
├── utils/                            # 工具函数（已有 clipboard/validate）
│   └── ...
└── views/                            # 页面组件（每个路由一个文件夹）
    ├── dashboard/
    │   └── index.tsx                 # 已有
    ├── login/
    │   ├── index.tsx                 # 已有（基础版）
    │   ├── LoginForm.tsx             # [待创建]
    │   └── ...
    ├── vm/
    │   ├── list.tsx                  # [待创建]
    │   ├── detail.tsx                # [待创建]
    │   └── ...
    └── ...
```

### 4.2 命名规范
- **组件文件名**：PascalCase（如 `VmForm.tsx`）
- **函数/变量名**：camelCase（如 `handleLogin`, `fetchVmList`）
- **常量名**：SCREAMING_SNAKE_CASE（如 `DEFAULT_PAGE_SIZE`）
- **类型名**：PascalCase（如 `VmListResponse`）
- **API 函数**：动词 + 名词（如 `listVm`, `createVm`, `deleteVm`）

### 4.3 类型安全要求
- ✅ 所有 props/state/context 必须定义 TypeScript 类型
- ✅ API 响应必须定义接口（避免 `any`）
- ✅ 禁止隐式 `any`，编译器严格模式开启

### 4.4 组件拆分原则
1. **单一职责**：每个组件只做一件事（如 `VmStatusTag` 只负责显示状态 tag）
2. **大小控制**：单个 `.tsx` 文件尽量不超过 300 行
3. **父子通信**：优先使用 props/events，复杂状态用 Zustand
4. **动态导入**：大型组件（VNC/SPICE）使用 `lazy()` 懒加载

### 4.5 测试要求
- **单元测试**：每个核心函数（如 validatePassword）至少覆盖正常/异常路径
- **集成测试**：每个页面组件需要覆盖主要交互流程（登录→首页→操作）
- **手动测试清单**：见各模块"测试用例清单"

---

## 5. 合并上游代码规范

### 5.1 拒绝前端修改
- ❌ 标准仓库（`https://github.com/QVMConsole/QVMConsole`）任何 `web/` 目录修改必须拒绝
- ✅ 仅合并 `server/` 目录的后端修改

### 5.2 合并策略
```bash
# 策略 1：Merge + checkout（适合大版本）
git pull upstream main
git checkout --theirs server/  # 取上游服务端代码
git commit -m "Merge upstream: server changes only"

# 策略 2：Cherry-Pick（适合小修复）
git cherry-pick <commit-hash-from-upstream> --grep="server" --ignore-all-space

# 策略 3：Subtree（适合长期并行）
git subtree pull --prefix=server upstream main --squash
```

### 5.3 冲突处理
- 遇到 `server/` 文件冲突 → 以标准为基准
- 遇到 `.gitignore`/`docs/`/`web-backup/` 冲突 → 保留本仓库版本
- 不确定时 → 通知维护者确认

---

## 6. 部署与发布

### 6.1 构建脚本
```bash
# web/目录执行
npm install
npm run build  # 输出 dist/

# 产物验证
ls dist/
# 应包含：index.html, assets/index-*.css, assets/react-vendor-*.js, assets/semi-vendor-*.js
```

### 6.2 环境变量配置
| 变量 | 默认值 | 说明 |
|------|--------|------|
| VITE_APP_BASE_API | `/api` | 后端 API 前缀（生产环境可改为 `http://api.example.com/api`） |

### 6.3 Nginx 配置示例
```nginx
server {
    listen 80;
    server_name qvmconsole.example.com;
    
    root /var/www/qvmconsole/dist;
    index index.html;
    
    location / {
        try_files $uri $uri/ /index.html;  # SPA 路由重写
    }
    
    location /api {
        proxy_pass http://localhost:8080;  # 反向代理到后端
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";  # WebSocket 支持
    }
}
```

---

## 7. 常见问题解答（FAQ）

### Q1: 如何处理大量 API 接口？
**A**: 建议使用脚本自动生成 API 类型定义：
```bash
# 整理所有后端 handler 文件中的 route 和 struct，导出为 OpenAPI JSON
# 或使用 @hey-api/openapi-ts 生成 TypeScript SDK
```

### Q2: Semi Design 没有某组件怎么办？
**A**: 
1. 检查 Semi 是否可通过组合实现（如 Dialog+Table）
2. 参考 Element Plus 旧实现，用 Semi API 重新封装
3. 必要时自建基础组件（放在 `components/shared/`）

### Q3: Vue 时代的 Pinia Store 如何迁移？
**A**: 
```javascript
// Vue 3 Pinia
export const useUserStore = defineStore('user', {
  state: () => ({ token: '' }),
  actions: { setToken(t) { this.token = t } }
})

// React Zustand
export const useUserStore = create((set) => ({
  token: '',
  setToken: (t) => set({ token: t })
}))
```

### Q4: SSE 断线重连逻辑如何保证不泄漏？
**A**: `useEffect` cleanup 中调用 `eventSource.close()`，并用 ref 引用防止闭包问题

### Q5: 多人协作如何避免冲突？
**A**: 
- 各负责人认领 P0/P1/P2/P3 模块，独占相应目录
- 每日站会同步进度/问题
- PR 必须关联 Issue/Ticket 号
- 代码审查：至少 1 人 Review 通过

---

## 8. 附录

### 8.1 参考资料
- [React 19 官方文档](https://react.dev/)
- [Semi Design 中文文档](https://semi.design/zh-CN/docs/start-with-semi)
- [Zustand 官方仓库](https://github.com/pmndrs/zustand)
- [@novnc/novnc 官方文档](https://github.com/novnc/noVNC)
- [旧前端代码库（web-backup）](../../web-backup/)

### 8.2 术语表
| 术语 | 英文 | 解释 |
|------|------|------|
| 轻量云 | Lightweight Cloud | 轻量级云服务，限制多（无编辑/锁定等） |
| 弹性云 | Elastic Cloud | 标准云服务，功能完整 |
| 链式克隆 | Linked Clone | 基于 backing chain 的快速克隆 |
| 热删除 | Hot Remove | 在线删除不回滚虚拟机 |
| SSH 密钥认证 | SSH Key Authentication | 免密登录虚拟机的 SSH 密钥对 |
| SPICE | Simple Protocol for Independent Computing Environments | QEMU 显示协议 |
| VNC | Virtual Network Computing | 远程桌面协议 |
| HIBP | Have I Been Pwned | 密码泄露数据库 API |
| TOTP | Time-based One-Time Password | 时间基一次性密码（Google Authenticator 等） |
| NFTables | Netfilter Tables | Linux 下一代防火墙规则引擎 |

---

## 9. 更新日志

| 日期 | 版本 | 变更说明 |
|------|------|----------|
| 2026-07-26 | v1.0 | 初始版本发布，覆盖 P0-P5 所有模块迁移细则 |

---

**📧 联系方式**：如有问题请通过项目 Issue 或 QQ 群 654641487 联系核心维护团队。
