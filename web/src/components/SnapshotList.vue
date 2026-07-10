<template>
  <div class="snapshot-list-container">
    <div style="margin-bottom: 20px;">
      <el-button type="primary" icon="Plus" :disabled="createDisabled" @click="handleCreate">创建快照</el-button>
      <el-button icon="Refresh" @click="fetchData">刷新</el-button>
      <el-button type="danger" plain :disabled="tableData.length === 0" @click="handleDeleteAll">删除全部快照</el-button>
      <el-tag v-if="quotaInfo" style="margin-left: 12px;" :type="quotaExceeded ? 'danger' : 'info'">
        {{ quotaText }}
      </el-tag>
    </div>
    <el-table :data="tableData" border style="width: 100%" v-loading="loading">
      <el-table-column prop="name" label="名称" min-width="200" show-overflow-tooltip />
      <el-table-column prop="created_at" label="创建时间" width="180" />
      <el-table-column label="类型" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.location === 'external' || row.state === 'disk-snapshot'" type="warning" size="small">外部快照</el-tag>
          <el-tag v-else type="success" size="small">内部快照</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <span>{{ stateLabel(row.state) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="120" show-overflow-tooltip />
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <div class="action-cell">
            <el-tag v-if="row.is_current" size="small" type="success">当前</el-tag>
            <el-button size="small" type="warning" @click="handleRestore(row)">恢复</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog width="520px" title="新建快照" v-model="createVisible" append-to-body>
      <el-form :model="form" ref="formRef" label-width="100px">
        <el-form-item label="描述" prop="description">
          <el-input v-model="form.description" type="textarea" />
        </el-form-item>
        <el-form-item label="包含内存" v-if="vmIsRunning">
          <el-checkbox v-model="form.include_memory">创建快照时保存虚拟机内存状态</el-checkbox>
          <div class="el-form-item__help" style="color: #909399; font-size: 12px; line-height: 1.4; margin-top: 4px;">
            勾选后将创建包含内存的内部快照，恢复时虚拟机将回到运行状态。<br/>
            不勾选则创建仅磁盘的外部快照。<br/>
            <span style="color: #E6A23C;">⚠ 内存快照耗时取决于虚拟机内存大小，大内存虚拟机可能需要数分钟，请耐心等待。</span>
          </div>
        </el-form-item>
        <el-form-item label="创建方式" v-if="vmIsRunning && form.include_memory">
          <el-radio-group v-model="form.pause_for_memory_snapshot">
            <el-radio :value="true">暂停后创建（推荐）</el-radio>
            <el-radio :value="false">不主动暂停（实验）</el-radio>
          </el-radio-group>
          <div class="el-form-item__help" style="color: #909399; font-size: 12px; line-height: 1.4; margin-top: 4px;">
            <template v-if="form.pause_for_memory_snapshot">
              面板会先暂停虚拟机，快照写入完成后自动恢复运行，一致性更稳但期间业务会停顿。<br/>
              整个过程约需数秒至数分钟，取决于虚拟机内存大小。
            </template>
            <template v-else>
              面板不主动暂停虚拟机，由 libvirt/QEMU 自行管理快照创建。<br/>
              <span style="color: #E6A23C;">⚠ 注意：QEMU 保存内存状态时，虚拟机仍会自动进入 paused (saving) 状态，
              这不是面板行为，而是虚拟化层的固有机制。该模式仅减少面板层面的暂停窗口，
              但不能避免虚拟机暂停。</span>
            </template>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="submitCreate">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { getSnapshots, createSnapshot, revertSnapshot, deleteSnapshot, deleteAllSnapshots } from '@/api/vm'
import { ElMessage, ElMessageBox } from 'element-plus'

const props = defineProps({
  vmName: {
    type: String,
    required: true
  },
  vmStatus: {
    type: String,
    default: ''
  }
})
const emit = defineEmits(['quota-change'])

const createVisible = ref(false)
const loading = ref(false)
const submitLoading = ref(false)
const tableData = ref([])
const quotaInfo = ref(null)
const formRef = ref(null)

const form = reactive({ description: '', include_memory: false, pause_for_memory_snapshot: true })
const vmIsRunning = computed(() => props.vmStatus === 'running')
const createDisabled = computed(() => quotaInfo.value && quotaInfo.value.max_snapshots > 0 && (quotaInfo.value.used_snapshots || 0) >= quotaInfo.value.max_snapshots)
const quotaExceeded = computed(() => createDisabled.value)
const quotaText = computed(() => {
  if (!quotaInfo.value) return ''
  const used = quotaInfo.value.used_snapshots || 0
  const max = quotaInfo.value.max_snapshots || 0
  if (max > 0) {
    return `快照配额：${used} / ${max}`
  }
  return `快照配额：已用 ${used} / 不限`
})

// 快照状态显示文字
const stateLabel = (state) => {
  const map = {
    'running': '运行中',
    'shutoff': '关机',
    'disk-snapshot': '仅磁盘',
    'paused': '暂停'
  }
  return map[state] || state
}

const fetchData = async () => {
  if (!props.vmName) return
  loading.value = true
  try {
    const res = await getSnapshots(props.vmName)
    tableData.value = res.data || []
    quotaInfo.value = res.quota || null
    emit('quota-change', quotaInfo.value)
  } catch (err) {
    quotaInfo.value = null
    emit('quota-change', null)
    console.error(err)
  } finally {
    loading.value = false
  }
}

watch(() => props.vmName, (newVal) => {
  if (newVal) {
    fetchData()
  }
})

onMounted(() => {
  fetchData()
})

const handleCreate = () => {
  if (createDisabled.value) {
    ElMessage.warning('当前快照数量已达到配额上限，请先删除旧快照或联系管理员调整配额')
    return
  }
  form.description = ''
  form.include_memory = vmIsRunning.value
  form.pause_for_memory_snapshot = true
  createVisible.value = true
}

const submitCreate = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (valid) {
      submitLoading.value = true
      try {
        const includeMemory = vmIsRunning.value ? form.include_memory : false
        const payload = {
          description: form.description,
          include_memory: includeMemory,
          pause_for_memory_snapshot: includeMemory ? form.pause_for_memory_snapshot : true
        }
        await submitCreateSnapshot(payload)
        ElMessage.success('快照创建任务已提交，可在任务中心查看进度')
        createVisible.value = false
      } catch (err) {
        console.error(err)
      } finally {
        submitLoading.value = false
      }
    }
  })
}

