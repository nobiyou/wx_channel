// ============================================
// Settings Page Functions - Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 13.7
// ============================================

// Toggle auto cleanup switch
function toggleAutoCleanup() {
    const toggle = document.getElementById('autoCleanupToggle');
    const checkbox = document.getElementById('settingAutoCleanup');
    if (toggle && checkbox) {
        checkbox.checked = !checkbox.checked;
        toggle.classList.toggle('active', checkbox.checked);
        toggleAutoCleanupDays();
    }
}

// Toggle auto cleanup days visibility
function toggleAutoCleanupDays() {
    const checkbox = document.getElementById('settingAutoCleanup');
    const daysGroup = document.getElementById('autoCleanupDaysGroup');
    const toggle = document.getElementById('autoCleanupToggle');
    
    if (checkbox && daysGroup) {
        daysGroup.style.opacity = checkbox.checked ? '1' : '0.5';
        daysGroup.style.pointerEvents = checkbox.checked ? 'auto' : 'none';
    }
    
    // Sync toggle switch state
    if (toggle && checkbox) {
        toggle.classList.toggle('active', checkbox.checked);
    }
}

// Load settings from localStorage and server
async function loadSettings() {
    // Load service URL from localStorage - Requirements: 13.7
    const savedUrl = StorageManager.loadServiceUrl();
    document.getElementById('settingServiceUrl').value = savedUrl;

    // Initialize auto cleanup days visibility
    toggleAutoCleanupDays();

    // Try to load from server
    if (ConnectionManager.getStatus() === 'connected') {
        try {
            const result = await ApiClient.getSettings();
            if (result.success && result.data) {
                const settings = result.data;
                document.getElementById('settingDownloadDir').value = settings.downloadDir || '';
                document.getElementById('settingChunkSize').value = (settings.chunkSize || 10485760) / 1048576;
                document.getElementById('settingConcurrentLimit').value = settings.concurrentLimit || 3;
                document.getElementById('settingAutoCleanup').checked = settings.autoCleanupEnabled || false;
                document.getElementById('settingAutoCleanupDays').value = settings.autoCleanupDays || 30;
                toggleAutoCleanupDays();
            }
        } catch (e) {
            console.error('Failed to load settings:', e);
        }
    }
}

// ============================================
// Validation Functions - Requirements: 11.2, 11.3, 11.4
// ============================================

// Validate service URL - Requirements: 13.7
function validateServiceUrl() {
    const input = document.getElementById('settingServiceUrl');
    const errorDiv = document.getElementById('serviceUrlError');
    const value = input.value.trim();
    
    // Clear previous state
    input.classList.remove('error', 'valid');
    errorDiv.classList.remove('visible');
    errorDiv.textContent = '';
    
    if (!value) {
        input.classList.add('error');
        errorDiv.textContent = '服务地址不能为空';
        errorDiv.classList.add('visible');
        return false;
    }
    
    // Validate URL format
    try {
        const url = new URL(value);
        if (url.protocol !== 'http:' && url.protocol !== 'https:') {
            throw new Error('Invalid protocol');
        }
        input.classList.add('valid');
        return true;
    } catch (e) {
        input.classList.add('error');
        errorDiv.textContent = '请输入有效的 URL 地址（如 http://127.0.0.1:2025）';
        errorDiv.classList.add('visible');
        return false;
    }
}

