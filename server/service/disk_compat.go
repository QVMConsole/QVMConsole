package service

import (
	"kvm_console/service/storage/disk"
)

// Backward-compatible type aliases for types moved to storage/disk package.
// Existing code referencing service.DiskIOPSTune etc. continues to work.
type DiskIOPSTune = disk.DiskIOPSTune
type DiskInfo = disk.DiskInfo
type DiskSimpleInfo = disk.DiskSimpleInfo
type ExtraDiskParam = disk.ExtraDiskParam
type IOPSField = disk.IOPSField
type PCIEInfo = disk.PCIEInfo

var ErrNoPCIESlots = disk.ErrNoPCIESlots

// Backward-compatible function wrappers for functions moved to storage/disk package.
// These delegate to the disk package so existing callers don't need to change imports.
func ListDisks(vmName string) ([]DiskInfo, error) {
	return disk.ListDisks(vmName)
}

func SetDiskBus(vmName, device, newBus string) error {
	return disk.SetDiskBus(vmName, device, newBus)
}

func SetCDROMBus(vmName, device, newBus string) error {
	return disk.SetCDROMBus(vmName, device, newBus)
}

func ResizeDisk(vmName, device string, newSizeGB int) error {
	return disk.ResizeDisk(vmName, device, newSizeGB)
}

func RemoveDisk(vmName, device string, deleteFile bool) error {
	return disk.RemoveDisk(vmName, device, deleteFile)
}

func AddDisk(vmName string, sizeGB int, format string) (string, error) {
	return disk.AddDisk(vmName, sizeGB, format)
}

func AddDiskWithBus(vmName string, sizeGB int, format, bus string) (string, error) {
	return disk.AddDiskWithBus(vmName, sizeGB, format, bus)
}

func AddDiskWithBusInDir(vmName string, sizeGB int, format, bus, diskDir string) (string, error) {
	return disk.AddDiskWithBusInDir(vmName, sizeGB, format, bus, diskDir)
}

func AddExtraDisksForVM(vmName string, disks []ExtraDiskParam, defaultDir, defaultBus string, isAdmin bool, progressFn func(int, string)) error {
	return disk.AddExtraDisksForVM(vmName, disks, defaultDir, defaultBus, isAdmin, progressFn)
}

func AttachExistingDisk(vmName, diskPath, bus string) (string, error) {
	return disk.AttachExistingDisk(vmName, diskPath, bus)
}

func ChangeCDROM(vmName, isoPath, device string, forceNew bool, requestedBus ...string) error {
	return disk.ChangeCDROM(vmName, isoPath, device, forceNew, requestedBus...)
}

func EjectCDROM(vmName, device string) error {
	return disk.EjectCDROM(vmName, device)
}

func RemoveCDROM(vmName, device string) error {
	return disk.RemoveCDROM(vmName, device)
}

func ChangeFloppy(vmName, imagePath, device string, forceNew bool) error {
	return disk.ChangeFloppy(vmName, imagePath, device, forceNew)
}

func EjectFloppy(vmName, device string) error {
	return disk.EjectFloppy(vmName, device)
}

func RemoveFloppy(vmName, device string) error {
	return disk.RemoveFloppy(vmName, device)
}

func SetDiskIOPSTune(vmName, dev string, iops *DiskIOPSTune) error {
	return disk.SetDiskIOPSTune(vmName, dev, iops)
}

func GetDiskIOPSTune(vmName, dev string) (*DiskIOPSTune, error) {
	return disk.GetDiskIOPSTune(vmName, dev)
}

func SetVMPCIERootPorts(vmName string, targetCount int) error {
	return disk.SetVMPCIERootPorts(vmName, targetCount)
}

func GetVMPCIERootPorts(vmName string) (int, error) {
	return disk.GetVMPCIERootPorts(vmName)
}

func GetVMPCIEInfo(vmName string) (*PCIEInfo, error) {
	return disk.GetVMPCIEInfo(vmName)
}

func CheckVMSnapshotSafety(vmName string) (bool, []string, error) {
	return disk.CheckVMSnapshotSafety(vmName)
}

func GetVMQcow2Disks(vmName string) ([]DiskSimpleInfo, error) {
	return disk.GetVMQcow2Disks(vmName)
}

func GetDiskFilePath(vmName, device string) string {
	return disk.GetDiskFilePath(vmName, device)
}

func TransferDiskFile(diskPath, username string) error {
	return disk.TransferDiskFile(diskPath, username)
}

func NormalizeVMDiskBus(value string) string {
	return disk.NormalizeVMDiskBus(value)
}

// ParseQemuInfoStr parses a string value from qemu-img info JSON (top-level field only).
func ParseQemuInfoStr(output, key string) string {
	return disk.ParseQemuInfoStr(output, key)
}

// ParseQemuInfoGB parses a capacity value from qemu-img info JSON.
func ParseQemuInfoGB(output, key string) string {
	return disk.ParseQemuInfoGB(output, key)
}

// GetDevPrefix returns the device name prefix based on bus type.
func GetDevPrefix(bus string) string {
	return disk.GetDevPrefix(bus)
}
