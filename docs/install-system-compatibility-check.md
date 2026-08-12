# 首次安装系统兼容性实机测试

## 功能说明

首次安装时，`install.sh` 会在依赖、运行目录和基础 OVS 网络准备完成后询问：

```text
是否运行系统兼容性测试？首次安装强烈推荐 [Y/n]:
```

直接回车或输入 `y` 会执行实机测试；输入其他内容会跳过测试并继续安装。更新、修复配置和卸载流程不会显示该提示。

测试通过后才代表宿主机已实际完成一次“创建虚拟机 → 接入基础 OVS 网络 → 启动 → 联合验证 → 清理”的完整流程。单纯通过依赖命令检查不等同于实机兼容。

## 脚本查找与下载

选择执行后，安装器只按以下顺序取得脚本：

1. 检查启动 `install.sh` 时的当前工作目录下是否存在 `check-system-compatibility.sh`。
2. 对本地文件执行非空、可读及 `bash -n` 语法校验。
3. 本地文件缺失或校验失败时，将 `install.sh` 顶部的变量作为下载地址：

   ```bash
   COMPATIBILITY_CHECK_URL=""
   ```

4. 下载优先使用 `curl`，失败后回退 `wget`。内容先写入同目录临时文件，校验通过后设置为 `700` 权限并原子替换目标文件。

下载地址当前预留为空。发布时只需填写该变量，无需调整其他下载逻辑。地址为空、下载失败或脚本校验失败都会记录为“兼容性测试未执行”，并进入是否继续安装的确认流程。

发行打包时，`build.sh` 会把源码文件 `scripts/check-system-compatibility.sh` 复制到发行包根目录。因此在解压后的发行目录内直接运行 `install.sh`，默认会命中本地脚本。

有效脚本会安装到：

```text
/opt/kvm-console/scripts/check-system-compatibility.sh
```

## 测试内容

脚本调用后端正式 CLI：

```bash
kvm-console system-compatibility-check \
  --vcpu 1 \
  --ram-gb 1 \
  --disk-gb 1 \
  --report-dir /opt/kvm-console/logs/compatibility
```

测试虚拟机固定使用 1 vCPU、1GB 内存和 1GB QCOW2 系统盘，不挂载 ISO、模板、软盘或直通设备。它通过与面板 ISO 创建一致的 `CreateVMParams → CreateVM` 业务链路生成 XML、定义域并启动，空系统盘启动到固件界面即可完成宿主能力验证。

关键参数如下：

| 项目 | x86_64 | aarch64 |
|------|--------|---------|
| 虚拟化类型 | KVM | KVM |
| 机器类型 | q35 | virt |
| 引导固件 | BIOS | UEFI |
| 系统盘/网卡 | VirtIO | VirtIO |
| 显示设备 | VirtIO | ramfb（与面板 ARM 推荐逻辑一致） |

创建过程继续复用面板的 Guest Agent、RTC、APIC/PAE、SMBIOS、CPU 拓扑、嵌套虚拟化、PCIe 和 VPC XML 注入逻辑。网络使用配置中的系统基础交换机和默认安全组，并复用管理员创建任务的 VPC 绑定逻辑。

测试依次检查：

- `/dev/kvm` 可打开，QEMU、`virt-install`、`qemu-img`、`virsh`、`ip6tables` 和 OVS 相关命令可用；
- libvirt RPC 连接可用；
- OVS 服务、基础网桥、网关、DHCP、IPv4 转发、NAT 和转发规则正常；物理上联口加入 OVS 网桥且默认路由已迁移时，NAT 检查使用该三层网桥作为实际出口；
- 隔离探测网桥支持 OpenFlow13、`pktps+burst` meter，并通过 `ovsdb-client` 读取 OVSDB Interface schema 验证 `ingress_policing_kpkts_rate/burst`；
- 端口安全探测 meter、引用该 meter 的 ARP 流表及 IPv6 ND 防伪造流表可以实际写入并回读；OpenFlow14 bundle 可用时验证原子事务，否则记录隔离顺序更新兼容模式；
- 测试域状态为 `running`；
- 持久化 XML 使用配置中的基础 OVS 网桥，并包含 `virtualport type='openvswitch'`；
- 运行态 `vnet` 端口已加入对应 OVS 网桥，且 `ofport` 有效。

## 清理与失败处理

成功、失败或收到 `SIGINT`/`SIGTERM` 后，后端都会按本次生成的唯一虚拟机名称清理域、NVRAM、系统盘、内存元数据和 VPC 绑定，同时删除唯一命名的端口安全隔离探测网桥、探测 meter、流表和临时文件。清理不会按名称范围扫描，也不会操作其他虚拟机。系统基础交换机、基础 OVS 网桥和默认安全组会保留，供面板正常运行。

测试退出码为 `0` 时表示兼容；其他退出码表示测试阶段或清理阶段存在错误。失败后安装器会显示失败阶段和报告目录，并询问：

```text
兼容性测试未通过，是否仍继续安装面板？[y/N]:
```

- 输入 `y`：继续配置并启动面板，安装完成信息持续显示兼容性警告。
- 直接回车或输入其他内容：撤回本次复制的后端、前端和兼容性脚本，保留依赖、网络地基、配置、数据库和诊断报告；重新运行时仍进入首次安装。

## 报告与权限

默认报告目录：

```text
/opt/kvm-console/logs/compatibility/
```

每次执行使用唯一名称保存以下内容：

- `*-report.json`：阶段、参数、兼容结论、错误和清理结果；
- `*.xml`：持久化 libvirt XML；
- `*-active.xml`：运行态 libvirt XML；
- `*-diagnostics.log`：相关 QEMU/libvirt/OVS 日志；
- `compatibility-run-*.log`：脚本终端输出。

目录权限为 `700`，报告文件权限为 `600`，仅 root 可读。

## 安装后手动重跑

```bash
sudo /opt/kvm-console/scripts/check-system-compatibility.sh
```

如需临时调整资源或报告目录：

```bash
sudo /opt/kvm-console/scripts/check-system-compatibility.sh \
  --vcpu 1 \
  --ram-gb 1 \
  --disk-gb 1 \
  --report-dir /opt/kvm-console/logs/compatibility
```

手动重跑同样只处理本次创建的唯一临时资源，不影响已有虚拟机。
