// Package upload 实现分片上传的纯引擎：分片落盘、断点续传状态、秒传判断、整文件校验。
// 不依赖任何业务子包——目标目录计算与配额检查由调用方（service/upload_wire.go）完成后传入。
package upload

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/utils"

	"gorm.io/gorm"
)

const (
	// DefaultChunkSize 默认分片大小 1MB
	DefaultChunkSize = 1 << 20
	// SessionTTL 会话无活动过期时间
	SessionTTL = 24 * time.Hour
	// cleanupInterval 过期会话清理间隔
	cleanupInterval = 30 * time.Minute

	// 抽样哈希参数（与前端 chunkUploader.ts 的 SAMPLE_WINDOW/STRIDE/MIN_SAMPLES 逐位一致）
	sampleWindow = 2 << 20 // 2MB
	sampleStride = 1 << 30 // 1GB
	minSamples   = 3
)

// 引擎层错误
var (
	ErrInvalidPath      = errors.New("非法的上传路径")
	ErrSessionNotFound  = errors.New("上传会话不存在或已过期，请重新开始上传")
	ErrForbidden        = errors.New("无权操作该上传会话")
	ErrNotUploading     = errors.New("上传会话不在上传中状态")
	ErrChunksIncomplete = errors.New("分片未全部上传完成")
	ErrHashMismatch     = errors.New("文件校验失败：哈希不匹配，请重新上传")
	ErrMissingHash      = errors.New("缺少文件哈希")
)

// InitParams 初始化上传会话参数
type InitParams struct {
	FilePath   string // 目标文件绝对路径（主键 / 续传凭证）
	AllowedDir string // 允许的目录前缀（路径越权校验）
	Username   string
	Category   string
	FileName   string
	TotalSize  int64
	ChunkSize  int
	FileHash   string // 整文件 MD5
}

// InitResult 初始化结果
type InitResult struct {
	SessionKey    string // = FilePath
	TotalChunks   int
	ChunkSize     int
	Received      []int // 已收分片索引（续传时前端据此跳过）
	UploadedBytes int64
	Instant       bool // 秒传命中
	Completed     bool // 已完成（秒传）
}

// StatusResult 会话状态查询结果
type StatusResult struct {
	Exists        bool
	Status        string
	TotalChunks   int
	ChunkSize     int
	Received      []int
	UploadedBytes int64
}

// ---- per-file 互斥锁：保护位图 read-modify-write 与 complete/chunk 竞争 ----

var (
	fileLocksMu sync.Mutex
	fileLocks   = map[string]*sync.Mutex{}
)

func lockFor(filePath string) *sync.Mutex {
	fileLocksMu.Lock()
	defer fileLocksMu.Unlock()
	mu, ok := fileLocks[filePath]
	if !ok {
		mu = &sync.Mutex{}
		fileLocks[filePath] = mu
	}
	return mu
}

// ---- 路径与分片辅助 ----

func validatePath(filePath, allowedDir string) error {
	if filePath == "" || !filepath.IsAbs(filePath) || allowedDir == "" {
		return ErrInvalidPath
	}
	clean := filepath.Clean(filePath)
	allowed := filepath.Clean(allowedDir)
	// filePath 必须严格位于 allowedDir 子路径下，防穿越/越权
	if !strings.HasPrefix(clean, allowed+string(filepath.Separator)) {
		return ErrInvalidPath
	}
	return nil
}

func totalChunksOf(totalSize int64, chunkSize int) int {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if totalSize <= 0 {
		return 0
	}
	return int((totalSize + int64(chunkSize) - 1) / int64(chunkSize))
}

// ---- 位图（big.Int 十六进制文本）----

func bmpDecode(s string) *big.Int {
	b := new(big.Int)
	if s != "" {
		// 必须显式按十六进制解析：big.Int.UnmarshalText 对无前缀字符串默认按十进制，
		// 会导致 bmpEncode 产出的 "ff"/"1ff" 等解析失败、位图归零（只记得最后一片）。
		b.SetString(s, 16)
	}
	return b
}

func bmpEncode(b *big.Int) string {
	if b == nil || b.Sign() == 0 {
		return ""
	}
	return b.Text(16)
}

