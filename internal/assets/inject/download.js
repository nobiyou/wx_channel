/**
 * @file 下载功能模块
 */
console.log('[download.js] 加载下载模块');

function __wx_channels_load_script_once__(src) {
  return new Promise(function (resolve, reject) {
    var existing = document.querySelector('script[data-wx-src="' + src + '"]');
    if (existing) {
      if (existing.getAttribute('data-loaded') === '1') {
        resolve();
        return;
      }
      existing.addEventListener('load', function () { resolve(); }, { once: true });
      existing.addEventListener('error', function (ev) { reject(ev || new Error('script load failed')); }, { once: true });
      return;
    }

    var script = document.createElement('script');
    script.type = 'text/javascript';
    script.src = src;
    script.setAttribute('data-wx-src', src);
    script.onload = function () {
      script.setAttribute('data-loaded', '1');
      resolve();
    };
    script.onerror = function (ev) {
      reject(ev || new Error('script load failed: ' + src));
    };
    document.head.appendChild(script);
  });
}

async function __wx_channels_ensure_saveas__() {
  if (typeof window.saveAs === 'function') return;

  var lastErr = null;
  var candidates = [
    '/FileSaver.min.js',
    'https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js'
  ];

  for (var i = 0; i < candidates.length; i++) {
    var src = candidates[i];
    try {
      __wx_log({ msg: '🌐 加载保存组件<' + src + '>' });
      await __wx_channels_load_script_once__(src);
      if (typeof window.saveAs === 'function') return;
    } catch (err) {
      lastErr = err;
      __wx_log({ msg: '⚠️ 保存组件加载失败<' + src + '>' });
    }
  }

  throw lastErr || new Error('saveAs is unavailable');
}

// ==================== 进度条显示 ====================
async function show_progress_or_loaded_size(response) {
  var content_length = response.headers.get("Content-Length");
  var chunks = [];
  var total_size = content_length ? parseInt(content_length, 10) : 0;

  var progressBarId = 'progress-' + Date.now();
  var progressBarHTML = '<div id="' + progressBarId + '" style="position: fixed; top: 20px; left: 50%; transform: translateX(-50%); z-index: 10000; background: rgba(0,0,0,0.7); border-radius: 8px; padding: 15px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); color: white; font-size: 14px; min-width: 280px; text-align: center;">' +
    '<div style="margin-bottom: 12px; font-weight: bold;">视频下载中</div>' +
    '<div class="progress-container" style="background: rgba(255,255,255,0.2); height: 10px; border-radius: 5px; overflow: hidden; margin-bottom: 10px;">' +
    '<div class="progress-bar" style="height: 100%; width: 0%; background: #07c160; transition: width 0.3s;"></div></div>' +
    '<div class="progress-details" style="display: flex; justify-content: space-between; font-size: 12px; opacity: 0.8;">' +
    '<span class="progress-size">准备下载...</span><span class="progress-speed"></span></div></div>';

  var progressBarContainer = document.createElement('div');
  progressBarContainer.innerHTML = progressBarHTML;
  document.body.appendChild(progressBarContainer.firstElementChild);

  var progressBar = document.querySelector('#' + progressBarId + ' .progress-bar');
  var progressSize = document.querySelector('#' + progressBarId + ' .progress-size');
  var progressSpeed = document.querySelector('#' + progressBarId + ' .progress-speed');

  var loaded_size = 0;
  var reader = response.body.getReader();
  var lastUpdate = Date.now();
  var lastLoaded = 0;

  while (true) {
    var result = await reader.read();
    if (result.done) break;

    chunks.push(result.value);
    loaded_size += result.value.length;

    var currentTime = Date.now();
    if (currentTime - lastUpdate > 200) {
      var percent = total_size ? (loaded_size / total_size * 100) : 0;
      if (progressBar) progressBar.style.width = percent + '%';

      if (total_size) {
        progressSize.textContent = formatFileSize(loaded_size) + ' / ' + formatFileSize(total_size);
      } else {
        progressSize.textContent = '已下载: ' + formatFileSize(loaded_size);
      }

      var timeElapsed = (currentTime - lastUpdate) / 1000;
      if (timeElapsed > 0) {
        var currentSpeed = (loaded_size - lastLoaded) / timeElapsed;
        progressSpeed.textContent = formatFileSize(currentSpeed) + '/s';
      }

      lastLoaded = loaded_size;
      lastUpdate = currentTime;
    }
  }

  var progressElement = document.getElementById(progressBarId);
  if (progressElement) {
    setTimeout(function () {
      progressElement.style.opacity = '0';
      progressElement.style.transition = 'opacity 0.5s';
      setTimeout(function () { progressElement.remove(); }, 500);
    }, 1000);
  }

  __wx_log({ msg: '下载完成，文件总大小<' + formatFileSize(loaded_size) + '>' });

  return new Blob(chunks);
}

