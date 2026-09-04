# VPC 交换机带宽限速

VPC 交换机的上行带宽使用 OVS 共享 packet meter，下行带宽在托管 NAT 模式使用 VPC 网关端口的 `tc` 队列整形；直通桥模式的上下行均使用 OVS 流表 meter。

## 规则落地要求

交换机带宽任务不能只判断 meter 是否创建成功，还必须存在引用该 meter 的 OpenFlow 流表。上行流表按虚拟机 vnet 的 `in_port` 匹配，不依赖来宾上报的源 IP。这样来宾使用错误地址、源地址伪造或攻击脚本产生异常源地址时，仍然受到交换机聚合带宽限制。

同一 `br-ovs` 上的 NAT 交换机通过 OVS access port 的 VLAN tag 区分端口。规则重建时会读取运行态 `virsh domiflist`，并同时使用数据库绑定和网桥/VLAN 反查，避免 VM 热插拔、迁移或历史绑定缺失时只创建 meter 而没有限速流。

## 现场诊断

```bash
ovs-ofctl -O OpenFlow13 dump-meters <bridge>
ovs-ofctl -O OpenFlow13 dump-flows <bridge>
ovs-vsctl get Port <vnet> tag
virsh domiflist <vm>
```

检查带宽时应同时确认：

1. meter 的单位为 `kbps`，速率值与面板配置一致；
2. 流表中存在对应 `meter:<id>`；
3. 流表的 `in_port` 是当前运行态 vnet 的 ofport；
4. NAT 交换机的 vnet access VLAN tag 与交换机 VLAN ID 一致。

VM 关机时不存在运行态 vnet/ofport，交换机只能保留配置，不能提前写入 VM 端口流表。VM 开机、网卡热插拔和服务启动恢复流程都会重新协调带宽规则。
