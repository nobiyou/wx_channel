<template>
    <div class="min-h-screen bg-bg p-4 lg:p-10 font-sans text-text">
    <Toast />
    <ConfirmDialog />

    <!-- Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3 lg:gap-4 mb-6 lg:mb-8">
        <div class="bg-surface-0 rounded-2xl p-4 lg:p-5 border border-surface-100 shadow-sm hover:shadow-md transition-shadow">
            <div class="flex justify-between items-center">
                <div>
                    <p class="text-xs text-text-muted font-medium uppercase tracking-wider mb-1">总用户数</p>
                    <p class="text-2xl lg:text-3xl font-bold text-text">{{ stats.users || 0 }}</p>
                </div>
                <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-xl bg-blue-500/10 text-blue-500 flex items-center justify-center text-lg lg:text-xl">
                    <i class="pi pi-users"></i>
                </div>
            </div>
        </div>
        <div class="bg-surface-0 rounded-2xl p-4 lg:p-5 border border-surface-100 shadow-sm hover:shadow-md transition-shadow">
            <div class="flex justify-between items-center">
                <div>
                    <p class="text-xs text-text-muted font-medium uppercase tracking-wider mb-1">活跃设备</p>
                    <p class="text-2xl lg:text-3xl font-bold text-text">{{ stats.devices || 0 }}</p>
                </div>
                <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-xl bg-purple-500/10 text-purple-500 flex items-center justify-center text-lg lg:text-xl">
                    <i class="pi pi-desktop"></i>
                </div>
            </div>
        </div>
        <div class="bg-surface-0 rounded-2xl p-4 lg:p-5 border border-surface-100 shadow-sm hover:shadow-md transition-shadow">
            <div class="flex justify-between items-center">
                <div>
                    <p class="text-xs text-text-muted font-medium uppercase tracking-wider mb-1">交易记录</p>
                    <p class="text-2xl lg:text-3xl font-bold text-text">{{ stats.transactions || 0 }}</p>
                </div>
                <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-xl bg-green-500/10 text-green-500 flex items-center justify-center text-lg lg:text-xl">
                    <i class="pi pi-receipt"></i>
                </div>
            </div>
        </div>
        <div class="bg-surface-0 rounded-2xl p-4 lg:p-5 border border-surface-100 shadow-sm hover:shadow-md transition-shadow">
            <div class="flex justify-between items-center">
                <div>
                    <p class="text-xs text-text-muted font-medium uppercase tracking-wider mb-1">积分流通量</p>
                    <p class="text-2xl lg:text-3xl font-bold text-amber-500">{{ stats.total_credits || 0 }}</p>
                </div>
                <div class="w-10 h-10 lg:w-12 lg:h-12 rounded-xl bg-amber-500/10 text-amber-500 flex items-center justify-center text-lg lg:text-xl">
                    <i class="pi pi-dollar"></i>
                </div>
            </div>
        </div>
    </div>

    <!-- Main Content with Custom Tabs -->
    <div class="bg-surface-0 rounded-2xl border border-surface-100 shadow-sm overflow-hidden flex flex-col h-full">
        <!-- Tab Header -->
        <div class="flex items-center gap-1 px-4 pt-4 pb-0 border-b border-surface-100 overflow-x-auto scrollbar-hide shrink-0">
            <button
                v-for="(tab, idx) in tabs" :key="idx"
                @click="activeTab = idx"
                class="flex items-center gap-2 px-3 py-2 lg:px-5 lg:py-3 text-xs lg:text-sm font-medium border-b-2 -mb-px transition-all cursor-pointer whitespace-nowrap"
                :class="activeTab === idx
                    ? 'border-primary text-primary'
                    : 'border-transparent text-text-muted hover:text-text hover:border-surface-300'"
            >
                <i :class="tab.icon" class="text-xs"></i>
                {{ tab.label }}
                <span v-if="tab.count !== undefined" class="text-[10px] px-1.5 py-0.5 rounded-full ml-0.5"
                    :class="activeTab === idx ? 'bg-primary/10 text-primary' : 'bg-surface-100 text-text-muted'">
                    {{ tab.count }}
                </span>
            </button>
        </div>

        <!-- Tab Panels -->
        <!-- Users Tab -->
        <div v-show="activeTab === 0">
            <DataTable :value="users" paginator :rows="10" :loading="loading"
                tableStyle="min-width: 50rem" :rowHover="true"
                class="admin-table" responsiveLayout="scroll">
                <template #empty>
                    <div class="flex flex-col items-center justify-center p-10 text-text-muted">
                        <i class="pi pi-users text-3xl mb-2 text-surface-300"></i>
                        <p class="text-sm">暂无用户数据</p>
                    </div>
                </template>
                <Column field="id" header="ID" sortable style="width: 70px">
                    <template #body="{ data }">
                        <span class="font-mono text-xs px-2 py-1 bg-surface-100 rounded-lg text-text-muted">#{{ data.id }}</span>
                    </template>
                </Column>
                <Column field="email" header="用户邮箱" sortable>
                    <template #body="{ data }">
                        <div class="flex items-center gap-3">
                            <div class="w-8 h-8 rounded-full bg-primary/10 text-primary flex items-center justify-center text-xs font-bold">
                                {{ (data.email || 'U').charAt(0).toUpperCase() }}
                            </div>
                            <span class="text-sm font-medium">{{ data.email || '-' }}</span>
                        </div>
                    </template>
                </Column>
                <Column field="role" header="角色" sortable style="width: 100px">
                    <template #body="{ data }">
                        <Tag :value="data.role === 'admin' ? '管理员' : '用户'" :severity="data.role === 'admin' ? 'warn' : 'info'" rounded class="!text-xs"></Tag>
                    </template>
                </Column>
                <Column field="credits" header="积分" sortable style="width: 100px">
                     <template #body="{ data }">
                        <div class="flex items-center gap-1.5">
                            <i class="pi pi-star-fill text-[10px] text-amber-400"></i>
                            <span class="font-mono font-bold text-sm text-amber-600">{{ data.credits }}</span>
                        </div>
                    </template>
                </Column>
                <Column field="created_at" header="注册时间" sortable style="width: 160px">
                    <template #body="{ data }">
                        <span class="text-xs text-text-muted">{{ formatDate(data.created_at) }}</span>
                    </template>
                </Column>
                <Column header="操作" style="width: 150px" frozen alignFrozen="right">
                    <template #body="{ data }">
                        <div class="flex gap-1">
                            <Button icon="pi pi-wallet" text rounded severity="warn" size="small" v-tooltip.top="'积分'" @click="openEditCredits(data)" />
                            <Button icon="pi pi-shield" text rounded severity="help" size="small" v-tooltip.top="'角色'" @click="openEditRole(data)" />
                            <Button icon="pi pi-trash" text rounded severity="danger" size="small" v-tooltip.top="'删除'" @click="confirmDeleteUser(data)" />
                        </div>
                    </template>
                </Column>
            </DataTable>
        </div>

        <!-- Devices Tab -->
        <div v-show="activeTab === 1">
            <DataTable :value="devices" paginator :rows="10" :loading="loading"
                tableStyle="min-width: 50rem" :rowHover="true"
                class="admin-table" responsiveLayout="scroll">
                <template #empty>
                    <div class="flex flex-col items-center justify-center p-10 text-text-muted">
                        <i class="pi pi-desktop text-3xl mb-2 text-surface-300"></i>
                        <p class="text-sm">暂无设备数据</p>
                    </div>
                </template>
                <Column field="id" header="设备 ID" sortable style="width: 220px">
                     <template #body="{ data }">
                        <span class="font-mono text-xs">{{ data.id }}</span>
                    </template>
                </Column>
                <Column field="hostname" header="主机名" sortable>
                    <template #body="{ data }">
                        <div class="flex items-center gap-2">
                            <i class="pi pi-desktop text-xs text-text-muted"></i>
                            <span class="text-sm">{{ data.hostname }}</span>
                        </div>
                    </template>
                </Column>
                <Column field="version" header="版本" style="width: 90px">
                    <template #body="{ data }">
                        <span class="text-xs px-2 py-0.5 bg-surface-100 rounded-full text-text-muted font-mono">{{ data.version }}</span>
                    </template>
                </Column>
                <Column field="status" header="状态" sortable style="width: 100px">
                     <template #body="{ data }">
                        <div class="flex items-center gap-1.5">
                            <div class="w-2 h-2 rounded-full" :class="data.status === 'online' ? 'bg-green-400' : 'bg-surface-300'"></div>
                            <span class="text-xs" :class="data.status === 'online' ? 'text-green-600 font-medium' : 'text-text-muted'">{{ data.status === 'online' ? '在线' : '离线' }}</span>
                        </div>
                    </template>
                </Column>
                <Column field="user_id" header="绑定用户" sortable style="width: 130px">
                    <template #body="{ data }">
                        <span v-if="data.user_id > 0" class="text-xs font-mono px-2 py-0.5 bg-primary/10 text-primary rounded-full">#{{ data.user_id }}</span>
                        <span v-else class="text-xs text-text-muted">未绑定</span>
                    </template>
                </Column>
                <Column field="last_seen" header="最后在线" sortable style="width: 160px">
                    <template #body="{ data }">
                        <span class="text-xs text-text-muted">{{ formatDate(data.last_seen) }}</span>
                    </template>
                </Column>
                <Column header="操作" style="width: 100px" frozen alignFrozen="right">
                    <template #body="{ data }">
                        <div class="flex gap-1">
                            <Button v-if="data.user_id > 0" icon="pi pi-times-circle" text rounded severity="warn" size="small" v-tooltip.top="'解绑'" @click="confirmUnbindDevice(data)" />
                            <Button icon="pi pi-trash" text rounded severity="danger" size="small" v-tooltip.top="'删除'" @click="confirmDeleteDevice(data)" />
                        </div>
                    </template>
                </Column>
            </DataTable>
        </div>

        <!-- Tasks Tab -->
        <div v-show="activeTab === 2">
            <DataTable :value="tasks" paginator :rows="10" :loading="loading"
                tableStyle="min-width: 50rem" :rowHover="true"
                class="admin-table" responsiveLayout="scroll">
                <template #empty>
                    <div class="flex flex-col items-center justify-center p-10 text-text-muted">
                        <i class="pi pi-list text-3xl mb-2 text-surface-300"></i>
                        <p class="text-sm">暂无任务数据</p>
                    </div>
                </template>
                <Column field="id" header="ID" sortable style="width: 70px">
                    <template #body="{ data }">
                        <span class="font-mono text-xs px-2 py-1 bg-surface-100 rounded-lg text-text-muted">#{{ data.id }}</span>
                    </template>
                </Column>
                <Column field="type" header="类型" sortable>
                    <template #body="{ data }">
                        <Tag :value="data.type" severity="secondary" rounded class="!text-xs"></Tag>
                    </template>
                </Column>
                <Column field="user_id" header="用户" sortable style="width: 90px">
                     <template #body="{ data }">
                        <span class="text-xs font-mono px-2 py-0.5 bg-primary/10 text-primary rounded-full">#{{ data.user_id }}</span>
                    </template>
                </Column>
                <Column field="node_id" header="设备 ID" style="width: 150px">
                     <template #body="{ data }">
                        <span class="font-mono text-xs truncate max-w-32 block" :title="data.node_id">{{ data.node_id }}</span>
                    </template>
                </Column>
                <Column field="status" header="状态" sortable style="width: 100px">
                     <template #body="{ data }">
                        <Tag :value="getTaskStatusLabel(data.status)" :severity="getTaskStatusSeverity(data.status)" rounded class="!text-xs"></Tag>
                    </template>
                </Column>
                <Column field="created_at" header="创建时间" sortable style="width: 160px">
                    <template #body="{ data }">
                        <span class="text-xs text-text-muted">{{ formatDate(data.created_at) }}</span>
                    </template>
                </Column>
                <Column header="操作" style="width: 70px" frozen alignFrozen="right">
                    <template #body="{ data }">
                        <Button icon="pi pi-trash" text rounded severity="danger" size="small" v-tooltip.top="'删除'" @click="confirmDeleteTask(data)" />
                    </template>
                </Column>
            </DataTable>
        </div>

        <!-- Subscriptions Tab -->
        <div v-show="activeTab === 3">
            <DataTable :value="subscriptions" paginator :rows="10" :loading="loading"
                tableStyle="min-width: 50rem" :rowHover="true"
                class="admin-table" responsiveLayout="scroll">
                <template #empty>
                    <div class="flex flex-col items-center justify-center p-10 text-text-muted">
                        <i class="pi pi-bookmark text-3xl mb-2 text-surface-300"></i>
                        <p class="text-sm">暂无订阅数据</p>
                    </div>
                </template>
                <Column field="id" header="ID" sortable style="width: 70px">
                    <template #body="{ data }">
                        <span class="font-mono text-xs px-2 py-1 bg-surface-100 rounded-lg text-text-muted">#{{ data.id }}</span>
                    </template>
                </Column>
                <Column field="nickname" header="昵称" sortable>
                    <template #body="{ data }">
                        <span class="text-sm font-medium">{{ data.nickname }}</span>
                    </template>
                </Column>
                <Column field="finder_id" header="Finder ID" style="width: 150px">
                     <template #body="{ data }">
                        <span class="font-mono text-xs">{{ data.finder_id }}</span>
                    </template>
                </Column>
                <Column field="user_id" header="用户" sortable style="width: 90px">
                     <template #body="{ data }">
                        <span class="text-xs font-mono px-2 py-0.5 bg-primary/10 text-primary rounded-full">#{{ data.user_id }}</span>
                    </template>
                </Column>
                <Column field="video_count" header="视频数" sortable style="width: 90px">
                    <template #body="{ data }">
                        <span class="font-mono text-sm font-bold text-text">{{ data.video_count }}</span>
                    </template>
                </Column>
                <Column field="created_at" header="创建时间" sortable style="width: 160px">
                    <template #body="{ data }">
                        <span class="text-xs text-text-muted">{{ formatDate(data.created_at) }}</span>
                    </template>
                </Column>
                <Column header="操作" style="width: 70px" frozen alignFrozen="right">
                    <template #body="{ data }">
                        <Button icon="pi pi-trash" text rounded severity="danger" size="small" v-tooltip.top="'删除'" @click="confirmDeleteSubscription(data)" />
                    </template>
                </Column>
            </DataTable>
        </div>

        <!-- Database Management Tab -->
        <div v-show="activeTab === 4" class="p-6">
            <div v-if="dbLoading" class="flex items-center justify-center p-10">
                <i class="pi pi-spin pi-spinner text-3xl text-primary"></i>
            </div>
            <div v-else class="space-y-6">
                <!-- Database Stats Cards -->
                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <div class="bg-surface-50 rounded-xl p-5 border border-surface-100">
                        <div class="flex items-center gap-3 mb-2">
                            <div class="w-10 h-10 rounded-lg bg-blue-500/10 text-blue-500 flex items-center justify-center">
                                <i class="pi pi-database text-lg"></i>
                            </div>
                            <div>
                                <p class="text-xs text-text-muted font-medium">数据库大小</p>
                                <p class="text-2xl font-bold text-text">{{ dbStats.size_mb }} MB</p>
                            </div>
                        </div>
                    </div>
                    <div class="bg-surface-50 rounded-xl p-5 border border-surface-100">
                        <div class="flex items-center gap-3 mb-2">
                            <div class="w-10 h-10 rounded-lg bg-green-500/10 text-green-500 flex items-center justify-center">
                                <i class="pi pi-list text-lg"></i>
                            </div>
                            <div>
                                <p class="text-xs text-text-muted font-medium">总记录数</p>
                                <p class="text-2xl font-bold text-text">{{ dbStats.total_records?.toLocaleString() || 0 }}</p>
                            </div>
                        </div>
                    </div>
                    <div class="bg-surface-50 rounded-xl p-5 border border-surface-100">
                        <div class="flex items-center gap-3 mb-2">
                            <div class="w-10 h-10 rounded-lg bg-purple-500/10 text-purple-500 flex items-center justify-center">
                                <i class="pi pi-table text-lg"></i>
                            </div>
                            <div>
                                <p class="text-xs text-text-muted font-medium">数据表数量</p>
                                <p class="text-2xl font-bold text-text">{{ dbStats.tables?.length || 0 }}</p>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Tables Stats -->
                <div class="bg-surface-50 rounded-xl p-5 border border-surface-100">
                    <h3 class="text-sm font-bold text-text mb-4 flex items-center gap-2">
                        <i class="pi pi-table text-primary"></i>
                        数据表统计
                    </h3>
                    <div v-if="dbStats.tables && dbStats.tables.length > 0" class="space-y-2">
                        <div v-for="table in dbStats.tables" :key="table.table_name || table.name" 
                            class="flex items-center justify-between p-3 bg-white rounded-lg border border-surface-100">
                            <div class="flex items-center gap-3">
                                <div class="w-8 h-8 rounded-lg bg-primary/10 text-primary flex items-center justify-center text-xs font-bold">
                                    {{ (table.table_name || table.name || 'T').charAt(0).toUpperCase() }}
                                </div>
                                <div>
                                    <p class="text-sm font-medium text-text">{{ table.table_name || table.name || '-' }}</p>
                                    <p class="text-xs text-text-muted">{{ table.record_count?.toLocaleString() || 0 }} 条记录</p>
                                </div>
                            </div>
                            <div class="text-right">
                                <p class="text-sm font-bold text-text">{{ table.size_mb || '0' }} MB</p>
                                <p class="text-xs text-text-muted">{{ table.oldest_record || '-' }}</p>
                            </div>
                        </div>
                    </div>
                    <div v-else class="flex flex-col items-center justify-center p-6 text-text-muted">
                        <i class="pi pi-table text-2xl mb-2 text-surface-300"></i>
                        <p class="text-sm">暂无数据表信息</p>
                    </div>
                </div>

                <!-- Optimization Section -->
                <div class="bg-surface-50 rounded-xl p-5 border border-surface-100">
                    <h3 class="text-sm font-bold text-text mb-3 flex items-center gap-2">
                        <i class="pi pi-cog text-primary"></i>
                        数据库优化
                    </h3>
                    <p class="text-xs text-text-muted mb-4">
                        执行 ANALYZE 和 VACUUM 操作，优化查询性能并回收空间。建议每月执行一次。
                    </p>
                    <Button 
                        label="立即优化" 
                        icon="pi pi-bolt" 
                        @click="optimizeDatabase" 
                        :loading="optimizing"
                        severity="info"
                        class="!rounded-xl"
                    />
                </div>

                <!-- Archive Section -->
                <div class="bg-surface-50 rounded-xl p-5 border border-surface-100">
                    <h3 class="text-sm font-bold text-text mb-3 flex items-center gap-2">
                        <i class="pi pi-trash text-amber-500"></i>
                        数据归档
                    </h3>
                    <p class="text-xs text-text-muted mb-4">
                        删除旧数据以释放空间。此操作不可恢复，请谨慎操作。
                    </p>
                    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
                        <div>
                            <label class="text-xs font-medium text-text mb-2 block">浏览记录保留</label>
                            <InputNumber 
                                v-model="archiveConfig.browse_months" 
                                suffix=" 个月" 
                                :min="1" 
                                :max="24"
                                showButtons
                                class="w-full"
                            />
                        </div>
                        <div>
                            <label class="text-xs font-medium text-text mb-2 block">下载记录保留</label>
                            <InputNumber 
                                v-model="archiveConfig.download_years" 
                                suffix=" 年" 
                                :min="1" 
                                :max="5"
                                showButtons
                                class="w-full"
                            />
                        </div>
                        <div>
                            <label class="text-xs font-medium text-text mb-2 block">同步历史保留</label>
                            <InputNumber 
                                v-model="archiveConfig.history_months" 
                                suffix=" 个月" 
                                :min="1" 
                                :max="12"
                                showButtons
                                class="w-full"
                            />
                        </div>
                    </div>
                    <Button 
                        label="执行归档" 
                        icon="pi pi-trash" 
                        @click="archiveOldData" 
                        :loading="archiving"
                        severity="danger"
                        class="!rounded-xl"
                    />
                </div>
            </div>
        </div>
    </div>

    <!-- Edit Credits Dialog -->
    <Dialog v-model:visible="dialogs.credits" modal :style="{ width: '28rem' }"
        :pt="{ root: { class: '!rounded-2xl !border-0' }, header: { class: '!pb-2' } }">
        <template #header>
            <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-xl bg-amber-500/10 text-amber-500 flex items-center justify-center">
                    <i class="pi pi-wallet text-lg"></i>
                </div>
                <div>
                    <h3 class="font-bold text-text">编辑积分</h3>
                    <p class="text-xs text-text-muted">{{ selectedUser?.email }}</p>
                </div>
            </div>
        </template>
        <div class="flex flex-col gap-4 pt-2">
            <div class="flex items-center justify-between bg-surface-50 rounded-xl p-4 border border-surface-100">
                <span class="text-sm text-text-muted">当前积分</span>
                <div class="flex items-center gap-1.5">
                    <i class="pi pi-star-fill text-xs text-amber-400"></i>
                    <span class="font-bold text-xl text-amber-600">{{ selectedUser?.credits }}</span>
                </div>
            </div>
            <div class="flex flex-col gap-2">
                <label for="credits" class="font-semibold text-sm text-text">调整金额</label>
                <InputNumber v-model="inputs.creditsAdjustment" inputId="credits" suffix=" 积分" showButtons placeholder="正数增加，负数减少" class="w-full" />
                <div class="flex items-center gap-2 text-xs">
                    <span class="text-text-muted">调整后:</span>
                    <span class="font-bold" :class="(selectedUser?.credits || 0) + (inputs.creditsAdjustment || 0) >= 0 ? 'text-green-600' : 'text-red-600'">
                        {{ (selectedUser?.credits || 0) + (inputs.creditsAdjustment || 0) }} 积分
                    </span>
                </div>
            </div>
            <div class="flex justify-end gap-2 pt-2 border-t border-surface-100">
                <Button label="取消" text severity="secondary" @click="dialogs.credits = false" class="!rounded-xl" />
                <Button label="确认调整" icon="pi pi-check" @click="updateCredits" :loading="actionLoading" class="!rounded-xl" />
            </div>
        </div>
    </Dialog>

    <!-- Edit Role Dialog -->
    <Dialog v-model:visible="dialogs.role" modal :style="{ width: '28rem' }"
        :pt="{ root: { class: '!rounded-2xl !border-0' }, header: { class: '!pb-2' } }">
        <template #header>
            <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-xl bg-purple-500/10 text-purple-500 flex items-center justify-center">
                    <i class="pi pi-shield text-lg"></i>
                </div>
                <div>
                    <h3 class="font-bold text-text">修改角色</h3>
                    <p class="text-xs text-text-muted">{{ selectedUser?.email }}</p>
                </div>
            </div>
        </template>
        <div class="flex flex-col gap-4 pt-2">
            <div class="flex items-center justify-between bg-surface-50 rounded-xl p-4 border border-surface-100">
                <span class="text-sm text-text-muted">当前角色</span>
                <Tag :value="selectedUser?.role === 'admin' ? '管理员' : '用户'" :severity="selectedUser?.role === 'admin' ? 'warn' : 'info'" rounded></Tag>
            </div>
            <div class="flex flex-col gap-2">
                <label class="font-semibold text-sm text-text">选择新角色</label>
                <div class="flex gap-3">
                    <button
                        v-for="role in ['user', 'admin']" :key="role"
                        @click="inputs.newRole = role"
                        class="flex-1 flex items-center gap-3 p-4 rounded-xl border-2 transition-all cursor-pointer"
                        :class="inputs.newRole === role
                            ? 'border-primary bg-primary/5'
                            : 'border-surface-100 hover:border-surface-200'"
                    >
                        <i :class="role === 'admin' ? 'pi pi-shield text-amber-500' : 'pi pi-user text-blue-500'" class="text-lg"></i>
                        <div class="text-left">
                            <p class="text-sm font-bold text-text">{{ role === 'admin' ? '管理员' : '普通用户' }}</p>
                            <p class="text-[10px] text-text-muted">{{ role === 'admin' ? '拥有所有权限' : '基础功能权限' }}</p>
                        </div>
                    </button>
                </div>
            </div>
            <div class="flex justify-end gap-2 pt-2 border-t border-surface-100">
                <Button label="取消" text severity="secondary" @click="dialogs.role = false" class="!rounded-xl" />
                <Button label="确认修改" icon="pi pi-check" @click="updateRole" :loading="actionLoading" class="!rounded-xl" />
            </div>
        </div>
    </Dialog>

  </div>
