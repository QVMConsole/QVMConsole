# QVMConsole 实机测试环境基准

> 本文件是**实机测试的唯一环境基线**：开始实机测试前必须逐项核对本文件；与基准不一致时先记录偏差、判断是否影响用例，再决定继续或中止。
> 本文件不是审查结论，也不是"理想状态"——它记录的是**当前可用的开发审查实例的真实状态**。
> 采集方式：全部通过面板 HTTP 接口实测（`GET /api/system-info`、`/api/vm/list`、`/api/vm/:name`、`/api/vm/:name/disks`、`/api/vm/:name/interfaces`、`/api/ovs/status`、`/api/ovs/leases`、`/api/network/bridges`、`/api/network/host/interfaces`、`/api/vpc/switches`、`/api/storage-pool/list`、`/api/settings`、`/api/firewall/host/status`、`/api/ovs/port-security/status`、`/api/task/list`），并用 `virsh list --all`、`ip -br addr` 交叉核对。
> 核对时间：`2026-09-19T16:18:23+08:00`

## 一、使用规则（先读这一节）

1. **测试前核对**：实机测试前按第七章清单逐项核对，核对结果写入本次审查报告。
2. **测试后无需恢复**：本条为明确约定——实机测试结束后**不需要**把环境恢复到本基准状态，环境最终会被整体还原。因此可以放心做破坏性验证（删除/格式化/写坏磁盘、杀进程、改配置、崩溃注入等）。
3. **唯一红线：不得造成宿主机断网/失联**。除断网以外的系统破坏都不需要考虑后果。断网相关操作见第九章。
4. **凭据唯一来源**：本文件第二章与 `review/GUIDE.md` §4.4.1；除已登记的开发实例口令外，其他凭据由用户当场提供。
5. **变更即更新**：新增/删除虚拟机、改动 OVS、改动存储池、改动面板开关后，必须更新本文件并在第八章追加记录。

## 二、访问入口与凭据（按用户要求明文登记）

| 用途 | 地址 / 位置 | 账号 | 口令（明文） |
| --- | --- | --- | --- |
| 面板 API（后端直连） | `http://127.0.0.1:8080`（同机 `http://127.0.0.1:8080`） | `admin` | `admin123` |
| 面板前端（Vite dev） | `http://127.0.0.1:5173` | `admin` | `admin123` |
| 虚拟机 `vmol65r8h0`（Debian 13） | 控制台 / SSH，网段 `192.168.122.0/24`（DHCP） | `root` | `2wzYxtegmV^ZyAr%` |
| 虚拟机 `vmtoawwgbv`（Windows LTSC 2021） | 控制台 / RDP，历史租约 `192.168.122.123` | `administrator` | `kjdszCa9o@C@cp9q` |

- 两台虚拟机的口令与面板 `vm_credentials` 表中记录的凭据一致（`/api/vm/:name` 的 `credential` 字段，`source=clone`）。
- `admin123` 命中公开泄露库（登录后 `password_breached=true`），且**首次登录会使该次 JWT 立即失效**（泄露检测刷新 `security_updated_at`），重新登录一次即可，详见 `review/GUIDE.md` §4.4.1。
- 上述口令仅限本机开发审查实例使用，禁止用于生产或任何对外环境。

## 三、宿主机基准

| 项 | 值 |
| --- | --- |
| 主机名 | `vm-huih6qoe` |
| 操作系统 | Ubuntu 26.04.1 LTS（`os_id=ubuntu`，`os_id_like=debian`） |
| 内核 | `7.0.0-31-generic` |
| CPU | 8 vCPU |
| libvirt | libvirtd 12.0.0 |
| QEMU | QEMU emulator version 10.2.1（SPICE 支持：是） |
| OVS | `openvswitch-switch`，服务 active |
| DHCP | `kvm-console-ovs-dnsmasq.service`，服务 active |
| 面板进程 | `/root/QVMConsole_pro/server/tmp/kvm-console`（工作目录 `server/`，分支 `pro`，HEAD `f5b8574`） |
| 面板数据库 | `/root/QVMConsole_pro/server/data/kvm_console.db` |
| 面板端口 / 前端端口 | `8080` / `5173` |
| 面板版本 | `dev`（`build_time` 为空） |
| 开发模式 | `development_mode=true` |
| 公网访问 | `public_access_enabled=false` |
| 运行时长 | 约 1 小时（采集时） |

