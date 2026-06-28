package spice

import (
	"fmt"
	"regexp"
	"strings"

	"kvm_console/service/arch"
)

// graphics.go — SPICE graphics 块与 QXL video 的 XML 注入/改写。
// 这是创建/克隆/导入链路以及运行时 EnableSpice/ExposeSpice 共用的核心 helper。

// spiceGraphicsBlockRe 匹配整块 <graphics type='spice' ...>...</graphics>
var spiceGraphicsBlockRe = regexp.MustCompile(`(?s)<graphics\s+type='spice'[^>]*>.*?</graphics>`)

// spiceVideoBlockRe 匹配整块 <video>...</video>
var spiceVideoBlockRe = regexp.MustCompile(`(?s)<video\b[^>]*>.*?</video>`)

// spiceVideoModelRe 匹配 <video> 内的 <model type='...' .../>
var spiceVideoModelRe = regexp.MustCompile(`(?s)<model\s+type='[^']*'[^>]*/?>`)

// buildSpiceGraphicsXML 构造一个 SPICE graphics 块。listenAddr 通常为 127.0.0.1 或 0.0.0.0。
func buildSpiceGraphicsXML(listenAddr, passwd string) string {
	passwdAttr := ""
	if passwd != "" {
		passwdAttr = fmt.Sprintf(" passwd='%s'", passwd)
	}
	return fmt.Sprintf(
		"<graphics type='spice' port='-1' autoport='yes' listen='%s'%s>\n"+
			"      <listen type='address' address='%s'/>\n"+
			"    </graphics>",
		listenAddr, passwdAttr, listenAddr)
}

// InjectSPICEGraphicsToDomainXML 注入或替换 SPICE graphics 块。
//   - 已存在 spice 块：整体替换为新的（保持最新 listen/passwd）
//   - 不存在：插入到 </devices> 前
func InjectSPICEGraphicsToDomainXML(xmlStr, passwd, listenAddr string) string {
	if listenAddr == "" {
		listenAddr = "127.0.0.1"
	}
	newBlock := buildSpiceGraphicsXML(listenAddr, passwd)
	if spiceGraphicsBlockRe.MatchString(xmlStr) {
		return spiceGraphicsBlockRe.ReplaceAllString(xmlStr, newBlock)
	}
	return strings.Replace(xmlStr, "</devices>", newBlock+"\n    </devices>", 1)
}

// spiceAudioRe 匹配 SPICE 音频后端元素（自闭合或带内容块）。移除 SPICE graphics 时需一并移除，
// 否则 libvirt 会因 "Spice audio is not supported without spice graphics" 拒绝定义域。
var spiceAudioRe = regexp.MustCompile(`(?s)<audio\s+[^>]*?type\s*=\s*['"]spice['"][^>]*?(?:/>|>.*?</audio>)`)

// RemoveSPICEGraphicsFromDomainXML 移除 SPICE graphics 块及关联的 spice 音频后端（关闭 SPICE 用）。
func RemoveSPICEGraphicsFromDomainXML(xmlStr string) string {
	// 删除 graphics 整块及其后的换行，避免留下空行
	out := spiceGraphicsBlockRe.ReplaceAllString(xmlStr, "")
	// 一并移除 spice 音频后端，避免 libvirt 拒绝定义（spice audio 依赖 spice graphics）
	out = spiceAudioRe.ReplaceAllString(out, "")
	return out
}

// HasSPICEGraphics 判断 XML 是否已含 SPICE graphics。
func HasSPICEGraphics(xmlStr string) bool {
	return spiceGraphicsBlockRe.MatchString(xmlStr)
}

// spiceListenAttrRe 匹配 graphics type='spice' 标签上的 listen='...' 属性
var spiceListenAttrRe = regexp.MustCompile(`(<graphics\s+type='spice'[^>]*?)listen='[^']*'`)

// spiceListenAddressRe 匹配 <listen type='address' .../> 内的 address='...'（仅 spice 块相关行）
var spiceListenAddressRe = regexp.MustCompile(`(<listen\s+type='address'[^>]*?)address='[^']*'`)

// SetSPICEListenAddress 仅修改 SPICE 块的监听地址（保留 vnc 块不动）。
// 由于 vnc 与 spice 的 listen 行结构相同，需先按 spice 块范围处理：
// 这里采取"替换 spice 块整体"的方式以保证精确，避免误改 vnc。
func SetSPICEListenAddress(xmlStr, listenAddr string) string {
	// 找到 spice 块，提取其 passwd 后整体重建
	if !spiceGraphicsBlockRe.MatchString(xmlStr) {
		return xmlStr
	}
	block := spiceGraphicsBlockRe.FindString(xmlStr)
	passwd := extractSPICEPasswd(block)
	return spiceGraphicsBlockRe.ReplaceAllString(xmlStr, buildSpiceGraphicsXML(listenAddr, passwd))
}

// spicePasswdAttrRe 匹配 passwd='...'
var spicePasswdAttrRe = regexp.MustCompile(`passwd='([^']*)'`)

func extractSPICEPasswd(block string) string {
	m := spicePasswdAttrRe.FindStringSubmatch(block)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// EnsureQXLVideo 确保 domain 有 QXL video 模型（SPICE 最佳搭档）。
//   - 已有 <video>：把其中 <model type=.../> 改为 qxl
//   - 无 <video>：在 </devices> 前插入 <video><model type='qxl' heads='1'/></video>
//
// ARM64 (aarch64) 架构不支持 QXL 显卡模型，直接返回原 XML 不做更改。
func EnsureQXLVideo(xmlStr string) string {
	if arch.IsHostArch(arch.ArchAarch64) {
		return xmlStr
	}
	if spiceVideoBlockRe.MatchString(xmlStr) {
		// 已有 video 块，替换其 model 为 qxl
		return spiceVideoBlockRe.ReplaceAllStringFunc(xmlStr, func(videoBlock string) string {
			if spiceVideoModelRe.MatchString(videoBlock) {
				return spiceVideoModelRe.ReplaceAllString(videoBlock, "<model type='qxl' heads='1'/>")
			}
			// video 块存在但无 model，补一个
			return strings.Replace(videoBlock, "</video>", "<model type='qxl' heads='1'/>\n    </video>", 1)
		})
	}
	qxl := "<video>\n      <model type='qxl' heads='1'/>\n    </video>"
	return strings.Replace(xmlStr, "</devices>", qxl+"\n    </devices>", 1)
}
