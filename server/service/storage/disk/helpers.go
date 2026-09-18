package disk

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// CheckVMSnapshotSafety checks whether a VM's snapshot state allows disk operations.
// Returns: hasExternalSnap (whether external snapshots exist), snapNames (external snapshot names), err.
// Uses the CheckSnapshotSafety function variable injected from service root.
func CheckVMSnapshotSafety(vmName string) (bool, []string, error) {
	if CheckSnapshotSafety != nil {
		return CheckSnapshotSafety(vmName)
	}
	// fallback: if not wired, assume safe (no external snapshots)
	return false, nil, nil
}

// GetVMQcow2Disks returns all qcow2 disks of a VM (for delete confirmation UI).
func GetVMQcow2Disks(vmName string) ([]DiskSimpleInfo, error) {
	disks, err := ListDisks(vmName)
	if err != nil {
		return nil, err
	}

	var result []DiskSimpleInfo
	firstDisk := true // mark first disk as system disk

	for _, disk := range disks {
		// only return qcow2 format disks (skip cdrom and other formats)
		if disk.DeviceType == "cdrom" || disk.Path == "" {
			continue
		}
		// only qcow2 format
		if disk.Format != "qcow2" {
			continue
		}

		info := DiskSimpleInfo{
			Device:     disk.Device,
			Path:       disk.Path,
			CapacityGB: disk.CapacityGB,
			Format:     disk.Format,
			IsSystem:   firstDisk,
		}

		// get actual file size via du
		duResult := utils.ExecShell(fmt.Sprintf("du -b %s 2>/dev/null | awk '{print $1}'", utils.ShellSingleQuote(disk.Path)))
		if duResult.Error == nil {
			info.SizeBytes, _ = strconv.ParseInt(strings.TrimSpace(duResult.Stdout), 10, 64)
		}

		result = append(result, info)
		firstDisk = false
	}

	return result, nil
}

// GetDiskFilePath returns the file path for a disk device.
func GetDiskFilePath(vmName, device string) string {
	domainXML, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		return ""
	}
	blkList := libvirt_rpc.ParseDisksFromDomainXML(domainXML)
	for _, blk := range blkList {
		if blk.Target == device {
			return blk.Source
		}
	}
	return ""
}

// ExtractFullDiskXML extracts the complete <disk>...</disk> XML block for a target device
// from a domain XML string. Returns the raw XML substring including all attributes,
// driver, source, target, address, and backingStore elements.
// This is needed because DetachDeviceFlagsRPC requires the full disk definition to
// properly remove the device from both live and persistent config.
func ExtractFullDiskXML(domainXML, targetDev string) (string, error) {
	lines := strings.Split(domainXML, "\n")
	var diskLines []string
	inDisk := false
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "<disk ") {
			inDisk = true
			diskLines = nil
		}

		if inDisk {
			diskLines = append(diskLines, line)

			// check if this <disk> block contains our target device
			if strings.Contains(trimmed, "<target") && strings.Contains(trimmed, "dev='"+targetDev+"'") {
				found = true
			}

			if strings.Contains(trimmed, "</disk>") {
				if found {
					return strings.Join(diskLines, "\n"), nil
				}
				inDisk = false
				diskLines = nil
			}
		}
	}

	if !found {
		return "", fmt.Errorf("未找到设备 %s 的磁盘XML定义", targetDev)
	}
	return "", fmt.Errorf("未找到设备 %s 的磁盘XML定义", targetDev)
}

// DiskDeviceExists checks whether a disk device still appears in the VM's domain XML.
// Used to verify that a detach operation succeeded.
func DiskDeviceExists(vmName, targetDev string) bool {
	domainXML, err := libvirt_rpc.GetDomainXMLRPC(vmName, 0)
	if err != nil {
		// if we can't get XML, assume the domain doesn't exist or the device is gone
		return false
	}
	blkList := libvirt_rpc.ParseDisksFromDomainXML(domainXML)
	for _, blk := range blkList {
		if blk.Target == targetDev {
			return true
		}
	}
	return false
}

// TransferDiskFile transfers a disk file to the user's virtual disk directory.
func TransferDiskFile(diskPath, username string) error {
	diskDir := GetUserDiskDirFn(username)
	// ensure target directory exists
	utils.ExecCommand("mkdir", "-p", diskDir)

	filename := filepath.Base(diskPath)
	destPath := filepath.Join(diskDir, filename)

	// if target already exists, add timestamp to avoid conflict
	checkResult := utils.ExecShell(fmt.Sprintf("test -f %s && echo exists", utils.ShellSingleQuote(destPath)))
	if strings.TrimSpace(checkResult.Stdout) == "exists" {
		ts := time.Now().Format("20060102150405")
		ext := filepath.Ext(filename)
		nameOnly := strings.TrimSuffix(filename, ext)
		destPath = filepath.Join(diskDir, fmt.Sprintf("%s_%s%s", nameOnly, ts, ext))
	}

	// move file
	if err := os.Rename(diskPath, destPath); err != nil {
		return fmt.Errorf("转移磁盘文件失败: %v", err)
	}

	// set file permissions
	_ = utils.ChownLibvirtQEMU(destPath)

	return nil
}