</template>

<script setup>
import { ref, onMounted, computed, watch } from 'vue'
import axios from 'axios'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'

// PrimeVue
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Tag from 'primevue/tag'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import Tooltip from 'primevue/tooltip'

const vTooltip = Tooltip

const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const stats = ref({})
const users = ref([])
const devices = ref([])
const tasks = ref([])
const subscriptions = ref([])
const loading = ref(true)
const activeTab = ref(0)

const tabs = computed(() => [
    { label: '用户列表', icon: 'pi pi-users', count: users.value.length },
    { label: '设备管理', icon: 'pi pi-desktop', count: devices.value.length },
    { label: '任务监控', icon: 'pi pi-list', count: tasks.value.length },
    { label: '订阅管理', icon: 'pi pi-bookmark', count: subscriptions.value.length },
    { label: '数据库管理', icon: 'pi pi-database', count: undefined },
])

const dialogs = ref({
    credits: false,
    role: false
})
const selectedUser = ref(null)
const inputs = ref({
    creditsAdjustment: 0,
    newRole: 'user'
})
const actionLoading = ref(false)

onMounted(() => {
    fetchData()
})

const fetchData = async () => {
    loading.value = true
    try {
        const [statsRes, usersRes] = await Promise.all([
            axios.get('/api/admin/stats'),
            axios.get('/api/admin/users')
        ])
        stats.value = statsRes.data
        users.value = usersRes.data.list

        const [devicesRes, tasksRes, subsRes] = await Promise.all([
             axios.get('/api/admin/devices'),
             axios.get('/api/admin/tasks'),
             axios.get('/api/admin/subscriptions')
        ])
        devices.value = devicesRes.data || []
        tasks.value = tasksRes.data.list || []
        subscriptions.value = subsRes.data || []

    } catch (err) {
        if (err.response && err.response.status === 403) {
            toast.add({ severity: 'error', summary: '拒绝访问', detail: '需要管理员权限', life: 3000 })
            router.push('/dashboard')
        } else {
             toast.add({ severity: 'error', summary: '错误', detail: '数据加载失败', life: 3000 })
        }
    } finally {
        loading.value = false
    }
}

