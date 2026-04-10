// ============================================
// Download Records Page (Task 16)
// ============================================

// Helper function to escape strings for use in onclick attributes
function escapeForOnclick(str) {
    if (!str) return '';
    return escapeHtml(str)
        .replace(/'/g, "\\'")
        .replace(/"/g, '&quot;')
        .replace(/\\/g, '\\\\');
}

// Download records state
const downloadState = {
    records: [],
    currentPage: 1,
    pageSize: 20,
    totalCount: 0,
    totalPages: 0,
    statusFilter: '',
    dateStart: '',
    dateEnd: '',
    selectedIds: new Set(),
    currentDetailId: null
};

// Load download records with pagination and filters - Requirements: 2.1, 2.3, 2.4
async function loadDownloadRecords() {
    if (ConnectionManager.getStatus() !== 'connected') {
        renderDownloadEmptyState('请先连接到本地服务');
        return;
    }

    try {
        const params = {
            page: downloadState.currentPage,
            pageSize: downloadState.pageSize
        };
        
        // Add status filter - Requirements: 2.4
        if (downloadState.statusFilter) {
            params.status = downloadState.statusFilter;
        }
        
        // Add date range filter - Requirements: 2.3
        if (downloadState.dateStart) {
            params.startDate = downloadState.dateStart;
        }
        if (downloadState.dateEnd) {
            params.endDate = downloadState.dateEnd;
        }

        const result = await ApiClient.getDownloadRecords(params);
        
        if (result.success) {
            // Handle paginated response structure: { items, total, page, pageSize, totalPages }
            const data = result.data || {};
            downloadState.records = data.items || [];
            downloadState.totalCount = data.total || 0;
            downloadState.totalPages = data.totalPages || Math.ceil(downloadState.totalCount / downloadState.pageSize);
            
            renderDownloadTable();
            renderDownloadPagination();
            updateDownloadBatchActions();
        } else {
            renderDownloadEmptyState('加载失败: ' + (result.error || '未知错误'));
        }
    } catch (e) {
        console.error('Failed to load download records:', e);
        renderDownloadEmptyState('加载失败: ' + e.message);
    }
}

// Render download records table - Requirements: 2.1
function renderDownloadTable() {
    const tbody = document.getElementById('downloadTableBody');
    
    if (!downloadState.records || downloadState.records.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="8">
                    <div class="table-empty-state">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                            <polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
                        </svg>
                        <p>${hasDownloadFilters() ? '没有找到匹配的记录' : '暂无下载记录'}</p>
                    </div>
                </td>
            </tr>
        `;
        return;
    }

    let html = '';
    for (const record of downloadState.records) {
        const isSelected = downloadState.selectedIds.has(record.id);
        const statusClass = record.status || 'completed';
        const statusText = getStatusText(record.status);
        
        // 封面图处理
        const thumbnail = record.coverUrl 
            ? `<img class="table-thumbnail" src="${escapeHtml(record.coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'"><div class="table-thumbnail-placeholder" style="display:none"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></div>`
            : `<div class="table-thumbnail-placeholder"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></div>`;
        
        // 构建视频信息摘要
        const metaItems = [];
        if (record.author) {
            metaItems.push(`<span class="meta-item"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>${escapeHtml(record.author)}</span>`);
        }
        if (record.duration) {
            metaItems.push(`<span class="meta-item"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>${formatDuration(record.duration)}</span>`);
        }
        if (record.resolution) {
            metaItems.push(`<span class="meta-item"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>${escapeHtml(record.resolution)}</span>`);
        }
        
        html += `
            <tr class="${isSelected ? 'selected' : ''} ${record.status === 'failed' ? 'error-row' : ''}" data-id="${escapeHtml(record.id)}">
                <td onclick="event.stopPropagation();">
                    <input type="checkbox" ${isSelected ? 'checked' : ''} onchange="toggleDownloadSelect('${escapeHtml(record.id)}', this.checked)">
                </td>
                <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">${thumbnail}</td>
                <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                    <div class="video-info-cell">
                        <div class="table-title" title="${escapeHtml(record.title || '无标题')}">${escapeHtml(record.title || '无标题')}</div>
                        ${metaItems.length > 0 ? `<div class="video-meta-row">${metaItems.join('')}</div>` : ''}
                        ${record.errorMessage ? `<div class="table-error-hint" title="${escapeHtml(record.errorMessage)}">⚠ ${escapeHtml(record.errorMessage.substring(0, 30))}${record.errorMessage.length > 30 ? '...' : ''}</div>` : ''}
                    </div>
                </td>
                <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                    <span class="table-meta">${record.fileSize ? formatBytes(record.fileSize) : '-'}</span>
                </td>
                <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                    <span class="table-meta">${escapeHtml(record.format || '-')}</span>
                </td>
                <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                    <span class="table-meta">${record.downloadTime ? formatDateTime(record.downloadTime) : '-'}</span>
                </td>
                <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                    <span class="download-status ${statusClass}">${statusText}</span>
                </td>
                <td onclick="event.stopPropagation();">
                    <div class="table-actions">
                        ${record.status === 'completed' ? `
                        <button class="table-action-btn" onclick="playDownloadedVideo('${escapeHtml(record.id)}')" title="播放视频">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <polygon points="5 3 19 12 5 21 5 3"/>
                            </svg>
                        </button>
                        <button class="table-action-btn" onclick="openDownloadFolder('${escapeHtml(record.id)}')" title="打开文件夹">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                            </svg>
                        </button>
                        ` : ''}
                        ${record.status === 'failed' ? `
                        <button class="table-action-btn" onclick="retryDownload('${escapeHtml(record.id)}')" title="重试下载">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/>
                                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
                            </svg>
                        </button>
                        ` : ''}
                        <button class="table-action-btn danger" onclick="deleteDownloadRecord('${escapeHtml(record.id)}')" title="删除">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                            </svg>
                        </button>
                    </div>
                </td>
            </tr>
        `;
    }
    
    tbody.innerHTML = html;
}

// Render empty state for download records
function renderDownloadEmptyState(message) {
    const tbody = document.getElementById('downloadTableBody');
    tbody.innerHTML = `
        <tr>
            <td colspan="8">
                <div class="table-empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="12" cy="12" r="10"/>
                        <line x1="12" y1="8" x2="12" y2="12"/>
                        <line x1="12" y1="16" x2="12.01" y2="16"/>
                    </svg>
                    <p>${escapeHtml(message)}</p>
                </div>
            </td>
        </tr>
    `;
}

// Render pagination controls for download records
function renderDownloadPagination() {
    const container = document.getElementById('downloadPagination');
    
    if (downloadState.totalPages <= 1) {
        container.innerHTML = downloadState.totalCount > 0 
            ? `<span class="pagination-info">共 ${downloadState.totalCount} 条记录</span>`
            : '';
        return;
    }

    let html = '';
    
    // Previous button
    html += `<button ${downloadState.currentPage === 1 ? 'disabled' : ''} onclick="goToDownloadPage(${downloadState.currentPage - 1})">上一页</button>`;
    
    // Page numbers
    const maxVisiblePages = 5;
    let startPage = Math.max(1, downloadState.currentPage - Math.floor(maxVisiblePages / 2));
    let endPage = Math.min(downloadState.totalPages, startPage + maxVisiblePages - 1);
    
    if (endPage - startPage < maxVisiblePages - 1) {
        startPage = Math.max(1, endPage - maxVisiblePages + 1);
    }
    
    if (startPage > 1) {
        html += `<button onclick="goToDownloadPage(1)">1</button>`;
        if (startPage > 2) {
            html += `<span class="pagination-info">...</span>`;
        }
    }
    
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="${i === downloadState.currentPage ? 'active' : ''}" onclick="goToDownloadPage(${i})">${i}</button>`;
    }
    
    if (endPage < downloadState.totalPages) {
        if (endPage < downloadState.totalPages - 1) {
            html += `<span class="pagination-info">...</span>`;
        }
        html += `<button onclick="goToDownloadPage(${downloadState.totalPages})">${downloadState.totalPages}</button>`;
    }
    
    // Next button
    html += `<button ${downloadState.currentPage === downloadState.totalPages ? 'disabled' : ''} onclick="goToDownloadPage(${downloadState.currentPage + 1})">下一页</button>`;
    
    // Info
    html += `<span class="pagination-info">共 ${downloadState.totalCount} 条记录</span>`;
    
    container.innerHTML = html;
}

// Go to specific page for download records
function goToDownloadPage(page) {
    if (page < 1 || page > downloadState.totalPages) return;
    downloadState.currentPage = page;
    loadDownloadRecords();
}

// Change page size for download records
function changeDownloadPageSize(size) {
    downloadState.pageSize = parseInt(size);
    downloadState.currentPage = 1;
    loadDownloadRecords();
}

// Filter downloads - Requirements: 2.3, 2.4
function filterDownloads() {
    downloadState.statusFilter = document.getElementById('downloadStatusFilter').value;
    downloadState.dateStart = document.getElementById('downloadDateStart').value;
    downloadState.dateEnd = document.getElementById('downloadDateEnd').value;
    downloadState.currentPage = 1;
    loadDownloadRecords();
}

// Check if any filters are active
function hasDownloadFilters() {
    return downloadState.statusFilter || downloadState.dateStart || downloadState.dateEnd;
}

// Clear all filters
function clearDownloadFilters() {
    document.getElementById('downloadStatusFilter').value = '';
    document.getElementById('downloadDateStart').value = '';
    document.getElementById('downloadDateEnd').value = '';
    downloadState.statusFilter = '';
    downloadState.dateStart = '';
    downloadState.dateEnd = '';
    downloadState.currentPage = 1;
    loadDownloadRecords();
}

// Show download detail panel - Requirements: 2.2
async function showDownloadDetail(id) {
    downloadState.currentDetailId = id;
    
    // Find record in current list or fetch from API
    let record = downloadState.records.find(r => r.id === id);
    
    if (!record) {
        try {
            const result = await ApiClient.getDownloadRecord(id);
            if (result.success) {
                record = result.data;
            }
        } catch (e) {
            console.error('Failed to fetch download record detail:', e);
            showMessage('获取详情失败', 'error');
            return;
        }
    }
    
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }
    
    renderDownloadDetailPanel(record);
    document.getElementById('downloadDetailPanel').style.display = 'block';
}

// Render download detail panel - Requirements: 2.2
function renderDownloadDetailPanel(record) {
    const container = document.getElementById('downloadDetailContent');
    const statusClass = record.status || 'completed';
    const statusText = getStatusText(record.status);
    
    // 封面图处理
    const thumbnail = record.coverUrl 
        ? `<img class="video-detail-thumbnail lightbox-trigger" 
               src="${escapeHtml(record.coverUrl)}" 
               alt="" 
               style="width: 100%; border-radius: 8px; cursor: pointer;" 
               data-cover-url="${escapeHtml(record.coverUrl)}"
               data-title="${escapeHtml(record.title || '')}"
               data-duration="${record.duration ? formatDuration(record.duration) : ''}"
               onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">`
        : '';
    
    container.innerHTML = `
        <div class="video-detail-thumbnail-wrapper" style="max-width: 300px;">
            ${thumbnail}
            <div class="video-detail-thumbnail-placeholder" style="${record.coverUrl ? 'display:none' : 'display:flex'}">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                    <polyline points="14 2 14 8 20 8"/>
                </svg>
            </div>
        </div>
        <div class="video-detail-info">
            <div class="video-detail-title">${escapeHtml(record.title || '无标题')}</div>
            
            <div class="video-detail-meta">
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">作者</span>
                    <span class="video-detail-meta-value">${escapeHtml(record.author || '未知')}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">时长</span>
                    <span class="video-detail-meta-value">${record.duration ? formatDuration(record.duration) : '-'}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">分辨率</span>
                    <span class="video-detail-meta-value">${escapeHtml(record.resolution || '-')}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">文件大小</span>
                    <span class="video-detail-meta-value">${record.fileSize ? formatBytes(record.fileSize) : '-'}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">格式</span>
                    <span class="video-detail-meta-value">${escapeHtml(record.format || '-')}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">下载时间</span>
                    <span class="video-detail-meta-value">${record.downloadTime ? formatDateTime(record.downloadTime) : '-'}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">状态</span>
                    <span class="download-status ${statusClass}">${statusText}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">视频ID</span>
                    <span class="video-detail-meta-value" style="font-family: monospace; font-size: 12px;">${escapeHtml(record.videoId || record.id || '-')}</span>
                </div>
            </div>
            
            ${record.filePath ? `
            <div style="margin-top: 16px;">
                <span class="video-detail-meta-label" style="display: block; margin-bottom: 8px;">下载路径</span>
                <div class="download-detail-path" style="background: var(--bg-hover); padding: 10px 12px; border-radius: 4px; font-family: monospace; font-size: 12px; word-break: break-all;">${escapeHtml(record.filePath)}</div>
            </div>
            ` : ''}
            
            ${record.errorMessage ? `
            <div style="margin-top: 16px;">
                <span class="video-detail-meta-label" style="display: block; margin-bottom: 8px;">错误信息</span>
                <div class="download-detail-error" style="background: #fef0f0; color: var(--danger-color); padding: 10px 12px; border-radius: 4px; font-size: 13px;">${escapeHtml(record.errorMessage)}</div>
            </div>
            ` : ''}
            
            <div class="video-detail-actions">
                ${record.status === 'completed' && record.filePath ? `
                <button class="btn btn-primary" onclick="playDownloadedVideo('${escapeHtml(record.id)}')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <polygon points="5 3 19 12 5 21 5 3"/>
                    </svg>
                    播放视频
                </button>
                <button class="btn btn-secondary" onclick="openDownloadFolder('${escapeHtml(record.id)}')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                    </svg>
                    打开文件夹
                </button>
                ` : ''}
                ${record.status === 'failed' ? `
                <button class="btn btn-primary" onclick="retryDownload('${escapeHtml(record.id)}')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/>
                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
                    </svg>
                    重试下载
                </button>
                ` : ''}
                ${record.coverUrl ? `
                <button class="btn btn-secondary lightbox-trigger" 
                        data-cover-url="${escapeHtml(record.coverUrl)}"
                        data-title="${escapeHtml(record.title || '')}"
                        data-duration="${record.duration ? formatDuration(record.duration) : ''}">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                        <circle cx="12" cy="12" r="3"/>
                    </svg>
                    预览封面
                </button>
                ` : ''}
                <button class="btn btn-danger" onclick="deleteDownloadRecord('${escapeHtml(record.id)}')">删除记录</button>
            </div>
        </div>
    `;
}

// Close download detail panel
function closeDownloadDetailPanel() {
    document.getElementById('downloadDetailPanel').style.display = 'none';
    downloadState.currentDetailId = null;
}

// Toggle select all for downloads - Requirements: 9.1
function toggleDownloadSelectAll(checked) {
    if (checked) {
        downloadState.records.forEach(r => downloadState.selectedIds.add(r.id));
    } else {
        downloadState.selectedIds.clear();
    }
    renderDownloadTable();
    updateDownloadBatchActions();
}

// Toggle single selection for downloads - Requirements: 9.1
function toggleDownloadSelect(id, checked) {
    if (checked) {
        downloadState.selectedIds.add(id);
    } else {
        downloadState.selectedIds.delete(id);
    }
    
    // Update select all checkbox
    const selectAllCheckbox = document.getElementById('downloadSelectAll');
    if (selectAllCheckbox) {
        selectAllCheckbox.checked = downloadState.selectedIds.size === downloadState.records.length && downloadState.records.length > 0;
    }
    
    // Update row styling
    const row = document.querySelector(`#downloadTable tr[data-id="${id}"]`);
    if (row) {
        row.classList.toggle('selected', checked);
    }
    
    updateDownloadBatchActions();
}

// Update batch actions bar visibility for downloads
function updateDownloadBatchActions() {
    const batchBar = document.getElementById('downloadBatchActions');
    const countSpan = document.getElementById('downloadSelectedCount');
    
    if (downloadState.selectedIds.size > 0) {
        batchBar.style.display = 'block';
        countSpan.textContent = `已选择 ${downloadState.selectedIds.size} 项`;
    } else {
        batchBar.style.display = 'none';
    }
}

// Clear download selection
function clearDownloadSelection() {
    downloadState.selectedIds.clear();
    document.getElementById('downloadSelectAll').checked = false;
    renderDownloadTable();
    updateDownloadBatchActions();
}

// Delete single download record
async function deleteDownloadRecord(id) {
    const deleteFiles = confirm('是否同时删除已下载的视频文件？\n\n点击"确定"删除记录和文件\n点击"取消"仅删除记录');
    
    try {
        await ApiClient.deleteDownloadRecords([id], deleteFiles);
        showMessage('删除成功', 'success');
        
        // Close detail panel if showing this record
        if (downloadState.currentDetailId === id) {
            closeDownloadDetailPanel();
        }
        
        // Remove from selection
        downloadState.selectedIds.delete(id);
        
        // Reload list
        loadDownloadRecords();
    } catch (e) {
        showMessage('删除失败: ' + e.message, 'error');
    }
}

// Export selected download records - Requirements: 9.4
function exportSelectedDownloads() {
    if (downloadState.selectedIds.size === 0) {
        showMessage('请先选择要导出的记录', 'error');
        return;
    }
    
    const ids = Array.from(downloadState.selectedIds);
    openExportDialog('downloads', ids, ids.length);
}

// Export all download records
function exportAllDownloads() {
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }

    // Use total count from state, or estimate
    const count = downloadState.totalCount || downloadState.records.length;
    openExportDialog('downloads', null, count);
}

