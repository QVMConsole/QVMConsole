package bridge

import (
	"fmt"
	"strings"

	ovspkg "kvm_console/service/ovs"
	"kvm_console/utils"
)

func BuildOVSInterfaceXMLForBridge(mac, modelName, bridge string) string {
	if strings.TrimSpace(modelName) == "" {
		modelName = "virtio"
	}
	if strings.TrimSpace(bridge) == "" {
		bridge = ovspkg.OvsBridgeName()
	}
	var b strings.Builder
	b.WriteString("    <interface type='bridge'>\n")
	if strings.TrimSpace(mac) != "" {
		b.WriteString(fmt.Sprintf("      <mac address='%s'/>\n", strings.TrimSpace(mac)))
	}
	b.WriteString(fmt.Sprintf("      <source bridge='%s'/>\n", strings.TrimSpace(bridge)))
	b.WriteString("      <virtualport type='openvswitch'/>\n")
	b.WriteString(fmt.Sprintf("      <model type='%s'/>\n", strings.TrimSpace(modelName)))
	b.WriteString("    </interface>")
	return b.String()
}

func BuildOVSVirtInstallNetworkArgForBridge(modelName, bridge string) string {
	if strings.TrimSpace(modelName) == "" {
		modelName = "virtio"
	}
	if strings.TrimSpace(bridge) == "" {
		bridge = ovspkg.OvsBridgeName()
	}
	value := fmt.Sprintf("bridge=%s,virtualport.type=openvswitch,model=%s", strings.TrimSpace(bridge), strings.TrimSpace(modelName))
	return "--network " + utils.ShellSingleQuote(value)
}
