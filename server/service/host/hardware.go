package host

import (
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"kvm_console/utils"
)

// ==================== 宿主机硬件信息 ====================
// 概览页 CPU / 内存卡片展开区数据来源：
// - CPU：型号 / 插槽 / 物理核心 / 线程数（静态，进程内缓存）+ 每核实时使用率（/proc/stat 差分）
// - 内存：内存条（DIMM）信息（dmidecode，静态，进程内缓存）

// --- CPU 硬件信息 ---

// cpuHardwareStatic CPU 静态硬件信息（型号等不会变化，进程内缓存一次）
var cpuHardwareStatic struct {
	sync.Once
	model   string
	sockets int
	cores   int
	threads int
}

// cpuCoreSample 单核 /proc/stat 采样快照
type cpuCoreSample struct {
	busy  uint64
	total uint64
}

// cpuCoreSampler 每核使用率采样缓存：与上一次调用的样本做差分，
// 前端轮询周期（约 3s）即为统计窗口，避免每次请求都同步 sleep 采样
var cpuCoreSampler = struct {
	sync.Mutex
	samples []cpuCoreSample
	takenAt time.Time
}{}

// GetHostCPUHardware 获取宿主机 CPU 硬件信息与每核实时使用率
func GetHostCPUHardware() *HostCPUHardware {
	cpuHardwareStatic.Do(loadCPUHardwareStatic)

	return &HostCPUHardware{
		Model:        cpuHardwareStatic.model,
		Sockets:      cpuHardwareStatic.sockets,
		Cores:        cpuHardwareStatic.cores,
		Threads:      cpuHardwareStatic.threads,
		PerCoreUsage: getPerCoreUsage(),
	}
}

// loadCPUHardwareStatic 解析 CPU 静态信息（/proc/cpuinfo 优先，lscpu 兜底）
func loadCPUHardwareStatic() {
	cpuHardwareStatic.threads = runtime.NumCPU()
	cpuHardwareStatic.sockets = 1

	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		content := string(data)
		// 型号：x86 为 model name；部分 ARM 平台无此字段
		if m := regexp.MustCompile(`(?m)^model name\s*:\s*(.+)$`).FindStringSubmatch(content); len(m) > 1 {
			cpuHardwareStatic.model = strings.TrimSpace(m[1])
		}
		// 物理插槽数：唯一 physical id 数量
		physIDs := make(map[string]struct{})
		coreIDs := make(map[string]struct{})
		var curPhys string
		for _, line := range strings.Split(content, "\n") {
			key, val, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val = strings.TrimSpace(val)
			switch key {
			case "physical id":
				curPhys = val
				physIDs[val] = struct{}{}
			case "core id":
				// 物理核心 = (physical id, core id) 去重
				coreIDs[curPhys+"/"+val] = struct{}{}
			}
		}
		if len(physIDs) > 0 {
			cpuHardwareStatic.sockets = len(physIDs)
		}
		if len(coreIDs) > 0 {
			cpuHardwareStatic.cores = len(coreIDs)
		}
	}

	// lscpu 兜底（ARM 平台 /proc/cpuinfo 通常没有 model name / core id）
	if cpuHardwareStatic.model == "" || cpuHardwareStatic.cores == 0 {
		result := utils.ExecShellQuiet(`LC_ALL=C lscpu 2>/dev/null`)
		if result.Error == nil {
			socketsPerLscpu := 0
			coresPerSocket := 0
			for _, line := range strings.Split(result.Stdout, "\n") {
				key, val, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				key = strings.TrimSpace(key)
				val = strings.TrimSpace(val)
				switch key {
				case "Model name":
					if cpuHardwareStatic.model == "" {
						cpuHardwareStatic.model = val
					}
				case "Socket(s)":
					socketsPerLscpu, _ = strconv.Atoi(val)
				case "Core(s) per socket":
					coresPerSocket, _ = strconv.Atoi(val)
				}
			}
			if socketsPerLscpu > 0 {
				cpuHardwareStatic.sockets = socketsPerLscpu
			}
			if cpuHardwareStatic.cores == 0 && coresPerSocket > 0 {
				cpuHardwareStatic.cores = coresPerSocket * cpuHardwareStatic.sockets
			}
		}
	}

	if cpuHardwareStatic.model == "" {
		cpuHardwareStatic.model = "未知型号"
	}
	if cpuHardwareStatic.cores == 0 {
		cpuHardwareStatic.cores = cpuHardwareStatic.threads
	}
}

// readCPUCoreSamples 读取 /proc/stat 中每个核心的累计时间片
func readCPUCoreSamples() []cpuCoreSample {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	var samples []cpuCoreSample
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		// 跳过汇总行 "cpu"，只取 "cpu0"、"cpu1"...
		if len(fields) < 8 || len(fields[0]) <= 3 {
			continue
		}
		var total, idle uint64
		for i := 1; i < len(fields) && i <= 8; i++ {
			v, parseErr := strconv.ParseUint(fields[i], 10, 64)
			if parseErr != nil {
				continue
			}
			total += v
			// idle(4) + iowait(5) 视为空闲
			if i == 4 || i == 5 {
				idle += v
			}
		}
		samples = append(samples, cpuCoreSample{busy: total - idle, total: total})
	}
	return samples
}

