package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"kvm_console/config"
	"kvm_console/logger"
)

// gormAppLogger 将 GORM 日志写入 appWriter，不直接输出到 stdout
// 可被 KVM_LOG_CONSOLE_TYPES=app 控制是否显示在终端
type gormAppLogger struct {
	slowThreshold time.Duration
}

func (l *gormAppLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l // 始终 Warn 级别，忽略外部设置
}

func (l *gormAppLogger) Info(_ context.Context, msg string, args ...interface{}) {
	// Info 级别不输出（GORM info 太嘈杂）
}

func (l *gormAppLogger) Warn(_ context.Context, msg string, args ...interface{}) {
	logger.App.Warn(msg, "source", "gorm")
}

func (l *gormAppLogger) Error(_ context.Context, msg string, args ...interface{}) {
	logger.App.Error(msg, "source", "gorm")
}

func (l *gormAppLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	if err != nil && err.Error() != "record not found" {
		logger.App.Error("数据库查询错误", "elapsed", elapsed, "rows", rows, "sql", sql, "error", err)
		return
	}
	if l.slowThreshold > 0 && elapsed > l.slowThreshold {
		logger.App.Warn("慢查询", "elapsed", elapsed, "rows", rows, "sql", sql)
	}
}

// DB 全局数据库实例
var DB *gorm.DB

// InitDB 初始化数据库
func InitDB() {
	// 确保数据目录存在
	dbDir := filepath.Dir(config.GlobalConfig.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		logger.App.Error("创建数据库目录失败", "error", err)
		os.Exit(1)
	}

	var err error
	// SQLite 并发写配置：WAL 模式 + busy_timeout + 立即事务锁。
	// 默认配置下并发写会立即返回 SQLITE_BUSY；分片上传等并发写入必须等待重试，而非静默失败。
	dsn := config.GlobalConfig.DBPath + "?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate"
	DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: &gormAppLogger{},
	})
	if err != nil {
		logger.App.Error("连接数据库失败", "error", err)
		os.Exit(1)
	}

	hadMaxPortForwardsColumn := DB.Migrator().HasColumn(&User{}, "max_port_forwards")
	hadEnablePortForwardColumn := DB.Migrator().HasColumn(&User{}, "enable_port_forward")
	hadUserMaxSnapshotsColumn := DB.Migrator().HasColumn(&User{}, "max_snapshots")
	hadLightweightQuotaMaxSnapshotsColumn := DB.Migrator().HasColumn(&LightweightVMQuota{}, "max_snapshots")
	hadLightweightRegistrationMaxSnapshotsColumn := DB.Migrator().HasColumn(&LightweightVMRegistration{}, "max_snapshots")
	hadLightweightQuotaMaxRuntimeColumn := DB.Migrator().HasColumn(&LightweightVMQuota{}, "max_runtime_hours")
	hadLightweightRegistrationMaxRuntimeColumn := DB.Migrator().HasColumn(&LightweightVMRegistration{}, "max_runtime_hours")
	hadVPCBindingInterfaceOrderColumn := DB.Migrator().HasColumn(&VPCVMBinding{}, "interface_order")
	hadVPCSwitchCIDRColumn := DB.Migrator().HasColumn(&VPCSwitch{}, "cidr")

	// 预修复: 在 AutoMigrate 之前清理 vpc_switches.cidr 重复数据并删除旧唯一索引
	preFixVPCSwitchCIDRIndex()

	// 自动迁移表结构
	if err := DB.AutoMigrate(&User{}, &UserAPIKey{}, &VmStatsRecord{}, &PortForwardIP{}, &HostStatsRecord{}, &UserTrafficDaily{}, &SystemSetting{}, &VMCredential{}, &VMCache{}, &AuthActionToken{}, &SecurityChallenge{}, &SchedulerEvent{}, &VMSchedule{}, &NetworkBridge{}, &HostStoragePool{}, &HostNode{},
		&LightweightVMQuota{}, &LightweightVMTrafficMonthly{}, &LightweightVMRegistration{},
		&VPCSwitch{}, &VPCSecurityGroup{}, &VPCSecurityGroupRule{}, &VPCVMBinding{}, &VPCSwitchTrafficMonthly{}, &PublicIP{}, &PublicIPBinding{},
		&VMLock{}, &UploadSession{}); err != nil {
		logger.App.Error("数据库迁移失败", "error", err)
		os.Exit(1)
	}
	migrateUserCloudType()
	migratePublicIPCIDRColumn()
	migrateUserPortForwardFeature(hadEnablePortForwardColumn)
	migrateUserPortForwardQuota(hadMaxPortForwardsColumn)
	migrateUserSnapshotQuota(hadUserMaxSnapshotsColumn)
	migrateLightweightSnapshotQuota(hadLightweightQuotaMaxSnapshotsColumn, hadLightweightRegistrationMaxSnapshotsColumn)
	migrateLightweightRuntimeQuota(hadLightweightQuotaMaxRuntimeColumn, hadLightweightRegistrationMaxRuntimeColumn)
	migrateVPCBindingInterfaceOrder(hadVPCBindingInterfaceOrderColumn)
	migrateVPCBindingInterfaceOrderNormalize()
	migrateVPCSwitchCIDRColumn(hadVPCSwitchCIDRColumn)

	// 兼容旧用户：补齐默认状态，确保升级后能继续登录
	if err := DB.Model(&User{}).Where("status = '' OR status IS NULL").Updates(map[string]interface{}{
		"status": "active",
	}).Error; err != nil {
		logger.App.Warn("修复旧用户状态失败", "error", err)
	}

	// 初始化默认管理员
	initDefaultAdmin()
	logger.App.Info("数据库初始化完成")
}

