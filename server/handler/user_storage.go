package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	vm_memory "kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// ==================== 用户存储池管理 ====================

// GetUserStorageInfo 获取当前用户存储池信息
func GetUserStorageInfo(c *gin.Context) {
	username, _ := c.Get("username")

	info, err := service.GetUserStorageInfo(username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取存储池信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    info,
	})
}

// InitUserStorageHandler 初始化用户存储池
func InitUserStorageHandler(c *gin.Context) {
	username, _ := c.Get("username")

	if err := service.InitUserStorage(username.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "初始化存储池失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "存储池初始化成功",
	})
}

// ListUserStorageFiles 列出指定类别的文件
func ListUserStorageFiles(c *gin.Context) {
	username, _ := c.Get("username")
	category := c.Param("category")

	if category != "iso" && category != "share" && category != "disk" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的存储类别，支持: iso, share, disk",
		})
		return
	}

	files, err := service.ListUserFiles(username.(string), category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取文件列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    files,
	})
}

// UploadUserStorageFile 上传文件到存储池
func UploadUserStorageFile(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)
	category := c.Param("category")

	if category != "iso" && category != "share" && category != "disk" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的存储类别，支持: iso, share, disk",
		})
		return
	}

	// 检查存储池是否初始化
	if !service.IsStorageInitialized(usernameStr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "存储池未初始化，请先初始化",
		})
		return
	}

	// 检查是否只读
	if service.IsStorageReadonly(usernameStr) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "存储空间已超出配额，处于只读模式，请先删除部分文件",
		})
		return
	}

	// 获取上传文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "未收到上传文件: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// ISO 类别要求 .iso 后缀
	if category == "iso" && !strings.HasSuffix(strings.ToLower(header.Filename), ".iso") {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "ISO 类别仅支持 .iso 文件",
		})
		return
	}

	// disk 类别要求虚拟磁盘文件后缀
	if category == "disk" {
		nameLower := strings.ToLower(header.Filename)
		validExts := []string{".qcow2", ".raw", ".vmdk", ".vhd", ".vhdx", ".img"}
		validExt := false
		for _, ext := range validExts {
			if strings.HasSuffix(nameLower, ext) {
				validExt = true
				break
			}
		}
		if !validExt {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "虚拟磁盘仅支持以下格式: .qcow2, .raw, .vmdk, .vhd, .vhdx, .img",
			})
			return
		}
	}

	// 安全检查文件名
	filename := filepath.Base(header.Filename)
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "非法文件名",
		})
		return
	}

	// 检查配额
	if err := service.CheckStorageQuota(usernameStr, header.Size); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}

	var destDir string
	switch category {
	case "iso":
		destDir = service.GetUserISODir(usernameStr)
	case "share":
		destDir = service.GetUserShareDir(usernameStr)
	case "disk":
		destDir = service.GetUserDiskDir(usernameStr)
	}

	destPath := filepath.Join(destDir, filename)

	// 保存文件
	out, err := os.Create(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建文件失败: " + err.Error(),
		})
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		os.Remove(destPath) // 清理不完整的文件
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "写入文件失败: " + err.Error(),
		})
		return
	}

	// 确保数据刷入物理磁盘后再返回响应（大文件尤为重要，避免前端以为完成实际还在缓存）
	if syncErr := out.Sync(); syncErr != nil {
		// Sync 失败不阻断流程，文件已写入但可能未完全落盘
		fmt.Printf("[WARN] 文件 Sync 失败 %s: %v\n", destPath, syncErr)
	}

	// 设置文件权限（project quota 不依赖文件 owner，保持 libvirt-qemu:kvm 确保 VM 可访问）
	utils.ExecCommand("chown", "libvirt-qemu:kvm", destPath)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "文件上传成功",
		"data": gin.H{
			"filename": filename,
			"size":     header.Size,
		},
	})
}

// DeleteUserStorageFile 删除存储池文件
func DeleteUserStorageFile(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_user_storage_file") {
		return
	}
	username, _ := c.Get("username")
	category := c.Param("category")
	filename := c.Param("filename")

	if category != "iso" && category != "share" && category != "disk" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的存储类别",
		})
		return
	}

	if err := service.DeleteUserFile(username.(string), category, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除文件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "文件已删除",
	})
}

