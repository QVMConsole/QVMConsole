package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	netservice "kvm_console/service/network"
)

// GetStaticIPList 获取静态 IP 列表（根据用户权限过滤）
func GetStaticIPList(c *gin.Context) {
	info, err := netservice.ListStaticIPs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	// 非admin用户只能看到自己拥有的虚拟机的绑定信息
	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		userVMs := service.GetUserVMList(username.(string))
		vmSet := make(map[string]bool)
		for _, vm := range userVMs {
			vmSet[vm] = true
		}

		// 过滤静态绑定
		var filteredBindings []netservice.StaticIPInfo
		for _, b := range info.StaticBindings {
			if vmSet[b.VMName] {
				filteredBindings = append(filteredBindings, b)
			}
		}
		info.StaticBindings = filteredBindings

		// 过滤 DHCP 租约：只保留属于用户VM的租约（通过MAC关联的VM名称匹配）
		var filteredLeases []netservice.DHCPLeaseInfo
		for _, lease := range info.DHCPLeases {
			if vmSet[lease.VMName] {
				filteredLeases = append(filteredLeases, lease)
			}
		}
		info.DHCPLeases = filteredLeases
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    info,
	})
}

// BindStaticIPRequest 绑定静态 IP 请求
type BindStaticIPRequest struct {
	VMName string `json:"vm_name" binding:"required"`
	IP     string `json:"ip"` // 可留空，为空时自动分配空闲 IP
}

// BindStaticIP 绑定静态 IP
func BindStaticIP(c *gin.Context) {
	var req BindStaticIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	// 非admin用户只能绑定自己拥有的虚拟机
	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		usernameStr := strings.TrimSpace(fmt.Sprint(username))
		if !service.UserOwnsVM(usernameStr, req.VMName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机",
			})
			return
		}
		if service.IsLightweightCloudUser(usernameStr) {
			assignedIP, err := netservice.ResolvePortForwardTargetIP(req.VMName, "")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": fmt.Sprintf("静态 IP 绑定成功: %s", assignedIP),
				"data": gin.H{
					"ip": assignedIP,
				},
			})
			return
		}
	}

	assignedIP, err := netservice.BindStaticIP(req.VMName, req.IP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("静态 IP 绑定成功: %s", assignedIP),
		"data": gin.H{
			"ip": assignedIP,
		},
	})
}

// UnbindStaticIPRequest 解绑请求
type UnbindStaticIPRequest struct {
	VMName string `json:"vm_name" binding:"required"`
}

// UnbindStaticIP 解绑静态 IP
func UnbindStaticIP(c *gin.Context) {
	var req UnbindStaticIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	// 非admin用户只能解绑自己拥有的虚拟机
	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		if !service.UserOwnsVM(username.(string), req.VMName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机",
			})
			return
		}
	}

	if err := netservice.UnbindStaticIP(req.VMName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "静态 IP 已解绑",
	})
}

// GetPortForwardList 获取端口转发列表
func GetPortForwardList(c *gin.Context) {
	rules, err := netservice.ListPortForwards()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		targetUser := strings.TrimSpace(username.(string))
		filtered := make([]netservice.PortForwardRule, 0, len(rules))
		for _, rule := range rules {
			if strings.TrimSpace(rule.OwnerUsername) == targetUser {
				filtered = append(filtered, rule)
			}
		}
		rules = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    rules,
	})
}

// AddPortForwardRequest 添加端口转发请求
type AddPortForwardRequest struct {
	VMName   string `json:"vm_name" binding:"required"` // 虚拟机名称
	VMIP     string `json:"vm_ip"`                      // 虚拟机 IP（可选，前端选择后直接传入）
	VMPort   string `json:"vm_port" binding:"required"` // 虚拟机端口
	HostPort string `json:"host_port"`                  // 宿主机端口，留空自动分配
	Protocol string `json:"protocol"`                   // tcp/udp/both
}

func portForwardProtocolCount(protocol string) int {
	if strings.EqualFold(strings.TrimSpace(protocol), "both") {
		return 2
	}
	return 1
}

