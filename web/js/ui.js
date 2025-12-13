/**
 * UI Module - 界面和工具函数
 * 包含：工具函数、UI导航、仪表盘
 */

// ============================================
// Utility Functions
// ============================================
function showMessage(message, type = 'info') {
    const messageArea = document.getElementById('messageArea');
    if (!messageArea) return;
    
    // 使用新的 toast 样式
    const toast = document.createElement('div');
    toast.className = `message-toast ${type}`;
    
    // 添加图标
    const icons = {
        success: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:18px;height:18px;flex-shrink:0"><polyline points="20 6 9 17 4 12"/></svg>',
        error: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:18px;height:18px;flex-shrink:0"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
        info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:18px;height:18px;flex-shrink:0"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>',
        warning: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:18px;height:18px;flex-shrink:0"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>'
    };
    
    toast.innerHTML = `${icons[type] || icons.info}<span>${escapeHtml(message)}</span>`;
    
    messageArea.appendChild(toast);
    
    // 自动移除
    setTimeout(() => {
        toast.style.animation = 'slideIn 0.3s ease reverse';
        setTimeout(() => toast.remove(), 300);
    }, 4000);
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatNumber(num) {
    if (num === null || num === undefined) return '0';
    return num.toLocaleString();
}

function formatDuration(ms) {
    if (!ms || ms <= 0) return '';
    const totalSeconds = Math.floor(ms / 1000);
    const h = Math.floor(totalSeconds / 3600);
    const m = Math.floor((totalSeconds % 3600) / 60);
    const s = totalSeconds % 60;
    
    if (h > 0) {
        return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
    }
    return `${m}:${s.toString().padStart(2, '0')}`;
}

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
        
        return `${date.getMonth() + 1}月${date.getDate()}日`;
    } catch (e) {
        return dateStr;
    }
}

function formatChartLabel(dateStr) {
    try {
        const date = new Date(dateStr);
        return `${date.getMonth() + 1}/${date.getDate()}`;
    } catch (e) {
        return dateStr;
    }
}

function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

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
// UI Navigation
// ============================================
let currentPage = 'dashboard';
const validPages = ['dashboard', 'browse', 'downloads', 'batch', 'queue', 'settings', 'help'];
const pageTitles = {
    dashboard: '仪表盘',
    browse: '浏览记录',
    downloads: '下载记录',
    batch: '批量下载',
    queue: '下载队列',
    settings: '设置',
    help: '帮助'
};

function navigateTo(page, updateHistory = true) {
    if (!validPages.includes(page)) {
        console.warn(`Invalid page: ${page}, defaulting to dashboard`);
        page = 'dashboard';
    }
    
    document.querySelectorAll('.nav-item').forEach(item => {
        item.classList.remove('active');
        if (item.dataset.page === page) {
            item.classList.add('active');
        }
    });

    document.querySelectorAll('.page-view').forEach(view => {
        view.classList.remove('active');
    });
    const pageElement = document.getElementById(`page-${page}`);
    if (pageElement) {
        pageElement.classList.add('active');
    }

    const titleEl = document.getElementById('pageTitle');
    if (titleEl) {
        titleEl.textContent = pageTitles[page] || page;
    }

    currentPage = page;
    
    if (updateHistory && window.location.hash !== `#${page}`) {
        history.pushState({ page }, pageTitles[page], `#${page}`);
    }

    if (window.innerWidth <= 768) {
        const sidebar = document.getElementById('sidebar');
        const overlay = document.getElementById('sidebarOverlay');
        if (sidebar) sidebar.classList.remove('open');
        if (overlay) overlay.classList.remove('open');
    }

    loadPageData(page);
}

function handlePopState(event) {
    const page = event.state?.page || getPageFromHash() || 'dashboard';
    navigateTo(page, false);
}

function getPageFromHash() {
    const hash = window.location.hash.slice(1);
    return validPages.includes(hash) ? hash : null;
}

function initRouter() {
    window.addEventListener('popstate', handlePopState);
    const initialPage = getPageFromHash() || 'dashboard';
    if (initialPage !== 'dashboard') {
        navigateTo(initialPage, false);
    }
    history.replaceState({ page: initialPage }, pageTitles[initialPage], `#${initialPage}`);
}

function toggleSidebar() {
    const sidebar = document.getElementById('sidebar');
    const overlay = document.getElementById('sidebarOverlay');
    if (sidebar) sidebar.classList.toggle('open');
    if (overlay) overlay.classList.toggle('open');
}

