package bandwidth

import (
	"sort"
	"strings"

	"kvm_console/utils"
)

type vmBandwidthInterface struct {
	Order  int
	Name   string
	Source string
	MAC    string
}

// listVMBandwidthInterfaces 按 libvirt 网卡顺序返回全部运行态或持久化网卡。
func listVMBandwidthInterfaces(vmName string) []vmBandwidthInterface {
	result := utils.ExecCommand("virsh", "domiflist", strings.TrimSpace(vmName))
	if result.Error != nil {
		return nil
	}
	interfaces := make([]vmBandwidthInterface, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] == "Interface" {
			continue
		}
		name := fields[0]
		if name == "-" {
			name = ""
		}
		interfaces = append(interfaces, vmBandwidthInterface{
			Order: len(interfaces), Name: name, Source: fields[2], MAC: strings.ToLower(fields[4]),
		})
	}
	sort.SliceStable(interfaces, func(i, j int) bool { return interfaces[i].Order < interfaces[j].Order })
	return interfaces
}
