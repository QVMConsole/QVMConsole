package snapshot

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"kvm_console/utils"
)

func commitActiveExternalOverlay(vmName, target, overlayPath string) error {
	if err := ensureSnapshotDiskAccessForPaths([]string{overlayPath}); err != nil {
		return err
	}
	// virsh blockcommit 需要合并整个磁盘链数据，属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout(
		"virsh",
		"blockcommit",
		vmName,
		target,
		"--shallow",
		"--active",
		"--pivot",
		"--verbose",
		"--delete",
	)
	if result.Error != nil {
		return fmt.Errorf("blockcommit 失败: %s", result.Stderr)
	}
	current, err := getCurrentVMDiskSources(vmName)
	if err != nil {
		return err
	}
	for _, disk := range current {
		if disk.Target == target && disk.Source == overlayPath {
			return fmt.Errorf("blockcommit 已完成但磁盘 %s 仍指向 overlay: %s", target, overlayPath)
		}
	}
	_ = os.Remove(overlayPath)
	return nil
}

func copyActiveExternalOverlayToStandalone(vmName, target, overlayPath string) error {
	destPath := generateStandaloneDiskPath(overlayPath)
	if err := ensureSnapshotDiskAccessForPaths([]string{overlayPath, destPath}); err != nil {
		return err
	}
	// virsh blockcopy 需要复制整个活动磁盘数据，属于大 IO 操作，不设置自动超时
	result := utils.ExecCommandNoTimeout(
		"virsh",
		"blockcopy",
		vmName,
		target,
		"--dest",
		destPath,
		"--format",
		"qcow2",
		"--wait",
		"--verbose",
		"--pivot",
	)
	if result.Error != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("blockcopy 失败: %s", result.Stderr)
	}
	_ = utils.ChownLibvirtQEMU(destPath)
	current, err := getCurrentVMDiskSources(vmName)
	if err != nil {
		return err
	}
	for _, disk := range current {
		if disk.Target == target && disk.Source == overlayPath {
			return fmt.Errorf("blockcopy 已完成但磁盘 %s 仍指向 overlay: %s", target, overlayPath)
		}
	}
	_ = os.Remove(overlayPath)
	return nil
}

func generateStandaloneDiskPath(sourcePath string) string {
	dir := path.Dir(sourcePath)
	base := path.Base(sourcePath)
	name := base
	if strings.HasSuffix(name, ".qcow2") {
		name = strings.TrimSuffix(name, ".qcow2")
	}
	return path.Join(dir, fmt.Sprintf("%s.consolidated_%s.qcow2", name, time.Now().Format("20060102_150405")))
}

func commitInactiveExternalOverlay(vmName, overlayPath, backingPath string) error {
	if strings.TrimSpace(backingPath) == "" {
		return fmt.Errorf("overlay %s 的 backing 为空", overlayPath)
	}
	if err := ensureSnapshotDiskAccessForPaths([]string{overlayPath, backingPath}); err != nil {
		return err
	}
	// qemu-img commit 需要合并整个 overlay 数据，属于大 IO 操作，不设置自动超时
	commitResult := utils.ExecCommandNoTimeout("qemu-img", "commit", overlayPath)
	if commitResult.Error != nil {
		return fmt.Errorf("qemu-img commit 失败: %s", commitResult.Stderr)
	}
	if err := replaceVMDiskSource(vmName, overlayPath, backingPath); err != nil {
		return err
	}
	_ = os.Remove(overlayPath)
	return nil
}

func replaceVMDiskSource(vmName, oldPath, newPath string) error {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return nil
	}
	escapedOld := strings.ReplaceAll(oldPath, "/", "\\/")
	escapedOld = strings.ReplaceAll(escapedOld, ".", "\\.")
	escapedNew := strings.ReplaceAll(newPath, "/", "\\/")
	shellCmd := fmt.Sprintf("EDITOR=\"sed -i 's|%s|%s|g'\" virsh edit %s", escapedOld, escapedNew, utils.ShellSingleQuote(vmName))
	editResult := utils.ExecShell(shellCmd)
	if editResult.Error != nil {
		return fmt.Errorf("修改虚拟机磁盘配置失败: %s", editResult.Stderr)
	}
	return nil
}