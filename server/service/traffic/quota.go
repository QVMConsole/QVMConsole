package traffic

import (
	"fmt"
	"sync"
	"time"

	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/bandwidth"
	"kvm_console/service/lightweight"
	"kvm_console/service/network/vpc"
	"kvm_console/service/user"
	"kvm_console/utils"
)

// ==================== 用户网络流量月配额管理 ====================
// 统计用户非 VPC VM 的月流量总和，超限后对非 VPC VM 设置惩罚速率。
// VPC VM 的外网流量按交换机统计和限制，避免旧用户级惩罚覆盖交换机策略。
// 惩罚速率：上行 1Mbps，下行 10Mbps

const (
	// 超限后的惩罚速率（Mbps）
	trafficPenaltyDownMbps = 10 // 下行惩罚速率
	trafficPenaltyUpMbps   = 1  // 上行惩罚速率
)

// trafficCheckLock 防止并发检查冲突
var trafficCheckLock sync.Mutex

// AggregateUserDailyTraffic 聚合用户所有 VM 的本月流量增量。
// 保留函数名用于兼容旧调用，实际统计窗口已改为月。
// 计算方式：每个 VM 取本月 net_rx_bytes/net_tx_bytes 的相邻正增量总和
// net_rx_bytes = VM 接收 = VM 下行，net_tx_bytes = VM 发送 = VM 上行
func AggregateUserDailyTraffic(username string) (downBytes, upBytes int64) {
	vms := user.GetUserVMList(username)
	if len(vms) == 0 {
		return 0, 0
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)

	if len(vms) > 0 {
		for _, vmName := range vms {
			if bandwidth.IsVPCBoundVM(vmName) {
				continue
			}
			// 查询该 VM 本月所有记录（按时间升序），通过相邻记录差值累加计算流量增量
			// 这种方式能正确处理 VM 重启导致计数器归零的情况
			var records []model.VmStatsRecord
			err := model.DB.Where("vm_name = ? AND recorded_at >= ? AND recorded_at < ?", vmName, monthStart, monthEnd).
				Order("recorded_at ASC").
				Find(&records).Error
			if err != nil || len(records) < 2 {
				continue
			}

			for i := 1; i < len(records); i++ {
				deltaRx := records[i].NetRxBytes - records[i-1].NetRxBytes
				deltaTx := records[i].NetTxBytes - records[i-1].NetTxBytes
				// 仅累加正增量，负值说明 VM 重启了计数器归零，跳过该段
				if deltaRx > 0 {
					downBytes += deltaRx
				}
				if deltaTx > 0 {
					upBytes += deltaTx
				}
			}
		}
	}

	return downBytes, upBytes
}

// GetUserTrafficUsage 获取用户本月流量配额使用情况
func GetUserTrafficUsage(username string) *TrafficUsageInfo {
	var u model.User
	if err := model.DB.Where("username = ?", username).First(&u).Error; err != nil {
		return nil
	}

	downBytes, upBytes := AggregateUserDailyTraffic(username)

	// 减去偏移量（管理员重置时设置的基线）
	month := vpc.CurrentTrafficMonth()
	var daily model.UserTrafficDaily
	hasDaily := model.DB.Where("username = ? AND date = ?", username, month).First(&daily).Error == nil

	effectiveDown := downBytes
	effectiveUp := upBytes
	if hasDaily {
		effectiveDown = downBytes - daily.OffsetDown
		effectiveUp = upBytes - daily.OffsetUp
		if effectiveDown < 0 {
			effectiveDown = 0
		}
		if effectiveUp < 0 {
			effectiveUp = 0
		}
	}

	info := &TrafficUsageInfo{
		MaxTrafficDown:    u.MaxTrafficDown,
		MaxTrafficUp:      u.MaxTrafficUp,
		UsedTrafficDown:   effectiveDown,
		UsedTrafficUp:     effectiveUp,
		UsedTrafficDownGB: FormatTrafficBytes(effectiveDown),
		UsedTrafficUpGB:   FormatTrafficBytes(effectiveUp),
	}

	// 查询本月限速状态
	if hasDaily {
		info.IsLimitedDown = daily.IsLimitedDown
		info.IsLimitedUp = daily.IsLimitedUp
	}

	return info
}

