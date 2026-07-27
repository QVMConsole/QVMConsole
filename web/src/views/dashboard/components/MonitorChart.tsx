/**
 * 资源监控图表（ECharts）
 * - 管理员仪表盘：近 24 小时 CPU / 内存使用率
 * - 跟随主题明暗切换配色，容器尺寸变化自适应
 */
import { useEffect, useRef } from 'react'
import * as echarts from 'echarts'
import type { HostStatsRecord } from '@/api/host'
import { useTheme } from '@/hooks/useTheme'

interface MonitorChartProps {
  records: HostStatsRecord[]
}

export default function MonitorChart({ records }: MonitorChartProps) {
  const boxRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<echarts.ECharts | null>(null)
  const { isDark } = useTheme()

  useEffect(() => {
    if (!boxRef.current) return
    const chart = echarts.init(boxRef.current)
    chartRef.current = chart
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(boxRef.current)
    return () => {
      observer.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    const chart = chartRef.current
    if (!chart) return

    const times = records.map((r) =>
      new Date(r.recorded_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }),
    )
    const cpuData = records.map((r) => Number(r.cpu_percent.toFixed(1)))
    const memData = records.map((r) =>
      r.mem_total > 0 ? Number(((r.mem_used / r.mem_total) * 100).toFixed(1)) : 0,
    )

    const axisColor = isDark ? '#4B5468' : '#8B97AD'
    const splitColor = isDark ? 'rgba(148,163,184,.08)' : 'rgba(30,41,59,.07)'

    chart.setOption({
      grid: { left: 40, right: 12, top: 16, bottom: 26 },
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(12,19,34,.92)' : 'rgba(255,255,255,.96)',
        borderColor: isDark ? 'rgba(148,163,184,.2)' : 'rgba(30,41,59,.12)',
        textStyle: { color: isDark ? '#E7EBF3' : '#1b2434', fontSize: 12 },
        valueFormatter: (v: number) => `${v}%`,
      },
      xAxis: {
        type: 'category',
        data: times,
        boundaryGap: false,
        axisLine: { lineStyle: { color: splitColor } },
        axisTick: { show: false },
        axisLabel: { color: axisColor, fontSize: 10, interval: 'auto' },
      },
      yAxis: {
        type: 'value',
        max: 100,
        axisLabel: { color: axisColor, fontSize: 10, formatter: '{value}%' },
        splitLine: { lineStyle: { color: splitColor } },
      },
      series: [
        {
          name: 'CPU',
          type: 'line',
          smooth: true,
          symbol: 'none',
          data: cpuData,
          lineStyle: { width: 2.2, color: '#2DD4BF', shadowColor: 'rgba(45,212,191,.5)', shadowBlur: 8 },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(45,212,191,.22)' },
              { offset: 1, color: 'rgba(45,212,191,0)' },
            ]),
          },
        },
        {
          name: '内存',
          type: 'line',
          smooth: true,
          symbol: 'none',
          data: memData,
          lineStyle: { width: 2.2, color: '#8B5CF6', shadowColor: 'rgba(139,92,246,.5)', shadowBlur: 8 },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(139,92,246,.18)' },
              { offset: 1, color: 'rgba(139,92,246,0)' },
            ]),
          },
        },
      ],
    })
  }, [records, isDark])

  if (records.length === 0) {
    return <div className="qvm-chart-empty">暂无监控数据，采集任务运行后将自动展示</div>
  }
  return <div ref={boxRef} className="qvm-chart-box" />
}
