package vpc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func extractFirstOVSInterfaceVLANTag(xmlText string) (int, bool) {
	if strings.TrimSpace(xmlText) == "" {
		return 0, false
	}
	bridge := HookOvsBridgeName()
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return 0, false
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return 0, false
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]
		if isOVSBridgeInterfaceBlock(block, bridge) {
			match := regexp.MustCompile(`<tag\s+id=['"]([0-9]+)['"]\s*/>`).FindStringSubmatch(block)
			if len(match) == 2 {
				vlanID, err := strconv.Atoi(match[1])
				return vlanID, err == nil && vlanID > 0
			}
			return 0, false
		}
		searchFrom = end
	}
}

func setFirstOVSInterfaceVPC(xmlText string, vlanID int) (string, bool) {
	if strings.TrimSpace(xmlText) == "" || vlanID <= 0 {
		return xmlText, false
	}
	updated := xmlText
	bridgeUpdated, bridgeChanged := setFirstOVSInterfaceBridge(updated, HookOvsBridgeName())
	if bridgeChanged {
		updated = bridgeUpdated
	}
	vlanUpdated, vlanChanged := SetFirstOVSInterfaceVLANTag(updated, vlanID)
	if vlanChanged {
		updated = vlanUpdated
	}
	return updated, bridgeChanged || vlanChanged
}

func setFirstOVSInterfaceDirectBridge(xmlText, bridge string, vlanID int) (string, bool) {
	updated, bridgeChanged := setFirstOVSInterfaceBridge(xmlText, bridge)
	if !bridgeChanged && !firstOVSInterfaceUsesBridge(updated, bridge) {
		return xmlText, false
	}
	if vlanID > 0 {
		vlanUpdated, vlanChanged := setFirstOVSInterfaceAnyVLANTag(updated, vlanID)
		return vlanUpdated, bridgeChanged || vlanChanged
	}
	vlanUpdated := removeFirstInterfaceVLAN(updated)
	return vlanUpdated, bridgeChanged || vlanUpdated != updated
}

func SetFirstOVSInterfaceVLANTag(xmlText string, vlanID int) (string, bool) {
	if strings.TrimSpace(xmlText) == "" || vlanID <= 0 {
		return xmlText, false
	}
	bridge := HookOvsBridgeName()
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return xmlText, false
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return xmlText, false
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]
		if isOVSBridgeInterfaceBlock(block, bridge) {
			updatedBlock, changed := setInterfaceBlockVLANTag(block, vlanID)
			if !changed {
				return xmlText, false
			}
			return xmlText[:start] + updatedBlock + xmlText[end:], true
		}
		searchFrom = end
	}
}

func setFirstOVSInterfaceAnyVLANTag(xmlText string, vlanID int) (string, bool) {
	if strings.TrimSpace(xmlText) == "" || vlanID <= 0 {
		return xmlText, false
	}
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return xmlText, false
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return xmlText, false
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]
		hasBridgeType := strings.Contains(block, "<interface type='bridge'") || strings.Contains(block, `<interface type="bridge"`)
		hasOVS := strings.Contains(block, "virtualport type='openvswitch'") || strings.Contains(block, `virtualport type="openvswitch"`)
		if hasBridgeType && hasOVS {
			updatedBlock, changed := setInterfaceBlockVLANTag(block, vlanID)
			if !changed {
				return xmlText, false
			}
			return xmlText[:start] + updatedBlock + xmlText[end:], true
		}
		searchFrom = end
	}
}

func setFirstOVSInterfaceBridge(xmlText, bridge string) (string, bool) {
	if strings.TrimSpace(xmlText) == "" || strings.TrimSpace(bridge) == "" {
		return xmlText, false
	}
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			break
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return xmlText, false
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]
		hasBridgeType := strings.Contains(block, "<interface type='bridge'") || strings.Contains(block, `<interface type="bridge"`)
		hasOVS := strings.Contains(block, "virtualport type='openvswitch'") || strings.Contains(block, `virtualport type="openvswitch"`)
		if hasBridgeType && hasOVS {
			sourceRe := regexp.MustCompile(`<source\s+bridge=['"][^'"]+['"]\s*/>`)
			updatedBlock := sourceRe.ReplaceAllString(block, fmt.Sprintf("<source bridge='%s'/>", strings.TrimSpace(bridge)))
			return xmlText[:start] + updatedBlock + xmlText[end:], updatedBlock != block
		}
		searchFrom = end
	}
	return setFirstCompatibleInterfaceAsOVSBridge(xmlText, bridge)
}