// Validate download directory - Requirements: 11.2
function validateDownloadDir() {
    const input = document.getElementById('settingDownloadDir');
    const errorDiv = document.getElementById('downloadDirError');
    const value = input.value.trim();
    
    // Clear previous state
    input.classList.remove('error', 'valid');
    errorDiv.classList.remove('visible');
    errorDiv.textContent = '';
    
    // Empty is allowed (will use default)
    if (!value) {
        return true;
    }
    
    // Check for invalid characters (basic validation)
    const invalidChars = /[<>:"|?*]/;
    if (invalidChars.test(value)) {
        input.classList.add('error');
        errorDiv.textContent = '路径包含无效字符（< > : " | ? *）';
        errorDiv.classList.add('visible');
        return false;
    }
    
    input.classList.add('valid');
    return true;
}

// Validate chunk size - Requirements: 11.3
function validateChunkSize() {
    const input = document.getElementById('settingChunkSize');
    const errorDiv = document.getElementById('chunkSizeError');
    const value = parseInt(input.value);
    
    // Clear previous state
    input.classList.remove('error', 'valid');
    errorDiv.classList.remove('visible');
    errorDiv.textContent = '';
    
    if (isNaN(value)) {
        input.classList.add('error');
        errorDiv.textContent = '请输入有效的数字';
        errorDiv.classList.add('visible');
        return false;
    }
    
    // Validate range: 1MB to 100MB - Requirements: 11.3
    if (value < 1 || value > 100) {
        input.classList.add('error');
        errorDiv.textContent = '分片大小必须在 1-100 MB 之间';
        errorDiv.classList.add('visible');
        return false;
    }
    
    input.classList.add('valid');
    return true;
}

// Validate concurrent limit - Requirements: 11.4
function validateConcurrentLimit() {
    const input = document.getElementById('settingConcurrentLimit');
    const errorDiv = document.getElementById('concurrentLimitError');
    const value = parseInt(input.value);
    
    // Clear previous state
    input.classList.remove('error', 'valid');
    errorDiv.classList.remove('visible');
    errorDiv.textContent = '';
    
    if (isNaN(value)) {
        input.classList.add('error');
        errorDiv.textContent = '请输入有效的数字';
        errorDiv.classList.add('visible');
        return false;
    }
    
    // Validate range: 1 to 5 - Requirements: 11.4
    if (value < 1 || value > 5) {
        input.classList.add('error');
        errorDiv.textContent = '并发下载数必须在 1-5 之间';
        errorDiv.classList.add('visible');
        return false;
    }
    
    input.classList.add('valid');
    return true;
}

// Validate auto cleanup days - Requirements: 11.5
function validateAutoCleanupDays() {
    const input = document.getElementById('settingAutoCleanupDays');
    const errorDiv = document.getElementById('autoCleanupDaysError');
    const value = parseInt(input.value);
    
    // Clear previous state
    input.classList.remove('error', 'valid');
    errorDiv.classList.remove('visible');
    errorDiv.textContent = '';
    
    if (isNaN(value)) {
        input.classList.add('error');
        errorDiv.textContent = '请输入有效的数字';
        errorDiv.classList.add('visible');
        return false;
    }
    
    // Validate range: 1 to 365 days
    if (value < 1 || value > 365) {
        input.classList.add('error');
        errorDiv.textContent = '清理天数必须在 1-365 天之间';
        errorDiv.classList.add('visible');
        return false;
    }
    
    input.classList.add('valid');
    return true;
}

// Toggle auto cleanup days input based on checkbox - Requirements: 11.5
function toggleAutoCleanupDays() {
    const checkbox = document.getElementById('settingAutoCleanup');
    const daysGroup = document.getElementById('autoCleanupDaysGroup');
    const daysInput = document.getElementById('settingAutoCleanupDays');
    
    if (checkbox.checked) {
        daysGroup.classList.remove('disabled');
        daysInput.disabled = false;
    } else {
        daysGroup.classList.add('disabled');
        daysInput.disabled = true;
    }
}

// ============================================
// Save Functions - Requirements: 11.1, 11.6, 13.7
// ============================================

// Test connection to service
async function testConnection() {
    const serviceUrl = document.getElementById('settingServiceUrl').value.trim();
    
    if (!validateServiceUrl()) {
        return;
    }
    
    showMessage('正在测试连接...', 'info');
    
    try {
        // Use /api/health endpoint for health check
        const response = await fetch(`${serviceUrl}/api/health`, {
            method: 'GET',
            signal: AbortSignal.timeout(5000)
        });
        
        if (response.ok) {
            showMessage('连接成功！', 'success');
        } else {
            showMessage('连接失败：服务返回错误', 'error');
        }
    } catch (e) {
        showMessage('连接失败：' + (e.message || '无法连接到服务'), 'error');
    }
}

// Save service URL - Requirements: 13.7
async function saveServiceUrl() {
    if (!validateServiceUrl()) {
        showMessage('请修正输入错误', 'error');
        return;
    }
    
    const serviceUrl = document.getElementById('settingServiceUrl').value.trim();
    
    // Save to localStorage - Requirements: 13.7
    StorageManager.saveServiceUrl(serviceUrl);
    
    // If URL changed, reconnect
    if (serviceUrl !== ConnectionManager.serviceUrl) {
        ConnectionManager.connect(serviceUrl);
    }
    
    showMessage('服务地址已保存', 'success');
}

// Save download settings - Requirements: 11.1, 11.2, 11.3, 11.4, 11.6
async function saveDownloadSettings() {
    // Validate all fields
    const isDownloadDirValid = validateDownloadDir();
    const isChunkSizeValid = validateChunkSize();
    const isConcurrentLimitValid = validateConcurrentLimit();
    
    if (!isDownloadDirValid || !isChunkSizeValid || !isConcurrentLimitValid) {
        showMessage('请修正输入错误', 'error');
        return;
    }
    
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }
    
    try {
        // Get current settings first to preserve other values
        const currentSettings = await ApiClient.getSettings();
        
        const settings = {
            ...currentSettings.data,
            downloadDir: document.getElementById('settingDownloadDir').value.trim(),
            chunkSize: parseInt(document.getElementById('settingChunkSize').value) * 1048576, // Convert MB to bytes
            concurrentLimit: parseInt(document.getElementById('settingConcurrentLimit').value)
        };
        
        await ApiClient.updateSettings(settings);
        showMessage('下载设置已保存', 'success');
    } catch (e) {
        showMessage('保存失败: ' + e.message, 'error');
    }
}

