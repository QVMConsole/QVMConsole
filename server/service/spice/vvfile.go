package spice

import (
	"fmt"
	"strings"
)

// vvfile.go — 生成 SPICE .vv 连接文件（virt-viewer / spicy 原生格式）。
// 用户下载后双击即可被系统注册的 virt-viewer 打开并直连。

// BuildVVFile 生成 .vv 文件内容。deleteThisFile=true 时写入 delete-this-file=1，
// virt-viewer 打开连接后会自动删除该文件（一次性，避免凭据留存）；false 时保留文件、可重复使用。
func BuildVVFile(info *SpiceConnInfo, vmName string, deleteThisFile bool) string {
	var b strings.Builder
	b.WriteString("[virt-viewer]\n")
	b.WriteString("type=spice\n")
	b.WriteString(fmt.Sprintf("host=%s\n", info.Host))
	if info.Port != "" {
		b.WriteString(fmt.Sprintf("port=%s\n", info.Port))
	}
	if info.TLSPort != "" {
		b.WriteString(fmt.Sprintf("tls-port=%s\n", info.TLSPort))
	}
	if info.Password != "" {
		b.WriteString(fmt.Sprintf("password=%s\n", info.Password))
	}
	b.WriteString(fmt.Sprintf("title=%s (SPICE)\n", vmName))
	b.WriteString("enable-smartcard=0\n")
	if deleteThisFile {
		b.WriteString("delete-this-file=1\n")
	} else {
		b.WriteString("delete-this-file=0\n")
	}
	return b.String()
}
