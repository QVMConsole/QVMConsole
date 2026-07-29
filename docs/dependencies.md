# 运行时第三方依赖清单

> 本文档记录面板功能实现中使用到的 apt / 第三方命令行工具（安装均已纳入 `install.sh`）。

| 依赖包 | 命令 | 用途 | 使用位置 |
|--------|------|------|----------|
| dmidecode | `dmidecode -t memory` | 读取宿主机内存条（DIMM）SMBIOS 信息，供概览页内存卡片「硬件详情」展开区展示 | `server/service/host/hardware.go` |

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
- 部分 ARM 设备与虚拟机的 SMBIOS 不提供内存设备数据，此时后端返回中文说明，前端正常降级展示，不影响其他功能。
