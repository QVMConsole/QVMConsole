# UEFI 克隆首次启动连续引导

## 现象

部分 CentOS、RHEL 及其衍生系统的 UEFI 模板没有保存发行版专属的 `Boot####` 启动项。克隆虚拟机首次启动时，`shim/fallback.efi` 会根据 EFI 分区中的 `BOOT*.CSV` 自动登记启动项。默认流程会显示 `Boot Option Restoration` 倒计时页面，并在登记完成后冷复位一次。

该页面不是 OVMF、系统盘或克隆任务故障。未启用连续引导处理时，倒计时结束后仍会自动复位并进入系统，后续启动不再出现。

## 面板处理

QVMConsole 在非 Windows UEFI 模板克隆完成、虚拟机首次启动前，对克隆副本的 QCOW2 NVRAM 预置 shim 官方支持的 `FB_NO_REBOOT=1` 变量：

1. `fallback.efi` 仍会读取 `BOOT*.CSV` 并登记正确的发行版启动项；
2. 登记完成后直接启动第一个新启动项；
3. 不显示恢复倒计时，也不执行额外的冷复位；
4. 模板 NVRAM、系统盘和安全启动证书不会被改写。

NVRAM 更新通过临时文件完成，确认输出仍为 QCOW2 并恢复 libvirt 权限后再原子替换，避免工具执行失败时损坏原文件。

## 依赖与兼容

宿主机需要提供 `virt-fw-vars`，Debian/Ubuntu 对应软件包为 `python3-virt-firmware`。`install.sh` 会自动尝试安装并检查是否支持 `--set-fallback-no-reboot`。

部分 RPM 系发行版的软件源可能没有该工具，或者旧版本不支持该参数。此时克隆任务不会失败，后端会记录中文警告并保留 shim 原有的一次性恢复与复位流程。

## 适用链路

- Linux、FnOS、OpenWrt 和“其它”类型的普通完整/链式模板克隆；
- 非 Windows 类型的原生完整/链式模板克隆；
- BIOS 模板和 Windows 专用克隆链路不写入该变量。
