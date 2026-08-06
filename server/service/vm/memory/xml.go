package memory

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"kvm_console/service/libvirt_rpc"
)

// ApplyDynamicMemoryConfigToDomainXML 将 XML 调整为最大内存 + 启动内存，并补齐 memballoon。
func ApplyDynamicMemoryConfigToDomainXML(xmlStr string, initialMB, maxMB int, enableFPR bool) (string, error) {
	if initialMB <= 0 || maxMB <= 0 {
		return "", fmt.Errorf("内存配置必须大于 0")
	}
	if initialMB > maxMB {
		return "", fmt.Errorf("启动内存不能大于最大内存")
	}
	maxKiB := maxMB * 1024
	initialKiB := initialMB * 1024
	memoryTag := fmt.Sprintf("<memory unit='KiB'>%d</memory>", maxKiB)
	currentTag := fmt.Sprintf("<currentMemory unit='KiB'>%d</currentMemory>", initialKiB)

	hadVirtioMemArtifacts := hasVirtioMemDomainDevice(xmlStr) || hasMemoryDeviceWithoutModel(xmlStr)
	xmlStr = removeVirtioMemDomainDevices(xmlStr)
	xmlStr = removeMemoryDevicesWithoutModel(xmlStr)
	if hadVirtioMemArtifacts {
		xmlStr = removeDomainMaxMemoryElement(xmlStr)
		xmlStr = removeVirtioMemNumaConfig(xmlStr)
	}
	var replaced bool
	xmlStr, replaced = replaceTopLevelMemoryElement(xmlStr, "memory", memoryTag)
	if !replaced {
		xmlStr = strings.Replace(xmlStr, "<vcpu", memoryTag+"\n  <vcpu", 1)
	}
	xmlStr, replaced = replaceTopLevelMemoryElement(xmlStr, "currentMemory", currentTag)
	if !replaced {
		xmlStr = strings.Replace(xmlStr, memoryTag, memoryTag+"\n  "+currentTag, 1)
	}
	if HookMemoryInjectMemballoonConfig != nil {
		xmlStr = HookMemoryInjectMemballoonConfig(xmlStr, enableFPR)
	}
	return xmlStr, nil
}

// ApplyMemoryMetadataToDomainXML 根据 metadata 应用内存配置到 XML。
func ApplyMemoryMetadataToDomainXML(xmlStr string, meta *VMMemoryMetadata, enableFPR bool) (string, error) {
	if meta == nil {
		return xmlStr, nil
	}
	if NormalizeMemoryBackend(meta.MemoryBackend) == MemoryBackendVirtioMem {
		return ApplyVirtioMemConfigToDomainXML(xmlStr, meta.MemoryInitialMB, meta.MemoryMaxMB)
	}
	return ApplyDynamicMemoryConfigToDomainXML(xmlStr, meta.MemoryInitialMB, meta.MemoryMaxMB, enableFPR)
}

