package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service"
	bw "kvm_console/service/bandwidth"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/utils"
)

type VmXMLUpdateRequest struct {
	XML string `json:"xml" binding:"required"`
}

func buildVirtioMemRequestFromSpec(specMemoryGB int) *vm_memory.VMMemoryDynamicRequest {
	if specMemoryGB <= 0 {
		specMemoryGB = 1
	}
	enabled := true
	autoBalloon := false
	initialGB := max(1, specMemoryGB/2)
	maxGB := max(specMemoryGB, (specMemoryGB*13+9)/10)
	return &vm_memory.VMMemoryDynamicRequest{
		DynamicEnabled: &enabled,
		MemoryBackend:  "virtio_mem",
		MemoryInitial:  initialGB,
		MemoryMin:      initialGB,
		MemoryMax:      maxGB,
		AutoBalloon:    &autoBalloon,
	}
}

func getVirtioMemSpecMemoryGB(vm *service.VmDetail) int {
	if vm == nil || vm.MemoryBackend != "virtio_mem" || !vm.MemoryDynamicEnabled {
		return 0
	}
	initialGB := 0
	if vm.MemoryInitial > 0 {
		initialGB = max(1, vm.MemoryInitial/1024)
	}
	maxDynamicGB := 0
	if vm.MemoryMaxDynamic > 0 {
		maxDynamicGB = max(1, vm.MemoryMaxDynamic/1024)
	}
	specFromInitial := 0
	if initialGB > 0 {
		specFromInitial = initialGB * 2
	}
	specFromMax := 0
	if maxDynamicGB > 0 {
		specFromMax = max(1, (maxDynamicGB*10)/13)
	}
	fallback := 0
	if specFromInitial > 0 || specFromMax > 0 {
		return max(1, max(specFromInitial, specFromMax))
	}
	if vm.MaxMemory > 0 {
		fallback = max(1, vm.MaxMemory/1024)
	} else if vm.Memory > 0 {
		fallback = max(1, vm.Memory/1024)
	}
	return max(1, fallback)
}

// VmAddDiskItem 编辑时新增磁盘的单项
type VmAddDiskItem struct {
	Size          int    `json:"size"`            // GB
	Format        string `json:"format"`          // qcow2/raw
	Bus           string `json:"bus"`             // 磁盘总线: virtio/scsi/sata/ide
	StoragePoolID string `json:"storage_pool_id"` // 新增磁盘落盘存储位置
}

// GetVmList 获取虚拟机列表
func GetVmList(c *gin.Context) {
	role, _ := c.Get("role")
	if role == "admin" {
		service.TriggerAdminVMCacheRefreshIfNeeded()
	}

	vms, err := service.ListCachedVMs(buildVMListOptions(c))
	if err != nil {
		respondVMListError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    vms,
	})
}

// GetVmDetail 获取虚拟机详情
func GetVmDetail(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	vm, err := service.GetVM(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    vm,
	})
}

// GetVmPCIEInfo 获取虚拟机 PCIe 热插槽使用情况
func GetVmPCIEInfo(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	info, err := service.GetVMPCIEInfo(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取 PCIe 信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    info,
	})
}

// GetVmXML 获取虚拟机持久化配置 XML
func GetVmXML(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	xmlContent, err := service.GetVMInactiveDomainXML(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"xml": xmlContent,
		},
	})
}

// GetVmIP 获取单个虚拟机 IP
func GetVmIP(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	ip, ipStatus, err := service.GetVMIPInfo(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"ip":        ip,
			"ip_status": ipStatus,
		},
	})
}

