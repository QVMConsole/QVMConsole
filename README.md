# QVMConsole - 开源虚拟机管理控制台

<div align="center">

<img width="2549" height="1333" alt="image" src="https://github.com/user-attachments/assets/1706a4b4-ac20-45cc-8612-1b2947dc5151" />


[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![GitHub Stars](https://img.shields.io/github/stars/qvmconsole/qvmconsole?style=social)](https://github.com/qvmconsole/qvmconsole)
[![GitHub Forks](https://img.shields.io/github/forks/qvmconsole/qvmconsole?style=social)](https://github.com/qvmconsole/qvmconsole)
[![GitHub Issues](https://img.shields.io/github/issues/qvmconsole/qvmconsole)](https://github.com/qvmconsole/qvmconsole/issues)
[![GitHub Pull Requests](https://img.shields.io/github/issues-pr/qvmconsole/qvmconsole)](https://github.com/qvmconsole/qvmconsole/pulls)

[**官方网站**](https://www.qvmconsole.cn/) | [**文档站点**](https://qvmcdocs.xiaozhuhouses.asia/) | [**部署指南**](https://qvmcdocs.xiaozhuhouses.asia/docs/install/)

</div>

## 项目简介

QVMConsole 是一个面向小型企业和个人私有云服务场景的开源虚拟机管理平台，基于 KVM/QEMU 虚拟化技术深度集成，提供从虚拟机生命周期管理、网络与存储编排、快照与克隆、防火墙与带宽治理，到 Web 控制台与 API 一体化交付的完整解决方案。

### 核心价值

- **降低运维门槛**：提供"即开即用"的虚拟化管理平台，减少重复造轮子的成本
- **模板即点即用**：预制 Linux/Windows/OpenWrt 等常用系统模板，无需了解 KVM 底层命令，只需填几个表单字段即可在数分钟内完成虚拟机创建；系统自动处理磁盘格式、引导类型、网络配置等复杂细节
- **模块化设计**：可插拔网络后端（如 Open vSwitch），适配多样化的网络拓扑与安全策略
- **双入口架构**：Web 控制台与 RESTful API 兼顾自动化与人工运维效率
- **可观测性**：任务队列与 SSE 机制实现长耗时操作的可观测与可中断，保障大规模并发下的稳定性

## 核心功能

### 虚拟机生命周期管理
- 完整的电源操作（开机/关机/重启/强制断电/重置）
- 配额控制与权限校验
- 维护模式与优雅关机
- 动态内存配置与调整

### 网络虚拟化
- VPC 逻辑交换机与安全组
- 端口转发与静态 IP 管理
- 防火墙策略（VM/宿主机双层）
- 网络诊断与抓包工具

### 存储管理
- 宿主机存储池管理（格式化/分区/LVM 卷）
- 模板管理（制作/导入/导出/删除）
- 磁盘管理与 IOPS 限制
- 用户 ISO 挂载

### 用户权限与配额
- 多租户支持（弹性云/轻量云）
- 细粒度配额管理（CPU/内存/磁盘/VM 数/存储/带宽/流量/公网 IP/端口转发/快照）
- SSH 访问控制与邀请注册流程

### 监控与任务调度
- VM/宿主机统计与历史数据
- 异步任务队列与 SSE 实时推送
- 定时事件中心与资源回收

### 快照备份
- 创建/恢复/删除/批量删除快照
- NVRAM 与共享目录兼容性检查
- 配额校验与任务跟踪

### 模板创建虚拟机
- **模板管理**：支持从运行中虚拟机一键制作模板、导入/导出模板包（tar.gz）、预览导入完整性校验
- **多类型模板支持**：Linux（cloud-init）、Windows（ConfigDrive）、OpenWrt（UCI 配置注入）、FnOS（virt-customize）及"不初始化"模式
- **统一克隆架构**：支持完整克隆与链式克隆两种模式，完整克隆产生独立磁盘镜像，链式克隆基于 backing chain 实现快速部署
- **系统初始化控制**：可禁用系统初始化，保持模板原始系统配置；支持阻塞式/非阻塞式启动后命令执行
- **智能引导检测**：自动检测 UEFI/BIOS 引导类型，复制 NVRAM 路径，确保跨架构兼容性
- **OpenWrt 双模式初始化**：自动检测 ext4 根分区和 squashfs+overlay 两种磁盘布局，智能选择 virt-customize 或 guestfish 注入网络配置
- **Windows ConfigDrive**：符合 OpenStack 标准的 ISO 镜像，通过 cloudbase-init 自动完成主机名、密码等初始化配置
- **元数据驱动**：模板类型、分类、默认硬件配置、哈希校验、模板族关系等均由 `.meta.json` 元数据文件管理
- **版本与完整性校验**：MD5 + SHA256 双重哈希校验，确保模板磁盘完整性
- **模板族管理**：支持模板父子关系、节点树、级联删除、静默提升与热提升操作

## 技术栈

### 后端
- **语言**: Go 1.26.0
- **Web 框架**: Gin v1.12.0
- **数据库**: SQLite + GORM v1.31.1
- **虚拟化**: go-libvirt RPC
- **认证**: JWT v5.3.1 + TOTP v1.5.0 + crypto
- **WebSocket**: gorilla/websocket v1.5.3
- **日志**: lumberjack v2.2.1

### 前端
- **UI 框架**: React v19.2.7 + TypeScript v6.0.2
- **组件库**: Semi Design v2.101.1（@douyinfe/semi-ui）
- **构建工具**: Vite v8.1.1
- **路由**: react-router-dom v7.18.1
- **状态管理**: Zustand v5.0.14
- **HTTP 客户端**: Axios v1.18.1
- **图表**: ECharts v6.1.0
- **终端**: @xterm/xterm v6.0.0
- **VNC**: @novnc/novnc v1.7.0
- **旧版 (备份)**: Vue 3.5.30 + Element Plus（位于 `web-backup/`）

### 虚拟化基础设施
- **虚拟化平台**: KVM/QEMU
- **网络虚拟化**: Open vSwitch
- **Windows 初始化**: ConfigDrive 标准支持

## 系统要求

### 硬件要求
- 支持 VT-x/AMD-V 的 CPU
- 至少 4GB RAM（推荐 8GB+）
- 至少 50GB 可用磁盘空间

### 软件要求
- **操作系统**: Debian/Ubuntu（推荐 Debian 12+）
- **虚拟化**: KVM/QEMU
- **网络**: Open vSwitch
- **依赖工具**: genisoimage（用于 Windows 虚拟机初始化）

### 开发贡献指南
作为一个由独立开发者维护的大型开源项目，QVMConsole 需要社区贡献者的支持才能持续完善。我们欢迎并鼓励您使用 AI 等工具进行功能修复与开发，但请务必遵守以下准则：

1. **规则遵守**：在使用 AI 工具时，必须将根目录的 `AGENTS.md` 文件作为核心提示词规则
2. **功能边界**：开源版本中不得提交包含 Pro 版功能的代码。Pro 版功能清单详见：[赞助功能说明](https://qvmcdocs.xiaozhuhouses.asia/docs/install/sponsorship)
3. **场景通用性**：提交的功能应面向通用化使用场景，符合广大用户的需求。针对特定场景的定制功能建议自行 fork 仓库维护

### 安全漏洞报告
如果您发现项目存在安全漏洞，无论严重程度如何，请勿在 GitHub Issues 中公开报告，以避免安全风险被恶意利用。

**安全报告渠道**：
- 作者QQ：3354416548
- 电子邮件：xiaozhuhs@foxmail.com

---

## 合并上游修改

当标准仓库后端有修改时，请参阅 [`docs/merge-from-upstream.md`](docs/merge-from-upstream.md) 获取详细合并指南。

核心原则：
1. 只合并 `server/` 目录的后端修改
2. 拒绝合并 `web/` 目录的任何前端修改
3. 本仓库的 `web-backup/`、`.gitignore`、`docs/` 中的独有内容不会被上游覆盖

## 致谢

感谢所有为 QVMConsole 做出贡献的开发者！

---

<div align="center">

**QVMConsole** - 让虚拟化管理更简单

[官方网站](https://www.qvmconsole.cn/) | [文档站点](https://qvmcdocs.xiaozhuhouses.asia/) | [部署指南](https://qvmcdocs.xiaozhuhouses.asia/docs/install/)

</div>
