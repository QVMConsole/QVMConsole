# 运行时第三方依赖清单

> 本文档记录面板功能实现中使用到的 apt / 第三方命令行工具（安装均已纳入 `install.sh`）。

| 依赖包 | 命令 | 用途 | 使用位置 |
|--------|------|------|----------|
| dmidecode | `dmidecode -t memory` | 读取宿主机内存条（DIMM）SMBIOS 信息，供概览页内存卡片「硬件详情」展开区展示 | `server/service/host/hardware.go` |
| qemu-utils（既有依赖） | `qemu-img info/convert` | 检查导入磁盘格式、将 OVF/OVA 包内磁盘转换为 QCOW2、将 OVA 导出磁盘转换为 streamOptimized VMDK | `server/service/vm/vmimport/`、`server/service/vm/export.go` |
| python3-virt-firmware（可选） | `virt-fw-vars` | 在 UEFI 克隆的 NVRAM 中预置 shim 连续引导标记，避免首次登记发行版启动项时显示恢复倒计时并冷复位 | `server/service/vm_xml/boot_type.go`、`server/service/clone/` |
| libvirt、QEMU、virtinst（既有依赖） | `virsh`、`qemu-system-*`/`qemu-kvm`、`qemu-img`、`virt-install` | 首次安装时通过正式创建链路定义并启动临时虚拟机，验证 KVM 与 libvirt 兼容性 | `scripts/check-system-compatibility.sh`、`server/service/compatibility/` |
| Open vSwitch、dnsmasq、iproute2、iptables（既有依赖） | `ovs-vsctl`、`dnsmasq`、`ip`、`iptables`、`ip6tables` | 验证基础 OVS 网桥、DHCP、网关、NAT、转发规则和测试虚拟机运行端口；公网 IPv6 使用 `ip -6`、Proxy NDP 与精确转发规则 | `server/service/compatibility/`、`server/service/public_ip/` |
| Open vSwitch（既有依赖） | `ovs-ofctl`、`ovsdb-client` | 端口安全 OpenFlow 多表、packet meter、Interface packet policing、OVSDB 端口事件与兼容性探测 | `server/service/network/portsecurity/`、`server/service/compatibility/` |
| iproute2、Open vSwitch、systemd（既有依赖） | `tc`、`ip`、`ovs-vsctl`、`ovs-ofctl`、`systemd-run` | 端口镜像的双向报文复制、veth 注入、OVS 转发和短时自动回滚看门狗 | `server/service/network/portmirror/` |
| curl / wget（既有下载能力） | `curl`、`wget` | 首次安装当前目录缺少有效兼容性脚本时下载脚本；优先 curl，回退 wget | `install.sh` |

## Linux 来宾磁盘自动化依赖

以下依赖安装在 Linux 模板或来宾系统内部，模板制作流程会自动尝试预装：

| 依赖包 | 主要命令 | 用途 |
|--------|----------|------|
| qemu-guest-agent | `qemu-ga` | 在线密码重置、磁盘识别和来宾命令执行 |
| cloud-guest-utils / cloud-utils-growpart | `growpart` | 扩展系统分区 |
| e2fsprogs | `resize2fs`、`mkfs.ext4` | ext4 创建与扩容 |
| xfsprogs | `mkfs.xfs`、`xfs_growfs` | XFS 创建与扩容 |
| btrfs-progs | `mkfs.btrfs`、`btrfs filesystem resize` | Btrfs 创建与扩容 |
| lvm2 | `pvs`、`pvresize`、`lvextend` | LVM 根卷扩容 |
| gdisk | `sgdisk` | 新数据盘 GPT 初始化 |
| parted | `partprobe` | 通知内核重新读取分区表 |

Windows 来宾使用系统自带 PowerShell 存储命令，无额外来宾软件包；仍需安装并运行 QEMU Guest Agent。

## 说明

- `dmidecode` 已加入 `install.sh` 的 `APT_DEPS`（RPM 系映射同名包）。
- OVF/OVA 功能复用安装脚本已有的 `qemu-utils` 与 Go 标准库归档能力，没有增加新的系统包。
- `install.sh` 会按发行版尽力安装 `virt-fw-vars`；部分 RPM 系软件源缺少该工具时仅给出警告，不阻断安装和克隆。后端同样采用兼容降级，工具缺失或版本过旧时保留 shim 原有的一次性恢复流程。
- 首次安装兼容性实机测试完全复用安装脚本已有的 libvirt、QEMU、virtinst、Open vSwitch、dnsmasq、iproute2、iptables 和下载工具，没有新增第三方依赖。
- 端口安全功能复用 `openvswitch-switch` / `openvswitch` 包提供的 `ovs-vsctl`、`ovs-ofctl` 与 `ovsdb-client`，没有新增软件包；较旧 OVS 是否具有 `pktps` meter 和 `ingress_policing_kpkts_*` 字段由启用预检与兼容性实机测试判定。
- 端口镜像复用安装流程已有的 `iproute2`、Open vSwitch 与 systemd，不新增软件包；启用预检会检查 `tc`、`ip`、`ovs-vsctl`、`ovs-ofctl`、`systemd-run` 和 `systemctl`。
- 公网 IPv6 功能复用 `iproute2` 与 `iptables` 软件包提供的 `ip`、`ip6tables`，没有新增软件包；上游公网前缀和 IPv6 默认路由属于网络环境条件，不作为面板安装依赖。
- Linux 来宾公网 IPv6 自动配置复用模板已有的 QEMU Guest Agent、`ip` 与 systemd；Agent 暂未连接时绑定仍保留，后台会在 VM 运行后重试。
- 部分 ARM 设备与虚拟机的 SMBIOS 不提供内存设备数据，此时后端返回中文说明，前端正常降级展示，不影响其他功能。
