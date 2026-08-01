package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/taskqueue"
	"kvm_console/utils"
)

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username                    string                                     `json:"username" binding:"required"`
	Email                       string                                     `json:"email"`
	Password                    string                                     `json:"password"`   // 选填；填写后直接创建激活用户
	Role                        string                                     `json:"role"`       // admin/user
	CloudType                   string                                     `json:"cloud_type"` // elastic/lightweight
	DedicatedVPCSwitchID        uint                                       `json:"dedicated_vpc_switch_id"`
	MaxCPU                      int                                        `json:"max_cpu"`           // CPU配额
	MaxMemory                   int                                        `json:"max_memory"`        // 内存配额(GB)
	MaxDisk                     int                                        `json:"max_disk"`          // 磁盘配额(GB)
	MaxVM                       int                                        `json:"max_vm"`            // 最大VM数量
	MaxStorage                  int                                        `json:"max_storage"`       // 存储配额(GB)
	MaxRuntimeHours             int                                        `json:"max_runtime_hours"` // 总运行时长配额(小时)
	EnablePortForward           *bool                                      `json:"enable_port_forward"`
	MaxPortForwards             *int                                       `json:"max_port_forwards"`  // 端口转发数量配额
	MaxSnapshots                *int                                       `json:"max_snapshots"`      // 快照数量配额
	MaxBandwidthUp              float64                                    `json:"max_bandwidth_up"`   // 上行带宽(Mbps)
	MaxBandwidthDown            float64                                    `json:"max_bandwidth_down"` // 下行带宽(Mbps)
	MaxTrafficDown              float64                                    `json:"max_traffic_down"`   // 下行日流量(GB)
	MaxTrafficUp                float64                                    `json:"max_traffic_up"`     // 上行日流量(GB)
	MaxPublicIPs                int                                        `json:"max_public_ips"`     // 公网 IP 数量
	LightweightVMRegistrations  []service.LightweightVMRegistrationRequest `json:"lightweight_vm_registrations"`
	LightweightExistingVMs      []string                                   `json:"lightweight_existing_vms"`       // 选择已有VM列表
	LightweightExistingVMQuotas []service.LightweightVMQuotaRequest        `json:"lightweight_existing_vm_quotas"` // 已有VM配额
}

// UpdateUserAccountRequest 更新用户账户资料请求
type UpdateUserAccountRequest struct {
	Email    *string `json:"email"`
	Password string  `json:"password"`
}

// UpdateQuotaRequest 更新配额请求
type UpdateQuotaRequest struct {
	MaxCPU               int     `json:"max_cpu"`
	MaxMemory            int     `json:"max_memory"`
	MaxDisk              int     `json:"max_disk"`
	MaxVM                int     `json:"max_vm"`
	MaxStorage           int     `json:"max_storage"`
	MaxRuntimeHours      int     `json:"max_runtime_hours"`
	EnablePortForward    *bool   `json:"enable_port_forward"`
	MaxPortForwards      int     `json:"max_port_forwards"`
	MaxSnapshots         int     `json:"max_snapshots"`
	MaxBandwidthUp       float64 `json:"max_bandwidth_up"`
	MaxBandwidthDown     float64 `json:"max_bandwidth_down"`
	MaxTrafficDown       float64 `json:"max_traffic_down"`
	MaxTrafficUp         float64 `json:"max_traffic_up"`
	MaxPublicIPs         int     `json:"max_public_ips"`
	CloudType            string  `json:"cloud_type"`
	DedicatedVPCSwitchID uint    `json:"dedicated_vpc_switch_id"`
}

// AssignVMsRequest 分配虚拟机请求
type AssignVMsRequest struct {
	VMs               []string                            `json:"vms" binding:"required"`
	LightweightQuotas []service.LightweightVMQuotaRequest `json:"lightweight_quotas"`
}

// UpdateUserStatusRequest 更新用户状态请求
type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// GetUserList 获取用户列表
func GetUserList(c *gin.Context) {
	users, err := service.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取用户列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    users,
	})
}

func resolveCreateUserMaxPortForwards(role string, value *int) int {
	if role != "user" {
		if value == nil {
			return 0
		}
		return *value
	}
	if value == nil {
		return 10
	}
	return *value
}