// AddPortForward 添加端口转发（自动获取 IP、自动绑定静态 IP、自动分配端口）
func AddPortForward(c *gin.Context) {
	var req AddPortForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 需要 vm_name 和 vm_port",
		})
		return
	}

	role, _ := c.Get("role")
	username, _ := c.Get("username")
	roleStr := strings.TrimSpace(fmt.Sprint(role))
	usernameStr := strings.TrimSpace(fmt.Sprint(username))
	if role != "admin" {
		if !service.UserOwnsVM(usernameStr, req.VMName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机的端口转发",
			})
			return
		}
		quotaDelta := portForwardProtocolCount(req.Protocol)
		if service.IsLightweightCloudUser(usernameStr) {
			if err := service.CheckLightweightVMPortForwardQuota(usernameStr, req.VMName, quotaDelta); err != nil {
				// lightweight cloud quota check stays in service root
				c.JSON(http.StatusForbidden, gin.H{
					"code":    403,
					"message": err.Error(),
				})
				return
			}
		} else if err := netservice.CheckUserPortForwardFeatureEnabled(usernameStr); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		} else if err := netservice.CheckUserPortForwardQuota(usernameStr, quotaDelta); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
	}

	if err := netservice.CheckRequestedPortForwardHostPortAvailable(req.HostPort, req.Protocol, nil); err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": err.Error(),
		})
		return
	}

	vmIP, err := netservice.ResolvePortForwardTargetIP(req.VMName, req.VMIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	// 宿主机端口为空时自动分配
	hostPort := req.HostPort
	if hostPort == "" {
		port, err := netservice.AutoAllocatePort()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": err.Error(),
			})
			return
		}
		hostPort = strconv.Itoa(port)
	}

	params := &netservice.PortForwardAddParams{
		VMIP:           vmIP,
		HostPort:       hostPort,
		VMPort:         req.VMPort,
		Protocol:       req.Protocol,
		Comment:        req.VMName,
		CreatedBy:      usernameStr,
		CreatedByAdmin: roleStr == "admin",
	}

	if err := netservice.AddPortForward(params); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}
	if err := service.EnsureSecurityGroupAllowsPortForward(req.VMName, req.Protocol, req.VMPort); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "端口转发已添加，但自动补安全组策略失败: " + err.Error(),
		})
		return
	}

	// 自动保存手动 IP 映射（检查 IP 是否为手动输入的）
	if req.VMName != "" && req.VMIP != "" && !service.IsVPCBoundVM(req.VMName) {
		// 检查是否已存在相同映射
		var count int64
		model.DB.Model(&model.PortForwardIP{}).Where("vm_name = ? AND ip = ?", req.VMName, vmIP).Count(&count)
		if count == 0 {
			model.DB.Create(&model.PortForwardIP{
				VMName: req.VMName,
				IP:     vmIP,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("端口转发已添加（%s → %s:%s）", netservice.BuildPortForwardAccessAddressForMessage(hostPort), vmIP, req.VMPort),
		"data": gin.H{
			"host_port":      hostPort,
			"vm_ip":          vmIP,
			"access_ip":      netservice.GetConfiguredPortForwardHostIP(),
			"access_address": netservice.BuildPortForwardAccessAddressForMessage(hostPort),
		},
	})
}

// UpdatePortForwardRequest 编辑端口转发请求
type UpdatePortForwardRequest struct {
	VMName   string `json:"vm_name"`
	VMIP     string `json:"vm_ip"`
	VMPort   string `json:"vm_port"`
	HostPort string `json:"host_port"`
	Protocol string `json:"protocol"`
}

// UpdatePortForward 编辑单条端口转发
func UpdatePortForward(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的规则编号",
		})
		return
	}

	var req UpdatePortForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	currentRule, err := netservice.GetPortForwardRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": err.Error(),
		})
		return
	}

	role, _ := c.Get("role")
	username, _ := c.Get("username")
	roleStr := strings.TrimSpace(fmt.Sprint(role))
	usernameStr := strings.TrimSpace(fmt.Sprint(username))
	if role != "admin" {
		if strings.TrimSpace(currentRule.OwnerUsername) != usernameStr {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权编辑此端口转发规则",
			})
			return
		}
		targetVMName := strings.TrimSpace(req.VMName)
		if targetVMName == "" {
			targetVMName = strings.TrimSpace(currentRule.VMName)
		}
		if targetVMName != "" && !service.UserOwnsVM(usernameStr, targetVMName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权将端口转发指向此虚拟机",
			})
			return
		}
		if service.IsLightweightCloudUser(usernameStr) {
			oldCount := portForwardProtocolCount(currentRule.Protocol)
			newProtocol := strings.TrimSpace(req.Protocol)
			if newProtocol == "" {
				newProtocol = currentRule.Protocol
			}
			newCount := portForwardProtocolCount(newProtocol)
			quotaDelta := newCount - oldCount
			if targetVMName != strings.TrimSpace(currentRule.VMName) {
				quotaDelta = newCount
			}
			if err := service.CheckLightweightVMPortForwardQuota(usernameStr, targetVMName, quotaDelta); err != nil {
				c.JSON(http.StatusForbidden, gin.H{
					"code":    403,
					"message": err.Error(),
				})
				return
			}
		} else if err := netservice.CheckUserPortForwardFeatureEnabled(usernameStr); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
	}

	if err := netservice.CheckRequestedPortForwardHostPortAvailable(req.HostPort, req.Protocol, currentRule); err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": err.Error(),
		})
		return
	}

	comment := strings.TrimSpace(req.VMName)
	if comment == "" {
		comment = strings.TrimSpace(currentRule.VMName)
	}
	vmIP := strings.TrimSpace(req.VMIP)
	if service.IsVPCBoundVM(comment) || vmIP != "" {
		var resolveErr error
		vmIP, resolveErr = netservice.ResolvePortForwardTargetIP(comment, vmIP)
		if resolveErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": resolveErr.Error(),
			})
			return
		}
	}
	if err := netservice.UpdatePortForward(id, &netservice.PortForwardUpdateParams{
		VMIP:           vmIP,
		VMPort:         req.VMPort,
		HostPort:       req.HostPort,
		Protocol:       req.Protocol,
		Comment:        comment,
		CreatedBy:      usernameStr,
		CreatedByAdmin: roleStr == "admin",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	// 编辑后为新端口补充安全组放行规则（与新增端口转发保持一致）。
	// deletePortForwardWithOptions 会移除旧端口的安全组规则，
	// 若不在此处为新端口补上，VPC 管理的 VM 会因 nftables ACL 拦截导致转发失效。
	if comment != "" {
		forwardVMPort := strings.TrimSpace(req.VMPort)
		if forwardVMPort == "" {
			forwardVMPort = strings.TrimSpace(currentRule.DestPort)
		}
		forwardProto := strings.TrimSpace(req.Protocol)
		if forwardProto == "" {
			forwardProto = strings.TrimSpace(currentRule.Protocol)
		}
		if forwardVMPort != "" {
			if err := service.EnsureSecurityGroupAllowsPortForward(comment, forwardProto, forwardVMPort); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "端口转发规则已更新，但自动补安全组策略失败: " + err.Error(),
				})
				return
			}
		}
	}

	if comment != "" && req.VMIP != "" && !service.IsVPCBoundVM(comment) {
		var count int64
		model.DB.Model(&model.PortForwardIP{}).Where("vm_name = ? AND ip = ?", comment, req.VMIP).Count(&count)
		if count == 0 {
			_ = model.DB.Create(&model.PortForwardIP{
				VMName: comment,
				IP:     req.VMIP,
			}).Error
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "端口转发规则已更新",
	})
}

