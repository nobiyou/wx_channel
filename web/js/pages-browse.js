// ============================================
// Browse History Page (Task 15)
// ============================================

// Browse history state
const browseState = {
    records: [],
    currentPage: 1,
    pageSize: 20,
    totalCount: 0,
    totalPages: 0,
    searchQuery: '',
    selectedIds: new Set(),
    currentDetailId: null
};

let browseSearchDebounceTimer = null;

// Load browse history with pagination - Requirements: 1.1, 1.4
async function loadBrowseHistory() {
    if (ConnectionManager.getStatus() !== 'connected') {
        renderBrowseEmptyState('请先连接到本地服务');
        return;
    }

    try {
        const params = {
            page: browseState.currentPage,
            pageSize: browseState.pageSize
        };
        
        if (browseState.searchQuery) {
            params.search = browseState.searchQuery;
        }

        const result = await ApiClient.getBrowseHistory(params);
        
        if (result.success) {
            // Handle paginated response structure: { items, total, page, pageSize, totalPages }
            const data = result.data || {};
            browseState.records = data.items || [];
            browseState.totalCount = data.total || 0;
            browseState.totalPages = data.totalPages || Math.ceil(browseState.totalCount / browseState.pageSize);
            
            renderBrowseTable();
            renderBrowsePagination();
            updateBrowseBatchActions();
        } else {
            renderBrowseEmptyState('加载失败: ' + (result.error || '未知错误'));
        }
    } catch (e) {
        console.error('Failed to load browse history:', e);
        renderBrowseEmptyState('加载失败: ' + e.message);
    }
}

