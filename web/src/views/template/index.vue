<template>
  <div class="template-list-container">
    <div class="page-header">
      <h2>模板管理</h2>
      <div class="header-actions">
        <el-button class="btn-outline-custom" @click="expandAll">全部展开</el-button>
        <el-button class="btn-outline-custom" @click="collapseAll">全部收起</el-button>
        <el-button class="btn-outline-custom" @click="openImportDialog">导入模板包</el-button>
        <el-button class="btn-outline-custom" :loading="loading" @click="fetchData">刷新</el-button>
      </div>
    </div>

    <div v-loading="loading" class="family-cards-container">
      <div
        v-for="family in families"
        :key="family.template_uid"
        class="family-card"
      >
        <div class="family-header">
          <div class="family-id">
            <div class="family-icon" :class="familyTypeClass(family.type)">
              <span>{{ familyTypeEmoji(family.type) }}</span>
            </div>
            <div>
              <div class="family-title">{{ family.root_name || family.template_uid }}</div>
              <div class="family-meta">模板族 {{ family.template_uid }}</div>
            </div>
          </div>
          <div class="family-stats">
            <span class="family-stat">🌿 <strong>{{ family.node_count }}</strong> 节点</span>
            <span class="family-stat">💻 <strong>{{ family.vm_count }}</strong> 关联 VM</span>
            <span class="family-stat">💾 <strong>{{ family.disk_size }}</strong> 磁盘</span>
          </div>
        </div>

        <ul class="node-list">
          <template v-for="node in family.visible_nodes" :key="node.node_id">
            <li
              class="node-item"
              :class="{ disabled: node.disabled }"
              :data-node="node.node_id"
              :data-parent="node.parent_node_id || ''"
            >
              <div class="node-row" @click="onNodeRowClick($event, node)">
                <div class="tree-guides" v-html="renderTreeGuides(node)"></div>
                <span
                  class="toggle-btn"
                  :class="{ 'no-children': !node.has_children, collapsed: !isNodeExpanded(node) }"
                  @click.stop="toggleNode(node)"
                >▼</span>
                <div class="level-bar" :class="'l' + (node.depth % 5)"></div>
                <div class="node-content">
                  <div class="node-identity">
                    <div class="node-name">{{ node.admin_name || node.name }}</div>
                    <div class="node-file">{{ node.name }}.qcow2</div>
                  </div>
                  <span class="os-tag" :class="osTypeClass(node.type)">{{ templateTypeLabel(node.type) }}</span>
                  <span v-if="templateCategoryLabel(node.type, node.category)" class="tag tag-default">
                    {{ templateCategoryLabel(node.type, node.category) }}
                  </span>
                  <span
                    class="tag"
                    :class="node.disabled ? 'tag-danger' : node.clone_visible ? 'tag-success' : 'tag-warning'"
                  >{{ node.disabled ? '已禁用' : node.clone_visible ? '用户可见' : '仅管理员' }}</span>
                  <div class="node-vm-stat">
                    <span class="vm-count">{{ node.tree_vm_count || node.direct_vm_count || 0 }}</span>
                    <span class="vm-label">VM</span>
                  </div>
                  <div class="node-size" title="虚拟大小 / 实际占用">
                    <span class="size-value">{{ node.virtual_size || '-' }}</span>
                    <span class="size-label">{{ node.actual_size || '-' }}</span>
                  </div>
                  <span class="tag" :class="node.exported ? 'tag-info' : 'tag-default'">{{ node.exported ? '已导出' : '未导出' }}</span>
                  <span class="tag" :class="hashStatusTagClass(node.hash_status)">{{ hashStatusText(node.hash_status) }}</span>
                  <span v-if="node.type === 'linux'" class="tag" :class="linuxInitStatusTagClass(node.linux_init_status)">{{ linuxInitStatusText(node.linux_init_status) }}</span>
                </div>
                <div class="node-actions">
                  <el-button
                    v-if="node.is_root"
                    size="small"
                    :loading="exportingName === `${node.name}:root`"
                    @click.stop="handleExport(node, 'root')"
                  >导出整树</el-button>
                  <el-button
                    size="small"
                    :loading="exportingName === `${node.name}:node`"
                    @click.stop="handleExport(node, 'node')"
                  >导出节点</el-button>
                  <el-button
                    v-if="node.exported"
                    size="small"
                    @click.stop="handleDownloadExport(node)"
                  >下载导出包</el-button>
                  <el-button
                    v-if="node.exported"
                    size="small"
                    type="warning"
                    :loading="deletingExportName === node.name"
                    @click.stop="handleDeleteExport(node)"
                  >删除导出包</el-button>
                  <el-button
                    size="small"
                    @click.stop="openPublishDialog(node)"
                  >设置</el-button>
                  <el-button
                    v-if="node.type === 'linux'"
                    size="small"
                    :loading="preparingLinuxName === node.name"
                    @click.stop="handlePrepareLinux(node)"
                  >离线预处理</el-button>
                  <el-button
                    size="small"
                    type="danger"
                    @click.stop="openDeleteDialog(node)"
                  >删除</el-button>
                </div>
              </div>
              <div
                v-if="node.has_children"
                class="chain-summary"
                :style="{ display: isNodeExpanded(node) ? 'none' : '' }"
                @click="toggleNode(node)"
              >
                📌 收起 · 最新派生链：
                <template v-for="(ancestor, idx) in node.chain_labels" :key="idx">
                  <span v-if="idx > 0" class="chain-arrow">→</span>
                  <span :class="idx === node.chain_labels.length - 1 ? 'chain-leaf' : ''">{{ ancestor }}</span>
                </template>
                <span class="chain-depth">(深{{ node.chain_depth || node.depth + 1 }}层 / 共{{ node.chain_total || 1 }}节点)</span>
              </div>
            </li>
          </template>
        </ul>
      </div>

      <div v-if="!loading && families.length === 0" class="empty-state">
        <div class="empty-icon">📦</div>
        <div>暂无模板</div>
        <div class="empty-hint">可通过「虚拟机列表 → 更多 → 制作模板」创建首个模板</div>
      </div>
    </div>

    <el-dialog v-model="importDialogVisible" title="导入模板包" width="860px" :close-on-click-modal="false" append-to-body @close="onImportDialogClose">
      <el-form :model="importForm" label-width="110px">
        <el-form-item label="导入来源" required>
          <el-radio-group v-model="importForm.import_mode">
            <el-radio value="upload">上传文件</el-radio>
            <el-radio value="source_path">主机绝对路径</el-radio>
          </el-radio-group>
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>仅支持新版 .tar.gz / .tgz 模板链路包</div>
        </el-form-item>
        <el-form-item v-if="importForm.import_mode === 'upload'" label="模板包" required>
          <el-upload
            :auto-upload="false"
            :limit="1"
            :file-list="importFileList"
            accept=".tar.gz,.tgz"
            @change="handleImportFileChange"
            @remove="handleImportFileRemove"
            @exceed="handleImportFileExceed"
          >
            <el-button type="primary">选择文件</el-button>
          </el-upload>
        </el-form-item>
        <el-form-item v-else label="主机路径" required>
          <el-input v-model="importForm.source_path" placeholder="/data/template/demo-template-export.tar.gz" />
        </el-form-item>
        <el-progress v-if="importUploading" :percentage="importProgress" :stroke-width="16" />
      </el-form>

      <div v-if="importPreview" class="import-preview">
        <el-alert :type="importPreview.can_import ? 'success' : 'error'" :closable="false" show-icon style="margin-bottom: 12px;">
          {{ importPreview.message }}，模式：{{ importPreview.mode === 'update' ? '增量更新' : '新模板树' }}
        </el-alert>
        <el-table :data="importPreview.nodes" border size="small" max-height="320">
          <el-table-column label="节点" min-width="220">
            <template #default="{ row }">
              <div class="node-title">
                <strong>{{ row.admin_name || row.name }}</strong>
                <span>{{ row.name }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="display_name" label="用户侧显示" min-width="160" />
          <el-table-column label="分类" min-width="120">
            <template #default="{ row }">{{ templateGroupLabel(row.type, row.category) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="120" align="center">
            <template #default="{ row }">
              <el-tag :type="row.conflict_reason ? 'danger' : row.exists ? 'info' : 'success'" size="small">
                {{ row.conflict_reason ? '冲突' : row.exists ? '已存在' : '将导入' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="用户可见" width="90" align="center">
            <template #default="{ row }">{{ row.clone_visible ? '是' : '否' }}</template>
          </el-table-column>
          <el-table-column label="禁用" width="80" align="center">
            <template #default="{ row }">{{ row.disabled ? '是' : '否' }}</template>
          </el-table-column>
          <el-table-column prop="md5" label="MD5" min-width="160" show-overflow-tooltip />
          <el-table-column prop="sha256" label="SHA256" min-width="180" show-overflow-tooltip />
          <el-table-column prop="conflict_reason" label="提示" min-width="160" />
        </el-table>
      </div>

      <template #footer>
        <el-button @click="closeImportDialog">取消</el-button>
        <el-button type="primary" :loading="importSubmitting" @click="handleImportPreview">解析预览</el-button>
        <el-button type="success" :disabled="!importPreview?.can_import" :loading="importConfirming" @click="handleImportConfirm">确认导入</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="publishDialogVisible" title="发布设置" width="520px" :close-on-click-modal="false" append-to-body>
      <el-form :model="publishForm" label-width="120px">
        <el-form-item label="模板文件">
          <el-input :model-value="publishForm.name" disabled />
        </el-form-item>
        <el-form-item label="管理员名称">
          <el-input v-model="publishForm.admin_name" placeholder="管理员侧名称" />
        </el-form-item>
        <el-form-item label="用户侧显示">
          <el-input v-model="publishForm.display_name" placeholder="从模板克隆下拉框中显示的标题" />
        </el-form-item>
        <el-form-item v-if="publishForm.type === 'linux' || publishForm.type === 'windows'" :label="publishForm.type === 'windows' ? 'Windows 分类' : 'Linux 分类'">
          <el-select
            v-model="publishForm.category"
            filterable
            :placeholder="publishCategoryPlaceholder"
            style="width: 100%;"
          >
            <el-option
              v-for="item in publishCategoryOptions"
              :key="item"
              :label="item"
              :value="item"
            />
          </el-select>
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>{{ publishCategoryTip }}</div>
        </el-form-item>
        <el-form-item label="启用克隆">
          <el-switch v-model="publishForm.clone_visible" active-text="用户可见" inactive-text="仅管理员" :disabled="publishForm.disabled" />
        </el-form-item>
        <el-form-item label="禁用模板">
          <el-switch v-model="publishForm.disabled" active-text="禁用" inactive-text="可用" />
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>禁用后管理员新建虚拟机下拉框也不会显示该模板</div>
        </el-form-item>
        <el-divider content-position="left">默认创建配置</el-divider>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="CPU 核心" label-width="90px">
              <el-input-number v-model="publishForm.vcpu" :min="0" :max="128" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="内存(GB)" label-width="90px">
              <el-input-number v-model="publishForm.ram" :min="0" :max="1024" style="width: 100%;" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="磁盘(GB)" label-width="90px">
              <el-input-number v-model="publishForm.disk_size" :min="0" :max="8192" style="width: 100%;" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="磁盘驱动" label-width="90px">
              <el-select v-model="publishForm.disk_bus" clearable placeholder="未设置" style="width: 100%;">
                <el-option label="VirtIO" value="virtio" />
                <el-option label="SCSI" value="scsi" />
                <el-option label="SATA" value="sata" />
                <el-option label="IDE" value="ide" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="网卡类型">
          <el-select v-model="publishForm.nic_model" clearable placeholder="未设置" style="width: 100%;">
            <el-option label="VirtIO" value="virtio" />
            <el-option label="e1000e (Intel)" value="e1000e" />
            <el-option label="rtl8139" value="rtl8139" />
          </el-select>
        </el-form-item>
        <el-form-item label="显示设备">
          <el-select v-model="publishForm.video_model" clearable placeholder="未设置" style="width: 100%;">
            <el-option label="VirtIO（高性能）" value="virtio" />
            <el-option label="VGA（兼容模式）" value="vga" />
            <el-option label="VMVGA（VMware 嵌套）" value="vmvga" />
            <el-option label="Cirrus（保守排障）" value="cirrus" />
            <el-option label="None（禁用虚拟显示）" value="none" />
          </el-select>
        </el-form-item>
        <el-form-item label="CPU 拓扑">
          <el-select v-model="publishForm.cpu_topology_mode" clearable placeholder="未设置" style="width: 100%;">
            <el-option label="自动（Windows 使用单插槽多核心）" value="auto" />
            <el-option label="单插槽多核心" value="single_socket" />
            <el-option label="宿主默认拓扑" value="host_default" />
          </el-select>
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>填写后，新建 VM 选择该模板时会自动带出这些默认值；填 0 或留空表示不指定</div>
        </el-form-item>
        <el-form-item label="首次重启">
          <el-select v-model="publishForm.first_boot_reboot_mode" clearable placeholder="未设置" style="width: 100%;">
            <el-option label="普通重启" value="normal" />
            <el-option label="宿主冷启动" value="cold" />
          </el-select>
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>Windows 模板 OOBE 自动重启黑屏时可设置为宿主冷启动</div>
        </el-form-item>
        <el-form-item v-if="publishForm.type === 'linux'" label="启动后命令">
          <el-input
            v-model="publishForm.post_boot_command"
            type="textarea"
            :rows="3"
            placeholder="克隆后首次启动时执行的自定义 Shell 命令（可多行）"
          />
          <div class="form-tip"><el-icon><InfoFilled /></el-icon>命令将以 root 权限执行，仅首次启动时运行</div>
          <el-checkbox v-model="publishForm.post_boot_blocking" :disabled="!publishForm.post_boot_command" style="margin-top: 6px;">
            等待命令执行完毕后再启动 SSH
          </el-checkbox>
          <div v-if="publishForm.post_boot_blocking" class="form-tip" style="color: #E6A23C;"><el-icon><InfoFilled /></el-icon>启用后系统启动期间将显示“正在启动 QVM 初始化服务”，用户在此期间无法通过 SSH 登录</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="publishDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="publishSaving" @click="handleSavePublish">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deleteDialogVisible" title="删除模板链路" width="820px" :close-on-click-modal="false" append-to-body>
      <div v-loading="deleteLoading">
        <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 16px;">
          {{ deleteModeTip }}
        </el-alert>
        <el-radio-group v-model="deleteForm.mode" style="margin-bottom: 14px;">
          <el-radio-button value="cascade">级联删除</el-radio-button>
          <el-radio-button value="promote_children" :disabled="!deleteForm.parentTemplate">仅删除当前节点并提升子节点</el-radio-button>
          <el-radio-button value="promote_children_hot" :disabled="!deleteForm.parentTemplate">热删除当前节点并提升子节点</el-radio-button>
        </el-radio-group>
        <el-descriptions :column="1" border style="margin-bottom: 14px;">
          <el-descriptions-item label="起始模板">{{ deleteForm.name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="上级模板">{{ deleteForm.parentTemplate ? (deleteForm.parentTemplate.admin_name || deleteForm.parentTemplate.name) : '无' }}</el-descriptions-item>
          <el-descriptions-item label="将删除节点">{{ isPromoteDeleteMode ? 1 : deleteForm.templates.length }}</el-descriptions-item>
          <el-descriptions-item v-if="isPromoteDeleteMode" label="提升子模板">{{ deleteForm.promotedTemplates.length }}</el-descriptions-item>
          <el-descriptions-item v-if="isPromoteDeleteMode" label="重定向 VM">{{ deleteForm.rebasedVMs.length }}</el-descriptions-item>
          <el-descriptions-item label="关联虚拟机">{{ deleteForm.relatedVMs.length }}</el-descriptions-item>
        </el-descriptions>
        <el-alert
          v-if="deleteForm.mode === 'promote_children' && deleteForm.promoteBlockers.length"
          type="error"
          :closable="false"
          show-icon
          style="margin-bottom: 14px;"
        >
          {{ deleteForm.promoteBlockers.join('；') }}
        </el-alert>
        <el-alert
          v-if="deleteForm.mode === 'promote_children_hot' && deleteForm.promoteHotBlockers.length"
          type="error"
          :closable="false"
          show-icon
          style="margin-bottom: 14px;"
        >
          {{ deleteForm.promoteHotBlockers.join('；') }}
        </el-alert>
        <el-table :data="isPromoteDeleteMode ? deleteForm.templates.slice(0, 1) : deleteForm.templates" border size="small" max-height="180" style="margin-bottom: 14px;">
          <el-table-column label="模板节点" min-width="220">
            <template #default="{ row }">{{ row.admin_name || row.name }}（{{ row.name }}）</template>
          </el-table-column>
          <el-table-column prop="display_name" label="用户侧显示" min-width="160" />
          <el-table-column label="VM" width="100" align="center">
            <template #default="{ row }">{{ row.direct_vm_count || 0 }} / {{ row.tree_vm_count || 0 }}</template>
          </el-table-column>
        </el-table>
        <el-table v-if="isPromoteDeleteMode && deleteForm.promotedTemplates.length" :data="deleteForm.promotedTemplates" border size="small" max-height="160" style="margin-bottom: 14px;">
          <el-table-column label="将提升的子模板" min-width="220">
            <template #default="{ row }">{{ row.admin_name || row.name }}（{{ row.name }}）</template>
          </el-table-column>
          <el-table-column label="新上级" min-width="180">
            <template #default>{{ deleteForm.parentTemplate?.admin_name || deleteForm.parentTemplate?.name || '-' }}</template>
          </el-table-column>
        </el-table>
        <el-table v-if="deleteForm.relatedVMs.length" :data="deleteForm.relatedVMs" border size="small" max-height="220">
          <el-table-column prop="name" label="虚拟机名称" min-width="180" />
          <el-table-column prop="template" label="来源模板" min-width="160" />
          <el-table-column v-if="isPromoteDeleteMode" label="处理方式" min-width="130">
            <template #default="{ row }">{{ deleteVMActionText(row) }}</template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="110" align="center" />
          <el-table-column prop="ip" label="IP 地址" min-width="150" />
        </el-table>
      </div>
      <template #footer>
        <el-button @click="deleteDialogVisible = false">取消</el-button>
        <el-button
          type="danger"
          :disabled="deleteButtonDisabled"
          :loading="deleteSubmitting"
          @click="handleDelete"
        >{{ isPromoteDeleteMode ? '确认删除当前节点' : '确认删除链路' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getTemplateList,
  deleteTemplate,
  deleteTemplateExport,
  getTemplateDeletePreview,
  getTemplateExportDownloadUrl,
  updateTemplatePublish,
  previewImportTemplate,
  confirmImportTemplate,
  exportTemplate,
} from '@/api/vm'
import { prepareImportedLinuxTemplate, templateUploadInit, templateUploadChunk, templateUploadComplete, templateUploadCancel } from '@/api/template'
import { ChunkUploader } from '@/utils/chunkUploader'
import {
  LINUX_TEMPLATE_CATEGORY_OPTIONS,
  WINDOWS_TEMPLATE_CATEGORY_OPTIONS,
  normalizeTemplateCategory,
  normalizeTemplateType,
  templateCategoryLabel,
  templateGroupLabel,
  templateTypeLabel,
} from '@/utils/templateCategory'

const tableData = ref([])
const families = ref([])
const nodeExpandState = ref({})
const loading = ref(false)
const exportingName = ref('')
const deletingExportName = ref('')
const preparingLinuxName = ref('')

const importDialogVisible = ref(false)
const importSubmitting = ref(false)
const importConfirming = ref(false)
const importUploading = ref(false)
const importProgress = ref(0)
const importFileList = ref([])
const importRawFile = ref(null)
const importPreview = ref(null)
const currentTemplateUploader = ref(null)
const templateSessionKey = ref('') // 分片上传产出的临时包路径，未导入而关闭时据此清理
const importForm = ref({ import_mode: 'upload', source_path: '' })

const publishDialogVisible = ref(false)
const publishSaving = ref(false)
const publishForm = ref({
  name: '',
  type: '',
  admin_name: '',
  display_name: '',
  clone_visible: false,
  disabled: false,
  category: '',
  vcpu: 0,
  ram: 0,
  disk_size: 0,
  disk_bus: '',
  nic_model: '',
  video_model: '',
  cpu_topology_mode: '',
  first_boot_reboot_mode: '',
  post_boot_command: '',
  post_boot_blocking: false,
})

const deleteDialogVisible = ref(false)
const deleteLoading = ref(false)
const deleteSubmitting = ref(false)
const emptyDeleteForm = () => ({
  name: '',
  mode: 'cascade',
  templates: [],
  relatedVMs: [],
  parentTemplate: null,
  promotedTemplates: [],
  rebasedVMs: [],
  canPromote: false,
  promoteBlockers: [],
  canPromoteHot: false,
  promoteHotBlockers: [],
})
const deleteForm = ref(emptyDeleteForm())

const isPromoteDeleteMode = computed(() => deleteForm.value.mode === 'promote_children' || deleteForm.value.mode === 'promote_children_hot')
const deleteModeTip = computed(() => {
  if (deleteForm.value.mode === 'promote_children_hot') {
    return '热删除会在线切换运行中 VM 的 backing，尽量不关机；失败时不会删除当前模板节点。'
  }
  if (deleteForm.value.mode === 'promote_children') {
    return '仅删除当前节点会安全 rebase 直接子模板和直接 VM，所有关联 VM 必须先关机。'
  }
  return '级联删除会移除当前节点及其所有子节点，并按确认范围删除关联虚拟机。'
})
const deleteButtonDisabled = computed(() => {
  if (deleteForm.value.mode === 'promote_children') return !deleteForm.value.canPromote
  if (deleteForm.value.mode === 'promote_children_hot') return !deleteForm.value.canPromoteHot
  return false
})

const publishCategoryOptions = computed(() => publishForm.value.type === 'windows'
  ? WINDOWS_TEMPLATE_CATEGORY_OPTIONS
  : LINUX_TEMPLATE_CATEGORY_OPTIONS)
const publishCategoryPlaceholder = computed(() => publishForm.value.type === 'windows'
  ? '默认归入 WindowsServer2022，可选择 WindowsServer2025 / Windows11 / Windows10 / WindowsServer2012R2 / 其它'
  : '默认归入 Ubuntu，可选择 Debian、CentOS')
const publishCategoryTip = computed(() => publishForm.value.type === 'windows'
  ? 'Windows 模板按版本分类展示，2012 R2 会保留模板默认硬件配置用于克隆'
  : 'Linux 模板按发行版分类展示')

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getTemplateList()
    tableData.value = res.data || []
    families.value = buildFamilies(tableData.value)
    nodeExpandState.value = {}
    refreshVisibleNodes()
  } catch (err) {
    console.error(err)
  } finally {
    loading.value = false
  }
}

const hashStatusText = (status) => ({ ok: '已记录', missing: '缺失', size_mismatch: '大小变化' }[status] || '未知')
const hashStatusTagClass = (status) => ({ ok: 'tag-success', missing: 'tag-warning', size_mismatch: 'tag-danger' }[status] || 'tag-default')
const linuxInitStatusText = (status) => ({ ready: '离线就绪', failed: '预处理失败', unknown: '待预处理' }[status] || '待预处理')
const linuxInitStatusTagClass = (status) => ({ ready: 'tag-success', failed: 'tag-danger', unknown: 'tag-warning' }[status] || 'tag-warning')

const familyTypeClass = (type) => normalizeTemplateType(type)
const familyTypeEmoji = (type) => ({ windows: '🪟', fnos: '📦', other: '💾' }[normalizeTemplateType(type)] || '🐧')
const osTypeClass = (type) => 'os-' + normalizeTemplateType(type)

const buildFamilies = (items) => {
  const nodes = items || []
  const byNodeId = {}
  nodes.forEach(n => { byNodeId[n.node_id] = n })

  const childrenMap = {}
  nodes.forEach(n => {
    const pid = n.parent_node_id || ''
    if (pid && byNodeId[pid]) {
      if (!childrenMap[pid]) childrenMap[pid] = []
      childrenMap[pid].push(n.node_id)
    }
  })

  const depthMap = {}
  const calcDepth = (nodeId) => {
    if (depthMap[nodeId] !== undefined) return depthMap[nodeId]
    const node = byNodeId[nodeId]
    if (!node || !node.parent_node_id || !byNodeId[node.parent_node_id]) {
      depthMap[nodeId] = 0
      return 0
    }
    depthMap[nodeId] = calcDepth(node.parent_node_id) + 1
    return depthMap[nodeId]
  }
  nodes.forEach(n => { calcDepth(n.node_id) })

  nodes.forEach(n => {
    n.depth = depthMap[n.node_id] || 0
    n.has_children = (childrenMap[n.node_id] && childrenMap[n.node_id].length > 0)
    n.is_root = !n.parent_node_id || !byNodeId[n.parent_node_id]

    const chain = getChainLabels(n, byNodeId, childrenMap)
    n.chain_labels = chain.labels
    n.chain_depth = chain.depth
    n.chain_total = chain.total
  })

  const familyMap = {}
  nodes.forEach(n => {
    const uid = n.template_uid || n.node_id
    if (!familyMap[uid]) {
      familyMap[uid] = {
        template_uid: uid,
        type: n.type || 'other',
        root_name: '',
        node_count: 0,
        vm_count: 0,
        disk_size: '-',
        root_nodes: [],
        all_node_ids: new Set()
      }
    }
    const fam = familyMap[uid]
    fam.all_node_ids.add(n.node_id)
    if (n.is_root) {
      fam.root_nodes.push(n)
      if (!fam.root_name) fam.root_name = n.admin_name || n.name
    }
    fam.type = fam.type || n.type || 'other'
  })

  for (const fam of Object.values(familyMap)) {
    let totalTreeVm = 0
    fam.all_node_ids.forEach(nid => {
      const node = byNodeId[nid]
      if (node) {
        totalTreeVm += node.tree_vm_count || node.direct_vm_count || 0
      }
    })
    fam.node_count = fam.all_node_ids.size
    fam.vm_count = totalTreeVm

    const sizes = []
    fam.all_node_ids.forEach(nid => {
      const node = byNodeId[nid]
      if (node && node.virtual_size) sizes.push(parseSize(node.virtual_size))
      else if (node && node.actual_size) sizes.push(parseSize(node.actual_size))
    })
    if (sizes.length > 0) {
      const totalBytes = sizes.reduce((a, b) => a + b, 0)
      fam.disk_size = formatBytes(totalBytes)
    }

    const sortNodes = (nodeList) => {
      nodeList.sort((a, b) => (a.admin_name || a.name).localeCompare(b.admin_name || b.name))
    }
    sortNodes(fam.root_nodes)

    const allVisible = []
    const addVisibleNodes = (nodeList, parentExpanded) => {
      nodeList.forEach(n => {
        n._family = fam
        allVisible.push(n)
      })
    }
    addVisibleNodes(fam.root_nodes, true)
    fam.visible_nodes = allVisible
  }

  return Object.values(familyMap).sort((a, b) => a.root_name.localeCompare(b.root_name))
}

const getChainLabels = (node, byNodeId, childrenMap) => {
  const labels = []
  let current = node
  while (current) {
    labels.unshift(current.admin_name || current.name)
    if (current.parent_node_id && byNodeId[current.parent_node_id]) {
      current = byNodeId[current.parent_node_id]
    } else {
      break
    }
  }
  const subtreeCount = countSubtree(node.node_id, childrenMap)
  return { labels, depth: labels.length, total: subtreeCount }
}

const countSubtree = (nodeId, childrenMap) => {
  let count = 1
  const childIds = childrenMap[nodeId]
  if (childIds) {
    childIds.forEach(childId => {
      count += countSubtree(childId, childrenMap)
    })
  }
  return count
}

const parseSize = (sizeStr) => {
  if (!sizeStr) return 0
  const num = parseFloat(sizeStr)
  if (isNaN(num)) return 0
  if (/GiB/i.test(sizeStr) || /GB/i.test(sizeStr)) return num * 1024 * 1024 * 1024
  if (/MiB/i.test(sizeStr) || /MB/i.test(sizeStr)) return num * 1024 * 1024
  if (/KiB/i.test(sizeStr) || /KB/i.test(sizeStr)) return num * 1024
  return num * 1024 * 1024 * 1024
}

const formatBytes = (bytes) => {
  if (bytes >= 1024 * 1024 * 1024) return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
  if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / 1024).toFixed(1) + ' KB'
}

const isNodeExpanded = (node) => {
  return nodeExpandState.value[node.node_id] === true
}

const toggleNode = (node) => {
  const key = node.node_id
  nodeExpandState.value[key] = !isNodeExpanded(node)
  refreshVisibleNodes()
}

const setSubtreeExpand = (node, expanded) => {
  nodeExpandState.value[node.node_id] = expanded
}

const expandAll = () => {
  tableData.value.forEach(n => {
    nodeExpandState.value[n.node_id] = true
  })
  refreshVisibleNodes()
}

const collapseAll = () => {
  tableData.value.forEach(n => {
    nodeExpandState.value[n.node_id] = false
  })
  refreshVisibleNodes()
}

const onNodeRowClick = (event, node) => {
  if (event.target.closest('.el-button') || event.target.closest('.node-actions')) return
  if (node.has_children) toggleNode(node)
}

const refreshVisibleNodes = () => {
  families.value.forEach(fam => {
    const visible = []
    const byNodeId = {}
    tableData.value.forEach(n => { byNodeId[n.node_id] = n })

    const walk = (nodeList) => {
      nodeList.forEach(n => {
        visible.push(n)
        if (!isNodeExpanded(n)) return
        const children = tableData.value.filter(item => item.parent_node_id === n.node_id)
        if (children.length > 0) {
          children.sort((a, b) => (a.admin_name || a.name).localeCompare(b.admin_name || b.name))
          walk(children)
        }
      })
    }
    walk(fam.root_nodes)
    fam.visible_nodes = visible
  })
}

const renderTreeGuides = (node) => {
  const depth = node.depth || 0
  if (depth === 0) return '<div class="tree-guide-col root-col"></div>'

  const byNodeId = {}
  tableData.value.forEach(n => { byNodeId[n.node_id] = n })

  let html = ''
  const ancestors = getAncestorChain(node, byNodeId)

  for (let i = 0; i < depth; i++) {
    const ancestor = ancestors[i]
    const isLastChild = ancestor ? isLastSibling(ancestor, byNodeId) : false
    if (i === depth - 1) {
      html += `<div class="tree-guide-col ${isLastChild ? 'last-end' : ''} last"></div>`
    } else {
      html += '<div class="tree-guide-col"></div>'
    }
  }
  return html
}

const getAncestorChain = (node, byNodeId) => {
  const chain = []
  let current = node
  while (current && current.parent_node_id && byNodeId[current.parent_node_id]) {
    current = byNodeId[current.parent_node_id]
    chain.unshift(current)
  }
  return chain
}

const isLastSibling = (node, byNodeId) => {
  if (!node || !node.parent_node_id) return true
  const siblings = tableData.value.filter(n => n.parent_node_id === node.parent_node_id)
  if (siblings.length === 0) return true
  siblings.sort((a, b) => (a.admin_name || a.name).localeCompare(b.admin_name || b.name))
  return siblings[siblings.length - 1].node_id === node.node_id
}

const resetImportForm = () => {
  importFileList.value = []
  importRawFile.value = null
  importUploading.value = false
  importProgress.value = 0
  importPreview.value = null
  templateSessionKey.value = ''
  importForm.value = { import_mode: 'upload', source_path: '' }
}

const openImportDialog = () => {
  resetImportForm()
  importDialogVisible.value = true
}

const closeImportDialog = () => {
  if (importSubmitting.value || importConfirming.value) return
  importDialogVisible.value = false // 关闭后由 onImportDialogClose 统一清理临时包并重置
}

// 对话框关闭（取消按钮 / X / ESC / 确认导入后）统一回调：
// 若尚未确认导入，则清理已上传的临时模板包，避免残留。
const onImportDialogClose = () => {
  const path = templateSessionKey.value
  if (path) {
    templateSessionKey.value = ''
    templateUploadCancel(path).catch(() => {})
  }
  resetImportForm()
}

const handleImportFileChange = (file, files) => {
  importFileList.value = files.slice(-1)
  importRawFile.value = file.raw || null
  importPreview.value = null
}

const handleImportFileRemove = () => {
  importFileList.value = []
  importRawFile.value = null
  importPreview.value = null
}

const handleImportFileExceed = (files) => {
  const [file] = files
  importFileList.value = file ? [{ name: file.name, status: 'ready', uid: Date.now() }] : []
  importRawFile.value = file || null
  importPreview.value = null
  ElMessage.warning('一次只能选择一个文件，已替换为最新选择')
}

const handleImportPreview = async () => {
  importSubmitting.value = true
  try {
    let sourcePath = ''
    if (importForm.value.import_mode === 'upload') {
      if (!importRawFile.value) {
        ElMessage.warning('请选择模板包')
        return
      }
      importUploading.value = true
      importProgress.value = 0
      // 分片上传模板包到导入临时目录（断点续传 + 秒传）
      const uploader = new ChunkUploader({
        init: templateUploadInit,
        chunk: templateUploadChunk,
        complete: templateUploadComplete,
      })
      currentTemplateUploader.value = uploader
      const { sessionKey } = await uploader.upload(importRawFile.value, {}, {
        onUploadProgress: (ratio) => {
          importProgress.value = Math.round(ratio * 100)
        },
      })
      sourcePath = sessionKey
      templateSessionKey.value = sessionKey // 记录临时包路径，未导入而关闭时清理
      importUploading.value = false
    } else {
      sourcePath = (importForm.value.source_path || '').trim()
      if (!sourcePath || !sourcePath.startsWith('/')) {
        ElMessage.warning('请输入宿主机上的绝对路径')
        return
      }
    }

    // 解析预览：以分片上传产出的临时路径（或主机路径）作为来源
    const formData = new FormData()
    formData.append('source_path', sourcePath)
    const res = await previewImportTemplate(formData)
    importPreview.value = res.data?.preview || null
    ElMessage.success(res.message || '模板包解析完成')
  } catch (err) {
    console.error('预览模板导入失败', err)
  } finally {
    importSubmitting.value = false
    importUploading.value = false
    currentTemplateUploader.value = null
  }
}

const handleImportConfirm = async () => {
  if (!importPreview.value?.token) {
    ElMessage.warning('请先解析模板包')
    return
  }
  importConfirming.value = true
  try {
    const res = await confirmImportTemplate(importPreview.value.token)
    ElMessage.success(res.message || '模板导入任务已提交，请在任务中心查看进度')
    templateSessionKey.value = '' // 已导入，临时包交给导入任务，onImportDialogClose 不再清理
    importDialogVisible.value = false
    fetchData()
  } catch (err) {
    console.error('确认导入模板失败', err)
  } finally {
    importConfirming.value = false
  }
}

const handleExport = async (row, scope) => {
  exportingName.value = `${row.name}:${scope}`
  try {
    const res = await exportTemplate(row.name, scope)
    ElMessage.success(res.message || '模板导出任务已提交')
  } catch (err) {
    console.error('导出模板失败', err)
  } finally {
    exportingName.value = ''
  }
}

const handlePrepareLinux = async (row) => {
  try {
    await ElMessageBox.confirm(
      `将检查并补齐模板「${row.admin_name || row.name}」的 cloud-init 与磁盘扩容依赖。任务执行期间模板不可用于新的预处理操作。`,
      'Linux 模板离线预处理',
      { type: 'warning', confirmButtonText: '提交任务', cancelButtonText: '取消' }
    )
    preparingLinuxName.value = row.name
    const res = await prepareImportedLinuxTemplate(row.name)
    ElMessage.success(res.message || 'Linux 模板离线预处理任务已提交，请在任务中心查看进度')
  } catch (err) {
    if (err !== 'cancel' && err?.toString?.() !== 'cancel') console.error('提交 Linux 模板预处理失败', err)
  } finally {
    preparingLinuxName.value = ''
  }
}

const handleDownloadExport = (row) => {
  if (!row.export_path) {
    ElMessage.warning('导出包地址不存在，请重新导出')
    return
  }
  window.open(getTemplateExportDownloadUrl(row.export_path), '_blank')
}

const handleDeleteExport = async (row) => {
  try {
    await ElMessageBox.confirm(`确认删除模板「${row.admin_name || row.name}」的导出包？`, '删除导出包', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  deletingExportName.value = row.name
  try {
    const res = await deleteTemplateExport(row.name)
    ElMessage.success(res.message || '模板导出包已删除')
    fetchData()
  } catch (err) {
    console.error('删除模板导出包失败', err)
  } finally {
    deletingExportName.value = ''
  }
}

const openPublishDialog = (row) => {
  const defaults = row.default_config || {}
  publishForm.value = {
    name: row.name,
    type: row.type || '',
    admin_name: row.admin_name || row.name,
    display_name: row.display_name || row.admin_name || row.name,
    clone_visible: !!row.clone_visible,
    disabled: !!row.disabled,
    category: normalizeTemplateCategory(row.type, row.category),
    vcpu: Number(defaults.vcpu || 0),
    ram: Number(defaults.ram || 0),
    disk_size: Number(defaults.disk_size || 0),
    disk_bus: defaults.disk_bus || '',
    nic_model: defaults.nic_model || '',
    video_model: defaults.video_model || '',
    cpu_topology_mode: defaults.cpu_topology_mode || '',
    first_boot_reboot_mode: defaults.first_boot_reboot_mode || '',
    post_boot_command: row.post_boot_command || '',
    post_boot_blocking: row.post_boot_blocking || false,
  }
  publishDialogVisible.value = true
}

const handleSavePublish = async () => {
  if (!publishForm.value.admin_name || !publishForm.value.display_name) {
    ElMessage.warning('请填写管理员名称和用户侧显示')
    return
  }
  publishSaving.value = true
  try {
    await updateTemplatePublish(publishForm.value.name, {
      admin_name: publishForm.value.admin_name,
      display_name: publishForm.value.display_name,
      clone_visible: publishForm.value.clone_visible,
      disabled: publishForm.value.disabled,
      category: ['linux', 'windows'].includes(publishForm.value.type)
        ? normalizeTemplateCategory(publishForm.value.type, publishForm.value.category)
        : '',
      vcpu: Number(publishForm.value.vcpu || 0),
      ram: Number(publishForm.value.ram || 0),
      disk_size: Number(publishForm.value.disk_size || 0),
      disk_bus: publishForm.value.disk_bus || '',
      nic_model: publishForm.value.nic_model || '',
      video_model: publishForm.value.video_model || '',
      cpu_topology_mode: publishForm.value.cpu_topology_mode || '',
      first_boot_reboot_mode: publishForm.value.first_boot_reboot_mode || '',
      post_boot_command: publishForm.value.post_boot_command || '',
      post_boot_blocking: publishForm.value.post_boot_blocking || false,
    })
    ElMessage.success('模板发布设置已保存')
    publishDialogVisible.value = false
    fetchData()
  } catch (err) {
    console.error('保存模板发布设置失败', err)
  } finally {
    publishSaving.value = false
  }
}

const openDeleteDialog = async (row) => {
  deleteForm.value = { ...emptyDeleteForm(), name: row.name }
  deleteDialogVisible.value = true
  deleteLoading.value = true
  try {
    const res = await getTemplateDeletePreview(row.name)
    deleteForm.value.templates = res.data?.templates || []
    deleteForm.value.relatedVMs = res.data?.related_vms || []
    deleteForm.value.parentTemplate = res.data?.parent_template || null
    deleteForm.value.promotedTemplates = res.data?.promoted_templates || []
    deleteForm.value.rebasedVMs = res.data?.rebased_vms || []
    deleteForm.value.canPromote = !!res.data?.can_promote
    deleteForm.value.promoteBlockers = res.data?.promote_blockers || []
    deleteForm.value.canPromoteHot = !!res.data?.can_promote_hot
    deleteForm.value.promoteHotBlockers = res.data?.promote_hot_blockers || []
  } catch (err) {
    deleteDialogVisible.value = false
    console.error('获取模板删除预览失败', err)
  } finally {
    deleteLoading.value = false
  }
}

const isDirectRebasedVM = (name) => {
  return deleteForm.value.rebasedVMs.some(vm => vm.name === name)
}

const deleteVMActionText = (row) => {
  if (isDirectRebasedVM(row.name)) {
    return deleteForm.value.mode === 'promote_children_hot' && row.status === 'running' ? '在线拉平到上级' : 'rebase 到上级'
  }
  if (deleteForm.value.mode === 'promote_children_hot' && row.status === 'running') {
    return '在线切换新 backing'
  }
  return deleteForm.value.mode === 'promote_children_hot' ? '随子模板提升' : '需保持关机'
}

const handleDelete = async () => {
  deleteSubmitting.value = true
  try {
    const expectedVMs = deleteForm.value.relatedVMs.map(vm => vm.name)
    const res = await deleteTemplate(deleteForm.value.name, {
      delete_mode: deleteForm.value.mode,
      delete_vms: !isPromoteDeleteMode.value && expectedVMs.length > 0,
      expected_vms: expectedVMs,
    })
    ElMessage.success(res.message || '删除模板任务已提交，请在任务中心查看进度')
    deleteDialogVisible.value = false
    fetchData()
  } catch (err) {
    console.error('删除模板失败', err)
  } finally {
    deleteSubmitting.value = false
  }
}

onMounted(fetchData)
</script>

<style scoped>
.template-list-container {
  padding: 10px 16px;
  width: 100%;
}

.page-header {
  margin-bottom: 18px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.page-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1a1a2e;
}
.header-actions {
  display: flex;
  gap: 10px;
}

.btn-outline-custom {
  background: #fff !important;
  color: #555 !important;
  border: 1px solid #d9d9d9 !important;
  border-radius: 8px !important;
  font-size: 14px !important;
  font-weight: 500 !important;
  padding: 8px 18px !important;
}
.btn-outline-custom:hover {
  border-color: #4f6ef6 !important;
  color: #4f6ef6 !important;
}

.family-cards-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.family-card {
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0,0,0,.06);
  overflow: hidden;
}

.family-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  background: #fafbfc;
  border-bottom: 1px solid #f0f0f0;
}

.family-id {
  display: flex;
  align-items: center;
  gap: 10px;
}

.family-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}
.family-icon.linux  { background: #e8f5e9; }
.family-icon.windows { background: #e3f2fd; }
.family-icon.fnos   { background: #fce4ec; }
.family-icon.other  { background: #fff3e0; }

.family-title {
  font-size: 15px;
  font-weight: 600;
  color: #1a1a2e;
}

.family-meta {
  font-size: 12px;
  color: #999;
  margin-top: 2px;
}

.family-stats {
  display: flex;
  gap: 20px;
  font-size: 13px;
  color: #666;
}

.family-stat {
  display: flex;
  align-items: center;
  gap: 5px;
}

.family-stat strong {
  color: #333;
  font-weight: 600;
}

.node-list {
  padding: 0;
  list-style: none;
}

.node-item {
  border-bottom: 1px solid #f5f5f5;
}
.node-item:last-child { border-bottom: none; }

.node-row {
  display: flex;
  align-items: center;
  padding: 3px 14px;
  transition: background .12s;
  position: relative;
  min-height: 32px;
  cursor: pointer;
}
.node-row:hover { background: #fafbfc; }

.tree-guides {
  flex-shrink: 0;
  display: flex;
  align-items: stretch;
  height: 14px;
}

.toggle-btn {
  flex-shrink: 0;
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: #999;
  font-size: 10px;
  border-radius: 3px;
  margin-right: 4px;
  user-select: none;
  transition: transform .2s, color .15s;
}
.toggle-btn:hover {
  color: #4f6ef6;
  background: #f0f3ff;
}
.toggle-btn.collapsed {
  transform: rotate(-90deg);
}
.toggle-btn.no-children {
  visibility: hidden;
}

.level-bar {
  flex-shrink: 0;
  width: 2px;
  height: 14px;
  border-radius: 1px;
  margin-right: 10px;
}
.level-bar.l0 { background: #4f6ef6; }
.level-bar.l1 { background: #52c41a; }
.level-bar.l2 { background: #fa8c16; }
.level-bar.l3 { background: #eb2f96; }
.level-bar.l4 { background: #13c2c2; }

.node-content {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex-wrap: wrap;
}

.node-identity {
  min-width: 150px;
  max-width: 200px;
}

.node-name {
  font-size: 13px;
  font-weight: 600;
  color: #1a1a2e;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-file {
  font-size: 11px;
  color: #aaa;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.tag-success { background: #f6ffed; color: #52c41a; border: 1px solid #b7eb8f; }
.tag-info    { background: #e6f7ff; color: #1890ff; border: 1px solid #91d5ff; }
.tag-warning { background: #fff7e6; color: #fa8c16; border: 1px solid #ffd591; }
.tag-danger  { background: #fff2f0; color: #ff4d4f; border: 1px solid #ffccc7; }
.tag-default { background: #fafafa; color: #999; border: 1px solid #d9d9d9; }

.os-tag {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: .3px;
}
.os-linux   { background: #e8f5e9; color: #2e7d32; }
.os-windows { background: #e3f2fd; color: #1565c0; }
.os-fnos    { background: #fce4ec; color: #c62828; }
.os-other   { background: #fff3e0; color: #e65100; }

.node-vm-stat {
  display: flex;
  align-items: baseline;
  gap: 3px;
  min-width: 44px;
}
.vm-count {
  font-size: 13px;
  font-weight: 700;
  color: #1a1a2e;
}
.vm-label {
  font-size: 10px;
  color: #bbb;
}

.node-size {
  min-width: 85px;
  text-align: right;
  line-height: 1.3;
}
.size-value {
  font-size: 12px;
  font-weight: 500;
  color: #555;
}
.size-label {
  font-size: 10px;
  color: #bbb;
}

.node-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
  margin-left: 8px;
}

.chain-summary {
  padding: 3px 14px 3px 76px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #888;
  border-bottom: 1px solid #f5f5f5;
  background: #fafbfc;
  cursor: pointer;
}
.chain-summary:hover { background: #f0f3ff; }

.chain-arrow { color: #bbb; margin: 0 2px; }
.chain-depth { color: #4f6ef6; font-weight: 500; }
.chain-leaf { color: #333; font-weight: 500; }

.node-item.disabled .node-row {
  opacity: .55;
  background: #fafafa;
}
.node-item.disabled .node-name {
  text-decoration: line-through;
}

.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: #bbb;
  font-size: 14px;
}
.empty-icon {
  font-size: 48px;
  margin-bottom: 12px;
}
.empty-hint {
  font-size: 12px;
  margin-top: 8px;
  color: #aaa;
}

.form-tip {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  font-size: 12px;
  color: #909399;
}
.import-preview {
  margin-top: 14px;
}

@media (max-width: 900px) {
  .node-content {
    flex-wrap: wrap;
    gap: 8px;
  }
  .node-actions {
    margin-left: auto;
  }
}

html.dark .page-header h2 { color: var(--el-text-color-primary); }

html.dark .btn-outline-custom {
  background: var(--app-bg-card) !important;
  color: var(--el-text-color-regular) !important;
  border-color: var(--app-border-light) !important;
}
html.dark .btn-outline-custom:hover {
  border-color: var(--el-color-primary) !important;
  color: var(--el-color-primary) !important;
}

html.dark .family-card {
  background: var(--app-bg-card);
  box-shadow: var(--app-shadow-sm);
}

html.dark .family-header {
  background: var(--app-bg-elevated);
  border-bottom-color: var(--app-border-light);
}

html.dark .family-title { color: var(--el-text-color-primary); }
html.dark .family-meta { color: var(--el-text-color-secondary); }
html.dark .family-stats { color: var(--el-text-color-regular); }
html.dark .family-stat strong { color: var(--el-text-color-primary); }

html.dark .family-icon.linux  { background: rgba(46, 125, 50, 0.2); }
html.dark .family-icon.windows { background: rgba(21, 101, 192, 0.2); }
html.dark .family-icon.fnos   { background: rgba(198, 40, 40, 0.2); }
html.dark .family-icon.other  { background: rgba(230, 81, 0, 0.2); }

html.dark .node-item { border-bottom-color: var(--app-border-extralight); }
html.dark .node-row:hover { background: var(--app-bg-hover); }

html.dark .toggle-btn { color: var(--el-text-color-secondary); }
html.dark .toggle-btn:hover {
  color: var(--el-color-primary);
  background: rgba(79, 110, 246, 0.15);
}

html.dark .node-name { color: var(--el-text-color-primary); }
html.dark .node-file { color: var(--el-text-color-placeholder); }

html.dark .tag-success { background: rgba(82, 196, 26, 0.15); color: #73d13d; border-color: rgba(82, 196, 26, 0.25); }
html.dark .tag-info    { background: rgba(24, 144, 255, 0.15); color: #69c0ff; border-color: rgba(24, 144, 255, 0.25); }
html.dark .tag-warning { background: rgba(250, 140, 22, 0.15); color: #ffc53d; border-color: rgba(250, 140, 22, 0.25); }
html.dark .tag-danger  { background: rgba(255, 77, 79, 0.15); color: #ff7875; border-color: rgba(255, 77, 79, 0.25); }
html.dark .tag-default { background: rgba(255, 255, 255, 0.06); color: var(--el-text-color-secondary); border-color: var(--app-border-light); }

html.dark .os-linux   { background: rgba(46, 125, 50, 0.2); color: #81c784; }
html.dark .os-windows { background: rgba(21, 101, 192, 0.2); color: #64b5f6; }
html.dark .os-fnos    { background: rgba(198, 40, 40, 0.2); color: #e57373; }
html.dark .os-other   { background: rgba(230, 81, 0, 0.2); color: #ffb74d; }

html.dark .vm-count { color: var(--el-text-color-primary); }
html.dark .vm-label { color: var(--el-text-color-secondary); }

html.dark .size-value { color: var(--el-text-color-regular); }
html.dark .size-label { color: var(--el-text-color-secondary); }

html.dark .chain-summary {
  color: var(--el-text-color-secondary);
  border-bottom-color: var(--app-border-extralight);
  background: var(--app-bg-elevated);
}
html.dark .chain-summary:hover { background: rgba(79, 110, 246, 0.08); }

html.dark .chain-arrow { color: var(--el-text-color-secondary); }
html.dark .chain-depth { color: #8ca0ff; }
html.dark .chain-leaf { color: var(--el-text-color-primary); }

html.dark .node-item.disabled .node-row { background: rgba(255, 255, 255, 0.02); }

html.dark .empty-state { color: var(--el-text-color-secondary); }
html.dark .empty-hint { color: var(--el-text-color-placeholder); }

html.dark .form-tip { color: var(--el-text-color-secondary); }
</style>
