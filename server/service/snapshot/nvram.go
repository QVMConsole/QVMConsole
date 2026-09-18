package snapshot

import (
	"fmt"
	"strings"
	"time"

	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

func ensureInternalSnapshotNVRAMCompatible(vmName string, isRunning bool) error {
	xmlContent, err := HookGetVMInactiveDomainXML(vmName)
	if err != nil {
		return err
	}
	if !vm_xml.DomainUsesPflashNVRAM(xmlContent) {
		return nil
	}
	nvramPath := HookExtractDomainNVRAMPath(xmlContent)
	if nvramPath == "" {
		return nil
	}
	xmlFormat := vm_xml.ExtractDomainNVRAMFormat(xmlContent)
	fileFormat := vm_xml.DetectQemuImageFormat(nvramPath)
	if xmlFormat == "qcow2" && (fileFormat == "" || fileFormat == "qcow2") {
		return nil
	}
	if fileFormat == "qcow2" {
		if isRunning {
			return fmt.Errorf("当前虚拟机的 UEFI NVRAM 文件已是 qcow2，但虚拟机配置仍未声明 qcow2。请先正常关机后重新创建内存快照，系统会自动修正配置；修正完成后再开机创建内存快照即可")
		}
		updatedXML := vm_xml.SetDomainNVRAMFormat(xmlContent, "qcow2")
		if updatedXML != xmlContent {
			return HookSetVMInactiveDomainXML(vmName, updatedXML)
		}
		return nil
	}
	if isRunning {
		return fmt.Errorf("当前虚拟机使用 UEFI pflash 且 NVRAM 仍是 raw 格式，libvirt 不支持为这种配置创建内部内存快照。请先正常关机后重新创建内存快照，系统会自动将 NVRAM 转换为 qcow2；转换完成后再开机创建内存快照即可")
	}
	if err := vm_xml.ConvertExistingNVRAMToQCOW2(nvramPath); err != nil {
		return fmt.Errorf("转换 UEFI NVRAM 为 qcow2 失败: %w", err)
	}
	updatedXML := vm_xml.SetDomainNVRAMFormat(xmlContent, "qcow2")
	if updatedXML == xmlContent {
		return nil
	}
	if err := HookSetVMInactiveDomainXML(vmName, updatedXML); err != nil {
		return fmt.Errorf("更新虚拟机 NVRAM 格式配置失败: %w", err)
	}
	return nil
}

// CheckInternalSnapshotNVRAMRepairRequired 检查运行中内存快照是否需要先修复 UEFI NVRAM。
func CheckInternalSnapshotNVRAMRepairRequired(vmName string) (bool, string, error) {
	stateResult := utils.ExecCommand("virsh", "domstate", vmName)
	if stateResult.Error != nil {
		return false, "", fmt.Errorf("获取虚拟机状态失败: %s", stateResult.Stderr)
	}
	if strings.TrimSpace(stateResult.Stdout) != "running" {
		return false, "", nil
	}

	xmlContent, err := HookGetVMInactiveDomainXML(vmName)
	if err != nil {
		return false, "", err
	}
	if !vm_xml.DomainUsesPflashNVRAM(xmlContent) {
		return false, "", nil
	}
	nvramPath := HookExtractDomainNVRAMPath(xmlContent)
	if nvramPath == "" {
		return false, "", nil
	}
	xmlFormat := vm_xml.ExtractDomainNVRAMFormat(xmlContent)
	fileFormat := vm_xml.DetectQemuImageFormat(nvramPath)
	if xmlFormat == "qcow2" && (fileFormat == "" || fileFormat == "qcow2") {
		return false, "", nil
	}

	message := "当前虚拟机使用 UEFI pflash，NVRAM 不是快照兼容的 qcow2 格式。若继续，系统将正常关机，转换 NVRAM，重新开机，然后继续创建内存快照。关机过程会中断当前业务，确定要立即修复吗？"
	return true, message, nil
}

func autoRepairRunningVMNVRAMForInternalSnapshot(vmName string, progressFn func(int, string)) error {
	if progressFn != nil {
		progressFn(20, "检测到 UEFI NVRAM 需要修复，正在正常关机...")
	}
	if err := HookShutdownVM(vmName); err != nil {
		return err
	}
	if err := waitVMShutoff(vmName, 180*time.Second); err != nil {
		return err
	}

	if progressFn != nil {
		progressFn(40, "虚拟机已关机，正在转换 UEFI NVRAM 为 qcow2...")
	}
	if err := ensureInternalSnapshotNVRAMCompatible(vmName, false); err != nil {
		return err
	}

	if progressFn != nil {
		progressFn(50, "NVRAM 修复完成，正在重新开机...")
	}
	if err := HookStartVM(vmName); err != nil {
		return fmt.Errorf("NVRAM 修复完成，但重新开机失败: %w", err)
	}
	if err := waitVMRunning(vmName, 120*time.Second); err != nil {
		return err
	}
	return nil
}

func waitVMShutoff(vmName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stateResult := utils.ExecCommand("virsh", "domstate", vmName)
		if stateResult.Error != nil {
			return fmt.Errorf("获取虚拟机状态失败: %s", stateResult.Stderr)
		}
		state := strings.ToLower(strings.TrimSpace(stateResult.Stdout))
		if state == "shut off" || state == "shutoff" {
			HookUpdateVMRuntimeState(vmName, "shut off", time.Now())
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("已发送正常关机指令，但虚拟机在 180 秒内未关机。为避免强制断电造成数据丢失，请先在系统内关机后再重试")
}

func waitVMRunning(vmName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stateResult := utils.ExecCommand("virsh", "domstate", vmName)
		if stateResult.Error != nil {
			return fmt.Errorf("获取虚拟机状态失败: %s", stateResult.Stderr)
		}
		if strings.TrimSpace(stateResult.Stdout) == "running" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("虚拟机已执行开机，但在 120 秒内未进入 running 状态，请检查虚拟机状态后重试")
}
