/**
 * 系统信息 Tab（详情页默认标签）
 * - 基本配置 / 登录凭证 / 网络与连接 / 高级设置 / 磁盘 IOPS 限制
 * - 挂载后自加载：PCIe 热插槽用量、磁盘 IOPS 列表、全部网口 IP
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Spin, Tag, Toast, Tooltip } from '@douyinfe/semi-ui'
import { IconCopy, IconEyeClosedSolid, IconEyeOpened, IconRefresh } from '@douyinfe/semi-icons'
import type { VmDetailInfo, VmDiskItem, VmNetworkInterface, VmPCIEInfo } from '@/api/vm'
import { getDiskList, getVmPCIEInfo, getVMNetworkStatus } from '@/api/vm'
import { copyTextWithFallback } from '@/utils/clipboard'
import {
  canResetVmPassword,
  formatMemoryMB,
  formatContinuousRuntime,
  ipSourceLabel,
  ipSourceTagColor,
} from '../utils'

interface InfoTabProps {
  vm: VmDetailInfo | null
  isLightweight: boolean
  onResetPassword: () => void
  onReinstall: () => void
  onRemark: () => void
}

/** 信息行 */
function Row({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="qvm-info-row">
      <span className="qvm-info-label">{label}</span>
      <span className="qvm-info-value">{children}</span>
    </div>
  )
}