// CheckAndApplyTrafficLimit 检查用户月流量是否超限，超限则对所有 VM 设置惩罚速率
func CheckAndApplyTrafficLimit(username string) {
	var u model.User
	if err := model.DB.Where("username = ?", username).First(&u).Error; err != nil {
		return
	}

	// 两个方向都不限制，跳过
	if u.MaxTrafficDown <= 0 && u.MaxTrafficUp <= 0 {
		return
	}

	downBytes, upBytes := AggregateUserDailyTraffic(username)
	month := vpc.CurrentTrafficMonth()

	// 获取或创建本月记录
	var daily model.UserTrafficDaily
	result := model.DB.Where("username = ? AND date = ?", username, month).First(&daily)
	if result.Error != nil {
		daily = model.UserTrafficDaily{
			Username: username,
			Date:     month,
		}
	}

	// 减去偏移量得到有效流量
	effectiveDown := downBytes - daily.OffsetDown
	effectiveUp := upBytes - daily.OffsetUp
	if effectiveDown < 0 {
		effectiveDown = 0
	}
	if effectiveUp < 0 {
		effectiveUp = 0
	}

	// 更新流量数据
	daily.TrafficDown = effectiveDown
	daily.TrafficUp = effectiveUp

	needApplyPenalty := false
	needRecover := false

	// 检查下行是否超限
	if u.MaxTrafficDown > 0 && float64(effectiveDown) >= u.MaxTrafficDown*1024*1024*1024 {
		if !daily.IsLimitedDown {
			daily.IsLimitedDown = true
			needApplyPenalty = true
			logger.App.Warn("用户下行流量超限，启用限速",
				"user", username, "used", FormatTrafficBytes(effectiveDown), "quota_gb", u.MaxTrafficDown, "penalty_mbps", trafficPenaltyDownMbps)
		}
	} else if daily.IsLimitedDown {
		daily.IsLimitedDown = false
		needRecover = true
		logger.App.Info("用户下行流量已低于当前月配额，恢复正常", "user", username)
	}

	// 检查上行是否超限
	if u.MaxTrafficUp > 0 && float64(effectiveUp) >= u.MaxTrafficUp*1024*1024*1024 {
		if !daily.IsLimitedUp {
			daily.IsLimitedUp = true
			needApplyPenalty = true
			logger.App.Warn("用户上行流量超限，启用限速",
				"user", username, "used", FormatTrafficBytes(effectiveUp), "quota_gb", u.MaxTrafficUp, "penalty_mbps", trafficPenaltyUpMbps)
		}
	} else if daily.IsLimitedUp {
		daily.IsLimitedUp = false
		needRecover = true
		logger.App.Info("用户上行流量已低于当前月配额，恢复正常", "user", username)
	}

	// 保存本月记录
	if daily.ID == 0 {
		model.DB.Create(&daily)
	} else {
		model.DB.Save(&daily)
	}

	if needRecover {
		if err := bandwidth.RebalanceUserBandwidth(username); err != nil {
			logger.App.Warn("恢复用户带宽失败", "component", "流量配额", "user", username, "error", err)
		}
	}

	// 如果有新的超限发生，对非 VPC VM 应用惩罚速率。
	if needApplyPenalty {
		applyTrafficPenalty(username, daily.IsLimitedDown, daily.IsLimitedUp)
	}
}

