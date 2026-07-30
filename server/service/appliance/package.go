package appliance

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/sha1" // OVF 兼容清单仍可能使用 SHA-1，只用于完整性校验。
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxArchiveEntries = 256
	maxOVFSize        = 16 * 1024 * 1024
	maxExpandedSize   = int64(2 * 1024 * 1024 * 1024 * 1024)
)

// InspectSource 检查 OVF/OVA 并返回标准化元数据，不写入虚拟机目录。
func InspectSource(sourcePath string) (*Metadata, error) {
	return InspectSourceIn(sourcePath, "")
}

// InspectSourceIn 在指定临时目录中完成全量结构、配套文件和清单校验。
func InspectSourceIn(sourcePath, tempBase string) (*Metadata, error) {
	ext := strings.ToLower(filepath.Ext(sourcePath))
	if ext == ".ovf" {
		resolved, err := resolveOVFDirectory(context.Background(), sourcePath)
		if err != nil {
			return nil, err
		}
		return resolved.Metadata, nil
	}
	if ext != ".ova" {
		return nil, fmt.Errorf("虚拟机包格式仅支持 .ovf 和 .ova")
	}
	resolved, err := ResolveSource(context.Background(), sourcePath, tempBase)
	if err != nil {
		return nil, err
	}
	defer resolved.Cleanup()
	return resolved.Metadata, nil
}

// ResolveSource 解析 OVF 或解包 OVA，并返回所有可导入磁盘路径。
func ResolveSource(ctx context.Context, sourcePath, tempBase string) (*ResolvedPackage, error) {
	cleaned := filepath.Clean(sourcePath)
	info, err := os.Stat(cleaned)
	if err != nil {
		return nil, fmt.Errorf("读取虚拟机包失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("虚拟机包需要是普通文件")
	}
	ext := strings.ToLower(filepath.Ext(cleaned))
	switch ext {
	case ".ovf":
		return resolveOVFDirectory(ctx, cleaned)
	case ".ova":
		return resolveOVA(ctx, cleaned, tempBase)
	default:
		return nil, fmt.Errorf("虚拟机包格式仅支持 .ovf 和 .ova")
	}
}

func resolveOVFDirectory(ctx context.Context, ovfPath string) (*ResolvedPackage, error) {
	data, err := os.ReadFile(ovfPath)
	if err != nil {
		return nil, fmt.Errorf("读取 OVF 描述失败: %w", err)
	}
	meta, err := ParseOVF(data)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(ovfPath)
	resolved := &ResolvedPackage{Metadata: meta, SourcePath: ovfPath, RootDir: root, SourceFiles: []string{ovfPath}}
	for index, disk := range meta.Disks {
		if filepath.Base(disk.FileRef) != disk.FileRef {
			return nil, fmt.Errorf("直接导入 OVF 时磁盘引用需要位于同一目录: %s", disk.FileRef)
		}
		path := filepath.Join(root, disk.FileRef)
		if err := requireRegularFile(path); err != nil {
			return nil, fmt.Errorf("OVF 配套磁盘 %s 校验失败: %w", disk.FileRef, err)
		}
		resolved.DiskPaths = append(resolved.DiskPaths, path)
		resolved.SourceFiles = append(resolved.SourceFiles, path)
		if disk.CapacityBytes <= 0 {
			if info, statErr := os.Stat(path); statErr == nil {
				meta.Disks[index].CapacityBytes = info.Size()
				meta.Warnings = append(meta.Warnings, fmt.Sprintf("磁盘 %s 未声明容量，配额预估暂按文件大小计算", disk.FileRef))
			}
		}
	}
	manifest := strings.TrimSuffix(ovfPath, filepath.Ext(ovfPath)) + ".mf"
	if _, err := os.Stat(manifest); err == nil {
		if err := verifyManifest(ctx, manifest, root); err != nil {
			return nil, err
		}
		resolved.SourceFiles = append(resolved.SourceFiles, manifest)
	} else {
		meta.Warnings = append(meta.Warnings, "虚拟机包未提供完整性清单")
	}
	return resolved, nil
}

func resolveOVA(ctx context.Context, ovaPath, tempBase string) (*ResolvedPackage, error) {
	if tempBase == "" {
		tempBase = filepath.Join(os.TempDir(), "kvm_console", "appliance")
	}
	if err := os.MkdirAll(tempBase, 0o755); err != nil {
		return nil, fmt.Errorf("创建虚拟机包临时目录失败: %w", err)
	}
	tempDir, err := os.MkdirTemp(tempBase, "ova-")
	if err != nil {
		return nil, fmt.Errorf("创建 OVA 解包目录失败: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	file, err := os.Open(ovaPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("打开 OVA 失败: %w", err)
	}
	defer file.Close()

	tr := tar.NewReader(file)
	entries := 0
	expandedBytes := int64(0)
	var ovfPath string
	seen := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			cleanup()
			return nil, ctx.Err()
		default:
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("读取 OVA 归档失败: %w", err)
		}
		entries++
		if entries > maxArchiveEntries {
			cleanup()
			return nil, fmt.Errorf("OVA 文件数量超过 %d 个限制", maxArchiveEntries)
		}
		name, err := safeArchiveName(header.Name)
		if err != nil {
			cleanup()
			return nil, err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Size < 0 || header.Size > maxExpandedSize-expandedBytes {
			cleanup()
			return nil, fmt.Errorf("OVA 展开大小超过安全限制")
		}
		expandedBytes += header.Size
		if sourceInfo, statErr := os.Stat(ovaPath); statErr == nil && expandedBytes > sourceInfo.Size()*128+maxOVFSize {
			cleanup()
			return nil, fmt.Errorf("OVA 展开大小与源文件比例异常")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			cleanup()
			return nil, fmt.Errorf("OVA 包含链接或特殊文件: %s", header.Name)
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			cleanup()
			return nil, fmt.Errorf("OVA 包含重复文件名: %s", name)
		}
		seen[key] = struct{}{}
		dest := filepath.Join(tempDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			cleanup()
			return nil, err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("创建 OVA 解包文件失败: %w", err)
		}
		_, copyErr := copyWithContext(ctx, out, tr)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			cleanup()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("解包 OVA 文件 %s 失败", name)
		}
		if strings.EqualFold(filepath.Ext(name), ".ovf") {
			if ovfPath != "" {
				cleanup()
				return nil, fmt.Errorf("OVA 包含多个 OVF 描述")
			}
			ovfPath = dest
		}
	}
	if ovfPath == "" {
		cleanup()
		return nil, fmt.Errorf("OVA 中未发现 OVF 描述")
	}
	data, err := os.ReadFile(ovfPath)
	if err != nil || len(data) > maxOVFSize {
		cleanup()
		return nil, fmt.Errorf("读取 OVA 中的 OVF 描述失败")
	}
	meta, err := ParseOVF(data)
	if err != nil {
		cleanup()
		return nil, err
	}
	meta.SourceFormat = "ova"
	root := filepath.Dir(ovfPath)
	resolved := &ResolvedPackage{Metadata: meta, SourcePath: ovaPath, RootDir: root, SourceFiles: []string{ovaPath}, cleanupDir: tempDir}
	for index, disk := range meta.Disks {
		path, err := safeJoin(root, disk.FileRef)
		if err != nil {
			cleanup()
			return nil, err
		}
		if err := requireRegularFile(path); err != nil {
			cleanup()
			return nil, fmt.Errorf("OVA 配套磁盘 %s 校验失败: %w", disk.FileRef, err)
		}
		resolved.DiskPaths = append(resolved.DiskPaths, path)
		if disk.CapacityBytes <= 0 {
			if info, statErr := os.Stat(path); statErr == nil {
				meta.Disks[index].CapacityBytes = info.Size()
				meta.Warnings = append(meta.Warnings, fmt.Sprintf("磁盘 %s 未声明容量，配额预估暂按文件大小计算", disk.FileRef))
			}
		}
	}
	manifest := findManifest(root)
	if manifest != "" {
		if err := verifyManifest(ctx, manifest, root); err != nil {
			cleanup()
			return nil, err
		}
	} else {
		meta.Warnings = append(meta.Warnings, "虚拟机包未提供完整性清单")
	}
	return resolved, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, 1024*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		read, readErr := src.Read(buffer)
		if read > 0 {
			count, writeErr := dst.Write(buffer[:read])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func safeArchiveName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if strings.Contains(name, "\\") || strings.Contains(name, "://") {
		return "", fmt.Errorf("OVA 包含不安全路径: %s", name)
	}
	name = filepath.ToSlash(name)
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if name == "" || strings.HasPrefix(name, "/") || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("OVA 包含不安全路径: %s", name)
	}
	return cleaned, nil
}