// BatchDeletePortForwardRequest 批量删除端口转发请求
type BatchDeletePortForwardRequest struct {
	IDs []int `json:"ids" binding:"required"`
}

// BatchDeletePortForward 批量删除端口转发
func BatchDeletePortForward(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_port_forward") {
		return
	}

	var req BatchDeletePortForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 需要规则编号列表",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		usernameStr := strings.TrimSpace(username.(string))
		for _, id := range req.IDs {
			rule, err := netservice.GetPortForwardRuleByID(id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{
					"code":    404,
					"message": err.Error(),
				})
				return
			}
			if strings.TrimSpace(rule.OwnerUsername) != usernameStr {
				c.JSON(http.StatusForbidden, gin.H{
					"code":    403,
					"message": "存在不属于当前用户的端口转发规则，无法批量删除",
				})
				return
			}
		}
	}

	if err := netservice.DeletePortForwards(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "端口转发规则已批量删除",
	})
}

// DeletePortForward 删除端口转发
func DeletePortForward(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_port_forward") {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的规则编号",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		usernameStr := strings.TrimSpace(username.(string))
		rule, err := netservice.GetPortForwardRuleByID(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": err.Error(),
			})
			return
		}
		if strings.TrimSpace(rule.OwnerUsername) != usernameStr {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权删除此端口转发规则",
			})
			return
		}
	}

	if err := netservice.DeletePortForward(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "端口转发规则已删除",
	})
}

