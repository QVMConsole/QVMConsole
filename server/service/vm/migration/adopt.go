package migration

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"kvm_console/model"
	"kvm_console/service"
	netpkg "kvm_console/service/network"
)

func AdoptMigratedVM(req MigrationAdoptRequest) (*MigrationAdoptResult, error) {
	req.VMName = strings.TrimSpace(req.VMName)
	req.Owner = strings.TrimSpace(req.Owner)
	if req.VMName == "" || req.Owner == "" {
		return nil, fmt.Errorf("虚拟机和用户不能为空")
	}
	if !service.DomainExists(req.VMName) {
		return nil, fmt.Errorf("目标节点尚未定义虚拟机 %s", req.VMName)
	}

	if err := ensureMigratedVMNVRAM(req.VMName); err != nil {
		return nil, fmt.Errorf("确保虚拟机 NVRAM 文件失败: %w", err)
	}

	createdUser, err := ensureMigrationTargetUser(req)
	if err != nil {
		return nil, err
	}
	if req.Owner != "admin" {
		if err := service.AddVMToUser(req.Owner, req.VMName); err != nil {
			return nil, fmt.Errorf("绑定 VM 到目标用户失败: %w", err)
		}
	}
	if service.IsLightweightCloudType(req.CloudType) {
		if req.LightweightQuota != nil {
			req.LightweightQuota.VMName = req.VMName
			if _, err := service.UpsertLightweightVMQuota(req.Owner, *req.LightweightQuota); err != nil {
				return nil, fmt.Errorf("同步轻量云配额失败: %w", err)
			}
		}
		if err := service.EnsureLightweightVMNetwork(req.Owner, req.VMName); err != nil {
			return nil, fmt.Errorf("绑定轻量云 VPC 失败: %w", err)
		}
	} else {
		switchID := req.TargetSwitchID
		securityGroupID := req.TargetSecurityGroupID
		if switchID == 0 && req.Owner != "admin" {
			sw, group, err := ensureMigrationDefaultVPC(req.Owner)
			if err != nil {
				return nil, fmt.Errorf("准备目标默认 VPC 失败: %w", err)
			}
			switchID = sw.ID
			securityGroupID = group.ID
		}
		if switchID > 0 {
			if err := service.BindVMToVPCAsAdmin(req.VMName, switchID, securityGroupID); err != nil {
				return nil, fmt.Errorf("绑定目标交换机失败: %w", err)
			}
		}
	}
	if req.Credential != nil {
		if err := service.SaveVMCredential(req.VMName, req.Credential.Username, req.Credential.Password, "migration", "migration", false); err != nil {
			return nil, fmt.Errorf("同步 VM 凭据失败: %w", err)
		}
	}
	if err := service.RefreshVMCacheByName(req.VMName); err != nil {
		return nil, fmt.Errorf("同步目标虚拟机缓存失败: %w", err)
	}
	result := &MigrationAdoptResult{VMName: req.VMName, Owner: req.Owner, CreatedUser: createdUser}
	for _, rule := range req.PortForwards {
		applied := rule
		hostPort := strings.TrimSpace(rule.TargetHostPort)
		if hostPort == "" {
			hostPort = strings.TrimSpace(rule.SourceHostPort)
		}
		if hostPort == "" {
			port, err := netpkg.AutoAllocatePort()
			if err != nil {
				result.Warnings = append(result.Warnings, "自动分配端口失败: "+err.Error())
				continue
			}
			hostPort = strconv.Itoa(port)
			applied.AutoAllocated = true
		} else if err := netpkg.CheckRequestedPortForwardHostPortAvailable(hostPort, rule.Protocol, nil); err != nil {
			port, allocErr := netpkg.AutoAllocatePort()
			if allocErr != nil {
				result.Warnings = append(result.Warnings, "端口 "+hostPort+" 被占用且自动分配失败: "+allocErr.Error())
				continue
			}
			hostPort = strconv.Itoa(port)
			applied.AutoAllocated = true
		}
		vmIP := strings.TrimSpace(rule.DestIP)
		if resolved, err := netpkg.ResolvePortForwardTargetIP(req.VMName, ""); err == nil && strings.TrimSpace(resolved) != "" {
			vmIP = resolved
		}
		if vmIP == "" {
			result.Warnings = append(result.Warnings, "未能解析 "+req.VMName+" 的端口转发目标 IP，已跳过")
			continue
		}
		applied.TargetHostPort = hostPort
		if err := netpkg.AddPortForward(&netpkg.PortForwardAddParams{
			VMIP:           vmIP,
			HostPort:       hostPort,
			VMPort:         rule.VMPort,
			Protocol:       rule.Protocol,
			Comment:        req.VMName,
			CreatedBy:      "migration",
			CreatedByAdmin: true,
		}); err != nil {
			result.Warnings = append(result.Warnings, "端口转发 "+rule.SourceHostPort+" 同步失败: "+err.Error())
			continue
		}
		result.PortForwards = append(result.PortForwards, applied)
		_ = service.EnsureSecurityGroupAllowsPortForward(req.VMName, rule.Protocol, rule.VMPort)
	}
	return result, nil
}