const formatDate = (dateStr) => {
    if (!dateStr) return '-'
    return new Date(dateStr).toLocaleString('zh-CN', {
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit'
    })
}

const getTaskStatusSeverity = (status) => {
    if (status === 'success' || status === 'completed') return 'success'
    if (status === 'failed' || status === 'error') return 'danger'
    if (status === 'running' || status === 'processing') return 'info'
    if (status === 'timeout') return 'warn'
    return 'warn'
}

const getTaskStatusLabel = (status) => {
    switch (status) {
        case 'success': return '成功'
        case 'failed': return '失败'
        case 'running': return '运行中'
        case 'pending': return '等待中'
        case 'timeout': return '超时'
        case 'completed': return '完成'
        default: return status
    }
}

// User Actions
const openEditCredits = (user) => {
    selectedUser.value = user
    inputs.value.creditsAdjustment = 0
    dialogs.value.credits = true
}

const updateCredits = async () => {
    if (!inputs.value.creditsAdjustment) return
    actionLoading.value = true
    try {
        await axios.post('/api/admin/user/credits', {
            user_id: selectedUser.value.id,
            adjustment: inputs.value.creditsAdjustment
        })
        selectedUser.value.credits += inputs.value.creditsAdjustment
        stats.value.total_credits += inputs.value.creditsAdjustment
        dialogs.value.credits = false
        toast.add({ severity: 'success', summary: '成功', detail: '积分已更新', life: 3000 })
    } catch (e) {
         toast.add({ severity: 'error', summary: '错误', detail: '更新失败', life: 3000 })
    } finally {
        actionLoading.value = false
    }
}