// getPerCoreUsage 计算每核使用率（%）
// 优先与上次调用样本差分（窗口≈前端轮询间隔）；无有效历史样本时同步短采样一次
func getPerCoreUsage() []float64 {
	cpuCoreSampler.Lock()
	defer cpuCoreSampler.Unlock()

	cur := readCPUCoreSamples()
	if len(cur) == 0 {
		return []float64{}
	}

	prev := cpuCoreSampler.samples
	age := time.Since(cpuCoreSampler.takenAt)
	// 历史样本失效（首次调用 / 间隔过久 / 核数变化）时，短暂 sleep 后重采样
	if len(prev) != len(cur) || age < 500*time.Millisecond || age > 2*time.Minute {
		if len(prev) != len(cur) || age > 2*time.Minute {
			prev = cur
			time.Sleep(300 * time.Millisecond)
			cur = readCPUCoreSamples()
			if len(cur) != len(prev) {
				return []float64{}
			}
		} else {
			// 距上次采样过近（<500ms），直接沿用上次样本差分，避免除以过小窗口
			prev = cpuCoreSampler.samples
		}
	}

	usage := make([]float64, len(cur))
	for i := range cur {
		totalDelta := cur[i].total - prev[i].total
		busyDelta := cur[i].busy - prev[i].busy
		if totalDelta > 0 && busyDelta <= totalDelta {
			usage[i] = float64(busyDelta) / float64(totalDelta) * 100
		}
	}

	cpuCoreSampler.samples = cur
	cpuCoreSampler.takenAt = time.Now()
	return usage
}

// --- 内存条（DIMM）信息 ---

// memoryModulesStatic 内存条信息（静态硬件，进程内缓存一次）
var memoryModulesStatic struct {
	sync.Once
	info *HostMemoryModulesInfo
}

// GetHostMemoryModules 获取宿主机内存条（DIMM）信息
func GetHostMemoryModules() *HostMemoryModulesInfo {
	memoryModulesStatic.Do(func() {
		memoryModulesStatic.info = loadMemoryModules()
	})
	return memoryModulesStatic.info
}

// loadMemoryModules 通过 dmidecode 解析内存条信息
func loadMemoryModules() *HostMemoryModulesInfo {
	info := &HostMemoryModulesInfo{Modules: []HostMemoryModule{}}

	result := utils.ExecShellQuiet(`dmidecode -t memory 2>/dev/null`)
	if result.Error != nil || strings.TrimSpace(result.Stdout) == "" {
		info.Message = "未能获取内存条信息：可能未安装 dmidecode，或该平台未提供 SMBIOS 数据（常见于部分 ARM 设备与虚拟机）"
		return info
	}

	// 按 "Memory Device" 块拆分（跳过第一段头部信息）
	blocks := strings.Split(result.Stdout, "Memory Device")
	for idx, block := range blocks {
		if idx == 0 {
			continue
		}
		fields := parseDmidecodeBlock(block)
		info.TotalSlots++

		sizeMB := parseDmiSizeToMB(fields["Size"])
		if sizeMB <= 0 {
			// 空插槽（No Module Installed）
			continue
		}
		info.Installed++
		info.Modules = append(info.Modules, HostMemoryModule{
			Slot:            cleanDmiValue(fields["Locator"]),
			SizeMB:          sizeMB,
			Type:            cleanDmiValue(fields["Type"]),
			Speed:           cleanDmiValue(fields["Speed"]),
			ConfiguredSpeed: cleanDmiValue(firstNonEmptyDmi(fields["Configured Memory Speed"], fields["Configured Clock Speed"])),
			Manufacturer:    cleanDmiValue(fields["Manufacturer"]),
			PartNumber:      cleanDmiValue(fields["Part Number"]),
		})
	}

	if info.Installed == 0 {
		info.Message = "未检测到内存条信息：该平台 SMBIOS 未上报内存设备（常见于虚拟机或部分 ARM 设备）"
	}
	return info
}

// parseDmidecodeBlock 解析 dmidecode 单个设备块为 key-value
func parseDmidecodeBlock(block string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(block, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key != "" && val != "" {
			// 只保留首次出现的键，避免后续块头部串扰
			if _, exists := fields[key]; !exists {
				fields[key] = val
			}
		}
	}
	return fields
}

// parseDmiSizeToMB 解析 dmidecode Size 字段（如 "8192 MB" / "8 GB"），空槽返回 0
func parseDmiSizeToMB(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "No Module") {
		return 0
	}
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return 0
	}
	num, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || num <= 0 {
		return 0
	}
	switch strings.ToUpper(parts[1]) {
	case "MB":
		return num
	case "GB":
		return num * 1024
	case "TB":
		return num * 1024 * 1024
	case "KB":
		return num / 1024
	}
	return 0
}

// cleanDmiValue 过滤 dmidecode 无效占位值
func cleanDmiValue(raw string) string {
	val := strings.TrimSpace(raw)
	switch val {
	case "Unknown", "Not Specified", "None", "NO DIMM", "Not Provided":
		return ""
	}
	return val
}

// firstNonEmptyDmi 返回第一个非空的 dmidecode 字段值
func firstNonEmptyDmi(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