func bmpReceivedList(b *big.Int, total int) []int {
	out := make([]int, 0, total)
	for i := 0; i < total; i++ {
		if b.Bit(i) == 1 {
			out = append(out, i)
		}
	}
	return out
}

// ---- 文件操作 ----

// ensureFile 创建文件并按 totalSize 稀疏预分配（truncate），不占实际空间。
func ensureFile(filePath string, totalSize int64) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()
	if err := f.Truncate(totalSize); err != nil {
		return fmt.Errorf("预分配文件失败: %w", err)
	}
	return nil
}

// sampleOffsets 计算抽样偏移量集合（升序、去重），与前端 sampleOffsets 逐位一致：
// 恒含头部 0 与尾部 size-2MB，每 1GB 加一窗口，不足 3 则补中点；不足 3 窗口的小文件回退为单窗口。
func sampleOffsets(size int64) []int64 {
	if size <= 0 {
		return nil
	}
	if size <= int64(sampleWindow)*int64(minSamples) {
		return []int64{0}
	}
	set := make(map[int64]struct{})
	set[0] = struct{}{}
	for o := int64(sampleStride); o+int64(sampleWindow) <= size; o += int64(sampleStride) {
		set[o] = struct{}{}
	}
	set[size-int64(sampleWindow)] = struct{}{}
	if len(set) < minSamples {
		set[size/2-int64(sampleWindow)/2] = struct{}{}
	}
	out := make([]int64, 0, len(set))
	for o := range set {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// sampleFileHash 按抽样窗口读取文件并计算 MD5，末尾追加 "<size>|<fileName>"。
// 与前端 calcFileSampleHash 逐字节一致，供 complete 校验与 init 既有文件秒传复算。
func sampleFileHash(filePath string, totalSize int64, fileName string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	buf := make([]byte, sampleWindow)
	for _, o := range sampleOffsets(totalSize) {
		end := o + int64(sampleWindow)
		if end > totalSize {
			end = totalSize
		}
		want := end - o
		n, err := f.ReadAt(buf[:want], o)
		if err != nil || int64(n) != want {
			return "", fmt.Errorf("读取抽样窗口失败 offset=%d want=%d got=%d: %w", o, want, n, err)
		}
		h.Write(buf[:want])
	}
	h.Write([]byte(fmt.Sprintf("%d|%s", totalSize, fileName)))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// saveSession 以 FilePath 为主键 upsert 会话全字段（CreatedAt/UpdatedAt 由 GORM 自动维护）
func saveSession(p InitParams, chunkSize, totalChunks int, bmp *big.Int, uploaded int64, status string, expiresAt time.Time) {
	sess := model.UploadSession{
		FilePath:      p.FilePath,
		Username:      p.Username,
		Category:      p.Category,
		FileName:      p.FileName,
		TotalSize:     p.TotalSize,
		ChunkSize:     chunkSize,
		TotalChunks:   totalChunks,
		FileHash:      p.FileHash,
		ReceivedBmp:   bmpEncode(bmp),
		UploadedBytes: uploaded,
		Status:        status,
		ExpiresAt:     expiresAt,
	}
	if err := model.DB.Save(&sess).Error; err != nil {
		logger.App.Error("保存上传会话失败", "path", p.FilePath, "error", err)
	}
}

// InitUpload 初始化/恢复上传会话，并做秒传判断。
// 规则：
//  1. 会话已完成且 hash 与文件大小匹配 → 秒传
//  2. 会话上传中且文件大小匹配 → 续传，返回已收分片
//  3. 文件已完整存在（大小匹配）但无有效会话 → 校验 MD5 命中则秒传
//  4. 其余（文件不存在 / 大小不符 / 会话失效）→ 重建：清空进度，重新预分配
func InitUpload(p InitParams) (*InitResult, error) {
	if err := validatePath(p.FilePath, p.AllowedDir); err != nil {
		return nil, err
	}
	if p.Username == "" {
		return nil, errors.New("缺少用户名")
	}
	if p.FileHash == "" {
		return nil, ErrMissingHash
	}
	chunkSize := p.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	totalChunks := totalChunksOf(p.TotalSize, chunkSize)
	expiresAt := time.Now().Add(SessionTTL)

	var sess model.UploadSession
	hasSess := model.DB.Where("file_path = ?", p.FilePath).First(&sess).Error == nil

	fi, statErr := os.Stat(p.FilePath)
	fileExists := statErr == nil && !fi.IsDir()
	fileSizeMatch := fileExists && fi.Size() == p.TotalSize

	mu := lockFor(p.FilePath)
	mu.Lock()
	defer mu.Unlock()

	empty := []int{}

	// 1) 秒传：会话已完成 + hash 匹配 + 文件大小匹配
	if hasSess && sess.Status == model.UploadStatusCompleted && sess.FileHash == p.FileHash && fileSizeMatch {
		return &InitResult{
			SessionKey: p.FilePath, TotalChunks: totalChunks, ChunkSize: chunkSize,
			Received: empty, UploadedBytes: p.TotalSize, Instant: true, Completed: true,
		}, nil
	}

	// 2) 续传：会话上传中 + 文件大小匹配 + hash 匹配（hash 不符说明同名不同内容，应重建而非续传）
	if hasSess && sess.Status == model.UploadStatusUploading && fileSizeMatch && sess.FileHash == p.FileHash {
		bmp := bmpDecode(sess.ReceivedBmp)
		saveSession(p, chunkSize, totalChunks, bmp, sess.UploadedBytes, model.UploadStatusUploading, expiresAt)
		return &InitResult{
			SessionKey: p.FilePath, TotalChunks: totalChunks, ChunkSize: chunkSize,
			Received: bmpReceivedList(bmp, totalChunks), UploadedBytes: sess.UploadedBytes,
			Instant: false, Completed: false,
		}, nil
	}

	// 3) 文件完整存在但无有效会话：复算抽样哈希判定是否可秒传（避免覆盖已存在文件）
	if fileSizeMatch {
		if h, err := sampleFileHash(p.FilePath, p.TotalSize, p.FileName); err == nil && h == p.FileHash {
			saveSession(p, chunkSize, totalChunks, new(big.Int), p.TotalSize, model.UploadStatusCompleted, expiresAt)
			return &InitResult{
				SessionKey: p.FilePath, TotalChunks: totalChunks, ChunkSize: chunkSize,
				Received: empty, UploadedBytes: p.TotalSize, Instant: true, Completed: true,
			}, nil
		}
	}

	// 4) 新建/重建：清旧会话 + 预分配文件 + 建 uploading 会话
	if hasSess {
		model.DB.Where("file_path = ?", p.FilePath).Delete(&model.UploadSession{})
	}
	if err := ensureFile(p.FilePath, p.TotalSize); err != nil {
		return nil, err
	}
	saveSession(p, chunkSize, totalChunks, new(big.Int), 0, model.UploadStatusUploading, expiresAt)
	return &InitResult{
		SessionKey: p.FilePath, TotalChunks: totalChunks, ChunkSize: chunkSize,
		Received: empty, UploadedBytes: 0, Instant: false, Completed: false,
	}, nil
}

// SaveChunk 写入单个分片到目标文件的对应 offset，并标记位图（幂等：重复片不重复计数）。
func SaveChunk(filePath, username string, index int, offset int64, data []byte) error {
	// 1) 锁外快速校验会话存在、归属与状态
	var sess model.UploadSession
	if err := model.DB.Where("file_path = ?", filePath).First(&sess).Error; err != nil {
		return ErrSessionNotFound
	}
	if sess.Username != username {
		return ErrForbidden
	}
	if sess.Status != model.UploadStatusUploading {
		return ErrNotUploading
	}
	if index < 0 || index >= sess.TotalChunks {
		return fmt.Errorf("分片号 %d 越界", index)
	}
	// 以 index 推导 offset，忽略客户端传入的 offset（防错位）
	offset = int64(index) * int64(sess.ChunkSize)

	// 2) 并发 WriteAt（不同 offset，POSIX 保证安全）
	f, err := os.OpenFile(filePath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	written, werr := f.WriteAt(data, offset)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("写入分片失败: %w", werr)
	}
	if serr != nil {
		logger.App.Warn("分片 Sync 失败", "path", filePath, "error", serr)
	}
	_ = cerr

	// 3) 在数据库事务内 read-modify-write，并严格检查错误。
	//    事务让位图更新原子串行；检查 Updates 错误避免 SQLITE_BUSY 被静默吞掉（位图丢失）。
	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var latest model.UploadSession
		if err := tx.Where("file_path = ?", filePath).First(&latest).Error; err != nil {
			return err
		}
		bmp := bmpDecode(latest.ReceivedBmp)
		already := bmp.Bit(index) == 1
		bmp.SetBit(bmp, index, 1)
		uploaded := latest.UploadedBytes
		if !already {
			uploaded += int64(written)
		}
		return tx.Model(&model.UploadSession{}).Where("file_path = ?", filePath).Updates(map[string]any{
			"received_bmp":   bmpEncode(bmp),
			"uploaded_bytes": uploaded,
			"expires_at":     time.Now().Add(SessionTTL),
		}).Error
	}); err != nil {
		return fmt.Errorf("更新分片进度失败: %w", err)
	}
	return nil
}

// CompleteResult 完成校验结果。Completed=false 时 Missing 为缺失分片号，供前端补传后重试。
type CompleteResult struct {
	Completed bool
	Missing   []int
}

// CompleteUpload 校验分片到齐情况：到齐则校验整文件 MD5 并置完成；
// 未到齐则返回 Missing 列表（不报错），让前端补传后重试，避免直接失败。
func CompleteUpload(filePath, username, expectedHash string) (*CompleteResult, error) {
	var sess model.UploadSession
	if err := model.DB.Where("file_path = ?", filePath).First(&sess).Error; err != nil {
		return nil, ErrSessionNotFound
	}
	if sess.Username != username {
		return nil, ErrForbidden
	}
	if sess.Status == model.UploadStatusCompleted {
		return &CompleteResult{Completed: true}, nil // 幂等
	}
	if sess.Status != model.UploadStatusUploading {
		return nil, ErrNotUploading
	}

	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()

	// 锁内重读最新位图，确保与并发 SaveChunk 的写入一致
	var latest model.UploadSession
	if err := model.DB.Where("file_path = ?", filePath).First(&latest).Error; err != nil {
		return nil, ErrSessionNotFound
	}
	bmp := bmpDecode(latest.ReceivedBmp)
	var missing []int
	for i := 0; i < latest.TotalChunks; i++ {
		if bmp.Bit(i) != 1 {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		logger.App.Warn("上传分片未到齐", "path", filePath, "total", latest.TotalChunks, "missing_count", len(missing), "received_bmp", latest.ReceivedBmp)
		return &CompleteResult{Completed: false, Missing: missing}, nil
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("目标文件不存在: %w", err)
	}
	if fi.Size() != latest.TotalSize {
		return nil, fmt.Errorf("文件大小不匹配: 期望 %d 实际 %d", latest.TotalSize, fi.Size())
	}
	actual, err := sampleFileHash(filePath, latest.TotalSize, latest.FileName)
	if err != nil {
		return nil, fmt.Errorf("计算文件校验值失败: %w", err)
	}
	if expectedHash != "" && actual != expectedHash {
		return nil, ErrHashMismatch
	}
	if err := utils.ChownLibvirtQEMU(filePath); err != nil {
		logger.App.Warn("设置上传文件属主失败", "path", filePath, "error", err)
	}
	if err := model.DB.Model(&model.UploadSession{}).Where("file_path = ?", filePath).Updates(map[string]any{
		"status":    model.UploadStatusCompleted,
		"file_hash": actual,
	}).Error; err != nil {
		return nil, fmt.Errorf("标记上传完成失败: %w", err)
	}
	return &CompleteResult{Completed: true}, nil
}

// GetUploadStatus 查询会话状态（找不到会话不报错，返回 Exists=false 以便前端重新 init）。
func GetUploadStatus(filePath, username string) (*StatusResult, error) {
	var sess model.UploadSession
	if err := model.DB.Where("file_path = ?", filePath).First(&sess).Error; err != nil {
		return &StatusResult{Exists: false}, nil
	}
	if sess.Username != username {
		return nil, ErrForbidden
	}
	return &StatusResult{
		Exists:        true,
		Status:        sess.Status,
		TotalChunks:   sess.TotalChunks,
		ChunkSize:     sess.ChunkSize,
		Received:      bmpReceivedList(bmpDecode(sess.ReceivedBmp), sess.TotalChunks),
		UploadedBytes: sess.UploadedBytes,
	}, nil
}

// PendingSession 未完成上传会话的摘要，供前端"主动恢复"展示。
type PendingSession struct {
	SessionKey    string `json:"session_key"`
	Category      string `json:"category"`
	FileName      string `json:"file_name"`
	TotalSize     int64  `json:"total_size"`
	UploadedBytes int64  `json:"uploaded_bytes"`
	TotalChunks   int    `json:"total_chunks"`
	Progress      int    `json:"progress"` // 0-100
	FileHash      string `json:"file_hash"`
}

// ListPendingSessions 列出某用户所有 status=uploading 的会话摘要（含模板，由调用方按需过滤）。
func ListPendingSessions(username string) ([]PendingSession, error) {
	var sessions []model.UploadSession
	if err := model.DB.Where("username = ? AND status = ?", username, model.UploadStatusUploading).Find(&sessions).Error; err != nil {
		return nil, err
	}
	out := make([]PendingSession, 0, len(sessions))
	for _, s := range sessions {
		received := len(bmpReceivedList(bmpDecode(s.ReceivedBmp), s.TotalChunks))
		progress := 0
		if s.TotalChunks > 0 {
			progress = received * 100 / s.TotalChunks
		}
		out = append(out, PendingSession{
			SessionKey: s.FilePath, Category: s.Category, FileName: s.FileName,
			TotalSize: s.TotalSize, UploadedBytes: s.UploadedBytes, TotalChunks: s.TotalChunks,
			Progress: progress, FileHash: s.FileHash,
		})
	}
	return out, nil
}

// CancelUpload 取消上传：删除未完成的文件并清除会话（已完成的文件不删，防误删）。
func CancelUpload(filePath, username string) error {
	var sess model.UploadSession
	if err := model.DB.Where("file_path = ?", filePath).First(&sess).Error; err != nil {
		return nil // 无会话，幂等成功
	}
	if sess.Username != username {
		return ErrForbidden
	}
	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()
	if sess.Status != model.UploadStatusCompleted {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			logger.App.Warn("清理未完成上传文件失败", "path", filePath, "error", err)
		}
	}
	model.DB.Where("file_path = ?", filePath).Delete(&model.UploadSession{})
	return nil
}

// RemoveUpload 强制删除上传文件与会话（不论状态），用于清理模板导入等已完成的上传临时包。
func RemoveUpload(filePath, username string) error {
	var sess model.UploadSession
	if err := model.DB.Where("file_path = ?", filePath).First(&sess).Error; err != nil {
		return nil // 无会话，幂等成功
	}
	if sess.Username != username {
		return ErrForbidden
	}
	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		logger.App.Warn("删除上传文件失败", "path", filePath, "error", err)
	}
	model.DB.Where("file_path = ?", filePath).Delete(&model.UploadSession{})
	return nil
}

// CleanupExpiredSessions 清理过期且未完成的会话及其文件。
func CleanupExpiredSessions() {
	now := time.Now()
	var sessions []model.UploadSession
	if err := model.DB.Where("expires_at < ? AND status != ?", now, model.UploadStatusCompleted).Find(&sessions).Error; err != nil {
		logger.App.Warn("查询过期上传会话失败", "error", err)
		return
	}
	for _, s := range sessions {
		if err := os.Remove(s.FilePath); err != nil && !os.IsNotExist(err) {
			logger.App.Warn("清理过期上传文件失败", "path", s.FilePath, "error", err)
		}
		model.DB.Where("file_path = ?", s.FilePath).Delete(&model.UploadSession{})
	}
	if len(sessions) > 0 {
		logger.App.Info("清理过期上传会话", "count", len(sessions))
	}
}

// StartExpiredSessionCleanup 启动后台 goroutine 定时清理过期会话。
func StartExpiredSessionCleanup() {
	go func() {
		defer utils.RecoverAndLog("chunkupload-cleanup")
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			CleanupExpiredSessions()
		}
	}()
}
