package model

import (
	"time"
)

// TaskStatus 任务状态常量
const (
	TaskStatusPending  = "pending"  // 等待中
	TaskStatusRunning  = "running"  // 执行中
	TaskStatusSuccess  = "success"  // 成功
	TaskStatusFailed   = "failed"   // 失败
	TaskStatusCanceled = "canceled" // 已取消
)

// TaskType 任务类型常量
const (
	TaskTypeClone                           = "clone"                              // 链式克隆
	TaskTypeLinkedClone                     = "linked_clone"                       // 原生链式克隆
	TaskTypeBatch                           = "batch"                              // 批量克隆
	TaskTypeReinstall                       = "reinstall"                          // 重装系统
	TaskTypePrepare                         = "prepare"                            // 制作模板
	TaskTypeTemplateExport                  = "template_export"                    // 导出模板
	TaskTypeTemplateImport                  = "template_import"                    // 导入模板
	TaskTypeTemplateLinuxPrepare            = "template_linux_prepare"             // 预处理已导入 Linux 模板
	TaskTypeDeleteTemplate                  = "delete_template"                    // 删除模板
	TaskTypeCreate                          = "create"                             // 普通创建虚拟机
	TaskTypeDelete                          = "delete"                             // 删除虚拟机
	TaskTypeSnapshot                        = "snapshot"                           // 快照操作
	TaskTypeDeleteUser                      = "deleteuser"                         // 删除用户（含资产清理）
	TaskTypeDisableUser                     = "disable_user"                       // 封禁用户并关闭其资源
	TaskTypeRuntimeQuotaShutdown            = "runtime_quota_shutdown"             // 运行时长配额耗尽后关闭用户全部虚拟机
	TaskTypeLightweightRuntimeQuotaShutdown = "lightweight_runtime_quota_shutdown" // 轻量云单 VM 运行时长配额耗尽后自动关机
	TaskTypeExport                          = "export"                             // 导出虚拟机
	TaskTypeImport                          = "import"                             // 导入虚拟机
	TaskTypeImportAppliance                 = "import_appliance"                   // 导入 OVF/OVA 虚拟机包
	TaskTypeDiskTransfer                    = "disk_transfer"                      // 磁盘转移到用户存储
	TaskTypeRescue                          = "rescue"                             // 救援系统
	TaskTypeResetVMPassword                 = "reset_vm_password"                  // 重置来宾虚拟机密码
	TaskTypeVMDiskResize                    = "vm_disk_resize"                     // 虚拟机磁盘扩容与来宾文件系统扩容
	TaskTypeVMDiskProvision                 = "vm_disk_provision"                  // 创建或关联磁盘并配置来宾挂载
	TaskTypeVMDiskGuestMount                = "vm_disk_guest_mount"                // 重试来宾磁盘挂载或扩容
	TaskTypeApplyFirewall                   = "apply_firewall"                     // 应用 KVM 网络防火墙
	TaskTypeDisableFirewall                 = "disable_firewall"                   // 禁用 KVM 网络防火墙
	TaskTypeRollbackFirewall                = "rollback_firewall"                  // 回滚 KVM 网络防火墙
	TaskTypeUpdateFirewallGeoIP             = "update_firewall_geoip"              // 更新防火墙 GeoIP 数据
	TaskTypeEnableHostFirewall              = "enable_host_firewall"               // 启用宿主机防火墙
	TaskTypeDisableHostFirewall             = "disable_host_firewall"              // 关闭宿主机防火墙
	TaskTypeOVSRepair                       = "ovs_repair"                         // 修复 OVS 网络基础配置
	TaskTypePortSecurity                    = "port_security"                      // OVS 端口安全启停、协调与隔离
	TaskTypePublicIPApply                   = "public_ip_apply"                    // 应用公网 IP 绑定/解绑/迁移
	TaskTypeEnterMaintenanceMode            = "enter_maintenance_mode"             // 启用维护模式
	TaskTypeExitMaintenanceMode             = "exit_maintenance_mode"              // 关闭维护模式
	TaskTypeStorageFormat                   = "storage_format"                     // 格式化并挂载宿主机硬盘
	TaskTypeStorageCreatePartition          = "storage_create_partition"           // 在宿主机硬盘上创建分区
	TaskTypeStorageDeletePartitions         = "storage_delete_partitions"          // 删除宿主机硬盘上所有分区
	TaskTypeStorageCreateLVMVolume          = "storage_create_lvm_volume"          // 创建 LVM 存储卷
	TaskTypeStorageDeleteLVMVolume          = "storage_delete_lvm_volume"          // 删除 LVM 存储卷
	TaskTypeNetworkCapture                  = "network_capture"                    // VM 网络抓包诊断
	TaskTypeVMScheduleAction                = "vm_schedule_action"                 // 虚拟机定时任务动作执行
	TaskTypeLightweightVMProvision          = "lightweight_vm_provision"           // 轻量云注册 VM 开通
	TaskTypeVMMigrate                       = "vm_migrate"                         // 跨节点迁移虚拟机
	TaskTypeVMDiskMigrate                   = "vm_disk_migrate"                    // 本机迁移虚拟机硬盘
	TaskTypeImportDisk                      = "import_disk"                        // 管理员通过绝对路径导入磁盘创建虚拟机
	TaskTypeImportDiskAttach                = "import_disk_attach"                 // 管理员通过绝对路径导入磁盘挂载到已有虚拟机
	TaskTypeMakeVMIndependent               = "make_vm_independent"                // 链式克隆虚拟机转为独立虚拟机
	TaskTypePasswordBreachScan              = "password_breach_scan"               // 泄露密码扫描
	TaskTypePasswordBreachNotify            = "password_breach_notify"             // 泄露密码通知
)

// Task 异步任务模型（纯内存存储，不持久化）
type Task struct {
	ID        uint      `json:"id"`
	Type      string    `json:"type"`       // 任务类型
	Status    string    `json:"status"`     // 任务状态
	Params    string    `json:"params"`     // 任务参数（JSON）
	Result    string    `json:"result"`     // 执行结果（JSON）
	Progress  int       `json:"progress"`   // 进度（0-100）
	Message   string    `json:"message"`    // 状态消息
	CreatedBy string    `json:"created_by"` // 创建人
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