// Save cleanup settings - Requirements: 11.5, 11.6
async function saveCleanupSettings() {
    const autoCleanupEnabled = document.getElementById('settingAutoCleanup').checked;
    
    // Validate days if enabled
    if (autoCleanupEnabled && !validateAutoCleanupDays()) {
        showMessage('请修正输入错误', 'error');
        return;
    }
    
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }
    
    try {
        // Get current settings first to preserve other values
        const currentSettings = await ApiClient.getSettings();
        
        const settings = {
            ...currentSettings.data,
            autoCleanupEnabled: autoCleanupEnabled,
            autoCleanupDays: parseInt(document.getElementById('settingAutoCleanupDays').value)
        };
        
        await ApiClient.updateSettings(settings);
        showMessage('清理设置已保存', 'success');
    } catch (e) {
        showMessage('保存失败: ' + e.message, 'error');
    }
}

// Legacy saveSettings function for backward compatibility
async function saveSettings() {
    await saveServiceUrl();
    await saveDownloadSettings();
    await saveCleanupSettings();
}

// ============================================
// Danger Zone Functions
// ============================================

// Clear all browse history - Requirements: 5.1
async function clearAllBrowseHistory() {
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }
    
    // Get count for dialog
    const count = browseState.totalCount || browseState.records.length || 0;
    openClearDataDialog('browse', count);
}

// Clear all download records - Requirements: 5.3
async function clearAllDownloadRecords() {
    if (ConnectionManager.getStatus() !== 'connected') {
        showMessage('请先连接到本地服务', 'error');
        return;
    }
    
    // Get count for dialog
    const count = downloadState.totalCount || downloadState.records.length || 0;
    openClearDataDialog('downloads', count);
}