// ApplyVirtioMemConfigToDomainXML 将 XML 调整为 virtio-mem 配置。
func ApplyVirtioMemConfigToDomainXML(xmlStr string, initialMB, maxMB int) (string, error) {
	if initialMB <= 0 || maxMB <= 0 {
		return "", fmt.Errorf("内存配置必须大于 0")
	}
	if initialMB >= maxMB {
		return "", fmt.Errorf("Windows 弹性内存最大内存必须大于基础内存")
	}
	initialKiB := initialMB * 1024
	maxKiB := maxMB * 1024
	expandKiB := maxKiB - initialKiB

	maxMemoryRe := regexp.MustCompile(`(?s)\s*<maxMemory\b[^>]*>.*?</maxMemory>\n?`)

	xmlStr = removeVirtioMemDomainDevices(xmlStr)
	xmlStr = removeMemoryDevicesWithoutModel(xmlStr)
	xmlStr = maxMemoryRe.ReplaceAllString(xmlStr, "\n")
	memoryTag := fmt.Sprintf("<memory unit='KiB'>%d</memory>", initialKiB)
	var replaced bool
	xmlStr, replaced = replaceTopLevelMemoryElement(xmlStr, "memory", memoryTag)
	if !replaced {
		xmlStr = strings.Replace(xmlStr, "<vcpu", memoryTag+"\n  <vcpu", 1)
	}
	currentTag := fmt.Sprintf("<currentMemory unit='KiB'>%d</currentMemory>", initialKiB)
	xmlStr, replaced = replaceTopLevelMemoryElement(xmlStr, "currentMemory", currentTag)
	if !replaced {
		xmlStr = strings.Replace(xmlStr, memoryTag, memoryTag+"\n  "+currentTag, 1)
	}
	xmlStr = strings.Replace(xmlStr, memoryTag, fmt.Sprintf("<maxMemory slots='16' unit='KiB'>%d</maxMemory>\n  %s", maxKiB, memoryTag), 1)
	xmlStr = ensureVirtioMemNumaCell(xmlStr, initialKiB)
	if HookMemoryInjectMemballoonConfig != nil {
		xmlStr = HookMemoryInjectMemballoonConfig(xmlStr, false)
	}

	memoryDevice := fmt.Sprintf(`    <memory model='virtio-mem'>
      <target>
        <size unit='KiB'>%d</size>
        <node>0</node>
        <block unit='KiB'>2048</block>
        <requested unit='KiB'>0</requested>
      </target>
    </memory>`, expandKiB)
	if strings.Contains(xmlStr, "</devices>") {
		xmlStr = strings.Replace(xmlStr, "</devices>", memoryDevice+"\n  </devices>", 1)
	} else {
		return "", fmt.Errorf("虚拟机 XML 缺少 devices 节点")
	}
	return xmlStr, nil
}

func ensureVirtioMemNumaCell(xmlStr string, memoryKiB int) string {
	vcpus := ParseVCPUCount(xmlStr)
	cpus := "0"
	if vcpus > 1 {
		cpus = fmt.Sprintf("0-%d", vcpus-1)
	}
	numaXML := fmt.Sprintf("    <numa>\n      <cell id='0' cpus='%s' memory='%d' unit='KiB'/>\n    </numa>", cpus, memoryKiB)
	cpuSelfRe := regexp.MustCompile(`(?s)<cpu\b([^>]*)/>`)
	if cpuSelfRe.MatchString(xmlStr) {
		return cpuSelfRe.ReplaceAllString(xmlStr, "<cpu$1>\n"+numaXML+"\n  </cpu>")
	}
	cpuBlockRe := regexp.MustCompile(`(?s)<cpu\b[^>]*>.*?</cpu>`)
	return cpuBlockRe.ReplaceAllStringFunc(xmlStr, func(cpuBlock string) string {
		numaRe := regexp.MustCompile(`(?s)\s*<numa>.*?</numa>\n?`)
		cpuBlock = numaRe.ReplaceAllString(cpuBlock, "")
		return strings.Replace(cpuBlock, "</cpu>", numaXML+"\n  </cpu>", 1)
	})
}

func replaceTopLevelMemoryElement(xmlStr, tag, replacement string) (string, bool) {
	limit := strings.Index(xmlStr, "<vcpu")
	if limit < 0 {
		limit = strings.Index(xmlStr, "<devices")
	}
	if limit < 0 {
		limit = len(xmlStr)
	}
	head := xmlStr[:limit]
	tail := xmlStr[limit:]
	re := regexp.MustCompile(fmt.Sprintf(`(?s)<%s\b[^>]*>.*?</%s>`, tag, tag))
	if !re.MatchString(head) {
		return xmlStr, false
	}
	return re.ReplaceAllString(head, replacement) + tail, true
}

func rewriteDevicesMemoryElements(xmlStr string, rewrite func(string) string) string {
	devicesRe := regexp.MustCompile(`(?s)<devices\b[^>]*>.*?</devices>`)
	return devicesRe.ReplaceAllStringFunc(xmlStr, rewrite)
}

func removeVirtioMemDomainDevices(xmlStr string) string {
	modelRe := regexp.MustCompile(`\bmodel=['"]virtio-mem['"]`)
	return removeMemoryDevicesByAttrs(xmlStr, func(attrs string) bool {
		return modelRe.MatchString(attrs)
	})
}

