/**
 * Standalone live-page capture and recording UI.
 *
 * This script intentionally has no dependency on the Home/Profile injection
 * runtime. The response interceptor supplies live API results through
 * window.__wx_channels_capture_live_data__ before the page bundle runs.
 */
(function () {
  'use strict';

  if (window.__wx_channels_live_page_initialized__) return;
  window.__wx_channels_live_page_initialized__ = true;

  // The live page owns a separate application runtime and may replace the
  // global fetch later while its bundles initialize. Keep the browser's
  // original fetch for local API calls so a page-level wrapper cannot drop
  // the JSON body or redirect the request to an upstream endpoint.
  var nativeFetch = typeof window.fetch === 'function' ? window.fetch.bind(window) : null;

  var state = window.__wx_channels_live_runtime__ = {
    profile: null,
    live: null,
    streamUrl: '',
    button: null,
    statusNode: null,
    statusText: '',
    downloading: false,
    downloadId: '',
    savePath: ''
  };

  function apiFetch(url, options) {
    var send = nativeFetch || (typeof window.fetch === 'function' ? window.fetch.bind(window) : null);
    if (!send) return Promise.reject(new Error('当前页面不支持网络请求'));
    return send(url, options);
  }

  function log(message) {
    apiFetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ msg: '[直播] ' + message })
    }).catch(function (error) {
      console.warn('[直播] 日志上报失败:', error);
    });
    console.log('[直播]', message);
  }

  function asObject(value) {
    return value && typeof value === 'object' ? value : {};
  }

  function firstHttp() {
    for (var i = 0; i < arguments.length; i++) {
      var value = String(arguments[i] || '').trim();
      if (/^https?:\/\//i.test(value)) return value;
    }
    return '';
  }

  function unwrap(value) {
    var result = asObject(value);
    if (result.data && typeof result.data === 'object') return result.data;
    return result;
  }

  function extractStream(value) {
    var data = unwrap(value);
    var sdk = asObject(data.liveSdkInfo || data.liveSDKInfo || data.sdkInfo);
    var stream = asObject(data.stream || data.liveStream || data.liveInfo);
    var payload = asObject(data.payload);
    var params = asObject(payload.channelParams);
    var channels = Array.isArray(params.cdn_trans_info) ? params.cdn_trans_info : [];
    var channelURL = channels.length > 0 ? channels[0].url : '';
    return firstHttp(
      sdk.liveCdnUrl,
      sdk.liveStreamUrl,
      sdk.streamUrl,
      data.liveCdnUrl,
      data.liveStreamUrl,
      data.streamUrl,
      data.flvUrl,
      data.hlsUrl,
      stream.liveCdnUrl,
      stream.streamUrl,
      stream.flvUrl,
      channelURL
    );
  }

  function extractProfile(value) {
    var data = unwrap(value);
    return asObject(data.object || data.feed || data.profile || data);
  }

  function title() {
    var profile = asObject(state.profile);
    var objectDesc = asObject(profile.objectDesc);
    var live = asObject(state.live);
    return String(
      profile.description || profile.title || objectDesc.description || live.description || '直播'
    ).replace(/<[^>]*>/g, '').trim() || '直播';
  }

  function author() {
    var profile = asObject(state.profile);
    var contact = asObject(profile.contact || profile.anchorContact);
    return String(contact.nickname || profile.nickname || '未知作者').trim() || '未知作者';
  }

  function liveID() {
    var profile = asObject(state.profile);
    var live = asObject(state.live);
    return String(profile.id || profile.objectId || live.liveId || live.id || '').trim() || ('live_' + Date.now());
  }

  function setButtonText(text, disabled) {
    if (!state.button) return;
    state.button.textContent = text;
    state.button.disabled = !!disabled;
    state.button.style.opacity = disabled ? '0.55' : '1';
    var cursor = disabled ? 'default' : 'pointer';
    if (state.button.style.setProperty) {
      state.button.style.setProperty('cursor', cursor, 'important');
      state.button.style.setProperty('pointer-events', 'auto', 'important');
    } else {
      state.button.style.cursor = cursor;
      state.button.style.pointerEvents = 'auto';
    }
  }

  function consumePointerEvent(event) {
    if (!event) return;
    if (event.preventDefault) event.preventDefault();
    if (event.stopImmediatePropagation) event.stopImmediatePropagation();
    if (event.stopPropagation) event.stopPropagation();
  }

  function setStatus(message, error) {
    state.statusText = String(message || '');
    if (!state.statusNode) return;
    state.statusNode.textContent = state.statusText;
    state.statusNode.title = state.statusText;
    state.statusNode.style.display = state.statusText ? 'inline-block' : 'none';
    state.statusNode.style.color = error ? '#ffb4b4' : 'inherit';
  }

  function getButtonContainer() {
    var selectors = ['.host__info .extra', '[class*="host__info"] .extra'];
    for (var i = 0; i < selectors.length; i++) {
      var node = document.querySelector(selectors[i]);
      if (node && node.offsetParent !== null) return node;
    }
    return null;
  }

  function createButton(container) {
    if (state.button || !container) return;
    var button = document.createElement('button');
    button.type = 'button';
    button.id = '__wx_channels_live_download_button__';
    button.title = '录制当前直播';
    button.setAttribute('aria-label', '下载直播');
    button.textContent = '等待直播流';
    button.style.cssText = [
      'display:inline-flex', 'align-items:center', 'justify-content:center',
      'height:32px', 'min-width:92px', 'margin-right:12px', 'padding:0 12px',
      'border:1px solid rgba(255,255,255,.34)', 'border-radius:6px',
      'background:rgba(0,0,0,.18)', 'color:inherit', 'font-size:13px',
      'line-height:30px', 'position:relative', 'z-index:2147483647',
      'pointer-events:auto', 'cursor:default', 'opacity:.55', 'user-select:none',
      'touch-action:manipulation'
    ].join(';');
    // The live page uses a full-screen click handler to toggle its controls.
    // Consume pointer events on this control before they reach that handler.
    button.addEventListener('pointerdown', consumePointerEvent, true);
    button.addEventListener('mousedown', consumePointerEvent, true);
    button.addEventListener('pointerup', consumePointerEvent, true);
    button.addEventListener('mouseup', consumePointerEvent, true);
    button.addEventListener('click', function (event) {
      consumePointerEvent(event);
      if (state.downloading) {
        return stopDownload();
      }
      return startDownload();
    });
    var first = container.children && container.children[0];
    if (first) container.insertBefore(button, first);
    else container.appendChild(button);
    state.button = button;
    var status = document.createElement('span');
    status.id = '__wx_channels_live_download_status__';
    status.setAttribute('aria-live', 'polite');
    status.style.cssText = [
      'display:none', 'max-width:360px', 'margin-right:12px', 'overflow:hidden',
      'text-overflow:ellipsis', 'white-space:nowrap', 'font-size:12px',
      'line-height:30px', 'vertical-align:middle', 'pointer-events:none'
    ].join(';');
    if (button.nextSibling) container.insertBefore(status, button.nextSibling);
    else container.appendChild(status);
    state.statusNode = status;
    updateButton();
  }

  function updateButton() {
    if (!state.button) return;
    if (state.downloading) {
      setButtonText('停止录制', false);
    } else if (state.streamUrl) {
      setButtonText('下载直播', false);
      state.button.title = state.statusText || '录制当前直播';
    } else {
      setButtonText('等待直播流', true);
      state.button.title = '等待页面捕获直播流';
    }
  }

  function requestHeaders() {
    var headers = { Referer: location.href, Origin: location.origin };
    if (navigator.userAgent) headers['User-Agent'] = navigator.userAgent;
    return headers;
  }

  async function startDownload() {
    if (!state.streamUrl) {
      log('尚未捕获到直播流，请等待页面开始播放');
      return;
    }
    var id = liveID();
    var request = {
      streamUrl: state.streamUrl,
      videoId: id,
      title: title(),
      author: author(),
      sourceUrl: location.href,
      userAgent: navigator.userAgent || '',
      headers: requestHeaders()
    };
    var headers = { 'Content-Type': 'application/json' };
    if (window.__WX_LOCAL_TOKEN__) headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
    setButtonText('创建任务…', true);
    setStatus('正在创建直播录制任务…', false);
    try {
      var response = await apiFetch('/__wx_channels_api/download_live', {
        method: 'POST', headers: headers, body: JSON.stringify(request)
      });
      var data = await response.json().catch(function () { return null; });
      if (!response.ok || !data || !data.success) {
        throw new Error((data && (data.error || data.message)) || ('HTTP ' + response.status));
      }
      state.downloading = true;
      state.downloadId = id;
      state.savePath = String(data.path || data.relativePath || '');
      var locationText = state.savePath || '程序目录/downloads/作者目录';
      setStatus('已启动，保存到：' + locationText, false);
      updateButton();
      log('直播录制已启动：' + title() + '；保存位置：' + locationText);
    } catch (error) {
      updateButton();
      var message = error && error.message ? error.message : String(error);
      setStatus('启动失败：' + message, true);
      setButtonText('重试下载', false);
      state.button.title = message;
      log('直播录制失败：' + message);
    }
  }

  async function stopDownload() {
    var id = state.downloadId;
    if (!id) return;
    var headers = { 'Content-Type': 'application/json' };
    if (window.__WX_LOCAL_TOKEN__) headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
    setStatus('正在停止录制…', false);
    try {
      var response = await apiFetch('/__wx_channels_api/cancel_download', {
        method: 'POST', headers: headers, body: JSON.stringify({ videoId: id })
      });
      var data = await response.json().catch(function () { return null; });
      if (!response.ok || (data && data.success === false)) {
        throw new Error((data && (data.error || data.message)) || ('HTTP ' + response.status));
      }
      log('已请求停止直播录制');
      setStatus('已请求停止，录制文件将在任务结束后保存', false);
    } catch (error) {
      var message = error && error.message ? error.message : String(error);
      setStatus('停止失败：' + message, true);
      log('停止直播录制失败：' + message);
    } finally {
      state.downloading = false;
      state.downloadId = '';
      updateButton();
    }
  }

  window.__wx_channels_capture_live_data__ = function (kind, raw) {
    var data = unwrap(raw);
    if (kind === 'finderGetCommentDetail') {
      state.profile = extractProfile(raw);
    } else if (kind === 'finderGetLiveInfo') {
      state.live = data;
      state.profile = state.profile || extractProfile(raw);
    } else if (kind === 'joinLive') {
      state.live = data;
    }
    var stream = extractStream(raw);
    if (stream) state.streamUrl = stream;
    window.dispatchEvent(new CustomEvent('wx_channels_live_ready', { detail: state }));
    updateButton();
  };

  function mount() {
    if (state.button) return;
    createButton(getButtonContainer());
  }

  function boot() {
    mount();
    if (document.body && !window.__wx_channels_live_observer__) {
      var observer = new MutationObserver(mount);
      observer.observe(document.body, { childList: true, subtree: true });
      window.__wx_channels_live_observer__ = observer;
    }
    setInterval(mount, 1000);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
