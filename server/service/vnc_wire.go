package service

import (
	vmpkg "kvm_console/service/vm"
	vncpkg "kvm_console/service/vnc"
)

// init wires vnc package hook variables to service root implementations.
// This breaks the circular dependency: vnc package cannot import service,
// so it exposes hook variables that we set here.
func init() {
	vncpkg.HookStartVM = StartVM
	vncpkg.HookDetectVMOSType = vmpkg.DetectVMOSType
}

// ── Type aliases（向后兼容，让 service 根包和外部调用方可直接使用类型名） ──

type VncInfo = vncpkg.VncInfo
type VncConnInfo = vncpkg.VncConnInfo

// ── Exported delegates ──

func GetVncStatus(vmName string) (*VncInfo, error) {
	return vncpkg.GetVncStatus(vmName)
}

func EnableVnc(vmName, password string) error {
	return vncpkg.EnableVnc(vmName, password)
}

func DisableVnc(vmName string) error {
	return vncpkg.DisableVnc(vmName)
}

func ChangeVncPassword(vmName, newPassword string) error {
	return vncpkg.ChangeVncPassword(vmName, newPassword)
}

func GetVncConnInfo(vmName string) (*VncConnInfo, error) {
	return vncpkg.GetVncConnInfo(vmName)
}

func ExposeVnc(vmName string, expose bool) error {
	return vncpkg.ExposeVnc(vmName, expose)
}