func hasVirtioMemDomainDevice(xmlStr string) bool {
	modelRe := regexp.MustCompile(`\bmodel=['"]virtio-mem['"]`)
	return hasMemoryDeviceByAttrs(xmlStr, func(attrs string) bool {
		return modelRe.MatchString(attrs)
	})
}

func removeMemoryDevicesWithoutModel(xmlStr string) string {
	modelRe := regexp.MustCompile(`\bmodel=['"][^'"]+['"]`)
	return removeMemoryDevicesByAttrs(xmlStr, func(attrs string) bool {
		return !modelRe.MatchString(attrs)
	})
}

func hasMemoryDeviceWithoutModel(xmlStr string) bool {
	modelRe := regexp.MustCompile(`\bmodel=['"][^'"]+['"]`)
	return hasMemoryDeviceByAttrs(xmlStr, func(attrs string) bool {
		return !modelRe.MatchString(attrs)
	})
}

func removeMemoryDevicesByAttrs(xmlStr string, shouldRemove func(string) bool) string {
	memoryDeviceRe := regexp.MustCompile(`(?s)\s*<memory\b([^>]*)>.*?</memory>\n?`)
	return rewriteDevicesMemoryElements(xmlStr, func(devicesBlock string) string {
		return memoryDeviceRe.ReplaceAllStringFunc(devicesBlock, func(memoryBlock string) string {
			matches := memoryDeviceRe.FindStringSubmatch(memoryBlock)
			if len(matches) < 2 || !shouldRemove(matches[1]) {
				return memoryBlock
			}
			return ""
		})
	})
}

func hasMemoryDeviceByAttrs(xmlStr string, match func(string) bool) bool {
	found := false
	memoryDeviceRe := regexp.MustCompile(`(?s)<memory\b([^>]*)>.*?</memory>`)
	rewriteDevicesMemoryElements(xmlStr, func(devicesBlock string) string {
		matches := memoryDeviceRe.FindAllStringSubmatch(devicesBlock, -1)
		for _, item := range matches {
			if len(item) > 1 && match(item[1]) {
				found = true
				break
			}
		}
		return devicesBlock
	})
	return found
}

func removeVirtioMemNumaConfig(xmlStr string) string {
	cpuBlockRe := regexp.MustCompile(`(?s)<cpu\b[^>]*>.*?</cpu>`)
	numaRe := regexp.MustCompile(`(?s)\s*<numa>.*?</numa>\n?`)
	return cpuBlockRe.ReplaceAllStringFunc(xmlStr, func(cpuBlock string) string {
		return numaRe.ReplaceAllString(cpuBlock, "")
	})
}

func removeDomainMaxMemoryElement(xmlStr string) string {
	maxMemoryRe := regexp.MustCompile(`(?s)\s*<maxMemory\b[^>]*>.*?</maxMemory>\n?`)
	return maxMemoryRe.ReplaceAllString(xmlStr, "\n")
}

// ParseVCPUCount 从 XML 中解析 vCPU 数量。
func ParseVCPUCount(xmlStr string) int {
	re := regexp.MustCompile(`(?s)<vcpu\b[^>]*>(.*?)</vcpu>`)
	m := re.FindStringSubmatch(xmlStr)
	if len(m) < 2 {
		return 1
	}
	value, err := strconv.Atoi(strings.TrimSpace(m[1]))
	if err != nil || value <= 0 {
		return 1
	}
	return value
}

// ApplyStaticMemoryConfigToDomainXML 将最大内存和启动内存恢复为同一个静态值。
func ApplyStaticMemoryConfigToDomainXML(xmlStr string, memoryMB int) (string, error) {
	got, err := ApplyDynamicMemoryConfigToDomainXML(xmlStr, memoryMB, memoryMB, false)
	if err != nil {
		return "", err
	}
	got = removeDomainMaxMemoryElement(got)
	got = removeVirtioMemDomainDevices(got)
	got = removeMemoryDevicesWithoutModel(got)
	return got, nil
}

