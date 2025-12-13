/**
 * Core Module - 核心功能
 * 包含：配置、存储、连接管理、API客户端、WebSocket
 */

// ============================================
// Constants and Configuration
// ============================================
const STORAGE_KEY_SERVICE_URL = 'wx_channel_service_url';
const DEFAULT_SERVICE_URL = 'http://127.0.0.1:2025';
const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000, 30000];

// ============================================
// LocalStorage Persistence
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
// Operation Queue
// ============================================
const OperationQueue = {
    queue: [],
    maxRetries: 3,

    add(operation) {
        operation.retries = operation.retries || 0;
        this.queue.push(operation);
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
                    this.queue.unshift(operation);
                } else {
                    console.error('Operation exceeded max retries:', operation);
                    showMessage(`操作失败: ${operation.type}`, 'error');
                }
                break;
            }
        }
    },

    async execute(operation) {
        const { method, endpoint, data } = operation;
        return await ApiClient.request(method, endpoint, data);
    },

    clear() { this.queue = []; },
    getQueueLength() { return this.queue.length; }
};

// ============================================
// Connection Manager
// ============================================
const ConnectionManager = {
    status: 'disconnected',
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
        if (status === 'connected') {
            OperationQueue.executeAll();
        }
    },

    getStatus() { return this.status; },

    onStatusChange(callback) {
        this.statusCallbacks.push(callback);
    },

    updateStatusUI() {
        const dot = document.getElementById('statusDot');
        const text = document.getElementById('statusText');
        if (!dot || !text) return;
        
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
// API Client
// ============================================
const ApiClient = {
    async request(method, endpoint, data = null) {
        if (ConnectionManager.getStatus() !== 'connected') {
            throw new Error('Not connected to service');
        }

        const url = `${ConnectionManager.serviceUrl}/api${endpoint}`;
        const options = {
            method,
            headers: { 'Content-Type': 'application/json' }
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
    async getBrowseRecord(id) { return await this.request('GET', `/browse/${id}`); },
    async deleteBrowseRecords(ids) { return await this.request('DELETE', '/browse', { ids }); },
    async clearBrowseHistory() { return await this.request('DELETE', '/browse/clear'); },

    // Download Records
    async getDownloadRecords(params = {}) {
        const query = new URLSearchParams(params).toString();
        return await this.request('GET', `/downloads${query ? '?' + query : ''}`);
    },
    async getDownloadRecord(id) { return await this.request('GET', `/downloads/${id}`); },
    async deleteDownloadRecords(ids, deleteFiles = false) { return await this.request('DELETE', '/downloads', { ids, deleteFiles }); },
    async clearDownloadRecords(deleteFiles = false) { return await this.request('DELETE', '/downloads/clear', { deleteFiles }); },
    async cleanupByDate(type, beforeDate, deleteFiles = false) {
        const endpoint = type === 'browse' ? '/browse/cleanup' : '/downloads/cleanup';
        return await this.request('DELETE', endpoint, { beforeDate, deleteFiles });
    },

    // Download Queue
    async getDownloadQueue() { return await this.request('GET', '/queue'); },
    async addToQueue(videos) { return await this.request('POST', '/queue', { videos }); },
    async pauseDownload(id) { return await this.request('PUT', `/queue/${id}/pause`); },
    async resumeDownload(id) { return await this.request('PUT', `/queue/${id}/resume`); },
    async removeFromQueue(id) { return await this.request('DELETE', `/queue/${id}`); },
    async reorderQueue(ids) { return await this.request('PUT', '/queue/reorder', { ids }); },
    async completeDownload(id) { return await this.request('PUT', `/queue/${id}/complete`); },
    async failDownload(id, error) { return await this.request('PUT', `/queue/${id}/fail`, { error }); },

    // Settings
    async getSettings() { return await this.request('GET', '/settings'); },
    async updateSettings(settings) { return await this.request('PUT', '/settings', settings); },

    // Statistics
    async getStatistics() { return await this.request('GET', '/stats'); },
    async getChartData() { return await this.request('GET', '/stats/chart'); },

    // Export
    async exportData(type, format, ids = null) {
        const params = { format };
        if (ids) params.ids = ids.join(',');
        const query = new URLSearchParams(params).toString();
        return await this.request('GET', `/export/${type}?${query}`);
    },

    // Search
    async search(query) { return await this.request('GET', `/search?q=${encodeURIComponent(query)}`); },

    // Batch Download
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
    async openFolder(filePath) { return await this.request('POST', '/files/open-folder', { path: filePath }); },
    async playVideo(filePath) { return await this.request('POST', '/files/play', { path: filePath }); },
    async retryDownloadRecord(id) { return await this.request('POST', `/downloads/${id}/retry`); }
};

// ============================================
// WebSocket Client
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
        if (this.ws && this.ws.readyState === WebSocket.OPEN) return;

        const serviceUrl = new URL(ConnectionManager.serviceUrl);
        const wsPort = parseInt(serviceUrl.port || '2025') + 1;
        const wsUrl = `ws://${serviceUrl.hostname}:${wsPort}/ws`;
        
        try {
            this.ws = new WebSocket(wsUrl);
            this.ws.onopen = () => console.log('WebSocket connected');
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
            this.ws.onerror = (error) => console.error('WebSocket error:', error);
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

    onDownloadProgress(callback) { this.callbacks.downloadProgress.push(callback); },
    onQueueChange(callback) { this.callbacks.queueChange.push(callback); },
    onStatsUpdate(callback) { this.callbacks.statsUpdate.push(callback); }
};

console.log('Core module loaded');