// applyTrafficPenalty 对用户所有 VM 应用流量超限惩罚速率
// 仅对超限方向设置惩罚速率，未超限方向保持原有设置
func applyTrafficPenalty(username string, downLimited, upLimited bool) {
	vms := user.GetUserVMList(username)
	if len(vms) == 0 {
		return
	}

	var u model.User
	if err := model.DB.Where("username = ?", username).First(&u).Error; err != nil {
		return
	}

	for _, vmName := range vms {
		if bandwidth.IsVPCBoundVM(vmName) {
			logger.App.Info("跳过 VPC VM 的用户级惩罚限速，交换机配额负责控制", "component", "流量配额", "vm", vmName)
			continue
		}
		// 获取当前带宽设置
		currentBW, err := bandwidth.GetVMBandwidth(vmName)
		if err != nil {
			// 可能没有网卡，跳过
			continue
		}

		// 确定各方向的速率
		downAvg := currentBW.InboundAvg // 当前下行平峰（Mbps）
		upAvg := currentBW.OutboundAvg  // 当前上行平峰（Mbps）

		if downLimited {
			downAvg = trafficPenaltyDownMbps
		}
		if upLimited {
			upAvg = trafficPenaltyUpMbps
		}

		// peak = avg（硬限制，不允许突发超过惩罚速率）
		downPeak := downAvg
		upPeak := upAvg

		// burst 最小值
		downBurst := bandwidth.MbpsToKBps(downAvg) * 30
		upBurst := bandwidth.MbpsToKBps(upAvg) * 30

		if err := bandwidth.ApplyVMBandwidth(vmName,
			bandwidth.MbpsToKBps(downAvg), bandwidth.MbpsToKBps(downPeak), downBurst,
			bandwidth.MbpsToKBps(upAvg), bandwidth.MbpsToKBps(upPeak), upBurst); err != nil {
			logger.App.Warn("为 VM 设置惩罚速率失败", "component", "流量配额", "vm", vmName, "error", err)
		}
	}
}

// ResetUserTrafficQuota 管理员手动重置用户流量配额
// 记录当前已用量作为偏移基线，等效于“从零开始重新计算配额”
func ResetUserTrafficQuota(username string) error {
	// 获取当前实际已用流量（未减偏移量的原始值）
	downBytes, upBytes := AggregateUserDailyTraffic(username)
	month := vpc.CurrentTrafficMonth()

	// 获取或创建本月记录
	var daily model.UserTrafficDaily
	result := model.DB.Where("username = ? AND date = ?", username, month).First(&daily)
	if result.Error != nil {
		daily = model.UserTrafficDaily{
			Username: username,
			Date:     month,
		}
	}

	// 将当前已用量设为偏移基线，后续计算时会减去这个值
	daily.OffsetDown = downBytes
	daily.OffsetUp = upBytes
	daily.TrafficDown = 0
	daily.TrafficUp = 0
	daily.IsLimitedDown = false
	daily.IsLimitedUp = false

	if daily.ID == 0 {
		model.DB.Create(&daily)
	} else {
		model.DB.Save(&daily)
	}

	// 恢复正常带宽设置
	if err := bandwidth.RebalanceUserBandwidth(username); err != nil {
		return fmt.Errorf("恢复带宽设置失败: %w", err)
	}

	logger.App.Info("管理员已重置用户的流量配额",
		"component", "流量配额", "user", username, "offset_down", FormatTrafficBytes(downBytes), "offset_up", FormatTrafficBytes(upBytes))
	return nil
}

// ResetAllDailyTraffic 每月定时重置所有用户的月流量配额。
// 保留函数名用于兼容旧调用，实际重置窗口已改为月。
func ResetAllDailyTraffic() {
	logger.App.Info("开始执行月流量配额重置", "component", "流量配额")

	// 查找上月有限速记录的用户
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")
	var limitedRecords []model.UserTrafficDaily
	model.DB.Where("date = ? AND (is_limited_down = ? OR is_limited_up = ?)", lastMonth, true, true).
		Find(&limitedRecords)

	// 恢复所有被限速用户的带宽
	for _, record := range limitedRecords {
		if err := bandwidth.RebalanceUserBandwidth(record.Username); err != nil {
			logger.App.Warn("恢复用户带宽失败", "component", "流量配额", "user", record.Username, "error", err)
		} else {
			logger.App.Info("已恢复用户的正常带宽", "component", "流量配额", "user", record.Username)
		}
	}

	// 清理 12 个月前的流量记录
	cleanupDate := time.Now().AddDate(0, -12, 0).Format("2006-01")
	model.DB.Where("date < ?", cleanupDate).Delete(&model.UserTrafficDaily{})
	vpc.ResetAllVPCSwitchMonthlyTraffic()
	lightweight.ResetAllLightweightVMTraffic()

	logger.App.Info("月流量配额重置完成", "component", "流量配额")
}