func migrateUserCloudType() {
	if DB == nil {
		return
	}
	if err := DB.Model(&User{}).
		Where("cloud_type = '' OR cloud_type IS NULL").
		Update("cloud_type", "elastic").Error; err != nil {
		logger.App.Warn("初始化用户云类型失败", "error", err)
	}
}

func migrateUserPortForwardQuota(hadColumn bool) {
	if DB == nil || hadColumn {
		return
	}
	if err := DB.Model(&User{}).
		Where("role <> ? AND (max_port_forwards IS NULL OR max_port_forwards = 0)", "admin").
		Update("max_port_forwards", 10).Error; err != nil {
		logger.App.Warn("初始化用户端口转发配额失败", "error", err)
	}
}

func migrateUserPortForwardFeature(hadColumn bool) {
	if DB == nil || hadColumn {
		return
	}
	if err := DB.Model(&User{}).
		Where("role <> ?", "admin").
		Update("enable_port_forward", true).Error; err != nil {
		logger.App.Warn("初始化用户端口转发开关失败", "error", err)
	}
}

func migrateUserSnapshotQuota(hadColumn bool) {
	if DB == nil || hadColumn {
		return
	}
	if err := DB.Model(&User{}).
		Where("role <> ? AND (max_snapshots IS NULL OR max_snapshots = 0)", "admin").
		Update("max_snapshots", 5).Error; err != nil {
		logger.App.Warn("初始化用户快照配额失败", "error", err)
	}
}

func migrateLightweightSnapshotQuota(hadQuotaColumn, hadRegistrationColumn bool) {
	if DB == nil {
		return
	}
	if !hadQuotaColumn {
		if err := DB.Model(&LightweightVMQuota{}).
			Where("max_snapshots IS NULL OR max_snapshots = 0").
			Update("max_snapshots", 2).Error; err != nil {
			logger.App.Warn("初始化轻量云VM快照配额失败", "error", err)
		}
	}
	if !hadRegistrationColumn {
		if err := DB.Model(&LightweightVMRegistration{}).
			Where("max_snapshots IS NULL OR max_snapshots = 0").
			Update("max_snapshots", 2).Error; err != nil {
			logger.App.Warn("初始化轻量云VM注册快照配额失败", "error", err)
		}
	}
}