// ParseDomainMemoryXML 从 XML 解析内存值。
func ParseDomainMemoryXML(xmlStr string) VMMemoryXMLValues {
	return VMMemoryXMLValues{
		MemoryMB:        parseMemoryTagMB(xmlStr, "memory"),
		CurrentMemoryMB: parseMemoryTagMB(xmlStr, "currentMemory"),
	}
}

func parseMemoryTagMB(xmlStr, tag string) int {
	if tag == "memory" || tag == "currentMemory" {
		limit := strings.Index(xmlStr, "<vcpu")
		if limit < 0 {
			limit = strings.Index(xmlStr, "<devices")
		}
		if limit > 0 {
			xmlStr = xmlStr[:limit]
		}
	}
	re := regexp.MustCompile(fmt.Sprintf(`(?s)<%s\b([^>]*)>(.*?)</%s>`, tag, tag))
	m := re.FindStringSubmatch(xmlStr)
	if len(m) < 3 {
		return 0
	}
	unit := "KiB"
	unitRe := regexp.MustCompile(`unit=['"]([^'"]+)['"]`)
	if um := unitRe.FindStringSubmatch(m[1]); len(um) > 1 {
		unit = um[1]
	}
	value, _ := strconv.ParseFloat(strings.TrimSpace(m[2]), 64)
	switch strings.ToLower(unit) {
	case "b", "bytes":
		return int(value / 1024 / 1024)
	case "kb", "k", "kib":
		return int(value / 1024)
	case "mb", "m", "mib":
		return int(value)
	case "gb", "g", "gib":
		return int(value * 1024)
	default:
		return int(value / 1024)
	}
}

// ParseVirtioMemCurrentMB 从 XML 解析 virtio-mem 当前大小（MB）。
func ParseVirtioMemCurrentMB(xmlStr string) int {
	deviceRe := regexp.MustCompile(`(?s)<memory\s+model=['"]virtio-mem['"][^>]*>(.*?)</memory>`)
	m := deviceRe.FindStringSubmatch(xmlStr)
	if len(m) < 2 {
		return 0
	}
	currentMB := parseMemoryTagMB(m[1], "current")
	if currentMB > 0 {
		return currentMB
	}
	return parseMemoryTagMB(m[1], "requested")
}

func parseVirtioMemRequestedMB(xmlStr string) int {
	deviceRe := regexp.MustCompile(`(?s)<memory\s+model=['"]virtio-mem['"][^>]*>(.*?)</memory>`)
	m := deviceRe.FindStringSubmatch(xmlStr)
	if len(m) < 2 {
		return 0
	}
	return parseMemoryTagMB(m[1], "requested")
}

func findVirtioMemAlias(xmlStr string) string {
	deviceRe := regexp.MustCompile(`(?s)<memory\s+model=['"]virtio-mem['"][^>]*>.*?</memory>`)
	device := deviceRe.FindString(xmlStr)
	if device == "" {
		return ""
	}
	aliasRe := regexp.MustCompile(`<alias\s+name=['"]([^'"]+)['"]\s*/>`)
	if m := aliasRe.FindStringSubmatch(device); len(m) > 1 {
		return m[1]
	}
	return ""
}

// HasUsableMemballoon 判断 XML 中是否有可用的 memballoon。
func HasUsableMemballoon(xmlStr string) bool {
	if !strings.Contains(xmlStr, "<memballoon") {
		return false
	}
	return !strings.Contains(xmlStr, "model='none'") && !strings.Contains(xmlStr, `model="none"`)
}

func getVMMemoryStats(name string) (*vmMemoryStatsValues, error) {
	statsMap, err := libvirt_rpc.GetDomainMemoryStatsRPC(name)
	if err != nil {
		return nil, err
	}
	stats := &vmMemoryStatsValues{
		ActualKB:    int64(statsMap["actual"]),
		UnusedKB:    int64(statsMap["unused"]),
		UsableKB:    int64(statsMap["usable"]),
		AvailableKB: int64(statsMap["available"]),
		RSSKB:       int64(statsMap["rss"]),
	}
	return stats, nil
}