// Render browse history table - Requirements: 1.1
function renderBrowseTable() {
    const tbody = document.getElementById('browseTableBody');
    
    if (!browseState.records || browseState.records.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7">
                    <div class="table-empty-state">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                            <circle cx="12" cy="12" r="3"/>
                        </svg>
                        <p>${browseState.searchQuery ? '没有找到匹配的记录' : '暂无浏览记录'}</p>
                    </div>
                </td>
            </tr>
        `;
        return;
    }

    let html = '';
    for (const record of browseState.records) {
        const isSelected = browseState.selectedIds.has(record.id);
        const thumbnail = record.coverUrl 
            ? `<img class="table-thumbnail" src="${escapeHtml(record.coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'"><div class="table-thumbnail-placeholder" style="display:none"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg></div>`
            : `<div class="table-thumbnail-placeholder"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg></div>`;
        
        // 构建视频信息摘要（更简洁的样式）
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
        
        // 构建互动数据（使用徽章样式）
        const statsItems = [];
        if (record.likeCount !== undefined && record.likeCount !== null && record.likeCount > 0) {
            statsItems.push(`<span class="stat-badge stat-like" title="点赞"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3zM7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3"/></svg>${formatNumber(record.likeCount)}</span>`);
        }
        if (record.commentCount !== undefined && record.commentCount !== null && record.commentCount > 0) {
            statsItems.push(`<span class="stat-badge stat-comment" title="评论"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>${formatNumber(record.commentCount)}</span>`);
        }
        if (record.favCount !== undefined && record.favCount !== null && record.favCount > 0) {
            statsItems.push(`<span class="stat-badge stat-fav" title="收藏"><svg viewBox="0 0 24 24" fill="currentColor"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>${formatNumber(record.favCount)}</span>`);
        }
        if (record.forwardCount !== undefined && record.forwardCount !== null && record.forwardCount > 0) {
            statsItems.push(`<span class="stat-badge stat-forward" title="转发"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/></svg>${formatNumber(record.forwardCount)}</span>`);
        }
        
        html += `
            <tr class="${isSelected ? 'selected' : ''}" data-id="${escapeHtml(record.id)}">
                <td onclick="event.stopPropagation();">
                    <input type="checkbox" ${isSelected ? 'checked' : ''} onchange="toggleBrowseSelect('${escapeHtml(record.id)}', this.checked)">
                </td>
                <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">${thumbnail}</td>
                <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                    <div class="video-info-cell">
                        <div class="table-title" title="${escapeHtml(record.title || '无标题')}">${escapeHtml(record.title || '无标题')}</div>
                        ${metaItems.length > 0 ? `<div class="video-meta-row">${metaItems.join('')}</div>` : ''}
                    </div>
                </td>
                <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                    <span class="table-meta">${record.size ? formatBytes(record.size) : '-'}</span>
                </td>
                <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                    <div class="stats-cell">
                        ${statsItems.length > 0 ? statsItems.join('') : '<span class="table-meta" style="color: var(--text-muted);">暂无数据</span>'}
                    </div>
                </td>
                <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                    <span class="table-meta">${record.browseTime ? formatDateTime(record.browseTime) : '-'}</span>
                </td>
                <td onclick="event.stopPropagation();">
                    <div class="table-actions">
                        <button class="table-action-btn" onclick="downloadBrowseRecord('${escapeHtml(record.id)}')" title="下载">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                                <polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
                            </svg>
                        </button>
                        <button class="table-action-btn danger" onclick="deleteBrowseRecord('${escapeHtml(record.id)}')" title="删除">
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

// Render empty state
function renderBrowseEmptyState(message) {
    const tbody = document.getElementById('browseTableBody');
    tbody.innerHTML = `
        <tr>
            <td colspan="7">
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

// Render pagination controls - Requirements: 1.4
function renderBrowsePagination() {
    const container = document.getElementById('browsePagination');
    
    if (browseState.totalPages <= 1) {
        container.innerHTML = browseState.totalCount > 0 
            ? `<span class="pagination-info">共 ${browseState.totalCount} 条记录</span>`
            : '';
        return;
    }

    let html = '';
    
    // Previous button
    html += `<button ${browseState.currentPage === 1 ? 'disabled' : ''} onclick="goToBrowsePage(${browseState.currentPage - 1})">上一页</button>`;
    
    // Page numbers
    const maxVisiblePages = 5;
    let startPage = Math.max(1, browseState.currentPage - Math.floor(maxVisiblePages / 2));
    let endPage = Math.min(browseState.totalPages, startPage + maxVisiblePages - 1);
    
    if (endPage - startPage < maxVisiblePages - 1) {
        startPage = Math.max(1, endPage - maxVisiblePages + 1);
    }
    
    if (startPage > 1) {
        html += `<button onclick="goToBrowsePage(1)">1</button>`;
        if (startPage > 2) {
            html += `<span class="pagination-info">...</span>`;
        }
    }
    
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="${i === browseState.currentPage ? 'active' : ''}" onclick="goToBrowsePage(${i})">${i}</button>`;
    }
    
    if (endPage < browseState.totalPages) {
        if (endPage < browseState.totalPages - 1) {
            html += `<span class="pagination-info">...</span>`;
        }
        html += `<button onclick="goToBrowsePage(${browseState.totalPages})">${browseState.totalPages}</button>`;
    }
    
    // Next button
    html += `<button ${browseState.currentPage === browseState.totalPages ? 'disabled' : ''} onclick="goToBrowsePage(${browseState.currentPage + 1})">下一页</button>`;
    
    // Info
    html += `<span class="pagination-info">共 ${browseState.totalCount} 条记录</span>`;
    
    container.innerHTML = html;
}

// Go to specific page
function goToBrowsePage(page) {
    if (page < 1 || page > browseState.totalPages) return;
    browseState.currentPage = page;
    loadBrowseHistory();
}

// Change page size - Requirements: 1.4
function changeBrowsePageSize(size) {
    browseState.pageSize = parseInt(size);
    browseState.currentPage = 1;
    loadBrowseHistory();
}

// Handle search with debounce - Requirements: 1.3
function handleBrowseSearch(query) {
    clearTimeout(browseSearchDebounceTimer);
    
    browseSearchDebounceTimer = setTimeout(() => {
        browseState.searchQuery = query.trim();
        browseState.currentPage = 1;
        loadBrowseHistory();
    }, 500); // 500ms debounce as per requirement 1.3
}

// Show video detail panel - Requirements: 1.2
async function showBrowseDetail(id) {
    browseState.currentDetailId = id;
    
    // Find record in current list or fetch from API
    let record = browseState.records.find(r => r.id === id);
    
    if (!record) {
        try {
            const result = await ApiClient.getBrowseRecord(id);
            if (result.success) {
                record = result.data;
            }
        } catch (e) {
            console.error('Failed to fetch record detail:', e);
            showMessage('获取详情失败', 'error');
            return;
        }
    }
    
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }
    
    renderBrowseDetailPanel(record);
    document.getElementById('browseDetailPanel').style.display = 'block';
}

// Render video detail panel - Requirements: 1.2, 8.1, 8.3
function renderBrowseDetailPanel(record) {
    const container = document.getElementById('browseDetailContent');
    
    // 封面图处理 - 与下载记录保持一致
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
                    <polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
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
                    <span class="video-detail-meta-label">大小</span>
                    <span class="video-detail-meta-value">${record.size ? formatBytes(record.size) : '-'}</span>
                </div>
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">浏览时间</span>
                    <span class="video-detail-meta-value">${record.browseTime ? formatDateTime(record.browseTime) : '-'}</span>
                </div>
                ${record.likeCount !== undefined && record.likeCount !== null ? `
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">点赞数</span>
                    <span class="video-detail-meta-value">${formatNumber(record.likeCount)}</span>
                </div>
                ` : ''}
                ${record.commentCount !== undefined && record.commentCount !== null ? `
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">评论数</span>
                    <span class="video-detail-meta-value">${formatNumber(record.commentCount)}</span>
                </div>
                ` : ''}
                ${record.favCount !== undefined && record.favCount !== null ? `
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">收藏数</span>
                    <span class="video-detail-meta-value">${formatNumber(record.favCount)}</span>
                </div>
                ` : ''}
                ${record.forwardCount !== undefined && record.forwardCount !== null ? `
                <div class="video-detail-meta-item">
                    <span class="video-detail-meta-label">转发数</span>
                    <span class="video-detail-meta-value">${formatNumber(record.forwardCount)}</span>
                </div>
                ` : ''}
            </div>
            
            <div class="video-detail-actions">
                <button class="btn btn-primary" onclick="downloadBrowseRecord('${escapeHtml(record.id)}')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                        <polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
                    </svg>
                    下载视频
                </button>
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
                ${record.pageUrl ? `
                <a class="btn btn-secondary" href="${escapeHtml(record.pageUrl)}" target="_blank">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>
                        <polyline points="15 3 21 3 21 9"/>
                        <line x1="10" y1="14" x2="21" y2="3"/>
                    </svg>
                    打开原页面
                </a>
                ` : ''}
                <button class="btn btn-danger" onclick="deleteBrowseRecord('${escapeHtml(record.id)}')">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                        <polyline points="3 6 5 6 21 6"/>
                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                    </svg>
                    删除记录
                </button>
            </div>
        </div>
    `;
}

// Close detail panel
function closeBrowseDetailPanel() {
    document.getElementById('browseDetailPanel').style.display = 'none';
    browseState.currentDetailId = null;
}

// Toggle select all - Requirements: 9.1
function toggleBrowseSelectAll(checked) {
    if (checked) {
        browseState.records.forEach(r => browseState.selectedIds.add(r.id));
    } else {
        browseState.selectedIds.clear();
    }
    renderBrowseTable();
    updateBrowseBatchActions();
}

// Toggle single selection - Requirements: 9.1
function toggleBrowseSelect(id, checked) {
    if (checked) {
        browseState.selectedIds.add(id);
    } else {
        browseState.selectedIds.delete(id);
    }
    
    // Update select all checkbox
    const selectAllCheckbox = document.getElementById('browseSelectAll');
    if (selectAllCheckbox) {
        selectAllCheckbox.checked = browseState.selectedIds.size === browseState.records.length && browseState.records.length > 0;
    }
    
    // Update row styling
    const row = document.querySelector(`tr[data-id="${id}"]`);
    if (row) {
        row.classList.toggle('selected', checked);
    }
    
    updateBrowseBatchActions();
}

// Update batch actions bar visibility
function updateBrowseBatchActions() {
    const batchBar = document.getElementById('browseBatchActions');
    const countSpan = document.getElementById('browseSelectedCount');
    
    if (browseState.selectedIds.size > 0) {
        batchBar.style.display = 'block';
        countSpan.textContent = `已选择 ${browseState.selectedIds.size} 项`;
    } else {
        batchBar.style.display = 'none';
    }
}

// Clear selection
function clearBrowseSelection() {
    browseState.selectedIds.clear();
    document.getElementById('browseSelectAll').checked = false;
    renderBrowseTable();
    updateBrowseBatchActions();
}

// Download single record
async function downloadBrowseRecord(id) {
    const record = browseState.records.find(r => r.id === id);
    if (!record) {
        showMessage('记录不存在', 'error');
        return;
    }
    
    try {
        await ApiClient.addToQueue([{
            videoId: record.id,
            videoUrl: record.videoUrl,
            decryptKey: record.decryptKey || '',
            coverUrl: record.coverUrl || '',
            duration: record.duration || 0,
            resolution: record.resolution || parseResolutionFromUrl(record.videoUrl) || '',
            title: record.title,
            author: record.author,
            size: record.size || 0
        }]);
        showMessage('已添加到下载队列', 'success');
    } catch (e) {
        showMessage('添加失败: ' + e.message, 'error');
    }
}

// Parse resolution from video URL (e.g., 1280x720 -> 720p)
function parseResolutionFromUrl(url) {
    if (!url) return '';
    // Try to match resolution pattern like 1280x720 or 1920x1080
    const match = url.match(/(\d{3,4})x(\d{3,4})/);
    if (match) {
        const height = parseInt(match[2]);
        if (height >= 1080) return '1080p';
        if (height >= 720) return '720p';
        if (height >= 480) return '480p';
        if (height >= 360) return '360p';
        return height + 'p';
    }
    return '';
}

// Delete single record
async function deleteBrowseRecord(id) {
    if (!confirm('确定要删除这条浏览记录吗？')) return;
    
    try {
        await ApiClient.deleteBrowseRecords([id]);
        showMessage('删除成功', 'success');
        
        // Close detail panel if showing this record
        if (browseState.currentDetailId === id) {
            closeBrowseDetailPanel();
        }
        
        // Remove from selection
        browseState.selectedIds.delete(id);
        
        // Reload list
        loadBrowseHistory();
    } catch (e) {
        showMessage('删除失败: ' + e.message, 'error');
    }
}

// Download selected records - Requirements: 9.3
async function downloadSelectedBrowse() {
    if (browseState.selectedIds.size === 0) {
        showMessage('请先选择要下载的记录', 'error');
        return;
    }
    
    const selectedRecords = browseState.records.filter(r => browseState.selectedIds.has(r.id));
    const videos = selectedRecords.map(r => ({
        videoId: r.id,
        videoUrl: r.videoUrl,
        decryptKey: r.decryptKey || '',
        coverUrl: r.coverUrl || '',
        duration: r.duration || 0,
        resolution: r.resolution || parseResolutionFromUrl(r.videoUrl) || '',
        title: r.title,
        author: r.author,
        size: r.size || 0
    }));
    
    try {
        await ApiClient.addToQueue(videos);
        showMessage(`已将 ${videos.length} 个视频添加到下载队列`, 'success');
        clearBrowseSelection();
    } catch (e) {
        showMessage('添加失败: ' + e.message, 'error');
    }
}

// ============================================
// Export Dialog Functions - Requirements: 4.1, 4.2, 4.4
// ============================================

// Export dialog state
const exportDialogState = {
    type: 'browse', // 'browse' or 'downloads'
    format: 'json',
    ids: null, // null means export all
    count: 0,
    isExporting: false
};

/**
 * Open export dialog - Requirements: 4.1, 4.2
 * @param {string} type - 'browse' or 'downloads'
 * @param {string[]} ids - Array of record IDs to export, or null for all
 * @param {number} count - Number of records to export
 */
function openExportDialog(type, ids = null, count = 0) {
    exportDialogState.type = type;
    exportDialogState.ids = ids;
    exportDialogState.count = count;
    exportDialogState.format = 'json';
    exportDialogState.isExporting = false;
    
    // Update dialog content
    const typeText = type === 'browse' ? '浏览记录' : '下载记录';
    document.getElementById('exportTypeValue').textContent = typeText;
    document.getElementById('exportCountValue').textContent = `${count} 条`;
    document.getElementById('exportDialogDescription').textContent = 
        ids ? `将导出选中的 ${count} 条${typeText}` : `将导出全部${typeText}`;
    
    // Reset format selection
    selectExportFormat('json');
    
    // Hide progress
    document.getElementById('exportProgress').classList.remove('active');
    document.getElementById('exportProgressBar').style.width = '0%';
    
    // Enable confirm button
    document.getElementById('exportConfirmBtn').disabled = false;
    
    // Show dialog
    document.getElementById('exportDialogOverlay').classList.add('active');
    document.body.style.overflow = 'hidden';
}

/**
 * Close export dialog
 * @param {Event} event
 */
function closeExportDialog(event) {
    if (event) event.preventDefault();
    if (exportDialogState.isExporting) return; // Don't close while exporting
    
    document.getElementById('exportDialogOverlay').classList.remove('active');
    document.body.style.overflow = '';
}

/**
 * Select export format
 * @param {string} format - 'json' or 'csv'
 */
function selectExportFormat(format) {
    exportDialogState.format = format;
    
    // Update UI
    document.querySelectorAll('.export-format-option').forEach(option => {
        const input = option.querySelector('input');
        if (input.value === format) {
            option.classList.add('selected');
            input.checked = true;
        } else {
            option.classList.remove('selected');
            input.checked = false;
        }
    });
}

/**
 * Confirm and execute export - Requirements: 4.1, 4.2, 4.4
 */
async function confirmExport() {
    if (exportDialogState.isExporting) return;
    
    exportDialogState.isExporting = true;
    const confirmBtn = document.getElementById('exportConfirmBtn');
    confirmBtn.disabled = true;
    
    // Show progress for large exports - Requirements: 4.4
    const showProgress = exportDialogState.count > 100;
    if (showProgress) {
        document.getElementById('exportProgress').classList.add('active');
        updateExportProgress(0, '准备导出...');
    }
    
    try {
        if (showProgress) updateExportProgress(20, '正在获取数据...');
        
        const result = await ApiClient.exportData(
            exportDialogState.type === 'browse' ? 'browse' : 'downloads',
            exportDialogState.format,
            exportDialogState.ids
        );
        
        if (showProgress) updateExportProgress(60, '正在生成文件...');
        
        // Generate file based on format
        let blob, filename, mimeType;
        const timestamp = new Date().toISOString().slice(0, 10);
        const typePrefix = exportDialogState.type === 'browse' ? 'browse' : 'downloads';
        const suffix = exportDialogState.ids ? 'selected' : 'all';
        
        if (exportDialogState.format === 'json') {
            const jsonData = JSON.stringify(result.data || result, null, 2);
            blob = new Blob([jsonData], { type: 'application/json' });
            filename = `${typePrefix}_export_${suffix}_${timestamp}.json`;
        } else {
            // CSV format
            const csvData = convertToCSV(result.data || result, exportDialogState.type);
            // Add BOM for Excel compatibility with Chinese characters
            const bom = '\uFEFF';
            blob = new Blob([bom + csvData], { type: 'text/csv;charset=utf-8' });
            filename = `${typePrefix}_export_${suffix}_${timestamp}.csv`;
        }
        
        if (showProgress) updateExportProgress(90, '正在下载...');
        
        // Trigger download
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        
        if (showProgress) updateExportProgress(100, '导出完成！');
        
        // Close dialog after short delay
        setTimeout(() => {
            closeExportDialog();
            showMessage(`已导出 ${exportDialogState.count} 条记录`, 'success');
        }, showProgress ? 500 : 0);
        
    } catch (e) {
        console.error('Export failed:', e);
        showMessage('导出失败: ' + e.message, 'error');
        document.getElementById('exportProgress').classList.remove('active');
    } finally {
        exportDialogState.isExporting = false;
        confirmBtn.disabled = false;
    }
}

/**
 * Update export progress indicator - Requirements: 4.4
 * @param {number} percent - Progress percentage (0-100)
 * @param {string} text - Progress text
 */
function updateExportProgress(percent, text) {
    document.getElementById('exportProgressBar').style.width = `${percent}%`;
    document.getElementById('exportProgressText').textContent = text;
}

/**
 * Convert data array to CSV format
 * @param {Array} data - Array of records
 * @param {string} type - 'browse' or 'downloads'
 * @returns {string} CSV string
 */
function convertToCSV(data, type) {
    if (!Array.isArray(data) || data.length === 0) {
        return '';
    }
    
    // Define columns based on type
    let columns;
    if (type === 'browse') {
        columns = [
            { key: 'id', header: 'ID' },
            { key: 'title', header: '标题' },
            { key: 'author', header: '作者' },
            { key: 'duration', header: '时长(秒)' },
            { key: 'size', header: '大小(字节)' },
            { key: 'browseTime', header: '浏览时间' },
            { key: 'likeCount', header: '点赞数' },
            { key: 'commentCount', header: '评论数' },
            { key: 'shareCount', header: '分享数' },
            { key: 'coverUrl', header: '封面URL' },
            { key: 'videoUrl', header: '视频URL' },
            { key: 'pageUrl', header: '页面URL' }
        ];
    } else {
        columns = [
            { key: 'id', header: 'ID' },
            { key: 'videoId', header: '视频ID' },
            { key: 'title', header: '标题' },
            { key: 'author', header: '作者' },
            { key: 'duration', header: '时长(秒)' },
            { key: 'fileSize', header: '文件大小(字节)' },
            { key: 'filePath', header: '文件路径' },
            { key: 'format', header: '格式' },
            { key: 'resolution', header: '分辨率' },
            { key: 'status', header: '状态' },
            { key: 'downloadTime', header: '下载时间' },
            { key: 'errorMessage', header: '错误信息' }
        ];
    }
    
    // Generate header row
    const header = columns.map(c => escapeCSVField(c.header)).join(',');
    
    // Generate data rows
    const rows = data.map(record => {
        return columns.map(c => {
            const value = record[c.key];
            return escapeCSVField(value !== undefined && value !== null ? String(value) : '');
        }).join(',');
    });
    
    return [header, ...rows].join('\n');
}

/**
 * Escape a field for CSV format
 * @param {string} field - Field value
 * @returns {string} Escaped field
 */
function escapeCSVField(field) {
    if (field.includes(',') || field.includes('"') || field.includes('\n') || field.includes('\r')) {
        return '"' + field.replace(/"/g, '""') + '"';
    }
    return field;
}

// Export selected browse records - Requirements: 9.4
function exportSelectedBrowse() {
    if (browseState.selectedIds.size === 0) {
        showMessage('请先选择要导出的记录', 'error');
        return;
    }
    
    const ids = Array.from(browseState.selectedIds);
    openExportDialog('browse', ids, ids.length);
}

// Export all browse records - Requirements: 4.1, 4.2
function exportAllBrowse() {
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }

    // Use total count from state, or estimate
    const count = browseState.totalCount || browseState.records.length;
    openExportDialog('browse', null, count);
}

// ============================================
// Clear Data Dialog Functions - Requirements: 5.1, 5.3
// ============================================

// Clear data dialog state
const clearDataDialogState = {
    type: 'browse', // 'browse' or 'downloads'
    count: 0,
    isClearing: false
};

/**
 * Open clear data confirmation dialog - Requirements: 5.1, 5.3
 * @param {string} type - 'browse' or 'downloads'
 * @param {number} count - Number of records to clear
 */
function openClearDataDialog(type, count = 0) {
    clearDataDialogState.type = type;
    clearDataDialogState.count = count;
    clearDataDialogState.isClearing = false;
    
    // Update dialog content based on type
    const isBrowse = type === 'browse';
    const typeText = isBrowse ? '浏览记录' : '下载记录';
    
    document.getElementById('clearDataDialogTitle').textContent = `清空${typeText}`;
    document.getElementById('clearDataWarningText').textContent = 
        isBrowse 
            ? '此操作将删除所有浏览记录，且不可撤销！'
            : '此操作将删除所有下载记录，且不可撤销！';
    document.getElementById('clearDataCount').textContent = count;
    
    // Show/hide delete files option - Requirements: 5.3
    const filesOption = document.getElementById('clearFilesOption');
    if (isBrowse) {
        filesOption.style.display = 'none';
    } else {
        filesOption.style.display = 'block';
        document.getElementById('clearDeleteFiles').checked = false;
    }
    
    // Reset confirmation input
    document.getElementById('clearConfirmInput').value = '';
    document.getElementById('clearDataConfirmBtn').disabled = true;
    
    // Show dialog
    document.getElementById('clearDataDialogOverlay').classList.add('active');
    document.body.style.overflow = 'hidden';
    
    // Focus on input
    setTimeout(() => {
        document.getElementById('clearConfirmInput').focus();
    }, 100);
}

/**
 * Close clear data dialog
 * @param {Event} event
 */
function closeClearDataDialog(event) {
    if (event) event.preventDefault();
    if (clearDataDialogState.isClearing) return;
    
    document.getElementById('clearDataDialogOverlay').classList.remove('active');
    document.body.style.overflow = '';
}

/**
 * Validate confirmation input
 */
function validateClearConfirmInput() {
    const input = document.getElementById('clearConfirmInput').value.trim().toUpperCase();
    const confirmBtn = document.getElementById('clearDataConfirmBtn');
    confirmBtn.disabled = input !== 'DELETE';
}

/**
 * Confirm and execute clear data - Requirements: 5.1, 5.3
 */
async function confirmClearData() {
    const input = document.getElementById('clearConfirmInput').value.trim().toUpperCase();
    if (input !== 'DELETE') {
        showMessage('请输入 DELETE 确认删除', 'error');
        return;
    }
    
    if (clearDataDialogState.isClearing) return;
    
    clearDataDialogState.isClearing = true;
    const confirmBtn = document.getElementById('clearDataConfirmBtn');
    confirmBtn.disabled = true;
    confirmBtn.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px; animation: spin 1s linear infinite;">
            <circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="32"/>
        </svg>
        清空中...
    `;
    
    try {
        if (clearDataDialogState.type === 'browse') {
            // Clear browse history - Requirements: 5.1
            await ApiClient.clearBrowseHistory();
            showMessage('浏览记录已清空', 'success');
            
            // Reload browse history if on that page
            if (currentPage === 'browse') {
                loadBrowseHistory();
            }
        } else {
            // Clear download records - Requirements: 5.3
            const deleteFiles = document.getElementById('clearDeleteFiles').checked;
            await ApiClient.clearDownloadRecords(deleteFiles);
            showMessage(deleteFiles ? '下载记录和文件已清空' : '下载记录已清空', 'success');
            
            // Reload download records if on that page
            if (currentPage === 'downloads') {
                loadDownloadRecords();
            }
        }
        
        // Close dialog
        closeClearDataDialog();
        
        // Refresh dashboard stats
        if (currentPage === 'dashboard') {
            loadDashboardData();
        }
        
    } catch (e) {
        console.error('Clear data failed:', e);
        showMessage('清空失败: ' + e.message, 'error');
    } finally {
        clearDataDialogState.isClearing = false;
        confirmBtn.disabled = false;
        confirmBtn.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                <polyline points="3 6 5 6 21 6"/>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
            清空数据
        `;
    }
}

