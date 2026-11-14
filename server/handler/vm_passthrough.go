package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/service"
	"kvm_console/utils"
)

// ==================== 硬件直通 API ====================

// GetPassthroughDevices 获取 PCI 设备列表，include_all 返回完整设备及默认策略风险说明。
func GetPassthroughDevices(c *gin.Context) {
	var query struct {
		IncludeAll bool `form:"include_all"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: include_all 必须为布尔值",
		})
		return
	}

	role, _ := c.Get("role")
	if query.IncludeAll && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "仅管理员可查看全部直通设备"})
		return
	}

	devices, err := service.ListPCIDevicesForPassthrough(query.IncludeAll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取可直通设备列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    devices,
	})
}

// GetVMPassthroughDevices 获取指定虚拟机的直通设备
func GetVMPassthroughDevices(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	devices, err := service.GetVMPCIDevices(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取虚拟机直通设备失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    devices,
	})
}

// BindPCIDevice 将 PCI 设备绑定到 vfio-pci 驱动
func BindPCIDevice(c *gin.Context) {
	var req struct {
		PCIAddress      string `json:"pci_address" binding:"required"`
		UnsafeConfirmed bool   `json:"unsafe_confirmed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: pci_address 为必填项",
		})
		return
	}

	if err := service.ValidatePCIPassthrough(req.PCIAddress, req.UnsafeConfirmed); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "直通验证失败: " + err.Error(),
		})
		return
	}

	if err := service.BindPCIDeviceToVfio(req.PCIAddress, req.UnsafeConfirmed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "绑定设备失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "设备已绑定到 vfio-pci",
	})
}

// UnbindPCIDevice 从 vfio-pci 驱动解绑 PCI 设备
func UnbindPCIDevice(c *gin.Context) {
	var req struct {
		PCIAddress string `json:"pci_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: pci_address 为必填项",
		})
		return
	}

	if err := service.UnbindPCIDeviceFromVfio(req.PCIAddress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "解绑设备失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "设备已从 vfio-pci 解绑",
	})
}

// AttachPCIDeviceToVM 将 PCI 设备直通到虚拟机
func AttachPCIDeviceToVM(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	var req struct {
		PCIAddress      string `json:"pci_address" binding:"required"`
		UnsafeConfirmed bool   `json:"unsafe_confirmed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: pci_address 为必填项",
		})
		return
	}

	// 检查虚拟机运行状态
	stateResult := utils.ExecCommand("virsh", "domstate", name)
	state := strings.TrimSpace(stateResult.Stdout)
	if stateResult.Error != nil || state != "shut off" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "添加硬件直通设备需要先关机，并确保能够读取关机状态",
		})
		return
	}

	if err := service.AttachPCIDeviceToVM(name, req.PCIAddress, req.UnsafeConfirmed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "添加直通设备失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "直通设备已添加到虚拟机",
	})
}

// DetachPCIDeviceFromVM 从虚拟机移除 PCI 直通设备
func DetachPCIDeviceFromVM(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "虚拟机名称不能为空",
		})
		return
	}

	var req struct {
		PCIAddress string `json:"pci_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: pci_address 为必填项",
		})
		return
	}

	// 检查虚拟机运行状态
	stateResult := utils.ExecCommand("virsh", "domstate", name)
	state := strings.TrimSpace(stateResult.Stdout)
	if state == "running" || state == "paused" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "移除硬件直通设备需要先关机",
		})
		return
	}

	if err := service.DetachPCIDeviceFromVM(name, req.PCIAddress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "移除直通设备失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "直通设备已从虚拟机移除",
	})
}
