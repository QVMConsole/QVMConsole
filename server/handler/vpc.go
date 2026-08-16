package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

func currentUserAndRole(c *gin.Context) (string, string) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	usernameStr, _ := username.(string)
	roleStr, _ := role.(string)
	return usernameStr, roleStr
}

func GetVPCQuota(c *gin.Context) {
	username, role := currentUserAndRole(c)
	target := username
	if role == "admin" && c.Query("username") != "" {
		target = c.Query("username")
	}
	info, err := service.GetVPCQuota(target)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": info})
}

func ListVPCSwitches(c *gin.Context) {
	username, role := currentUserAndRole(c)
	switches, err := service.ListVPCSwitches(username, role, c.Query("username"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": switches})
}

func CreateVPCSwitch(c *gin.Context) {
	username, role := currentUserAndRole(c)
	var req service.VPCSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if (role == "admin" && strings.TrimSpace(req.UplinkIF) != "") || (role != "admin" && req.InternetEnabled != nil && *req.InternetEnabled) {
		if !requireHighRiskVerification(c, "create_vpc_switch_physical_uplink") {
			return
		}
	}
	sw, err := service.CreateVPCSwitch(username, role, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "交换机已创建", "data": sw})
}

func UpdateVPCSwitch(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var req service.VPCSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	sw, err := service.UpdateVPCSwitch(username, role, uint(id), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "交换机已更新", "data": sw})
}

// ReconfigureVPCSwitch 提交交换机上行链路和 DHCP/NAT 拓扑重配置任务。
func ReconfigureVPCSwitch(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "交换机 ID 无效"})
		return
	}
	var req service.VPCSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if _, err := service.ValidateVPCSwitchReconfigure(username, role, uint(id), req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if !requireHighRiskVerification(c, "vpc_switch_reconfigure") {
		return
	}
	params := service.VPCSwitchReconfigureParams{SwitchID: uint(id), Request: req, Operator: username, Role: role}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeVPCSwitchReconfigure, params, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交交换机重配置任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "交换机重配置任务已提交",
		"data":    gin.H{"task_id": task.ID, "status": task.Status},
	})
}

func GetVPCSwitchVMs(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	vms, err := service.GetVPCSwitchVMs(username, role, uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": vms})
}

func DeleteVPCSwitch(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	force := c.Query("force") == "true"
	if id > 0 {
		var sw model.VPCSwitch
		if err := model.DB.First(&sw, uint(id)).Error; err == nil && strings.TrimSpace(sw.UplinkIF) != "" {
			if !requireHighRiskVerification(c, "delete_vpc_switch_physical_uplink") {
				return
			}
		}
	}
	if err := service.DeleteVPCSwitch(username, role, uint(id), force); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "交换机已删除"})
}

func ResetVPCSwitchTraffic(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	if err := service.ResetVPCSwitchTraffic(username, role, uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "交换机流量计数器已重置"})
}

func ListVPCSecurityGroups(c *gin.Context) {
	username, role := currentUserAndRole(c)
	groups, err := service.ListVPCSecurityGroups(username, role, c.Query("username"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": groups})
}

func CreateVPCSecurityGroup(c *gin.Context) {
	username, role := currentUserAndRole(c)
	var req service.VPCSecurityGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	group, err := service.CreateVPCSecurityGroup(username, role, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "安全组已创建", "data": group})
}

func UpdateVPCSecurityGroup(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var req service.VPCSecurityGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	group, err := service.UpdateVPCSecurityGroup(username, role, uint(id), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "安全组已更新", "data": group})
}

func DeleteVPCSecurityGroup(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	if err := service.DeleteVPCSecurityGroup(username, role, uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "安全组已删除"})
}

