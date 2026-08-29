// Public-account catalog console. The catalog is local and only contains
// accounts captured from a WeChat page session.

const officialAccountIcons = {
    search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.35-4.35"></path></svg>',
    refresh: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M20 11a8.1 8.1 0 0 0-15.5-2M4 5v4h4"></path><path d="M4 13a8.1 8.1 0 0 0 15.5 2M20 19v-4h-4"></path></svg>',
    history: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 7v5l3 2"></path><path d="M3 4v4h4"></path></svg>',
    recent: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M3 12a9 9 0 1 0 3-6.7"></path><path d="M3 4v5h5"></path><path d="M12 7v5l3 2"></path></svg>',
    cancel: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="m9 9 6 6M15 9l-6 6"></path></svg>',
    download: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M12 3v12"></path><path d="m7 10 5 5 5-5"></path><path d="M5 21h14"></path></svg>',
    upload: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M12 16V4"></path><path d="m7 9 5-5 5 5"></path><path d="M5 20h14"></path></svg>',
    copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>',
    external: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M14 3h7v7"></path><path d="M10 14 21 3"></path><path d="M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"></path></svg>',
    chevronLeft: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m15 18-6-6 6-6"></path></svg>',
    chevronRight: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m9 18 6-6-6-6"></path></svg>',
    database: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><ellipse cx="12" cy="5" rx="8" ry="3"></ellipse><path d="M4 5v7c0 1.7 3.6 3 8 3s8-1.3 8-3V5"></path><path d="M4 12v7c0 1.7 3.6 3 8 3s8-1.3 8-3v-7"></path></svg>',
    article: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 3h10l4 4v14H5z"></path><path d="M15 3v5h4M8 12h8M8 16h6"></path></svg>',
    video: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="5" width="13" height="14" rx="2"></rect><path d="m16 10 5-3v10l-5-3z"></path></svg>',
    audio: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M5 9v6M9 6v12M13 3v18M17 7v10M21 10v4"></path></svg>',
    clock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 7v5l3 2"></path></svg>',
    link: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M10 13a5 5 0 0 0 7.1.1l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1"></path><path d="M14 11a5 5 0 0 0-7.1-.1l-2 2A5 5 0 0 0 7 20l1.1-1.1"></path></svg>',
    check: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m5 12 4 4L19 6"></path></svg>'
};

const officialAccountConsoleState = {
    accountPage: 1,
    accountPageSize: 30,
    accountKeyword: '',
    accounts: [],
    accountTotal: 0,
    accountTotalPages: 0,
    selectedBiz: '',
    articlePage: 1,
    articlePageSize: 20,
    articleKeyword: '',
    archiveStatus: '',
    articleSort: 'publish_time',
    descending: true,
    syncMode: 'history',
    syncRun: null,
    metricSyncRun: null,
    accountRequestSequence: 0,
    articleRequestSequence: 0,
    metricSyncRequestSequence: 0,
    searchTimer: null,
    syncPollTimer: null,
    metricSyncPollTimer: null,
    metricSyncStartPromise: null
};

function officialAccountIcon(name) {
    return officialAccountIcons[name] || '';
}

function officialAccountRoot() {
    return document.getElementById('page-official-accounts');
}

function officialAccountListElement() {
    return document.getElementById('officialAccountsList');
}

function officialAccountData(result) {
    if (result && result.code !== undefined && Number(result.code) !== 0) {
        throw new Error(result.message || result.error || '公众号接口请求失败');
    }
    return result && result.data !== undefined ? result.data : result;
}

function officialAccountItems(data) {
    if (!data || typeof data !== 'object') return [];
    if (Array.isArray(data.items)) return data.items;
    if (Array.isArray(data.list)) return data.list;
    return [];
}

function officialAccountNumber(value) {
    const number = Number(value);
    return Number.isFinite(number) ? number : 0;
}

function officialAccountCount(value) {
    if (value === null || value === undefined || value === '') return '—';
    const number = Number(value);
    return Number.isFinite(number) ? number.toLocaleString('zh-CN') : '—';
}

function officialAccountDate(timestamp, fallback = '未记录') {
    const seconds = Number(timestamp || 0);
    if (!Number.isFinite(seconds) || seconds <= 0) return fallback;
    const date = new Date(seconds * 1000);
    if (typeof formatRelativeTime === 'function') {
        return `${formatRelativeTime(date.toISOString())} · ${date.toLocaleDateString('zh-CN')}`;
    }
    return date.toLocaleString('zh-CN');
}

function officialAccountDateOnly(timestamp, fallback = '未知时间') {
    const seconds = Number(timestamp || 0);
    if (!Number.isFinite(seconds) || seconds <= 0) return fallback;
    return new Date(seconds * 1000).toLocaleDateString('zh-CN');
}

function officialAccountDuration(value) {
    const seconds = Math.max(0, Math.floor(Number(value) || 0));
    if (!seconds) return '';
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const remainder = seconds % 60;
    if (hours > 0) {
        return `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`;
    }
    return `${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`;
}

function officialAccountExternalUrl(value) {
    const raw = String(value || '').trim();
    if (!raw) return '';
    try {
        const url = new URL(raw, window.location.href);
        return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : '';
    } catch (_) {
        return '';
    }
}

function officialAccountAvatarUrl(value) {
    const source = officialAccountExternalUrl(value);
    if (!source) return '';
    try {
        const parsed = new URL(source);
        const hostname = parsed.hostname.toLowerCase();
        const proxyable = hostname === 'mmbiz.qpic.cn' || hostname.endsWith('.qpic.cn');
        if (!proxyable || typeof ConnectionManager === 'undefined' || !ConnectionManager || typeof ConnectionManager.getServiceUrl !== 'function') {
            return source;
        }
        const serviceUrl = String(ConnectionManager.getServiceUrl() || '').replace(/\/+$/, '');
        return serviceUrl ? `${serviceUrl}/mp/proxy?url=${encodeURIComponent(parsed.href)}` : source;
    } catch (_) {
        return source;
    }
}

function officialAccountHomeUrl(biz) {
    return `https://mp.weixin.qq.com/mp/profile_ext?action=home&__biz=${encodeURIComponent(String(biz || '').trim())}&scene=124`;
}

function officialAccountInitial(nickname, biz) {
    const value = String(nickname || biz || '?').trim();
    return value ? value.slice(0, 1).toUpperCase() : '?';
}

function officialAccountStatusInfo(status) {
    const normalized = String(status || 'never').toLowerCase();
    const values = {
        queued: ['排队中', 'queued'],
        running: ['同步中', 'running'],
        completed: ['已完成', 'completed'],
        partial: ['部分完成', 'partial'],
        failed: ['失败', 'failed'],
        cancelled: ['已取消', 'cancelled'],
        never: ['未同步', 'never']
    };
    const item = values[normalized] || [normalized || '未知', 'unknown'];
    return { label: item[0], className: item[1] };
}

