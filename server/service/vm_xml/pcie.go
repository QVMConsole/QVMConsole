package vm_xml

import (
	"regexp"
	"strings"
)

const (
	DefaultCreatePCIERootPorts = 6
	MaxCreatePCIERootPorts     = 32
	defaultQ35BaseDevices      = 4
)

var createPCIERootPortPattern = regexp.MustCompile(`<controller\b[^>]*\bmodel=['"]pcie-root-port['"][^>]*>`)

// ResolveCreatePCIERootPortCount 根据初始 XML 和创建时追加的 PCIe 设备计算根端口下限。
func ResolveCreatePCIERootPortCount(domainXML string, requested, additionalDevices int) int {
	if !strings.Contains(domainXML, "q35") {
		return 0
	}

	portCount := requested
	if portCount <= 0 {
		portCount = DefaultCreatePCIERootPorts
	}

	baseDevices := len(createPCIERootPortPattern.FindAllStringIndex(domainXML, -1))
	if baseDevices == 0 {
		// 手工构建的 q35 XML 尚未包含 root-port，按常规磁盘、串口、网卡和显示设备预留。
		baseDevices = defaultQ35BaseDevices
	}
	if additionalDevices < 0 {
		additionalDevices = 0
	}
	if minimum := baseDevices + additionalDevices; portCount < minimum {
		portCount = minimum
	}
	if portCount > MaxCreatePCIERootPorts {
		portCount = MaxCreatePCIERootPorts
	}
	return portCount
}
