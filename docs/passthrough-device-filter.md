# 硬件直通设备准入过滤

## 背景

虚拟机「硬件直通」选择列表原先依赖 `lspci` 输出的英文类别名做黑名单过滤（`host bridge` / `pci bridge` / `smbus` / `raid` 等），存在两个问题：

1. **漏网设备**：Xeon E5/E7 v2 平台的 CPU uncore 设备（UBOX Registers、QPI Link、Home Agent、Power Control Unit 等）类别是 `System peripheral`（类别码 `0880`），不在黑名单中，会直接出现在列表里且显示为「无驱动 / 空闲」，用户勾选后就可能把 CPU 内部寄存器绑定到 `vfio-pci`，导致宿主机崩溃。同类的还有性能计数器（`1101`，`ivbep_uncore` 驱动）、管理引擎 MEI（`0780`）、BMC 管理显卡（`mgag200` / `ast`）。
2. **判定脆弱**：类别名受发行版与本地化影响，且 `0680`（Bridge）等名称与黑名单不匹配；`classCodeToName` 对未登记类别返回 `PCI 设备(xxxxxx)` 后会被丢弃、回退成英文名，等于判定完全依赖英文串。

现在过滤统一改为 **基于 sysfs 的 PCI class code（主类 + 子类）** 判定，并叠加宿主机实际使用情况与 IOMMU 组隔离判定。策略集中在 `server/service/pci/policy.go`，硬件直通列表（`server/service/vm/passthrough.go`）与核显直通页面（`server/service/host/igpu_passthrough.go`）共用同一份判定，避免两处结论不一致。

## 准入规则

判定顺序（任一命中即不可直通）：

| 顺序 | 规则 | 说明 |
| --- | --- | --- |
| 1 | 无法识别类别且无驱动 | 无效/残留设备 |
| 2 | 虚拟化平台设备 | 厂商 `1af4`/`1b36`/`1234`/`15ad`/`80ee`、`virtio-pci` 等 |
| 3 | 关键驱动占用 | `megaraid_sas`/`aacraid`/`lpc_ich`/`mei_me`/`vmwgfx`/`nvidia`/`ehci-pci`/`ohci-pci`/`pcieport`、含 `uncore` 的驱动、虚拟机与 BMC 显示驱动 |
| 4 | 主类硬排除 | `05` 内存控制器、`06` 桥设备、`08` 系统外设、`0b` 处理器、`11` 信号处理（性能计数器）、`13` Non-Essential Instrumentation |
| 5 | 子类硬排除 | `0101` IDE、`0104` 硬件 RAID、`0780` MEI/管理引擎、`0c05`/`0c08` SMBus、`0c07` IPMI 接口 |
| 6 | 承载宿主机存储 | 该 PCI 设备（含 LVM/软 RAID/多路径堆叠）上挂载了宿主机文件系统或活动交换分区 |
| 7 | 键鼠所在 USB 控制器 | 该 USB 3.0 控制器上挂了宿主机键盘或鼠标（含 BMC 虚拟键鼠） |
| 8 | IOMMU 组不可独立隔离 | 同一 IOMMU 组内除 Host/PCI 桥（`0600`/`0604` 等拓扑桥）外的成员存在不可直通设备 |

**雷电/USB4 例外**：雷电控制器在 PCI 类别上同样属于 `0880`，规则 4 允许其通过，通过内核 `thunderbolt` 驱动或产品名（`Thunderbolt`/`USB4`/`JHL`/`Titan Ridge` 等）识别，未硬编码设备 ID 表。

**桥片为何不阻断同组直通**：Host 桥与 PCIe 桥只承担拓扑转发，留在宿主机上不影响同组端点设备直通（常见于显卡与其上游根端口同组的情况）；而 ISA/EISA/MCA 桥（`0601`/`0602`/`0603`）连接的是宿主机实体器件，会阻断整组。

## 相对旧行为的放开与收紧

放开（原先被驱动黑名单一刀切挡掉）：