// ============================================
// Page Data Loading
// ============================================
async function loadPageData(page) {
    // 帮助页面不需要连接
    if (page === 'help') {
        await loadHelpDoc('INDEX');
        return;
    }
    
    if (ConnectionManager.getStatus() !== 'connected') return;

    try {
        switch (page) {
            case 'dashboard': await loadDashboardData(); break;
            case 'browse': await loadBrowseHistory(); break;
            case 'downloads': await loadDownloadRecords(); break;
            case 'queue': await loadDownloadQueue(); break;
            case 'settings': await loadSettings(); break;
        }
    } catch (e) {
        console.error('Failed to load page data:', e);
    }
}

async function loadDashboardData() {
    try {
        const stats = await ApiClient.getStatistics();
        if (stats.success && stats.data) {
            const data = stats.data;
            const el = (id) => document.getElementById(id);
            if (el('statTotalBrowse')) el('statTotalBrowse').textContent = formatNumber(data.totalBrowseCount || 0);
            if (el('statTotalDownload')) el('statTotalDownload').textContent = formatNumber(data.totalDownloadCount || 0);
            if (el('statTodayDownload')) el('statTodayDownload').textContent = formatNumber(data.todayDownloadCount || 0);
            if (el('statStorage')) el('statStorage').textContent = formatBytes(data.storageUsed || 0);
            
            // 更新图例数据
            if (el('legendCompleted')) el('legendCompleted').textContent = formatNumber(data.completedCount || data.totalDownloadCount || 0);
            if (el('legendPending')) el('legendPending').textContent = formatNumber(data.pendingCount || 0);
            if (el('legendFailed')) el('legendFailed').textContent = formatNumber(data.failedCount || 0);
            
            renderRecentBrowseList(data.recentBrowse || []);
            renderRecentDownloadList(data.recentDownload || []);
        }
        await loadChartData();
    } catch (e) {
        console.error('Failed to load dashboard data:', e);
    }
}

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

function renderChart(data) {
    const container = document.getElementById('chartBars');
    if (!container) return;
    
    if (!data.labels || !data.values || data.labels.length === 0) {
        renderEmptyChart();
        return;
    }
    
    const maxValue = Math.max(...data.values, 1);
    const maxHeight = 140;
    
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

function renderEmptyChart() {
    const container = document.getElementById('chartBars');
    if (container) {
        container.innerHTML = '<div class="chart-empty">暂无下载数据</div>';
    }
}

function renderRecentBrowseList(records) {
    const container = document.getElementById('recentBrowseList');
    if (!container) return;
    
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
        
        html += `
            <div class="recent-item" onclick="viewBrowseRecord('${escapeHtml(record.id)}')">
                ${thumbnail}
                <div class="recent-info">
                    <div class="recent-title">${escapeHtml(record.title || '无标题')}</div>
                    <div class="recent-meta">
                        <span>${escapeHtml(record.author || '未知作者')}</span>
                        ${record.duration ? `<span>${formatDuration(record.duration)}</span>` : ''}
                        ${record.browseTime ? `<span>${formatRelativeTime(record.browseTime)}</span>` : ''}
                    </div>
                </div>
            </div>
        `;
    }
    
    container.innerHTML = html;
}

function renderRecentDownloadList(records) {
    const container = document.getElementById('recentDownloadList');
    if (!container) return;
    
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
        const thumbnail = record.coverUrl 
            ? `<img class="recent-thumbnail" src="${escapeHtml(record.coverUrl)}" alt="" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'"><div class="recent-thumbnail-placeholder" style="display:none"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></div>`
            : `<div class="recent-thumbnail-placeholder"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></div>`;
        
        html += `
            <div class="recent-item" onclick="viewDownloadRecord('${escapeHtml(record.id)}')">
                ${thumbnail}
                <div class="recent-info">
                    <div class="recent-title">${escapeHtml(record.title || '无标题')}</div>
                    <div class="recent-meta">
                        <span>${escapeHtml(record.author || '未知作者')}</span>
                        ${record.fileSize ? `<span>${formatBytes(record.fileSize)}</span>` : ''}
                        ${record.downloadTime ? `<span>${formatRelativeTime(record.downloadTime)}</span>` : ''}
                    </div>
                </div>
                <span class="recent-status ${record.status || 'completed'}">${getStatusText(record.status)}</span>
            </div>
        `;
    }
    
    container.innerHTML = html;
}

