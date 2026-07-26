<template>
  <div
    class="recent-task-panel"
    :class="{ 'is-collapsed': !expanded, 'is-transitioning': useTransition }"
    :style="{ height: panelHeight + 'px' }"
  >
    <div
      class="resize-handle"
      @mousedown="startResize"
    />

    <div class="panel-header">
      <div class="header-left">
        <el-icon class="header-icon"><List /></el-icon>
        <span class="header-title">异步任务</span>
        <el-tag v-if="activeCount > 0" type="danger" size="small" effect="dark" class="active-badge">
          {{ activeCount }} 运行中
        </el-tag>
      </div>
      <div class="header-right">
        <el-tag :type="sseStatusType" size="small" effect="dark">
          {{ sseStatusText }}
        </el-tag>
        <el-button text size="small" @click="openFullTaskCenter" class="full-btn">
          <el-icon><Monitor /></el-icon>
          <span class="full-btn-text">完整任务中心</span>
        </el-button>
        <el-button text size="small" @click="toggleExpand" class="collapse-btn">
          <el-icon :class="{ 'rotate-icon': !expanded }"><ArrowDown /></el-icon>
        </el-button>
      </div>
    </div>

    <transition name="panel-body-fade">
      <div v-show="expanded" class="panel-body" v-loading="loading">
      <el-table
        :data="tableData"
        size="small"
        border
        stripe
        row-key="id"
        :show-header="true"
        class="compact-table"
        empty-text="暂无任务记录"
      >
        <el-table-column prop="type" label="类型" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="typeTagType(row.type)" effect="plain" size="small">
              {{ typeText(row.type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">
              {{ statusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="140">
          <template #default="{ row }">
            <el-progress
              :percentage="row.progress || 0"
              :status="progressStatus(row.status)"
              :stroke-width="14"
              :text-inside="true"
            />
          </template>
        </el-table-column>
        <el-table-column prop="message" label="状态消息" min-width="160" show-overflow-tooltip />
        <el-table-column prop="created_by" label="创建人" width="70" align="center" />
        <el-table-column label="时间" width="130" align="center">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="handleDetail(row)">详情</el-button>
            <el-button
              v-if="row.status === 'pending' || row.status === 'running'"
              size="small"
              type="warning"
              link
              @click="handleCancel(row)"
            >取消</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="panel-pagination">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :total="pagination.total"
          :page-sizes="[5, 10, 20]"
          layout="sizes, prev, pager, next"
          small
          @size-change="fetchData"
          @current-change="fetchData"
        />
      </div>
    </div>
    </transition>

    <el-drawer v-model="detailVisible" title="任务详情" size="500px" append-to-body>
      <div v-loading="detailLoading">
        <template v-if="currentTask">
          <el-descriptions :column="1" border>
            <el-descriptions-item label="任务 ID">{{ currentTask.id }}</el-descriptions-item>
            <el-descriptions-item label="任务类型">
              <el-tag :type="typeTagType(currentTask.type)" effect="plain" size="small">
                {{ typeText(currentTask.type) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="statusTagType(currentTask.status)" size="small">
                {{ statusText(currentTask.status) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="进度">
              <el-progress
                :percentage="currentTask.progress || 0"
                :status="progressStatus(currentTask.status)"
                :stroke-width="14"
                :text-inside="true"
                style="width: 100%;"
              />
            </el-descriptions-item>
            <el-descriptions-item label="状态消息">{{ currentTask.message || '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建人">{{ currentTask.created_by || '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatTime(currentTask.created_at) }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ formatTime(currentTask.updated_at) }}</el-descriptions-item>
          </el-descriptions>

          <div v-if="currentTask.params" style="margin-top: 16px;">
            <h4 style="margin: 0 0 8px;">任务参数</h4>
            <el-card shadow="never" class="json-card">
              <pre class="json-pre">{{ formatJSON(currentTask.params) }}</pre>
            </el-card>
          </div>

          <div v-if="currentTask.result" style="margin-top: 16px;">
            <h4 style="margin: 0 0 8px;">执行结果</h4>
            <el-card shadow="never" class="json-card">
              <pre class="json-pre">{{ formatJSON(currentTask.result) }}</pre>
            </el-card>
          </div>

          <div v-if="downloadLinks.length > 0" style="margin-top: 16px;">
            <h4 style="margin: 0 0 8px;">结果下载</h4>
            <div class="download-actions">
              <el-button
                v-for="link in downloadLinks"
                :key="link.path"
                type="primary"
                plain
                @click="handleDownload(link)"
              >
                {{ link.label }}
              </el-button>
            </div>
          </div>
        </template>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { getTaskList, getTaskDetail, cancelTask } from '@/api/task'
import { getTemplateExportDownloadUrl } from '@/api/vm'
import { ElMessage, ElMessageBox } from 'element-plus'
import { List, ArrowDown, Monitor } from '@element-plus/icons-vue'

const emit = defineEmits(['update:activeCount', 'open-full'])

const STORAGE_KEY_HEIGHT = 'recent-task-panel-height'
const STORAGE_KEY_EXPANDED = 'recent-task-panel-expanded'
const COLLAPSED_HEIGHT = 40
const DEFAULT_HEIGHT = 250
const MAX_HEIGHT_RATIO = 0.6

const expanded = ref(localStorage.getItem(STORAGE_KEY_EXPANDED) !== 'false')
const panelHeight = ref(
  localStorage.getItem(STORAGE_KEY_HEIGHT)
    ? Number(localStorage.getItem(STORAGE_KEY_HEIGHT))
    : DEFAULT_HEIGHT
)
const useTransition = ref(false)

const tableData = ref([])
const loading = ref(false)
const pagination = ref({ page: 1, pageSize: 10, total: 0 })

const detailVisible = ref(false)
const currentTask = ref(null)
const detailLoading = ref(false)

const sseConnected = ref('connecting')
let eventSource = null

const sseStatusType = computed(() => {
  const map = { connecting: 'warning', connected: 'success', disconnected: 'info' }
  return map[sseConnected.value] || 'info'
})

const sseStatusText = computed(() => {
  const map = { connecting: '连接中...', connected: '实时连接', disconnected: '已断开' }
  return map[sseConnected.value] || '已断开'
})

const terminalStatuses = ['success', 'failed', 'canceled']

const activeCount = computed(() => {
  return tableData.value.filter(t => t.status === 'pending' || t.status === 'running').length
})

const downloadLinks = computed(() => extractDownloadLinks(currentTask.value?.result))

watch(activeCount, (val) => {
  emit('update:activeCount', val)
})

const typeText = (type) => {
  const map = {
    clone: '链式克隆', batch: '批量克隆', reinstall: '重装系统', prepare: '制作模板',
    template_export: '导出模板', template_import: '导入模板', template_linux_prepare: 'Linux 模板预处理', delete_template: '删除模板',
    create: '普通创建', lightweight_vm_provision: '轻量云开通',
    lightweight_runtime_quota_shutdown: '轻量云时长关机', delete: '删除虚拟机',
    snapshot: '快照操作', export: '导出虚拟机', import: '导入虚拟机',
    vm_migrate: '迁移虚拟机', vm_disk_migrate: '迁移硬盘', storage_format: '格式化存储',
    storage_create_partition: '创建分区',
    storage_delete_partitions: '删除分区',
    ovs_repair: 'OVS 修复', network_capture: '网络抓包', vm_schedule_action: '虚拟机定时任务',
  }
  return map[type] || type
}

const typeTagType = (type) => {
  const map = {
    clone: 'primary', batch: 'primary', reinstall: 'warning', prepare: 'success',
    template_export: 'success', template_import: 'primary', template_linux_prepare: 'warning', delete_template: 'danger',
    create: '', lightweight_vm_provision: 'primary', lightweight_runtime_quota_shutdown: 'warning',
    delete: 'danger', snapshot: 'info', export: 'success', import: 'primary',
    vm_migrate: 'warning', vm_disk_migrate: 'warning', storage_format: 'warning',
    storage_create_partition: 'warning',
    storage_delete_partitions: 'danger',
    ovs_repair: 'warning', network_capture: 'warning', vm_schedule_action: 'primary',
  }
  return map[type] || 'info'
}

const statusText = (status) => {
  const map = { pending: '等待中', running: '执行中', success: '成功', failed: '失败', canceled: '已取消' }
  return map[status] || status
}

const statusTagType = (status) => {
  const map = { pending: 'info', running: 'primary', success: 'success', failed: 'danger', canceled: 'warning' }
  return map[status] || 'info'
}

const progressStatus = (status) => {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'exception'
  if (status === 'canceled') return 'warning'
  return ''
}

const formatTime = (time) => {
  if (!time) return '-'
  const d = new Date(time)
  return d.toLocaleString('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

const formatJSON = (jsonStr) => {
  try { return JSON.stringify(JSON.parse(jsonStr), null, 2) } catch { return jsonStr }
}

const extractDownloadLinks = (resultStr) => {
  if (!resultStr) return []
  try {
    const parsed = JSON.parse(resultStr)
    const links = []
    if (parsed.download_path) {
      links.push({ label: parsed.file_name ? `下载 ${parsed.file_name}` : '下载文件', path: parsed.download_path })
    }
    if (Array.isArray(parsed.extra_downloads)) {
      parsed.extra_downloads.forEach((item) => {
        if (item?.download_path) {
          links.push({ label: item.label || (item.file_name ? `下载 ${item.file_name}` : '下载附加文件'), path: item.download_path })
        }
      })
    }
    return links
  } catch { return [] }
}

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getTaskList({
      page: pagination.value.page,
      page_size: pagination.value.pageSize,
    })
    tableData.value = res.data?.list || []
    pagination.value.total = res.data?.total || 0
  } catch (err) {
    console.error('获取任务列表失败', err)
  } finally {
    loading.value = false
  }
}

const fetchTaskDetailData = async (taskId, silent = false) => {
  if (!taskId) return
  detailLoading.value = !silent
  try {
    const res = await getTaskDetail(taskId)
    currentTask.value = res.data || null
  } catch (err) {
    if (!silent) console.error('获取任务详情失败', err)
  } finally {
    detailLoading.value = false
  }
}

const connectSSE = () => {
  const token = localStorage.getItem('token')
  if (!token) return
  const baseURL = import.meta.env.VITE_APP_BASE_API || '/api'
  const url = `${baseURL}/task/sse?token=${encodeURIComponent(token)}`

  sseConnected.value = 'connecting'
  eventSource = new EventSource(url)

  eventSource.addEventListener('connected', () => {
    sseConnected.value = 'connected'
  })

  eventSource.addEventListener('task_progress', (e) => {
    try {
      const event = JSON.parse(e.data)
      updateTaskInList(event)
      if (currentTask.value && currentTask.value.id === event.task_id) {
        currentTask.value.status = event.status
        currentTask.value.progress = event.progress
        currentTask.value.message = event.message
        if (terminalStatuses.includes(event.status)) {
          fetchTaskDetailData(event.task_id, true)
        }
      }
    } catch (err) {
      console.error('解析 SSE 事件失败', err)
    }
  })

  eventSource.onerror = () => {
    sseConnected.value = 'disconnected'
    if (eventSource) eventSource.close()
    setTimeout(connectSSE, 5000)
  }
}

const updateTaskInList = (event) => {
  const idx = tableData.value.findIndex(t => t.id === event.task_id)
  if (idx !== -1) {
    tableData.value[idx].status = event.status
    tableData.value[idx].progress = event.progress
    tableData.value[idx].message = event.message
  } else {
    fetchData()
  }
}

const disconnectSSE = () => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
    sseConnected.value = 'disconnected'
  }
}

const handleDetail = async (row) => {
  currentTask.value = { ...row }
  detailVisible.value = true
  await fetchTaskDetailData(row.id)
}

const handleCancel = async (row) => {
  try {
    const isRunning = row.status === 'running'
    const msg = isRunning
      ? `任务 #${row.id} 正在执行中，取消后已创建的资源将被自动清理。确定要取消吗？`
      : `确定要取消任务 #${row.id} 吗？`
    await ElMessageBox.confirm(msg, '取消任务', { type: 'warning' })
    await cancelTask(row.id)
    ElMessage.success(isRunning ? '取消信号已发送，任务将尽快停止' : '任务已取消')
    fetchData()
  } catch {}
}

const handleDownload = (link) => {
  const url = getTemplateExportDownloadUrl(link.path)
  if (!url) return
  window.open(url, '_blank')
}

const toggleExpand = () => {
  useTransition.value = true
  expanded.value = !expanded.value
  localStorage.setItem(STORAGE_KEY_EXPANDED, String(expanded.value))
  if (expanded.value) {
    const maxH = window.innerHeight * MAX_HEIGHT_RATIO
    if (panelHeight.value < DEFAULT_HEIGHT) {
      panelHeight.value = Math.min(DEFAULT_HEIGHT, maxH)
    }
  } else {
    panelHeight.value = COLLAPSED_HEIGHT
  }
  localStorage.setItem(STORAGE_KEY_HEIGHT, String(panelHeight.value))
  setTimeout(() => { useTransition.value = false }, 350)
}

const collapse = () => {
  if (!expanded.value) return
  useTransition.value = true
  expanded.value = false
  panelHeight.value = COLLAPSED_HEIGHT
  localStorage.setItem(STORAGE_KEY_EXPANDED, 'false')
  localStorage.setItem(STORAGE_KEY_HEIGHT, String(COLLAPSED_HEIGHT))
  setTimeout(() => { useTransition.value = false }, 350)
}

const openFullTaskCenter = () => {
  emit('open-full')
}

let resizing = false
let startY = 0
let startHeight = 0

const startResize = (e) => {
  e.preventDefault()
  resizing = true
  useTransition.value = false
  startY = e.clientY
  startHeight = panelHeight.value
  document.addEventListener('mousemove', onResize)
  document.addEventListener('mouseup', stopResize)
  document.body.style.cursor = 'row-resize'
  document.body.style.userSelect = 'none'
}

const onResize = (e) => {
  if (!resizing) return
  const diff = startY - e.clientY
  const maxH = window.innerHeight * MAX_HEIGHT_RATIO
  const newHeight = Math.max(COLLAPSED_HEIGHT, Math.min(startHeight + diff, maxH))
  panelHeight.value = newHeight
  expanded.value = newHeight > COLLAPSED_HEIGHT + 10
}

const stopResize = () => {
  resizing = false
  document.removeEventListener('mousemove', onResize)
  document.removeEventListener('mouseup', stopResize)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  localStorage.setItem(STORAGE_KEY_HEIGHT, String(panelHeight.value))
  localStorage.setItem(STORAGE_KEY_EXPANDED, String(expanded.value))
}

const handleResize = () => {
  const maxH = window.innerHeight * MAX_HEIGHT_RATIO
  if (panelHeight.value > maxH) {
    panelHeight.value = maxH
    localStorage.setItem(STORAGE_KEY_HEIGHT, String(maxH))
  }
}

defineExpose({ toggleExpand, collapse })

onMounted(() => {
  if (panelHeight.value <= COLLAPSED_HEIGHT) {
    expanded.value = false
  }
  fetchData()
  connectSSE()
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  disconnectSSE()
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.recent-task-panel {
  position: relative;
  background: var(--el-bg-color);
  border-top: 1px solid var(--el-border-color-light);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  flex-shrink: 0;
}

.recent-task-panel.is-transitioning {
  transition: height 0.3s ease;
}

.resize-handle {
  height: 6px;
  cursor: row-resize;
  background: transparent;
  transition: background 0.2s;
  flex-shrink: 0;
}

.resize-handle:hover,
.resize-handle:active {
  background: var(--el-color-primary-light-5);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  height: 34px;
  flex-shrink: 0;
  user-select: none;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-icon {
  font-size: 14px;
  color: var(--el-color-primary);
}

.header-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.active-badge {
  margin-left: 4px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.full-btn-text {
  margin-left: 4px;
}

.collapse-btn .el-icon {
  transition: transform 0.3s;
}

.collapse-btn .rotate-icon {
  transform: rotate(180deg);
}

.panel-body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 0 8px 4px;
}

.compact-table {
  flex: 1;
  overflow: auto;
}

.compact-table :deep(.el-table__body-wrapper) {
  overflow-y: auto;
}

.compact-table :deep(.el-table__header th) {
  padding: 4px 0;
  font-size: 12px;
  background: var(--el-fill-color-lighter);
}

.compact-table :deep(.el-table__body td) {
  padding: 3px 0;
  font-size: 12px;
}

.panel-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 4px 0 0;
  flex-shrink: 0;
}

.json-card {
  background-color: var(--el-fill-color-light);
}

.json-pre {
  margin: 0;
  padding: 0;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--el-text-color-regular);
  font-family: 'Consolas', 'Monaco', monospace;
}

.download-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* 面板内容淡入淡出过渡 */
.panel-body-fade-enter-active {
  transition: opacity 0.3s ease, max-height 0.3s ease;
  overflow: hidden;
}

.panel-body-fade-leave-active {
  transition: opacity 0.2s ease, max-height 0.2s ease;
  overflow: hidden;
}

.panel-body-fade-enter-from {
  opacity: 0;
  max-height: 0;
}

.panel-body-fade-leave-to {
  opacity: 0;
  max-height: 0;
}

@media (max-width: 768px) {
  .panel-header {
    padding: 0 8px;
    height: 30px;
  }

  .header-title {
    font-size: 12px;
  }

  .full-btn-text {
    display: none;
  }

  .compact-table :deep(.el-table__header th),
  .compact-table :deep(.el-table__body td) {
    padding: 2px 0;
    font-size: 11px;
  }
}
</style>
