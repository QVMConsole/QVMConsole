package clone

import (
	"fmt"

	vmpkg "kvm_console/service/vm"
)

// applyCloneHostDevicesToDomainXML 为克隆链路统一处理 VFIO 绑定、hostdev 注入，
// 以及 video_model=none 时唯一直通 VGA 的 x-vga 联动。
func applyCloneHostDevicesToDomainXML(xmlContent string, hostDevices []vmpkg.HostDeviceParam, isAdmin bool) (string, error) {
	if len(hostDevices) == 0 {
		return xmlContent, nil
	}
	if !isAdmin {
		return "", fmt.Errorf("仅管理员可配置硬件直通设备")
	}
	if err := vmpkg.EnsureVfioModuleLoaded(); err != nil {
		return "", fmt.Errorf("加载 vfio-pci 模块失败: %w", err)
	}
	for _, device := range hostDevices {
		if err := vmpkg.ValidatePCIPassthrough(device.PCIAddress, device.UnsafeConfirmed); err != nil {
			return "", fmt.Errorf("设备 %s 直通验证失败: %w", device.PCIAddress, err)
		}
		if !vmpkg.IsDeviceVfioBound(device.PCIAddress) {
			if err := vmpkg.BindPCIDeviceToVfio(device.PCIAddress, device.UnsafeConfirmed); err != nil {
				return "", fmt.Errorf("绑定设备 %s 到 vfio-pci 失败: %w", device.PCIAddress, err)
			}
		}
	}
	updated, err := vmpkg.ApplyHostDevsToDomainXML(xmlContent, hostDevices)
	if err != nil {
		return "", fmt.Errorf("应用硬件直通设备失败: %w", err)
	}
	return updated, nil
}
