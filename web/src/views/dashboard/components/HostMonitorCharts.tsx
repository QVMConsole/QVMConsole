/**
 * 宿主机资源监控图表区（管理员仪表盘）
 * - 实时监控：由宿主机 SSE 推送驱动（CPU / 内存 / 网络 / 磁盘 IO 四图，5s 一个点）
 * - 历史查询：日期范围 + 近 24 小时快捷查询（基于累计值增量计算速率）
 * - 网络 / 磁盘卡片支持选择查看的具体物理网卡 / 物理硬盘（实时与历史均生效）
 * - 支持滚轮缩放 + 底部滑块框选（dataZoom），配色跟随明暗主题
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import * as echarts from 'echarts'
import { Button, DatePicker, Radio, Select, Toast } from '@douyinfe/semi-ui'
import type { HostDiskDeviceStat, HostNetDeviceStat, HostStats, HostStatsRecord } from '@/api/host'
import { getHostStatsHistory } from '@/api/host'
import { useTheme } from '@/hooks/useTheme'

interface HostMonitorChartsProps {
  /** SSE 推送的宿主机实时数据 */
  externalStats: HostStats | null
}

type ChartMode = 'realtime' | 'history'

/** 实时曲线最多保留的数据点数 */
const MAX_REALTIME_POINTS = 60
/** 宿主机 SSE 推送间隔（秒），与后端保持一致 */
const SSE_INTERVAL_SECONDS = 5
/** 设备选择器"全部"占位值 */
const DEVICE_ALL = 'all'

/** KB/s 流量自动换算 */
function formatAxisTraffic(valueKB: number): string {
  if (valueKB == null || Number(valueKB) < 0) return '0 KB/s'
  const units = ['KB/s', 'MB/s', 'GB/s', 'TB/s']
  let val = Number(valueKB)
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx += 1
  }
  return `${val.toFixed(1)} ${units[idx]}`
}