### 3.1 网络基准

| 项 | 值 |
| --- | --- |
| 物理网卡（上行，红线） | `enp1s0`，MAC `52:54:00:3a:f8:25`，UP，MTU 1500，`127.0.0.1/24`，**承载默认路由**，网关 `192.168.11.2` |
| OVS 网桥 | `br-ovs`，存在且有网关 `192.168.122.1/24`，uplink `enp1s0` |
| NAT | `iptables -t nat -C POSTROUTING -s 192.168.122.0/24 -o enp1s0 -j MASQUERADE` 存在 |
| 转发 | 出站/回程 FORWARD 规则存在，`ip_forward` 已开启 |
| 虚拟机网段 | `192.168.122.0/24`，网关 `192.168.122.1`，DHCP 池 `192.168.122.2–254` |
| 网络后端 | `network_backend=ovs`；默认网络开关 `default_network=default` |
| ufw | `active=false`；已有规则 `allow 8090/tcp`；受保护规则含 SSH(22) 等由面板管理 |
| 端口安全 | `enabled=false`，`healthy=true`，已应用端口 0 |
| 可信代理 | `KVM_TRUSTED_PROXIES` 未配置，无反向代理 |

> ⚠️ `enp1s0` 的接口风险提示（面板原文）：**"承载默认路由，桥接时可能短暂中断宿主机网络"**——这是本环境唯一不可承受的破坏点。

### 3.2 存储基准

| 设备 | 大小 | 文件系统 | 挂载点 | 默认池 | VM 目录 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `/dev/vda1` | 96 GiB | ext4 | `/` | **是** | `/var/lib/libvirt/images` | 已用 23%（约 24.6 GB / 96 GB） |
| `/dev/vda13` | ~1.0 GiB | ext4 | — | 否 | `/boot/vm-disks` | 启动分区 |
| `/dev/vda14` / `/dev/vda15` | 4 MiB / ~104 MiB | — / vfat | — | 否 | — / `/boot/efi/vm-disks` | 启动分区 |
| `/dev/vdb` | 100 GiB | 无 | 未挂载 | 否 | — | **未配置的空盘，适合作为破坏性存储测试靶盘** |

| 目录 | 值 |
| --- | --- |
| 克隆目录 / 镜像目录 | `/var/lib/libvirt/images` |
| 模板目录 | `/var/lib/libvirt/images/templates` |
| ISO 目录 | `/var/lib/libvirt/images/ISO` |
| 端口转发目录 | `/etc/kvm-portforward` |

## 四、虚拟机基准

### 4.1 `vmol65r8h0`（Debian 13，关机）

| 项 | 值 |
| --- | --- |
| 模板 | `debian-13-generic-amd64-20260601-2496` |
| os_type / arch | `linux` / `x86_64` |
| UUID | `b4d17d77-0731-40b6-a132-536e90333e62` |
| 规格 | 2 vCPU / 2048 MB（max 2048） |
| 机型 / 固件 | `q35` / `uefi`，APIC、PAE 开启，`nested_virt=true` |
| 显示 | `virtio`；RTC `utc` |
| 磁盘 | `vda` → `/var/lib/libvirt/images/vmol65r8h0.qcow2`，qcow2，10.00 GB（已用 0.02 GB），bus `virtio`，支持热插拔 |
| 网卡 | `virtio`，MAC `52:54:00:08:f2:4b`，网络模式 `bridge`（`br-ovs`），交换机 id=1「基础网络」，安全组 id=2 |
| 凭据（面板记录） | `root` / `2wzYxtegmV^ZyAr%`（`source=clone`，operator `admin`） |
| Guest Agent | configured=true，**connected=false**（未启动） |
| 电源状态 | `shut off`（与 `virsh list --all` 一致） |
| 创建时间 | `2026-09-19 08:12:22` |
| 磁盘健康 / 锁定 | healthy=true / locked=false |

