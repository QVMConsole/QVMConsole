# 虚拟机详情页 IP 与 QEMU Guest Agent

## 页面行为

- 网络与连接中的 IP 展示项命名为「虚拟机IP」。IP 来源为 QEMU Guest Agent 时不再在地址后显示 Guest Agent 标签，其他来源标签保持不变。
- 虚拟机未添加网卡时隐藏「虚拟机IP」行。
- 虚拟机关机或正在关机时隐藏「虚拟机IP」行。
- 已添加网卡但暂未获取到 IP 时显示「详情」按钮，点击后按 QEMU Guest Agent 状态展示原因：
  - 未配置：在「虚拟机IP」空状态区域提示并支持立即启用。
  - 已配置未连接：提示检查来宾服务并提供安装文档链接。
  - 已连接但无 IP：提示可能是上游网关或网络链路异常。
- 「公网 IP」未配置时隐藏整行。

## 立即启用 Guest Agent

在「虚拟机IP」为空且 QEMU Guest Agent 状态为「未配置」时，点击该区域的图标按钮即可复用虚拟机编辑接口写入启用配置。运行中的虚拟机建议重启后使通道配置生效；来宾系统仍需安装并运行 QEMU Guest Agent 服务。

安装与配置说明：

<https://qvmcdocs.xiaozhuhouses.asia/docs/install/category/%E8%BF%9B%E9%98%B6%E5%86%85%E5%AE%B9>