function officialAccountArchiveInfo(status) {
    const normalized = String(status || 'not_archived').toLowerCase();
    const values = {
        not_archived: ['未归档', 'not-archived'],
        queued: ['归档中', 'queued'],
        archived: ['已归档', 'archived'],
        partial: ['部分归档', 'partial'],
        failed: ['归档失败', 'failed']
    };
    const item = values[normalized] || [normalized || '未知', 'unknown'];
    return { label: item[0], className: item[1] };
}

function officialAccountMetricValue(metrics, key) {
    if (!metrics || metrics[key] === null || metrics[key] === undefined) return '—';
    return officialAccountCount(metrics[key]);
}

function officialAccountMetricStateInfo(status) {
    const normalized = String(status || 'pending').toLowerCase();
    const values = {
        pending: ['未采集', 'pending'],
        success: ['已采集', 'success'],
        unknown: ['上游未返回', 'unknown'],
        failed: ['采集失败', 'failed']
    };
    const item = values[normalized] || values.pending;
    return { label: item[0], className: item[1] };
}

function officialAccountRenderMetricStatePill(state) {
    const info = officialAccountMetricStateInfo(state && state.status);
    return `<span class="official-account-metric-pill ${info.className}">${info.label}</span>`;
}

function officialAccountAvatarMarkup(account, large = false) {
    const avatar = officialAccountAvatarUrl(account && account.avatar_url);
    const nickname = account && account.nickname;
    const biz = account && account.biz;
    const className = large ? 'official-account-avatar official-account-avatar-large' : 'official-account-avatar';
    return `<div class="${className}" aria-hidden="true"><span>${escapeHtml(officialAccountInitial(nickname, biz))}</span>${avatar ? `<img src="${escapeHtml(avatar)}" alt="" loading="lazy" onerror="this.remove()">` : ''}</div>`;
}

function officialAccountMediaInfo(article) {
    const item = article || {};
    const videoID = String(item.video_id || item.videoId || '').trim();
    const duration = Math.max(0, Math.floor(Number(item.duration) || 0));
    const audioFileID = Math.max(0, Math.floor(Number(item.audio_fileid || item.audioFileID) || 0));
    const playSource = String(item.play_url || item.playUrl || '').trim();
    // audio_fileid is stronger evidence than duration/play_url for audio rows;
    // video_id remains authoritative when both media fields are present.
    const hasVideo = !!videoID || (audioFileID === 0 && (duration > 0 || playSource !== ''));
    const hasAudio = !hasVideo && audioFileID > 0;
    const kind = hasVideo ? 'video' : hasAudio ? 'audio' : 'article';
    return {
        kind,
        kindLabel: kind === 'video' ? '视频' : kind === 'audio' ? '音频' : '图文',
        videoID,
        duration,
        durationLabel: officialAccountDuration(duration),
        audioFileID,
        playSource,
        hasPlayURL: !!officialAccountExternalUrl(playSource)
    };
}

function officialAccountRenderArticleMedia(article) {
    const media = officialAccountMediaInfo(article);
    const parts = [
        `<span class="official-article-type-badge ${media.kind}">${officialAccountIcon(media.kind)}${media.kindLabel}</span>`
    ];
    if (media.durationLabel) {
        parts.push(`<span class="official-article-media-detail">${officialAccountIcon('clock')}${media.durationLabel}</span>`);
    }
    if (media.videoID) {
        parts.push(`<span class="official-article-media-detail" title="video_id">${officialAccountIcon('video')}视频 ID <code>${escapeHtml(media.videoID)}</code></span>`);
    }
    if (media.audioFileID) {
        parts.push(`<span class="official-article-media-detail" title="audio_fileid">${officialAccountIcon('audio')}音频 ID ${officialAccountCount(media.audioFileID)}</span>`);
    }
    if (media.kind !== 'article') {
        const state = media.playSource ? '媒体地址已记录' : '媒体地址待采集';
        const title = media.playSource ? `已保存地址：${media.playSource}` : '需要打开原文后重新采集播放地址';
        parts.push(`<span class="official-article-media-detail official-article-media-state ${media.hasPlayURL ? 'available' : 'pending'}" title="${escapeHtml(title)}">${officialAccountIcon('link')}${state}</span>`);
    }
    return `<div class="official-article-media-meta">${parts.join('')}</div>`;
}

function officialAccountRenderStatusPill(status, prefix = '') {
    const info = officialAccountStatusInfo(status);
    return `<span class="official-account-sync-pill ${info.className}">${escapeHtml(prefix + info.label)}</span>`;
}

function renderOfficialAccount(account) {
    const nickname = String(account.nickname || '').trim() || '未命名公众号';
    const biz = String(account.biz || '').trim();
    const safeBiz = escapeHtml(biz);
    const selected = biz === officialAccountConsoleState.selectedBiz ? ' selected' : '';
    const effective = account.is_effective ? 'effective' : 'expired';
    const effectiveText = account.is_effective ? '凭证有效' : '需刷新凭证';
    return `
        <article class="official-account-item${selected}" data-select-biz="${safeBiz}" tabindex="0" role="button" aria-pressed="${selected ? 'true' : 'false'}" title="${escapeHtml(nickname)}">
            <div class="official-account-item-head">
                ${officialAccountAvatarMarkup(account)}
                <div class="official-account-info">
                    <div class="official-account-name-row">
                        <div class="official-account-name">${escapeHtml(nickname)}</div>
                        <span class="official-account-status-pill ${effective}">${effectiveText}</span>
                    </div>
                    <div class="official-account-biz-line"><span class="official-account-biz-label">biz</span><code class="official-account-biz" title="${safeBiz}">${safeBiz || '未提供'}</code></div>
                </div>
                <button class="icon-btn official-account-copy" type="button" data-copy-biz="${safeBiz}" title="复制公众号 biz" aria-label="复制公众号 biz">${officialAccountIcon('copy')}</button>
            </div>
            <div class="official-account-item-footer">
                <div class="official-account-account-meta">
                    <span><b>${officialAccountCount(account.article_count)}</b> 篇文章</span>
                    <span><b>${officialAccountCount(account.archived_count)}</b> 已归档</span>
                </div>
                ${officialAccountRenderStatusPill(account.sync_status)}
            </div>
        </article>
    `;
}

