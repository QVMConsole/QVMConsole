package model

// HostNetDeviceStat 宿主机单个网络接口的累计流量统计（供实时推送与历史记录共用）
type HostNetDeviceStat struct {
	Name    string `json:"name"`     // 接口名（如 eth0 / enp3s0）
	RxBytes int64  `json:"rx_bytes"` // 累计接收字节数
	TxBytes int64  `json:"tx_bytes"` // 累计发送字节数
}

// HostDiskDeviceStat 宿主机单个物理硬盘的累计 IO 统计（供实时推送与历史记录共用）
type HostDiskDeviceStat struct {
	Name    string `json:"name"`     // 磁盘设备名（如 sda / nvme0n1）
	RdBytes int64  `json:"rd_bytes"` // 累计读取字节数
	WrBytes int64  `json:"wr_bytes"` // 累计写入字节数
}
