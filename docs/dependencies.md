# 运行时第三方依赖清单

> 本文档记录面板功能实现中使用到的 apt / 第三方命令行工具（安装均已纳入 `install.sh`）。

| 依赖包 | 命令 | 用途 | 使用位置 |
|--------|------|------|----------|
| dmidecode | `dmidecode -t memory` | 读取宿主机内存条（DIMM）SMBIOS 信息，供概览页内存卡片「硬件详情」展开区展示 | `server/service/host/hardware.go` |

## 说明

- `dmidecode` 已加入 `install.sh` 的 `APT_DEPS`（RPM 系映射同名包）。
- 部分 ARM 设备与虚拟机的 SMBIOS 不提供内存设备数据，此时后端返回中文说明，前端正常降级展示，不影响其他功能。
