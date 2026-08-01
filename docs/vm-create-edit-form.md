# 创建 / 编辑虚拟机表单（共享表单模块）

> 适用范围：`web/`（React 19 + Semi Design）
> 核心目录：`web/src/features/vm-form/`
> 使用方：列表页「新建虚拟机」（全屏向导）、详情页「编辑」标签页

---

## 1. 设计目标

旧前端 `VmForm.vue`（约 7100 行）同时承载创建向导与编辑选项卡，字段与规则高度耦合。
新前端将其拆分为**共享表单模块 + 两个壳**：

- **共享模块（features/vm-form）**：统一表单模型、默认值、常量、校验规则、联动推荐、提交载荷构建、分区 Section 组件、配置弹窗。**创建与编辑只此一份规则，一处改动两处生效。**
- **创建壳（CreateVmWizard）**：全屏 Modal + 左侧步骤导航 + 分区表单 + 确认信息页。
- **编辑壳（EditVmForm）**：详情页「编辑」标签页内嵌，选项卡布局，差异快照提交。

新增字段 / 调整规则时的改动点：

1. `types.ts` 表单模型加字段（含默认值 `defaults.ts`）
2. 对应 Section 加控件（创建/编辑自动同时生效）
3. 如涉及提交：`payload.ts` 对应链路补字段（ISO 创建、模板克隆、批量克隆、导入、编辑 5 条链路独立）

## 2. 目录结构

```
web/src/features/vm-form/
├── types.ts                  # VmFormModel 统一模型（创建+编辑字段全集）、上下文、快照类型
├── constants.ts              # 常量选项（总线/网卡/显示设备/引导/机型/OS/拓扑/帮助文案）
├── defaults.ts               # 默认值工厂、随机名称/主机名生成
├── templateUtils.ts          # 模板默认配置解析（vcpu/ram/disk/bus/nic/video/拓扑/重启模式/引导）
├── recommend.ts              # 联动推荐（RTC/显示设备/引导/动态内存推荐值/归一化）
├── validators.ts             # 校验规则（名称/主机名/用户名/密码/FnOS ID/磁盘/IP + 步骤校验 + 必填汇总）
├── payload.ts                # 提交载荷构建（创建三链路 + 批量 + 编辑差异快照捕获）
├── scope.tsx                 # VmFormProvider + useVmFormScope（向 Section 注入表单/选项/上下文）
├── vpcOptionUtils.ts         # VPC 交换机归属解析、安全组选项过滤与标签格式化
├── useVmForm.ts              # 核心 hook：表单状态 + 全部联动（OS/ISO/模板/架构/机型/引导/动态内存）
│                             #   + buildEditFormState（编辑回填纯函数）
├── useVmFormOptions.ts       # 选项数据加载（ISO/模板/存储池/VPC/磁盘文件/直通设备/宿主信息）
├── useVmEditDevices.ts       # 编辑设备管理（磁盘/光驱/软盘列表与纯数据操作，无 JSX）
├── CreateVmWizard.tsx        # 创建壳：全屏 Modal 步骤向导
├── EditVmForm.tsx            # 编辑壳：详情页选项卡表单
├── vm-form.css               # 全部样式（aurora 令牌，浅色优先 + 深色适配 + 响应式）
├── sections/
│   ├── SectionCard.tsx       # 分区卡片容器
│   ├── FormField.tsx         # 字段行（label + 控件 + 提示/错误 + 帮助 Tooltip）
│   ├── TextSwitch.tsx        # 带内部单字符状态文字的开关
│   ├── storageTargetUtils.ts # 存储位置选项标签工具
│   ├── CreateModeSection.tsx # 【创建】创建方式四卡（ISO/模板/已有磁盘/OVF-OVA）
│   ├── ApplianceImportSection.tsx # 【创建】OVF/OVA 来源、检查、元数据与包内磁盘摘要
│   ├── BasicInfoSection.tsx  # 【创建】名称/批量/备注/系统类型/系统版本/导入初始化
│   ├── TemplateSection.tsx   # 【创建】模板/克隆模式/初始化/凭据/FnOS/OpenWrt/登记摘要
│   ├── StoragePoolSection.tsx# 【创建】存储位置（含选项标签工具函数）
│   ├── IsoStorageSection.tsx # 【创建】ISO 多选/系统盘/软盘（额外磁盘复用 ExtraDiskSection）
│   ├── ImportStorageSection.tsx # 【创建】导入磁盘（来源/处理/IOPS/导入后操作/额外导入磁盘）
│   ├── ExtraDiskSection.tsx  # 【创建】额外数据盘（ISO/模板共用）
│   ├── ConfirmSection.tsx    # 【创建】最后一步：配置确认摘要
│   ├── CpuMemorySection.tsx  # 【共用】CPU/内存/热添加/CPU 限制(管理员)/动态内存
│   ├── VirtEngineSection.tsx # 【共用】虚拟化方案/架构/机型/引导类型
│   ├── NicSection.tsx        # 【共用】网卡类型（编辑运行中禁用）+ 网口列表（创建）
│   ├── BootOrderSection.tsx  # 【共用】引导顺序（创建类型排序 / 编辑设备列表）
│   ├── SystemBehaviorSection.tsx # 【共用】Watchdog/开机自启
│   ├── AdvancedSection.tsx   # 【共用】高级选项全集 + 首次进入提醒遮罩
│   ├── PassthroughSection.tsx# 【共用·管理员】硬件直通
│   ├── DiskManageSection.tsx # 【编辑】磁盘/光驱/软盘管理
│   └── NicManageSection.tsx  # 【编辑】主网口 VPC 绑定 + 多网口列表
└── dialogs/
    ├── RtcConfigDialog.tsx   # 【共用】RTC 配置
    ├── GuestAgentDialog.tsx  # 【共用】QEMU Guest Agent
    ├── MemoryDynamicDialog.tsx   # 【共用】动态内存详细配置
    ├── VirtioMemDetailDialog.tsx # 【共用】Windows 弹性内存说明
    ├── SmbiosDialog.tsx      # 【共用】SMBIOS 类型 1
    ├── DiskIopsDialog.tsx    # 【共用】磁盘 IOPS（受控通用组件）
    ├── PassthroughPickerDialog.tsx # 【共用·管理员】直通设备选择
    ├── VmXmlDialog.tsx       # 【编辑】持久化 XML 查看/保存
    ├── AttachDiskDialog.tsx  # 【编辑】挂载已有磁盘（管理员可绝对路径导入）
    ├── ResizeDiskDialog.tsx  # 【编辑】磁盘扩容（仅扩大）
    ├── RemoveDiskDialog.tsx  # 【编辑】删除磁盘（连文件删除/转移到存储）
    └── NicEditDialog.tsx     # 【编辑·管理员】添加/编辑网口（型号/交换机/安全组/速率）
```