func migrateLightweightRuntimeQuota(hadQuotaColumn, hadRegistrationColumn bool) {
	if DB == nil {
		return
	}
	if !hadQuotaColumn {
		if err := DB.Model(&LightweightVMQuota{}).
			Where("max_runtime_hours IS NULL").
			Update("max_runtime_hours", 0).Error; err != nil {
			logger.App.Warn("初始化轻量云VM运行时长配额失败", "error", err)
		}
	}
	if !hadRegistrationColumn {
		if err := DB.Model(&LightweightVMRegistration{}).
			Where("max_runtime_hours IS NULL").
			Update("max_runtime_hours", 0).Error; err != nil {
			logger.App.Warn("初始化轻量云VM注册运行时长配额失败", "error", err)
		}
	}
}

func migratePublicIPCIDRColumn() {
	if DB == nil || !DB.Migrator().HasTable(&PublicIP{}) {
		return
	}
	if !DB.Migrator().HasColumn(&PublicIP{}, "c_id_r") || !DB.Migrator().HasColumn(&PublicIP{}, "cidr") {
		return
	}
	if err := DB.Exec("UPDATE public_ips SET cidr = c_id_r WHERE (cidr IS NULL OR cidr = '') AND c_id_r IS NOT NULL AND c_id_r <> ''").Error; err != nil {
		logger.App.Warn("迁移公网IP CIDR字段失败", "error", err)
	}
	// 删除遗留的 c_id_r 列
	if err := DB.Exec("ALTER TABLE public_ips DROP COLUMN c_id_r").Error; err != nil {
		logger.App.Warn("删除公网IP遗留列 c_id_r 失败", "error", err)
	} else {
		logger.App.Info("已删除公网IP遗留列 c_id_r")
	}
}

func migrateVPCBindingInterfaceOrder(hadColumn bool) {
	if DB == nil {
		return
	}
	// 修复联合唯一索引：从 vm_name 单列索引迁移到 (vm_name, interface_order) 联合索引
	// GORM AutoMigrate 可能无法正确重建索引，需要手动处理
	if !hadColumn {
		// 首次迁移：填充默认值
		if err := DB.Model(&VPCVMBinding{}).
			Where("interface_order IS NULL OR interface_order = 0").
			Update("interface_order", 0).Error; err != nil {
			logger.App.Warn("初始化VPC绑定interface_order失败", "error", err)
		}
		if err := DB.Model(&VPCVMBinding{}).
			Where("nic_model IS NULL OR nic_model = ''").
			Update("nic_model", "virtio").Error; err != nil {
			logger.App.Warn("初始化VPC绑定nic_model失败", "error", err)
		}
	}

	// 始终确保索引正确：删除可能的旧单列唯一索引，创建新联合唯一索引
	migrateVPCBindingUniqueIndex()
}

// allowedIndexNames 已知安全的索引名称白名单，用于 DROP INDEX 等 DDL 操作。
// SQLite 不支持参数化 DDL，为防范 SQL 注入，所有索引名称必须在此白名单中。
var allowedIndexNames = map[string]bool{
	"uni_vpc_vm_bindings_vm_name": true,
	"idx_vpc_vm_bindings_vm_name": true,
	"uq_vpc_vm_bindings_vm_name":  true,
	"idx_vpc_switches_cidr":       true,
	"idx_vpc_switches_c_id_r":     true,
	"idx_vm_interface":            true,
}

// isAllowedIndexName 校验索引名称是否在允许的白名单中。
func isAllowedIndexName(name string) bool {
	return allowedIndexNames[name]
}

func migrateVPCBindingUniqueIndex() {
	if DB == nil {
		return
	}
	// GORM 可能生成多种索引名称，逐一尝试删除旧索引
	oldIndexNames := []string{
		"uni_vpc_vm_bindings_vm_name",
		"idx_vpc_vm_bindings_vm_name",
		"uq_vpc_vm_bindings_vm_name",
	}
	for _, name := range oldIndexNames {
		if !isAllowedIndexName(name) {
			logger.App.Warn("跳过非白名单索引删除", "index", name)
			continue
		}
		DB.Exec("DROP INDEX IF EXISTS " + name)
	}
	// 创建新的联合唯一索引
	if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vm_interface ON vpc_vm_bindings(vm_name, interface_order)").Error; err != nil {
		logger.App.Warn("创建VPC绑定联合唯一索引失败", "error", err)
	}
}

