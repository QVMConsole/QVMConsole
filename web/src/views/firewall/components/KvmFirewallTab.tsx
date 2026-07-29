/**
 * KVM 网络防火墙 Tab
 * - 仅管控 KVM 虚拟机 IPv4 转发流量，不影响宿主机与 Docker 容器网络
 * - 全局策略表单 + GeoIP 区域数据管理 + 单 VM 覆盖策略
 * - 保存/预览即时生效；应用/禁用/回滚走任务队列（高风险，428 由请求层处理）
 */
import { useMemo } from 'react'
import {
  Button,
  Col,
  Divider,
  Input,
  Radio,
  Row,
  Select,
  Table,
  Tag,
  TextArea,
} from '@douyinfe/semi-ui'
import {
  IconCheckChoiceStroked,
  IconClose,
  IconDesktop,
  IconDownload,
  IconEyeOpenedStroked,
  IconHistogram,
  IconInfoCircle,
  IconPlay,
  IconRotate,
  IconSaveStroked,
  IconSettingStroked,
  IconUpload,
} from '@douyinfe/semi-icons'
import type { ColumnProps } from '@douyinfe/semi-ui/lib/es/table'
import type {
  FirewallPolicy,
  FirewallRegion,
  FirewallStatus,
  FirewallVmOverride,
} from '@/api/firewall'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import {
  BLOCK_ACTION_OPTIONS,
  VM_OVERRIDE_MODE_OPTIONS,
  buildRegionOptions,
} from '../utils'

interface KvmFirewallTabProps {
  status: FirewallStatus | null
  policy: FirewallPolicy
  onPolicyChange: (patch: Partial<FirewallPolicy>) => void
  whitelistText: string
  onWhitelistTextChange: (text: string) => void
  geoCodesText: string
  onGeoCodesTextChange: (text: string) => void
  vmNames: string[]
  onVmOverrideChange: (name: string, patch: Partial<FirewallVmOverride>) => void
  saving: boolean
  applying: boolean
  onPreview: () => void
  onSave: () => void
  onApply: () => void
  onDisable: () => void
  onRollback: () => void
  onImport: () => void
  onGeoUpdate: () => void
}

