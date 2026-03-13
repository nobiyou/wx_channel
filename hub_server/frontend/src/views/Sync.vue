<template>
  <div class="min-h-screen bg-bg p-4 lg:p-12 font-sans text-text">
    <Toast />
    
    <header class="flex justify-between items-center mb-4 lg:mb-8">
      <div>
        <h1 class="text-2xl lg:text-3xl font-bold text-text">数据同步</h1>
        <p class="text-text-muted text-sm mt-1">客户端自动推送数据，无需手动触发</p>
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
                icon="pi pi-chart-line" 
                text 
                rounded 
                severity="info" 
                size="small" 
                @click="showDetails(data)" 
                v-tooltip="'查看详情'" 
              />
              <Button 
                icon="pi pi-eye" 
                text 
                rounded 
                severity="success" 
                size="small" 
                @click="showBrowseRecords(data)" 
                v-tooltip="'浏览记录'" 
              />
              <Button 
                icon="pi pi-download" 
                text 
                rounded 
                severity="warning" 
                size="small" 
                @click="showDownloadRecords(data)" 
                v-tooltip="'下载记录'" 
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

    <!-- Browse Records Dialog -->
    <Dialog v-model:visible="dialogs.browseRecords" header="浏览记录" modal :style="{ width: '80rem' }" :breakpoints="{ '960px': '90vw', '640px': '95vw' }">
      <div v-if="selectedStatus">
        <p class="text-text-muted mb-4">设备: <span class="font-bold">{{ selectedStatus.device_name }}</span> - 共 {{ browseRecordsTotal }} 条记录</p>
        
        <DataTable 
          :value="browseRecords" 
          :loading="browseRecordsLoading"
          paginator 
          :rows="20"
          :totalRecords="browseRecordsTotal"
          :lazy="true"
          @page="onBrowseRecordsPageChange"
          stripedRows
        >
          <template #empty>
            <div class="text-center p-8 text-text-muted">暂无浏览记录</div>
          </template>

          <Column field="cover_url" header="封面" style="width: 100px">
            <template #body="{ data }">
              <img v-if="data.cover_url" :src="data.cover_url" class="w-16 h-16 object-cover rounded" @error="e => e.target.style.display='none'" />
              <div v-else class="w-16 h-16 bg-surface-100 rounded flex items-center justify-center">
                <i class="pi pi-image text-surface-400"></i>
              </div>
            </template>
          </Column>

          <Column field="title" header="标题" style="min-width: 250px">
            <template #body="{ data }">
              <div class="flex flex-col gap-1">
                <span class="font-medium line-clamp-2">{{ data.title || '无标题' }}</span>
                <span class="text-xs text-text-muted">{{ data.author || '未知作者' }}</span>
              </div>
            </template>
          </Column>

          <Column field="duration" header="时长" style="width: 100px">
            <template #body="{ data }">
              <span v-if="data.duration">{{ formatDuration(data.duration) }}</span>
              <span v-else class="text-text-muted">-</span>
            </template>
          </Column>

          <Column field="resolution" header="分辨率" style="width: 120px">
            <template #body="{ data }">
              <Tag v-if="data.resolution" :value="data.resolution" severity="info" size="small" />
              <span v-else class="text-text-muted">-</span>
            </template>
          </Column>

          <Column header="互动数据" style="width: 200px">
            <template #body="{ data }">
              <div class="flex gap-3 text-xs">
                <span v-if="data.like_count" class="flex items-center gap-1">
                  <i class="pi pi-heart text-red-500"></i> {{ formatNumber(data.like_count) }}
                </span>
                <span v-if="data.comment_count" class="flex items-center gap-1">
                  <i class="pi pi-comment text-blue-500"></i> {{ formatNumber(data.comment_count) }}
                </span>
                <span v-if="data.fav_count" class="flex items-center gap-1">
                  <i class="pi pi-star text-yellow-500"></i> {{ formatNumber(data.fav_count) }}
                </span>
              </div>
            </template>
          </Column>

          <Column field="browse_time" header="浏览时间" sortable style="min-width: 180px">
            <template #body="{ data }">
              {{ formatTime(data.browse_time) }}
            </template>
          </Column>

          <Column header="操作" style="width: 100px">
            <template #body="{ data }">
              <Button 
                icon="pi pi-play" 
                text 
                rounded 
                severity="success" 
                size="small" 
                @click="goToVideoDetail(data)" 
                v-tooltip="'查看详情并播放'" 
              />
            </template>
          </Column>
        </DataTable>
      </div>
    </Dialog>

    <!-- Download Records Dialog -->
    <Dialog v-model:visible="dialogs.downloadRecords" header="下载记录" modal :style="{ width: '80rem' }" :breakpoints="{ '960px': '90vw', '640px': '95vw' }">
      <div v-if="selectedStatus">
        <p class="text-text-muted mb-4">设备: <span class="font-bold">{{ selectedStatus.device_name }}</span> - 共 {{ downloadRecordsTotal }} 条记录</p>
        
        <DataTable 
          :value="downloadRecords" 
          :loading="downloadRecordsLoading"
          paginator 
          :rows="20"
          :totalRecords="downloadRecordsTotal"
          :lazy="true"
          @page="onDownloadRecordsPageChange"
          stripedRows
        >
          <template #empty>
            <div class="text-center p-8 text-text-muted">暂无下载记录</div>
          </template>

          <Column field="cover_url" header="封面" style="width: 100px">
            <template #body="{ data }">
              <img v-if="data.cover_url" :src="data.cover_url" class="w-16 h-16 object-cover rounded" @error="e => e.target.style.display='none'" />
              <div v-else class="w-16 h-16 bg-surface-100 rounded flex items-center justify-center">
                <i class="pi pi-image text-surface-400"></i>
              </div>
            </template>
          </Column>

          <Column field="title" header="标题" style="min-width: 250px">
            <template #body="{ data }">
              <div class="flex flex-col gap-1">
                <span class="font-medium line-clamp-2">{{ data.title || '无标题' }}</span>
                <span class="text-xs text-text-muted">{{ data.author || '未知作者' }}</span>
              </div>
            </template>
          </Column>

          <Column field="status" header="状态" style="width: 100px">
            <template #body="{ data }">
              <Tag 
                :value="data.status === 'completed' ? '完成' : data.status === 'failed' ? '失败' : data.status" 
                :severity="data.status === 'completed' ? 'success' : data.status === 'failed' ? 'danger' : 'info'"
                rounded
              />
            </template>
          </Column>

          <Column field="file_size" header="文件大小" style="width: 120px">
            <template #body="{ data }">
              <span v-if="data.file_size">{{ formatFileSize(data.file_size) }}</span>
              <span v-else class="text-text-muted">-</span>
            </template>
          </Column>

          <Column field="format" header="格式" style="width: 100px">
            <template #body="{ data }">
              <Tag v-if="data.format" :value="data.format.toUpperCase()" severity="secondary" size="small" />
              <span v-else class="text-text-muted">-</span>
            </template>
          </Column>

          <Column field="resolution" header="分辨率" style="width: 120px">
            <template #body="{ data }">
              <Tag v-if="data.resolution" :value="data.resolution" severity="info" size="small" />
              <span v-else class="text-text-muted">-</span>
            </template>
          </Column>

          <Column field="download_time" header="下载时间" sortable style="min-width: 180px">
            <template #body="{ data }">
              {{ formatTime(data.download_time) }}
            </template>
          </Column>
        </DataTable>
      </div>
    </Dialog>

  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { FilterMatchMode } from '@primevue/core/api'