function renderOfficialAccountPager(page, totalPages, total) {
    const pager = document.getElementById('officialAccountPager');
    if (!pager) return;
    const normalizedPage = Math.max(1, Number(page) || 1);
    const pages = Math.max(0, Number(totalPages) || 0);
    pager.innerHTML = `
        <button class="icon-btn" type="button" data-account-page="${normalizedPage - 1}" ${normalizedPage <= 1 ? 'disabled' : ''} title="上一页" aria-label="上一页">${officialAccountIcon('chevronLeft')}</button>
        <span>第 ${pages ? normalizedPage : 0} / ${pages || 0} 页 · ${officialAccountCount(total)} 个账号</span>
        <button class="icon-btn" type="button" data-account-page="${normalizedPage + 1}" ${!pages || normalizedPage >= pages ? 'disabled' : ''} title="下一页" aria-label="下一页">${officialAccountIcon('chevronRight')}</button>
    `;
}

function renderOfficialAccounts(accounts, total, page, totalPages, keyword) {
    const list = officialAccountListElement();
    const count = document.getElementById('officialAccountCount');
    const status = document.getElementById('officialAccountStatus');
    if (!list) return;
    const normalizedAccounts = Array.isArray(accounts) ? accounts : [];
    const normalizedTotal = Number.isFinite(Number(total)) ? Number(total) : normalizedAccounts.length;
    const label = keyword ? `${officialAccountCount(normalizedTotal)} 个匹配账号` : `${officialAccountCount(normalizedTotal)} 个已捕获账号`;
    if (count) count.textContent = label;
    if (status) {
        status.textContent = keyword
            ? `已在本地捕获记录中查找“${keyword}”`
            : '账号来自已打开的公众号文章或主页，数据保存在本机。';
    }
    if (!normalizedAccounts.length) {
        list.innerHTML = `<div class="official-account-empty"><div class="official-account-empty-icon">${officialAccountIcon('database')}</div><p>${keyword ? '没有找到匹配的公众号' : '暂无已捕获的公众号'}</p><span>${keyword ? '请尝试其他名称或 biz' : '打开公众号文章或主页后，账号会自动出现在这里'}</span></div>`;
    } else {
        list.innerHTML = normalizedAccounts.map(renderOfficialAccount).join('');
    }
    renderOfficialAccountPager(page, totalPages, normalizedTotal);
}

function officialAccountRenderError(message) {
    const list = officialAccountListElement();
    const count = document.getElementById('officialAccountCount');
    const status = document.getElementById('officialAccountStatus');
    if (count) count.textContent = '加载失败';
    if (status) status.textContent = message || '公众号目录加载失败';
    if (list) list.innerHTML = `<div class="official-account-empty official-account-error"><div class="official-account-empty-icon">${officialAccountIcon('cancel')}</div><p>公众号目录加载失败</p><span>${escapeHtml(message || '请确认本地服务已连接后重试')}</span></div>`;
    renderOfficialAccountPager(0, 0, 0);
}

function officialAccountSetLoading() {
    const list = officialAccountListElement();
    const count = document.getElementById('officialAccountCount');
    const status = document.getElementById('officialAccountStatus');
    if (count) count.textContent = '加载中';
    if (status) status.textContent = '正在读取本地公众号目录...';
    if (list) list.innerHTML = '<div class="official-account-loading">加载中...</div>';
}

function officialAccountSelected() {
    return officialAccountConsoleState.accounts.find((account) => String(account.biz || '') === officialAccountConsoleState.selectedBiz) || null;
}

function renderOfficialAccountStats(account) {
    const stats = document.getElementById('officialAccountStats');
    if (!stats || !account) return;
    const total = officialAccountNumber(account.article_count);
    const archived = officialAccountNumber(account.archived_count);
    const ratio = total > 0 ? `${Math.round((archived / total) * 100)}%` : '—';
    stats.innerHTML = `
        <div class="official-account-stat"><span>文章总数</span><strong>${officialAccountCount(account.article_count)}</strong><small>持久化目录</small></div>
        <div class="official-account-stat"><span>已归档</span><strong>${officialAccountCount(account.archived_count)}</strong><small>本地文件索引</small></div>
        <div class="official-account-stat"><span>归档覆盖率</span><strong>${ratio}</strong><small>按文章计</small></div>
        <div class="official-account-stat"><span>最近同步</span><strong class="official-account-stat-date">${escapeHtml(officialAccountDate(account.last_sync_at))}</strong><small>${escapeHtml(officialAccountDate(account.update_time))}更新</small></div>
    `;
}

function officialAccountMetricRunProgress(run) {
    if (!run) return 0;
    const total = Math.max(0, officialAccountNumber(run.total));
    const attempted = Math.max(0, officialAccountNumber(run.attempted));
    if (total === 0) return run.status === 'completed' ? 100 : 0;
    return Math.min(100, Math.max(0, Math.round((attempted / total) * 100)));
}

function renderOfficialAccountMetricSync(run) {
    const panel = document.getElementById('officialAccountMetricSyncPanel');
    const account = officialAccountSelected();
    if (!panel || !account) return;
    const active = officialAccountSyncIsActive(run);
    const status = run && run.status ? run.status : 'never';
    const info = officialAccountStatusInfo(status);
    const total = officialAccountNumber(run && run.total);
    const attempted = officialAccountNumber(run && run.attempted);
    const remaining = Math.max(0, total - attempted);
    const percent = officialAccountMetricRunProgress(run);
    const mode = run && run.force ? '刷新全部指标' : '采集待采集指标';
    const error = run && run.error ? `<div class="official-account-sync-error">${escapeHtml(run.error)}</div>` : '';
    const canResume = run && !active && ['partial', 'failed', 'cancelled'].includes(run.status) && account.is_effective;
    panel.innerHTML = `
        <div class="official-account-sync-title"><span>互动指标采集</span><span class="official-account-metric-run-label">${escapeHtml(mode)}</span>${officialAccountRenderStatusPill(status)}</div>
        <div class="official-account-sync-row"><span>${run ? `总数 ${officialAccountCount(total)} · 已尝试 ${officialAccountCount(attempted)} · 剩余 ${officialAccountCount(remaining)}` : '尚未采集互动指标'}</span><strong>${run ? `${percent}%` : '—'}</strong></div>
        <div class="official-account-progress" aria-hidden="true"><span style="width:${percent}%"></span></div>
        <div class="official-account-sync-metrics"><span>已采集 <b>${officialAccountCount(run && run.stored)}</b></span><span>未返回 <b>${officialAccountCount(run && run.unknown)}</b></span><span>失败 <b>${officialAccountCount(run && run.failed)}</b></span></div>
        <div class="official-account-sync-meta"><span>${run ? `每篇完成后保存 · ${escapeHtml(officialAccountDate(run.started_at))}开始` : '指标缺失显示为“—”，不会伪造为 0'}</span><span>${run ? escapeHtml(officialAccountDate(run.finished_at)) : ''}</span></div>
        ${error}
        <div class="official-account-metric-sync-actions">
            ${active && run.id ? `<button class="btn btn-danger btn-small" type="button" onclick="cancelOfficialAccountMetricSync('${escapeHtml(run.id)}')">${officialAccountIcon('cancel')}取消采集</button>` : ''}
            ${canResume ? `<button class="btn btn-secondary btn-small" type="button" onclick="resumeOfficialAccountMetricSync('${escapeHtml(run.id)}')">${officialAccountIcon('recent')}继续</button>` : ''}
        </div>
    `;
    const statusText = document.getElementById('officialAccountMetricSyncStatusText');
    if (statusText) statusText.textContent = `${info.label}${run && run.error ? ' · ' + run.error : ''}`;
}

