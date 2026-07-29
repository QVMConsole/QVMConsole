# 存储池页（新前端）

> 对应路由：`/storage-pool`（仅管理员）
> 源码目录：`web/src/views/storage-pool/`
> 旧版对照：`web-backup/src/views/storage-pool/index.vue`
> 设计稿：`plan/storage-pool-redesign.html`

## 功能总览

| 功能 | 说明 |
|------|------|
| 角色限制 | 仅管理员可访问（页面内非管理员显示无权限提示，与公网 IP 页一致；后端接口由 AdminMiddleware 保护） |
| 页头操作 | 创建存储卷 / 刷新 |
| 顶部提示 | 格式化挂载、分区与存储卷操作触发高风险验证并走任务队列 |
| 概览统计 | 统一卡片：总容量（物理硬盘数）、已用空间（已挂载分区数）、可用空间、存储卷数量（VG 计数及名称）+ 底部全局空间分配进度条（已用按总使用率变色 / 可用 / 未挂载 / 未分配四段）；统计口径为树形结构叶子节点 + 物理盘总量 |
| 状态筛选 Tab | 全部 / 待初始化 / 使用中 / 存储卷，带计数徽章；待初始化计数用 warn 橙色高亮 |
| 磁盘图标 | 按类型区分：HDD（机械盘）/ SSD（固态）/ NVMe（M.2）/ VG（卷组）/ USB（可移除） |
| 徽章系统 | 系统盘 / 默认存储 / 已启用 / 待初始化 / 存在数据 / 只读 / 可移除 / LVM 卷组 / PV |
| 空间分配条 | 分段显示 sys（系统占用）/ used（存储池）/ held（旧数据）/ free（未分配），含图例；used 按使用率变色（≥70 橙 / ≥90 红） |
| 初始化引导区 | 待初始化盘显示引导操作：全新盘主操作「格式化挂载」+ 次「创建分区」；历史数据盘主操作「创建分区」+ 次「格式化挂载」 |
| 磁盘操作 | 普通盘：高频「配置」图标外露，⋯ 下拉收折「设为默认 / 格式化挂载 / 创建分区 / 清除磁盘（危险）」；VG 卡：外露「删除存储卷」按钮（danger-ghost sm） |
| 分区行 | 树形扁平化缩进渲染；徽章（系统分区/默认/已启用/存在数据/只读）；自绘细条 capbar（按使用率变色）；操作同磁盘（配置外露 + ⋯ 下拉）。有分区或 LVM 卷信息时，分区标题行始终保留，可随时展开或折叠内容。 |
| VM 占用展示 | 分区/整盘下方显示虚拟机磁盘占用（共享组件 `VMUsageSection.tsx`，**默认折叠**为一行汇总：台数 + 实际占用/虚拟总配，点击展开明细）；展开后显示总占用进度条（按实际占用率变色 ≥70 橙 / ≥90 红）+ 每台 VM 名称和实际/虚拟大小（同一 VM 多块盘聚合为一行）；整盘挂载（无子分区）磁盘的 VM 占用数据在磁盘节点本身，常驻显示于磁盘卡 SpaceBar 下方（此类盘默认折叠且无展开入口，故不放入折叠区）；未配置为存储池但存在 VM 磁盘的已挂载分区同样显示 |
| 历史数据警告 | 磁盘存在 `has_existing_data` 时卡片顶部展示警告 Banner |

只读判定同时覆盖块设备只读标记和实际挂载目录的 `ro` 挂载选项；此外，独立挂载至 `/boot`、`/boot/efi` 的启动分区也不会出现在虚拟机硬盘的存储位置下拉选项中。

## 目录结构

```
web/src/views/storage-pool/
├── index.tsx                        # 主入口：数据加载/Tab 筛选/弹窗分发
├── storage-pool.css                 # 页面样式（深空极光，浅色优先 + 深色适配）
├── utils.ts                         # 类型文案/使用率配色/磁盘分类/概览统计/VG 统计
├── components/
│   ├── OverviewCards.tsx            # 概览统一卡（4 项统计 + 全局空间分配进度条）
│   ├── DiskCard.tsx                 # 磁盘/VG 卡片（空间分配条 + VM 虚拟总量进度条 + 引导区 + 徽章 + 图标 + 整盘挂载磁盘级 VM 占用）
│   ├── PartitionRow.tsx             # 分区/PV/LV 行（徽章 + 自绘 capbar + VM 占用展示 + 行内操作）
│   └── VMUsageSection.tsx           # 共享 VM 占用展示区（默认折叠，点击展开进度条 + VM 明细）
└── dialogs/
    ├── ConfigDialog.tsx             # 配置存储池（显示名称/启用）
    ├── FormatDialog.tsx             # 格式化并挂载（高风险，任务队列）
    ├── CreatePartitionDialog.tsx    # 创建分区（高风险，任务队列）
    ├── ClearDiskDialog.tsx          # 清除磁盘/删除所有分区（高风险，任务队列）
    ├── CreateVolumeDialog.tsx       # 创建 LVM 存储卷两步向导（高风险，任务队列）
    └── DeleteVolumeDialog.tsx       # 删除 LVM 存储卷（高风险，任务队列）
```