// DownloadUserStorageFile 下载存储池文件
func DownloadUserStorageFile(c *gin.Context) {
	username, _ := c.Get("username")
	category := c.Param("category")
	filename := c.Param("filename")

	filePath, err := service.GetUserFilePath(username.(string), category, filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}

	c.FileAttachment(filePath, filename)
}

// GetUserISOsForVM 获取用户的 ISO 列表（给 VM 创建用）
func GetUserISOsForVM(c *gin.Context) {
	username, _ := c.Get("username")

	isos := service.GetUserISOs(username.(string))

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    isos,
	})
}

// VMShareInfo 虚拟机挂载信息（含VM名称）
type VMShareInfo struct {
	VMName     string `json:"vm_name"`
	Tag        string `json:"tag"`
	Source     string `json:"source"`
	AccessMode string `json:"access_mode"`
}

// ListUserMounts 获取用户所有VM的挂载列表
func ListUserMounts(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)
	role, _ := c.Get("role")

	var vmNames []string
	if role == "admin" {
		// 管理员扫描所有虚拟机
		result := utils.ExecCommand("virsh", "list", "--all", "--name")
		if result.Error == nil {
			for _, name := range strings.Split(result.Stdout, "\n") {
				name = strings.TrimSpace(name)
				if name != "" {
					vmNames = append(vmNames, name)
				}
			}
		}
	} else {
		// 普通用户只扫描自己拥有的VM
		vmNames = service.GetUserVMList(usernameStr)
	}

	var allMounts []VMShareInfo
	// 当前用户的 tag 前缀
	userTagPrefix := fmt.Sprintf("user_%s_", usernameStr)

	for _, vmName := range vmNames {
		shares, err := service.ListSharesInactive(vmName)
		if err != nil {
			continue
		}
		for _, s := range shares {
			// 只返回属于当前用户的挂载
			if role != "admin" && !strings.HasPrefix(s.Tag, userTagPrefix) {
				continue
			}
			allMounts = append(allMounts, VMShareInfo{
				VMName:     vmName,
				Tag:        s.Tag,
				Source:     s.Source,
				AccessMode: s.AccessMode,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    allMounts,
	})
}

// MountStorageRequest 挂载存储池请求
type MountStorageRequest struct {
	VMName   string `json:"vm_name" binding:"required"`
	Category string `json:"category" binding:"required"` // iso/share
	Readonly bool   `json:"readonly"`
}

// MountStorageToVM 挂载用户存储池到虚拟机
func MountStorageToVM(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)

	var req MountStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 需要 vm_name 和 category",
		})
		return
	}

	// 检查用户是否拥有该 VM
	if !service.UserOwnsVM(usernameStr, req.VMName) {
		// 管理员也可以操作
		role, _ := c.Get("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机",
			})
			return
		}
	}

	if err := service.MountStorageToVM(usernameStr, req.VMName, req.Category, req.Readonly); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "挂载失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "存储池已挂载到虚拟机",
	})
}

// UnmountStorageFromVM 卸载存储池
func UnmountStorageFromVM(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)
	vmName := c.Param("vmName")
	tag := c.Param("tag")

	// 检查用户是否拥有该 VM
	if !service.UserOwnsVM(usernameStr, vmName) {
		role, _ := c.Get("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机",
			})
			return
		}
	}

	if err := service.UnmountStorageFromVM(vmName, tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "卸载失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "存储池已从虚拟机卸载",
	})
}

// ==================== 用户自助创建 VM ====================