function renderOfficialAccountSync(run) {
    const panel = document.getElementById('officialAccountSyncPanel');
    const account = officialAccountSelected();
    if (!panel || !account) return;
    const active = run && (run.status === 'queued' || run.status === 'running' || run.status === 'cancelling');
    const info = officialAccountStatusInfo(run && run.status ? run.status : account.sync_status);
    const pages = officialAccountNumber(run && run.page_count);
    const fetched = officialAccountNumber(run && run.fetched);
    const percent = run && run.can_continue && pages > 0 ? Math.min(96, Math.max(4, pages * 4)) : (run && run.status === 'completed' ? 100 : 0);
    const error = run && run.error ? `<div class="official-account-sync-error">${escapeHtml(run.error)}</div>` : '';
    panel.innerHTML = `
        <div class="official-account-sync-title"><span>同步状态</span>${officialAccountRenderStatusPill(run && run.status ? run.status : account.sync_status)}</div>
        <div class="official-account-sync-row"><span>${run ? `${escapeHtml(run.mode === 'recent' ? '最近一页' : '全部历史')} · ${pages} 页 · ${fetched} 条` : '尚未运行历史同步'}</span><span>${run && run.status === 'completed' ? '100%' : (active ? '进行中' : '')}</span></div>
        <div class="official-account-progress" aria-hidden="true"><span style="width:${percent}%"></span></div>
        <div class="official-account-sync-meta"><span>${run ? `新增 ${officialAccountCount(run.inserted)} · 更新 ${officialAccountCount(run.updated)}` : '每页提交后保存进度，可中断后续跑'}</span><span>${run ? escapeHtml(officialAccountDate(run.finished_at || run.started_at)) : ''}</span></div>
        ${error}
        ${active && run.id ? `<button class="btn btn-danger btn-small" type="button" onclick="cancelOfficialAccountSync('${escapeHtml(run.id)}')">${officialAccountIcon('cancel')}取消同步</button>` : ''}
    `;
    const statusText = document.getElementById('officialAccountSyncStatusText');
    if (statusText) statusText.textContent = `${info.label}${run && run.error ? ' · ' + run.error : ''}`;
}

function renderOfficialAccountDetail(account) {
    const detail = document.getElementById('officialAccountDetail');
    if (!detail) return;
    if (!account) {
        detail.innerHTML = `<div class="official-account-empty official-account-detail-empty"><div class="official-account-empty-icon">${officialAccountIcon('article')}</div><p>选择一个公众号</p><span>这里会显示文章目录、指标快照和同步状态</span></div>`;
        return;
    }
    const nickname = String(account.nickname || '').trim() || '未命名公众号';
    const biz = String(account.biz || '').trim();
    const safeBiz = escapeHtml(biz);
    const homeUrl = escapeHtml(officialAccountHomeUrl(biz));
    const disabled = account.is_effective ? '' : ' disabled';
    detail.innerHTML = `
        <div class="official-account-detail-head">
            <div class="official-account-detail-identity">
                ${officialAccountAvatarMarkup(account, true)}
                <div class="official-account-detail-copy"><div class="official-account-kicker">公众号资料库</div><h2>${escapeHtml(nickname)}</h2><div class="official-account-detail-biz-row"><span class="official-account-biz-label">biz</span><code class="official-account-detail-biz" title="${safeBiz}">${safeBiz || '未提供'}</code><span class="official-account-status-pill ${account.is_effective ? 'effective' : 'expired'}">${account.is_effective ? '凭证有效' : '需刷新凭证'}</span></div></div>
            </div>
            <div class="official-account-detail-actions">
                <button class="btn btn-primary btn-small" type="button" onclick="startOfficialAccountSync('history')"${disabled} title="同步当前账号可访问的全部历史文章">${officialAccountIcon('history')}同步全部历史</button>
                <button class="btn btn-secondary btn-small" type="button" onclick="startOfficialAccountSync('recent')"${disabled} title="只同步当前账号最近一页文章">${officialAccountIcon('recent')}同步最近一页</button>
                <button class="btn btn-secondary btn-small" type="button" onclick="startOfficialAccountMetricSync(false)"${disabled} title="采集尚未成功或可重试的互动指标">${officialAccountIcon('check')}采集待采集指标</button>
                <button class="btn btn-secondary btn-small" type="button" onclick="startOfficialAccountMetricSync(true)"${disabled} title="重新采集当前账号全部文章的互动指标">${officialAccountIcon('refresh')}刷新全部指标</button>
                <button class="icon-btn" type="button" onclick="refreshOfficialAccountDetail()" title="刷新账号和文章" aria-label="刷新账号和文章">${officialAccountIcon('refresh')}</button>
                <a class="icon-btn" href="${homeUrl}" target="_blank" rel="noopener noreferrer" title="打开公众号主页" aria-label="打开公众号主页">${officialAccountIcon('external')}</a>
            </div>
        </div>
        <div class="official-account-stats" id="officialAccountStats"></div>
        <div class="official-account-sync-panel" id="officialAccountSyncPanel"></div>
        <div class="official-account-sync-panel official-account-metric-sync-panel" id="officialAccountMetricSyncPanel"></div>
        <form class="official-article-filter" onsubmit="searchOfficialArticles(event)">
            <div class="official-article-filter-title"><span>${officialAccountIcon('article')}文章目录</span><span id="officialArticleCount">等待加载</span></div>
            <div class="official-article-filter-controls">
                <label class="official-article-search"><span class="sr-only">搜索文章</span><input type="search" id="officialArticleSearchInput" placeholder="搜索标题、摘要或作者" value="${escapeHtml(officialAccountConsoleState.articleKeyword)}"></label>
                <select id="officialArticleArchiveStatus" aria-label="归档状态"><option value="">全部归档状态</option><option value="not_archived">未归档</option><option value="archived">已归档</option><option value="partial">部分归档</option><option value="failed">归档失败</option></select>
                <select id="officialArticleSort" aria-label="排序方式"><option value="publish_time">发布时间</option><option value="last_seen_at">最近发现</option><option value="updated_at">目录更新时间</option><option value="title">标题</option></select>
                <button class="btn btn-secondary btn-small" type="submit">${officialAccountIcon('search')}筛选</button>
                <button class="icon-btn" type="button" onclick="toggleOfficialArticleOrder()" title="切换排序方向" aria-label="切换排序方向">${officialAccountConsoleState.descending ? '↓' : '↑'}</button>
            </div>
        </form>
        <div class="official-article-table-wrap"><div id="officialArticleTable" class="official-article-table"></div></div>
        <div class="official-article-pager" id="officialArticlePager"></div>
    `;
    renderOfficialAccountStats(account);
    renderOfficialAccountSync(officialAccountConsoleState.syncRun);
    renderOfficialAccountMetricSync(officialAccountConsoleState.metricSyncRun);
    const archiveSelect = document.getElementById('officialArticleArchiveStatus');
    const sortSelect = document.getElementById('officialArticleSort');
    if (archiveSelect) archiveSelect.value = officialAccountConsoleState.archiveStatus;
    if (sortSelect) sortSelect.value = officialAccountConsoleState.articleSort;
}

