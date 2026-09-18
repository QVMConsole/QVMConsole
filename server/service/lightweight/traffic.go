package lightweight

import (
	"fmt"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/model"
	bw "kvm_console/service/bandwidth"
	"kvm_console/service/network"
	"kvm_console/service/snapshot"
)

func aggregateLightweightVMMonthlyTrafficRaw(vmName string) (downBytes, upBytes int64) {
	vmName = strings.TrimSpace(vmName)
	if vmName == "" || model.DB == nil {
		return 0, 0
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)
	var records []model.VmStatsRecord
	model.DB.Where("vm_name = ? AND recorded_at >= ? AND recorded_at < ?", vmName, monthStart, monthEnd).
		Order("recorded_at ASC").
		Find(&records)
	for i := 1; i < len(records); i++ {
		if delta := records[i].NetRxBytes - records[i-1].NetRxBytes; delta > 0 {
			downBytes += delta
		}
		if delta := records[i].NetTxBytes - records[i-1].NetTxBytes; delta > 0 {
			upBytes += delta
		}
	}
	return downBytes, upBytes
}

func AggregateLightweightVMMonthlyTraffic(vmName string) (downBytes, upBytes int64) {
	rawDown, rawUp := aggregateLightweightVMMonthlyTrafficRaw(vmName)
	var record model.LightweightVMTrafficMonthly
	if err := model.DB.Where("vm_name = ? AND month = ?", strings.TrimSpace(vmName), HookCurrentTrafficMonth()).First(&record).Error; err != nil {
		return rawDown, rawUp
	}
	return HookClampTrafficBytes(rawDown - record.OffsetDown), HookClampTrafficBytes(rawUp - record.OffsetUp)
}

func getOrCreateLightweightVMTrafficMonthly(quota model.LightweightVMQuota, month string) model.LightweightVMTrafficMonthly {
	var record model.LightweightVMTrafficMonthly
	if err := model.DB.Where("vm_name = ? AND month = ?", quota.VMName, month).First(&record).Error; err == nil {
		return record
	}
	return model.LightweightVMTrafficMonthly{
		VMName:   quota.VMName,
		Username: quota.Username,
		Month:    month,
	}
}

func saveLightweightVMTrafficMonthly(record model.LightweightVMTrafficMonthly) error {
	if record.ID == 0 {
		return model.DB.Create(&record).Error
	}
	return model.DB.Save(&record).Error
}

func IsLightweightVMTrafficLimited(vmName string) (downLimited, upLimited bool) {
	if model.DB == nil || strings.TrimSpace(vmName) == "" {
		return false, false
	}
	var record model.LightweightVMTrafficMonthly
	if err := model.DB.Where("vm_name = ? AND month = ?", strings.TrimSpace(vmName), HookCurrentTrafficMonth()).First(&record).Error; err != nil {
		return false, false
	}
	return record.IsLimitedDown, record.IsLimitedUp
}

func ApplyLightweightVMBandwidth(vmName string) error {
	var quota model.LightweightVMQuota
	if err := model.DB.Where("vm_name = ?", strings.TrimSpace(vmName)).First(&quota).Error; err != nil {
		return nil
	}
	downMbps := quota.BandwidthDownMbps
	upMbps := quota.BandwidthUpMbps
	downLimited, upLimited := IsLightweightVMTrafficLimited(quota.VMName)
	penaltyMbps := lightweightTrafficPenaltyMbps()
	if downLimited {
		downMbps = penaltyMbps
	}
	if upLimited {
		upMbps = penaltyMbps
	}
	if downMbps <= 0 && upMbps <= 0 {
		return HookClearVMBandwidth(quota.VMName)
	}
	downKB := bw.MbpsToKBps(downMbps)
	upKB := bw.MbpsToKBps(upMbps)
	return HookApplyVMNICBandwidth(quota.VMName, downKB, downKB, downKB*30, upKB, upKB, upKB*30)
}

func CheckAndApplyLightweightVMTrafficLimit(quota model.LightweightVMQuota) {
	if quota.VMName == "" || model.DB == nil {
		return
	}
	rawDown, rawUp := aggregateLightweightVMMonthlyTrafficRaw(quota.VMName)
	record := getOrCreateLightweightVMTrafficMonthly(quota, HookCurrentTrafficMonth())
	effectiveDown := HookClampTrafficBytes(rawDown - record.OffsetDown)
	effectiveUp := HookClampTrafficBytes(rawUp - record.OffsetUp)
	record.Username = quota.Username
	record.TrafficDown = effectiveDown
	record.TrafficUp = effectiveUp

	downLimited := quota.TrafficDownGB > 0 && float64(effectiveDown) >= HookTrafficQuotaBytes(quota.TrafficDownGB)
	upLimited := quota.TrafficUpGB > 0 && float64(effectiveUp) >= HookTrafficQuotaBytes(quota.TrafficUpGB)
	changed := record.IsLimitedDown != downLimited || record.IsLimitedUp != upLimited
	record.IsLimitedDown = downLimited
	record.IsLimitedUp = upLimited
	if err := saveLightweightVMTrafficMonthly(record); err != nil {
		logger.App.Warn("保存 VM 月流量失败", "component", "轻量云流量配额", "vm", quota.VMName, "error", err)
		return
	}
	if changed {
		if err := ApplyLightweightVMBandwidth(quota.VMName); err != nil {
			logger.App.Warn("应用 VM 限速状态失败", "component", "轻量云流量配额", "vm", quota.VMName, "error", err)
		}
	}
	if (downLimited || upLimited) && changed {
		logger.App.Warn("VM 本月流量超限，已按超限方向强制限速",
			"component", "轻量云流量配额",
			"vm", quota.VMName,
			"penalty_mbps", lightweightTrafficPenaltyMbps(),
			"down", HookFormatTrafficBytes(effectiveDown),
			"down_quota_gb", quota.TrafficDownGB,
			"up", HookFormatTrafficBytes(effectiveUp),
			"up_quota_gb", quota.TrafficUpGB)
	}
}

