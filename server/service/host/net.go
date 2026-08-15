package host

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kvm_console/model"
)

// 虚拟接口前缀：这些接口属于回环、虚拟机、容器、网桥等，不属于宿主机物理网卡
var virtualInterfacePrefixes = []string{
	"lo", "virbr", "vnet", "veth", "docker", "br-", "ovs",
	"tap", "tun", "macvtap", "gretap", "dummy", "vxlan", "wg",
}

// isPhysicalHostInterface 判断接口是否为宿主机物理网卡：
// 先排除常见虚拟接口前缀，再检查 /sys/class/net/<name>/device 是否存在
// （真实硬件设备（PCI/USB 等）才会创建该符号链接）。
func isPhysicalHostInterface(name string) bool {
	if name == "" {
		return false
	}
	for _, prefix := range virtualInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	if _, err := os.Stat(filepath.Join("/sys/class/net", name, "device")); err != nil {
		return false
	}
	return true
}

// CollectHostNetIOBytesPerDevice 采集宿主机各物理网卡的累计收发字节数。
// 数据来源 /proc/net/dev，汇总值等于各物理网卡之和，避免虚拟接口（网桥/容器/veth 等）重复计数。
func CollectHostNetIOBytesPerDevice() ([]model.HostNetDeviceStat, error) {
	content, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}

	var devices []model.HostNetDeviceStat
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		// 跳过前两行标题（Inter-| Receive ... 与 face |bytes ...）
		if lineNo <= 2 {
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if !isPhysicalHostInterface(name) {
			continue
		}
		rx, err1 := strconv.ParseInt(fields[1], 10, 64)
		tx, err2 := strconv.ParseInt(fields[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		devices = append(devices, model.HostNetDeviceStat{Name: name, RxBytes: rx, TxBytes: tx})
	}

	// 按名称稳定排序，保证前端下拉选项顺序稳定
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return devices, nil
}
