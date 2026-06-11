<template>
  <el-dialog :title="dialogTitle" v-model="visible" width="1100px" @closed="onClosed" align-center
    :close-on-click-modal="false" class="vm-dialog-new" append-to-body>
    <!-- ==================== 双栏布局 ==================== -->
    <div class="vm-form-layout">
      <div class="vm-form-left">
        <el-form :model="form" :rules="currentRules" ref="formRef" label-width="140px" class="vm-form">
      <!-- ==================== 编辑模式：选项卡 ==================== -->
      <template v-if="isEdit">
        <div class="edit-tabs-bar">
          <div v-for="tab in editTabs" :key="tab.name"
            :class="['edit-tab-item', { active: activeTabEdit === tab.name }]"
            @click="activeTabEdit = tab.name">
            <el-icon class="edit-tab-icon"><component :is="tab.icon" /></el-icon>
            <span class="edit-tab-label">{{ tab.label }}</span>
          </div>
        </div>
        <div class="edit-tab-content">
        <el-tabs v-model="activeTabEdit" class="vm-tabs vm-tabs-hide-header">
        <el-tab-pane label="基础配置" name="basic">
          <div class="tab-content-wrapper">
            <el-form-item label="虚拟机名称" prop="name">
              <el-input v-model="form.name" disabled />
            </el-form-item>
            <el-form-item label="状态">
              <el-tag :type="editVmStatus === 'running' ? 'success' : 'info'" style="margin-right: 12px;">
                {{ editVmStatus === 'running' ? '运行中' : '已关机' }}
              </el-tag>
              <template v-if="editVmVNC">
                <el-tag type="warning">VNC {{ editVmVNC }}</el-tag>
              </template>
            </el-form-item>
            <el-divider content-position="left"><el-icon style="margin-right: 4px;"><Cpu /></el-icon>CPU / 内存</el-divider>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="CPU 核心" prop="vcpu">
                  <el-input-number v-model="form.vcpu" :min="vcpuMin" :max="32" style="width: 100%;" :disabled="isEdit && editVmStatus === 'running' && !form.cpu_hotplug_enabled" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="内存(GB)" prop="memory">
                  <el-input-number v-model="form.memory" :min="memoryMin" :max="64" :step="1" style="width: 100%;" @change="handleBaseMemoryChange" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="CPU 热添加">
              <div class="memory-basic-switch">
                <div class="memory-basic-switch-row">
                  <el-switch v-model="form.cpu_hotplug_enabled" active-text="启用" inactive-text="关闭" @change="handleCPUHotplugChange" />
                  <template v-if="form.cpu_hotplug_enabled && hostCPUCores > 0">
                    <span class="memory-dynamic-unit" style="margin-left: 8px;">上限 {{ hostCPUCores }} 核</span>
                  </template>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  启用后可在宿主机 {{ hostCPUCores > 0 ? hostCPUCores : '?' }} 核范围内随时热添加 vCPU，无需重启。新建时需重启一次后热添加功能生效
                </div>
              </div>
            </el-form-item>
            <el-form-item v-if="isAdmin" label="CPU 限制">
              <div class="memory-basic-switch">
                <div class="memory-basic-switch-row">
                  <el-switch v-model="form.cpu_limit_enabled" active-text="启用限制" inactive-text="无限制" />
                  <template v-if="form.cpu_limit_enabled">
                    <el-input-number v-model="form.cpu_limit_percent" :min="1" :max="100" :step="1" style="width: 160px;" />
                    <span class="memory-dynamic-unit">%</span>
                  </template>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  按当前配置的 vCPU 总能力限速；50% 表示限制为当前已分配 CPU 总能力的一半
                </div>
              </div>
            </el-form-item>
            <el-form-item label="动态内存">
              <div class="memory-basic-switch">
                <div class="memory-basic-switch-row">
                  <el-switch v-model="form.memory_dynamic_enabled" active-text="启用" inactive-text="关闭" @change="handleDynamicMemoryEnabledChange" />
                  <el-radio-group v-if="showMemoryBackendQuickSelect" v-model="form.memory_backend" size="small" @change="handleMemoryBackendChange">
                    <el-radio-button label="balloon">气球调度</el-radio-button>
                    <el-radio-button label="virtio_mem" :disabled="windowsElasticMemoryDisabled">Windows 弹性内存</el-radio-button>
                  </el-radio-group>
                  <el-tag v-if="showMemoryBackendQuickSelect && form.memory_backend === 'virtio_mem'" type="warning" effect="plain">弹性内存</el-tag>
                  <el-button v-if="showMemoryBackendQuickSelect && form.memory_backend === 'virtio_mem'" size="small" text type="primary" @click="memoryVirtioMemDetailVisible = true">详情</el-button>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  {{ basicMemoryDynamicTip }}
                </div>
              </div>
            </el-form-item>
            <div v-if="editVmStatus === 'running'" class="form-tip" style="margin-bottom: 16px;">
              <el-icon><WarningFilled /></el-icon>
              运行中修改 CPU/内存为热插拔，部分配置可能需要重启后生效
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="磁盘与光驱" name="disk">
          <div class="tab-content-wrapper">
            <el-form-item label="当前磁盘" v-if="editDisks.length > 0">
              <el-table :data="editDisks" size="small" border style="width: 100%;">
                <el-table-column prop="device" label="设备" width="70" />
                <el-table-column label="容量" width="90">
                  <template #default="{ row }">{{ row.capacity_gb ? row.capacity_gb + ' GB' : '-' }}</template>
                </el-table-column>
                <el-table-column label="占用" width="90">
                  <template #default="{ row }">{{ row.used_gb ? row.used_gb + ' GB' : '-' }}</template>
                </el-table-column>
                <el-table-column prop="format" label="格式" width="70" />
                <el-table-column label="驱动" width="110">
                  <template #default="{ row }">
                    <el-select v-model="row.bus" size="small" :disabled="editVmStatus === 'running'" @change="(val) => handleChangeDiskBus(row, val)" style="width: 90px;">
                      <el-option label="VirtIO" value="virtio" />
                      <el-option label="SCSI" value="scsi" />
                      <el-option label="SATA" value="sata" />
                      <el-option label="IDE" value="ide" />
                    </el-select>
                  </template>
                </el-table-column>
                <el-table-column prop="path" label="路径" min-width="150" show-overflow-tooltip />
                <el-table-column label="操作" width="200" align="center">
                  <template #default="{ row }">
                    <el-button size="small" type="primary" plain @click="handleResizeDisk(row)">扩容</el-button>
                    <el-button v-if="isAdmin" size="small" type="warning" plain @click="openDiskIOPSDialog(row)">IOPS</el-button>
                    <el-button size="small" type="danger" plain @click="handleRemoveDisk(row)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </el-form-item>

            <el-form-item label="新增磁盘">
              <div v-for="(disk, index) in form.add_disks" :key="index" style="display: flex; gap: 8px; margin-bottom: 8px; width: 100%;">
                <el-input-number v-model="disk.size" :min="1" :max="2000" placeholder="大小(GB)" style="width: 120px;" />
                <el-select v-model="disk.format" style="width: 100px;">
                  <el-option label="qcow2" value="qcow2" />
                  <el-option v-if="isAdmin" label="raw" value="raw" />
                </el-select>
                <el-select v-model="disk.bus" placeholder="驱动" style="width: 110px;">
                  <el-option label="VirtIO" value="virtio" />
                  <el-option label="SCSI" value="scsi" />
                  <el-option label="SATA" value="sata" />
                  <el-option label="IDE" value="ide" />
                </el-select>
                <el-select v-model="disk.storage_pool_id" placeholder="默认存储" clearable filterable style="width: 180px;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
                  <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id" />
                </el-select>
                <span style="line-height: 32px; color: #909399;">GB</span>
                <el-button type="danger" icon="Delete" size="small" circle @click="form.add_disks.splice(index, 1)" />
              </div>
              <el-button type="primary" size="small" plain icon="Plus" @click="addEditDisk">
                新建磁盘
              </el-button>
              <el-button type="success" size="small" plain icon="Link" @click="openAttachDiskDialog">
                挂载已有磁盘
              </el-button>
              <div v-if="editVmStatus === 'running'" class="form-tip">
                <el-icon><WarningFilled /></el-icon>
                运行中添加磁盘将使用热插拔，修改驱动类型需关机
              </div>
            </el-form-item>

            <el-divider content-position="left"><el-icon style="margin-right: 4px;"><Disc /></el-icon>CD/DVD 光驱</el-divider>
            <el-form-item label="光驱管理">
              <div style="width: 100%;">
                <div v-if="editCdroms.length > 0">
                  <div v-for="cdrom in editCdroms" :key="cdrom.device" style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px; padding: 6px 8px; background: #f5f7fa; border-radius: 4px;">
                    <el-tag type="info" size="small">{{ cdrom.device }}</el-tag>
                    <span style="flex: 1; font-size: 13px; color: #606266; word-break: break-all;">{{ cdrom.path || '（空光驱）' }}</span>
                    <el-button v-if="!cdrom.path" size="small" type="success" plain :disabled="!cdromIsoPath" @click="handleInsertCDROM(cdrom.device)">插入</el-button>
                    <el-button v-if="cdrom.path" size="small" plain @click="handleEjectCDROM(cdrom.device)">弹出</el-button>
                    <el-button size="small" type="danger" plain @click="handleRemoveCDROM(cdrom.device)">移除</el-button>
                  </div>
                </div>
                <div v-else style="color: #909399; font-size: 13px; margin-bottom: 8px;">无光驱设备</div>
                <div style="display: flex; gap: 8px; margin-top: 4px;">
                  <el-select v-model="cdromIsoPath" placeholder="从存储池选择 ISO" style="flex: 1;" filterable clearable @focus="loadISOs">
                    <el-option-group v-for="group in groupedISOs" :key="group.label" :label="group.label">
                      <el-option v-for="iso in group.items" :key="iso.path" :label="`${iso.name} (${iso.size})`" :value="iso.path" />
                    </el-option-group>
                  </el-select>
                  <el-button type="primary" size="default" :disabled="!cdromIsoPath" @click="handleAddNewCDROM">添加光驱</el-button>
                </div>
                <div v-if="editVmStatus === 'running'" class="form-tip" style="margin-top: 8px;">
                  <el-icon><InfoFilled /></el-icon>
                  运行中新增光驱会自动改用支持热插的 SCSI 总线；已有光驱插入 ISO 仍复用原设备
                </div>
              </div>
            </el-form-item>
          </div>
        </el-tab-pane>

        <el-tab-pane label="启动与安全" name="security">
          <div class="tab-content-wrapper">
            <el-divider content-position="left"><el-icon style="margin-right: 4px;"><Connection /></el-icon>网络配置</el-divider>
            <el-form-item label="网卡类型">
              <el-select v-model="form.nic_model" style="width: 240px;" :disabled="editVmStatus === 'running'">
                <el-option label="VirtIO（推荐）" value="virtio">
                  <div style="display: flex; justify-content: space-between;">
                    <span>VirtIO</span>
                    <el-tag size="small" type="success">高性能</el-tag>
                  </div>
                </el-option>
                <el-option label="e1000e (Intel)" value="e1000e">
                  <div style="display: flex; justify-content: space-between;">
                    <span>e1000e</span>
                    <el-tag size="small" type="info">Intel</el-tag>
                  </div>
                </el-option>
                <el-option label="rtl8139" value="rtl8139">
                  <div style="display: flex; justify-content: space-between;">
                    <span>rtl8139</span>
                    <el-tag size="small" type="warning">传统</el-tag>
                  </div>
                </el-option>
              </el-select>
              <div v-if="editVmStatus === 'running'" class="form-tip">
                <el-icon><WarningFilled /></el-icon>
                修改网卡类型需要先关机
              </div>
              <div v-else class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                VirtIO 性能最佳，部分系统需安装驱动
              </div>
            </el-form-item>

            <el-divider content-position="left"><el-icon style="margin-right: 4px;"><Cpu /></el-icon>引导方式</el-divider>
            <el-form-item label="引导方式">
              <el-radio-group v-model="form.boot_type" :disabled="editVmStatus === 'running' || editVmStatus === 'paused'" @change="onBootTypeChange">
                <el-radio value="bios" :disabled="form.arch === 'aarch64'"><span>BIOS</span></el-radio>
                <el-radio value="uefi"><span>UEFI</span></el-radio>
                <el-radio value="uefi-secure" :disabled="form.machine_type === 'i440fx' || form.arch === 'aarch64' || form.arch === 'riscv64'"><span>UEFI + 安全引导</span></el-radio>
              </el-radio-group>
              <div class="form-tip" style="margin-top: 8px;">
                <el-icon><WarningFilled /></el-icon>
                更改引导会导致原有已安装的操作系统无法启动
              </div>
              <div v-if="editVmStatus === 'running' || editVmStatus === 'paused'" class="form-tip" style="margin-top: 8px;">
                <el-icon><WarningFilled /></el-icon>
                修改引导方式需要先关机后再保存
              </div>
              <div v-else class="form-tip" style="margin-top: 8px;">
                <el-icon><InfoFilled /></el-icon>
                BIOS、UEFI 和安全引导切换都会改写固件配置，保存前请确认当前系统支持新的引导方式
              </div>
            </el-form-item>

            <el-divider content-position="left"><el-icon style="margin-right: 4px;"><Operation /></el-icon>引导顺序</el-divider>
            <el-form-item label="更改引导顺序" class="boot-devices-form-item">
              <div class="boot-devices-panel">
                <div v-if="editBootDevices.length > 0" class="boot-devices-list">
                  <div
                    v-for="(dev, index) in editBootDevices"
                    :key="dev.device || dev.file || index"
                    :class="['boot-device-row', { 'boot-device-disabled': !dev.enabled }]"
                  >
                    <el-checkbox v-model="dev.enabled" class="boot-device-checkbox" />
                    <div class="boot-device-info">
                      <span class="boot-device-type">
                        <el-icon style="margin-right: 4px;">
                          <component :is="bootDeviceIcon(dev.type)" />
                        </el-icon>
                        {{ bootDeviceTypeLabel(dev.type) }}
                      </span>
                      <div class="boot-device-details">
                        <span v-if="dev.type === 'network'" class="boot-device-label">MAC</span>
                        <span v-else class="boot-device-label">文件</span>
                        <span class="boot-device-file" :title="dev.file || '（空）'">{{ dev.file || '（空）' }}</span>
                      </div>
                      <div v-if="dev.type !== 'network' && (dev.device || dev.bus)" class="boot-device-meta">
                        <el-tag v-if="dev.type === 'cdrom'" size="small" type="info">设备: cdrom</el-tag>
                        <el-tag v-else size="small" type="info">设备: {{ dev.device }}</el-tag>
                        <el-tag v-if="dev.bus" size="small" type="info">总线: {{ dev.bus }}</el-tag>
                      </div>
                    </div>
                    <div class="boot-device-actions">
                      <el-button size="small" :icon="Top" :disabled="index === 0" @click="moveBootDeviceUp(index)" />
                      <el-button size="small" :icon="Bottom" :disabled="index === editBootDevices.length - 1" @click="moveBootDeviceDown(index)" />
                    </div>
                  </div>
                </div>
                <div v-else class="boot-devices-empty">
                  <el-icon><InfoFilled /></el-icon> 暂无可引导设备
                </div>
              </div>
              <div v-if="editVmStatus === 'running'" class="form-tip" style="margin-top: 8px;"><el-icon><WarningFilled /></el-icon>修改引导顺序需要关机后重启才能生效</div>
              <div v-else class="form-tip" style="margin-top: 8px;"><el-icon><InfoFilled /></el-icon>勾选并拖动设备调整引导优先级，取消勾选可从引导列表中移除</div>
            </el-form-item>
            <el-form-item label="开机自启">
              <el-switch v-model="form.autostart" active-text="启用" inactive-text="关闭" />
            </el-form-item>
          </div>
        </el-tab-pane>

        <el-tab-pane label="高级设置" name="advanced">
          <div class="tab-content-wrapper advanced-settings-page">
            <div class="advanced-settings-main" :class="{ 'content-blurred': shouldShowAdvancedIntro }">
              <el-alert type="warning" :closable="false" class="advanced-inline-alert">
                <template #title>
                  高级设置仅建议在调试或排查启动问题时使用，普通情况下请保持默认配置。
                </template>
              </el-alert>
              <el-divider content-position="left">开发者选项</el-divider>
              <el-form-item label="启动时冻结 CPU">
                <div class="advanced-field-row">
                  <el-switch v-model="form.freeze" active-text="启用" inactive-text="关闭" />
                  <el-tooltip :content="freezeHelpText" placement="top" effect="dark">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
              </el-form-item>
              <el-form-item label="APIC">
                <div class="advanced-field-row">
                  <el-switch v-model="form.apic" active-text="启用" inactive-text="关闭" />
                  <el-tooltip :content="apicHelpText" placement="top" effect="dark">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  常规虚拟机建议保持启用；仅在排查非常早期的启动兼容性问题时再尝试关闭
                </div>
              </el-form-item>
              <el-form-item label="PAE">
                <div class="advanced-field-row">
                  <el-switch v-model="form.pae" active-text="启用" inactive-text="关闭" />
                  <el-tooltip :content="paeHelpText" placement="top" effect="dark">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  主要用于 x86 老系统或 32 位来宾的大内存兼容场景；非 x86 架构会自动忽略该设置
                </div>
              </el-form-item>
              <el-form-item label="CPU 拓扑">
                <el-select v-model="form.cpu_topology_mode" style="width: 280px;" :disabled="editVmStatus === 'running' || editVmStatus === 'paused'">
                  <el-option v-for="item in cpuTopologyModeOptions" :key="item.value" :label="item.label" :value="item.value" />
                </el-select>
                <div v-if="editVmStatus === 'running' || editVmStatus === 'paused'" class="form-tip">
                  <el-icon><WarningFilled /></el-icon>
                  修改 CPU 拓扑需要先关机
                </div>
                <div v-else class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  Windows 10/11 建议使用自动或单插槽多核心，避免把多核识别成多颗物理 CPU
                </div>
              </el-form-item>
              <el-form-item v-if="isAdmin" label="CPU 亲和性">
                <el-select v-model="form.cpu_affinity" filterable allow-create default-first-option clearable placeholder="例如: 0,2,4 或 0-3" style="width: 320px;">
                  <el-option v-for="preset in cpuAffinityPresets" :key="preset.name" :label="preset.name" :value="preset.value" />
                </el-select>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  将虚拟机的 vCPU 绑定到指定的物理 CPU 核心上，多个核心用逗号或空格分隔，支持范围格式（如 0-3）。留空表示不限制亲和性
                </div>
              </el-form-item>
              <el-form-item label="PCIe 热插槽">
                <el-input-number v-model="form.pcie_root_ports" :min="0" :max="32" :step="1" style="width: 160px;"
                  :disabled="form.machine_type !== 'q35' || editVmStatus === 'running' || editVmStatus === 'paused'" />
                <div v-if="form.machine_type !== 'q35'" class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  仅 Q35 机型需要预留 PCIe 插槽，i440FX 无需关注此设置
                </div>
                <div v-else-if="editVmStatus === 'running' || editVmStatus === 'paused'" class="form-tip">
                  <el-icon><WarningFilled /></el-icon>
                  修改 PCIe 插槽数量需要先关机
                </div>
                <div v-else class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  预留的 pcie-root-port 数量（当前: {{ currentPCIERootPorts }}）。设为 0 表示恢复默认（新建时默认 4 个）
                </div>
              </el-form-item>
              <el-form-item label="显示设备">
                <el-select v-model="form.video_model" style="width: 280px;" :disabled="editVmStatus === 'running' || editVmStatus === 'paused'">
                  <el-option v-for="item in videoModelOptions" :key="item.value" :label="item.label" :value="item.value">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                      <span>{{ item.label }}</span>
                      <el-tag size="small" :type="item.tagType">{{ item.tag }}</el-tag>
                    </div>
                  </el-option>
                </el-select>
                <div v-if="editVmStatus === 'running' || editVmStatus === 'paused'" class="form-tip">
                  <el-icon><WarningFilled /></el-icon>
                  修改显示设备需要先关机
                </div>
                <div v-else class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  Windows 或 VMware 嵌套环境可优先尝试 VGA / VMVGA；Linux 默认推荐 VirtIO
                </div>
              </el-form-item>
              <el-form-item label="RTC">
                <div class="advanced-config-entry" @click="openRTCConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">配置</div>
                    <div class="advanced-config-entry-summary">{{ rtcConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item label="QEMU Guest Agent">
                <div class="advanced-config-entry" @click="openGuestAgentConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">配置</div>
                    <div class="advanced-config-entry-summary">{{ guestAgentConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item v-if="isAdmin" label="动态内存">
                <div class="advanced-config-entry" @click="openMemoryDynamicConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">配置</div>
                    <div class="advanced-config-entry-summary">{{ memoryDynamicSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item label="SMBIOS">
                <div class="advanced-config-entry" @click="openSMBIOSConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">类型 1</div>
                    <div class="advanced-config-entry-summary">{{ smbiosConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item v-if="isEdit && isAdmin" label="Domain XML">
                <div class="advanced-config-entry" @click="openVmXMLConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">编辑 XML</div>
                    <div class="advanced-config-entry-summary">{{ vmXMLConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
            </div>
            <div v-if="shouldShowAdvancedIntro" class="advanced-intro-mask">
              <el-card shadow="always" class="advanced-intro-card">
                <template #header>
                  <div class="advanced-intro-header">进阶设置提醒</div>
                </template>
                <div class="advanced-intro-content">
                  <p>此页面属于进阶设置，通常您并不需要调整此页面功能，若您不是开发者或不熟悉虚拟机的情况下请保持默认。</p>
                  <p class="advanced-intro-warning">仅在明确了解这些选项的用途及可能影响时再修改这里的选项。</p>
                  <div class="advanced-intro-actions">
                    <el-button type="primary" @click="dismissAdvancedIntro">我知道了</el-button>
                  </div>
                </div>
              </el-card>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="硬件直通" name="hardware">
          <div class="tab-content-wrapper">
            <el-alert type="info" :closable="false" show-icon class="advanced-inline-alert" style="margin-bottom: 16px;">
              <template #title>
                硬件直通可将宿主机的 PCI 设备（如 GPU、网卡、NVMe 硬盘等）直接分配给虚拟机使用，获得接近原生的性能。
              </template>
            </el-alert>
            <el-form-item label="直通设备">
              <div style="width: 100%;">
                <div v-if="form.host_devices.length > 0">
                  <el-table :data="form.host_devices" size="small" border style="width: 100%; margin-bottom: 8px;">
                    <el-table-column label="PCI 地址" prop="pci_address" width="160" />
                    <el-table-column label="设备名称" min-width="180">
                      <template #default="{ row }">
                        <span>{{ getPassthroughDeviceName(row.pci_address) }}</span>
                      </template>
                    </el-table-column>
                    <el-table-column label="状态" width="100">
                      <template #default="{ row }">
                        <el-tag v-if="isDeviceVfioBound(row.pci_address)" type="success" size="small">已绑定</el-tag>
                        <el-tag v-else type="warning" size="small">待绑定</el-tag>
                      </template>
                    </el-table-column>
                    <el-table-column label="操作" width="120" align="center">
                      <template #default="{ row, $index }">
                        <el-button size="small" type="danger" plain @click="removeHostDevice($index)">移除</el-button>
                      </template>
                    </el-table-column>
                  </el-table>
                </div>
                <div v-else style="color: #909399; font-size: 13px; margin-bottom: 8px;">未配置直通设备</div>
                <el-button type="primary" size="small" plain icon="Plus" @click="openPassthroughDialog">
                  添加直通设备
                </el-button>
                <div v-if="editVmStatus === 'running' || editVmStatus === 'paused'" class="form-tip" style="margin-top: 8px;">
                  <el-icon><WarningFilled /></el-icon>
                  修改硬件直通设备需要先关机
                </div>
              </div>
            </el-form-item>
          </div>
        </el-tab-pane>
      </el-tabs>
        </div><!-- /edit-tab-content -->
      </template>

      <!-- ==================== 创建模式：步骤引导 ==================== -->
      <template v-else>
        <div class="step-indicator-bar">
          <div v-for="(step, index) in createSteps" :key="index"
            :class="['step-dot-item', { active: createStep === index, done: createStep > index }]"
            @click="goToCreateStep(index)">
            <div class="step-dot-badge">
              <span v-if="createStep > index">✓</span>
              <span v-else>{{ index + 1 }}</span>
            </div>
            <div class="step-dot-label">{{ step.title }}</div>
          </div>
        </div>

        <!-- Step 0: Mode Selection -->
        <div v-show="createSteps[createStep]?.name === 'mode'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon basic"><FormIcons icon="step-clipboard" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">选择创建方式</div>
              <div class="step-pane-desc">根据需求选择最适合的虚拟机创建途径</div>
            </div>
          </div>
          <div class="mode-cards-new">
            <div class="mode-card-new" :class="{ selected: form.create_mode === 'iso' }" @click="selectMode('iso')">
              <div class="mode-card-check"><FormIcons icon="check" :size="16" /></div>
              <div class="mode-card-icon"><FormIcons icon="mode-disc" :size="40" /></div>
              <div class="mode-card-title">使用 ISO 镜像安装</div>
              <div class="mode-card-desc">使用标准系统镜像安装全新操作环境，适合全新部署</div>
            </div>
            <div class="mode-card-new" :class="{ selected: form.create_mode === 'template' }" @click="selectMode('template')">
              <div class="mode-card-check"><FormIcons icon="check" :size="16" /></div>
              <div class="mode-card-icon"><FormIcons icon="mode-copy" :size="40" /></div>
              <div class="mode-card-title">从模板快速克隆</div>
              <div class="mode-card-desc">基于预配置模板秒级创建，适合批量部署</div>
            </div>
            <div v-if="isAdmin" class="mode-card-new" :class="{ selected: form.create_mode === 'linked_clone' }" @click="selectMode('linked_clone')">
              <div class="mode-card-check"><FormIcons icon="check" :size="16" /></div>
              <div class="mode-card-icon"><FormIcons icon="mode-link" :size="40" /></div>
              <div class="mode-card-title">原生链式克隆</div>
              <div class="mode-card-desc">直接基于模板生成 backing 磁盘并启动，不修改来宾系统</div>
            </div>
            <div class="mode-card-new" :class="{ selected: form.create_mode === 'import' }" @click="selectMode('import')">
              <div class="mode-card-check"><FormIcons icon="check" :size="16" /></div>
              <div class="mode-card-icon"><FormIcons icon="mode-import" :size="40" /></div>
              <div class="mode-card-title">导入已有磁盘</div>
              <div class="mode-card-desc">使用已有的虚拟机磁盘文件快速运行</div>
            </div>
          </div>
        </div>

        <div v-show="createSteps[createStep]?.name === 'basic'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon basic"><FormIcons icon="step-info" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">基础信息</div>
              <div class="step-pane-desc">设置虚拟机名称、用途和操作系统类型</div>
            </div>
          </div>
          <div class="step-pane-body">
            <el-form-item label="虚拟机名称" prop="name">
              <el-input v-model="form.name" placeholder="默认自动生成，也可手动修改">
                <template #append><el-button @click="handleGenerateVmName">随机生成</el-button></template>
              </el-input>
              <div class="form-tip" v-if="!isTemplateSourceMode || isLinkedCloneMode || form.batch_count <= 1"><el-icon><InfoFilled /></el-icon>名称仅支持字母、数字和短横线，且不能以短横线开头或结尾，例如 "web01" 或 "vm-2026"</div>
              <div class="form-tip" v-else><el-icon><InfoFilled /></el-icon>批量模式下名称将作为前缀，最终命名为 {{ form.name || 'vm' }}-01, {{ form.name || 'vm' }}-02...</div>
            </el-form-item>
            <el-form-item v-if="isTemplateSourceMode && !isLinkedCloneMode" label="创建数量">
              <el-input-number v-model="form.batch_count" :min="1" :max="100" style="width: 100%;" />
              <div class="form-tip" v-if="form.batch_count > 1">
                <el-icon><InfoFilled /></el-icon>
                将创建 {{ form.batch_count }} 台虚拟机，共 {{ form.batch_count * form.ram }}GB 内存 / {{ form.batch_count * form.disk_size }}GB 磁盘。留空密码将为每台虚拟机自动生成独立随机强密码。
              </div>
            </el-form-item>
            <el-form-item label="备注">
              <el-input v-model="form.remark" type="textarea" :rows="3" maxlength="200" show-word-limit placeholder="用于记录用途、环境或业务信息" />
            </el-form-item>

            <el-form-item label="应用场景">
              <el-select v-model="useCase" placeholder="-- 请选择（可选）--" style="width: 100%;" clearable @change="onUseCaseChange">
                <el-option label="Web 应用 / API 服务" value="web" />
                <el-option label="数据库服务器" value="database" />
                <el-option label="开发测试环境" value="dev" />
                <el-option label="AI / 机器学习" value="ai" />
                <el-option label="文件存储 / NAS" value="storage" />
              </el-select>
              <div class="form-tip"><el-icon><InfoFilled /></el-icon>选择场景后系统将在下一步推荐最优硬件配置</div>
            </el-form-item>

            <template v-if="!isTemplateSourceMode">
              <el-form-item label="系统类型">
                <div style="display: flex; gap: 12px;">
                  <div v-for="os in osQuickOptions" :key="os.value"
                    :class="['os-quick-card', { selected: form.os_type === os.value }]"
                    @click="onOsQuickSelect(os.value)">
                    <div class="os-qc-icon">{{ os.icon }}</div>
                    <div class="os-qc-name">{{ os.label }}</div>
                    <div class="os-qc-examples">{{ os.examples }}</div>
                  </div>
                </div>
              </el-form-item>

              <el-form-item v-if="form.create_mode === 'iso'" label="系统版本">
                <el-select v-model="form.os_variant" placeholder="选择系统版本（可搜索）" style="width: 100%;" filterable clearable @focus="loadOSVariants">
                  <el-option-group v-for="group in filteredOSVariants" :key="group.label" :label="group.label">
                    <el-option v-for="v in group.items" :key="v.id" :label="v.name" :value="v.id" />
                  </el-option-group>
                </el-select>
              </el-form-item>
            </template>

            <!-- 模板克隆：模板选择 + 磁盘 + 登录凭据 -->
            <template v-if="isTemplateSourceMode">
              <div class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Monitor /></el-icon>
                  <span>模板选择</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="选择模板" prop="template">
                    <el-select v-model="form.template" filterable placeholder="选择模板" style="width: 100%;" @change="onTemplateChange" @focus="() => loadTemplates(true)">
                      <el-option-group v-for="group in groupedTemplates" :key="group.label" :label="group.label">
                        <el-option v-for="t in group.items" :key="t.name" :label="templateOptionLabel(t)" :value="t.name" />
                      </el-option-group>
                    </el-select>
                  </el-form-item>
                  <el-row :gutter="20">
                    <el-col :span="12">
                      <el-form-item label="磁盘大小(GB)" prop="disk_size">
                        <el-input-number v-model="form.disk_size" :min="templateMinDiskSize || 1" :max="2000" :step="10" style="width: 100%;" />
                        <div style="font-size: 12px; color: #909399; margin-top: 4px;">{{ templateDiskSizeTip }}</div>
                      </el-form-item>
                    </el-col>
                    <el-col :span="12">
                      <el-form-item label="系统盘驱动" prop="disk_bus">
                        <el-select v-model="form.disk_bus" style="width: 100%;">
                          <el-option label="VirtIO" value="virtio" />
                          <el-option label="SCSI" value="scsi" />
                          <el-option label="SATA" value="sata" />
                          <el-option label="IDE" value="ide" />
                        </el-select>
                        <div class="form-tip"><el-icon><InfoFilled /></el-icon>优先按模板记录自动带出；旧模板会回退到当前默认值</div>
                      </el-form-item>
                    </el-col>
                  </el-row>

                  <!-- 系统盘 IOPS 限制（仅管理员） -->
                  <el-row v-if="isAdmin" :gutter="20" style="margin-top: 8px;">
                    <el-col :span="24">
                      <el-form-item label="系统盘 IOPS">
                        <div style="display: flex; gap: 8px; align-items: center; width: 100%; flex-wrap: wrap;">
                          <span style="font-size: 12px; color: #909399; white-space: nowrap;">总</span>
                          <el-input-number v-model="form.system_disk_iops_total" :min="0" :step="100" size="small" style="width: 100px;" placeholder="总IOPS" />
                          <span style="font-size: 12px; color: #909399; white-space: nowrap;">读</span>
                          <el-input-number v-model="form.system_disk_iops_read" :min="0" :step="100" size="small" style="width: 100px;" placeholder="读IOPS" :disabled="form.system_disk_iops_total > 0" />
                          <span style="font-size: 12px; color: #909399; white-space: nowrap;">写</span>
                          <el-input-number v-model="form.system_disk_iops_write" :min="0" :step="100" size="small" style="width: 100px;" placeholder="写IOPS" :disabled="form.system_disk_iops_total > 0" />
                          <span style="font-size: 11px; color: var(--el-color-warning);">互斥</span>
                        </div>
                      </el-form-item>
                    </el-col>
                  </el-row>
                </div>
              </div>
              <div class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Operation /></el-icon>
                  <span>克隆模式</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="克隆模式">
                    <el-radio-group v-model="form.clone_mode">
                      <el-radio value="linked">链式克隆</el-radio>
                      <el-radio value="full">完整克隆</el-radio>
                    </el-radio-group>
                    <div class="form-tip" v-if="form.clone_mode === 'linked'">
                      <el-icon><InfoFilled /></el-icon>
                      基于模板创建 backing_file 链式磁盘，依赖模板存在。磁盘创建速度快，节省存储空间。
                    </div>
                    <div class="form-tip" v-else>
                      <el-icon><InfoFilled /></el-icon>
                      将模板数据完整复制到独立磁盘，不依赖模板，脱离链式条件。磁盘创建较慢，占用完整磁盘空间。
                    </div>
                  </el-form-item>
                </div>
              </div>
              <el-alert
                v-if="registrationMode"
                type="info"
                :closable="false"
                show-icon
                class="registration-inline-tip"
                title="当前仅登记服务器配置。登录用户名和密码会由用户登录后确认开通时自行填写，邮件不会包含密码。"
              />
              <div v-if="registrationMode" class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Connection /></el-icon>
                  <span>网络与配额摘要</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="主机名" prop="hostname">
                    <el-input v-model="form.hostname" placeholder="自动随机生成">
                      <template #append><el-button @click="handleGenerateTemplateHostname">随机生成</el-button></template>
                    </el-input>
                  </el-form-item>
                  <div class="registration-summary-grid">
                    <div><span>专用 VPC</span><strong>{{ registrationContext.dedicated_vpc_label || '管理员已配置' }}</strong></div>
                    <div><span>存储位置</span><strong>{{ selectedStorageTargetLabel }}</strong></div>
                    <div><span>网卡类型</span><strong>{{ form.nic_model || 'virtio' }}</strong></div>
                    <div><span>系统盘驱动</span><strong>{{ form.disk_bus || 'virtio' }}</strong></div>
                    <div><span>月流量</span><strong>下行 {{ form.traffic_down_gb || 0 }}GB / 上行 {{ form.traffic_up_gb || 0 }}GB</strong></div>
                    <div><span>带宽</span><strong>下行 {{ form.bandwidth_down_mbps || 0 }}Mbps / 上行 {{ form.bandwidth_up_mbps || 0 }}Mbps</strong></div>
                    <div><span>端口转发上限</span><strong>{{ form.max_port_forwards ?? 10 }}</strong></div>
                    <div><span>运行时长配额</span><strong>{{ form.max_runtime_hours ? `${form.max_runtime_hours}小时` : '不限' }}</strong></div>
                  </div>
                </div>
              </div>
              <div v-else-if="isLinkedCloneMode" class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><InfoFilled /></el-icon>
                  <span>原生链式克隆说明</span>
                </div>
                <div class="form-section-card-body">
                  <el-alert type="warning" :closable="false" show-icon>
                    <template #title>
                      该模式不会修改模板内的主机名、用户名、密码、machine-id 或网络配置，只会创建 backing qcow2、定义并启动虚拟机。
                    </template>
                  </el-alert>
                </div>
              </div>
              <div v-else class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><User /></el-icon>
                  <span>登录凭据</span>
                </div>
                <div class="form-section-card-body">
                  <el-row :gutter="20">
                    <el-col :span="12">
                      <el-form-item label="主机名" prop="hostname">
                        <el-input v-model="form.hostname" placeholder="自动随机生成">
                          <template #append><el-button @click="handleGenerateTemplateHostname">随机生成</el-button></template>
                        </el-input>
                      </el-form-item>
                    </el-col>
                    <el-col :span="12">
                      <el-form-item prop="import_user">
                        <template #label>
                          <span>用户名</span>
                          <el-tooltip :content="templateUserTip" placement="top" effect="dark">
                            <el-icon class="label-tip-icon"><QuestionFilled /></el-icon>
                          </el-tooltip>
                        </template>
                        <el-input v-model="form.import_user" :placeholder="templateUserPlaceholder" :disabled="isWindowsTemplate" />
                      </el-form-item>
                    </el-col>
                  </el-row>
                  <el-form-item prop="import_password">
                    <template #label>
                      <span>密码</span>
                      <el-tooltip :content="templatePasswordTip" placement="top" effect="dark">
                        <el-icon class="label-tip-icon"><QuestionFilled /></el-icon>
                      </el-tooltip>
                    </template>
                    <el-input v-model="form.import_password" placeholder="请输入强密码" type="password" show-password autocomplete="new-password" style="width: 100%;">
                      <template #append><el-button @click="handleGenerateTemplatePassword">生成强密码</el-button></template>
                    </el-input>
                  </el-form-item>
                </div>
              </div>
              <div v-if="isFnOSTemplate && !isLinkedCloneMode" class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><InfoFilled /></el-icon>
                  <span>FnOS 标识</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="设备 ID" prop="fnos_device_id">
                    <el-radio-group v-model="form.fnos_device_id_mode">
                      <el-radio value="regenerate">重新生成</el-radio>
                      <el-radio value="preserve">保留设备 ID</el-radio>
                      <el-radio value="custom">指定设备 ID</el-radio>
                    </el-radio-group>
                    <el-input
                      v-if="form.fnos_device_id_mode === 'custom'"
                      v-model="form.fnos_device_id"
                      placeholder="请输入 32 位或 40 位十六进制设备 ID"
                      style="margin-top: 8px; max-width: 420px;"
                    />
                    <div class="form-tip">
                      <el-icon><InfoFilled /></el-icon>
                      适合需要特殊使用设备ID的授权场景；
                    </div>
                  </el-form-item>
                </div>
              </div>
            </template>
          </div>
        </div>

        <div v-show="createSteps[createStep]?.name === 'hardware'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon hardware"><FormIcons icon="step-gear" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">硬件规格</div>
              <div class="step-pane-desc">配置 CPU、内存和虚拟化引擎参数</div>
            </div>
          </div>
          <div class="step-pane-body">
            <!-- 智能推荐横幅 -->
            <div v-if="showSmartRecommend" class="smart-recommend-banner">
              <div class="rec-icon-col"><FormIcons icon="tip-lightbulb" :size="24" /></div>
              <div class="rec-content-col">
                <div class="rec-title">智能推荐方案</div>
                <div class="rec-desc">{{ smartRecommendDesc }}</div>
                <div class="rec-actions">
                  <el-button type="primary" size="small" @click="applySmartRecommend">一键应用推荐</el-button>
                  <el-button size="small" @click="showSmartRecommend = false">忽略</el-button>
                </div>
              </div>
            </div>

            <div class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Cpu /></el-icon>
                <span>CPU 与内存</span>
              </div>
              <div class="form-section-card-body">
                <el-row :gutter="20">
                  <el-col :span="12">
                    <el-form-item label="CPU 核心" prop="vcpu">
                      <el-input-number v-model="form.vcpu" :min="vcpuMin" :max="32" style="width: 100%;" :disabled="isEdit && editVmStatus === 'running' && !form.cpu_hotplug_enabled" />
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item label="内存(GB)" prop="ram">
                      <el-input-number v-model="form.ram" :min="1" :max="64" style="width: 100%;" @change="handleBaseMemoryChange" />
                    </el-form-item>
                  </el-col>
                </el-row>
                <el-form-item label="CPU 热添加">
                  <div class="memory-basic-switch">
                    <div class="memory-basic-switch-row">
                      <el-switch v-model="form.cpu_hotplug_enabled" active-text="启用" inactive-text="关闭" @change="handleCPUHotplugChange" />
                      <template v-if="form.cpu_hotplug_enabled && hostCPUCores > 0">
                        <span class="memory-dynamic-unit" style="margin-left: 8px;">上限 {{ hostCPUCores }} 核</span>
                      </template>
                    </div>
                    <div class="form-tip">
                      <el-icon><InfoFilled /></el-icon>
                      启用后可在宿主机 {{ hostCPUCores > 0 ? hostCPUCores : '?' }} 核范围内随时热添加 vCPU，无需重启。新建时需重启一次后热添加功能生效
                    </div>
                  </div>
                </el-form-item>
                <el-form-item v-if="isAdmin" label="CPU 限制">
                  <div class="memory-basic-switch">
                    <div class="memory-basic-switch-row">
                      <el-switch v-model="form.cpu_limit_enabled" active-text="启用限制" inactive-text="无限制" />
                      <template v-if="form.cpu_limit_enabled">
                        <el-input-number v-model="form.cpu_limit_percent" :min="1" :max="100" :step="1" style="width: 160px;" />
                        <span class="memory-dynamic-unit">%</span>
                      </template>
                    </div>
                    <div class="form-tip">
                      <el-icon><InfoFilled /></el-icon>
                      按当前配置的 vCPU 总能力限速；50% 表示限制为当前已分配 CPU 总能力的一半
                    </div>
                  </div>
                </el-form-item>
                <el-form-item label="动态内存">
                  <div class="memory-basic-switch">
                    <div class="memory-basic-switch-row">
                      <el-switch v-model="form.memory_dynamic_enabled" active-text="启用" inactive-text="关闭" @change="handleDynamicMemoryEnabledChange" />
                      <el-radio-group v-if="showMemoryBackendQuickSelect" v-model="form.memory_backend" size="small" @change="handleMemoryBackendChange">
                        <el-radio-button label="balloon">气球调度</el-radio-button>
                        <el-radio-button label="virtio_mem" :disabled="windowsElasticMemoryDisabled">Windows 弹性内存</el-radio-button>
                      </el-radio-group>
                      <el-tag v-if="showMemoryBackendQuickSelect && form.memory_backend === 'virtio_mem'" type="warning" effect="plain">弹性内存</el-tag>
                      <el-button v-if="showMemoryBackendQuickSelect && form.memory_backend === 'virtio_mem'" size="small" text type="primary" @click="memoryVirtioMemDetailVisible = true">详情</el-button>
                    </div>
                    <div class="form-tip">
                      <el-icon><InfoFilled /></el-icon>
                      {{ basicMemoryDynamicTip }}
                    </div>
                  </div>
                </el-form-item>
              </div>
            </div>

            <div class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Setting /></el-icon>
                <span>虚拟化引擎</span>
              </div>
              <div class="form-section-card-body">
                <el-form-item label="虚拟化方案">
                  <el-radio-group v-model="form.virt_type" @change="onVirtTypeChange">
                    <el-radio value="kvm">
                      <span style="display: flex; align-items: center; gap: 4px;"><FormIcons icon="virt-kvm" :size="16" /> 硬件虚拟化 (KVM)</span>
                    </el-radio>
                    <el-radio value="qemu">
                      <span style="display: flex; align-items: center; gap: 4px;"><FormIcons icon="virt-qemu" :size="16" /> 软件虚拟化 (QEMU)</span>
                    </el-radio>
                  </el-radio-group>
                  <div class="form-tip">
                    <el-icon><InfoFilled /></el-icon>
                    {{ form.virt_type === 'kvm' ? 'KVM 利用硬件虚拟化加速，性能最佳（需 CPU 支持 VT-x/AMD-V）' : 'QEMU 纯软件模拟，性能较低但可模拟不同平台架构' }}
                  </div>
                </el-form-item>

                <el-form-item v-if="form.virt_type === 'qemu'" label="平台架构">
                  <el-select v-model="form.arch" style="width: 260px;" @change="onArchChange">
                    <el-option label="x86_64（默认）" value="x86_64">
                      <div style="display: flex; justify-content: space-between;">
                        <span>x86_64</span>
                        <el-tag size="small" type="info">默认</el-tag>
                      </div>
                    </el-option>
                    <el-option label="aarch64 (ARM)" value="aarch64">
                      <div style="display: flex; justify-content: space-between;">
                        <span>aarch64</span>
                        <el-tag size="small" type="success">ARM</el-tag>
                      </div>
                    </el-option>
                    <el-option label="riscv64 (RISC-V)" value="riscv64">
                      <div style="display: flex; justify-content: space-between;">
                        <span>riscv64</span>
                        <el-tag size="small" type="warning">RISC-V</el-tag>
                      </div>
                    </el-option>
                  </el-select>
                  <div class="form-tip">
                    <el-icon><InfoFilled /></el-icon>
                    软件虚拟化可模拟不同 CPU 架构，适合交叉编译和测试
                  </div>
                </el-form-item>

                <el-form-item label="虚拟机类型" prop="machine_type">
                  <el-radio-group v-model="form.machine_type">
                    <el-radio value="q35" :disabled="form.arch === 'aarch64' || form.arch === 'riscv64'"><span>Q35</span></el-radio>
                    <el-radio value="i440fx" :disabled="form.arch === 'aarch64' || form.arch === 'riscv64'"><span>i440FX</span></el-radio>
                    <el-radio v-if="form.arch === 'aarch64' || form.arch === 'riscv64'" value="virt"><span>virt</span></el-radio>
                  </el-radio-group>
                </el-form-item>

                <el-form-item label="引导类型" prop="boot_type">
                  <template v-if="form.create_mode === 'import'">
                    <el-tag type="info">自动识别</el-tag>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>导入时将自动检测磁盘的引导类型（BIOS/UEFI），无需手动选择</div>
                  </template>
                  <template v-else>
                    <el-radio-group v-model="form.boot_type" @change="onBootTypeChange">
                      <el-radio value="bios" :disabled="form.arch === 'aarch64'"><span>BIOS</span></el-radio>
                      <el-radio value="uefi"><span>UEFI</span></el-radio>
                      <el-radio value="uefi-secure" :disabled="form.machine_type === 'i440fx' || form.arch === 'aarch64' || form.arch === 'riscv64'"><span>UEFI + 安全引导</span></el-radio>
                    </el-radio-group>
                  </template>
                </el-form-item>
              </div>
            </div>

          </div>
        </div>

        <div v-show="createSteps[createStep]?.name === 'storage'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon storage"><FormIcons icon="step-storage" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">存储介质</div>
              <div class="step-pane-desc" v-if="form.create_mode === 'iso'">选择 ISO 镜像并配置系统磁盘</div>
              <div class="step-pane-desc" v-else-if="isTemplateSourceMode">{{ isLinkedCloneMode ? '选择模板并直接执行原生链式克隆' : '选择模板并配置克隆参数' }}</div>
              <div class="step-pane-desc" v-else>选择要导入的磁盘文件并配置初始化选项</div>
            </div>
          </div>
          <div class="step-pane-body">
            <div v-if="form.create_mode !== 'import'" class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Coin /></el-icon>
                <span>存储位置</span>
              </div>
              <div class="form-section-card-body">
                <el-form-item label="虚拟机硬盘">
                  <el-select v-model="form.storage_pool_id" placeholder="使用默认存储位置" clearable filterable style="width: 100%;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
                    <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id">
                      <div style="display: flex; justify-content: space-between; align-items: center; gap: 8px;">
                        <span>{{ target.display_name }}</span>
                        <div style="display: flex; gap: 4px;">
                          <el-tag v-if="target.is_default" size="small" type="success">默认</el-tag>
                          <el-tag size="small" type="info">{{ formatBytes(target.available) }} 可用</el-tag>
                        </div>
                      </div>
                    </el-option>
                  </el-select>
                  <div class="form-tip"><el-icon><InfoFilled /></el-icon>留空时使用管理员设置的默认存储位置，没有默认时回退系统克隆目录</div>
                </el-form-item>
              </div>
            </div>

            <!-- ISO 模式：ISO 选择 + 磁盘配置 -->
            <template v-if="form.create_mode === 'iso'">
              <div class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Disc /></el-icon>
                  <span>安装镜像</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="ISO 镜像">
                    <el-select v-model="form.iso_paths" placeholder="选择一个或多个 ISO 镜像" style="width: 100%;" filterable clearable multiple collapse-tags collapse-tags-tooltip @focus="loadISOs" @change="onISOChange">
                      <el-option-group v-for="group in groupedISOs" :key="group.label" :label="group.label">
                        <el-option v-for="iso in group.items" :key="iso.path" :label="`${iso.name} (${iso.size})`" :value="iso.path">
                          <div style="display: flex; justify-content: space-between; align-items: center;">
                            <span>{{ iso.name }}</span>
                            <div style="display: flex; gap: 4px;">
                              <el-tag size="small" :type="iso.os_type === 'windows' ? 'warning' : 'success'">{{ iso.os_type === 'windows' ? 'Win' : 'Linux' }}</el-tag>
                              <el-tag size="small" type="info">{{ iso.size }}</el-tag>
                            </div>
                          </div>
                        </el-option>
                      </el-option-group>
                    </el-select>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>支持同时挂载多个 ISO，首个 ISO 会作为主安装盘并自动补全系统类型和版本，其余 ISO 会作为额外挂载光驱</div>
                    <div v-if="form.iso_paths.length > 0" class="form-tip" style="flex-wrap: wrap; gap: 6px;">
                      <el-icon><Collection /></el-icon>
                      <span>当前挂载顺序：</span>
                      <el-tag v-for="(path, index) in form.iso_paths" :key="path" size="small" :type="index === 0 ? 'success' : 'info'">
                        {{ index + 1 }}. {{ path.split('/').pop() }}
                      </el-tag>
                    </div>
                  </el-form-item>
                </div>
              </div>

              <div class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Coin /></el-icon>
                  <span>系统磁盘</span>
                </div>
                <div class="form-section-card-body">
                  <el-row :gutter="20">
                    <el-col :span="12">
                      <el-form-item label="系统盘(GB)" prop="disk_size">
                        <el-input-number v-model="form.disk_size" :min="10" :max="2000" :step="10" style="width: 100%;" />
                      </el-form-item>
                    </el-col>
                    <el-col :span="12">
                      <el-form-item label="磁盘格式" prop="disk_format">
                        <el-select v-model="form.disk_format" style="width: 100%;">
                          <el-option label="QCOW2（推荐）" value="qcow2" />
                          <el-option label="RAW" value="raw" />
                        </el-select>
                      </el-form-item>
                    </el-col>
                  </el-row>
                  <el-row :gutter="20">
                    <el-col :span="12">
                      <el-form-item label="驱动类型" prop="disk_bus">
                        <el-select v-model="form.disk_bus" style="width: 100%;">
                          <el-option label="VirtIO" value="virtio" />
                          <el-option label="SCSI" value="scsi" />
                          <el-option label="SATA" value="sata" />
                          <el-option label="IDE" value="ide" />
                        </el-select>
                      </el-form-item>
                    </el-col>
                  </el-row>

                  <!-- 系统盘 IOPS 限制（仅管理员） -->
                  <el-row v-if="isAdmin" :gutter="20" style="margin-top: 8px;">
                    <el-col :span="24">
                      <el-form-item label="系统盘 IOPS">
                        <div style="display: flex; gap: 8px; align-items: center; width: 100%; flex-wrap: wrap;">
                          <span style="font-size: 12px; color: #909399; white-space: nowrap;">总</span>
                          <el-input-number v-model="form.system_disk_iops_total" :min="0" :step="100" size="small" style="width: 100px;" placeholder="总IOPS" />
                          <span style="font-size: 12px; color: #909399; white-space: nowrap;">读</span>
                          <el-input-number v-model="form.system_disk_iops_read" :min="0" :step="100" size="small" style="width: 100px;" placeholder="读IOPS" :disabled="form.system_disk_iops_total > 0" />
                          <span style="font-size: 12px; color: #909399; white-space: nowrap;">写</span>
                          <el-input-number v-model="form.system_disk_iops_write" :min="0" :step="100" size="small" style="width: 100px;" placeholder="写IOPS" :disabled="form.system_disk_iops_total > 0" />
                          <span style="font-size: 11px; color: var(--el-color-warning);">互斥</span>
                        </div>
                      </el-form-item>
                    </el-col>
                  </el-row>

                  <el-form-item label="额外磁盘">
                    <div v-for="(disk, index) in form.extra_disks" :key="index" style="display: flex; gap: 8px; margin-bottom: 8px; width: 100%; flex-wrap: wrap; align-items: center;">
                      <el-input-number v-model="disk.size" :min="1" :max="2000" placeholder="大小(GB)" style="width: 120px;" />
                      <el-select v-model="disk.format" style="width: 100px;">
                        <el-option label="qcow2" value="qcow2" />
                        <el-option label="raw" value="raw" />
                      </el-select>
                      <el-select v-model="disk.bus" placeholder="驱动" style="width: 110px;">
                        <el-option label="VirtIO" value="virtio" />
                        <el-option label="SCSI" value="scsi" />
                        <el-option label="SATA" value="sata" />
                        <el-option label="IDE" value="ide" />
                      </el-select>
                      <el-select v-model="disk.storage_pool_id" placeholder="默认存储" clearable filterable style="width: 180px;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
                        <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id" />
                      </el-select>
                      <span style="line-height: 32px; color: #909399;">GB</span>
                      <el-button v-if="isAdmin" size="small" @click="openCreateExtraDiskIOPSDialog(index)" style="font-size: 11px;">IOPS</el-button>
                      <el-button type="danger" icon="Delete" size="small" circle @click="form.extra_disks.splice(index, 1)" />
                      <span v-if="isAdmin && (disk.iops_total > 0 || disk.iops_read > 0 || disk.iops_write > 0)" style="font-size: 11px; color: var(--el-color-warning); width: 100%;">
                        IOPS: 总{{ disk.iops_total || 0 }} / 读{{ disk.iops_read || 0 }} / 写{{ disk.iops_write || 0 }}
                      </span>
                    </div>
                    <el-button type="primary" size="small" plain icon="Plus" @click="addCreateExtraDisk">
                      添加额外磁盘
                    </el-button>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>额外磁盘默认跟随系统盘驱动类型；存储位置留空时使用上方虚拟机硬盘默认位置</div>
                  </el-form-item>
                </div>
              </div>
            </template>

            <!-- 模板克隆提示 -->
            <template v-if="isTemplateSourceMode">
              <el-alert type="info" :closable="false" show-icon>
                <template #title>
                  {{ isLinkedCloneMode ? '原生链式克隆的模板与系统盘参数已在「基础信息」步骤中配置完成，可在下方追加数据盘。' : '模板克隆的系统盘和登录凭据已在「基础信息」步骤中配置完成，可在下方追加数据盘。' }}
                </template>
              </el-alert>
              <div class="form-section-card" style="margin-top: 14px;">
                <div class="form-section-card-header">
                  <el-icon><Coin /></el-icon>
                  <span>额外数据盘</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="额外磁盘">
                    <div v-for="(disk, index) in form.extra_disks" :key="index" style="display: flex; gap: 8px; margin-bottom: 8px; width: 100%; flex-wrap: wrap; align-items: center;">
                      <el-input-number v-model="disk.size" :min="1" :max="2000" placeholder="大小(GB)" style="width: 120px;" />
                      <el-select v-model="disk.format" style="width: 100px;">
                        <el-option label="qcow2" value="qcow2" />
                        <el-option v-if="isAdmin" label="raw" value="raw" />
                      </el-select>
                      <el-select v-model="disk.bus" placeholder="驱动" style="width: 110px;">
                        <el-option label="VirtIO" value="virtio" />
                        <el-option label="SCSI" value="scsi" />
                        <el-option label="SATA" value="sata" />
                        <el-option label="IDE" value="ide" />
                      </el-select>
                      <el-select v-model="disk.storage_pool_id" placeholder="默认存储" clearable filterable style="width: 180px;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
                        <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id" />
                      </el-select>
                      <span style="line-height: 32px; color: #909399;">GB</span>
                      <el-button v-if="isAdmin" size="small" @click="openCreateExtraDiskIOPSDialog(index)" style="font-size: 11px;">IOPS</el-button>
                      <el-button type="danger" icon="Delete" size="small" circle @click="form.extra_disks.splice(index, 1)" />
                      <span v-if="isAdmin && (disk.iops_total > 0 || disk.iops_read > 0 || disk.iops_write > 0)" style="font-size: 11px; color: var(--el-color-warning); width: 100%;">
                        IOPS: 总{{ disk.iops_total || 0 }} / 读{{ disk.iops_read || 0 }} / 写{{ disk.iops_write || 0 }}
                      </span>
                    </div>
                    <el-button type="primary" size="small" plain icon="Plus" @click="addCreateExtraDisk">
                      添加额外磁盘
                    </el-button>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>额外磁盘会在模板克隆完成后自动挂载，并计入普通用户硬盘配额</div>
                  </el-form-item>
                </div>
              </div>
            </template>

            <!-- 导入磁盘模式 -->
            <template v-if="form.create_mode === 'import'">
              <!-- 存储位置选择（与其他创建方式对齐） -->
              <div class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Coin /></el-icon>
                  <span>存储位置</span>
                </div>
                <div class="form-section-card-body">
                  <el-form-item label="虚拟机硬盘">
                    <el-select v-model="form.storage_pool_id" placeholder="使用默认存储位置" clearable filterable style="width: 100%;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
                      <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id">
                        <div style="display: flex; justify-content: space-between; align-items: center; gap: 8px;">
                          <span>{{ target.display_name }}</span>
                          <div style="display: flex; gap: 4px;">
                            <el-tag v-if="target.is_default" size="small" type="success">默认</el-tag>
                            <el-tag size="small" type="info">{{ formatBytes(target.available) }} 可用</el-tag>
                          </div>
                        </div>
                      </el-option>
                    </el-select>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>选择磁盘最终存储位置，留空时使用管理员设置的默认存储位置</div>
                  </el-form-item>
                </div>
              </div>

              <div class="form-section-card">
                <div class="form-section-card-header">
                  <el-icon><Upload /></el-icon>
                  <span>磁盘导入</span>
                </div>
                <div class="form-section-card-body">
                  <!-- 磁盘来源选择（管理员显示绝对路径选项） -->
                  <el-form-item v-if="isAdmin" label="磁盘来源">
                    <el-radio-group v-model="form.disk_source_type" @change="onDiskSourceTypeChange">
                      <el-radio value="storage">从我的存储选择</el-radio>
                      <el-radio value="path">输入绝对路径</el-radio>
                    </el-radio-group>
                  </el-form-item>

                  <!-- 模式1: 从我的存储选择 -->
                  <template v-if="!isAdmin || form.disk_source_type === 'storage'">
                    <el-form-item label="磁盘文件" :prop="!isAdmin ? 'disk_file' : ''">
                      <el-select v-model="form.disk_file" placeholder="从我的存储选择磁盘文件" style="width: 100%;" v-loading="diskFilesLoading" @focus="loadDiskFiles">
                        <el-option v-for="file in diskFileList" :key="file.name" :label="file.name" :value="file.name">
                          <div style="display: flex; justify-content: space-between;">
                            <span>{{ file.name }}</span>
                            <span style="color: #999; font-size: 12px;">{{ file.size_text }}</span>
                          </div>
                        </el-option>
                      </el-select>
                      <div style="margin-top: 4px; color: #909399; font-size: 12px;">
                        从「我的存储 → 虚拟磁盘」中选择导出或上传的磁盘文件
                      </div>
                    </el-form-item>
                  </template>

                  <!-- 模式2: 输入绝对路径（仅管理员） -->
                  <template v-if="isAdmin && form.disk_source_type === 'path'">
                    <el-form-item label="磁盘路径" prop="disk_path">
                      <el-input v-model="form.disk_path" placeholder="请输入磁盘文件的绝对路径，如 /data/disk.qcow2" clearable />
                      <div class="form-tip">
                        <el-icon><InfoFilled /></el-icon>支持 qcow2、raw、vmdk、vhd、vhdx、img 等格式，非 qcow2 格式将自动转换为 qcow2
                      </div>
                    </el-form-item>
                  </template>

                  <el-form-item label="磁盘处理">
                    <el-radio-group v-model="form.copy_disk">
                      <el-radio :value="false">不保留原磁盘文件（推荐，节省空间）</el-radio>
                      <el-radio :value="true">保留原磁盘文件</el-radio>
                    </el-radio-group>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>无论原磁盘是 qcow2 还是其他格式（raw/vmdk 等），选择不保留都会在导入完成后删除原文件</div>
                  </el-form-item>

                  <el-form-item label="初始化类型">
                    <el-select v-model="form.init_type" placeholder="不初始化" clearable style="width: 100%;" @change="onInitTypeChange">
                      <el-option label="不初始化" value="" />
                      <el-option label="Linux（SSH 初始化）" value="linux" />
                      <el-option label="Windows（仅创建 VM）" value="windows" />
                      <el-option label="其他（仅创建 VM）" value="other" />
                    </el-select>
                    <div style="margin-top: 4px; color: #909399; font-size: 12px;">
                      Linux 初始化会通过 SSH 设置主机名、用户密码等；Windows 和其他类型仅创建虚拟机
                    </div>
                  </el-form-item>

                  <!-- 系统盘 IOPS 限制（仅管理员） -->
                  <el-form-item v-if="isAdmin" label="系统盘 IOPS">
                    <div style="display: flex; gap: 8px; align-items: center; width: 100%; flex-wrap: wrap;">
                      <span style="font-size: 12px; color: #909399; white-space: nowrap;">总</span>
                      <el-input-number v-model="form.system_disk_iops_total" :min="0" :step="100" size="small" style="width: 100px;" placeholder="总IOPS" />
                      <span style="font-size: 12px; color: #909399; white-space: nowrap;">读</span>
                      <el-input-number v-model="form.system_disk_iops_read" :min="0" :step="100" size="small" style="width: 100px;" placeholder="读IOPS" :disabled="form.system_disk_iops_total > 0" />
                      <span style="font-size: 12px; color: #909399; white-space: nowrap;">写</span>
                      <el-input-number v-model="form.system_disk_iops_write" :min="0" :step="100" size="small" style="width: 100px;" placeholder="写IOPS" :disabled="form.system_disk_iops_total > 0" />
                      <span style="font-size: 11px; color: var(--el-color-warning);">互斥</span>
                    </div>
                  </el-form-item>

                  <!-- 导入完成后是否开启虚拟机 -->
                  <el-form-item label="导入后操作">
                    <el-switch v-model="form.start_after_import" active-text="导入完成后开启虚拟机" inactive-text="仅创建不开启" />
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>选择&quot;仅创建不开启&quot;时，虚拟机将被定义但不会启动，您可以稍后手动开机</div>
                  </el-form-item>

                  <!-- 额外导入磁盘 -->
                  <template v-if="isAdmin">
                    <el-divider content-position="left">额外磁盘导入</el-divider>
                    <div v-for="(disk, index) in form.extra_import_disks" :key="index" class="extra-import-disk-item" style="margin-bottom: 12px; padding: 8px; background: #f5f7fa; border-radius: 6px;">
                      <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px;">
                        <span style="font-weight: 500; font-size: 13px;">磁盘 {{ index + 1 }}</span>
                        <el-button type="danger" size="small" icon="Delete" circle @click="form.extra_import_disks.splice(index, 1)" />
                      </div>
                      <el-radio-group v-model="disk.disk_source_type" size="small" style="margin-bottom: 6px;">
                        <el-radio value="path">绝对路径</el-radio>
                        <el-radio value="storage">从存储选择</el-radio>
                      </el-radio-group>
                      <el-input v-if="disk.disk_source_type === 'path' || !disk.disk_source_type" v-model="disk.disk_path" placeholder="磁盘文件绝对路径，如 /data/disk.qcow2" size="small" style="margin-bottom: 6px;" />
                      <el-select v-else v-model="disk.disk_file" placeholder="选择磁盘文件" size="small" style="width: 100%; margin-bottom: 6px;" v-loading="diskFilesLoading" @focus="loadDiskFiles">
                        <el-option v-for="file in diskFileList" :key="file.name" :label="file.name" :value="file.name" />
                      </el-select>
                      <div style="display: flex; gap: 6px;">
                        <el-select v-model="disk.storage_pool_id" placeholder="目标存储位置" clearable filterable size="small" style="flex: 1;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
                          <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id" />
                        </el-select>
                        <el-select v-model="disk.bus" size="small" style="width: 100px;">
                          <el-option label="VirtIO" value="virtio" />
                          <el-option label="SCSI" value="scsi" />
                          <el-option label="SATA" value="sata" />
                          <el-option label="IDE" value="ide" />
                        </el-select>
                      </div>
                      <div style="display: flex; align-items: center; gap: 8px; margin-top: 4px;">
                        <el-button size="small" @click="openImportExtraDiskIOPSDialog(index)" style="font-size: 11px;">IOPS 限制</el-button>
                        <span v-if="disk.iops_total > 0 || disk.iops_read > 0 || disk.iops_write > 0" style="font-size: 11px; color: var(--el-color-warning);">
                          总:{{ disk.iops_total || 0 }} 读:{{ disk.iops_read || 0 }} 写:{{ disk.iops_write || 0 }}
                        </span>
                      </div>
                      <el-checkbox v-model="disk.copy_disk" size="small" style="margin-top: 4px;">保留原文件</el-checkbox>
                    </div>
                    <el-button type="primary" size="small" plain icon="Plus" @click="addExtraImportDisk" style="width: 100%;">
                      添加额外导入磁盘
                    </el-button>
                    <div class="form-tip"><el-icon><InfoFilled /></el-icon>额外磁盘将在虚拟机创建后依次挂载，支持绝对路径和存储选择，非 qcow2 格式自动转换</div>
                  </template>
                </div>
              </div>

              <!-- Linux 初始化配置 -->
              <template v-if="form.init_type === 'linux'">
                <div class="form-section-card">
                  <div class="form-section-card-header">
                    <el-icon><Setting /></el-icon>
                    <span>Linux 初始化配置</span>
                  </div>
                  <div class="form-section-card-body">
                    <el-form-item label="主机名">
                      <el-input v-model="form.hostname" placeholder="留空使用虚拟机名称" />
                    </el-form-item>
                    <el-row :gutter="20">
                      <el-col :span="12">
                        <el-form-item label="原系统用户名">
                          <el-input v-model="form.template_user" placeholder="磁盘中已有的普通用户名" />
                        </el-form-item>
                      </el-col>
                      <el-col :span="12">
                        <el-form-item label="原系统root密码">
                          <el-input v-model="form.template_root_pass" placeholder="磁盘中的root密码（SSH连接用）" type="password" show-password />
                        </el-form-item>
                      </el-col>
                    </el-row>
                    <el-row :gutter="20">
                      <el-col :span="12">
                        <el-form-item label="新用户名">
                          <el-input v-model="form.import_user" placeholder="要创建的新用户名" />
                        </el-form-item>
                      </el-col>
                      <el-col :span="12">
                        <el-form-item label="新密码">
                          <el-input v-model="form.import_password" placeholder="新用户的密码" type="password" show-password />
                        </el-form-item>
                      </el-col>
                    </el-row>
                  </div>
                </div>
              </template>
            </template>

          </div>
        </div>

        <div v-show="createSteps[createStep]?.name === 'network'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon network"><FormIcons icon="step-network" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">网络设置</div>
              <div class="step-pane-desc">配置网卡类型和 VPC 安全组</div>
            </div>
          </div>
          <div class="step-pane-body">
            <div class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Connection /></el-icon>
                <span>网络接口</span>
              </div>
              <div class="form-section-card-body">
                <el-row :gutter="20">
                  <el-col :span="24">
                    <el-form-item label="网卡类型">
                      <el-select v-model="form.nic_model" style="width: 100%;">
                        <el-option label="VirtIO" value="virtio" />
                        <el-option label="e1000e (Intel)" value="e1000e" />
                        <el-option label="rtl8139" value="rtl8139" />
                      </el-select>
                    </el-form-item>
                  </el-col>
                </el-row>
              </div>
            </div>

            <div class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Lock /></el-icon>
                <span>VPC 安全</span>
              </div>
              <div class="form-section-card-body">
                <el-alert
                  v-if="registrationMode"
                  type="info"
                  :closable="false"
                  show-icon
                  style="margin-bottom: 14px;"
                  :title="`将自动接入该轻量云用户的专用 VPC：${registrationContext.dedicated_vpc_label || '管理员已配置'}`"
                />
                <el-row v-if="!registrationMode" :gutter="20">
                  <el-col :span="12">
                    <el-form-item label="VPC 交换机" prop="switch_id">
                      <el-select v-model="form.switch_id" placeholder="选择交换机" style="width: 100%;" filterable @focus="loadVPCOptions">
                        <el-option
                          v-for="item in vpcSwitches"
                          :key="item.id"
                          :label="item.bridge_mode === 'bridge' ? `${item.name}（桥接直通${item.bridge_vlan_id > 0 ? ' VLAN ' + item.bridge_vlan_id : ''}）` : `${item.name} (${item.cidr})`"
                          :value="item.id"
                        >
                          <div style="display: flex; justify-content: space-between; align-items: center;">
                            <span>{{ item.name }}</span>
                            <el-tag size="small" :type="item.bridge_mode === 'bridge' ? 'warning' : 'info'">{{ item.bridge_mode === 'bridge' ? `${item.bridge_name || '桥接'}${item.bridge_vlan_id > 0 ? ' / VLAN ' + item.bridge_vlan_id : ''}` : item.cidr }}</el-tag>
                          </div>
                        </el-option>
                      </el-select>
                      <div v-if="selectedVPCSwitch?.bridge_mode === 'bridge'" class="form-tip">
                        <el-icon><InfoFilled /></el-icon>桥接直通由上级路由器分配 IP，不启用内部 DHCP、NAT、安全组和端口转发
                      </div>
                      <div v-if="!isAdmin && vpcSwitches.length === 0" class="form-tip">
                        <el-icon><InfoFilled /></el-icon>请先在左侧 VPC 网络中创建交换机
                      </div>
                    </el-form-item>
                  </el-col>
                  <el-col v-if="selectedVPCSwitch?.bridge_mode !== 'bridge'" :span="12">
                    <el-form-item label="安全组" prop="security_group_id">
                      <el-select v-model="form.security_group_id" placeholder="选择安全组" style="width: 100%;" filterable @focus="loadVPCOptions">
                        <el-option
                          v-for="item in vpcSecurityGroups"
                          :key="item.id"
                          :label="item.is_default ? `${item.name}（默认）` : item.name"
                          :value="item.id"
                        />
                      </el-select>
                    </el-form-item>
                  </el-col>
                </el-row>
              </div>
            </div>

            <div v-if="registrationMode" class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Connection /></el-icon>
                <span>轻量云网络配额</span>
              </div>
              <div class="form-section-card-body">
                <el-row :gutter="20">
                  <el-col :span="12">
                    <el-form-item label="下行月流量(GB)">
                      <el-input-number v-model="form.traffic_down_gb" :min="0" :precision="2" style="width: 100%;" />
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item label="上行月流量(GB)">
                      <el-input-number v-model="form.traffic_up_gb" :min="0" :precision="2" style="width: 100%;" />
                    </el-form-item>
                  </el-col>
                </el-row>
                <el-row :gutter="20">
                  <el-col :span="12">
                    <el-form-item label="下行带宽(Mbps)">
                      <el-input-number v-model="form.bandwidth_down_mbps" :min="0" style="width: 100%;" />
                    </el-form-item>
                  </el-col>
                  <el-col :span="12">
                    <el-form-item label="上行带宽(Mbps)">
                      <el-input-number v-model="form.bandwidth_up_mbps" :min="0" style="width: 100%;" />
                    </el-form-item>
                  </el-col>
                </el-row>
                <el-form-item label="端口转发上限">
                  <el-input-number v-model="form.max_port_forwards" :min="0" style="width: 100%;" />
                  <div class="form-tip"><el-icon><InfoFilled /></el-icon>设为 0 表示不限制；默认建议 10 条。</div>
                </el-form-item>
                <el-form-item label="运行时长配额(小时)">
                  <el-input-number v-model="form.max_runtime_hours" :min="0" style="width: 100%;" />
                  <div class="form-tip"><el-icon><InfoFilled /></el-icon>设为 0 表示不限制；到时系统会自动关机并提前邮件提醒。</div>
                </el-form-item>
              </div>
            </div>

            <!-- 多网口配置（仅管理员，创建/批量模式） -->
            <div v-if="isAdmin && !registrationMode" class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Plus /></el-icon>
                <span>额外网口（可选）</span>
                <el-button size="small" type="primary" plain style="margin-left: auto;" @click="addExtraNic">
                  <el-icon><Plus /></el-icon> 添加网口
                </el-button>
              </div>
              <div class="form-section-card-body" v-if="extraNics.length > 0">
                <div
                  v-for="(nic, index) in extraNics"
                  :key="index"
                  class="extra-nic-row"
                >
                  <div class="extra-nic-header">
                    <el-tag size="small" type="info">网口 #{{ index + 2 }}</el-tag>
                    <el-button size="small" type="danger" text @click="removeExtraNic(index)">
                      <el-icon><Delete /></el-icon>
                    </el-button>
                  </div>
                  <el-row :gutter="16">
                    <el-col :span="8">
                      <el-form-item label="网卡型号" style="margin-bottom: 8px;" label-width="80px">
                        <el-select :model-value="nic.nic_model" style="width: 100%;" @update:model-value="(v) => nic.nic_model = v">
                          <el-option label="VirtIO" value="virtio" />
                          <el-option label="e1000e (Intel)" value="e1000e" />
                          <el-option label="rtl8139" value="rtl8139" />
                        </el-select>
                      </el-form-item>
                    </el-col>
                    <el-col :span="8">
                      <el-form-item label="VPC 交换机" style="margin-bottom: 8px;" label-width="85px">
                        <el-select :model-value="nic.switch_id" placeholder="选择交换机" style="width: 100%;" filterable @update:model-value="(v) => nic.switch_id = v" @focus="loadVPCOptions">
                          <el-option
                            v-for="item in vpcSwitches"
                            :key="item.id"
                            :label="switchOptionLabelNic(item)"
                            :value="item.id"
                          />
                        </el-select>
                      </el-form-item>
                    </el-col>
                    <el-col :span="8">
                      <el-form-item label="安全组" style="margin-bottom: 8px;" label-width="60px">
                        <el-select :model-value="nic.security_group_id" placeholder="可选" style="width: 100%;" filterable @update:model-value="(v) => nic.security_group_id = v" @focus="loadVPCOptions">
                          <el-option
                            v-for="item in vpcSecurityGroups"
                            :key="item.id"
                            :label="item.is_default ? `${item.name}（默认）` : item.name"
                            :value="item.id"
                          />
                        </el-select>
                      </el-form-item>
                    </el-col>
                  </el-row>
                </div>
              </div>
              <div v-else class="form-section-card-body">
                <el-empty description="暂无额外网口，点击上方「添加网口」为虚拟机配置多网卡" :image-size="48" />
              </div>
            </div>
          </div>
        </div>

        <div v-show="createSteps[createStep]?.name === 'security'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon system"><FormIcons icon="step-lock" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">系统配置</div>
              <div class="step-pane-desc">设置引导顺序、守护服务和开机自启</div>
            </div>
          </div>
          <div class="step-pane-body">
            <div class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Operation /></el-icon>
                <span>引导顺序</span>
              </div>
              <div class="form-section-card-body">
                <el-form-item label="引导顺序" class="boot-devices-form-item">
                  <div class="boot-devices-panel">
                    <div class="boot-devices-list">
                      <div
                        v-for="(item, index) in form.boot_order"
                        :key="item"
                        class="boot-device-row"
                      >
                        <div class="boot-device-info">
                          <span class="boot-device-type">
                            <el-icon style="margin-right: 4px;"><component :is="bootOrderIcon(item)" /></el-icon>
                            {{ bootOrderLabel(item) }}
                          </span>
                        </div>
                        <div class="boot-device-actions">
                          <el-button size="small" :icon="Top" :disabled="index === 0" @click="moveBootOrderUp(index)" />
                          <el-button size="small" :icon="Bottom" :disabled="index === form.boot_order.length - 1" @click="moveBootOrderDown(index)" />
                          <el-button size="small" type="danger" plain :disabled="form.boot_order.length <= 1" @click="removeBootOrder(index)">
                            <el-icon><Delete /></el-icon>
                          </el-button>
                        </div>
                      </div>
                    </div>
                    <div v-if="availableBootDevices.length > 0" style="padding: 8px 16px; border-top: 1px solid #ebeef5;">
                      <el-dropdown @command="addBootOrder">
                        <el-button size="small" type="primary" plain><el-icon><Plus /></el-icon> 添加引导设备</el-button>
                        <template #dropdown>
                          <el-dropdown-menu>
                            <el-dropdown-item v-for="dev in availableBootDevices" :key="dev.value" :command="dev.value">
                              <el-icon style="margin-right: 4px;"><component :is="bootOrderIcon(dev.value)" /></el-icon>
                              {{ dev.label }}
                            </el-dropdown-item>
                          </el-dropdown-menu>
                        </template>
                      </el-dropdown>
                    </div>
                  </div>
                </el-form-item>
              </div>
            </div>

            <div class="form-section-card">
              <div class="form-section-card-header">
                <el-icon><Switch /></el-icon>
                <span>系统行为</span>
              </div>
              <div class="form-section-card-body">
                <el-form-item label="监督者 (Watchdog)">
                  <el-select v-model="form.watchdog" style="width: 100%;">
                    <el-option label="不启用" value="none" />
                    <el-option label="i6300esb" value="i6300esb" />
                    <el-option label="iTCO (推荐)" value="itco" />
                  </el-select>
                </el-form-item>

                <el-form-item label="开机自启">
                  <el-switch v-model="form.autostart" active-text="启用" inactive-text="关闭" />
                </el-form-item>
              </div>
            </div>
          </div>
        </div>

        <div v-show="createSteps[createStep]?.name === 'advanced'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon advanced"><FormIcons icon="step-lightning" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">高级选项</div>
              <div class="step-pane-desc">开发者选项和底层参数，一般保持默认即可</div>
            </div>
          </div>
          <div class="step-pane-body">  
            <div class="tab-content-wrapper advanced-settings-page">
            <div class="advanced-settings-main" :class="{ 'content-blurred': shouldShowAdvancedIntro }">
              <el-alert type="warning" :closable="false" class="advanced-inline-alert">
                <template #title>
                  高级设置仅建议在调试或排查启动问题时使用，普通情况下请保持默认配置。
                </template>
              </el-alert>
              <el-divider content-position="left">开发者选项</el-divider>
              <el-form-item label="启动时冻结 CPU">
                <div class="advanced-field-row">
                  <el-switch v-model="form.freeze" active-text="启用" inactive-text="关闭" />
                  <el-tooltip :content="freezeHelpText" placement="top" effect="dark">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
              </el-form-item>
              <el-form-item label="APIC">
                <div class="advanced-field-row">
                  <el-switch v-model="form.apic" active-text="启用" inactive-text="关闭" />
                  <el-tooltip :content="apicHelpText" placement="top" effect="dark">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  常规虚拟机建议保持启用；仅在排查非常早期的启动兼容性问题时再尝试关闭
                </div>
              </el-form-item>
              <el-form-item label="PAE">
                <div class="advanced-field-row">
                  <el-switch v-model="form.pae" active-text="启用" inactive-text="关闭" />
                  <el-tooltip :content="paeHelpText" placement="top" effect="dark">
                    <el-icon class="help-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  主要用于 x86 老系统或 32 位来宾的大内存兼容场景；非 x86 架构会自动忽略该设置
                </div>
              </el-form-item>
              <el-form-item label="CPU 拓扑">
                <el-select v-model="form.cpu_topology_mode" style="width: 280px;">
                  <el-option v-for="item in cpuTopologyModeOptions" :key="item.value" :label="item.label" :value="item.value" />
                </el-select>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  自动模式会为 Windows 使用单插槽多核心；排查兼容性时可显式选择宿主默认拓扑
                </div>
              </el-form-item>
              <el-form-item v-if="isAdmin" label="CPU 亲和性">
                <el-select v-model="form.cpu_affinity" filterable allow-create default-first-option clearable placeholder="例如: 0,2,4 或 0-3" style="width: 320px;">
                  <el-option v-for="preset in cpuAffinityPresets" :key="preset.name" :label="preset.name" :value="preset.value" />
                </el-select>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  将虚拟机的 vCPU 绑定到指定的物理 CPU 核心上，多个核心用逗号或空格分隔，支持范围格式（如 0-3）。留空表示不限制亲和性
                </div>
              </el-form-item>
              <el-form-item label="PCIe 热插槽" v-if="form.machine_type === 'q35'">
                <el-input-number v-model="form.pcie_root_ports" :min="0" :max="32" :step="1" style="width: 160px;" />
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  预留的 pcie-root-port 数量，设为 0 使用默认值（4）。足够的插槽可避免后续热添加磁盘时提示"无可用 PCI 插槽"
                </div>
              </el-form-item>
              <el-form-item v-if="isTemplateSourceMode && isWindowsTemplate" label="首次重启">
                <el-select v-model="form.first_boot_reboot_mode" style="width: 280px;">
                  <el-option v-for="item in firstBootRebootModeOptions" :key="item.value" :label="item.label" :value="item.value" />
                </el-select>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  LTSC 等模板若在 Sysprep/OOBE 自动重启后黑屏，可选择宿主冷启动
                </div>
              </el-form-item>
              <el-form-item label="显示设备">
                <el-select v-model="form.video_model" style="width: 280px;">
                  <el-option v-for="item in videoModelOptions" :key="item.value" :label="item.label" :value="item.value">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                      <span>{{ item.label }}</span>
                      <el-tag size="small" :type="item.tagType">{{ item.tag }}</el-tag>
                    </div>
                  </el-option>
                </el-select>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  Windows 安装或 VMware 嵌套环境可优先尝试 VGA / VMVGA；若系统已安装 VirtIO 驱动可再切回 VirtIO
                </div>
              </el-form-item>
              <el-form-item label="RTC">
                <div class="advanced-config-entry" @click="openRTCConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">配置</div>
                    <div class="advanced-config-entry-summary">{{ rtcConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item label="QEMU Guest Agent">
                <div class="advanced-config-entry" @click="openGuestAgentConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">配置</div>
                    <div class="advanced-config-entry-summary">{{ guestAgentConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item v-if="isAdmin" label="动态内存">
                <div class="advanced-config-entry" @click="openMemoryDynamicConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">配置</div>
                    <div class="advanced-config-entry-summary">{{ memoryDynamicSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
              <el-form-item label="SMBIOS">
                <div class="advanced-config-entry" @click="openSMBIOSConfigDialog">
                  <div class="advanced-config-entry-main">
                    <div class="advanced-config-entry-title">类型 1</div>
                    <div class="advanced-config-entry-summary">{{ smbiosConfigSummary }}</div>
                  </div>
                  <el-icon class="advanced-config-entry-icon"><ArrowRight /></el-icon>
                </div>
              </el-form-item>
            </div>
            <div v-if="shouldShowAdvancedIntro" class="advanced-intro-mask">
              <el-card shadow="always" class="advanced-intro-card">
                <template #header>
                  <div class="advanced-intro-header">进阶设置提醒</div>
                </template>
                <div class="advanced-intro-content">
                  <p>此页面属于进阶设置，通常您并不需要调整此页面功能，若您不是开发者或不熟悉虚拟机的情况下请保持默认。</p>
                  <p class="advanced-intro-warning">仅在明确了解这些选项的用途及可能影响时再修改这里的选项。</p>
                  <div class="advanced-intro-actions">
                    <el-button type="primary" @click="dismissAdvancedIntro">我知道了</el-button>
                  </div>
                </div>
              </el-card>
            </div>
          </div>
            </div><!-- /step-pane-body -->
        </div>
        <!-- 硬件直通（仅管理员） -->
        <div v-if="isAdmin" v-show="createSteps[createStep]?.name === 'passthrough'" class="step-pane">
          <div class="step-pane-header">
            <div class="step-pane-icon advanced"><FormIcons icon="step-plug" :size="22" /></div>
            <div class="step-pane-info">
              <div class="step-pane-title">硬件直通</div>
              <div class="step-pane-desc">将宿主机 PCI 设备直接分配给虚拟机，获得接近原生的性能</div>
            </div>
          </div>
          <div class="step-pane-body">
            <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px;">
              <template #title>
                硬件直通可将 GPU、NVMe 硬盘、网卡等 PCI 设备直接分配给虚拟机。配置后设备将在虚拟机启动时自动绑定到 vfio-pci 驱动。
              </template>
            </el-alert>
            <el-form-item label="直通设备">
              <div style="width: 100%;">
                <div v-if="form.host_devices.length > 0">
                  <el-table :data="form.host_devices" size="small" border style="width: 100%; margin-bottom: 8px;">
                    <el-table-column label="PCI 地址" prop="pci_address" width="160" />
                    <el-table-column label="设备名称" min-width="180">
                      <template #default="{ row }">
                        <span>{{ getPassthroughDeviceName(row.pci_address) }}</span>
                      </template>
                    </el-table-column>
                    <el-table-column label="状态" width="100">
                      <template #default="{ row }">
                        <el-tag v-if="isDeviceVfioBound(row.pci_address)" type="success" size="small">已绑定</el-tag>
                        <el-tag v-else type="warning" size="small">待绑定</el-tag>
                      </template>
                    </el-table-column>
                    <el-table-column label="操作" width="120" align="center">
                      <template #default="{ row, $index }">
                        <el-button size="small" type="danger" plain @click="removeHostDevice($index)">移除</el-button>
                      </template>
                    </el-table-column>
                  </el-table>
                </div>
                <div v-else style="color: #909399; font-size: 13px; margin-bottom: 8px;">未配置直通设备</div>
                <el-button type="primary" size="small" plain icon="Plus" @click="openPassthroughDialog">
                  添加直通设备
                </el-button>
              </div>
            </el-form-item>
          </div>
        </div>
      </template><!-- /创建模式 -->

    </el-form>

      </div><!-- /vm-form-left -->

      <!-- ==================== 右侧预览面板 ==================== -->
      <div class="vm-form-right">
        <div class="preview-panel">
          <div class="preview-header">
            <span><FormIcons icon="preview-chart" :size="18" /> 配置预览</span>
          </div>
          <div class="preview-body">
            <div v-if="!isEdit" class="preview-section">
              <div class="preview-section-title">基础信息</div>
              <div class="preview-row"><span class="pr-label">创建模式</span><span class="pr-value">{{ createModeLabel }}</span></div>
              <div class="preview-row"><span class="pr-label">虚拟机名称</span><span class="pr-value highlight">{{ form.name || '未命名' }}</span></div>
              <div v-if="form.remark" class="preview-row"><span class="pr-label">备注</span><span class="pr-value">{{ form.remark }}</span></div>
              <div class="preview-row"><span class="pr-label">操作系统</span><span class="pr-value">{{ osLabel }}</span></div>
              <div v-if="form.os_variant" class="preview-row"><span class="pr-label">系统版本</span><span class="pr-value">{{ form.os_variant }}</span></div>
            </div>
            <div v-else class="preview-section">
              <div class="preview-section-title">虚拟机信息</div>
              <div class="preview-row"><span class="pr-label">名称</span><span class="pr-value highlight">{{ form.name }}</span></div>
              <div class="preview-row"><span class="pr-label">状态</span><span class="pr-value"><FormIcons :icon="editVmStatus === 'running' ? 'status-run' : 'status-stop'" :size="14" /> {{ editVmStatus === 'running' ? '运行中' : '已关机' }}</span></div>
              <div class="preview-row"><span class="pr-label">操作系统</span><span class="pr-value">{{ form.os_type || '未知' }}</span></div>
            </div>
            <div class="preview-section">
              <div class="preview-section-title">硬件规格</div>
              <div class="preview-row"><span class="pr-label">CPU</span><span class="pr-value">{{ form.vcpu || 0 }} 核</span></div>
              <div class="preview-row"><span class="pr-label">内存</span><span class="pr-value">{{ previewMemory }} GB</span></div>
              <div v-if="!isEdit" class="preview-row"><span class="pr-label">虚拟化引擎</span><span class="pr-value"><FormIcons :icon="form.virt_type === 'kvm' ? 'virt-kvm' : 'virt-qemu'" :size="14" /> {{ form.virt_type === 'kvm' ? 'KVM' : 'QEMU' }}</span></div>
              <div v-if="!isEdit" class="preview-row"><span class="pr-label">芯片组</span><span class="pr-value">{{ form.machine_type?.toUpperCase() }}</span></div>
              <div v-if="!isEdit" class="preview-row"><span class="pr-label">引导固件</span><span class="pr-value">{{ bootTypePreviewLabel }}</span></div>
            </div>
            <div v-if="!isEdit || (isEdit && activeTabEdit === 'disk')" class="preview-section">
              <div class="preview-section-title">{{ isEdit ? '磁盘管理' : '存储' }}</div>
              <template v-if="isEdit">
                <div class="preview-row"><span class="pr-label">磁盘数</span><span class="pr-value">{{ editDisks.length }} 个</span></div>
                <div class="preview-row"><span class="pr-label">光驱数</span><span class="pr-value">{{ editCdroms.length }} 个</span></div>
                <div class="preview-row"><span class="pr-label">待新增</span><span class="pr-value">{{ form.add_disks.length }} 个</span></div>
              </template>
              <template v-else>
                <template v-if="form.create_mode === 'iso'">
                  <div class="preview-row"><span class="pr-label">系统盘</span><span class="pr-value">{{ form.disk_size }} GB ({{ form.disk_format?.toUpperCase() }})</span></div>
                  <div class="preview-row"><span class="pr-label">磁盘驱动</span><span class="pr-value">{{ form.disk_bus }}</span></div>
                  <div v-if="form.iso_path" class="preview-row"><span class="pr-label">ISO</span><span class="pr-value" style="max-width: 120px; overflow: hidden; text-overflow: ellipsis;">{{ form.iso_path.split('/').pop() }}</span></div>
                  <div v-if="form.iso_paths.length > 1" class="preview-row"><span class="pr-label">额外挂载</span><span class="pr-value">+{{ form.iso_paths.length - 1 }} 个 ISO</span></div>
                </template>
                <template v-else-if="isTemplateSourceMode">
                  <div class="preview-row"><span class="pr-label">模板</span><span class="pr-value" style="max-width: 120px; overflow: hidden; text-overflow: ellipsis;">{{ form.template || '未选择' }}</span></div>
                  <div class="preview-row"><span class="pr-label">磁盘大小</span><span class="pr-value">{{ form.disk_size }} GB</span></div>
                  <div class="preview-row"><span class="pr-label">磁盘驱动</span><span class="pr-value">{{ form.disk_bus || 'virtio' }}</span></div>
                </template>
                <template v-else>
                  <div class="preview-row"><span class="pr-label">磁盘文件</span><span class="pr-value" style="max-width: 120px; overflow: hidden; text-overflow: ellipsis;">{{ form.disk_file || '未选择' }}</span></div>
                  <div v-if="form.init_type" class="preview-row"><span class="pr-label">初始化</span><span class="pr-value">{{ form.init_type }}</span></div>
                </template>
              </template>
            </div>
            <div v-if="!isEdit || (isEdit && activeTabEdit === 'security')" class="preview-section">
              <div class="preview-section-title">网络与系统</div>
              <template v-if="isEdit">
                <div class="preview-row"><span class="pr-label">网卡型号</span><span class="pr-value">{{ form.nic_model }}</span></div>
                <div class="preview-row"><span class="pr-label">开机自启</span><span class="pr-value"><FormIcons :icon="form.autostart ? 'check' : 'close'" :size="14" /> {{ form.autostart ? '启用' : '关闭' }}</span></div>
              </template>
                <template v-else>
                  <div class="preview-row"><span class="pr-label">网卡型号</span><span class="pr-value">{{ form.nic_model }}</span></div>
                  <div class="preview-row"><span class="pr-label">开机自启</span><span class="pr-value"><FormIcons :icon="form.autostart ? 'check' : 'close'" :size="14" /> {{ form.autostart ? '启用' : '关闭' }}</span></div>
                </template>
            </div>
            <div class="preview-total">
              <span class="total-label">预估资源占用</span>
              <div class="total-values">
                <div class="total-value">CPU {{ form.vcpu || 0 }} 核</div>
                <div class="total-value">内存 {{ previewMemory }} GB</div>
                <div v-if="!isEdit && form.disk_size > 0" class="total-value">磁盘 {{ form.disk_size }} GB</div>
              </div>
            </div>
          </div>
        </div>
      </div><!-- /vm-form-right -->
    </div><!-- /vm-form-layout -->

    <template #footer>
      <span class="dialog-footer">
        <el-button @click="visible = false">取消</el-button>
        <template v-if="!isEdit">
          <el-button v-if="createStep > 0" @click="prevStep">上一步</el-button>
          <el-button v-if="createStep < maxStep" type="primary" @click="nextStep">下一步</el-button>
          <el-tooltip :content="allRequiredTip" placement="top" :disabled="allRequiredFilled">
            <el-button
              type="warning"
              :loading="loading"
              :disabled="!allRequiredFilled"
              @click="submitForm"
            >
              {{ submitButtonText }}
            </el-button>
          </el-tooltip>
        </template>
        <template v-else>
          <el-button type="primary" :loading="loading" @click="submitForm">保存修改</el-button>
        </template>
      </span>
    </template>
  </el-dialog>

  <!-- 挂载已有磁盘对话框 -->
  <el-dialog v-model="attachDiskVisible" title="导入磁盘到虚拟机" width="560px" append-to-body>
    <el-form label-width="110px">
      <el-form-item v-if="isAdmin" label="磁盘来源">
        <el-radio-group v-model="attachDiskSourceType" @change="onAttachDiskSourceTypeChange">
          <el-radio value="storage">从我的存储选择</el-radio>
          <el-radio value="path">输入绝对路径</el-radio>
        </el-radio-group>
      </el-form-item>
      <template v-if="!isAdmin || attachDiskSourceType === 'storage'">
        <el-form-item label="磁盘文件">
          <el-select v-model="attachDiskPath" placeholder="请选择磁盘文件" style="width: 100%;" filterable :loading="attachDiskLoading">
            <el-option
              v-for="file in attachDiskFiles"
              :key="file.name"
              :label="`${file.name}（${file.size_text}）`"
              :value="file.path"
            />
          </el-select>
          <div v-if="attachDiskFiles.length === 0 && !attachDiskLoading" style="margin-top: 6px; color: #909399; font-size: 12px;">
            没有可用的磁盘文件，请先在「我的存储 → 虚拟磁盘」中上传
          </div>
        </el-form-item>
      </template>
      <template v-if="isAdmin && attachDiskSourceType === 'path'">
        <el-form-item label="磁盘路径">
          <el-input v-model="attachDiskAbsolutePath" placeholder="请输入磁盘文件的绝对路径，如 /data/disk.qcow2" clearable />
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>支持 qcow2、raw、vmdk 等格式，非 qcow2 自动转换</div>
        </el-form-item>
        <el-form-item label="目标存储">
          <el-select v-model="attachDiskStoragePoolId" placeholder="使用默认存储位置" clearable filterable style="width: 100%;" v-loading="storageTargetsLoading" @focus="loadStorageTargets">
            <el-option v-for="target in storageTargets" :key="target.id" :label="storageTargetLabel(target)" :value="target.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="磁盘处理">
          <el-radio-group v-model="attachDiskCopyDisk">
            <el-radio :value="false">不保留原磁盘文件（推荐）</el-radio>
            <el-radio :value="true">保留原磁盘文件</el-radio>
          </el-radio-group>
        </el-form-item>
      </template>
      <el-form-item label="总线类型">
        <el-select v-model="attachDiskBus" style="width: 100%;">
          <el-option label="VirtIO（推荐）" value="virtio" />
          <el-option label="SCSI" value="scsi" />
          <el-option label="SATA" value="sata" />
          <el-option label="IDE" value="ide" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="attachDiskVisible = false">取消</el-button>
      <el-button type="primary" :disabled="attachDiskSubmitDisabled" :loading="attachDiskSubmitting" @click="handleAttachDisk">
        {{ isAdmin && attachDiskSourceType === 'path' ? '提交导入任务' : '挂载' }}
      </el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="rtcConfigVisible" title="RTC 配置" width="560px" append-to-body>
    <el-form label-width="120px">
      <el-form-item label="RTC 时间基准">
        <div class="advanced-field-stack">
          <div class="advanced-field-row">
            <el-radio-group v-model="form.rtc_offset">
              <el-radio-button value="utc">UTC</el-radio-button>
              <el-radio-button value="localtime">本地时间</el-radio-button>
            </el-radio-group>
            <el-tooltip :content="rtcHelpText" placement="top" effect="dark">
              <el-icon class="help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="advanced-field-hint">
            Linux 通常使用 UTC；Windows 默认使用本地时间。运行中的虚拟机修改后需重启生效。
          </div>
        </div>
      </el-form-item>
      <el-form-item label="RTC 开始日期">
        <div class="advanced-field-stack advanced-field-wide">
          <div class="advanced-field-row advanced-field-wide">
            <el-input
              v-model="form.rtc_startdate"
              placeholder="now 或 2026-04-26 12:00:00"
              clearable
            />
            <el-tooltip :content="rtcStartDateHelpText" placement="top" effect="dark">
              <el-icon class="help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="advanced-field-hint">
            默认 `now` 表示每次启动时使用当前时间。若填写固定日期时间，将按该时间初始化 RTC，并切换为固定时间模式。
          </div>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="rtcConfigVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="guestAgentConfigVisible" title="QEMU Guest Agent 配置" width="620px" append-to-body>
    <el-form label-width="150px">
      <el-alert type="info" :closable="false" style="margin-bottom: 16px;">
        <template #title>
          客户机代理安装在虚拟机系统内部，可协助宿主机获取来宾信息、执行更可靠的关机，并为冻结文件系统后再做快照等场景提供支持。
        </template>
      </el-alert>
      <el-form-item label="使用 QEMU Guest Agent">
        <div class="advanced-field-stack advanced-field-wide">
          <div class="advanced-field-row">
            <el-switch v-model="form.guest_agent.enabled" active-text="启用" inactive-text="关闭" />
            <el-tooltip :content="guestAgentHelpText" placement="top" effect="dark">
              <el-icon class="help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="advanced-field-hint">
            Linux / Windows 服务器建议启用，但需要虚拟机内部已安装 `qemu-guest-agent`。编辑已存在虚拟机时，建议关机后修改并重新开机生效。
          </div>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="guestAgentConfigVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="memoryDynamicConfigVisible" title="动态内存配置" width="640px" append-to-body>
    <el-form label-width="150px">
      <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
        <template #title>
          已有运行中的虚拟机启用后会先保存为待迁移状态，下次关机后启动时再应用最大内存和气球设备配置。
        </template>
      </el-alert>
      <el-form-item label="启用动态内存">
        <el-switch v-model="form.memory_dynamic_enabled" active-text="启用" inactive-text="关闭" @change="markMemoryDynamicTouched" />
      </el-form-item>
      <template v-if="form.memory_dynamic_enabled">
        <el-form-item label="内存模式">
          <div class="memory-dynamic-field memory-dynamic-wide">
            <el-radio-group v-model="form.memory_backend" @change="handleMemoryBackendChange">
              <el-radio-button label="balloon">气球调度</el-radio-button>
              <el-radio-button label="virtio_mem" :disabled="windowsElasticMemoryDisabled">Windows 弹性内存</el-radio-button>
            </el-radio-group>
            <el-tag v-if="form.memory_backend === 'virtio_mem'" type="warning" effect="plain" class="memory-experimental-tag">实验</el-tag>
            <el-button v-if="form.memory_backend === 'virtio_mem'" size="small" text type="primary" @click="memoryVirtioMemDetailVisible = true">详情</el-button>
            <div class="form-tip">
              {{ memoryBackendTip }}
            </div>
          </div>
        </el-form-item>
        <el-form-item label="启动内存">
          <div class="memory-dynamic-field">
            <el-input-number v-model="form.memory_initial" class="memory-dynamic-input" :min="1" :max="256" controls-position="right" @change="markMemoryDynamicTouched" />
            <span class="memory-dynamic-unit">GB</span>
            <div class="form-tip">{{ form.memory_backend === 'virtio_mem' ? 'Windows 启动时固定拥有的基础内存，计入用户内存配额' : '计入用户内存配额' }}</div>
          </div>
        </el-form-item>
        <el-form-item v-if="form.memory_backend !== 'virtio_mem'" label="最小内存">
          <div class="memory-dynamic-field">
            <el-input-number v-model="form.memory_min" class="memory-dynamic-input" :min="1" :max="256" controls-position="right" @change="markMemoryDynamicTouched" />
            <span class="memory-dynamic-unit">GB</span>
            <div class="form-tip">普通状态不会自动回收到启动内存以下</div>
          </div>
        </el-form-item>
        <el-form-item label="最大内存">
          <div class="memory-dynamic-field">
            <el-input-number v-model="form.memory_max_dynamic" class="memory-dynamic-input" :min="1" :max="512" controls-position="right" @change="markMemoryDynamicTouched" />
            <span class="memory-dynamic-unit">GB</span>
            <div class="form-tip">{{ form.memory_backend === 'virtio_mem' ? '基础内存 + 可热插拔弹性内存的总上限' : '动态增长上限' }}</div>
          </div>
        </el-form-item>
        <el-form-item v-if="form.memory_backend !== 'virtio_mem'" label="自动调度">
          <el-switch v-model="form.memory_auto_balloon" active-text="启用" inactive-text="关闭" @change="markMemoryDynamicTouched" />
          <div class="form-tip">
            后台会按可用内存和宿主机余量自动调整，手动调整当前内存后会暂停自动调度 10 分钟
          </div>
        </el-form-item>
        <el-form-item v-if="isEdit" label="当前内存">
          <div class="memory-dynamic-field">
            <el-input-number v-model="form.memory_current" class="memory-dynamic-input" :min="0" :max="form.memory_max_dynamic || 512" controls-position="right" @change="markMemoryDynamicTouched" />
            <span class="memory-dynamic-unit">GB</span>
            <div class="form-tip">
              {{ form.memory_backend === 'virtio_mem' ? '仅对已启用 Windows 弹性内存的运行中虚拟机立即生效；0 表示不手动调整' : '仅对运行中的虚拟机立即生效；0 表示不手动调整' }}
            </div>
          </div>
        </el-form-item>
        <el-form-item v-if="isEdit" label="兼容状态">
          <el-tag :type="memoryCompatTagType">{{ memoryCompatLabel }}</el-tag>
          <el-tag v-if="form.memory_backend === 'virtio_mem'" style="margin-left: 8px;" type="warning" effect="plain">
            Windows 弹性内存实验
          </el-tag>
          <el-tag style="margin-left: 8px;" :type="form.memory_balloon_supported ? 'success' : 'warning'">
            {{ form.memory_balloon_supported ? '已配置气球设备' : '缺少气球设备' }}
          </el-tag>
          <div class="form-tip">{{ memoryBalloonStatusText }}</div>
          <div v-if="form.memory_backend === 'virtio_mem'" class="form-tip">
            当前已插入弹性内存：{{ form.memory_virtio_mem_current }}GB
          </div>
        </el-form-item>
      </template>
    </el-form>
    <template #footer>
      <el-button @click="resetMemoryDynamicDefaults">推荐值</el-button>
      <el-button @click="memoryDynamicConfigVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="memoryVirtioMemDetailVisible" title="Windows 弹性内存（实验）" width="620px" append-to-body>
    <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
      <template #title>
        该功能基于 virtio-mem / viomem 驱动，当前作为实验能力提供，请先在测试虚拟机验证后再用于生产。
      </template>
    </el-alert>
    <div class="virtio-mem-detail">
      <p>启用后，主表单填写的内存会作为规格内存。系统自动将基础内存设为规格的 50%（最低 1GB），并把可调度上限设为规格内存上浮 30%。</p>
      <p>虚拟机会先启动基础内存，再通过 virtio-mem 设备按需热插拔额外内存。宿主机占用会跟随基础内存和已插入的弹性内存变化。</p>
      <p>自动调度会按虚拟机内部内存使用率判断：超过 70% 时每次扩容 1GB，低于 50% 时计算缩容目标，并确保缩容后的使用率不超过 70%。</p>
      <p>首次启用或修改基础配置需要虚拟机关机后应用；Windows 内部必须已安装 VirtIO Viomem Driver，否则只能看到设备但无法正常调整。</p>
      <p>缩小弹性内存时，Windows 可能因为当前占用或内存碎片无法立刻全部释放，面板会尽量调低 requested，但实际 current 可能短时间保留一部分。</p>
      <p>该模式不使用气球回收机制。用户看到的内存规格始终按最大规格理解，基础内存由系统自动计算。</p>
    </div>
    <template #footer>
      <el-button type="primary" @click="memoryVirtioMemDetailVisible = false">我知道了</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="smbiosConfigVisible" title="SMBIOS 配置（类型 1）" width="680px" append-to-body>
    <el-form label-width="120px">
      <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
        <template #title>
          SMBIOS 用于向虚拟机暴露厂商、产品、序列号、UUID 等机器身份信息。一般保持默认，只有在授权绑定、资产识别或迁移兼容场景下才建议修改。
        </template>
      </el-alert>
      <el-form-item label="Base64">
        <div class="advanced-field-stack advanced-field-wide">
          <div class="advanced-field-row">
            <el-switch v-model="form.smbios1.base64" active-text="启用" inactive-text="关闭" />
            <el-tooltip :content="smbiosBase64HelpText" placement="top" effect="dark">
              <el-icon class="help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="advanced-field-hint">
            启用后，下面填写的 SMBIOS 字段会先按 Base64 解码，再写入虚拟机定义。回显始终显示解码后的实际值。
          </div>
        </div>
      </el-form-item>
      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item label="厂商名">
            <el-input v-model="form.smbios1.manufacturer" placeholder="例如 QEMU" clearable />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="产品 ID">
            <el-input v-model="form.smbios1.product" placeholder="例如 Standard PC" clearable />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item label="版本">
            <el-input v-model="form.smbios1.version" placeholder="例如 1.0" clearable />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="序列号">
            <el-input v-model="form.smbios1.serial" placeholder="例如 SN-001" clearable />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="16">
        <el-col :span="12">
          <el-form-item label="SKU">
            <el-input v-model="form.smbios1.sku" placeholder="例如 SKU-001" clearable />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="家族名称">
            <el-input v-model="form.smbios1.family" placeholder="例如 Virtual Machine" clearable />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item label="UUID">
        <div class="advanced-field-stack advanced-field-wide">
          <div class="advanced-field-row advanced-field-wide">
            <el-input
              v-if="!isEdit"
              v-model="form.smbios1.uuid"
              placeholder="留空时由系统自动生成"
              clearable
            />
            <el-input
              v-else
              :model-value="currentVmUUID || form.smbios1.uuid || ''"
              disabled
            />
            <el-tooltip :content="smbiosUUIDHelpText" placement="top" effect="dark">
              <el-icon class="help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="advanced-field-hint">
            <template v-if="isEdit">
              已存在虚拟机的 SMBIOS UUID 必须与当前虚拟机 UUID 保持一致，因此编辑页仅支持查看，不支持直接修改。
            </template>
            <template v-else>
              创建、导入或克隆时可选填标准 UUID。若填写，系统会同时使用该 UUID 作为虚拟机 UUID，避免 libvirt 拒绝定义。
            </template>
          </div>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="resetSMBIOS1Config">恢复默认</el-button>
      <el-button @click="smbiosConfigVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="vmXMLConfigVisible" title="编辑虚拟机 XML" width="920px" append-to-body>
    <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
      <template #title>
        这里编辑的是当前虚拟机的持久化 domain XML。保存后会立即写入 libvirt 定义，并刷新当前表单；未提交的普通设置改动会被覆盖，且不支持通过此功能修改虚拟机名称。
      </template>
    </el-alert>
    <div class="vm-xml-editor-toolbar">
      <el-tag :type="editVmStatus === 'running' || editVmStatus === 'paused' ? 'warning' : 'success'" effect="plain">
        {{ editVmStatus === 'running' || editVmStatus === 'paused' ? '运行中：保存后通常需重启生效' : '已关机：可直接保存持久化配置' }}
      </el-tag>
      <span class="vm-xml-editor-hint">建议先关机后再修改，以免运行态与持久化配置出现理解偏差。</span>
    </div>
    <el-input
      v-model="vmXMLContent"
      type="textarea"
      :rows="24"
      resize="vertical"
      class="vm-xml-editor-textarea"
      :disabled="vmXMLLoading || vmXMLSaving"
      spellcheck="false"
      placeholder="正在加载虚拟机 XML..."
    />
    <template #footer>
      <el-button :loading="vmXMLLoading" :disabled="vmXMLSaving" @click="reloadVmXMLContent">重新加载</el-button>
      <el-button @click="vmXMLConfigVisible = false">关闭</el-button>
      <el-button type="primary" :loading="vmXMLSaving" :disabled="vmXMLLoading || !vmXMLDirty || !normalizeVmXMLText(vmXMLContent)" @click="saveVmXMLConfig">保存 XML</el-button>
    </template>
  </el-dialog>

  <!-- 磁盘 IOPS 设置对话框 -->
  <el-dialog v-model="diskIOPSDialogVisible" title="设置磁盘 IOPS 限制" width="480px" :close-on-click-modal="false" :destroy-on-close="false" append-to-body>
    <el-alert type="warning" :closable="false" style="margin-bottom: 12px;">
      <template #title>总 IOPS 与 读/写 IOPS 互斥，请只设置其中一组</template>
    </el-alert>
    <div style="margin-bottom: 16px;">
      <span style="font-weight: 600;">磁盘: </span>
      <code style="background: var(--el-fill-color-light); padding: 2px 8px; border-radius: 4px;">{{ diskIOPSDialogDevice }}</code>
      <span style="margin-left: 12px; color: var(--el-text-color-secondary); font-size: 13px;">{{ diskIOPSDialogPath }}</span>
    </div>
    <el-form label-width="90px" size="default">
      <el-form-item label="总 IOPS">
        <el-input-number v-model="diskIOPSForm.total_iops_sec" :min="0" :step="100" style="width: 100%;" placeholder="0 表示不限制" />
        <div class="form-tip">磁盘每秒总 I/O 操作数限制，设置后将忽略读/写 IOPS</div>
      </el-form-item>
      <el-form-item label="读 IOPS">
        <el-input-number v-model="diskIOPSForm.read_iops_sec" :min="0" :step="100" style="width: 100%;" placeholder="0 表示不限制" :disabled="diskIOPSForm.total_iops_sec > 0" />
        <div class="form-tip">磁盘每秒读取操作数限制（总 IOPS > 0 时无效）</div>
      </el-form-item>
      <el-form-item label="写 IOPS">
        <el-input-number v-model="diskIOPSForm.write_iops_sec" :min="0" :step="100" style="width: 100%;" placeholder="0 表示不限制" :disabled="diskIOPSForm.total_iops_sec > 0" />
        <div class="form-tip">磁盘每秒写入操作数限制（总 IOPS > 0 时无效）</div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="diskIOPSDialogVisible = false">取消</el-button>
      <el-button type="primary" @click="applyDiskIOPSForEditForm">保存 IOPS 设置</el-button>
    </template>
  </el-dialog>

  <!-- 创建模式额外磁盘 IOPS 设置对话框 -->
  <el-dialog v-model="createDiskIOPSDialogVisible" title="设置额外磁盘 IOPS 限制" width="480px" :close-on-click-modal="false" :destroy-on-close="false" append-to-body>
    <el-alert type="warning" :closable="false" style="margin-bottom: 12px;">
      <template #title>总 IOPS 与 读/写 IOPS 互斥，请只设置其中一组</template>
    </el-alert>
    <div style="margin-bottom: 16px;">
      <span style="font-weight: 600;">额外磁盘 #{{ createDiskIOPSIndex + 1 }}</span>
      <span style="margin-left: 12px; color: var(--el-text-color-secondary); font-size: 13px;">{{ createDiskIOPSDesc }}</span>
    </div>
    <el-form label-width="90px" size="default">
      <el-form-item label="总 IOPS">
        <el-input-number v-model="createDiskIOPSForm.total_iops_sec" :min="0" :step="100" style="width: 100%;" placeholder="0 表示不限制" />
        <div class="form-tip">磁盘每秒总 I/O 操作数限制，设置后将忽略读/写 IOPS</div>
      </el-form-item>
      <el-form-item label="读 IOPS">
        <el-input-number v-model="createDiskIOPSForm.read_iops_sec" :min="0" :step="100" style="width: 100%;" placeholder="0 表示不限制" :disabled="createDiskIOPSForm.total_iops_sec > 0" />
        <div class="form-tip">磁盘每秒读取操作数限制（总 IOPS > 0 时无效）</div>
      </el-form-item>
      <el-form-item label="写 IOPS">
        <el-input-number v-model="createDiskIOPSForm.write_iops_sec" :min="0" :step="100" style="width: 100%;" placeholder="0 表示不限制" :disabled="createDiskIOPSForm.total_iops_sec > 0" />
        <div class="form-tip">磁盘每秒写入操作数限制（总 IOPS > 0 时无效）</div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="createDiskIOPSDialogVisible = false">取消</el-button>
      <el-button type="primary" @click="applyCreateDiskIOPS">保存 IOPS 设置</el-button>
    </template>
  </el-dialog>

  <!-- 硬件直通设备选择对话框 -->
  <el-dialog v-model="passthroughDialogVisible" title="选择直通设备" width="700px" :close-on-click-modal="false" :destroy-on-close="false" append-to-body>
    <el-alert type="warning" :closable="false" style="margin-bottom: 12px;">
      <template #title>请确认设备未被其他虚拟机使用，且 IOMMU 组已正确隔离。直通操作需要虚拟机在关机状态下进行。</template>
    </el-alert>
    <el-table :data="passthroughDeviceList" size="small" border style="width: 100%;" max-height="400" v-loading="passthroughLoading" @selection-change="onPassthroughSelectionChange" ref="passthroughTableRef">
      <el-table-column type="selection" width="50" :selectable="isPassthroughDeviceSelectable" />
      <el-table-column label="PCI 地址" prop="pci_address" width="150" />
      <el-table-column label="设备名称" min-width="200">
        <template #default="{ row }">
          <div>{{ row.vendor_name }} {{ row.product_name || '未知设备' }}</div>
          <div style="font-size: 11px; color: #909399;">{{ row.class_name }} · {{ row.vendor_id }}:{{ row.product_id }}</div>
        </template>
      </el-table-column>
      <el-table-column label="驱动" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.is_vfio_bound" type="success" size="small">vfio-pci</el-tag>
          <el-tag v-else-if="row.driver_in_use" type="warning" size="small">{{ row.driver_in_use }}</el-tag>
          <el-tag v-else type="info" size="small">无驱动</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="占用" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.is_used_by_vm" type="danger" size="small">{{ row.used_by_vm_name }}</el-tag>
          <el-tag v-else type="success" size="small">空闲</el-tag>
        </template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button @click="passthroughDialogVisible = false">取消</el-button>
      <el-button type="primary" @click="confirmAddPassthroughDevices">添加选中设备</el-button>
    </template>
  </el-dialog>
</template>


<script setup>
import { ref, reactive, computed } from 'vue'
import { updateVm, getVmXML, updateVmXML, createVm, cloneVm, linkedCloneVm, batchCloneVm, getTemplateList, getOSVariants, getVmDetail, getDiskList, resizeDisk, removeDisk, changeDiskBus, attachDisk, changeCDROM, ejectCDROM, removeCDROM, adminImportDisk, adminImportDiskForVM, getPassthroughDevices, getVmPassthroughDevices, bindPCIDevice } from '@/api/vm'
import { getAllISOs, getVMStorageTargets } from '@/api/infra'
import { getUserISOs, selfCreateVm, importVM } from '@/api/storage'
import { getStorageFiles } from '@/api/storage'
import { selfCloneVm } from '@/api/user'
import { getVPCSecurityGroups, getVPCSwitches } from '@/api/vpc'
import { getCPUAffinityPresets, getSettings, getHostCPUCores } from '@/api/settings'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Top, Bottom, Delete, Plus, ArrowRight } from '@element-plus/icons-vue'
import FormIcons from '@/components/icons/FormIcons.vue'
import { useUserStore } from '@/store/user'
import { templateCategoryLabel, templateGroupLabel } from '@/utils/templateCategory'

const userStore = useUserStore()
const isAdmin = computed(() => userStore.role === 'admin')
const emit = defineEmits(['success', 'draft'])

const activeTabEdit = ref('basic')

const editTabs = [
  { name: 'basic', label: '基础配置', desc: 'CPU/内存', icon: 'Cpu' },
  { name: 'disk', label: '磁盘与光驱', desc: '存储管理', icon: 'Coin' },
  { name: 'security', label: '启动与安全', desc: '网络/引导', icon: 'Operation' },
  { name: 'hardware', label: '硬件直通', desc: 'PCI设备', icon: 'Monitor' },
  { name: 'advanced', label: '高级设置', desc: '开发者选项', icon: 'Setting' },
]

const visible = ref(false)
const registrationMode = ref(false)
const cpuAffinityPresets = ref([])
const cpuAffinityPresetsLoaded = ref(false)
const hostCPUCores = ref(0)
const registrationContext = reactive({
  dedicated_vpc_switch_id: 0,
  dedicated_vpc_label: ''
})

const isTemplateSourceMode = computed(() => form.create_mode === 'template' || form.create_mode === 'linked_clone')
const isLinkedCloneMode = computed(() => form.create_mode === 'linked_clone')

const useCase = ref('')
const showSmartRecommend = ref(false)

const osQuickOptions = [
  { value: 'linux', label: 'Linux', icon: '🐧', examples: 'Ubuntu / CentOS / Debian' },
  { value: 'windows', label: 'Windows', icon: '🪟', examples: 'Server 2022 / 2019 / 11' },
  { value: 'other', label: '其他', icon: '🖥️', examples: 'FreeBSD / 自定义' },
]

const osLabel = computed(() => {
  const map = { linux: '🐧 Linux', windows: '🪟 Windows', other: '🖥️ 其他' }
  return map[form.os_type] || form.os_type
})

const createModeLabel = computed(() => {
  if (registrationMode.value) return '轻量云注册'
  const map = { iso: 'ISO 镜像安装', template: '模板克隆', linked_clone: '原生链式克隆', import: '导入磁盘' }
  const base = map[form.create_mode] || form.create_mode
  if (isTemplateSourceMode.value) {
    const modeLabel = form.clone_mode === 'full' ? '（完整克隆）' : '（链式克隆）'
    return base + modeLabel
  }
  return base
})

const previewMemory = computed(() => isEdit.value ? (form.memory || 0) : (form.ram || 0))
const submitButtonText = computed(() => registrationMode.value ? '加入注册列表' : '创建虚拟机')

const allRequiredFilled = computed(() => {
  if (isEdit.value) return true
  const nameOk = /^[a-zA-Z0-9]+$/.test(form.name)
  const vcpuOk = form.vcpu > 0
  const ramOk = form.ram > 0
  if (!nameOk || !vcpuOk || !ramOk) return false
  if (!isAdmin.value && (!form.switch_id || !form.security_group_id)) return false
  if (form.create_mode === 'iso') {
    if (form.disk_size <= 0) return false
  } else if (form.create_mode === 'import') {
    if (isAdmin.value && form.disk_source_type === 'path') {
      if (!form.disk_path) return false
    } else {
      if (!form.disk_file) return false
    }
  } else if (isTemplateSourceMode.value) {
    if (!form.template || form.disk_size <= 0) return false
    if (!registrationMode.value && !isLinkedCloneMode.value && (!form.import_user || (!form.import_password && form.batch_count <= 1))) return false
    if (isFnOSTemplate.value && !isLinkedCloneMode.value && form.fnos_device_id_mode === 'custom' && !hasCustomFnOSDeviceID.value) return false
  }
  return true
})

const allRequiredTip = computed(() => {
  const missing = []
  if (!/^[a-zA-Z0-9]+$/.test(form.name)) missing.push('虚拟机名称')
  if (form.vcpu <= 0) missing.push('CPU 核心')
  if (form.ram <= 0) missing.push('内存')
  if (!isAdmin.value && (!form.switch_id || !form.security_group_id)) missing.push('VPC 交换机/安全组')
  if (form.create_mode === 'iso') {
    if (form.disk_size <= 0) missing.push('系统盘大小')
  } else if (form.create_mode === 'import') {
    if (isAdmin.value && form.disk_source_type === 'path') {
      if (!form.disk_path) missing.push('磁盘路径')
    } else {
      if (!form.disk_file) missing.push('磁盘文件')
    }
  } else if (isTemplateSourceMode.value) {
    if (!form.template) missing.push('模板')
    if (form.disk_size <= 0) missing.push('磁盘大小')
    if (!registrationMode.value && !isLinkedCloneMode.value) {
      if (!form.import_user) missing.push('用户名')
      if (!form.import_password && form.batch_count <= 1) missing.push('密码')
    }
    if (isFnOSTemplate.value && !isLinkedCloneMode.value && form.fnos_device_id_mode === 'custom' && !hasCustomFnOSDeviceID.value) {
      missing.push('FnOS 设备 ID')
    }
  }
  if (missing.length === 0) return ''
  return `以下必填项未完成：${missing.join('、')}`
})
const rtcConfigVisible = ref(false)
const guestAgentConfigVisible = ref(false)
const memoryDynamicConfigVisible = ref(false)
const memoryVirtioMemDetailVisible = ref(false)
const smbiosConfigVisible = ref(false)
const vmXMLConfigVisible = ref(false)
const vmXMLLoading = ref(false)
const vmXMLSaving = ref(false)
const vmXMLContent = ref('')
const vmXMLOriginal = ref('')
const isEdit = ref(false)
const formRef = ref(null)
const loading = ref(false)

// 引导式菜单处理
const createStep = ref(0)
const createSteps = computed(() => {
  if (registrationMode.value) {
    return [
      { title: '基础信息', name: 'basic', icon: 'InfoFilled' },
      { title: '硬件规格', name: 'hardware', icon: 'Cpu' },
      { title: '存储介质', name: 'storage', icon: 'Coin' },
      { title: '网络配额', name: 'network', icon: 'Connection' },
      { title: '系统配置', name: 'security', icon: 'Operation' },
      { title: '高级选项', name: 'advanced', icon: 'Setting' }
    ]
  }
  return [
    { title: '创建方式', name: 'mode', icon: 'Guide' },
    { title: '基础信息', name: 'basic', icon: 'InfoFilled' },
    { title: '硬件规格', name: 'hardware', icon: 'Cpu' },
    { title: '存储介质', name: 'storage', icon: 'Coin' },
    { title: '网络设置', name: 'network', icon: 'Connection' },
    { title: '系统配置', name: 'security', icon: 'Operation' },
    { title: '高级选项', name: 'advanced', icon: 'Setting' },
    ...(isAdmin.value ? [{ title: '硬件直通', name: 'passthrough', icon: 'Monitor' }] : [])
  ]
})
const maxStep = computed(() => createSteps.value.length - 1)
const advancedIntroDismissed = ref(false)
const advancedIntroStorageKey = computed(() => `vm-advanced-settings-intro-seen:${userStore.username || 'default'}`)
const isAdvancedSectionActive = computed(() => {
  if (isEdit.value) return activeTabEdit.value === 'advanced'
  return createSteps.value[createStep.value]?.name === 'advanced'
})
const shouldShowAdvancedIntro = computed(() => isAdvancedSectionActive.value && !advancedIntroDismissed.value)

const templateHostnamePattern = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/
const templateUsernamePattern = /^[a-z_][a-z0-9_-]{0,31}$/
const allowedPasswordPattern = /^[A-Za-z0-9!@#$%^&*_\-+=?]+$/
const strongPasswordMinLength = 12
const parseTemplateDiskSizeGB = (virtualSize) => {
  if (!virtualSize || typeof virtualSize !== 'string') {
    return 0
  }
  const matched = virtualSize.match(/([\d.]+)\s*GiB/i)
  if (!matched) {
    return 0
  }
  const size = Number.parseFloat(matched[1])
  if (!Number.isFinite(size) || size <= 0) {
    return 0
  }
  return Math.ceil(size)
}
const parsePositiveInt = (value) => {
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}
const resolveTemplateDefaultConfig = (tpl) => {
  if (!tpl || typeof tpl !== 'object' || !tpl.default_config || typeof tpl.default_config !== 'object') {
    return null
  }
  return tpl.default_config
}
const resolveTemplateDefaultVCPU = (tpl) => parsePositiveInt(resolveTemplateDefaultConfig(tpl)?.vcpu)
const resolveTemplateDefaultRAM = (tpl) => parsePositiveInt(resolveTemplateDefaultConfig(tpl)?.ram)
const resolveTemplateDefaultDiskSize = (tpl) => {
  const configured = parsePositiveInt(resolveTemplateDefaultConfig(tpl)?.disk_size)
  if (configured > 0) {
    return configured
  }
  return parseTemplateDiskSizeGB(tpl?.virtual_size)
}
const resolveTemplateDefaultDiskBus = (tpl) => {
  const bus = String(resolveTemplateDefaultConfig(tpl)?.disk_bus || '').trim().toLowerCase()
  return ['virtio', 'scsi', 'sata', 'ide'].includes(bus) ? bus : ''
}
const resolveTemplateDefaultNicModel = (tpl) => {
  const nicModel = String(resolveTemplateDefaultConfig(tpl)?.nic_model || '').trim().toLowerCase()
  return ['virtio', 'e1000e', 'rtl8139'].includes(nicModel) ? nicModel : ''
}
const resolveTemplateDefaultVideoModel = (tpl) => {
  const videoModel = String(resolveTemplateDefaultConfig(tpl)?.video_model || '').trim().toLowerCase()
  return ['virtio', 'vga', 'vmvga', 'cirrus'].includes(videoModel) ? videoModel : ''
}
const resolveTemplateDefaultCPUTopologyMode = (tpl) => {
  const mode = String(resolveTemplateDefaultConfig(tpl)?.cpu_topology_mode || '').trim().toLowerCase()
  return ['auto', 'single_socket', 'host_default'].includes(mode) ? mode : ''
}
const resolveTemplateDefaultFirstBootRebootMode = (tpl) => {
  const mode = String(resolveTemplateDefaultConfig(tpl)?.first_boot_reboot_mode || '').trim().toLowerCase()
  return ['normal', 'cold'].includes(mode) ? mode : ''
}
const isFnOSTemplate = computed(() => isTemplateSourceMode.value && form.template_type === 'fnos')
const selectedTemplate = computed(() => templates.value.find(tpl => tpl.name === form.template) || null)
const templateMinDiskSize = computed(() => parseTemplateDiskSizeGB(selectedTemplate.value?.virtual_size))
const windowsTemplateUsername = 'administrator'
const isWindowsTemplate = computed(() => isTemplateSourceMode.value && form.template_type === 'windows')
const bootTypePreviewLabel = computed(() => {
  switch (form.boot_type) {
    case 'bios':
      return 'BIOS'
    case 'uefi':
      return 'UEFI'
    case 'uefi-secure':
      return 'UEFI + 安全引导'
    default:
      return '自动'
  }
})
const isWindowsMemoryTarget = computed(() => {
  if (isEdit.value) return form.os_type === 'windows' || form.memory_backend === 'virtio_mem'
  if (isTemplateSourceMode.value) return form.template_type === 'windows'
  if (form.create_mode === 'import') return form.init_type === 'windows'
  return form.os_type === 'windows'
})
const showMemoryBackendQuickSelect = computed(() => form.memory_dynamic_enabled)
const windowsElasticMemoryDisabled = computed(() => !isWindowsMemoryTarget.value)
const templateDiskSizeTip = computed(() => {
  if (templateMinDiskSize.value > 0) {
    return `默认值为模板磁盘大小 ${templateMinDiskSize.value} GB，且不能小于该值`
  }
  return '选择模板后会自动带出模板磁盘大小，且不能小于模板原始磁盘大小'
})
const templateUserPlaceholder = computed(() => {
  if (isWindowsTemplate.value) {
    return windowsTemplateUsername
  }
  if (isFnOSTemplate.value) {
    return '请输入 fnOS 首次管理员用户名'
  }
  return '请输入克隆后的登录用户名'
})
const templateUserTip = computed(() => {
  if (isWindowsTemplate.value) {
    return 'Windows 模板默认使用 administrator 账号；Windows Server 请保持默认，不支持修改。'
  }
  if (isFnOSTemplate.value) {
    return 'fnOS 会将该账号离线注入为首次管理员账号，克隆完成后可直接在网页登录；仅支持小写字母、数字、下划线和短横线，且需以字母或下划线开头'
  }
  return '仅支持小写字母、数字、下划线和短横线，且需以字母或下划线开头'
})
const templatePasswordTip = computed(() => {
  const baseTip = '至少 12 位，需包含大写字母、小写字母、数字和符号（支持 !@#$%^&*_-+=?）'
  if (isFnOSTemplate.value) {
    return `fnOS 会将该密码作为首次管理员网页登录密码；${baseTip}`
  }
  return baseTip
})
const normalizedFnOSDeviceID = computed(() => `${form.fnos_device_id || ''}`.trim())
const hasCustomFnOSDeviceID = computed(() => isFnOSTemplate.value && !isLinkedCloneMode.value && fnosDeviceIdPattern.test(normalizedFnOSDeviceID.value))
const shouldPreserveFnOSDeviceID = computed(() => {
  if (!isFnOSTemplate.value || isLinkedCloneMode.value) return false
  return form.fnos_device_id_mode === 'preserve' || form.fnos_device_id_mode === 'custom' || hasCustomFnOSDeviceID.value
})
const vmNameCharset = 'abcdefghijklmnopqrstuvwxyz0123456789'
const hostnameCharset = 'abcdefghijklmnopqrstuvwxyz0123456789'
const uppercaseCharset = 'ABCDEFGHJKLMNPQRSTUVWXYZ'
const lowercaseCharset = 'abcdefghijkmnopqrstuvwxyz'
const digitCharset = '23456789'
const symbolCharset = '!@#$%^&*_-+=?'
const fnosDeviceIdPattern = /^[0-9a-fA-F]{32}([0-9a-fA-F]{8})?$/
const createEmptyGuestAgentConfig = () => ({
  enabled: false,
})
const createEmptySMBIOS1Config = () => ({
  base64: false,
  family: '',
  manufacturer: '',
  product: '',
  serial: '',
  sku: '',
  uuid: '',
  version: '',
})
const freezeHelpText = '虚拟机启动时自动冻结CPU（使用监视器命令c可继续启动过程）。宿主机原生自启不会附带该行为。'
const apicHelpText = '控制虚拟机是否向来宾系统暴露 APIC（高级可编程中断控制器）。绝大多数系统都应保持启用。'
const paeHelpText = '控制虚拟机是否暴露 PAE（Physical Address Extension，物理地址扩展）。常见于 x86 老系统、32 位大内存兼容场景。'

// 当前 PCIe 端口数（编辑模式从 VM 详情中读取）
const currentPCIERootPorts = computed(() => {
  if (!isEdit.value || !currentEditRow.value) return '未知'
  return currentEditRow.value?.pcie_root_ports ?? '未知'
})
const cpuTopologyModeOptions = [
  { label: '自动（Windows 使用单插槽多核心）', value: 'auto' },
  { label: '单插槽多核心', value: 'single_socket' },
  { label: '宿主默认拓扑', value: 'host_default' },
]
const firstBootRebootModeOptions = [
  { label: '普通重启', value: 'normal' },
  { label: '宿主冷启动', value: 'cold' },
]
const rtcHelpText = '控制虚拟机 RTC 硬件时钟使用 UTC 还是本地时间。Linux 通常建议使用 UTC，Windows 默认建议使用本地时间。'
const rtcStartDateHelpText = '默认使用 now。若填写固定时间，后端会将其转换为固定起始时间模式，支持 RFC3339、Unix 时间戳、YYYY-MM-DD HH:mm:ss 等格式。'
const guestAgentHelpText = '启用后会在虚拟机定义中添加 QEMU Guest Agent 通道，便于宿主机读取 VM 内部信息、做更可靠的关机与文件系统冻结协作。'
const smbiosBase64HelpText = '仅在需要兼容特殊字符或沿用外部配置时启用。开启后，厂商、产品、版本、序列号、SKU、家族名称、UUID 字段会先按 Base64 解码。'
const smbiosUUIDHelpText = 'libvirt 要求 SMBIOS UUID 与虚拟机 UUID 保持一致。新建时可显式指定，编辑已有虚拟机时建议保持当前值。'
const getRecommendedRTCOffset = (guestType) => guestType === 'windows' ? 'localtime' : 'utc'
const applyRTCOffsetRecommendation = (guestType) => {
  form.rtc_offset = getRecommendedRTCOffset(guestType)
}
const normalizeRTCOffsetForForm = (value) => value === 'localtime' ? 'localtime' : 'utc'
const normalizeRTCStartDate = (value) => {
  const normalized = `${value || ''}`.trim()
  return normalized || 'now'
}
const normalizeSMBIOS1Value = (value) => `${value || ''}`.trim()
const normalizeVmXMLText = (value) => `${value || ''}`.replace(/\r\n/g, '\n').trim()
const currentVmUUID = ref('')
const rtcConfigSummary = computed(() => {
  const offsetText = form.rtc_offset === 'localtime' ? '本地时间' : 'UTC'
  const startDateText = normalizeRTCStartDate(form.rtc_startdate)
  return `时间基准：${offsetText} / 开始日期：${startDateText}`
})
const guestAgentConfigSummary = computed(() => {
  return form.guest_agent.enabled
    ? '已启用，需虚拟机内安装 qemu-guest-agent'
    : '默认（已禁用）'
})
const memoryDynamicSummary = computed(() => {
  if (!form.memory_dynamic_enabled) {
    if (form.memory_compat_mode === 'legacy_static') return '静态兼容模式，可由管理员启用'
    return '默认（已关闭）'
  }
  if (form.memory_backend === 'virtio_mem') {
    const status = form.memory_pending_apply ? '待下次启动应用' : '实验'
    return `${status} / 基础 ${form.memory_initial}GB / 最大 ${form.memory_max_dynamic}GB`
  }
  const status = form.memory_pending_apply ? '待下次启动应用' : '已启用'
  return `${status} / 启动 ${form.memory_initial}GB / 最小 ${form.memory_min}GB / 最大 ${form.memory_max_dynamic}GB`
})
const memoryBackendTip = computed(() => {
  if (form.memory_backend === 'virtio_mem') {
    return '实验功能：主表单内存作为规格内存，基础内存自动按 50% 计算，最大上限默认上浮 30%；运行后按 70%/50% 阈值自动伸缩。'
  }
  return '默认模式：基于 virtio-balloon 调整当前内存，Linux 可配合 free page reporting 回收空闲页。'
})
const basicMemoryDynamicTip = computed(() => {
  if (form.memory_backend === 'virtio_mem') {
    return '弹性内存：设定内存作为规格内存，基础内存自动按 50% 计算，并额外提供 30% 突发上限；运行后按使用率自动伸缩。'
  }
  return '开启后以设定内存作为启动/保障内存，最大内存默认上浮 30% 应对突发；宿主机内存紧张时可按策略回收。'
})
const memoryCompatLabel = computed(() => {
  const map = {
    legacy_static: '静态兼容',
    dynamic: '动态内存',
    pending_apply: '待迁移应用',
  }
  return map[form.memory_compat_mode] || '未识别'
})
const memoryCompatTagType = computed(() => {
  if (form.memory_compat_mode === 'dynamic') return 'success'
  if (form.memory_compat_mode === 'pending_apply') return 'warning'
  return 'info'
})
const memoryBalloonStatusText = computed(() => {
  const map = {
    ok: '气球统计正常，自动调度可使用。',
    no_stats: '未获取到气球统计，可能需要来宾系统驱动或等待统计上报。',
    not_running: '虚拟机未运行，启动后才能读取实时气球统计。',
    missing_balloon: '缺少气球设备，运行中不能热兼容，需关机后应用。',
    pending_apply: '配置已保存，等待下次关机后启动时应用。',
  }
  return map[form.memory_balloon_status] || '暂无状态'
})
const smbiosConfigSummary = computed(() => {
  const parts = []
  if (normalizeSMBIOS1Value(form.smbios1.manufacturer)) parts.push(`厂商：${normalizeSMBIOS1Value(form.smbios1.manufacturer)}`)
  if (normalizeSMBIOS1Value(form.smbios1.product)) parts.push(`产品：${normalizeSMBIOS1Value(form.smbios1.product)}`)
  if (normalizeSMBIOS1Value(form.smbios1.serial)) parts.push(`序列号：${normalizeSMBIOS1Value(form.smbios1.serial)}`)
  if (normalizeSMBIOS1Value(form.smbios1.uuid)) parts.push(`UUID：${normalizeSMBIOS1Value(form.smbios1.uuid)}`)
  if (parts.length === 0) {
    if (isEdit.value && currentVmUUID.value) {
      return `未额外配置 / 当前 UUID：${currentVmUUID.value}`
    }
    return '未配置，保持系统默认'
  }
  const summary = parts.slice(0, 3).join(' / ')
  return form.smbios1.base64 ? `${summary} / Base64 解码写入` : summary
})
const vmXMLConfigSummary = computed(() => {
  if (!isEdit.value) return '仅支持已有虚拟机'
  if (editVmStatus.value === 'running' || editVmStatus.value === 'paused') {
    return '运行中保存后通常需重启生效 / 不支持改名'
  }
  return '直接查看并编辑持久化 XML / 不支持改名'
})
const vmXMLDirty = computed(() => normalizeVmXMLText(vmXMLContent.value) !== normalizeVmXMLText(vmXMLOriginal.value))

const smartRecommendDesc = computed(() => {
  const osLabelName = form.os_type === 'windows' ? 'Windows' : 'Linux'
  let cpu = 2, mem = 2, disk = 20, driver = 'VirtIO'
  if (form.os_type === 'windows') { cpu = 4; mem = 4; disk = 40; driver = 'SATA/e1000e' }
  if (useCase.value === 'database') { cpu += 2; mem += 4; disk += 40 }
  else if (useCase.value === 'ai') { cpu += 4; mem += 8; disk += 80 }
  else if (useCase.value === 'web') { cpu += 1; mem += 2; disk += 10 }
  return `根据${osLabelName}系统${useCase.value ? '和应用场景' : ''}，推荐配置：CPU ${cpu} 核、内存 ${mem}GB、系统盘 ${disk}GB、${driver} 驱动`
})

const openRTCConfigDialog = () => {
  rtcConfigVisible.value = true
}

const openGuestAgentConfigDialog = () => {
  guestAgentConfigVisible.value = true
}

const openMemoryDynamicConfigDialog = () => {
  ensureMemoryDynamicDefaults()
  memoryDynamicConfigVisible.value = true
}

const openSMBIOSConfigDialog = () => {
  smbiosConfigVisible.value = true
}

const currentEditRow = ref(null)

const openVmXMLConfigDialog = async () => {
  if (!isEdit.value || !form.name) return
  vmXMLConfigVisible.value = true
  await loadVmXMLContent()
}

const loadVmXMLContent = async (showSuccess = false) => {
  if (!form.name) return
  vmXMLLoading.value = true
  try {
    const res = await getVmXML(form.name)
    const xmlContent = res.data?.xml || ''
    vmXMLContent.value = xmlContent
    vmXMLOriginal.value = xmlContent
    if (showSuccess) {
      ElMessage.success('XML 已重新加载')
    }
  } finally {
    vmXMLLoading.value = false
  }
}

const reloadVmXMLContent = async () => {
  await loadVmXMLContent(true)
}

const markMemoryDynamicTouched = () => {
  form.memory_dynamic_touched = true
}

const getRecommendedMemoryMax = (base) => Math.max(base, Math.ceil(base * 1.3))
const getRecommendedElasticMemoryInitial = (spec) => Math.max(1, Math.floor(spec / 2))
const getCurrentBaseMemory = () => isEdit.value ? (form.memory || form.ram || 1) : (form.ram || 1)
const getElasticMemorySpecFromConfig = (initial, maxDynamic, fallback) => {
  const initialSpec = initial > 0 ? initial * 2 : 0
  const maxSpec = maxDynamic > 0 ? Math.max(1, Math.floor(maxDynamic * 10 / 13)) : 0
  return Math.max(1, initialSpec, maxSpec, fallback || 0)
}

const applyRecommendedMemoryDynamicValues = (spec = getCurrentBaseMemory()) => {
  if (form.memory_backend === 'virtio_mem') {
    form.memory_initial = getRecommendedElasticMemoryInitial(spec)
    form.memory_min = form.memory_initial
    form.memory_max_dynamic = getRecommendedMemoryMax(spec)
    form.memory_auto_balloon = false
  } else {
    form.memory_initial = spec
    form.memory_min = Math.max(1, Math.floor(spec / 2))
    form.memory_max_dynamic = getRecommendedMemoryMax(spec)
    form.memory_auto_balloon = true
  }
}

const handleDynamicMemoryEnabledChange = () => {
  if (form.memory_dynamic_enabled) {
    applyRecommendedMemoryDynamicValues()
  }
  markMemoryDynamicTouched()
}

// CPU 热添加开关状态变化
const handleCPUHotplugChange = () => {
  // 启用时不做额外处理，提交时 max_vcpu 将设为 hostCPUCores
  // 关闭时 max_vcpu 为 0（不启用热添加）
}

const cpuHotplugMaxVCPU = computed(() => {
  return form.cpu_hotplug_enabled && hostCPUCores.value > 0 ? hostCPUCores.value : 0
})

const handleMemoryBackendChange = () => {
  applyRecommendedMemoryDynamicValues()
  markMemoryDynamicTouched()
}

const normalizeMemoryBackendForGuest = () => {
  if (!isWindowsMemoryTarget.value && form.memory_backend === 'virtio_mem') {
    form.memory_backend = 'balloon'
    applyRecommendedMemoryDynamicValues()
  }
}

const handleBaseMemoryChange = () => {
  if (!form.memory_dynamic_enabled) return
  applyRecommendedMemoryDynamicValues()
  markMemoryDynamicTouched()
}

const ensureMemoryDynamicDefaults = () => {
  const base = getCurrentBaseMemory()
  if (!form.memory_initial || form.memory_initial < 1) form.memory_initial = base
  if (form.memory_backend === 'virtio_mem') {
    form.memory_initial = getRecommendedElasticMemoryInitial(base)
    form.memory_min = form.memory_initial
  } else if (!form.memory_min || form.memory_min < 1) {
    form.memory_min = Math.max(1, Math.floor(form.memory_initial / 2))
  }
  const recommendedMax = form.memory_backend === 'virtio_mem' ? getRecommendedMemoryMax(base) : getRecommendedMemoryMax(form.memory_initial)
  if (isEdit.value && !form.memory_dynamic_enabled && form.memory_compat_mode === 'legacy_static' && form.memory_max_dynamic <= base) {
    form.memory_max_dynamic = recommendedMax
  }
  if (!form.memory_max_dynamic || form.memory_max_dynamic < form.memory_initial) form.memory_max_dynamic = recommendedMax
}

const resetMemoryDynamicDefaults = () => {
  applyRecommendedMemoryDynamicValues()
  markMemoryDynamicTouched()
}

const buildGuestAgentPayload = () => ({
  enabled: !!form.guest_agent.enabled,
})

const buildMemoryDynamicPayload = () => {
  if (!isEdit.value && !form.memory_dynamic_enabled) return undefined
  if (isEdit.value && !form.memory_dynamic_touched) return undefined
  if (!form.memory_dynamic_enabled) {
    return {
      dynamic_enabled: false,
      memory_backend: form.memory_backend || 'balloon',
      memory_initial: getCurrentBaseMemory(),
    }
  }
  return {
    dynamic_enabled: !!form.memory_dynamic_enabled,
    memory_backend: form.memory_backend || 'balloon',
    memory_initial: form.memory_initial,
    memory_min: form.memory_backend === 'virtio_mem' ? form.memory_initial : form.memory_min,
    memory_max: form.memory_max_dynamic,
    memory_auto_balloon: form.memory_backend === 'virtio_mem' ? false : !!form.memory_auto_balloon,
    memory_current: form.memory_current || 0,
  }
}

const buildCPULimitPercentPayload = () => {
  if (!isAdmin.value) return undefined
  if (!form.cpu_limit_enabled) return 0
  const value = Number(form.cpu_limit_percent) || 0
  return Math.min(Math.max(value, 1), 100)
}

const validateCPUAffinityInput = (value) => {
  if (!value || !value.trim()) return true
  const trimmed = value.trim()
  // 支持数字、空格、逗号、连字符
  if (!/^[0-9,\s-]+$/.test(trimmed)) {
    return false
  }
  return true
}

const buildCPUAffinityPayload = () => {
  if (!isAdmin.value) return ''
  const trimmed = (form.cpu_affinity || '').trim()
  if (!trimmed) return ''
  if (!validateCPUAffinityInput(trimmed)) {
    ElMessage.warning('CPU 亲和性格式不正确，请使用数字、逗号、空格或连字符（如 0,2,4 或 0-3）')
    return null
  }
  return trimmed
}

const normalizeAPICForForm = (value) => value !== false
const normalizePAEForForm = (value) => value !== false

const buildSMBIOS1Payload = () => ({
  base64: !!form.smbios1.base64,
  family: normalizeSMBIOS1Value(form.smbios1.family),
  manufacturer: normalizeSMBIOS1Value(form.smbios1.manufacturer),
  product: normalizeSMBIOS1Value(form.smbios1.product),
  serial: normalizeSMBIOS1Value(form.smbios1.serial),
  sku: normalizeSMBIOS1Value(form.smbios1.sku),
  uuid: normalizeSMBIOS1Value(form.smbios1.uuid),
  version: normalizeSMBIOS1Value(form.smbios1.version),
})

const resetSMBIOS1Config = () => {
  Object.assign(form.smbios1, createEmptySMBIOS1Config())
}

const initAdvancedIntro = () => {
  advancedIntroDismissed.value = localStorage.getItem(advancedIntroStorageKey.value) === '1'
}

const dismissAdvancedIntro = () => {
  advancedIntroDismissed.value = true
  localStorage.setItem(advancedIntroStorageKey.value, '1')
}

// 使用浏览器安全随机数生成表单默认值和密码
const getRandomInt = (max) => {
  if (max <= 0) return 0
  const cryptoApi = globalThis.crypto
  if (cryptoApi && typeof cryptoApi.getRandomValues === 'function') {
    const randomValues = new Uint32Array(1)
    cryptoApi.getRandomValues(randomValues)
    return randomValues[0] % max
  }
  return Math.floor(Math.random() * max)
}

const randomCharFrom = (charset) => charset[getRandomInt(charset.length)]

const randomStringFrom = (charset, length) => Array.from({ length }, () => randomCharFrom(charset)).join('')

const shuffleChars = (chars) => {
  for (let index = chars.length - 1; index > 0; index--) {
    const swapIndex = getRandomInt(index + 1)
    ;[chars[index], chars[swapIndex]] = [chars[swapIndex], chars[index]]
  }
  return chars
}

const generateRandomVmName = () => `vm${randomStringFrom(vmNameCharset, 8)}`
const generateRandomHostname = () => `vm-${randomStringFrom(hostnameCharset, 8)}`

const generateStrongPassword = () => {
  const passwordChars = [
    randomCharFrom(uppercaseCharset),
    randomCharFrom(lowercaseCharset),
    randomCharFrom(digitCharset),
  ]
  const baseCharset = uppercaseCharset + lowercaseCharset + digitCharset
  while (passwordChars.length < strongPasswordMinLength - 1) {
    passwordChars.push(randomCharFrom(baseCharset))
  }
  return `${shuffleChars(passwordChars).join('')}${randomCharFrom(symbolCharset)}`
}

const validateTemplateHostname = (_, value, callback) => {
  if (form.create_mode !== 'template') {
    callback()
    return
  }
  if (!value) {
    callback()
    return
  }
  if (!templateHostnamePattern.test(value)) {
    callback(new Error('主机名只能包含字母、数字和短横线，且不能以短横线开头或结尾'))
    return
  }
  callback()
}

const validateTemplateUsername = (_, value, callback) => {
  if (form.create_mode !== 'template') {
    callback()
    return
  }
  const normalizedValue = String(value || '').trim()
  if (isWindowsTemplate.value) {
    if (normalizedValue !== windowsTemplateUsername) {
      callback(new Error('Windows 模板用户名固定为 administrator，不支持修改'))
      return
    }
    callback()
    return
  }
  if (!normalizedValue) {
    callback(new Error('请输入用户名'))
    return
  }
  if (!templateUsernamePattern.test(normalizedValue)) {
    callback(new Error('用户名只能以小写字母或下划线开头，且只能包含小写字母、数字、下划线和短横线'))
    return
  }
  callback()
}

const validateTemplatePassword = (_, value, callback) => {
  if (form.create_mode !== 'template') {
    callback()
    return
  }
  if (!value) {
    if (form.batch_count > 1) {
      callback()
      return
    }
    callback(new Error('请输入密码'))
    return
  }
  if (value.length < strongPasswordMinLength) {
    callback(new Error(`密码长度不能少于 ${strongPasswordMinLength} 位`))
    return
  }
  if (!allowedPasswordPattern.test(value)) {
    callback(new Error('密码只能包含字母、数字和 !@#$%^&*_-+=? 符号'))
    return
  }
  if (!/[A-Z]/.test(value) || !/[a-z]/.test(value) || !/[0-9]/.test(value) || !/[!@#$%^&*_\-+=?]/.test(value)) {
    callback(new Error('密码必须同时包含大写字母、小写字母、数字和符号'))
    return
  }
  callback()
}

const handleGenerateTemplateHostname = async () => {
  form.hostname = generateRandomHostname()
  await formRef.value?.validateField('hostname').catch(() => false)
}

const handleGenerateVmName = async () => {
  form.name = generateRandomVmName()
  await formRef.value?.validateField('name').catch(() => false)
}

const handleGenerateTemplatePassword = async () => {
  form.import_password = generateStrongPassword()
  await formRef.value?.validateField('import_password').catch(() => false)
}

const resolveTemplateBootTypeForForm = (tpl) => {
  if (!tpl) {
    return ''
  }
  const bootType = String(tpl.boot_type || '').trim().toLowerCase()
  if (bootType === 'uefi') {
    return tpl.type === 'windows' ? 'uefi-secure' : 'uefi'
  }
  if (bootType === 'bios') {
    return 'bios'
  }
  return ''
}

const applySelectedTemplateSettings = (tpl, options = {}) => {
  if (!tpl) {
    return
  }
  const applyProfile = options.applyProfile !== false
  form.template_type = tpl.type || ''
  if (form.template_type !== 'fnos') {
    form.preserve_fnos_device_id = false
    form.fnos_device_id_mode = 'regenerate'
    form.fnos_device_id = ''
  }
  form.machine_type = 'q35'
  const templateBootType = resolveTemplateBootTypeForForm(tpl)
  if (templateBootType) {
    form.boot_type = templateBootType
  }
  if (!applyProfile) {
    return
  }
  const defaultVCPU = resolveTemplateDefaultVCPU(tpl)
  if (defaultVCPU > 0) {
    form.vcpu = defaultVCPU
  }
  const defaultRAM = resolveTemplateDefaultRAM(tpl)
  if (defaultRAM > 0) {
    form.ram = defaultRAM
  }
  const defaultDiskSize = resolveTemplateDefaultDiskSize(tpl)
  if (defaultDiskSize > 0) {
    form.disk_size = defaultDiskSize
  }
  const defaultDiskBus = resolveTemplateDefaultDiskBus(tpl)
  if (defaultDiskBus) {
    form.disk_bus = defaultDiskBus
  }
  const defaultNicModel = resolveTemplateDefaultNicModel(tpl)
  if (defaultNicModel) {
    form.nic_model = defaultNicModel
  }
  const defaultVideoModel = resolveTemplateDefaultVideoModel(tpl)
  if (defaultVideoModel) {
    form.video_model = defaultVideoModel
  }
  const defaultCPUTopologyMode = resolveTemplateDefaultCPUTopologyMode(tpl)
  if (defaultCPUTopologyMode) {
    form.cpu_topology_mode = defaultCPUTopologyMode
  }
  const defaultFirstBootRebootMode = resolveTemplateDefaultFirstBootRebootMode(tpl)
  if (defaultFirstBootRebootMode) {
    form.first_boot_reboot_mode = defaultFirstBootRebootMode
  }
}

const ensureTemplateDefaults = () => {
  if (form.create_mode === 'template' && !form.hostname) {
    form.hostname = generateRandomHostname()
  }
  if (isTemplateSourceMode.value && isWindowsTemplate.value) {
    form.import_user = windowsTemplateUsername
  } else {
    form.first_boot_reboot_mode = 'normal'
  }
  if (isTemplateSourceMode.value && templateMinDiskSize.value > 0 && form.disk_size < templateMinDiskSize.value) {
    form.disk_size = templateMinDiskSize.value
  }
}

const selectMode = (mode) => {
  form.create_mode = mode
  onCreateModeChange(mode)
  createStep.value++
}

const goToCreateStep = (index) => {
  createStep.value = index
}

const onOsQuickSelect = (os) => {
  form.os_type = os
  onOsTypeChange()
  showSmartRecommend.value = true
}

const onUseCaseChange = () => {
  if (useCase.value) {
    showSmartRecommend.value = true
  }
}

const applySmartRecommend = () => {
  let cpu = 2, mem = 2, disk = 20
  if (form.os_type === 'windows') { cpu = 4; mem = 4; disk = 40 }
  if (useCase.value === 'database') { cpu += 2; mem += 4; disk += 40 }
  else if (useCase.value === 'ai') { cpu += 4; mem += 8; disk += 80 }
  else if (useCase.value === 'web') { cpu += 1; mem += 2; disk += 10 }
  form.vcpu = cpu
  if (form.create_mode === 'import') form.ram = mem
  else form.ram = mem
  if (form.create_mode === 'iso') form.disk_size = disk
  showSmartRecommend.value = false
}

const nextStep = async () => {
  if (!formRef.value) return
  
  let isValid = true
  const currentStepName = createSteps.value[createStep.value]?.name
  
  if (currentStepName === 'basic') {
    const fieldsToValidate = ['name']
    if (isTemplateSourceMode.value) {
      fieldsToValidate.push('template', 'disk_size')
      if (!registrationMode.value && !isLinkedCloneMode.value) {
        fieldsToValidate.push('hostname')
        fieldsToValidate.push('import_user', 'import_password')
      } else if (registrationMode.value) {
        fieldsToValidate.push('hostname')
      }
      if (isFnOSTemplate.value && !isLinkedCloneMode.value && form.fnos_device_id_mode === 'custom') {
        fieldsToValidate.push('fnos_device_id')
      }
    }
    try {
      await formRef.value.validateField(fieldsToValidate)
    } catch (err) {
      isValid = false
    }
  }

  if (currentStepName === 'hardware') {
    const fieldsToValidate = ['vcpu', 'ram']
    try {
      await formRef.value.validateField(fieldsToValidate)
    } catch (err) {
      isValid = false
    }
  }

  if (currentStepName === 'storage') {
    const fieldsToValidate = []
    if (form.create_mode === 'iso') {
      fieldsToValidate.push('disk_size')
    } else if (form.create_mode === 'import') {
      fieldsToValidate.push('disk_file')
    }
    if (fieldsToValidate.length > 0) {
      try {
        await formRef.value.validateField(fieldsToValidate)
      } catch (err) {
        isValid = false
      }
    }
  }

  if (currentStepName === 'network') {
    let fieldsToValidate = []
    if (!isAdmin.value) {
      fieldsToValidate = ['switch_id', 'security_group_id']
    }
    if (fieldsToValidate.length > 0) {
      try {
        await formRef.value.validateField(fieldsToValidate)
      } catch (err) {
        isValid = false
      }
    }
  }

  if (isValid && createStep.value < maxStep.value) {
    createStep.value++
  }
}

const prevStep = () => {
  if (createStep.value > 0) {
    createStep.value--
  }
}

// 数据列表
const osVariants = ref([])
const isoList = ref([])
const isoStorageDir = ref('/var/lib/libvirt/images/ISO')
const templates = ref([])
const vpcSwitches = ref([])
const vpcSecurityGroups = ref([])
const selectedVPCSwitch = computed(() => vpcSwitches.value.find(item => item.id === form.switch_id))
// 额外网口（仅管理员多网口功能）
const extraNics = ref([])
const addExtraNic = () => {
  extraNics.value.push({
    nic_model: form.nic_model || 'virtio',
    switch_id: vpcSwitches.value.length > 0 ? vpcSwitches.value[0].id : null,
    security_group_id: null
  })
}
const removeExtraNic = (index) => {
  extraNics.value.splice(index, 1)
}
const switchOptionLabelNic = (item) => {
  const prefix = isAdmin.value && item.username ? `${item.username} / ` : ''
  if (item.bridge_mode === 'bridge') {
    const vlan = item.bridge_vlan_id > 0 ? `，VLAN ${item.bridge_vlan_id}` : ''
    return `${prefix}${item.name}（桥接直通：${item.bridge_name || '-'}${vlan}）`
  }
  return `${prefix}${item.name} (${item.cidr})`
}
const getExtraNicSwitch = (nic) => vpcSwitches.value.find(item => item.id === nic.switch_id) || null

// 构建额外网口提交数据
const buildExtraNicsPayload = () => {
  return extraNics.value
    .filter(n => n.switch_id)
    .map(n => ({
      switch_id: n.switch_id,
      security_group_id: n.security_group_id || 0,
      nic_model: n.nic_model || 'virtio'
    }))
}
const storageTargets = ref([])
const storageTargetsLoading = ref(false)
const selectedStorageTargetLabel = computed(() => {
  const target = storageTargets.value.find(item => item.id === form.storage_pool_id)
  if (!target) return '默认存储位置'
  return target.display_name || target.name || target.id
})

// 编辑模式数据
const editVmStatus = ref('shut off')
const editVmVNC = ref('')
const editDisks = ref([])
const editCdroms = ref([])  // [{device, path}]
const cdromIsoPath = ref('')
const editOrigNicModel = ref('')  // 编辑模式下原始网卡类型，用于判断是否已修改
const editOrigBootType = ref('')  // 编辑模式下原始引导方式，用于判断是否已修改
const editOrigPCIERootPorts = ref(0)  // 编辑模式下原始 PCIe 端口数，用于判断是否已修改
const editOrigVcpu = ref(1)  // 编辑模式下原始 CPU 核心数，运行态下限制不可减少
const editOrigMemory = ref(1)  // 编辑模式下原始内存(GB)，运行态下限制不可减少
const editBootDevices = ref([])   // 编辑模式下的可引导设备列表（Cockpit 风格）
const bootTypeTouched = ref(false) // 仅在用户手动切换引导类型后置为 true

// 运行态下 CPU/内存最小值为原始值，禁止减少
const vcpuMin = computed(() => (isEdit.value && editVmStatus.value === 'running') ? editOrigVcpu.value : 1)
const memoryMin = computed(() => (isEdit.value && editVmStatus.value === 'running') ? editOrigMemory.value : 1)

// 挂载已有磁盘对话框
const attachDiskVisible = ref(false)
const attachDiskPath = ref('')
const attachDiskBus = ref('virtio')
const attachDiskFiles = ref([])
const attachDiskLoading = ref(false)
const attachDiskSubmitting = ref(false)
// 管理员绝对路径导入模式
const attachDiskSourceType = ref('storage')
const attachDiskAbsolutePath = ref('')
const attachDiskStoragePoolId = ref('')
const attachDiskCopyDisk = ref(false)

const attachDiskSubmitDisabled = computed(() => {
  if (isAdmin.value && attachDiskSourceType.value === 'path') {
    return !attachDiskAbsolutePath.value
  }
  return !attachDiskPath.value
})

const form = reactive({
  name: '',
  remark: '',
  vcpu: 2,
  memory: 2,
  ram: 2,
  disk_size: 20,
  // 创建模式
  create_mode: 'iso',  // iso / import
  // 克隆模式（仅在 template / linked_clone 模式下生效）
  clone_mode: 'linked',  // linked（链式克隆，默认）/ full（完整克隆）
  // 普通创建模式字段
  os_type: 'linux',
  os_variant: '',
  disk_format: 'qcow2',
  disk_bus: 'virtio',
  system_disk_iops_total: 0,
  system_disk_iops_read: 0,
  system_disk_iops_write: 0,
  iso_path: '',
  iso_paths: [],
  switch_id: null,
  security_group_id: null,
  nic_model: 'virtio',
  video_model: 'virtio',
  cpu_topology_mode: 'auto',
  cpu_hotplug_enabled: false,
  first_boot_reboot_mode: 'normal',
  autostart: false,
  freeze: false,
  apic: true,
  pae: true,
  rtc_offset: 'utc',
  rtc_startdate: 'now',
  guest_agent: createEmptyGuestAgentConfig(),
  smbios1: createEmptySMBIOS1Config(),
  memory_dynamic_enabled: false,
  memory_backend: 'balloon',
  memory_initial: 2,
  memory_min: 1,
  memory_max_dynamic: 3,
  memory_auto_balloon: true,
  memory_current: 0,
  memory_virtio_mem_current: 0,
  cpu_limit_enabled: false,
  cpu_limit_percent: 100,
  cpu_affinity: '',   // CPU 亲和性，如 "0,2,4"
  memory_dynamic_touched: false,
  memory_pending_apply: false,
  memory_compat_mode: 'legacy_static',
  memory_balloon_supported: false,
  memory_balloon_status: 'not_running',
  // 新增字段
  machine_type: 'q35',
  boot_type: 'bios',
  watchdog: 'none',
  boot_order: ['hd'],
  // PCIe 热插槽数量（q35 机型预留的 pcie-root-port 数量，0 使用默认 4）
  pcie_root_ports: 4,
  // 虚拟化方案
  virt_type: 'kvm',
  arch: 'x86_64',
  // 编辑模式 - 新增磁盘
  add_disks: [],
  // 创建模式 - 额外磁盘
  extra_disks: [],
  // 硬件直通设备
  host_devices: [],
  host_devices_touched: false,
  // 导入模式字段
  disk_file: '',
  disk_path: '',
  disk_source_type: 'storage',  // 'storage'(从存储选择) / 'path'(绝对路径)
  copy_disk: false,
  init_type: '',
  start_after_import: true,  // 导入完成后是否开启虚拟机
  extra_import_disks: [],  // 额外导入磁盘列表
  hostname: '',
  import_user: '',
  import_password: '',
  template_root_pass: '',
  template_user: '',
  // 模板模式字段
  template: '',
  template_type: '',
  preserve_fnos_device_id: false,
  fnos_device_id_mode: 'regenerate',
  fnos_device_id: '',
  storage_pool_id: '',
  traffic_down_gb: 0,
  traffic_up_gb: 0,
  bandwidth_down_mbps: 0,
  bandwidth_up_mbps: 0,
  max_port_forwards: 10,
  max_runtime_hours: 0,
  batch_count: 1,       // 批量创建数量（仅模板克隆模式）
})

const getRecommendedVideoModel = (osType) => {
  return osType === 'windows' ? 'vga' : 'virtio'
}

const applyVideoModelRecommendation = (osType) => {
  form.video_model = getRecommendedVideoModel(osType)
}

const applyBootTypeRecommendation = (bootType, options = {}) => {
  const { force = false } = options
  if (!bootType) {
    return
  }
  if (!force && bootTypeTouched.value) {
    return
  }
  form.boot_type = bootType
}

// 磁盘文件列表（导入模式用）
const diskFileList = ref([])
const diskFilesLoading = ref(false)

// 对话框标题
const dialogTitle = computed(() => {
  if (isEdit.value) return '编辑虚拟机'
  if (registrationMode.value) return '注册轻量云 VM'
  return form.create_mode === 'import' ? '导入虚拟机' : '新建虚拟机'
})

// 启动设备定义
const allBootDevices = [
  { value: 'hd', label: '硬盘', icon: 'Coin' },
  { value: 'cdrom', label: '光驱 (CD-ROM)', icon: 'Disc' },
  { value: 'network', label: '网络 (PXE)', icon: 'Connection' },
]

const videoModelOptions = [
  { value: 'virtio', label: 'VirtIO（高性能）', tag: '推荐', tagType: 'success' },
  { value: 'vga', label: 'VGA（兼容模式）', tag: '兼容', tagType: 'warning' },
  { value: 'vmvga', label: 'VMVGA（VMware 嵌套）', tag: '嵌套', tagType: 'primary' },
  { value: 'cirrus', label: 'Cirrus（保守排障）', tag: '排障', tagType: 'info' }
]

// 可添加的启动设备（已添加的排除）
const availableBootDevices = computed(() => {
  return allBootDevices.filter(d => !form.boot_order.includes(d.value))
})

const bootOrderLabel = (value) => {
  const found = allBootDevices.find(d => d.value === value)
  return found ? found.label : value
}

const bootOrderIcon = (value) => {
  const map = { hd: 'Coin', cdrom: 'Disc', network: 'Connection' }
  return map[value] || 'Document'
}

const bootOrderTagType = (value) => {
  const map = { hd: 'primary', cdrom: 'success', network: 'warning' }
  return map[value] || 'info'
}

const addBootOrder = (value) => {
  form.boot_order.push(value)
}

const removeBootOrder = (index) => {
  if (form.boot_order.length <= 1) {
    ElMessage.warning('至少保留一个启动设备')
    return
  }
  form.boot_order.splice(index, 1)
}

const moveBootOrderUp = (index) => {
  if (index <= 0) return
  const temp = form.boot_order[index]
  form.boot_order[index] = form.boot_order[index - 1]
  form.boot_order[index - 1] = temp
}

const moveBootOrderDown = (index) => {
  if (index >= form.boot_order.length - 1) return
  const temp = form.boot_order[index]
  form.boot_order[index] = form.boot_order[index + 1]
  form.boot_order[index + 1] = temp
}

// ==================== Cockpit 风格引导设备管理（编辑模式专用） ====================

// 设备类型图标映射
const bootDeviceIcon = (type) => {
  const map = { disk: 'Coin', cdrom: 'Disc', network: 'Connection' }
  return map[type] || 'Document'
}

// 设备类型中文标签
const bootDeviceTypeLabel = (type) => {
  const map = { disk: '磁盘', cdrom: '光驱', network: '网络' }
  return map[type] || type
}

// 上移设备
const moveBootDeviceUp = (index) => {
  if (index <= 0) return
  const temp = editBootDevices.value[index]
  editBootDevices.value[index] = editBootDevices.value[index - 1]
  editBootDevices.value[index - 1] = temp
}

// 下移设备
const moveBootDeviceDown = (index) => {
  if (index >= editBootDevices.value.length - 1) return
  const temp = editBootDevices.value[index]
  editBootDevices.value[index] = editBootDevices.value[index + 1]
  editBootDevices.value[index + 1] = temp
}

// 从 editBootDevices 生成 boot_order（用于提交）
const buildBootOrderFromDevices = () => {
  const order = []
  const seen = new Set()
  for (const dev of editBootDevices.value) {
    if (!dev.enabled) continue
    const key = dev.type === 'cdrom' ? 'cdrom' : dev.type === 'network' ? 'network' : 'hd'
    if (!seen.has(key)) {
      order.push(key)
      seen.add(key)
    }
  }
  return order.length > 0 ? order : ['hd']
}

// 验证规则
const createRules = computed(() => {
  const base = {
    name: [
      { required: true, message: '请输入虚拟机名称', trigger: 'blur' },
      { pattern: /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$/, message: '虚拟机名称只能包含字母、数字和短横线，且不能以短横线开头或结尾', trigger: 'blur' }
    ],
    vcpu: [{ required: true, message: '请设置 CPU 核心数', trigger: 'change' }],
    ram: [{ required: true, message: '请设置内存大小', trigger: 'change' }],
  }
  if (!isAdmin.value) {
    base.switch_id = [{ required: true, type: 'number', min: 1, message: '请选择 VPC 交换机', trigger: 'change' }]
    base.security_group_id = [{ required: true, type: 'number', min: 1, message: '请选择安全组', trigger: 'change' }]
  }
  if (form.create_mode === 'iso') {
    base.disk_size = [{ required: true, message: '请设置磁盘大小', trigger: 'change' }]
    base.os_type = [{ required: true, message: '请选择系统类型', trigger: 'change' }]
  } else if (form.create_mode === 'import') {
    if (isAdmin.value && form.disk_source_type === 'path') {
      base.disk_path = [{ required: true, message: '请输入磁盘文件绝对路径', trigger: 'blur' }]
    } else {
      base.disk_file = [{ required: true, message: '请选择磁盘文件', trigger: 'change' }]
    }
  } else if (isTemplateSourceMode.value) {
    base.template = [{ required: true, message: '请选择模板', trigger: 'change' }]
    base.disk_size = [{
      validator: (_rule, value, callback) => {
        if (!form.template) {
          callback()
          return
        }
        if (!value || value <= 0) {
          callback(new Error('请选择模板后使用默认磁盘大小，或填写更大的磁盘'))
          return
        }
        if (templateMinDiskSize.value > 0 && value < templateMinDiskSize.value) {
          callback(new Error(`磁盘大小不能小于模板磁盘大小 ${templateMinDiskSize.value} GB`))
          return
        }
        callback()
      },
      trigger: 'change'
    }]
    if (!isLinkedCloneMode.value || registrationMode.value) {
      base.hostname = [{ validator: validateTemplateHostname, trigger: 'blur' }]
    }
    if (!registrationMode.value && !isLinkedCloneMode.value) {
      base.import_user = [{ validator: validateTemplateUsername, trigger: 'blur' }]
      base.import_password = [{ validator: validateTemplatePassword, trigger: 'blur' }]
    }
    if (isFnOSTemplate.value && !isLinkedCloneMode.value && form.fnos_device_id_mode === 'custom') {
      base.fnos_device_id = [{
        validator: (_rule, value, callback) => {
          if (!fnosDeviceIdPattern.test(value || '')) {
            callback(new Error('请输入 32 位或 40 位十六进制设备 ID'))
            return
          }
          callback()
        },
        trigger: 'blur'
      }]
    }
  }
  return base
})

const editRules = reactive({
  name: [{ required: true, message: '请输入虚拟机名称', trigger: 'blur' }],
})

const currentRules = computed(() => {
  if (isEdit.value) return editRules
  return createRules.value
})

// 根据系统类型过滤 OS 变体
const filteredOSVariants = computed(() => {
  const category = form.os_type === 'windows' ? 'Windows' : 'Linux'
  const filtered = osVariants.value.filter(v => v.category === category)

  // 按前缀分组
  const groups = {}
  filtered.forEach(v => {
    let prefix = v.id.replace(/[\d.]+$/, '')
    if (!prefix) prefix = v.id
    if (!groups[prefix]) groups[prefix] = []
    groups[prefix].push(v)
  })

  return Object.entries(groups)
    .map(([label, items]) => ({ label, items }))
    .sort((a, b) => a.label.localeCompare(b.label))
})

// ISO 按存储池分组
const groupedISOs = computed(() => {
  const groups = {}
  isoList.value.forEach(iso => {
    const pool = iso.pool || '默认路径'
    if (!groups[pool]) groups[pool] = []
    groups[pool].push(iso)
  })
  return Object.entries(groups)
    .map(([label, items]) => ({ label, items }))
})

// 模板按类型分组
const groupedTemplates = computed(() => {
  const groups = {}
  templates.value.filter(tpl => !tpl.disabled).forEach(tpl => {
    const key = templateGroupLabel(tpl.type, tpl.category)
    if (!groups[key]) groups[key] = []
    groups[key].push(tpl)
  })
  return Object.entries(groups)
    .filter(([, items]) => items.length > 0)
    .map(([label, items]) => ({
      label,
      items: items.slice().sort((a, b) => templateOptionLabel(a).localeCompare(templateOptionLabel(b))),
    }))
    .sort((a, b) => a.label.localeCompare(b.label))
})

const templateOptionLabel = (tpl) => {
  if (!tpl) return ''
  const display = tpl.display_name || tpl.admin_name || tpl.name
  const categoryLabel = templateCategoryLabel(tpl.type, tpl.category)
  const categoryPrefix = categoryLabel
    ? `${categoryLabel} / `
    : ''
  if (isAdmin.value) {
    const status = tpl.clone_visible ? '用户可见' : '仅管理员'
    const prefix = tpl.level > 0 ? `${'　'.repeat(tpl.level)}└ ` : ''
    return `${prefix}${categoryPrefix}${tpl.admin_name || tpl.name} / ${display}（${status}）`
  }
  return `${categoryPrefix}${display}`
}

// 加载 OS 变体列表
const loadOSVariants = async () => {
  if (osVariants.value.length > 0) return
  try {
    const res = await getOSVariants()
    osVariants.value = res.data || []
  } catch (err) {
    console.error(err)
  }
}

// 加载 ISO 列表（管理员从存储池聚合，普通用户从自己的存储池）
const loadISOs = async () => {
  try {
    if (isAdmin.value) {
      const res = await getAllISOs()
      isoList.value = res.data || []
    } else {
      const res = await getUserISOs()
      isoList.value = res.data || []
    }
  } catch (err) {
    console.error(err)
  }
}

const loadStorageTargets = async (applyDefault = false) => {
  storageTargetsLoading.value = true
  try {
    const res = await getVMStorageTargets()
    storageTargets.value = res.data || []
    if (applyDefault && !form.storage_pool_id) {
      const defaultTarget = storageTargets.value.find(target => target.is_default)
      if (defaultTarget) {
        form.storage_pool_id = defaultTarget.id
      }
    }
  } catch (err) {
    console.error(err)
    storageTargets.value = []
  } finally {
    storageTargetsLoading.value = false
  }
}

const storageTargetLabel = (target) => {
  const suffix = target.is_default ? '默认' : `${formatBytes(target.available)} 可用`
  return `${target.display_name}（${suffix}）`
}

const getDefaultStoragePoolID = () => {
  const defaultTarget = storageTargets.value.find(target => target.is_default)
  return defaultTarget?.id || form.storage_pool_id || ''
}

const addEditDisk = () => {
  if (storageTargets.value.length === 0) {
    loadStorageTargets()
  }
  form.add_disks.push({
    size: 20,
    format: 'qcow2',
    bus: 'virtio',
    storage_pool_id: getDefaultStoragePoolID(),
  })
}

const addCreateExtraDisk = () => {
  if (storageTargets.value.length === 0) {
    loadStorageTargets()
  }
  form.extra_disks.push({
    size: 20,
    format: 'qcow2',
    bus: form.disk_bus || 'virtio',
    storage_pool_id: getDefaultStoragePoolID(),
    iops_total: 0,
    iops_read: 0,
    iops_write: 0,
  })
}

const addExtraImportDisk = () => {
  if (storageTargets.value.length === 0) {
    loadStorageTargets()
  }
  form.extra_import_disks.push({
    disk_path: '',
    disk_file: '',
    disk_source_type: 'path',
    storage_pool_id: getDefaultStoragePoolID(),
    copy_disk: false,
    bus: 'virtio',
    iops_total: 0,
    iops_read: 0,
    iops_write: 0,
  })
}

const formatBytes = (bytes) => {
  if (!bytes || bytes <= 0) return '0'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let val = Number(bytes)
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx += 1
  }
  return `${val.toFixed(2)} ${units[idx]}`
}

// 加载模板列表
const loadTemplates = async (forceReload = false) => {
  if (!forceReload && templates.value.length > 0) {
    if (isTemplateSourceMode.value && form.template) {
      const currentTemplate = templates.value.find(t => t.name === form.template)
      if (currentTemplate) {
        applySelectedTemplateSettings(currentTemplate, { applyProfile: false })
        ensureTemplateDefaults()
      }
    }
    return
  }
  try {
    const res = await getTemplateList()
    templates.value = res.data || []
    if (isTemplateSourceMode.value && form.template) {
      const currentTemplate = templates.value.find(t => t.name === form.template)
      if (currentTemplate) {
        applySelectedTemplateSettings(currentTemplate, { applyProfile: false })
        ensureTemplateDefaults()
      }
    }
  } catch (err) {
    console.error('获取模板列表失败:', err)
  }
}

// 加载用户磁盘文件列表（导入模式用）
const loadDiskFiles = async () => {
  diskFilesLoading.value = true
  try {
    const res = await getStorageFiles('disk')
    diskFileList.value = res.data || []
  } catch (err) {
    console.error(err)
  } finally {
    diskFilesLoading.value = false
  }
}

// 切换磁盘来源类型
const onDiskSourceTypeChange = (type) => {
  if (type === 'path') {
    form.disk_file = ''
  } else {
    form.disk_path = ''
  }
}

// 切换创建模式
const onCreateModeChange = (mode) => {
  if (mode === 'import') {
    // 导入模式默认启动顺序只有硬盘
    form.boot_order = ['hd']
    form.disk_source_type = 'storage'
    form.disk_path = ''
    form.extra_import_disks = []
    loadDiskFiles()
  } else if (mode === 'template' || mode === 'linked_clone') {
    form.boot_order = ['hd']
    form.disk_size = templateMinDiskSize.value || 0
    form.clone_mode = 'linked'
    ensureTemplateDefaults()
    loadTemplates(true)
  } else {
    form.disk_file = ''
    form.disk_path = ''
    form.init_type = ''
  }
  if (mode === 'template' || mode === 'linked_clone') {
    applyRTCOffsetRecommendation(form.template_type === 'windows' ? 'windows' : 'linux')
    applyVideoModelRecommendation(form.template_type === 'windows' ? 'windows' : 'linux')
    normalizeMemoryBackendForGuest()
    return
  }
  if (mode === 'import') {
    applyRTCOffsetRecommendation(form.init_type === 'windows' ? 'windows' : 'linux')
    applyVideoModelRecommendation(form.init_type === 'windows' ? 'windows' : 'linux')
    normalizeMemoryBackendForGuest()
    return
  }
  applyRTCOffsetRecommendation(form.os_type)
  applyVideoModelRecommendation(form.os_type)
  normalizeMemoryBackendForGuest()
}

const onTemplateChange = async (templateName) => {
  // 每次切换模板时都刷新一次模板列表，确保刚编辑过的默认配置能立即带出
  await loadTemplates(true)
  const tpl = templates.value.find(t => t.name === templateName)
  if (tpl) {
    applyRTCOffsetRecommendation(tpl.type === 'windows' ? 'windows' : 'linux')
    applyVideoModelRecommendation(tpl.type === 'windows' ? 'windows' : 'linux')
    applySelectedTemplateSettings(tpl)
    const minDiskSize = parseTemplateDiskSizeGB(tpl.virtual_size)
    if (minDiskSize > 0 && form.disk_size < minDiskSize) {
      form.disk_size = minDiskSize
    }
    if (tpl.type === 'windows') {
      form.import_user = windowsTemplateUsername
    }
    normalizeMemoryBackendForGuest()
    ensureTemplateDefaults()
  }
}

// 加载 VPC 交换机和安全组
const loadVPCOptions = async () => {
  try {
    const params = isAdmin.value && userStore.username ? { username: userStore.username } : undefined
    const [switchRes, groupRes] = await Promise.all([
      getVPCSwitches(params),
      getVPCSecurityGroups(params)
    ])
    vpcSwitches.value = switchRes.data || []
    vpcSecurityGroups.value = groupRes.data || []
    if (!form.switch_id) {
      const defaultSwitch = vpcSwitches.value.find(item => item.name === '默认交换机')
      form.switch_id = defaultSwitch?.id || (vpcSwitches.value.length === 1 ? vpcSwitches.value[0].id : null)
    }
    if (!form.security_group_id) {
      const defaultGroup = vpcSecurityGroups.value.find(item => item.is_default)
      form.security_group_id = defaultGroup?.id || vpcSecurityGroups.value[0]?.id || null
    }
    if (selectedVPCSwitch.value?.bridge_mode === 'bridge') {
      form.security_group_id = null
    }
  } catch (err) {
    console.error(err)
  }
}

// 选择 ISO 后自动补全系统信息
const onISOChange = (paths) => {
  const selectedPaths = Array.isArray(paths) ? paths.filter(Boolean) : (paths ? [paths] : [])
  form.iso_paths = selectedPaths
  form.iso_path = selectedPaths[0] || ''
  if (!form.iso_path) return

  const iso = isoList.value.find(i => i.path === form.iso_path)
  if (!iso) return

  // 自动补全系统类型
  if (iso.os_type) {
    form.os_type = iso.os_type
    applyRTCOffsetRecommendation(iso.os_type)
    applyVideoModelRecommendation(iso.os_type)
    normalizeMemoryBackendForGuest()
  }
  // 自动补全系统版本
  if (iso.os_variant) {
    form.os_variant = iso.os_variant
  }
  // 设置最小磁盘
  const minDisk = iso.min_disk || (iso.os_type === 'windows' ? 20 : 10)
  if (form.disk_size < minDisk) {
    form.disk_size = minDisk
  }
  // Windows 自动设置 UEFI + Q35
  if (iso.os_type === 'windows') {
    applyBootTypeRecommendation('uefi')
    form.machine_type = 'q35'
    // Windows 默认使用 SATA 磁盘驱动和 e1000e 网卡（兼容性更好）
    form.disk_bus = 'sata'
    form.nic_model = 'e1000e'
  }
  // 有 ISO 时启动顺序设置 cdrom 优先
  if (!form.boot_order.includes('cdrom')) {
    form.boot_order = ['cdrom', 'hd']
  }
}

// 切换系统类型
const onOsTypeChange = () => {
  form.os_variant = ''
  if (form.os_type === 'windows') {
    applyBootTypeRecommendation('uefi')
    form.machine_type = 'q35'
    form.disk_bus = 'sata'
    form.nic_model = 'e1000e'
  } else {
    form.disk_bus = 'virtio'
    form.nic_model = 'virtio'
  }
  applyRTCOffsetRecommendation(form.os_type)
  applyVideoModelRecommendation(form.os_type)
  normalizeMemoryBackendForGuest()
}

const onInitTypeChange = (value) => {
  applyRTCOffsetRecommendation(value === 'windows' ? 'windows' : 'linux')
  applyVideoModelRecommendation(value === 'windows' ? 'windows' : 'linux')
  normalizeMemoryBackendForGuest()
}

// 切换引导类型
const onBootTypeChange = () => {
  bootTypeTouched.value = true
  // 安全引导需要 Q35
  if (form.boot_type === 'uefi-secure') {
    form.machine_type = 'q35'
  }
}

// 切换虚拟化方案
const onVirtTypeChange = (val) => {
  if (val === 'kvm') {
    // 切回 KVM 时重置架构和机器类型
    form.arch = 'x86_64'
    form.machine_type = 'q35'
    applyBootTypeRecommendation('bios', { force: true })
  } else {
    // QEMU 模式默认 x86_64
    form.arch = 'x86_64'
  }
}

// 切换平台架构
const onArchChange = (val) => {
  if (val === 'aarch64') {
    // ARM 强制 virt 机器类型 + UEFI
    form.machine_type = 'virt'
    applyBootTypeRecommendation('uefi', { force: true })
  } else if (val === 'riscv64') {
    // RISC-V 强制 virt 机器类型
    form.machine_type = 'virt'
    applyBootTypeRecommendation('bios', { force: true })
  } else {
    // x86_64 恢复默认
    form.machine_type = 'q35'
  }
}

const applyEditVmDetail = (detail, row = {}) => {
  editVmStatus.value = detail.status || row.status || 'shut off'
  editVmVNC.value = detail.vnc_port || row.vnc_port || ''
  form.autostart = detail.autostart || false
  form.freeze = detail.freeze || false
  form.apic = normalizeAPICForForm(detail.apic)
  form.pae = normalizePAEForForm(detail.pae)
  form.rtc_offset = normalizeRTCOffsetForForm(detail.rtc_offset)
  form.rtc_startdate = detail.rtc_startdate || 'now'
  form.os_type = detail.os_type || row.os_type || 'linux'
  Object.assign(form.guest_agent, createEmptyGuestAgentConfig(), detail.guest_agent || {})
  currentVmUUID.value = detail.uuid || ''
  Object.assign(form.smbios1, createEmptySMBIOS1Config(), detail.smbios1 || {})
  form.memory_dynamic_enabled = !!detail.memory_dynamic_enabled
  form.memory_backend = detail.memory_backend || 'balloon'
  form.memory_initial = Math.max(1, Math.round((detail.memory_initial || detail.memory || 1024) / 1024))
  form.memory_min = Math.max(1, Math.round((detail.memory_min || 1024) / 1024))
  form.memory_max_dynamic = Math.max(1, Math.round((detail.memory_max_dynamic || detail.max_memory || detail.memory || 1024) / 1024))
  form.cpu_limit_enabled = Number(detail.cpu_limit_percent || 0) > 0
  form.cpu_limit_percent = form.cpu_limit_enabled ? Number(detail.cpu_limit_percent) : 100
  form.cpu_affinity = detail.cpu_affinity || ''
  if (form.memory_dynamic_enabled && form.memory_backend === 'virtio_mem') {
    const fallbackMemoryGB = Math.max(1, Math.round((detail.memory || row.memory || 1024) / 1024))
    form.memory = getElasticMemorySpecFromConfig(form.memory_initial, form.memory_max_dynamic, fallbackMemoryGB)
  } else {
    form.memory = Math.max(1, Math.round((detail.memory || row.memory || 1024) / 1024))
  }
  form.memory_auto_balloon = !!detail.memory_auto_balloon
  form.memory_current = 0
  form.memory_virtio_mem_current = Math.max(0, Math.round((detail.memory_virtio_mem_current || 0) / 1024))
  form.memory_dynamic_touched = false
  form.memory_pending_apply = !!detail.memory_pending_apply
  form.memory_compat_mode = detail.memory_compat_mode || 'legacy_static'
  form.memory_balloon_supported = !!detail.memory_balloon_supported
  form.memory_balloon_status = detail.memory_balloon_status || 'not_running'
  if (detail.nic_model) {
    form.nic_model = detail.nic_model
    editOrigNicModel.value = detail.nic_model
  }
  form.arch = detail.arch || form.arch || 'x86_64'
  form.machine_type = detail.machine_type || form.machine_type || 'q35'
  form.pcie_root_ports = detail.pcie_root_ports || 4
  editOrigPCIERootPorts.value = form.pcie_root_ports
  form.boot_type = detail.boot_type || form.boot_type || 'bios'
  editOrigBootType.value = form.boot_type
  form.video_model = detail.video_model || getRecommendedVideoModel(detail.os_type || 'linux')
  form.cpu_topology_mode = detail.cpu_topology_mode || 'auto'
  form.boot_order = detail.boot_order && detail.boot_order.length > 0 ? [...detail.boot_order] : ['hd']
  editBootDevices.value = []
  if (detail.boot_devices && detail.boot_devices.length > 0) {
    const sorted = [...detail.boot_devices].sort((a, b) => {
      if (a.enabled && !b.enabled) return -1
      if (!a.enabled && b.enabled) return 1
      if (a.enabled && b.enabled) return a.order - b.order
      return 0
    })
    editBootDevices.value = sorted
  }
  // 加载已有直通设备
  if (isAdmin.value && detail.name) {
    getVmPassthroughDevices(detail.name).then(res => {
      const devices = res.data || res || []
      form.host_devices = devices.map(d => ({ pci_address: d.pci_address }))
      form.host_devices_touched = false
    }).catch(() => {
      form.host_devices = []
      form.host_devices_touched = false
    })
    // 同时加载设备列表用于显示名称和 vfio 状态
    getPassthroughDevices().then(res => {
      passthroughDeviceList.value = (res.data || res || [])
    }).catch(() => {
      passthroughDeviceList.value = []
    })
  } else {
    form.host_devices = []
    form.host_devices_touched = false
  }
}

const reloadEditVmDetail = async (row = currentEditRow.value || {}) => {
  if (!form.name) return
  const detailRes = await getVmDetail(form.name)
  applyEditVmDetail(detailRes.data || {}, row)
}

const saveVmXMLConfig = async () => {
  if (!form.name || !normalizeVmXMLText(vmXMLContent.value) || !vmXMLDirty.value) return
  await ElMessageBox.confirm(
    '保存会立即写入当前虚拟机的持久化 XML，并刷新当前表单。建议先关机后再执行保存，是否继续？',
    '保存虚拟机 XML',
    {
      type: 'warning',
      confirmButtonText: '继续保存',
      cancelButtonText: '取消'
    }
  )
  vmXMLSaving.value = true
  try {
    await updateVmXML(form.name, { xml: vmXMLContent.value })
    await loadVmXMLContent()
    await reloadEditVmDetail()
    await refreshEditDisks()
    ElMessage.success('虚拟机 XML 保存成功')
  } finally {
    vmXMLSaving.value = false
  }
}

const open = async (row, mode, options = {}) => {
  visible.value = true
  activeTabEdit.value = 'basic'
  if (!cpuAffinityPresetsLoaded.value) {
    cpuAffinityPresetsLoaded.value = true
    try {
      const res = await getCPUAffinityPresets()
      if (res.code === 200) {
        cpuAffinityPresets.value = res.data || []
      }
    } catch {}
  }
  // 获取宿主机 CPU 核心数（用于 CPU 热添加上限）
  if (hostCPUCores.value <= 0) {
    try {
      const cpuRes = await getHostCPUCores()
      if (cpuRes.code === 200 && cpuRes.data?.cores) {
        hostCPUCores.value = cpuRes.data.cores
      }
    } catch {}
  }
  // 获取系统设置中的 ISO 存储位置
  try {
    const settingsRes = await getSettings()
    if (settingsRes.data?.iso_dir) {
      isoStorageDir.value = settingsRes.data.iso_dir
    }
  } catch {}
  initAdvancedIntro()
  bootTypeTouched.value = false
  registrationMode.value = mode === 'lightweight-register'
  registrationContext.dedicated_vpc_switch_id = Number(options.dedicated_vpc_switch_id || 0)
  registrationContext.dedicated_vpc_label = options.dedicated_vpc_label || ''
  if (row) {
    registrationMode.value = false
    isEdit.value = true
    currentEditRow.value = { ...row }
    editVmStatus.value = row.status || 'shut off'
    editVmVNC.value = row.vnc_port || ''
    editDisks.value = []
    editCdroms.value = []
    cdromIsoPath.value = ''
    currentVmUUID.value = ''
    const rowMemoryGB = Math.max(1, Math.round((row.memory || 2048) / 1024))
    const rowInitialGB = Math.max(1, Math.round((row.memory_initial || row.memory || 2048) / 1024))
    const rowMaxDynamicGB = Math.max(1, Math.round((row.memory_max_dynamic || row.max_memory || row.memory || 2048) / 1024))
    const rowSpecMemoryGB = row.memory_backend === 'virtio_mem' && row.memory_dynamic_enabled
      ? getElasticMemorySpecFromConfig(rowInitialGB, rowMaxDynamicGB, rowMemoryGB)
      : rowMemoryGB
    Object.assign(form, {
      name: row.name,
      remark: '',
      vcpu: row.vcpu,
      memory: rowSpecMemoryGB,
      os_type: row.os_type || 'linux',
      autostart: row.autostart || false,
      freeze: false,
      apic: true,
      pae: true,
      rtc_offset: 'utc',
      rtc_startdate: 'now',
      guest_agent: createEmptyGuestAgentConfig(),
      memory_dynamic_enabled: false,
      memory_backend: row.memory_backend || 'balloon',
      memory_initial: rowInitialGB,
      memory_min: Math.max(1, Math.floor(rowInitialGB / 2)),
      memory_max_dynamic: rowMaxDynamicGB,
      memory_auto_balloon: true,
      memory_current: 0,
      memory_virtio_mem_current: Math.max(0, Math.round((row.memory_virtio_mem_current || 0) / 1024)),
      cpu_limit_enabled: Number(row.cpu_limit_percent || 0) > 0,
      cpu_limit_percent: Number(row.cpu_limit_percent || 0) > 0 ? Number(row.cpu_limit_percent) : 100,
      cpu_affinity: row.cpu_affinity || '',
      memory_dynamic_touched: false,
      memory_pending_apply: false,
      memory_compat_mode: row.memory_compat_mode || 'legacy_static',
      memory_balloon_supported: row.memory_balloon_supported || false,
      memory_balloon_status: row.memory_balloon_status || 'not_running',
      iso_path: '',
      iso_paths: [],
      nic_model: row.nic_model || 'virtio',
      machine_type: 'q35',
      boot_type: 'bios',
      arch: 'x86_64',
      video_model: row.video_model || getRecommendedVideoModel(row.os_type || 'linux'),
      cpu_topology_mode: row.cpu_topology_mode || 'auto',
      boot_order: ['hd'],
      add_disks: [],
    })
    Object.assign(form.guest_agent, createEmptyGuestAgentConfig())
    Object.assign(form.smbios1, createEmptySMBIOS1Config())
    editOrigNicModel.value = row.nic_model || 'virtio'
    editOrigBootType.value = 'bios'
    editBootDevices.value = []
    // 异步获取详细信息（包含引导顺序和设备列表）
    try {
      await reloadEditVmDetail(row)
    } catch {}
    // 记录原始 CPU/内存值，运行态下禁止减少
    editOrigVcpu.value = form.vcpu
    editOrigMemory.value = form.memory
    // 异步获取磁盘列表（含 CD/DVD 分离）
    refreshEditDisks()
  } else {
    isEdit.value = false
    currentEditRow.value = null
    createStep.value = 0
    editVmStatus.value = 'shut off'
    editVmVNC.value = ''
    editDisks.value = []
    editCdroms.value = []
    cdromIsoPath.value = ''
    currentVmUUID.value = ''
    Object.assign(form, {
      name: generateRandomVmName(), remark: '', vcpu: 2, memory: 2, ram: 2,
      disk_size: (mode === 'template' || mode === 'linked_clone' || registrationMode.value) ? 0 : 20, create_mode: mode === 'import' ? 'import' : (mode === 'template' || mode === 'linked_clone' || registrationMode.value) ? (mode === 'linked_clone' ? 'linked_clone' : 'template') : 'iso',
      os_type: 'linux', os_variant: '', disk_format: 'qcow2', disk_bus: 'virtio',
      system_disk_iops_total: 0, system_disk_iops_read: 0, system_disk_iops_write: 0,
      iso_path: '', iso_paths: [], switch_id: registrationContext.dedicated_vpc_switch_id || null, security_group_id: null, nic_model: 'virtio', video_model: 'virtio', cpu_topology_mode: 'auto', first_boot_reboot_mode: 'normal', autostart: false, freeze: false, apic: true, pae: true, rtc_offset: 'utc', rtc_startdate: 'now',
      storage_pool_id: '',
      guest_agent: createEmptyGuestAgentConfig(),
      memory_dynamic_enabled: false, memory_backend: 'balloon', memory_initial: 2, memory_min: 1, memory_max_dynamic: 3, memory_auto_balloon: true, memory_current: 0, memory_virtio_mem_current: 0,
      cpu_limit_enabled: false, cpu_limit_percent: 100,
      cpu_affinity: '',
      memory_dynamic_touched: false, memory_pending_apply: false, memory_compat_mode: 'legacy_static', memory_balloon_supported: false, memory_balloon_status: 'not_running',
      machine_type: 'q35', boot_type: 'bios', watchdog: 'none',
      pcie_root_ports: 4,
      boot_order: ['hd'], virt_type: 'kvm', arch: 'x86_64',
      add_disks: [], extra_disks: [],
      host_devices: [], host_devices_touched: false,
      disk_file: '', copy_disk: false, init_type: '', start_after_import: true, hostname: (mode === 'template' || registrationMode.value) ? generateRandomHostname() : '',
      import_user: '', import_password: '', template_root_pass: '', template_user: '',
      template: '', template_type: '', preserve_fnos_device_id: false, fnos_device_id_mode: 'regenerate', fnos_device_id: '',
      traffic_down_gb: 0, traffic_up_gb: 0, bandwidth_down_mbps: 0, bandwidth_up_mbps: 0, max_port_forwards: 10, max_runtime_hours: 0, batch_count: 1,
    })
    Object.assign(form.guest_agent, createEmptyGuestAgentConfig())
    Object.assign(form.smbios1, createEmptySMBIOS1Config())
    if (!registrationMode.value) {
      loadVPCOptions()
    }
    loadStorageTargets(mode !== 'import')
    if (mode === 'import') {
      loadDiskFiles()
    } else if (mode === 'template' || mode === 'linked_clone' || registrationMode.value) {
      loadTemplates(true)
    }
  }
}

// 刷新编辑模式下的磁盘列表（分离 CD/DVD 和普通磁盘）
const refreshEditDisks = async () => {
  if (!form.name) return
  try {
    const diskRes = await getDiskList(form.name)
    const allDisks = diskRes.data || []

    // 分离 CD/DVD 和普通磁盘
    const normalDisks = []
    const cdroms = []
    for (const d of allDisks) {
      const isCdrom = d.device_type === 'cdrom' || (d.path && d.path.endsWith('.iso'))
      if (isCdrom) {
        cdroms.push({
          device: d.device,
          path: (d.path && d.path !== '-') ? d.path : '',
        })
      } else {
        normalDisks.push(d)
      }
    }
    editCdroms.value = cdroms
    editDisks.value = normalDisks
  } catch {}
}

// ==================== CD/DVD 操作 ====================

// 插入 CD/DVD（替换已有光驱的 ISO）
const handleInsertCDROM = async (device) => {
  if (!cdromIsoPath.value) return
  try {
    await changeCDROM(form.name, {
      iso_path: cdromIsoPath.value,
      device: device || '',
    })
    ElMessage.success('光盘已插入')
    cdromIsoPath.value = ''
    refreshEditDisks()
  } catch (err) {
    console.error('插入光盘失败', err)
  }
}

// 添加新光驱设备（强制新增，不替换已有的）
const handleAddNewCDROM = async () => {
  if (!cdromIsoPath.value) return
  try {
    await changeCDROM(form.name, {
      iso_path: cdromIsoPath.value,
      force_new: true,
    })
    ElMessage.success('光驱已添加')
    cdromIsoPath.value = ''
    refreshEditDisks()
  } catch (err) {
    console.error('添加光驱失败', err)
  }
}

// 弹出 CD/DVD
const handleEjectCDROM = async (device) => {
  try {
    await ejectCDROM(form.name, device)
    ElMessage.success('光盘已弹出')
    refreshEditDisks()
  } catch {
    console.error('弹出光盘失败')
  }
}

// 移除 CD/DVD 设备
const handleRemoveCDROM = async (device) => {
  try {
    await ElMessageBox.confirm(
      `确定要移除光驱设备 ${device} 吗？`,
      '移除光驱',
      { type: 'warning' }
    )
    await removeCDROM(form.name, device)
    ElMessage.success('光驱已移除')
    refreshEditDisks()
  } catch {}
}

// 磁盘扩容
const handleResizeDisk = async (disk) => {
  try {
    const { value } = await ElMessageBox.prompt(
      `当前容量: ${disk.capacity_gb || '未知'} GB\n请输入新的容量（GB），只能扩大不能缩小:`,
      `扩容磁盘 ${disk.device}`,
      {
        confirmButtonText: '扩容',
        cancelButtonText: '取消',
        inputValue: '',
        inputPattern: /^\d+$/,
        inputErrorMessage: '请输入有效的数字',
      }
    )
    const newSize = parseInt(value)
    if (newSize <= 0) {
      ElMessage.error('容量必须大于 0')
      return
    }
    await resizeDisk(form.name, disk.device, newSize)
    ElMessage.success(`磁盘 ${disk.device} 扩容成功`)
    refreshEditDisks()
  } catch {}
}

// 删除/转移磁盘
const handleRemoveDisk = async (disk) => {
  try {
    const action = await ElMessageBox.confirm(
      `确定要删除磁盘 ${disk.device}（${disk.path}）吗？`,
      '删除磁盘',
      {
        confirmButtonText: '删除',
        cancelButtonText: '转移到我的存储',
        distinguishCancelAndClose: true,
        type: 'warning',
        showClose: true,
      }
    )
    // 用户点击“连同文件一起删除”
    await removeDisk(form.name, disk.device, true)
    ElMessage.success(`磁盘 ${disk.device} 已删除（含文件）`)
    refreshEditDisks()
  } catch (action) {
    // 用户点击右上角关闭等
    if (action === 'close') return

    // 用户点击“转移到我的存储”
    if (action === 'cancel') {
      try {
        await removeDisk(form.name, disk.device, false, true)
        ElMessage.success(`磁盘 ${disk.device} 已卸载并转移到「我的存储-虚拟磁盘」`)
        refreshEditDisks()
      } catch {}
    }
  }
}

// 修改已有磁盘驱动类型
const handleChangeDiskBus = async (disk, newBus) => {
  try {
    await changeDiskBus(form.name, disk.device, newBus)
    ElMessage.success(`磁盘 ${disk.device} 驱动已修改为 ${newBus.toUpperCase()}`)
    refreshEditDisks()
  } catch (err) {
    console.error('修改磁盘驱动失败', err)
    refreshEditDisks() // 回滚 UI
  }
}

// ==================== 磁盘 IOPS 设置 ====================
const diskIOPSDialogVisible = ref(false)
const diskIOPSDialogDevice = ref('')
const diskIOPSDialogPath = ref('')
const diskIOPSCurrentDisk = ref(null)

const diskIOPSForm = reactive({
  total_iops_sec: 0,
  read_iops_sec: 0,
  write_iops_sec: 0
})

const openDiskIOPSDialog = (disk) => {
  diskIOPSDialogDevice.value = disk.device || ''
  diskIOPSDialogPath.value = disk.path || ''
  diskIOPSCurrentDisk.value = disk
  diskIOPSForm.total_iops_sec = disk.iops_total?.value || 0
  diskIOPSForm.read_iops_sec = disk.iops_read?.value || 0
  diskIOPSForm.write_iops_sec = disk.iops_write?.value || 0
  diskIOPSDialogVisible.value = true
}

const applyDiskIOPSForEditForm = () => {
  if (!diskIOPSCurrentDisk.value) return
  const disk = diskIOPSCurrentDisk.value
  // 将 IOPS 设置存储在磁盘对象上，稍后提交编辑时一并发送
  disk._iops_total = diskIOPSForm.total_iops_sec
  disk._iops_read = diskIOPSForm.read_iops_sec
  disk._iops_write = diskIOPSForm.write_iops_sec
  // 更新显示值
  disk.iops_total = { value: diskIOPSForm.total_iops_sec, is_set: diskIOPSForm.total_iops_sec > 0 }
  disk.iops_read = { value: diskIOPSForm.read_iops_sec, is_set: diskIOPSForm.read_iops_sec > 0 }
  disk.iops_write = { value: diskIOPSForm.write_iops_sec, is_set: diskIOPSForm.write_iops_sec > 0 }
  ElMessage.success(`磁盘 ${diskIOPSDialogDevice.value} IOPS 限制已设置（将在保存编辑时生效）`)
  diskIOPSDialogVisible.value = false
}

// ==================== 创建模式额外磁盘 IOPS 设置 ====================
const createDiskIOPSDialogVisible = ref(false)
const createDiskIOPSIndex = ref(-1)
const createDiskIOPSDesc = ref('')

const createDiskIOPSForm = reactive({
  total_iops_sec: 0,
  read_iops_sec: 0,
  write_iops_sec: 0
})

const openCreateExtraDiskIOPSDialog = (index) => {
  const disk = form.extra_disks[index]
  if (!disk) return
  createDiskIOPSIndex.value = index
  createDiskIOPSDesc.value = `${disk.size}GB ${disk.format} ${disk.bus}`
  createDiskIOPSForm.total_iops_sec = disk.iops_total || 0
  createDiskIOPSForm.read_iops_sec = disk.iops_read || 0
  createDiskIOPSForm.write_iops_sec = disk.iops_write || 0
  createDiskIOPSDialogVisible.value = true
}

const applyCreateDiskIOPS = () => {
  const index = createDiskIOPSIndex.value
  // 判断是导入模式额外磁盘还是创建模式额外磁盘
  const targetArr = form.create_mode === 'import' ? form.extra_import_disks : form.extra_disks
  if (index < 0 || index >= targetArr.length) return
  const disk = targetArr[index]
  disk.iops_total = createDiskIOPSForm.total_iops_sec
  disk.iops_read = createDiskIOPSForm.read_iops_sec
  disk.iops_write = createDiskIOPSForm.write_iops_sec
  ElMessage.success(`额外磁盘 #${index + 1} IOPS 限制已设置`)
  createDiskIOPSDialogVisible.value = false
}

const openImportExtraDiskIOPSDialog = (index) => {
  const disk = form.extra_import_disks[index]
  if (!disk) return
  createDiskIOPSIndex.value = index
  createDiskIOPSDesc.value = `${disk.disk_path || disk.disk_file || '(未选择)'} ${disk.bus}`
  createDiskIOPSForm.total_iops_sec = disk.iops_total || 0
  createDiskIOPSForm.read_iops_sec = disk.iops_read || 0
  createDiskIOPSForm.write_iops_sec = disk.iops_write || 0
  createDiskIOPSDialogVisible.value = true
}

// 挂载已有磁盘文件
// 打开挂载已有磁盘对话框
const openAttachDiskDialog = async () => {
  attachDiskPath.value = ''
  attachDiskBus.value = 'virtio'
  attachDiskSourceType.value = 'storage'
  attachDiskAbsolutePath.value = ''
  attachDiskStoragePoolId.value = ''
  attachDiskCopyDisk.value = false
  attachDiskVisible.value = true
  // 加载用户存储中的虚拟磁盘列表
  attachDiskLoading.value = true
  try {
    const res = await getStorageFiles('disk')
    attachDiskFiles.value = res.data || []
  } catch {
    attachDiskFiles.value = []
  } finally {
    attachDiskLoading.value = false
  }
}

// 切换磁盘来源类型（编辑模式挂载对话框）
const onAttachDiskSourceTypeChange = (type) => {
  if (type === 'path') {
    attachDiskPath.value = ''
  } else {
    attachDiskAbsolutePath.value = ''
  }
}

// 执行挂载
const handleAttachDisk = async () => {
  attachDiskSubmitting.value = true
  try {
    if (isAdmin.value && attachDiskSourceType.value === 'path') {
      // 管理员绝对路径导入（异步任务）
      if (!attachDiskAbsolutePath.value) return
      await adminImportDiskForVM(form.name, {
        disk_path: attachDiskAbsolutePath.value,
        disk_source_type: 'path',
        storage_pool_id: attachDiskStoragePoolId.value,
        copy_disk: attachDiskCopyDisk.value,
        bus: attachDiskBus.value,
      })
      ElMessage.success('导入磁盘任务已提交，请在任务中心查看进度')
    } else {
      await attachDisk(form.name, attachDiskPath.value, attachDiskBus.value)
      ElMessage.success('磁盘挂载成功')
    }
    attachDiskVisible.value = false
    refreshEditDisks()
  } catch {}
  attachDiskSubmitting.value = false
}

const onClosed = () => {
  activeTabEdit.value = 'basic'
  createStep.value = 0
  rtcConfigVisible.value = false
  guestAgentConfigVisible.value = false
  memoryDynamicConfigVisible.value = false
  memoryVirtioMemDetailVisible.value = false
  smbiosConfigVisible.value = false
  vmXMLConfigVisible.value = false
  vmXMLLoading.value = false
  vmXMLSaving.value = false
  vmXMLContent.value = ''
  vmXMLOriginal.value = ''
  currentVmUUID.value = ''
  currentEditRow.value = null
  editOrigBootType.value = ''
  editOrigPCIERootPorts.value = 0
  registrationMode.value = false
  registrationContext.dedicated_vpc_switch_id = 0
  registrationContext.dedicated_vpc_label = ''
  initAdvancedIntro()
  extraNics.value = []
  formRef.value?.resetFields()
}

const submitForm = async () => {
  if (!formRef.value) return
  if (!isEdit.value && !isAdmin.value && (!form.switch_id || !form.security_group_id)) {
    await loadVPCOptions()
  }
  await formRef.value.validate(async (valid) => {
    if (valid) {
      loading.value = true
      try {
        if (isEdit.value) {
          // 从引导设备列表生成 boot_order
          const computedBootOrder = editBootDevices.value.length > 0
            ? buildBootOrderFromDevices()
            : form.boot_order
          const editPayload = {
            vcpu: form.vcpu,
            max_vcpu: cpuHotplugMaxVCPU.value,
            memory: form.memory,
            autostart: form.autostart,
            freeze: form.freeze,
            start_after_import: form.start_after_import,
            apic: !!form.apic,
            pae: !!form.pae,
            rtc_offset: form.rtc_offset,
            rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
            guest_agent: buildGuestAgentPayload(),
            smbios1: buildSMBIOS1Payload(),
            boot_order: computedBootOrder,
            add_disks: form.add_disks.filter(d => d.size > 0),
          }
          // PCIe 热插槽数量（仅当用户修改后发送）
          if (form.machine_type === 'q35' && form.pcie_root_ports !== editOrigPCIERootPorts.value) {
            editPayload.pcie_root_ports = form.pcie_root_ports
          }
          // 硬件直通设备：仅在管理员修改后发送
          if (isAdmin.value && form.host_devices_touched) {
            editPayload.host_devices = form.host_devices
          }
          // 收集 IOPS 设置（仅管理员）
          if (isAdmin.value) {
            const diskIops = {}
            editDisks.value.forEach(disk => {
              if (disk.device && disk._iops_total !== undefined) {
                diskIops[disk.device] = {
                  total_iops_sec: disk._iops_total || 0,
                  read_iops_sec: disk._iops_read || 0,
                  write_iops_sec: disk._iops_write || 0
                }
              }
            })
            if (Object.keys(diskIops).length > 0) {
              editPayload.disk_iops = diskIops
            }
          }
          const cpuLimitPercent = buildCPULimitPercentPayload()
          if (cpuLimitPercent !== undefined) {
            editPayload.cpu_limit_percent = cpuLimitPercent
          }
          if (isAdmin.value) {
            editPayload.cpu_affinity = (form.cpu_affinity || '').trim()
          }
          if (form.cpu_topology_mode && editVmStatus.value !== 'running' && editVmStatus.value !== 'paused') {
            editPayload.cpu_topology_mode = form.cpu_topology_mode
          }
          // 只有网卡类型变化时才传递，避免运行中无谓报错
          if (form.nic_model && form.nic_model !== editOrigNicModel.value) {
            editPayload.nic_model = form.nic_model
          }
          if (form.boot_type && form.boot_type !== editOrigBootType.value) {
            editPayload.boot_type = form.boot_type
          }
          if (form.video_model && editVmStatus.value !== 'running' && editVmStatus.value !== 'paused') {
            editPayload.video_model = form.video_model
          }
          const memoryPayload = buildMemoryDynamicPayload()
          if (memoryPayload) {
            editPayload.memory_dynamic = memoryPayload
          }
          await updateVm(form.name, editPayload)
          ElMessage.success('配置修改成功')
        } else if (form.create_mode === 'import') {
          // 导入模式
          if (isAdmin.value && form.disk_source_type === 'path') {
            // 管理员绝对路径导入
            if (!form.disk_path) {
              ElMessage.warning('请输入磁盘文件的绝对路径')
              return
            }
            const importPayload = {
              name: form.name, remark: form.remark,
              disk_path: form.disk_path,
              disk_source_type: 'path',
              storage_pool_id: form.storage_pool_id,
              vcpu: form.vcpu, ram: form.ram,
              switch_id: form.switch_id || 0,
              security_group_id: selectedVPCSwitch.value?.bridge_mode === 'bridge' ? 0 : (form.security_group_id || 0),
              copy_disk: form.copy_disk, init_type: form.init_type,
              hostname: form.hostname || form.name,
              user: form.import_user, password: form.import_password,
              template_root_pass: form.template_root_pass, template_user: form.template_user,
              autostart: form.autostart, freeze: form.freeze,
              start_after_import: form.start_after_import,
              apic: !!form.apic, pae: !!form.pae,
              rtc_offset: form.rtc_offset, rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
              guest_agent: buildGuestAgentPayload(), smbios1: buildSMBIOS1Payload(),
              boot_type: form.boot_type, machine_type: form.machine_type,
              nic_model: form.nic_model, video_model: form.video_model,
              cpu_topology_mode: form.cpu_topology_mode,
              first_boot_reboot_mode: form.first_boot_reboot_mode,
              extra_nics: buildExtraNicsPayload(),
              extra_import_disks: form.extra_import_disks.filter(d => d.disk_path || d.disk_file).map(d => ({
                disk_path: d.disk_path,
                disk_file: d.disk_file,
                disk_source_type: d.disk_source_type,
                storage_pool_id: d.storage_pool_id,
                copy_disk: d.copy_disk,
                bus: d.bus,
                iops_total: d.iops_total || 0,
                iops_read: d.iops_read || 0,
                iops_write: d.iops_write || 0,
              })),
              system_disk_iops: isAdmin.value && (form.system_disk_iops_total > 0 || form.system_disk_iops_read > 0 || form.system_disk_iops_write > 0) ? {
                total_iops_sec: form.system_disk_iops_total,
                read_iops_sec: form.system_disk_iops_read,
                write_iops_sec: form.system_disk_iops_write,
              } : undefined,
            }
            const cpuLimitPercent = buildCPULimitPercentPayload()
            if (cpuLimitPercent !== undefined) { importPayload.cpu_limit_percent = cpuLimitPercent }
            if (isAdmin.value) { importPayload.cpu_affinity = (form.cpu_affinity || '').trim() }
            const memoryPayload = buildMemoryDynamicPayload()
            if (memoryPayload) { importPayload.memory_dynamic = memoryPayload }
            await adminImportDisk(importPayload)
            ElMessage.success('导入磁盘任务已提交，请在任务中查看进度')
          } else {
            // 从存储导入
            const importPayload = {
            name: form.name,
            remark: form.remark,
            disk_file: form.disk_file,
            vcpu: form.vcpu,
            max_vcpu: cpuHotplugMaxVCPU.value,
            ram: form.ram,
            switch_id: form.switch_id || 0,
            security_group_id: selectedVPCSwitch.value?.bridge_mode === 'bridge' ? 0 : (form.security_group_id || 0),
            copy_disk: form.copy_disk,
            init_type: form.init_type,
            hostname: form.hostname || form.name,
            user: form.import_user,
            password: form.import_password,
            template_root_pass: form.template_root_pass,
            template_user: form.template_user,
            autostart: form.autostart,
            freeze: form.freeze,
            start_after_import: form.start_after_import,
            apic: !!form.apic,
            pae: !!form.pae,
            rtc_offset: form.rtc_offset,
            rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
            guest_agent: buildGuestAgentPayload(),
            smbios1: buildSMBIOS1Payload(),
            boot_type: form.boot_type,
            machine_type: form.machine_type,
            nic_model: form.nic_model,
            video_model: form.video_model,
            cpu_topology_mode: form.cpu_topology_mode,
            first_boot_reboot_mode: form.first_boot_reboot_mode,
            extra_nics: buildExtraNicsPayload(),
          }
          const cpuLimitPercent = buildCPULimitPercentPayload()
          if (cpuLimitPercent !== undefined) {
            importPayload.cpu_limit_percent = cpuLimitPercent
          }
          if (isAdmin.value) { importPayload.cpu_affinity = (form.cpu_affinity || '').trim() }
          const memoryPayload = buildMemoryDynamicPayload()
          if (memoryPayload) {
            importPayload.memory_dynamic = memoryPayload
          }
          await importVM(importPayload)
          ElMessage.success('导入任务已提交，请在任务中查看进度')
          }
        } else if (form.create_mode === 'linked_clone') {
          ensureTemplateDefaults()
          const linkedClonePayload = {
            name: form.name,
            remark: form.remark,
            template: form.template,
            template_type: form.template_type,
            clone_mode: form.clone_mode,
            vcpu: form.vcpu,
            max_vcpu: cpuHotplugMaxVCPU.value,
            ram: form.ram,
            disk_size: form.disk_size,
            switch_id: form.switch_id || 0,
            security_group_id: selectedVPCSwitch.value?.bridge_mode === 'bridge' ? 0 : (form.security_group_id || 0),
            storage_pool_id: form.storage_pool_id,
            autostart: form.autostart,
            freeze: form.freeze,
            apic: !!form.apic,
            pae: !!form.pae,
            rtc_offset: form.rtc_offset,
            rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
            guest_agent: buildGuestAgentPayload(),
            smbios1: buildSMBIOS1Payload(),
            boot_type: form.boot_type,
            disk_bus: form.disk_bus,
            nic_model: form.nic_model,
            video_model: form.video_model,
            cpu_topology_mode: form.cpu_topology_mode,
            first_boot_reboot_mode: form.first_boot_reboot_mode,
            extra_nics: buildExtraNicsPayload(),
            extra_disks: form.extra_disks.filter(d => d.size > 0).map(d => ({
              size: d.size,
              format: d.format,
              bus: d.bus,
              storage_pool_id: d.storage_pool_id,
              iops_total: d.iops_total || 0,
              iops_read: d.iops_read || 0,
              iops_write: d.iops_write || 0,
            })),
            host_devices: form.host_devices,
            pcie_root_ports: form.pcie_root_ports,
          }
          const cpuLimitPercent = buildCPULimitPercentPayload()
          if (cpuLimitPercent !== undefined) {
            linkedClonePayload.cpu_limit_percent = cpuLimitPercent
          }
          const memoryPayload = buildMemoryDynamicPayload()
          if (memoryPayload) {
            linkedClonePayload.memory_dynamic = memoryPayload
          }
          await linkedCloneVm(linkedClonePayload)
          ElMessage.success('原生链式克隆任务已提交，请在任务中查看进度')
        } else if (form.create_mode === 'template') {
          ensureTemplateDefaults()

          // 批量克隆模式
          if (form.batch_count > 1) {
            if (registrationMode.value) {
              ElMessage.warning('批量创建暂不支持服务器登记模式')
              return
            }
            const batchPayload = {
              prefix: form.name,
              start_num: 1,
              count: form.batch_count,
              template: form.template,
              template_type: form.template_type,
              clone_mode: form.clone_mode,
              vcpu: form.vcpu,
              max_vcpu: cpuHotplugMaxVCPU.value,
              ram: form.ram,
              disk_size: form.disk_size,
              hostname: '', // 批量模式下每台虚拟机由后端自动生成独立主机名
              user: isWindowsTemplate.value ? windowsTemplateUsername : form.import_user.trim(),
              password: form.import_password,
              autostart: form.autostart,
              freeze: form.freeze,
              apic: !!form.apic,
              pae: !!form.pae,
              rtc_offset: form.rtc_offset,
              rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
              guest_agent: buildGuestAgentPayload(),
              smbios1: buildSMBIOS1Payload(),
              uefi: (form.boot_type === 'uefi' || form.boot_type === 'uefi-secure') ? true : undefined,
              template_root_pass: form.template_root_pass,
              template_user: isWindowsTemplate.value ? windowsTemplateUsername : form.import_user.trim(),
              video_model: form.video_model,
              disk_bus: form.disk_bus,
              nic_model: form.nic_model,
              storage_pool_id: form.storage_pool_id,
              cpu_topology_mode: form.cpu_topology_mode,
              first_boot_reboot_mode: form.first_boot_reboot_mode,
              switch_id: form.switch_id || 0,
              security_group_id: form.security_group_id || 0,
              extra_nics: buildExtraNicsPayload(),
            }
            const cpuLimitPercent = buildCPULimitPercentPayload()
            if (cpuLimitPercent !== undefined) { batchPayload.cpu_limit_percent = cpuLimitPercent }
            if (isAdmin.value) { batchPayload.cpu_affinity = (form.cpu_affinity || '').trim() }
            await batchCloneVm(batchPayload)
            ElMessage.success(`批量克隆任务已提交（${form.batch_count} 台），请在任务中查看进度`)
            visible.value = false
            emit('success')
            return
          }

          // 模板模式（单台）
          const clonePayload = {
            name: form.name,
            remark: form.remark,
            template: form.template,
            template_type: form.template_type,
            clone_mode: form.clone_mode,
            vcpu: form.vcpu,
            max_vcpu: cpuHotplugMaxVCPU.value,
            ram: form.ram,
            disk_size: form.disk_size,
            hostname: form.hostname,
            user: isWindowsTemplate.value ? windowsTemplateUsername : form.import_user.trim(),
            password: form.import_password,
            switch_id: form.switch_id || 0,
            security_group_id: selectedVPCSwitch.value?.bridge_mode === 'bridge' ? 0 : (form.security_group_id || 0),
            storage_pool_id: form.storage_pool_id,
            autostart: form.autostart,
            freeze: form.freeze,
            apic: !!form.apic,
            pae: !!form.pae,
            rtc_offset: form.rtc_offset,
            rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
            guest_agent: buildGuestAgentPayload(),
            smbios1: buildSMBIOS1Payload(),
            uefi: form.boot_type === 'uefi' || form.boot_type === 'uefi-secure',
            disk_bus: form.disk_bus,
            system_disk_iops: isAdmin.value && (form.system_disk_iops_total > 0 || form.system_disk_iops_read > 0 || form.system_disk_iops_write > 0) ? {
              total_iops_sec: form.system_disk_iops_total,
              read_iops_sec: form.system_disk_iops_read,
              write_iops_sec: form.system_disk_iops_write,
            } : undefined,
            nic_model: form.nic_model,
            video_model: form.video_model,
            cpu_topology_mode: form.cpu_topology_mode,
            first_boot_reboot_mode: form.first_boot_reboot_mode,
            extra_nics: buildExtraNicsPayload(),
            preserve_fnos_device_id: shouldPreserveFnOSDeviceID.value,
            fnos_device_id: hasCustomFnOSDeviceID.value ? normalizedFnOSDeviceID.value : '',
            extra_disks: form.extra_disks.filter(d => d.size > 0).map(d => ({
              size: d.size,
              format: d.format,
              bus: d.bus,
              storage_pool_id: d.storage_pool_id,
              iops_total: d.iops_total || 0,
              iops_read: d.iops_read || 0,
              iops_write: d.iops_write || 0,
            })),
            host_devices: form.host_devices,
            pcie_root_ports: form.pcie_root_ports,
          }
          const cpuLimitPercent = buildCPULimitPercentPayload()
          if (cpuLimitPercent !== undefined) {
            clonePayload.cpu_limit_percent = cpuLimitPercent
          }
          if (isAdmin.value) { clonePayload.cpu_affinity = (form.cpu_affinity || '').trim() }
          const memoryPayload = buildMemoryDynamicPayload()
          if (memoryPayload) {
            clonePayload.memory_dynamic = memoryPayload
          }
          if (registrationMode.value) {
            const registrationDraft = {
              vm_name: clonePayload.name,
              template: clonePayload.template,
              template_type: clonePayload.template_type,
              clone_mode: clonePayload.clone_mode,
              vcpu: clonePayload.vcpu,
              ram: clonePayload.ram,
              disk_size: clonePayload.disk_size,
              hostname: clonePayload.hostname,
              autostart: clonePayload.autostart,
              freeze: clonePayload.freeze,
              apic: clonePayload.apic,
              pae: clonePayload.pae,
              rtc_offset: clonePayload.rtc_offset,
              rtc_startdate: clonePayload.rtc_startdate,
              guest_agent: clonePayload.guest_agent,
              smbios1: clonePayload.smbios1,
              memory_dynamic: clonePayload.memory_dynamic,
              disk_bus: clonePayload.disk_bus,
              video_model: clonePayload.video_model,
              cpu_topology_mode: clonePayload.cpu_topology_mode,
              cpu_limit_percent: clonePayload.cpu_limit_percent,
              cpu_affinity: clonePayload.cpu_affinity,
              first_boot_reboot_mode: clonePayload.first_boot_reboot_mode,
              nic_model: clonePayload.nic_model,
              storage_pool_id: clonePayload.storage_pool_id,
              extra_disks: clonePayload.extra_disks,
              preserve_fnos_device_id: clonePayload.preserve_fnos_device_id,
              fnos_device_id: clonePayload.fnos_device_id,
              traffic_down_gb: form.traffic_down_gb || 0,
              traffic_up_gb: form.traffic_up_gb || 0,
              bandwidth_down_mbps: form.bandwidth_down_mbps || 0,
              bandwidth_up_mbps: form.bandwidth_up_mbps || 0,
              max_port_forwards: form.max_port_forwards ?? 10,
              max_runtime_hours: form.max_runtime_hours || 0,
            }
            emit('draft', registrationDraft)
            ElMessage.success('已加入注册列表，请在列表中确认后保存')
            visible.value = false
            return
          }
          if (isAdmin.value) {
            await cloneVm(clonePayload)
          } else {
            await selfCloneVm(clonePayload)
          }
          ElMessage.success('克隆任务已提交，请在任务中查看进度')
        } else {
          const createPayload = {
            name: form.name,
            remark: form.remark,
            vcpu: form.vcpu,
            max_vcpu: cpuHotplugMaxVCPU.value,
            ram: form.ram,
            disk_size: form.disk_size,
            disk_format: form.disk_format,
            disk_bus: form.disk_bus,
            system_disk_iops: isAdmin.value && (form.system_disk_iops_total > 0 || form.system_disk_iops_read > 0 || form.system_disk_iops_write > 0) ? {
              total_iops_sec: form.system_disk_iops_total,
              read_iops_sec: form.system_disk_iops_read,
              write_iops_sec: form.system_disk_iops_write,
            } : undefined,
            os_variant: form.os_variant,
            iso_path: form.iso_path,
            iso_paths: form.iso_paths.filter(Boolean),
            switch_id: form.switch_id || 0,
            security_group_id: selectedVPCSwitch.value?.bridge_mode === 'bridge' ? 0 : (form.security_group_id || 0),
            storage_pool_id: form.storage_pool_id,
            nic_model: form.nic_model,
            autostart: form.autostart,
            freeze: form.freeze,
            apic: !!form.apic,
            pae: !!form.pae,
            rtc_offset: form.rtc_offset,
            rtc_startdate: normalizeRTCStartDate(form.rtc_startdate),
            guest_agent: buildGuestAgentPayload(),
            smbios1: buildSMBIOS1Payload(),
            os_type: form.os_type,
            machine_type: form.machine_type,
            boot_type: form.boot_type,
            watchdog: form.watchdog,
            boot_order: form.boot_order,
            video_model: form.video_model,
            cpu_topology_mode: form.cpu_topology_mode,
            virt_type: form.virt_type,
            arch: form.virt_type === 'qemu' ? form.arch : undefined,
            pcie_root_ports: form.machine_type === 'q35' ? form.pcie_root_ports : undefined,
            extra_disks: form.extra_disks.filter(d => d.size > 0).map(d => ({
              size: d.size,
              format: d.format,
              bus: d.bus,
              storage_pool_id: d.storage_pool_id,
              iops_total: d.iops_total || 0,
              iops_read: d.iops_read || 0,
              iops_write: d.iops_write || 0,
            })),
            host_devices: form.host_devices,
            extra_nics: buildExtraNicsPayload(),
          }
          const cpuLimitPercent = buildCPULimitPercentPayload()
          if (cpuLimitPercent !== undefined) {
            createPayload.cpu_limit_percent = cpuLimitPercent
          }
          if (isAdmin.value) { createPayload.cpu_affinity = (form.cpu_affinity || '').trim() }
          const memoryPayload = buildMemoryDynamicPayload()
          if (memoryPayload) {
            createPayload.memory_dynamic = memoryPayload
          }
          // 普通用户使用自助创建接口，管理员使用管理接口
          if (isAdmin.value) {
            await createVm(createPayload)
          } else {
            await selfCreateVm(createPayload)
          }
          ElMessage.success('创建任务已提交，请在任务中查看进度')
        }
        visible.value = false
        emit('success')
      } catch (error) {
        console.error(error)
      } finally {
        loading.value = false
      }
    }
  })
}

// ==================== 硬件直通 ====================
const passthroughDialogVisible = ref(false)
const passthroughDeviceList = ref([])
const passthroughLoading = ref(false)
const passthroughSelectedDevices = ref([])
const passthroughTableRef = ref(null)

const openPassthroughDialog = async () => {
  passthroughDialogVisible.value = true
  passthroughLoading.value = true
  passthroughSelectedDevices.value = []
  try {
    const res = await getPassthroughDevices()
    passthroughDeviceList.value = (res.data || res || [])
  } catch (e) {
    console.error('加载直通设备列表失败:', e)
    ElMessage.error('加载直通设备列表失败')
    passthroughDeviceList.value = []
  } finally {
    passthroughLoading.value = false
  }
}

const isPassthroughDeviceSelectable = (row) => {
  if (row.is_used_by_vm) return false
  const alreadyAdded = form.host_devices.some(d => d.pci_address === row.pci_address)
  if (alreadyAdded) return false
  return true
}

const onPassthroughSelectionChange = (selection) => {
  passthroughSelectedDevices.value = selection
}

const confirmAddPassthroughDevices = () => {
  for (const dev of passthroughSelectedDevices.value) {
    if (!form.host_devices.some(d => d.pci_address === dev.pci_address)) {
      form.host_devices.push({ pci_address: dev.pci_address })
    }
  }
  form.host_devices_touched = true
  passthroughDialogVisible.value = false
}

const removeHostDevice = (index) => {
  form.host_devices.splice(index, 1)
  form.host_devices_touched = true
}

const getPassthroughDeviceName = (pciAddress) => {
  const cached = passthroughDeviceList.value.find(d => d.pci_address === pciAddress)
  if (cached) {
    return [cached.vendor_name, cached.product_name].filter(Boolean).join(' ') || '未知设备'
  }
  return pciAddress
}

const isDeviceVfioBound = (pciAddress) => {
  const cached = passthroughDeviceList.value.find(d => d.pci_address === pciAddress)
  return cached ? cached.is_vfio_bound : false
}

defineExpose({
  open
})
</script>

<style scoped>
.vm-form :deep(.el-divider__text) {
  font-size: 13px;
  color: #606266;
  font-weight: 500;
}

.vm-tabs {
  margin-top: -10px;
}
.vm-tabs :deep(.el-tabs__header) {
  margin-bottom: 20px;
}
.tab-content-wrapper {
  padding: 0 10px;
  min-height: 400px;
}
.registration-summary-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.registration-summary-grid div {
  padding: 10px 12px;
  border-radius: 8px;
  background: #f7fbff;
  border: 1px solid #e4edf8;
}
.registration-summary-grid span {
  display: block;
  color: #909399;
  font-size: 12px;
  margin-bottom: 4px;
}
.registration-summary-grid strong {
  color: #303133;
  font-size: 13px;
  font-weight: 600;
}
.step-pane {
  animation: fadeIn 0.3s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

/* ==================== 选择创建方式 Mode Cards ==================== */
.mode-selection-wrapper {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 380px;
}
.mode-cards {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
  justify-content: center;
}
.mode-card {
  width: 200px;
  padding: 30px 20px;
  background-color: #f5f7fa;
  border: 2px solid transparent;
  border-radius: 12px;
  text-align: center;
  cursor: pointer;
  transition: all 0.25s cubic-bezier(0.175, 0.885, 0.32, 1.275);
}
.mode-card:hover {
  transform: translateY(-8px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.1);
  background-color: #ffffff;
}
.mode-card.active {
  border-color: #409eff;
  background-color: #ecf5ff;
  box-shadow: 0 4px 12px rgba(64, 158, 255, 0.2);
}
.mode-icon {
  font-size: 48px;
  margin-bottom: 16px;
  line-height: 1;
}
.mode-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}
.mode-desc {
  font-size: 12px;
  color: #909399;
  line-height: 1.4;
}

.form-tip {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  font-size: 12px;
  color: #909399;
}

.advanced-settings-page {
  position: relative;
  min-height: 400px;
}

.advanced-settings-main {
  transition: filter 0.2s ease, opacity 0.2s ease;
}

.content-blurred {
  filter: blur(8px);
  opacity: 0.35;
  pointer-events: none;
  user-select: none;
}

.advanced-inline-alert {
  margin-bottom: 16px;
}

.advanced-field-row {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.advanced-field-stack {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.advanced-field-wide {
  width: 100%;
}

.advanced-field-hint {
  font-size: 12px;
  color: #909399;
  line-height: 1.5;
}

.memory-dynamic-field {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  width: 100%;
}

.memory-dynamic-input {
  width: 220px;
  flex: 0 0 220px;
}

.memory-dynamic-unit {
  color: #606266;
  font-size: 13px;
}

.memory-dynamic-field .form-tip {
  width: 100%;
  margin-top: 0;
}

.memory-dynamic-wide {
  align-items: center;
}

.memory-experimental-tag {
  margin-left: 4px;
}

.virtio-mem-detail {
  color: #606266;
  font-size: 14px;
  line-height: 1.7;
}

.virtio-mem-detail p {
  margin: 0 0 10px;
}

.memory-basic-switch {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  width: 100%;
}

.memory-basic-switch-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.advanced-config-entry {
  width: 100%;
  min-height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid #dcdfe6;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;
}

.advanced-config-entry:hover {
  border-color: #409eff;
  background: #f5f9ff;
  box-shadow: 0 4px 12px rgba(64, 158, 255, 0.12);
}

.advanced-config-entry-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.advanced-config-entry-title {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
}

.advanced-config-entry-summary {
  font-size: 12px;
  color: #909399;
  line-height: 1.5;
  word-break: break-all;
}

.advanced-config-entry-icon {
  color: #909399;
  font-size: 16px;
  flex-shrink: 0;
}

.vm-xml-editor-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  margin-bottom: 12px;
}

.vm-xml-editor-hint {
  color: #606266;
  font-size: 13px;
}

.vm-xml-editor-textarea :deep(textarea) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}

.advanced-intro-mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.advanced-intro-card {
  width: min(620px, 100%);
  border-radius: 12px;
}

.advanced-intro-header {
  font-weight: 600;
  color: #303133;
}

.advanced-intro-content {
  color: #606266;
  line-height: 1.8;
}

.advanced-intro-content p {
  margin: 0 0 12px;
}

.advanced-intro-warning {
  color: #f56c6c;
  font-weight: 600;
}

.advanced-intro-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
}

.help-icon {
  color: #909399;
  cursor: help;
  font-size: 14px;
}

.help-icon:hover {
  color: #409eff;
}

.label-tip-icon {
  color: #c0c4cc;
  cursor: help;
  font-size: 14px;
  margin-left: 4px;
  vertical-align: middle;
}

.label-tip-icon:hover {
  color: #409eff;
}

@media (max-width: 768px) {
  .advanced-settings-page {
    min-height: 360px;
  }

  .advanced-field-row {
    flex-wrap: wrap;
  }

  .memory-dynamic-input {
    width: 100%;
    flex-basis: 100%;
  }

  .advanced-intro-mask {
    padding: 12px;
  }
}

.boot-order-container {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.boot-order-tag {
  font-size: 13px;
  cursor: default;
}

.boot-order-move {
  margin-left: 4px;
  cursor: pointer;
  font-weight: bold;
  opacity: 0.7;
}
.boot-order-move:hover {
  opacity: 1;
}

/* ==================== Cockpit 风格引导设备列表 ==================== */
.boot-devices-form-item :deep(.el-form-item__content) {
  display: block;
}

.boot-devices-panel {
  width: 100%;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  overflow: hidden;
}



.boot-device-row {
  display: flex;
  align-items: flex-start;
  padding: 12px 16px;
  border-bottom: 1px solid #ebeef5;
  gap: 12px;
  transition: background-color 0.15s;
}
.boot-device-row:last-child {
  border-bottom: none;
}
.boot-device-row:hover {
  background-color: #f5f7fa;
}
.boot-device-disabled {
  opacity: 0.45;
}

.boot-device-checkbox {
  flex-shrink: 0;
  margin-top: 2px;
}

.boot-device-info {
  flex: 1;
  min-width: 0;
}

.boot-device-type {
  display: flex;
  align-items: center;
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  margin-bottom: 4px;
}

.boot-device-details {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 4px;
}

.boot-device-label {
  flex-shrink: 0;
  font-size: 13px;
  color: #909399;
  font-weight: 500;
}

.boot-device-file {
  font-size: 13px;
  color: #606266;
  word-break: break-all;
}

.boot-device-meta {
  display: flex;
  gap: 6px;
  margin-top: 2px;
}

.boot-device-actions {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
  align-self: center;
}

.boot-devices-empty {
  padding: 24px;
  text-align: center;
  color: #909399;
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
}

:deep(.el-radio-group) {
  display: flex;
  gap: 0;
  flex-wrap: wrap;
}

:deep(.el-radio-button__inner) {
  display: flex;
  align-items: center;
}

/* ==================== 双栏布局 ==================== */
.vm-form-layout {
  display: flex;
  gap: 20px;
  min-height: 500px;
  max-height: calc(100vh - 160px);
  align-items: flex-start;
}

.vm-form-left {
  flex: 1 1 55%;
  min-width: 0;
  max-height: calc(100vh - 160px);
  overflow-y: auto;
  padding-right: 4px;
}

.vm-form-right {
  flex: 0 0 380px;
  min-width: 320px;
}

/* ==================== 编辑模式选项卡 ==================== */
.edit-tabs-bar {
  display: flex;
  gap: 4px;
  padding: 4px;
  background: #f2f3f5;
  border-radius: 10px;
  margin-bottom: 16px;
}

.edit-tab-item {
  flex: 1;
  padding: 8px 12px;
  text-align: center;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  font-size: 12px;
  color: #606266;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.edit-tab-item:hover {
  color: #303133;
  background: rgba(255,255,255,0.6);
}

.edit-tab-item.active {
  background: #fff;
  color: #409eff;
  box-shadow: 0 1px 4px rgba(0,0,0,0.08);
  font-weight: 500;
}

.edit-tab-icon {
  font-size: 16px;
}

.edit-tab-label {
  font-size: 12px;
  font-weight: inherit;
}

.edit-tab-desc {
  font-size: 10px;
  color: #909399;
}

.edit-tab-content {
  min-height: 420px;
}

.edit-tab-content .tab-content-wrapper {
  padding: 0;
}

.vm-tabs-hide-header :deep(.el-tabs__header) {
  display: none;
}

/* ==================== 步骤指示器 ==================== */
.step-indicator-bar {
  display: flex;
  gap: 6px;
  padding: 8px 12px;
  background: #f5f6f8;
  border-radius: 10px;
  margin-bottom: 16px;
  overflow-x: auto;
}

.step-dot-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  white-space: nowrap;
  font-size: 12px;
  color: #909399;
}

.step-dot-item:hover {
  color: #606266;
  background: rgba(64,158,255,0.05);
}

.step-dot-item.active {
  color: #409eff;
  background: #ecf5ff;
}

.step-dot-item.done {
  color: #67c23a;
}

.step-dot-badge {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 600;
  background: #dcdfe6;
  color: #909399;
  flex-shrink: 0;
}

.step-dot-item.active .step-dot-badge {
  background: #409eff;
  color: #fff;
}

.step-dot-item.done .step-dot-badge {
  background: #67c23a;
  color: #fff;
}

.step-dot-label {
  font-weight: 500;
}

/* ==================== 模式选择卡片（新版） ==================== */
.mode-cards-new {
  display: flex;
  gap: 16px;
  padding: 8px 0;
}

.mode-card-new {
  flex: 1;
  padding: 24px 16px;
  border: 2px solid #e5e6eb;
  border-radius: 12px;
  text-align: center;
  cursor: pointer;
  transition: all 0.25s;
  position: relative;
  background: #fff;
}

.mode-card-new:hover {
  border-color: #409eff;
  box-shadow: 0 4px 12px rgba(64,158,255,0.1);
}

.mode-card-new.selected {
  border-color: #409eff;
  background: #ecf5ff;
  box-shadow: 0 0 0 3px rgba(64,158,255,0.1);
}

.mode-card-check {
  position: absolute;
  top: 10px;
  right: 10px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: #409eff;
  display: none;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.mode-card-check .form-icon svg {
  stroke: #fff;
}

.mode-card-new.selected .mode-card-check {
  display: flex;
}

.mode-card-icon {
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-color-primary);
}

.mode-card-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 6px;
}

.mode-card-desc {
  font-size: 11px;
  color: #909399;
  line-height: 1.4;
}

/* ==================== 智能推荐横幅 ==================== */
.smart-recommend-banner {
  display: flex;
  gap: 12px;
  padding: 14px 18px;
  background: linear-gradient(135deg, #ecf5ff, #f0f7ff);
  border: 1px solid #b3d8ff;
  border-radius: 10px;
  margin-bottom: 16px;
  align-items: flex-start;
}

.rec-icon-col {
  flex-shrink: 0;
  line-height: 1;
  display: flex;
  align-items: center;
  color: var(--el-color-warning);
}

.rec-content-col {
  flex: 1;
  min-width: 0;
}

.rec-title {
  font-size: 13px;
  font-weight: 600;
  color: #409eff;
  margin-bottom: 4px;
}

.rec-desc {
  font-size: 12px;
  color: #606266;
  margin-bottom: 8px;
  line-height: 1.5;
}

.rec-actions {
  display: flex;
  gap: 8px;
}

/* ==================== 分区标题 ==================== */
.section-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f0f0f0;
}

.section-icon-box {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.section-icon-box.basic { background: #ecf5ff; }
.section-icon-box.hardware { background: #e8f8f2; }
.section-icon-box.storage { background: #fef3e6; }
.section-icon-box.network { background: #ecf5ff; }
.section-icon-box.system { background: #fef0f0; }
.section-icon-box.advanced { background: #f3f0ff; }

.section-info {
  flex: 1;
  min-width: 0;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.section-desc {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

/* ==================== 新版步骤头部 ==================== */
.step-pane-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #ebeef5;
}

.step-pane-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.step-pane-icon.basic { background: #ecf5ff; }
.step-pane-icon.hardware { background: #e8f8f2; }
.step-pane-icon.storage { background: #fef3e6; }
.step-pane-icon.network { background: #e6f7ff; }
.step-pane-icon.system { background: #fef0f0; }
.step-pane-icon.advanced { background: #f3f0ff; }

.step-pane-info {
  flex: 1;
  min-width: 0;
}

.step-pane-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 2px;
}

.step-pane-desc {
  font-size: 12px;
  color: #909399;
}

.step-pane-body {
  padding: 0 2px;
}

/* ==================== 表单区块卡片 ==================== */
.form-section-card {
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 10px;
  margin-bottom: 16px;
  overflow: hidden;
  transition: border-color 0.2s ease;
}

.form-section-card:hover {
  border-color: #c0c4cc;
}

.form-section-card:last-child {
  margin-bottom: 0;
}

.form-section-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: #fafbfc;
  border-bottom: 1px solid #ebeef5;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.form-section-card-header .el-icon {
  font-size: 15px;
  color: #409eff;
}

.form-section-card-body {
  padding: 16px;
}

.form-section-card-body .el-form-item {
  margin-bottom: 16px;
}

.form-section-card-body .el-form-item:last-child {
  margin-bottom: 0;
}

.form-section-card-body .el-row {
  margin-bottom: 16px;
}

.form-section-card-body .el-row:last-child {
  margin-bottom: 0;
}

/* ==================== 操作系统快捷选择卡片 ==================== */
.os-quick-card {
  flex: 1;
  padding: 14px 12px;
  border: 2px solid #e5e6eb;
  border-radius: 10px;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s;
  background: #fff;
}

.os-quick-card:hover {
  border-color: #409eff;
  box-shadow: 0 2px 8px rgba(64,158,255,0.08);
}

.os-quick-card.selected {
  border-color: #409eff;
  background: #ecf5ff;
}

.os-qc-icon {
  font-size: 26px;
  margin-bottom: 6px;
}

.os-qc-name {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.os-qc-examples {
  font-size: 10px;
  color: #909399;
  margin-top: 4px;
}

/* ==================== 预览面板 ==================== */
.preview-panel {
  background: #fff;
  border: 1px solid #e5e6eb;
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.04);
  overflow: hidden;
  position: sticky;
  top: 0;
}

.preview-header {
  padding: 12px 16px;
  background: linear-gradient(135deg, #1d2332, #2a3a52);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
}

.preview-body {
  padding: 12px 16px;
}

.preview-section {
  margin-bottom: 12px;
}

.preview-section:last-child {
  margin-bottom: 0;
}

.preview-section-title {
  font-size: 10px;
  font-weight: 600;
  color: #909399;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 6px;
  padding-bottom: 4px;
  border-bottom: 1px solid #f0f0f0;
}

.preview-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 12px;
}

.pr-label {
  color: #909399;
}

.pr-value {
  font-weight: 500;
  text-align: right;
}

.pr-value.highlight {
  color: #409eff;
}

.preview-total {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 2px solid #e5e6eb;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.total-label {
  font-size: 12px;
  font-weight: 600;
  color: #303133;
}

.total-values {
  text-align: right;
}

.total-value {
  font-size: 12px;
  font-weight: 700;
  color: #409eff;
  margin-bottom: 2px;
}

/* ==================== 隐藏原有模式选择 ==================== */
.mode-selection-wrapper,
.mode-cards,
.mode-icon,
.mode-title,
.mode-desc {
  display: none;
}

/* ==================== 隐藏原有步骤条 ==================== */
.vm-create-wizard {
  display: none;
}

/* ==================== 响应式 ==================== */
@media (max-width: 1000px) {
  .vm-form-layout {
    flex-direction: column;
    max-height: none;
    gap: 14px;
  }
  .vm-form-left {
    flex: 1 1 auto;
    max-height: 60vh;
    overflow-y: auto;
  }
  .vm-form-right {
    flex: 0 0 auto;
    width: 100%;
    min-width: auto;
  }
  .preview-panel {
    position: static;
  }
  .mode-cards-new {
    flex-direction: column;
  }
}

/* ==================== 手机独立布局 ==================== */
@media (max-width: 768px) {
  /* 对话框全屏 */
  .vm-dialog-new :deep(.el-dialog) {
    width: 100% !important;
    max-width: 100vw !important;
    margin: 0 !important;
    border-radius: 0 !important;
    height: 100vh !important;
    max-height: 100vh !important;
    display: flex !important;
    flex-direction: column !important;
  }

  .vm-dialog-new :deep(.el-dialog__header) {
    padding: 12px 14px !important;
  }

  .vm-dialog-new :deep(.el-dialog__title) {
    font-size: 15px !important;
  }

  .vm-dialog-new :deep(.el-dialog__body) {
    padding: 10px !important;
    flex: 1 !important;
    overflow-y: auto !important;
    max-height: none !important;
  }

  .vm-dialog-new :deep(.el-dialog__footer) {
    padding: 10px 14px !important;
  }

  /* 双栏布局 → 单列 */
  .vm-form-layout {
    flex-direction: column;
    max-height: none;
    gap: 10px;
    min-height: auto;
  }

  .vm-form-left {
    flex: 1 1 auto;
    max-height: none;
    overflow-y: visible;
    padding-right: 0;
  }

  .vm-form-right {
    flex: 0 0 auto;
    width: 100%;
    min-width: auto;
  }

  .preview-panel {
    position: static;
  }

  /* 表单标签在上方 */
  .vm-form :deep(.el-form-item) {
    display: flex;
    flex-direction: column;
    margin-bottom: 14px;
  }

  .vm-form :deep(.el-form-item__label) {
    width: 100% !important;
    text-align: left !important;
    justify-content: flex-start !important;
    padding-bottom: 4px;
    font-size: 12px;
  }

  .vm-form :deep(.el-form-item__content) {
    margin-left: 0 !important;
    width: 100%;
  }

  /* 输入控件全宽 */
  .vm-form :deep(.el-input-number),
  .vm-form :deep(.el-select),
  .vm-form :deep(.el-input) {
    width: 100% !important;
  }

  .vm-form :deep(.el-input-number .el-input__inner) {
    text-align: left;
  }

  /* 行内表单列折行 */
  .vm-form :deep(.el-row) {
    flex-direction: column;
    gap: 10px;
  }

  .vm-form :deep(.el-col) {
    max-width: 100% !important;
    flex: 0 0 100% !important;
    margin-bottom: 0;
  }

  /* 编辑选项卡横向滚动 */
  .edit-tabs-bar {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    flex-wrap: nowrap;
    gap: 2px;
    padding: 3px;
  }

  .edit-tab-item {
    flex: 0 0 auto;
    padding: 6px 10px;
    flex-direction: row;
    gap: 4px;
    font-size: 11px;
    white-space: nowrap;
  }

  .edit-tab-icon {
    font-size: 14px;
  }

  .edit-tab-label {
    font-size: 11px;
  }

  /* 标签内容 */
  .tab-content-wrapper {
    padding: 0;
    min-height: auto;
  }

  /* 步骤指示器 */
  .step-indicator-bar {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    flex-wrap: nowrap;
    padding: 6px 8px;
  }

  .step-dot-item {
    flex: 0 0 auto;
    padding: 4px 8px;
    font-size: 11px;
  }

  /* 模式选择卡片 */
  .mode-cards-new {
    flex-direction: column;
    gap: 10px;
  }

  .mode-card-new {
    padding: 16px 12px;
  }

  /* 操作系统快捷选择 */
  .os-quick-card {
    padding: 10px 8px;
  }

  .os-qc-icon {
    font-size: 20px;
  }

  .os-qc-name {
    font-size: 11px;
  }

  /* 表单分区卡片 */
  .form-section-card-header {
    padding: 10px 12px;
    font-size: 12px;
  }

  .form-section-card-body {
    padding: 10px;
  }

  /* 分割线 */
  .vm-form :deep(.el-divider__text) {
    font-size: 12px;
  }

  /* 磁盘表格（编辑模式） */
  .edit-tab-content :deep(.el-table) {
    font-size: 11px;
  }

  .edit-tab-content :deep(.el-table td),
  .edit-tab-content :deep(.el-table th) {
    padding: 6px 4px;
  }

  /* 引导设备项 */
  .boot-device-row {
    flex-wrap: wrap;
    padding: 10px 12px;
  }

  .boot-device-actions {
    width: 100%;
    justify-content: flex-end;
    margin-top: 4px;
  }
}

@media (max-width: 480px) {
  .vm-dialog-new :deep(.el-dialog__header) {
    padding: 10px 10px !important;
  }

  .vm-dialog-new :deep(.el-dialog__body) {
    padding: 6px !important;
  }

  .vm-dialog-new :deep(.el-dialog__footer) {
    padding: 8px 10px !important;
  }

  .edit-tab-item {
    padding: 4px 8px;
    font-size: 10px;
  }

  .edit-tab-icon {
    font-size: 12px;
  }

  .edit-tab-label {
    font-size: 10px;
  }
}


html.dark .edit-tabs-bar { background: var(--app-bg-elevated); }
html.dark .edit-tab-item { color: var(--el-text-color-secondary); }
html.dark .edit-tab-item:hover { color: var(--el-text-color-primary); background: rgba(255,255,255,0.06); }
html.dark .edit-tab-item.active { background: var(--el-color-primary); color: #fff; box-shadow: 0 1px 8px rgba(64,158,255,0.25); }
html.dark .edit-tab-desc { color: var(--el-text-color-placeholder); }

html.dark .step-indicator-bar { background: var(--app-bg-elevated); }
html.dark .step-dot-item { color: var(--el-text-color-secondary); }
html.dark .step-dot-item:hover { color: var(--el-text-color-primary); background: rgba(64,158,255,0.08); }
html.dark .step-dot-item.active { color: var(--el-color-primary); background: rgba(64,158,255,0.12); }
html.dark .step-dot-item.done { color: #95de64; }
html.dark .step-dot-badge { background: rgba(255,255,255,0.08); color: var(--el-text-color-secondary); }

html.dark .mode-card-new { border-color: var(--app-border-light); background: var(--app-bg-card); }
html.dark .mode-card-new.selected { border-color: var(--el-color-primary); background: rgba(64,158,255,0.08); }
html.dark .mode-card-title { color: var(--el-text-color-primary); }
html.dark .mode-card-desc { color: var(--el-text-color-secondary); }

html.dark .smart-recommend-banner { background: rgba(64,158,255,0.06); border-color: rgba(64,158,255,0.18); }
html.dark .rec-title { color: var(--el-color-primary-light-3); }
html.dark .rec-desc { color: var(--el-text-color-regular); }

html.dark .section-header { border-bottom-color: var(--app-border-light); }
html.dark .section-icon-box.basic { background: rgba(64,158,255,0.12); }
html.dark .section-icon-box.hardware { background: rgba(82,196,26,0.12); }
html.dark .section-icon-box.storage { background: rgba(250,140,22,0.12); }
html.dark .section-icon-box.network { background: rgba(64,158,255,0.12); }
html.dark .section-icon-box.system { background: rgba(255,77,79,0.12); }
html.dark .section-icon-box.advanced { background: rgba(124,77,255,0.12); }
html.dark .section-title { color: var(--el-text-color-primary); }
html.dark .section-desc { color: var(--el-text-color-secondary); }

html.dark .step-pane-header { border-bottom-color: var(--app-border-light); }
html.dark .step-pane-icon.basic { background: rgba(64,158,255,0.12); }
html.dark .step-pane-icon.hardware { background: rgba(82,196,26,0.12); }
html.dark .step-pane-icon.storage { background: rgba(250,140,22,0.12); }
html.dark .step-pane-icon.network { background: rgba(64,158,255,0.12); }
html.dark .step-pane-icon.system { background: rgba(255,77,79,0.12); }
html.dark .step-pane-icon.advanced { background: rgba(124,77,255,0.12); }
html.dark .step-pane-title { color: var(--el-text-color-primary); }
html.dark .step-pane-desc { color: var(--el-text-color-secondary); }

html.dark .form-section-card { background: var(--app-bg-card); border-color: var(--app-border-light); }
html.dark .form-section-card:hover { border-color: var(--el-text-color-placeholder); }
html.dark .form-section-card-header { background: var(--app-bg-elevated); border-bottom-color: var(--app-border-light); color: var(--el-text-color-primary); }

html.dark .os-quick-card { border-color: var(--app-border-light); background: var(--app-bg-card); }
html.dark .os-quick-card.selected { border-color: var(--el-color-primary); background: rgba(64,158,255,0.08); }
html.dark .os-qc-name { color: var(--el-text-color-primary); }
html.dark .os-qc-examples { color: var(--el-text-color-secondary); }

html.dark .preview-panel { background: var(--app-bg-card); border-color: var(--app-border-light); box-shadow: var(--app-shadow-md); }
html.dark .preview-section-title { color: var(--el-text-color-secondary); border-bottom-color: var(--app-border-extralight); }
html.dark .pr-label { color: var(--el-text-color-secondary); }
html.dark .pr-value.highlight { color: var(--el-color-primary-light-3); }
html.dark .preview-total { border-top-color: var(--app-border-light); }
html.dark .total-label { color: var(--el-text-color-primary); }
html.dark .total-value { color: var(--el-color-primary-light-3); }

/* 多网口样式 */
.extra-nic-row {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
  background: var(--el-fill-color-lighter);
}
.extra-nic-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
</style>
