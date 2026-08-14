package handler

// helpers.go 存放跨 handler 使用的公共工具函数，供同一 package 内各 handler 文件共享。

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
)

// respondVMListError 统一处理虚拟机列表查询失败的响应（区分 libvirt 不可用与其他错误）
func respondVMListError(c *gin.Context, err error) {
	if service.IsLibvirtUnavailableError(err) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    http.StatusServiceUnavailable,
			"message": "libvirt 服务未启动或未就绪，当前无法获取虚拟机列表",
		})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{
		"code":    500,
		"message": "获取虚拟机列表失败: " + err.Error(),
	})
}

// parseBoolQuery 解析布尔型查询参数，支持 1/true/yes/on（不区分大小写）
func parseBoolQuery(c *gin.Context, key string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// buildVMListOptions 从查询参数构建 VM 列表选项
func buildVMListOptions(c *gin.Context) service.VMListOptions {
	return service.VMListOptions{
		IncludeResourceUsage: parseBoolQuery(c, "include_resource_usage"),
		IncludeIP:            parseBoolQuery(c, "include_ip"),
	}
}

// ── VM 创建/克隆同步参数校验 ──

// validateDiskSize 校验磁盘大小必须 > 0
// resolvedSize 为经过 Resolve 等处理后的最终磁盘大小
func validateDiskSize(c *gin.Context, resolvedSize int) bool {
	if resolvedSize <= 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"code": 422, "message": "磁盘大小必须大于0GB"})
		return false
	}
	return true
}

// validateVMNameNotExists 校验虚拟机名称是否已被占用
// 使用 libvirt RPC 查询域是否已存在，若存在则返回 409
func validateVMNameNotExists(c *gin.Context, name string) bool {
	exists, err := service.DomainExistsRPC(name)
	if err != nil {
		// libvirt 连接失败时不阻断创建流程，由后续异步任务处理
		return true
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": fmt.Sprintf("虚拟机 '%s' 已存在", name)})
		return false
	}
	return true
}

// validateSwitchBridges 校验所有涉及到的 VPC 交换机对应的 OVS 网桥是否存在
// switchID: 主网口交换机 ID（0 表示不使用交换机，跳过校验）
// extraNics: 额外网口列表，每项含 SwitchID
func validateSwitchBridges(c *gin.Context, switchID uint, extraNics []service.AddVMInterfaceRequest) bool {
	switchIDsToCheck := map[uint]bool{}
	if switchID != 0 {
		switchIDsToCheck[switchID] = true
	}
	for _, nic := range extraNics {
		if nic.SwitchID != 0 {
			switchIDsToCheck[nic.SwitchID] = true
		}
	}
	for sid := range switchIDsToCheck {
		bridgeName, err := getSwitchBridgeName(sid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": fmt.Sprintf("交换机 ID %d 不存在", sid)})
			return false
		}
		if err := service.EnsureOVSBridgeExists(bridgeName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": fmt.Sprintf("网桥 %s 不存在", bridgeName)})
			return false
		}
	}
	return true
}

// getSwitchBridgeName 从数据库查询 VPC 交换机的网桥名称
func getSwitchBridgeName(switchID uint) (string, error) {
	var sw model.VPCSwitch
	if err := model.DB.Where("id = ?", switchID).First(&sw).Error; err != nil {
		return "", fmt.Errorf("交换机 ID %d 不存在", switchID)
	}
	return sw.BridgeName, nil
}

// validateBatchVMNamesNotExists 批量校验虚拟机名称是否已被占用
// prefix: 名称前缀, startNum: 起始编号, count: 批量数量
func validateBatchVMNamesNotExists(c *gin.Context, prefix string, startNum, count int) bool {
	for i := startNum; i < startNum + count; i++ {
		name := fmt.Sprintf("%s%d", prefix, i)
		exists, err := service.DomainExistsRPC(name)
		if err != nil {
			continue // libvirt 连接失败时跳过，由异步任务处理
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": fmt.Sprintf("虚拟机 '%s' 已存在", name)})
			return false
		}
	}
	return true
}