## 3. 复用机制

### 作用域注入（scope.tsx）

每个 Section 组件通过 `useVmFormScope()` 获取三样东西，**不接收业务 props**（个别编辑专有数据除外）：

| 注入项 | 内容 |
|--------|------|
| `form` | `useVmForm` 返回值：表单模型 + setField/patch/replaceForm + 派生状态 + 联动 action |
| `options` | `useVmFormOptions` 返回值：ISO/模板/存储池/VPC/磁盘文件/直通设备/宿主信息 |
| `ctx` | `VmFormContext`：`mode`(create/edit)、`isAdmin`、`vmStatus`（编辑）、宿主架构/核数、SPICE 支持、登记上下文、编辑原始 CPU/内存 |

两个壳各自搭建 `VmFormProvider`，Section 内部按 `ctx.mode` / `ctx.isAdmin` / `ctx.vmStatus` 控制显隐与禁用，因此**同一 Section 在创建向导与编辑表单中表现自动分化**。

### 关键共享点

- **校验**：`validators.ts` 全部字段校验函数 + `validateCreateStep`（向导按步阻断）+ `collectMissingRequired`（提交按钮禁用与缺失清单）
- **联动**：`useVmForm` 集中处理 OS 类型 / ISO 选择 / 模板切换 / 虚拟化方案 / 架构 / 机型 / 引导切换的全部推荐逻辑（含 bootTypeTouched：用户手动改过引导后不再自动推荐）
- **动态内存**：`recommend.ts` 推荐值计算（balloon：启动=规格、最小=50%、最大=+30%；virtio_mem：基础=50%、最大=+30%）
- **模板默认值**：`templateUtils.ts` 从模板 `default_config` 带出 vcpu/ram/disk/bus/nic/video/拓扑/重启模式
- **提交载荷**：`payload.ts` 五条链路各自独立构建函数（`buildCreatePayload` / `buildClonePayload` / `buildBatchClonePayload` / `buildImportPayload` / `buildEditPayload`）

