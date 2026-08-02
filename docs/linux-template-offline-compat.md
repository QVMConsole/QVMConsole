# Linux 导入模板离线兼容

## 适用范围

适用于历史导入或新导入的 Linux 模板。克隆阶段不会访问软件源，所有 `cloud-init` 与磁盘扩容依赖均在模板预处理阶段完成。

## 历史模板处理

在“模板管理”中，对 Linux 模板点击“离线预处理”；任务会：

1. 先检查当前模板节点及其派生节点关联的链式克隆 VM；只要仍存在链式依赖，任务不会提交或执行；
2. 管理员需先关机，再前往“虚拟机管理”，在每台关联 VM 的“更多”菜单中执行“转为独立虚拟机”，并等待全部转换任务完成；
3. 临时解除模板磁盘的不可变标记；
4. 先以无网络方式检查 `cloud-init` 和对应的磁盘扩容工具；仅在缺失时才启动 guestfs 网络后端并安装；
5. 写入 `linux_init_status`、`linux_init_checked` 与错误摘要到模板元数据；
6. 恢复模板磁盘不可变标记。

检查接口：`GET /api/template/{name}/prepare-linux/check`，返回 `linked_vms` 与 `can_prepare`。预处理接口：`POST /api/template/{name}/prepare-linux`。管理员会话和管理员 API Key 均可调用；响应返回任务 ID，执行进度通过任务中心查询。链式依赖尚未转换时，预处理接口返回 `409` 与关联 VM 列表。

## 克隆网络兼容

Linux 模板克隆时会预先生成主网卡 MAC，并将该 MAC 同时用于 libvirt XML 与克隆副本内的 Netplan 配置。若模板 Netplan 使用固定 `match.macaddress`，仅替换首个主网卡 MAC，保留已有 DHCP、静态地址、网关和 DNS 配置。若创建时不带主网口且模板为固定 MAC 的 DHCP 配置，则克隆副本会改为匹配 `en*` 的 DHCP 配置，以支持之后从面板添加网口；静态网络模板保持原样。模板本体不被修改。

## 运行约束

克隆阶段只运行一次 `virt-customize --no-network`，不执行 `apt`、`dnf` 或 `yum`。依赖预处理失败的模板仍保留在列表中，并显示“预处理失败”；修复软件源后可再次提交预处理任务。