import { useToast } from 'primevue/usetoast'
import axios from 'axios'

const router = useRouter()
const toast = useToast()

const syncStatuses = ref([])
const loading = ref(false)
const selectedStatus = ref(null)
const syncHistory = ref([])
const historyLoading = ref(false)
const browseRecords = ref([])
const browseRecordsLoading = ref(false)
const browseRecordsTotal = ref(0)
const browseRecordsPage = ref(1)
const downloadRecords = ref([])
const downloadRecordsLoading = ref(false)
const downloadRecordsTotal = ref(0)
const downloadRecordsPage = ref(1)

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
  history: false,
  browseRecords: false,
  downloadRecords: false,
  videoPlayer: false
})

const selectedVideo = ref(null)

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

const showBrowseRecords = async (status) => {
  selectedStatus.value = status
  dialogs.value.browseRecords = true
  browseRecordsPage.value = 1
  await loadBrowseRecords()
}

const loadBrowseRecords = async () => {
  browseRecordsLoading.value = true
  try {
    const response = await axios.get('/api/sync/browse', {
      params: {
        machine_id: selectedStatus.value.machine_id,
        page: browseRecordsPage.value,
        page_size: 20
      }
    })
    if (response.data.code === 0) {
      browseRecords.value = response.data.data.records || []
      browseRecordsTotal.value = response.data.data.total || 0
    }
  } catch (error) {
    console.error('Failed to load browse records:', error)
    toast.add({ severity: 'error', summary: '错误', detail: '加载浏览记录失败', life: 3000 })
  } finally {
    browseRecordsLoading.value = false
  }
}