func ensureMigrationTargetUser(req MigrationAdoptRequest) (bool, error) {
	if req.Owner == "admin" {
		return false, nil
	}
	var user model.User
	if err := model.DB.Where("username = ?", req.Owner).First(&user).Error; err == nil {
		updates := migrationUserUpdateMap(req)
		if service.IsLightweightCloudType(req.CloudType) {
			updates["cloud_type"] = service.CloudTypeLightweight
			updates["dedicated_vpc_switch_id"] = req.TargetSwitchID
		}
		if len(updates) > 0 {
			if err := model.DB.Model(&user).Updates(updates).Error; err != nil {
				return false, err
			}
		}
		return false, nil
	} else if err != gorm.ErrRecordNotFound {
		return false, err
	}
	_ = model.DB.Unscoped().Where("username = ? AND deleted_at IS NOT NULL", req.Owner).Delete(&model.User{}).Error
	systemPassword, err := generateMigrationPassword()
	if err != nil {
		return false, err
	}
	passwordHash := strings.TrimSpace(req.User.PasswordHash)
	if passwordHash == "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(systemPassword), bcrypt.DefaultCost)
		if err != nil {
			return false, err
		}
		passwordHash = string(hash)
	}
	user = model.User{
		Username:             req.Owner,
		PasswordHash:         passwordHash,
		Email:                strings.TrimSpace(req.User.Email),
		Role:                 "user",
		CloudType:            service.NormalizeCloudType(req.CloudType),
		DedicatedVPCSwitchID: req.TargetSwitchID,
		Status:               firstNonEmpty(req.User.Status, service.UserStatusActive),
		EmailVerifiedAt:      req.User.EmailVerifiedAt,
		LoginVerifiedUntil:   req.User.LoginVerifiedUntil,
		SecurityUpdatedAt:    req.User.SecurityUpdatedAt,
		MaxCPU:               req.User.MaxCPU,
		MaxMemory:            req.User.MaxMemory,
		MaxDisk:              req.User.MaxDisk,
		MaxVM:                req.User.MaxVM,
		MaxStorage:           req.User.MaxStorage,
		MaxRuntimeHours:      req.User.MaxRuntimeHours,
		EnablePortForward:    req.User.EnablePortForward,
		MaxPortForwards:      req.User.MaxPortForwards,
		MaxSnapshots:         req.User.MaxSnapshots,
		MaxBandwidthUp:       req.User.MaxBandwidthUp,
		MaxBandwidthDown:     req.User.MaxBandwidthDown,
		MaxTrafficDown:       req.User.MaxTrafficDown,
		MaxTrafficUp:         req.User.MaxTrafficUp,
		MaxPublicIPs:         req.User.MaxPublicIPs,
	}
	if err := model.DB.Create(&user).Error; err != nil {
		var existing model.User
		if fetchErr := model.DB.Where("username = ?", req.Owner).First(&existing).Error; fetchErr == nil {
			return false, nil
		}
		return false, err
	}
	if err := service.ProvisionSystemUserResources(&user, systemPassword); err != nil {
		return true, err
	}
	if service.IsLightweightCloudType(user.CloudType) {
		if req.TargetSwitchID == 0 {
			sw, err := service.EnsureDefaultVPCSwitch(user.Username)
			if err != nil {
				return true, err
			}
			if sw != nil {
				user.DedicatedVPCSwitchID = sw.ID
				if err := model.DB.Model(&user).Update("dedicated_vpc_switch_id", sw.ID).Error; err != nil {
					return true, err
				}
			}
		}
	} else {
		if _, _, err := ensureMigrationDefaultVPC(user.Username); err != nil {
			return true, err
		}
	}
	return true, nil
}