const openEditRole = (user) => {
    selectedUser.value = user
    inputs.value.newRole = user.role
    dialogs.value.role = true
}

const updateRole = async () => {
    actionLoading.value = true
    try {
        await axios.post('/api/admin/user/role', {
            user_id: selectedUser.value.id,
            role: inputs.value.newRole
        })
        selectedUser.value.role = inputs.value.newRole
        dialogs.value.role = false
        toast.add({ severity: 'success', summary: '成功', detail: '角色已更新', life: 3000 })
    } catch (e) {
         toast.add({ severity: 'error', summary: '错误', detail: '更新失败', life: 3000 })
    } finally {
         actionLoading.value = false
    }
}

const confirmDeleteUser = (user) => {
    confirm.require({
        message: `确定要删除用户 ${user.email} 吗？此操作不可恢复。`,
        header: '确认删除',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: { label: '取消', severity: 'secondary', outlined: true },
        acceptProps: { label: '删除', severity: 'danger' },
        accept: async () => {
            try {
                await axios.delete(`/api/admin/user/${user.id}`)
                users.value = users.value.filter(u => u.id !== user.id)
                toast.add({ severity: 'success', summary: '已删除', detail: '用户已删除', life: 3000 })
            } catch (e) {
                toast.add({ severity: 'error', summary: '错误', detail: '删除失败', life: 3000 })
            }
        }
    })
}