// migrateVPCBindingInterfaceOrderNormalize 修复非连续的 interface_order，
// 确保每个 VM 的绑定从 0 开始连续编号。
// 解决因删除旧网口、重新添加导致的 interface_order 间隙（如 2, 3 而非 0, 1）。
func migrateVPCBindingInterfaceOrderNormalize() {
	if DB == nil {
		return
	}
	var vmNames []string
	if err := DB.Model(&VPCVMBinding{}).Distinct("vm_name").Pluck("vm_name", &vmNames).Error; err != nil {
		logger.App.Warn("查询VM绑定列表失败", "error", err)
		return
	}
	totalFixed := 0
	for _, vmName := range vmNames {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			continue
		}
		var bindings []VPCVMBinding
		if err := DB.Where("vm_name = ?", vmName).Order("interface_order ASC, id ASC").Find(&bindings).Error; err != nil || len(bindings) == 0 {
			continue
		}
		needsFix := false
		for i, b := range bindings {
			if b.InterfaceOrder != i {
				needsFix = true
				break
			}
		}
		if !needsFix {
			continue
		}
		const tempBase = -10000
		for i := range bindings {
			DB.Model(&bindings[i]).Update("interface_order", tempBase-i)
		}
		for i := range bindings {
			if err := DB.Model(&bindings[i]).Update("interface_order", i).Error; err != nil {
				logger.App.Warn("修复interface_order失败", "vm", vmName, "id", bindings[i].ID, "target", i, "error", err)
			} else {
				totalFixed++
			}
		}
	}
	if totalFixed > 0 {
		logger.App.Info("已修复非连续 interface_order", "count", totalFixed)
	}
}

// preFixVPCSwitchCIDRIndex 在 AutoMigrate 之前修复 vpc_switches.cidr 索引问题。
// 当数据库中存在多个空字符串 CIDR（桥接/直通模式交换机）时，唯一索引创建会失败。
// 此函数需在 AutoMigrate 之前调用。
func preFixVPCSwitchCIDRIndex() {
	if DB == nil {
		return
	}
	// 检查表是否存在
	if !DB.Migrator().HasTable(&VPCSwitch{}) {
		return
	}
	// 删除可能存在的旧唯一索引（避免 AutoMigrate 与手动索引冲突）
	DB.Exec("DROP INDEX IF EXISTS idx_vpc_switches_cidr")
	// 将空字符串 CIDR 更新为 NULL（SQLite 允许多个 NULL 在 UNIQUE 索引中共存）
	DB.Exec("UPDATE vpc_switches SET cidr = NULL WHERE cidr = ''")
}