function renderOfficialArticlePager(page, totalPages, total) {
    const pager = document.getElementById('officialArticlePager');
    if (!pager) return;
    const normalizedPage = Math.max(1, Number(page) || 1);
    const pages = Math.max(0, Number(totalPages) || 0);
    pager.innerHTML = `
        <button class="icon-btn" type="button" data-article-page="${normalizedPage - 1}" ${normalizedPage <= 1 ? 'disabled' : ''} title="上一页" aria-label="上一页">${officialAccountIcon('chevronLeft')}</button>
        <span>第 ${pages ? normalizedPage : 0} / ${pages || 0} 页 · ${officialAccountCount(total)} 篇文章</span>
        <button class="icon-btn" type="button" data-article-page="${normalizedPage + 1}" ${!pages || normalizedPage >= pages ? 'disabled' : ''} title="下一页" aria-label="下一页">${officialAccountIcon('chevronRight')}</button>
    `;
}

function renderOfficialArticleMetrics(article) {
    const metrics = article && article.metrics;
    const state = article && article.metric_state;
    const observedAt = metrics && metrics.observed_at ? metrics.observed_at : state && state.last_observed_at;
    return `
        <div class="official-article-metric-status">${officialAccountRenderMetricStatePill(state)}<span>${escapeHtml(officialAccountDate(observedAt, '尚无快照'))}</span></div>
        <div class="official-article-metrics">
            <span><b>${officialAccountMetricValue(metrics, 'view_count')}</b><small>阅读</small></span>
            <span><b>${officialAccountMetricValue(metrics, 'like_count')}</b><small>点赞</small></span>
            <span><b>${officialAccountMetricValue(metrics, 'comment_count')}</b><small>评论</small></span>
            <span><b>${officialAccountMetricValue(metrics, 'share_count')}</b><small>转发</small></span>
            <span><b>${officialAccountMetricValue(metrics, 'collect_count')}</b><small>收藏</small></span>
            <span><b>${officialAccountMetricValue(metrics, 'reward_count')}</b><small>赞赏</small></span>
        </div>
    `;
}

function renderOfficialArticle(article) {
    const title = String(article.title || '').trim() || '未命名文章';
    const url = officialAccountExternalUrl(article.content_url || article.source_url);
    const archive = officialAccountArchiveInfo(article.archive_status);
    const key = escapeHtml(article.key || '');
    const sourceDeleted = article.source_deleted ? '<span class="official-article-deleted">源站已不可见</span>' : '';
    const action = url
        ? `<div class="official-article-actions"><a class="icon-btn" href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer" title="打开文章" aria-label="打开文章">${officialAccountIcon('external')}</a><button class="icon-btn" type="button" data-copy-article-url="${escapeHtml(url)}" title="复制文章链接" aria-label="复制文章链接">${officialAccountIcon('copy')}</button></div>`
        : '<span class="official-article-no-url">无链接</span>';
    return `
        <article class="official-article-row" data-article-key="${key}">
            <div class="official-article-primary">
                <div class="official-article-title-line">${url ? `<a href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(title)}</a>` : `<span>${escapeHtml(title)}</span>`}${sourceDeleted}</div>
                ${officialAccountRenderArticleMedia(article)}
                <p>${escapeHtml(article.digest || '暂无摘要')}</p>
                <div class="official-article-meta"><span>${escapeHtml(article.author || '未知作者')}</span><span>${officialAccountDateOnly(article.publish_time)}</span><span>发现于 ${escapeHtml(officialAccountDate(article.last_seen_at))}</span></div>
            </div>
            <div class="official-article-metric-cell">${renderOfficialArticleMetrics(article)}</div>
            <div class="official-article-archive-cell"><span class="official-account-archive-pill ${archive.className}">${archive.label}</span><small>${article.assets && article.assets.length ? `${article.assets.length} 个资源` : ''}</small></div>
            <div class="official-article-action-cell">${action}</div>
        </article>
    `;
}

function renderOfficialArticles(items, total, page, totalPages) {
    const table = document.getElementById('officialArticleTable');
    const count = document.getElementById('officialArticleCount');
    if (!table) return;
    const normalized = Array.isArray(items) ? items : [];
    if (count) count.textContent = `${officialAccountCount(total)} 篇 · 第 ${page || 0}/${totalPages || 0} 页`;
    if (!normalized.length) {
        table.innerHTML = `<div class="official-account-empty"><div class="official-account-empty-icon">${officialAccountIcon('article')}</div><p>没有匹配的文章</p><span>同步历史文章后，目录会在这里分页显示</span></div>`;
    } else {
        table.innerHTML = `<div class="official-article-header"><span>文章</span><span>互动指标</span><span>归档</span><span></span></div>${normalized.map(renderOfficialArticle).join('')}`;
    }
    renderOfficialArticlePager(page, totalPages, total);
}

async function loadOfficialAccounts() {
    const input = document.getElementById('officialAccountSearchInput');
    const keyword = input ? String(input.value || '').trim() : officialAccountConsoleState.accountKeyword;
    const requestSequence = ++officialAccountConsoleState.accountRequestSequence;
    officialAccountConsoleState.accountKeyword = keyword;
    officialAccountSetLoading();
    if (ConnectionManager.getStatus() !== 'connected') {
        officialAccountRenderError('请先连接到本地服务');
        return;
    }
    try {
        const result = await ApiClient.getOfficialAccounts({
            keyword,
            page: officialAccountConsoleState.accountPage,
            page_size: officialAccountConsoleState.accountPageSize
        });
        if (requestSequence !== officialAccountConsoleState.accountRequestSequence) return;
        const data = officialAccountData(result) || {};
        const accounts = officialAccountItems(data);
        officialAccountConsoleState.accounts = accounts;
        officialAccountConsoleState.accountTotal = Number(data.total || 0);
        officialAccountConsoleState.accountTotalPages = Number(data.total_pages || 0);
        const current = officialAccountSelected();
        if (!current) {
            officialAccountConsoleState.selectedBiz = accounts.length ? String(accounts[0].biz || '') : '';
        }
        renderOfficialAccounts(accounts, data.total, data.page, data.total_pages, keyword);
        renderOfficialAccountDetail(officialAccountSelected());
        if (officialAccountSelected()) {
            await Promise.all([loadOfficialArticles(), loadOfficialSyncStatus(), loadOfficialMetricSyncStatus()]);
        }
    } catch (error) {
        if (requestSequence !== officialAccountConsoleState.accountRequestSequence) return;
        officialAccountRenderError(error && error.message ? error.message : '公众号目录加载失败');
        renderOfficialAccountDetail(null);
    }
}