// Device Actions
const confirmUnbindDevice = (device) => {
    confirm.require({
        message: `确定要解绑设备 ${device.id} 吗？`,
        header: '确认解绑',
        icon: 'pi pi-info-circle',
        rejectProps: { label: '取消', severity: 'secondary', outlined: true },
        acceptProps: { label: '解绑', severity: 'warn' },
        accept: async () => {
            try {
                await axios.post('/api/admin/device/unbind', { device_id: device.id })
                device.user_id = 0
                toast.add({ severity: 'success', summary: '已解绑', detail: '设备已解绑', life: 3000 })
            } catch (e) {
                toast.add({ severity: 'error', summary: '错误', detail: '解绑失败', life: 3000 })
            }
        }
    })
}

const confirmDeleteDevice = (device) => {
     confirm.require({
        message: `确定要删除设备 ${device.id} 吗？`,
        header: '确认删除',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: { label: '取消', severity: 'secondary', outlined: true },
        acceptProps: { label: '删除', severity: 'danger' },
        accept: async () => {
            try {
                await axios.delete(`/api/admin/device/${device.id}`)
                devices.value = devices.value.filter(d => d.id !== device.id)
                toast.add({ severity: 'success', summary: '已删除', detail: '设备已删除', life: 3000 })
            } catch (e) {
                 toast.add({ severity: 'error', summary: '错误', detail: '删除失败', life: 3000 })
            }
        }
    })
}