// Delete selected download records - Requirements: 9.2
async function deleteSelectedDownloads() {
    if (downloadState.selectedIds.size === 0) {
        showMessage('请先选择要删除的记录', 'error');
        return;
    }
    
    const deleteFiles = confirm(`确定要删除选中的 ${downloadState.selectedIds.size} 条记录吗？\n\n点击"确定"同时删除记录和文件\n点击"取消"仅删除记录`);
    
    try {
        const ids = Array.from(downloadState.selectedIds);
        await ApiClient.deleteDownloadRecords(ids, deleteFiles);
        showMessage(`已删除 ${ids.length} 条记录`, 'success');
        
        // Close detail panel if showing a deleted record
        if (downloadState.selectedIds.has(downloadState.currentDetailId)) {
            closeDownloadDetailPanel();
        }
        
        clearDownloadSelection();
        loadDownloadRecords();
    } catch (e) {
        showMessage('删除失败: ' + e.message, 'error');
    }
}

// Open downloaded file folder
async function openDownloadFolder(id) {
    const record = downloadState.records.find(r => r.id === id);
    if (record && record.filePath) {
        try {
            // 调用后端API打开文件夹
            const result = await ApiClient.openFolder(record.filePath);
            if (result.success) {
                showMessage('已打开文件夹', 'success');
            } else {
                // 如果后端不支持，显示文件路径
                showMessage('文件路径: ' + record.filePath, 'info');
            }
        } catch (e) {
            // 后端可能不支持此功能，显示文件路径
            showMessage('文件路径: ' + record.filePath, 'info');
        }
    } else {
        showMessage('文件路径不可用', 'error');
    }
}

