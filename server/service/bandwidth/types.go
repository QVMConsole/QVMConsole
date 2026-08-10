package bandwidth

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/digitalocean/go-libvirt"
)

// BandwidthInfo 带宽信息（KB/s 或 KB）
type BandwidthInfo struct {
	Average int `json:"average"` // 平峰速率 KB/s
	Peak    int `json:"peak"`    // 峰值速率 KB/s
	Burst   int `json:"burst"`   // 突发量 KB
}

// BandwidthDetail VM 带宽详情（用户可读的 Mbps）
type BandwidthDetail struct {
	InboundAvg    int `json:"inbound_avg"`    // 下行平峰 Mbps
	InboundPeak   int `json:"inbound_peak"`   // 下行峰值 Mbps
	InboundBurst  int `json:"inbound_burst"`  // 下行突发量 KB
	OutboundAvg   int `json:"outbound_avg"`   // 上行平峰 Mbps
	OutboundPeak  int `json:"outbound_peak"`  // 上行峰值 Mbps
	OutboundBurst int `json:"outbound_burst"` // 上行突发量 KB
}

// VmBandwidthConfigRaw VM 带宽配置原始值
type VmBandwidthConfigRaw struct {
	InboundAvg    int
	InboundPeak   int
	InboundBurst  int
	OutboundAvg   int
	OutboundPeak  int
	OutboundBurst int
}

// MbpsToKBps 将 Mbps 转换为 KB/s（1 Mbps = 1000/8 KB/s = 125 KB/s）
func MbpsToKBps(mbps int) int {
	return mbps * 125
}

// KBpsToMbps 将 KB/s 转换为 Mbps（向下取整）
func KBpsToMbps(kbps int) int {
	if kbps <= 0 {
		return 0
	}
	return kbps / 125
}

// TcRateKbit 将平均 KB/s 转换为 tc 的 kbit 速率
func TcRateKbit(avgKBps int) int {
	if avgKBps <= 0 {
		return 0
	}
	return avgKBps * 8
}

// TcBurstBytes 计算 tc 的 burst 字节数
func TcBurstBytes(avgKBps int) int {
	burstBytes := avgKBps * 1024 / 10
	if burstBytes < 15360 {
		return 15360
	}
	return burstBytes
}

// TcIFBTxQueueLen 返回 IFB 接口的 txqueuelen
func TcIFBTxQueueLen() int {
	return 100
}

// TcUploadIFBName 根据 vnet 接口名生成对应的 IFB 接口名
func TcUploadIFBName(vnetIF string) string {
	cleaned := strings.TrimSpace(vnetIF)
	if cleaned == "" {
		return ""
	}
	cleaned = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, cleaned)
	name := "ifb-" + cleaned
	if len(name) <= 15 {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(cleaned))
	return fmt.Sprintf("ifb%x", h.Sum32())
}

// OvsBandwidthCookie 生成 OVS 带宽流表的 cookie 值
func OvsBandwidthCookie(vmName string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte("kvm-console-bandwidth:" + vmName))
	return fmt.Sprintf("0x%x", h.Sum64())
}

// OvsBandwidthQueueID 生成 OVS 队列 ID
func OvsBandwidthQueueID(vmName, direction string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte("kvm-console-bandwidth:" + vmName + ":" + direction))
	// OVS 队列 ID 使用较小正整数，便于 set_queue 和人工排查。
	return 1000 + h.Sum32()%60000
}

// OvsBandwidthMeterID 生成 OVS meter ID
func OvsBandwidthMeterID(vmName, direction string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte("kvm-console-bandwidth:" + vmName + ":" + direction))
	// 与端口安全 meter 使用不同的有界区间，兼容常见 max_meter=200000 的宿主机。
	return 100000 + h.Sum32()%40000
}

// OvsBandwidthMaxRateBps 将平均 KB/s 转换为 bps
func OvsBandwidthMaxRateBps(avgKBps int) int {
	if avgKBps <= 0 {
		return 0
	}
	return avgKBps * 8000
}

// OvsBandwidthQueueKey 将队列 ID 转换为字符串键
func OvsBandwidthQueueKey(queueID uint32) string {
	return strconv.FormatUint(uint64(queueID), 10)
}

// OvsBandwidthMeterArg 将 meter ID 转换为 meter 参数字符串
func OvsBandwidthMeterArg(meterID uint32) string {
	return "meter=" + strconv.FormatUint(uint64(meterID), 10)
}

// OvsBandwidthRateKbit 将平均 KB/s 转换为 kbit
func OvsBandwidthRateKbit(avgKBps int) int {
	if avgKBps <= 0 {
		return 0
	}
	return avgKBps * 8
}

// BuildBandwidthParams 构建 inbound/outbound 带宽参数的 TypedParam 列表
// average 始终包含（值为 0 表示不限制），peak/burst 仅当值 > 0 时添加
func BuildBandwidthParams(downAvg, downPeak, downBurst, upAvg, upPeak, upBurst int) []libvirt.TypedParam {
	var params []libvirt.TypedParam
	// 始终包含 average 参数（值为 0 表示不限制），避免空参数传给 libvirt 导致 "params must not be NULL" 错误
	params = append(params, libvirt.TypedParam{
		Field: libvirt.DomainBandwidthInAverage,
		Value: *libvirt.NewTypedParamValueUint(uint32(downAvg)),
	})
	if downPeak > 0 {
		params = append(params, libvirt.TypedParam{
			Field: libvirt.DomainBandwidthInPeak,
			Value: *libvirt.NewTypedParamValueUint(uint32(downPeak)),
		})
	}
	if downBurst > 0 {
		params = append(params, libvirt.TypedParam{
			Field: libvirt.DomainBandwidthInBurst,
			Value: *libvirt.NewTypedParamValueUint(uint32(downBurst)),
		})
	}
	params = append(params, libvirt.TypedParam{
		Field: libvirt.DomainBandwidthOutAverage,
		Value: *libvirt.NewTypedParamValueUint(uint32(upAvg)),
	})
	if upPeak > 0 {
		params = append(params, libvirt.TypedParam{
			Field: libvirt.DomainBandwidthOutPeak,
			Value: *libvirt.NewTypedParamValueUint(uint32(upPeak)),
		})
	}
	if upBurst > 0 {
		params = append(params, libvirt.TypedParam{
			Field: libvirt.DomainBandwidthOutBurst,
			Value: *libvirt.NewTypedParamValueUint(uint32(upBurst)),
		})
	}
	return params
}
