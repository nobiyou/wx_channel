// Radar Feature JS

// 状态变量
let radarTargets = [];
let radarEnabled = false;

const radarEnabledMessage = '系统会在后台静默轮询以下博主，发现新视频后会自动推入您的下载队列。(如果博主列表为空，请先在视频号添加博主)';
const radarDisabledMessage = '雷达未开启。请在 config.yaml 中将 radar_enabled 设置为 true 后重启程序，当前不会执行自动监控。';

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    // 监听导航切换，若切到 radar 页面则刷新列表
    const navItems = document.querySelectorAll('.nav-item');
    navItems.forEach(item => {
        item.addEventListener('click', () => {
            if (item.dataset.page === 'radar') {
                loadRadarTargets();
            }
        });
    });
});

// 加载雷达监控目标
async function loadRadarTargets() {
    try {
        const [response, settingsResult] = await Promise.all([
            fetch('/api/v1/radar/targets'),
            ApiClient.getSettings().catch(() => null)
        ]);
        if (!response.ok) throw new Error('加载失败');

        radarEnabled = !!(settingsResult && settingsResult.success && settingsResult.data && settingsResult.data.radarEnabled);
        renderRadarGlobalStatus();

        const res = await response.json();
        if (res.code === 0 || res.code === 200) {
            radarTargets = res.data || [];
            renderRadarTable();
        } else {
            showMessage(res.message || '加载目标失败', 'error');
        }
    } catch (err) {
        console.error('加载监控目标失败:', err);
        showMessage('加载失败，请检查网络', 'error');
    }
}

function renderRadarGlobalStatus() {
    const alert = document.getElementById('radarGlobalStatusAlert');
    const text = document.getElementById('radarGlobalStatusText');
    const addButton = document.getElementById('radarAddButton');
    if (!alert || !text) return;

    if (radarEnabled) {
        alert.classList.remove('alert-warning');
        alert.classList.add('alert-info');
        text.textContent = radarEnabledMessage;
        if (addButton) {
            addButton.disabled = false;
            addButton.title = '';
        }
        return;
    }

    alert.classList.remove('alert-info');
    alert.classList.add('alert-warning');
    text.textContent = radarDisabledMessage;
    if (addButton) {
        addButton.disabled = false;
        addButton.title = '可先配置监控目标，开启 radar_enabled 并重启后生效';
    }
}