func resolveCreateUserMaxSnapshots(role string, value *int) int {
	if role != "user" {
		if value == nil {
			return 0
		}
		return *value
	}
	if value == nil {
		return 5
	}
	return *value
}

func resolveCreateUserEnablePortForward(role string, value *bool) bool {
	if value != nil {
		return *value
	}
	if role != "user" {
		return true
	}
	return true
}

func resolveUpdateUserEnablePortForward(username string, value *bool) (bool, error) {
	if value != nil {
		return *value, nil
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return false, err
	}
	return user.EnablePortForward, nil
}

func validateManagedPassword(password string) error {
	return service.ValidateStrongPassword(password)
}

// CreateUser 创建用户
func CreateUser(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_user") {
		return
	}
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "user"
	}
	cloudType := service.NormalizeCloudType(req.CloudType)
	enablePortForward := resolveCreateUserEnablePortForward(role, req.EnablePortForward)
	maxPortForwards := resolveCreateUserMaxPortForwards(role, req.MaxPortForwards)
	maxSnapshots := resolveCreateUserMaxSnapshots(role, req.MaxSnapshots)

	email := strings.TrimSpace(req.Email)
	password := req.Password
	dedicatedVPCSwitchID := req.DedicatedVPCSwitchID
	useExistingVMs := service.IsLightweightCloudType(cloudType) && len(req.LightweightExistingVMs) > 0
	if useExistingVMs {
		dedicatedVPCSwitchID = 0
	}

	// 填写密码时直接创建激活用户；留空时才进入邮件邀请流程。
	smtpConfigured := service.IsSMTPConfigured()
	if password == "" && !smtpConfigured {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "SMTP 未配置时，必须为用户设置初始密码",
		})
		return
	}
	if password == "" && email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "未设置初始密码时，必须填写邮箱以发送注册邀请",
		})
		return
	}
	if password != "" {
		if err := validateManagedPassword(password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "密码不符合要求: " + err.Error(),
			})
			return
		}

		// 直接创建激活用户
		user, err := service.CreateActiveUserDirectly(req.Username, email, password, role, cloudType, dedicatedVPCSwitchID, useExistingVMs,
			req.MaxCPU, req.MaxMemory, req.MaxDisk, req.MaxVM, req.MaxStorage, req.MaxRuntimeHours,
			enablePortForward, maxPortForwards, maxSnapshots,
			req.MaxBandwidthUp, req.MaxBandwidthDown, req.MaxTrafficDown, req.MaxTrafficUp, req.MaxPublicIPs)
		if err != nil {
			if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "已被使用") {
				c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "创建用户失败: " + err.Error()})
			return
		}

		// 处理轻量云 VM 注册
		if service.IsLightweightCloudType(cloudType) && len(req.LightweightVMRegistrations) > 0 {
			if _, err := service.CreateLightweightVMRegistrations(user.Username, req.LightweightVMRegistrations, "admin"); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 200, "message": "用户已创建，但轻量云 VM 注册失败: " + err.Error(), "data": gin.H{"username": user.Username}})
				return
			}
		}
		if service.IsLightweightCloudType(cloudType) && len(req.LightweightExistingVMs) > 0 {
			quotaByVM := make(map[string]service.LightweightVMQuotaRequest)
			for _, quota := range req.LightweightExistingVMQuotas {
				quotaByVM[quota.VMName] = quota
			}
			quotas := make([]service.LightweightVMQuotaRequest, 0, len(req.LightweightExistingVMs))
			for _, vmName := range req.LightweightExistingVMs {
				if q, ok := quotaByVM[vmName]; ok {
					quotas = append(quotas, q)
				} else {
					quotas = append(quotas, service.LightweightVMQuotaRequest{
						VMName: vmName, MaxPortForwards: 10, MaxSnapshots: 2,
					})
				}
			}
			_ = service.AssignVMsToUserWithQuotas(user.Username, req.LightweightExistingVMs, quotas)
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "用户已创建，可直接使用初始密码登录",
			"data":    gin.H{"username": user.Username},
		})
		return
	}

	user, inviteToken, err := service.CreatePendingInvitedUserWithExistingVMs(req.Username, req.Email, role, cloudType, dedicatedVPCSwitchID, useExistingVMs,
		req.MaxCPU, req.MaxMemory, req.MaxDisk, req.MaxVM, req.MaxStorage, req.MaxRuntimeHours, enablePortForward, maxPortForwards, maxSnapshots,
		req.MaxBandwidthUp, req.MaxBandwidthDown, req.MaxTrafficDown, req.MaxTrafficUp, req.MaxPublicIPs)
	if err != nil {
		// 用户名或邮箱冲突返回 409 Conflict
		if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "已被使用") {
			c.JSON(http.StatusConflict, gin.H{
				"code":    409,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建用户失败: " + err.Error(),
		})
		return
	}

	if service.IsLightweightCloudType(cloudType) && len(req.LightweightVMRegistrations) > 0 {
		if _, err := service.CreateLightweightVMRegistrations(user.Username, req.LightweightVMRegistrations, "admin"); err != nil {
			model.DB.Delete(user)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "创建轻量云 VM 注册失败: " + err.Error(),
			})
			return
		}
	}

	// 分配已有VM给用户
	if service.IsLightweightCloudType(cloudType) && len(req.LightweightExistingVMs) > 0 {
		// 将配额请求转换为VM名到配额的映射
		quotaByVM := make(map[string]service.LightweightVMQuotaRequest)
		for _, quota := range req.LightweightExistingVMQuotas {
			quotaByVM[quota.VMName] = quota
		}

		// 构建配额请求列表
		quotas := make([]service.LightweightVMQuotaRequest, 0, len(req.LightweightExistingVMs))
		for _, vmName := range req.LightweightExistingVMs {
			if q, ok := quotaByVM[vmName]; ok {
				quotas = append(quotas, q)
			} else {
				quotas = append(quotas, service.LightweightVMQuotaRequest{
					VMName:          vmName,
					MaxPortForwards: 10,
					MaxSnapshots:    2,
				})
			}
		}

		if err := service.AssignVMsToUserWithQuotas(user.Username, req.LightweightExistingVMs, quotas); err != nil {
			model.DB.Delete(user)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "分配已有 VM 失败: " + err.Error(),
			})
			return
		}
	}

	inviteURL := buildBaseURL(c) + "/invite?token=" + inviteToken
	if sendErr := service.SendInviteEmail(user, inviteURL); sendErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "邀请用户已创建，但邮件发送失败，请检查 SMTP 配置后重发邀请",
			"data": gin.H{
				"invite_url": inviteURL,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "邀请邮件已发送",
		"data": gin.H{
			"invite_url": inviteURL,
		},
	})
}