// setFirstCompatibleInterfaceAsOVSBridge 兼容历史 bridge/network 网卡，将其转换为 OVS 网桥网卡。
// 只处理可由 VPC 接管的虚拟网卡，避免改动 hostdev、direct 等特殊设备。
func setFirstCompatibleInterfaceAsOVSBridge(xmlText, bridge string) (string, bool) {
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return xmlText, false
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return xmlText, false
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]
		isBridge := strings.Contains(block, "<interface type='bridge'") || strings.Contains(block, `<interface type="bridge"`)
		isNetwork := strings.Contains(block, "<interface type='network'") || strings.Contains(block, `<interface type="network"`)
		if !isBridge && !isNetwork {
			searchFrom = end
			continue
		}

		updatedBlock := block
		if isNetwork {
			interfaceTypeRe := regexp.MustCompile(`(<interface\s+)type=['"][^'"]+['"]`)
			updatedBlock = interfaceTypeRe.ReplaceAllString(updatedBlock, "${1}type='bridge'")
		}

		sourceRe := regexp.MustCompile(`<source\b[^>]*/\s*>`)
		sourceXML := fmt.Sprintf("<source bridge='%s'/>", strings.TrimSpace(bridge))
		if sourceRe.MatchString(updatedBlock) {
			updatedBlock = sourceRe.ReplaceAllString(updatedBlock, sourceXML)
		} else {
			openingEnd := strings.Index(updatedBlock, ">")
			if openingEnd < 0 {
				searchFrom = end
				continue
			}
			updatedBlock = updatedBlock[:openingEnd+1] + "\n      " + sourceXML + updatedBlock[openingEnd+1:]
		}
		if !strings.Contains(updatedBlock, "virtualport type='openvswitch'") && !strings.Contains(updatedBlock, `virtualport type="openvswitch"`) {
			updatedBlock = sourceRe.ReplaceAllString(updatedBlock, sourceXML+"\n      <virtualport type='openvswitch'/>")
		}
		if updatedBlock == block {
			return xmlText, false
		}
		return xmlText[:start] + updatedBlock + xmlText[end:], true
	}
}

func removeFirstInterfaceVLAN(xmlText string) string {
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return xmlText
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return xmlText
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]
		hasOVS := strings.Contains(block, "virtualport type='openvswitch'") || strings.Contains(block, `virtualport type="openvswitch"`)
		if hasOVS {
			vlanRe := regexp.MustCompile(`(?s)\n\s*<vlan>.*?</vlan>`)
			updatedBlock := vlanRe.ReplaceAllString(block, "")
			return xmlText[:start] + updatedBlock + xmlText[end:]
		}
		searchFrom = end
	}
}

func isOVSBridgeInterfaceBlock(block, bridge string) bool {
	hasBridgeType := strings.Contains(block, "<interface type='bridge'") || strings.Contains(block, `<interface type="bridge"`)
	hasOVS := strings.Contains(block, "virtualport type='openvswitch'") || strings.Contains(block, `virtualport type="openvswitch"`)
	hasSource := strings.Contains(block, fmt.Sprintf("source bridge='%s'", bridge)) || strings.Contains(block, fmt.Sprintf(`source bridge="%s"`, bridge))
	return hasBridgeType && hasOVS && hasSource
}

func setInterfaceBlockVLANTag(block string, vlanID int) (string, bool) {
	if regexp.MustCompile(fmt.Sprintf(`<tag\s+id=['"]%d['"]\s*/>`, vlanID)).MatchString(block) {
		return block, true
	}
	indent := "      "
	if match := regexp.MustCompile(`(?m)^(\s*)<interface\s`).FindStringSubmatch(block); len(match) > 1 {
		indent = match[1] + "  "
	}
	vlanBlock := fmt.Sprintf("%s<vlan>\n%s  <tag id='%d'/>\n%s</vlan>", indent, indent, vlanID, indent)
	vlanRe := regexp.MustCompile(`(?s)\n\s*<vlan>.*?</vlan>`)
	if vlanRe.MatchString(block) {
		return vlanRe.ReplaceAllString(block, "\n"+vlanBlock), true
	}
	closeIdx := strings.LastIndex(block, "</interface>")
	if closeIdx < 0 {
		return block, false
	}
	return block[:closeIdx] + vlanBlock + "\n" + block[closeIdx:], true
}