function handleOfficialAccountSearchInput() {
    clearTimeout(officialAccountConsoleState.searchTimer);
    officialAccountConsoleState.searchTimer = setTimeout(() => {
        officialAccountConsoleState.accountPage = 1;
        loadOfficialAccounts();
    }, 250);
}

function searchOfficialAccounts(event) {
    if (event) event.preventDefault();
    clearTimeout(officialAccountConsoleState.searchTimer);
    officialAccountConsoleState.accountPage = 1;
    loadOfficialAccounts();
}

function resetOfficialAccountSearch() {
    clearTimeout(officialAccountConsoleState.searchTimer);
    const input = document.getElementById('officialAccountSearchInput');
    if (input) input.value = '';
    officialAccountConsoleState.accountPage = 1;
    loadOfficialAccounts();
}

function selectOfficialAccount(biz) {
    const value = String(biz || '').trim();
    if (!value || value === officialAccountConsoleState.selectedBiz) return;
    officialAccountConsoleState.selectedBiz = value;
    officialAccountConsoleState.articlePage = 1;
    officialAccountConsoleState.articleKeyword = '';
    officialAccountConsoleState.archiveStatus = '';
    officialAccountConsoleState.syncRun = null;
    officialAccountConsoleState.metricSyncRun = null;
    clearOfficialAccountSyncPoll();
    clearOfficialAccountMetricSyncPoll();
    renderOfficialAccounts(officialAccountConsoleState.accounts, officialAccountConsoleState.accountTotal, officialAccountConsoleState.accountPage, officialAccountConsoleState.accountTotalPages, officialAccountConsoleState.accountKeyword);
    renderOfficialAccountDetail(officialAccountSelected());
    if (officialAccountSelected()) {
        loadOfficialArticles();
        loadOfficialSyncStatus();
        loadOfficialMetricSyncStatus();
    }
}

async function refreshOfficialAccountDetail() {
    const account = officialAccountSelected();
    if (!account) return;
    await loadOfficialAccounts();
}

function readOfficialArticleFilters() {
    const keyword = document.getElementById('officialArticleSearchInput');
    const archive = document.getElementById('officialArticleArchiveStatus');
    const sort = document.getElementById('officialArticleSort');
    officialAccountConsoleState.articleKeyword = keyword ? String(keyword.value || '').trim() : '';
    officialAccountConsoleState.archiveStatus = archive ? String(archive.value || '') : '';
    officialAccountConsoleState.articleSort = sort ? String(sort.value || 'publish_time') : 'publish_time';
}

async function loadOfficialArticles() {
    const table = document.getElementById('officialArticleTable');
    const account = officialAccountSelected();
    if (!account) return;
    const requestSequence = ++officialAccountConsoleState.articleRequestSequence;
    if (table) table.innerHTML = '<div class="official-account-loading">正在读取文章目录...</div>';
    if (ConnectionManager.getStatus() !== 'connected') {
        if (table) table.innerHTML = '<div class="official-account-empty official-account-error"><p>请先连接到本地服务</p></div>';
        return;
    }
    try {
        const result = await ApiClient.getOfficialArticles({
            biz: account.biz,
            keyword: officialAccountConsoleState.articleKeyword,
            archive_status: officialAccountConsoleState.archiveStatus,
            sort: officialAccountConsoleState.articleSort,
            descending: officialAccountConsoleState.descending,
            page: officialAccountConsoleState.articlePage,
            page_size: officialAccountConsoleState.articlePageSize
        });
        if (requestSequence !== officialAccountConsoleState.articleRequestSequence) return;
        const data = officialAccountData(result) || {};
        const totalPages = Number(data.total_pages || 0);
        if (totalPages > 0 && officialAccountConsoleState.articlePage > totalPages) {
            officialAccountConsoleState.articlePage = totalPages;
            return loadOfficialArticles();
        }
        renderOfficialArticles(officialAccountItems(data), data.total, data.page, totalPages);
    } catch (error) {
        if (requestSequence !== officialAccountConsoleState.articleRequestSequence) return;
        if (table) table.innerHTML = `<div class="official-account-empty official-account-error"><div class="official-account-empty-icon">${officialAccountIcon('cancel')}</div><p>文章目录加载失败</p><span>${escapeHtml(error && error.message ? error.message : '请稍后重试')}</span></div>`;
        renderOfficialArticlePager(0, 0, 0);
    }
}

function searchOfficialArticles(event) {
    if (event) event.preventDefault();
    readOfficialArticleFilters();
    officialAccountConsoleState.articlePage = 1;
    loadOfficialArticles();
}

function resetOfficialArticleFilters() {
    officialAccountConsoleState.articleKeyword = '';
    officialAccountConsoleState.archiveStatus = '';
    officialAccountConsoleState.articleSort = 'publish_time';
    officialAccountConsoleState.articlePage = 1;
    renderOfficialAccountDetail(officialAccountSelected());
    loadOfficialArticles();
}

function toggleOfficialArticleOrder() {
    officialAccountConsoleState.descending = !officialAccountConsoleState.descending;
    loadOfficialArticles();
    const orderButton = document.querySelector('#page-official-accounts .official-article-filter .icon-btn[aria-label="切换排序方向"]');
    if (orderButton) orderButton.textContent = officialAccountConsoleState.descending ? '↓' : '↑';
}

function setOfficialAccountPage(page) {
    const value = Math.max(1, Number(page) || 1);
    if (value === officialAccountConsoleState.accountPage) return;
    officialAccountConsoleState.accountPage = value;
    loadOfficialAccounts();
}

function setOfficialArticlePage(page) {
    const value = Math.max(1, Number(page) || 1);
    if (value === officialAccountConsoleState.articlePage) return;
    officialAccountConsoleState.articlePage = value;
    loadOfficialArticles();
}

function clearOfficialAccountSyncPoll() {
    if (officialAccountConsoleState.syncPollTimer) {
        clearTimeout(officialAccountConsoleState.syncPollTimer);
        officialAccountConsoleState.syncPollTimer = null;
    }
}

