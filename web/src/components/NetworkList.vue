<template>
  <div class="network-list-container">
    <el-tabs v-model="activeTab">
      <!-- 端口转发 -->
      <el-tab-pane v-if="portForwardTabVisible" label="端口转发" name="forward">
        <div class="forward-panel">
          <div class="forward-panel-main" :class="{ 'content-blurred': showPortForwardIntro }">
            <el-alert
              v-if="showForwardWhitelistBanner"
              type="warning"
              :closable="false"
              show-icon
              class="forward-whitelist-banner"
              title="注意：您当前不在白名单中，不能建站以及网页类型。系统会自动探测每个端口若发现建站则自动封禁。如果需要申请网页访问权限请联系管理员。"
            />
            <div class="forward-toolbar">
              <div class="forward-toolbar-actions">
                <el-button type="primary" size="small" @click="handleAddForward">添加转发</el-button>
                <el-button v-if="isAdmin" type="warning" plain size="small" :loading="probeSubmitting" @click="handleRunCurrentVMProbe">探测当前 VM TCP 转发</el-button>
                <el-button type="danger" plain size="small" :disabled="selectedForwardIds.length === 0" @click="handleBatchDeleteForward">批量删除</el-button>
              </div>
              <el-tag
                v-if="!isAdmin && portForwardQuotaVisible"
                size="small"
                :type="portForwardQuotaReached ? 'danger' : 'info'"
              >
                端口转发已用 {{ portForwardUsed }} / {{ portForwardLimitText }}
              </el-tag>
            </div>
            <el-table :data="currentVmForwardRules" border size="small" v-loading="forwardLoading" row-key="rule_key" @selection-change="handleForwardSelectionChange">
              <el-table-column type="selection" width="45" :selectable="forwardRowSelectable" />
              <el-table-column prop="id" label="#" width="50" />
              <el-table-column prop="protocol" label="协议" width="70" />
              <el-table-column prop="host_port" label="宿主机端口" />
              <el-table-column label="完整访问地址" min-width="180">
                <template #default="{ row }">
                  <div class="forward-access-cell">
                    <span>{{ row.access_address || '-' }}</span>
                    <el-tooltip content="复制完整访问地址" placement="top">
                      <span>
                        <el-button
                          type="primary"
                          text
                          :icon="CopyDocument"
                          :disabled="!row.access_address"
                          @click="copyForwardAccessAddress(row.access_address)"
                        />
                      </span>
                    </el-tooltip>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="130" align="center">
                <template #default="{ row }">
                  <el-tooltip :content="forwardProbeReason(row)" placement="top" :disabled="!forwardProbeReason(row)">
                    <el-tag :type="forwardProbeTagType(row)" size="small" effect="light">
                      {{ forwardProbeText(row) }}
                    </el-tag>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column prop="dest_ip" label="目标IP" />
              <el-table-column prop="dest_port" label="目标端口" />
              <el-table-column label="入站区域限制" width="130" align="center">
                <template #default="{ row }">
                  <el-switch
                    :model-value="row.region_filter_enabled"
                    size="small"
                    inline-prompt
                    active-text="开"
                    inactive-text="关"
                    :disabled="!isAdmin || !row.live"
                    @change="(val) => handleFirewallToggle(row, val)"
                  />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="140">
                <template #default="{ row }">
                  <el-button v-if="row.live" type="primary" size="small" link @click="handleEditForward(row)">编辑</el-button>
                  <el-button type="danger" size="small" @click="handleDeleteForward(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <div v-if="showPortForwardIntro" class="forward-intro-mask">
            <el-card shadow="always" class="forward-intro-card">
              <template #header>
                <div class="forward-intro-title">端口转发说明</div>
              </template>
              <div class="forward-intro-content">
                <p>端口转发用于从外网的访问流量转发到虚拟机，实现公网访问目的。</p>
                <p>开通端口转发时，系统将自动把当前虚拟机 IP 绑定为静态地址，避免转发目标在 DHCP 变化后失效。</p>
                <p>请确认您暴露到公网的服务已经完成必要的安全加固；若涉及网页访问，还会受到建站探测与白名单策略约束。</p>
                <div class="forward-intro-actions">
                  <el-button type="primary" :loading="portForwardOpening" @click="openPortForwardPanel">立即开通</el-button>
                </div>
              </div>
            </el-card>
          </div>
        </div>
      </el-tab-pane>

      <!-- 静态 IP -->
      <el-tab-pane v-if="!isLightweight && hasNatSwitch" label="静态 IP" name="staticip">
        <div style="margin-bottom: 10px;">
          <el-button type="primary" size="small" :disabled="staticIPDisabled" @click="handleBindIP">绑定 IP</el-button>
        </div>
        <el-alert
          v-if="currentSwitchIsBridge"
          type="info"
          show-icon
          :closable="false"
          title="当前 VM 接入桥接直通交换机，不使用面板 DHCP；静态 IP 请在虚拟机系统或上级路由器中配置。"
          style="margin-bottom: 10px;"
        />
        <h4>静态绑定</h4>
        <el-table :data="currentVmBindings" border size="small" v-loading="ipLoading">
          <el-table-column prop="ip" label="IP 地址" />
          <el-table-column prop="mac" label="MAC 地址" />
          <el-table-column label="操作" width="80">
            <template #default="{ row }">
              <el-button type="danger" size="small" @click="handleUnbindIP(row)">解绑</el-button>
            </template>
          </el-table-column>
        </el-table>
        <h4 style="margin-top: 16px;">DHCP 租约</h4>
        <el-table :data="currentVmDhcpLeases" border size="small">
          <el-table-column prop="hostname" label="主机名" />
          <el-table-column prop="ip" label="IP" />
          <el-table-column prop="mac" label="MAC" />
          <el-table-column prop="expiry_time" label="过期时间" />
        </el-table>
      </el-tab-pane>

      <!-- 网口管理 -->
      <el-tab-pane label="网口管理" name="interfaces">
        <div class="multi-nic-panel" v-loading="vpcLoading || multiNicLoading">
          <!-- 轻量云配额摘要 -->
          <el-descriptions v-if="isLightweight || isLightweightVM" :column="2" border size="small" class="quota-summary lightweight-summary" style="margin-bottom: 14px;">
            <el-descriptions-item label="VM 网卡实时下行">{{ lightweightQuota?.current_net_rx_rate || '0 B/s' }}</el-descriptions-item>
            <el-descriptions-item label="VM 网卡实时上行">{{ lightweightQuota?.current_net_tx_rate || '0 B/s' }}</el-descriptions-item>
            <el-descriptions-item label="本月 VM 下行">{{ lightweightQuota?.used_traffic_down_gb || '-' }} / {{ trafficQuotaText(lightweightQuota?.traffic_down_gb) }}</el-descriptions-item>
            <el-descriptions-item label="本月 VM 上行">{{ lightweightQuota?.used_traffic_up_gb || '-' }} / {{ trafficQuotaText(lightweightQuota?.traffic_up_gb) }}</el-descriptions-item>
            <el-descriptions-item label="下行带宽上限">
              {{ lightweightQuota?.is_limited_down ? '1 Mbps（已限速）' : `${lightweightQuota?.bandwidth_down_mbps || 0} Mbps` }}
            </el-descriptions-item>
            <el-descriptions-item label="上行带宽上限">
              {{ lightweightQuota?.is_limited_up ? '1 Mbps（已限速）' : `${lightweightQuota?.bandwidth_up_mbps || 0} Mbps` }}
            </el-descriptions-item>
            <el-descriptions-item label="端口转发">{{ portForwardUsed }} / {{ portForwardLimitText }}</el-descriptions-item>
            <el-descriptions-item label="限速状态">
              <el-tag :type="lightweightQuota?.is_limited_down || lightweightQuota?.is_limited_up ? 'danger' : 'success'" size="small">
                {{ lightweightLimitText }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="已用运行时长">{{ lightweightQuota?.used_runtime_display || '0秒' }}</el-descriptions-item>
            <el-descriptions-item label="剩余运行时长">{{ lightweightQuota?.max_runtime_hours > 0 ? (lightweightQuota?.remaining_runtime_display || '0秒') : '不限' }}</el-descriptions-item>
            <el-descriptions-item label="运行时长配额">{{ lightweightQuota?.max_runtime_hours > 0 ? `${lightweightQuota?.max_runtime_hours} 小时` : '不限' }}</el-descriptions-item>
            <el-descriptions-item label="运行时长状态">
              <el-tag :type="lightweightQuota?.runtime_quota_reached ? 'danger' : 'success'" size="small">
                {{ lightweightRuntimeLimitText }}
              </el-tag>
            </el-descriptions-item>
          </el-descriptions>

          <el-alert
            v-if="vpcRuntimeNotice"
            class="vpc-runtime-notice"
            type="warning"
            :title="vpcRuntimeNotice"
            show-icon
            :closable="false"
          />

          <!-- 轻量云专属安全组规则 -->
          <div v-if="(isLightweight || isLightweightVM) && !currentSwitchIsBridge" class="lightweight-security" style="margin-top: 14px;">
            <div class="vpc-static-ip-header">
              <h4>专属安全组规则</h4>
              <el-button type="primary" size="small" plain :disabled="!vpcInfo?.security_group?.id" @click="openSecurityRuleDialog">添加规则</el-button>
            </div>
            <el-alert
              class="vpc-static-ip-tip"
              type="info"
              show-icon
              :closable="false"
              title="轻量云服务器使用管理员分配的专属安全组，你可以管理当前服务器的入站/出站允许规则。"
            />
            <el-table :data="vpcInfo?.security_group?.rules || []" border size="small">
              <el-table-column prop="direction" label="方向" width="90">
                <template #default="{ row }">{{ row.direction === 'egress' ? '出站' : '入站' }}</template>
              </el-table-column>
              <el-table-column prop="protocol" label="协议" width="90" />
              <el-table-column label="端口" width="120">
                <template #default="{ row }">{{ securityRulePortText(row) }}</template>
              </el-table-column>
              <el-table-column prop="target_value" label="目标" />
              <el-table-column prop="remark" label="备注" />
              <el-table-column label="操作" width="90">
                <template #default="{ row }">
                  <el-button type="danger" size="small" link @click="deleteSecurityRule(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <!-- 网口列表 -->
          <div class="vpc-interface-list" style="margin-top: 14px;">
            <div class="vpc-static-ip-header">
              <h4>网口列表</h4>
              <div style="display: flex; gap: 8px;">
                <el-button v-if="isAdmin" type="primary" size="small" plain icon="Plus" @click="handleAddNic">添加网口</el-button>
                <el-button size="small" icon="Refresh" @click="refreshInterfaces" :loading="multiNicLoading">刷新</el-button>
              </div>
            </div>
            <el-alert
              v-if="(multiNicInterfaces.length || 0) > 1"
              class="vpc-static-ip-tip"
              type="info"
              show-icon
              :closable="false"
              title="此虚拟机配置了多个网口，每个网口可接入不同的 VPC 交换机。运行中热插拔需要虚拟机操作系统支持。"
            />
            <el-table :data="multiNicInterfaces" border size="small" v-loading="multiNicLoading">
              <el-table-column label="序号" width="60" align="center">
                <template #default="{ row }">
                  {{ row.binding?.interface_order ?? 0 }}
                  <el-tag v-if="(row.binding?.interface_order ?? 0) === 0" type="info" size="small" style="margin-left: 4px;">主</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="网卡型号" width="100">
                <template #default="{ row }">{{ row.binding?.nic_model || 'virtio' }}</template>
              </el-table-column>
              <el-table-column label="IP 地址" min-width="130">
                <template #default="{ row }">
                  <code v-if="getInterfaceIP(row)" class="ip-code">{{ getInterfaceIP(row) }}</code>
                  <span v-else style="color: var(--el-text-color-placeholder);">-</span>
                </template>
              </el-table-column>
              <el-table-column label="VPC 交换机" min-width="160">
                <template #default="{ row }">
                  <span v-if="row.switch">
                    {{ row.switch.name }}
                    <el-tag size="small" :type="row.switch.bridge_mode === 'bridge' ? 'warning' : 'info'">
                      {{ row.switch.bridge_mode === 'bridge' ? '桥接直通' : (row.switch.cidr || '-') }}
                    </el-tag>
                  </span>
                  <span v-else>-</span>
                </template>
              </el-table-column>
              <el-table-column label="安全组" min-width="150">
                <template #default="{ row }">
                  <span v-if="row.security_group">{{ row.security_group.name }}{{ row.security_group.is_default ? '（默认）' : '' }}</span>
                  <span v-else-if="row.switch?.bridge_mode === 'bridge'" class="form-hint">桥接直通不使用安全组</span>
                  <span v-else>-</span>
                </template>
              </el-table-column>
              <el-table-column label="下行速率" width="100" align="center">
                <template #default="{ row }">
                  <span v-if="(row.binding?.bandwidth_inbound_avg || 0) > 0">{{ row.binding?.bandwidth_inbound_avg || 0 }} Mbps</span>
                  <span v-else style="color: var(--el-text-color-placeholder);">未限制</span>
                </template>
              </el-table-column>
              <el-table-column label="上行速率" width="100" align="center">
                <template #default="{ row }">
                  <span v-if="(row.binding?.bandwidth_outbound_avg || 0) > 0">{{ row.binding?.bandwidth_outbound_avg || 0 }} Mbps</span>
                  <span v-else style="color: var(--el-text-color-placeholder);">未限制</span>
                </template>
              </el-table-column>
              <el-table-column v-if="isAdmin" label="操作" width="150" align="center">
                <template #default="{ row }">
                  <el-button type="primary" size="small" link @click="handleEditInterface(row)">编辑</el-button>
                  <el-popconfirm
                    title="确定要删除此网口吗？删除后虚拟机内对应网卡将不可用。"
                    placement="top"
                    @confirm="handleRemoveNic(row)"
                  >
                    <template #reference>
                      <el-button type="danger" size="small" :loading="multiNicSubmitting">删除</el-button>
                    </template>
                  </el-popconfirm>
                </template>
              </el-table-column>
            </el-table>
          </div>

        </div>
      </el-tab-pane>

      <!-- 运行状态 -->
      <el-tab-pane v-if="isAdmin" label="运行状态" name="runtime">
        <div class="runtime-toolbar">
          <el-button size="small" icon="Refresh" @click="fetchRuntimeStatus" :loading="runtimeLoading">刷新状态</el-button>
          <el-tag :type="runtimeStatus?.issues?.length ? 'warning' : 'success'" effect="plain">
            {{ runtimeStatus?.issues?.length ? '存在异常' : '状态正常' }}
          </el-tag>
        </div>
        <el-descriptions v-if="runtimeStatus" :column="2" border size="small" class="runtime-summary">
          <el-descriptions-item label="虚拟机">{{ runtimeStatus.vm_name }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ runtimeStatus.state || '-' }}</el-descriptions-item>
          <el-descriptions-item label="OVS 网桥">{{ runtimeStatus.bridge || '-' }}</el-descriptions-item>
          <el-descriptions-item label="限速状态">
            <el-tag :type="runtimeStatus.bandwidth?.enabled ? 'success' : 'info'" size="small">
              {{ runtimeStatus.bandwidth?.enabled ? '已配置' : '未配置' }}
            </el-tag>
          </el-descriptions-item>
        </el-descriptions>
        <el-alert
          v-if="runtimeStatus?.issues?.length"
          class="runtime-alert"
          type="warning"
          :closable="false"
          :title="runtimeStatus.issues.join('；')"
        />
        <el-table :data="runtimeStatus?.interfaces || []" border size="small" v-loading="runtimeLoading">
          <el-table-column prop="target" label="接口" width="90" />
          <el-table-column prop="source_bridge" label="网桥" width="110" />
          <el-table-column prop="virtualport_type" label="VirtualPort" width="120" />
          <el-table-column prop="ofport" label="ofport" width="80" />
          <el-table-column prop="model" label="模型" width="90" />
          <el-table-column prop="mac" label="MAC" min-width="150" />
          <el-table-column prop="ip" label="IP" min-width="130">
            <template #default="{ row }">{{ row.ip || '-' }}</template>
          </el-table-column>
          <el-table-column prop="ip_source" label="IP 来源" width="120">
            <template #default="{ row }">{{ ipSourceText(row.ip_source) }}</template>
          </el-table-column>
          <el-table-column label="异常" min-width="180">
            <template #default="{ row }">{{ row.issues?.length ? row.issues.join('；') : '-' }}</template>
          </el-table-column>
        </el-table>
        <el-descriptions v-if="runtimeStatus?.bandwidth" :column="3" border size="small" class="bandwidth-status">
          <el-descriptions-item label="Cookie">{{ runtimeStatus.bandwidth.cookie || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Flow">{{ runtimeStatus.bandwidth.flow_exists ? '存在' : '不存在' }}</el-descriptions-item>
          <el-descriptions-item label="检查端口">{{ runtimeStatus.bandwidth.checked_port || '-' }}</el-descriptions-item>
          <el-descriptions-item label="下行 QoS">{{ runtimeStatus.bandwidth.down_qos ? '存在' : '不存在' }}</el-descriptions-item>
          <el-descriptions-item label="上行 Bridge QoS">{{ runtimeStatus.bandwidth.bridge_qos ? '存在' : '不存在' }}</el-descriptions-item>
          <el-descriptions-item label="Queue">{{ queueStatusText(runtimeStatus.bandwidth) }}</el-descriptions-item>
          <el-descriptions-item label="网卡下行 tc">{{ runtimeStatus.bandwidth.tc_root ? '存在' : '不存在' }}</el-descriptions-item>
          <el-descriptions-item label="网卡上行 tc">{{ runtimeStatus.bandwidth.tc_upload_police ? '存在' : '不存在' }}</el-descriptions-item>
          <el-descriptions-item label="网卡 ingress">{{ runtimeStatus.bandwidth.tc_ingress ? '存在' : '不存在' }}</el-descriptions-item>
        </el-descriptions>
      </el-tab-pane>

      <!-- 网络诊断 -->
      <el-tab-pane v-if="isAdmin" label="网络诊断" name="diagnostics">
        <div class="diagnostics-panel" v-loading="diagnosticsLoading">
          <div class="diagnostics-toolbar">
            <el-button size="small" icon="Refresh" @click="fetchNetworkDiagnostics">刷新诊断</el-button>
            <el-tag :type="networkDiagnostics?.issues?.length ? 'warning' : 'success'" effect="plain">
              {{ networkDiagnostics?.issues?.length ? '需要关注' : '可抓包' }}
            </el-tag>
          </div>
          <el-alert
            v-if="networkDiagnostics?.issues?.length"
            class="diagnostics-alert"
            type="warning"
            :closable="false"
            show-icon
            :title="networkDiagnostics.issues.join('；')"
          />
          <el-descriptions v-if="networkDiagnostics" :column="2" border size="small" class="diagnostics-summary">
            <el-descriptions-item label="默认接口">{{ networkDiagnostics.default_interface || '-' }}</el-descriptions-item>
            <el-descriptions-item label="默认 IP">{{ networkDiagnostics.default_ip || '-' }}</el-descriptions-item>
            <el-descriptions-item label="状态">{{ networkDiagnostics.state || '-' }}</el-descriptions-item>
            <el-descriptions-item label="端口转发">{{ networkDiagnostics.port_forwards?.length || 0 }} 条</el-descriptions-item>
          </el-descriptions>

          <el-form label-width="120px" class="capture-form">
            <el-row :gutter="16">
              <el-col :xs="24" :sm="12">
                <el-form-item label="抓包接口">
                  <el-select v-model="captureForm.interface_name" style="width: 100%;" placeholder="选择运行态接口">
                    <el-option
                      v-for="item in captureInterfaces"
                      :key="item.target"
                      :label="`${item.target} / ${item.ip || '无 IP'} / ofport ${item.ofport || '-'}`"
                      :value="item.target"
                    />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="12">
                <el-form-item label="协议模板">
                  <el-select v-model="captureForm.filter.protocol" style="width: 100%;">
                    <el-option label="全部" value="any" />
                    <el-option label="TCP" value="tcp" />
                    <el-option label="UDP" value="udp" />
                    <el-option label="ICMP" value="icmp" />
                    <el-option label="ARP" value="arp" />
                    <el-option label="DHCP" value="dhcp" />
                    <el-option label="DNS" value="dns" />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="12">
                <el-form-item label="源 IP">
                  <el-input v-model="captureForm.filter.source_ip" placeholder="可留空" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="12">
                <el-form-item label="目标 IP">
                  <el-input v-model="captureForm.filter.dest_ip" placeholder="可留空" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="8">
                <el-form-item label="任意端口">
                  <el-input-number v-model="captureForm.filter.port" :min="0" :max="65535" style="width: 100%;" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="8">
                <el-form-item label="源端口">
                  <el-input-number v-model="captureForm.filter.source_port" :min="0" :max="65535" style="width: 100%;" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="8">
                <el-form-item label="目标端口">
                  <el-input-number v-model="captureForm.filter.dest_port" :min="0" :max="65535" style="width: 100%;" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="8">
                <el-form-item label="时长(秒)">
                  <el-input-number v-model="captureForm.duration_seconds" :min="1" :max="120" style="width: 100%;" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="8">
                <el-form-item label="大小(MB)">
                  <el-input-number v-model="captureForm.max_mb" :min="1" :max="256" style="width: 100%;" />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :sm="8">
                <el-form-item label="包数">
                  <el-input-number v-model="captureForm.max_packets" :min="1" :max="100000" style="width: 100%;" />
                </el-form-item>
              </el-col>
            </el-row>
          </el-form>

          <div class="diagnostic-templates">
            <el-button
              v-for="item in networkDiagnostics?.templates || []"
              :key="item.key"
              size="small"
              plain
              @click="applyCaptureTemplate(item)"
            >
              {{ item.name }}
            </el-button>
          </div>

          <div class="capture-actions">
            <el-button type="primary" icon="VideoPlay" :loading="captureSubmitting" :disabled="!captureForm.interface_name" @click="submitNetworkCapture">开始抓包</el-button>
            <el-button v-if="captureTaskId && captureSession?.status === 'running'" type="warning" plain icon="Close" @click="cancelNetworkCapture">取消抓包</el-button>
            <el-button v-if="captureSession?.download_path && captureSession?.file_size > 0" type="success" plain icon="Download" @click="downloadNetworkCapture">下载 pcap</el-button>
            <el-button v-if="captureSession?.download_path && captureSession?.file_size > 0" type="danger" plain icon="Delete" @click="deleteNetworkCaptureFile">删除 pcap</el-button>
          </div>

          <el-descriptions v-if="captureSession" :column="3" border size="small" class="capture-session">
            <el-descriptions-item label="任务 ID">{{ captureSession.task_id }}</el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="captureStatusType(captureSession.status)" size="small">{{ captureStatusText(captureSession.status) }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="文件大小">{{ formatBytes(captureSession.file_size) }}</el-descriptions-item>
            <el-descriptions-item label="接口">{{ captureSession.interface_name || '-' }}</el-descriptions-item>
            <el-descriptions-item label="BPF">{{ captureSession.bpf || '全部流量' }}</el-descriptions-item>
            <el-descriptions-item label="消息">{{ captureSession.message || '-' }}</el-descriptions-item>
          </el-descriptions>

          <div v-if="captureSession?.summary_lines?.length" class="capture-output">
            <div class="capture-output-title">实时摘要</div>
            <pre>{{ captureSession.summary_lines.join('\n') }}</pre>
          </div>

          <div v-if="networkDiagnostics?.neighbors?.length" class="neighbors-panel">
            <div class="capture-output-title">邻居表</div>
            <pre>{{ networkDiagnostics.neighbors.join('\n') }}</pre>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 添加端口转发对话框 -->
    <el-dialog title="添加端口转发" v-model="addForwardVisible" width="400px" append-to-body>
      <el-form :model="forwardForm" label-width="100px">
        <el-form-item label="目标 IP">
          <el-select
            v-model="forwardForm.vm_ip"
            style="width: 100%;"
            placeholder="选择或输入目标 IP"
            filterable
            allow-create
            default-first-option
          >
            <el-option
              v-for="item in vmIPOptions"
              :key="item.ip"
              :label="`${item.ip}（${item.source}）`"
              :value="item.ip"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="宿主机端口">
          <el-input v-model="forwardForm.host_port" placeholder="留空自动分配" />
        </el-form-item>
        <el-form-item label="虚拟机端口">
          <el-input v-model="forwardForm.vm_port" placeholder="如 22" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="forwardForm.protocol">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
            <el-option label="TCP+UDP" value="both" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addForwardVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAddForward" :loading="forwardSubmitting">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog title="编辑端口转发" v-model="editForwardVisible" width="400px" append-to-body>
      <el-form :model="editForwardForm" label-width="100px">
        <el-form-item label="目标 IP">
          <el-select
            v-model="editForwardForm.vm_ip"
            style="width: 100%;"
            placeholder="选择或输入目标 IP"
            filterable
            allow-create
            default-first-option
          >
            <el-option
              v-for="item in vmIPOptions"
              :key="item.ip"
              :label="`${item.ip} (${item.source})`"
              :value="item.ip"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="宿主机端口">
          <el-input v-model="editForwardForm.host_port" placeholder="如 10022" />
        </el-form-item>
        <el-form-item label="虚拟机端口">
          <el-input v-model="editForwardForm.vm_port" placeholder="如 22" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="editForwardForm.protocol">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editForwardVisible = false">取消</el-button>
        <el-button type="primary" @click="submitEditForward" :loading="forwardSubmitting">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog title="添加安全组规则" v-model="securityRuleVisible" width="460px" append-to-body>
      <el-form :model="securityRuleForm" label-width="100px">
        <el-form-item label="方向">
          <el-select v-model="securityRuleForm.direction" style="width: 100%;">
            <el-option label="入站" value="ingress" />
            <el-option label="出站" value="egress" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="securityRuleForm.protocol" style="width: 100%;">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
            <el-option label="ICMP" value="icmp" />
            <el-option label="全部" value="all" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="securityRuleForm.protocol === 'tcp' || securityRuleForm.protocol === 'udp'" label="端口">
          <div style="display: flex; align-items: center; gap: 12px; width: 100%;">
            <el-input v-model="securityRuleForm.port_text" :disabled="securityRuleForm.port_all" placeholder="例如 80 或 80-90" style="flex: 1;" />
            <el-checkbox v-model="securityRuleForm.port_all">全端口</el-checkbox>
          </div>
        </el-form-item>
        <el-form-item label="CIDR">
          <el-input v-model="securityRuleForm.target_value" placeholder="例如 0.0.0.0/0 或 1.2.3.4/32" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="securityRuleForm.remark" maxlength="80" show-word-limit />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="securityRuleVisible = false">取消</el-button>
        <el-button type="primary" :loading="securityRuleSubmitting" @click="submitSecurityRule">确定</el-button>
      </template>
    </el-dialog>

    <!-- 绑定静态 IP 对话框 -->
    <el-dialog title="绑定静态 IP" v-model="bindIPVisible" width="400px" append-to-body>
      <el-form :model="bindForm" label-width="100px">
        <el-form-item label="DHCP 租约">
          <el-select v-model="selectedLeaseIP" style="width: 100%;" placeholder="选择要绑定的租约" @change="onLeaseSelected">
            <el-option
              v-for="item in currentVmDhcpLeases"
              :key="item.ip"
              :label="`${item.ip}（${item.hostname || item.vm_name}）`"
              :value="item.ip"
            />
            <el-option label="自动分配 IP" value="" />
          </el-select>
        </el-form-item>
        <el-form-item label="IP 地址" v-if="selectedLeaseIP === ''">
          <el-input v-model="bindForm.ip" placeholder="留空自动分配，或输入完整IP/最后一位数字" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindIPVisible = false" :disabled="bindIPSubmitting">取消</el-button>
        <el-button type="primary" @click="submitBindIP" :loading="bindIPSubmitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 添加网口对话框（仅管理员） -->
    <el-dialog title="添加网口" v-model="addNicVisible" width="500px" append-to-body>
      <el-form :model="addNicForm" label-width="110px">
        <el-form-item label="网卡型号">
          <el-select v-model="addNicForm.nic_model" style="width: 100%;">
            <el-option label="VirtIO（推荐）" value="virtio" />
            <el-option label="e1000e (Intel)" value="e1000e" />
            <el-option label="rtl8139" value="rtl8139" />
          </el-select>
        </el-form-item>
        <el-form-item label="VPC 交换机">
          <el-select v-model="addNicForm.switch_id" placeholder="选择交换机" style="width: 100%;" filterable>
            <el-option
              v-for="item in vpcSwitches"
              :key="item.id"
              :label="switchOptionLabel(item)"
              :value="item.id"
            >
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span>{{ item.name }}</span>
                <el-tag size="small" :type="item.bridge_mode === 'bridge' ? 'warning' : 'info'">
                  {{ item.bridge_mode === 'bridge' ? `${item.bridge_name || '桥接'}${item.bridge_vlan_id > 0 ? ' / VLAN ' + item.bridge_vlan_id : ''}` : item.cidr }}
                </el-tag>
              </div>
            </el-option>
          </el-select>
          <div v-if="addNicSelectedSwitch?.bridge_mode === 'bridge'" class="form-hint">
            桥接直通由上级路由器分配 IP，不使用内部 DHCP 和安全组
          </div>
        </el-form-item>
        <el-form-item v-if="addNicSelectedSwitch?.bridge_mode !== 'bridge'" label="安全组">
          <el-select v-model="addNicForm.security_group_id" placeholder="选择安全组（可选）" style="width: 100%;" filterable>
            <el-option
              v-for="item in addNicSecurityGroups"
              :key="item.id"
              :label="item.is_default ? `${item.name}（默认）` : item.name"
              :value="item.id"
            />
          </el-select>
          <div class="form-hint">不选则使用该交换机用户默认安全组</div>
        </el-form-item>
        <el-divider content-position="left" style="margin: 12px 0;">速率限制</el-divider>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="下行速率 (Mbps)">
              <el-input-number v-model="addNicForm.bandwidth_inbound_avg" :min="0" :max="100000" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="上行速率 (Mbps)">
              <el-input-number v-model="addNicForm.bandwidth_outbound_avg" :min="0" :max="100000" style="width: 100%;" />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="form-hint">0 表示不限制，设置后通过 libvirt domiftune 对该网口生效</div>
      </el-form>
      <template #footer>
        <el-button @click="addNicVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAddNic" :loading="multiNicSubmitting">确定添加</el-button>
      </template>
    </el-dialog>

    <!-- 编辑网口对话框（仅管理员） -->
    <el-dialog title="编辑网口" v-model="editNicVisible" width="500px" append-to-body>
      <el-form :model="editNicForm" label-width="110px">
        <el-form-item label="网卡型号">
          <el-select v-model="editNicForm.nic_model" style="width: 100%;">
            <el-option label="VirtIO（推荐）" value="virtio" />
            <el-option label="e1000e (Intel)" value="e1000e" />
            <el-option label="rtl8139" value="rtl8139" />
          </el-select>
        </el-form-item>
        <el-form-item label="VPC 交换机">
          <el-select v-model="editNicForm.switch_id" placeholder="选择交换机" style="width: 100%;" filterable @change="onEditNicSwitchChange">
            <el-option
              v-for="item in vpcSwitches"
              :key="item.id"
              :label="switchOptionLabel(item)"
              :value="item.id"
            >
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span>{{ item.name }}</span>
                <el-tag size="small" :type="item.bridge_mode === 'bridge' ? 'warning' : 'info'">
                  {{ item.bridge_mode === 'bridge' ? `${item.bridge_name || '桥接'}${item.bridge_vlan_id > 0 ? ' / VLAN ' + item.bridge_vlan_id : ''}` : item.cidr }}
                </el-tag>
              </div>
            </el-option>
          </el-select>
          <div v-if="editNicSelectedSwitch?.bridge_mode === 'bridge'" class="form-hint">
            桥接直通由上级路由器分配 IP，不使用内部 DHCP 和安全组
          </div>
        </el-form-item>
        <el-form-item v-if="editNicSelectedSwitch?.bridge_mode !== 'bridge'" label="安全组">
          <el-select v-model="editNicForm.security_group_id" placeholder="选择安全组（可选）" style="width: 100%;" filterable>
            <el-option
              v-for="item in editNicSecurityGroups"
              :key="item.id"
              :label="item.is_default ? `${item.name}（默认）` : item.name"
              :value="item.id"
            />
          </el-select>
          <div class="form-hint">不选则使用该交换机用户默认安全组</div>
        </el-form-item>
        <el-divider content-position="left" style="margin: 12px 0;">速率限制</el-divider>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="下行速率 (Mbps)">
              <el-input-number v-model="editNicForm.bandwidth_inbound_avg" :min="0" :max="100000" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="上行速率 (Mbps)">
              <el-input-number v-model="editNicForm.bandwidth_outbound_avg" :min="0" :max="100000" style="width: 100%;" />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="form-hint">0 表示不限制，设置后通过 libvirt domiftune 对该网口生效</div>
      </el-form>
      <template #footer>
        <el-button @click="editNicVisible = false">取消</el-button>
        <el-button type="primary" @click="submitEditNic" :loading="multiNicSubmitting">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import {
  getStaticIPList,
  bindStaticIP,
  unbindStaticIP,
  getPortForwardList,
  addPortForward,
  updatePortForward,
  deletePortForward,
  deletePortForwardByRuleKey,
  batchDeletePortForward,
  getPortForwardIPs,
  setPortForwardFirewall,
  getNetworkCaptureSession,
  getNetworkCaptureDownloadUrl,
  deleteNetworkCapture,
  runPortForwardHTTPProbe,
  getPortForwardWhitelistSummary
} from '@/api/network'
import { getVMNetworkStatus, getVMNetworkDiagnostics, startVMNetworkCapture } from '@/api/vm'
import {
  addVPCSecurityGroupRule,
  bindVMVPC,
  deleteVPCSecurityGroupRule,
  getVPCSecurityGroups,
  getVPCSwitches,
  getVMVPCBinding,
  switchVMSecurityGroup,
  listVMInterfaces,
  addVMInterface,
  removeVMInterface,
  updateVMInterface
} from '@/api/vpc'
import { getSelfQuota } from '@/api/user'
import { cancelTask } from '@/api/task'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import { useUserStore } from '@/store/user'
import { copyTextWithFallback } from '@/utils/clipboard'

const props = defineProps({
  vmName: {
    type: String,
    required: true
  }
})

const activeTab = ref('forward')
const userStore = useUserStore()
const vpcInfo = ref(null)
const isAdmin = computed(() => userStore.role === 'admin')
const isLightweight = computed(() => userStore.role !== 'admin' && userStore.cloudType === 'lightweight')
const isLightweightVM = computed(() => !!vpcInfo.value?.lightweight_quota)
const forwardBannedReason = '检测到存在建站或HTTP访问且未报备，当前转发已封禁，请联系管理员'
const showPortForwardIntro = ref(false)

// 端口转发
const forwardRules = ref([])
const forwardLoading = ref(false)
const forwardSubmitting = ref(false)
const portForwardOpening = ref(false)
const addForwardVisible = ref(false)
const editForwardVisible = ref(false)
const forwardForm = reactive({ vm_name: '', vm_ip: '', host_port: '', vm_port: '', protocol: 'tcp' })
const editForwardForm = reactive({ id: null, vm_name: '', vm_ip: '', host_port: '', vm_port: '', protocol: 'tcp' })
const selectedForwardIds = ref([])
const probeSubmitting = ref(false)
// 记录手动输入的 IP（从数据库持久化加载）
const manualIPs = ref([])
const whitelistSummary = ref(null)

// 静态 IP
const staticBindings = ref([])
const dhcpLeases = ref([])
const ipLoading = ref(false)
const bindIPVisible = ref(false)
const bindIPSubmitting = ref(false)
const bindForm = reactive({ vm_name: '', ip: '' })
const selectedLeaseIP = ref('')
const runtimeStatus = ref(null)
const runtimeLoading = ref(false)
const selfQuota = ref(null)
const vpcSwitches = ref([])
const vpcSecurityGroups = ref([])
const vpcLoading = ref(false)
const vpcSubmitting = ref(false)
const vpcForm = reactive({
  switch_id: null,
  security_group_id: null
})
const securityRuleVisible = ref(false)
const securityRuleSubmitting = ref(false)
// 多网口管理
const multiNicInterfaces = ref([])
const multiNicLoading = ref(false)
const multiNicSubmitting = ref(false)
const addNicVisible = ref(false)
const addNicForm = reactive({
  nic_model: 'virtio',
  switch_id: null,
  security_group_id: null,
  bandwidth_inbound_avg: 0,
  bandwidth_outbound_avg: 0
})
// 编辑网口对话框
const editNicVisible = ref(false)
const editNicEditingOrder = ref(-1)
const editNicForm = reactive({
  nic_model: 'virtio',
  switch_id: null,
  security_group_id: null,
  bandwidth_inbound_avg: 0,
  bandwidth_outbound_avg: 0
})
const editNicSelectedSwitch = computed(() => vpcSwitches.value.find(item => item.id === editNicForm.switch_id) || null)
const editNicSecurityGroups = computed(() => {
  if (!editNicSelectedSwitch.value?.username) return vpcSecurityGroups.value
  return vpcSecurityGroups.value.filter(item => item.username === editNicSelectedSwitch.value.username)
})
const addNicSelectedSwitch = computed(() => vpcSwitches.value.find(item => item.id === addNicForm.switch_id) || null)
const addNicSecurityGroups = computed(() => {
  if (!addNicSelectedSwitch.value?.username) return vpcSecurityGroups.value
  return vpcSecurityGroups.value.filter(item => item.username === addNicSelectedSwitch.value.username)
})

const securityRuleForm = reactive({
  direction: 'ingress',
  protocol: 'tcp',
  port_start: 22,
  port_end: 22,
  port_text: '22',
  port_all: false,
  target_type: 'cidr',
  target_value: '0.0.0.0/0',
  remark: ''
})
const networkDiagnostics = ref(null)
const diagnosticsLoading = ref(false)
const captureSubmitting = ref(false)
const captureTaskId = ref(null)
const captureSession = ref(null)
let capturePollTimer = null
const captureForm = reactive({
  interface_name: '',
  filter: {
    protocol: 'any',
    source_ip: '',
    dest_ip: '',
    port: 0,
    source_port: 0,
    dest_port: 0
  },
  duration_seconds: 30,
  max_mb: 64,
  max_packets: 5000
})

const selectedVPCSwitch = computed(() => {
  return vpcSwitches.value.find(item => item.id === vpcForm.switch_id) || null
})

const selectedVPCSwitchIsBridge = computed(() => selectedVPCSwitch.value?.bridge_mode === 'bridge')
const currentSwitchIsBridge = computed(() => vpcInfo.value?.switch?.bridge_mode === 'bridge')
// 网口列表中是否存在 NAT 交换机：列表非空时以列表为准，列表为空时回退到 VPC 交换机判断
const hasNatSwitch = computed(() => {
  const interfaces = multiNicInterfaces.value
  if (interfaces.length > 0) {
    return interfaces.some(item => item.switch?.bridge_mode !== 'bridge')
  }
  return !currentSwitchIsBridge.value
})
const lightweightQuota = computed(() => vpcInfo.value?.lightweight_quota || null)
const portForwardTabVisible = computed(() => hasNatSwitch.value && (isAdmin.value || isLightweight.value || isLightweightVM.value || !!selfQuota.value?.enable_port_forward))
const portForwardQuotaVisible = computed(() => isLightweight.value || isLightweightVM.value || !!selfQuota.value)
const portForwardUsed = computed(() => (isLightweight.value || isLightweightVM.value) ? (lightweightQuota.value?.used_port_forwards || 0) : (selfQuota.value?.used_port_forwards || 0))
const portForwardLimit = computed(() => (isLightweight.value || isLightweightVM.value) ? (lightweightQuota.value?.max_port_forwards || 0) : (selfQuota.value?.max_port_forwards || 0))
const portForwardLimitText = computed(() => portForwardLimit.value > 0 ? portForwardLimit.value : '不限')
const portForwardQuotaReached = computed(() => {
  if (isAdmin.value) return false
  return portForwardLimit.value > 0 && portForwardUsed.value >= portForwardLimit.value
})
const lightweightLimitText = computed(() => {
  const parts = []
  if (lightweightQuota.value?.is_limited_down) parts.push('下行已限速')
  if (lightweightQuota.value?.is_limited_up) parts.push('上行已限速')
  return parts.length ? parts.join('，') : '未限速'
})
const lightweightRuntimeLimitText = computed(() => {
  if (lightweightQuota.value?.runtime_quota_reached) return '已耗尽'
  if ((lightweightQuota.value?.max_runtime_hours || 0) > 0) return '正常'
  return '不限'
})
const staticIPDisabled = computed(() => currentSwitchIsBridge.value || selectedVPCSwitchIsBridge.value)
const vpcRuntimeNotice = computed(() => {
  if (runtimeStatus.value?.state !== 'running') return ''
  if (selectedVPCSwitchIsBridge.value) {
    return '运行中的虚拟机不能直接切换到桥接直通交换机，请先关闭虚拟机后再保存绑定。'
  }
  return '运行中的虚拟机切换 VPC 后，宿主机侧会立即更新 OVS VLAN 和 ACL，但虚拟机内部仍可能保留旧 DHCP 地址；请在虚拟机内重新获取 DHCP，或重启虚拟机后再查看新 IP。'
})
const currentSwitchNetworkText = computed(() => {
  const sw = vpcInfo.value?.switch
  if (!sw) return '-'
  if (sw.bridge_mode === 'bridge') return `${sw.bridge_name || '桥接网桥'} / 上级路由分配${sw.bridge_vlan_id > 0 ? ' / VLAN ' + sw.bridge_vlan_id : ''}`
  return sw.cidr || '-'
})

const filteredVPCSecurityGroups = computed(() => {
  if (!isAdmin.value || !selectedVPCSwitch.value?.username) {
    return vpcSecurityGroups.value
  }
  return vpcSecurityGroups.value.filter(item => item.username === selectedVPCSwitch.value.username)
})

const captureInterfaces = computed(() => {
  const interfaces = networkDiagnostics.value?.interfaces || runtimeStatus.value?.interfaces || []
  return interfaces.filter(item => item.target && item.target !== '-' && item.ofport && item.ofport !== '-1')
})

// 只显示当前虚拟机的静态绑定
const currentVmBindings = computed(() => {
  return staticBindings.value.filter(b => b.vm_name === props.vmName)
})

// 只显示当前虚拟机的 DHCP 租约，并过滤掉已有静态绑定的
const currentVmDhcpLeases = computed(() => {
  const staticIPs = new Set(currentVmBindings.value.map(b => b.ip))
  const staticMACs = new Set(currentVmBindings.value.map(b => b.mac))
  return dhcpLeases.value.filter(lease =>
    lease.vm_name === props.vmName &&
    !staticIPs.has(lease.ip) &&
    !staticMACs.has(lease.mac)
  )
})

// 端口转发 IP 下拉：当前虚拟机的静态绑定 + DHCP 租约 IP + 手动输入的 IP（按 IP 去重）
const vmIPOptions = computed(() => {
  const options = []
  const seen = new Set()

  // 静态绑定
  for (const b of staticBindings.value) {
    if (b.vm_name === props.vmName && !seen.has(b.ip)) {
      options.push({ ip: b.ip, source: '静态绑定' })
      seen.add(b.ip)
    }
  }

  // DHCP 租约
  for (const lease of dhcpLeases.value) {
    if (lease.vm_name === props.vmName && !seen.has(lease.ip)) {
      options.push({ ip: lease.ip, source: 'DHCP' })
      seen.add(lease.ip)
    }
  }

  // 手动输入的 IP（从数据库加载）
  for (const item of manualIPs.value) {
    if (!seen.has(item.ip)) {
      options.push({ ip: item.ip, source: '手动输入' })
      seen.add(item.ip)
    }
  }

  return options
})

// 只显示当前虚拟机的端口转发规则（根据目标 IP 匹配，包含手动输入的 IP）
const currentVmForwardRules = computed(() => {
  // 收集当前虚拟机的所有 IP（静态绑定 + DHCP 租约 + 手动输入）
  const vmIPs = new Set()
  for (const b of staticBindings.value) {
    if (b.vm_name === props.vmName) {
      vmIPs.add(b.ip)
    }
  }
  for (const lease of dhcpLeases.value) {
    if (lease.vm_name === props.vmName) {
      vmIPs.add(lease.ip)
    }
  }
  // 加入手动输入的 IP（从数据库加载）
  for (const item of manualIPs.value) {
    vmIPs.add(item.ip)
  }
  // 过滤出目标 IP 属于当前虚拟机的转发规则
  return forwardRules.value.filter(rule => vmIPs.has(rule.dest_ip))
})

const showForwardWhitelistBanner = computed(() => {
  if (isAdmin.value) return false
  if (!whitelistSummary.value) return false
  return !whitelistSummary.value.effective_whitelisted
})

const portForwardIntroStorageKey = computed(() => {
  const username = userStore.username || 'default'
  const vmName = props.vmName || 'default'
  return `vm-port-forward-intro-seen:${username}:${vmName}`
})

const resolveDefaultTab = () => {
  if (portForwardTabVisible.value) return 'forward'
  if (isLightweight.value || isLightweightVM.value) return 'interfaces'
  if (hasNatSwitch.value) return 'staticip'
  return 'interfaces'
}

const initPortForwardIntro = () => {
  if (!portForwardTabVisible.value) {
    showPortForwardIntro.value = false
    return
  }
  if (currentVmBindings.value.length > 0) {
    showPortForwardIntro.value = false
    localStorage.setItem(portForwardIntroStorageKey.value, '1')
    return
  }
  showPortForwardIntro.value = localStorage.getItem(portForwardIntroStorageKey.value) !== '1'
}

const currentVMPreferredIP = () => {
  const staticIP = staticBindings.value.find(item => item.vm_name === props.vmName)?.ip
  if (staticIP) return staticIP
  const runtimeIP = runtimeStatus.value?.interfaces?.find(item => item.ip)?.ip
  if (runtimeIP) return runtimeIP
  const dhcpIP = currentVmDhcpLeases.value[0]?.ip
  return dhcpIP || ''
}

const openPortForwardPanel = async () => {
  if (staticIPDisabled.value) {
    showPortForwardIntro.value = false
    localStorage.setItem(portForwardIntroStorageKey.value, '1')
    return
  }
  portForwardOpening.value = true
  try {
    const targetIP = currentVMPreferredIP()
    const res = await bindStaticIP({ vm_name: props.vmName, ip: targetIP })
    ElMessage.success(res.message || '端口转发已开通，已固定当前 VM IP')
    showPortForwardIntro.value = false
    localStorage.setItem(portForwardIntroStorageKey.value, '1')
    await Promise.all([fetchStaticIPs(), fetchRuntimeStatus(), fetchVPCBinding()])
  } catch (err) {
    console.error(err)
  } finally {
    portForwardOpening.value = false
  }
}

const fetchData = async () => {
  if (!props.vmName) return
  forwardLoading.value = true
  await Promise.all([fetchForwardRules(), fetchStaticIPs(), fetchManualIPs(), fetchRuntimeStatus(), fetchSelfQuota(), fetchVPCBinding(), fetchWhitelistSummary()])
  initPortForwardIntro()
  forwardLoading.value = false
}

const fetchForwardRules = async () => {
  try {
    const res = await getPortForwardList()
    forwardRules.value = res.data || []
    selectedForwardIds.value = selectedForwardIds.value.filter(id => forwardRules.value.some(rule => rule.id === id))
  } catch (err) {
    console.error(err)
  }
}

// 加载手动 IP 映射（从数据库）
const fetchManualIPs = async () => {
  try {
    const res = await getPortForwardIPs(props.vmName)
    manualIPs.value = res.data || []
  } catch (err) {
    console.error(err)
  }
}

const fetchStaticIPs = async () => {
  ipLoading.value = true
  try {
    const res = await getStaticIPList()
    staticBindings.value = res.data?.static_bindings || []
    dhcpLeases.value = res.data?.dhcp_leases || []
  } catch (err) {
    console.error(err)
  } finally {
    ipLoading.value = false
  }
}

const fetchRuntimeStatus = async () => {
  if (!props.vmName) return
  runtimeLoading.value = true
  try {
    const res = await getVMNetworkStatus(props.vmName)
    runtimeStatus.value = res.data
  } catch (err) {
    console.error(err)
  } finally {
    runtimeLoading.value = false
  }
}

const fetchNetworkDiagnostics = async () => {
  if (!props.vmName || !isAdmin.value) return
  diagnosticsLoading.value = true
  try {
    const res = await getVMNetworkDiagnostics(props.vmName)
    networkDiagnostics.value = res.data
    if (!captureForm.interface_name && res.data?.default_interface) {
      captureForm.interface_name = res.data.default_interface
    }
  } catch (err) {
    console.error(err)
  } finally {
    diagnosticsLoading.value = false
  }
}

const resetCaptureFilter = () => {
  Object.assign(captureForm.filter, {
    protocol: 'any',
    source_ip: '',
    dest_ip: '',
    port: 0,
    source_port: 0,
    dest_port: 0
  })
}

const applyCaptureTemplate = (template) => {
  resetCaptureFilter()
  Object.assign(captureForm.filter, {
    protocol: template.filter?.protocol || 'any',
    source_ip: template.filter?.source_ip || '',
    dest_ip: template.filter?.dest_ip || '',
    port: template.filter?.port || 0,
    source_port: template.filter?.source_port || 0,
    dest_port: template.filter?.dest_port || 0
  })
}

const submitNetworkCapture = async () => {
  if (!captureForm.interface_name) {
    ElMessage.warning('请选择抓包接口')
    return
  }
  try {
    await ElMessageBox.confirm('抓包会临时读取该 VM 的网络流量并生成 pcap 文件，确认继续？', '高风险操作', {
      type: 'warning',
      confirmButtonText: '开始抓包',
      cancelButtonText: '取消'
    })
    captureSubmitting.value = true
    const res = await startVMNetworkCapture(props.vmName, {
      interface_name: captureForm.interface_name,
      filter: { ...captureForm.filter },
      duration_seconds: captureForm.duration_seconds,
      max_mb: captureForm.max_mb,
      max_packets: captureForm.max_packets
    })
    captureTaskId.value = res.data?.task_id
    captureSession.value = null
    ElMessage.success(res.message || '抓包任务已提交')
    startCapturePolling()
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') console.error(err)
  } finally {
    captureSubmitting.value = false
  }
}

const startCapturePolling = () => {
  stopCapturePolling()
  if (!captureTaskId.value) return
  fetchCaptureSession()
  capturePollTimer = window.setInterval(fetchCaptureSession, 2000)
}

const stopCapturePolling = () => {
  if (capturePollTimer) {
    window.clearInterval(capturePollTimer)
    capturePollTimer = null
  }
}

const fetchCaptureSession = async () => {
  if (!captureTaskId.value) return
  try {
    const res = await getNetworkCaptureSession(captureTaskId.value)
    captureSession.value = res.data
    if (['success', 'failed', 'canceled'].includes(res.data?.status)) {
      stopCapturePolling()
    }
  } catch (err) {
    console.error(err)
  }
}

const cancelNetworkCapture = async () => {
  if (!captureTaskId.value) return
  try {
    await ElMessageBox.confirm('确定取消当前抓包任务？', '取消抓包', { type: 'warning' })
    await cancelTask(captureTaskId.value)
    ElMessage.success('已请求取消抓包')
    await fetchCaptureSession()
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') console.error(err)
  }
}

const downloadNetworkCapture = () => {
  if (!captureTaskId.value) return
  window.open(getNetworkCaptureDownloadUrl(captureTaskId.value), '_blank')
}

const deleteNetworkCaptureFile = async () => {
  if (!captureTaskId.value) return
  try {
    await ElMessageBox.confirm('确定删除当前 pcap 文件？删除后不能再下载该文件。', '删除 pcap', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    await deleteNetworkCapture(captureTaskId.value)
    ElMessage.success('pcap 文件已删除')
    await fetchCaptureSession()
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') console.error(err)
  }
}

const fetchSelfQuota = async () => {
  if (isAdmin.value || isLightweight.value) return
  try {
    const res = await getSelfQuota()
    selfQuota.value = res.data
  } catch (err) {
    console.error(err)
  }
}

const refreshQuotaAfterForwardChange = async () => {
  if (isLightweight.value || isLightweightVM.value) {
    await fetchVPCBinding()
  } else {
    await fetchSelfQuota()
  }
}

const fetchWhitelistSummary = async () => {
  if (!props.vmName) return
  try {
    const res = await getPortForwardWhitelistSummary(props.vmName)
    whitelistSummary.value = res.data || null
  } catch (err) {
    console.error(err)
  }
}

const handleForwardSelectionChange = (rows) => {
  selectedForwardIds.value = (rows || []).map(item => item.id)
}

const forwardRowSelectable = (row) => !!row?.live

const forwardProbeText = (row) => {
  if (!row) return '未知'
  return (!row.live && row.banned) ? '封禁' : '正常'
}

const forwardProbeTagType = (row) => {
  if (!row) return 'info'
  return (!row.live && row.banned) ? 'danger' : 'success'
}

const forwardProbeReason = (row) => {
  if (!row) return ''
  if (row.banned) return forwardBannedReason
  return ''
}

const fetchVPCBinding = async () => {
	if (!props.vmName) return
	vpcLoading.value = true
	try {
    const res = await getVMVPCBinding(props.vmName)
    vpcInfo.value = res.data || {}
    vpcSwitches.value = vpcInfo.value.switches || []
    vpcSecurityGroups.value = vpcInfo.value.groups || []
    if (vpcSwitches.value.length === 0) {
      const switchRes = await getVPCSwitches()
      vpcSwitches.value = switchRes.data || []
    }
		if (vpcSecurityGroups.value.length === 0) {
			const groupRes = await getVPCSecurityGroups()
			vpcSecurityGroups.value = groupRes.data || []
		}
		vpcForm.switch_id = vpcInfo.value.binding?.switch_id || vpcInfo.value.switch?.id || vpcSwitches.value[0]?.id || null
		await ensureSecurityGroupsForSelectedSwitch()
		if (selectedVPCSwitchIsBridge.value) {
			vpcForm.security_group_id = null
			return
		}
		const boundGroupID = vpcInfo.value.binding?.security_group_id || vpcInfo.value.security_group?.id || null
		const boundGroup = vpcSecurityGroups.value.find(item => item.id === boundGroupID)
		vpcForm.security_group_id = boundGroup && isSecurityGroupAllowedForSwitch(boundGroup) ? boundGroup.id : pickDefaultSecurityGroupForSwitch()
  } catch (err) {
    console.error(err)
  } finally {
    vpcLoading.value = false
	}
}

const mergeVPCSecurityGroups = (groups) => {
	const exists = new Set(vpcSecurityGroups.value.map(item => item.id))
	for (const group of groups || []) {
		if (!exists.has(group.id)) {
			vpcSecurityGroups.value.push(group)
			exists.add(group.id)
		}
	}
}

const ensureSecurityGroupsForSelectedSwitch = async () => {
	const sw = selectedVPCSwitch.value
	if (sw?.bridge_mode === 'bridge') {
		vpcForm.security_group_id = null
		return
	}
	if (!isAdmin.value || !sw?.username) {
		return
	}
	if (vpcSecurityGroups.value.some(item => item.username === sw.username)) {
		return
	}
	const groupRes = await getVPCSecurityGroups({ username: sw.username })
	mergeVPCSecurityGroups(groupRes.data || [])
}

const isSecurityGroupAllowedForSwitch = (group) => {
  if (!group) return false
  if (!isAdmin.value || !selectedVPCSwitch.value?.username) return true
  return group.username === selectedVPCSwitch.value.username
}

const pickDefaultSecurityGroupForSwitch = () => {
  const groups = filteredVPCSecurityGroups.value
  return groups.find(item => item.is_default)?.id || groups[0]?.id || null
}

watch(() => vpcForm.switch_id, async () => {
	await ensureSecurityGroupsForSelectedSwitch()
	if (selectedVPCSwitchIsBridge.value) {
		vpcForm.security_group_id = null
		return
	}
	const currentGroup = vpcSecurityGroups.value.find(item => item.id === vpcForm.security_group_id)
	if (!isSecurityGroupAllowedForSwitch(currentGroup)) {
		vpcForm.security_group_id = pickDefaultSecurityGroupForSwitch()
	}
})

watch(hasNatSwitch, (hasNat) => {
  if (!hasNat && (activeTab.value === 'forward' || activeTab.value === 'staticip')) {
    activeTab.value = resolveDefaultTab()
  }
}, { immediate: true })

watch(portForwardTabVisible, (visible, oldVisible) => {
  if (visible && !oldVisible && activeTab.value === 'staticip') {
    activeTab.value = 'forward'
  }
})

watch([portForwardTabVisible, currentSwitchIsBridge], () => {
  if (activeTab.value === 'forward' && !portForwardTabVisible.value) {
    activeTab.value = resolveDefaultTab()
  }
  initPortForwardIntro()
}, { immediate: true })

watch(activeTab, (tab) => {
  if (tab === 'diagnostics' && isAdmin.value) {
    fetchNetworkDiagnostics()
  }
  if (tab === 'interfaces') {
    refreshInterfaces()
  }
})

const submitVPCBinding = async () => {
  if (isLightweight.value) {
    ElMessage.warning('轻量云服务器的 VPC 由管理员分配，不能自行切换')
    return
  }
  if (!vpcForm.switch_id || (!selectedVPCSwitchIsBridge.value && !vpcForm.security_group_id)) {
    ElMessage.warning(selectedVPCSwitchIsBridge.value ? '请选择交换机' : '请选择交换机和安全组')
    return
  }
  vpcSubmitting.value = true
  try {
    await bindVMVPC(props.vmName, {
      switch_id: vpcForm.switch_id,
      security_group_id: selectedVPCSwitchIsBridge.value ? 0 : vpcForm.security_group_id
    })
    if (runtimeStatus.value?.state === 'running' && !selectedVPCSwitchIsBridge.value) {
      ElMessage.warning('VPC 绑定已更新；运行中的虚拟机需要重新获取 DHCP 或重启后才会显示新 IP')
    } else {
      ElMessage.success('VPC 绑定已更新')
    }
    await fetchVPCBinding()
  } catch (err) {
    console.error(err)
  } finally {
    vpcSubmitting.value = false
  }
}

const submitSecurityGroupOnly = async () => {
  if (isLightweight.value) {
    ElMessage.warning('轻量云服务器使用专属安全组，不能切换安全组')
    return
  }
  if (!vpcInfo.value?.binding) {
    ElMessage.warning('请先保存 VPC 绑定，再单独切换安全组')
    return
  }
  if (selectedVPCSwitchIsBridge.value) {
    ElMessage.warning('桥接直通交换机不使用安全组')
    return
  }
  if (!vpcForm.security_group_id) {
    ElMessage.warning('请选择安全组')
    return
  }
  vpcSubmitting.value = true
  try {
    await switchVMSecurityGroup(props.vmName, vpcForm.security_group_id)
    ElMessage.success('安全组已切换')
    await fetchVPCBinding()
  } catch (err) {
    console.error(err)
  } finally {
    vpcSubmitting.value = false
  }
}

const openSecurityRuleDialog = () => {
  Object.assign(securityRuleForm, {
    direction: 'ingress',
    protocol: 'tcp',
    port_start: 22,
    port_end: 22,
    port_text: '22',
    port_all: false,
    target_type: 'cidr',
    target_value: '0.0.0.0/0',
    remark: ''
  })
  securityRuleVisible.value = true
}

const securityRulePortText = (row) => {
  if (!row || row.protocol === 'icmp' || row.protocol === 'all') return '-'
  if (!row.port_start) return '-'
  if (!row.port_end || row.port_end === row.port_start) return row.port_start
  return `${row.port_start}-${row.port_end}`
}

const submitSecurityRule = async () => {
  const groupID = vpcInfo.value?.security_group?.id
  if (!groupID) {
    ElMessage.warning('当前服务器尚未绑定专属安全组')
    return
  }
  securityRuleSubmitting.value = true
  try {
    const payload = {
      ...securityRuleForm,
      target_type: 'cidr'
    }
    // 解析端口
    if (payload.port_all) {
      payload.port_start = 1
      payload.port_end = 65535
    } else if (payload.port_text) {
      const parts = payload.port_text.split('-')
      payload.port_start = parseInt(parts[0]) || 22
      payload.port_end = parts.length > 1 ? (parseInt(parts[1]) || 65535) : payload.port_start
    }
    if (payload.protocol === 'icmp' || payload.protocol === 'all') {
      payload.port_start = 0
      payload.port_end = 0
    }
    await addVPCSecurityGroupRule(groupID, payload)
    ElMessage.success('安全组规则已添加')
    securityRuleVisible.value = false
    await fetchVPCBinding()
  } catch (err) {
    console.error(err)
  } finally {
    securityRuleSubmitting.value = false
  }
}

const deleteSecurityRule = async (row) => {
  try {
    await ElMessageBox.confirm('确定删除此安全组规则？', '提示', { type: 'warning' })
    await deleteVPCSecurityGroupRule(row.id)
    ElMessage.success('安全组规则已删除')
    await fetchVPCBinding()
  } catch {}
}

// ==================== 多网口管理 ====================
const fetchMultiNicInterfaces = async () => {
  if (!props.vmName) return
  multiNicLoading.value = true
  try {
    const res = await listVMInterfaces(props.vmName)
    multiNicInterfaces.value = res.data || []
  } catch (err) {
    console.error(err)
  } finally {
    multiNicLoading.value = false
  }
}

const handleAddNic = async () => {
  // 确保开关机和交换机列表已加载
  if (vpcSwitches.value.length === 0) {
    try {
      const switchRes = await getVPCSwitches()
      vpcSwitches.value = switchRes.data || []
    } catch (err) {
      console.error(err)
    }
  }
  if (vpcSecurityGroups.value.length === 0) {
    try {
      const groupRes = await getVPCSecurityGroups()
      vpcSecurityGroups.value = groupRes.data || []
    } catch (err) {
      console.error(err)
    }
  }
  addNicForm.nic_model = 'virtio'
  addNicForm.switch_id = vpcSwitches.value.length > 0 ? vpcSwitches.value[0].id : null
  addNicForm.security_group_id = null
  addNicForm.bandwidth_inbound_avg = 0
  addNicForm.bandwidth_outbound_avg = 0
  addNicVisible.value = true
}

const submitAddNic = async () => {
  if (!addNicForm.switch_id) {
    ElMessage.warning('请选择 VPC 交换机')
    return
  }
  multiNicSubmitting.value = true
  try {
    const res = await addVMInterface(props.vmName, {
      nic_model: addNicForm.nic_model,
      switch_id: addNicForm.switch_id,
      security_group_id: addNicForm.security_group_id || 0,
      bandwidth_inbound_avg: addNicForm.bandwidth_inbound_avg || 0,
      bandwidth_outbound_avg: addNicForm.bandwidth_outbound_avg || 0
    })
    ElMessage.success(res.message || '网口已添加')
    addNicVisible.value = false
    await fetchMultiNicInterfaces()
    await fetchVPCBinding()
    // 延迟刷新运行时状态以获取新网口 IP
    setTimeout(() => fetchRuntimeStatus(), 2000)
  } catch (err) {
    console.error(err)
  } finally {
    multiNicSubmitting.value = false
  }
}

const handleRemoveNic = async (row) => {
  const order = row.binding?.interface_order
  if (order == null) {
    ElMessage.warning('无效的网口序号')
    return
  }
  multiNicSubmitting.value = true
  try {
    const res = await removeVMInterface(props.vmName, order)
    ElMessage.success(res.message || '网口已删除')
    await fetchMultiNicInterfaces()
    await fetchVPCBinding()
    fetchRuntimeStatus()
  } catch (err) {
    console.error(err)
  } finally {
    multiNicSubmitting.value = false
  }
}

const refreshInterfaces = async () => {
  multiNicLoading.value = true
  try {
    if (isAdmin.value) {
      await fetchMultiNicInterfaces()
    } else {
      // 非管理员从 vpcInfo 构建网口数据
      await fetchVPCBinding()
      buildInterfacesFromVpcInfo()
    }
    await fetchRuntimeStatus()
  } finally {
    multiNicLoading.value = false
  }
}

// 非管理员从 vpcInfo 构建网口列表
const buildInterfacesFromVpcInfo = () => {
  const bindings = vpcInfo.value?.bindings || []
  if (bindings.length === 0) {
    multiNicInterfaces.value = []
    return
  }
  const switches = vpcInfo.value?.switches || []
  const groups = vpcInfo.value?.groups || []
  multiNicInterfaces.value = bindings.map(b => ({
    binding: b,
    switch: switches.find(s => s.id === b.switch_id) || null,
    security_group: groups.find(g => g.id === b.security_group_id) || null
  }))
}

// 编辑网口
const handleEditInterface = async (row) => {
  // 确保交换机和安全组列表已加载
  if (vpcSwitches.value.length === 0) {
    try {
      const switchRes = await getVPCSwitches()
      vpcSwitches.value = switchRes.data || []
    } catch (err) {
      console.error(err)
    }
  }
  if (vpcSecurityGroups.value.length === 0) {
    try {
      const groupRes = await getVPCSecurityGroups()
      vpcSecurityGroups.value = groupRes.data || []
    } catch (err) {
      console.error(err)
    }
  }
  const order = row.binding?.interface_order ?? 0
  editNicEditingOrder.value = order
  editNicForm.nic_model = row.binding?.nic_model || 'virtio'
  editNicForm.switch_id = row.binding?.switch_id || row.switch?.id || null
  editNicForm.security_group_id = row.binding?.security_group_id || row.security_group?.id || null
  editNicForm.bandwidth_inbound_avg = row.binding?.bandwidth_inbound_avg || 0
  editNicForm.bandwidth_outbound_avg = row.binding?.bandwidth_outbound_avg || 0
  editNicVisible.value = true
}

const onEditNicSwitchChange = () => {
  // 当切换交换机时，自动选择默认安全组
  if (editNicSelectedSwitch.value?.bridge_mode === 'bridge') {
    editNicForm.security_group_id = null
    return
  }
  // 保持当前安全组不变，如果当前安全组不属于新交换机用户则清空
  const currentGroup = vpcSecurityGroups.value.find(item => item.id === editNicForm.security_group_id)
  if (currentGroup && editNicSelectedSwitch.value?.username && currentGroup.username !== editNicSelectedSwitch.value.username) {
    editNicForm.security_group_id = null
  }
}

const submitEditNic = async () => {
  if (!editNicForm.switch_id) {
    ElMessage.warning('请选择 VPC 交换机')
    return
  }
  multiNicSubmitting.value = true
  try {
    await updateVMInterface(props.vmName, editNicEditingOrder.value, {
      nic_model: editNicForm.nic_model,
      switch_id: editNicForm.switch_id,
      security_group_id: editNicForm.security_group_id || 0,
      bandwidth_inbound_avg: editNicForm.bandwidth_inbound_avg || 0,
      bandwidth_outbound_avg: editNicForm.bandwidth_outbound_avg || 0
    })
    ElMessage.success('网口已更新')
    editNicVisible.value = false
    await fetchMultiNicInterfaces()
    await fetchVPCBinding()
    fetchRuntimeStatus()
  } catch (err) {
    console.error(err)
  } finally {
    multiNicSubmitting.value = false
  }
}

const ipSourceText = (source) => {
  const map = {
    guest_agent: 'Guest Agent',
    arp: 'ARP',
    ovs_dhcp: 'OVS DHCP',
    vpc_dhcp: 'VPC DHCP',
    libvirt_lease: 'libvirt 租约',
    static: '静态绑定'
  }
  return map[source] || '-'
}

// 根据网口行获取运行时 IP（通过接口序号匹配 runtimeStatus）
const getInterfaceIP = (row) => {
  const order = row.binding?.interface_order
  if (order === undefined || order === null) return ''
  const ifaces = runtimeStatus.value?.interfaces || []
  if (order < ifaces.length) {
    return ifaces[order].ip || ''
  }
  return ''
}

const queueStatusText = (bandwidth) => {
  const down = bandwidth?.down_queue ? `下行 ${bandwidth.down_queue_id}` : '下行无'
  const up = bandwidth?.up_queue ? `上行 ${bandwidth.up_queue_id}` : '上行无'
  return `${down} / ${up}`
}

const quotaText = (value) => {
  if (isAdmin.value) return '管理员不受用户配额限制'
  if (!value || value <= 0) return '不限'
  return `${value} Mbps`
}

const trafficQuotaText = (value) => {
  if (!value || value <= 0) return '不限'
  return `${value} GB`
}

const captureStatusText = (status) => {
  const map = {
    pending: '等待中',
    running: '运行中',
    success: '已完成',
    failed: '失败',
    canceled: '已取消'
  }
  return map[status] || status || '-'
}

const captureStatusType = (status) => {
  const map = {
    pending: 'info',
    running: 'warning',
    success: 'success',
    failed: 'danger',
    canceled: 'info'
  }
  return map[status] || 'info'
}

const formatBytes = (value) => {
  const size = Number(value || 0)
  if (size <= 0) return '0 B'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(2)} MB`
}

const switchOptionLabel = (item) => {
  const prefix = isAdmin.value && item.username ? `${item.username} / ` : ''
  if (item.bridge_mode === 'bridge') {
    const vlan = item.bridge_vlan_id > 0 ? `，VLAN ${item.bridge_vlan_id}` : ''
    return `${prefix}${item.name}（桥接直通：${item.bridge_name || '-'}${vlan}）`
  }
  return `${prefix}${item.name} (${item.cidr})`
}

const groupOptionLabel = (item) => {
  const prefix = isAdmin.value && item.username ? `${item.username} / ` : ''
  return item.is_default ? `${prefix}${item.name}（默认）` : `${prefix}${item.name}`
}

watch(() => props.vmName, (newVal) => {
  if (newVal) {
    stopCapturePolling()
    captureTaskId.value = null
    captureSession.value = null
    networkDiagnostics.value = null
    captureForm.interface_name = ''
    activeTab.value = resolveDefaultTab()
    fetchData()
  }
})

onMounted(() => {
  activeTab.value = resolveDefaultTab()
  fetchData()
})

onBeforeUnmount(() => {
  stopCapturePolling()
})

// 端口转发 - 添加
const handleAddForward = () => {
  Object.assign(forwardForm, { vm_name: props.vmName, vm_ip: '', host_port: '', vm_port: '', protocol: 'tcp' })
  // 如果只有一个 IP，默认选中
  if (vmIPOptions.value.length === 1) {
    forwardForm.vm_ip = vmIPOptions.value[0].ip
  }
  addForwardVisible.value = true
}

const submitAddForward = async () => {
  if (!forwardForm.vm_port) {
    ElMessage.warning('请输入虚拟机端口')
    return
  }
  if (!forwardForm.vm_ip) {
    ElMessage.warning('请选择或输入目标 IP')
    return
  }
  forwardSubmitting.value = true
  try {
    const res = await addPortForward(forwardForm)
    ElMessage.success(res.message || '端口转发规则已添加')
    addForwardVisible.value = false
    // 刷新所有数据（后端 AddPortForward 已自动保存手动 IP 映射）
    fetchForwardRules()
    fetchStaticIPs()
    fetchManualIPs()
    refreshQuotaAfterForwardChange()
  } catch (err) {
    console.error(err)
  } finally {
    forwardSubmitting.value = false
  }
}

const handleEditForward = (row) => {
  Object.assign(editForwardForm, {
    id: row.id,
    vm_name: row.vm_name || props.vmName,
    vm_ip: row.dest_ip || '',
    host_port: row.host_port || '',
    vm_port: row.dest_port || '',
    protocol: String(row.protocol || 'tcp').toLowerCase()
  })
  editForwardVisible.value = true
}

const submitEditForward = async () => {
  if (!editForwardForm.vm_port) {
    ElMessage.warning('请输入虚拟机端口')
    return
  }
  if (!editForwardForm.vm_ip) {
    ElMessage.warning('请选择或输入目标 IP')
    return
  }
  if (!editForwardForm.host_port) {
    ElMessage.warning('请输入宿主机端口')
    return
  }
  forwardSubmitting.value = true
  try {
    await updatePortForward(editForwardForm.id, {
      vm_name: editForwardForm.vm_name || props.vmName,
      vm_ip: editForwardForm.vm_ip,
      vm_port: editForwardForm.vm_port,
      host_port: editForwardForm.host_port,
      protocol: editForwardForm.protocol
    })
    ElMessage.success('端口转发规则已更新')
    editForwardVisible.value = false
    fetchForwardRules()
    fetchStaticIPs()
    fetchManualIPs()
    refreshQuotaAfterForwardChange()
  } catch (err) {
    console.error(err)
  } finally {
    forwardSubmitting.value = false
  }
}

// 端口转发 - 删除
const handleDeleteForward = async (row) => {
  try {
    await ElMessageBox.confirm('确定删除此转发规则？', '提示', { type: 'warning' })
    if (row.live) {
      await deletePortForward(row.id)
      ElMessage.success('规则已删除')
    } else {
      await deletePortForwardByRuleKey(row.rule_key)
      ElMessage.success('封禁记录已删除')
    }
    fetchForwardRules()
    refreshQuotaAfterForwardChange()
  } catch {}
}

const handleBatchDeleteForward = async () => {
  if (selectedForwardIds.value.length === 0) {
    ElMessage.warning('请先选择要删除的端口转发规则')
    return
  }
  try {
    await ElMessageBox.confirm(`确定批量删除已选中的 ${selectedForwardIds.value.length} 条端口转发规则？`, '提示', { type: 'warning' })
    await batchDeletePortForward({ ids: selectedForwardIds.value })
    ElMessage.success('端口转发规则已批量删除')
    selectedForwardIds.value = []
    fetchForwardRules()
    refreshQuotaAfterForwardChange()
  } catch {}
}

const handleFirewallToggle = async (row, enabled) => {
  try {
    await setPortForwardFirewall({
      key: row.firewall_key,
      exempt: !enabled
    })
    row.region_filter_enabled = enabled
    ElMessage.success(enabled ? '已继承入站区域限制' : '已豁免入站区域限制')
  } catch (err) {
    console.error(err)
    fetchForwardRules()
  }
}

const copyForwardAccessAddress = async (value) => {
  if (!value) {
    ElMessage.warning('没有可复制的完整访问地址')
    return
  }
  try {
    await copyTextWithFallback(value)
    ElMessage.success('完整访问地址已复制到剪贴板')
  } catch {
    ElMessage.warning('复制完整访问地址失败，请手动复制')
  }
}

const handleRunCurrentVMProbe = async () => {
  probeSubmitting.value = true
  try {
    const res = await runPortForwardHTTPProbe({ vm_name: props.vmName })
    ElMessage.success(res.message || '当前虚拟机端口转发 HTTP 探测任务已提交')
  } catch (err) {
    console.error(err)
  } finally {
    probeSubmitting.value = false
  }
}

// 静态 IP - 绑定
const handleBindIP = () => {
  if (staticIPDisabled.value) {
    ElMessage.warning('桥接直通交换机不使用面板 DHCP，不能在这里绑定静态 IP')
    return
  }
  bindForm.vm_name = props.vmName
  bindForm.ip = ''
  selectedLeaseIP.value = ''

  // 如果只有一个 DHCP 租约，默认选中
  if (currentVmDhcpLeases.value.length === 1) {
    selectedLeaseIP.value = currentVmDhcpLeases.value[0].ip
    bindForm.ip = currentVmDhcpLeases.value[0].ip
  }

  bindIPVisible.value = true
}

// 选择 DHCP 租约时更新 IP
const onLeaseSelected = (val) => {
  bindForm.ip = val
}

const submitBindIP = async () => {
  bindIPSubmitting.value = true
  try {
    const res = await bindStaticIP(bindForm)
    ElMessage.success(res.message || '静态 IP 绑定成功')
    bindIPVisible.value = false
    await fetchStaticIPs()
  } catch (err) {
    console.error(err)
  } finally {
    bindIPSubmitting.value = false
  }
}

// 静态 IP - 解绑
const handleUnbindIP = async (row) => {
  try {
    await ElMessageBox.confirm(`确定解绑 ${row.vm_name} 的静态 IP（${row.ip}）？`, '提示', { type: 'warning' })
    await unbindStaticIP({ vm_name: row.vm_name })
    ElMessage.success('静态 IP 已解绑')
    fetchStaticIPs()
  } catch {}
}

defineExpose({ refresh: fetchData })
</script>

<style scoped>
.runtime-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.runtime-summary,
.runtime-alert,
.bandwidth-status,
.bandwidth-tip,
.quota-summary,
.vpc-summary,
.diagnostics-summary,
.diagnostics-alert,
.capture-session {
  margin-bottom: 12px;
}

.diagnostics-toolbar,
.capture-actions,
.diagnostic-templates {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.capture-form {
  margin-top: 12px;
}

.capture-output,
.neighbors-panel {
  margin-top: 12px;
}

.capture-output-title {
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.capture-output pre,
.neighbors-panel pre {
  max-height: 220px;
  overflow: auto;
  margin: 0;
  padding: 10px;
  border: 1px solid var(--el-border-color);
  border-radius: 6px;
  background: var(--el-fill-color-lighter);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
}

.vpc-runtime-notice {
  margin-bottom: 12px;
}

.vpc-static-ip {
  margin-top: 16px;
}

.vpc-static-ip-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.vpc-static-ip-header h4 {
  margin: 0;
}

.vpc-static-ip-tip {
  margin-bottom: 10px;
}

.bandwidth-actions,
.vpc-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.vpc-form {
  margin-top: 12px;
}

.form-hint {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

.forward-access-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.forward-panel {
  position: relative;
  min-height: 360px;
}

.forward-panel-main {
  min-height: 360px;
  transition: filter 0.2s ease, opacity 0.2s ease;
}

.forward-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}

.forward-whitelist-banner {
  margin-bottom: 10px;
}

.forward-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.content-blurred {
  filter: blur(8px);
  opacity: 0.35;
  pointer-events: none;
  user-select: none;
}

.forward-intro-mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 24px;
  overflow-y: auto;
  box-sizing: border-box;
}

.forward-intro-card {
  width: min(640px, 100%);
  border-radius: 12px;
  margin: 0 auto;
  overflow: hidden;
}

.forward-intro-title {
  font-weight: 600;
  color: #303133;
}

.forward-intro-content {
  color: #606266;
  line-height: 1.8;
}

.forward-intro-content p {
  margin: 0 0 12px;
}

.forward-intro-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
}

@media (max-width: 768px) {
  .forward-panel {
    overflow: hidden;
    max-width: 100%;
  }

  .forward-intro-mask {
    position: absolute;
    inset: 0;
    padding: 12px;
    align-items: center;
    width: 100%;
    max-width: 100vw;
    box-sizing: border-box;
  }

  .forward-intro-card {
    width: auto !important;
    min-width: unset !important;
    max-width: calc(100vw - 40px) !important;
    box-sizing: border-box;
    margin: 0 auto;
  }

  .forward-intro-card :deep(.el-card__header) {
    padding: 12px 14px;
  }

  .forward-intro-card :deep(.el-card__body) {
    padding: 12px 14px;
  }

  .forward-intro-content {
    font-size: 13px;
    word-break: break-all;
  }
}

@media (max-width: 480px) {
  .forward-panel {
    overflow: hidden;
    max-width: 100%;
  }

  .forward-intro-mask {
    padding: 6px;
    max-width: 100vw;
    box-sizing: border-box;
  }

  .forward-intro-card {
    max-width: calc(100vw - 24px) !important;
  }

  .forward-intro-card :deep(.el-card__header) {
    padding: 10px 12px;
  }

  .forward-intro-card :deep(.el-card__body) {
    padding: 10px 12px;
  }

  .forward-intro-content {
    font-size: 12px;
  }
}

/* 多网口管理 */
.multi-nic-panel {
  min-height: 200px;
}
.multi-nic-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

/* IP 列内联代码样式 */
.ip-code {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  padding: 1px 6px;
  border-radius: 3px;
}
</style>
