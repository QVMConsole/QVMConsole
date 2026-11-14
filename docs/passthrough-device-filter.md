# 硬件直通设备准入过滤

## 背景

虚拟机「硬件直通」选择列表原先依赖 `lspci` 输出的英文类别名做黑名单过滤（`host bridge` / `pci bridge` / `smbus` / `raid` 等），存在两个问题：

1. **漏网设备**：Xeon E5/E7 v2 平台的 CPU uncore 设备（UBOX Registers、QPI Link、Home Agent、Power Control Unit 等）类别是 `System peripheral`（类别码 `0880`），不在黑名单中，会直接出现在列表里且显示为「无驱动 / 空闲」，用户勾选后就可能把 CPU 内部寄存器绑定到 `vfio-pci`，导致宿主机崩溃。同类的还有性能计数器（`1101`，`ivbep_uncore` 驱动）、管理引擎 MEI（`0780`）、BMC 管理显卡（`mgag200` / `ast`）。
2. **判定脆弱**：类别名受发行版与本地化影响，且 `0680`（Bridge）等名称与黑名单不匹配；`classCodeToName` 对未登记类别返回 `PCI 设备(xxxxxx)` 后会被丢弃、回退成英文名，等于判定完全依赖英文串。

现在过滤统一改为 **基于 sysfs 的 PCI class code（主类 + 子类）** 判定，并叠加宿主机实际使用情况与 IOMMU 组隔离判定。策略集中在 `server/service/pci/policy.go`，硬件直通列表（`server/service/vm/passthrough.go`）与核显直通页面（`server/service/host/igpu_passthrough.go`）共用同一份判定，避免两处结论不一致。

## 查看全部设备

创建、编辑虚拟机共用的硬件直通选择器默认仍按下方规则筛选。滚动设备列表到底部，点击「查看全部直通设备」，阅读风险弹窗并完整等待 5 秒后，才能点击「我已了解风险，继续查看」。取消后重新打开风险弹窗会重新计时；关闭设备选择器后，全量模式失效，下次打开仍为默认筛选列表。

全量模式通过 `GET /host/passthrough?include_all=true` 读取全部成功枚举的 PCI 设备，不再按白名单、驱动、类别码、宿主机占用或 IOMMU 分组丢弃列表项。`lspci` 快速路径与 `virsh` 兼容路径均保留完整快照及 `is_passthrough_capable` / `capability_note`，默认与全量请求复用同一份扫描缓存。接口仍沿用管理员鉴权，并兼容 API Key；5 秒等待属于网页查看流程，不是服务端授权机制。

全量列表显示 IOMMU 分组及默认策略风险说明，原先未通过准入策略的设备也允许勾选；已分配给其他虚拟机或已经加入当前表单的设备不能重复选择。「未分配」只表示没有虚拟机占用，并不表示宿主机没有使用该设备。

从全量模式加入的设备携带 `host_devices[].unsafe_confirmed=true`，创建（ISO/空白）、普通模板克隆、链接克隆、Windows 克隆以及编辑设备时均透传到实际绑定/挂载步骤。批量克隆继续沿用既有约束：含直通设备时只允许创建一台，避免多个虚拟机争抢同一设备。管理员 API Key 可直接在绑定或挂载请求中传 `unsafe_confirmed=true`；不传或为 false 时仍执行原有准入策略。5 秒等待由网页强制执行，API 调用由调用者明确声明已确认风险。

确认风险后跳过默认准入策略，包括 USB 接口/驱动限制、关键设备类别、宿主机存储/键鼠占用、IOMMU 同组风险及活动帧缓冲控制台限制。仍要求 PCI 地址格式正确、设备真实存在、有 IOMMU 分组、VFIO 能正常接管，且设备未分配给其他虚拟机；编辑和挂载必须明确处于关机状态。真实 libvirt/VFIO 操作失败时照常返回错误，不保证所有硬件均支持成功直通。风险绕过会记录到应用日志，不持久化为全局开关或设备白名单。

编辑时先验证全部新增设备，只移除不再需要的设备，保留地址未变的现有直通配置。因此以后保留已配置设备无需再次传入风险确认；重新添加该设备则需重新确认。

### USB 设备为何没有出现在默认列表

旧策略并非放行所有 USB：只放开 USB 3.0 xHCI（`0c0330`）和 USB4 主机接口（`0c0340`），USB 1.1/2.0 由驱动及接口类型两层排除。Intel C610/X99 的 EHCI 控制器（例如截图中的 `8086:8d2d`、`8086:8d26`）属于 USB 2.0，因此不会出现在默认列表。全量模式会显示这类控制器及默认策略的风险说明，并允许在确认风险后选择和实际尝试直通；默认 USB 准入策略继续保留。本次未验证该用户机器上的实际驱动、键鼠占用与 IOMMU 分组。

此处列出的是 PCI USB 控制器，不是 `lsusb` 中单独的 U 盘、键盘或鼠标。控制器整体直通会影响它下挂的所有 USB 设备。现有策略文案中「USB 2.0 不支持直通」表达的是本项目当前的准入限制，不能据此推断所有 EHCI 硬件或客户机都不具备直通能力。

手工验收建议：分别在创建和编辑表单打开选择器，验证滚动到底部入口、空列表入口、取消后重新计时、5 秒前无法继续、5 秒后显示且可以勾选 EHCI/桥片等原先隐藏设备、提交载荷中对应设备携带 `unsafe_confirmed=true`、加载失败可重试、关闭后恢复默认模式，并检查深浅色主题。真实绑定验证请使用管理员确认可交给客户机的闲置设备，检查 VFIO 驱动与虚拟机实际识别结果。

兼容性检查影响评估：本次只在显式传入单个设备风险确认时改变直通准入，未配置直通设备的创建默认行为、XML、网络、依赖与清理逻辑不变。已核对系统兼容性脚本、CLI 和正式测试虚拟机参数；兼容性实机测试不配置宿主机 PCI 直通设备，因此无须同步增加设备绑定测试或修改安装流程。

开发校验包括后端完整编译、前端 TypeScript 与生产构建、修改文件局部静态检查及 `git diff --check`；未新增测试代码，也未执行真实硬件解绑/绑定或测试环境部署。

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

未明确传入风险确认时，以下三个入口都会再次执行同一套准入判定，防止仅绕过列表就绑定危险设备。管理员明确传入 `unsafe_confirmed=true` 时，这三处一致跳过默认准入策略，继续执行各自的格式、存在性、IOMMU、占用及绑定状态检查：

- `ValidatePCIPassthrough`（绑定、创建虚拟机、克隆虚拟机前置校验）
- `BindPCIDeviceToVfio`（绑定到 `vfio-pci`）
- `AttachPCIDeviceToVM`（挂载到虚拟机）

错误信息会说明具体原因，例如：

```
设备 0000:3f:0b.3（Intel Corporation Xeon E7 v2/Xeon E5 v2/Core i7 UBOX Registers）不允许直通：
系统外设（CPU uncore 寄存器、IOMMU/VT-d 等），直通会导致宿主机崩溃
```

## 显示类设备的额外保护

显示类设备（`03xx`）仍受驱动、虚拟化平台等默认准入规则约束；绑定环节还会额外拒绝宿主机当前活动的帧缓冲控制台（`isDeviceActiveFramebuffer`）。全量模式明确确认风险后，这些策略限制也会跳过。单显卡（主显卡）直通建议先按 `docs/hardware-passthrough-primary-display.md` 在宿主机完成准备。

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