function clearOfficialAccountMetricSyncPoll() {
    if (officialAccountConsoleState.metricSyncPollTimer) {
        clearTimeout(officialAccountConsoleState.metricSyncPollTimer);
        officialAccountConsoleState.metricSyncPollTimer = null;
    }
}

function officialAccountSyncIsActive(run) {
    return !!(run && (run.status === 'queued' || run.status === 'running' || run.status === 'cancelling'));
}

async function loadOfficialSyncStatus() {
    const account = officialAccountSelected();
    if (!account || ConnectionManager.getStatus() !== 'connected') return;
    try {
        const result = await ApiClient.getOfficialSyncStatus(account.biz, officialAccountConsoleState.syncMode);
        const run = officialAccountData(result);
        officialAccountConsoleState.syncRun = run || null;
        renderOfficialAccountStats(account);
        renderOfficialAccountSync(run || null);
        if (officialAccountSyncIsActive(run)) pollOfficialAccountSync(run.id);
    } catch (error) {
        const panel = document.getElementById('officialAccountSyncPanel');
        if (panel) panel.innerHTML = `<div class="official-account-sync-error">同步状态读取失败：${escapeHtml(error && error.message ? error.message : '请稍后重试')}</div>`;
    }
}

async function loadOfficialMetricSyncStatus() {
    const account = officialAccountSelected();
    if (!account || ConnectionManager.getStatus() !== 'connected') return;
    const biz = String(account.biz || '').trim();
    const requestSequence = ++officialAccountConsoleState.metricSyncRequestSequence;
    try {
        const result = await ApiClient.getOfficialMetricSyncStatus(biz);
        if (requestSequence !== officialAccountConsoleState.metricSyncRequestSequence || officialAccountConsoleState.selectedBiz !== biz) return;
        const run = officialAccountData(result);
        officialAccountConsoleState.metricSyncRun = run || null;
        renderOfficialAccountMetricSync(run || null);
        if (officialAccountSyncIsActive(run)) pollOfficialAccountMetricSync(run.id);
    } catch (error) {
        const panel = document.getElementById('officialAccountMetricSyncPanel');
        if (panel) panel.innerHTML = `<div class="official-account-sync-error">指标状态读取失败：${escapeHtml(error && error.message ? error.message : '请稍后重试')}</div>`;
    }
}

async function pollOfficialAccountSync(runID) {
    clearOfficialAccountSyncPoll();
    const tick = async () => {
        const root = officialAccountRoot();
        if (!root || !root.classList.contains('active') || officialAccountConsoleState.selectedBiz === '') return;
        try {
            const result = await ApiClient.getOfficialSyncStatus(officialAccountConsoleState.selectedBiz, officialAccountConsoleState.syncMode);
            const run = officialAccountData(result);
            if (!run || (runID && run.id !== runID)) return;
            officialAccountConsoleState.syncRun = run;
            renderOfficialAccountSync(run);
            if (officialAccountSyncIsActive(run)) {
                officialAccountConsoleState.syncPollTimer = setTimeout(tick, 1200);
            } else {
                await loadOfficialAccounts();
            }
        } catch (_) {
            officialAccountConsoleState.syncPollTimer = setTimeout(tick, 2500);
        }
    };
    await tick();
}

async function pollOfficialAccountMetricSync(runID) {
    clearOfficialAccountMetricSyncPoll();
    const biz = officialAccountConsoleState.selectedBiz;
    const tick = async () => {
        const root = officialAccountRoot();
        if (!root || !root.classList.contains('active') || officialAccountConsoleState.selectedBiz !== biz) return;
        try {
            const result = await ApiClient.getOfficialMetricSyncStatus(biz);
            if (officialAccountConsoleState.selectedBiz !== biz) return;
            const run = officialAccountData(result);
            if (!run || (runID && run.id !== runID)) return;
            officialAccountConsoleState.metricSyncRun = run;
            renderOfficialAccountMetricSync(run);
            if (officialAccountSyncIsActive(run)) {
                officialAccountConsoleState.metricSyncPollTimer = setTimeout(tick, 1200);
            } else {
                await loadOfficialArticles();
            }
        } catch (_) {
            if (officialAccountConsoleState.selectedBiz === biz) {
                officialAccountConsoleState.metricSyncPollTimer = setTimeout(tick, 2500);
            }
        }
    };
    await tick();
}

async function startOfficialAccountSync(mode) {
    const account = officialAccountSelected();
    if (!account) {
        showMessage('请先选择公众号', 'warning');
        return;
    }
    if (!account.is_effective) {
        showMessage('请先重新打开该公众号页面，刷新短期凭证', 'warning');
        return;
    }
    officialAccountConsoleState.syncMode = mode === 'recent' ? 'recent' : 'history';
    try {
        const result = await ApiClient.startOfficialSync(account.biz, officialAccountConsoleState.syncMode, true);
        const run = officialAccountData(result);
        officialAccountConsoleState.syncRun = run;
        renderOfficialAccountSync(run);
        showMessage(officialAccountConsoleState.syncMode === 'history' ? '已开始同步全部历史文章' : '已开始同步最近一页文章', 'success');
        pollOfficialAccountSync(run && run.id);
    } catch (error) {
        showMessage(error && error.message ? error.message : '启动同步失败', 'error');
        loadOfficialSyncStatus();
    }
}

async function cancelOfficialAccountSync(id) {
    const runID = String(id || '').trim();
    if (!runID) return;
    try {
        await ApiClient.cancelOfficialSync(runID);
        showMessage('已发出取消请求，正在等待当前请求结束', 'info');
        pollOfficialAccountSync(runID);
    } catch (error) {
        showMessage(error && error.message ? error.message : '取消同步失败', 'error');
    }
}

async function startOfficialAccountMetricSync(force = false) {
    const account = officialAccountSelected();
    if (!account) {
        showMessage('请先选择公众号', 'warning');
        return;
    }
    if (!account.is_effective) {
        showMessage('请先重新打开该公众号页面，刷新短期凭证', 'warning');
        return;
    }
    if (officialAccountConsoleState.metricSyncStartPromise) {
        showMessage('指标采集请求正在启动，请稍候', 'info');
        return;
    }
    clearOfficialAccountMetricSyncPoll();
    let startRequest = null;
    try {
        startRequest = ApiClient.startOfficialMetricSync(account.biz, !!force, false);
        officialAccountConsoleState.metricSyncStartPromise = startRequest;
        const result = await startRequest;
        const run = officialAccountData(result);
        officialAccountConsoleState.metricSyncRun = run;
        renderOfficialAccountMetricSync(run);
        showMessage(force ? '已开始刷新全部互动指标' : '已开始采集待采集/失败指标', 'success');
        pollOfficialAccountMetricSync(run && run.id);
    } catch (error) {
        showMessage(error && error.message ? error.message : '启动指标采集失败', 'error');
        loadOfficialMetricSyncStatus();
    } finally {
        if (officialAccountConsoleState.metricSyncStartPromise === startRequest) {
            officialAccountConsoleState.metricSyncStartPromise = null;
        }
    }
}