// Task Actions
const confirmDeleteTask = (task) => {
     confirm.require({
        message: `确定要删除任务 #${task.id} 吗？`,
        header: '确认删除',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: { label: '取消', severity: 'secondary', outlined: true },
        acceptProps: { label: '删除', severity: 'danger' },
        accept: async () => {
            try {
                await axios.delete(`/api/admin/task/${task.id}`)
                tasks.value = tasks.value.filter(t => t.id !== task.id)
                toast.add({ severity: 'success', summary: '已删除', detail: '任务已删除', life: 3000 })
            } catch (e) {
                 toast.add({ severity: 'error', summary: '错误', detail: '删除失败', life: 3000 })
            }
        }
    })
}

// Subscription Actions
const confirmDeleteSubscription = (sub) => {
     confirm.require({
        message: `确定要删除订阅 ${sub.nickname} 吗？`,
        header: '确认删除',
        icon: 'pi pi-exclamation-triangle',
        rejectProps: { label: '取消', severity: 'secondary', outlined: true },
        acceptProps: { label: '删除', severity: 'danger' },
        accept: async () => {
            try {
                await axios.delete(`/api/admin/subscription/${sub.id}`)
                subscriptions.value = subscriptions.value.filter(s => s.id !== sub.id)
                toast.add({ severity: 'success', summary: '已删除', detail: '订阅已删除', life: 3000 })
            } catch (e) {
                 toast.add({ severity: 'error', summary: '错误', detail: '删除失败', life: 3000 })
            }
        }
    })
}