export default function KvmFirewallTab({
  status,
  policy,
  onPolicyChange,
  whitelistText,
  onWhitelistTextChange,
  geoCodesText,
  onGeoCodesTextChange,
  vmNames,
  onVmOverrideChange,
  saving,
  applying,
  onPreview,
  onSave,
  onApply,
  onDisable,
  onRollback,
  onImport,
  onGeoUpdate,
}: KvmFirewallTabProps) {
  const regionOptions = useMemo(() => buildRegionOptions(policy.regions), [policy.regions])

  // ==================== VM 覆盖策略表格 ====================
  interface VmRow {
    name: string
  }
  const vmRows: VmRow[] = vmNames.map((name) => ({ name }))

  const vmColumns: ColumnProps<VmRow>[] = [
    {
      title: '虚拟机',
      dataIndex: 'name',
      width: 220,
      render: (text) => (
        <div className="fw-vm-name">
          <IconDesktop />
          <span>{text}</span>
        </div>
      ),
    },
    {
      title: '管控模式',
      dataIndex: 'mode',
      width: 180,
      render: (_text, row) => {
        const override = policy.vm_overrides[row.name]
        return (
          <Select
            size="small"
            value={override?.mode || 'inherit'}
            onChange={(v) => onVmOverrideChange(row.name, { mode: v as string })}
            style={{ width: 150 }}
            optionList={VM_OVERRIDE_MODE_OPTIONS.map((o) => ({ label: o.label, value: o.value }))}
          />
        )
      },
    },
    {
      title: '限制区域',
      dataIndex: 'regions',
      render: (_text, row) => {
        const override = policy.vm_overrides[row.name]
        const mode = override?.mode || 'inherit'
        const regionEnabled = mode === 'allow' || mode === 'block'
        return (
          <Select
            size="small"
            multiple
            filter
            value={override?.regions || []}
            onChange={(v) => onVmOverrideChange(row.name, { regions: v as string[] })}
            disabled={!regionEnabled}
            placeholder={regionEnabled ? '选择区域' : '当前模式无需选择区域'}
            style={{ width: '100%' }}
            optionList={regionOptions}
            maxTagCount={3}
          />
        )
      },
    },
  ]

  // ==================== 区域数据表格 ====================
  const regionColumns: ColumnProps<FirewallRegion>[] = [
    {
      title: '代码',
      dataIndex: 'code',
      width: 80,
      align: 'center',
      render: (text) => <Tag size="small">{text}</Tag>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      render: (text, row) => text || row.code,
    },
    {
      title: 'CIDR 数',
      dataIndex: 'cidrs',
      width: 90,
      align: 'center',
      render: (text) => <Tag size="small">{(text as string[] | undefined)?.length || 0}</Tag>,
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      width: 170,
      render: (text) => text || '-',
    },
  ]

  return (
    <div className="fw-tab-pane">
      {/* 操作条 */}
      <div className="fw-kvm-bar">
        <div>
          <h3>KVM 网络防火墙</h3>
          <p>仅管控 KVM 虚拟机 IPv4 转发流量，不影响宿主机和 Docker 容器网络。</p>
        </div>
        <div className="fw-kvm-bar-actions">
          <Button icon={<IconEyeOpenedStroked />} onClick={onPreview}>
            预览规则
          </Button>
          <Button
            type="primary"
            theme="light"
            icon={<IconSaveStroked />}
            loading={saving}
            onClick={onSave}
          >
            保存策略
          </Button>
          <Button
            type="primary"
            icon={<IconPlay />}
            loading={applying}
            onClick={onApply}
          >
            应用规则
          </Button>
          <Button type="warning" theme="light" icon={<IconClose />} onClick={onDisable}>
            禁用
          </Button>
          <Button type="danger" theme="light" icon={<IconRotate />} onClick={onRollback}>
            回滚
          </Button>
        </div>
      </div>

      {/* 状态横幅 */}
      <div className={`fw-banner ${status?.active ? 'enabled' : 'info'}`}>
        <div className="fw-banner-icon">
          {status?.active ? <IconCheckChoiceStroked /> : <IconInfoCircle />}
        </div>
        <div className="fw-banner-body">
          <div className="fw-banner-title">{status?.active ? '规则已生效' : '规则未应用'}</div>
          <div className="fw-banner-desc">
            {status?.active
              ? 'KVM 网络防火墙规则已在 nftables 中生效'
              : '当前仅保存或预览策略，尚未应用到 nftables'}
          </div>
        </div>
      </div>

      <Row gutter={[16, 16]}>
        {/* 全局策略 */}
        <Col xs={24} lg={14}>
          <div className="fw-card">
            <div className="fw-card-header">
              <IconSettingStroked />
              <span>全局策略</span>
            </div>
            <div className="fw-card-body">
              <div className="qvm-form-item">
                <div className="qvm-form-label">虚拟网桥</div>
                <Input
                  value={policy.bridge}
                  onChange={(v) => onPolicyChange({ bridge: v })}
                  placeholder="br-ovs"
                />
              </div>
              <div className="qvm-form-item">
                <div className="qvm-form-label">虚拟机网段</div>
                <Input
                  value={policy.vm_subnet}
                  onChange={(v) => onPolicyChange({ vm_subnet: v })}
                  placeholder="192.168.122.0/24"
                />
              </div>

              <Divider align="left" className="fw-subdivider">
                区域限制
              </Divider>
              <div className="qvm-form-item">
                <div className="qvm-form-label">出站区域限制</div>
                <TextSwitch
                  checked={policy.outbound_enabled}
                  onChange={(v) => onPolicyChange({ outbound_enabled: v })}
                  checkedText="开"
                  uncheckedText="关"
                />
              </div>
              {policy.outbound_enabled && (
                <div className="qvm-form-item">
                  <div className="qvm-form-label">允许出站区域</div>
                  <Select
                    multiple
                    filter
                    value={policy.outbound_allowed_regions}
                    onChange={(v) => onPolicyChange({ outbound_allowed_regions: v as string[] })}
                    placeholder="选择允许访问的目标区域"
                    style={{ width: '100%' }}
                    optionList={regionOptions}
                  />
                </div>
              )}
              <div className="qvm-form-item">
                <div className="qvm-form-label">入站区域限制</div>
                <TextSwitch
                  checked={policy.inbound_enabled}
                  onChange={(v) => onPolicyChange({ inbound_enabled: v })}
                  checkedText="开"
                  uncheckedText="关"
                />
              </div>
              {policy.inbound_enabled && (
                <div className="qvm-form-item">
                  <div className="qvm-form-label">允许入站区域</div>
                  <Select
                    multiple
                    filter
                    value={policy.inbound_allowed_regions}
                    onChange={(v) => onPolicyChange({ inbound_allowed_regions: v as string[] })}
                    placeholder="选择允许访问端口转发的来源区域"
                    style={{ width: '100%' }}
                    optionList={regionOptions}
                  />
                </div>
              )}

              <Divider align="left" className="fw-subdivider">
                高级设置
              </Divider>
              <div className="qvm-form-item">
                <div className="qvm-form-label">禁用 VM IPv6</div>
                <TextSwitch
                  checked={policy.disable_vm_ipv6}
                  onChange={(v) => onPolicyChange({ disable_vm_ipv6: v })}
                  checkedText="开"
                  uncheckedText="关"
                />
              </div>
              <div className="qvm-form-item">
                <div className="qvm-form-label">拦截动作</div>
                <Radio.Group
                  type="button"
                  value={policy.block_action}
                  onChange={(e) => onPolicyChange({ block_action: e.target.value as string })}
                >
                  {BLOCK_ACTION_OPTIONS.map((o) => (
                    <Radio key={o.value} value={o.value}>
                      {o.label}
                    </Radio>
                  ))}
                </Radio.Group>
              </div>
              <div className="qvm-form-item">
                <div className="qvm-form-label">白名单 CIDR</div>
                <TextArea
                  rows={5}
                  value={whitelistText}
                  onChange={onWhitelistTextChange}
                  placeholder="每行一个 IPv4 CIDR 或 IP，例如 203.0.113.10/32"
                />
              </div>
            </div>
          </div>
        </Col>

        {/* 区域数据 */}
        <Col xs={24} lg={10}>
          <div className="fw-card">
            <div className="fw-card-header">
              <IconHistogram />
              <span>区域数据</span>
              <div className="fw-card-header-actions">
                <Button size="small" icon={<IconUpload />} onClick={onImport}>
                  本地导入
                </Button>
                <Button
                  size="small"
                  type="primary"
                  theme="light"
                  icon={<IconDownload />}
                  onClick={onGeoUpdate}
                >
                  在线更新
                </Button>
              </div>
            </div>
            <div className="fw-card-body">
              <div className="qvm-form-item">
                <div className="qvm-form-label">下载源</div>
                <Input
                  value={policy.geoip_base_url}
                  onChange={(v) => onPolicyChange({ geoip_base_url: v })}
                  placeholder="https://www.ipdeny.com/ipblocks/data/aggregated"
                />
              </div>
              <div className="qvm-form-item">
                <div className="qvm-form-label">更新区域代码</div>
                <Input
                  value={geoCodesText}
                  onChange={onGeoCodesTextChange}
                  placeholder="如 cn,us,jp"
                />
              </div>
            </div>
            <Table<FirewallRegion>
              rowKey="code"
              columns={regionColumns}
              dataSource={policy.regions || []}
              pagination={false}
              size="small"
              empty="暂无区域数据"
              scroll={{ y: 240 }}
            />
            {status?.geoip_copyright && (
              <div className="fw-card-footer">
                <IconInfoCircle />
                <span>{status.geoip_copyright}</span>
              </div>
            )}
          </div>
        </Col>
      </Row>

      {/* VM 覆盖策略 */}
      <div className="fw-card" style={{ marginTop: 16 }}>
        <div className="fw-card-header">
          <IconDesktop />
          <span>VM 覆盖策略</span>
          <Tag size="small">{vmRows.length} 台</Tag>
        </div>
        <Table<VmRow>
          rowKey="name"
          columns={vmColumns}
          dataSource={vmRows}
          pagination={false}
          size="small"
          empty="暂无虚拟机"
          scroll={{ y: 360 }}
        />
      </div>
    </div>
  )
}