function viewBrowseRecord(id) {
    navigateTo('browse');
}

function viewDownloadRecord(id) {
    navigateTo('downloads');
}

console.log('UI module loaded');


// ============================================
// Theme Toggle - 主题切换
// ============================================
function toggleTheme() {
    const html = document.documentElement;
    const currentTheme = html.getAttribute('data-theme') || 'light';
    const newTheme = currentTheme === 'light' ? 'dark' : 'light';
    
    html.setAttribute('data-theme', newTheme);
    localStorage.setItem('theme', newTheme);
    
    // 更新图标和文字
    updateThemeUI(newTheme);
    
    // 显示提示
    showMessage(`已切换到${newTheme === 'light' ? '浅色' : '深色'}模式`, 'success');
}

function updateThemeUI(theme) {
    const lightIcon = document.querySelector('.theme-icon-light');
    const darkIcon = document.querySelector('.theme-icon-dark');
    const themeText = document.getElementById('themeText');
    
    if (theme === 'dark') {
        lightIcon.style.display = 'none';
        darkIcon.style.display = 'block';
        themeText.textContent = '深色模式';
    } else {
        lightIcon.style.display = 'block';
        darkIcon.style.display = 'none';
        themeText.textContent = '浅色模式';
    }
}

// 初始化主题
function initTheme() {
    const savedTheme = localStorage.getItem('theme') || 'light';
    document.documentElement.setAttribute('data-theme', savedTheme);
    updateThemeUI(savedTheme);
}

// 页面加载时初始化主题
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initTheme);
} else {
    initTheme();
}


// ============================================
// Help Page - 帮助页面
// ============================================
async function loadHelpDoc(docName) {
    const helpContent = document.getElementById('helpContent');
    
    if (!docName) {
        docName = 'INDEX';
    }
    
    // 显示加载状态
    helpContent.innerHTML = `
        <div style="text-align: center; padding: 60px 20px; color: var(--text-muted);">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 48px; height: 48px; margin: 0 auto 16px; animation: spin 1s linear infinite;">
                <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
            </svg>
            <p>加载中...</p>
        </div>
    `;
    
    try {
        // 尝试加载 Markdown 文件
        const response = await fetch(`docs/${docName}.md`);
        
        if (!response.ok) {
            throw new Error('文档不存在');
        }
        
        const markdown = await response.text();
        
        // 简单的 Markdown 转 HTML（基础支持）
        const html = convertMarkdownToHTML(markdown);
        
        helpContent.innerHTML = html;
    } catch (error) {
        console.error('加载帮助文档失败:', error);
        helpContent.innerHTML = `
            <div style="text-align: center; padding: 60px 20px; color: var(--text-muted);">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 48px; height: 48px; margin: 0 auto 16px; color: var(--danger-color);">
                    <circle cx="12" cy="12" r="10"/>
                    <line x1="15" y1="9" x2="9" y2="15"/>
                    <line x1="9" y1="9" x2="15" y2="15"/>
                </svg>
                <p>加载文档失败</p>
                <p style="font-size: 13px; margin-top: 8px;">${escapeHtml(error.message)}</p>
            </div>
        `;
    }
}

// 简单的 Markdown 转 HTML 转换器
function convertMarkdownToHTML(markdown) {
    let html = markdown;
    
    // 转义 HTML
    html = html.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    
    // 代码块
    html = html.replace(/```(\w+)?\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>');
    
    // 标题
    html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
    html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
    html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');
    
    // 粗体和斜体
    html = html.replace(/\*\*\*(.*?)\*\*\*/g, '<strong><em>$1</em></strong>');
    html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/\*(.*?)\*/g, '<em>$1</em>');
    
    // 行内代码
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    
    // 链接
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');
    
    // 列表
    html = html.replace(/^\* (.*$)/gim, '<li>$1</li>');
    html = html.replace(/^- (.*$)/gim, '<li>$1</li>');
    html = html.replace(/(<li>.*<\/li>)/s, '<ul>$1</ul>');
    
    // 段落
    html = html.split('\n\n').map(para => {
        if (para.trim() && !para.startsWith('<')) {
            return '<p>' + para.trim() + '</p>';
        }
        return para;
    }).join('\n');
    
    return html;
}

// 页面加载时初始化帮助页面
document.addEventListener('DOMContentLoaded', function() {
    // 如果当前在帮助页面，加载默认文档
    if (window.location.hash === '#help') {
        loadHelpDoc('INDEX');
    }
});
