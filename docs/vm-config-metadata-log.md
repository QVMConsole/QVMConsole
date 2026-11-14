# 虚拟机配置元数据日志

面板通过 libvirt 虚拟机持久配置中的 `https://kvm-console.local/domain-config` 元数据保存备注、分组、标签和冻结标记。虚拟机没有设置这些信息时，该元数据可以不存在，面板按空配置处理。

读取或删除元数据时，如果 libvirt 返回元数据不存在，面板不会将其记录为错误；其他命令失败仍记录错误并返回给调用方。可以在宿主机用以下只读命令检查指定虚拟机的持久元数据：

```bash
virsh metadata <虚拟机名称> https://kvm-console.local/domain-config --config
```

缺少元数据时，`virsh` 会返回非零退出码和 `metadata not found`，这不表示虚拟机运行故障。检查本身不会修改配置，也不要求关机。