### 4.2 `vmtoawwgbv`（Windows Server LTSC 2021，关机）

| 项 | 值 |
| --- | --- |
| 模板 | `SW_DVD9_WIN_ENT_LTSC_2021_64BIT_ChnSimp_MLF_X22-84402` |
| os_type / arch | `windows` / `x86_64` |
| UUID | `ad2e07a5-0584-4bec-9e61-6437c227bdfe` |
| 规格 | 4 vCPU / 4096 MB（max 4096），CPU 拓扑 `single_socket` |
| 机型 / 固件 | `q35` / `uefi-secure`（安全启动），APIC、PAE 开启，`nested_virt=true` |
| 显示 | `vga`；RTC `localtime` |
| 磁盘 | `vda` → `/var/lib/libvirt/images/vmtoawwgbv.qcow2`，qcow2，30.00 GB（已用 1.17 GB）；另有空光驱 `sda`（sata，未挂载） |
| 网卡 | `virtio`，MAC `52:54:00:14:51:ec`，网络模式 `bridge`（`br-ovs`），交换机 id=1「基础网络」，安全组 id=2 |
| 凭据（面板记录） | `administrator` / `kjdszCa9o@C@cp9q`（`source=clone`，operator `admin`） |
| Guest Agent | configured=true，**connected=false**（关机中） |
| 电源状态 | `shut off`（与 `virsh list --all` 一致） |
| 创建时间 | `2026-09-19 08:15:45` |
| 历史 DHCP 租约 | `192.168.122.123`，hostname `DESKTOP-2MPSVT8`，到期 `2026-09-19 20:14:52` |

### 4.3 任务与模板基准

- 任务总数 7，最近三条：`#7 clone success 08:11:27Z`、`#6 clone success 08:11:05Z`、`#5 clone failed 08:10:39Z`；更早为 `#4/#3 template_import success`。
- 采集时无 pending/running 任务；无遗留锁定（两台 VM `locked=false`）。

## 五、面板安全开关基准

| 开关 | 值 | 对测试的含义 |
| --- | --- | --- |
| `development_mode` | `true` | 二段登录、428 高风险验证、公网门禁**不可验证** |
| `public_access_enabled` | `false` | 非 LAN 403、空闲失效、可信代理解析不可验证 |
| `smtp_configured` | `false` | 邮箱验证码、找回密码链路不可验证 |
| 管理员 TOTP | 未绑定 | `login_verify`、TOTP 高风险验证不可验证 |
| `session_fingerprint_enabled` | `true` | 会话指纹可验证（同源 IP 变化除外） |
| `request_filter_enabled` | `true`、`request_detail_log_enabled=true` | 请求过滤与日志脱敏可验证 |
| `password_breach_check_enabled` | `false` | 定时泄露扫描关闭，但登录仍会做在线抽查 |
| `scheduled_storage_trim_enabled` | `true` | 存在定时 trim 行为，测试存储时需注意 |
| `maintenance_mode` | `false` | — |
| `hardware_passthrough_enabled` | `false` | IOMMU/VFIO 相关界面与接口受此开关限制 |
| `port_security_enabled` | `false` | 端口安全默认关闭 |

## 六、已知偏差与观察项