// migrateVPCSwitchCIDRColumn 为旧版 vpc_switches 表补齐 cidr 列
// GORM 默认将 CIDR 映射为 c_id_r（连续大写字母被拆分为独立单词），
// 旧版数据库中存在 c_id_r 列存储实际 CIDR 值，需迁移至显式指定的 cidr 列。
func migrateVPCSwitchCIDRColumn(hadColumn bool) {
	if DB == nil {
		return
	}

	// Stage 0: 检查并修复 GORM 错误命名的 c_id_r 列 → cidr 列
	hasOldColumn := DB.Migrator().HasColumn(&VPCSwitch{}, "c_id_r")
	hasNewColumn := DB.Migrator().HasColumn(&VPCSwitch{}, "cidr")

	if hasOldColumn && hasNewColumn {
		// 将 c_id_r 中的数据迁移到 cidr（仅更新 cidr 为空的记录）
		if err := DB.Exec("UPDATE vpc_switches SET cidr = c_id_r WHERE (cidr IS NULL OR cidr = '') AND c_id_r IS NOT NULL AND c_id_r <> ''").Error; err != nil {
			logger.App.Warn("迁移 c_id_r → cidr 数据失败", "error", err)
		} else {
			logger.App.Info("已从 c_id_r 迁移数据到 cidr 列")
		}
		// 删除遗留的 c_id_r 列（SQLite 3.35+ 支持 DROP COLUMN）
		if err := DB.Exec("ALTER TABLE vpc_switches DROP COLUMN c_id_r").Error; err != nil {
			logger.App.Warn("删除遗留列 c_id_r 失败", "error", err)
		} else {
			logger.App.Info("已删除遗留列 c_id_r")
		}
		// 创建唯一索引（可能因之前迁移失败而缺失）——使用部分索引排除空值
		if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vpc_switches_cidr ON vpc_switches(cidr) WHERE cidr IS NOT NULL AND cidr != ''").Error; err != nil {
			logger.App.Warn("创建 vpc_switches.cidr 唯一索引失败", "error", err)
		}
		// 删除旧的无效索引（c_id_r 列上的索引，如果有的话）
		DB.Exec("DROP INDEX IF EXISTS idx_vpc_switches_c_id_r")
		return
	}

	if hadColumn {
		return
	}

	logger.App.Info("开始迁移 vpc_switches.cidr 列")

	// 1. 添加 cidr 列（暂不设 NOT NULL，避免与已有数据冲突）
	if !hasNewColumn {
		if err := DB.Exec("ALTER TABLE vpc_switches ADD COLUMN cidr TEXT DEFAULT ''").Error; err != nil {
			logger.App.Warn("添加 vpc_switches.cidr 列失败", "error", err)
			return
		}
	}

	// 2. 收集已占用的 CIDR，避免冲突
	prefix := strings.Trim(config.GlobalConfig.VPCSubnetPrefix, ". ")
	if prefix == "" {
		prefix = "10.200"
	}

	var switches []VPCSwitch
	if err := DB.Order("id ASC").Find(&switches).Error; err != nil {
		logger.App.Warn("迁移时查询交换机列表失败", "error", err)
		return
	}

	used := map[string]bool{}
	for _, sw := range switches {
		if sw.CIDR != "" {
			used[sw.CIDR] = true
		}
	}

	// 3. 为每个未设置 CIDR 的交换机分配一个唯一 CIDR
	idx := 1
	for _, sw := range switches {
		if sw.CIDR != "" {
			continue
		}
		var cidr string
		for {
			cidr = fmt.Sprintf("%s.%d.0/24", prefix, idx)
			idx++
			if !used[cidr] {
				break
			}
		}
		used[cidr] = true
		if err := DB.Model(&VPCSwitch{}).Where("id = ?", sw.ID).Update("cidr", cidr).Error; err != nil {
			logger.App.Warn("更新交换机 cidr 失败", "sw_id", sw.ID, "error", err)
		}
	}

	// 4. 创建部分唯一索引（排除空值，桥接/直通模式交换机无CIDR）
	if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vpc_switches_cidr ON vpc_switches(cidr) WHERE cidr IS NOT NULL AND cidr != ''").Error; err != nil {
		logger.App.Warn("创建 vpc_switches.cidr 唯一索引失败", "error", err)
	}

	logger.App.Info("vpc_switches.cidr 列迁移完成")
}

// initDefaultAdmin 创建默认管理员账号
func initDefaultAdmin() {
	var count int64
	DB.Model(&User{}).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(config.GlobalConfig.DefaultAdminPass), bcrypt.DefaultCost,
	)
	if err != nil {
		logger.App.Error("生成密码哈希失败", "error", err)
		os.Exit(1)
	}

	admin := User{
		Username:            config.GlobalConfig.DefaultAdminUser,
		PasswordHash:        string(hashedPassword),
		Role:                "admin",
		Status:              "active",
		ForcePasswordChange: true,
	}

	if err := DB.Create(&admin).Error; err != nil {
		logger.App.Warn("创建默认管理员失败", "error", err)
	} else {
		logger.App.Info("默认管理员账号已创建", "username", admin.Username)
	}
}