// Reset settings to defaults
async function resetSettings() {
    if (!confirm('确定要重置所有设置为默认值吗？')) {
        return;
    }
    
    // Reset form values
    document.getElementById('settingServiceUrl').value = DEFAULT_SERVICE_URL;
    document.getElementById('settingDownloadDir').value = '';
    document.getElementById('settingChunkSize').value = 10;
    document.getElementById('settingConcurrentLimit').value = 3;
    document.getElementById('settingAutoCleanup').checked = false;
    document.getElementById('settingAutoCleanupDays').value = 30;
    
    // Clear validation states
    document.querySelectorAll('#page-settings input').forEach(input => {
        input.classList.remove('error', 'valid');
    });
    document.querySelectorAll('#page-settings .form-error').forEach(error => {
        error.classList.remove('visible');
        error.textContent = '';
    });
    
    // Update auto cleanup days visibility
    toggleAutoCleanupDays();
    
    // Save to localStorage
    StorageManager.saveServiceUrl(DEFAULT_SERVICE_URL);
    
    // Save to server if connected
    if (ConnectionManager.getStatus() === 'connected') {
        try {
            await ApiClient.updateSettings({
                downloadDir: '',
                chunkSize: 10485760, // 10MB
                concurrentLimit: 3,
                autoCleanupEnabled: false,
                autoCleanupDays: 30
            });
        } catch (e) {
            console.error('Failed to reset server settings:', e);
        }
    }
    
    showMessage('设置已重置为默认值', 'success');
}

// ============================================
// Global Search - Requirements: 12.1, 12.2, 12.3, 12.4
// ============================================
let searchDebounceTimer = null;
let currentSearchResults = null;
let currentSearchQuery = '';

// Handle global search input with debounce - Requirements: 12.4
function handleGlobalSearch(query) {
    clearTimeout(searchDebounceTimer);
    currentSearchQuery = query.trim();
    
    // Hide dropdown if query is too short - Requirements: 12.4
    if (currentSearchQuery.length < 2) {
        hideSearchDropdown();
        return;
    }
    
    // Show loading state
    showSearchLoading();
    
    // Debounce 300ms - Requirements: 12.4
    searchDebounceTimer = setTimeout(async () => {
        if (ConnectionManager.getStatus() !== 'connected') {
            showSearchError('请先连接到本地服务');
            return;
        }
        
        try {
            const results = await ApiClient.search(currentSearchQuery);
            currentSearchResults = results;
            renderSearchResults(results, currentSearchQuery);
        } catch (e) {
            console.error('Search failed:', e);
            showSearchError('搜索失败: ' + e.message);
        }
    }, 300);
}

// Show search dropdown if there are results
function showSearchDropdownIfResults() {
    if (currentSearchResults && currentSearchQuery.length >= 2) {
        showSearchDropdown();
    }
}

// Show search dropdown
function showSearchDropdown() {
    const dropdown = document.getElementById('searchResultsDropdown');
    dropdown.classList.add('active');
}

// Hide search dropdown
function hideSearchDropdown() {
    const dropdown = document.getElementById('searchResultsDropdown');
    dropdown.classList.remove('active');
}

// Show loading state in dropdown
function showSearchLoading() {
    const content = document.getElementById('searchResultsContent');
    content.innerHTML = '<div class="search-results-loading">搜索中</div>';
    showSearchDropdown();
}

// Show error in dropdown
function showSearchError(message) {
    const content = document.getElementById('searchResultsContent');
    content.innerHTML = `<div class="search-results-empty">${escapeHtml(message)}</div>`;
    showSearchDropdown();
}

// Render search results grouped by source - Requirements: 12.2
function renderSearchResults(results, query) {
    const content = document.getElementById('searchResultsContent');
    
    if (!results.success) {
        content.innerHTML = `<div class="search-results-empty">搜索失败: ${escapeHtml(results.error || '未知错误')}</div>`;
        showSearchDropdown();
        return;
    }
    
    const data = results.data || {};
    const browseResults = data.browseResults || [];
    const downloadResults = data.downloadResults || [];
    const browseCount = data.browseCount || browseResults.length;
    const downloadCount = data.downloadCount || downloadResults.length;
    
    // Check if no results
    if (browseCount === 0 && downloadCount === 0) {
        content.innerHTML = '<div class="search-results-empty">没有找到匹配的结果</div>';
        showSearchDropdown();
        return;
    }
    
    let html = '';
    
    // Browse results group - Requirements: 12.2
    if (browseCount > 0) {
        html += `
            <div class="search-results-group">
                <div class="search-results-group-header">
                    <span>浏览记录</span>
                    <span class="search-results-group-count">${browseCount}</span>
                </div>
                ${browseResults.slice(0, 5).map(record => renderSearchResultItem(record, 'browse', query)).join('')}
                ${browseCount > 5 ? `
                    <div class="search-result-item" onclick="navigateToSearchResults('browse', '${escapeHtml(query)}')">
                        <div class="search-result-info" style="text-align: center; color: var(--primary-color);">
                            查看全部 ${browseCount} 条浏览记录
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    }
    
    // Download results group - Requirements: 12.2
    if (downloadCount > 0) {
        html += `
            <div class="search-results-group">
                <div class="search-results-group-header">
                    <span>下载记录</span>
                    <span class="search-results-group-count">${downloadCount}</span>
                </div>
                ${downloadResults.slice(0, 5).map(record => renderSearchResultItem(record, 'download', query)).join('')}
                ${downloadCount > 5 ? `
                    <div class="search-result-item" onclick="navigateToSearchResults('downloads', '${escapeHtml(query)}')">
                        <div class="search-result-info" style="text-align: center; color: var(--primary-color);">
                            查看全部 ${downloadCount} 条下载记录
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    }
    
    content.innerHTML = html;
    showSearchDropdown();
}