// ===== 数据库管理 =====
const dbStats = ref({
    tables: [],
    size_mb: '0',
    total_records: 0
})
const dbLoading = ref(false)
const optimizing = ref(false)
const archiving = ref(false)
const archiveConfig = ref({
    browse_months: 6,
    download_years: 1,
    history_months: 3
})

// 加载数据库统计
const loadDatabaseStats = async () => {
    dbLoading.value = true
    try {
        const token = localStorage.getItem('token')
        const res = await fetch('/api/admin/database/stats', {
            headers: { 'Authorization': `Bearer ${token}` }
        })
        const data = await res.json()
        if (data.code === 0) {
            dbStats.value = data.data
        }
    } catch (e) {
        console.error('Failed to load database stats:', e)
        toast.add({ severity: 'error', summary: '错误', detail: '加载数据库统计失败', life: 3000 })
    } finally {
        dbLoading.value = false
    }
}

// 优化数据库
const optimizeDatabase = async () => {
    confirm.require({
        message: '优化数据库会执行 ANALYZE 和 VACUUM 操作，可能需要几分钟时间。是否继续？',
        header: '确认优化',
        icon: 'pi pi-exclamation-triangle',
        acceptLabel: '确认',
        rejectLabel: '取消',
        accept: async () => {
            optimizing.value = true
            try {
                const token = localStorage.getItem('token')
                const res = await fetch('/api/admin/database/optimize', {
                    method: 'POST',
                    headers: { 'Authorization': `Bearer ${token}` }
                })
                const data = await res.json()
                if (data.code === 0) {
                    toast.add({ severity: 'success', summary: '成功', detail: '数据库优化完成', life: 3000 })
                    await loadDatabaseStats()
                } else {
                    toast.add({ severity: 'error', summary: '错误', detail: data.message || '优化失败', life: 3000 })
                }
            } catch (e) {
                console.error('Failed to optimize database:', e)
                toast.add({ severity: 'error', summary: '错误', detail: '优化数据库失败', life: 3000 })
            } finally {
                optimizing.value = false
            }
        }
    })
}

