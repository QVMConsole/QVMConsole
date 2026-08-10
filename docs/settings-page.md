# 系统设置页（新前端 React + Semi Design）

> 迁移自旧前端 `web-backup/src/views/settings/index.vue`（2300+ 行单文件），按模块拆分为多个组件。
> 页面路由：`/settings`（仅管理员），支持 `?tab=xxx` 直接定位标签页。

## 功能总览

共 8 个标签页，与旧前端功能一一对应：

| Tab 键位 | 名称 | 内容 |
|----------|------|------|
| `basic` | 基础设置 | 网站标题、端口自动分配范围、访问链接、服务端口（只读） |
| `network` | 存储与网络 | 模板/克隆/ISO/OVF-OVA 临时目录等存储路径（含"替换为我的存储"）、OVS 网络设置、全局带宽限制、端口安全速率阈值、默认磁盘 IOPS |
| `host` | 宿主机设置 | KSM 内存去重挡位、zRAM 压缩内存挡位、KVM Unrestricted Guest、硬件直通环境诊断（IOMMU 一键开启 / vfio-pci 一键加载）、网络等待就绪检测 |
| `advanced` | 调度与高级 | 动态内存调度参数（NumField 网格）、SPICE 默认开启、批量克隆并发、救援系统 ISO、CPU 亲和性预设 |
| `security` | 安全与维护 | 开发环境开关、SMTP 配置与测试发信、会话指纹/请求过滤/泄露密码检测、JWT 密钥轮换、维护模式 |
| `log` | 日志管理 | 日志最大备份数、磁盘占用统计、日志文件多选删除 / 导出 ZIP |
| `diagnostics` | 诊断导出 | 按类别收集诊断信息并导出 ZIP |
| `storage` | 存储管理 | 用户存储镜像信息、存储回收（fstrim + fallocate --dig-holes） |

## 文件结构

```
web/src/views/settings/
├── index.tsx                     # 主入口：权限门禁 / Tabs / 表单状态 / 保存与重置
├── types.ts                      # SettingsForm 类型、默认值、校验、提交负载构造
├── hostUtils.ts                  # 宿主机 Tab 辅助：兜底挡位、格式化、状态摘要
├── logUtils.ts                   # 日志类型标签配色
├── helpContents.ts               # KSM / zRAM / KVM 帮助弹窗文案
├── settings.css                  # 页面样式（--qvm- 设计令牌，深色适配）
├── components/
│   ├── SettingRow.tsx            # 通用表单行（label + 控件 + 提示）与分区标题
│   ├── NumField.tsx              # 数字参数字段卡片（配合 .stg-field-grid 多列网格）
│   ├── BasicTab.tsx              # 基础设置
│   ├── StorageNetworkTab.tsx     # 存储与网络
│   ├── HostTab.tsx               # 宿主机设置（KSM/zRAM/KVM 即时保存）
│   ├── HostProfilePanel.tsx      # KSM/zRAM 共用挡位面板（单选按钮 + 挡位说明卡片）
│   ├── PassthroughSection.tsx    # 硬件直通配置区
│   ├── AdvancedTab.tsx           # 调度与高级
│   ├── SecurityTab.tsx           # 安全与维护
│   ├── LogTab.tsx                # 日志管理
│   ├── DiagnosticsTab.tsx        # 诊断导出
│   └── StorageMaintainTab.tsx    # 存储管理
└── dialogs/
    └── LogExportDialog.tsx       # 日志导出选择对话框
```

相关公共代码：

- `web/src/api/settings.ts`：设置模块全部接口与类型（updateSettings / testSMTP / rotateJWTSecret / KSM / zRAM / KVM / 硬件直通 / 日志 / 诊断 / 存储回收等）
- `web/src/utils/download.ts`：`downloadBlob` / `timestampFilename`（blob 下载工具）
- `web/src/utils/format.ts`：新增 `formatFileSize`（B/KB/MB/GB/TB）

## 交互与保存机制

- **整体保存**：底部"保存设置"按钮提交 `buildSettingsPayload(form)`，校验逻辑集中在 `validateSettingsForm`（与旧前端一致的边界值）。诊断导出 / 存储管理 Tab 为独立操作区，不显示保存按钮。
- **即时保存项**（不随整体表单提交，操作前有二次确认弹窗）：
  - KSM / zRAM 挡位切换（`PUT /host/ksm`、`PUT /host/zram`），取消或失败会回滚选中态并重新拉取状态
  - KVM Unrestricted Guest 开关（`PUT /host/kvm-intel-unrestricted-guest`）
  - CPU 亲和性预设（`PUT /settings/cpu-affinity-presets`，独立"保存预设"按钮）
  - IOMMU 一键开启 / vfio-pci 一键加载