// 渲染雷达列表表格
function renderRadarTable() {
    const tbody = document.getElementById('radarTableBody');
    if (!tbody) return;

    if (radarTargets.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 40px;">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 48px; height: 48px; margin: 0 auto 16px; opacity: 0.5;">
                        <circle cx="12" cy="12" r="10" />
                        <line x1="12" y1="8" x2="12" y2="12" />
                        <line x1="12" y1="16" x2="12.01" y2="16" />
                    </svg>
                    <p>暂无监控目标</p>
                </td>
            </tr>
        `;
        return;
    }

    tbody.innerHTML = radarTargets.map(target => {
        let statusClass = 'text-warning';
        let statusText = '雷达未开启';
        if (radarEnabled) {
            statusClass = target.status === 'active' ? 'text-success' : 'text-warning';
            statusText = target.status === 'active' ? '监控中' : '已暂停';
        }

        let lastCheck = '从未检测';
        if (target.last_check_time) {
            lastCheck = new Date(target.last_check_time).toLocaleString();
        }

        return `
            <tr>
                <td><strong>${escapeHtml(target.author_name)}</strong></td>
                <td>
                    <div style="max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${target.username}">
                        ${target.username}
                    </div>
                </td>
                <td>${target.interval_minutes} 分钟</td>
                <td><span style="font-size: 13px; color: var(--text-muted);">${lastCheck}</span></td>
                <td><span class="${statusClass}" style="font-weight: 500;">${statusText}</span></td>
                <td>
                    <div style="display: flex; gap: 8px; flex-wrap: wrap;">
                        ${target.status === 'active'
                ? `<button class="btn btn-secondary" onclick="toggleRadarStatus('${target.id}', 'paused')" style="padding: 4px 8px; font-size: 13px; flex-shrink: 0;">暂停</button>`
                : `<button class="btn btn-primary" onclick="toggleRadarStatus('${target.id}', 'active')" style="padding: 4px 8px; font-size: 13px; flex-shrink: 0;">恢复</button>`
            }
                        <button class="btn btn-secondary" onclick="editRadarTarget('${target.id}')" style="padding: 4px 8px; font-size: 13px; flex-shrink: 0;">编辑</button>
                        <button class="btn btn-secondary" onclick="showRadarLogs('${target.id}', '${escapeHtml(target.author_name)}')" style="padding: 4px 8px; font-size: 13px; flex-shrink: 0;">详情</button>
                        <button class="btn btn-danger" onclick="deleteRadarTarget('${target.id}')" style="padding: 4px 8px; font-size: 13px; flex-shrink: 0;">删除</button>
                    </div>
                </td>
            </tr>
        `;
    }).join('');
}

// 打开添加模态框
function openAddRadarModal() {
    document.getElementById('radarId').value = '';
    document.getElementById('radarAuthorName').value = '';
    document.getElementById('radarUsername').value = '';
    document.getElementById('radarInterval').value = 60;

    document.getElementById('radarDialogTitle').innerText = '添加监控目标';
    const overlay = document.getElementById('addRadarDialogOverlay');
    overlay.style.display = 'flex';
    // Use setTimeout to allow display:flex to apply before adding class for transition
    setTimeout(() => overlay.classList.add('active'), 10);
}

// 打开编辑模态框
function editRadarTarget(id) {
    const target = radarTargets.find(t => t.id === id);
    if (!target) return;

    document.getElementById('radarId').value = target.id;
    document.getElementById('radarAuthorName').value = target.author_name;
    document.getElementById('radarUsername').value = target.username;
    document.getElementById('radarInterval').value = target.interval_minutes;

    document.getElementById('radarDialogTitle').innerText = '编辑监控目标';
    const overlay = document.getElementById('addRadarDialogOverlay');
    overlay.style.display = 'flex';
    setTimeout(() => overlay.classList.add('active'), 10);
}

// 关闭模态框
function closeAddRadarModal(event) {
    if (event) event.preventDefault();
    const overlay = document.getElementById('addRadarDialogOverlay');
    overlay.classList.remove('active');
    setTimeout(() => {
        overlay.style.display = 'none';
    }, 300); // 匹配 CSS 的 transition 时间
}

// 保存监控目标
async function saveRadarTarget() {
    const id = document.getElementById('radarId').value;
    const authorName = document.getElementById('radarAuthorName').value.trim();
    const username = document.getElementById('radarUsername').value.trim();
    const intervalMinutes = parseInt(document.getElementById('radarInterval').value, 10);

    if (!authorName) return showMessage('请输入博主名称', 'warning');
    if (!username) return showMessage('请输入视频号ID', 'warning');
    if (intervalMinutes < 5) return showMessage('监控频率不能低于5分钟', 'warning');

    const data = {
        author_name: authorName,
        username: username,
        interval_minutes: intervalMinutes
    };

    try {
        let url = '/api/v1/radar/targets';
        let method = 'POST';

        if (id) {
            url += `/${id}`;
            method = 'PUT';
        }

        const response = await fetch(url, {
            method: method,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });

        const res = await response.json();
        if (res.code === 0 || res.code === 200) {
            showMessage(id ? '更新成功' : '添加成功', 'success');
            closeAddRadarModal();
            loadRadarTargets();
        } else {
            showMessage(res.message || '操作失败', 'error');
        }
    } catch (err) {
        console.error('保存监控目标失败:', err);
        showMessage('保存失败，请检查网络', 'error');
    }
}

// 切换监控状态
async function toggleRadarStatus(id, newStatus) {
    try {
        const response = await fetch(`/api/v1/radar/targets/${id}/status`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ status: newStatus })
        });

        if (response.ok) {
            showMessage(newStatus === 'active' ? '监控已恢复' : '监控已暂停', 'success');
            loadRadarTargets();
        } else {
            showMessage('状态更新失败', 'error');
        }
    } catch (err) {
        console.error('更新状态失败:', err);
        showMessage('网络错误', 'error');
    }
}

// 删除监控目标
async function deleteRadarTarget(id) {
    if (!confirm('确定要删除该监控目标吗？')) return;

    try {
        const response = await fetch(`/api/v1/radar/targets/${id}`, {
            method: 'DELETE'
        });

        if (response.ok) {
            showMessage('已删除', 'success');
            loadRadarTargets();
        } else {
            showMessage('删除失败', 'error');
        }
    } catch (err) {
        console.error('删除目标失败:', err);
        showMessage('网络错误', 'error');
    }
}

// ------------------- 雷达日志相关 -------------------

function closeRadarLogsModal() {
    const overlay = document.getElementById('radarLogsDialogOverlay');
    overlay.classList.remove('active');
    setTimeout(() => overlay.style.display = 'none', 300);
}

async function showRadarLogs(id, authorName) {
    document.getElementById('radarLogsTitle').innerText = `${authorName} 的监控日志`;

    const tbody = document.getElementById('radarLogsTableBody');
    tbody.innerHTML = `
        <tr>
            <td colspan="4" class="empty-state">
                <div class="video-player-spinner" style="margin: 20px auto;"></div>
                <p>正在加载日志...</p>
            </td>
        </tr>
    `;

    const overlay = document.getElementById('radarLogsDialogOverlay');
    overlay.style.display = 'flex';
    setTimeout(() => overlay.classList.add('active'), 10);

    try {
        const res = await fetch('/api/v1/radar/targets/' + id + '/logs');
        const json = await res.json();
        if (json.code === 0 || json.code === 200) {
            const logs = json.data || [];
            if (logs.length === 0) {
                tbody.innerHTML = `<tr><td colspan="5" class="empty-state">暂无执行日志，请等待首次检测...</td></tr>`;
                return;
            }

            tbody.innerHTML = logs.map((log, idx) => {
                const timeStr = new Date(log.check_time).toLocaleString();
                const statusHtml = log.status === 'success'
                    ? `<span style="color: var(--success-color);">成功</span>`
                    : `<span style="color: var(--danger-color);" title="${escapeHtml(log.error_message)}">失败: ${escapeHtml(log.error_message)}</span>`;

                let videoListHtml = '';
                let hasVideos = false;
                if (log.video_list) {
                    try {
                        const videos = JSON.parse(log.video_list);
                        if (videos && videos.length > 0) {
                            hasVideos = true;
                            videoListHtml = `
                                <tr id="radar-vlist-${idx}" style="display:none;">
                                    <td colspan="5" style="padding: 0 12px 12px; background: var(--bg-secondary);">
                                        <div style="max-height:200px; overflow-y:auto; font-size:12px;">
                                            <table style="width:100%; border-collapse:collapse;">
                                                <thead><tr style="color:var(--text-muted);">
                                                    <th style="padding:4px 8px; text-align:left; font-weight:500;">视频标题</th>
                                                    <th style="padding:4px 8px; width:70px; text-align:center; font-weight:500;">状态</th>
                                                </tr></thead>
                                                <tbody>${videos.map(v => `
                                                <tr style="border-top:1px solid var(--border-color);">
                                                    <td style="padding:4px 8px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:350px;" title="${escapeHtml(v.title)}">${escapeHtml(v.title)}</td>
                                                    <td style="padding:4px 8px; text-align:center;">${v.is_new ? '<span style="color:var(--success-color); font-weight:bold;">🆕新增</span>' : '<span style="color:var(--text-muted);">已有</span>'}</td>
                                                </tr>`).join('')}</tbody>
                                            </table>
                                        </div>
                                    </td>
                                </tr>`;
                        }
                    } catch (e) { }
                }

                return `
                    <tr style="cursor:${hasVideos ? 'pointer' : 'default'};" onclick="${hasVideos ? `toggleRadarVideoList(${idx})` : ''}">
                        <td style="color: var(--text-muted);">${timeStr}</td>
                        <td>${log.found_videos}</td>
                        <td style="color: ${log.new_videos > 0 ? 'var(--success-color)' : 'inherit'}; font-weight: ${log.new_videos > 0 ? 'bold' : 'normal'};">${log.new_videos}</td>
                        <td>${statusHtml}</td>
                        <td style="text-align:center; color:var(--text-muted);">${hasVideos ? '▶' : ''}</td>
                    </tr>
                    ${videoListHtml}`;
            }).join('');
        } else {
            tbody.innerHTML = `<tr><td colspan="5" class="empty-state" style="color:red">加载失败: ${escapeHtml(json.message)}</td></tr>`;
        }
    } catch (error) {
        tbody.innerHTML = `<tr><td colspan="5" class="empty-state" style="color:red">请求异常: ${error.message}</td></tr>`;
    }
}

function toggleRadarVideoList(idx) {
    const row = document.getElementById('radar-vlist-' + idx);
    if (!row) return;
    row.style.display = row.style.display === 'none' ? 'table-row' : 'none';
}

