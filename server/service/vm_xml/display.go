package vm_xml

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	VMVideoModelVirtio = "virtio"
	VMVideoModelVGA    = "vga"
	VMVideoModelVMVGA  = "vmvga"
	VMVideoModelCirrus = "cirrus"
	VMVideoModelRamfb  = "ramfb"
)

var (
	vmVideoBlockRegexp       = regexp.MustCompile(`(?s)<video>.*?</video>`)
	vmVideoModelRegexp       = regexp.MustCompile(`<video\b[^>]*>\s*<model\b[^>]*type=['"]([^'"]+)['"]`)
	vmHyperVBlockRegexp      = regexp.MustCompile(`(?s)<hyperv\b[^>]*>.*?</hyperv>`)
	vmClockBlockRegexp       = regexp.MustCompile(`(?s)<clock\b[^>]*(?:/>|>.*?</clock>)`)
	vmSelfClosingClockRegexp = regexp.MustCompile(`^<clock\b[^>]*/>$`)
	vmHyperVClockTimerRegexp = regexp.MustCompile(`<timer\b[^>]*\bname=['"]hypervclock['"][^>]*/>`)
)

// ResolveVMVideoModel 规范化视频模型，并根据系统类型和架构给出默认值。
// arch 为目标架构（x86_64/aarch64/riscv64），空字符串表示不启用架构感知默认。
func ResolveVMVideoModel(videoModel, osType, arch string) string {
	normalized := strings.ToLower(strings.TrimSpace(videoModel))
	switch normalized {
	case VMVideoModelVirtio, VMVideoModelVGA, VMVideoModelVMVGA, VMVideoModelCirrus, VMVideoModelRamfb:
		return normalized
	}

	// ARM 架构默认使用 ramfb，避免 virtio/vga 在 aarch64 下的兼容性问题
	if strings.EqualFold(strings.TrimSpace(arch), "aarch64") {
		return VMVideoModelRamfb
	}

	if strings.EqualFold(strings.TrimSpace(osType), "windows") {
		return VMVideoModelVGA
	}
	return VMVideoModelVirtio
}

// ParseVMVideoModelFromDomainXML 从 domain XML 中解析当前视频模型。
func ParseVMVideoModelFromDomainXML(xmlStr string) string {
	matches := vmVideoModelRegexp.FindStringSubmatch(xmlStr)
	if len(matches) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(matches[1]))
}

func renderVMVideoBlock(videoModel string) string {
	modelXML := "<model type='vga'/>"
	switch ResolveVMVideoModel(videoModel, "", "") {
	case VMVideoModelVirtio:
		modelXML = "<model type='virtio' heads='1' primary='yes'/>"
	case VMVideoModelVMVGA:
		modelXML = "<model type='vmvga'/>"
	case VMVideoModelCirrus:
		modelXML = "<model type='cirrus'/>"
	case VMVideoModelRamfb:
		modelXML = "<model type='ramfb'/>"
	}

	return fmt.Sprintf("    <video>\n      %s\n    </video>", modelXML)
}

func renderWindowsHyperVBlock() string {
	return `    <hyperv mode='custom'>
      <relaxed state='on'/>
      <vapic state='on'/>
      <spinlocks state='on' retries='8191'/>
      <vpindex state='on'/>
      <runtime state='on'/>
      <synic state='on'/>
      <stimer state='on'>
        <direct state='on'/>
      </stimer>
      <frequencies state='on'/>
      <tlbflush state='on'>
        <direct state='on'/>
        <extended state='on'/>
      </tlbflush>
      <ipi state='on'/>
    </hyperv>`
}

