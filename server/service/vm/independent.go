package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/utils"
)

// MakeVMIndependentParams 转为独立虚拟机参数
type MakeVMIndependentParams struct {
	VMName string `json:"vm_name"`
}

// ParseMakeVMIndependentParams 从 JSON 解析参数
func ParseMakeVMIndependentParams(jsonStr string) (*MakeVMIndependentParams, error) {
	var params MakeVMIndependentParams
	if err := json.Unmarshal([]byte(jsonStr), &params); err != nil {
		return nil, err
	}
	params.VMName = strings.TrimSpace(params.VMName)
	if params.VMName == "" {
		return nil, fmt.Errorf("参数解析失败: vm_name 为空")
	}
	return &params, nil
}

// MakeVMIndependent 将链式克隆虚拟机转为独立虚拟机（脱离 backing chain）
// 通过 qemu-img convert 将 backing chain 合并为独立镜像
func MakeVMIndependent(ctx context.Context, params *MakeVMIndependentParams, progressFn func(int, string)) error {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}

	vmName := strings.TrimSpace(params.VMName)
	if vmName == "" {
		return fmt.Errorf("虚拟机名称为空")
	}

	progressFn(5, "检查虚拟机状态...")

	// 1. 检查虚拟机必须为关机状态
	status, err := getVMStatusForIndependent(vmName)
	if err != nil {
		return fmt.Errorf("获取虚拟机状态失败: %w", err)
	}
	if status != "shut off" {
		return fmt.Errorf("虚拟机必须处于关机状态才能转为独立虚拟机，当前状态: %s", status)
	}

	progressFn(10, "获取虚拟机磁盘信息...")

	// 2. 获取系统磁盘信息
	diskInfo := GetVMDiskInfo(vmName)
	if diskInfo.Path == "" {
		return fmt.Errorf("未找到虚拟机系统磁盘")
	}
	if !diskInfo.HasBackingFile {
		return fmt.Errorf("虚拟机已经是独立虚拟机，无需转换")
	}

	diskPath := diskInfo.Path

	progressFn(15, "校验磁盘文件...")

	// 3. 检查磁盘文件存在
	if _, err := os.Stat(diskPath); os.IsNotExist(err) {
		return fmt.Errorf("磁盘文件不存在: %s", diskPath)
	}

	// 4. 创建临时文件路径
	dir := filepath.Dir(diskPath)
	baseName := filepath.Base(diskPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)
	tempPath := filepath.Join(dir, nameWithoutExt+"_independent"+ext)

	progressFn(20, "开始合并磁盘链（将 backing 数据合并到当前层）...")

	// 5. 使用 qemu-img convert 创建独立副本（会自动拉平 backing chain）
	// 大容量磁盘转换可能非常耗时，不设置自动超时，仅响应任务取消
	convertCmd := fmt.Sprintf("qemu-img convert -f qcow2 -O qcow2 %s %s",
		utils.ShellSingleQuote(diskPath), utils.ShellSingleQuote(tempPath))
	result := utils.ExecShellContext(ctx, convertCmd)
	if result.Error != nil {
		// 清理临时文件
		os.Remove(tempPath)
		errMsg := result.Stderr
		if ctx.Err() != nil {
			errMsg = "任务已取消"
		}
		return fmt.Errorf("磁盘转换失败: %s", errMsg)
	}

	progressFn(80, "替换原始磁盘文件...")

	// 6. 备份原始磁盘并替换
	backupPath := diskPath + ".linked_backup"
	if err := os.Rename(diskPath, backupPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("备份原始磁盘失败: %w", err)
	}

	if err := os.Rename(tempPath, diskPath); err != nil {
		// 恢复原始磁盘
		os.Rename(backupPath, diskPath)
		os.Remove(tempPath)
		return fmt.Errorf("替换磁盘文件失败: %w", err)
	}

	progressFn(85, "清理备份文件...")

	// 7. 删除备份
	if err := os.Remove(backupPath); err != nil {
		logger.App.Warn("删除备份磁盘文件失败", "path", backupPath, "error", err)
	}

	progressFn(90, "更新模板源元数据...")

	// 8. 更新模板源元数据（标记为全量克隆模式，脱离链式关系）
	if D.WriteVMTemplateSource != nil && diskInfo.Template != "" {
		if err := D.WriteVMTemplateSource(vmName, diskInfo.Template, "full"); err != nil {
			logger.App.Warn("更新模板源元数据失败", "vm", vmName, "error", err)
			// 不阻断流程
		}
	}

	progressFn(100, "转为独立虚拟机完成")

	logger.App.Info("虚拟机已转为独立虚拟机", "vm", vmName, "disk", diskPath)
	return nil
}

// getVMStatusForIndependent 获取虚拟机状态（内部函数）
func getVMStatusForIndependent(name string) (string, error) {
	// try libvirt RPC first
	status, err := libvirt_rpc.GetDomainStateRPC(name)
	if err == nil && status != "" {
		return status, nil
	}
	// fallback to virsh
	stateResult := utils.ExecCommand("virsh", "domstate", name)
	if stateResult.Error != nil {
		return "", stateResult.Error
	}
	return strings.TrimSpace(stateResult.Stdout), nil
}