func CheckAllLightweightVMTrafficQuota() {
	if model.DB == nil {
		return
	}
	var quotas []model.LightweightVMQuota
	model.DB.Find(&quotas)
	for _, quota := range quotas {
		CheckAndApplyLightweightVMTrafficLimit(quota)
	}
}

func CheckLightweightVMTrafficAfterQuotaUpdate(vmName string) {
	var quota model.LightweightVMQuota
	if err := model.DB.Where("vm_name = ?", strings.TrimSpace(vmName)).First(&quota).Error; err != nil {
		return
	}
	rawDown, rawUp := aggregateLightweightVMMonthlyTrafficRaw(quota.VMName)
	record := getOrCreateLightweightVMTrafficMonthly(quota, HookCurrentTrafficMonth())
	effectiveDown := HookClampTrafficBytes(rawDown - record.OffsetDown)
	effectiveUp := HookClampTrafficBytes(rawUp - record.OffsetUp)
	record.Username = quota.Username
	record.TrafficDown = effectiveDown
	record.TrafficUp = effectiveUp
	if !record.IsLimitedDown && !record.IsLimitedUp {
		if err := saveLightweightVMTrafficMonthly(record); err != nil {
			logger.App.Warn("轻量VM流量月度记录失败", "vm", quota.VMName, "error", err)
		}
		if err := ApplyLightweightVMBandwidth(quota.VMName); err != nil {
			logger.App.Warn("轻量VM带宽应用失败", "vm", quota.VMName, "error", err)
		}
		return
	}
	downLimited := quota.TrafficDownGB > 0 && float64(effectiveDown) >= HookTrafficQuotaBytes(quota.TrafficDownGB)
	upLimited := quota.TrafficUpGB > 0 && float64(effectiveUp) >= HookTrafficQuotaBytes(quota.TrafficUpGB)
	record.IsLimitedDown = downLimited
	record.IsLimitedUp = upLimited
	if err := saveLightweightVMTrafficMonthly(record); err != nil {
		logger.App.Warn("保存 VM 配额调整状态失败", "component", "轻量云流量配额", "vm", quota.VMName, "error", err)
		return
	}
	if err := ApplyLightweightVMBandwidth(quota.VMName); err != nil {
		logger.App.Warn("配额调整后应用 VM 带宽失败", "component", "轻量云流量配额", "vm", quota.VMName, "error", err)
	}
}

func ResetAllLightweightVMTraffic() {
	if model.DB == nil {
		return
	}
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")
	var records []model.LightweightVMTrafficMonthly
	model.DB.Where("month = ? AND (is_limited_down = ? OR is_limited_up = ?)", lastMonth, true, true).Find(&records)
	for _, record := range records {
		if err := ApplyLightweightVMBandwidth(record.VMName); err != nil {
			logger.App.Warn("月重置后恢复 VM 带宽失败", "component", "轻量云流量配额", "vm", record.VMName, "error", err)
		}
	}
	cleanupMonth := time.Now().AddDate(0, -12, 0).Format("2006-01")
	model.DB.Where("month < ?", cleanupMonth).Delete(&model.LightweightVMTrafficMonthly{})
}

func GetLightweightVMPortForwardUsage(vmName string) int {
	rules, err := network.ListLivePortForwardsFromIPTables()
	if err != nil {
		return 0
	}
	count := 0
	for _, rule := range rules {
		if strings.TrimSpace(rule.VMName) == strings.TrimSpace(vmName) {
			count++
		}
	}
	return count
}

func CheckLightweightVMPortForwardQuota(username, vmName string, delta int) error {
	if delta <= 0 {
		return nil
	}
	var quota model.LightweightVMQuota
	if err := model.DB.Where("username = ? AND vm_name = ?", strings.TrimSpace(username), strings.TrimSpace(vmName)).First(&quota).Error; err != nil {
		return fmt.Errorf("当前轻量云服务器未配置端口转发配额")
	}
	if quota.MaxPortForwards <= 0 {
		return nil
	}
	used := GetLightweightVMPortForwardUsage(vmName)
	if used+delta > quota.MaxPortForwards {
		return fmt.Errorf("当前服务器端口转发数量超出配额限制（已用 %d / 上限 %d）", used, quota.MaxPortForwards)
	}
	return nil
}

func CheckLightweightVMSnapshotQuota(username, vmName string, delta int) error {
	if delta <= 0 {
		return nil
	}
	var quota model.LightweightVMQuota
	if err := model.DB.Where("username = ? AND vm_name = ?", strings.TrimSpace(username), strings.TrimSpace(vmName)).First(&quota).Error; err != nil {
		return fmt.Errorf("当前服务器未配置快照配额")
	}
	if quota.MaxSnapshots <= 0 {
		return nil
	}
	used := snapshot.CountVMSnapshots(vmName)
	if used+delta > quota.MaxSnapshots {
		return fmt.Errorf("当前服务器快照数量超出配额限制（已用 %d / 上限 %d）", used, quota.MaxSnapshots)
	}
	return nil
}
