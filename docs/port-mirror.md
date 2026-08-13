# OVS 端口镜像

## 功能定位

管理员可以在“网络中心 → 网络概览 → 端口镜像”中选择一个或多个宿主机源接口，将入方向、出方向或双向报文复制到一个或多个空交换机。原报文继续沿原路径转发，目标交换机收到的是副本。

典型用途：

- 选择系统基础 OVS 网桥（例如配置项 `KVM_OVS_BRIDGE` 对应网桥），在 NAT 前采集并保留虚拟机局域网 IP；
- 选择物理出口，在 NAT 后采集实际线上报文；
- 将副本交给接入空交换机的审计、IDS 或抓包虚拟机。

## 数据路径

```text
每个源接口 ingress / egress
        │ 一条 tc 规则，包含多个 mirred mirror 动作
        ├──────────────┬──────────────┐
        ▼              ▼              ▼
源 A→目标 1 veth  源 A→目标 2 veth  其它目标
        │              │
        ▼              ▼
目标 OVS 空交换机 1   目标 OVS 空交换机 2
        │ FLOOD         │ FLOOD
        ▼               ▼
各目标交换机内全部虚拟机端口
```

目标限制为空交换机：系统基础网络、启用内置 DHCP/NAT 或带物理上行的交换机不会出现在候选项。源接口与目标网桥不能相同，以避免二层环路。

来源和目标按笛卡尔积建立连接。例如选择 2 个来源和 3 个目标时会建立 6 对 veth/OVS 注入口。每个源方向仅使用一条 tc 过滤器，在其中串联 3 个复制动作，避免同一源流量被重复匹配。

当前转发动作是 `FLOOD`，因此每个目标交换机中的全部虚拟机网口都能收到对应镜像副本。只应将审计设备接入专用交换机。多选目标会按目标数量增加复制、内存带宽和 OVS 转发开销。

## 安全与回滚

1. 启用或更新前只读校验接口、目标交换机、OpenFlow13、依赖命令和预留 tc 优先级。
2. 修改前创建唯一名称的 systemd 瞬态定时器，两分钟后执行自动回滚。
3. 后端依次创建 veth、OVS 端口、专用 cookie 流表和 tc 过滤器，并逐项回读。
4. 所有验证通过后才写入持久配置并停止看门狗。
5. 任一步失败会立即清理本次对象；更新旧配置失败时还会尝试恢复旧镜像。
6. 清理只匹配本模块固定 tc 优先级、专用 veth/OVS 元数据和 cookie 前缀，不改写其他网络规则。
7. 启动与停用兼容早期单来源配置格式；运行态文件丢失时会按专用接口前缀、tc 动作和 OVS 元数据联合清理残留。
8. 自动回滚启动前会确认全部临时接口名称空闲；名称冲突只返回错误，不删除冲突接口。

配置文件：

```text
/etc/kvm-console/port-mirror/config.json
```

运行态文件：

```text
/run/kvm-console/port-mirror-runtime.json
/run/kvm-console/port-mirror-watchdog.json
```

服务启动时会先恢复 VPC 交换机，再根据持久配置恢复端口镜像。运行状态和计数始终从 `tc`、OVS 端口与 OpenFlow 流表回读，不使用数据库保存虚拟网络运行态。

## API

所有接口仅管理员可用，同时兼容 API Key：

- `GET /api/ovs/port-mirror/options`
- `GET /api/ovs/port-mirror/status`
- `POST /api/ovs/port-mirror/enable`
- `POST /api/ovs/port-mirror/disable`

启用示例：

```json
{
  "source_interfaces": ["br-ovs", "enp61s0f0np0"],
  "target_switch_ids": [101, 102],
  "direction": "both"
}
```

`direction` 支持 `ingress`、`egress` 和 `both`。启用和停用属于敏感网络操作，保留二次验证并通过任务队列执行。

## 诊断

```bash
tc -s filter show dev SOURCE ingress
tc -s filter show dev SOURCE egress
ovs-vsctl --data=bare --no-heading --columns=name find Interface external_ids:qvm-purpose=port-mirror
ovs-ofctl -O OpenFlow13 dump-flows TARGET_BRIDGE 'cookie=0x51564d4d00000000/0xffffffff00000000'
```

如果状态显示异常，可在界面停用后重新启用。停用会清理模块持有的过滤器、veth、OVS 端口、专用流表、配置文件和看门狗。

项目同时提供 `scripts/port-mirror.sh` 运维脚本，供脱离面板时进行临时验证或紧急回滚；来源和目标均使用逗号分隔，脚本同样按笛卡尔积创建连接：

```bash
sudo scripts/port-mirror.sh apply br-ovs,enp61s0f0np0 qvsw101,qvsw102 both
sudo scripts/port-mirror.sh status
sudo scripts/port-mirror.sh rollback
```

脚本的 tc 优先级、OVS 元数据、cookie、看门狗和清理边界与后端实现一致。
