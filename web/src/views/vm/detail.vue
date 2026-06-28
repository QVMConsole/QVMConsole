<template>
  <div class="vm-detail-container">
    <!-- 顶部导航栏 -->
    <div class="detail-top-bar">
      <el-button link class="back-link" @click="$router.back()">
        <el-icon><ArrowLeft /></el-icon>
        <span>返回虚拟机列表</span>
      </el-button>
      <div class="top-bar-right">
        <div class="quick-nav">
          <a href="#functions" @click.prevent="scrollToSection('functions')">⚡ 功能</a>
          <a href="#monitor" @click.prevent="scrollToSection('monitor')">📈 监控</a>
          <a href="#config" @click.prevent="scrollToSection('config')">📋 配置</a>
        </div>
        <el-button v-if="!isLightweight" type="primary" plain size="small" icon="Edit" @click="handleEdit">编辑虚拟机</el-button>
      </div>
    </div>

    <!-- ① Hero 状态横幅（始终优先渲染，不等待 SSE 数据） -->
      <section class="hero-banner" :class="'hero-' + heroStatusClass">
        <div class="hero-status-indicator" :class="vmInfo.status === 'running' ? 'running' : (vmInfo.status === 'paused' ? 'paused' : 'stopped')">
          <VmStatusIcons :status="vmInfo.status || 'shut off'" :size="32" />
        </div>
        <div class="hero-info">
          <h1 class="hero-vm-name">{{ vmInfo.name || vmName }}</h1>
          <div class="hero-meta">
            <el-tag :type="statusTagType(vmInfo.status)" effect="dark" round size="small">
              {{ statusText(vmInfo.status) }}
            </el-tag>
            <el-tag v-if="vmInfo.locked" type="warning" effect="dark" round size="small" style="margin-left: 6px;">
              <el-icon style="margin-right: 2px; vertical-align: middle;"><Lock /></el-icon> 已锁定
            </el-tag>
            <span class="meta-divider"></span>
            <span class="meta-item">
              <el-icon><Cpu /></el-icon> {{ vmInfo.vcpu }} 核
            </span>
            <span class="meta-divider"></span>
            <span class="meta-item">
              <el-icon><Odometer /></el-icon> {{ formatMemory(vmInfo.memory) }}
              <el-tooltip v-if="vmInfo.memory_dynamic_enabled" :content="memoryTooltipText" placement="top" effect="dark">
                <el-tag size="small" :type="memoryTagType" effect="plain" class="dynamic-mem-tag">{{ memoryTagText }}</el-tag>
              </el-tooltip>
            </span>
            <span class="meta-divider"></span>
            <span class="meta-item">
              <el-icon><Coin /></el-icon> {{ vmInfo.disk_size || '-' }}
              <el-tooltip v-if="vmInfo.disk_healthy === false" content="系统磁盘文件缺失，虚拟机可能无法正常启动" placement="top" effect="dark">
                <el-tag size="small" type="danger" effect="dark" round style="margin-left: 6px;">磁盘异常</el-tag>
              </el-tooltip>
            </span>
            <span class="meta-divider"></span>
            <span class="meta-item">
              <el-icon><Timer /></el-icon> {{ runtimeSummaryText }}
            </span>
            <span class="meta-divider"></span>
            <span class="meta-item hero-ip-label">
              <el-icon><Location /></el-icon> IP
              <el-tooltip :content="ipTooltipText" :disabled="!ipTooltipText" placement="top" effect="dark">
                <code class="hero-ip-value" :class="{ 'ip-unreachable': ipTooltipText }">{{ ipDisplayText }}</code>
              </el-tooltip>
            </span>
            <span v-if="publicIPs.length" class="meta-divider"></span>
            <span v-if="publicIPs.length" class="meta-item hero-ip-label">
              <el-icon><Location /></el-icon> 公网
              <span class="hero-public-ip-list">
                <code v-for="item in publicIPs" :key="item.public_ip" class="hero-ip-value public-ip-value">{{ item.public_ip }}</code>
              </span>
            </span>
          </div>
        </div>
        <div class="hero-power-actions" v-if="vmInfo.status !== 'migrating'">
          <template v-if="vmInfo.status !== 'running'">
            <el-popconfirm :title="startConfirmText" placement="bottom" @confirm="handleAction('start')">
              <template #reference>
                <el-button type="success" :loading="operating" icon="VideoPlay" class="power-btn">{{ startActionText }}</el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm v-if="vmInfo.status === 'paused'" title="确定要重置虚拟机吗？这相当于硬重启，适用于无法继续启动的暂停状态。" placement="bottom" @confirm="handleAction('reset')">
              <template #reference>
                <el-button type="danger" :loading="operating" icon="RefreshRight" class="power-btn">重置</el-button>
              </template>
            </el-popconfirm>
          </template>
          <template v-else>
            <el-popconfirm title="确定要重启吗？" placement="bottom" @confirm="handleAction('reboot')">
              <template #reference>
                <el-button type="warning" :loading="operating" icon="Refresh" class="power-btn">重启</el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm v-if="vmInfo.locked" title="⚠️ 虚拟机已锁定，关机可能影响正在运行的服务，确定要关机吗？" placement="bottom" @confirm="handleAction('shutdown')">
              <template #reference>
                <el-button plain :loading="operating" icon="SwitchButton" class="power-btn" style="border-color: #E6A23C; color: #E6A23C;">关机（已锁定）</el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm v-else title="确定要关机吗？" placement="bottom" @confirm="handleAction('shutdown')">
              <template #reference>
                <el-button plain :loading="operating" icon="SwitchButton" class="power-btn">关机</el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm v-if="vmInfo.locked" title="⚠️ 虚拟机已锁定，强制断电可能影响正在运行的服务，确定要断电吗？" placement="bottom" @confirm="handleAction('destroy')">
              <template #reference>
                <el-button :loading="operating" icon="CircleClose" class="power-btn" style="border-color: #F56C6C; color: #F56C6C;">强制断电（已锁定）</el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm v-else title="确定要断电吗？" placement="bottom" @confirm="handleAction('destroy')">
              <template #reference>
                <el-button type="danger" :loading="operating" icon="CircleClose" class="power-btn">强制断电</el-button>
              </template>
            </el-popconfirm>
          </template>
        </div>
        <!-- 锁定/解锁操作 -->
        <div v-if="!isLightweight && vmInfo.status !== 'migrating'" class="hero-lock-action">
          <template v-if="vmInfo.locked">
            <el-popconfirm title="解除锁定需要进行二次验证，确定要解锁吗？" placement="bottom" @confirm="handleLockAction('unlock')">
              <template #reference>
                <el-button type="warning" plain size="small" icon="Unlock">解除锁定</el-button>
              </template>
            </el-popconfirm>
          </template>
          <template v-else>
            <el-popconfirm title="锁定后虚拟机将无法删除，关机需二次确认，确定要锁定吗？" placement="bottom" @confirm="handleLockAction('lock')">
              <template #reference>
                <el-button plain size="small" icon="Lock">锁定虚拟机</el-button>
              </template>
            </el-popconfirm>
          </template>
        </div>
        <el-alert
          v-else
          type="warning"
          title="虚拟机正在迁移中，暂不能执行电源、快照、磁盘、救援或密码重置等操作"
          :closable="false"
          class="migration-alert"
        />
      </section>

    <!-- Hero 下方区域：SSE 数据到达后逐步加载 -->
    <template v-if="pageReady">

      <!-- ② 实时资源概览条（仅运行中显示） -->
      <section v-if="vmInfo.status === 'running'" class="live-stats-strip" id="stats">
        <div class="live-stat-card cpu-card">
          <div class="live-stat-icon cpu-icon">
            <el-icon><Cpu /></el-icon>
          </div>
          <div class="live-stat-info">
            <div class="live-stat-label">CPU 使用率</div>
            <div class="live-stat-value">{{ cpuPercentStr }}</div>
            <div class="mini-progress"><div class="mini-progress-fill cpu-fill" :style="{ width: cpuPercentStr }"></div></div>
          </div>
        </div>
        <div class="live-stat-card mem-card">
          <div class="live-stat-icon mem-icon">
            <el-icon><Odometer /></el-icon>
          </div>
          <div class="live-stat-info">
            <div class="live-stat-label">内存使用率</div>
            <div class="live-stat-value">{{ memPercentStr }}</div>
            <div class="mini-progress"><div class="mini-progress-fill mem-fill" :style="{ width: memPercentStr }"></div></div>
          </div>
        </div>
        <div class="live-stat-card net-card">
          <div class="live-stat-icon net-icon">
            <el-icon><Connection /></el-icon>
          </div>
          <div class="live-stat-info">
            <div class="live-stat-label">网络流量</div>
            <div class="live-stat-value">{{ netRxStr }}</div>
            <div class="live-stat-sub">出 {{ netTxStr }}</div>
          </div>
        </div>
        <div class="live-stat-card disk-card">
          <div class="live-stat-icon disk-icon">
            <el-icon><Coin /></el-icon>
          </div>
          <div class="live-stat-info">
            <div class="live-stat-label">磁盘 IO</div>
            <div class="live-stat-value">{{ diskRdStr }}</div>
            <div class="live-stat-sub">写 {{ diskWrStr }}</div>
          </div>
        </div>
      </section>

      <!-- ③ 功能管理 Tabs -->
      <section class="function-tabs-section" id="functions">
        <div class="tabs-header">
          <el-tabs v-model="activeTab" class="custom-tabs" @tab-click="handleTabClick">
            <el-tab-pane name="snapshot">
              <template #label>
                <span class="tab-label-text">
                  <el-icon><PictureFilled /></el-icon> 快照管理
                  <el-tag v-if="snapshotQuotaText" size="small" type="info" effect="plain" class="tab-quota-tag">{{ snapshotQuotaText }}</el-tag>
                </span>
              </template>
              <SnapshotList :vm-name="vmName" :vm-status="vmInfo.status" @quota-change="handleSnapshotQuotaChange" />
            </el-tab-pane>
            <el-tab-pane name="network" lazy>
              <template #label>
                <span class="tab-label-text">
                  <el-icon><Connection /></el-icon> 网络管理
                </span>
              </template>
              <NetworkList :vm-name="vmName" />
            </el-tab-pane>
            <el-tab-pane v-if="!isLightweight" name="schedule" lazy>
              <template #label>
                <span class="tab-label-text">
                  <el-icon><AlarmClock /></el-icon> 定时任务
                </span>
              </template>
              <VmSchedulePanel :vm-name="vmName" />
            </el-tab-pane>
            <el-tab-pane name="vnc" lazy>
              <template #label>
                <span class="tab-label-text">
                  <el-icon><Monitor /></el-icon> VNC 控制台
                </span>
              </template>
              <VncConsole :vm-name="vmName" :vm-status="vmInfo.status" :guest-password="vmInfo.credential?.password || ''" />
            </el-tab-pane>

            <el-tab-pane name="spice" lazy>
              <template #label>
                <span class="tab-label-text">
                  <el-icon><VideoCamera /></el-icon> SPICE 控制台
                </span>
              </template>

              <!-- SPICE 面板：供外部客户端（virt-viewer/spicy）连接 -->
              <div class="spice-panel">
                <div class="spice-panel-header">
                  <span class="spice-title">SPICE 协议（外部客户端）</span>
                  <el-tag v-if="vmInfo.spiceInfo.enabled" :type="vmInfo.spiceInfo.exposed ? 'danger' : 'success'" size="small">
                    {{ vmInfo.spiceInfo.exposed ? '已对外暴露' : '仅本地' }}
                  </el-tag>
                  <el-tag v-else type="info" size="small">未开启</el-tag>
                </div>

                <div class="spice-actions">
                  <el-button v-if="!vmInfo.spiceInfo.enabled" type="primary" size="small" :loading="vmInfo.spiceLoading" @click="handleSpiceEnable">
                    开启 SPICE
                  </el-button>
                  <template v-else>
                    <el-button type="warning" size="small" :loading="vmInfo.spiceLoading" @click="handleSpiceDisable">
                    关闭 SPICE
                    </el-button>
                    <el-button size="small" :loading="vmInfo.spiceLoading" @click="handleSpiceChangePassword">
                      {{ vmInfo.spiceInfo.has_password ? '修改密码' : '设置密码' }}
                    </el-button>
                    <el-tooltip content="开启后通过公网 IP 供外部客户端连接（自动放行防火墙端口）" placement="top">
                      <span>
                        <el-switch
                          :model-value="vmInfo.spiceInfo.exposed"
                          :loading="vmInfo.spiceLoading"
                          active-text="对外暴露"
                          inline-prompt
                          @change="handleSpiceExpose"
                        />
                      </span>
                    </el-tooltip>
                  </template>
                </div>

                <div v-if="vmInfo.spiceInfo.enabled" class="spice-info">
                  <div class="spice-info-row">
                    <span class="info-label">SPICE 端口</span>
                    <span class="info-value mono">{{ vmInfo.spiceInfo.port || '-' }}</span>
                  </div>
                  <div v-if="vmInfo.spiceInfo.exposed && vmInfo.spiceConnInfo.host" class="spice-info-row">
                    <span class="info-label">外部地址</span>
                    <span class="info-value mono">{{ vmInfo.spiceConnInfo.host }}:{{ vmInfo.spiceConnInfo.port }}</span>
                  </div>
                  <div class="spice-hint">
                    使用 virt-viewer / spicy 等客户端连接；下载 .vv 文件可直接双击由 virt-viewer 打开。
                  </div>
                  <el-dropdown trigger="click" @command="handleDownloadSpiceVV">
                    <el-button type="primary" plain size="small" :disabled="!spiceConnReady">
                      下载 .vv 连接文件<el-icon class="el-icon--right"><ArrowDown /></el-icon>
                    </el-button>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item :command="true">一次性（连接后自动删除）</el-dropdown-item>
                        <el-dropdown-item :command="false">可重复使用（保留文件）</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </div>
              </div>
            </el-tab-pane>
            <el-tab-pane v-if="showDeveloperTab" name="monitor" lazy>
              <template #label>
                <span class="tab-label-text">
                  <el-icon><Operation /></el-icon> 开发者监视器
                </span>
              </template>
              <VmMonitorPanel :vm-name="vmName" :vm-status="vmInfo.status" />
            </el-tab-pane>
          </el-tabs>
          <el-button v-if="!isLightweight" link type="primary" class="dev-toggle-link" @click="toggleDeveloperTab">
            {{ developerTabButtonText }}
          </el-button>
        </div>
      </section>

      <!-- ④ 监控图表区（按需加载） -->
      <section ref="monitorSectionRef" class="monitor-section" id="monitor">
        <div class="section-title-bar">
          <h3 class="section-title">
            <el-icon><TrendCharts /></el-icon> 监控图表
          </h3>
        </div>
        <template v-if="showMonitor">
          <ResourceCharts type="vm" :name="vmName" :status="vmInfo.status" :externalStats="vmInfo.stats" :disablePolling="true" />
        </template>
        <div v-else class="lazy-section-placeholder">
          <el-skeleton :rows="8" animated />
        </div>
      </section>

      <!-- ⑤ 信息卡片区（按需加载） -->
      <section ref="infoSectionRef" class="info-grid" id="config">
        <template v-if="showInfoCards">
          <!-- 左列 -->
          <div class="info-column">
            <div class="info-card">
              <div class="info-card-header">
                <div class="card-icon config-icon">
                  <el-icon><Setting /></el-icon>
                </div>
                <span class="card-title">基本配置</span>
              </div>
              <div class="info-card-body">
                <div class="info-row">
                  <span class="info-label">CPU</span>
                  <span class="info-value">
                    <span class="info-tag tag-primary">{{ vmInfo.vcpu }} 核</span>
                    <span v-if="vmInfo.cpu_limit_percent > 0" class="sub-label">限制 {{ vmInfo.cpu_limit_percent }}%</span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">
                    内存
                    <span v-if="vmInfo.memory_dynamic_enabled" class="sub-label">动态</span>
                  </span>
                  <span class="info-value">
                    <span>{{ formatMemory(vmInfo.memory) }}</span>
                    <el-tooltip v-if="vmInfo.memory_dynamic_enabled" :content="memoryTooltipText" placement="top" effect="dark">
                      <span class="info-tag" :class="memoryTagType === 'warning' ? 'tag-warning' : 'tag-success'">{{ memoryTagText }}</span>
                    </el-tooltip>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">PCIe 热插槽</span>
                  <span class="info-value">
                    <template v-if="vmInfo.machine_type === 'q35' || vmInfo.machine_type === 'virt'">
                      <span class="info-tag tag-primary">{{ vmInfo.pcie_root_ports || 0 }} 槽</span>
                      <template v-if="vmInfo.pcie_info">
                        <span class="sub-label" style="margin-left: 6px;">
                          已用 {{ vmInfo.pcie_info.used_ports }} · 空闲 {{ vmInfo.pcie_info.free_ports }}
                        </span>
                        <el-tooltip v-if="vmInfo.pcie_info.free_ports <= 1" content="热插槽即将用尽，建议关机后增加 PCIe 热插槽数量" placement="top" effect="dark">
                          <el-tag size="small" type="danger" effect="plain" style="margin-left: 4px;">紧张</el-tag>
                        </el-tooltip>
                      </template>
                      <span v-else class="sub-label" style="margin-left: 6px;">加载中...</span>
                    </template>
                    <span v-else>i440FX 不支持热插槽</span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">系统磁盘</span>
                  <span class="info-value mono">
                    {{ vmInfo.disk_size || '-' }}
                    <el-tooltip v-if="vmInfo.disk_healthy === false" content="系统磁盘文件缺失，虚拟机可能无法正常启动" placement="top" effect="dark">
                      <el-tag size="small" type="danger" effect="plain" style="margin-left: 6px;">磁盘缺失</el-tag>
                    </el-tooltip>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">操作系统</span>
                  <span class="info-value">{{ vmInfo.os_type || '-' }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">模板来源</span>
                  <span class="info-value">{{ vmInfo.template || '-' }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">备注</span>
                  <span class="info-value remark-info-value">
                    <span class="remark-text">{{ vmInfo.remark || '-' }}</span>
                    <el-button
                      v-if="!isLightweight"
                      link
                      type="primary"
                      :disabled="vmInfo.status === 'migrating'"
                      @click="handleEditRemark"
                    >
                      编辑备注
                    </el-button>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">连续运行</span>
                  <span class="info-value">
                    <span>{{ runtimeSummaryText }}</span>
                    <span v-if="vmInfo.continuous_running_since" class="sub-label runtime-sub-label">
                      自 {{ vmInfo.continuous_running_since }}
                    </span>
                  </span>
                </div>
              </div>
            </div>

            <div class="info-card credential-card">
              <div class="info-card-header">
                <div class="card-icon credential-icon">
                  <el-icon><Lock /></el-icon>
                </div>
                <span class="card-title">登录凭证</span>
              </div>
              <div class="info-card-body">
                <div class="info-row">
                  <span class="info-label">用户名</span>
                  <span class="info-value">
                    <span v-if="vmInfo.credential?.username" class="credential-value-wrap">
                      <code class="credential-code">{{ vmInfo.credential.username }}</code>
                      <el-button link type="primary" size="small" @click="copyCredentialField(vmInfo.credential.username, '账号')">复制</el-button>
                    </span>
                    <span v-else>-</span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">密码</span>
                  <span class="info-value">
                    <span v-if="vmInfo.credential?.password" class="credential-value-wrap">
                      <el-input
                        :model-value="vmInfo.credential.password"
                        type="password"
                        show-password
                        readonly
                        size="small"
                        class="credential-input"
                      />
                      <el-button link type="primary" size="small" @click="copyCredentialField(vmInfo.credential.password, '密码')">复制</el-button>
                    </span>
                    <span v-else>-</span>
                  </span>
                </div>
                <div class="info-row info-row-highlight">
                  <span class="info-label highlight-label">⚡ 离线重置</span>
                  <span class="info-value">
                    <el-button
                      type="warning"
                      size="small"
                      plain
                      :disabled="!canResetGuestPassword"
                      @click="openResetPasswordDialog"
                    >重置密码</el-button>
                  </span>
                </div>
                <div v-if="!isLightweight" class="info-row info-row-highlight">
                  <span class="info-label highlight-label">🧨 重装系统</span>
                  <span class="info-value">
                    <el-button
                      type="danger"
                      size="small"
                      plain
                      :disabled="vmInfo.status === 'migrating'"
                      @click="openReinstallDialog"
                    >提交重装</el-button>
                  </span>
                </div>
              </div>
            </div>
          </div>

          <!-- 右列 -->
          <div class="info-column">
            <div class="info-card">
              <div class="info-card-header">
                <div class="card-icon network-icon">
                  <el-icon><Connection /></el-icon>
                </div>
                <span class="card-title">网络与连接</span>
              </div>
              <div class="info-card-body">
                <div class="info-row">
                  <span class="info-label">IP 地址</span>
                  <span class="info-value mono ip-highlight">
                    <el-tooltip :content="ipTooltipText" :disabled="!ipTooltipText" placement="top" effect="dark">
                      <span :class="{ 'ip-unreachable': ipTooltipText }">{{ ipDisplayText }}</span>
                    </el-tooltip>
                  </span>
                </div>
                <div v-if="allInterfaceIPs.length > 0" class="info-row">
                  <span class="info-label">全部 IP</span>
                  <span class="info-value mono" style="display: flex; flex-direction: column; gap: 3px;">
                    <div v-for="(item, idx) in (showInfoIPs ? allInterfaceIPs : allInterfaceIPs.slice(0, 3))" :key="idx" style="display: flex; align-items: center; gap: 6px;">
                      <code class="credential-code" style="margin: 0;">{{ item.ip }}</code>
                      <el-tag v-if="item.source" size="small" :type="ipSourceTagType(item.source)" effect="plain">{{ ipSourceLabel(item.source) }}</el-tag>
                    </div>
                    <el-button v-if="allInterfaceIPs.length > 3" link type="primary" size="small" @click="showInfoIPs = !showInfoIPs">
                      {{ showInfoIPs ? '收起' : `显示全部 (${allInterfaceIPs.length})` }}
                    </el-button>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">公网 IP</span>
                  <span class="info-value public-ip-list">
                    <template v-if="publicIPs.length">
                      <el-tag
                        v-for="item in publicIPs"
                        :key="item.public_ip"
                        size="small"
                        effect="plain"
                        class="public-ip-tag"
                      >
                        {{ item.public_ip }} · {{ item.mode_label || item.mode }}
                      </el-tag>
                    </template>
                    <span v-else>-</span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">VNC 端口</span>
                  <span class="info-value mono">{{ vmInfo.vnc_port || '-' }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">网络接口</span>
                  <span class="info-value">{{ vmInfo.network || '-' }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">显示设备</span>
                  <span class="info-value">{{ vmInfo.video_model || '-' }}</span>
                </div>
              </div>
            </div>

            <div class="info-card">
              <div class="info-card-header">
                <div class="card-icon advanced-icon">
                  <el-icon><Operation /></el-icon>
                </div>
                <span class="card-title">高级设置</span>
              </div>
              <div class="info-card-body">
                <div class="info-row">
                  <span class="info-label">开机自启</span>
                  <span class="info-value">
                    <span class="info-tag" :class="vmInfo.autostart ? 'tag-success' : 'tag-info'">
                      {{ vmInfo.autostart ? '已启用' : '已禁用' }}
                    </span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">
                    启动冻结
                    <span class="sub-label">开发者</span>
                  </span>
                  <span class="info-value">
                    <el-tooltip
                      v-if="vmInfo.freeze"
                      content="已开启：启动时冻结 CPU。虚拟机启动后会先进入暂停态，可在开发者监视器执行 c 继续。"
                      placement="top"
                    >
                      <span class="info-tag tag-warning">已开启</span>
                    </el-tooltip>
                    <span v-else class="info-tag tag-info">未开启</span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">APIC</span>
                  <span class="info-value">
                    <span class="info-tag" :class="vmInfo.apic ? 'tag-success' : 'tag-warning'">
                      {{ vmInfo.apic ? '已启用' : '已关闭' }}
                    </span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">PAE</span>
                  <span class="info-value">
                    <span class="info-tag" :class="vmInfo.pae ? 'tag-success' : 'tag-info'">
                      {{ vmInfo.pae ? '已启用' : '已关闭' }}
                    </span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">QEMU Guest Agent</span>
                  <span class="info-value">
                    <el-tooltip v-if="vmInfo.guest_agent_status?.version" :content="'版本: ' + vmInfo.guest_agent_status.version" placement="top">
                      <span class="info-tag" :class="guestAgentStatusClass">{{ guestAgentStatusText }}</span>
                    </el-tooltip>
                    <span v-else class="info-tag" :class="guestAgentStatusClass">{{ guestAgentStatusText }}</span>
                  </span>
                </div>
                <div class="info-row">
                  <span class="info-label">CPU 限制</span>
                  <span class="info-value">{{ vmInfo.cpu_limit_percent > 0 ? `${vmInfo.cpu_limit_percent}%` : '无限制' }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">CPU 亲和性</span>
                  <span class="info-value">{{ vmInfo.cpu_affinity || '未设置' }}</span>
                </div>
                <div class="info-row">
                  <span class="info-label">内存策略</span>
                  <span class="info-value">{{ vmInfo.memory_backend === 'virtio_mem' ? 'virtio-mem 弹性' : 'Balloon 动态' }}</span>
                </div>
              </div>
            </div>

            <!-- 磁盘 IOPS 限制信息 -->
            <div class="info-card" v-if="!isLightweight">
              <div class="info-card-header">
                <div class="card-icon disk-iops-icon">
                  <el-icon><Coin /></el-icon>
                </div>
                <span class="card-title">磁盘 IOPS 限制</span>
                <el-button link size="small" type="primary" class="card-refresh-btn" @click="loadDiskIOPSList" :loading="diskIopsLoading">刷新</el-button>
              </div>
              <div class="info-card-body" v-loading="diskIopsLoading">
                <template v-if="diskIopsList.length > 0">
                  <div class="disk-iops-table">
                    <div class="disk-iops-header">
                      <span class="disk-iops-col device-col">设备</span>
                      <span class="disk-iops-col capacity-col">容量</span>
                      <span class="disk-iops-col iops-col">总IOPS</span>
                      <span class="disk-iops-col iops-col">读IOPS</span>
                      <span class="disk-iops-col iops-col">写IOPS</span>
                    </div>
                    <div v-for="disk in diskIopsList" :key="disk.device" class="disk-iops-row">
                      <span class="disk-iops-col device-col">
                        <span class="disk-dev-name">{{ disk.device }}</span>
                        <span class="disk-dev-bus">({{ disk.bus }})</span>
                      </span>
                      <span class="disk-iops-col capacity-col">{{ disk.capacity_gb ? disk.capacity_gb + ' GB' : '-' }}</span>
                      <span class="disk-iops-col iops-col">
                        <span :class="disk.iops_total?.is_set && disk.iops_total?.value > 0 ? 'iops-limited' : 'iops-none'">
                          {{ disk.iops_total?.is_set && disk.iops_total?.value > 0 ? disk.iops_total.value : '无限制' }}
                        </span>
                      </span>
                      <span class="disk-iops-col iops-col">
                        <span :class="disk.iops_read?.is_set && disk.iops_read?.value > 0 ? 'iops-limited' : 'iops-none'">
                          {{ disk.iops_read?.is_set && disk.iops_read?.value > 0 ? disk.iops_read.value : '无限制' }}
                        </span>
                      </span>
                      <span class="disk-iops-col iops-col">
                        <span :class="disk.iops_write?.is_set && disk.iops_write?.value > 0 ? 'iops-limited' : 'iops-none'">
                          {{ disk.iops_write?.is_set && disk.iops_write?.value > 0 ? disk.iops_write.value : '无限制' }}
                        </span>
                      </span>
                    </div>
                  </div>
                </template>
                <div v-else class="disk-iops-empty">暂无磁盘数据</div>
              </div>
            </div>
          </div>
        </template>
        <template v-else>
          <div class="info-column">
            <div class="info-card lazy-card-skeleton">
              <div class="info-card-header">
                <div class="card-icon config-icon">
                  <el-icon><Setting /></el-icon>
                </div>
                <span class="card-title">基本配置</span>
              </div>
              <div class="info-card-body"><el-skeleton :rows="7" animated /></div>
            </div>
            <div class="info-card lazy-card-skeleton">
              <div class="info-card-header">
                <div class="card-icon credential-icon">
                  <el-icon><Lock /></el-icon>
                </div>
                <span class="card-title">登录凭证</span>
              </div>
              <div class="info-card-body"><el-skeleton :rows="4" animated /></div>
            </div>
          </div>
          <div class="info-column">
            <div class="info-card lazy-card-skeleton">
              <div class="info-card-header">
                <div class="card-icon network-icon">
                  <el-icon><Connection /></el-icon>
                </div>
                <span class="card-title">网络与连接</span>
              </div>
              <div class="info-card-body"><el-skeleton :rows="5" animated /></div>
            </div>
            <div class="info-card lazy-card-skeleton">
              <div class="info-card-header">
                <div class="card-icon advanced-icon">
                  <el-icon><Operation /></el-icon>
                </div>
                <span class="card-title">高级设置</span>
              </div>
              <div class="info-card-body"><el-skeleton :rows="6" animated /></div>
            </div>
          </div>
        </template>
      </section>
    </template>

    <!-- Hero 已展示，下方区域加载中 -->
    <div v-else class="below-hero-loading">
      <div class="lazy-section-placeholder">
        <el-skeleton :rows="3" animated />
      </div>
      <div class="lazy-section-placeholder" style="margin-top: 16px;">
        <el-skeleton :rows="4" animated />
      </div>
    </div>

    <!-- 返回顶部 -->
    <transition name="fade-up">
      <el-button
        v-show="showBackToTop"
        class="back-to-top-btn"
        circle
        :icon="ArrowUp"
        @click="scrollToTop"
        title="返回顶部"
      />
    </transition>

    <!-- 编辑虚拟机表单 -->
    <VmForm ref="vmFormRef" @success="handleEditSuccess" />
    <VmRemarkDialog ref="vmRemarkDialogRef" @success="handleRemarkSuccess" />
    <VmReinstallDialog ref="reinstallRef" @success="loadVmDetail" />

    <!-- 重置密码对话框 -->
    <el-dialog
      v-model="resetPasswordDialogVisible"
      :title="resetPasswordDialogTitle"
      width="520px"
      :close-on-click-modal="false"
      append-to-body
    >
      <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
        <template #title>{{ resetPasswordAlertText }}</template>
      </el-alert>

      <el-form ref="resetPasswordFormRef" :model="resetPasswordForm" :rules="resetPasswordRules" label-width="100px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="resetPasswordForm.username" placeholder="请输入要重置的用户名" />
          <div v-if="resetPasswordUsernameHint" class="form-inline-hint">{{ resetPasswordUsernameHint }}</div>
        </el-form-item>
        <el-form-item label="新密码" prop="password">
          <el-input v-model="resetPasswordForm.password" type="password" show-password placeholder="请输入强密码">
            <template #append>
              <el-button @click="handleGenerateResetPassword">生成强密码</el-button>
            </template>
          </el-input>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="resetPasswordDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="resetPasswordSubmitting" :disabled="!canResetGuestPassword" @click="submitResetPassword">
          提交任务
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { createVmDetailSSE, operateVm, resetVmLinuxPassword, lockVm, unlockVm, getVmPCIEInfo } from '@/api/vm'
import { getDiskList, getVMNetworkStatus } from '@/api/vm'
import {
  getSpiceStatus,
  getSpiceConnInfo,
  enableSpice,
  disableSpice,
  changeSpicePassword,
  exposeSpice,
  downloadSpiceVV
} from '@/api/vm'
import { ElMessage, ElMessageBox } from 'element-plus'
import SnapshotList from '@/components/SnapshotList.vue'
import NetworkList from '@/components/NetworkList.vue'
import VncConsole from '@/components/VncConsole.vue'
import VmSchedulePanel from '@/components/VmSchedulePanel.vue'
import VmMonitorPanel from '@/components/VmMonitorPanel.vue'
import ResourceCharts from '@/components/ResourceCharts.vue'
import VmForm from '@/components/VmForm.vue'
import VmRemarkDialog from '@/components/VmRemarkDialog.vue'
import VmReinstallDialog from '@/components/VmReinstallDialog.vue'
import VmStatusIcons from '@/components/icons/VmStatusIcons.vue'
import { passwordValidator, checkPasswordBreachAsync, generatePassword as genPwd, STRONG_PASSWORD_MIN_LENGTH } from '@/utils/validate'
import { useVmStore } from '@/store/vm'
import { useUserStore } from '@/store/user'
import { copyTextWithFallback } from '@/utils/clipboard'
import {
  ArrowLeft, ArrowUp, ArrowDown, Cpu, Odometer, Location, Coin,
  RefreshRight, Refresh, SwitchButton, CircleClose, VideoPlay,
  Edit, Connection, Monitor, Operation, Setting, Lock, AlarmClock, VideoCamera,
  PictureFilled, TrendCharts, Timer
} from '@element-plus/icons-vue'

const route = useRoute()
const vmName = computed(() => route.params.id)
const pageReady = ref(false)
const operating = ref(false)
const activeTab = ref('snapshot')
const snapshotQuota = ref(null)
const showDeveloperTab = ref(false)
const vmFormRef = ref(null)
const vmRemarkDialogRef = ref(null)
const reinstallRef = ref(null)
const vmStore = useVmStore()
const userStore = useUserStore()
const diskIopsList = ref([])
const diskIopsLoading = ref(false)
const isLightweight = computed(() => userStore.role !== 'admin' && userStore.cloudType === 'lightweight')

// ==================== 区域懒加载 ====================
const showMonitor = ref(false)
const showInfoCards = ref(false)
const monitorSectionRef = ref(null)
const infoSectionRef = ref(null)
let sectionObserver = null

const initLazySections = () => {
  if (sectionObserver) {
    sectionObserver.disconnect()
  }

  // 使用 IntersectionObserver 检测监控区和信息卡片区是否进入视口
  sectionObserver = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue
      const el = entry.target
      if (el === monitorSectionRef.value && !showMonitor.value) {
        showMonitor.value = true
      }
      if (el === infoSectionRef.value && !showInfoCards.value) {
        showInfoCards.value = true
        // 信息卡片区可见时加载磁盘 IOPS 数据
        nextTick(() => {
          if (!isLightweight.value && diskIopsList.value.length === 0) {
            loadDiskIOPSList()
          }
        })
      }
      // 如果两个区域都已加载，断开观察器
      if (showMonitor.value && showInfoCards.value) {
        sectionObserver.disconnect()
        sectionObserver = null
      }
    }
  }, {
    rootMargin: '200px', // 提前 200px 开始加载，确保滚动到时已渲染完毕
    threshold: 0
  })
}

const observeLazySections = () => {
  if (!sectionObserver) return
  if (monitorSectionRef.value) {
    sectionObserver.observe(monitorSectionRef.value)
  }
  if (infoSectionRef.value) {
    sectionObserver.observe(infoSectionRef.value)
  }
}
const ipDisplayText = computed(() => {
  if (vmInfo.ip) return vmInfo.ip
  if (vmInfo.ip_status) return '无法获取'
  return '获取中...'
})
// 多网口 IP 列表
const allInterfaceIPs = ref([])
const showInfoIPs = ref(false)
const hasMultipleIPs = computed(() => allInterfaceIPs.value.length > 0)
const allIPsDisplayText = computed(() => {
  return allInterfaceIPs.value.map(i => i.ip).join(', ')
})
const fetchAllInterfaceIPs = async () => {
  try {
    const res = await getVMNetworkStatus(vmName.value)
    const ifaces = res.data?.interfaces || []
    const ips = ifaces
      .filter(i => i.ip && i.ip !== '0.0.0.0')
      .map(i => ({ ip: i.ip, source: i.ip_source || '' }))
    // 去重
    const seen = new Set()
    allInterfaceIPs.value = ips.filter(item => {
      if (seen.has(item.ip)) return false
      seen.add(item.ip)
      return true
    })
  } catch {
    allInterfaceIPs.value = []
  }
}
const ipTooltipText = computed(() => {
  if (vmInfo.ip) return ''
  if (vmInfo.ip_status === 'vlan_bridge') return '桥接 VLAN 模式下上游路由器分配的 IP 无法从宿主机获取'
  if (vmInfo.ip_status === 'shut_off') return '虚拟机处于关机状态，无法获取 IP'
  return ''
})

// Guest Agent 状态
const guestAgentStatusText = computed(() => {
  const s = vmInfo.guest_agent_status
  if (!s) return '未知'
  if (s.connected) return '已连接'
  if (s.configured) return '已配置未连接'
  return '未配置'
})
const guestAgentStatusClass = computed(() => {
  const s = vmInfo.guest_agent_status
  if (!s) return 'tag-info'
  if (s.connected) return 'tag-success'
  if (s.configured) return 'tag-warning'
  return 'tag-info'
})

// IP 来源标签
const ipSourceLabel = (source) => {
  const map = {
    guest_agent: 'Guest Agent',
    arp: 'ARP',
    ovs_dhcp: 'OVS DHCP',
    vpc_dhcp: 'VPC DHCP',
    libvirt_lease: 'libvirt 租约',
    static: '静态绑定'
  }
  return map[source] || source || '-'
}
const ipSourceTagType = (source) => {
  if (source === 'guest_agent') return 'success'
  if (source === 'static') return 'warning'
  return 'info'
}
const resetPasswordDialogVisible = ref(false)
const resetPasswordSubmitting = ref(false)
const resetPasswordFormRef = ref(null)
const showBackToTop = ref(false)

const strongPasswordMinLength = STRONG_PASSWORD_MIN_LENGTH
const usernamePattern = /^[a-z_][a-z0-9_-]{0,31}$/
const windowsUsernameInvalidPattern = /["/\\[\]:;|=,+*?<>]/

const vmInfo = reactive({
  name: '',
  remark: '',
  status: 'shut off',
  vcpu: 0,
  memory: 0,
  network: '',
  ip: '',
  template: '',
  disk_size: '',
  disk_healthy: null,
  os_type: '',
  video_model: '',
  cpu_limit_percent: 0,
  cpu_affinity: '',
  stats: null,
  credential: null,
  freeze: false,
  apic: true,
  pae: true,
  memory_dynamic_enabled: false,
  memory_backend: 'balloon',
  autostart: false,
  vnc_port: '',
  public_ips: [],
  // SPICE 显示协议（与 VNC 共存，供外部客户端连接）
  spiceInfo: { enabled: false, port: '', has_password: false, exposed: false },
  spiceConnInfo: { host: '', port: '', password: '', exposed: false },
  spiceLoading: false,
  continuous_runtime_seconds: 0,
  continuous_running_since: '',
  pcie_root_ports: 0,
  pcie_info: null
})

const startActionText = computed(() => vmInfo.status === 'paused' ? '继续启动' : '开机')
const startConfirmText = computed(() => vmInfo.status === 'paused' ? '确定要继续启动吗？' : '确定要开机吗？')
const developerTabButtonText = computed(() => showDeveloperTab.value ? '隐藏开发者页面' : '显示开发者页面')
const memoryTagText = computed(() => vmInfo.memory_backend === 'virtio_mem' ? '弹性内存' : '动态内存')
const memoryTagType = computed(() => vmInfo.memory_backend === 'virtio_mem' ? 'warning' : 'success')
const heroStatusClass = computed(() => {
  if (vmInfo.status === 'migrating') return 'paused'
  if (vmInfo.status === 'running') return 'running'
  if (vmInfo.status === 'paused') return 'paused'
  return 'stopped'
})
const memoryTooltipText = computed(() => {
  if (vmInfo.memory_backend === 'virtio_mem') {
    return 'Windows 弹性内存基于 virtio-mem：主内存为规格值，基础内存自动计算；运行后使用率超过 70% 每次扩容 1GB，低于 50% 时按目标使用率缩容。'
  }
  return '此处显示虚拟机当前可调度的内存上限。系统会根据宿主机资源和面板调度策略动态调整：宿主机内存紧张时可能回收，但最低不低于设定内存的 50%；宿主机资源充足且虚拟机需要更多内存时，可额外调度约 30% 内存应对突发负载。'
})

const formatMemory = (mem) => {
  if (!mem) return '-'
  return mem >= 1024 ? (mem / 1024).toFixed(1) + ' GB' : mem + ' MB'
}

const formatTraffic = (bytesPerSec) => {
  if (bytesPerSec == null || bytesPerSec < 0) return '0 KB/s'
  if (bytesPerSec >= 1048576) return (bytesPerSec / 1048576).toFixed(1) + ' MB/s'
  if (bytesPerSec >= 1024) return (bytesPerSec / 1024).toFixed(1) + ' KB/s'
  return bytesPerSec.toFixed(1) + ' B/s'
}

const formatContinuousRuntime = (seconds, status) => {
  const normalized = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0
  if (normalized <= 0) {
    return status === 'running' || status === 'paused' ? '不足 1 分钟' : '-'
  }

  const days = Math.floor(normalized / 86400)
  const hours = Math.floor((normalized % 86400) / 3600)
  const minutes = Math.floor((normalized % 3600) / 60)
  const remainSeconds = normalized % 60

  if (days > 0) {
    return `${days} 天 ${hours} 小时 ${minutes} 分钟`
  }
  if (hours > 0) {
    return `${hours} 小时 ${minutes} 分钟`
  }
  if (minutes > 0) {
    return `${minutes} 分钟 ${remainSeconds} 秒`
  }
  return `${remainSeconds} 秒`
}

const runtimeSummaryText = computed(() => {
  return formatContinuousRuntime(vmInfo.continuous_runtime_seconds, vmInfo.status)
})
const publicIPs = computed(() => {
  return Array.isArray(vmInfo.public_ips) ? vmInfo.public_ips : []
})
const snapshotQuotaText = computed(() => {
  const quota = snapshotQuota.value
  if (!quota) return ''
  const used = quota.used_snapshots || 0
  const max = quota.max_snapshots || 0
  return max > 0 ? `${used}/${max}` : `${used}/不限`
})

const cpuPercentStr = computed(() => {
  const v = vmInfo.stats?.cpu_percent
  return v != null && v >= 0 ? v.toFixed(1) + '%' : '0.0%'
})
const memPercentStr = computed(() => {
  const used = vmInfo.stats?.mem_used
  const total = vmInfo.stats?.mem_total
  if (used != null && total != null && total > 0) {
    return (used / total * 100).toFixed(1) + '%'
  }
  return '0.0%'
})
const netRxStr = computed(() => {
  const v = vmInfo.stats?.net_rx_rate
  return v != null ? formatTraffic(v) : '0 KB/s'
})
const netTxStr = computed(() => {
  const v = vmInfo.stats?.net_tx_rate
  return v != null ? formatTraffic(v) : '0 KB/s'
})
const diskRdStr = computed(() => {
  const v = vmInfo.stats?.disk_rd_rate
  return v != null ? formatTraffic(v) : '0 KB/s'
})
const diskWrStr = computed(() => {
  const v = vmInfo.stats?.disk_wr_rate
  return v != null ? formatTraffic(v) : '0 KB/s'
})

const scrollToSection = (id) => {
  // 点击导航时主动触发对应区域加载
  if (id === 'monitor' && !showMonitor.value) {
    showMonitor.value = true
  }
  if (id === 'config' && !showInfoCards.value) {
    showInfoCards.value = true
    nextTick(() => {
      if (!isLightweight.value && diskIopsList.value.length === 0) {
        loadDiskIOPSList()
      }
    })
  }
  const el = document.getElementById(id)
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}

const handleSnapshotQuotaChange = (quota) => {
  snapshotQuota.value = quota || null
}

const scrollToTop = () => {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

const handleScroll = () => {
  showBackToTop.value = window.scrollY > 400
}

const resetPasswordForm = reactive({
  username: '',
  password: ''
})

const canResetGuestPassword = computed(() => {
  return ['linux', 'fnos', 'windows'].includes(vmInfo.os_type) && vmInfo.status === 'shut off'
})
const isWindowsGuest = computed(() => vmInfo.os_type === 'windows')
const resetPasswordDialogTitle = computed(() => {
  if (vmInfo.os_type === 'fnos') return '重置 fnOS 登录密码'
  if (vmInfo.os_type === 'windows') return '重置 Windows 登录密码'
  return '重置虚拟机登录密码'
})
const resetPasswordAlertText = computed(() => {
  if (vmInfo.os_type === 'windows') {
    return '该操作会在虚拟机关机状态下注入 Windows 一次性重置脚本，不需要旧密码。任务完成后请手动开机一次，系统会自动处理并自动关机。'
  }
  return '该操作会在虚拟机关机状态下离线修改登录密码，不需要旧密码。提交后请在任务中心查看进度。'
})
const resetPasswordUsernameHint = computed(() => {
  if (vmInfo.os_type !== 'windows') return ''
  return 'Windows 默认账号为 administrator；Windows Server 修改此项通常无效，建议保持默认。'
})

const statusText = (status) => {
  const map = { running: '运行中', 'shut off': '已关机', paused: '已暂停', migrating: '迁移中' }
  return map[status] || status
}

const statusTagType = (status) => {
  const map = { running: 'success', 'shut off': 'info', paused: 'warning', migrating: 'danger' }
  return map[status] || 'info'
}

const validateResetUsername = (_, value, callback) => {
  if (!value) {
    callback(new Error('请输入要重置的用户名'))
    return
  }
  if (isWindowsGuest.value) {
    const trimmed = value.trim()
    if (!trimmed) {
      callback(new Error('请输入要重置的用户名'))
      return
    }
    if (trimmed.length > 64) {
      callback(new Error('Windows 用户名长度不能超过 64 个字符'))
      return
    }
    if (windowsUsernameInvalidPattern.test(trimmed)) {
      callback(new Error('Windows 用户名包含不支持的字符'))
      return
    }
    callback()
    return
  }
  if (!usernamePattern.test(value)) {
    callback(new Error('用户名只能以小写字母或下划线开头，且只能包含小写字母、数字、下划线和短横线'))
    return
  }
  callback()
}

const validateResetPassword = (_, value, callback) => {
  if (!value) {
    callback(new Error('请输入新密码'))
    return
  }
  passwordValidator(_, value, callback)
}

const resetPasswordRules = {
  username: [{ validator: validateResetUsername, trigger: 'blur' }],
  password: [{ validator: validateResetPassword, trigger: 'blur' }]
}

const openResetPasswordDialog = () => {
  if (!['linux', 'fnos', 'windows'].includes(vmInfo.os_type)) {
    ElMessage.warning('当前仅支持 Linux、Windows 或 fnOS 虚拟机重置密码')
    return
  }
  if (vmInfo.status !== 'shut off') {
    ElMessage.warning('请先将虚拟机关机后再重置密码')
    return
  }
  resetPasswordForm.username = vmInfo.credential?.username || (vmInfo.os_type === 'windows' ? 'administrator' : '')
  resetPasswordForm.password = vmInfo.credential?.password || genPwd()
  resetPasswordDialogVisible.value = true
}

const handleGenerateResetPassword = async () => {
  resetPasswordForm.password = genPwd()
  await resetPasswordFormRef.value?.validateField('password').catch(() => false)
}

const copyCredentialField = async (value, fieldName) => {
  if (!value) {
    ElMessage.warning(`暂无可复制的${fieldName}`)
    return
  }
  try {
    await copyTextWithFallback(value)
    ElMessage.success(`${fieldName}已复制到剪贴板`)
  } catch {
    ElMessage.warning(`复制${fieldName}失败，请手动复制`)
  }
}

const submitResetPassword = async () => {
  const valid = await resetPasswordFormRef.value?.validate().catch(() => false)
  if (!valid) return
  // 异步泄露密码检测（HIBP API）
  const breach = await checkPasswordBreachAsync(resetPasswordForm.password)
  if (breach.enabled && breach.breached) {
    ElMessage.error('该密码已在已知泄露数据库中发现，请更换为更安全的密码')
    return
  }
  resetPasswordSubmitting.value = true
  try {
    const res = await resetVmLinuxPassword(vmName.value, {
      username: resetPasswordForm.username.trim(),
      password: resetPasswordForm.password
    })
    const defaultMessage = vmInfo.os_type === 'windows'
      ? 'Windows 重置任务已提交，任务完成后请手动开机一次等待系统自动处理并关机'
      : '重置密码任务已提交'
    ElMessage.success(vmInfo.os_type === 'windows' ? defaultMessage : (res.message || defaultMessage))
    resetPasswordDialogVisible.value = false
  } finally {
    resetPasswordSubmitting.value = false
  }
}

// ==================== SSE 连接管理 ====================
let eventSource = null
let reconnectTimer = null
let prevStats = null
let prevStatsTime = 0

const initSSE = () => {
  if (!vmName.value || vmName.value === 'undefined') return
  closeSSE()

  const token = userStore.token
  eventSource = createVmDetailSSE(vmName.value, token)

  eventSource.addEventListener('vm_detail', (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data && data.name) {
        if (vmInfo.status !== data.status) {
          operating.value = false
        }
        // 网络/磁盘增量计算：后端返回累计字节，前端计算速率
        if (data.stats) {
          const now = Date.now()
          if (prevStats && prevStatsTime > 0) {
            const dt = (now - prevStatsTime) / 1000
            data.stats.net_rx_rate = Math.max(0, (data.stats.net_rx_bytes - prevStats.net_rx_bytes) / dt)
            data.stats.net_tx_rate = Math.max(0, (data.stats.net_tx_bytes - prevStats.net_tx_bytes) / dt)
            data.stats.disk_rd_rate = Math.max(0, (data.stats.disk_rd_bytes - prevStats.disk_rd_bytes) / dt)
            data.stats.disk_wr_rate = Math.max(0, (data.stats.disk_wr_bytes - prevStats.disk_wr_bytes) / dt)
          }
          prevStats = {
            net_rx_bytes: data.stats.net_rx_bytes,
            net_tx_bytes: data.stats.net_tx_bytes,
            disk_rd_bytes: data.stats.disk_rd_bytes,
            disk_wr_bytes: data.stats.disk_wr_bytes
          }
          prevStatsTime = now
        }
        Object.assign(vmInfo, data)
        vmStore.addVisitedVm({ id: vmName.value, name: vmInfo.name || vmName.value })
        if (!pageReady.value) {
          pageReady.value = true
          // 页面数据准备就绪后，下一帧初始化懒加载观察器
          nextTick(() => {
            initLazySections()
            observeLazySections()
          })
          // 获取所有网口 IP
          fetchAllInterfaceIPs()
          // 获取 PCIe 热插槽使用信息
          loadPCIEInfo()
        }
      }
    } catch (e) {
      console.error('解析 SSE 详情数据失败', e)
    }
  })

  eventSource.onerror = () => {
    console.error('VM 详情 SSE 连接错误，5秒后重连...')
    closeSSE()
    reconnectTimer = setTimeout(() => {
      initSSE()
    }, 5000)
  }
}

const closeSSE = () => {
  prevStats = null
  prevStatsTime = 0
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

// ==================== 电源操作 ====================
const handleAction = async (action) => {
  if (vmInfo.status === 'migrating') {
    ElMessage.warning('虚拟机正在迁移中，暂不能执行操作')
    return
  }
  operating.value = true
  try {
    const res = await operateVm(vmName.value, action)
    const actionMap = {
      start: '开机', reboot: '重启', shutdown: '关机',
      destroy: '断电', reset: '重置'
    }
    const msg = actionMap[action] || action
    if (res.status === 'no_change') {
      ElMessage.info(res.message || `${msg}操作已执行，状态未改变`)
    } else {
      ElMessage.success(res.message || `${msg}操作成功`)
    }
  } catch (e) {
    console.error(`${action} 操作失败`, e)
  }
}

// ==================== 虚拟机锁定 ====================
const handleLockAction = async (action) => {
  operating.value = true
  try {
    if (action === 'lock') {
      await lockVm(vmName.value)
      ElMessage.success('虚拟机已锁定')
      vmInfo.locked = true
    } else {
      await unlockVm(vmName.value)
      ElMessage.success('虚拟机已解锁')
      vmInfo.locked = false
    }
  } catch (e) {
    console.error(`${action} 操作失败`, e)
  } finally {
    operating.value = false
  }
}

// ==================== 编辑虚拟机 ====================
const handleEdit = () => {
  if (isLightweight.value) return
  if (vmFormRef.value) {
    vmFormRef.value.open(vmInfo)
  }
}

const openReinstallDialog = () => {
  if (isLightweight.value || vmInfo.status === 'migrating') return
  reinstallRef.value?.open(vmInfo)
}

const handleEditSuccess = () => {
  initSSE()
}

const handleEditRemark = () => {
  if (isLightweight.value || vmInfo.status === 'migrating') return
  vmRemarkDialogRef.value?.open(vmInfo.name || vmName.value, vmInfo.remark || '')
}

const handleRemarkSuccess = ({ remark }) => {
  vmInfo.remark = remark || ''
}

// ==================== 开发者面板切换 ====================
const toggleDeveloperTab = () => {
  showDeveloperTab.value = !showDeveloperTab.value
}

const previousTab = ref('snapshot')

const handleTabClick = (tab) => {
  if (tab.paneName === 'vnc' && window.innerWidth <= 768) {
    window.open(`/vm/${vmName.value}/vnc-window`, '_blank')
    activeTab.value = previousTab.value
    return
  }
  if (tab.paneName === 'spice') {
    refreshSpiceStatus()
    refreshSpiceConnInfo()
  }
  previousTab.value = tab.paneName
}

// ==================== 生命周期 ====================
watch(() => vmName.value, (newVal, oldVal) => {
  if (newVal && newVal !== oldVal && route.path.includes('/vm/detail/')) {
    activeTab.value = 'snapshot'
    showDeveloperTab.value = false
    pageReady.value = false
    showMonitor.value = false
    showInfoCards.value = false
    diskIopsList.value = []
    if (sectionObserver) {
      sectionObserver.disconnect()
      sectionObserver = null
    }
    initSSE()
  }
})

// 加载磁盘 IOPS 列表（仅在信息卡片区可见时调用）
async function loadDiskIOPSList() {
  diskIopsLoading.value = true
  try {
    const res = await getDiskList(vmName.value)
    diskIopsList.value = (res.data || []).filter(d => d.device_type !== 'cdrom')
  } catch (e) {
    console.error('加载磁盘 IOPS 列表失败', e)
    diskIopsList.value = []
  } finally {
    diskIopsLoading.value = false
  }
}

// 加载 PCIe 热插槽使用信息
async function loadPCIEInfo() {
  try {
    const res = await getVmPCIEInfo(vmName.value)
    if (res.data) {
      vmInfo.pcie_info = res.data
    }
  } catch (e) {
    console.error('加载 PCIe 热插槽信息失败', e)
    vmInfo.pcie_info = null
  }
}

onMounted(() => {
  initSSE()
  window.addEventListener('scroll', handleScroll)
  // 磁盘 IOPS 不再在页面挂载时加载，改为信息卡片区进入视口时按需加载
})

// ==================== SPICE 显示协议 ====================
// 与 VNC 共存，供使用 virt-viewer/spicy 的外部客户端连接。
const refreshSpiceStatus = async () => {
  try {
    const res = await getSpiceStatus(vmName.value)
    vmInfo.spiceInfo = res.data || { enabled: false, port: '', has_password: false, exposed: false }
  } catch (e) {
    // 静默失败，不打扰用户
  }
}

const refreshSpiceConnInfo = async () => {
  try {
    const res = await getSpiceConnInfo(vmName.value)
    vmInfo.spiceConnInfo = res.data || { host: '', port: '', password: '', exposed: false }
  } catch (e) {
    // 静默失败
  }
}

const handleSpiceEnable = async () => {
  try {
    await ElMessageBox.confirm('开启 SPICE 将修改虚拟机配置。运行中的虚拟机会立即重启以使其生效，关机的虚拟机在下次启动时生效。确认继续？', '开启 SPICE', {
      confirmButtonText: '确认开启',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch (e) {
    return
  }
  vmInfo.spiceLoading = true
  try {
    await enableSpice(vmName.value)
    ElMessage.success('SPICE 已开启')
    await refreshSpiceStatus()
  } catch (e) {
    /* request.js 已弹错 */
  } finally {
    vmInfo.spiceLoading = false
  }
}

const handleSpiceDisable = async () => {
  try {
    await ElMessageBox.confirm('关闭 SPICE 将修改虚拟机配置，运行中的虚拟机会立即重启以生效并断开所有外部 SPICE 客户端连接。确认？', '关闭 SPICE', {
      confirmButtonText: '确认关闭',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch (e) {
    return
  }
  vmInfo.spiceLoading = true
  try {
    await disableSpice(vmName.value)
    ElMessage.success('SPICE 已关闭')
    await refreshSpiceStatus()
  } catch (e) {
    /* request.js 已弹错 */
  } finally {
    vmInfo.spiceLoading = false
  }
}

const handleSpiceExpose = async (expose) => {
  vmInfo.spiceLoading = true
  try {
    await exposeSpice(vmName.value, expose)
    ElMessage.success(expose ? 'SPICE 已对外暴露' : 'SPICE 已关闭对外暴露')
    await refreshSpiceStatus()
    if (expose) {
      await refreshSpiceConnInfo()
    }
  } catch (e) {
    // 失败时回滚开关显示
    vmInfo.spiceInfo.exposed = !expose
  } finally {
    vmInfo.spiceLoading = false
  }
}

const handleSpiceChangePassword = async () => {
  let password
  try {
    const res = await ElMessageBox.prompt('请输入新的 SPICE 密码', '修改 SPICE 密码', {
      confirmButtonText: '确认',
      cancelButtonText: '取消',
      inputPattern: /^\S+$/,
      inputErrorMessage: '密码不能包含空格'
    })
    password = res.value
  } catch (e) {
    return
  }
  await changeSpicePassword(vmName.value, password)
  ElMessage.success('SPICE 密码已修改')
  await refreshSpiceStatus()
}

const spiceConnReady = computed(() => vmInfo.status === 'running' || vmInfo.status === 'paused')
const handleDownloadSpiceVV = async (deleteFile = true) => {
  // SPICE 端口由 QEMU 运行时分配（autoport），关机态无端口，下载的 .vv 会连不上
  if (!spiceConnReady.value) {
    ElMessage.warning('虚拟机未运行，SPICE 端口尚未分配，请先启动虚拟机后再下载连接文件')
    return
  }
  try {
    const blob = await downloadSpiceVV(vmName.value, deleteFile)
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${vmName.value}.vv`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    window.URL.revokeObjectURL(url)
  } catch (e) {
    ElMessage.error('下载 .vv 文件失败')
  }
}

onUnmounted(() => {
  closeSSE()
  window.removeEventListener('scroll', handleScroll)
  if (sectionObserver) {
    sectionObserver.disconnect()
    sectionObserver = null
  }
})
</script>

<style scoped>
.vm-detail-container {
  margin: 0 -16px;
  padding: 0 0 60px;
  font-family: inherit;
}

/* ==================== 顶部导航栏 ==================== */
.detail-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 0 18px;
  gap: 16px;
}
.back-link {
  font-size: 15px;
  color: var(--el-text-color-secondary);
  padding: 0;
}
.back-link:hover {
  color: var(--el-color-primary);
}
.back-link :deep(.el-icon) {
  margin-right: 6px;
}
.top-bar-right {
  display: flex;
  align-items: center;
  gap: 18px;
}
.quick-nav {
  display: flex;
  gap: 4px;
}
.quick-nav a {
  text-decoration: none;
  font-size: 13px;
  color: var(--el-text-color-secondary);
  padding: 5px 12px;
  border-radius: 12px;
  transition: all 0.2s;
  white-space: nowrap;
}
.quick-nav a:hover {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

/* ==================== Hero 状态横幅 ==================== */
.hero-banner {
  background: var(--el-bg-color);
  border-radius: 16px;
  padding: 24px 32px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.05);
  display: flex;
  align-items: center;
  gap: 24px;
  margin-bottom: 24px;
  border: 1px solid var(--el-border-color-lighter);
  position: relative;
  overflow: hidden;
  transition: box-shadow 0.3s;
}
.hero-banner::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  border-radius: 0 4px 4px 0;
}
.hero-banner.hero-running::before {
  background: var(--el-color-success);
}
.hero-banner.hero-paused::before {
  background: var(--el-color-warning);
}
.hero-banner.hero-stopped::before {
  background: var(--el-text-color-placeholder);
}

.hero-status-indicator {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.hero-status-indicator.running {
  background: linear-gradient(135deg, #ecfdf5, #d1fae5);
}
.hero-status-indicator.paused {
  background: linear-gradient(135deg, #fffbeb, #fef3c7);
}
.hero-status-indicator.stopped {
  background: var(--el-fill-color-light);
}


.hero-info {
  flex: 1;
  min-width: 0;
}
.hero-vm-name {
  font-size: 24px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  margin: 0 0 8px;
  letter-spacing: -0.3px;
}
.hero-meta {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  font-size: 14px;
  color: var(--el-text-color-secondary);
}
.meta-divider {
  width: 1px;
  height: 14px;
  background: var(--el-border-color);
}
.meta-item {
  display: flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}
.hero-ip-label {
  color: var(--el-text-color-secondary);
}
.hero-ip-value {
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
}
.ip-unreachable {
  color: var(--el-color-warning);
  background: var(--el-color-warning-light-9);
  cursor: help;
  text-decoration: underline;
  text-decoration-style: dotted;
  text-underline-offset: 2px;
}
.hero-public-ip-list {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.public-ip-value {
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
}
.dynamic-mem-tag {
  cursor: help;
}

.hero-power-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-shrink: 0;
}
.hero-lock-action {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-shrink: 0;
  margin-left: 12px;
}
.migration-alert {
  max-width: 460px;
}
.power-btn {
  white-space: nowrap;
}

/* ==================== 实时资源概览条 ==================== */
.live-stats-strip {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 18px;
  margin-bottom: 24px;
}
.live-stat-card {
  background: var(--el-bg-color);
  border-radius: 14px;
  padding: 18px 22px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.04);
  border: 1px solid var(--el-border-color-lighter);
  display: flex;
  align-items: center;
  gap: 16px;
  transition: box-shadow 0.2s;
}
.live-stat-card:hover {
  box-shadow: 0 4px 12px rgba(0,0,0,0.06);
}
.live-stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}
.cpu-icon { background: #eef2ff; color: #6366f1; }
.mem-icon { background: #fffbeb; color: #d97706; }
.net-icon { background: #ecfdf5; color: #059669; }
.disk-icon { background: var(--el-fill-color); color: #475569; }
.live-stat-info { flex: 1; min-width: 0; }
.live-stat-label {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  text-transform: uppercase;
  letter-spacing: 0.3px;
  margin-bottom: 5px;
}
.live-stat-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}
.live-stat-sub {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  margin-top: 3px;
}
.mini-progress {
  height: 5px;
  border-radius: 10px;
  background: var(--el-border-color-lighter);
  overflow: hidden;
  margin-top: 8px;
}
.mini-progress-fill {
  height: 100%;
  border-radius: 10px;
  transition: width 0.8s ease;
}
.cpu-fill { background: #6366f1; }
.mem-fill { background: #d97706; }

/* ==================== 功能 Tabs 区 ==================== */
.function-tabs-section {
  background: var(--el-bg-color);
  border-radius: 16px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.04);
  border: 1px solid var(--el-border-color-lighter);
  overflow: hidden;
  margin-bottom: 24px;
}
.tabs-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  position: relative;
}
.custom-tabs {
  flex: 1;
  padding: 0;
}
.custom-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 140px 0 20px;
  background: var(--el-fill-color-lighter);
}
.custom-tabs :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
}
.custom-tabs :deep(.el-tabs__item) {
  padding: 0 22px;
  height: 48px;
  line-height: 48px;
  font-size: 14px;
  font-weight: 500;
}
.tab-label-text {
  display: flex;
  align-items: center;
  gap: 7px;
}
.tab-quota-tag {
  margin-left: 2px;
}
.custom-tabs :deep(.el-tabs__content) {
  padding: 28px;
}
.dev-toggle-link {
  position: absolute;
  right: 16px;
  top: 10px;
  font-size: 13px;
  white-space: nowrap;
  z-index: 1;
}

/* ==================== 监控图表区 ==================== */
.monitor-section {
  margin-bottom: 24px;
}
.section-title-bar {
  margin-bottom: 16px;
}
.section-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

/* ==================== 信息卡片区（底部） ==================== */
.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
}
.info-card {
  background: var(--el-bg-color);
  border-radius: 14px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.04);
  border: 1px solid var(--el-border-color-lighter);
  overflow: hidden;
  transition: box-shadow 0.25s;
}
.info-card:hover {
  box-shadow: 0 4px 12px rgba(0,0,0,0.06);
}
.info-card + .info-card {
  margin-top: 24px;
}
.info-card-header {
  padding: 17px 22px 13px;
  display: flex;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.card-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
}
.config-icon { background: var(--el-color-primary-light-9); color: var(--el-color-primary); }
.credential-icon { background: var(--el-color-warning-light-9); color: var(--el-color-warning); }
.network-icon { background: var(--el-color-success-light-9); color: var(--el-color-success); }
.advanced-icon { background: var(--el-fill-color); color: var(--el-text-color-secondary); }
.card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.info-card-body { padding: 2px 0; }
.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 22px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  transition: background 0.15s;
  font-size: 14px;
}
.info-row:last-child { border-bottom: none; }
.info-row:hover { background: var(--el-fill-color-lighter); }
.public-ip-list {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  flex-wrap: wrap;
  min-width: 0;
}
.public-ip-tag {
  margin: 0;
}
.info-row-highlight {
  background: var(--el-color-primary-light-9);
  margin: 4px 12px 0;
  border-radius: 8px;
  padding: 12px 12px !important;
  border-bottom: none;
}
.info-label {
  color: var(--el-text-color-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
}
.sub-label {
  font-size: 10px;
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
  padding: 1px 6px;
  border-radius: 3px;
}
.runtime-sub-label {
  white-space: nowrap;
}
.highlight-label { color: var(--el-color-primary); }
.info-value {
  color: var(--el-text-color-primary);
  font-weight: 500;
  text-align: right;
  display: flex;
  align-items: center;
  gap: 8px;
}
.remark-info-value {
  max-width: 70%;
  justify-content: flex-end;
}
.remark-text {
  white-space: normal;
  word-break: break-word;
}
.info-value.mono {
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  font-size: 13px;
}
.ip-highlight { color: var(--el-color-primary); }
.info-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 10px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 600;
}
.tag-primary { background: var(--el-color-primary-light-9); color: var(--el-color-primary); }
.tag-success { background: var(--el-color-success-light-9); color: var(--el-color-success); }
.tag-warning { background: var(--el-color-warning-light-9); color: var(--el-color-warning); }
.tag-info { background: var(--el-fill-color); color: var(--el-text-color-secondary); }
.credential-value-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}
.credential-code {
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  background: var(--el-fill-color-lighter);
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 13px;
  color: var(--el-text-color-primary);
  border: 1px solid var(--el-border-color-lighter);
}
.credential-input {
  width: 200px;
}

/* ==================== 磁盘 IOPS 表格 ==================== */
.disk-iops-icon { background: var(--el-color-primary-light-9); color: var(--el-color-primary); }

.disk-iops-table {
  width: 100%;
  padding: 0 12px;
}

.disk-iops-header {
  display: flex;
  align-items: center;
  padding: 8px 10px;
  background: var(--el-fill-color-lighter);
  border-radius: 6px;
  margin-bottom: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}

.disk-iops-row {
  display: flex;
  align-items: center;
  padding: 8px 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
  transition: background 0.15s;
}

.disk-iops-row:last-child { border-bottom: none; }
.disk-iops-row:hover { background: var(--el-fill-color-lighter); }

.disk-iops-col { flex-shrink: 0; }
.device-col { width: 90px; }
.capacity-col { width: 70px; }
.iops-col { width: 80px; text-align: right; padding-right: 8px; }

.disk-dev-name {
  font-weight: 600;
  color: var(--el-text-color-primary);
  font-family: 'JetBrains Mono', 'Consolas', monospace;
}
.disk-dev-bus { margin-left: 4px; font-size: 11px; color: var(--el-text-color-placeholder); }

.iops-limited { color: var(--el-color-warning); font-weight: 600; }
.iops-none { color: var(--el-text-color-placeholder); }

.disk-iops-empty {
  text-align: center;
  padding: 20px;
  color: var(--el-text-color-placeholder);
  font-size: 13px;
}

.card-refresh-btn {
  margin-left: auto;
  font-size: 12px;
}

/* ==================== 返回顶部 ==================== */
.back-to-top-btn {
  position: fixed;
  bottom: 28px;
  right: 28px;
  z-index: 200;
  box-shadow: 0 4px 16px rgba(0,0,0,0.1);
}
.fade-up-enter-active,
.fade-up-leave-active {
  transition: all 0.3s ease;
}
.fade-up-enter-from,
.fade-up-leave-to {
  opacity: 0;
  transform: translateY(12px);
}

/* ==================== 懒加载骨架屏 ==================== */
.below-hero-loading {
  margin-top: 0;
}

.lazy-section-placeholder {
  background: var(--el-bg-color);
  border-radius: 14px;
  padding: 20px;
  border: 1px solid var(--el-border-color-lighter);
}

.lazy-card-skeleton .info-card-body {
  padding: 16px 22px;
}

/* ==================== 表单辅助 ==================== */
.form-inline-hint {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

/* ==================== 响应式布局 ==================== */
@media (max-width: 1024px) {
  .live-stats-strip {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 900px) {
  .info-grid {
    grid-template-columns: 1fr;
  }
  .hero-banner {
    flex-direction: column;
    align-items: flex-start;
    gap: 14px;
  }
  .hero-power-actions {
    width: 100%;
  }
  .quick-nav {
    display: none;
  }
}
@media (max-width: 600px) {
  .live-stats-strip {
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .live-stat-card {
    padding: 14px 16px;
    gap: 12px;
  }
  .live-stat-value {
    font-size: 18px;
  }
  .hero-vm-name {
    font-size: 20px;
  }
  .hero-meta {
    gap: 12px;
    font-size: 13px;
  }

  .custom-tabs :deep(.el-tabs__header) {
    padding: 0 4px !important;
    margin: 0 !important;
    overflow-x: auto !important;
    -webkit-overflow-scrolling: touch;
  }

  .custom-tabs :deep(.el-tabs__item) {
    padding: 0 14px !important;
    font-size: 13px !important;
  }
}
@media (max-width: 480px) {
  .vm-detail-container {
    margin: 0 -8px;
    overflow-x: hidden;
  }

  .detail-top-bar {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 0 14px;
  }

  .top-bar-right {
    width: 100%;
    justify-content: flex-end;
  }

  .hero-banner {
    padding: 16px 20px;
    border-radius: 12px;
    margin-bottom: 16px;
  }

  .hero-vm-name {
    font-size: 18px;
  }

  .hero-meta {
    gap: 8px;
    font-size: 12px;
  }

  .meta-item {
    white-space: normal;
    flex-wrap: wrap;
  }

  .meta-divider {
    display: none;
  }

  .hero-ip-value {
    font-size: 11px;
    padding: 1px 6px;
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hero-power-actions {
    flex-wrap: wrap;
  }

  .power-btn {
    font-size: 12px;
    padding: 7px 12px;
  }

  .live-stats-strip {
    grid-template-columns: 1fr 1fr;
    gap: 8px;
    margin-bottom: 16px;
  }

  .live-stat-card {
    padding: 12px 14px;
    gap: 8px;
    border-radius: 10px;
  }

  .live-stat-icon {
    width: 36px;
    height: 36px;
    border-radius: 10px;
    font-size: 16px;
  }

  .live-stat-value {
    font-size: 16px;
  }

  .custom-tabs :deep(.el-tabs__header) {
    padding: 0 4px !important;
    margin: 0 !important;
    overflow-x: auto !important;
    overflow-y: hidden !important;
    -webkit-overflow-scrolling: touch;
    width: 100% !important;
    max-width: 100vw !important;
  }

  .custom-tabs :deep(.el-tabs__nav-wrap) {
    overflow: visible !important;
    width: auto !important;
  }

  .custom-tabs :deep(.el-tabs__nav-scroll) {
    overflow-x: auto !important;
    overflow-y: hidden !important;
    -webkit-overflow-scrolling: touch;
  }

  .custom-tabs :deep(.el-tabs__nav) {
    white-space: nowrap !important;
    display: flex !important;
    flex-wrap: nowrap !important;
  }

  .custom-tabs :deep(.el-tabs__item) {
    padding: 0 10px !important;
    font-size: 12px !important;
    height: 40px !important;
    line-height: 40px !important;
    flex-shrink: 0;
  }

  .tab-label-text {
    gap: 4px;
    font-size: 12px;
  }

  .custom-tabs :deep(.el-tabs__content) {
    padding: 12px !important;
  }

  .info-grid {
    gap: 16px;
  }

  .info-row {
    padding: 10px 14px;
    font-size: 13px;
    flex-wrap: wrap;
    gap: 6px;
  }

  .info-card-header {
    padding: 14px 16px 10px;
  }

  .info-value {
    font-size: 12px;
  }

  .back-to-top-btn {
    bottom: 80px;
    right: 16px;
  }

  .migration-alert {
    max-width: 100%;
  }

  .dev-toggle-link {
    position: static;
    margin: 6px 12px 10px;
    display: block;
    text-align: right;
  }
}

/* SPICE 面板（与 VNC 控制台共处一个 tab） */
.spice-panel {
  margin-top: 16px;
  padding: 14px 16px;
  border: 1px solid var(--el-border-color-lighter, #e4e7ed);
  border-radius: 8px;
  background: var(--el-bg-color-page, #f5f7fa);
}
.spice-panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.spice-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--el-text-color-primary, #303133);
}
.spice-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 8px;
}
.spice-info {
  margin-top: 6px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.spice-info-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.spice-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
  line-height: 1.5;
}
</style>