// ============================================
// Date-based Cleanup Dialog Functions - Requirements: 5.5
// ============================================

// Cleanup dialog state
const cleanupDialogState = {
    type: 'browse', // 'browse' or 'downloads'
    date: null,
    isCleaning: false
};

/**
 * Open date-based cleanup dialog - Requirements: 5.5
 */
function openCleanupDialog() {
    cleanupDialogState.type = 'browse';
    cleanupDialogState.date = null;
    cleanupDialogState.isCleaning = false;
    
    // Reset form
    selectCleanupType('browse');
    document.getElementById('cleanupDate').value = '';
    document.getElementById('cleanupPreviewText').textContent = '请选择截止日期';
    document.getElementById('cleanupConfirmBtn').disabled = true;
    document.getElementById('cleanupDeleteFiles').checked = false;
    
    // Show dialog
    document.getElementById('cleanupDialogOverlay').classList.add('active');
    document.body.style.overflow = 'hidden';
}

/**
 * Close cleanup dialog
 * @param {Event} event
 */
function closeCleanupDialog(event) {
    if (event) event.preventDefault();
    if (cleanupDialogState.isCleaning) return;
    
    document.getElementById('cleanupDialogOverlay').classList.remove('active');
    document.body.style.overflow = '';
}

/**
 * Select cleanup type
 * @param {string} type - 'browse' or 'downloads'
 */
