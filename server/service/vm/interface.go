package vm

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/guest_agent"
	"kvm_console/utils"
)

// AttachVMInterface 为虚拟机添加一个新的网口并连接到 VPC 交换机
func AttachVMInterface(vmName string, sw model.VPCSwitch, nicModel string, interfaceOrder int) error {
	nicModel = strings.TrimSpace(nicModel)
	if nicModel == "" {
		nicModel = "virtio"
	}

	// 生成网口 XML
	bridgeName := D.BridgeNameForSwitch(sw)
	interfaceXML := buildVMInterfaceXML(vmName, bridgeName, sw, nicModel, interfaceOrder)

	// 检查虚拟机状态
	state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)

	if state == "running" {
		// 运行中：热插网口
		xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("_vm-nic-attach-%s-%d.xml", D.SafeVMXMLFileName(vmName), interfaceOrder))
		if err := os.WriteFile(xmlPath, []byte(interfaceXML), 0600); err != nil {
			return fmt.Errorf("写入网口 XML 失败: %w", err)
		}
		defer os.Remove(xmlPath)

		attach := utils.ExecCommandWithTimeout("virsh", 60*time.Second, "attach-device", vmName, xmlPath, "--live")
		if attach.Error != nil {
			return fmt.Errorf("热插网口失败: %s", D.FirstNonEmpty(attach.Stderr, attach.Error.Error()))
		}
		// 同时持久化
		attachPersist := utils.ExecCommandWithTimeout("virsh", 60*time.Second, "attach-device", vmName, xmlPath, "--config")
		if attachPersist.Error != nil {
			logger.App.Warn("持久化网口失败（运行态已生效）", "detail", D.FirstNonEmpty(attachPersist.Stderr, attachPersist.Error.Error()))
		}
		if err := guest_agent.ConfigureLinuxDHCPHotplugNetwork(vmName); err != nil {
			logger.App.Warn("为来宾附加网口配置 DHCP 失败", "vm", vmName, "error", err)
		}
	} else {
		// 关机状态：修改持久化 XML
		result := utils.ExecCommand("virsh", "dumpxml", vmName, "--inactive")
		if result.Error != nil {
			result = utils.ExecCommand("virsh", "dumpxml", vmName)
		}
		if result.Error != nil {
			return fmt.Errorf("读取 VM XML 失败: %s", result.Stderr)
		}

		// 在 </devices> 之前插入新网口
		xmlText := result.Stdout
		devicesEnd := strings.LastIndex(xmlText, "</devices>")
		if devicesEnd < 0 {
			return fmt.Errorf("VM XML 格式错误，未找到 </devices>")
		}

		updatedXML := xmlText[:devicesEnd] + interfaceXML + "\n" + xmlText[devicesEnd:]

		xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("_vm-nic-define-%s-%d.xml", D.SafeVMXMLFileName(vmName), interfaceOrder))
		if err := os.WriteFile(xmlPath, []byte(updatedXML), 0600); err != nil {
			return fmt.Errorf("写入 VM 持久化 XML 失败: %w", err)
		}
		defer os.Remove(xmlPath)

		define := utils.ExecCommand("virsh", "define", xmlPath)
		if define.Error != nil {
			return fmt.Errorf("持久化 VM 网口失败: %s", define.Stderr)
		}
	}

	return nil
}

