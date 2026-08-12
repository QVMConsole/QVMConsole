# OVS 端口安全防护

## 功能定位

端口安全防护为全局可选功能。安装、升级及配置缺省值均为关闭；关闭时不会改写既有端口流表、带宽规则、虚拟机启动流程或网络选择。管理员在网络中心完成预检并主动开启后，系统才会为所有运行中虚拟机网卡安装策略。

策略由 `server/service/network/portsecurity/` 统一编译，使用专属 cookie、独立表级和 OVSDB `external_ids` 归属标记，带宽、公网 IP、VPC 与身份策略通过统一流水线衔接。清理只处理本模块持有的流表、meter、packet policing 字段和元数据。

## 默认配置

| 环境变量 / 设置键 | 默认值 | 说明 |
|---|---:|---|
| `KVM_PORT_SECURITY_ENABLED` | `false` | 总开关；应通过网络中心预检后启用 |
| `KVM_PORT_SECURITY_TOTAL_KPPS` | `50` | 单端口总入站包速率，单位 kpps |
| `KVM_PORT_SECURITY_TOTAL_BURST_KPACKETS` | `40` | 总包速率突发，单位 kpackets |
| `KVM_PORT_SECURITY_NEIGHBOR_PPS` | `200` | ARP/ND packet meter 速率 |
| `KVM_PORT_SECURITY_NEIGHBOR_BURST_PACKETS` | `400` | ARP/ND 突发报文数 |
| `KVM_PORT_SECURITY_BROADCAST_PPS` | `1000` | 其他广播/组播 packet meter 速率 |
| `KVM_PORT_SECURITY_BROADCAST_BURST_PACKETS` | `2000` | 其他广播/组播突发报文数 |
| `KVM_PORT_SECURITY_RECONCILE_INTERVAL_SECONDS` | `60` | 周期协调间隔；OVSDB 端口变化仍即时触发 |

速率参数位于“系统设置 → 存储与网络”。总开关位于“网络中心 → 网络概览”。总开关关闭时高级阈值和地址策略表单隐藏，保存设置也不提交这些字段。

## 策略模式

- **空交换机信任网络**：没有物理上行且未启用内置 DHCP 的独立交换机完全排除在端口安全流表、源地址限制、DHCP/RA 拦截和 packet policing 管理范围之外。来宾 DHCP、DHCPv6 与 RA 可正常通过，适合软路由 LAN；交换机和网口的流量/带宽配额仍单独生效。
- **系统/NAT 网络**：严格校验源 MAC、ARP SHA、IPv4 源地址与 ARP SPA；允许地址由静态绑定、dnsmasq 租约和公网 IP 绑定汇总。默认丢弃 IPv6；绑定路由型公网 IPv6 后，仅允许该 VM 对应的精确 `/128`、DAD 与必要 ND 报文。
- **直通桥兼容保护**：始终校验源 MAC、ARP SHA、DHCP 服务端行为和速率；`allowed_ipv4_addresses` 为空时保留未知 IPv4 连通性，填写后切换为精确 IPv4 校验。
- **直通桥 IPv6**：交换机需启用 `ipv6_security_enabled` 并配置 `trusted_ipv6_prefixes`；每张网卡必须配置可信前缀内的 `allowed_ipv6_addresses`。策略校验 IPv6 源、ND SLL/TLL、DAD，并阻断 RA、Redirect 与 DHCPv6 服务端报文。
- **手工隔离**：隔离状态记录于 OVS Interface `external_ids`，协调和服务重启后继续保持，直到管理员释放。

除空交换机信任网络外，其余受管模式都会阻断虚拟机发出的 DHCP 服务端报文。ARP/ND 与其他广播/组播分别使用 packet meter；总包速率使用 `ingress_policing_kpkts_rate` 与 `ingress_policing_kpkts_burst`。

## 开启流程

1. 在交换机编辑页配置需要 IPv6 防护的直通桥可信前缀。
2. 在虚拟机“网口管理”或创建向导中登记对应网卡的精确地址。
3. 在网络概览点击“预检”。预检只读取 OVS、libvirt 与绑定信息，不替换流表。
4. 处理按虚拟机、网卡分组显示的阻断项。
5. 提交启用操作并完成二次验证。任务会在所有检查通过后原子持久化总开关。