function selectCleanupType(type) {
    cleanupDialogState.type = type;
    
    // Update UI
    document.querySelectorAll('.cleanup-type-option').forEach(option => {
        const input = option.querySelector('input');
        if (input.value === type) {
            option.classList.add('selected');
            input.checked = true;
        } else {
            option.classList.remove('selected');
            input.checked = false;
        }
    });
    
    // Show/hide delete files option
    const filesOption = document.getElementById('cleanupFilesOption');
    filesOption.style.display = type === 'downloads' ? 'block' : 'none';
    
    // Update preview
    updateCleanupPreview();
}

/**
 * Set cleanup date using shortcut
 * @param {number} daysAgo - Number of days ago
 */
function setCleanupDate(daysAgo) {
    const date = new Date();
    date.setDate(date.getDate() - daysAgo);
    const dateStr = date.toISOString().slice(0, 10);
    document.getElementById('cleanupDate').value = dateStr;
    updateCleanupPreview();
}

/**
 * Update cleanup preview
 */
function updateCleanupPreview() {
    const dateInput = document.getElementById('cleanupDate');
    const previewText = document.getElementById('cleanupPreviewText');
    const confirmBtn = document.getElementById('cleanupConfirmBtn');
    
    if (!dateInput.value) {
        previewText.textContent = '请选择截止日期';
        confirmBtn.disabled = true;
        return;
    }
    
    cleanupDialogState.date = dateInput.value;
    const date = new Date(dateInput.value);
    const typeText = cleanupDialogState.type === 'browse' ? '浏览记录' : '下载记录';
    const dateText = `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日`;
    
    previewText.textContent = `将删除 ${dateText} 之前的所有${typeText}`;
    confirmBtn.disabled = false;
}