const onBrowseRecordsPageChange = (event) => {
  browseRecordsPage.value = event.page + 1
  loadBrowseRecords()
}

const showDownloadRecords = async (status) => {
  selectedStatus.value = status
  dialogs.value.downloadRecords = true
  downloadRecordsPage.value = 1
  await loadDownloadRecords()
}

const loadDownloadRecords = async () => {
  downloadRecordsLoading.value = true
  try {
    const response = await axios.get('/api/sync/download', {
      params: {
        machine_id: selectedStatus.value.machine_id,
        page: downloadRecordsPage.value,
        page_size: 20
      }
    })
    if (response.data.code === 0) {
      downloadRecords.value = response.data.data.records || []
      downloadRecordsTotal.value = response.data.data.total || 0
    }
  } catch (error) {
    console.error('Failed to load download records:', error)
    toast.add({ severity: 'error', summary: '错误', detail: '加载下载记录失败', life: 3000 })
  } finally {
    downloadRecordsLoading.value = false
  }
}

const onDownloadRecordsPageChange = (event) => {
  downloadRecordsPage.value = event.page + 1
  loadDownloadRecords()
}

const formatDuration = (milliseconds) => {
  if (!milliseconds) return '-'
  // 数据库存储的是毫秒，需要转换为秒
  const seconds = Math.floor(milliseconds / 1000)
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  return `${m}:${s.toString().padStart(2, '0')}`
}

const formatFileSize = (bytes) => {
  if (!bytes) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB'
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

// 跳转到视频详情页
const goToVideoDetail = (record) => {
  // 浏览记录中的 id 字段就是 object_id
  // 但是没有 nonce_id，所以需要特殊处理
  if (record.id) {
    // 如果有 video_url 和 decrypt_key，可以直接播放
    // 将完整的记录信息通过 state 传递给视频详情页
    router.push({
      path: `/video/${record.id}`,
      query: {
        from: 'browse_history' // 标记来源
      },
      state: {
        // 传递浏览记录的完整信息
        browseRecord: {
          id: record.id,
          title: record.title,
          author: record.author,
          cover_url: record.cover_url,
          video_url: record.video_url,
          decrypt_key: record.decrypt_key,
          duration: record.duration,
          resolution: record.resolution,
          file_format: record.file_format, // 传递 file_format 字段
          like_count: record.like_count,
          comment_count: record.comment_count,
          fav_count: record.fav_count
        }
      }
    })
  } else {
    toast.add({ 
      severity: 'warn', 
      summary: '提示', 
      detail: '视频 ID 不存在，无法跳转', 
      life: 3000 
    })
  }
}
</script>
