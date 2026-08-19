package handler

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/logger"
)

// logReadTailBlockSize 从文件尾部向前扫描时的块大小
const logReadTailBlockSize = 64 * 1024

// logReadMaxContentBytes 单次读取返回的内容硬上限（防止极端大行数撑爆响应）
const logReadMaxContentBytes = 16 * 1024 * 1024

// readLogTail 从 startPos（<=0 表示文件末尾）向前读取最多 lines 行内容。
// 返回内容字节与新起点 prevOffset（内容起点之前的字节位置，0 表示已到文件头）。
// 不会将整个文件读入内存，仅按块反向扫描。
func readLogTail(f *os.File, startPos int64, lines int) ([]byte, int64, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := st.Size()
	if startPos <= 0 || startPos > size {
		startPos = size
	}
	if startPos <= 0 {
		return []byte{}, 0, nil
	}

	buf := make([]byte, logReadTailBlockSize)
	pos := startPos
	newlineCount := 0
	contentStart := startPos

	for pos > 0 {
		readLen := int64(len(buf))
		if pos < readLen {
			readLen = pos
		}
		if _, err := f.ReadAt(buf[:readLen], pos-readLen); err != nil && err != io.EOF {
			return nil, 0, err
		}
		// 从块尾向块头扫描换行符，数够 lines+1 个即可确定内容起点
		for i := readLen - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				newlineCount++
				if newlineCount > lines {
					contentStart = pos - readLen + i + 1
					pos = 0
					break
				}
			}
		}
		if pos == 0 {
			break
		}
		pos -= readLen
	}

	// 扫描过程中未数够 lines+1 个换行，说明整个文件不超过 lines 行，起点回到文件头
	if newlineCount <= lines {
		contentStart = 0
	}
	if contentStart < 0 {
		contentStart = 0
	}
	if contentStart >= startPos {
		// 没有可读内容（文件为空或起始位置无内容）
		return []byte{}, contentStart, nil
	}

	content := make([]byte, startPos-contentStart)
	if _, err := f.ReadAt(content, contentStart); err != nil && err != io.EOF {
		return nil, 0, err
	}
	return content, contentStart, nil
}

// ReadLogFile 读取日志文件内容（在线预览，向前分页）。
//   - file: 日志文件名（仅 .log，拒绝路径穿越；.log.gz 压缩归档不支持预览）
//   - lines: 每次读取行数（默认 200，上限 1000）
//   - offset: 上一页返回的 prev_offset；缺省/<=0 时从文件末尾读取
func ReadLogFile(c *gin.Context) {
	name := strings.TrimSpace(c.Query("file"))

	lines := 200
	if v := strings.TrimSpace(c.Query("lines")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			lines = n
		}
	}
	if lines > 1000 {
		lines = 1000
	}

	var offset int64
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			offset = n
		}
	}

	// 安全校验：仅允许 logDir 下的 .log 文件，拒绝路径穿越与压缩归档
	baseName := filepath.Base(name)
	if baseName != name {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "文件名不合法"})
		return
	}
	if !strings.HasSuffix(baseName, ".log") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "仅支持查看 .log 日志文件"})
		return
	}
	if strings.HasSuffix(baseName, ".gz") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "压缩归档日志不支持在线预览，请下载后查看"})
		return
	}

	logDir := logger.GetLogDir()
	if logDir == "" {
		if config.GlobalConfig != nil {
			logDir = config.GlobalConfig.LogDir
		}
	}

	f, err := os.Open(filepath.Join(logDir, baseName))
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "日志文件不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取日志文件失败"})
		return
	}
	defer f.Close()

	content, prevOffset, err := readLogTail(f, offset, lines)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取日志文件失败"})
		return
	}
	if len(content) > logReadMaxContentBytes {
		content = content[:logReadMaxContentBytes]
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"name":        baseName,
			"content":     string(content),
			"lines":       bytes.Count(content, []byte{'\n'}),
			"prev_offset": prevOffset,
			"eof":         prevOffset <= 0,
		},
	})
}