func ensureWindowsHyperVClockTimer(xmlStr string) string {
	if vmHyperVClockTimerRegexp.MatchString(xmlStr) {
		return xmlStr
	}

	const timerTag = "<timer name='hypervclock' present='yes'/>"

	if vmClockBlockRegexp.MatchString(xmlStr) {
		return vmClockBlockRegexp.ReplaceAllStringFunc(xmlStr, func(clockBlock string) string {
			indent := leadingWhitespace(clockBlock)
			childIndent := indent + "  "
			trimmed := strings.TrimSpace(clockBlock)

			if vmSelfClosingClockRegexp.MatchString(trimmed) {
				openTag := strings.TrimSuffix(trimmed, "/>")
				openTag = strings.TrimRight(openTag, " ")
				return fmt.Sprintf("%s%s>\n%s%s\n%s</clock>", indent, openTag, childIndent, timerTag, indent)
			}

			closingLine := "\n" + indent + "</clock>"
			insertedClosingLine := "\n" + childIndent + timerTag + closingLine
			if strings.Contains(clockBlock, closingLine) {
				return strings.Replace(clockBlock, closingLine, insertedClosingLine, 1)
			}
			return strings.Replace(clockBlock, "</clock>", "\n"+childIndent+timerTag+"\n"+indent+"</clock>", 1)
		})
	}

	clockXML := "  <clock offset='localtime'>\n    " + timerTag + "\n  </clock>\n"
	if strings.Contains(xmlStr, "<on_poweroff>") {
		return strings.Replace(xmlStr, "<on_poweroff>", clockXML+"  <on_poweroff>", 1)
	}
	if strings.Contains(xmlStr, "<devices>") {
		return strings.Replace(xmlStr, "<devices>", clockXML+"  <devices>", 1)
	}
	if strings.Contains(xmlStr, "</features>") {
		return strings.Replace(xmlStr, "</features>", "</features>\n"+clockXML, 1)
	}
	return xmlStr
}

// ApplyVMVideoModelToDomainXML 将视频模型写入 domain XML。
func ApplyVMVideoModelToDomainXML(xmlStr, videoModel, osType string) string {
	arch := ParseVMArchFromDomainXML(xmlStr)
	block := renderVMVideoBlock(ResolveVMVideoModel(videoModel, osType, arch))
	if vmVideoBlockRegexp.MatchString(xmlStr) {
		return vmVideoBlockRegexp.ReplaceAllString(xmlStr, block)
	}
	if strings.Contains(xmlStr, "</devices>") {
		return strings.Replace(xmlStr, "</devices>", block+"\n  </devices>", 1)
	}
	return xmlStr
}

// ApplyWindowsGuestOptimizationsToDomainXML 为 Windows 来宾补充更完整的 Hyper-V 优化配置。
// 仅在 x86_64 架构上应用 Hyper-V enlightenments；ARM/RISC-V 不支持这些特性，
// 若 XML 中已包含 hyperv 块则移除。
func ApplyWindowsGuestOptimizationsToDomainXML(xmlStr string) string {
	// 解析架构，非 x86_64 架构不注入 Hyper-V 优化
	vmArch := ParseVMArchFromDomainXML(xmlStr)
	if vmArch != "" && vmArch != "x86_64" {
		// 移除已有的 hyperv 块和 hypervclock 定时器
		xmlStr = vmHyperVBlockRegexp.ReplaceAllString(xmlStr, "")
		xmlStr = removeHyperVClockTimer(xmlStr)
		return xmlStr
	}

	block := renderWindowsHyperVBlock()
	if vmHyperVBlockRegexp.MatchString(xmlStr) {
		return ensureWindowsHyperVClockTimer(vmHyperVBlockRegexp.ReplaceAllString(xmlStr, block))
	}
	if strings.Contains(xmlStr, "</features>") {
		return ensureWindowsHyperVClockTimer(strings.Replace(xmlStr, "</features>", block+"\n  </features>", 1))
	}
	return ensureWindowsHyperVClockTimer(xmlStr)
}

// removeHyperVClockTimer 从 XML 中移除 hypervclock 定时器
func removeHyperVClockTimer(xmlStr string) string {
	return vmHyperVClockTimerRegexp.ReplaceAllString(xmlStr, "")
}

func leadingWhitespace(value string) string {
	for i, r := range value {
		if r != ' ' && r != '\t' {
			return value[:i]
		}
	}
	return ""
}