// Play downloaded video
async function playDownloadedVideo(id) {
    const record = downloadState.records.find(r => r.id === id);
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }
    
    if (record.status !== 'completed') {
        showMessage('视频尚未下载完成', 'error');
        return;
    }
    
    if (!record.filePath) {
        showMessage('文件路径不可用', 'error');
        return;
    }
    
    try {
        // 尝试调用后端API播放视频
        const result = await ApiClient.playVideo(record.filePath);
        if (result.success) {
            showMessage('正在打开视频播放器...', 'info');
        } else {
            showMessage('无法播放视频: ' + (result.error || '未知错误'), 'error');
        }
    } catch (e) {
        // 后端可能不支持此功能
        showMessage('文件路径: ' + record.filePath + '\n请手动打开播放', 'info');
    }
}

// Retry failed download
async function retryDownload(id) {
    const record = downloadState.records.find(r => r.id === id);
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }
    
    try {
        // Add back to queue for retry
        await ApiClient.addToQueue([{
            videoId: record.videoId || record.id,
            videoUrl: record.videoUrl || '',
            decryptKey: record.decryptKey || '',
            coverUrl: record.coverUrl || '',
            duration: record.duration || 0,
            resolution: record.resolution || parseResolutionFromUrl(record.videoUrl) || '',
            title: record.title,
            author: record.author,
            size: record.fileSize || 0
        }]);
        showMessage('已添加到下载队列', 'success');
    } catch (e) {
        showMessage('重试失败: ' + e.message, 'error');
    }
}