- NVMe 控制器（`0108`）：已挂载/根盘所在设备仍会被规则 6 拦下；
- SATA/AHCI、SAS、SCSI HBA（`0106`/`0107`/`0100`）：承载宿主机已挂载存储时不显示；
- USB 3.0 xHCI 控制器（`0c03` 接口 `30`）与 USB4 主机接口（`0c03` 接口 `40`）：挂有宿主机键鼠时不显示，USB 1.1/2.0（UHCI/OHCI/EHCI）仍不放开；
- 雷电控制器（`0880`，见上方例外说明）。

收紧（原先会出现在列表里的设备）：

- CPU uncore / UBOX / QPI / PCU / Home Agent / RTC / IOMMU 等 `08xx` 全类；
- 性能计数器 `1101`（`ivbep_uncore` 等）；
- 管理引擎 MEI `0780`；
- BMC 管理显卡（`mgag200`、`ast`、`mga`）与虚拟机显示设备（`vmwgfx`、`qxl`、`bochs-drm` 等）；
- `0680` 等英文名为 `Bridge` 的桥片（原先黑名单只匹配 `pci bridge`）。

## 二次复检

直通列表过滤只影响界面，为防止绕过列表直接调用接口绑定危险设备，以下三个入口都会再次执行同一套准入判定：

- `ValidatePCIPassthrough`（绑定、创建虚拟机、克隆虚拟机前置校验）
- `BindPCIDeviceToVfio`（绑定到 `vfio-pci`）
- `AttachPCIDeviceToVM`（挂载到虚拟机）

错误信息会说明具体原因，例如：

```
设备 0000:3f:0b.3（Intel Corporation Xeon E7 v2/Xeon E5 v2/Core i7 UBOX Registers）不允许直通：
系统外设（CPU uncore 寄存器、IOMMU/VT-d 等），直通会导致宿主机崩溃
```

## 显示类设备的额外保护

显示类设备（`03xx`）不参与列表过滤，改由绑定环节单独保护：`BindPCIDeviceToVfio` 会拒绝绑定宿主机当前活动的帧缓冲控制台（`isDeviceActiveFramebuffer`），同机的非控制台显卡（如 BMC 显卡之外的第二块 GPU）不受影响。单显卡（主显卡）直通请先按 `docs/hardware-passthrough-primary-display.md` 在宿主机完成准备。

## 运维验证

列出宿主机全部设备类别码，确认关键设备已被过滤：

```bash
# 类别码统计（直通列表只会出现白名单外的普通外设类别）
lspci -D -mm -n | awk '{gsub(/"/,"",$2); print $2}' | sort | uniq -c | sort -rn

# 查看某设备类别码、驱动与 IOMMU 组
cat /sys/bus/pci/devices/0000:3f:0b.3/class
basename "$(readlink -f /sys/bus/pci/devices/0000:3f:0b.3/driver)"
basename "$(readlink -f /sys/bus/pci/devices/0000:3f:0b.3/iommu_group)"
```

列表中设备明显偏少时，按以下顺序自查：

1. 设备是否属于上表硬排除类别（如 `08xx`、`0604`）；
2. 设备是否承载宿主机已挂载文件系统：`findmnt -S <设备>`；
3. 设备是否与 LPC/ISA 桥同 IOMMU 组（Intel PCH 的 SATA 控制器常见，例如 `00:1f.0` + `00:1f.2` 同组），此时整组无法独立隔离，建议改用独立 PCIe 直通卡或 NVMe；
4. 控制器上是否挂了宿主机键鼠：`grep -A5 Handlers /proc/bus/input/devices | grep -E 'kbd|mouse'`。

## 调整策略

新增/放开某类设备时，只需修改 `server/service/pci/policy.go` 中的策略表：

- `blockedClassBaseReasons` / `blockedClassSubReasons`：类别码级排除；
- `blockedDriverReasons` / `blockedDriverPrefixes`：驱动级排除；
- `virtualVendorReasons`：虚拟化平台设备；
- `virtualOrBMCGPUDrivers`：虚拟机显示与 BMC 管理显卡驱动。

保护规则（宿主机存储、键鼠、IOMMU 组）不建议放开，它们拦下的是「绑定后宿主机必然出问题」的场景。

本功能未新增 apt 或第三方依赖。设备扫描性能设计参见 `docs/passthrough-scan-performance.md`。