func AddVPCSecurityGroupRule(c *gin.Context) {
	username, role := currentUserAndRole(c)
	groupID, _ := strconv.Atoi(c.Param("id"))
	var req service.VPCSecurityGroupRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	rule, err := service.AddVPCSecurityGroupRule(username, role, uint(groupID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.ApplyVPCACLRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "规则已保存，但应用 VPC ACL 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "规则已添加", "data": rule})
}

func UpdateVPCSecurityGroupRule(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var req service.VPCSecurityGroupRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	rule, err := service.UpdateVPCSecurityGroupRule(username, role, uint(id), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.ApplyVPCACLRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "规则已保存，但应用 VPC ACL 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "规则已更新", "data": rule})
}

func DeleteVPCSecurityGroupRule(c *gin.Context) {
	username, role := currentUserAndRole(c)
	id, _ := strconv.Atoi(c.Param("id"))
	if err := service.DeleteVPCSecurityGroupRule(username, role, uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := service.ApplyVPCACLRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "规则已删除，但应用 VPC ACL 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "规则已删除"})
}

func PreviewVPCACL(c *gin.Context) {
	_, role := currentUserAndRole(c)
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "仅管理员可预览 VPC ACL"})
		return
	}
	rules, err := service.PreviewVPCACLRules()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": rules})
}

func ApplyVPCACL(c *gin.Context) {
	_, role := currentUserAndRole(c)
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "仅管理员可应用 VPC ACL"})
		return
	}
	if !requireHighRiskVerification(c, "apply_vpc_acl") {
		return
	}
	if err := service.ApplyVPCACLRules(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "VPC ACL 已应用"})
}

func GetVMVPCBinding(c *gin.Context) {
	username, role := currentUserAndRole(c)
	info, err := service.GetVPCBindingInfo(username, role, c.Param("name"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": info})
}

type bindVMVPCRequest struct {
	SwitchID        uint `json:"switch_id"`
	SecurityGroupID uint `json:"security_group_id"`
}

func BindVMVPC(c *gin.Context) {
	username, role := currentUserAndRole(c)
	vmName := c.Param("name")
	var req bindVMVPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if role == "admin" {
		if err := service.BindVMToVPCAsAdmin(vmName, req.SwitchID, req.SecurityGroupID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "VPC 绑定已更新"})
		return
	}
	if service.IsLightweightCloudUser(username) {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "轻量云服务器使用管理员分配的专用 VPC，不能切换 VPC"})
		return
	}
	if service.IsSystemVPCSwitch(req.SwitchID) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "系统基础网络交换机不可选择，请使用自己的交换机"})
		return
	}
	if err := service.BindVMToVPC(username, vmName, req.SwitchID, req.SecurityGroupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "VPC 绑定已更新"})
}

type switchSecurityGroupRequest struct {
	SecurityGroupID uint `json:"security_group_id"`
}

func SwitchVMSecurityGroup(c *gin.Context) {
	username, role := currentUserAndRole(c)
	var req switchSecurityGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if err := service.SwitchVMSecurityGroup(username, role, c.Param("name"), req.SecurityGroupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "安全组已切换"})
}

// ==================== 多网口管理（管理员全量；弹性云用户可自助管理本人虚拟机的附加网口） ====================

// ListVMInterfaces 列出虚拟机所有网口绑定
func ListVMInterfaces(c *gin.Context) {
	username, role := currentUserAndRole(c)
	vmName := c.Param("name")
	if role != "admin" && !service.UserOwnsVM(username, vmName) {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "无权操作此虚拟机"})
		return
	}
	interfaces, err := service.ListVMInterfaces(vmName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": interfaces})
}

// AddVMInterface 为虚拟机新增网口
func AddVMInterface(c *gin.Context) {
	username, role := currentUserAndRole(c)
	vmName := c.Param("name")
	var req service.AddVMInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	var info *service.VMInterfaceInfo
	var err error
	if role == "admin" {
		info, err = service.AddVMInterface(vmName, req)
	} else {
		info, err = service.AddVMInterfaceAsUser(username, vmName, req)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "网口已添加", "data": info})
}

// RemoveVMInterface 删除虚拟机指定网口
func RemoveVMInterface(c *gin.Context) {
	username, role := currentUserAndRole(c)
	vmName := c.Param("name")
	orderStr := c.Param("order")
	order, err := strconv.Atoi(orderStr)
	if err != nil || order < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "网口序号无效"})
		return
	}
	if role == "admin" {
		err = service.RemoveVMInterface(vmName, order)
	} else {
		err = service.RemoveVMInterfaceAsUser(username, vmName, order)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "网口已删除"})
}

// UpdateVMInterface 更新虚拟机指定网口的 VPC 绑定
func UpdateVMInterface(c *gin.Context) {
	username, role := currentUserAndRole(c)
	vmName := c.Param("name")
	orderStr := c.Param("order")
	order, err := strconv.Atoi(orderStr)
	if err != nil || order < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "网口序号无效"})
		return
	}
	var req service.AddVMInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if role == "admin" {
		err = service.UpdateVMInterface(vmName, order, req)
	} else {
		err = service.UpdateVMInterfaceAsUser(username, vmName, order, req)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "网口已更新"})
}