// Render a single search result item
function renderSearchResultItem(record, type, query) {
    const thumbnail = record.coverUrl 
        ? `<img class="search-result-thumbnail" src="${escapeHtml(record.coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'"><div class="search-result-thumbnail-placeholder" style="display:none"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg></div>`
        : `<div class="search-result-thumbnail-placeholder"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg></div>`;
    
    const title = highlightSearchTerm(record.title || '无标题', query);
    const author = record.author || '未知作者';
    const meta = type === 'browse' 
        ? (record.browseTime ? formatRelativeTime(record.browseTime) : '')
        : (record.downloadTime ? formatRelativeTime(record.downloadTime) : '');
    
    return `
        <div class="search-result-item" onclick="navigateToSearchResult('${type}', '${escapeHtml(record.id)}')">
            ${thumbnail}
            <div class="search-result-info">
                <div class="search-result-title">${title}</div>
                <div class="search-result-meta">${escapeHtml(author)}${meta ? ' · ' + meta : ''}</div>
            </div>
        </div>
    `;
}

// Highlight search term in text
function highlightSearchTerm(text, query) {
    if (!query || !text) return escapeHtml(text);
    
    const escapedText = escapeHtml(text);
    const escapedQuery = escapeHtml(query);
    const regex = new RegExp(`(${escapedQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
    return escapedText.replace(regex, '<span class="search-highlight">$1</span>');
}

// Navigate to a specific search result - Requirements: 12.3
function navigateToSearchResult(type, id) {
    hideSearchDropdown();
    clearSearchInput();
    
    if (type === 'browse') {
        navigateTo('browse');
        // Wait for page to load then show detail
        setTimeout(() => {
            showBrowseDetail(id);
        }, 100);
    } else if (type === 'download') {
        navigateTo('downloads');
        // Wait for page to load then show detail
        setTimeout(() => {
            showDownloadDetail(id);
        }, 100);
    }
}

// Navigate to search results page with filter - Requirements: 12.3
function navigateToSearchResults(page, query) {
    hideSearchDropdown();
    clearSearchInput();
    
    if (page === 'browse') {
        navigateTo('browse');
        // Set search query in browse page
        setTimeout(() => {
            const searchInput = document.getElementById('browseSearchInput');
            if (searchInput) {
                searchInput.value = query;
                browseState.searchQuery = query;
                browseState.currentPage = 1;
                loadBrowseHistory();
            }
        }, 100);
    } else if (page === 'downloads') {
        navigateTo('downloads');
        // Downloads page doesn't have search, but we navigate there
    }
}

// Clear search input
function clearSearchInput() {
    const input = document.getElementById('globalSearchInput');
    input.value = '';
    currentSearchQuery = '';
    currentSearchResults = null;
}

// Close search dropdown when clicking outside
document.addEventListener('click', function(event) {
    const searchContainer = document.getElementById('globalSearchContainer');
    if (searchContainer && !searchContainer.contains(event.target)) {
        hideSearchDropdown();
    }
});

// Close search dropdown on Escape key
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        hideSearchDropdown();
        document.getElementById('globalSearchInput').blur();
    }
});

