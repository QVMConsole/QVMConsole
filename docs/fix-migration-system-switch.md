# 修复：跨节点迁移无法选择目标系统基础网络交换机 + 文案改名

## 问题现象

1. 迁移虚拟机弹窗中「目标 VPC」下拉列表为空（或缺少目标节点上的**系统基础网络交换机**）；
2. 目标节点上管理员新建的交换机（DHCP/NAT）可以正常选择，但系统基础网络交换机始终不在可选列表中；
3. 用户界面文案「目标 VPC」与实际产品语义（交换机）不一致。

## 根因

迁移选项接口（`GET /nodes/:id/migration-options`）处理目标交换机列表时，使用

`server/service/vm/migration/helpers.go` 的 `filterTargetMigrationNetworks`：

```go
if owner != "" && sw.Username != owner {
    continue
}
```

而**系统基础网络交换机**（`vpc_switches` 表中 `is_system = 1` 的记录）的 `username` 字段为空字符串（`""`）：

- 由 `EnsureSystemBaseNetwork()` 创建，`Username: ""`、`IsSystem: true`、`BridgeMode: nat`、`DHCPEnabled: true`；
- 它属于全局共享资源，**仅管理员可使用**（创建 VM 链路 `ResolveVPCForVMCreate` 中明确“系统基础网络交换机仅管理员可选”）。

因此迁移时，管理员（owner=admin）也会因为 `"" != "admin"` 被过滤掉，系统基础交换机从下拉中消失，
导致列表为空或只能看到管理员/用户自建的交换机。

## 修复内容

### 后端

| 文件 | 修改 |
| --- | --- |
| `server/service/vm/migration/helpers.go` | ① `filterTargetMigrationNetworks`：系统基础交换机（`IsSystem`）仅管理员可见可选，不再按 `username` 过滤；普通用户仍不可选择系统基础网络，语义与创建 VM 一致。② `validateTargetNetwork` 文案「目标 VPC」→「目标交换机」 |
| `server/service/vm/migration/preview.go` | `matchTargetSwitch`：源 VM 绑定的交换机为系统基础网络时，按名称 + CIDR 匹配目标节点的系统基础网络交换机（此前按 `username` 匹配恒失败） |
| `server/service/vm/migration/adopt.go` | 错误文案「绑定目标 VPC 失败」→「绑定目标交换机失败」 |

### 前端

| 文件 | 修改 |
| --- | --- |
| `web/src/views/vm/dialogs/VmMigrationDialog.tsx` | ① 标签「目标 VPC」→「目标交换机」，placeholder「请选择目标 VPC」→「请选择目标交换机」；② 下拉选项新增 `switchLabel` 展示函数：系统基础交换机的 `username` 为空时显示为「系统」 |
| `web/src/views/api-docs/endpointDescriptions.ts` | 接口说明文案「该用户下的 VPC/安全组」→「该用户下的交换机/安全组」 |

> 「轻量云 VPC」为独立产品语义，保留原名不变。

## 验证方法

1. 管理员登录，配置一个目标节点（该节点需保持系统基础网络交换机存在）；
2. 对任一台由管理员创建的虚拟机打开「迁移虚拟机」弹窗，选择目标节点后，目标交换机下拉应能列出：
   - 系统基础网络（显示为「系统 / 基础网络 / 网段」）；
   - 管理员自建的交换机。
3. 对普通租户虚拟机迁移时，系统基础网络不应对该租户出现（与创建 VM 行为一致）。
4. 源 VM 绑定系统基础网络时，预检后应能自动匹配目标节点同名、同网段的系统基础网络交换机。