const submitCreateSnapshot = async (payload) => {
  try {
    return await createSnapshot(props.vmName, payload)
  } catch (err) {
    if (!err?.response?.data?.data?.require_nvram_fix) {
      throw err
    }
    const message = err.response?.data?.message || '当前虚拟机需要先修复 UEFI NVRAM 才能创建内存快照。是否立即正常关机、修复并重新开机后继续创建快照？'
    await ElMessageBox.confirm(message, '修复 UEFI NVRAM', {
      type: 'warning',
      confirmButtonText: '立即修复并创建',
      cancelButtonText: '取消'
    })
    return createSnapshot(props.vmName, { ...payload, auto_fix_nvram: true })
  }
}

const handleRestore = async (row) => {
  const isExternal = row.location === 'external' || row.state === 'disk-snapshot'
  const message = isExternal
    ? `快照 [${row.name}] 是外部快照，恢复时将：\n1. 关闭虚拟机\n2. 切回快照创建时的磁盘状态\n3. 重新启动虚拟机\n\n恢复流程不会合并或删除快照后的增量文件，也不会自动删除快照记录。\n确定要继续吗？`
    : `确定要恢复到快照 [${row.name}] 吗？当前未保存的数据将丢失。`

  try {
    await ElMessageBox.confirm(message, '恢复快照', {
      type: 'warning',
      confirmButtonText: '确定恢复',
      cancelButtonText: '取消'
    })
    await revertSnapshot(props.vmName, row.name)
    ElMessage.success('快照恢复任务已提交，可在任务中心查看进度')
  } catch {}
}

const handleDelete = async (row) => {
  if ((row.children || 0) > 0) {
    ElMessage.warning(`快照 [${row.name}] 还有 ${row.children} 个子快照，不能直接删除父级快照。请先从快照树最末端的子快照开始处理。`)
    return
  }

  const isExternal = row.location === 'external' || row.state === 'disk-snapshot'
  const message = isExternal
    ? `快照 [${row.name}] 是外部快照，删除时将合并增量数据到原始磁盘并清理overlay文件。确定要删除吗？`
    : `确定要删除快照 [${row.name}] 吗？`

  try {
    await ElMessageBox.confirm(message, '删除快照', { type: 'info' })
    await deleteSnapshot(props.vmName, row.name)
    ElMessage.success('快照删除任务已提交，可在任务中心查看进度')
  } catch {}
}

const handleDeleteAll = async () => {
  if (tableData.value.length === 0) {
    ElMessage.info('当前虚拟机没有快照')
    return
  }

  const message = `确定要删除当前虚拟机的全部 ${tableData.value.length} 个快照吗？\n\n系统会按快照树从末端开始删除；外部快照会尽量合并并切回当前磁盘状态。该操作不会回滚虚拟机，但会清空快照记录。`
  try {
    await ElMessageBox.confirm(message, '删除全部快照', {
      type: 'warning',
      confirmButtonText: '删除全部',
      cancelButtonText: '取消'
    })
    await deleteAllSnapshots(props.vmName)
    ElMessage.success('全部快照删除任务已提交，可在任务中心查看进度')
  } catch {}
}

// Expose refresh functionality to parent if needed
defineExpose({ refresh: fetchData })
</script>

<style scoped>
.snapshot-list-container {
  padding: 0;
}

.action-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
}
</style>