预检检查 OpenFlow13、OpenFlow14 bundle 可用性、packet meter、packet policing、meter 容量、OVS 端口归属、NAT IPv4 身份资料及直通桥 IPv6 清单。任何阻断项都会结束本次任务并保留原状态。

## 生命周期

- 创建、克隆、批量克隆和导入在首次启动前保存主网卡绑定；防护开启时先暂停启动，策略安装和回读成功后再恢复。
- 热添加先以 link-down 接入，绑定和策略校验完成后置为 up；失败会删除绑定并卸载网卡。
- 启动、恢复、重置、网卡变更、公网 IP 与带宽变更触发协调；OVSDB Interface 事件和周期任务负责运行时恢复。
- 动态公网 IPv6 前缀变化会先更新绑定地址，再协调端口身份策略；旧前缀源地址随协调删除，防止 VM 继续伪造失效地址。
- 运行态网卡优先按逻辑序号对应的稳定 MAC 关联绑定，再按实际 OVS 网桥与 VLAN 兼容旧虚拟机，最后才回退到 libvirt 列表位置；热插拔或重定义导致 XML 网卡节点换序时不会串用其他网卡的交换机与地址策略。
- VM 级和交换机级带宽策略按全部 libvirt 网卡分别落地；没有租约的网卡使用 MAC/ofport 匹配，避免额外网卡形成限速旁路。
- OpenFlow14 bundle 可用时，将本模块旧 cookie 删除和新规则添加放入同一事务；bundle 不可用或被交换机拒绝时，先用高优先级隔离目标端口，再顺序更新、验证并释放。
- DHCP 初次启动允许客户端报文，租约出现后自动收紧为精确 IPv4 策略。
- 交换机从受管网络切换为空交换机时，协调器会清理该独立网桥上的端口安全 cookie、隔离 cookie、packet meter 和归属元数据；切换回物理直通或托管网络后重新纳入目标策略。

## API

接口均位于管理员授权路由并兼容 API Key：

- `GET /api/ovs/port-security/status`
- `POST /api/ovs/port-security/preflight`
- `POST /api/ovs/port-security/enable`
- `POST /api/ovs/port-security/disable`
- `POST /api/ovs/port-security/reconcile`
- `POST /api/ovs/port-security/ports/:port/isolate`
- `POST /api/ovs/port-security/ports/:port/release`

启用、停用、隔离和释放保留二次验证；所有修改操作通过任务队列执行。状态响应包含逐端口策略模式、允许地址、meter ID、policing、丢包计数、最后协调时间及异常原因。

## 诊断命令

```bash
ovs-ofctl -O OpenFlow13 dump-flows <bridge> 'cookie=0x51564d5053454301/0xffffffffffffffff'
ovs-ofctl -O OpenFlow13 dump-meters <bridge>
ovs-vsctl get Interface <port> external_ids
ovs-vsctl get Interface <port> ingress_policing_kpkts_rate ingress_policing_kpkts_burst
virsh domiflist <vm>
```

虚拟机网络状态和网络诊断接口也会返回逐网卡端口安全信息。总开关开启后，网络概览可手动协调、隔离或释放端口。

启动恢复或后台协调预检失败时，日志会直接记录阻断项对应的虚拟机、逻辑网卡序号和原因，便于区分地址资料缺失与运行态端口映射异常。

## 停用与回滚

停用任务删除专属 cookie、登记 meter 和本模块持有的 policing/metadata，然后恢复现有普通转发；公网 IP 和带宽模块随后重新生成关闭模式下的既有表级规则。单端口更新失败时保持隔离并记录错误，修复 OVS 或地址资料后执行“协调”释放。

兼容性实机测试会创建唯一命名的隔离 OVS 探测网桥，实际验证 packet meter、packet policing、OpenFlow bundle 或隔离兼容路径以及规则回读；测试结束统一删除探测流表、meter、文件和网桥。