/**
 * Confirm and execute date-based cleanup - Requirements: 5.5
 */
async function confirmCleanup() {
    if (!cleanupDialogState.date) {
        showMessage('请选择截止日期', 'error');
        return;
    }
    
    if (cleanupDialogState.isCleaning) return;
    
    cleanupDialogState.isCleaning = true;
    const confirmBtn = document.getElementById('cleanupConfirmBtn');
    confirmBtn.disabled = true;
    confirmBtn.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px; animation: spin 1s linear infinite;">
            <circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="32"/>
        </svg>
        清理中...
    `;
    
    try {
        const deleteFiles = cleanupDialogState.type === 'downloads' 
            ? document.getElementById('cleanupDeleteFiles').checked 
            : false;
        
        const result = await ApiClient.cleanupByDate(
            cleanupDialogState.type,
            cleanupDialogState.date,
            deleteFiles
        );
        
        const deletedCount = result.deletedCount || 0;
        const typeText = cleanupDialogState.type === 'browse' ? '浏览记录' : '下载记录';
        showMessage(`已清理 ${deletedCount} 条${typeText}`, 'success');
        
        // Close dialog
        closeCleanupDialog();
        
        // Reload data
        if (currentPage === 'browse' && cleanupDialogState.type === 'browse') {
            loadBrowseHistory();
        } else if (currentPage === 'downloads' && cleanupDialogState.type === 'downloads') {
            loadDownloadRecords();
        } else if (currentPage === 'dashboard') {
            loadDashboardData();
        }
        
    } catch (e) {
        console.error('Cleanup failed:', e);
        showMessage('清理失败: ' + e.message, 'error');
    } finally {
        cleanupDialogState.isCleaning = false;
        confirmBtn.disabled = false;
        confirmBtn.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                <polyline points="3 6 5 6 21 6"/>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
            执行清理
        `;
    }
}

