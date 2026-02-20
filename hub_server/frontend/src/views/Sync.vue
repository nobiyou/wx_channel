<template>
  <div class="min-h-screen bg-bg p-4 lg:p-12 font-sans text-text">
    <Toast />
    <ConfirmDialog />
    
    <header class="flex justify-between items-center mb-4 lg:mb-8">
      <div>
        <h1 class="text-2xl lg:text-3xl font-bold text-text">数据同步</h1>
        <p class="text-text-muted text-sm mt-1">管理设备数据同步状态</p>
      </div>
      <div class="flex gap-2">
        <Button 
          label="刷新" 
          icon="pi pi-refresh" 
          :loading="loading"
          rounded
          size="small"
          @click="refreshSyncStatus"
        />
        <Button 
          label="同步所有设备" 
          icon="pi pi-sync" 
          :loading="syncingAll"
          rounded
          size="small"
          severity="success"
          @click="syncAllDevices"
        />
      </div>
    </header>

    <!-- Stats Cards -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-3 lg:gap-6 mb-6 lg:mb-8">
      <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm border border-surface-100 flex items-center justify-between">
        <div>
          <p class="text-text-muted text-xs lg:text-sm font-medium uppercase tracking-wider mb-1">总设备</p>
          <div class="text-2xl lg:text-3xl font-bold text-text">{{ syncStatuses.length }}</div>
        </div>
        <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-full bg-blue-50 text-blue-500 flex items-center justify-center">
          <i class="pi pi-desktop text-lg lg:text-xl"></i>
        </div>
      </div>
      
      <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm border border-surface-100 flex items-center justify-between">
        <div>
          <p class="text-text-muted text-xs lg:text-sm font-medium uppercase tracking-wider mb-1">同步中</p>
          <div class="text-2xl lg:text-3xl font-bold text-blue-500">{{ syncingCount }}</div>
        </div>
        <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-full bg-blue-50 text-blue-500 flex items-center justify-center">
          <i class="pi pi-spin pi-spinner text-lg lg:text-xl"></i>
        </div>
      </div>
      
      <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm border border-surface-100 flex items-center justify-between">
        <div>
          <p class="text-text-muted text-xs lg:text-sm font-medium uppercase tracking-wider mb-1">成功</p>
          <div class="text-2xl lg:text-3xl font-bold text-green-500">{{ successCount }}</div>
        </div>
        <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-full bg-green-50 text-green-500 flex items-center justify-center">
          <i class="pi pi-check-circle text-lg lg:text-xl"></i>
        </div>
      </div>
      
      <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm border border-surface-100 flex items-center justify-between">
        <div>
          <p class="text-text-muted text-xs lg:text-sm font-medium uppercase tracking-wider mb-1">失败</p>
          <div class="text-2xl lg:text-3xl font-bold" :class="failedCount > 0 ? 'text-red-500' : 'text-text-muted'">{{ failedCount }}</div>
        </div>
        <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-full flex items-center justify-center transition-colors"
             :class="failedCount > 0 ? 'bg-red-50 text-red-500' : 'bg-surface-100 text-text-muted'">
          <i class="pi pi-times-circle text-lg lg:text-xl"></i>
        </div>
      </div>
    </div>

    <!-- Filter Panel -->
    <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm border border-surface-100 mb-6 lg:mb-8">
      <div class="flex flex-col md:flex-row gap-3 lg:gap-4">
        <IconField class="flex-1">
          <InputIcon class="pi pi-search" />
          <InputText v-model="filters['global'].value" placeholder="搜索设备 ID、名称..." class="w-full" size="small" />
        </IconField>
        <Select 
          v-model="filterStatus" 
          :options="statusOptions" 
          optionLabel="label" 
          optionValue="value" 
          placeholder="全部状态" 
          class="w-full md:w-48 !text-sm" 
          showClear 
          size="small" 
        />
      </div>
    </div>

    <!-- Sync Status DataTable -->
    <div class="bg-surface-0 rounded-2xl p-4 lg:p-6 shadow-sm border border-surface-100">
      <DataTable 
        v-model:filters="filters"
        :value="filteredStatuses" 
        :loading="loading"
        paginator 
        :rows="10" 
        :rowsPerPageOptions="[5, 10, 20, 50]"
        stripedRows 
        removableSort
        tableStyle="min-width: 60rem"
        :globalFilterFields="['machine_id', 'device_name']"
      >
        <template #empty>
          <div class="text-center p-8 text-text-muted">暂无同步数据</div>
        </template>

        <Column field="last_sync_status" header="状态" sortable style="width: 120px">
          <template #body="slotProps">
            <Tag 
              v-if="slotProps.data.last_sync_status === 'success'"
              value="成功" 
              severity="success"
              icon="pi pi-check-circle"
              rounded
            />
            <Tag 
              v-else-if="slotProps.data.last_sync_status === 'failed'"
              value="失败" 
              severity="danger"
              icon="pi pi-times-circle"
              rounded
            />
            <Tag 
              v-else-if="slotProps.data.last_sync_status === 'in_progress'"
              value="同步中" 
              severity="info"
              icon="pi pi-spin pi-spinner"
              rounded
            />
            <Tag 
              v-else
              value="未同步" 
              severity="secondary"
              rounded
            />
          </template>
        </Column>

        <Column field="device_name" header="设备" sortable style="min-width: 200px">
          <template #body="{ data }">
            <div class="flex flex-col">
              <span class="font-bold text-text">{{ data.device_name || '未命名设备' }}</span>
              <span class="text-xs font-mono text-text-muted">{{ data.machine_id }}</span>
            </div>
          </template>
        </Column>

        <Column field="browse_record_count" header="浏览记录" sortable style="min-width: 120px">
          <template #body="{ data }">
            <div class="flex items-center gap-2">
              <i class="pi pi-eye text-blue-500"></i>
              <span class="font-mono">{{ formatNumber(data.browse_record_count) }}</span>
            </div>
          </template>
        </Column>

        <Column field="download_record_count" header="下载记录" sortable style="min-width: 120px">
          <template #body="{ data }">
            <div class="flex items-center gap-2">
              <i class="pi pi-download text-green-500"></i>
              <span class="font-mono">{{ formatNumber(data.download_record_count) }}</span>
            </div>
          </template>
        </Column>

        <Column field="last_browse_sync_time" header="浏览同步时间" sortable style="min-width: 180px">
          <template #body="{ data }">
            <span class="text-sm">{{ formatTime(data.last_browse_sync_time) }}</span>
          </template>
        </Column>

        <Column field="last_download_sync_time" header="下载同步时间" sortable style="min-width: 180px">
          <template #body="{ data }">
            <span class="text-sm">{{ formatTime(data.last_download_sync_time) }}</span>
          </template>
        </Column>
        
        <Column header="操作" style="width: 200px">
          <template #body="{ data }">
            <div class="flex gap-2">
              <Button 
                icon="pi pi-sync" 
                text 
                rounded 
                severity="success" 
                size="small" 
                @click="syncDevice(data)" 
                v-tooltip="'立即同步'"
                :loading="data.syncing"
              />
              <Button 
                icon="pi pi-chart-line" 
                text 
                rounded 
                severity="info" 
                size="small" 
                @click="showDetails(data)" 
                v-tooltip="'查看详情'" 
              />
              <Button 
                icon="pi pi-history" 
                text 
                rounded 
                severity="secondary" 
                size="small" 
                @click="showHistory(data)" 
                v-tooltip="'同步历史'" 
              />
            </div>
          </template>
        </Column>
      </DataTable>
    </div>

    <!-- Details Dialog -->
    <Dialog v-model:visible="dialogs.details" header="同步详情" modal :style="{ width: '50rem' }" :breakpoints="{ '960px': '75vw', '640px': '90vw' }">
      <div v-if="selectedStatus" class="space-y-6">
        <!-- Device Info -->
        <div class="bg-surface-50 p-4 rounded-xl">
          <h3 class="font-bold text-lg mb-3">设备信息</h3>
          <div class="space-y-2 text-sm">
            <div class="flex justify-between border-b border-surface-200 pb-2">
              <span class="text-text-muted">设备名称</span>
              <span class="font-bold">{{ selectedStatus.device_name || '未命名' }}</span>
            </div>
            <div class="flex justify-between border-b border-surface-200 pb-2">
              <span class="text-text-muted">Machine ID</span>
              <span class="font-mono text-xs">{{ selectedStatus.machine_id }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-text-muted">同步状态</span>
              <Tag 
                :value="selectedStatus.last_sync_status === 'success' ? '成功' : selectedStatus.last_sync_status === 'failed' ? '失败' : '进行中'" 
                :severity="selectedStatus.last_sync_status === 'success' ? 'success' : selectedStatus.last_sync_status === 'failed' ? 'danger' : 'info'"
                rounded
              />
            </div>
          </div>
        </div>

        <!-- Sync Statistics -->
        <div class="grid grid-cols-2 gap-4">
          <div class="bg-blue-50 p-4 rounded-xl">
            <div class="flex items-center gap-3 mb-2">
              <i class="pi pi-eye text-blue-500 text-2xl"></i>
              <div>
                <p class="text-xs text-text-muted">浏览记录</p>
                <p class="text-2xl font-bold text-blue-500">{{ formatNumber(selectedStatus.browse_record_count) }}</p>
              </div>
            </div>
            <p class="text-xs text-text-muted">最后同步: {{ formatTime(selectedStatus.last_browse_sync_time) }}</p>
          </div>
          
          <div class="bg-green-50 p-4 rounded-xl">
            <div class="flex items-center gap-3 mb-2">
              <i class="pi pi-download text-green-500 text-2xl"></i>
              <div>
                <p class="text-xs text-text-muted">下载记录</p>
                <p class="text-2xl font-bold text-green-500">{{ formatNumber(selectedStatus.download_record_count) }}</p>
              </div>
            </div>
            <p class="text-xs text-text-muted">最后同步: {{ formatTime(selectedStatus.last_download_sync_time) }}</p>
          </div>
        </div>

        <!-- Error Message -->
        <div v-if="selectedStatus.last_sync_error" class="bg-red-50 border border-red-200 p-4 rounded-xl">
          <div class="flex items-start gap-2">
            <i class="pi pi-exclamation-triangle text-red-500 mt-1"></i>
            <div>
              <p class="font-bold text-red-700 mb-1">同步错误</p>
              <p class="text-sm text-red-600">{{ selectedStatus.last_sync_error }}</p>
            </div>
          </div>
        </div>

        <!-- Actions -->
        <div class="flex justify-end gap-2 pt-4 border-t border-surface-200">
          <Button label="关闭" text severity="secondary" @click="dialogs.details = false" />
          <Button label="立即同步" icon="pi pi-sync" @click="syncDevice(selectedStatus); dialogs.details = false" />
        </div>
      </div>
    </Dialog>

    <!-- History Dialog -->
    <Dialog v-model:visible="dialogs.history" header="同步历史" modal :style="{ width: '60rem' }" :breakpoints="{ '960px': '85vw', '640px': '95vw' }">
      <div v-if="selectedStatus">
        <p class="text-text-muted mb-4">设备: <span class="font-bold">{{ selectedStatus.device_name }}</span> ({{ selectedStatus.machine_id }})</p>
        
        <DataTable 
          :value="syncHistory" 
          :loading="historyLoading"
          paginator 
          :rows="10"
          stripedRows
        >
          <template #empty>
            <div class="text-center p-8 text-text-muted">暂无历史记录</div>
          </template>

          <Column field="sync_time" header="同步时间" sortable style="min-width: 180px">
            <template #body="{ data }">
              {{ formatTime(data.sync_time) }}
            </template>
          </Column>

          <Column field="sync_type" header="类型" style="width: 120px">
            <template #body="{ data }">
              <Tag 
                :value="data.sync_type === 'browse' ? '浏览' : '下载'" 
                :severity="data.sync_type === 'browse' ? 'info' : 'success'"
                rounded
              />
            </template>
          </Column>

          <Column field="records_synced" header="同步数量" sortable style="width: 120px">
            <template #body="{ data }">
              <span class="font-mono">{{ formatNumber(data.records_synced) }}</span>
            </template>
          </Column>

          <Column field="status" header="状态" style="width: 100px">
            <template #body="{ data }">
              <Tag 
                :value="data.status === 'success' ? '成功' : '失败'" 
                :severity="data.status === 'success' ? 'success' : 'danger'"
                :icon="data.status === 'success' ? 'pi pi-check' : 'pi pi-times'"
                rounded
              />
            </template>
          </Column>

          <Column field="error_message" header="错误信息" style="min-width: 200px">
            <template #body="{ data }">
              <span v-if="data.error_message" class="text-red-500 text-sm">{{ data.error_message }}</span>
              <span v-else class="text-text-muted">-</span>
            </template>
          </Column>
        </DataTable>
      </div>
    </Dialog>

  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { FilterMatchMode } from '@primevue/core/api'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import axios from 'axios'

const toast = useToast()
const confirm = useConfirm()

const syncStatuses = ref([])
const loading = ref(false)
const syncingAll = ref(false)
const selectedStatus = ref(null)
const syncHistory = ref([])
const historyLoading = ref(false)

const filters = ref({
  global: { value: null, matchMode: FilterMatchMode.CONTAINS },
})

const filterStatus = ref(null)

const statusOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '同步中', value: 'in_progress' },
  { label: '未同步', value: 'never' }
]