// UpdateUserAccount 管理员更新用户邮箱和密码。
func UpdateUserAccount(c *gin.Context) {
	if !requireHighRiskVerification(c, "update_user_account") {
		return
	}

	username := strings.TrimSpace(c.Param("username"))
	var req UpdateUserAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if req.Email == nil && req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请至少提交邮箱或新密码"})
		return
	}
	if req.Password != "" {
		if err := validateManagedPassword(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "密码不符合要求: " + err.Error()})
			return
		}
	}

	user, activated, err := service.UpdateManagedUserAccount(username, req.Email, req.Password)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不能为空") || strings.Contains(err.Error(), "密码") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"code": status, "message": err.Error()})
		return
	}
	message := "用户账户资料已更新"
	if activated {
		message = "用户账户资料已更新，待邀请账户已激活，可直接登录"
	} else if req.Password != "" {
		if req.Email == nil {
			message = "用户登录密码已更新"
		} else {
			message = "用户邮箱和密码已更新"
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": message,
		"data": gin.H{
			"username":  user.Username,
			"email":     user.Email,
			"status":    user.Status,
			"activated": activated,
		},
	})
}

// CreateLightweightVMRegistrations 管理员为轻量云用户登记待开通 VM。
func CreateLightweightVMRegistrations(c *gin.Context) {
	username := c.Param("username")
	var req struct {
		Registrations []service.LightweightVMRegistrationRequest `json:"registrations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if len(req.Registrations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请至少添加一台待注册 VM"})
		return
	}
	operator, _ := c.Get("username")
	createdBy, _ := operator.(string)
	regs, err := service.CreateLightweightVMRegistrations(username, req.Registrations, createdBy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err == nil && user.Status == service.UserStatusActive && strings.TrimSpace(user.Email) != "" {
		if sendErr := service.SendLightweightVMRegistrationEmail(&user, buildBaseURL(c)+"/vm/list", regs); sendErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "轻量云 VM 已登记，但确认邮件发送失败，请检查 SMTP 配置",
				"data":    regs,
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "轻量云 VM 已登记",
		"data":    regs,
	})
}

// DeleteLightweightVMRegistration 删除未开通的轻量云 VM 注册。
func DeleteLightweightVMRegistration(c *gin.Context) {
	username := c.Param("username")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "注册记录 ID 无效"})
		return
	}
	if err := service.DeleteLightweightVMRegistration(username, uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "注册记录已删除"})
}

// RemoveLightweightVMRegistrationByVMName 将已开通 VM 从轻量云注册列表中移除。
func RemoveLightweightVMRegistrationByVMName(c *gin.Context) {
	username := c.Param("username")
	vmName := c.Param("vmName")
	if err := service.RemoveLightweightVMRegistrationByVMName(username, vmName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "轻量云 VM 已移除"})
}

// DeleteLightweightVM 删除已开通的轻量云 VM，并在删除成功后移除所属记录。
func DeleteLightweightVM(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_vm") {
		return
	}
	username := c.Param("username")
	vmName := c.Param("vmName")
	if err := service.ValidateLightweightVMRemoval(username, vmName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if _, _, _, _, err := libvirt_rpc.GetDomainInfoRPC(vmName); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "虚拟机不存在"})
		return
	}
	if service.IsVMLocked(vmName) {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "该虚拟机已锁定，无法删除。请先解锁后再操作。"})
		return
	}

	operator, _ := c.Get("username")
	operatorName, _ := operator.(string)
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDelete, map[string]interface{}{
		"name":                 vmName,
		"lightweight_username": username,
	}, operatorName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交删除任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除任务已提交",
		"data":    gin.H{"task_id": task.ID},
	})
}

// UpdateLightweightVMQuota 更新轻量云单 VM 的流量、带宽和端口转发配额。
func UpdateLightweightVMQuota(c *gin.Context) {
	username := c.Param("username")
	var req service.LightweightVMQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	quota, reg, err := service.UpdateLightweightVMQuotaByVMName(username, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "轻量云 VM 配额已更新",
		"data": gin.H{
			"quota":        quota,
			"registration": reg,
		},
	})
}

// ResendInvite 重发邀请
func ResendInvite(c *gin.Context) {
	username := c.Param("username")
	user, inviteToken, err := service.ResendInviteToken(username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	inviteURL := buildBaseURL(c) + "/invite?token=" + inviteToken
	if err := service.SendInviteEmail(user, inviteURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "重发邀请失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "邀请邮件已重发",
		"data": gin.H{
			"invite_url": inviteURL,
		},
	})
}

// UpdateUserQuota 更新用户配额
func UpdateUserQuota(c *gin.Context) {
	username := c.Param("username")

	// 管理员不能为自己设置配额
	operator, _ := c.Get("username")
	operatorStr, _ := operator.(string)
	if username == operatorStr {
		var targetUser model.User
		if err := model.DB.Where("username = ?", username).First(&targetUser).Error; err == nil && targetUser.Role == "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "管理员不能为自己设置配额",
			})
			return
		}
	}

	var req UpdateQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	enablePortForward, err := resolveUpdateUserEnablePortForward(username, req.EnablePortForward)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "读取端口转发开关失败: " + err.Error(),
		})
		return
	}

	cloudType := service.NormalizeCloudType(req.CloudType)
	if strings.TrimSpace(req.CloudType) == "" {
		var user model.User
		if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "读取用户类型失败: " + err.Error(),
			})
			return
		}
		cloudType = service.NormalizeCloudType(user.CloudType)
		req.DedicatedVPCSwitchID = user.DedicatedVPCSwitchID
	}
	if err := service.UpdateUserCloudProfile(username, cloudType, req.DedicatedVPCSwitchID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新用户类型失败: " + err.Error(),
		})
		return
	}

	if !service.IsLightweightCloudType(cloudType) {
		if err := service.UpdateUserQuota(username, req.MaxCPU, req.MaxMemory, req.MaxDisk, req.MaxVM, req.MaxStorage, req.MaxRuntimeHours, enablePortForward, req.MaxPortForwards, req.MaxSnapshots,
			req.MaxBandwidthUp, req.MaxBandwidthDown, req.MaxTrafficDown, req.MaxTrafficUp, req.MaxPublicIPs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "更新配额失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "配额更新成功",
	})
}

// GetUserQuotaUsage 获取用户配额使用情况
func GetUserQuotaUsage(c *gin.Context) {
	username := c.Param("username")

	usage, err := service.GetUserQuotaUsage(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取配额信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    usage,
	})
}

// AssignVMs 分配虚拟机给用户
func AssignVMs(c *gin.Context) {
	username := c.Param("username")
	var req AssignVMsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	if err := service.AssignVMsToUserWithQuotas(username, req.VMs, req.LightweightQuotas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "分配失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "虚拟机分配成功",
	})

	// 分配后重新计算该用户所有 VM 的带宽
	assignedVMs := append([]string(nil), req.VMs...)
	go func() {
		defer utils.RecoverAndLog("user-assign-vm-bandwidth")
		if service.IsLightweightCloudUser(username) {
			for _, vmName := range assignedVMs {
				if err := service.ApplyLightweightVMBandwidth(vmName); err != nil {
					logger.App.Warn("分配轻量云 VM 后应用带宽失败", "vm", vmName, "error", err)
				}
			}
			return
		}
		if err := service.RebalanceUserBandwidth(username); err != nil {
			logger.App.Warn("分配VM后重新分配用户带宽失败", "user", username, "error", err)
		}
	}()
}

// UpdateUserStatus 更新用户状态（封禁/解封）
func UpdateUserStatus(c *gin.Context) {
	if !requireHighRiskVerification(c, "change_user_status") {
		return
	}

	username := c.Param("username")
	var req UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	targetStatus := strings.TrimSpace(req.Status)
	operator, _ := c.Get("username")
	operatorStr, _ := operator.(string)

	// 不允许修改内置超级管理员的状态
	if username == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不能修改内置超级管理员的状态",
		})
		return
	}

	// 管理员不能修改自己的状态
	if username == operatorStr {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "管理员不能修改自己的状态",
		})
		return
	}

	switch targetStatus {
	case service.UserStatusDisabled:
		params := map[string]string{"username": username}
		task, err := taskqueue.SubmitWithStruct(model.TaskTypeDisableUser, params, operator.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "提交封禁用户任务失败: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "封禁用户任务已提交",
			"data": gin.H{
				"task_id": task.ID,
			},
		})
	case service.UserStatusActive:
		if err := service.UpdateUserStatus(username, service.UserStatusActive); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "用户已解封",
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不支持的用户状态",
		})
	}
}

// DeleteUser 删除用户（异步任务，级联删除所有资产）
func DeleteUser(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_user") {
		return
	}
	username := c.Param("username")

	// 不允许删除内置超级管理员
	if username == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不能删除内置超级管理员用户",
		})
		return
	}

	// 管理员不能删除自己
	operator, _ := c.Get("username")
	operatorStr, _ := operator.(string)
	if username == operatorStr {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "管理员不能删除自己",
		})
		return
	}

	// 提交异步删除用户任务
	params := map[string]string{"username": username}
	operator, _ = c.Get("username")
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDeleteUser, params, operator.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交删除用户任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除用户任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ToggleUserSSHRequest SSH 开关请求
type ToggleUserSSHRequest struct {
	Enabled bool `json:"enabled"`
}

// ToggleUserSSH 切换用户 SSH 访问权限
func ToggleUserSSH(c *gin.Context) {
	username := c.Param("username")
	var req ToggleUserSSHRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	if err := service.SetUserSSH(username, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "切换 SSH 状态失败: " + err.Error(),
		})
		return
	}

	status := "已关闭"
	if req.Enabled {
		status = "已开启"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "用户 " + username + " 的 SSH 访问" + status,
	})
}

// ResetUserTraffic 重置用户流量配额
func ResetUserTraffic(c *gin.Context) {
	username := c.Param("username")

	if err := service.ResetUserTrafficQuota(username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "重置流量配额失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "用户 " + username + " 的流量配额已重置",
	})
}
