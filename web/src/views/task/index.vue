<template>
  <div class="task-list-container">
    <div class="task-list-content">
      <!-- 顶部操作 -->
      <div class="header-bar">
        <div class="header-actions" style="margin-left: auto;">
          <el-button type="success" icon="Refresh" @click="fetchData" :loading="loading">刷新</el-button>
          <el-button type="danger" plain icon="Delete" @click="handleClear" :disabled="!hasFinished">
            清理已完成
          </el-button>
        </div>
      </div>

      <!-- 筛选栏 -->
      <div class="filter-bar">
        <el-select v-model="filters.status" placeholder="任务状态" clearable @change="fetchData" style="width: 140px;">
          <el-option label="全部" value="" />
          <el-option label="等待中" value="pending" />
          <el-option label="执行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="已取消" value="canceled" />
        </el-select>
        <el-select v-model="filters.type" placeholder="任务类型" clearable @change="fetchData" style="width: 140px; margin-left: 10px;">
          <el-option label="全部" value="" />
          <el-option label="链式克隆" value="clone" />
          <el-option label="批量克隆" value="batch" />
          <el-option label="重装系统" value="reinstall" />
          <el-option label="制作模板" value="prepare" />
          <el-option label="导出模板" value="template_export" />
          <el-option label="导入模板" value="template_import" />
          <el-option label="Linux 模板预处理" value="template_linux_prepare" />
          <el-option label="删除模板" value="delete_template" />
          <el-option label="普通创建" value="create" />
          <el-option label="轻量云开通" value="lightweight_vm_provision" />
          <el-option label="轻量云时长关机" value="lightweight_runtime_quota_shutdown" />
          <el-option label="删除虚拟机" value="delete" />
          <el-option label="快照操作" value="snapshot" />
          <el-option label="导出虚拟机" value="export" />
          <el-option label="导入虚拟机" value="import" />
          <el-option label="迁移虚拟机" value="vm_migrate" />
          <el-option label="迁移硬盘" value="vm_disk_migrate" />
          <el-option label="格式化存储" value="storage_format" />
          <el-option label="创建分区" value="storage_create_partition" />
          <el-option label="删除分区" value="storage_delete_partitions" />
          <el-option label="OVS 修复" value="ovs_repair" />
          <el-option label="网络抓包" value="network_capture" />
          <el-option label="虚拟机定时任务" value="vm_schedule_action" />
        </el-select>
        <div class="sse-status" style="margin-left: auto;">
          <el-tag :type="sseStatusType" size="small" effect="dark">
            <el-icon style="vertical-align: middle;"><Connection v-if="sseConnected === 'connected'" /><Loading v-else-if="sseConnected === 'connecting'" /><WarningFilled v-else /></el-icon>
            {{ sseStatusText }}
          </el-tag>
        </div>
      </div>

      <!-- 任务表格 -->
      <el-table :data="tableData" border style="width: 100%" v-loading="loading" row-key="id">
        <el-table-column prop="id" label="ID" width="60" align="center" />
        <el-table-column prop="type" label="类型" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="typeTagType(row.type)" effect="plain" size="small">
              {{ typeText(row.type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">
              {{ statusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="200">
          <template #default="{ row }">
            <el-progress
              :percentage="row.progress || 0"
              :status="progressStatus(row.status)"
              :stroke-width="16"
              :text-inside="true"
            />
          </template>
        </el-table-column>
        <el-table-column prop="message" label="状态消息" min-width="200" show-overflow-tooltip />
        <el-table-column prop="created_by" label="创建人" width="90" align="center" />
        <el-table-column label="创建时间" width="170" align="center">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" type="primary" plain @click="handleDetail(row)">详情</el-button>
            <el-button
              v-if="row.status === 'pending' || row.status === 'running'"
              size="small"
              type="warning"
              plain
              @click="handleCancel(row)"
            >取消</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-bar">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :total="pagination.total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @size-change="fetchData"
          @current-change="fetchData"
        />
      </div>
    </div>

    <!-- 任务详情抽屉 -->
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
              :stroke-width="16"
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
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { getTaskList, getTaskDetail, cancelTask, clearFinishedTasks } from '@/api/task'
import { getTemplateExportDownloadUrl } from '@/api/vm'
import { ElMessage, ElMessageBox } from 'element-plus'

// 数据
const tableData = ref([])
const loading = ref(false)
const filters = ref({ status: '', type: '' })
const pagination = ref({ page: 1, pageSize: 20, total: 0 })

// 详情抽屉
const detailVisible = ref(false)
const currentTask = ref(null)
const detailLoading = ref(false)

// SSE 连接
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

// 是否有已完成的任务
const hasFinished = computed(() => {
  return tableData.value.some(t => ['success', 'failed', 'canceled'].includes(t.status))
})

const downloadLinks = computed(() => extractDownloadLinks(currentTask.value?.result))

const terminalStatuses = ['success', 'failed', 'canceled']

// ===================== 类型/状态映射 =====================

const typeText = (type) => {
  const map = {
    clone: '链式克隆',
    batch: '批量克隆',
    reinstall: '重装系统',
    prepare: '制作模板',
    template_export: '导出模板',
    template_import: '导入模板',
    template_linux_prepare: 'Linux 模板预处理',
    delete_template: '删除模板',
    create: '普通创建',
    lightweight_vm_provision: '轻量云开通',
    lightweight_runtime_quota_shutdown: '轻量云时长关机',
    delete: '删除虚拟机',
    snapshot: '快照操作',
    export: '导出虚拟机',
    import: '导入虚拟机',
    vm_migrate: '迁移虚拟机',
    vm_disk_migrate: '迁移硬盘',
    storage_format: '格式化存储',
    storage_create_partition: '创建分区',
    storage_delete_partitions: '删除分区',
    ovs_repair: 'OVS 修复',
    network_capture: '网络抓包',
    vm_schedule_action: '虚拟机定时任务',
  }
  return map[type] || type
}

const typeTagType = (type) => {
  const map = {
    clone: 'primary',
    batch: 'primary',
    reinstall: 'warning',
    prepare: 'success',
    template_export: 'success',
    template_import: 'primary',
    template_linux_prepare: 'warning',
    delete_template: 'danger',
    create: '',
    lightweight_vm_provision: 'primary',
    lightweight_runtime_quota_shutdown: 'warning',
    delete: 'danger',
    snapshot: 'info',
    export: 'success',
    import: 'primary',
    vm_migrate: 'warning',
    vm_disk_migrate: 'warning',
    storage_format: 'warning',
    storage_create_partition: 'warning',
    storage_delete_partitions: 'danger',
    ovs_repair: 'warning',
    network_capture: 'warning',
    vm_schedule_action: 'primary',
  }
  return map[type] || 'info'
}

const statusText = (status) => {
  const map = {
    pending: '等待中',
    running: '执行中',
    success: '成功',
    failed: '失败',
    canceled: '已取消',
  }
  return map[status] || status
}

const statusTagType = (status) => {
  const map = {
    pending: 'info',
    running: 'primary',
    success: 'success',
    failed: 'danger',
    canceled: 'warning',
  }
  return map[status] || 'info'
}

const progressStatus = (status) => {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'exception'
  if (status === 'canceled') return 'warning'
  return ''
}

// ===================== 时间/JSON 格式化 =====================

const formatTime = (time) => {
  if (!time) return '-'
  const d = new Date(time)
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

const formatJSON = (jsonStr) => {
  try {
    return JSON.stringify(JSON.parse(jsonStr), null, 2)
  } catch {
    return jsonStr
  }
}

const extractDownloadLinks = (resultStr) => {
  if (!resultStr) return []
  try {
    const parsed = JSON.parse(resultStr)
    const links = []
    if (parsed.download_path) {
      links.push({
        label: parsed.file_name ? `下载 ${parsed.file_name}` : '下载文件',
        path: parsed.download_path,
      })
    }
    if (Array.isArray(parsed.extra_downloads)) {
      parsed.extra_downloads.forEach((item) => {
        if (item?.download_path) {
          links.push({
            label: item.label || (item.file_name ? `下载 ${item.file_name}` : '下载附加文件'),
            path: item.download_path,
          })
        }
      })
    }
    return links
  } catch {
    return []
  }
}

// ===================== 数据获取 =====================

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getTaskList({
      page: pagination.value.page,
      page_size: pagination.value.pageSize,
      status: filters.value.status || undefined,
      type: filters.value.type || undefined,
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
    if (!silent) {
      console.error('获取任务详情失败', err)
    }
  } finally {
    detailLoading.value = false
  }
}

// ===================== SSE 实时连接 =====================

const connectSSE = () => {
  const token = localStorage.getItem('token')
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
      // 同时更新详情抽屉（如果打开的是这个任务）
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
    // 断线重连
    if (eventSource) {
      eventSource.close()
    }
    setTimeout(connectSSE, 5000)
  }
}

const updateTaskInList = (event) => {
  const idx = tableData.value.findIndex(t => t.id === event.task_id)
  if (idx !== -1) {
    // 更新已有任务
    tableData.value[idx].status = event.status
    tableData.value[idx].progress = event.progress
    tableData.value[idx].message = event.message
  } else {
    // 新任务，刷新列表
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

// ===================== 操作 =====================

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
  } catch { }
}

const handleClear = async () => {
  try {
    await ElMessageBox.confirm('确定要清理所有已完成/失败/已取消的任务记录吗？', '提示', { type: 'warning' })
    const res = await clearFinishedTasks()
    ElMessage.success(res.message || '清理完成')
    fetchData()
  } catch { }
}

const handleDownload = (link) => {
  const url = getTemplateExportDownloadUrl(link.path)
  if (!url) return
  window.open(url, '_blank')
}

// ===================== 生命周期 =====================

onMounted(() => {
  fetchData()
  connectSSE()
})

onBeforeUnmount(() => {
  disconnectSSE()
})
</script>

<style scoped>
.task-list-container {
  padding: 0;
}

.task-list-content {
  background: var(--el-bg-color);
  border-radius: 4px;
}

.header-bar {
  margin-bottom: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-bar h2 {
  margin: 0;
  font-size: 18px;
  color: var(--el-text-color-primary);
}

.header-actions {
  display: flex;
  gap: 8px;
}

.filter-bar {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 8px;
}

.sse-status {
  display: flex;
  align-items: center;
}

.pagination-bar {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
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

@media (max-width: 768px) {
  .task-container {
    padding: 4px;
  }

  .task-container h2 {
    font-size: 16px;
    margin-bottom: 12px;
  }
}
</style>