// 归档旧数据
const archiveOldData = async () => {
    confirm.require({
        message: `将删除 ${archiveConfig.value.browse_months} 个月前的浏览记录、${archiveConfig.value.download_years} 年前的下载记录和 ${archiveConfig.value.history_months} 个月前的同步历史。此操作不可恢复！`,
        header: '确认归档',
        icon: 'pi pi-exclamation-triangle',
        acceptLabel: '确认删除',
        rejectLabel: '取消',
        acceptClass: 'p-button-danger',
        accept: async () => {
            archiving.value = true
            try {
                const token = localStorage.getItem('token')
                const res = await fetch('/api/admin/database/archive', {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${token}`,
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(archiveConfig.value)
                })
                const data = await res.json()
                if (data.code === 0) {
                    toast.add({ 
                        severity: 'success', 
                        summary: '成功', 
                        detail: `已删除 ${data.data.total_deleted} 条记录`, 
                        life: 5000 
                    })
                    await loadDatabaseStats()
                } else {
                    toast.add({ severity: 'error', summary: '错误', detail: data.message || '归档失败', life: 3000 })
                }
            } catch (e) {
                console.error('Failed to archive data:', e)
                toast.add({ severity: 'error', summary: '错误', detail: '归档数据失败', life: 3000 })
            } finally {
                archiving.value = false
            }
        }
    })
}

// 监听标签切换，加载数据库统计
watch(activeTab, (newTab) => {
    if (newTab === 4) { // 数据库管理标签
        loadDatabaseStats()
    }
})
</script>

<style scoped>
/* DataTable Styling */
:deep(.admin-table .p-datatable-thead > tr > th) {
    background-color: var(--color-surface-50);
    color: var(--p-text-muted-color);
    font-weight: 600;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--p-surface-200);
    border-top: none;
}
:deep(.admin-table .p-datatable-tbody > tr > td) {
    padding: 0.625rem 1rem;
    border-bottom: 1px solid var(--p-surface-100);
    font-size: 0.875rem;
}
:deep(.admin-table .p-datatable-tbody > tr:last-child > td) {
    border-bottom: none;
}
:deep(.admin-table .p-datatable-tbody > tr:hover > td) {
    background-color: var(--color-surface-50) !important;
}
:deep(.admin-table .p-datatable-tbody > tr.p-datatable-row-odd) {
    background-color: transparent;
}
:deep(.admin-table .p-paginator) {
    border: none;
    background: transparent;
    padding: 0.5rem 1rem;
    border-top: 1px solid var(--p-surface-100);
}
</style>

<!-- 全局样式：美化弹窗 -->