// OperateVm 操作虚拟机（开机/关机/强制关机/重启/重置）
func OperateVm(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	var req VmOperateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请指定操作类型（action: start/shutdown/destroy/reboot/reset）",
		})
		return
	}
	if err := service.EnsureVMNotMigrating(name, "电源操作"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	// 检查是否正在执行快照操作（VM 处于 paused/saving 状态）
	if err := service.EnsureVMNotSnapshotting(name, "电源操作"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	// 关机/强制断电时若虚拟机已锁定，在响应中附加提示信息
	var lockWarning string
	if (req.Action == "shutdown" || req.Action == "destroy") && service.IsVMLocked(name) {
		lockWarning = "该虚拟机已锁定，关机操作将继续执行"
	}

	// 开机前检查配额（仅非管理员用户）
	if req.Action == "start" {
		role, _ := c.Get("role")
		if role != "admin" {
			username, _ := c.Get("username")
			usernameStr, _ := username.(string)
			if !service.IsLightweightCloudUser(usernameStr) {
				if err := service.CheckQuotaForStart(usernameStr, name); err != nil {
					c.JSON(http.StatusForbidden, gin.H{
						"code":    403,
						"message": err.Error(),
					})
					return
				}
			}
		}
	}

	var err error
	var actionText string

	switch req.Action {
	case "start":
		err = service.StartVM(name)
		actionText = "开机"
	case "shutdown":
		err = service.ShutdownVM(name)
		actionText = "关机"
	case "destroy":
		err = service.DestroyVM(name)
		actionText = "强制断电"
	case "reboot":
		err = service.RebootVM(name)
		actionText = "重启"
	case "reset":
		err = service.ResetVM(name)
		actionText = "重置"
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不支持的操作: " + req.Action,
		})
		return
	}

	if err != nil {
		statusCode := http.StatusInternalServerError
		if service.IsMaintenanceModeError(err) {
			statusCode = http.StatusForbidden
		}
		c.JSON(statusCode, gin.H{
			"code":    statusCode,
			"message": actionText + "失败: " + err.Error(),
		})
		return
	}

	resp := gin.H{
		"code":    200,
		"message": actionText + "指令已下发",
	}
	if lockWarning != "" {
		resp["warning"] = lockWarning
	}
	c.JSON(http.StatusOK, resp)
	service.RefreshVMCacheByNameAsync(name)

	// 开机/关机/强制断电后异步触发全局带宽重新分配
	if req.Action == "start" || req.Action == "shutdown" || req.Action == "destroy" {
		cfg := config.GlobalConfig
		if cfg.MaxBurstInbound > 0 || cfg.MaxBurstOutbound > 0 {
			go func() {
				defer utils.RecoverAndLog("vm-operate-bandwidth")
				// 等待VM状态稳定后再重新分配（关机需要时间）
				if req.Action != "start" {
					time.Sleep(3 * time.Second)
				}
				if err := service.ApplyGlobalBandwidthLimit(); err != nil {
					logger.App.Warn("VM 操作后重新分配带宽失败", "component", "全局带宽", "vm", name, "action", req.Action, "error", err)
				}
			}()
		}
	}

	// 开机后应用带宽限速（确保运行中的 VM 应用最新限速规则）
	if req.Action == "start" {
		role, _ := c.Get("role")
		if role != "admin" {
			username, _ := c.Get("username")
			usernameStr, _ := username.(string)
			go func() {
				defer utils.RecoverAndLog("vm-start-bandwidth")
				if service.IsLightweightCloudUser(usernameStr) {
					if err := service.ApplyLightweightVMBandwidth(name); err != nil {
						logger.App.Warn("开机后应用轻量云 VM 带宽失败", "vm", name, "error", err)
					}
					return
				}
				if err := service.RebalanceUserBandwidth(usernameStr); err != nil {
					logger.App.Warn("开机后应用带宽限速失败", "error", err)
				}
			}()
		}
	}
}