## 4. 创建向导（CreateVmWizard）

### 步骤流（普通模式）

| 步骤 | 内容 | 阻断校验 |
|------|------|----------|
| 创建方式 | ISO 镜像安装 / 模板快速克隆 / 导入已有磁盘 / 导入虚拟机 四卡 | - |
| 虚拟机包 | 仅导入虚拟机模式：使用大卡片选择“跟随 OVF 配置 / 自定义”；创建向导不读取包内容 | 必须选择源文件；跟随模式直接提交，自定义模式继续完整向导；包校验由异步任务执行 |
| 基础信息 | 名称、批量数量（模板）、备注、系统类型/版本（ISO）、导入初始化（导入）、模板与凭据（模板） | 名称格式、模板/磁盘/凭据按模式分支 |
| 硬件规格 | CPU/内存/热添加/CPU 限制/动态内存 + 虚拟化方案/架构/机型/引导 | vcpu、ram > 0 |
| 存储介质 | 存储位置 + ISO/系统盘（ISO）、导入磁盘（导入）、额外数据盘（模板） | ISO 磁盘大小、导入磁盘文件/路径 |
| 网络设置 | 默认网卡型号 + 网口列表（交换机/安全组/型号） | - |
| 系统配置 | 引导顺序、Watchdog、开机自启 | - |
| 高级选项 | 开发者选项全集（首入提醒遮罩，localStorage 记忆） | - |
| 硬件直通 | 仅管理员，PCI 设备选择 | - |
| 确认信息 | 全量配置摘要 + 预估资源占用 | 必填汇总未齐禁用提交并列出缺失项 |

### 提交规则

- ISO：管理员 `POST /vm/create`，用户 `POST /self/vm/create`
- 模板单台：管理员 `POST /vm/clone`，用户 `POST /self/vm/clone`
- 模板批量（batch_count > 1）：`POST /vm/batch-clone`（登记模式禁用批量；批量不允许直通设备；密码留空每台自动生成）
- 导入：用户存储 `POST /self/vm/import`；管理员绝对路径 `POST /vm/import-disk`
- 导入虚拟机：创建向导直接调用对应 `import-appliance` 接口并创建独立任务；只读 `inspect` 接口保留给 API 调试或独立预览
- 提交前：CPU 亲和性格式校验 + 密码 HIBP 泄露检测（`utils/validate.ts`）
- 轻量云登记模式（`registration` 上下文 + `onDraft` 回调）已预留，供用户管理页复用

### 联动规则（与旧版一致）

- Windows（ISO/OS 卡/ISO 自动识别）：UEFI 引导 + SATA 磁盘 + e1000e 网卡；i440FX + Windows 强制 BIOS
- ARM（aarch64）：强制 virt 机型 + UEFI + ramfb 显示；KVM 模式回宿主机架构
- 模板切换：带出默认 vcpu/ram/disk/bus/nic/video/拓扑/重启模式；Windows 模板固定 administrator；UEFI 模板自动升级安全引导（Windows）
- 非 Windows 目标禁用 virtio_mem 弹性内存（自动回退 balloon）
- 选择 ISO 自动补全系统类型/版本/最小磁盘，首个 ISO 为主安装盘，启动顺序自动 cdrom 优先

## 5. 编辑表单（EditVmForm · 详情页「编辑」）

### 选项卡

| 选项卡 | 内容 |
|--------|------|
| 基础配置 | 名称（只读）、状态、CPU/内存/热添加/CPU 限制/动态内存 |
| 磁盘与驱动器 | 现有磁盘表（扩容/删除/驱动/IOPS）、新建磁盘、挂载已有磁盘、光驱、软盘 |
| 启动与安全 | 网卡类型（运行中禁用）、机型（只读）、引导方式（关机可改）、引导顺序（Cockpit 风格设备列表）、开机自启 |
| 网口管理 | 主网口 VPC 绑定切换（交换机/安全组，轻量云禁用）+ 多网口列表（添加/编辑/删除，仅管理员，运行态实时 IP，热插拔提示） |
| 硬件直通 | 仅管理员 |
| 高级设置 | 高级选项全集 + XML 编辑器入口 |

### VPC 交换机与安全组联动