// detachVMInterface 从虚拟机中移除指定 interface_order（逻辑序号）对应的网口
// 优先用 interface_order 作为 XML 位置索引匹配，失败时回退到 MAC 匹配
func DetachVMInterface(vmName string, interfaceOrder int) error {
	// 获取虚拟机所有网口
	result := utils.ExecCommand("virsh", "dumpxml", vmName)
	isRunning := result.Error == nil
	if !isRunning {
		result = utils.ExecCommand("virsh", "dumpxml", vmName, "--inactive")
	}
	if result.Error != nil {
		return fmt.Errorf("读取 VM XML 失败: %s", result.Stderr)
	}

	xmlText := result.Stdout

	// 查找目标 interface 块：先用位置索引，失败后用 MAC 回退
	interfaceBlock, ok := extractInterfaceByIndex(xmlText, interfaceOrder)
	if !ok {
		// 位置匹配失败（常见于 interface_order 与 XML 实际位置不一致的场景）
		// 回退：通过确定性 MAC 在 XML 中查找匹配的 interface 块
		targetMAC := generateInterfaceMAC(vmName, interfaceOrder)
		interfaceBlock, ok = extractInterfaceByMAC(xmlText, targetMAC)
		if !ok {
			return fmt.Errorf("未找到第 %d 个网口", interfaceOrder+1)
		}
		logger.App.Info("网口位置匹配失败，已通过 MAC 回退匹配", "vm", vmName, "order", interfaceOrder, "mac", targetMAC)
	}

	// 清理运行时特有元素以便 detach
	cleanBlock := D.StripRuntimeOnlyInterfaceElements(interfaceBlock)

	state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", vmName).Stdout)

	if state == "running" {
		// 运行中：热拔网口
		xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("_vm-nic-detach-%s-%d.xml", D.SafeVMXMLFileName(vmName), interfaceOrder))
		if err := os.WriteFile(xmlPath, []byte(cleanBlock), 0600); err != nil {
			return fmt.Errorf("写入 detach XML 失败: %w", err)
		}
		defer os.Remove(xmlPath)

		detach := utils.ExecCommandWithTimeout("virsh", 60*time.Second, "detach-device", vmName, xmlPath, "--live")
		if detach.Error != nil {
			return fmt.Errorf("热拔网口失败: %s", D.FirstNonEmpty(detach.Stderr, detach.Error.Error()))
		}
		// 同时持久化
		detachPersist := utils.ExecCommandWithTimeout("virsh", 60*time.Second, "detach-device", vmName, xmlPath, "--config")
		if detachPersist.Error != nil {
			logger.App.Warn("持久化移除网口失败（运行态已生效）", "detail", D.FirstNonEmpty(detachPersist.Stderr, detachPersist.Error.Error()))
		}
	} else {
		// 关机状态：修改持久化 XML，从 XML 中移除对应的 interface 块
		searchFrom := 0
		foundCount := 0
		for {
			startRel := strings.Index(xmlText[searchFrom:], "<interface ")
			if startRel < 0 {
				break
			}
			start := searchFrom + startRel
			endRel := strings.Index(xmlText[start:], "</interface>")
			if endRel < 0 {
				break
			}
			end := start + endRel + len("</interface>")

			if foundCount == interfaceOrder {
				// 找到目标，移除
				updatedXML := xmlText[:start] + xmlText[end:]
				xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("_vm-nic-define-%s-%d.xml", D.SafeVMXMLFileName(vmName), interfaceOrder))
				if err := os.WriteFile(xmlPath, []byte(updatedXML), 0600); err != nil {
					return fmt.Errorf("写入 VM 持久化 XML 失败: %w", err)
				}
				defer os.Remove(xmlPath)

				define := utils.ExecCommand("virsh", "define", xmlPath)
				if define.Error != nil {
					return fmt.Errorf("持久化移除 VM 网口失败: %s", define.Stderr)
				}
				return nil
			}

			foundCount++
			searchFrom = end
		}
		return fmt.Errorf("未在 XML 中找到第 %d 个网口", interfaceOrder+1)
	}

	return nil
}

// extractInterfaceByIndex 从 VM XML 中提取第 N 个（从0开始）interface 块
func extractInterfaceByIndex(xmlText string, targetIndex int) (string, bool) {
	searchFrom := 0
	foundCount := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return "", false
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return "", false
		}
		end := start + endRel + len("</interface>")

		if foundCount == targetIndex {
			return xmlText[start:end], true
		}

		foundCount++
		searchFrom = end
	}
}

// extractInterfaceByMAC 从 VM XML 中按 MAC 地址查找第一个匹配的 interface 块
func extractInterfaceByMAC(xmlText, targetMAC string) (string, bool) {
	targetMAC = strings.ToLower(strings.TrimSpace(targetMAC))
	searchFrom := 0
	for {
		startRel := strings.Index(xmlText[searchFrom:], "<interface ")
		if startRel < 0 {
			return "", false
		}
		start := searchFrom + startRel
		endRel := strings.Index(xmlText[start:], "</interface>")
		if endRel < 0 {
			return "", false
		}
		end := start + endRel + len("</interface>")
		block := xmlText[start:end]

		// 在 interface 块内查找 mac address='...'
		macStartRel := strings.Index(strings.ToLower(block), "mac address='")
		if macStartRel >= 0 {
			macValStart := macStartRel + len("mac address='")
			macValEnd := strings.Index(block[macValStart:], "'")
			if macValEnd >= 0 {
				macAddr := strings.ToLower(strings.TrimSpace(block[macValStart : macValStart+macValEnd]))
				if macAddr == targetMAC {
					return block, true
				}
			}
		}

		searchFrom = end
	}
}