func safeJoin(root, relative string) (string, error) {
	name, err := safeArchiveName(relative)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	dest := filepath.Clean(filepath.Join(root, filepath.FromSlash(name)))
	if dest != root && !strings.HasPrefix(dest, root+string(filepath.Separator)) {
		return "", fmt.Errorf("文件引用越过虚拟机包目录: %s", relative)
	}
	return dest, nil
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("不是普通文件")
	}
	return nil
}

func findManifest(root string) string {
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".mf") {
			return filepath.Join(root, entry.Name())
		}
	}
	return ""
}

var manifestLinePattern = regexp.MustCompile(`(?i)^\s*(SHA-?1|SHA-?256)\s*\(([^)]+)\)\s*=\s*([0-9a-f]+)\s*$`)

func verifyManifest(ctx context.Context, manifestPath, root string) error {
	file, err := os.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("打开完整性清单失败: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	checked := 0
	for scanner.Scan() {
		matches := manifestLinePattern.FindStringSubmatch(scanner.Text())
		if len(matches) != 4 {
			continue
		}
		path, err := safeJoin(root, matches[2])
		if err != nil {
			return err
		}
		var h hash.Hash
		algorithm := strings.ReplaceAll(strings.ToUpper(matches[1]), "-", "")
		if algorithm == "SHA1" {
			h = sha1.New()
		} else {
			h = sha256.New()
		}
		input, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("清单引用文件不存在: %s", matches[2])
		}
		_, copyErr := copyWithContext(ctx, h, input)
		_ = input.Close()
		if copyErr != nil {
			return fmt.Errorf("计算清单摘要失败: %w", copyErr)
		}
		actual := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(actual, matches[3]) {
			return fmt.Errorf("完整性清单校验失败: %s", matches[2])
		}
		checked++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取完整性清单失败: %w", err)
	}
	if checked == 0 {
		return fmt.Errorf("完整性清单中没有可识别的摘要")
	}
	return nil
}