// ==================== 下载函数 ====================

/** 下载非加密视频 */
async function __wx_channels_download2(profile, filename) {
  console.log("__wx_channels_download2");
  try {
    __wx_log({ msg: '🌐 正在加载保存组件...' });
    await __wx_channels_ensure_saveas__();
    __wx_log({ msg: '🚀 正在发起页面直连请求...' });
    var response = await fetch(profile.url);
    __wx_log({
      msg: '📡 页面直连响应<status=' + response.status + ' length=' + (response.headers.get('Content-Length') || 'unknown') + ' type=' + (response.headers.get('Content-Type') || '') + ' range=' + (response.headers.get('Content-Range') || '') + '>'
    });
    var blob = await show_progress_or_loaded_size(response);
    __wx_log({ msg: '📦 页面抓流大小<' + formatFileSize(blob.size) + '>' });
    __wx_log({ msg: '💾 正在保存视频文件...' });
    saveAs(blob, filename + ".mp4");
    __wx_log({ msg: '✓ 页面直连保存完成' });
  } catch (err) {
    __wx_log({ msg: '❌ 页面直连下载失败<' + (err && err.message ? err.message : err) + '>' });
    throw err;
  }
}

/** 下载图片 */
async function __wx_channels_download3(profile, filename) {
  console.log("__wx_channels_download3");
  await __wx_load_script("https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js");
  await __wx_load_script("https://res.wx.qq.com/t/wx_fed/cdn_libs/res/jszip.min.js");

  var zip = new JSZip();
  zip.file("contact.txt", JSON.stringify(profile.contact, null, 2));
  var folder = zip.folder("images");

  var fetchPromises = profile.files.map(function (f, index) {
    return fetch(f.url).then(function (response) {
      return response.blob();
    }).then(function (blob) {
      folder.file((index + 1) + ".png", blob);
    });
  });

  try {
    await Promise.all(fetchPromises);
    var content = await zip.generateAsync({ type: "blob" });
    saveAs(content, filename + ".zip");
  } catch (err) {
    __wx_log({ msg: "下载失败\n" + err.message });
  }
}

/** 下载加密视频 */
async function __wx_channels_download4(profile, filename) {
  console.log("__wx_channels_download4");
  try {
    __wx_log({ msg: '🌐 正在加载保存组件...' });
    await __wx_channels_ensure_saveas__();

    if (profile.key && !profile.decryptor_array) {
      __wx_log({ msg: '🔑 正在生成解密数组...' });
      console.log('🔑 检测到加密key，正在生成解密数组...');
      profile.decryptor_array = await __wx_channels_decrypt(profile.key);
    }

    __wx_log({ msg: '🚀 正在发起页面直连请求...' });
    var response = await fetch(profile.url);
    __wx_log({
      msg: '📡 页面直连响应<status=' + response.status + ' length=' + (response.headers.get('Content-Length') || 'unknown') + ' type=' + (response.headers.get('Content-Type') || '') + ' range=' + (response.headers.get('Content-Range') || '') + '>'
    });
    var blob = await show_progress_or_loaded_size(response);
    __wx_log({ msg: '📦 页面抓流大小<' + formatFileSize(blob.size) + '>' });

    var array = new Uint8Array(await blob.arrayBuffer());
    if (profile.decryptor_array) {
      __wx_log({ msg: '🔐 正在解密视频...' });
      console.log('🔐 开始解密视频');
      array = __wx_channels_video_decrypt(array, 0, profile);
      console.log('✓ 视频解密完成');
    }

    var result = new Blob([array], { type: "video/mp4" });
    __wx_log({ msg: '💾 正在保存视频文件...' });
    saveAs(result, filename + ".mp4");
    __wx_log({ msg: '✓ 页面直连保存完成' });
  } catch (err) {
    __wx_log({ msg: '❌ 页面直连下载失败<' + (err && err.message ? err.message : err) + '>' });
    throw err;
  }
}

