# 虚拟机动态内存兼容处理

## 背景

部分 libvirt / QEMU 环境在 balloon 动态内存与 virtio-mem 弹性内存切换后，可能保留旧的 `<devices><memory model='virtio-mem'>` 设备。旧版本在应用 balloon 配置时会全局替换 `<memory>` 节点，可能把设备区内存节点改成缺少 `model` 属性的普通 `<memory>`，导致虚拟机开机前定义域失败。

典型错误：

```text
XML error: Missing required attribute 'model' in element 'memory'
```

## 处理策略

- 应用 balloon 动态内存配置时，只修改 `<vcpu>` 之前的顶层 `<memory>` 与 `<currentMemory>`。
- 从 virtio-mem 切换到 balloon 或静态内存时，自动移除旧的 virtio-mem 设备。
- 从 virtio-mem 切换回 balloon 时，自动清理 virtio-mem 专用 `maxMemory` 与 NUMA 配置，避免 libvirt 继续按内存热插拔校验。
- 如果旧配置中已经残留缺少 `model` 的设备区 `<memory>` 节点，应用动态内存配置时会自动清理。
- 应用 virtio-mem 配置时，同样先清理旧 virtio-mem 设备与坏的设备区内存节点，再重新注入合法的 `<memory model='virtio-mem'>`。

## 运维建议

遇到该错误时，优先让虚拟机关机后重新应用动态内存配置或再次开机。开机前的待迁移配置会自动修复 XML；不需要手工编辑数据库。

若仍失败，可在宿主机上检查持久化 XML：

```bash
virsh -c qemu:///system dumpxml <虚拟机名>
```

确认 `<devices>` 内不存在缺少 `model` 属性的 `<memory>` 节点。