1. **`/api/vm/list` 状态可能短暂滞后**：同一分钟内 `/api/vm/list` 曾把 `vmtoawwgbv` 报为 `running`，而 `/api/vm/:name` 与 `virsh list --all` 均为 `shut off`；约 1 分钟后列表自愈为 `shut off`。**存疑**，非本次变更引入，实机测试若依赖列表状态需以 `/api/vm/:name` 或 `virsh` 为准。
2. **列表与详情的 `is_linked_clone` 不一致**：同一时刻 `/api/vm/list` 对两台 VM 返回 `is_linked_clone=true`，而 `/api/vm/:name` 返回 `false`，磁盘为完整 qcow2 而非 backing file。**存疑**，值得作为后续审查项。
3. 两台 VM `guest_agent_status.connected=false`；未启动的 VM 属正常，启动后应复测。
4. `/api/ovs/leases` 保留 `vmtoawwgbv` 的旧租约，VM 关机后不清理，属正常 DHCP 行为。
5. `/root/QVMConsole_pro/.git/config` 的 remote URL 内嵌**明文 GitHub PAT**，属环境风险，建议轮换并改用凭据助手（本文件不记录该值）。
6. `/root/QVMConsole_pro/review/` 下存在未跟踪文件 `.last-review.json`、`reports/`（上一轮审查产物）。

## 七、实机测试前核对清单

逐项执行并把结果写入本次报告（全部为只读操作）：

- [ ] 面板可访问：`curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/public/version` → 期望 200
- [ ] `admin` / `admin123` 登录成功（首次 401 需重登，见第二章说明）
- [ ] `GET /api/vm/list` 与 `GET /api/vm/<name>` 均返回两台 VM，状态与第四章一致
- [ ] `virsh list --all` 输出与第四章一致（用于识别列表缓存偏差）
- [ ] `GET /api/ovs/status`：`bridge_exists`、`bridge_has_gateway`、`ip_forward_enabled`、NAT 规则均为真
- [ ] `ip -br addr show enp1s0` 仍为 `127.0.0.1/24`，默认路由仍指向 `192.168.11.2`（**红线项**）
- [ ] `GET /api/storage-pool/list` 默认池 `vda1` 可用、剩余空间满足本次用例
- [ ] `GET /api/task/list` 无 pending/running 残留；两台 VM `locked=false`
- [ ] 记录本次将使用的靶机/靶盘：`vmol65r8h0`、`vmtoawwgbv`、空盘 `/dev/vdb`
- [ ] 若使用网络类用例，先确认第九章红线条目与恢复方式

## 八、变更记录

| 时间（+08:00） | 变更 | 操作者 |
| --- | --- | --- |
| `2026-09-19 08:12` / `08:15` | 新增虚拟机 `vmol65r8h0`、`vmtoawwgbv`（由模板克隆） | 用户 |
| `2026-09-19 15:55` | 面板管理员口令重置为 `admin123`（改前已备份 DB 与旧哈希） | 审查会话 |
| `2026-09-19 16:18` | 通过接口采集并建立本环境基准 | 审查会话 |

> 后续任何变更请在上表追加一行，并同步修订对应章节。

## 九、断网红线（唯一不可承受的破坏面）

以下操作可能切断宿主机管理通道（`enp1s0` 默认路由 / `br-ovs` 网关 / 面板 8080 / SSH 22），一旦断网将无法自动恢复，**默认禁止**；确需验证时必须先向用户说明并获得批准，且准备带外恢复手段（云控制台/串口）：

- 把 `enp1s0` 桥接进 OVS、删除默认路由、修改默认网关；
- `iptables`/`nftables` 默认策略置为 DROP，或清空 `FORWARD`/`NAT` 链；
- 关闭、重启或重配 `openvswitch-switch`、`kvm-console-ovs-dnsmasq.service`、`systemd-networkd`；
- 大规模清理 OVS 流表/端口 / 删除 `br-ovs`；
- 修改面板监听端口而未同步放行，或误改 `KVM_TRUSTED_PROXIES` 导致门禁误判；
- 切换 `public_access_enabled` 组合错误配置导致自身被判定为非 LAN。

**除上述断网风险外，其余破坏性操作（删除/格式化磁盘、杀进程、写坏配置、触发崩溃、制造并发冲突等）均允许执行，测试后无需恢复。**