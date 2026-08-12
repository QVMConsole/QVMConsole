package public_ip

import (
	"context"
	"sync"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/utils"
)

var publicIPv6MonitorOnce sync.Once

// StartPublicIPv6PrefixMonitor 周期检测动态公网 IPv6 前缀变化并原位更新 /128 分配。
func StartPublicIPv6PrefixMonitor() {
	publicIPv6MonitorOnce.Do(func() {
		go func() {
			// 启动后先同步已有绑定，避免仅依赖后续面板操作触发来宾配置。
			defer utils.RecoverAndLog("public-ipv6-prefix-monitor")
			initialTimer := time.NewTimer(5 * time.Second)
			<-initialTimer.C
			ReconcilePendingPublicIPv4Guests(context.Background())
			ReconcilePendingPublicIPv6Guests(context.Background())
			for {
				interval := 60
				if config.GlobalConfig != nil && config.GlobalConfig.PublicIPv6SyncIntervalSeconds >= 10 {
					interval = config.GlobalConfig.PublicIPv6SyncIntervalSeconds
				}
				timer := time.NewTimer(time.Duration(interval) * time.Second)
				<-timer.C
				ReconcilePendingPublicIPv4Guests(context.Background())
				ReconcilePendingPublicIPv6Guests(context.Background())
				publicIPApplyMu.Lock()
				changed, err := SyncManagedPublicIPv6Addresses()
				if err == nil && changed > 0 {
					err = applyPublicIPRulesLocked(false)
				}
				publicIPApplyMu.Unlock()
				if err != nil {
					logger.App.Warn("同步动态公网 IPv6 前缀失败", "error", err)
					continue
				}
				if changed == 0 {
					continue
				}
				ReconcilePendingPublicIPv6Guests(context.Background())
				if HookApplyVPCACLRules != nil {
					if applyErr := HookApplyVPCACLRules(); applyErr != nil {
						logger.App.Warn("公网 IPv6 前缀变化后同步 VPC ACL 失败", "error", applyErr)
					}
				}
				if config.GlobalConfig != nil && config.GlobalConfig.PortSecurityEnabled && HookReconcilePortSecurity != nil {
					if reconcileErr := HookReconcilePortSecurity(); reconcileErr != nil {
						logger.App.Warn("公网 IPv6 前缀变化后协调端口安全失败", "error", reconcileErr)
					}
				}
				logger.App.Info("动态公网 IPv6 前缀已同步", "address_count", changed)
			}
		}()
	})
}
