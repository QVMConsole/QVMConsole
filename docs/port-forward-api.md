# 端口转发 API 标识

端口转发规则的 API 层 `id` 使用稳定组合键：`{protocol}|{host_port}`。例如 TCP 宿主机 10022 端口的标识为 `tcp|10022`。

列表接口 `GET /network/port-forward/list` 返回该字符串 `id`。以下接口均接收该标识：

- `PUT /network/port-forward/:id`
- `DELETE /network/port-forward/:id`
- `POST /network/port-forward/batch-delete`，请求体为 `{ "ids": ["tcp|10022", "udp|10022"] }`

客户端拼接路径时必须对 `id` 使用 URL 编码。后端每次操作前均重新读取实时规则，并以协议、宿主机端口、目标地址、目标端口和来源 IP 的完整参数删除 NAT 规则；不会再按 iptables 行号删除。因此规则更新或批量删除造成行号变化时，不会误操作其他端口转发规则。

`rule_key` 与 `firewall_key` 仍保留给防火墙区域限制关联使用，不应用于更新或删除端口转发规则。