// ============================================
// Download Queue Page (Task 17) - Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6, 3.3
// ============================================

// Queue state
const queueState = {
    items: [],
    draggedItem: null,
    draggedIndex: -1
};

// Load download queue - Requirements: 10.1
async function loadDownloadQueue() {
    if (ConnectionManager.getStatus() !== 'connected') {
        renderQueueEmptyState('请先连接到本地服务');
        return;
    }

    try {
        const result = await ApiClient.getDownloadQueue();
        
        if (result.success) {
            queueState.items = result.data || [];
            renderQueueList();
            updateQueueStats();
        } else {
            renderQueueEmptyState('加载失败: ' + (result.error || '未知错误'));
        }
    } catch (e) {
        console.error('Failed to load download queue:', e);
        renderQueueEmptyState('加载失败: ' + e.message);
    }
}

// Render queue list with drag-and-drop support - Requirements: 10.1, 10.4
function renderQueueList() {
    const container = document.getElementById('downloadQueueList');
    
    if (!queueState.items || queueState.items.length === 0) {
        container.innerHTML = `
            <div class="queue-empty-state">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/>
                    <line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/>
                    <line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>
                </svg>
                <p>下载队列为空</p>
                <p style="margin-top: 8px; font-size: 12px;">从浏览记录中选择视频添加到队列</p>
            </div>
        `;
        return;
    }

    let html = '';
    for (let i = 0; i < queueState.items.length; i++) {
        const item = queueState.items[i];
        html += renderQueueItem(item, i);
    }
    
    container.innerHTML = html;
    
    // Add drag-and-drop event listeners
    setupDragAndDrop();
}

// Render single queue item - Requirements: 10.1, 3.3, 10.6
function renderQueueItem(item, index) {
    const statusClass = item.status || 'pending';
    const statusText = getQueueStatusText(item.status);
    const progress = calculateProgress(item);
    const progressClass = item.status === 'paused' ? 'paused' : (item.status === 'failed' ? 'failed' : '');
    
    // Calculate speed and ETA - Requirements: 3.3
    const speedText = item.speed > 0 ? formatSpeed(item.speed) : '-';
    const etaText = calculateETA(item);
    const downloadedText = formatBytes(item.downloadedSize || 0);
    const totalText = formatBytes(item.totalSize || 0);
    
    return `
        <div class="queue-item" 
             data-id="${escapeHtml(item.id)}" 
             data-index="${index}"
             draggable="true"
             ondragstart="handleDragStart(event, ${index})"
             ondragend="handleDragEnd(event)"
             ondragover="handleDragOver(event)"
             ondragleave="handleDragLeave(event)"
             ondrop="handleDrop(event, ${index})">
            
            <!-- Drag Handle - Requirements: 10.4 -->
            <div class="queue-item-drag-handle" title="拖拽排序">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="8" y1="6" x2="8" y2="6.01"/><line x1="12" y1="6" x2="12" y2="6.01"/><line x1="16" y1="6" x2="16" y2="6.01"/>
                    <line x1="8" y1="12" x2="8" y2="12.01"/><line x1="12" y1="12" x2="12" y2="12.01"/><line x1="16" y1="12" x2="16" y2="12.01"/>
                    <line x1="8" y1="18" x2="8" y2="18.01"/><line x1="12" y1="18" x2="12" y2="18.01"/><line x1="16" y1="18" x2="16" y2="18.01"/>
                </svg>
            </div>
            
            <!-- Item Info -->
            <div class="queue-item-info">
                <div class="queue-item-title" title="${escapeHtml(item.title || '无标题')}">${escapeHtml(item.title || '无标题')}</div>
                <div class="queue-item-meta">
                    <span>${escapeHtml(item.author || '未知作者')}</span>
                    ${item.totalSize ? `<span>${totalText}</span>` : ''}
                </div>
            </div>
            
            <!-- Progress Bar - Requirements: 3.3, 10.6 -->
            <div class="queue-item-progress">
                <div class="queue-progress-bar">
                    <div class="queue-progress-fill ${progressClass}" style="width: ${progress}%"></div>
                </div>
                <div class="queue-progress-text">
                    <span>${downloadedText} / ${totalText}</span>
                    <span>${progress.toFixed(1)}%</span>
                </div>
            </div>
            
            <!-- Speed and ETA - Requirements: 3.3 -->
            <div style="width: 100px; text-align: center; flex-shrink: 0;">
                <div style="font-size: 13px; color: var(--text-primary);">${speedText}</div>
                <div style="font-size: 11px; color: var(--text-secondary);">${etaText}</div>
            </div>
            
            <!-- Status Badge -->
            <span class="queue-item-status ${statusClass}">${statusText}</span>
            
            <!-- Action Buttons - Requirements: 10.2, 10.3, 10.5 -->
            <div class="queue-item-actions">
                ${item.status === 'pending' ? `
                <button class="queue-action-btn primary" onclick="startQueueItemDownload('${escapeHtml(item.id)}')" title="开始下载">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                        <polyline points="7 10 12 15 17 10"/>
                        <line x1="12" y1="15" x2="12" y2="3"/>
                    </svg>
                </button>
                ` : ''}
                ${item.status === 'downloading' ? `
                <button class="queue-action-btn" onclick="pauseQueueItem('${escapeHtml(item.id)}')" title="暂停">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/>
                    </svg>
                </button>
                ` : ''}
                ${item.status === 'paused' ? `
                <button class="queue-action-btn" onclick="resumeQueueItem('${escapeHtml(item.id)}')" title="恢复">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polygon points="5 3 19 12 5 21 5 3"/>
                    </svg>
                </button>
                ` : ''}
                ${item.status === 'failed' ? `
                <button class="queue-action-btn" onclick="retryQueueItem('${escapeHtml(item.id)}')" title="重试">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/>
                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
                    </svg>
                </button>
                ` : ''}
                <button class="queue-action-btn danger" onclick="removeQueueItem('${escapeHtml(item.id)}')" title="移除">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                    </svg>
                </button>
            </div>
        </div>
    `;
}