export default function InfoTab({ vm, isLightweight, onResetPassword, onReinstall, onRemark }: InfoTabProps) {
  const [pcieInfo, setPcieInfo] = useState<VmPCIEInfo | null>(null)
  const [disks, setDisks] = useState<VmDiskItem[]>([])
  const [disksLoading, setDisksLoading] = useState(false)
  const [interfaceIPs, setInterfaceIPs] = useState<{ ip: string; source: string }[]>([])
  const [showAllIPs, setShowAllIPs] = useState(false)
  const [credentialPasswordVisible, setCredentialPasswordVisible] = useState(false)

  const vmName = vm?.name || ''
  const canResetPassword = canResetVmPassword(vm)

  // 切换虚拟机或凭据更新后，密码恢复为隐藏状态。
  useEffect(() => {
    setCredentialPasswordVisible(false)
  }, [vmName, vm?.credential?.password])

  // 加载 PCIe 热插槽信息
  useEffect(() => {
    if (!vmName) return
    let cancelled = false
    getVmPCIEInfo(vmName)
      .then((res) => !cancelled && setPcieInfo(res.data || null))
      .catch(() => !cancelled && setPcieInfo(null))
    return () => {
      cancelled = true
    }
  }, [vmName])

  // 加载磁盘 IOPS 列表
  const loadDisks = useCallback(async () => {
    if (!vmName || isLightweight) return
    setDisksLoading(true)
    try {
      const res = await getDiskList(vmName)
      setDisks((res.data || []).filter((d) => d.device_type !== 'cdrom'))
    } catch {
      setDisks([])
    } finally {
      setDisksLoading(false)
    }
  }, [vmName, isLightweight])

  // 加载全部网口 IP
  useEffect(() => {
    if (!vmName) return
    let cancelled = false
    getVMNetworkStatus(vmName)
      .then((res) => {
        if (cancelled) return
        const ifaces: VmNetworkInterface[] = res.data?.interfaces || []
        const seen = new Set<string>()
        const ips = ifaces
          .filter((i) => i.ip && i.ip !== '0.0.0.0')
          .map((i) => ({ ip: i.ip, source: i.ip_source || '' }))
          .filter((item) => {
            if (seen.has(item.ip)) return false
            seen.add(item.ip)
            return true
          })
        setInterfaceIPs(ips)
      })
      .catch(() => !cancelled && setInterfaceIPs([]))
    return () => {
      cancelled = true
    }
  }, [vmName])

  useEffect(() => {
    void loadDisks()
  }, [loadDisks])

  const copyField = useCallback(async (value: string, fieldName: string) => {
    if (!value) {
      Toast.warning(`暂无可复制的${fieldName}`)
      return
    }
    try {
      await copyTextWithFallback(value)
      Toast.success(`${fieldName}已复制到剪贴板`)
    } catch {
      Toast.warning(`复制${fieldName}失败，请手动复制`)
    }
  }, [])

  // Guest Agent 状态
  const guestAgent = useMemo(() => {
    const s = vm?.guest_agent_status
    if (!s) return { text: '未知', color: 'grey' as const }
    if (s.connected) return { text: '已连接', color: 'green' as const }
    if (s.configured) return { text: '已配置未连接', color: 'orange' as const }
    return { text: '未配置', color: 'grey' as const }
  }, [vm?.guest_agent_status])

  // 内存策略标签
  const memoryBackendText = vm?.memory_backend === 'virtio_mem' ? 'virtio-mem 弹性' : 'Balloon 动态'
  const memoryTooltip =
    vm?.memory_backend === 'virtio_mem'
      ? 'Windows 弹性内存基于 virtio-mem：主内存为规格值，基础内存自动计算；运行后使用率超过 70% 每次扩容 1GB，低于 50% 时按目标使用率缩容。'
      : '系统会根据宿主机资源和面板调度策略动态调整：宿主机内存紧张时可能回收，但最低不低于设定内存的 50%；资源充足时可额外调度约 30% 内存应对突发负载。'

  // IP 显示（已获取到 IP 时不再展示「无法获取」悬停提示）
  const ipDisplay = vm?.ip || (vm?.ip_status ? '无法获取' : '获取中…')
  const ipTooltip = vm?.ip
    ? ''
    : vm?.ip_status === 'vlan_bridge'
      ? '桥接 VLAN 模式下上游路由器分配的 IP 无法从宿主机获取'
      : vm?.ip_status === 'shut_off'
        ? '虚拟机处于关机状态，无法获取 IP'
        : ''

  const visibleIPs = showAllIPs ? interfaceIPs : interfaceIPs.slice(0, 3)

  if (!vm) {
    return (
      <div className="qvm-tab-loading">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="qvm-info-grid">
      {/* 基本配置 */}
      <div className="qvm-info-card">
        <div className="qvm-info-card-title">基本配置</div>
        <Row label="CPU">
          <Tag color="blue" size="small">{vm.vcpu} 核</Tag>
          {vm.cpu_limit_percent > 0 && <span className="qvm-sub-label">限制 {vm.cpu_limit_percent}%</span>}
        </Row>
        <Row label="内存">
          {formatMemoryMB(vm.memory)}
          {vm.memory_dynamic_enabled && (
            <Tooltip content={memoryTooltip} position="top">
              <Tag size="small" color={vm.memory_backend === 'virtio_mem' ? 'orange' : 'green'}>
                {vm.memory_backend === 'virtio_mem' ? '弹性内存' : '动态内存'}
              </Tag>
            </Tooltip>
          )}
        </Row>
        <Row label="PCIe 热插槽">
          {vm.machine_type === 'q35' || vm.machine_type === 'virt' ? (
            <>
              <Tag color="blue" size="small">{vm.pcie_root_ports || 0} 槽</Tag>
              {pcieInfo ? (
                <span className="qvm-sub-label">
                  已用 {pcieInfo.used_ports} · 空闲 {pcieInfo.free_ports}
                  {pcieInfo.free_ports <= 1 && (
                    <Tooltip content="热插槽即将用尽，建议关机后增加 PCIe 热插槽数量" position="top">
                      <Tag size="small" color="red">紧张</Tag>
                    </Tooltip>
                  )}
                </span>
              ) : (
                <span className="qvm-sub-label">加载中…</span>
              )}
            </>
          ) : (
            'i440FX 不支持热插槽'
          )}
        </Row>
        <Row label="系统磁盘">
          <span className="qvm-mono">{vm.disk_size || '-'}</span>
          {vm.disk_healthy === false && (
            <Tooltip content="系统磁盘文件缺失，虚拟机可能无法正常启动" position="top">
              <Tag size="small" color="red">磁盘缺失</Tag>
            </Tooltip>
          )}
        </Row>
        <Row label="操作系统">{vm.os_type || '-'}</Row>
        <Row label="机器类型">{vm.machine_type || '-'}</Row>
        <Row label="模板来源">{vm.template || '-'}</Row>
        <Row label="备注">
          <span className="qvm-remark-text">{vm.remark || '-'}</span>
          {!isLightweight && (
            <Button size="small" theme="borderless" type="primary" onClick={onRemark}>
              编辑备注
            </Button>
          )}
        </Row>
        <Row label="连续运行">
          {formatContinuousRuntime(vm.continuous_runtime_seconds, vm.status)}
          {vm.continuous_running_since && (
            <span className="qvm-sub-label">自 {vm.continuous_running_since}</span>
          )}
        </Row>
      </div>

      {/* 登录凭证 */}
      <div className="qvm-info-card">
        <div className="qvm-info-card-title">登录凭证</div>
        <Row label="用户名">
          {vm.credential?.username ? (
            <span className="qvm-credential-wrap">
              <code className="qvm-code">{vm.credential.username}</code>
              <Button
                size="small"
                theme="borderless"
                icon={<IconCopy size="small" />}
                onClick={() => void copyField(vm.credential?.username || '', '账号')}
              />
            </span>
          ) : (
            '-'
          )}
        </Row>
        <Row label="密码">
          {vm.credential?.password ? (
            <span className="qvm-credential-wrap">
              <code className="qvm-code qvm-code-pwd">
                {credentialPasswordVisible ? vm.credential.password : '••••••••'}
              </code>
              <Tooltip content={credentialPasswordVisible ? '隐藏密码' : '显示密码'} position="top">
                <Button
                  size="small"
                  theme="borderless"
                  icon={credentialPasswordVisible ? <IconEyeOpened size="small" /> : <IconEyeClosedSolid size="small" />}
                  aria-label={credentialPasswordVisible ? '隐藏密码' : '显示密码'}
                  onClick={() => setCredentialPasswordVisible((visible) => !visible)}
                />
              </Tooltip>
              <Tooltip content="复制密码" position="top">
                <Button
                  size="small"
                  theme="borderless"
                  icon={<IconCopy size="small" />}
                  aria-label="复制密码"
                  onClick={() => void copyField(vm.credential?.password || '', '密码')}
                />
              </Tooltip>
            </span>
          ) : (
            '-'
          )}
        </Row>
        <div className="qvm-info-actions">
          <div className="qvm-info-action-row">
            <span className="qvm-info-action-label">在线/离线密码重置</span>
            <Button
              size="small"
              type="warning"
              theme="light"
              disabled={!canResetPassword}
              onClick={onResetPassword}
            >
              重置密码
            </Button>
          </div>
          {!isLightweight && (
            <div className="qvm-info-action-row">
              <span className="qvm-info-action-label">重装系统</span>
              <Button
                size="small"
                type="danger"
                theme="light"
                disabled={vm.status === 'migrating'}
                onClick={onReinstall}
              >
                提交重装
              </Button>
            </div>
          )}
        </div>
      </div>

      {/* 网络与连接 */}
      <div className="qvm-info-card">
        <div className="qvm-info-card-title">网络与连接</div>
        <Row label="IP 地址">
          {ipTooltip ? (
            <Tooltip content={ipTooltip} position="top">
              <span className={`qvm-mono qvm-ip-unreachable`}>{ipDisplay}</span>
            </Tooltip>
          ) : (
            <span className="qvm-mono">{ipDisplay}</span>
          )}
        </Row>
        {interfaceIPs.length > 0 && (
          <Row label="全部 IP">
            <div className="qvm-ip-list">
              {visibleIPs.map((item) => (
                <span key={item.ip} className="qvm-ip-list-item">
                  <code className="qvm-code">{item.ip}</code>
                  {item.source && (
                    <Tag size="small" color={ipSourceTagColor(item.source)}>
                      {ipSourceLabel(item.source)}
                    </Tag>
                  )}
                </span>
              ))}
              {interfaceIPs.length > 3 && (
                <Button size="small" theme="borderless" type="primary" onClick={() => setShowAllIPs((v) => !v)}>
                  {showAllIPs ? '收起' : `显示全部 (${interfaceIPs.length})`}
                </Button>
              )}
            </div>
          </Row>
        )}
        <Row label="公网 IP">
          {vm.public_ips && vm.public_ips.length > 0 ? (
            <span className="qvm-ip-list">
              {vm.public_ips.map((item) => (
                <Tag key={item.public_ip} size="small" color="violet">
                  {item.public_ip} · {item.mode_label || item.mode}
                </Tag>
              ))}
            </span>
          ) : (
            '-'
          )}
        </Row>
        {vm.video_model !== 'none' && <Row label="VNC 端口"><span className="qvm-mono">{vm.vnc_port || '-'}</span></Row>}
        <Row label="网络接口">{vm.network || '-'}</Row>
        <Row label="显示设备">{vm.video_model || '-'}</Row>
      </div>

      {/* 高级设置 */}
      <div className="qvm-info-card">
        <div className="qvm-info-card-title">高级设置</div>
        <Row label="开机自启">
          <Tag size="small" color={vm.autostart ? 'green' : 'grey'}>{vm.autostart ? '已启用' : '已禁用'}</Tag>
        </Row>
        <Row label="启动冻结">
          {vm.freeze ? (
            <Tooltip content="已开启：启动时冻结 CPU。虚拟机启动后会先进入暂停态，可在开发者监视器执行 c 继续。" position="top">
              <Tag size="small" color="orange">已开启</Tag>
            </Tooltip>
          ) : (
            <Tag size="small" color="grey">未开启</Tag>
          )}
        </Row>
        <Row label="APIC">
          <Tag size="small" color={vm.apic ? 'green' : 'orange'}>{vm.apic ? '已启用' : '已关闭'}</Tag>
        </Row>
        <Row label="PAE">
          <Tag size="small" color={vm.pae ? 'green' : 'grey'}>{vm.pae ? '已启用' : '已关闭'}</Tag>
        </Row>
        <Row label="QEMU Guest Agent">
          {vm.guest_agent_status?.version ? (
            <Tooltip content={`版本: ${vm.guest_agent_status.version}`} position="top">
              <Tag size="small" color={guestAgent.color}>{guestAgent.text}</Tag>
            </Tooltip>
          ) : (
            <Tag size="small" color={guestAgent.color}>{guestAgent.text}</Tag>
          )}
        </Row>
        <Row label="CPU 限制">{vm.cpu_limit_percent > 0 ? `${vm.cpu_limit_percent}%` : '无限制'}</Row>
        <Row label="CPU 亲和性">{vm.cpu_affinity || '未设置'}</Row>
        <Row label="内存策略">{memoryBackendText}</Row>
      </div>

      {/* 磁盘 IOPS 限制 */}
      {!isLightweight && (
        <div className="qvm-info-card">
          <div className="qvm-info-card-title">
            磁盘 IOPS 限制
            <Button
              size="small"
              theme="borderless"
              icon={<IconRefresh size="small" spin={disksLoading} />}
              onClick={() => void loadDisks()}
            >
              刷新
            </Button>
          </div>
          {disksLoading ? (
            <div className="qvm-tab-loading"><Spin /></div>
          ) : disks.length > 0 ? (
            <div className="qvm-iops-table">
              <div className="qvm-iops-row qvm-iops-head">
                <span>设备</span>
                <span>容量</span>
                <span>总 IOPS</span>
                <span>读 IOPS</span>
                <span>写 IOPS</span>
              </div>
              {disks.map((disk) => (
                <div key={disk.device} className="qvm-iops-row">
                  <span>
                    <b>{disk.device}</b> <span className="qvm-sub-label">({disk.bus || '-'})</span>
                  </span>
                  <span>{disk.capacity_gb ? `${disk.capacity_gb} GB` : '-'}</span>
                  <IopsCell field={(disk as unknown as Record<string, unknown>).iops_total} />
                  <IopsCell field={(disk as unknown as Record<string, unknown>).iops_read} />
                  <IopsCell field={(disk as unknown as Record<string, unknown>).iops_write} />
                </div>
              ))}
            </div>
          ) : (
            <div className="qvm-empty-text">暂无磁盘数据</div>
          )}
        </div>
      )}
    </div>
  )
}

/** IOPS 单元格（{is_set, value} 结构） */
function IopsCell({ field }: { field: unknown }) {
  const f = field as { is_set?: boolean; value?: number } | undefined
  const limited = !!(f?.is_set && (f?.value ?? 0) > 0)
  return <span className={limited ? 'qvm-iops-limited' : 'qvm-sub-label'}>{limited ? f?.value : '无限制'}</span>
}