// Delete selected records - Requirements: 9.2
async function deleteSelectedBrowse() {
    if (browseState.selectedIds.size === 0) {
        showMessage('请先选择要删除的记录', 'error');
        return;
    }
    
    if (!confirm(`确定要删除选中的 ${browseState.selectedIds.size} 条记录吗？`)) return;
    
    try {
        const ids = Array.from(browseState.selectedIds);
        await ApiClient.deleteBrowseRecords(ids);
        showMessage(`已删除 ${ids.length} 条记录`, 'success');
        
        // Close detail panel if showing a deleted record
        if (browseState.selectedIds.has(browseState.currentDetailId)) {
            closeBrowseDetailPanel();
        }
        
        clearBrowseSelection();
        loadBrowseHistory();
    } catch (e) {
        showMessage('删除失败: ' + e.message, 'error');
    }
}

// Format date time
function formatDateTime(dateStr) {
    if (!dateStr) return '';
    try {
        const date = new Date(dateStr);
        return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
    } catch (e) {
        return dateStr;
    }
}



// ============================================
// Other UI Functions (from browse page)
// ============================================

// Clear browse history from browse page - Requirements: 5.1
function clearBrowseHistory() {
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }
    
    // Get count for dialog
    const count = browseState.totalCount || browseState.records.length || 0;
    openClearDataDialog('browse', count);
}