// Get queue status text
function getQueueStatusText(status) {
    const statusMap = {
        'downloading': '下载中',
        'pending': '等待中',
        'paused': '已暂停',
        'completed': '已完成',
        'failed': '失败'
    };
    return statusMap[status] || status || '未知';
}

// Calculate progress percentage
function calculateProgress(item) {
    if (!item.totalSize || item.totalSize === 0) return 0;
    return Math.min(100, (item.downloadedSize || 0) / item.totalSize * 100);
}

// Format download speed - Requirements: 3.3
function formatSpeed(bytesPerSecond) {
    if (!bytesPerSecond || bytesPerSecond <= 0) return '-';
    return formatBytes(bytesPerSecond) + '/s';
}

// Calculate ETA - Requirements: 3.3
function calculateETA(item) {
    if (!item.speed || item.speed <= 0) return '-';
    if (!item.totalSize || !item.downloadedSize) return '-';
    
    const remaining = item.totalSize - item.downloadedSize;
    if (remaining <= 0) return '完成';
    
    const seconds = Math.ceil(remaining / item.speed);
    
    if (seconds < 60) return `${seconds}秒`;
    if (seconds < 3600) return `${Math.ceil(seconds / 60)}分钟`;
    return `${Math.floor(seconds / 3600)}小时${Math.ceil((seconds % 3600) / 60)}分`;
}

// Update queue statistics
function updateQueueStats() {
    const total = queueState.items.length;
    const downloading = queueState.items.filter(i => i.status === 'downloading').length;
    const pending = queueState.items.filter(i => i.status === 'pending').length;
    const paused = queueState.items.filter(i => i.status === 'paused').length;
    
    document.getElementById('queueTotalCount').textContent = total;
    document.getElementById('queueDownloadingCount').textContent = downloading;
    document.getElementById('queuePendingCount').textContent = pending;
    document.getElementById('queuePausedCount').textContent = paused;
}

// Render empty state
function renderQueueEmptyState(message) {
    const container = document.getElementById('downloadQueueList');
    container.innerHTML = `
        <div class="queue-empty-state">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="12" y1="8" x2="12" y2="12"/>
                <line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>
            <p>${escapeHtml(message)}</p>
        </div>
    `;
}

// ============================================
// Drag and Drop for Queue Reordering - Requirements: 10.4
// ============================================

function setupDragAndDrop() {
    // Event listeners are added inline in the HTML for simplicity
}

function handleDragStart(event, index) {
    queueState.draggedIndex = index;
    queueState.draggedItem = queueState.items[index];
    event.target.classList.add('dragging');
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', index.toString());
}

function handleDragEnd(event) {
    event.target.classList.remove('dragging');
    queueState.draggedIndex = -1;
    queueState.draggedItem = null;
    
    // Remove drag-over class from all items
    document.querySelectorAll('.queue-item').forEach(item => {
        item.classList.remove('drag-over');
    });
}

function handleDragOver(event) {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    
    const target = event.target.closest('.queue-item');
    if (target && !target.classList.contains('dragging')) {
        target.classList.add('drag-over');
    }
}

function handleDragLeave(event) {
    const target = event.target.closest('.queue-item');
    if (target) {
        target.classList.remove('drag-over');
    }
}

async function handleDrop(event, targetIndex) {
    event.preventDefault();
    
    const target = event.target.closest('.queue-item');
    if (target) {
        target.classList.remove('drag-over');
    }
    
    const sourceIndex = queueState.draggedIndex;
    
    if (sourceIndex === -1 || sourceIndex === targetIndex) return;
    
    // Reorder locally first for immediate feedback
    const items = [...queueState.items];
    const [movedItem] = items.splice(sourceIndex, 1);
    items.splice(targetIndex, 0, movedItem);
    queueState.items = items;
    
    // Re-render the list
    renderQueueList();
    
    // Send reorder request to server - Requirements: 10.4
    try {
        const newOrder = items.map(item => item.id);
        await ApiClient.reorderQueue(newOrder);
    } catch (e) {
        console.error('Failed to reorder queue:', e);
        showMessage('重新排序失败: ' + e.message, 'error');
        // Reload to get correct order from server
        loadDownloadQueue();
    }
}

// ============================================
// Queue Item Actions - Requirements: 10.2, 10.3, 10.5
// ============================================

// Pause single item - Requirements: 10.2
async function pauseQueueItem(id) {
    try {
        await ApiClient.pauseDownload(id);
        showMessage('已暂停下载', 'success');
        
        // Update local state
        const item = queueState.items.find(i => i.id === id);
        if (item) {
            item.status = 'paused';
            renderQueueList();
            updateQueueStats();
        }
    } catch (e) {
        showMessage('暂停失败: ' + e.message, 'error');
    }
}

// Resume single item - Requirements: 10.3
async function resumeQueueItem(id) {
    try {
        await ApiClient.resumeDownload(id);
        showMessage('已恢复下载', 'success');
        
        // Update local state
        const item = queueState.items.find(i => i.id === id);
        if (item) {
            item.status = 'downloading';
            renderQueueList();
            updateQueueStats();
        }
    } catch (e) {
        showMessage('恢复失败: ' + e.message, 'error');
    }
}

// Remove item from queue - Requirements: 10.5
async function removeQueueItem(id) {
    if (!confirm('确定要从队列中移除此项吗？\n注意：部分下载的数据将被保留。')) return;
    
    try {
        await ApiClient.removeFromQueue(id);
        showMessage('已从队列移除', 'success');
        
        // Update local state
        queueState.items = queueState.items.filter(i => i.id !== id);
        renderQueueList();
        updateQueueStats();
    } catch (e) {
        showMessage('移除失败: ' + e.message, 'error');
    }
}

// Retry failed item
async function retryQueueItem(id) {
    try {
        await ApiClient.resumeDownload(id);
        showMessage('正在重试下载', 'success');
        
        // Update local state
        const item = queueState.items.find(i => i.id === id);
        if (item) {
            item.status = 'pending';
            item.retryCount = (item.retryCount || 0) + 1;
            renderQueueList();
            updateQueueStats();
        }
    } catch (e) {
        showMessage('重试失败: ' + e.message, 'error');
    }
}