function __wx_channels_export_current_raw_json__() {
  var store = window.__wx_channels_store__ || {};
  var profile = store.profile || null;
  var rawFeed = store.rawFeed || null;
  var rawProfile = store.rawProfile || profile || null;

  if (!rawFeed && !rawProfile) {
    __wx_log({ msg: '❌ 当前没有可导出的原始视频数据' });
    alert('当前没有可导出的原始视频数据');
    return;
  }

  var payload = {
    exportedAt: new Date().toISOString(),
    pageUrl: location.href,
    profile: rawProfile,
    rawFeed: rawFeed
  };

  var title = (profile && (profile.title || profile.id)) || 'current_video';
  var safeTitle = String(title)
    .replace(/[\\/:*?"<>|]/g, '_')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80) || 'current_video';

  var blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
  var url = URL.createObjectURL(blob);
  var a = document.createElement('a');
  a.href = url;
  a.download = safeTitle + '_raw.json';
  a.click();
  URL.revokeObjectURL(url);

  __wx_log({ msg: '📤 已导出当前视频原始数据 JSON' });
}

function __wx_channels_has_true_original__(profile) {
  if (!profile || !profile.media) return false;
  var media = profile.media;
  return !!(
    (media.fullUrl && String(media.fullUrl).trim()) ||
    (media.fullFileSize && Number(media.fullFileSize) > 0) ||
    (media.duplicateFileSize && Number(media.duplicateFileSize) > 0)
  );
}

function __wx_channels_primary_download_label__(profile) {
  return '原始视频';
}

function __wx_channels_append_query_param__(url, key, value) {
  if (!url || !key) return url || '';

  var separator = url.indexOf('?') >= 0 ? '&' : '?';
  return url + separator + key + '=' + encodeURIComponent(value);
}

function __wx_channels_build_original_video_url__(rawUrl) {
  var source = String(rawUrl || '').trim();
  if (!source) return '';

  try {
    var parsed = new URL(source);
    var filekey = (parsed.searchParams.get('encfilekey') || '').trim();
    var token = (parsed.searchParams.get('token') || '').trim();
    if (!filekey || !token) {
      return source;
    }

    var cleaned = new URL(parsed.origin + parsed.pathname);
    cleaned.searchParams.set('encfilekey', filekey);
    cleaned.searchParams.set('token', token);
    return cleaned.toString();
  } catch (err) {
    __wx_log({ msg: '⚠️ 原始视频链接归一化失败<' + (err && err.message ? err.message : err) + '>' });
    return source;
  }
}

function __wx_channels_get_profile_video_dimensions__(profile) {
  var media = profile && profile.media ? profile.media : {};
  var width = Number((profile && profile.width) || media.fullWidth || media.width || 0);
  var height = Number((profile && profile.height) || media.fullHeight || media.height || 0);

  return {
    width: width > 0 ? width : 0,
    height: height > 0 ? height : 0
  };
}

function __wx_channels_normalize_video_download__(profile, spec) {
  var normalized = {
    mode: 'original',
    url: '',
    resolution: '',
    width: 0,
    height: 0,
    fileFormat: '',
    qualityInfo: '',
    useDirectDownload: false,
    spec: spec || null
  };

  if (!profile) {
    return normalized;
  }

  var media = profile.media || {};
  var originalCandidate = '';
  if (profile.url && String(profile.url).trim()) {
    originalCandidate = String(profile.url).trim();
  } else if (profile.originalUrl && profile.urlToken) {
    originalCandidate = String(profile.originalUrl) + String(profile.urlToken);
  } else if (profile.originalUrl && String(profile.originalUrl).trim()) {
    originalCandidate = String(profile.originalUrl).trim();
  } else if (media.url && media.urlToken) {
    originalCandidate = String(media.url) + String(media.urlToken);
  } else if (media.url && String(media.url).trim()) {
    originalCandidate = String(media.url).trim();
  }
  normalized.url = __wx_channels_build_original_video_url__(originalCandidate);

  var explicitSpec = spec && spec.fileFormat ? spec : null;
  if (explicitSpec) {
    normalized.mode = 'specific';
    normalized.fileFormat = explicitSpec.fileFormat || '';
    normalized.width = Number(explicitSpec.width || 0);
    normalized.height = Number(explicitSpec.height || 0);
    normalized.qualityInfo = normalized.fileFormat;

    if (normalized.width > 0 && normalized.height > 0) {
      normalized.resolution = normalized.width + 'x' + normalized.height;
      normalized.qualityInfo += '_' + normalized.resolution;
    }

    var specUrl = '';
    if (profile.url && String(profile.url).trim()) {
      specUrl = String(profile.url).trim();
    } else if (profile.originalUrl && profile.urlToken) {
      specUrl = String(profile.originalUrl) + String(profile.urlToken);
    } else if (profile.originalUrl && String(profile.originalUrl).trim()) {
      specUrl = String(profile.originalUrl).trim();
    } else if (media.url && media.urlToken) {
      specUrl = String(media.url) + String(media.urlToken);
    } else if (media.url && String(media.url).trim()) {
      specUrl = String(media.url).trim();
    }
    normalized.url = __wx_channels_append_query_param__(specUrl, 'X-snsvideoflag', normalized.fileFormat);
    normalized.useDirectDownload = false;
    return normalized;
  }

  var originalDimensions = __wx_channels_get_profile_video_dimensions__(profile);
  normalized.width = originalDimensions.width;
  normalized.height = originalDimensions.height;
  if (normalized.width > 0 && normalized.height > 0) {
    normalized.resolution = normalized.width + 'x' + normalized.height;
  }

  return normalized;
}

// ==================== 点击下载处理 ====================
async function __wx_channels_handle_click_download__(spec) {
  var profile = __wx_channels_store__.profile;
  if (!profile) {
    alert("检测不到视频，请将本工具更新到最新版");
    return;
  }

  var filename = profile.title || profile.id || String(new Date().valueOf());
  var _profile = Object.assign({}, profile);
  var normalized = __wx_channels_normalize_video_download__(profile, spec);
  _profile.url = normalized.url;

  if (normalized.qualityInfo) {
    filename = filename + "_" + normalized.qualityInfo;
  }

  __wx_log({ msg: '下载模式<' + normalized.mode + '>' });
  __wx_log({ msg: '下载文件名<' + filename + '>' });
  __wx_log({ msg: '视频链接<' + _profile.url + '>' });

  if (_profile.type === "picture") {
    __wx_channels_download3(_profile, filename);
    return;
  }

  if (!_profile.url) {
    alert("视频URL为空，无法下载");
    return;
  }

  var authorName = _profile.nickname || (_profile.contact && _profile.contact.nickname) || '未知作者';
  var hasKey = !!(_profile.key && _profile.key.length > 0);

  // 获取分辨率信息
  var requestData = {
    videoUrl: _profile.url,
    videoId: _profile.id || '',
    title: filename,
    author: authorName,
    sourceUrl: location.href,
    userAgent: navigator.userAgent || '',
    headers: {
      'Referer': location.href,
      'Origin': location.origin || 'https://channels.weixin.qq.com'
    },
    key: _profile.key || '',
    forceSave: false,
    resolution: normalized.resolution,
    width: normalized.width,
    height: normalized.height,
    fileFormat: normalized.fileFormat,
    likeCount: _profile.likeCount || 0,
    commentCount: _profile.commentCount || 0,
    forwardCount: _profile.forwardCount || 0,
    favCount: _profile.favCount || 0
  };

  var headers = { 'Content-Type': 'application/json' };
  if (window.__WX_LOCAL_TOKEN__) {
    headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
  }

  __wx_log({ msg: '📥 开始下载: ' + filename.substring(0, 30) + '...' });

  fetch('/__wx_channels_api/download_video', {
    method: 'POST',
    headers: headers,
    body: JSON.stringify(requestData)
  })
    .then(function (response) { return response.json(); })
    .then(function (data) {
      if (data.success) {
        var msg = data.skipped
          ? '⏭️ 文件已存在，跳过下载'
          : (data.started ? '⏳ 下载任务已在后台启动' : (hasKey ? '✓ 视频已下载并解密' : '✓ 视频已下载'));
        __wx_log({ msg: msg });
      } else {
        __wx_log({ msg: '❌ ' + (data.error || '下载视频失败') });
        alert('下载失败: ' + (data.error || '下载视频失败'));
      }
    })
    .catch(function (error) {
      __wx_log({ msg: '❌ 下载视频失败: ' + error.message });
      alert('下载失败: ' + error.message);
    });
}

// ==================== 封面下载 ====================
async function __wx_channels_handle_download_cover() {
  var profile = __wx_channels_store__.profile;
  if (!profile) {
    alert("未找到视频信息");
    return;
  }

  var coverUrl = profile.thumbUrl || profile.fullThumbUrl || profile.coverUrl;
  if (!coverUrl) {
    alert("未找到封面图片");
    return;
  }

  __wx_log({ msg: '正在保存封面到服务器...' });

  var requestData = {
    coverUrl: coverUrl,
    videoId: profile.id || '',
    title: profile.title || '',
    author: profile.nickname || (profile.contact && profile.contact.nickname) || '未知作者',
    forceSave: false
  };

  var headers = { 'Content-Type': 'application/json' };
  if (window.__WX_LOCAL_TOKEN__) {
    headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
  }

  fetch('/__wx_channels_api/save_cover', {
    method: 'POST',
    headers: headers,
    body: JSON.stringify(requestData)
  })
    .then(function (response) { return response.json(); })
    .then(function (data) {
      if (data.success) {
        __wx_log({ msg: '✓ ' + (data.message || '封面已保存') });
      } else {
        __wx_log({ msg: '❌ ' + (data.error || '保存封面失败') });
        alert('保存封面失败: ' + (data.error || '未知错误'));
      }
    })
    .catch(function (error) {
      __wx_log({ msg: '❌ 保存封面失败: ' + error.message });
      alert("保存封面失败: " + error.message);
    });
}

console.log('[download.js] 下载模块加载完成');