async function resumeOfficialAccountMetricSync(id) {
    const account = officialAccountSelected();
    const runID = String(id || '').trim();
    if (!account || !runID) return;
    if (!account.is_effective) {
        showMessage('请先重新打开该公众号页面，刷新短期凭证', 'warning');
        return;
    }
    const previous = officialAccountConsoleState.metricSyncRun;
    const force = !!(previous && previous.id === runID && previous.force);
    if (officialAccountConsoleState.metricSyncStartPromise) {
        showMessage('指标采集请求正在启动，请稍候', 'info');
        return;
    }
    let startRequest = null;
    try {
        startRequest = ApiClient.startOfficialMetricSync(account.biz, force, true);
        officialAccountConsoleState.metricSyncStartPromise = startRequest;
        const result = await startRequest;
        const run = officialAccountData(result);
        officialAccountConsoleState.metricSyncRun = run;
        renderOfficialAccountMetricSync(run);
        showMessage('已继续指标采集', 'success');
        pollOfficialAccountMetricSync(run && run.id);
    } catch (error) {
        showMessage(error && error.message ? error.message : '继续指标采集失败', 'error');
        loadOfficialMetricSyncStatus();
    } finally {
        if (officialAccountConsoleState.metricSyncStartPromise === startRequest) {
            officialAccountConsoleState.metricSyncStartPromise = null;
        }
    }
}

async function cancelOfficialAccountMetricSync(id) {
    const runID = String(id || '').trim();
    if (!runID) return;
    try {
        await ApiClient.cancelOfficialMetricSync(runID);
        showMessage('已发出取消请求，正在等待当前请求结束', 'info');
        pollOfficialAccountMetricSync(runID);
    } catch (error) {
        showMessage(error && error.message ? error.message : '取消指标采集失败', 'error');
    }
}

async function copyOfficialText(value, successMessage) {
    const text = String(value || '').trim();
    if (!text) {
        showMessage('没有可复制的内容', 'warning');
        return;
    }
    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(text);
        } else {
            const textarea = document.createElement('textarea');
            textarea.value = text;
            textarea.style.position = 'fixed';
            textarea.style.opacity = '0';
            document.body.appendChild(textarea);
            textarea.select();
            document.execCommand('copy');
            textarea.remove();
        }
        showMessage(successMessage, 'success');
    } catch (_) {
        showMessage('复制失败', 'error');
    }
}

function copyOfficialAccountBiz(biz) {
    return copyOfficialText(biz, '公众号 biz 已复制');
}

function saveOfficialCatalogFile(result) {
    if (!result || !result.blob) throw new Error('导出文件为空');
    const url = URL.createObjectURL(result.blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = result.filename || 'wx_channels_official_account_catalog.json';
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

async function exportOfficialCatalog(format) {
    try {
        const result = await ApiClient.downloadOfficialCatalog(format === 'zip' ? 'zip' : 'json');
        saveOfficialCatalogFile(result);
        showMessage(`已导出公众号目录（${format === 'zip' ? 'ZIP' : 'JSON'}）`, 'success');
    } catch (error) {
        showMessage(error && error.message ? error.message : '导出公众号目录失败', 'error');
    }
}

function triggerOfficialCatalogImport() {
    const input = document.getElementById('officialCatalogImportInput');
    if (input) input.click();
}

function officialAccountImportSummary(summary) {
    if (!summary) return '导入完成';
    const parts = [
        `账号新增 ${officialAccountCount(summary.accounts_added)}`,
        `文章新增 ${officialAccountCount(summary.articles_added)}`,
        `资源新增 ${officialAccountCount(summary.assets_added)}`,
        `指标新增 ${officialAccountCount(summary.metrics_added)}`
    ];
    if (summary.dry_run) parts.unshift('仅预检，未写入');
    if (summary.conflicts) parts.push(`冲突 ${officialAccountCount(summary.conflicts)}`);
    return parts.join(' · ');
}

async function importOfficialCatalogFile(input) {
    const file = input && input.files ? input.files[0] : null;
    if (!file) return;
    const policy = document.getElementById('officialCatalogConflictPolicy');
    const dryRun = document.getElementById('officialCatalogDryRun');
    try {
        const result = await ApiClient.importOfficialCatalog(file, {
            conflictPolicy: policy ? policy.value : 'merge',
            dryRun: !!(dryRun && dryRun.checked)
        });
        const summary = officialAccountData(result);
        showMessage(officialAccountImportSummary(summary), 'success');
        if (!(summary && summary.dry_run)) await loadOfficialAccounts();
    } catch (error) {
        showMessage(error && error.message ? error.message : '导入公众号目录失败', 'error');
    } finally {
        input.value = '';
    }
}

function bindOfficialAccountConsole() {
    const root = officialAccountRoot();
    if (!root || root.dataset.actionsBound === 'true') return;
    root.dataset.actionsBound = 'true';
    root.addEventListener('click', (event) => {
        const target = event.target && event.target.closest ? event.target.closest('[data-select-biz]') : null;
        if (target && !event.target.closest('[data-copy-biz]')) {
            selectOfficialAccount(target.dataset.selectBiz || '');
            return;
        }
        const copyBiz = event.target && event.target.closest ? event.target.closest('[data-copy-biz]') : null;
        if (copyBiz) {
            event.stopPropagation();
            copyOfficialAccountBiz(copyBiz.dataset.copyBiz || '');
            return;
        }
        const accountPage = event.target && event.target.closest ? event.target.closest('[data-account-page]') : null;
        if (accountPage && !accountPage.disabled) {
            setOfficialAccountPage(accountPage.dataset.accountPage);
            return;
        }
        const articlePage = event.target && event.target.closest ? event.target.closest('[data-article-page]') : null;
        if (articlePage && !articlePage.disabled) {
            setOfficialArticlePage(articlePage.dataset.articlePage);
            return;
        }
        const copyArticle = event.target && event.target.closest ? event.target.closest('[data-copy-article-url]') : null;
        if (copyArticle) copyOfficialText(copyArticle.dataset.copyArticleUrl || '', '文章链接已复制');
    });
    root.addEventListener('keydown', (event) => {
        const target = event.target && event.target.closest ? event.target.closest('[data-select-biz]') : null;
        if (target && (event.key === 'Enter' || event.key === ' ')) {
            event.preventDefault();
            selectOfficialAccount(target.dataset.selectBiz || '');
        }
    });
    const importInput = document.getElementById('officialCatalogImportInput');
    if (importInput) importInput.addEventListener('change', () => importOfficialCatalogFile(importInput));
}

bindOfficialAccountConsole();
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', bindOfficialAccountConsole);
}
