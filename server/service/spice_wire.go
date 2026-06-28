package service

import (
	netpkg "kvm_console/service/network"
	spicepkg "kvm_console/service/spice"
	vmpkg "kvm_console/service/vm"
)

// init wires spice package hook variables to service root implementations.
// 与 vnc_wire.go 同构：spice 包不能反向 import service 根包，故通过 hook 注入。
func init() {
	spicepkg.HookStartVM = StartVM
	spicepkg.HookDetectVMOSType = vmpkg.DetectVMOSType
	spicepkg.HookGetHostIP = netpkg.GetHostIP
	spicepkg.HookManageUFWRule = func(action, rule string) error {
		return netpkg.ManageUFWRule(action, rule)
	}
}

// ── Type aliases ──

type SpiceInfo = spicepkg.SpiceInfo
type SpiceConnInfo = spicepkg.SpiceConnInfo

// ── Exported delegates ──

func GetSpiceStatus(vmName string) (*SpiceInfo, error) {
	return spicepkg.GetSpiceStatus(vmName)
}

func EnableSpice(vmName, password string) error {
	return spicepkg.EnableSpice(vmName, password)
}

func DisableSpice(vmName string) error {
	return spicepkg.DisableSpice(vmName)
}

func ChangeSpicePassword(vmName, newPassword string) error {
	return spicepkg.ChangeSpicePassword(vmName, newPassword)
}

func ExposeSpice(vmName string, expose bool) error {
	return spicepkg.ExposeSpice(vmName, expose)
}

func GetSpiceConnInfo(vmName string) (*SpiceConnInfo, error) {
	return spicepkg.GetSpiceConnInfo(vmName)
}

func BuildSpiceVVFile(info *SpiceConnInfo, vmName string, deleteThisFile bool) string {
	return spicepkg.BuildVVFile(info, vmName, deleteThisFile)
}

// InjectSPICEGraphicsToDomainXML / EnsureQXLVideo 暴露给创建/克隆/导入链路调用。
func InjectSPICEGraphicsToDomainXML(xmlStr, passwd, listenAddr string) string {
	return spicepkg.InjectSPICEGraphicsToDomainXML(xmlStr, passwd, listenAddr)
}

func EnsureQXLVideo(xmlStr string) string {
	return spicepkg.EnsureQXLVideo(xmlStr)
}
