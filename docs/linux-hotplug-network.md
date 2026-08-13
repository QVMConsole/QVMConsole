# Linux 热插拔网口 DHCP 兼容

## 问题与适用范围

部分 Linux 模板使用 Netplan 为主网口生成按 MAC 精确匹配的 systemd-networkd 规则。运行中的虚拟机热插拔额外网口后，额外网口在系统内会显示为 `DOWN` 或 `unmanaged`，因为没有可匹配的网络配置。

本次调整适用于使用 `systemd-networkd` 的 Linux 来宾。主网口原有的 DHCP、静态地址、网关和 DNS 配置保持不变。

## 处理方式

- Linux 克隆初始化时会写入 `/etc/systemd/network/99-qvm-hotplug.network`。
- 该规则匹配 `en*` 网口并启用 DHCP，DHCP 路由度量为 `200`。
- 主网口的 Netplan 规则名称优先级更高（通常为 `10-netplan-*`），因此仍优先使用原有主网口配置和路由度量。
- 面板在运行中的虚拟机添加网口后，若 QEMU Guest Agent 已连接且来宾使用 systemd-networkd，会写入同一规则并重新配置网卡，使新增网口立即启用；Guest Agent 不可用时，网口 XML 仍会持久化，来宾下次启动后按模板规则生效。

## 验证命令

在 Linux 来宾中执行：

```bash
ip -br addr
networkctl list --no-legend
```

新增网口应为 `UP` 且显示为 `configured`；若对应 VPC 提供 DHCP 服务，应获得 IPv4 地址。主网口默认路由的度量仍应低于附加网口。

## 物理直通 VLAN 网口

- 附加网口选择“物理直通 / VLAN”交换机时，网口 XML 使用交换机的 `bridge_vlan_id`。
- 运行中的虚拟机热插网口后，系统会同步设置对应 `vnet` OVS 端口的 VLAN tag。
- 持久化 XML 同时包含 `<vlan><tag id='...'/></vlan>`，虚拟机重启后仍保持相同 VLAN。

宿主机可使用以下命令核对运行态与持久化配置：

```bash
ovs-vsctl get Port <vnet端口> tag
virsh dumpxml --inactive <虚拟机名称>
```