const dialogs = ref({
  details: false,
  history: false
})

// Computed
const syncingCount = computed(() => syncStatuses.value.filter(s => s.last_sync_status === 'in_progress').length)
const successCount = computed(() => syncStatuses.value.filter(s => s.last_sync_status === 'success').length)
const failedCount = computed(() => syncStatuses.value.filter(s => s.last_sync_status === 'failed').length)

const filteredStatuses = computed(() => {
  if (!filterStatus.value) return syncStatuses.value
  return syncStatuses.value.filter(s => s.last_sync_status === filterStatus.value)
})

// Auto refresh interval
let refreshInterval = null

onMounted(() => {
  refreshSyncStatus()
  // Auto refresh every 30 seconds
  refreshInterval = setInterval(refreshSyncStatus, 30000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})

// Methods
const refreshSyncStatus = async () => {
  loading.value = true
  try {
    const response = await axios.get('/api/sync/status')
    if (response.data.code === 0) {
      syncStatuses.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to load sync status:', error)
    toast.add({ severity: 'error', summary: '错误', detail: '加载同步状态失败', life: 3000 })
  } finally {
    loading.value = false
  }
}

const syncAllDevices = async () => {
  confirm.require({
    message: '确定要同步所有设备的数据吗？这可能需要一些时间。',
    header: '同步确认',
    icon: 'pi pi-sync',
    acceptLabel: '确认',
    rejectLabel: '取消',
    accept: async () => {
      syncingAll.value = true
      try {
        const response = await axios.post('/api/sync/trigger', { sync_all: true })
        if (response.data.code === 0) {
          toast.add({ severity: 'success', summary: '成功', detail: '已开始同步所有设备', life: 3000 })
          setTimeout(refreshSyncStatus, 2000)
        }
      } catch (error) {
        toast.add({ severity: 'error', summary: '错误', detail: '触发同步失败', life: 3000 })
      } finally {
        syncingAll.value = false
      }
    }
  })
}

const syncDevice = async (status) => {
  status.syncing = true
  try {
    const response = await axios.post('/api/sync/trigger', { machine_id: status.machine_id })
    if (response.data.code === 0) {
      toast.add({ severity: 'success', summary: '成功', detail: '已开始同步设备', life: 2000 })
      setTimeout(refreshSyncStatus, 2000)
    }
  } catch (error) {
    toast.add({ severity: 'error', summary: '错误', detail: '触发同步失败: ' + (error.response?.data?.message || error.message), life: 3000 })
  } finally {
    status.syncing = false
  }
}

const showDetails = (status) => {
  selectedStatus.value = status
  dialogs.value.details = true
}

const showHistory = async (status) => {
  selectedStatus.value = status
  dialogs.value.history = true
  historyLoading.value = true
  
  try {
    const response = await axios.get(`/api/sync/history/${status.machine_id}`)
    if (response.data.code === 0) {
      syncHistory.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to load sync history:', error)
    toast.add({ severity: 'error', summary: '错误', detail: '加载同步历史失败', life: 3000 })
  } finally {
    historyLoading.value = false
  }
}

const formatTime = (time) => {
  if (!time) return '从未'
  const date = new Date(time)
  return date.toLocaleString('zh-CN')
}

const formatNumber = (num) => {
  if (num === null || num === undefined) return '0'
  return num.toLocaleString('zh-CN')
}
</script>
