# 外部快照恢复与自定义存储访问

## 适用范围

本文说明虚拟机外部快照在自定义宿主机存储池中的恢复流程，以及 AppArmor 开启时的磁盘访问处理。

## 恢复流程

外部快照恢复会执行以下步骤：

1. 关闭虚拟机并读取当前磁盘链。
2. 以选中的快照层创建新的恢复叠加盘，保留原快照文件和元数据。
3. 将虚拟机磁盘源切换到恢复叠加盘。
4. 使用 `qemu-img info --backing-chain` 展开完整 backing chain。
5. 修正活动磁盘与快照层的 libvirt 访问权限。
6. 根据实际磁盘路径更新 `virt-aa-helper` 与 `libvirt-qemu` 的 AppArmor 本地规则。
7. 同步当前快照指针并启动虚拟机。

## 自定义挂载点

虚拟机磁盘可以位于 `/var/lib/kvm-storage`，也可以位于管理员配置的其他挂载点，例如挂载点下的 `vm-disks` 目录。后端不会假定固定盘符或固定挂载目录：

- 已知的面板存储根目录继续使用根目录级规则。
- 对其他绝对路径，按实际磁盘所在目录生成最小范围规则。
- 目录参数直接使用该目录；文件参数使用其父目录。
- 根目录和相对路径不会被写入 AppArmor 规则。

该处理也覆盖没有 `.qcow2` 扩展名的外部快照层。`virt-aa-helper` 能读取完整 backing chain 后，libvirt 才能为模板盘、基础盘、历史快照层和当前恢复叠加盘生成完整的动态访问规则。

## 故障核对

启动错误包含 `Could not open ... Permission denied` 时，可依次核对：

```bash
namei -l /path/to/vm-disk.qcow2
qemu-img info --backing-chain /path/to/current-overlay.qcow2
journalctl -k --no-pager | grep 'apparmor="DENIED"' | tail -50
cat /etc/apparmor.d/local/usr.lib.libvirt.virt-aa-helper
cat /etc/apparmor.d/abstractions/libvirt-qemu.d/kvm-console-storage
```

如果 Unix 属主和目录遍历权限正常，而内核日志显示 `virt-aa-helper` 或对应 `libvirt-UUID` 配置被拒绝，应重点检查实际磁盘目录是否已进入上述本地规则。

## 回滚说明

面板写入的内容位于 `# BEGIN kvm_console managed storage access` 与 `# END kvm_console managed storage access` 标记之间。需要回滚时可删除该标记块并重载 `/etc/apparmor.d/usr.lib.libvirt.virt-aa-helper`；标记外的系统或管理员自定义规则不会被覆盖。
