/**
 * Radar Module - 对标雷达功能
 * 管理对标账号的监控和轮询
 */

// ============================================
// Radar Target Management
// ============================================

let radarTargets = [];

async function loadRadarTargets() {
    try {
        const data = await ApiClient.request('GET', '/v1/radar/targets');
        radarTargets = data.targets || [];
        renderRadarTable();
    } catch (e) {
        console.warn('[Radar] Failed to load targets:', e);
        const tbody = document.getElementById('radarTableBody');
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-secondary);padding:40px;">暂无监控目标，点击"添加监控"开始</td></tr>';
        }
    }
}

function renderRadarTable() {
    const tbody = document.getElementById('radarTableBody');
    if (!tbody) return;

    if (!radarTargets || radarTargets.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-secondary);padding:40px;">暂无监控目标，点击"添加监控"开始</td></tr>';
        return;
    }

    tbody.innerHTML = radarTargets.map(t => `
        <tr>
            <td>${escapeHtml(t.author_name || t.name || '')}</td>
            <td title="${escapeHtml(t.username || '')}">${(t.username || '').substring(0, 20)}...</td>
            <td>${t.interval || 60}分钟</td>
            <td>${t.last_check ? new Date(t.last_check).toLocaleString('zh-CN') : '从未'}</td>
            <td><span class="status-badge ${t.enabled ? 'success' : 'secondary'}">${t.enabled ? '运行中' : '已暂停'}</span></td>
            <td>
                <button class="btn btn-sm btn-secondary" onclick="viewRadarLogs('${t.id}', '${escapeHtml(t.author_name || t.name || '')}')">日志</button>
                <button class="btn btn-sm btn-secondary" onclick="editRadarTarget('${t.id}')">编辑</button>
                <button class="btn btn-sm btn-danger" onclick="deleteRadarTarget('${t.id}')">删除</button>
            </td>
        </tr>
    `).join('');
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// ============================================
// Radar Dialog Operations
// ============================================

function openAddRadarModal() {
    document.getElementById('radarDialogTitle').textContent = '添加监控目标';
    document.getElementById('radarId').value = '';
    document.getElementById('radarAuthorName').value = '';
    document.getElementById('radarUsername').value = '';
    document.getElementById('radarInterval').value = '60';
    document.getElementById('addRadarDialogOverlay').style.display = 'flex';
}

function closeAddRadarModal(event) {
    if (event) event.stopPropagation();
    document.getElementById('addRadarDialogOverlay').style.display = 'none';
}

async function saveRadarTarget() {
    const id = document.getElementById('radarId').value;
    const name = document.getElementById('radarAuthorName').value.trim();
    const username = document.getElementById('radarUsername').value.trim();
    const interval = parseInt(document.getElementById('radarInterval').value) || 60;

    if (!name || !username) {
        showMessage('请填写博主名称和唯一ID', 'error');
        return;
    }

    try {
        const payload = { author_name: name, username, interval };
        if (id) {
            await ApiClient.request('PUT', `/v1/radar/targets/${id}`, payload);
            showMessage('监控目标已更新', 'success');
        } else {
            await ApiClient.request('POST', '/v1/radar/targets', payload);
            showMessage('监控目标已添加', 'success');
        }
        closeAddRadarModal();
        loadRadarTargets();
    } catch (e) {
        showMessage('操作失败: ' + e.message, 'error');
    }
}

function editRadarTarget(id) {
    const target = radarTargets.find(t => t.id === id);
    if (!target) return;

    document.getElementById('radarDialogTitle').textContent = '编辑监控目标';
    document.getElementById('radarId').value = target.id;
    document.getElementById('radarAuthorName').value = target.author_name || target.name || '';
    document.getElementById('radarUsername').value = target.username || '';
    document.getElementById('radarInterval').value = target.interval || 60;
    document.getElementById('addRadarDialogOverlay').style.display = 'flex';
}

async function deleteRadarTarget(id) {
    if (!confirm('确定要删除此监控目标吗？')) return;
    try {
        await ApiClient.request('DELETE', `/v1/radar/targets/${id}`);
        showMessage('监控目标已删除', 'success');
        loadRadarTargets();
    } catch (e) {
        showMessage('删除失败: ' + e.message, 'error');
    }
}

// ============================================
// Radar Logs
// ============================================

function viewRadarLogs(targetId, authorName) {
    document.getElementById('radarLogsTitle').textContent = `${authorName} - 监控日志`;
    document.getElementById('radarLogsDialogOverlay').style.display = 'flex';
    loadRadarLogs(targetId);
}

async function loadRadarLogs(targetId) {
    const tbody = document.getElementById('radarLogsTableBody');
    try {
        const data = await ApiClient.request('GET', `/v1/radar/targets/${targetId}/logs`);
        const logs = data.logs || [];
        if (logs.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;padding:20px;">暂无日志</td></tr>';
            return;
        }
        tbody.innerHTML = logs.map(log => `
            <tr>
                <td>${new Date(log.timestamp).toLocaleString('zh-CN')}</td>
                <td>${log.video_count || 0}</td>
                <td>${log.new_count || 0}</td>
                <td>${log.status || '-'}</td>
                <td></td>
            </tr>
        `).join('');
    } catch (e) {
        tbody.innerHTML = `<tr><td colspan="5" style="text-align:center;padding:20px;">加载失败: ${e.message}</td></tr>`;
    }
}

function closeRadarLogsModal(event) {
    if (event) event.stopPropagation();
    document.getElementById('radarLogsDialogOverlay').style.display = 'none';
}

// ============================================
// Radar Settings Toggle
// ============================================

function toggleRadarEnabled() {
    const toggle = document.getElementById('radarEnabledToggle');
    const checkbox = document.getElementById('settingRadarEnabled');
    if (!toggle || !checkbox) return;
    checkbox.checked = !checkbox.checked;
    toggle.classList.toggle('active', checkbox.checked);
}

// Initialize radar when page loads
ConnectionManager.onStatusChange(function(status) {
    if (status === 'connected') {
        loadRadarTargets();
    }
});