// SavePortForwardRules 持久化端口转发规则
func SavePortForwardRules(c *gin.Context) {
	if err := netservice.SavePortForwardRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "规则已保存",
	})
}

// GetUFWStatus 获取 UFW 状态
func GetUFWStatus(c *gin.Context) {
	status, err := netservice.GetUFWStatus()
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
		"data":    status,
	})
}

// UFWRuleRequest UFW 规则请求
type UFWRuleRequest struct {
	Action string `json:"action" binding:"required"` // allow/deny/delete
	Rule   string `json:"rule" binding:"required"`   // 端口/规则
}

// ManageUFWRule 管理 UFW 规则
func ManageUFWRule(c *gin.Context) {
	var req UFWRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	if err := netservice.ManageUFWRule(req.Action, req.Rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "UFW 规则操作成功",
	})
}

// ============================================================
// 端口转发手动 IP 映射管理
// ============================================================

// GetPortForwardIPs 获取指定虚拟机的手动 IP 映射列表
func GetPortForwardIPs(c *gin.Context) {
	vmName := c.Query("vm_name")
	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		usernameStr := username.(string)
		if vmName == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "普通用户查询端口转发 IP 映射时必须指定虚拟机",
			})
			return
		}
		if !service.UserOwnsVM(usernameStr, vmName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权查看此虚拟机的 IP 映射",
			})
			return
		}
	}

	var ips []model.PortForwardIP
	query := model.DB.Model(&model.PortForwardIP{})
	if vmName != "" {
		query = query.Where("vm_name = ?", vmName)
	}
	query.Find(&ips)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    ips,
	})
}

// AddPortForwardIPRequest 添加手动 IP 映射请求
type AddPortForwardIPRequest struct {
	VMName string `json:"vm_name" binding:"required"`
	IP     string `json:"ip" binding:"required"`
}

// AddPortForwardIP 添加手动 IP 映射
func AddPortForwardIP(c *gin.Context) {
	var req AddPortForwardIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: 需要 vm_name 和 ip",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		if !service.UserOwnsVM(username.(string), req.VMName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权操作此虚拟机的 IP 映射",
			})
			return
		}
	}

	// 检查是否已存在
	var count int64
	model.DB.Model(&model.PortForwardIP{}).Where("vm_name = ? AND ip = ?", req.VMName, req.IP).Count(&count)
	if count > 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "映射已存在",
		})
		return
	}

	record := model.PortForwardIP{
		VMName: req.VMName,
		IP:     req.IP,
	}
	if err := model.DB.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": fmt.Sprintf("保存失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "映射已保存",
	})
}

// DeletePortForwardIP 删除手动 IP 映射
func DeletePortForwardIP(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_port_forward_ip") {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的 ID",
		})
		return
	}

	role, _ := c.Get("role")
	if role != "admin" {
		username, _ := c.Get("username")
		usernameStr := username.(string)
		var record model.PortForwardIP
		if err := model.DB.First(&record, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "映射不存在",
			})
			return
		}
		if !service.UserOwnsVM(usernameStr, record.VMName) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权删除此 IP 映射",
			})
			return
		}
	}

	if err := model.DB.Delete(&model.PortForwardIP{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": fmt.Sprintf("删除失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "映射已删除",
	})
}