function filterDownloads() {
    loadDownloadRecords();
}

// Legacy function - redirects to new implementation
async function exportDownloads() {
    await exportAllDownloads();
}

function retryConnection() {
    ConnectionManager.retry();
}


// ============================================
// Export for Batch Download
// ============================================

// Export browse records to batch download input
async function exportForBatchDownload() {
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }
    
    // Check if there are selected records
    const useSelected = browseState.selectedIds.size > 0;
    const ids = useSelected ? Array.from(browseState.selectedIds) : null;
    
    try {
        showMessage('正在准备数据...', 'info');
        
        // Get records from API - returns array directly (not wrapped in { data: ... })
        const result = await ApiClient.exportData('browse', 'json', ids);
        
        // Handle both formats: direct array or { data: [...] }
        const records = Array.isArray(result) ? result : (Array.isArray(result?.data) ? result.data : []);
        
        if (records.length === 0) {
            showMessage('没有可导出的记录', 'error');
            return;
        }
        
        // Convert to batch download format with metadata
        // Note: database fields use different naming (VideoURL vs videoUrl, etc.)
        const batchData = {
            generated_at: new Date().toLocaleString(),
            count: records.length,
            videos: records.map((record, index) => ({
                index: index + 1,
                title: record.Title || record.title || '',
                id: record.ID || record.id || '',
                url: record.VideoURL || record.videoUrl || '',
                key: record.DecryptKey || record.decryptKey || '',
                author: record.Author || record.author || '',
                duration: formatDurationForExport(record.Duration || record.duration || 0),
                sizeMB: formatSizeForExport(record.Size || record.size || 0),
                resolution: record.Resolution || record.resolution || '',
                like: record.LikeCount || record.likeCount || 0,
                comment: record.CommentCount || record.commentCount || 0,
                forward: record.ShareCount || record.shareCount || 0,
                created: formatDateTimeForExport(record.BrowseTime || record.browseTime),
                cover: record.CoverURL || record.coverUrl || ''
            }))
        };
        
        // Fill the batch download input textarea
        const batchInput = document.getElementById('batchVideoList');
        if (batchInput) {
            batchInput.value = JSON.stringify(batchData, null, 2);
            
            // Switch to batch download page
            navigateTo('batch');
            
            showMessage(`已填入 ${records.length} 条记录到批量下载`, 'success');
        } else {
            showMessage('找不到批量下载输入框', 'error');
        }
    } catch (e) {
        console.error('Export failed:', e);
        showMessage('导出失败: ' + e.message, 'error');
    }
}

// Format duration (ms) to string like "00:22"
function formatDurationForExport(durationMs) {
    if (!durationMs || durationMs <= 0) return '';
    const totalSeconds = Math.floor(durationMs / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
}

// Format size (bytes) to string like "28.77MB"
function formatSizeForExport(sizeBytes) {
    if (!sizeBytes || sizeBytes <= 0) return '';
    const sizeMB = sizeBytes / (1024 * 1024);
    return `${sizeMB.toFixed(2)}MB`;
}

// Format datetime for export like "2025-04-04 19:31"
function formatDateTimeForExport(dateStr) {
    if (!dateStr) return '';
    try {
        const date = new Date(dateStr);
        if (isNaN(date.getTime())) return '';
        const year = date.getFullYear();
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const day = String(date.getDate()).padStart(2, '0');
        const hours = String(date.getHours()).padStart(2, '0');
        const minutes = String(date.getMinutes()).padStart(2, '0');
        return `${year}-${month}-${day} ${hours}:${minutes}`;
    } catch (e) {
        return '';
    }
}