// buildVMInterfaceXML 构建虚拟机网口的 XML 块
func buildVMInterfaceXML(vmName, bridgeName string, sw model.VPCSwitch, nicModel string, interfaceOrder int) string {
	model := nicModel
	if model == "" {
		model = "virtio"
	}

	// 生成 MAC 地址和 UUID（interfaceid 必须是 UUID 格式）
	macAddr := generateInterfaceMAC(vmName, interfaceOrder)
	ifaceUUID := generateInterfaceUUID(vmName, interfaceOrder)

	xml := fmt.Sprintf(`    <interface type='bridge'>
      <mac address='%s'/>
      <source bridge='%s'/>
      <model type='%s'/>
      <virtualport type='openvswitch'>
        <parameters interfaceid='%s'/>
      </virtualport>`, macAddr, bridgeName, model, ifaceUUID)

	if !D.SwitchUsesDirectBridge(sw) && sw.VLANID > 0 {
		xml += fmt.Sprintf(`
      <vlan>
        <tag id='%d'/>
      </vlan>
      <bandwidth>
        <inbound average='0' burst='0' peak='0'/>
        <outbound average='0' burst='0' peak='0'/>
      </bandwidth>`, sw.VLANID)
	}
	if D.IsPortSecurityEnabled != nil && D.IsPortSecurityEnabled() {
		xml += "\n      <link state='down'/>"
	}

	xml += "\n    </interface>"
	return xml
}

// generateInterfaceMAC 为接口生成 MAC 地址
func generateInterfaceMAC(vmName string, order int) string {
	// 使用简单的哈希生成最后三个字节
	h := uint32(0)
	for _, c := range vmName {
		h = h*31 + uint32(c)
	}
	h = h*31 + uint32(order)

	// MAC 前缀 52:54:00 是 QEMU/KVM 常用的
	b3 := (h >> 16) & 0xFF
	b4 := (h >> 8) & 0xFF
	b5 := h & 0xFF
	// 确保不超过 0x7F 避免多播位
	b3 = b3 & 0x7F
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", b3, b4, b5)
}

// generateInterfaceUUID 为接口生成确定性 UUID（interfaceid 必须为 UUID 格式）
func generateInterfaceUUID(vmName string, order int) string {
	input := fmt.Sprintf("%s:%d:kvm-console", vmName, order)
	hash := sha1.Sum([]byte(input))
	// 使用 hash 的前 16 字节构造 UUID v5 格式
	hash[6] = (hash[6] & 0x0f) | 0x50 // version 5
	hash[8] = (hash[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

// CountVMInterfaces 统计虚拟机网口数量
func CountVMInterfaces(vmName string) int {
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	count := 0
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		count++
	}
	return count
}

// GetVMMACByOrder 获取虚拟机第 N 个网口的 MAC 地址
func GetVMMACByOrder(vmName string, order int) string {
	if order < 0 {
		return ""
	}
	expectedMAC := strings.ToLower(generateInterfaceMAC(vmName, order))
	for _, mac := range getAllVMInterfaceMACs(vmName) {
		if strings.EqualFold(mac, expectedMAC) {
			return expectedMAC
		}
	}

	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	idx := 0
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		if idx == order {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				return fields[4]
			}
		}
		idx++
	}
	return ""
}

// UpsertVMBindingNicModel 更新绑定的网卡型号
func UpsertVMBindingNicModel(vmName string, interfaceOrder int, nicModel string) error {
	if model.DB == nil {
		return nil
	}
	return model.DB.Model(&model.VPCVMBinding{}).
		Where("vm_name = ? AND interface_order = ?", vmName, interfaceOrder).
		Update("nic_model", nicModel).Error
}

// GetNextVMBindingOrder 获取下一个可用的接口序号（找第一个空闲槽位，从0开始）
func GetNextVMBindingOrder(vmName string) int {
	if model.DB == nil {
		return 0
	}
	var orders []int
	model.DB.Model(&model.VPCVMBinding{}).
		Where("vm_name = ?", vmName).
		Pluck("interface_order", &orders)
	used := make(map[int]bool, len(orders))
	for _, o := range orders {
		used[o] = true
	}
	for i := 0; ; i++ {
		if !used[i] {
			return i
		}
	}
}

// getAllVMInterfaceMACs 获取虚拟机所有网口的 MAC 地址列表
func getAllVMInterfaceMACs(vmName string) []string {
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return nil
	}
	var macs []string
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			macs = append(macs, strings.ToLower(strings.TrimSpace(fields[4])))
		}
	}
	return macs
}
