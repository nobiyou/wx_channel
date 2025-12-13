// ============================================
    // Constants and Configuration
    // ============================================
    const STORAGE_KEY_SERVICE_URL = 'wx_channel_service_url';
    const DEFAULT_SERVICE_URL = 'http://127.0.0.1:2025';
    const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000, 30000]; // Exponential backoff

    // ============================================
    // LocalStorage Persistence (Task 12.7)
    // ============================================
    const StorageManager = {
        saveServiceUrl(url) {
            try {
                localStorage.setItem(STORAGE_KEY_SERVICE_URL, url);
                return true;
            } catch (e) {
                console.error('Failed to save service URL:', e);
                return false;
            }
        },

        loadServiceUrl() {
            try {
                return localStorage.getItem(STORAGE_KEY_SERVICE_URL) || DEFAULT_SERVICE_URL;
            } catch (e) {
                console.error('Failed to load service URL:', e);
                return DEFAULT_SERVICE_URL;
            }
        },

        saveSettings(settings) {
            try {
                localStorage.setItem('wx_channel_settings', JSON.stringify(settings));
                return true;
            } catch (e) {
                console.error('Failed to save settings:', e);
                return false;
            }
        },

        loadSettings() {
            try {
                const data = localStorage.getItem('wx_channel_settings');
                return data ? JSON.parse(data) : null;
            } catch (e) {
                console.error('Failed to load settings:', e);
                return null;
            }
        }
    };

    // ============================================
    // Connection Manager (Task 12.2)
    // ============================================
    const ConnectionManager = {
        status: 'disconnected', // connected, disconnected, connecting, error
        serviceUrl: DEFAULT_SERVICE_URL,
        reconnectAttempt: 0,
        reconnectTimer: null,
        statusCallbacks: [],

        async connect(url) {
            if (url) {
                this.serviceUrl = url;
                StorageManager.saveServiceUrl(url);
            }
            
            this.setStatus('connecting');
            
            try {
                // Use /api/health endpoint for health check
                const response = await fetch(`${this.serviceUrl}/api/health`, {
                    method: 'GET',
                    signal: AbortSignal.timeout(5000)
                });
                
                if (response.ok) {
                    this.setStatus('connected');
                    this.reconnectAttempt = 0;
                    WebSocketClient.connect();
                    return true;
                } else {
                    this.setStatus('error');
                    return false;
                }
            } catch (e) {
                console.error('Connection failed:', e);
                this.setStatus('disconnected');
                this.scheduleReconnect();
                return false;
            }
        },

        disconnect() {
            this.setStatus('disconnected');
            this.clearReconnectTimer();
            WebSocketClient.disconnect();
        },

        async retry() {
            this.reconnectAttempt = 0;
            return await this.connect();
        },

        scheduleReconnect() {
            this.clearReconnectTimer();
            const delay = RECONNECT_DELAYS[Math.min(this.reconnectAttempt, RECONNECT_DELAYS.length - 1)];
            this.reconnectAttempt++;
            
            console.log(`Scheduling reconnect in ${delay}ms (attempt ${this.reconnectAttempt})`);
            this.reconnectTimer = setTimeout(() => this.connect(), delay);
        },

        clearReconnectTimer() {
            if (this.reconnectTimer) {
                clearTimeout(this.reconnectTimer);
                this.reconnectTimer = null;
            }
        },

        setStatus(status) {
            this.status = status;
            this.updateStatusUI();
            this.statusCallbacks.forEach(cb => cb(status));
            
            // Execute queued operations when connected
            if (status === 'connected') {
                OperationQueue.executeAll();
            }
        },

        getStatus() {
            return this.status;
        },

        onStatusChange(callback) {
            this.statusCallbacks.push(callback);
        },

        updateStatusUI() {
            const dot = document.getElementById('statusDot');
            const text = document.getElementById('statusText');
            
            dot.className = 'status-dot';
            
            switch (this.status) {
                case 'connected':
                    dot.classList.add('connected');
                    text.textContent = '已连接';
                    break;
                case 'connecting':
                    dot.classList.add('connecting');
                    text.textContent = '连接中...';
                    break;
                case 'error':
                    text.textContent = '连接错误';
                    break;
                default:
                    text.textContent = '未连接';
            }
        }
    };

    // ============================================
    // Operation Queue (Task 12.5)
    // ============================================
    const OperationQueue = {
        queue: [],
        maxRetries: 3,

        add(operation) {
            // operation: { type, method, endpoint, data, retries }
            operation.retries = operation.retries || 0;
            this.queue.push(operation);
            
            // Try to execute immediately if connected
            if (ConnectionManager.getStatus() === 'connected') {
                this.executeAll();
            }
        },

        async executeAll() {
            while (this.queue.length > 0 && ConnectionManager.getStatus() === 'connected') {
                const operation = this.queue.shift();
                try {
                    await this.execute(operation);
                } catch (e) {
                    console.error('Operation failed:', e);
                    if (operation.retries < this.maxRetries) {
                        operation.retries++;
                        this.queue.unshift(operation); // Put back at front
                    } else {
                        console.error('Operation exceeded max retries:', operation);
                        showMessage(`操作失败: ${operation.type}`, 'error');
                    }
                    break; // Stop processing on failure
                }
            }
        },

        async execute(operation) {
            const { method, endpoint, data } = operation;
            return await ApiClient.request(method, endpoint, data);
        },

        clear() {
            this.queue = [];
        },

        getQueueLength() {
            return this.queue.length;
        }
    };

    // ============================================
    // API Client (Task 12.3)
    // ============================================
    const ApiClient = {
        async request(method, endpoint, data = null) {
            if (ConnectionManager.getStatus() !== 'connected') {
                throw new Error('Not connected to service');
            }

            // Use /api/ prefix for REST API endpoints
            const url = `${ConnectionManager.serviceUrl}/api${endpoint}`;
            const options = {
                method,
                headers: {
                    'Content-Type': 'application/json'
                }
            };

            if (data && (method === 'POST' || method === 'PUT' || method === 'DELETE')) {
                options.body = JSON.stringify(data);
            }

            const response = await fetch(url, options);
            
            if (!response.ok) {
                const error = await response.json().catch(() => ({ error: 'Unknown error' }));
                throw new Error(error.error || `HTTP ${response.status}`);
            }

            return await response.json();
        },

        // Browse History
        async getBrowseHistory(params = {}) {
            const query = new URLSearchParams(params).toString();
            return await this.request('GET', `/browse${query ? '?' + query : ''}`);
        },

        async getBrowseRecord(id) {
            return await this.request('GET', `/browse/${id}`);
        },

        async deleteBrowseRecords(ids) {
            return await this.request('DELETE', '/browse', { ids });
        },

        async clearBrowseHistory() {
            return await this.request('DELETE', '/browse/clear');
        },

        // Download Records
        async getDownloadRecords(params = {}) {
            const query = new URLSearchParams(params).toString();
            return await this.request('GET', `/downloads${query ? '?' + query : ''}`);
        },

        async getDownloadRecord(id) {
            return await this.request('GET', `/downloads/${id}`);
        },

        async deleteDownloadRecords(ids, deleteFiles = false) {
            return await this.request('DELETE', '/downloads', { ids, deleteFiles });
        },

        async clearDownloadRecords(deleteFiles = false) {
            return await this.request('DELETE', '/downloads/clear', { deleteFiles });
        },

        // Date-based cleanup - Requirements: 5.5
        async cleanupByDate(type, beforeDate, deleteFiles = false) {
            const endpoint = type === 'browse' ? '/browse/cleanup' : '/downloads/cleanup';
            return await this.request('DELETE', endpoint, { beforeDate, deleteFiles });
        },

        // Download Queue
        async getDownloadQueue() {
            return await this.request('GET', '/queue');
        },

        async addToQueue(videos) {
            return await this.request('POST', '/queue', { videos });
        },

        async pauseDownload(id) {
            return await this.request('PUT', `/queue/${id}/pause`);
        },

        async resumeDownload(id) {
            return await this.request('PUT', `/queue/${id}/resume`);
        },

        async removeFromQueue(id) {
            return await this.request('DELETE', `/queue/${id}`);
        },

        async reorderQueue(ids) {
            return await this.request('PUT', '/queue/reorder', { ids });
        },

        async completeDownload(id) {
            return await this.request('PUT', `/queue/${id}/complete`);
        },

        async failDownload(id, error) {
            return await this.request('PUT', `/queue/${id}/fail`, { error });
        },

        // Settings
        async getSettings() {
            return await this.request('GET', '/settings');
        },

        async updateSettings(settings) {
            return await this.request('PUT', '/settings', settings);
        },

        // Statistics
        async getStatistics() {
            return await this.request('GET', '/stats');
        },

        async getChartData() {
            return await this.request('GET', '/stats/chart');
        },

        // Export
        async exportData(type, format, ids = null) {
            const params = { format };
            if (ids) params.ids = ids.join(',');
            const query = new URLSearchParams(params).toString();
            return await this.request('GET', `/export/${type}?${query}`);
        },

        // Search
        async search(query) {
            return await this.request('GET', `/search?q=${encodeURIComponent(query)}`);
        },

        // Batch Download (existing API)
        async startBatchDownload(videos, forceRedownload = false) {
            const url = `${ConnectionManager.serviceUrl}/__wx_channels_api/batch_start`;
            const response = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ videos, forceRedownload })
            });
            return await response.json();
        },

        async getBatchProgress() {
            const url = `${ConnectionManager.serviceUrl}/__wx_channels_api/batch_progress`;
            const response = await fetch(url);
            return await response.json();
        },

        async cancelBatchDownload() {
            const url = `${ConnectionManager.serviceUrl}/__wx_channels_api/batch_cancel`;
            const response = await fetch(url, { method: 'POST' });
            return await response.json();
        },

        // File operations
        async openFolder(filePath) {
            return await this.request('POST', '/files/open-folder', { path: filePath });
        },

        async playVideo(filePath) {
            return await this.request('POST', '/files/play', { path: filePath });
        },

        // Retry download
        async retryDownloadRecord(id) {
            return await this.request('POST', `/downloads/${id}/retry`);
        }
    };

    // ============================================
    // WebSocket Client (Task 12.4)
    // ============================================
    const WebSocketClient = {
        ws: null,
        reconnectTimer: null,
        callbacks: {
            downloadProgress: [],
            queueChange: [],
            statsUpdate: []
        },

        connect() {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                return;
            }

            // WebSocket server runs on proxy port + 1
            // Parse the service URL to get the port and construct WebSocket URL
            const serviceUrl = new URL(ConnectionManager.serviceUrl);
            const wsPort = parseInt(serviceUrl.port || '2025') + 1;
            const wsUrl = `ws://${serviceUrl.hostname}:${wsPort}/ws`;
            
            try {
                this.ws = new WebSocket(wsUrl);
                
                this.ws.onopen = () => {
                    console.log('WebSocket connected');
                };

                this.ws.onmessage = (event) => {
                    try {
                        const message = JSON.parse(event.data);
                        this.handleMessage(message);
                    } catch (e) {
                        console.error('Failed to parse WebSocket message:', e);
                    }
                };

                this.ws.onclose = () => {
                    console.log('WebSocket disconnected');
                    this.scheduleReconnect();
                };

                this.ws.onerror = (error) => {
                    console.error('WebSocket error:', error);
                };
            } catch (e) {
                console.error('Failed to create WebSocket:', e);
            }
        },

        disconnect() {
            this.clearReconnectTimer();
            if (this.ws) {
                this.ws.close();
                this.ws = null;
            }
        },

        scheduleReconnect() {
            this.clearReconnectTimer();
            if (ConnectionManager.getStatus() === 'connected') {
                this.reconnectTimer = setTimeout(() => this.connect(), 5000);
            }
        },

        clearReconnectTimer() {
            if (this.reconnectTimer) {
                clearTimeout(this.reconnectTimer);
                this.reconnectTimer = null;
            }
        },

        handleMessage(message) {
            switch (message.type) {
                case 'download_progress':
                    this.callbacks.downloadProgress.forEach(cb => cb(message));
                    break;
                case 'queue_change':
                    this.callbacks.queueChange.forEach(cb => cb(message));
                    break;
                case 'stats_update':
                    this.callbacks.statsUpdate.forEach(cb => cb(message));
                    break;
            }
        },

        onDownloadProgress(callback) {
            this.callbacks.downloadProgress.push(callback);
        },

        onQueueChange(callback) {
            this.callbacks.queueChange.push(callback);
        },

        onStatsUpdate(callback) {
            this.callbacks.statsUpdate.push(callback);
        }
    };

    // ============================================
    // UI Functions
    // ============================================
    let currentPage = 'dashboard';
    
    // Valid pages for SPA routing - Requirements: 6.2, 6.3
    const validPages = ['dashboard', 'browse', 'downloads', 'batch', 'queue', 'settings'];
    
    // Page titles for display
    const pageTitles = {
        dashboard: '仪表盘',
        browse: '浏览记录',
        downloads: '下载记录',
        batch: '批量下载',
        queue: '下载队列',
        settings: '设置'
    };

    /**
     * Navigate to a page using SPA routing - Requirements: 6.2, 6.3
     * @param {string} page - Page name to navigate to
     * @param {boolean} updateHistory - Whether to update browser history (default: true)
     */
    function navigateTo(page, updateHistory = true) {
        // Validate page
        if (!validPages.includes(page)) {
            console.warn(`Invalid page: ${page}, defaulting to dashboard`);
            page = 'dashboard';
        }
        
        // Update nav items - Requirements: 6.3
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.remove('active');
            if (item.dataset.page === page) {
                item.classList.add('active');
            }
        });

        // Update page views - Requirements: 6.2
        document.querySelectorAll('.page-view').forEach(view => {
            view.classList.remove('active');
        });
        const pageElement = document.getElementById(`page-${page}`);
        if (pageElement) {
            pageElement.classList.add('active');
        }

        // Update page title
        document.getElementById('pageTitle').textContent = pageTitles[page] || page;

        currentPage = page;
        
        // Update URL hash for SPA routing - Requirements: 6.2
        if (updateHistory && window.location.hash !== `#${page}`) {
            history.pushState({ page }, pageTitles[page], `#${page}`);
        }

        // Close sidebar on mobile
        if (window.innerWidth <= 768) {
            document.getElementById('sidebar').classList.remove('open');
            document.getElementById('sidebarOverlay').classList.remove('open');
        }

        // Load page data
        loadPageData(page);
    }
    
    /**
     * Handle browser back/forward navigation - Requirements: 6.2
     */
    function handlePopState(event) {
        const page = event.state?.page || getPageFromHash() || 'dashboard';
        navigateTo(page, false);
    }
    
    /**
     * Get page name from URL hash
     * @returns {string|null} Page name or null
     */
    function getPageFromHash() {
        const hash = window.location.hash.slice(1); // Remove #
        return validPages.includes(hash) ? hash : null;
    }
    
    /**
     * Initialize SPA routing - Requirements: 6.2
     */
    function initRouter() {
        // Listen for browser back/forward
        window.addEventListener('popstate', handlePopState);
        
        // Handle initial page load from URL hash
        const initialPage = getPageFromHash() || 'dashboard';
        if (initialPage !== 'dashboard') {
            navigateTo(initialPage, false);
        }
        
        // Set initial history state
        history.replaceState({ page: initialPage }, pageTitles[initialPage], `#${initialPage}`);
    }

    function toggleSidebar() {
        document.getElementById('sidebar').classList.toggle('open');
        document.getElementById('sidebarOverlay').classList.toggle('open');
    }

    async function loadPageData(page) {
        if (ConnectionManager.getStatus() !== 'connected') return;

        try {
            switch (page) {
                case 'dashboard':
                    await loadDashboardData();
                    break;
                case 'browse':
                    await loadBrowseHistory();
                    break;
                case 'downloads':
                    await loadDownloadRecords();
                    break;
                case 'queue':
                    await loadDownloadQueue();
                    break;
                case 'settings':
                    await loadSettings();
                    break;
            }
        } catch (e) {
            console.error('Failed to load page data:', e);
        }
    }

    async function loadDashboardData() {
        try {
            // Load statistics - Requirements: 7.1
            const stats = await ApiClient.getStatistics();
            if (stats.success && stats.data) {
                const data = stats.data;
                document.getElementById('statTotalBrowse').textContent = formatNumber(data.totalBrowseCount || 0);
                document.getElementById('statTotalDownload').textContent = formatNumber(data.totalDownloadCount || 0);
                document.getElementById('statTodayDownload').textContent = formatNumber(data.todayDownloadCount || 0);
                document.getElementById('statStorage').textContent = formatBytes(data.storageUsed || 0);
                
                // Render recent browse list - Requirements: 7.3
                renderRecentBrowseList(data.recentBrowse || []);
                
                // Render recent download list - Requirements: 7.4
                renderRecentDownloadList(data.recentDownload || []);
            }
            
            // Load chart data - Requirements: 7.2
            await loadChartData();
        } catch (e) {
            console.error('Failed to load dashboard data:', e);
        }
    }

    // Load chart data for past 7 days - Requirements: 7.2
    async function loadChartData() {
        try {
            const chartData = await ApiClient.getChartData();
            if (chartData.success && chartData.data) {
                renderChart(chartData.data);
            } else {
                renderEmptyChart();
            }
        } catch (e) {
            console.error('Failed to load chart data:', e);
            renderEmptyChart();
        }
    }

    // Render the bar chart - Requirements: 7.2
    function renderChart(data) {
        const container = document.getElementById('chartBars');
        
        if (!data.labels || !data.values || data.labels.length === 0) {
            renderEmptyChart();
            return;
        }
        
        const maxValue = Math.max(...data.values, 1);
        const maxHeight = 140; // pixels
        
        let html = '';
        for (let i = 0; i < data.labels.length; i++) {
            const value = data.values[i] || 0;
            const height = Math.max((value / maxValue) * maxHeight, 4);
            const label = formatChartLabel(data.labels[i]);
            
            html += `
                <div class="chart-bar-wrapper">
                    <div class="chart-bar" style="height: ${height}px;" title="${data.labels[i]}: ${value} 次下载">
                        ${value > 0 ? `<span class="chart-bar-value">${value}</span>` : ''}
                    </div>
                    <span class="chart-label">${label}</span>
                </div>
            `;
        }
        
        container.innerHTML = html;
    }

    // Render empty chart state
    function renderEmptyChart() {
        const container = document.getElementById('chartBars');
        container.innerHTML = '<div class="chart-empty">暂无下载数据</div>';
    }

    // Format chart label (date to short format)
    function formatChartLabel(dateStr) {
        try {
            const date = new Date(dateStr);
            return `${date.getMonth() + 1}/${date.getDate()}`;
        } catch (e) {
            return dateStr;
        }
    }

    // Render recent browse list - Requirements: 7.3
    function renderRecentBrowseList(records) {
        const container = document.getElementById('recentBrowseList');
        
        if (!records || records.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                        <circle cx="12" cy="12" r="3"/>
                    </svg>
                    <p>暂无浏览记录</p>
                </div>
            `;
            return;
        }
        
        let html = '';
        for (const record of records) {
            const thumbnail = record.coverUrl 
                ? `<img class="recent-thumbnail" src="${escapeHtml(record.coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'"><div class="recent-thumbnail-placeholder" style="display:none"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg></div>`
                : `<div class="recent-thumbnail-placeholder"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg></div>`;
            
            const duration = record.duration ? formatDuration(record.duration) : '';
            const author = record.author || '未知作者';
            const browseTime = record.browseTime ? formatRelativeTime(record.browseTime) : '';
            
            html += `
                <div class="recent-item" onclick="viewBrowseRecord('${escapeHtml(record.id)}')">
                    ${thumbnail}
                    <div class="recent-info">
                        <div class="recent-title">${escapeHtml(record.title || '无标题')}</div>
                        <div class="recent-meta">
                            <span>${escapeHtml(author)}</span>
                            ${duration ? `<span>${duration}</span>` : ''}
                            ${browseTime ? `<span>${browseTime}</span>` : ''}
                        </div>
                    </div>
                </div>
            `;
        }
        
        container.innerHTML = html;
    }

    // Render recent download list - Requirements: 7.4
    function renderRecentDownloadList(records) {
        const container = document.getElementById('recentDownloadList');
        
        if (!records || records.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                        <polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
                    </svg>
                    <p>暂无下载记录</p>
                </div>
            `;
            return;
        }
        
        let html = '';
        for (const record of records) {
            const statusClass = record.status || 'completed';
            const statusText = getStatusText(record.status);
            const author = record.author || '未知作者';
            const fileSize = record.fileSize ? formatBytes(record.fileSize) : '';
            const downloadTime = record.downloadTime ? formatRelativeTime(record.downloadTime) : '';
            
            html += `
                <div class="recent-item" onclick="viewDownloadRecord('${escapeHtml(record.id)}')">
                    <div class="recent-thumbnail-placeholder">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                            <polyline points="14 2 14 8 20 8"/>
                        </svg>
                    </div>
                    <div class="recent-info">
                        <div class="recent-title">${escapeHtml(record.title || '无标题')}</div>
                        <div class="recent-meta">
                            <span>${escapeHtml(author)}</span>
                            ${fileSize ? `<span>${fileSize}</span>` : ''}
                            ${downloadTime ? `<span>${downloadTime}</span>` : ''}
                        </div>
                    </div>
                    <span class="recent-status ${statusClass}">${statusText}</span>
                </div>
            `;
        }
        
        container.innerHTML = html;
    }

    // View browse record detail
    function viewBrowseRecord(id) {
        // Navigate to browse page and show detail
        navigateTo('browse');
        // TODO: Show detail panel for specific record
    }

    // View download record detail
    function viewDownloadRecord(id) {
        // Navigate to downloads page and show detail
        navigateTo('downloads');
        // TODO: Show detail panel for specific record
    }

    // Get status display text
    function getStatusText(status) {
        const statusMap = {
            'completed': '已完成',
            'failed': '失败',
            'in_progress': '下载中',
            'pending': '等待中',
            'paused': '已暂停'
        };
        return statusMap[status] || status || '未知';
    }

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
                    <td colspan="8">
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
            
            html += `
                <tr class="${isSelected ? 'selected' : ''}" data-id="${escapeHtml(record.id)}">
                    <td onclick="event.stopPropagation();">
                        <input type="checkbox" ${isSelected ? 'checked' : ''} onchange="toggleBrowseSelect('${escapeHtml(record.id)}', this.checked)">
                    </td>
                    <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">${thumbnail}</td>
                    <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                        <div class="table-title" title="${escapeHtml(record.title || '无标题')}">${escapeHtml(record.title || '无标题')}</div>
                    </td>
                    <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                        <span class="table-author">${escapeHtml(record.author || '未知')}</span>
                    </td>
                    <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                        <span class="table-meta">${record.duration ? formatDuration(record.duration) : '-'}</span>
                    </td>
                    <td onclick="showBrowseDetail('${escapeHtml(record.id)}')">
                        <span class="table-meta">${record.size ? formatBytes(record.size) : '-'}</span>
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
                            <button class="table-action-btn" onclick="deleteBrowseRecord('${escapeHtml(record.id)}')" title="删除">
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
        
        // Use the thumbnail component with lightbox support - Requirements: 8.1, 8.3
        const thumbnail = createThumbnailComponent({
            coverUrl: record.coverUrl,
            duration: record.duration,
            videoId: record.id,
            title: record.title,
            isDownloaded: false
        });
        
        container.innerHTML = `
            <div class="video-detail-thumbnail-wrapper" style="max-width: 300px;">
                ${thumbnail}
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
                </div>
                
                <div class="video-detail-stats">
                    <div class="video-detail-stat">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3zM7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3"/>
                        </svg>
                        <span class="video-detail-stat-value">${formatNumber(record.likeCount || 0)}</span>
                    </div>
                    <div class="video-detail-stat">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
                        </svg>
                        <span class="video-detail-stat-value">${formatNumber(record.commentCount || 0)}</span>
                    </div>
                    <div class="video-detail-stat">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/>
                            <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
                        </svg>
                        <span class="video-detail-stat-value">${formatNumber(record.shareCount || 0)}</span>
                    </div>
                </div>
                
                <div class="video-detail-actions">
                    ${record.coverUrl ? `
                    <button class="btn btn-secondary" onclick="openLightbox('${escapeHtml(record.coverUrl)}', '${escapeHtml(record.title || '')}', '${record.duration ? formatDuration(record.duration) : ''}')">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                            <circle cx="12" cy="12" r="3"/>
                        </svg>
                        预览封面
                    </button>
                    ` : ''}
                    <button class="btn btn-primary" onclick="downloadBrowseRecord('${escapeHtml(record.id)}')">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 16px; height: 16px;">
                            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                            <polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
                        </svg>
                        下载视频
                    </button>
                    ${record.pageUrl ? `<a class="btn btn-secondary" href="${escapeHtml(record.pageUrl)}" target="_blank">打开原页面</a>` : ''}
                    <button class="btn btn-danger" onclick="deleteBrowseRecord('${escapeHtml(record.id)}')">删除记录</button>
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
    // Download Records Page (Task 16)
    // ============================================
    
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
                    <td colspan="11">
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
            
            html += `
                <tr class="${isSelected ? 'selected' : ''} ${record.status === 'failed' ? 'error-row' : ''}" data-id="${escapeHtml(record.id)}">
                    <td onclick="event.stopPropagation();">
                        <input type="checkbox" ${isSelected ? 'checked' : ''} onchange="toggleDownloadSelect('${escapeHtml(record.id)}', this.checked)">
                    </td>
                    <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">${thumbnail}</td>
                    <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                        <div class="table-title" title="${escapeHtml(record.title || '无标题')}">${escapeHtml(record.title || '无标题')}</div>
                        ${record.errorMessage ? `<div class="table-error-hint" title="${escapeHtml(record.errorMessage)}">⚠ ${escapeHtml(record.errorMessage.substring(0, 30))}${record.errorMessage.length > 30 ? '...' : ''}</div>` : ''}
                    </td>
                    <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                        <span class="table-author">${escapeHtml(record.author || '未知')}</span>
                    </td>
                    <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                        <span class="table-meta">${record.duration ? formatDuration(record.duration) : '-'}</span>
                    </td>
                    <td onclick="showDownloadDetail('${escapeHtml(record.id)}')">
                        <span class="table-meta">${escapeHtml(record.resolution || '-')}</span>
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
                            <button class="table-action-btn" onclick="deleteDownloadRecord('${escapeHtml(record.id)}')" title="删除">
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
                <td colspan="11">
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
            ? `<img class="video-detail-thumbnail" src="${escapeHtml(record.coverUrl)}" alt="" style="width: 100%; border-radius: 8px; cursor: pointer;" onclick="openLightbox('${escapeHtml(record.coverUrl)}', '${escapeHtml(record.title || '')}', '${record.duration ? formatDuration(record.duration) : ''}')" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">`
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
                    <button class="btn btn-secondary" onclick="openLightbox('${escapeHtml(record.coverUrl)}', '${escapeHtml(record.title || '')}', '${record.duration ? formatDuration(record.duration) : ''}')">
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
    // Settings Page Functions - Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 13.7
    // ============================================

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
            const settings = {
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
            const settings = {
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

                if (data.done + data.failed === data.total && data.total > 0) {
                    clearInterval(batchProgressInterval);
                    batchProgressInterval = null;
                    showMessage(`下载完成！成功: ${data.done}, 失败: ${data.failed}`, data.failed > 0 ? 'info' : 'success');
                }
            }
        } catch (e) {
            console.error('Failed to update batch progress:', e);
        }
    }

    async function cancelBatchDownload() {
        if (!confirm('确定要取消当前的下载任务吗？')) return;

        try {
            const result = await ApiClient.cancelBatchDownload();
            if (result.success) {
                showMessage('下载已取消', 'info');
                if (batchProgressInterval) {
                    clearInterval(batchProgressInterval);
                    batchProgressInterval = null;
                }
            } else {
                showMessage('取消失败: ' + (result.error || '未知错误'), 'error');
            }
        } catch (e) {
            showMessage('请求失败: ' + e.message, 'error');
        }
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

    // ============================================
    // Other UI Functions
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
    // Utility Functions
    // ============================================
    function showMessage(message, type = 'info') {
        const messageArea = document.getElementById('messageArea');
        const alertClass = type === 'success' ? 'alert-success' : 
                          type === 'error' ? 'alert-error' : 'alert-info';
        
        const alertDiv = document.createElement('div');
        alertDiv.className = `alert ${alertClass}`;
        alertDiv.textContent = message;
        alertDiv.style.marginBottom = '8px';
        
        messageArea.appendChild(alertDiv);
        
        setTimeout(() => {
            alertDiv.remove();
        }, 5000);
    }

    function formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    // Format number with thousands separator
    function formatNumber(num) {
        if (num === null || num === undefined) return '0';
        return num.toLocaleString();
    }

    // Format duration in seconds to HH:MM:SS or MM:SS
    function formatDuration(ms) {
        if (!ms || ms <= 0) return '';
        // 输入是毫秒，转换为秒
        const totalSeconds = Math.floor(ms / 1000);
        const h = Math.floor(totalSeconds / 3600);
        const m = Math.floor((totalSeconds % 3600) / 60);
        const s = totalSeconds % 60;
        
        if (h > 0) {
            return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
        }
        return `${m}:${s.toString().padStart(2, '0')}`;
    }

    // Format relative time (e.g., "2小时前", "昨天")
    function formatRelativeTime(dateStr) {
        if (!dateStr) return '';
        
        try {
            const date = new Date(dateStr);
            const now = new Date();
            const diffMs = now - date;
            const diffSec = Math.floor(diffMs / 1000);
            const diffMin = Math.floor(diffSec / 60);
            const diffHour = Math.floor(diffMin / 60);
            const diffDay = Math.floor(diffHour / 24);
            
            if (diffSec < 60) return '刚刚';
            if (diffMin < 60) return `${diffMin}分钟前`;
            if (diffHour < 24) return `${diffHour}小时前`;
            if (diffDay === 1) return '昨天';
            if (diffDay < 7) return `${diffDay}天前`;
            
            // Format as date
            return `${date.getMonth() + 1}月${date.getDate()}日`;
        } catch (e) {
            return dateStr;
        }
    }

    // Escape HTML to prevent XSS
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ============================================
    // Video Preview and Player Functions - Requirements: 8.1, 8.2, 8.3, 8.4
    // ============================================

    // Video player state
    const videoPlayerState = {
        isPlaying: false,
        isMuted: false,
        isFullscreen: false,
        currentVideo: null,
        controlsTimeout: null
    };

    // ============================================
    // Thumbnail Component - Requirements: 8.1
    // ============================================

    /**
     * Create a thumbnail component HTML
     * @param {Object} options - Thumbnail options
     * @param {string} options.coverUrl - Cover image URL
     * @param {number} options.duration - Video duration in seconds
     * @param {string} options.videoId - Video ID for click handling
     * @param {string} options.videoUrl - Video URL for playback
     * @param {string} options.title - Video title
     * @param {string} options.filePath - Local file path (for downloaded videos)
     * @param {boolean} options.isDownloaded - Whether the video is downloaded
     * @returns {string} HTML string for the thumbnail
     */
    function createThumbnailComponent(options) {
        const { coverUrl, duration, videoId, videoUrl, title, filePath, isDownloaded } = options;
        const durationText = duration ? formatDuration(duration) : '';
        
        // Determine click action based on whether video is downloaded
        const clickAction = isDownloaded && filePath 
            ? `openVideoPlayer('${escapeHtml(filePath)}', '${escapeHtml(title || '')}', true)`
            : coverUrl 
                ? `openLightbox('${escapeHtml(coverUrl)}', '${escapeHtml(title || '')}', '${durationText}')`
                : '';
        
        const thumbnailContent = coverUrl 
            ? `<img src="${escapeHtml(coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">`
            : '';
        
        return `
            <div class="video-thumbnail" onclick="${clickAction}">
                ${thumbnailContent}
                <div class="video-thumbnail-placeholder" ${coverUrl ? 'style="display:none"' : ''}>
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polygon points="23 7 16 12 23 17 23 7"/>
                        <rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
                    </svg>
                </div>
                <div class="video-thumbnail-overlay">
                    <div class="video-thumbnail-play-btn">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polygon points="5 3 19 12 5 21 5 3"/>
                        </svg>
                    </div>
                </div>
                ${durationText ? `<span class="video-thumbnail-duration">${durationText}</span>` : ''}
            </div>
        `;
    }

    /**
     * Render a simple thumbnail image (for tables and lists)
     * @param {string} coverUrl - Cover image URL
     * @param {string} title - Video title for alt text
     * @param {Function} onClick - Click handler
     * @returns {string} HTML string
     */
    function renderThumbnailImage(coverUrl, title, onClick) {
        if (coverUrl) {
            return `
                <img class="table-thumbnail" 
                     src="${escapeHtml(coverUrl)}" 
                     alt="${escapeHtml(title || '')}"
                     onclick="${onClick || ''}"
                     onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
                <div class="table-thumbnail-placeholder" style="display:none">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polygon points="23 7 16 12 23 17 23 7"/>
                        <rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
                    </svg>
                </div>
            `;
        }
        return `
            <div class="table-thumbnail-placeholder">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polygon points="23 7 16 12 23 17 23 7"/>
                    <rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>
                </svg>
            </div>
        `;
    }

    // ============================================
    // Lightbox for Cover Image Preview - Requirements: 8.3
    // ============================================

    /**
     * Open the lightbox to preview a cover image
     * @param {string} imageUrl - URL of the image to display
     * @param {string} title - Title to display below the image
     * @param {string} meta - Additional metadata (e.g., duration)
     */
    function openLightbox(imageUrl, title, meta) {
        if (!imageUrl) {
            showMessage('没有可预览的图片', 'info');
            return;
        }

        const overlay = document.getElementById('lightboxOverlay');
        const image = document.getElementById('lightboxImage');
        const titleEl = document.getElementById('lightboxTitle');
        const metaEl = document.getElementById('lightboxMeta');

        // Set content
        image.src = imageUrl;
        titleEl.textContent = title || '';
        metaEl.textContent = meta || '';

        // Show lightbox
        overlay.classList.add('active');

        // Add keyboard listener for ESC
        document.addEventListener('keydown', handleLightboxKeydown);

        // Prevent body scroll
        document.body.style.overflow = 'hidden';
    }

    /**
     * Close the lightbox
     * @param {Event} event - Click event (optional)
     */
    function closeLightbox(event) {
        if (event) {
            event.stopPropagation();
        }

        const overlay = document.getElementById('lightboxOverlay');
        overlay.classList.remove('active');

        // Remove keyboard listener
        document.removeEventListener('keydown', handleLightboxKeydown);

        // Restore body scroll
        document.body.style.overflow = '';

        // Clear image source after animation
        setTimeout(() => {
            document.getElementById('lightboxImage').src = '';
        }, 300);
    }

    /**
     * Handle keyboard events for lightbox
     * @param {KeyboardEvent} event
     */
    function handleLightboxKeydown(event) {
        if (event.key === 'Escape') {
            closeLightbox();
        }
    }

    // ============================================
    // Video Player Modal - Requirements: 8.2, 8.4
    // ============================================

    /**
     * Open the video player modal
     * @param {string} videoSource - URL or file path of the video
     * @param {string} title - Video title
     * @param {boolean} isLocalFile - Whether the source is a local file
     */
    function openVideoPlayer(videoSource, title, isLocalFile = false) {
        if (!videoSource) {
            showMessage('没有可播放的视频', 'error');
            return;
        }

        const modal = document.getElementById('videoPlayerModal');
        const video = document.getElementById('videoPlayer');
        const titleEl = document.getElementById('videoPlayerTitle');
        const loadingEl = document.getElementById('videoPlayerLoading');
        const errorEl = document.getElementById('videoPlayerError');

        // Reset state
        videoPlayerState.isPlaying = false;
        videoPlayerState.currentVideo = { source: videoSource, title: title, isLocal: isLocalFile };

        // Set title
        titleEl.textContent = title || '视频播放';

        // Show loading state
        loadingEl.style.display = 'flex';
        errorEl.style.display = 'none';

        // Set video source
        if (isLocalFile) {
            // For local files, we need to use a special API endpoint
            // The backend should serve the file through an API
            video.src = `${ConnectionManager.serviceUrl}/api/video/stream?path=${encodeURIComponent(videoSource)}`;
        } else {
            video.src = videoSource;
        }

        // Show modal
        modal.classList.add('active');

        // Add event listeners
        video.addEventListener('loadedmetadata', handleVideoLoaded);
        video.addEventListener('error', handleVideoError);
        video.addEventListener('timeupdate', updateVideoProgress);
        video.addEventListener('ended', handleVideoEnded);
        video.addEventListener('play', handleVideoPlay);
        video.addEventListener('pause', handleVideoPause);

        // Add keyboard listener
        document.addEventListener('keydown', handleVideoPlayerKeydown);

        // Prevent body scroll
        document.body.style.overflow = 'hidden';

        // Setup controls visibility
        setupControlsVisibility();

        // Load the video
        video.load();
    }

    /**
     * Close the video player modal
     */
    function closeVideoPlayer() {
        const modal = document.getElementById('videoPlayerModal');
        const video = document.getElementById('videoPlayer');

        // Pause and reset video
        video.pause();
        video.currentTime = 0;

        // Remove event listeners
        video.removeEventListener('loadedmetadata', handleVideoLoaded);
        video.removeEventListener('error', handleVideoError);
        video.removeEventListener('timeupdate', updateVideoProgress);
        video.removeEventListener('ended', handleVideoEnded);
        video.removeEventListener('play', handleVideoPlay);
        video.removeEventListener('pause', handleVideoPause);

        // Remove keyboard listener
        document.removeEventListener('keydown', handleVideoPlayerKeydown);

        // Exit fullscreen if active
        if (videoPlayerState.isFullscreen) {
            exitFullscreen();
        }

        // Hide modal
        modal.classList.remove('active');

        // Restore body scroll
        document.body.style.overflow = '';

        // Clear video source after animation
        setTimeout(() => {
            video.src = '';
            videoPlayerState.currentVideo = null;
        }, 300);

        // Clear controls timeout
        if (videoPlayerState.controlsTimeout) {
            clearTimeout(videoPlayerState.controlsTimeout);
        }
    }

    /**
     * Handle video loaded event
     */
    function handleVideoLoaded() {
        const loadingEl = document.getElementById('videoPlayerLoading');
        loadingEl.style.display = 'none';
        updateVideoTime();
    }

    /**
     * Handle video error event
     */
    function handleVideoError() {
        const loadingEl = document.getElementById('videoPlayerLoading');
        const errorEl = document.getElementById('videoPlayerError');
        const errorText = document.getElementById('videoPlayerErrorText');

        loadingEl.style.display = 'none';
        errorEl.style.display = 'flex';

        const video = document.getElementById('videoPlayer');
        let errorMessage = '无法加载视频';

        if (video.error) {
            switch (video.error.code) {
                case MediaError.MEDIA_ERR_ABORTED:
                    errorMessage = '视频加载被中止';
                    break;
                case MediaError.MEDIA_ERR_NETWORK:
                    errorMessage = '网络错误，无法加载视频';
                    break;
                case MediaError.MEDIA_ERR_DECODE:
                    errorMessage = '视频解码错误';
                    break;
                case MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED:
                    errorMessage = '不支持的视频格式';
                    break;
            }
        }

        errorText.textContent = errorMessage;
    }

    /**
     * Handle video ended event
     */
    function handleVideoEnded() {
        videoPlayerState.isPlaying = false;
        updatePlayPauseButton();
    }

    /**
     * Handle video play event
     */
    function handleVideoPlay() {
        videoPlayerState.isPlaying = true;
        updatePlayPauseButton();
    }

    /**
     * Handle video pause event
     */
    function handleVideoPause() {
        videoPlayerState.isPlaying = false;
        updatePlayPauseButton();
    }

    /**
     * Toggle play/pause - Requirements: 8.4
     */
    function togglePlayPause() {
        const video = document.getElementById('videoPlayer');

        if (video.paused) {
            video.play().catch(e => {
                console.error('Failed to play video:', e);
                showMessage('无法播放视频', 'error');
            });
        } else {
            video.pause();
        }
    }

    /**
     * Update play/pause button icon
     */
    function updatePlayPauseButton() {
        const playIcon = document.getElementById('playIcon');
        const pauseIcon = document.getElementById('pauseIcon');

        if (videoPlayerState.isPlaying) {
            playIcon.style.display = 'none';
            pauseIcon.style.display = 'block';
        } else {
            playIcon.style.display = 'block';
            pauseIcon.style.display = 'none';
        }
    }

    /**
     * Seek video to position - Requirements: 8.4
     * @param {MouseEvent} event
     */
    function seekVideo(event) {
        const video = document.getElementById('videoPlayer');
        const progressContainer = document.getElementById('videoProgressContainer');
        const rect = progressContainer.getBoundingClientRect();
        const pos = (event.clientX - rect.left) / rect.width;
        video.currentTime = pos * video.duration;
    }

    /**
     * Update video progress bar
     */
    function updateVideoProgress() {
        const video = document.getElementById('videoPlayer');
        const progressBar = document.getElementById('videoProgressBar');

        if (video.duration) {
            const progress = (video.currentTime / video.duration) * 100;
            progressBar.style.width = `${progress}%`;
        }

        updateVideoTime();
    }

    /**
     * Update video time display
     */
    function updateVideoTime() {
        const video = document.getElementById('videoPlayer');
        const timeEl = document.getElementById('videoTime');

        const current = formatVideoTime(video.currentTime);
        const duration = formatVideoTime(video.duration || 0);
        timeEl.textContent = `${current} / ${duration}`;
    }

    /**
     * Format time for video player display
     * @param {number} seconds
     * @returns {string}
     */
    function formatVideoTime(seconds) {
        if (isNaN(seconds) || !isFinite(seconds)) return '00:00';

        const h = Math.floor(seconds / 3600);
        const m = Math.floor((seconds % 3600) / 60);
        const s = Math.floor(seconds % 60);

        if (h > 0) {
            return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
        }
        return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
    }

    /**
     * Toggle mute - Requirements: 8.4
     */
    function toggleMute() {
        const video = document.getElementById('videoPlayer');
        video.muted = !video.muted;
        videoPlayerState.isMuted = video.muted;
        updateVolumeButton();
    }

    /**
     * Change volume - Requirements: 8.4
     * @param {number} value - Volume value (0-1)
     */
    function changeVolume(value) {
        const video = document.getElementById('videoPlayer');
        video.volume = parseFloat(value);

        if (video.volume === 0) {
            video.muted = true;
        } else if (video.muted) {
            video.muted = false;
        }

        videoPlayerState.isMuted = video.muted;
        updateVolumeButton();
    }

    /**
     * Update volume button icon
     */
    function updateVolumeButton() {
        const volumeIcon = document.getElementById('volumeIcon');
        const muteIcon = document.getElementById('muteIcon');
        const volumeSlider = document.getElementById('volumeSlider');
        const video = document.getElementById('videoPlayer');

        if (videoPlayerState.isMuted || video.volume === 0) {
            volumeIcon.style.display = 'none';
            muteIcon.style.display = 'block';
        } else {
            volumeIcon.style.display = 'block';
            muteIcon.style.display = 'none';
        }

        volumeSlider.value = video.muted ? 0 : video.volume;
    }

    /**
     * Toggle fullscreen - Requirements: 8.4
     */
    function toggleFullscreen() {
        if (videoPlayerState.isFullscreen) {
            exitFullscreen();
        } else {
            enterFullscreen();
        }
    }

    /**
     * Enter fullscreen mode
     */
    function enterFullscreen() {
        const wrapper = document.getElementById('videoPlayerWrapper');

        if (wrapper.requestFullscreen) {
            wrapper.requestFullscreen();
        } else if (wrapper.webkitRequestFullscreen) {
            wrapper.webkitRequestFullscreen();
        } else if (wrapper.msRequestFullscreen) {
            wrapper.msRequestFullscreen();
        }

        videoPlayerState.isFullscreen = true;
        updateFullscreenButton();

        // Listen for fullscreen change
        document.addEventListener('fullscreenchange', handleFullscreenChange);
        document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
    }

    /**
     * Exit fullscreen mode
     */
    function exitFullscreen() {
        if (document.exitFullscreen) {
            document.exitFullscreen();
        } else if (document.webkitExitFullscreen) {
            document.webkitExitFullscreen();
        } else if (document.msExitFullscreen) {
            document.msExitFullscreen();
        }

        videoPlayerState.isFullscreen = false;
        updateFullscreenButton();
    }

    /**
     * Handle fullscreen change event
     */
    function handleFullscreenChange() {
        videoPlayerState.isFullscreen = !!(document.fullscreenElement || document.webkitFullscreenElement);
        updateFullscreenButton();

        if (!videoPlayerState.isFullscreen) {
            document.removeEventListener('fullscreenchange', handleFullscreenChange);
            document.removeEventListener('webkitfullscreenchange', handleFullscreenChange);
        }
    }

    /**
     * Update fullscreen button icon
     */
    function updateFullscreenButton() {
        const fullscreenIcon = document.getElementById('fullscreenIcon');
        const exitFullscreenIcon = document.getElementById('exitFullscreenIcon');

        if (videoPlayerState.isFullscreen) {
            fullscreenIcon.style.display = 'none';
            exitFullscreenIcon.style.display = 'block';
        } else {
            fullscreenIcon.style.display = 'block';
            exitFullscreenIcon.style.display = 'none';
        }
    }

    /**
     * Handle keyboard events for video player
     * @param {KeyboardEvent} event
     */
    function handleVideoPlayerKeydown(event) {
        const video = document.getElementById('videoPlayer');

        switch (event.key) {
            case 'Escape':
                if (videoPlayerState.isFullscreen) {
                    exitFullscreen();
                } else {
                    closeVideoPlayer();
                }
                break;
            case ' ':
            case 'k':
                event.preventDefault();
                togglePlayPause();
                break;
            case 'ArrowLeft':
                event.preventDefault();
                video.currentTime = Math.max(0, video.currentTime - 5);
                break;
            case 'ArrowRight':
                event.preventDefault();
                video.currentTime = Math.min(video.duration, video.currentTime + 5);
                break;
            case 'ArrowUp':
                event.preventDefault();
                video.volume = Math.min(1, video.volume + 0.1);
                updateVolumeButton();
                break;
            case 'ArrowDown':
                event.preventDefault();
                video.volume = Math.max(0, video.volume - 0.1);
                updateVolumeButton();
                break;
            case 'm':
                toggleMute();
                break;
            case 'f':
                toggleFullscreen();
                break;
        }
    }

    /**
     * Setup controls visibility (auto-hide)
     */
    function setupControlsVisibility() {
        const wrapper = document.getElementById('videoPlayerWrapper');
        const controls = document.getElementById('videoControls');

        wrapper.addEventListener('mousemove', () => {
            controls.classList.add('visible');

            if (videoPlayerState.controlsTimeout) {
                clearTimeout(videoPlayerState.controlsTimeout);
            }

            videoPlayerState.controlsTimeout = setTimeout(() => {
                if (videoPlayerState.isPlaying) {
                    controls.classList.remove('visible');
                }
            }, 3000);
        });

        wrapper.addEventListener('mouseleave', () => {
            if (videoPlayerState.isPlaying) {
                controls.classList.remove('visible');
            }
        });
    }

    /**
     * Preview video from browse record (opens lightbox for cover or player for downloaded)
     * @param {string} id - Record ID
     */
    function previewBrowseVideo(id) {
        const record = browseState.records.find(r => r.id === id);
        if (!record) {
            showMessage('记录不存在', 'error');
            return;
        }

        // For browse records, show cover image in lightbox
        if (record.coverUrl) {
            openLightbox(record.coverUrl, record.title, record.duration ? formatDuration(record.duration) : '');
        } else {
            showMessage('没有可预览的封面图片', 'info');
        }
    }

    /**
     * Play downloaded video
     * @param {string} id - Download record ID
     */
    function playDownloadedVideo(id) {
        const record = downloadState.records.find(r => r.id === id);
        if (!record) {
            showMessage('记录不存在', 'error');
            return;
        }

        if (record.status !== 'completed') {
            showMessage('视频尚未下载完成', 'info');
            return;
        }

        if (!record.filePath) {
            showMessage('文件路径不可用', 'error');
            return;
        }

        openVideoPlayer(record.filePath, record.title, true);
    }

    // ============================================
    // WebSocket Event Handlers - Requirements: 10.6
    // ============================================
    WebSocketClient.onDownloadProgress((progress) => {
        // Update queue item progress in real-time - Requirements: 10.6
        if (currentPage === 'queue') {
            updateQueueItemProgress(progress);
        }
    });

    WebSocketClient.onQueueChange((change) => {
        if (currentPage === 'queue') {
            // Handle different queue change actions
            if (change.action === 'add' && change.item) {
                queueState.items.push(change.item);
                renderQueueList();
                updateQueueStats();
            } else if (change.action === 'remove' && change.item) {
                queueState.items = queueState.items.filter(i => i.id !== change.item.id);
                renderQueueList();
                updateQueueStats();
            } else if (change.queue) {
                // Full queue update
                queueState.items = change.queue;
                renderQueueList();
                updateQueueStats();
            } else {
                // Fallback: reload entire queue
                loadDownloadQueue();
            }
        }
    });

    WebSocketClient.onStatsUpdate((stats) => {
        if (currentPage === 'dashboard') {
            loadDashboardData();
        }
    });

    // ============================================
    // Initialization
    // ============================================
    document.addEventListener('DOMContentLoaded', () => {
        // Initialize SPA router - Requirements: 6.2
        initRouter();
        
        // Load saved service URL
        const savedUrl = StorageManager.loadServiceUrl();
        document.getElementById('settingServiceUrl').value = savedUrl;
        
        // Connect to service
        ConnectionManager.connect(savedUrl);
        
        // Load initial page data when connected
        ConnectionManager.onStatusChange((status) => {
            if (status === 'connected') {
                loadPageData(currentPage);
            }
        });
    });