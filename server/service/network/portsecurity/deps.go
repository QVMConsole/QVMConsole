package portsecurity

// HookGetVMMACByOrder 根据逻辑网卡序号读取虚拟机的稳定 MAC 地址。
// 由 service 根包注入，避免 network/portsecurity 反向依赖 vm 包。
var HookGetVMMACByOrder func(vmName string, order int) string