相关共享模块：

- `web/src/api/storagePool.ts`：存储池管理接口与完整树形类型 `HostStoragePoolInfo`
- `web/src/api/storage.ts`：原简化版 `StoragePoolInfo`/`getStoragePoolList` 改为从 `storagePool.ts` re-export
- `web/src/features/vm-form/sections/TextSwitch.tsx`：带内部单字符状态文字的共享开关（本页已移除）
- `web/src/views/dashboard/components/AdminBottom.tsx`：存储池「管理 →」跳转 `/storage-pool`

## 后端实现（VM 磁盘占用统计）

实现文件：`server/service/storage/pool/vm_usage_inject.go`

- `getAllVMDiskUsage()`：遍历所有虚拟机（`virsh list --all --name` + `virsh dumpxml`），复用 `libvirt_rpc.ParseDisksFromDomainXML` 解析**所有磁盘**（自动跳过 backingStore 内部 source；通过 `DiskBlockInfo.Device` 字段过滤 cdrom/软盘，ISO 不计入占用），再用 `qemu-img info --output=json` 获取每块盘的虚拟配置大小和实际占用大小
- `injectVMUsageIntoPools()`：将 VM 磁盘使用统计注入存储池树，为每个分区节点填充 `VmUsageList`、`VmTotalVirtual`、`VmTotalActual` 字段。匹配策略为**磁盘路径最长前缀匹配**可承载虚拟机的挂载点（不依赖 vm-disks 目录约定）；同一 VM 在同一挂载点下的多块盘按名称聚合为一条记录
- 注意：`vm` 包依赖本包，无法复用 `vm.GetVMDiskInfo`（会 import cycle），故复用底层 `libvirt_rpc` 解析器

关联改动：`server/service/libvirt_rpc/domain.go` 的 `DiskBlockInfo` 新增 `Device` 字段（解析 `<disk device='xxx'>` 属性），向后兼容，不影响既有调用方（vm/detail.go、storage/disk 等）。

已修复的历史 bug：旧实现通过 `walkStoragePools` 回调（按值传递）收集节点指针，实际指向副本，注入结果从未写回树中，导致接口返回的 `vm_usage_list` 恒为空。现改为递归索引遍历写回实际节点。

涉及结构体扩展（`server/service/storage/pool/types.go`）：

```go
type HostStoragePoolInfo struct {
    // ... 原有字段 ...
    VmUsageList     []VMDiskUsageInfo `json:"vm_usage_list,omitempty"`
    VmTotalVirtual  int64             `json:"vm_total_virtual,omitempty"`
    VmTotalActual   int64             `json:"vm_total_actual,omitempty"`
}
```

## 涉及接口（均为管理员）

- `GET /storage-pool/list`：宿主机块设备树形列表
- `PUT /storage-pool/:id/config`：更新显示名称与启用状态
- `POST /storage-pool/:id/default`：设为默认虚拟机存储位置
- `POST /storage-pool/:id/format-mount`：格式化并挂载（高风险 428 二次验证，任务队列）
- `POST /storage-pool/:id/create-partition`：创建分区（高风险，任务队列）
- `POST /storage-pool/:id/delete-partitions`：清除磁盘所有分区（高风险，任务队列）
- `GET /storage-pool/pv-targets`：创建存储卷弹窗的可用 PV 磁盘列表
- `POST /storage-pool/create-volume`：创建 LVM 存储卷（高风险，任务队列）
- `POST /storage-pool/delete-volume`：删除 LVM 存储卷（高风险，任务队列）

## 与旧版差异（设计稿 v1）

1. **状态筛选 Tab**：取代「仅显示可用磁盘」开关，按「全部 / 待初始化 / 使用中 / 存储卷」分组，带计数徽章
2. **磁盘图标类型**：按 HDD/SSD/NVMe/VG/USB 显示不同图标（颜色与形状区分）
3. **徽章系统**：统一使用 `.sp-badge`，支持系统盘/默认存储/已启用/待初始化/存在数据/只读/可移除/LVM 卷组/PV
4. **空间分配条**：每块盘顶部分段显示空间占比（sys 斜纹 / used 渐变 / held 斜纹 / free 透明），下方带图例
5. **初始化引导区**：待初始化盘直接给出「格式化挂载 / 创建分区」操作按钮，无需下拉
6. **删除存储卷按钮**：VG 卡外露为 `sp-btn danger-ghost sm` 按钮（带文字）
7. **分区行 capbar**：自绘细条进度条（6px 高），按使用率变色；存储池显示 `78% 269.3 / 345.3 GB` 格式，系统分区/未挂载显示容量 + 状态
8. **概览统一卡**：4 项统计与全局空间分配进度条合并为一张卡片，页面宽度占满容器
9. **深色模式适配**：大面积文字降对比为 `#b8c1cf`，仅少量强调文字保留高亮
10. **响应式适配**：小屏下卡片头部换行、操作区右对齐、分区行换行