func ensureMigrationDefaultVPC(username string) (*model.VPCSwitch, *model.VPCSecurityGroup, error) {
	group, err := service.EnsureDefaultSecurityGroup(username)
	if err != nil {
		return nil, nil, err
	}
	sw, err := service.EnsureDefaultVPCSwitch(username)
	if err != nil {
		return nil, nil, err
	}
	if sw == nil {
		return nil, nil, fmt.Errorf("无法为用户 %s 准备默认 VPC", username)
	}
	return sw, group, nil
}

func migrationUserUpdateMap(req MigrationAdoptRequest) map[string]interface{} {
	updates := map[string]interface{}{}
	if passwordHash := strings.TrimSpace(req.User.PasswordHash); passwordHash != "" {
		updates["password_hash"] = passwordHash
	}
	if email := strings.TrimSpace(req.User.Email); email != "" {
		updates["email"] = email
	}
	if status := strings.TrimSpace(req.User.Status); status != "" {
		updates["status"] = status
	}
	if req.User.EmailVerifiedAt != nil {
		updates["email_verified_at"] = req.User.EmailVerifiedAt
	}
	if req.User.LoginVerifiedUntil != nil {
		updates["login_verified_until"] = req.User.LoginVerifiedUntil
	}
	if req.User.SecurityUpdatedAt != nil {
		updates["security_updated_at"] = req.User.SecurityUpdatedAt
	}
	return updates
}

func loadUserSnapshot(username string) MigrationUserSnapshot {
	snap := MigrationUserSnapshot{Username: username, CloudType: service.CloudTypeElastic, Status: service.UserStatusActive, EnablePortForward: true, MaxPortForwards: 10, MaxSnapshots: 5}
	var user model.User
	if model.DB.Where("username = ?", username).First(&user).Error != nil {
		return snap
	}
	snap.PasswordHash = user.PasswordHash
	snap.Email = user.Email
	snap.CloudType = service.NormalizeCloudType(user.CloudType)
	snap.Status = user.Status
	snap.EmailVerifiedAt = user.EmailVerifiedAt
	snap.LoginVerifiedUntil = user.LoginVerifiedUntil
	snap.SecurityUpdatedAt = user.SecurityUpdatedAt
	snap.MaxCPU = user.MaxCPU
	snap.MaxMemory = user.MaxMemory
	snap.MaxDisk = user.MaxDisk
	snap.MaxVM = user.MaxVM
	snap.MaxStorage = user.MaxStorage
	snap.MaxRuntimeHours = user.MaxRuntimeHours
	snap.EnablePortForward = user.EnablePortForward
	snap.MaxPortForwards = user.MaxPortForwards
	snap.MaxSnapshots = user.MaxSnapshots
	snap.MaxBandwidthUp = user.MaxBandwidthUp
	snap.MaxBandwidthDown = user.MaxBandwidthDown
	snap.MaxTrafficDown = user.MaxTrafficDown
	snap.MaxTrafficUp = user.MaxTrafficUp
	snap.MaxPublicIPs = user.MaxPublicIPs
	snap.DedicatedVPCSwitchID = user.DedicatedVPCSwitchID
	return snap
}

func generateMigrationPassword() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}