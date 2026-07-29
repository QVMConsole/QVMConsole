package host

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"kvm_console/utils"
)

var fallbackHostDiskNamePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^sd[a-z]+$`),
	regexp.MustCompile(`^vd[a-z]+$`),
	regexp.MustCompile(`^xvd[a-z]+$`),
	regexp.MustCompile(`^hd[a-z]+$`),
	regexp.MustCompile(`^nvme[0-9]+n[0-9]+$`),
	regexp.MustCompile(`^mmcblk[0-9]+$`),
	regexp.MustCompile(`^md[0-9]+$`),
}

// CollectHostDiskIOBytes 汇总宿主机物理磁盘的累计读写字节数。
// 优先使用 lsblk 枚举 type=disk 的顶层设备，避免硬编码设备名规则；
// 若 lsblk 不可用或返回空，则退回到 /proc/diskstats 的常见顶层磁盘名匹配。
func CollectHostDiskIOBytes() (int64, int64, error) {
	diskstatsContent, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return 0, 0, err
	}

	deviceNames := loadHostDiskDeviceNames()
	if len(deviceNames) == 0 {
		deviceNames = detectTopLevelDiskDevicesFromDiskstats(string(diskstatsContent))
	}
	if len(deviceNames) == 0 {
		return 0, 0, nil
	}

	readSectors, writeSectors := parseDiskStatsSectors(string(diskstatsContent), deviceNames)
	return readSectors * 512, writeSectors * 512, nil
}

func loadHostDiskDeviceNames() []string {
	result := utils.ExecCommand("lsblk", "-dn", "-o", "NAME,TYPE")
	if result.Error != nil {
		return nil
	}
	return parseHostDiskDeviceNames(result.Stdout)
}

func parseHostDiskDeviceNames(output string) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	devices := make([]string, 0)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if fields[1] != "disk" {
			continue
		}
		devices = append(devices, fields[0])
	}

	return devices
}

func detectTopLevelDiskDevicesFromDiskstats(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	devices := make([]string, 0)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		deviceName := fields[2]
		if isLikelyTopLevelDiskDevice(deviceName) && !slices.Contains(devices, deviceName) {
			devices = append(devices, deviceName)
		}
	}

	return devices
}

func isLikelyTopLevelDiskDevice(deviceName string) bool {
	for _, pattern := range fallbackHostDiskNamePatterns {
		if pattern.MatchString(deviceName) {
			return true
		}
	}
	return false
}

func parseDiskStatsSectors(content string, deviceNames []string) (int64, int64) {
	if len(deviceNames) == 0 {
		return 0, 0
	}

	targets := make(map[string]struct{}, len(deviceNames))
	for _, name := range deviceNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		targets[name] = struct{}{}
	}

	var readSectors int64
	var writeSectors int64

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		deviceName := fields[2]
		if _, ok := targets[deviceName]; !ok {
			continue
		}

		rd, err := strconv.ParseInt(fields[5], 10, 64)
		if err != nil {
			continue
		}
		wr, err := strconv.ParseInt(fields[9], 10, 64)
		if err != nil {
			continue
		}

		readSectors += rd
		writeSectors += wr
	}

	return readSectors, writeSectors
}

// GetHostDiskInfos 获取宿主机所有挂载磁盘信息
func GetHostDiskInfos() ([]HostDiskInfo, error) {
	result := utils.ExecShell(`df -k --output=source,target,fstype,size,used,avail,pcent 2>/dev/null | tail -n +2`)
	if result.Error != nil {
		return nil, result.Error
	}

	var disks []HostDiskInfo
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		if fsType == "tmpfs" || fsType == "devtmpfs" || fsType == "efivarfs" || device == "none" || device == "tmpfs" {
			continue
		}

		totalKB, _ := strconv.ParseInt(fields[3], 10, 64)
		usedKB, _ := strconv.ParseInt(fields[4], 10, 64)
		freeKB, _ := strconv.ParseInt(fields[5], 10, 64)
		usePercent := fields[6]

		disks = append(disks, HostDiskInfo{
			MountPoint: mountPoint,
			Device:     device,
			FSType:     fsType,
			TotalKB:    totalKB,
			UsedKB:     usedKB,
			FreeKB:     freeKB,
			UsePercent: usePercent,
			ReadOnly:   utils.IsFilesystemReadOnly(mountPoint),
		})
	}
	return disks, nil
}
