package arch

import "os"

// ==================== aarch64 Profile ====================

type aarch64Profile struct{}

func (p *aarch64Profile) Arch() string                    { return ArchAarch64 }
func (p *aarch64Profile) DisplayName() string             { return "aarch64 (ARM64)" }
func (p *aarch64Profile) EmulatorPath() string            { return "/usr/bin/qemu-system-aarch64" }
func (p *aarch64Profile) DefaultMachineType() string      { return "virt" }
func (p *aarch64Profile) SupportedMachineTypes() []string { return []string{"virt"} }
func (p *aarch64Profile) DefaultBootType() string         { return "uefi" }
func (p *aarch64Profile) SupportedBootTypes() []string    { return []string{"uefi"} }
func (p *aarch64Profile) DefaultCPUMode() string          { return "host-passthrough" }
func (p *aarch64Profile) SupportedDiskBus() []string      { return []string{"virtio", "scsi"} }
func (p *aarch64Profile) GetCDROMBus() string             { return "usb" }
func (p *aarch64Profile) SupportedNicModels() []string    { return []string{"virtio"} }
func (p *aarch64Profile) SupportsBIOS() bool              { return false }
func (p *aarch64Profile) SupportsSecureBoot() bool        { return false }
func (p *aarch64Profile) SupportsPAE() bool               { return false }
func (p *aarch64Profile) SupportsAPIC() bool              { return false }
func (p *aarch64Profile) DefaultWatchdogModel() string    { return "diag288" }

func (p *aarch64Profile) DefaultCPUModel(virtType string) string {
	if virtType == "qemu" {
		return "cortex-a72"
	}
	return ""
}

func (p *aarch64Profile) UEFIFirmwarePath(secureBoot bool) string {
	// ARM 暂不支持安全引导，忽略 secureBoot 参数
	candidates := []string{
		"/usr/share/AAVMF/AAVMF_CODE.fd",
		"/usr/share/AAVMF/AAVMF_CODE.no-secboot.fd",
		"/usr/share/qemu-efi-aarch64/QEMU_EFI.fd",
	}
	return pickFirstExistingPath(candidates, candidates[0])
}

// UEFILegacyFirmwarePath 返回兼容旧版固件路径（EDK2 2024.02）。
// 用于解决某些 OS（如统信 UOS）的 EFI 引导器与新版 EDK2 不兼容的问题。
func (p *aarch64Profile) UEFILegacyFirmwarePath() string {
	candidates := []string{
		"/opt/kvm-console/firmware/AAVMF_CODE_legacy.fd",
		"/opt/project/QVMConsole/firmware/AAVMF_CODE_legacy.fd",
		"/usr/share/AAVMF/AAVMF_CODE_2024.fd",
	}
	return pickFirstExistingPath(candidates, candidates[0])
}

// UEFILegacyVarsTemplatePath 返回兼容旧版 NVRAM 模板路径。
func (p *aarch64Profile) UEFILegacyVarsTemplatePath() string {
	candidates := []string{
		"/opt/kvm-console/firmware/AAVMF_VARS_legacy.fd",
		"/opt/project/QVMConsole/firmware/AAVMF_VARS_legacy.fd",
		"/usr/share/AAVMF/AAVMF_VARS_2024.fd",
	}
	return pickFirstExistingPath(candidates, candidates[0])
}

func (p *aarch64Profile) UEFIVarsTemplatePath(secureBoot bool) string {
	_ = secureBoot
	candidates := []string{
		"/usr/share/AAVMF/AAVMF_VARS.fd",
		"/usr/share/qemu-efi-aarch64/vars-template-pflash.raw",
	}
	return pickFirstExistingPath(candidates, candidates[0])
}

func pickFirstExistingPath(candidates []string, fallback string) string {
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return fallback
}

func init() {
	RegisterProfile(&aarch64Profile{})
}
