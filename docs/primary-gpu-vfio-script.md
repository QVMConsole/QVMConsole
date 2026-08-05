# 宿主机主显卡 VFIO 准备脚本

## 适用场景

当待直通的显示控制器仍被宿主机 `fb0` 使用时，QVMConsole 会拒绝绑定到 `vfio-pci`，避免宿主机显示崩溃。若确认宿主机可以无头运行，并且可通过 SSH、BMC 或其他带外方式恢复管理，可使用 `scripts/prepare-vfio-primary-gpu.sh` 在启动早期将该显示控制器绑定到 `vfio-pci`。

此脚本适用于单独隔离在一个 IOMMU 组内的显示控制器。它不会修改 QVMConsole 数据库，也不会自动重启宿主机。

## 风险与前置条件

- 应用配置并重启后，目标显卡的宿主机本地显示输出会不可用。
- 必须先确认 SSH 或带外管理可在重启后恢复连接。
- IOMMU 组必须只包含目标设备；脚本发现同组有其他设备时会拒绝继续。
- 宿主机需要使用 Debian 系列的 GRUB、`update-grub` 和 `update-initramfs`。
- 不要在虚拟机运行时切换主显示控制器；应先关闭相关虚拟机。

## 检查

先以普通用户执行检查，不写入任何配置：

```bash
bash scripts/prepare-vfio-primary-gpu.sh --device 0000:00:02.0 --check
```

检查会显示当前驱动、IOMMU 组成员、显示控制器数量，以及设备是否正在承载 `fb0`。

## 应用

仅在确认可失去宿主机本地画面后，以 root 权限执行：

```bash
sudo bash scripts/prepare-vfio-primary-gpu.sh \
  --device 0000:00:02.0 \
  --apply \
  --confirm-host-console-loss
```

脚本会执行以下操作：

1. 在 `/etc/default/grub.d/99-qvmconsole-vfio-primary-gpu.cfg` 写入 IOMMU、`vfio-pci.ids`、原显示驱动黑名单和 `video=efifb:off` 参数。
2. 在 `/etc/modprobe.d/qvmconsole-vfio-primary-gpu.conf` 为目标设备配置 `vfio-pci`。
3. 将 VFIO 模块加入 `/etc/initramfs-tools/modules` 的脚本专属标记块。
4. 备份被覆盖前的配置到 `/var/lib/qvmconsole/vfio-primary-gpu/`，并更新 initramfs 和 GRUB。

完成后脚本只提示重启，不会自动执行 `reboot`。重启后验证：

```bash
readlink -f /sys/bus/pci/devices/0000:00:02.0/driver
```

预期结果为指向 `vfio-pci` 的路径。此时再通过 QVMConsole 为已关机虚拟机添加该 PCI 设备。

## 回退

若重启前需要撤销，或需要恢复宿主机本地显示，执行：

```bash
sudo bash scripts/prepare-vfio-primary-gpu.sh --revert
```

该命令仅删除本脚本写入的 GRUB、modprobe 与 initramfs 标记配置，并更新启动文件；仍需手动重启才能恢复原驱动。
