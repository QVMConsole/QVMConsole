package service

import (
	"context"

	portmirror "kvm_console/service/network/portmirror"
)

type PortMirrorConfig = portmirror.Config
type PortMirrorEnableRequest = portmirror.EnableRequest
type PortMirrorTaskParams = portmirror.TaskParams
type PortMirrorOptions = portmirror.Options
type PortMirrorStatus = portmirror.Status

func GetPortMirrorOptions() (*PortMirrorOptions, error) { return portmirror.ListOptions() }
func GetPortMirrorStatus() (*PortMirrorStatus, error)   { return portmirror.GetStatus() }
func PreflightPortMirror(req PortMirrorEnableRequest) (*PortMirrorConfig, error) {
	return portmirror.Preflight(req)
}
func ExecutePortMirrorTask(ctx context.Context, params PortMirrorTaskParams, progress func(int, string)) (string, error) {
	return portmirror.ExecuteTask(ctx, params, progress)
}
func RestorePortMirror() error                 { return portmirror.Restore() }
func RunPortMirrorWatchdog(token string) error { return portmirror.RunWatchdog(token) }
