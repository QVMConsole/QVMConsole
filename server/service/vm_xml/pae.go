package vm_xml

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	vmPAERegex          = regexp.MustCompile(`(?s)\n?\s*<pae\b[^>]*/>`)
	vmDomainArchRegex   = regexp.MustCompile(`<type\b[^>]*\barch=['"]([^'"]+)['"]`)
	vmFeaturesBlockExpr = regexp.MustCompile(`(?s)<features\b[^>]*>.*?</features>`)
)

// ResolveVMPAEEnabled 解析 PAE 最终状态，未显式传入时默认启用。
func ResolveVMPAEEnabled(enabled *bool) bool {
	if enabled == nil {
		return true
	}
	return *enabled
}

// ParseVMPAEFromDomainXML 从 domain XML 中解析 PAE 是否启用。
func ParseVMPAEFromDomainXML(xmlContent string) bool {
	return vmPAERegex.MatchString(xmlContent)
}

func extractDomainArchFromXML(xmlContent string) string {
	matches := vmDomainArchRegex.FindStringSubmatch(xmlContent)
	if len(matches) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(matches[1]))
}

func supportsVMPAEForArch(arch string) bool {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "", "x86_64", "i686", "i586", "i486", "i386":
		return true
	default:
		return false
	}
}

func supportsVMPAEForDomainXML(xmlContent string) bool {
	return supportsVMPAEForArch(extractDomainArchFromXML(xmlContent))
}

// ApplyVMPAEToDomainXML 将 PAE 开关写入 domain XML。
func ApplyVMPAEToDomainXML(xmlContent string, enabled *bool) (string, error) {
	updated := vmPAERegex.ReplaceAllString(xmlContent, "")
	if !supportsVMPAEForDomainXML(updated) {
		return updated, nil
	}
	if !ResolveVMPAEEnabled(enabled) {
		return updated, nil
	}

	if vmFeaturesBlockExpr.MatchString(updated) {
		return vmFeaturesBlockExpr.ReplaceAllStringFunc(updated, func(block string) string {
			return strings.Replace(block, "</features>", "    <pae/>\n  </features>", 1)
		}), nil
	}

	featuresXML := "  <features>\n    <pae/>\n  </features>\n"
	switch {
	case strings.Contains(updated, "<clock "):
		return strings.Replace(updated, "<clock ", featuresXML+"  <clock ", 1), nil
	case strings.Contains(updated, "<clock>"):
		return strings.Replace(updated, "<clock>", featuresXML+"  <clock>", 1), nil
	case strings.Contains(updated, "<devices/>"):
		return strings.Replace(updated, "<devices/>", featuresXML+"  <devices/>", 1), nil
	case strings.Contains(updated, "<devices />"):
		return strings.Replace(updated, "<devices />", featuresXML+"  <devices />", 1), nil
	case strings.Contains(updated, "<devices>"):
		return strings.Replace(updated, "<devices>", featuresXML+"  <devices>", 1), nil
	case strings.Contains(updated, "<on_poweroff>"):
		return strings.Replace(updated, "<on_poweroff>", featuresXML+"  <on_poweroff>", 1), nil
	default:
		return "", fmt.Errorf("写入 PAE 配置失败：未找到可插入 features 的位置")
	}
}