- 创建虚拟机、编辑主网口绑定以及添加/编辑额外网口共用同一套安全组选项过滤规则。
- 普通 VPC 交换机只展示该交换机所属用户的通用安全组；系统基础网络改用虚拟机归属用户筛选安全组。
- 编辑模式通过 `GET /vm/:name/vpc` 返回的 `owner_username` 识别虚拟机归属，管理员编辑其他用户虚拟机时不会误用管理员自己的安全组。
- VM 专属安全组只会出现在对应虚拟机的编辑选项中；创建流程和其他虚拟机不会展示该安全组。
- 切换交换机后，如果原安全组不再属于目标用户或不适用于当前虚拟机，前端会自动清空选择；桥接直通交换机不使用安全组。
- 管理员的交换机选项会显示“用户名 / 交换机名”，用于区分不同用户的同名交换机。

### 差异快照提交

1. 加载时：`buildEditFormState` 同步构建表单 → `captureEditFormSnapshot`（表单字段）+ `captureEditDiskIopsSnapshot`（磁盘 IOPS）
2. 保存时：`buildEditPayload` 逐字段对比快照，**仅发送变化字段**（含 boot_order/device_order 由设备列表归并、磁盘 IOPS 按设备 diff、CPU 限制/亲和性仅管理员、运行态下 CPU 拓扑/显示设备跳过）
3. SPICE 联动：开关变化随保存自动 `enable/disable`（失败仅提示不阻断）
4. 保存成功后重新加载详情并重捕快照

### 运行态约束

- CPU：未启用热添加时禁用修改；启用后仅允许增加（下限为原始值）
- 内存：下限为原始值（禁止缩小）
- 引导方式 / 网卡类型 / 显示设备 / SPICE / CPU 拓扑：运行与暂停态禁用
- 直通设备：提示需关机

## 6. 涉及接口

- 创建：`POST /vm/create`、`POST /self/vm/create`、`POST /vm/clone`、`POST /self/vm/clone`、`POST /vm/batch-clone`、`POST /self/vm/import`、`POST /vm/import-disk`
- 选项：`GET /vm/os-variants`、`GET /template/list`、`GET /storage-pool/vm-targets`、`GET /storage-pool/all-isos`、`GET /self/storage/isos`、`GET /self/storage/files/:category`、`GET /vpc/switches`、`GET /vpc/security-groups`、`GET /host/cpus`、`GET /system-info`、`GET /settings`、`GET /cpu-affinity-presets`、`GET /public/settings`
- 编辑：`PUT /vm/:name`、`GET /vm/:name`、`GET /vm/:name/disks`、`GET /vm/:name/xml`、`PUT /vm/:name/xml`、`GET /vm/:name/passthrough`、`GET /host/passthrough`、`POST /host/passthrough/bind`
- 磁盘/光驱/软盘：`POST /vm/:name/disk/:dev/resize`、`DELETE /vm/:name/disk/:dev`、`PUT /vm/:name/disk/:dev/bus`、`POST /vm/:name/disk/attach`、`POST /vm/:name/disk/import`、`POST /vm/:name/cdrom(/eject)`、`DELETE /vm/:name/cdrom`、`POST /vm/:name/floppy(/eject)`、`DELETE /vm/:name/floppy`
- SPICE：`GET /vm/:name/spice/status`、`POST /vm/:name/spice/enable|disable`
- 网口：`GET /vm/:name/vpc`、`PUT /vm/:name/vpc`、`PUT /vm/:name/security-group`、`GET|POST /vm/:name/interfaces`、`PUT|DELETE /vm/:name/interfaces/:order`、`GET /vm/:name/network/status`

## 7. 本轮裁剪说明

- 智能推荐（应用场景 + 一键应用推荐配置）未迁移
- 右侧常驻配置预览面板未迁移，改为向导最后一步「确认信息」整页摘要
- 轻量云登记模式（registrationMode）仅预留接口（`registration` / `onDraft`），待用户管理页迁移时启用
- 高级选项提醒遮罩的 localStorage key 简化为按站点记忆（`vm-advanced-settings-intro-seen`）

## 8. 存储位置过滤

`GET /storage-pool/vm-targets` 会统一过滤块设备只读、实际挂载目录以 `ro` 方式挂载，以及独立挂载到 `/boot` 或 `/boot/efi` 的存储位置。创建向导的「虚拟机硬盘」及复用该选项列表的磁盘目标下拉框均不会展示这类目录，避免创建时写入失败或误选启动分区。