// EditVm 编辑虚拟机配置
func EditVm(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}
	if err := service.EnsureVMNotMigrating(name, "编辑虚拟机配置"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}
	if !service.DomainExists(name) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "虚拟机不存在"})
		return
	}

	var req VmEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	// 非管理员用户需要检查配额
	role, _ := c.Get("role")
	username, _ := c.Get("username")
	isAdmin := role == "admin"
	existingVM, _ := service.GetVM(name)
	existingVirtioMem := existingVM != nil && existingVM.MemoryDynamicEnabled && existingVM.MemoryBackend == "virtio_mem"
	existingVirtioMemSpecGB := getVirtioMemSpecMemoryGB(existingVM)
	var targetCPULimitPercent *int

	if req.CPULimitPercent != nil {
		if !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "仅管理员可修改 CPU 限制",
			})
			return
		}
		normalizedLimit := service.NormalizeVMCPULimitPercent(*req.CPULimitPercent)
		if err := service.ValidateVMCPULimitPercent(normalizedLimit); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
		targetCPULimitPercent = &normalizedLimit
	} else if isAdmin && req.VCPU > 0 && existingVM != nil && existingVM.CPULimitPercent > 0 {
		preservedLimit := existingVM.CPULimitPercent
		targetCPULimitPercent = &preservedLimit
	}

	if !isAdmin {
		usernameStr, _ := username.(string)

		// 计算 CPU/内存 增量
		deltaCPU := 0
		deltaMemoryGB := 0
		if req.VCPU > 0 || req.Memory > 0 {
			oldCPU, oldMemMB := service.GetVMCPUAndMemory(name)
			if req.VCPU > 0 {
				deltaCPU = req.VCPU - oldCPU
			}
			if req.Memory > 0 {
				oldMemoryGB := oldMemMB / 1024
				if existingVirtioMemSpecGB > 0 {
					oldMemoryGB = existingVirtioMemSpecGB
				}
				deltaMemoryGB = req.Memory - oldMemoryGB // GB
			}
		}

		// 计算新增磁盘总量
		totalNewDiskGB := 0
		for _, disk := range req.AddDisks {
			if disk.Size > 0 {
				totalNewDiskGB += disk.Size
			}
		}

		// 只要有资源增加就检查配额
		if deltaCPU > 0 || deltaMemoryGB > 0 || totalNewDiskGB > 0 {
			if err := service.CheckQuotaForEdit(usernameStr, deltaCPU, deltaMemoryGB, totalNewDiskGB); err != nil {
				c.JSON(http.StatusForbidden, gin.H{
					"code":    403,
					"message": err.Error(),
				})
				return
			}
		}
	}

	// 修改 CPU 和内存。动态内存配置存在时，传统 memory 字段只作为动态配置的一部分处理。
	if existingVirtioMem && req.MemoryDynamic == nil && req.Memory > 0 && existingVirtioMemSpecGB > 0 && req.Memory != existingVirtioMemSpecGB {
		req.MemoryDynamic = buildVirtioMemRequestFromSpec(req.Memory)
	}
	if req.MemoryDynamic != nil && !isAdmin {
		baseMemoryGB := req.Memory
		if baseMemoryGB <= 0 {
			baseMemoryGB = existingVirtioMemSpecGB
		}
		if baseMemoryGB <= 0 {
			_, oldMemMB := service.GetVMCPUAndMemory(name)
			baseMemoryGB = max(1, oldMemMB/1024)
		}
		req.MemoryDynamic = sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, baseMemoryGB)
	}
	legacyMemoryMB := req.Memory * 1024
	if req.MemoryDynamic != nil || existingVirtioMem {
		legacyMemoryMB = 0
	}
	if req.VCPU > 0 || legacyMemoryMB > 0 {
		if err := service.EditVMConfig(name, req.VCPU, req.MaxVCPU, legacyMemoryMB); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "编辑 CPU/内存失败: " + err.Error(),
			})
			return
		}
	}
	if targetCPULimitPercent != nil {
		effectiveVCPU := req.VCPU
		if effectiveVCPU <= 0 && existingVM != nil {
			effectiveVCPU = existingVM.VCPU
		}
		if effectiveVCPU <= 0 {
			effectiveVCPU, _ = service.GetVMCPUAndMemory(name)
		}
		if err := service.SetVMCPULimitPercent(name, effectiveVCPU, *targetCPULimitPercent); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 CPU 限制失败: " + err.Error(),
			})
			return
		}
	}
	if req.CPUAffinity != nil {
		if !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "仅管理员可修改 CPU 亲和性",
			})
			return
		}
		if err := service.SetVMCPUAffinity(name, *req.CPUAffinity); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 CPU 亲和性失败: " + err.Error(),
			})
			return
		}
	}
	if req.MemoryDynamic != nil {
		if req.MemoryDynamic.MemoryCurrent > 0 {
			if err := vm_memory.SetVMMemoryCurrent(name, req.MemoryDynamic.MemoryCurrent*1024, true); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "调整当前内存失败: " + err.Error(),
				})
				return
			}
		}
		if req.MemoryDynamic.DynamicEnabled != nil || req.MemoryDynamic.MemoryInitial > 0 || req.MemoryDynamic.MemoryMax > 0 || req.MemoryDynamic.MemoryMin > 0 || req.MemoryDynamic.AutoBalloon != nil {
			msg, err := vm_memory.SetVMMemoryDynamicConfig(name, req.MemoryDynamic)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "设置动态内存失败: " + err.Error(),
				})
				return
			}
			if msg != "" {
				logger.App.Info("动态内存调整", "vm", name, "message", msg)
			}
		}
	}

	// 修改 PCIe 热插槽数量（仅关机时可修改）
	if req.PCIERootPorts != nil {
		if err := service.SetVMPCIERootPorts(name, *req.PCIERootPorts); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "修改 PCIe 端口数量失败: " + err.Error(),
			})
			return
		}
	}

	// 修改 UEFI 固件兼容模式（ARM 专用）
	if req.FirmwareCompat != nil {
		if err := service.SetVMFirmwareCompat(name, *req.FirmwareCompat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 UEFI 固件兼容模式失败: " + err.Error(),
			})
			return
		}
	}

	// 修改直接内核引导
	if req.DirectBoot != nil {
		if err := service.SetVMDirectBoot(name, req.DirectBoot); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置直接内核引导失败: " + err.Error(),
			})
			return
		}
	}

	// 修改自动启动
	if req.Remark != nil {
		if err := service.SetVMRemark(name, *req.Remark); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置备注失败: " + err.Error(),
			})
			return
		}
	}

	if req.Group != nil {
		if err := service.SetVMGroup(name, *req.Group); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置分组失败: " + err.Error(),
			})
			return
		}
	}

	if req.Autostart != nil {
		if err := service.SetVMAutostart(name, *req.Autostart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置自动启动失败: " + err.Error(),
			})
			return
		}
	}

	if req.Freeze != nil {
		if err := service.SetVMFreeze(name, *req.Freeze); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置启动冻结失败: " + err.Error(),
			})
			return
		}
	}

	if req.APIC != nil {
		if err := service.SetVMAPICConfig(name, *req.APIC); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 APIC 配置失败: " + err.Error(),
			})
			return
		}
	}

	if req.PAE != nil {
		if err := service.SetVMPAEConfig(name, *req.PAE); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 PAE 配置失败: " + err.Error(),
			})
			return
		}
	}

	if req.RTCOffset != nil || req.RTCStartDate != nil {
		detail, err := service.GetVM(name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "读取当前 RTC 配置失败: " + err.Error(),
			})
			return
		}
		rtcOffset := detail.RTCOffset
		rtcStartDate := detail.RTCStartDate
		if req.RTCOffset != nil {
			rtcOffset = *req.RTCOffset
		}
		if req.RTCStartDate != nil {
			rtcStartDate = *req.RTCStartDate
		}
		if err := service.SetVMRTCConfig(name, rtcOffset, rtcStartDate); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 RTC 配置失败: " + err.Error(),
			})
			return
		}
	}

	if req.GuestAgent != nil {
		if err := service.SetVMGuestAgentConfig(name, req.GuestAgent); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 QEMU Guest Agent 配置失败: " + err.Error(),
			})
			return
		}
	}

	if req.SMBIOS1 != nil {
		if err := service.SetVMSMBIOS1Config(name, req.SMBIOS1); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置 SMBIOS 配置失败: " + err.Error(),
			})
			return
		}
	}

	if strings.TrimSpace(req.BootType) != "" {
		if err := service.SetVMBootType(name, req.BootType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置引导方式失败: " + err.Error(),
			})
			return
		}
	}

	// 修改启动顺序
	if len(req.BootOrder) > 0 {
		if err := service.SetVMBootOrder(name, req.BootOrder); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "设置启动顺序失败: " + err.Error(),
			})
			return
		}
	}

	// 重排设备顺序（设备级排序，如多个 cdrom 的先后）
	if len(req.DeviceOrder) > 0 {
		if err := service.ReorderVMDevices(name, req.DeviceOrder); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "重排设备顺序失败: " + err.Error(),
			})
			return
		}
	}

	// 修改网卡类型
	if req.NicModel != "" {
		if err := service.SetVMNicModel(name, req.NicModel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "修改网卡类型失败: " + err.Error(),
			})
			return
		}
	}

	if req.VideoModel != "" {
		if err := service.SetVMVideoModel(name, req.VideoModel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "修改显示设备失败: " + err.Error(),
			})
			return
		}
	}

	if req.CPUTopologyMode != "" {
		if err := service.SetVMCPUTopologyMode(name, req.CPUTopologyMode); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "修改 CPU 拓扑失败: " + err.Error(),
			})
			return
		}
	}

	// 新增磁盘
	for _, disk := range req.AddDisks {
		if disk.Size <= 0 {
			continue
		}
		bus := disk.Bus
		if bus == "" {
			bus = "virtio"
		}
		format := disk.Format
		// 普通用户只能使用 qcow2 格式
		if !isAdmin {
			format = "qcow2"
		}
		diskDir, _, err := service.ResolveVMStorageDir(disk.StoragePoolID, isAdmin)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "解析新增磁盘存储位置失败: " + err.Error(),
			})
			return
		}
		_, err = service.AddDiskWithBusInDir(name, disk.Size, format, bus, diskDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "添加磁盘失败: " + err.Error(),
			})
			return
		}
	}

	// 磁盘 IOPS 限制（仅管理员）
	if isAdmin && req.DiskIOPS != nil {
		for dev, iopsCfg := range req.DiskIOPS {
			iops := &service.DiskIOPSTune{
				TotalIopsSec: iopsCfg.TotalIopsSec,
				ReadIopsSec:  iopsCfg.ReadIopsSec,
				WriteIopsSec: iopsCfg.WriteIopsSec,
			}
			if iops.TotalIopsSec == 0 && iops.ReadIopsSec == 0 && iops.WriteIopsSec == 0 {
				iops = nil
			}
			if err := service.SetDiskIOPSTune(name, dev, iops); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": fmt.Sprintf("设置磁盘 %s IOPS 限制失败: %s", dev, err.Error()),
				})
				return
			}
		}
	} else if req.DiskIOPS != nil && !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "仅管理员可设置磁盘 IOPS 限制",
		})
		return
	}

	// 如果修改了带宽，必须在响应前完成校验和应用，避免配额错误被吞掉。
	if !isAdmin && (req.BandwidthInboundAvg != nil || req.BandwidthOutboundAvg != nil) {
		usernameStr, _ := username.(string)
		inAvg := 0
		outAvg := 0
		currentBandwidth, _ := service.GetVMBandwidth(name)
		if currentBandwidth != nil {
			inAvg = currentBandwidth.InboundAvg
			outAvg = currentBandwidth.OutboundAvg
		}
		if req.BandwidthInboundAvg != nil {
			inAvg = *req.BandwidthInboundAvg
		}
		if req.BandwidthOutboundAvg != nil {
			outAvg = *req.BandwidthOutboundAvg
		}
		if err := service.SetVMCustomAverage(usernameStr, name, inAvg, outAvg); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
	} else if isAdmin && (req.BandwidthInboundAvg != nil || req.BandwidthOutboundAvg != nil) {
		if service.IsLightweightCloudVM(name) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "轻量云服务器的带宽由单机配额控制，请在轻量云配置中调整",
			})
			return
		}
		inAvg := 0
		outAvg := 0
		currentBandwidth, _ := service.GetVMBandwidth(name)
		if currentBandwidth != nil {
			inAvg = currentBandwidth.InboundAvg
			outAvg = currentBandwidth.OutboundAvg
		}
		if req.BandwidthInboundAvg != nil {
			inAvg = *req.BandwidthInboundAvg
		}
		if req.BandwidthOutboundAvg != nil {
			outAvg = *req.BandwidthOutboundAvg
		}
		// 仅当管理员明确设置了非零值才直接应用
		if inAvg > 0 || outAvg > 0 {
			inAvgKB := bw.MbpsToKBps(inAvg)
			outAvgKB := bw.MbpsToKBps(outAvg)
			if err := service.ApplyVMBandwidth(name, inAvgKB, inAvgKB, inAvgKB*30, outAvgKB, outAvgKB, outAvgKB*30); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "设置速率限制失败: " + err.Error(),
				})
				return
			}
		}
		// 管理员设置为 0 表示清除该 VM 的单机限速；VPC 场景由交换机负责聚合限速。
		if inAvg == 0 && outAvg == 0 {
			if err := service.ClearVMBandwidth(name); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "清除速率限制失败: " + err.Error(),
				})
				return
			}
		}
	}

	// 硬件直通设备
	if req.HostDevices != nil {
		if !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "仅管理员可配置硬件直通设备",
			})
			return
		}
		stateResult := utils.ExecCommand("virsh", "domstate", name)
		state := strings.TrimSpace(stateResult.Stdout)
		if state == "running" || state == "paused" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "修改硬件直通设备需要先关机",
			})
			return
		}
		if err := service.EnsureVfioModuleLoaded(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "加载 vfio-pci 模块失败: " + err.Error(),
			})
			return
		}
		existingDevices, err := service.GetVMPCIDevices(name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "获取当前直通设备失败: " + err.Error(),
			})
			return
		}
		for _, existing := range existingDevices {
			if detachErr := service.DetachPCIDeviceFromVM(name, existing.PCIAddress); detachErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "移除现有直通设备失败: " + detachErr.Error(),
				})
				return
			}
		}
		for _, hd := range req.HostDevices {
			if validateErr := service.ValidatePCIPassthrough(hd.PCIAddress); validateErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    400,
					"message": "设备 " + hd.PCIAddress + " 直通验证失败: " + validateErr.Error(),
				})
				return
			}
			if !service.IsDeviceVfioBound(hd.PCIAddress) {
				if bindErr := service.BindPCIDeviceToVfio(hd.PCIAddress); bindErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"code":    500,
						"message": "绑定设备 " + hd.PCIAddress + " 到 vfio-pci 失败: " + bindErr.Error(),
					})
					return
				}
			}
			if attachErr := service.AttachPCIDeviceToVM(name, hd.PCIAddress); attachErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "添加直通设备 " + hd.PCIAddress + " 失败: " + attachErr.Error(),
				})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "配置修改成功",
	})
	service.RefreshVMCacheByNameAsync(name)
}

