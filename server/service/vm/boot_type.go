package vm

import (
	"fmt"
	"strings"

	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

// SetVMBootType 修改虚拟机引导方式。该操作要求虚拟机关机后执行。
func SetVMBootType(name, bootType string) error {
	normalized := vm_xml.NormalizeVMBootType(bootType)
	if normalized == "" {
		return fmt.Errorf("不支持的引导方式: %s", bootType)
	}

	stateResult := utils.ExecCommand("virsh", "domstate", name)
	if stateResult.Error != nil {
		return fmt.Errorf("获取虚拟机状态失败: %s", stateResult.Stderr)
	}
	state := strings.TrimSpace(stateResult.Stdout)
	if state == "running" || state == "paused" {
		return fmt.Errorf("请先关机后再修改引导方式")
	}

	xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	currentBootType := vm_xml.ParseVMBootTypeFromDomainXML(xmlResult.Stdout)
	if currentBootType == normalized {
		return nil
	}
	if err := vm_xml.EnsureVMUEFINVRAMFile(name, xmlResult.Stdout, normalized); err != nil {
		return err
	}

	updatedXML, err := vm_xml.ApplyVMBootTypeToDomainXML(name, xmlResult.Stdout, normalized)
	if err != nil {
		return err
	}
	if err := SetVMInactiveDomainXML(name, updatedXML); err != nil {
		return fmt.Errorf("设置引导方式失败: %w", err)
	}
	return nil
}

// SetVMFirmwareCompat 设置虚拟机 UEFI 固件兼容模式（ARM 专用）。
func SetVMFirmwareCompat(name string, enabled bool) error {
	xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	current := vm_xml.DetectFirmwareCompatFromDomainXML(xmlResult.Stdout)
	if current == enabled {
		return nil // 无需修改
	}

	var updatedXML string
	var err error
	if enabled {
		// 启用兼容模式：替换为旧版固件
		e := true
		updatedXML, err = vm_xml.ApplyFirmwareCompatToDomainXML(name, xmlResult.Stdout, &e)
	} else {
		// 关闭兼容模式：重新应用默认 UEFI 固件
		updatedXML, err = vm_xml.ApplyVMBootTypeToDomainXML(name, xmlResult.Stdout, "uefi")
	}
	if err != nil {
		return err
	}

	if err := vm_xml.EnsureVMUEFINVRAMFile(name, updatedXML, "uefi"); err != nil {
		return err
	}
	return SetVMInactiveDomainXML(name, updatedXML)
}

// SetVMDirectBoot 设置虚拟机直接内核引导配置。
func SetVMDirectBoot(name string, cfg *vm_xml.DirectBootConfig) error {
	xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	var updatedXML string
	var err error

	if cfg == nil || !cfg.Enabled {
		// 关闭直接内核引导：移除 kernel/initrd/cmdline
		updatedXML = vm_xml.RemoveDirectBootFromDomainXML(xmlResult.Stdout)
	} else {
		// 启用直接内核引导
		updatedXML, err = vm_xml.ApplyDirectBootToDomainXML(xmlResult.Stdout, cfg)
		if err != nil {
			return err
		}
	}

	return SetVMInactiveDomainXML(name, updatedXML)
}
