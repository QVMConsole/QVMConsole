<template>
  <div class="network-page">
    <div class="page-header">
      <div class="page-header-left">
        <div class="page-title-row">
          <el-icon class="page-icon"><Connection /></el-icon>
          <h2>{{ isAdmin ? '网络中心' : 'VPC 网络' }}</h2>
        </div>
        <p>{{ isAdmin ? '管理 OVS 基础网络、交换机、安全组策略、端口转发与 ACL' : '管理交换机、安全组策略与月流量配额' }}</p>
      </div>
      <div class="page-header-right">
        <el-input
          v-if="isAdmin"
          v-model="usernameFilter"
          placeholder="按用户筛选"
          clearable
          :prefix-icon="Search"
          size="large"
          style="width: 200px;"
          @change="loadAll"
        />
        <el-button size="large" :icon="Refresh" @click="loadAll" :loading="loading">刷新</el-button>
      </div>
    </div>

    <div class="tabs-wrapper">
      <div class="custom-tabs">
        <div
          v-for="tab in visibleTabs"
          :key="tab.name"
          :class="['custom-tab', { active: activeTab === tab.name }]"
          @click="switchTab(tab.name)"
        >
          <el-icon><component :is="tab.icon" /></el-icon>
          <span>{{ tab.label }}</span>
        </div>
      </div>

      <div class="tab-content">
        <!-- 网络概览 -->
        <div v-show="activeTab === 'overview'" class="tab-pane">
          <div class="overview-stats">
            <div class="stat-card" :class="status?.healthy ? 'healthy' : 'warning'">
              <div class="stat-card-icon">
                <el-icon :size="28"><CircleCheckFilled v-if="status?.healthy" /><WarningFilled v-else /></el-icon>
              </div>
              <div class="stat-card-body">
                <div class="stat-card-label">OVS 状态</div>
                <div class="stat-card-value">{{ status?.healthy ? '运行正常' : '需要关注' }}</div>
              </div>
            </div>
            <div class="stat-card">
              <div class="stat-card-icon">
                <el-icon :size="28"><Connection /></el-icon>
              </div>
              <div class="stat-card-body">
                <div class="stat-card-label">网桥</div>
                <div class="stat-card-value code">{{ status?.bridge || '-' }}</div>
              </div>
            </div>
            <div class="stat-card">
              <div class="stat-card-icon">
                <el-icon :size="28"><Monitor /></el-icon>
              </div>
              <div class="stat-card-body">
                <div class="stat-card-label">端口数</div>
                <div class="stat-card-value">{{ ports?.ports?.length || 0 }}</div>
              </div>
            </div>
            <div class="stat-card">
              <div class="stat-card-icon">
                <el-icon :size="28"><Link /></el-icon>
              </div>
              <div class="stat-card-body">
                <div class="stat-card-label">内网 CIDR</div>
                <div class="stat-card-value code">{{ status?.subnet_cidr || '-' }}</div>
              </div>
            </div>
          </div>

          <div class="overview-actions-bar">
            <el-button :icon="View" @click="handleCheck" :loading="checking" size="large">检测</el-button>
            <el-button type="warning" :icon="Tools" @click="handleRepair" :loading="repairing" size="large">修复</el-button>
            <el-button type="primary" :icon="Plus" @click="openBridgeDialog" size="large">创建桥接网桥</el-button>
          </div>

          <el-row :gutter="20">
            <el-col :xs="24" :lg="12">
              <div class="info-card">
                <div class="info-card-header">
                  <el-icon><Cpu /></el-icon>
                  <span>基础状态</span>
                </div>
                <div class="info-list">
                  <div class="info-item">
                    <span class="info-label">网桥</span>
                    <span class="info-value code">{{ status?.bridge || '-' }}</span>
                  </div>
                  <div class="info-item">
                    <span class="info-label">网关 IP</span>
                    <span class="info-value code">{{ status?.gateway_ip || '-' }}</span>
                  </div>
                  <div class="info-item">
                    <span class="info-label">内网 CIDR</span>
                    <span class="info-value code">{{ status?.subnet_cidr || '-' }}</span>
                  </div>
                  <div class="info-item">
                    <span class="info-label">出口网卡</span>
                    <span class="info-value code">{{ status?.uplink || '未检测到' }}</span>
                  </div>
                  <div class="info-item">
                    <span class="info-label">ip_forward</span>
                    <el-tag :type="status?.ip_forward_enabled ? 'success' : 'danger'" size="small" effect="light">{{ yesNo(status?.ip_forward_enabled) }}</el-tag>
                  </div>
                  <div class="info-item">
                    <span class="info-label">NAT</span>
                    <el-tag :type="status?.nat_rule?.exists ? 'success' : 'danger'" size="small" effect="light">{{ yesNo(status?.nat_rule?.exists) }}</el-tag>
                  </div>
                </div>
              </div>
            </el-col>

            <el-col :xs="24" :lg="12">
              <div class="info-card">
                <div class="info-card-header">
                  <el-icon><Setting /></el-icon>
                  <span>服务状态</span>
                </div>
                <div class="info-list">
                  <div class="info-item">
                    <span class="info-label">openvswitch-switch</span>
                    <el-tag :type="status?.openvswitch_service?.active ? 'success' : 'danger'" size="small" effect="light">
                      {{ status?.openvswitch_service?.state || '-' }}
                    </el-tag>
                  </div>
                  <div class="info-item">
                    <span class="info-label">OVS dnsmasq</span>
                    <el-tag :type="status?.dnsmasq_service?.active ? 'success' : 'danger'" size="small" effect="light">
                      {{ status?.dnsmasq_service?.state || '-' }}
                    </el-tag>
                  </div>
                  <div class="info-item">
                    <span class="info-label">出站 FORWARD</span>
                    <el-tag :type="status?.forward_out_rule?.exists ? 'success' : 'danger'" size="small" effect="light">{{ yesNo(status?.forward_out_rule?.exists) }}</el-tag>
                  </div>
                  <div class="info-item">
                    <span class="info-label">回程 FORWARD</span>
                    <el-tag :type="status?.forward_return_rule?.exists ? 'success' : 'danger'" size="small" effect="light">{{ yesNo(status?.forward_return_rule?.exists) }}</el-tag>
                  </div>
                </div>
              </div>
            </el-col>
          </el-row>

          <div class="info-card table-card">
            <div class="info-card-header">
              <el-icon><Connection /></el-icon>
              <span>宿主机网桥</span>
              <el-tag size="small" effect="plain" round>{{ bridges.length }} 个网桥</el-tag>
            </div>
            <el-table :data="bridges" border style="width: 100%;" max-height="280">
              <el-table-column prop="name" label="网桥" min-width="130">
                <template #default="{ row }"><span class="code">{{ row.name }}</span></template>
              </el-table-column>
              <el-table-column label="类型" width="120">
                <template #default="{ row }">
                  <el-tag :type="row.mode === 'bridge' ? 'warning' : 'success'" size="small" effect="light">{{ bridgeModeText(row.mode) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="uplink_if" label="物理网卡" min-width="120">
                <template #default="{ row }"><span class="code">{{ row.uplink_if || '—' }}</span></template>
              </el-table-column>
              <el-table-column label="状态" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :type="row.exists && row.active ? 'success' : 'danger'" size="small">{{ row.exists && row.active ? '正常' : '异常' }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="交换机" width="100" align="center">
                <template #default="{ row }">{{ row.switch_count || 0 }}</template>
              </el-table-column>
              <el-table-column label="操作" width="110" align="center">
                <template #default="{ row }">
                  <el-button v-if="!row.is_default" size="small" type="danger" plain @click="handleDeleteBridge(row)">删除</el-button>
                  <span v-else class="text-muted">—</span>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <div class="info-card table-card">
            <div class="info-card-header">
              <el-icon><Monitor /></el-icon>
              <span>物理网卡</span>
              <el-tag size="small" effect="plain" round>{{ hostInterfaces.length }} 张网卡</el-tag>
            </div>
            <el-table :data="hostInterfaces" border style="width: 100%;" max-height="320">
              <el-table-column prop="name" label="名称" min-width="120">
                <template #default="{ row }"><span class="code">{{ row.name }}</span></template>
              </el-table-column>
              <el-table-column prop="state" label="状态" width="90" />
              <el-table-column prop="mac" label="MAC" min-width="150">
                <template #default="{ row }"><span class="code">{{ row.mac }}</span></template>
              </el-table-column>
              <el-table-column label="IP" min-width="190">
                <template #default="{ row }">
                  <span v-if="row.addresses?.length" class="code">{{ row.addresses.join(', ') }}</span>
                  <span v-else class="text-muted">—</span>
                </template>
              </el-table-column>
              <el-table-column label="默认路由" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :type="row.default_route ? 'warning' : 'info'" size="small">{{ yesNo(row.default_route) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="OVS 网桥" min-width="130">
                <template #default="{ row }"><span class="code">{{ row.ovs_bridge || row.managed_bridge || '—' }}</span></template>
              </el-table-column>
              <el-table-column prop="risk" label="风险提示" min-width="220" show-overflow-tooltip />
            </el-table>
          </div>

          <div class="info-card table-card">
            <div class="info-card-header">
              <el-icon><List /></el-icon>
              <span>OVS 端口列表</span>
              <el-tag size="small" effect="plain" round>{{ ports?.ports?.length || 0 }} 个端口</el-tag>
            </div>
            <el-table :data="ports?.ports || []" border style="width: 100%;" max-height="400">
              <el-table-column prop="name" label="端口名称" min-width="140">
                <template #default="{ row }">
                  <span class="code">{{ row.name }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="ofport" label="ofport" width="100" />
              <el-table-column label="类型" width="110">
                <template #default="{ row }">
                  <el-tag size="small" effect="plain" :type="row.type === 'internal' ? 'info' : ''">{{ row.type }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="关联 VM" min-width="140">
                <template #default="{ row }">{{ row.vm_name || '—' }}</template>
              </el-table-column>
              <el-table-column label="IP 地址" min-width="150">
                <template #default="{ row }">
                  <span class="code">{{ row.ip || '—' }}</span>
                </template>
              </el-table-column>
              <el-table-column label="异常信息" min-width="200">
                <template #default="{ row }">
                  <template v-if="row.issues?.length">
                    <el-tag v-for="issue in row.issues" :key="issue" type="warning" size="small" effect="light" style="margin-right: 4px;">{{ issue }}</el-tag>
                  </template>
                  <span v-else class="text-muted">—</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>

        <!-- 交换机 -->
        <div v-show="activeTab === 'switches'" class="tab-pane">
          <div v-if="quota" class="quota-summary">
            <div class="quota-card">
              <div class="quota-card-icon down">
                <el-icon :size="24"><Download /></el-icon>
              </div>
              <div class="quota-card-body">
                <div class="quota-card-label">下行流量</div>
                <div class="quota-card-value">{{ quota.remaining_traffic_down === -1 ? '不限' : quota.remaining_traffic_down + ' GB' }}</div>
                <div class="quota-card-sub">已分配 {{ quota.allocated_traffic_down || 0 }} GB</div>
              </div>
            </div>
            <div class="quota-card">
              <div class="quota-card-icon up">
                <el-icon :size="24"><Upload /></el-icon>
              </div>
              <div class="quota-card-body">
                <div class="quota-card-label">上行流量</div>
                <div class="quota-card-value">{{ quota.remaining_traffic_up === -1 ? '不限' : quota.remaining_traffic_up + ' GB' }}</div>
                <div class="quota-card-sub">已分配 {{ quota.allocated_traffic_up || 0 }} GB</div>
              </div>
            </div>
            <div class="quota-card">
              <div class="quota-card-icon down-bandwidth">
                <el-icon :size="24"><Connection /></el-icon>
              </div>
              <div class="quota-card-body">
                <div class="quota-card-label">下行带宽</div>
                <div class="quota-card-value">{{ quota.remaining_bandwidth_down === -1 ? '不限' : quota.remaining_bandwidth_down + ' Mbps' }}</div>
                <div class="quota-card-sub">已分配 {{ quota.allocated_bandwidth_down || 0 }} Mbps</div>
              </div>
            </div>
            <div class="quota-card">
              <div class="quota-card-icon up-bandwidth">
                <el-icon :size="24"><Connection /></el-icon>
              </div>
              <div class="quota-card-body">
                <div class="quota-card-label">上行带宽</div>
                <div class="quota-card-value">{{ quota.remaining_bandwidth_up === -1 ? '不限' : quota.remaining_bandwidth_up + ' Mbps' }}</div>
                <div class="quota-card-sub">已分配 {{ quota.allocated_bandwidth_up || 0 }} Mbps</div>
              </div>
            </div>
          </div>

          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <span class="table-title">交换机列表</span>
              <el-tag size="small" effect="plain" round>{{ switches.length }} 个</el-tag>
            </div>
            <el-button type="primary" :icon="Plus" size="large" @click="openSwitchDialog()">创建交换机</el-button>
          </div>

          <div class="filter-bar">
            <el-input
              v-model="switchSearchText"
              placeholder="搜索名称"
              clearable
              :prefix-icon="Search"
              size="small"
              style="width: 160px;"
            />
            <el-input
              v-model="switchSubnetSearch"
              placeholder="搜索子网"
              clearable
              :prefix-icon="Search"
              size="small"
              style="width: 180px;"
            />
          </div>

          <div class="data-table-card switch-data-card">
            <el-table :data="paginatedSwitches" border v-loading="loading" style="width: 100%;" row-key="id">
              <el-table-column v-if="isAdmin" prop="username" label="所属用户" width="130" />
              <el-table-column prop="name" label="名称" min-width="160">
                <template #default="{ row }">
                  <div class="switch-name-cell">
                    <el-icon :size="18"><Connection /></el-icon>
                    <span>{{ row.name }}</span>
                    <el-tag v-if="row.is_system" type="info" size="small" effect="light" round>系统</el-tag>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="VLAN" width="90" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.is_system" type="info" effect="plain" size="small" round>基础网络</el-tag>
                  <el-tag v-else effect="plain" size="small" round>{{ row.bridge_mode === 'bridge' ? '桥接' : row.vlan_id }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column v-if="isAdmin" label="目标网桥" min-width="130">
                <template #default="{ row }">
                  <span class="code">{{ row.bridge_name || 'br-ovs' }}</span>
                </template>
              </el-table-column>
              <el-table-column v-if="isAdmin" label="桥接 VLAN" width="110" align="center">
                <template #default="{ row }">
                  <span>{{ row.bridge_mode === 'bridge' ? (row.bridge_vlan_id > 0 ? row.bridge_vlan_id : '不打标签') : '—' }}</span>
                </template>
              </el-table-column>
              <el-table-column v-if="isAdmin" label="桥接安全" min-width="210">
                <template #default="{ row }">
                  <div v-if="row.bridge_mode === 'bridge'" class="security-tags">
                    <el-tag :type="row.allow_promiscuous ? 'warning' : 'success'" size="small">混杂{{ row.allow_promiscuous ? '允许' : '拒绝' }}</el-tag>
                    <el-tag :type="row.allow_mac_change ? 'warning' : 'success'" size="small">MAC{{ row.allow_mac_change ? '允许' : '拒绝' }}</el-tag>
                    <el-tag :type="row.allow_forged_transmits ? 'warning' : 'success'" size="small">伪传输{{ row.allow_forged_transmits ? '允许' : '拒绝' }}</el-tag>
                  </div>
                  <span v-else>—</span>
                </template>
              </el-table-column>
              <el-table-column label="子网" min-width="160">
                <template #default="{ row }">
                  <span class="code">{{ row.bridge_mode === 'bridge' ? '上级路由分配' : row.cidr }}</span>
                </template>
              </el-table-column>
              <el-table-column label="网关" min-width="140">
                <template #default="{ row }">
                  <span class="code">{{ row.gateway_ip }}</span>
                </template>
              </el-table-column>
              <el-table-column label="月下行流量" width="190">
                <template #default="{ row }">
                  <div class="traffic-cell">
                    <span>{{ switchTrafficText(row, 'down') }}</span>
                    <el-progress
                      v-if="row.traffic_down_gb > 0"
                      :percentage="switchTrafficPercent(row, 'down')"
                      :stroke-width="6"
                      :show-text="false"
                      :color="row.is_limited_down ? '#F56C6C' : '#409EFF'"
                    />
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="月上行流量" width="190">
                <template #default="{ row }">
                  <div class="traffic-cell">
                    <span>{{ switchTrafficText(row, 'up') }}</span>
                    <el-progress
                      v-if="row.traffic_up_gb > 0"
                      :percentage="switchTrafficPercent(row, 'up')"
                      :stroke-width="6"
                      :show-text="false"
                      :color="row.is_limited_up ? '#F56C6C' : '#409EFF'"
                    />
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="下行带宽" width="130" align="center">
                <template #default="{ row }">
                  <span :class="{ 'text-danger': row.is_limited_down }">{{ switchBandwidthText(row.effective_bandwidth_down_mbps || row.bandwidth_down_mbps) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="上行带宽" width="130" align="center">
                <template #default="{ row }">
                  <span :class="{ 'text-danger': row.is_limited_up }">{{ switchBandwidthText(row.effective_bandwidth_up_mbps || row.bandwidth_up_mbps) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="限速状态" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :type="row.is_limited_down || row.is_limited_up ? 'danger' : 'success'" size="small" effect="light">
                    {{ row.is_limited_down || row.is_limited_up ? '已限速' : '正常' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="280" fixed="right">
                <template #default="{ row }">
                  <el-button size="small" plain @click="openSwitchDialog(row)" :disabled="row.is_system">编辑</el-button>
                  <el-button v-if="isAdmin" size="small" plain type="warning" @click="handleResetSwitchTraffic(row)" :disabled="row.is_system">重置流量</el-button>
                  <el-button size="small" plain type="danger" @click="handleDeleteSwitch(row)" :disabled="row.is_system">删除</el-button>
                </template>
              </el-table-column>
            </el-table>

            <div class="pagination-wrap">
              <el-pagination
                v-if="filteredSwitches.length > switchPageSize"
                background
                layout="total, prev, pager, next"
                :total="filteredSwitches.length"
                :page-size="switchPageSize"
                :current-page="switchCurrentPage"
                @current-change="switchCurrentPage = $event"
              />
            </div>

            <!-- 移动端交换机卡片 -->
            <div class="mobile-card-list">
              <el-card v-for="row in paginatedSwitches" :key="row.id" class="switch-mobile-card" shadow="hover">
                <div class="switch-card-header">
                  <div class="switch-card-name-row">
                    <span class="switch-card-name">{{ row.name }}</span>
                    <el-tag v-if="row.is_system" type="info" effect="plain" size="small" round>系统</el-tag>
                    <el-tag v-else effect="plain" size="small" round>{{ row.bridge_mode === 'bridge' ? '桥接' : row.vlan_id }}</el-tag>
                  </div>
                  <div v-if="isAdmin && !row.is_system" class="switch-card-user">{{ row.username }}</div>
                </div>
                <div class="switch-card-body">
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">子网</span>
                    <span class="switch-card-value code">{{ row.bridge_mode === 'bridge' ? '上级路由分配' : row.cidr }}</span>
                  </div>
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">网关</span>
                    <span class="switch-card-value code">{{ row.gateway_ip }}</span>
                  </div>
                  <div v-if="isAdmin" class="switch-card-info-row">
                    <span class="switch-card-label">目标网桥</span>
                    <span class="switch-card-value code">{{ row.bridge_name || 'br-ovs' }}</span>
                  </div>
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">下行流量</span>
                    <span class="switch-card-value">{{ switchTrafficText(row, 'down') }}</span>
                  </div>
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">上行流量</span>
                    <span class="switch-card-value">{{ switchTrafficText(row, 'up') }}</span>
                  </div>
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">下行带宽</span>
                    <span class="switch-card-value" :class="{ 'text-danger': row.is_limited_down }">{{ switchBandwidthText(row.effective_bandwidth_down_mbps || row.bandwidth_down_mbps) }}</span>
                  </div>
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">上行带宽</span>
                    <span class="switch-card-value" :class="{ 'text-danger': row.is_limited_up }">{{ switchBandwidthText(row.effective_bandwidth_up_mbps || row.bandwidth_up_mbps) }}</span>
                  </div>
                  <div class="switch-card-info-row">
                    <span class="switch-card-label">限速状态</span>
                    <span class="switch-card-value">
                      <el-tag :type="row.is_limited_down || row.is_limited_up ? 'danger' : 'success'" size="small" effect="light">
                        {{ row.is_limited_down || row.is_limited_up ? '已限速' : '正常' }}
                      </el-tag>
                    </span>
                  </div>
                </div>
                <div class="switch-card-actions">
                  <el-button size="small" plain @click="openSwitchDialog(row)" :disabled="row.is_system">编辑</el-button>
                  <el-button v-if="isAdmin" size="small" plain type="warning" @click="handleResetSwitchTraffic(row)" :disabled="row.is_system">重置流量</el-button>
                  <el-button size="small" plain type="danger" @click="handleDeleteSwitch(row)" :disabled="row.is_system">删除</el-button>
                </div>
              </el-card>
            </div>
          </div>
        </div>

        <!-- 安全组策略 -->
        <div v-show="activeTab === 'securityGroups'" class="tab-pane">
          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <span class="table-title">安全组列表</span>
              <el-tag size="small" effect="plain" round>{{ securityGroups.length }} 个</el-tag>
            </div>
            <el-button type="primary" :icon="Plus" size="large" @click="openGroupDialog()">创建安全组</el-button>
          </div>

          <div class="filter-bar">
            <el-input
              v-model="sgSearchText"
              placeholder="搜索名称"
              clearable
              :prefix-icon="Search"
              size="small"
              style="width: 160px;"
            />
            <el-select
              v-model="sgTypeFilter"
              placeholder="类型筛选"
              clearable
              size="small"
              style="width: 130px;"
            >
              <el-option label="默认" :value="true" />
              <el-option label="自定义" :value="false" />
            </el-select>
          </div>

          <div class="data-table-card security-group-data-card">
            <el-table :data="paginatedSecurityGroups" border v-loading="loading" style="width: 100%;" row-key="id">
              <el-table-column type="expand">
                <template #default="{ row }">
                  <div class="expand-rule-panel">
                    <div class="expand-rule-header">
                      <span class="expand-rule-title">安全组规则 ({{ row.name }})</span>
                      <el-button size="small" type="primary" :icon="Plus" @click="openRuleDialog(row)">添加规则</el-button>
                    </div>
                    <el-table v-if="row.rules?.length" :data="row.rules" border size="small" class="rule-inner-table">
                      <el-table-column label="方向" width="90" align="center">
                        <template #default="{ row: rule }">
                          <el-tag :type="rule.direction === 'ingress' ? 'success' : 'warning'" size="small" effect="light">
                            {{ directionText(rule.direction) }}
                          </el-tag>
                        </template>
                      </el-table-column>
                      <el-table-column label="协议" width="90" align="center">
                        <template #default="{ row: rule }">
                          <span class="code">{{ rule.protocol?.toUpperCase() }}</span>
                        </template>
                      </el-table-column>
                      <el-table-column label="端口范围" width="130">
                        <template #default="{ row: rule }">
                          <span class="code">{{ portText(rule) }}</span>
                        </template>
                      </el-table-column>
                      <el-table-column label="目标" min-width="220">
                        <template #default="{ row: rule }">
                          <span class="code">{{ targetText(rule) }}</span>
                        </template>
                      </el-table-column>
                      <el-table-column prop="remark" label="备注" min-width="180" show-overflow-tooltip />
                      <el-table-column label="操作" width="90" align="center">
                        <template #default="{ row: rule }">
                          <el-button size="small" type="danger" plain @click="handleDeleteRule(rule)">删除</el-button>
                        </template>
                      </el-table-column>
                    </el-table>
                    <el-empty v-else description="暂无规则，点击上方按钮添加" :image-size="60" />
                  </div>
                </template>
              </el-table-column>
              <el-table-column v-if="isAdmin" prop="username" label="所属用户" width="130" />
              <el-table-column prop="name" label="名称" min-width="180">
                <template #default="{ row }">
                  <div class="group-name-cell">
                    <el-icon :size="18"><Lock /></el-icon>
                    <span>{{ row.name }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="类型" width="100" align="center">
                <template #default="{ row }">
                  <el-tag :type="row.is_default ? 'success' : 'info'" size="small" effect="light" round>
                    {{ row.is_default ? '默认' : '自定义' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="remark" label="备注" min-width="220" show-overflow-tooltip>
                <template #default="{ row }">
                  <span v-if="row.remark" class="text-muted">{{ row.remark }}</span>
                  <span v-else class="text-muted">—</span>
                </template>
              </el-table-column>
              <el-table-column label="规则数" width="100" align="center">
                <template #default="{ row }">
                  <el-tag effect="plain" size="small" round>{{ row.rules?.length || 0 }} 条</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="180" align="center">
                <template #default="{ row }">
                  <div style="display: flex; justify-content: center; gap: 8px;">
                    <el-button size="small" plain @click="openGroupDialog(row)">编辑</el-button>
                    <el-tooltip :disabled="!row.is_default" content="默认安全组用于兜底策略，不能删除" placement="top">
                      <span>
                        <el-button size="small" type="danger" plain :disabled="row.is_default" @click="handleDeleteGroup(row)">
                          {{ row.is_default ? '受保护' : '删除' }}
                        </el-button>
                      </span>
                    </el-tooltip>
                  </div>
                </template>
              </el-table-column>
            </el-table>

            <div class="pagination-wrap">
              <el-pagination
                v-if="filteredSecurityGroups.length > sgPageSize"
                background
                layout="total, prev, pager, next"
                :total="filteredSecurityGroups.length"
                :page-size="sgPageSize"
                :current-page="sgCurrentPage"
                @current-change="sgCurrentPage = $event"
              />
            </div>

            <!-- 移动端安全组卡片 -->
            <div class="mobile-card-list">
              <el-card v-for="row in paginatedSecurityGroups" :key="row.id" class="sg-mobile-card" shadow="hover">
                <div class="sg-card-header">
                  <div class="sg-card-name-row">
                    <span class="sg-card-name">{{ row.name }}</span>
                    <el-tag :type="row.is_default ? 'success' : 'info'" size="small" effect="light" round>{{ row.is_default ? '默认' : '自定义' }}</el-tag>
                  </div>
                  <div v-if="isAdmin" class="sg-card-user">{{ row.username }}</div>
                </div>
                <div class="sg-card-body">
                  <div v-if="row.remark" class="sg-card-info-row">
                    <span class="sg-card-label">备注</span>
                    <span class="sg-card-value">{{ row.remark }}</span>
                  </div>
                  <div class="sg-card-info-row">
                    <span class="sg-card-label">规则数</span>
                    <span class="sg-card-value"><el-tag effect="plain" size="small" round>{{ row.rules?.length || 0 }} 条</el-tag></span>
                  </div>
                </div>
                <div class="sg-card-actions">
                  <el-button size="small" plain @click="openGroupDialog(row)">编辑</el-button>
                  <el-button size="small" type="danger" plain :disabled="row.is_default" @click="handleDeleteGroup(row)">{{ row.is_default ? '受保护' : '删除' }}</el-button>
                </div>
              </el-card>
            </div>
          </div>
        </div>

        <!-- 端口转发 -->
        <div v-show="activeTab === 'forwards'" class="tab-pane">
          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <span class="table-title">端口转发规则</span>
              <el-tag size="small" effect="plain" round>{{ forwardRules.length }} 条</el-tag>
            </div>
            <div class="table-toolbar-right">
              <el-button type="primary" plain :icon="Connection" :loading="probeSubmitting" @click="handleRunForwardProbe">立即探测全部 TCP 转发</el-button>
              <el-button plain @click="openUserWhitelistDialog">用户白名单</el-button>
              <el-button plain @click="openVMWhitelistDialog">虚拟机白名单</el-button>
              <el-button type="danger" plain :disabled="selectedForwardIds.length === 0" @click="handleBatchDeleteForward">批量删除</el-button>
            </div>
          </div>

          <div class="filter-bar">
            <el-input
              v-model="forwardSearchText"
              placeholder="搜索外部端口"
              clearable
              :prefix-icon="Search"
              size="small"
              style="width: 160px;"
            />
            <el-input
              v-model="forwardVmSearch"
              placeholder="搜索虚拟机"
              clearable
              :prefix-icon="Search"
              size="small"
              style="width: 160px;"
            />
            <el-select
              v-model="forwardProtocolFilter"
              placeholder="协议筛选"
              clearable
              size="small"
              style="width: 120px;"
            >
              <el-option label="TCP" value="tcp" />
              <el-option label="UDP" value="udp" />
            </el-select>
          </div>

          <div class="data-table-card forward-data-card">
            <el-table :data="paginatedForwardRules" border v-loading="loading || whitelistLoading" style="width: 100%;" row-key="rule_key" @selection-change="handleForwardSelectionChange">
              <el-table-column type="selection" width="48" :selectable="forwardRowSelectable" />
              <el-table-column prop="id" label="#" width="70" align="center" />
              <el-table-column label="协议" width="100" align="center">
                <template #default="{ row }">
                  <el-tag effect="plain" size="small" round>{{ row.protocol?.toUpperCase() }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="外部端口" width="120" align="center">
                <template #default="{ row }">
                  <span class="code port-highlight">{{ row.host_port }}</span>
                </template>
              </el-table-column>
              <el-table-column label="完整访问地址" min-width="220">
                <template #default="{ row }">
                  <div class="forward-access-cell">
                    <span class="code">{{ row.access_address || '-' }}</span>
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
              <el-table-column label="探测状态" width="150" align="center">
                <template #default="{ row }">
                  <el-tooltip :content="forwardProbeReason(row)" placement="top" :disabled="!forwardProbeReason(row)">
                    <el-tag :type="forwardProbeTagType(row)" size="small" effect="light">
                      {{ forwardProbeText(row) }}
                    </el-tag>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column label="内部 IP" min-width="160">
                <template #default="{ row }">
                  <span class="code">{{ row.dest_ip }}</span>
                </template>
              </el-table-column>
              <el-table-column label="内部端口" width="120" align="center">
                <template #default="{ row }">
                  <span class="code port-highlight">{{ row.dest_port }}</span>
                </template>
              </el-table-column>
              <el-table-column label="归属用户" min-width="120">
                <template #default="{ row }">
                  <span>{{ row.owner_username || '—' }}</span>
                </template>
              </el-table-column>
              <el-table-column label="关联 VM" min-width="180">
                <template #default="{ row }">
                  <div class="vm-name-cell">
                    <el-icon :size="16"><Monitor /></el-icon>
                    <span>{{ row.vm_name || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="安全组放行" width="120" align="center">
                <template #default>
                  <el-tag type="success" size="small" effect="light">自动补齐</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="150" align="center" fixed="right">
                <template #default="{ row }">
                  <el-button v-if="row.live" type="primary" link @click="openForwardDialog(row)">编辑</el-button>
                  <el-button type="danger" link @click="handleDeleteForward(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <el-empty v-if="!loading && forwardRules.length === 0" description="暂无端口转发规则" :image-size="80" />

            <div class="pagination-wrap">
              <el-pagination
                v-if="filteredForwardRules.length > forwardPageSize"
                background
                layout="total, prev, pager, next"
                :total="filteredForwardRules.length"
                :page-size="forwardPageSize"
                :current-page="forwardCurrentPage"
                @current-change="forwardCurrentPage = $event"
              />
            </div>

            <!-- ===== 移动端卡片视图 ===== -->
            <div class="mobile-card-list">
              <el-card
                v-for="row in paginatedForwardRules"
                :key="row.rule_key"
                class="forward-mobile-card"
                shadow="hover"
              >
                <div class="forward-card-header">
                  <div class="forward-card-port-row">
                    <span class="forward-card-external">{{ row.protocol?.toUpperCase() }} {{ row.host_port }}</span>
                    <el-icon class="forward-card-arrow"><ArrowRight /></el-icon>
                    <span class="forward-card-internal">{{ row.dest_ip }}:{{ row.dest_port }}</span>
                  </div>
                  <div class="forward-card-meta">
                    <el-tag size="small" effect="plain" round>{{ row.protocol?.toUpperCase() }}</el-tag>
                    <el-tag :type="forwardProbeTagType(row)" size="small" effect="light">{{ forwardProbeText(row) }}</el-tag>
                  </div>
                </div>
                <div class="forward-card-body">
                  <div v-if="row.access_address" class="forward-card-info-row">
                    <span class="forward-card-label">访问地址</span>
                    <span class="forward-card-value">
                      <span class="code">{{ row.access_address }}</span>
                      <el-button type="primary" link size="small" :icon="CopyDocument" @click="copyForwardAccessAddress(row.access_address)" />
                    </span>
                  </div>
                  <div class="forward-card-info-row">
                    <span class="forward-card-label">归属用户</span>
                    <span class="forward-card-value">{{ row.owner_username || '—' }}</span>
                  </div>
                  <div class="forward-card-info-row">
                    <span class="forward-card-label">关联 VM</span>
                    <span class="forward-card-value">{{ row.vm_name || '—' }}</span>
                  </div>
                  <div class="forward-card-info-row">
                    <span class="forward-card-label">安全组</span>
                    <span class="forward-card-value"><el-tag type="success" size="small">自动补齐</el-tag></span>
                  </div>
                </div>
                <div class="forward-card-actions">
                  <el-button v-if="row.live" size="small" type="primary" @click="openForwardDialog(row)">编辑</el-button>
                  <el-button size="small" type="danger" @click="handleDeleteForward(row)">删除</el-button>
                </div>
              </el-card>
            </div>
          </div>
        </div>

        <!-- ACL 预览/应用 -->
        <div v-show="activeTab === 'acl'" class="tab-pane">
          <div class="table-toolbar">
            <div class="table-toolbar-left">
              <span class="table-title">VPC ACL 规则预览</span>
            </div>
            <div class="table-toolbar-right">
              <el-button :icon="Refresh" @click="loadACLPreview" :loading="aclLoading">刷新预览</el-button>
              <el-button type="warning" :icon="DocumentChecked" @click="handleApplyACL" :loading="aclApplying">应用 ACL</el-button>
            </div>
          </div>
          <div class="data-table-card">
            <div class="code-block-wrapper">
              <div class="code-block-header">
                <el-icon><Document /></el-icon>
                <span>nftables 规则</span>
                <el-button size="small" text :icon="CopyDocument" class="copy-btn" @click="copyACL">复制</el-button>
              </div>
              <pre class="code-block"><code>{{ aclPreview || '点击「刷新预览」加载规则' }}</code></pre>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 交换机对话框 -->
    <el-dialog
      v-model="switchDialogVisible"
      :title="editingSwitch ? '编辑交换机' : '创建交换机'"
      width="560px"
      append-to-body
      destroy-on-close
      class="modern-dialog"
    >
      <el-form :model="switchForm" label-width="140px" label-position="left">
        <el-form-item v-if="isAdmin" label="所属用户">
          <el-input v-model="switchForm.username" placeholder="留空使用筛选用户或当前管理员" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="switchForm.name" placeholder="请输入交换机名称" />
        </el-form-item>
        <el-form-item v-if="isAdmin" label="目标网桥">
          <el-select v-model="switchForm.bridge_name" style="width: 100%;" placeholder="选择目标网桥">
            <el-option v-for="item in bridges" :key="item.name" :label="bridgeOptionLabel(item)" :value="item.name" />
          </el-select>
          <div v-if="selectedSwitchBridge?.mode === 'bridge'" class="form-hint">桥接直通由上级路由器分配 IP，不启用内部 DHCP、NAT、安全组和端口转发。</div>
        </el-form-item>
        <el-form-item v-if="selectedSwitchBridge?.mode === 'bridge'" label="桥接 VLAN ID">
          <el-input-number v-model="switchForm.bridge_vlan_id" :min="0" :max="4094" style="width: 100%;" controls-position="right" />
          <div class="form-hint">0 表示不打 VLAN；填写 1-4094 时 VM 会以该 VLAN 接入上级网络。</div>
        </el-form-item>
        <template v-if="selectedSwitchBridge?.mode === 'bridge'">
          <el-divider content-position="left">桥接安全</el-divider>
          <el-form-item label="混杂模式">
            <el-switch v-model="switchForm.allow_promiscuous" active-text="允许" inactive-text="拒绝" />
            <div class="form-hint">拒绝时会对 VM 端口启用 no-flood，减少未知单播泛洪到该 VM。</div>
          </el-form-item>
          <el-form-item label="MAC 地址更改">
            <el-switch v-model="switchForm.allow_mac_change" active-text="允许" inactive-text="拒绝" />
            <div class="form-hint">拒绝时 VM 只能使用 XML 中配置的 MAC 作为源 MAC 发包。</div>
          </el-form-item>
          <el-form-item label="伪传输">
            <el-switch v-model="switchForm.allow_forged_transmits" active-text="允许" inactive-text="拒绝" />
            <div class="form-hint">拒绝时源 MAC 与配置 MAC 不一致的发包会被 OVS 丢弃。</div>
          </el-form-item>
        </template>
        <el-divider content-position="left">流量配额</el-divider>
        <el-form-item label="下行月配额(GB)">
          <el-input-number v-model="switchForm.traffic_down_gb" :min="quotaMinDown" :max="quotaMaxDown" style="width: 100%;" controls-position="right" />
          <div class="form-hint">用户下行总配额不限时可填 0，表示不限</div>
        </el-form-item>
        <el-form-item label="上行月配额(GB)">
          <el-input-number v-model="switchForm.traffic_up_gb" :min="quotaMinUp" :max="quotaMaxUp" style="width: 100%;" controls-position="right" />
          <div class="form-hint">用户上行总配额不限时可填 0，表示不限</div>
        </el-form-item>
        <el-divider content-position="left">带宽配额</el-divider>
        <el-form-item label="下行总带宽(Mbps)">
          <el-input-number v-model="switchForm.bandwidth_down_mbps" :min="bandwidthMinDown" :max="bandwidthMaxDown" style="width: 100%;" controls-position="right" />
          <div class="form-hint">用户下行带宽配额不限时可填 0，表示不限</div>
        </el-form-item>
        <el-form-item label="上行总带宽(Mbps)">
          <el-input-number v-model="switchForm.bandwidth_up_mbps" :min="bandwidthMinUp" :max="bandwidthMaxUp" style="width: 100%;" controls-position="right" />
          <div class="form-hint">用户上行带宽配额不限时可填 0，表示不限</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="switchDialogVisible = false" size="large">取消</el-button>
        <el-button type="primary" @click="submitSwitch" :loading="submitting" size="large">保存</el-button>
      </template>
    </el-dialog>

    <!-- 编辑端口转发对话框 -->
    <el-dialog
      v-model="forwardDialogVisible"
      title="编辑端口转发"
      width="460px"
      append-to-body
      destroy-on-close
      class="modern-dialog"
    >
      <el-form :model="forwardForm" label-width="110px" label-position="left">
        <el-form-item label="归属虚拟机">
          <el-input v-model="forwardForm.vm_name" placeholder="用于归属和手动 IP 映射" />
        </el-form-item>
        <el-form-item label="目标 IP">
          <el-input v-model="forwardForm.vm_ip" placeholder="请输入目标 IP" />
        </el-form-item>
        <el-form-item label="宿主机端口">
          <el-input v-model="forwardForm.host_port" placeholder="请输入宿主机端口" />
        </el-form-item>
        <el-form-item label="虚拟机端口">
          <el-input v-model="forwardForm.vm_port" placeholder="请输入虚拟机端口" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="forwardForm.protocol" style="width: 100%;">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="forwardDialogVisible = false" size="large">取消</el-button>
        <el-button type="primary" @click="submitForwardEdit" :loading="forwardSubmitting" size="large">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="userWhitelistDialogVisible" title="用户白名单" width="560px" append-to-body>
      <div class="whitelist-toolbar">
        <el-select
          v-model="userWhitelistForm.username"
          filterable
          clearable
          :loading="userOptionsLoading"
          placeholder="搜索并选择用户"
          style="flex: 1"
        >
          <el-option
            v-for="item in userOptions"
            :key="item.username"
            :label="item.label"
            :value="item.username"
          />
        </el-select>
        <el-button type="primary" @click="submitUserWhitelist" :loading="whitelistLoading">添加用户白名单</el-button>
      </div>
      <el-table :data="whitelistData.users" border size="small" v-loading="whitelistLoading">
        <el-table-column prop="scope_value" label="用户名" min-width="160" />
        <el-table-column prop="created_by" label="创建人" width="120" />
        <el-table-column prop="updated_at" label="更新时间" min-width="180" />
        <el-table-column label="操作" width="90" align="center">
          <template #default="{ row }">
            <el-button type="danger" link @click="handleDeleteUserWhitelist(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="vmWhitelistDialogVisible" title="虚拟机白名单" width="560px" append-to-body>
      <div class="whitelist-toolbar">
        <el-select
          v-model="vmWhitelistForm.vm_name"
          filterable
          clearable
          :loading="vmOptionsLoading"
          placeholder="搜索并选择虚拟机"
          style="flex: 1"
        >
          <el-option
            v-for="item in vmOptions"
            :key="item.name"
            :label="item.label"
            :value="item.name"
          />
        </el-select>
        <el-button type="primary" @click="submitVMWhitelist" :loading="whitelistLoading">添加虚拟机白名单</el-button>
      </div>
      <el-table :data="whitelistData.vms" border size="small" v-loading="whitelistLoading">
        <el-table-column prop="scope_value" label="虚拟机名称" min-width="180" />
        <el-table-column prop="created_by" label="创建人" width="120" />
        <el-table-column prop="updated_at" label="更新时间" min-width="180" />
        <el-table-column label="操作" width="90" align="center">
          <template #default="{ row }">
            <el-button type="danger" link @click="handleDeleteVMWhitelist(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 网桥对话框 -->
    <el-dialog
      v-model="bridgeDialogVisible"
      title="创建桥接网桥"
      width="560px"
      append-to-body
      destroy-on-close
      class="modern-dialog"
    >
      <el-alert type="warning" show-icon :closable="false" title="桥接承载默认路由的物理网卡可能短暂中断宿主机网络，请确认已具备回滚方式。" style="margin-bottom: 16px;" />
      <el-form :model="bridgeForm" label-width="130px" label-position="left">
        <el-form-item label="网桥名称" required>
          <el-input v-model="bridgeForm.name" maxlength="15" placeholder="例如 brpub0" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="bridgeForm.mode" style="width: 100%;">
            <el-option label="桥接直通" value="bridge" />
          </el-select>
        </el-form-item>
        <el-form-item label="物理网卡" required>
          <el-select v-model="bridgeForm.uplink_if" style="width: 100%;" filterable placeholder="选择物理网卡">
            <el-option v-for="item in hostInterfaces" :key="item.name" :label="interfaceOptionLabel(item)" :value="item.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="迁移宿主机 IP">
          <el-switch v-model="bridgeForm.migrate_host_ip" />
          <div class="form-hint">网卡当前承载管理 IP 或默认路由时通常需要开启，否则宿主机 IP 仍留在物理网卡上。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bridgeDialogVisible = false" size="large">取消</el-button>
        <el-button type="primary" @click="submitBridge" :loading="submitting" size="large">保存</el-button>
      </template>
    </el-dialog>

    <!-- 安全组对话框 -->
    <el-dialog
      v-model="groupDialogVisible"
      :title="groupDialogTitle"
      width="500px"
      append-to-body
      destroy-on-close
      @closed="resetGroupDialog"
      class="modern-dialog"
    >
      <el-form :model="groupForm" label-width="100px" label-position="left">
        <el-form-item v-if="isAdmin" label="所属用户">
          <el-input v-model="groupForm.username" :disabled="!!editingGroup" placeholder="留空使用筛选用户或当前管理员" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="groupForm.name" :disabled="!!editingGroup?.is_default" placeholder="请输入安全组名称" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="groupForm.remark" type="textarea" :rows="2" placeholder="请输入备注信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupDialogVisible = false" size="large">取消</el-button>
        <el-button type="primary" @click="submitGroup" :loading="submitting" size="large">保存</el-button>
      </template>
    </el-dialog>

    <!-- 规则对话框 -->
    <el-dialog
      v-model="ruleDialogVisible"
      :title="`添加规则 — ${selectedGroup?.name || ''}`"
      width="600px"
      append-to-body
      destroy-on-close
      class="modern-dialog"
    >
      <el-form :model="ruleForm" label-width="110px" label-position="left">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="方向">
              <el-select v-model="ruleForm.direction" style="width: 100%;" placeholder="选择方向">
                <el-option label="入站" value="ingress" />
                <el-option label="出站" value="egress" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="协议">
              <el-select v-model="ruleForm.protocol" style="width: 100%;" placeholder="选择协议">
                <el-option label="TCP" value="tcp" />
                <el-option label="UDP" value="udp" />
                <el-option label="ICMP" value="icmp" />
                <el-option label="全部" value="all" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="起始端口">
              <el-input-number v-model="ruleForm.port_start" :min="0" :max="65535" style="width: 100%;" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="结束端口">
              <el-input-number v-model="ruleForm.port_end" :min="0" :max="65535" style="width: 100%;" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="目标类型">
          <el-select v-model="ruleForm.target_type" style="width: 100%;" @change="onRuleTargetTypeChange">
            <el-option label="CIDR / IP 地址" value="cidr" />
            <el-option label="指定交换机" value="switch" />
            <el-option label="指定安全组" value="security_group" />
          </el-select>
        </el-form-item>
        <el-form-item label="目标值">
          <el-input
            v-show="ruleForm.target_type === 'cidr'"
            v-model="ruleForm.target_value"
            placeholder="例如 0.0.0.0/0、192.168.1.10 或 10.200.1.0/24"
          />
          <el-select
            v-show="ruleForm.target_type === 'switch'"
            v-model="ruleForm.target_value"
            style="width: 100%;"
            filterable
            placeholder="选择允许访问的交换机"
            no-data-text="当前用户没有可选交换机"
          >
            <el-option v-for="item in ruleSwitchOptions" :key="item.id" :label="switchOptionLabel(item)" :value="String(item.id)" />
          </el-select>
          <el-select
            v-show="ruleForm.target_type === 'security_group'"
            v-model="ruleForm.target_value"
            style="width: 100%;"
            filterable
            placeholder="选择允许访问的安全组"
            no-data-text="当前用户没有可选安全组"
          >
            <el-option v-for="item in ruleSecurityGroupOptions" :key="item.id" :label="groupOptionLabel(item)" :value="String(item.id)" />
          </el-select>
          <div class="form-hint">{{ ruleTargetHelp }}</div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="ruleForm.remark" type="textarea" :rows="2" placeholder="规则说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialogVisible = false" size="large">取消</el-button>
        <el-button type="primary" @click="submitRule" :loading="submitting" size="large">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, watch, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '@/store/user'
import { getUserList } from '@/api/user'
import { getVmList } from '@/api/vm'
import {
  createNetworkBridge,
  deleteNetworkBridge,
  getHostInterfaces,
  getNetworkBridges,
  getPortForwardList,
  updatePortForward,
  deletePortForward,
  deletePortForwardByRuleKey,
  batchDeletePortForward,
  runPortForwardHTTPProbe,
  getPortForwardWhitelistList,
  addPortForwardUserWhitelist,
  deletePortForwardUserWhitelist,
  addPortForwardVMWhitelist,
  deletePortForwardVMWhitelist
} from '@/api/network'
import { checkOVSNetwork, repairOVSNetwork } from '@/api/ovs'
import {
  addVPCSecurityGroupRule,
  applyVPCACL,
  createVPCSecurityGroup,
  createVPCSwitch,
  deleteVPCSecurityGroup,
  deleteVPCSecurityGroupRule,
  deleteVPCSwitch,
  getVPCQuota,
  getVPCSecurityGroups,
  getVPCSwitches,
  previewVPCACL,
  resetVPCSwitchTraffic,
  updateVPCSecurityGroup,
  updateVPCSwitch
} from '@/api/vpc'
import {
  Connection, Monitor, Setting, List, Cpu, Lock, Link,
  View, Tools, Refresh, Plus, Search, Document, DocumentChecked,
  Download, Upload, CircleCheckFilled, WarningFilled, CopyDocument, ArrowRight
} from '@element-plus/icons-vue'
import { copyTextWithFallback } from '@/utils/clipboard'

const userStore = useUserStore()
const isAdmin = computed(() => userStore.role === 'admin')
const forwardBannedReason = '检测到存在建站或HTTP访问且未报备，当前转发已封禁，请联系管理员'

const visibleTabs = computed(() => {
  const tabs = []
  if (isAdmin.value) tabs.push({ name: 'overview', label: '网络概览', icon: 'DataAnalysis' })
  tabs.push({ name: 'switches', label: '交换机', icon: 'Connection' })
  tabs.push({ name: 'securityGroups', label: '安全组策略', icon: 'Lock' })
  if (isAdmin.value) tabs.push({ name: 'forwards', label: '端口转发', icon: 'Sort' })
  if (isAdmin.value) tabs.push({ name: 'acl', label: 'ACL', icon: 'Document' })
  return tabs
})

const activeTab = ref(isAdmin.value ? 'overview' : 'switches')
const usernameFilter = ref('')
const loading = ref(false)
const submitting = ref(false)

const status = ref(null)
const ports = ref(null)
const checking = ref(false)
const repairing = ref(false)

const switches = ref([])
const switchSearchText = ref('')
const switchSubnetSearch = ref('')
const bridges = ref([])
const hostInterfaces = ref([])
const quota = ref(null)
const securityGroups = ref([])
const sgSearchText = ref('')
const sgTypeFilter = ref(null)
const forwardRules = ref([])
const forwardSearchText = ref('')
const forwardVmSearch = ref('')
const forwardProtocolFilter = ref('')
const selectedForwardIds = ref([])
const aclPreview = ref('')
const aclLoading = ref(false)
const aclApplying = ref(false)
const forwardDialogVisible = ref(false)
const forwardSubmitting = ref(false)
const forwardForm = reactive({ id: null, vm_name: '', vm_ip: '', host_port: '', vm_port: '', protocol: 'tcp' })
const whitelistLoading = ref(false)
const userOptionsLoading = ref(false)
const vmOptionsLoading = ref(false)
const userWhitelistDialogVisible = ref(false)
const vmWhitelistDialogVisible = ref(false)
const whitelistData = ref({ users: [], vms: [] })
const userOptions = ref([])
const vmOptions = ref([])
const userWhitelistForm = reactive({ username: '' })
const vmWhitelistForm = reactive({ vm_name: '' })
const probeSubmitting = ref(false)

const switchDialogVisible = ref(false)
const editingSwitch = ref(null)
const switchForm = reactive({ username: '', name: '', bridge_name: 'br-ovs', bridge_vlan_id: 0, allow_promiscuous: false, allow_mac_change: false, allow_forged_transmits: false, traffic_down_gb: 0, traffic_up_gb: 0, bandwidth_down_mbps: 0, bandwidth_up_mbps: 0, bandwidth_mbps: 0 })

const bridgeDialogVisible = ref(false)
const bridgeForm = reactive({ name: '', mode: 'bridge', uplink_if: '', migrate_host_ip: true })

const groupDialogVisible = ref(false)
const editingGroup = ref(null)
const groupForm = reactive({ username: '', name: '', remark: '' })

const ruleDialogVisible = ref(false)
const selectedGroup = ref(null)
const ruleForm = reactive({ direction: 'ingress', protocol: 'tcp', port_start: 0, port_end: 0, target_type: 'cidr', target_value: '0.0.0.0/0', remark: '' })

const params = computed(() => isAdmin.value && usernameFilter.value ? { username: usernameFilter.value } : {})

const switchPageSize = ref(100)
const switchCurrentPage = ref(1)
const sgPageSize = ref(100)
const sgCurrentPage = ref(1)
const forwardPageSize = ref(100)
const forwardCurrentPage = ref(1)

const filteredSwitches = computed(() => {
  let data = switches.value
  if (switchSearchText.value) {
    const q = switchSearchText.value.toLowerCase()
    data = data.filter(s => s.name.toLowerCase().includes(q))
  }
  if (switchSubnetSearch.value) {
    const q = switchSubnetSearch.value.toLowerCase()
    data = data.filter(s => (s.cidr || '').toLowerCase().includes(q))
  }
  return data
})

const paginatedSwitches = computed(() => {
  const start = (switchCurrentPage.value - 1) * switchPageSize.value
  return filteredSwitches.value.slice(start, start + switchPageSize.value)
})

const filteredSecurityGroups = computed(() => {
  let data = securityGroups.value
  if (sgSearchText.value) {
    const q = sgSearchText.value.toLowerCase()
    data = data.filter(g => g.name.toLowerCase().includes(q))
  }
  if (sgTypeFilter.value !== null) {
    data = data.filter(g => g.is_default === sgTypeFilter.value)
  }
  return data
})

const paginatedSecurityGroups = computed(() => {
  const start = (sgCurrentPage.value - 1) * sgPageSize.value
  return filteredSecurityGroups.value.slice(start, start + sgPageSize.value)
})

const filteredForwardRules = computed(() => {
  let data = forwardRules.value
  if (forwardSearchText.value) {
    const q = forwardSearchText.value.toLowerCase()
    data = data.filter(r => String(r.host_port).toLowerCase().includes(q))
  }
  if (forwardVmSearch.value) {
    const q = forwardVmSearch.value.toLowerCase()
    data = data.filter(r => (r.vm_name || '').toLowerCase().includes(q))
  }
  if (forwardProtocolFilter.value) {
    data = data.filter(r => r.protocol === forwardProtocolFilter.value)
  }
  return data
})

const paginatedForwardRules = computed(() => {
  const start = (forwardCurrentPage.value - 1) * forwardPageSize.value
  return filteredForwardRules.value.slice(start, start + forwardPageSize.value)
})

const defaultTrafficDown = computed(() => quota.value && quota.value.max_traffic_down > 0 ? quota.value.remaining_traffic_down : 0)
const defaultTrafficUp = computed(() => quota.value && quota.value.max_traffic_up > 0 ? quota.value.remaining_traffic_up : 0)
const defaultBandwidthDown = computed(() => quota.value && quota.value.max_bandwidth_down > 0 ? quota.value.remaining_bandwidth_down : 0)
const defaultBandwidthUp = computed(() => quota.value && quota.value.max_bandwidth_up > 0 ? quota.value.remaining_bandwidth_up : 0)

const quotaMinDown = computed(() => editingSwitch.value ? -defaultTrafficDown.value : 0)
const quotaMaxDown = computed(() => quota.value && quota.value.max_traffic_down > 0 ? quota.value.remaining_traffic_down : 999999)
const quotaMinUp = computed(() => editingSwitch.value ? -defaultTrafficUp.value : 0)
const quotaMaxUp = computed(() => quota.value && quota.value.max_traffic_up > 0 ? quota.value.remaining_traffic_up : 999999)
const bandwidthMinDown = computed(() => editingSwitch.value ? -defaultBandwidthDown.value : 0)
const bandwidthMaxDown = computed(() => quota.value && quota.value.max_bandwidth_down > 0 ? quota.value.remaining_bandwidth_down : 999999)
const bandwidthMinUp = computed(() => editingSwitch.value ? -defaultBandwidthUp.value : 0)
const bandwidthMaxUp = computed(() => quota.value && quota.value.max_bandwidth_up > 0 ? quota.value.remaining_bandwidth_up : 999999)

const ruleSwitchOptions = computed(() => switches.value.map(item => ({ id: item.id, name: item.name, cidr: item.cidr })))
const ruleSecurityGroupOptions = computed(() => securityGroups.value.map(item => ({ id: item.id, name: item.name })))
const selectedSwitchBridge = computed(() => bridges.value.find(item => item.name === switchForm.bridge_name))
const groupDialogTitle = computed(() => editingGroup.value ? '编辑安全组' : '创建安全组')

const ruleTargetOptions = computed(() => {
  if (ruleForm.target_type === 'switch') return ruleSwitchOptions.value
  if (ruleForm.target_type === 'security_group') return ruleSecurityGroupOptions.value
  return []
})

const ruleTargetHelp = computed(() => {
  if (ruleForm.target_type === 'cidr') return '支持 IPv4 地址或 CIDR 格式，如 0.0.0.0/0 表示所有'
  if (ruleForm.target_type === 'switch') return '选择当前用户可访问的交换机'
  if (ruleForm.target_type === 'security_group') return '选择当前用户拥有的安全组'
  return ''
})

function yesNo(val) { return val ? '是' : '否' }
function switchQuotaText(val) { return !val || val <= 0 ? '不限' : val + ' GB' }
function formatBytes(bytes) {
  const value = Number(bytes) || 0
  if (value < 1024) return `${value} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = value / 1024
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unitIndex]}`
}
function switchTrafficText(row, direction) {
  const usedText = direction === 'down'
    ? (row.used_traffic_down_gb || formatBytes(row.used_traffic_down))
    : (row.used_traffic_up_gb || formatBytes(row.used_traffic_up))
  const quota = direction === 'down' ? row.traffic_down_gb : row.traffic_up_gb
  return `${usedText} / ${switchQuotaText(quota)}`
}
function switchTrafficPercent(row, direction) {
  const quota = Number(direction === 'down' ? row.traffic_down_gb : row.traffic_up_gb) || 0
  if (quota <= 0) return 0
  const usedBytes = Number(direction === 'down' ? row.used_traffic_down : row.used_traffic_up) || 0
  const quotaBytes = quota * 1024 * 1024 * 1024
  return Math.min(Math.round((usedBytes / quotaBytes) * 100), 100)
}
function switchBandwidthText(val) { return !val || val <= 0 ? '不限' : val + ' Mbps' }
function switchLimitText(row) { return (row.is_limited_down || row.is_limited_up) ? '已限速' : '正常' }
function bridgeModeText(val) { return val === 'bridge' ? '桥接直通' : '内网 NAT' }
function bridgeOptionLabel(item) { return `${item.name} - ${bridgeModeText(item.mode)}` }
function interfaceOptionLabel(item) { return `${item.name}${item.default_route ? '（默认路由）' : ''}${item.addresses?.length ? ' - ' + item.addresses.join(', ') : ''}` }
function directionText(val) { return val === 'ingress' ? '入站' : val === 'egress' ? '出站' : val }
function portText(rule) {
  if (!rule.port_start && !rule.port_end) return '全部'
  if (rule.port_start === rule.port_end) return String(rule.port_start || '0')
  return `${rule.port_start || 0}-${rule.port_end || 65535}`
}
function targetText(rule) {
  if (rule.target_type === 'switch') return `交换机: ${rule.target_name || rule.target_value || '-'}`
  if (rule.target_type === 'security_group') return `安全组: ${rule.target_name || rule.target_value || '-'}`
  return rule.target_value || '0.0.0.0/0'
}
function switchOptionLabel(item) { return `${item.name} (${item.cidr})` }
function groupOptionLabel(item) { return item.name }
function trafficQuotaText(remaining, max) {
  if (!max || max === -1) return '不限'
  if (remaining === -1) return '不限'
  return remaining + ' GB'
}
function bandwidthQuotaText(remaining, max) {
  if (!max || max === -1) return '不限'
  if (remaining === -1) return '不限'
  return remaining + ' Mbps'
}

function switchTab(name) {
  activeTab.value = name
  if (name === 'forwards') loadForwards()
  if (name === 'acl') loadACLPreview()
  if (name === 'overview') loadOverview()
}

async function loadOverview() {
  const [checkRes, bridgeRes, ifaceRes] = await Promise.all([
    checkOVSNetwork(),
    getNetworkBridges(),
    getHostInterfaces()
  ])
  status.value = checkRes.data?.status
  ports.value = checkRes.data?.ports
  bridges.value = bridgeRes.data || []
  hostInterfaces.value = ifaceRes.data || []
}

async function loadSwitches() {
  const [switchRes, quotaRes, bridgeRes] = await Promise.all([
    getVPCSwitches(params.value),
    getVPCQuota(params.value),
    isAdmin.value ? getNetworkBridges() : Promise.resolve({ data: [] })
  ])
  switches.value = switchRes.data || []
  quota.value = quotaRes.data
  if (isAdmin.value) bridges.value = bridgeRes.data || bridges.value
}

async function loadSecurityGroups() {
  const res = await getVPCSecurityGroups(params.value)
  securityGroups.value = res.data || []
}

async function loadForwards() {
  if (!isAdmin.value) return
  const [forwardRes, whitelistRes] = await Promise.all([
    getPortForwardList(),
    getPortForwardWhitelistList()
  ])
  forwardRules.value = forwardRes.data || []
  whitelistData.value = whitelistRes.data || { users: [], vms: [] }
  selectedForwardIds.value = selectedForwardIds.value.filter(id => forwardRules.value.some(rule => rule.id === id))
}

async function loadACLPreview() {
  if (!isAdmin.value) return
  aclLoading.value = true
  try {
    const res = await previewVPCACL()
    aclPreview.value = res.data || ''
  } finally {
    aclLoading.value = false
  }
}

async function loadAll() {
  loading.value = true
  try {
    const jobs = [loadSwitches(), loadSecurityGroups()]
    if (isAdmin.value) {
      if (activeTab.value === 'overview') jobs.push(loadOverview())
      if (activeTab.value === 'forwards') jobs.push(loadForwards())
      if (activeTab.value === 'acl') jobs.push(loadACLPreview())
    }
    await Promise.all(jobs)
  } finally {
    loading.value = false
  }
}

async function handleCheck() {
  checking.value = true
  try {
    const res = await checkOVSNetwork()
    status.value = res.data?.status
    ports.value = res.data?.ports
    ElMessage.success(res.data?.healthy ? '网络检测通过' : '网络检测完成')
  } finally {
    checking.value = false
  }
}

async function handleRepair() {
  await ElMessageBox.confirm('修复会补齐 OVS 网桥、dnsmasq、ip_forward、NAT 和 FORWARD 规则，确认继续？', '高风险操作', { type: 'warning' })
  repairing.value = true
  try {
    const res = await repairOVSNetwork()
    ElMessage.success(res.message || '修复任务已提交')
  } finally {
    repairing.value = false
  }
}

function openSwitchDialog(row) {
  if (row?.is_system) {
    ElMessage.warning('系统基础网络交换机仅供查看，不可编辑')
    return
  }
  editingSwitch.value = row || null
  const legacyBandwidth = row?.bandwidth_mbps || 0
  Object.assign(switchForm, {
    username: row?.username || usernameFilter.value || '',
    name: row?.name || '',
    bridge_name: row?.bridge_name || bridges.value[0]?.name || 'br-ovs',
    bridge_vlan_id: row?.bridge_vlan_id || 0,
    allow_promiscuous: !!row?.allow_promiscuous,
    allow_mac_change: !!row?.allow_mac_change,
    allow_forged_transmits: !!row?.allow_forged_transmits,
    traffic_down_gb: row ? (row.traffic_down_gb ?? 0) : defaultTrafficDown.value,
    traffic_up_gb: row ? (row.traffic_up_gb ?? 0) : defaultTrafficUp.value,
    bandwidth_down_mbps: row ? (row.bandwidth_down_mbps ?? legacyBandwidth) : defaultBandwidthDown.value,
    bandwidth_up_mbps: row ? (row.bandwidth_up_mbps ?? legacyBandwidth) : defaultBandwidthUp.value,
    bandwidth_mbps: 0
  })
  switchDialogVisible.value = true
}

function forwardRowSelectable(row) {
  return !!row?.live
}

function forwardProbeText(row) {
  if (!row) return '未知'
  return (!row.live && row.banned) ? '封禁' : '正常'
}

function forwardProbeTagType(row) {
  if (!row) return 'info'
  return (!row.live && row.banned) ? 'danger' : 'success'
}

function forwardProbeReason(row) {
  if (!row) return ''
  if (row.banned) return forwardBannedReason
  return ''
}

async function loadWhitelistList() {
  if (!isAdmin.value) return
  whitelistLoading.value = true
  try {
    const res = await getPortForwardWhitelistList()
    whitelistData.value = res.data || { users: [], vms: [] }
  } finally {
    whitelistLoading.value = false
  }
}

async function loadUserOptions() {
  if (!isAdmin.value) return
  userOptionsLoading.value = true
  try {
    const res = await getUserList()
    userOptions.value = (res.data || [])
      .filter(item => item.role !== 'admin')
      .map(item => ({
        username: item.username,
        label: item.email ? `${item.username} (${item.email})` : item.username
      }))
  } finally {
    userOptionsLoading.value = false
  }
}

async function loadVMOptions() {
  if (!isAdmin.value) return
  vmOptionsLoading.value = true
  try {
    const res = await getVmList()
    vmOptions.value = (res.data || []).map(item => ({
      name: item.name,
      label: item.name
    }))
  } finally {
    vmOptionsLoading.value = false
  }
}

function openUserWhitelistDialog() {
  userWhitelistForm.username = ''
  userWhitelistDialogVisible.value = true
  loadWhitelistList()
  loadUserOptions()
}

function openVMWhitelistDialog() {
  vmWhitelistForm.vm_name = ''
  vmWhitelistDialogVisible.value = true
  loadWhitelistList()
  loadVMOptions()
}

async function submitUserWhitelist() {
  const username = String(userWhitelistForm.username || '').trim()
  if (!username) {
    ElMessage.warning('请输入用户名')
    return
  }
  whitelistLoading.value = true
  try {
    const res = await addPortForwardUserWhitelist({ username })
    ElMessage.success(res.message || '用户白名单已保存')
    const warnings = res.data?.warnings || []
    if (warnings.length) {
      ElMessage.warning(warnings.join('；'))
    }
    userWhitelistForm.username = ''
    await Promise.all([loadWhitelistList(), loadForwards()])
  } finally {
    whitelistLoading.value = false
  }
}

async function handleDeleteUserWhitelist(row) {
  await ElMessageBox.confirm(`确定删除用户白名单 ${row.scope_value}？`, '删除用户白名单', { type: 'warning' })
  await deletePortForwardUserWhitelist(row.scope_value)
  ElMessage.success('用户白名单已删除')
  await loadWhitelistList()
}

async function submitVMWhitelist() {
  const vmName = String(vmWhitelistForm.vm_name || '').trim()
  if (!vmName) {
    ElMessage.warning('请输入虚拟机名称')
    return
  }
  whitelistLoading.value = true
  try {
    const res = await addPortForwardVMWhitelist({ vm_name: vmName })
    ElMessage.success(res.message || '虚拟机白名单已保存')
    const warnings = res.data?.warnings || []
    if (warnings.length) {
      ElMessage.warning(warnings.join('；'))
    }
    vmWhitelistForm.vm_name = ''
    await Promise.all([loadWhitelistList(), loadForwards()])
  } finally {
    whitelistLoading.value = false
  }
}

async function handleDeleteVMWhitelist(row) {
  await ElMessageBox.confirm(`确定删除虚拟机白名单 ${row.scope_value}？`, '删除虚拟机白名单', { type: 'warning' })
  await deletePortForwardVMWhitelist(row.scope_value)
  ElMessage.success('虚拟机白名单已删除')
  await loadWhitelistList()
}

async function handleRunForwardProbe() {
  probeSubmitting.value = true
  try {
    const res = await runPortForwardHTTPProbe()
    ElMessage.success(res.message || '端口转发 HTTP 探测任务已提交')
  } finally {
    probeSubmitting.value = false
  }
}

function openBridgeDialog() {
  Object.assign(bridgeForm, {
    name: '',
    mode: 'bridge',
    uplink_if: hostInterfaces.value.find(item => !item.ovs_port && !item.managed_bridge)?.name || hostInterfaces.value[0]?.name || '',
    migrate_host_ip: true
  })
  bridgeDialogVisible.value = true
}

async function submitBridge() {
  submitting.value = true
  try {
    await createNetworkBridge(bridgeForm)
    ElMessage.success('网桥已创建')
    bridgeDialogVisible.value = false
    await loadOverview()
  } finally {
    submitting.value = false
  }
}

async function handleDeleteBridge(row) {
  await ElMessageBox.confirm(`确定删除网桥 ${row.name}？`, '删除网桥', { type: 'warning' })
  await deleteNetworkBridge(row.id)
  ElMessage.success('网桥已删除')
  await loadOverview()
}

async function submitSwitch() {
  submitting.value = true
  try {
    if (editingSwitch.value) {
      await updateVPCSwitch(editingSwitch.value.id, switchForm)
      ElMessage.success('交换机已更新')
    } else {
      await createVPCSwitch(switchForm)
      ElMessage.success('交换机已创建')
    }
    switchDialogVisible.value = false
    await loadSwitches()
  } finally {
    submitting.value = false
  }
}

async function handleDeleteSwitch(row) {
  await ElMessageBox.confirm(`确定删除交换机 ${row.name}？`, '删除交换机', { type: 'warning' })
  await deleteVPCSwitch(row.id)
  ElMessage.success('交换机已删除')
  await loadSwitches()
}

async function handleResetSwitchTraffic(row) {
  await ElMessageBox.confirm(`确定将交换机 ${row.name} 的本月流量计数值重置？若当前因超限被强制限速，会立即解除。`, '重置流量计数器', { type: 'warning' })
  await resetVPCSwitchTraffic(row.id)
  ElMessage.success('交换机流量计数器已重置')
  await loadSwitches()
}

function openGroupDialog(row = null) {
  const group = row && typeof row === 'object' && 'id' in row ? row : null
  editingGroup.value = group
  Object.assign(groupForm, {
    username: group?.username || usernameFilter.value || '',
    name: group?.name || '',
    remark: group?.remark || ''
  })
  groupDialogVisible.value = true
}

function resetGroupDialog() {
  editingGroup.value = null
  Object.assign(groupForm, {
    username: usernameFilter.value || '',
    name: '',
    remark: ''
  })
}

async function submitGroup() {
  submitting.value = true
  try {
    if (editingGroup.value) {
      await updateVPCSecurityGroup(editingGroup.value.id, groupForm)
      ElMessage.success('安全组已更新')
    } else {
      await createVPCSecurityGroup(groupForm)
      ElMessage.success('安全组已创建')
    }
    groupDialogVisible.value = false
    await loadSecurityGroups()
  } finally {
    submitting.value = false
  }
}

async function handleDeleteGroup(row) {
  await ElMessageBox.confirm(`确定删除安全组 ${row.name}？`, '删除安全组', { type: 'warning' })
  await deleteVPCSecurityGroup(row.id)
  ElMessage.success('安全组已删除')
  await loadSecurityGroups()
}

function openRuleDialog(group) {
  selectedGroup.value = group
  Object.assign(ruleForm, { direction: 'ingress', protocol: 'tcp', port_start: 0, port_end: 0, target_type: 'cidr', target_value: '0.0.0.0/0', remark: '' })
  ruleDialogVisible.value = true
}

function onRuleTargetTypeChange(type) {
  if (type === 'cidr') {
    ruleForm.target_value = '0.0.0.0/0'
    return
  }
  ruleForm.target_value = ruleTargetOptions.value[0]?.id ? String(ruleTargetOptions.value[0].id) : ''
}

async function submitRule() {
  if (!selectedGroup.value) return
  if (ruleForm.target_type !== 'cidr' && !ruleForm.target_value) {
    onRuleTargetTypeChange(ruleForm.target_type)
  }
  if (!ruleForm.target_value) {
    ElMessage.warning(ruleForm.target_type === 'cidr' ? '请填写 CIDR/IP' : '请选择目标值')
    return
  }
  if (ruleForm.target_type === 'switch' && !ruleSwitchOptions.value.some(item => String(item.id) === String(ruleForm.target_value))) {
    ElMessage.warning('请选择当前用户可用的交换机')
    return
  }
  if (ruleForm.target_type === 'security_group' && !ruleSecurityGroupOptions.value.some(item => String(item.id) === String(ruleForm.target_value))) {
    ElMessage.warning('请选择当前用户可用的安全组')
    return
  }
  submitting.value = true
  try {
    await addVPCSecurityGroupRule(selectedGroup.value.id, ruleForm)
    ElMessage.success('规则已添加')
    ruleDialogVisible.value = false
    await loadSecurityGroups()
  } finally {
    submitting.value = false
  }
}

async function handleDeleteRule(rule) {
  await deleteVPCSecurityGroupRule(rule.id)
  ElMessage.success('规则已删除')
  await loadSecurityGroups()
}

async function handleApplyACL() {
  await ElMessageBox.confirm('应用 ACL 会重建 VPC 防火墙规则，确认继续？', '高风险操作', { type: 'warning' })
  aclApplying.value = true
  try {
    const res = await applyVPCACL()
    ElMessage.success(res.message || 'ACL 已应用')
    await loadACLPreview()
  } finally {
    aclApplying.value = false
  }
}

async function copyACL() {
  if (aclPreview.value) {
    try {
      await copyTextWithFallback(aclPreview.value)
      ElMessage.success('ACL 规则已复制到剪贴板')
    } catch {
      ElMessage.warning('复制失败，请手动复制')
    }
  }
}

async function copyForwardAccessAddress(value) {
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

function handleForwardSelectionChange(rows) {
  selectedForwardIds.value = (rows || []).map(item => item.id)
}

function openForwardDialog(row) {
  Object.assign(forwardForm, {
    id: row.id,
    vm_name: row.vm_name || '',
    vm_ip: row.dest_ip || '',
    host_port: row.host_port || '',
    vm_port: row.dest_port || '',
    protocol: String(row.protocol || 'tcp').toLowerCase()
  })
  forwardDialogVisible.value = true
}

async function submitForwardEdit() {
  if (!forwardForm.vm_ip || !forwardForm.host_port || !forwardForm.vm_port) {
    ElMessage.warning('请完整填写目标 IP、宿主机端口和虚拟机端口')
    return
  }
  forwardSubmitting.value = true
  try {
    await updatePortForward(forwardForm.id, {
      vm_name: forwardForm.vm_name,
      vm_ip: forwardForm.vm_ip,
      host_port: forwardForm.host_port,
      vm_port: forwardForm.vm_port,
      protocol: forwardForm.protocol
    })
    ElMessage.success('端口转发规则已更新')
    forwardDialogVisible.value = false
    await loadForwards()
  } finally {
    forwardSubmitting.value = false
  }
}

async function handleDeleteForward(row) {
  await ElMessageBox.confirm(`确定删除端口转发 ${row.access_address || row.host_port} → ${row.dest_ip}:${row.dest_port}？`, '删除端口转发', { type: 'warning' })
  if (row.live) {
    await deletePortForward(row.id)
    ElMessage.success('端口转发规则已删除')
  } else {
    await deletePortForwardByRuleKey(row.rule_key)
    ElMessage.success('封禁端口转发记录已删除')
  }
  await loadForwards()
}

async function handleBatchDeleteForward() {
  if (selectedForwardIds.value.length === 0) {
    ElMessage.warning('请先选择要删除的端口转发规则')
    return
  }
  await ElMessageBox.confirm(`确定批量删除已选中的 ${selectedForwardIds.value.length} 条端口转发规则？`, '批量删除端口转发', { type: 'warning' })
  await batchDeletePortForward({ ids: selectedForwardIds.value })
  selectedForwardIds.value = []
  ElMessage.success('端口转发规则已批量删除')
  await loadForwards()
}

watch([switchSearchText, switchSubnetSearch], () => {
  switchCurrentPage.value = 1
})
watch([sgSearchText, sgTypeFilter], () => {
  sgCurrentPage.value = 1
})
watch([forwardSearchText, forwardVmSearch, forwardProtocolFilter], () => {
  forwardCurrentPage.value = 1
})

onMounted(loadAll)
</script>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.network-page {
  padding: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 24px 24px 20px;
  background: var(--el-bg-color);
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.page-header-left .page-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}

.page-header-left .page-icon {
  font-size: 24px;
  color: var(--el-color-primary);
}

.page-header-left h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.page-header-left p {
  margin: 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.page-header-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

/* 自定义标签页 */
.tabs-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.custom-tabs {
  display: flex;
  gap: 0;
  padding: 0 24px;
  background: var(--el-bg-color);
  border-bottom: 1px solid var(--el-border-color-lighter);
  overflow-x: auto;
}

.custom-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 14px 20px;
  font-size: 14px;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
  white-space: nowrap;
  user-select: none;
  position: relative;
}

.custom-tab:hover {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.custom-tab.active {
  color: var(--el-color-primary);
  font-weight: 600;
  border-bottom-color: var(--el-color-primary);
}

.tab-content {
  flex: 1;
  overflow-y: auto;
  padding: 20px 24px 24px;
  background: var(--el-bg-color-page);
}

.tab-pane {
  animation: fadeIn 0.25s ease;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 网络概览统计卡片 */
.overview-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 20px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  transition: all 0.25s;
}

.stat-card:hover {
  border-color: var(--el-color-primary-light-5);
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.stat-card.healthy .stat-card-icon {
  color: #67C23A;
}

.stat-card.warning .stat-card-icon {
  color: #E6A23C;
}

.stat-card-icon {
  color: var(--el-color-primary);
  flex-shrink: 0;
}

.stat-card-body {
  min-width: 0;
}

.stat-card-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}

.stat-card-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stat-card-value.code {
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
  font-size: 14px;
}

.overview-actions-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
}

/* 信息卡片 */
.info-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  margin-bottom: 20px;
  overflow: hidden;
}

.info-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 20px;
  font-weight: 600;
  font-size: 14px;
  color: var(--el-text-color-primary);
  background: var(--el-bg-color-page);
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.info-card-header .el-icon {
  color: var(--el-color-primary);
}

.info-list {
  padding: 4px 0;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 20px;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.info-item:last-child {
  border-bottom: none;
}

.info-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
}

.info-value {
  font-size: 13px;
  color: var(--el-text-color-primary);
  text-align: right;
  word-break: break-all;
}

.code {
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
  font-size: 13px;
  background: var(--el-fill-color-light);
  padding: 2px 8px;
  border-radius: 4px;
}

.text-muted {
  color: var(--el-text-color-placeholder);
}

.text-danger {
  color: #F56C6C;
  font-weight: 600;
}

/* 配额概览卡片 */
.quota-summary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 20px;
}

.quota-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  transition: all 0.25s;
}

.quota-card:hover {
  border-color: var(--el-color-primary-light-5);
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.quota-card-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.quota-card-icon.down {
  background: rgba(64, 158, 255, 0.1);
  color: #409EFF;
}

.quota-card-icon.up {
  background: rgba(103, 194, 58, 0.1);
  color: #67C23A;
}

.quota-card-icon.down-bandwidth {
  background: rgba(230, 162, 60, 0.1);
  color: #E6A23C;
}

.quota-card-icon.up-bandwidth {
  background: rgba(245, 108, 108, 0.1);
  color: #F56C6C;
}

.quota-card-body {
  min-width: 0;
}

.quota-card-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 2px;
}

.quota-card-value {
  font-size: 18px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}

.quota-card-sub {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  margin-top: 2px;
}

/* 表格工具栏 */
.table-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.table-toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.table-toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.table-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

/* 数据表格卡片 */
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 0 16px 16px;
}
.data-table-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;
}

.data-table-card .el-table {
  --el-table-border-color: var(--el-border-color-lighter);
}

.data-table-card .el-table th {
  background: var(--el-fill-color-light);
  font-weight: 600;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

/* 交换机名称单元格 */
.switch-name-cell,
.group-name-cell,
.vm-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--el-text-color-primary);
}

/* 流量单元格 */
.traffic-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.traffic-cell span {
  font-size: 13px;
  font-weight: 500;
}

/* 展开规则面板 */
.expand-rule-panel {
  padding: 16px 20px;
  background: var(--el-fill-color-lighter);
}

.expand-rule-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.expand-rule-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.rule-inner-table {
  margin-bottom: 0;
}

/* 代码块 */
.code-block-wrapper {
  border-radius: 8px;
  overflow: hidden;
}

.code-block-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--el-fill-color);
  border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.code-block-header .copy-btn {
  margin-left: auto;
  opacity: 0.6;
}

.code-block-header .copy-btn:hover {
  opacity: 1;
}

.code-block {
  margin: 0;
  padding: 16px;
  background: #1e1e2e;
  color: #cdd6f4;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
  font-size: 13px;
  line-height: 1.7;
  overflow-x: auto;
  max-height: 500px;
  overflow-y: auto;
  border-radius: 0 0 8px 8px;
}

.code-block code {
  white-space: pre;
}

.port-highlight {
  font-weight: 700;
  font-size: 15px;
  color: var(--el-color-primary);
}

.whitelist-toolbar {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
}

.security-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.forward-access-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

/* 表单提示 */
.form-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  line-height: 1.5;
}

/* 现代化对话框 */
:deep(.modern-dialog) .el-dialog {
  border-radius: 12px;
  overflow: hidden;
}

:deep(.modern-dialog) .el-dialog__header {
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  margin: 0;
}

:deep(.modern-dialog) .el-dialog__title {
  font-size: 16px;
  font-weight: 600;
}

:deep(.modern-dialog) .el-dialog__body {
  padding: 24px;
}

:deep(.modern-dialog) .el-dialog__footer {
  padding: 16px 24px;
  border-top: 1px solid var(--el-border-color-lighter);
}

:deep(.modern-dialog) .el-divider__text {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
  background: var(--el-bg-color);
}

.table-card {
  margin-top: 0;
}

/* ===== 默认隐藏移动卡片（必须在 @media 之前） ===== */
.mobile-card-list {
  display: none;
}

/* ===== 端口转发移动卡片样式 ===== */
.forward-mobile-card {
  border-radius: 10px;
  overflow: hidden;
}

.forward-mobile-card .el-card__body {
  padding: 0;
}

.forward-card-header {
  padding: 14px 16px 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  overflow: hidden;
}

.forward-card-port-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.forward-card-external {
  font-size: 18px;
  font-weight: 700;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
  color: var(--el-color-primary);
  word-break: break-all;
}

.forward-card-arrow {
  color: var(--el-text-color-secondary);
  font-size: 14px;
  flex-shrink: 0;
}

.forward-card-internal {
  font-size: 14px;
  font-weight: 500;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace;
  color: var(--el-text-color-primary);
  word-break: break-all;
}

.forward-card-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.forward-card-body {
  padding: 8px 16px;
  overflow: hidden;
}

.forward-card-info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 5px 0;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.forward-card-info-row:last-child {
  border-bottom: none;
}

.forward-card-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
  margin-right: 12px;
}

.forward-card-value {
  font-size: 13px;
  color: var(--el-text-color-primary);
  text-align: right;
  display: flex;
  align-items: center;
  gap: 4px;
  word-break: break-all;
}

.forward-card-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 10px 16px;
  border-top: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-lighter);
  border-radius: 0 0 10px 10px;
}

.forward-card-actions .el-button {
  margin-left: 0;
}

/* ===== 交换机移动卡片样式 ===== */
.switch-mobile-card {
  border-radius: 10px;
  overflow: hidden;
}

.switch-mobile-card .el-card__body {
  padding: 0;
}

.switch-card-header {
  padding: 14px 16px 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.switch-card-name-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
}

.switch-card-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.switch-card-user {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.switch-card-body {
  padding: 8px 16px;
}

.switch-card-info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 5px 0;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.switch-card-info-row:last-child {
  border-bottom: none;
}

.switch-card-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
  margin-right: 12px;
}

.switch-card-value {
  font-size: 13px;
  color: var(--el-text-color-primary);
  text-align: right;
  word-break: break-all;
}

.switch-card-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 10px 16px;
  border-top: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-lighter);
  border-radius: 0 0 10px 10px;
}

.switch-card-actions .el-button {
  margin-left: 0;
}

/* ===== 安全组移动卡片样式 ===== */
.sg-mobile-card {
  border-radius: 10px;
  overflow: hidden;
}

.sg-mobile-card .el-card__body {
  padding: 0;
}

.sg-card-header {
  padding: 14px 16px 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.sg-card-name-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
}

.sg-card-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.sg-card-user {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.sg-card-body {
  padding: 8px 16px;
}

.sg-card-info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 5px 0;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.sg-card-info-row:last-child {
  border-bottom: none;
}

.sg-card-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
  margin-right: 12px;
}

.sg-card-value {
  font-size: 13px;
  color: var(--el-text-color-primary);
  text-align: right;
  word-break: break-all;
}

.sg-card-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 10px 16px;
  border-top: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-lighter);
  border-radius: 0 0 10px 10px;
}

.sg-card-actions .el-button {
  margin-left: 0;
}

@media (max-width: 1200px) {
  .overview-stats,
  .quota-summary {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    gap: 12px;
    padding: 16px;
  }

  .page-header-right {
    width: 100%;
  }

  .page-header-right .el-input,
  .page-header-right .el-button {
    width: 100% !important;
  }

  .custom-tabs {
    padding: 0 8px;
    gap: 0;
  }

  .custom-tab {
    padding: 12px 14px;
    font-size: 13px;
  }

  .tab-content {
    padding: 12px;
  }

  .overview-stats,
  .quota-summary {
    grid-template-columns: 1fr;
  }

  .table-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }

  /* 端口转发 - 隐藏表格，显示卡片 */
  .forward-data-card .el-table {
    display: none !important;
  }

  .forward-data-card .mobile-card-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  /* 交换机 - 隐藏表格，显示卡片 */
  .switch-data-card .el-table {
    display: none !important;
  }

  .switch-data-card .mobile-card-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  /* 安全组策略 - 隐藏表格，显示卡片 */
  .security-group-data-card .el-table {
    display: none !important;
  }

  .security-group-data-card .mobile-card-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
}

@media (max-width: 480px) {
  .forward-card-header {
    padding: 10px 12px 8px;
  }

  .forward-card-body {
    padding: 6px 12px;
  }

  .forward-card-actions {
    padding: 8px 12px;
  }

  .forward-card-external {
    font-size: 15px;
  }

  .forward-card-internal {
    font-size: 12px;
  }
}

/* 端口转发卡片内的 code 文本防溢出 */
.forward-card-value .code {
  white-space: normal;
  word-break: break-all;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 180px;
  display: inline-block;
  vertical-align: middle;
}
</style>
