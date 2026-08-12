package public_ip

import "kvm_console/model"

const (
	PublicIPModeNAT           = "nat"
	PublicIPModeClassicRoute  = "classic_route"
	PublicIPModeClassicBridge = "classic_bridge"

	PublicIPStatusFree  = "free"
	PublicIPStatusBound = "bound"

	publicIPConfigDir   = "/etc/kvm-console/public-ip"
	publicIPRulesPath   = "/etc/kvm-console/public-ip/rules.sh"
	publicIPRuleComment = "kvm-console:public-ip"
	publicIPFlowPrefix  = "0x9a"
	publicIPFlowMask    = "0xff00000000000000"
)

type PublicIPRequest struct {
	IP             string `json:"ip"`
	CIDR           string `json:"cidr"`
	Gateway        string `json:"gateway"`
	UplinkIF       string `json:"uplink_if"`
	SupportedModes string `json:"supported_modes"`
	Status         string `json:"status"`
	Remark         string `json:"remark"`
}

// PublicIPBatchRequest 批量新增公网 IP 请求。
// 除 IPs 外的其他字段（CIDR、网关、出口网卡、支持模式、状态、备注）对整批 IP 共用，
// 每条 IP 仍会按地址族独立校验（IPv6 会自动剔除 NAT 模式）。
type PublicIPBatchRequest struct {
	IPs            []string `json:"ips"`
	CIDR           string   `json:"cidr"`
	Gateway        string   `json:"gateway"`
	UplinkIF       string   `json:"uplink_if"`
	SupportedModes string   `json:"supported_modes"`
	Status         string   `json:"status"`
	Remark         string   `json:"remark"`
}

// PublicIPBatchItemStatus 批量新增中单条 IP 的处理状态。
const (
	PublicIPBatchItemCreated = "created" // 新建成功
	PublicIPBatchItemSkipped = "skipped" // 跳过（批内重复或数据库已存在）
	PublicIPBatchItemFailed  = "failed"  // 校验或创建失败
)

// PublicIPBatchItemResult 批量新增中每条 IP 的处理结果。
type PublicIPBatchItemResult struct {
	IP     string           `json:"ip"`
	Status string           `json:"status"`
	Reason string           `json:"reason,omitempty"`
	Row    *model.PublicIP `json:"row,omitempty"`
}

// PublicIPBatchResult 批量新增公网 IP 的汇总结果。
type PublicIPBatchResult struct {
	Created int                       `json:"created"`
	Skipped int                       `json:"skipped"`
	Failed  int                       `json:"failed"`
	Items   []PublicIPBatchItemResult `json:"items"`
}

// PublicIPv6PrefixInfo 描述从宿主机上联网卡发现的公网 IPv6 前缀。
type PublicIPv6PrefixInfo struct {
	UplinkIF string `json:"uplink_if"`
	Address  string `json:"address"`
	Prefix   string `json:"prefix"`
	Gateway  string `json:"gateway,omitempty"`
}

// PublicIPv6PrefixImportRequest 将动态发现的 IPv6 前缀展开为可独立绑定的 /128 资源。
type PublicIPv6PrefixImportRequest struct {
	UplinkIF string `json:"uplink_if"`
	Prefix   string `json:"prefix"`
	Count    int    `json:"count"`
	Remark   string `json:"remark"`
}

type PublicIPv6PrefixImportResult struct {
	Prefix  string           `json:"prefix"`
	Created []model.PublicIP `json:"created"`
	Skipped int              `json:"skipped"`
}

type PublicIPBindRequest struct {
	Username    string `json:"username"`
	VMName      string `json:"vm_name"`
	VMPrivateIP string `json:"vm_private_ip"`
	Mode        string `json:"mode"`
}

type PublicIPOperationParams struct {
	Action      string              `json:"action"`
	PublicIPID  uint                `json:"public_ip_id"`
	TargetVM    string              `json:"target_vm"`
	TargetUser  string              `json:"target_user"`
	BindRequest PublicIPBindRequest `json:"bind_request"`
	// BatchItems 用于批量绑定/解绑操作；batch_bind 时每项需携带 BindRequest，
	// batch_unbind 时只需 PublicIPID。单条操作不使用此字段。
	BatchItems []PublicIPBatchOpItem `json:"batch_items,omitempty"`
}

// PublicIPBatchOpItem 批量操作中的单条参数。
// 批量绑定：PublicIPID + BindRequest 都必填。
// 批量解绑：仅 PublicIPID 必填。
type PublicIPBatchOpItem struct {
	PublicIPID  uint                `json:"public_ip_id"`
	BindRequest PublicIPBindRequest `json:"bind_request,omitempty"`
}

// PublicIPBatchOpRequest 批量绑定/解绑请求体（前端 → handler）。
// IDs 用于批量解绑；Items 用于批量绑定（含每条 IP 的绑定参数）。
type PublicIPBatchOpRequest struct {
	IDs   []uint                 `json:"ids,omitempty"`
	Items []PublicIPBatchOpItem  `json:"items,omitempty"`
}

// PublicIPBatchOpResult 批量绑定/解绑单条处理结果。
type PublicIPBatchOpResult struct {
	ID     uint   `json:"id"`
	IP     string `json:"ip"`
	Status string `json:"status"` // success / failed / skipped
	Reason string `json:"reason,omitempty"`
}

// PublicIPBatchOpSummary 批量操作汇总。
type PublicIPBatchOpSummary struct {
	Success int                      `json:"success"`
	Failed  int                      `json:"failed"`
	Skipped int                      `json:"skipped"`
	Items   []PublicIPBatchOpResult  `json:"items"`
}

type PublicIPInfo struct {
	model.PublicIP
	AddressFamily string                 `json:"address_family"`
	Modes         []string               `json:"modes"`
	ModeLabels    []string               `json:"mode_labels"`
	Binding       *model.PublicIPBinding `json:"binding,omitempty"`
	RuntimeRules  []string               `json:"runtime_rules,omitempty"`
	Issues        []string               `json:"issues,omitempty"`
}

type PublicIPPreview struct {
	PublicIP   model.PublicIP      `json:"public_ip"`
	Binding    PublicIPBindRequest `json:"binding"`
	Commands   []string            `json:"commands"`
	ConfigHint string              `json:"config_hint"`
	Warnings   []string            `json:"warnings"`
}

type PublicIPAttachment struct {
	PublicIP      string `json:"public_ip"`
	Mode          string `json:"mode"`
	ModeLabel     string `json:"mode_label"`
	VMPrivateIP   string `json:"vm_private_ip"`
	RuntimeStatus string `json:"runtime_status"`
}