// UpdateVmXML 更新虚拟机持久化配置 XML
func UpdateVmXML(c *gin.Context) {
	if !requireHighRiskVerification(c, "edit_vm_xml") {
		return
	}

	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}
	if err := service.EnsureVMNotMigrating(name, "编辑虚拟机 XML"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	var req VmXMLUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	if err := service.SetVMInactiveDomainXML(name, req.XML); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "虚拟机 XML 保存成功",
	})
}

// GetVmStats 获取虚拟机实时资源使用
func GetVmStats(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	if parseBoolQuery(c, "prefer_cache") {
		if cached := service.GetCachedStats(name); cached != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "ok",
				"data":    cached,
			})
			return
		}
	}

	stats, err := service.GetVMStats(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取资源使用失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    stats,
	})
}

// GetVmStatsHistory 获取虚拟机资源使用历史（按日期范围查询）
func GetVmStatsHistory(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	if startStr == "" || endStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请指定查询时间范围（start, end），格式: 2006-01-02 或 2006-01-02T15:04:05",
		})
		return
	}

	// 支持两种日期格式
	var start, end time.Time
	var err error

	start, err = time.ParseInLocation("2006-01-02", startStr, time.Local)
	if err != nil {
		start, err = time.ParseInLocation("2006-01-02T15:04:05", startStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "start 日期格式错误，支持: 2006-01-02 或 2006-01-02T15:04:05",
			})
			return
		}
	}

	end, err = time.ParseInLocation("2006-01-02", endStr, time.Local)
	if err != nil {
		end, err = time.ParseInLocation("2006-01-02T15:04:05", endStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "end 日期格式错误，支持: 2006-01-02 或 2006-01-02T15:04:05",
			})
			return
		}
	}

	// 如果 end 只有日期没有时间，将其设为当天的 23:59:59
	if end.Hour() == 0 && end.Minute() == 0 && end.Second() == 0 {
		end = end.Add(24*time.Hour - time.Second)
	}

	records, err := service.QueryVMStatsHistory(name, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    records,
	})
}
