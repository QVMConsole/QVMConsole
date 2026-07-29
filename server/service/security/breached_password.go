package security

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"kvm_console/utils"
)

// ============================================================
// 泄露密码检测服务
// 基于 Have I Been Pwned (HIBP) Pwned Passwords API
// 采用 k-匿名性模型：仅发送 SHA-1 哈希前 5 位，密码本身永不离开本机
// 离线时回退到内置常见弱密码列表
// ============================================================

const (
	hibpAPIURL       = "https://api.pwnedpasswords.com/range/"
	hibpTimeout      = 5 * time.Second
	hibpCacheTTL     = 30 * time.Minute // 哈希前缀 → 后缀列表的缓存有效期
	hibpCacheCleanup = 1 * time.Hour    // 缓存清理间隔
)

// hibpCacheEntry 缓存 HIBP API 返回的哈希后缀集合
type hibpCacheEntry struct {
	suffixes  map[string]int64 // 后缀与泄露次数（大写）
	expiresAt time.Time
}

var (
	hibpCache      = make(map[string]*hibpCacheEntry)
	hibpCacheMu    sync.RWMutex
	hibpClient     *http.Client
	hibpClientOnce sync.Once
)

// getHIBPClient 单例 HTTP 客户端
func getHIBPClient() *http.Client {
	hibpClientOnce.Do(func() {
		hibpClient = &http.Client{
			Timeout: hibpTimeout,
		}
	})
	return hibpClient
}

// init 启动缓存清理协程
func init() {
	go func() {
		defer utils.RecoverAndLog("hibp-cache-cleanup")
		ticker := time.NewTicker(hibpCacheCleanup)
		defer ticker.Stop()
		for range ticker.C {
			hibpCacheMu.Lock()
			now := time.Now()
			for prefix, entry := range hibpCache {
				if now.After(entry.expiresAt) {
					delete(hibpCache, prefix)
				}
			}
			hibpCacheMu.Unlock()
		}
	}()
}

// CheckPasswordBreached 检查密码是否在已知泄露数据库中
// 返回 (是否泄露, 是否使用了离线兜底, 错误)
func CheckPasswordBreached(password string) (breached bool, fallback bool, err error) {
	breached, _, fallback, err = CheckPasswordBreachedWithCount(password)
	return breached, fallback, err
}

// CheckPasswordBreachedWithCount 检查密码并返回 HIBP 泄露次数。
func CheckPasswordBreachedWithCount(password string) (breached bool, count int64, fallback bool, err error) {
	// 先检查本地常见密码列表
	if isCommonPassword(password) {
		return true, 1, true, nil
	}

	// 通过 HIBP API 检查
	breached, count, err = checkHIBP(password)
	if err != nil {
		// API 不可用时不阻止操作，仅记录错误
		// 调用方可以根据 fallback 判断是否仅使用了本地兜底
		return false, 0, false, err
	}
	return breached, count, false, nil
}

// checkHIBP 使用 HIBP API 的 k-匿名性模型检查密码
// 仅发送 SHA-1 哈希前 5 位，后缀在本地比对
func checkHIBP(password string) (bool, int64, error) {
	hash := sha1.Sum([]byte(password))
	fullHash := strings.ToUpper(hex.EncodeToString(hash[:]))
	prefix := fullHash[:5]
	suffix := fullHash[5:]
	suffixes, err := GetHIBPRange(prefix)
	if err != nil {
		return false, 0, err
	}
	count := suffixes[suffix]
	return count > 0, count, nil
}

// GetHIBPRange 获取指定 SHA-1 前缀对应的后缀及泄露次数。
func GetHIBPRange(prefix string) (map[string]int64, error) {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if len(prefix) != 5 {
		return nil, fmt.Errorf("HIBP 哈希前缀格式错误")
	}

	// 查缓存
	hibpCacheMu.RLock()
	entry, ok := hibpCache[prefix]
	hibpCacheMu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return cloneHIBPSuffixes(entry.suffixes), nil
	}

	// 调用 HIBP API
	client := getHIBPClient()
	url := hibpAPIURL + prefix
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("HIBP 请求构建失败: %w", err)
	}
	// 添加 User-Agent（HIBP 建议但不强制）
	req.Header.Set("User-Agent", "QVMConsole-PasswordCheck")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HIBP API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HIBP API 返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("HIBP 响应读取失败: %w", err)
	}

	// 解析响应：每行格式为 "HASH_SUFFIX:COUNT"
	suffixes := make(map[string]int64)
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		s := strings.TrimSpace(parts[0])
		count, parseErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if s != "" && parseErr == nil && count > 0 {
			suffixes[s] = count
		}
	}

	// 写入缓存
	hibpCacheMu.Lock()
	hibpCache[prefix] = &hibpCacheEntry{
		suffixes:  suffixes,
		expiresAt: time.Now().Add(hibpCacheTTL),
	}
	hibpCacheMu.Unlock()

	return cloneHIBPSuffixes(suffixes), nil
}

func cloneHIBPSuffixes(source map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(source))
	for suffix, count := range source {
		result[suffix] = count
	}
	return result
}

// IsPasswordBreached 简化版：仅返回是否泄露（API 不可用时回退到本地列表）
func IsPasswordBreached(password string) bool {
	breached, _, err := CheckPasswordBreached(password)
	if err != nil {
		// API 出错时仅依赖本地列表结果
		return isCommonPassword(password)
	}
	return breached
}