// SelfCreateVmRequest 用户自助创建VM请求
type SelfCreateVmRequest struct {
	Name            string                            `json:"name" binding:"required"`
	Remark          string                            `json:"remark"`
	VCPU            int                               `json:"vcpu" binding:"required"`
	RAM             int                               `json:"ram" binding:"required"`
	DiskSize        int                               `json:"disk_size" binding:"required"`
	DiskFormat      string                            `json:"disk_format"`
	DiskBus         string                            `json:"disk_bus"`
	OSVariant       string                            `json:"os_variant"`
	ISOPath         string                            `json:"iso_path"`
	ISOPaths        []string                          `json:"iso_paths"`
	NicModel        string                            `json:"nic_model"`
	Autostart       bool                              `json:"autostart"`
	Freeze          bool                              `json:"freeze"`
	APIC            *bool                             `json:"apic"`
	PAE             *bool                             `json:"pae"`
	RTCOffset       string                            `json:"rtc_offset"`
	RTCStartDate    string                            `json:"rtc_startdate"`
	GuestAgent      *vm_xml.VMGuestAgentConfig        `json:"guest_agent"`
	SMBIOS1         *vm_xml.VMSMBIOS1Config           `json:"smbios1"`
	OSType          string                            `json:"os_type"`
	MachineType     string                            `json:"machine_type"`
	BootType        string                            `json:"boot_type"`
	BootOrder       []string                          `json:"boot_order"`
	VideoModel      string                            `json:"video_model"`
	SpiceEnabled    *bool                             `json:"spice_enabled"` // 是否启用 SPICE 显示协议（不传=回退全局默认）
	CPUTopologyMode string                            `json:"cpu_topology_mode"`
	MemoryDynamic   *vm_memory.VMMemoryDynamicRequest `json:"memory_dynamic"`
	SwitchID        uint                              `json:"switch_id"`
	SecurityGroupID uint                              `json:"security_group_id"`
	ExtraNics       []service.AddVMInterfaceRequest   `json:"extra_nics"`
	StoragePoolID   string                            `json:"storage_pool_id"`
	PCIERootPorts   int                               `json:"pcie_root_ports,omitempty"` // q35 预留 pcie-root-port 数量
	ExtraDisks      []struct {
		Size          int    `json:"size"`
		Format        string `json:"format"`
		Bus           string `json:"bus"`
		StoragePoolID string `json:"storage_pool_id"`
	} `json:"extra_disks"`
}