宿主机设置不再显示无实际作用的“启用硬件直通”开关，也不列出可直通设备；PCI 设备在虚拟机创建或编辑时的“硬件直通”分区中选择。
- **高风险二次验证**：维护模式切换保存、JWT 密钥手动轮换和立即执行密码泄露扫描会触发后端 428，由请求层（`api/client.ts`）自动弹出验证弹窗后重试。
- **定时泄露检测**：独立 `TextSwitch` 控制每天本地时间 `00:00` 的扫描，默认开启；旁边“立即执行”按钮不受实时检测或定时检测开关限制。运行期间按钮显示旋转图标并禁用，状态区展示管理员与普通用户泄露数量。
- **测试发信**：先静默保存当前配置再调用 `POST /settings/smtp/test`（按钮在 SMTP 表单区内，不在页脚）。
- **站点标题同步**：保存成功后调用 `useAppStore.setSiteTitle`，同时用 `setPublicFlags` 同步泄露密码检测 / SPICE 默认开关的公开标志。
- **?tab= 定位**：`VALID_SETTINGS_TABS` 白名单校验，切换 Tab 时 `replace` 方式回写 URL，供其他页面（如虚拟机表单空状态）跳转到指定标签页。
- **多数字参数布局**：动态内存调度 / 全局带宽 / 默认 IOPS 等多数字字段统一使用 `NumField` 卡片 + `.stg-field-grid` 自适应网格，环境变量说明汇总到网格下方的单行提示。
- **端口安全参数**：安装与升级后总开关默认关闭；关闭时隐藏速率阈值并跳过提交校验。总开关由网络中心预检后切换，设置页仅在开启状态编辑总包、邻居协议、广播/组播和协调周期。

> 变更记录：端口转发 HTTP 探测功能已随后端移除（仅剩 config 白名单残留键），设置页不再提供相关配置项。

## 后端接口清单

均为旧前端已存在的接口，本次未修改后端：

| 接口 | 方法 | 说明 |
|------|------|------|
| `/settings` | GET / PUT | 获取 / 更新系统设置（含定时泄露检测开关） |
| `/security/password-breach/status` | GET | 获取泄露扫描状态与受影响账户 |
| `/security/password-breach/scan` | POST | 立即提交完整扫描任务（高风险） |
| `/settings/smtp/test` | POST | 测试发信 |
| `/settings/jwt-secret/rotate` | POST | 手动轮换 JWT 密钥（高风险） |
| `/settings/user-storage-iso-path` | GET | 当前用户存储 ISO 目录 |
| `/settings/cpu-affinity-presets` | PUT | 保存 CPU 亲和性预设 |
| `/cpu-affinity-presets` | GET | 获取 CPU 亲和性预设 |
| `/settings/log/status` | GET | 日志文件列表与占用 |
| `/settings/log/delete` | POST | 删除日志 |
| `/settings/log/export` | POST | 导出日志（blob ZIP） |
| `/settings/diagnostics/categories` | GET | 诊断类别 |
| `/settings/diagnostics/export` | POST | 导出诊断（blob ZIP，120s 超时） |
| `/settings/storage/trim` | POST | 用户存储回收 |
| `/host/ksm` | GET / PUT | KSM 状态 / 挡位 |
| `/host/zram` | GET / PUT | zRAM 状态 / 挡位 |
| `/host/kvm-intel-unrestricted-guest` | GET / PUT | KVM 兼容性参数 |
| `/host/hardware-passthrough/status` | GET | 硬件直通状态 |
| `/host/hardware-passthrough/enable-iommu` | POST | 一键开启 IOMMU |
| `/host/hardware-passthrough/load-vfio` | POST | 一键加载 vfio-pci |
| `/storage-pool/all-isos` | GET | 救援系统 ISO 候选列表 |

## 设计规范落实

- 所有开关使用共享组件 `TextSwitch`，并在 Switch 内嵌单字符状态文字
- 行内小按钮（预设行删除）为纯图标 + Tooltip；弹窗底部与表单级主按钮保留文字
- Banner 替代旧 el-alert，提示区不使用 emoji，统一 Semi 图标
- 深色模式下页头 h2 / 分区标题 / 挡位卡片标题降对比为 `#b8c1cf`
- 720px 以下表单行改纵向堆叠，数字字段网格退化为单列