// Start download for a queue item - uses existing batch download API
// Note: Video download requires decrypt key which is only available in the injected script
async function startQueueItemDownload(id) {
    const item = queueState.items.find(i => i.id === id);
    if (!item) {
        showMessage('找不到队列项', 'error');
        return;
    }
    
    if (!item.videoUrl) {
        showMessage('视频链接不可用，无法下载', 'error');
        return;
    }
    
    // Check if we have the decrypt key
    if (!item.decryptKey) {
        showMessage('缺少解密密钥，请从微信视频号页面使用批量下载功能', 'warning');
        console.warn('下载队列项缺少解密密钥。视频号视频是加密的，需要从浏览器页面获取解密密钥。');
        return;
    }
    
    try {
        // Update local status first for immediate UI feedback
        item.status = 'downloading';
        renderQueueList();
        updateQueueStats();
        showMessage('开始下载: ' + item.title, 'info');
        
        // Use existing batch download API with the video URL and decrypt key
        // Field mapping: authorName (backend) = author (frontend)
        const videos = [{
            id: item.videoId || item.id,
            title: item.title,
            url: item.videoUrl,
            authorName: item.author,
            key: item.decryptKey  // Decrypt key for encrypted videos
        }];
        
        const result = await ApiClient.startBatchDownload(videos, false);
        
        if (result.success) {
            showMessage('下载已开始，请在终端查看进度', 'success');
            // Mark as completed after batch download starts
            // The actual download happens in the background
            // Poll for batch progress
            pollBatchProgress(id);
        } else {
            item.status = 'pending';
            renderQueueList();
            updateQueueStats();
            showMessage('下载启动失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (e) {
        item.status = 'pending';
        renderQueueList();
        updateQueueStats();
        showMessage('下载失败: ' + e.message, 'error');
    }
}

// Poll batch download progress
// batch_progress API returns: { success, total, done, failed, running, currentTask }
async function pollBatchProgress(queueItemId) {
    const item = queueState.items.find(i => i.id === queueItemId);
    if (!item) return;
    
    let pollCount = 0;
    const maxPolls = 600; // 10 minutes max (1 second intervals)
    let lastDone = 0;
    
    const pollInterval = setInterval(async () => {
        pollCount++;
        
        try {
            const progress = await ApiClient.getBatchProgress();
            
            // Check if download completed (done increased or all tasks done)
            if (progress.success && progress.total > 0) {
                // Download completed
                if (progress.done > lastDone || (progress.running === 0 && progress.done > 0)) {
                    clearInterval(pollInterval);
                    // Update backend status
                    try {
                        await ApiClient.completeDownload(queueItemId);
                    } catch (e) {
                        console.warn('Failed to update backend status:', e);
                    }
                    item.status = 'completed';
                    item.downloadedSize = item.totalSize;
                    renderQueueList();
                    updateQueueStats();
                    showMessage('下载完成: ' + item.title, 'success');
                    return;
                }
                
                // Download failed
                if (progress.failed > 0 && progress.running === 0) {
                    clearInterval(pollInterval);
                    // Update backend status
                    try {
                        await ApiClient.failDownload(queueItemId, '下载失败');
                    } catch (e) {
                        console.warn('Failed to update backend status:', e);
                    }
                    item.status = 'failed';
                    item.errorMessage = '下载失败';
                    renderQueueList();
                    updateQueueStats();
                    showMessage('下载失败: ' + item.title, 'error');
                    return;
                }
                
                // Update progress from currentTask
                if (progress.currentTask && progress.currentTask.progress) {
                    const percent = progress.currentTask.progress;
                    item.downloadedSize = Math.floor((percent / 100) * item.totalSize);
                    renderQueueList();
                }
                
                lastDone = progress.done;
            }
            
            // No tasks running and no done - batch might have finished or not started
            if (progress.success && progress.total === 0) {
                // Batch download finished, mark as completed
                clearInterval(pollInterval);
                // Update backend status
                try {
                    await ApiClient.completeDownload(queueItemId);
                } catch (e) {
                    console.warn('Failed to update backend status:', e);
                }
                item.status = 'completed';
                item.downloadedSize = item.totalSize;
                renderQueueList();
                updateQueueStats();
                showMessage('下载完成: ' + item.title, 'success');
                return;
            }
            
            if (pollCount >= maxPolls) {
                clearInterval(pollInterval);
                item.status = 'pending';
                renderQueueList();
                updateQueueStats();
            }
        } catch (e) {
            // Ignore polling errors, just continue
            console.warn('Poll progress error:', e);
            if (pollCount >= maxPolls) {
                clearInterval(pollInterval);
            }
        }
    }, 1000);
}

// Pause all downloads
async function pauseAllDownloads() {
    const activeItems = queueState.items.filter(i => i.status === 'downloading' || i.status === 'pending');
    if (activeItems.length === 0) {
        showMessage('没有正在进行的下载', 'info');
        return;
    }
    
    try {
        for (const item of activeItems) {
            await ApiClient.pauseDownload(item.id);
            item.status = 'paused';
        }
        showMessage(`已暂停 ${activeItems.length} 个下载`, 'success');
        renderQueueList();
        updateQueueStats();
    } catch (e) {
        showMessage('暂停失败: ' + e.message, 'error');
        loadDownloadQueue(); // Reload to get correct state
    }
}

// Resume all downloads
async function resumeAllDownloads() {
    const pausedItems = queueState.items.filter(i => i.status === 'paused');
    if (pausedItems.length === 0) {
        showMessage('没有已暂停的下载', 'info');
        return;
    }
    
    try {
        for (const item of pausedItems) {
            await ApiClient.resumeDownload(item.id);
            item.status = 'pending';
        }
        showMessage(`已恢复 ${pausedItems.length} 个下载`, 'success');
        renderQueueList();
        updateQueueStats();
    } catch (e) {
        showMessage('恢复失败: ' + e.message, 'error');
        loadDownloadQueue(); // Reload to get correct state
    }
}

// Clear completed items from queue
async function clearCompletedQueue() {
    const completedItems = queueState.items.filter(i => i.status === 'completed');
    if (completedItems.length === 0) {
        showMessage('没有已完成的下载', 'info');
        return;
    }
    
    if (!confirm(`确定要清除 ${completedItems.length} 个已完成的下载吗？`)) return;
    
    try {
        for (const item of completedItems) {
            await ApiClient.removeFromQueue(item.id);
        }
        queueState.items = queueState.items.filter(i => i.status !== 'completed');
        showMessage(`已清除 ${completedItems.length} 个已完成的下载`, 'success');
        renderQueueList();
        updateQueueStats();
    } catch (e) {
        showMessage('清除失败: ' + e.message, 'error');
        loadDownloadQueue(); // Reload to get correct state
    }
}

// Update queue item progress from WebSocket - Requirements: 10.6
function updateQueueItemProgress(progressData) {
    const item = queueState.items.find(i => i.id === progressData.queueId);
    if (item) {
        item.downloadedSize = progressData.downloaded;
        item.totalSize = progressData.total;
        item.speed = progressData.speed;
        item.status = progressData.status;
        
        // Re-render just this item for performance
        const itemElement = document.querySelector(`.queue-item[data-id="${item.id}"]`);
        if (itemElement) {
            const index = queueState.items.indexOf(item);
            itemElement.outerHTML = renderQueueItem(item, index);
        }
        
        updateQueueStats();
    }
}

// ============================================
// Batch Download Functions
// ============================================
let batchProgressInterval = null;

async function startBatchDownload() {
    const videoListText = document.getElementById('batchVideoList').value.trim();
    const forceRedownload = document.getElementById('forceRedownload').checked;

    if (!videoListText) {
        showMessage('请输入视频列表', 'error');
        return;
    }

    let videos;
    try {
        videos = JSON.parse(videoListText);
        if (!Array.isArray(videos)) {
            if (videos.videos && Array.isArray(videos.videos)) {
                videos = videos.videos;
            } else {
                throw new Error('Invalid format');
            }
        }
    } catch (e) {
        showMessage('JSON 格式错误: ' + e.message, 'error');
        return;
    }

    try {
        const result = await ApiClient.startBatchDownload(videos, forceRedownload);
        if (result.success) {
            showMessage(`下载任务已提交，共 ${videos.length} 个视频`, 'success');
            document.getElementById('batchProgressCard').style.display = 'block';
            document.getElementById('batchLogCard').style.display = 'block';
            clearBatchLog();
            addBatchLogEntry(`🚀 开始批量下载: ${videos.length} 个视频`, 'info');
            startBatchProgressMonitoring();
        } else {
            showMessage('提交失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (e) {
        showMessage('请求失败: ' + e.message, 'error');
    }
}

function startBatchProgressMonitoring() {
    if (batchProgressInterval) {
        clearInterval(batchProgressInterval);
    }
    updateBatchProgress();
    batchProgressInterval = setInterval(updateBatchProgress, 2000);
}

async function updateBatchProgress() {
    try {
        const data = await ApiClient.getBatchProgress();
        if (data.success) {
            document.getElementById('batchTotal').textContent = data.total;
            document.getElementById('batchDone').textContent = data.done;
            document.getElementById('batchFailed').textContent = data.failed;
            document.getElementById('batchRunning').textContent = data.running;

            const percentage = data.total > 0 ? Math.round((data.done + data.failed) / data.total * 100) : 0;
            const progressBar = document.getElementById('batchProgressBar');
            progressBar.style.width = percentage + '%';
            progressBar.textContent = percentage + '%';
            
            // 显示当前任务信息
            const currentTaskEl = document.getElementById('batchCurrentTask');
            if (data.currentTask && data.running > 0) {
                currentTaskEl.style.display = 'block';
                document.getElementById('batchCurrentTitle').textContent = data.currentTask.title || '未知';
                document.getElementById('batchCurrentAuthor').textContent = data.currentTask.authorName || data.currentTask.author || '-';
                document.getElementById('batchCurrentProgress').textContent = (data.currentTask.progress || 0).toFixed(1) + '%';
                document.getElementById('batchCurrentSize').textContent = (data.currentTask.downloadedMB || 0).toFixed(2) + ' MB';
            } else {
                currentTaskEl.style.display = 'none';
            }
            
            // 显示日志卡片
            const logCard = document.getElementById('batchLogCard');
            if (logCard) {
                logCard.style.display = 'block';
            }
            
            // 更新日志（如果有任务完成或失败）
            if (data.tasks && Array.isArray(data.tasks)) {
                updateBatchLog(data.tasks);
            }

            // 保存任务数据用于导出
            if (data.tasks) {
                lastBatchTasks = data.tasks;
            }

            if (data.done + data.failed === data.total && data.total > 0) {
                clearInterval(batchProgressInterval);
                batchProgressInterval = null;
                showMessage(`下载完成！成功: ${data.done}, 失败: ${data.failed}`, data.failed > 0 ? 'info' : 'success');
                addBatchLogEntry(`✅ 批量下载完成: 成功 ${data.done}, 失败 ${data.failed}`, 'success');
                
                // 更新按钮状态，传递失败数量
                updateBatchButtons(true, data.failed);
            }
        }
    } catch (e) {
        console.error('Failed to update batch progress:', e);
    }
}

// 批量下载日志
let batchLogEntries = [];

function updateBatchLog(tasks) {
    const logList = document.getElementById('batchLogList');
    if (!logList) return;
    
    tasks.forEach(task => {
        const logId = `log-${task.id}-${task.status}`;
        if (!batchLogEntries.includes(logId)) {
            batchLogEntries.push(logId);
            
            if (task.status === 'done') {
                addBatchLogEntry(`✅ 下载完成: ${task.title} (${task.authorName || task.author || '未知作者'})`, 'success');
            } else if (task.status === 'failed') {
                addBatchLogEntry(`❌ 下载失败: ${task.title} - ${task.error || '未知错误'}`, 'error');
            } else if (task.status === 'downloading' && !batchLogEntries.includes(`log-${task.id}-start`)) {
                batchLogEntries.push(`log-${task.id}-start`);
                addBatchLogEntry(`⬇️ 开始下载: ${task.title}`, 'info');
            }
        }
    });
}

function addBatchLogEntry(message, type = 'info') {
    const logList = document.getElementById('batchLogList');
    if (!logList) return;
    
    // 移除初始提示
    const placeholder = logList.querySelector('div[style*="color: #888"]');
    if (placeholder && placeholder.textContent.includes('等待')) {
        placeholder.remove();
    }
    
    const colors = {
        success: '#4ec9b0',
        error: '#f14c4c',
        info: '#9cdcfe',
        warning: '#dcdcaa'
    };
    
    const time = new Date().toLocaleTimeString();
    const entry = document.createElement('div');
    entry.style.cssText = `color: ${colors[type] || colors.info}; margin-bottom: 4px;`;
    entry.textContent = `[${time}] ${message}`;
    
    logList.appendChild(entry);
    logList.scrollTop = logList.scrollHeight;
}

function clearBatchLog() {
    const logList = document.getElementById('batchLogList');
    if (logList) {
        logList.innerHTML = '<div style="color: #888;">等待下载任务...</div>';
        batchLogEntries = [];
    }
}

async function cancelBatchDownload() {
    if (!confirm('确定要取消当前的下载任务吗？')) return;

    try {
        const result = await ApiClient.cancelBatchDownload();
        if (result.success) {
            showMessage('下载已取消', 'info');
            addBatchLogEntry('⚠️ 下载已取消', 'warning');
            if (batchProgressInterval) {
                clearInterval(batchProgressInterval);
                batchProgressInterval = null;
            }
            updateBatchButtons(true);
        } else {
            showMessage('取消失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (e) {
        showMessage('请求失败: ' + e.message, 'error');
    }
}

// 保存最后一次的任务数据用于导出
let lastBatchTasks = [];

// 更新按钮状态
function updateBatchButtons(isCompleted, failedCount = 0) {
    const cancelBtn = document.getElementById('batchCancelBtn');
    const resetBtn = document.getElementById('batchResetBtn');
    const exportFailedBtn = document.getElementById('batchExportFailedBtn');
    const retryFailedBtn = document.getElementById('batchRetryFailedBtn');
    
    if (isCompleted) {
        if (cancelBtn) cancelBtn.style.display = 'none';
        if (resetBtn) resetBtn.style.display = 'inline-flex';
        // 如果有失败的任务，显示导出和重试按钮
        if (failedCount > 0) {
            if (exportFailedBtn) exportFailedBtn.style.display = 'inline-flex';
            if (retryFailedBtn) retryFailedBtn.style.display = 'inline-flex';
        } else {
            if (exportFailedBtn) exportFailedBtn.style.display = 'none';
            if (retryFailedBtn) retryFailedBtn.style.display = 'none';
        }
    } else {
        if (cancelBtn) cancelBtn.style.display = 'inline-flex';
        if (resetBtn) resetBtn.style.display = 'none';
        if (exportFailedBtn) exportFailedBtn.style.display = 'none';
        if (retryFailedBtn) retryFailedBtn.style.display = 'none';
    }
}

// 重新开始批量下载
function resetBatchDownload() {
    // 隐藏进度卡片
    document.getElementById('batchProgressCard').style.display = 'none';
    document.getElementById('batchLogCard').style.display = 'none';
    
    // 重置进度
    document.getElementById('batchTotal').textContent = '0';
    document.getElementById('batchDone').textContent = '0';
    document.getElementById('batchFailed').textContent = '0';
    document.getElementById('batchRunning').textContent = '0';
    document.getElementById('batchProgressBar').style.width = '0%';
    document.getElementById('batchProgressBar').textContent = '0%';
    
    // 重置按钮状态
    updateBatchButtons(false);
    
    // 清空日志
    clearBatchLog();
    
    showMessage('可以开始新的批量下载', 'info');
}

function loadBatchExample() {
    const example = [
        {
            id: "video_001",
            url: "https://example.com/video.mp4",
            title: "示例视频1",
            authorName: "示例作者"
        }
    ];
    document.getElementById('batchVideoList').value = JSON.stringify(example, null, 2);
    showMessage('已加载示例数据', 'info');
}

// 导出失败的任务记录
function exportFailedTasks() {
    const failedTasks = lastBatchTasks.filter(t => t.status === 'failed');
    
    if (failedTasks.length === 0) {
        showMessage('没有失败的任务', 'info');
        return;
    }
    
    // 生成导出数据
    const exportData = {
        generated_at: new Date().toLocaleString(),
        count: failedTasks.length,
        failed_videos: failedTasks.map(t => ({
            id: t.id,
            title: t.title,
            author: t.authorName || t.author || '未知作者',
            error: t.error || '未知错误'
        }))
    };
    
    // 下载 JSON 文件
    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `failed_downloads_${new Date().toISOString().slice(0,10)}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    
    showMessage(`已导出 ${failedTasks.length} 条失败记录`, 'success');
    addBatchLogEntry(`📤 已导出 ${failedTasks.length} 条失败记录`, 'info');
}

// 重试失败的任务
async function retryFailedTasks() {
    const failedTasks = lastBatchTasks.filter(t => t.status === 'failed');
    
    if (failedTasks.length === 0) {
        showMessage('没有失败的任务', 'info');
        return;
    }
    
    if (!confirm(`确定要重试 ${failedTasks.length} 个失败的任务吗？`)) {
        return;
    }
    
    // 构建重试的视频列表（需要从原始数据中获取URL等信息）
    // 由于后端返回的tasks可能不包含完整的URL信息，我们需要从输入框获取
    const videoListText = document.getElementById('batchVideoList').value.trim();
    let originalVideos = [];
    
    try {
        const parsed = JSON.parse(videoListText);
        originalVideos = Array.isArray(parsed) ? parsed : (parsed.videos || []);
    } catch (e) {
        showMessage('无法解析原始视频列表，请手动重新提交', 'error');
        return;
    }
    
    // 找到失败任务对应的原始数据
    const retryVideos = failedTasks.map(failed => {
        const original = originalVideos.find(v => v.id === failed.id);
        if (original) {
            return original;
        }
        // 如果找不到原始数据，尝试用失败任务的信息
        return {
            id: failed.id,
            title: failed.title,
            authorName: failed.authorName || failed.author
        };
    }).filter(v => v.url); // 只保留有URL的
    
    if (retryVideos.length === 0) {
        showMessage('无法获取失败任务的下载链接，请手动重新提交', 'error');
        return;
    }
    
    try {
        const forceRedownload = document.getElementById('forceRedownload').checked;
        const result = await ApiClient.startBatchDownload(retryVideos, forceRedownload);
        
        if (result.success) {
            showMessage(`重试任务已提交，共 ${retryVideos.length} 个视频`, 'success');
            addBatchLogEntry(`🔄 开始重试 ${retryVideos.length} 个失败任务`, 'info');
            
            // 重置按钮状态
            updateBatchButtons(false);
            
            // 开始监控进度
            startBatchProgressMonitoring();
        } else {
            showMessage('重试失败: ' + (result.error || '未知错误'), 'error');
        }
    } catch (e) {
        showMessage('请求失败: ' + e.message, 'error');
    }
}



// Add event delegation for lightbox triggers
document.addEventListener('click', function(e) {
    const trigger = e.target.closest('.lightbox-trigger');
    if (trigger) {
        e.preventDefault();
        const coverUrl = trigger.getAttribute('data-cover-url');
        const title = trigger.getAttribute('data-title');
        const duration = trigger.getAttribute('data-duration');
        if (coverUrl) {
            openLightbox(coverUrl, title, duration);
        }
    }
});