// CheckAllUsersTrafficQuota 检查所有用户的流量配额（定时调用）
func CheckAllUsersTrafficQuota() {
	trafficCheckLock.Lock()
	defer trafficCheckLock.Unlock()

	var users []model.User
	model.DB.Where("role = ? AND (max_traffic_down > 0 OR max_traffic_up > 0)", "user").Find(&users)

	for _, u := range users {
		if lightweight.IsLightweightCloudType(u.CloudType) {
			continue
		}
		CheckAndApplyTrafficLimit(u.Username)
	}
	vpc.CheckAllVPCSwitchTrafficQuota()
	lightweight.CheckAllLightweightVMTrafficQuota()
}

// CheckTrafficAfterQuotaUpdate 配额调整后检查是否需要恢复限速
// 如果新配额大于当前用量，自动恢复正常带宽
func CheckTrafficAfterQuotaUpdate(username string) {
	month := vpc.CurrentTrafficMonth()
	var daily model.UserTrafficDaily
	if err := model.DB.Where("username = ? AND date = ?", username, month).First(&daily).Error; err != nil {
		return // 没有本月记录，无需处理
	}

	// 如果没有限速状态，不需要处理
	if !daily.IsLimitedDown && !daily.IsLimitedUp {
		return
	}

	var u model.User
	if err := model.DB.Where("username = ?", username).First(&u).Error; err != nil {
		return
	}

	needRecover := false

	// 检查下行：配额调大了或设为不限制，且当前已限速
	if daily.IsLimitedDown {
		if u.MaxTrafficDown <= 0 || float64(daily.TrafficDown) < u.MaxTrafficDown*1024*1024*1024 {
			daily.IsLimitedDown = false
			needRecover = true
			logger.App.Info("用户下行配额调整后恢复正常", "component", "流量配额", "user", username)
		}
	}

	// 检查上行：同理
	if daily.IsLimitedUp {
		if u.MaxTrafficUp <= 0 || float64(daily.TrafficUp) < u.MaxTrafficUp*1024*1024*1024 {
			daily.IsLimitedUp = false
			needRecover = true
			logger.App.Info("用户上行配额调整后恢复正常", "component", "流量配额", "user", username)
		}
	}

	if needRecover {
		model.DB.Save(&daily)
		// 恢复正常带宽
		if err := bandwidth.RebalanceUserBandwidth(username); err != nil {
			logger.App.Warn("恢复用户带宽失败", "component", "流量配额", "user", username, "error", err)
		}
	}
}

// IsUserTrafficLimited 查询用户某方向是否处于流量限速状态
func IsUserTrafficLimited(username string) (downLimited, upLimited bool) {
	month := vpc.CurrentTrafficMonth()
	var daily model.UserTrafficDaily
	// 避免在本月记录不存在时认为已被限制
	if err := model.DB.Where("username = ? AND date = ?", username, month).First(&daily).Error; err != nil {
		return false, false
	}
	return daily.IsLimitedDown, daily.IsLimitedUp
}

// CheckUserTrafficQuotaForStart 在启动容器和编排时强制检查用户是否有可用的网络配额
func CheckUserTrafficQuotaForStart(username string) error {
	downLimited, upLimited := IsUserTrafficLimited(username)
	if downLimited || upLimited {
		return fmt.Errorf("本月流量配额已超限，无法启动新容器。请联系管理员处理")
	}
	return nil
}

// StartTrafficQuotaChecker 启动流量配额检查定时器
// 在 StatsCollector 中调用
func StartTrafficQuotaChecker() {
	// 每 60 秒检查一次所有用户的流量配额
	go func() {
		defer utils.RecoverAndLog("traffic-quota-checker")
		checkTicker := time.NewTicker(60 * time.Second)
		defer checkTicker.Stop()

		logger.App.Info("定时检查器已启动", "component", "流量配额", "interval", "60s")

		for range checkTicker.C {
			CheckAllUsersTrafficQuota()
		}
	}()

	// 每月 1 日 0 点重置定时器
	go func() {
		defer utils.RecoverAndLog("traffic-quota-reset")
		for {
			now := time.Now()
			// 计算距下个月 1 日 0 点的时间
			next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
			duration := next.Sub(now)
			logger.App.Info("下次重置时间", "component", "流量配额", "next", next.Format("2006-01-02 15:04:05"), "duration", duration)

			timer := time.NewTimer(duration)
			<-timer.C

			ResetAllDailyTraffic()
		}
	}()
}

// FormatTrafficBytes 将字节数格式化为人类可读的流量字符串
func FormatTrafficBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