// SelfCreateVm 用户自助创建虚拟机
func SelfCreateVm(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	if !requireMaintenanceModeDisabled(c, "创建并启动虚拟机") {
		return
	}
	var req SelfCreateVmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 名称、CPU、内存、磁盘大小为必填项",
		})
		return
	}

	// 名称验证
	if err := service.ValidateVMName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)
	primaryISOPath, extraISOPaths := service.NormalizeInstallISOSelection(req.ISOPath, req.ISOPaths)
	req.ISOPath = primaryISOPath
	req.ISOPaths = extraISOPaths

	// ISO 路径安全检查：必须在用户自己的 ISO 目录下
	if req.ISOPath != "" || len(req.ISOPaths) > 0 {
		userISODir := service.GetUserISODir(usernameStr)
		selectedISOPaths := append([]string{req.ISOPath}, req.ISOPaths...)
		for _, isoPath := range selectedISOPaths {
			if isoPath == "" {
				continue
			}
			// 确保 ISO 路径以用户 ISO 目录开头
			if !strings.HasPrefix(isoPath, userISODir+"/") {
				c.JSON(http.StatusForbidden, gin.H{
					"code":    403,
					"message": "只能使用自己存储池中的 ISO 镜像",
				})
				return
			}
		}
	}

	// 配额检查：系统盘和额外磁盘都计入普通用户硬盘总容量。
	totalDiskGB := req.DiskSize
	for _, disk := range req.ExtraDisks {
		if disk.Size > 0 {
			totalDiskGB += disk.Size
		}
	}
	if err := service.CheckQuota(usernameStr, req.VCPU, req.RAM, totalDiskGB); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}
	// 仅当用户指定了交换机时才解析 VPC
	if req.SwitchID != 0 {
		switchID, securityGroupID, err := service.ResolveVPCForVMCreate(usernameStr, req.SwitchID, req.SecurityGroupID)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
		req.SwitchID = switchID
		req.SecurityGroupID = securityGroupID
	}

	params := &service.CreateVMParams{
		Name:            req.Name,
		Remark:          req.Remark,
		VCPU:            req.VCPU,
		RAM:             req.RAM,
		DiskSize:        req.DiskSize,
		DiskFormat:      req.DiskFormat,
		DiskBus:         req.DiskBus,
		OSVariant:       req.OSVariant,
		ISOPath:         req.ISOPath,
		ISOPaths:        req.ISOPaths,
		NicModel:        req.NicModel,
		Autostart:       req.Autostart,
		Freeze:          req.Freeze,
		APIC:            req.APIC,
		PAE:             req.PAE,
		RTCOffset:       req.RTCOffset,
		RTCStartDate:    req.RTCStartDate,
		GuestAgent:      req.GuestAgent,
		SMBIOS1:         req.SMBIOS1,
		OSType:          req.OSType,
		MachineType:     req.MachineType,
		BootType:        req.BootType,
		BootOrder:       req.BootOrder,
		VideoModel:      req.VideoModel,
		SpiceEnabled:    req.SpiceEnabled,
		CPUTopologyMode: req.CPUTopologyMode,
		VirtType:        "kvm",
		SwitchID:        req.SwitchID,
		SecurityGroupID: req.SecurityGroupID,
		StoragePoolID:   req.StoragePoolID,
		IsAdmin:         false,
		ExtraNics:       req.ExtraNics,
		PCIERootPorts:   req.PCIERootPorts,
		MemoryDynamic: sanitizeUserMemoryDynamicRequest(
			req.MemoryDynamic,
			req.RAM,
		),
	}
	for _, disk := range req.ExtraDisks {
		if disk.Size <= 0 {
			continue
		}
		params.ExtraDisks = append(params.ExtraDisks, service.ExtraDiskParam{
			Size:          disk.Size,
			Format:        "qcow2",
			Bus:           disk.Bus,
			StoragePoolID: disk.StoragePoolID,
		})
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeCreate, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交创建任务失败: " + err.Error(),
		})
		return
	}

	// 将VM添加到用户访问列表
	_ = service.AddVMToUser(usernameStr, req.Name)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "创建任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// GetUserISOListForCreate 获取用户ISO列表（VM创建页面用）
// 管理员获取全部ISO，普通用户获取自己存储池的ISO
func GetUserISOListForCreate(c *gin.Context) {
	role, _ := c.Get("role")
	username, _ := c.Get("username")

	if role == "admin" {
		// 管理员使用全局ISO列表
		isos, err := service.GetAllISOs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "获取 ISO 列表失败: " + err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data":    isos,
		})
		return
	}

	// 普通用户返回自己存储池的 ISO
	isos := service.GetUserISOs(username.(string))
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    isos,
	})
}

// GetSelfStorageInfoForAdmin 管理员获取指定用户的存储池信息
func GetSelfStorageInfoForAdmin(c *gin.Context) {
	targetUsername := c.Param("username")
	if targetUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "用户名不能为空",
		})
		return
	}

	info, err := service.GetUserStorageInfo(targetUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取存储池信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    info,
	})
}

// CheckLargeUpload 检测上传文件是否过大需要走落盘模式
// GET /self/storage/upload-check?size=<bytes>
func CheckLargeUpload(c *gin.Context) {
	sizeStr := c.Query("size")
	fileSize, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil || fileSize <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请提供有效的文件大小参数 size（字节）",
		})
		return
	}

	// 如果未启用落盘模式（/tmp 不是 tmpfs 或空间充裕），无需提示
	if !utils.IsLargeUploadDiskMode() {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data": gin.H{
				"large_upload": false,
			},
		})
		return
	}

	// 落盘模式已启用，检测文件是否超过 /tmp 当前可用空间
	tmpAvail := utils.GetTmpAvailableBytes()
	isLarge := fileSize > tmpAvail

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"large_upload": isLarge,
			"disk_mode":    true,
			"tmp_avail":    tmpAvail,
			"file_size":    fileSize,
			"warning":      "由于文件较大，文件将实时改为落盘机制，上传速度可能会受限于磁盘写入速度。推荐在弱网环境下使用SFTP方式",
		},
	})
}