/** 格式化后端历史查询时间参数（YYYY-MM-DDTHH:mm:ss） */
function formatQueryTime(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 判断两个设备名列表是否相同（避免选项列表无变化时反复触发重渲染） */
function sameNames(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

/** 历史记录中按设备取网络累计值（all 取汇总字段） */
function pickNetValue(record: HostStatsRecord, device: string): { rx: number; tx: number; exists: boolean } {
  if (device === DEVICE_ALL) return { rx: record.net_rx_bytes, tx: record.net_tx_bytes, exists: true }
  const dev = (record.net_devices || []).find((d: HostNetDeviceStat) => d.name === device)
  return dev ? { rx: dev.rx_bytes, tx: dev.tx_bytes, exists: true } : { rx: 0, tx: 0, exists: false }
}

/** 历史记录中按设备取磁盘累计值（all 取汇总字段） */
function pickDiskValue(record: HostStatsRecord, device: string): { rd: number; wr: number; exists: boolean } {
  if (device === DEVICE_ALL) return { rd: record.disk_rd_bytes, wr: record.disk_wr_bytes, exists: true }
  const dev = (record.disk_devices || []).find((d: HostDiskDeviceStat) => d.name === device)
  return dev ? { rd: dev.rd_bytes, wr: dev.wr_bytes, exists: true } : { rd: 0, wr: 0, exists: false }
}

export default function HostMonitorCharts({ externalStats }: HostMonitorChartsProps) {
  const { isDark } = useTheme()
  const [mode, setMode] = useState<ChartMode>('realtime')
  const [historyRange, setHistoryRange] = useState<Date[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  // 网络 / 磁盘设备选择器状态（'all' = 全部设备汇总）
  const [netDevice, setNetDevice] = useState(DEVICE_ALL)
  const [diskDevice, setDiskDevice] = useState(DEVICE_ALL)
  const [netDeviceNames, setNetDeviceNames] = useState<string[]>([])
  const [diskDeviceNames, setDiskDeviceNames] = useState<string[]>([])

  const cpuRef = useRef<HTMLDivElement>(null)
  const memRef = useRef<HTMLDivElement>(null)
  const netRef = useRef<HTMLDivElement>(null)
  const diskRef = useRef<HTMLDivElement>(null)
  const chartsRef = useRef<echarts.ECharts[]>([])
  const prevNetRef = useRef({ rx: 0, tx: 0 })
  const prevDiskRef = useRef({ rd: 0, wr: 0 })
  const modeRef = useRef<ChartMode>('realtime')
  modeRef.current = mode

  // ============ 初始化图表实例 ============
  useEffect(() => {
    const refs = [cpuRef, memRef, netRef, diskRef]
    const charts: echarts.ECharts[] = []
    refs.forEach((ref) => {
      if (ref.current) {
        charts.push(echarts.init(ref.current))
      }
    })
    chartsRef.current = charts

    const observer = new ResizeObserver(() => {
      charts.forEach((c) => c.resize())
    })
    refs.forEach((ref) => ref.current && observer.observe(ref.current))

    return () => {
      observer.disconnect()
      charts.forEach((c) => c.dispose())
      chartsRef.current = []
    }
  }, [])

  // ============ 基础 option（跟随主题） ============
  const baseOption = useCallback(
    (yMax?: number, yFormatter?: (v: number) => string): echarts.EChartsOption => ({
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(12,19,34,.92)' : 'rgba(255,255,255,.96)',
        borderColor: isDark ? 'rgba(148,163,184,.2)' : 'rgba(30,41,59,.12)',
        textStyle: { color: isDark ? '#E7EBF3' : '#1b2434', fontSize: 12 },
      },
      grid: { left: 8, right: 12, top: 34, bottom: 44, containLabel: true },
      // 缩放：滚轮/拖拽（inside）+ 底部滑块（slider）
      dataZoom: [
        { type: 'inside', start: 0, end: 100 },
        {
          type: 'slider',
          start: 0,
          end: 100,
          height: 14,
          bottom: 4,
          borderColor: 'transparent',
          backgroundColor: isDark ? 'rgba(148,163,184,.06)' : 'rgba(30,41,59,.05)',
          fillerColor: isDark ? 'rgba(45,212,191,.18)' : 'rgba(13,148,136,.14)',
          handleStyle: { color: isDark ? '#2DD4BF' : '#0D9488' },
          textStyle: { color: isDark ? '#59627A' : '#8B97AD', fontSize: 9 },
        },
      ],
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: [],
        axisLine: { lineStyle: { color: isDark ? 'rgba(148,163,184,.12)' : 'rgba(30,41,59,.1)' } },
        axisTick: { show: false },
        axisLabel: { color: isDark ? '#59627A' : '#8B97AD', fontSize: 10 },
      },
      yAxis: {
        type: 'value',
        max: yMax,
        axisLabel: {
          color: isDark ? '#59627A' : '#8B97AD',
          fontSize: 10,
          formatter: yFormatter,
        },
        splitLine: { lineStyle: { color: isDark ? 'rgba(148,163,184,.07)' : 'rgba(30,41,59,.06)' } },
      },
      series: [],
    }),
    [isDark],
  )

  const lineSeries = useCallback(
    (name: string, color: string, area = true): echarts.SeriesOption => ({
      name,
      type: 'line',
      smooth: true,
      symbol: 'none',
      data: [],
      lineStyle: { width: 2, color },
      itemStyle: { color },
      areaStyle: area
        ? {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: `${color}38` },
              { offset: 1, color: `${color}00` },
            ]),
          }
        : undefined,
    }),
    [],
  )

  // ============ 主题变化时重建 option ============
  useEffect(() => {
    const [cpu, mem, net, disk] = chartsRef.current
    if (!cpu || !mem || !net || !disk) return

    const titleStyle = { fontSize: 12, color: isDark ? '#8B95AB' : '#55617A' }
    cpu.setOption({
      ...baseOption(100),
      title: { text: 'CPU 使用率 (%)', textStyle: titleStyle, left: 4, top: 4 },
      series: [lineSeries('CPU', '#2DD4BF')],
    })
    mem.setOption({
      ...baseOption(100),
      title: { text: '内存使用率 (%)', textStyle: titleStyle, left: 4, top: 4 },
      series: [lineSeries('内存', '#8B5CF6')],
    })
    net.setOption({
      ...baseOption(undefined, formatAxisTraffic),
      title: { text: '网络流量', textStyle: titleStyle, left: 4, top: 4 },
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(12,19,34,.92)' : 'rgba(255,255,255,.96)',
        borderColor: isDark ? 'rgba(148,163,184,.2)' : 'rgba(30,41,59,.12)',
        textStyle: { color: isDark ? '#E7EBF3' : '#1b2434', fontSize: 12 },
        valueFormatter: (v: number) => formatAxisTraffic(v),
      },
      series: [lineSeries('接收', '#38BDF8'), lineSeries('发送', '#34D399')],
    })
    disk.setOption({
      ...baseOption(undefined, formatAxisTraffic),
      title: { text: '磁盘 I/O', textStyle: titleStyle, left: 4, top: 4 },
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(12,19,34,.92)' : 'rgba(255,255,255,.96)',
        borderColor: isDark ? 'rgba(148,163,184,.2)' : 'rgba(30,41,59,.12)',
        textStyle: { color: isDark ? '#E7EBF3' : '#1b2434', fontSize: 12 },
        valueFormatter: (v: number) => formatAxisTraffic(v),
      },
      series: [lineSeries('读取', '#FBBF24'), lineSeries('写入', '#FB7185')],
    })

    prevNetRef.current = { rx: 0, tx: 0 }
    prevDiskRef.current = { rd: 0, wr: 0 }
  }, [baseOption, lineSeries, isDark])

  // ============ 实时数据驱动（SSE externalStats） ============
  useEffect(() => {
    if (!externalStats || modeRef.current !== 'realtime') return
    const [cpu, mem, net, disk] = chartsRef.current
    if (!cpu || !mem || !net || !disk) return

    // 同步物理网卡 / 物理硬盘下拉选项；所选设备已不存在时退回"全部"并跳过本次推送
    const netNames = (externalStats.net_devices || []).map((d) => d.name)
    const diskNames = (externalStats.disk_devices || []).map((d) => d.name)
    setNetDeviceNames((prev) => (sameNames(prev, netNames) ? prev : netNames))
    setDiskDeviceNames((prev) => (sameNames(prev, diskNames) ? prev : diskNames))
    if (netDevice !== DEVICE_ALL && !netNames.includes(netDevice)) {
      setNetDevice(DEVICE_ALL)
      prevNetRef.current = { rx: 0, tx: 0 }
      return
    }
    if (diskDevice !== DEVICE_ALL && !diskNames.includes(diskDevice)) {
      setDiskDevice(DEVICE_ALL)
      prevDiskRef.current = { rd: 0, wr: 0 }
      return
    }

    const now = new Date()
    const timeStr = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`

    const pushPoint = (chart: echarts.ECharts, values: (number | null)[]) => {
      const opt = chart.getOption() as echarts.EChartsOption
      const xAxis = (opt.xAxis as { data?: string[] }[])[0]
      const series = opt.series as echarts.SeriesOption[]
      const xData = xAxis?.data || []
      if (xData.length > MAX_REALTIME_POINTS) xData.shift()
      xData.push(timeStr)
      values.forEach((v, i) => {
        const data = (series[i].data as number[]) || []
        if (data.length > MAX_REALTIME_POINTS) data.shift()
        data.push(v ?? 0)
      })
      chart.setOption({ xAxis: { data: xData }, series })
    }

    pushPoint(cpu, [Number((externalStats.cpu_percent ?? 0).toFixed(1))])
    pushPoint(mem, [
      externalStats.mem_total > 0
        ? Number(((externalStats.mem_used / externalStats.mem_total) * 100).toFixed(1))
        : 0,
    ])

    // 网络速率：按所选物理网卡取累计值（all = 汇总字段），基于增量计算（KB/s）
    let rxBase = externalStats.net_rx_bytes
    let txBase = externalStats.net_tx_bytes
    if (netDevice !== DEVICE_ALL) {
      const dev = (externalStats.net_devices || []).find((d) => d.name === netDevice)
      rxBase = dev ? dev.rx_bytes : 0
      txBase = dev ? dev.tx_bytes : 0
    }
    const rxRate =
      prevNetRef.current.rx > 0 ? Math.max(0, (rxBase - prevNetRef.current.rx) / 1024 / SSE_INTERVAL_SECONDS) : 0
    const txRate =
      prevNetRef.current.tx > 0 ? Math.max(0, (txBase - prevNetRef.current.tx) / 1024 / SSE_INTERVAL_SECONDS) : 0
    prevNetRef.current = { rx: rxBase, tx: txBase }
    pushPoint(net, [Number(rxRate.toFixed(1)), Number(txRate.toFixed(1))])

    // 磁盘速率：按所选物理硬盘取累计值（all = 汇总字段），基于增量计算（KB/s）
    let rdBase = externalStats.disk_rd_bytes
    let wrBase = externalStats.disk_wr_bytes
    if (diskDevice !== DEVICE_ALL) {
      const dev = (externalStats.disk_devices || []).find((d) => d.name === diskDevice)
      rdBase = dev ? dev.rd_bytes : 0
      wrBase = dev ? dev.wr_bytes : 0
    }
    const rdRate =
      prevDiskRef.current.rd > 0 ? Math.max(0, (rdBase - prevDiskRef.current.rd) / 1024 / SSE_INTERVAL_SECONDS) : 0
    const wrRate =
      prevDiskRef.current.wr > 0 ? Math.max(0, (wrBase - prevDiskRef.current.wr) / 1024 / SSE_INTERVAL_SECONDS) : 0
    prevDiskRef.current = { rd: rdBase, wr: wrBase }
    pushPoint(disk, [Number(rdRate.toFixed(1)), Number(wrRate.toFixed(1))])
  }, [externalStats, netDevice, diskDevice])

  // ============ 历史查询渲染 ============
  // 保存最近一次成功查询的历史记录，供切换设备时立即重渲染
  const historyRecordsRef = useRef<HostStatsRecord[]>([])
  // 设备选择的 ref 同步，供 renderHistory 等回调即时读取（避免闭包捕获旧值）
  const netDeviceRef = useRef(DEVICE_ALL)
  const diskDeviceRef = useRef(DEVICE_ALL)
  netDeviceRef.current = netDevice
  diskDeviceRef.current = diskDevice

  const renderHistory = useCallback((records: HostStatsRecord[]) => {
    const [cpu, mem, net, disk] = chartsRef.current
    if (!cpu || !mem || !net || !disk || records.length === 0) return
    historyRecordsRef.current = records

    // 同步设备下拉选项（来自历史记录中的设备数据）
    const netNames = Array.from(new Set(records.flatMap((r) => (r.net_devices || []).map((d) => d.name))))
    const diskNames = Array.from(new Set(records.flatMap((r) => (r.disk_devices || []).map((d) => d.name))))
    setNetDeviceNames((prev) => (sameNames(prev, netNames) ? prev : netNames))
    setDiskDeviceNames((prev) => (sameNames(prev, diskNames) ? prev : diskNames))

    const timeLabels = records.map((r) => {
      const d = new Date(r.recorded_at)
      return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
    })

    cpu.setOption({
      xAxis: { data: timeLabels },
      series: [{ data: records.map((r) => Number(r.cpu_percent.toFixed(1))) }],
    })
    mem.setOption({
      xAxis: { data: timeLabels },
      series: [
        {
          data: records.map((r) =>
            r.mem_total > 0 ? Number(((r.mem_used / r.mem_total) * 100).toFixed(1)) : 0,
          ),
        },
      ],
    })

    // 累计值增量计算速率（KB/s），按所选设备取值（all = 汇总字段）
    const activeNetDevice = netDeviceRef.current
    const activeDiskDevice = diskDeviceRef.current
    const netRx: number[] = [0]
    const netTx: number[] = [0]
    const diskRd: number[] = [0]
    const diskWr: number[] = [0]
    for (let i = 1; i < records.length; i++) {
      const dt =
        (new Date(records[i].recorded_at).getTime() - new Date(records[i - 1].recorded_at).getTime()) / 1000
      if (dt > 0) {
        const prevNet = pickNetValue(records[i - 1], activeNetDevice)
        const curNet = pickNetValue(records[i], activeNetDevice)
        const prevDisk = pickDiskValue(records[i - 1], activeDiskDevice)
        const curDisk = pickDiskValue(records[i], activeDiskDevice)
        // 设备数据缺失（如记录点早于设备上线）时该点速率为 0，避免假尖峰
        if (prevNet.exists && curNet.exists) {
          netRx.push(Number(Math.max(0, (curNet.rx - prevNet.rx) / 1024 / dt).toFixed(1)))
          netTx.push(Number(Math.max(0, (curNet.tx - prevNet.tx) / 1024 / dt).toFixed(1)))
        } else {
          netRx.push(0)
          netTx.push(0)
        }
        if (prevDisk.exists && curDisk.exists) {
          let rd = Math.max(0, (curDisk.rd - prevDisk.rd) / 1024 / dt)
          let wr = Math.max(0, (curDisk.wr - prevDisk.wr) / 1024 / dt)
          // 防抖动限制（异常尖峰置零）
          if (rd > 10 * 1024 * 1024) rd = 0
          if (wr > 10 * 1024 * 1024) wr = 0
          diskRd.push(Number(rd.toFixed(1)))
          diskWr.push(Number(wr.toFixed(1)))
        } else {
          diskRd.push(0)
          diskWr.push(0)
        }
      } else {
        netRx.push(0)
        netTx.push(0)
        diskRd.push(0)
        diskWr.push(0)
      }
    }
    net.setOption({ xAxis: { data: timeLabels }, series: [{ data: netRx }, { data: netTx }] })
    disk.setOption({ xAxis: { data: timeLabels }, series: [{ data: diskRd }, { data: diskWr }] })
  }, [])

  const fetchHistory = useCallback(
    async (start: string, end: string) => {
      setHistoryLoading(true)
      try {
        const res = await getHostStatsHistory({ start, end })
        const records = res.data || []
        if (records.length === 0) {
          Toast.info('所选时间范围内无监控数据')
          return
        }
        renderHistory(records)
      } catch {
        // 请求层已提示
      } finally {
        setHistoryLoading(false)
      }
    },
    [renderHistory],
  )

  const handleQueryRange = () => {
    if (!historyRange || historyRange.length !== 2) {
      Toast.warning('请选择查询日期范围')
      return
    }
    const [start, end] = historyRange
    void fetchHistory(formatQueryTime(start), formatQueryTime(end))
  }

  const handleLast24h = () => {
    const end = new Date()
    const start = new Date(end.getTime() - 24 * 3600 * 1000)
    void fetchHistory(formatQueryTime(start), formatQueryTime(end))
  }

  /** 切换查看的物理网卡：重置实时差分基准，历史模式立即按新设备重渲染 */
  const handleNetDeviceChange = (value: string) => {
    setNetDevice(value)
    netDeviceRef.current = value
    prevNetRef.current = { rx: 0, tx: 0 }
    if (modeRef.current === 'history' && historyRecordsRef.current.length > 0) {
      renderHistory(historyRecordsRef.current)
    }
  }

  /** 切换查看的物理硬盘：重置实时差分基准，历史模式立即按新设备重渲染 */
  const handleDiskDeviceChange = (value: string) => {
    setDiskDevice(value)
    diskDeviceRef.current = value
    prevDiskRef.current = { rd: 0, wr: 0 }
    if (modeRef.current === 'history' && historyRecordsRef.current.length > 0) {
      renderHistory(historyRecordsRef.current)
    }
  }

  const handleModeChange = (next: ChartMode) => {
    setMode(next)
    if (next === 'realtime') {
      // 切回实时模式：清空历史曲线与增量基准
      chartsRef.current.forEach((chart) => {
        const opt = chart.getOption() as echarts.EChartsOption
        const series = (opt.series as echarts.SeriesOption[]) || []
        chart.setOption({
          xAxis: { data: [] },
          series: series.map((s) => ({ ...s, data: [] })),
        })
      })
      prevNetRef.current = { rx: 0, tx: 0 }
      prevDiskRef.current = { rd: 0, wr: 0 }
    } else {
      // 进入历史模式默认加载近 24 小时
      handleLast24h()
    }
  }

  return (
    <section
      className="qvm-panel-card qvm-g-border qvm-fade-up qvm-hostmon"
      style={{ '--qvm-delay': '240ms' } as React.CSSProperties}
    >
      <div className="qvm-panel-head">
        <span className="qvm-panel-title">资源监控</span>
        <span className="qvm-panel-sub">{mode === 'realtime' ? '实时刷新' : '历史趋势'}</span>
        <div className="qvm-hostmon-ops">
          <Radio.Group
            type="button"
            value={mode}
            onChange={(e) => handleModeChange(e.target.value as ChartMode)}
          >
            <Radio value="realtime">实时监控</Radio>
            <Radio value="history">历史查询</Radio>
          </Radio.Group>
          {mode === 'history' && (
            <>
              <DatePicker
                type="dateRange"
                size="small"
                style={{ width: 260 }}
                placeholder={['开始日期', '结束日期']}
                value={historyRange}
                onChange={(v) => setHistoryRange((v as Date[]) || [])}
              />
              <Button size="small" type="primary" loading={historyLoading} onClick={handleQueryRange}>
                查询
              </Button>
              <Button size="small" theme="light" loading={historyLoading} onClick={handleLast24h}>
                近 24 小时
              </Button>
            </>
          )}
        </div>
      </div>
      <div className="qvm-hostmon-grid">
        <div className="qvm-hostmon-card">
          <div ref={cpuRef} className="qvm-chart-box" />
        </div>
        <div className="qvm-hostmon-card">
          <div ref={memRef} className="qvm-chart-box" />
        </div>
        <div className="qvm-hostmon-card">
          <div className="qvm-hostmon-dev">
            <span>网卡</span>
            <Select
              size="small"
              value={netDevice}
              onChange={(v) => handleNetDeviceChange(String(v))}
              style={{ width: 170 }}
              placeholder="选择网卡"
            >
              <Select.Option value={DEVICE_ALL}>全部网卡</Select.Option>
              {netDeviceNames.map((name) => (
                <Select.Option key={name} value={name}>
                  {name}
                </Select.Option>
              ))}
            </Select>
          </div>
          <div ref={netRef} className="qvm-chart-box" />
        </div>
        <div className="qvm-hostmon-card">
          <div className="qvm-hostmon-dev">
            <span>硬盘</span>
            <Select
              size="small"
              value={diskDevice}
              onChange={(v) => handleDiskDeviceChange(String(v))}
              style={{ width: 170 }}
              placeholder="选择硬盘"
            >
              <Select.Option value={DEVICE_ALL}>全部硬盘</Select.Option>
              {diskDeviceNames.map((name) => (
                <Select.Option key={name} value={name}>
                  {name}
                </Select.Option>
              ))}
            </Select>
          </div>
          <div ref={diskRef} className="qvm-chart-box" />
        </div>
      </div>
    </section>
  )
}
