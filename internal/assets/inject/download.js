/**
 * @file 下载功能模块
 */
console.log('[download.js] 加载下载模块');

function __wx_channels_parse_video_size__(value) {
  if (typeof value === 'number') {
    return isFinite(value) && value > 0 ? Math.round(value) : 0;
  }

  var text = String(value || '').trim();
  if (!text) return 0;

  var match = text.match(/^([0-9]+(?:\.[0-9]+)?)\s*(B|KB|MB|GB)?$/i);
  if (!match) return 0;

  var number = Number(match[1]);
  if (!isFinite(number) || number <= 0) return 0;

  var unit = (match[2] || 'B').toUpperCase();
  var multiplier = unit === 'GB' ? 1024 * 1024 * 1024
    : unit === 'MB' ? 1024 * 1024
      : unit === 'KB' ? 1024
        : 1;
  return Math.round(number * multiplier);
}

function __wx_channels_get_expected_video_size__(profile) {
  if (!profile) return 0;

  var media = profile.media || {};
  var candidates = [
    media.fullFileSize,
    media.duplicateFileSize,
    profile.fullFileSize,
    media.fileSize,
    media.cdnFileSize,
    profile.fileSize,
    profile.size
  ];

  for (var i = 0; i < candidates.length; i++) {
    var size = __wx_channels_parse_video_size__(candidates[i]);
    if (size > 0) return size;
  }
  return 0;
}

// ==================== 下载函数 ====================

// 浏览器直连无法获知用户实际的下载目录，因此使用与后端相同的标题预算；
// 后端会在已知作者目录后进一步按完整路径收紧预算。
var __WX_CHANNELS_MAX_DOWNLOAD_FILENAME_BODY_UTF16__ = 180;

function __wx_channels_clean_download_filename__(filename) {
  var cleaned = String(filename == null ? '' : filename)
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&amp;/gi, '&')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&apos;|&#39;/gi, "'")
    .replace(/&#34;/gi, '"')
    .replace(/&[a-zA-Z0-9#]+;/g, '')
    .replace(/[<>:"/\\|?*\u0000-\u001f\u007f]/g, '_')
    .trim()
    .replace(/[ .]+$/g, '');

  return cleaned || ('video_' + Date.now());
}

function __wx_channels_truncate_download_filename_utf16__(value, maxUnits) {
  var text = String(value == null ? '' : value);
  if (maxUnits <= 0) return '';
  if (text.length <= maxUnits) return text;

  var end = maxUnits;
  var code = text.charCodeAt(end - 1);
  if (code >= 0xd800 && code <= 0xdbff) end -= 1;
  return text.slice(0, end);
}

function __wx_channels_prepare_download_filename__(filename, requiredSuffix, extension) {
  var cleaned = __wx_channels_clean_download_filename__(filename);
  var suffix = String(requiredSuffix || '');
  var ext = String(extension || '');
  var lowerCleaned = cleaned.toLowerCase();
  var lowerExt = ext.toLowerCase();

  if (ext && lowerCleaned.slice(-lowerExt.length) === lowerExt) {
    cleaned = cleaned.slice(0, cleaned.length - ext.length);
  }

  var base = cleaned;
  if (suffix && base.slice(-suffix.length) !== suffix) suffix = '';

  var title = suffix ? base.slice(0, base.length - suffix.length) : base;
  title = __wx_channels_truncate_download_filename_utf16__(
    title,
    __WX_CHANNELS_MAX_DOWNLOAD_FILENAME_BODY_UTF16__
  ).replace(/[ .]+$/g, '');

  if (!title && !suffix) title = 'video';
  return title + suffix;
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

function __wx_channels_get_true_original_url__(profile) {
  if (!profile) return '';

  var media = profile.media || {};
  var candidates = [
    media.fullUrl,
    media.fullURL,
    profile.fullUrl,
    profile.fullURL
  ];

  for (var i = 0; i < candidates.length; i++) {
    var candidate = String(candidates[i] || '').trim();
    if (/^https?:\/\//i.test(candidate)) return candidate;
  }
  return '';
}

function __wx_channels_build_original_video_url__(rawUrl) {
  var source = String(rawUrl || '').trim();
  if (!source) return '';

  var baseOrigin = window.location && window.location.origin ? window.location.origin : 'http://localhost';
  var parsed = null;
  try {
    parsed = new URL(source, baseOrigin);
  } catch (err) {
    try {
      parsed = new URL(decodeURIComponent(source), baseOrigin);
    } catch (decodeErr) {
      __wx_log({ msg: '⚠️ 原始视频链接归一化失败<' + (decodeErr && decodeErr.message ? decodeErr.message : decodeErr) + '>' });
      return source;
    }
  }

  // The complete signed query is required by the video endpoint. Keep every
  // parameter and only remove the legacy rendition marker.
  parsed.searchParams.delete('X-snsvideoflag');
  return parsed.toString();
}

function __wx_channels_get_original_video_url__(profile) {
  if (!profile) return '';

  var candidate = __wx_channels_get_true_original_url__(profile) || __wx_channels_select_video_url__(profile);
  if (!/^https?:\/\//i.test(String(candidate || '').trim())) return '';
  return __wx_channels_build_original_video_url__(candidate);
}

function __wx_channels_has_true_original__(profile) {
  // A signed playable URL is only a rendition entry point. Treat it as the
  // original stream when the page exposes an explicit fullUrl; otherwise the
  // caller must select a concrete media.spec such as xWT111.
  var candidate = __wx_channels_get_true_original_url__(profile);
  return /^https?:\/\//i.test(String(candidate || '').trim());
}

function __wx_channels_get_best_available_spec__(profile) {
  var specs = profile && Array.isArray(profile.spec) ? profile.spec : [];
  var best = null;
  var bestScore = -1;

  for (var i = 0; i < specs.length; i++) {
    var spec = specs[i];
    if (!spec || !spec.fileFormat) continue;

    var videoBitrate = Number(spec.videoBitrate || 0);
    var audioBitrate = Number(spec.audioBitrate || 0);
    var bitRate = videoBitrate + audioBitrate;
    if (bitRate <= 0) bitRate = Number(spec.bitRate || 0);

    // xWT111 is the stable highest-quality fallback on the current desktop
    // feed; keep it ahead of codec-specific bitrate hints when present.
    var format = String(spec.fileFormat);
    var score = bitRate > 0 ? bitRate : 0;
    if (format === 'xWT111') score += 1000000000000;
    if (!best || score > bestScore || (score === bestScore && format === 'xWT111')) {
      best = spec;
      bestScore = score;
    }
  }

  return best;
}

function __wx_channels_primary_download_label__(profile) {
  if (__wx_channels_has_true_original__(profile)) return '原始视频';

  var best = __wx_channels_get_best_available_spec__(profile);
  if (best && best.fileFormat) return '最高可用画质 (' + best.fileFormat + ')';
  return '原始流不可用';
}

function __wx_channels_append_query_param__(url, key, value) {
  if (!url || !key) return url || '';

  var separator = url.indexOf('?') >= 0 ? '&' : '?';
  return url + separator + key + '=' + encodeURIComponent(value);
}

function __wx_channels_remove_legacy_original_marker__(rawUrl) {
  var source = String(rawUrl || '').trim();
  if (!source || source.indexOf('X-snsvideoflag=original') < 0) return source;

  try {
    var baseOrigin = window.location && window.location.origin ? window.location.origin : 'http://localhost';
    var parsed = new URL(source, baseOrigin);
    if (parsed.searchParams.get('X-snsvideoflag') !== 'original') return source;
    parsed.searchParams.delete('X-snsvideoflag');
    return parsed.toString();
  } catch (err) {
    __wx_log({ msg: '⚠️ 旧版原始视频标记清理失败<' + (err && err.message ? err.message : err) + '>' });
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

function __wx_channels_join_video_url_parts__(baseUrl, urlToken) {
  var base = String(baseUrl || '').trim();
  var token = String(urlToken || '').trim();
  if (!base) return token;
  if (!token) return base;
  if (/^https?:\/\//i.test(token)) return token;
  if (/%(?:26|3d|3f)/i.test(token)) {
    // Decode only query structure markers. Decoding the complete fragment
    // would turn escaped signature bytes such as %2F into different text.
    token = token
      .replace(/%26/gi, '&')
      .replace(/%3d/gi, '=')
      .replace(/%3f/gi, '?');
  }
  if (/^https?:\/\//i.test(token)) return token;

  var fragment = '';
  var fragmentIndex = base.indexOf('#');
  if (fragmentIndex >= 0) {
    fragment = base.substring(fragmentIndex);
    base = base.substring(0, fragmentIndex);
  }
  var query = token.replace(/^[?&]+/, '');
  if (!query) return base + fragment;
  var separator = base.indexOf('?') >= 0 ? '&' : '?';
  if (/[?&]$/.test(base)) separator = '';
  return base + separator + query + fragment;
}

function __wx_channels_video_url_score__(rawUrl) {
  var source = String(rawUrl || '').trim();
  if (!source) return 0;

  var signatureKeys = {
    encfilekey: true,
    token: true,
    basedata: true,
    sign: true,
    web: true,
    extg: true,
    svrbypass: true,
    svrnonce: true
  };

  try {
    var baseOrigin = window.location && window.location.origin ? window.location.origin : 'http://localhost';
    var parsed = new URL(source, baseOrigin);
    var score = 0;
    parsed.searchParams.forEach(function (value, key) {
      if (!value || key === 'X-snsvideoflag') return;
      score += signatureKeys[key] ? 100 : 1;
    });
    return score;
  } catch (err) {
    var query = source.indexOf('?') >= 0 ? source.substring(source.indexOf('?') + 1) : '';
    return query ? query.split('&').filter(function (part) { return part; }).length : 0;
  }
}

function __wx_channels_select_video_url__(profile) {
  if (!profile) return '';

  var candidates = [];
  function addCandidate(value) {
    var candidate = String(value || '').trim();
    if (candidate) candidates.push(candidate);
  }

  addCandidate(profile.url);
  addCandidate(__wx_channels_join_video_url_parts__(profile.originalUrl || profile.original_url, profile.urlToken || profile.url_token || profile.urltoken));
  addCandidate(profile.originalUrl || profile.original_url);

  var media = profile.media || {};
  addCandidate(media.url);
  addCandidate(__wx_channels_join_video_url_parts__(media.url, media.urlToken || media.url_token || media.urltoken));
  addCandidate(media.fullUrl || media.fullURL || media.full_url);
  addCandidate(profile.fullUrl || profile.fullURL || profile.full_url);

  var selected = '';
  var selectedScore = 0;
  for (var i = 0; i < candidates.length; i++) {
    var score = __wx_channels_video_url_score__(candidates[i]);
    if (!selected || score > selectedScore) {
      selected = candidates[i];
      selectedScore = score;
    }
  }
  return selected;
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
    spec: spec || null
  };

  if (!profile) {
    return normalized;
  }

  var originalCandidate = __wx_channels_remove_legacy_original_marker__(
    __wx_channels_select_video_url__(profile)
  );
  normalized.url = __wx_channels_get_original_video_url__(profile);

  var explicitSpec = spec && spec.fileFormat && String(spec.fileFormat).toLowerCase() !== 'original' ? spec : null;
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

    normalized.url = __wx_channels_append_query_param__(originalCandidate, 'X-snsvideoflag', normalized.fileFormat);
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

function __wx_channels_normalize_batch_video_download__(profile) {
  var spec = null;
  if (!__wx_channels_has_true_original__(profile)) {
    spec = __wx_channels_get_best_available_spec__(profile);
  }
  return __wx_channels_normalize_video_download__(profile, spec);
}

async function __wx_channels_download_via_backend__(profile, filename, normalized) {
  var authorName = profile.nickname || (profile.contact && profile.contact.nickname) || '未知作者';
  var hasKey = !!(profile.key && profile.key.length > 0);
  var expectedSize = normalized.mode === 'original'
    ? __wx_channels_get_expected_video_size__(profile)
    : 0;
  var requestData = {
    videoUrl: normalized.url,
    videoId: profile.id || '',
    // 文件名是落盘投影，数据库和下载记录应保留原始标题。
    title: profile.title || profile.id || filename,
    author: authorName,
    sourceUrl: location.href,
    userAgent: navigator.userAgent || '',
    headers: {
      'Referer': location.href,
      'Origin': location.origin || 'https://channels.weixin.qq.com'
    },
    key: profile.key || '',
    forceSave: false,
    resolution: normalized.resolution,
    width: normalized.width,
    height: normalized.height,
    fileFormat: normalized.fileFormat,
    expectedSize: expectedSize,
    likeCount: profile.likeCount || 0,
    commentCount: profile.commentCount || 0,
    forwardCount: profile.forwardCount || 0,
    favCount: profile.favCount || 0
  };

  var headers = { 'Content-Type': 'application/json' };
  if (window.__WX_LOCAL_TOKEN__) {
    headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
  }

  __wx_log({ msg: '🚀 使用 Gopeed 后端下载: ' + filename.substring(0, 30) + '...' });
  var response = await fetch('/__wx_channels_api/download_video', {
    method: 'POST',
    headers: headers,
    body: JSON.stringify(requestData)
  });

  var data = null;
  try {
    data = await response.json();
  } catch (err) {
    throw new Error('后端下载响应不是有效 JSON');
  }

  if (!response.ok || !data || !data.success) {
    throw new Error((data && (data.error || data.message)) || ('HTTP ' + response.status));
  }

  var msg = data.skipped
    ? '⏭️ 文件已存在，跳过下载'
    : (data.started ? '⏳ 后端下载任务已在后台启动' : (hasKey ? '✓ 视频已下载并解密' : '✓ 视频已下载'));
  __wx_log({ msg: msg });
  return data;
}

// ==================== 点击下载处理 ====================
async function __wx_channels_handle_click_download__(spec) {
  var profile = __wx_channels_store__.profile;
  if (!profile) {
    alert("检测不到视频，请将本工具更新到最新版");
    return;
  }

  var hasTrueOriginal = __wx_channels_has_true_original__(profile);
  if (!spec && !hasTrueOriginal) {
    spec = __wx_channels_get_best_available_spec__(profile);
    if (!spec) {
      __wx_log({ msg: '❌ 微信当前未提供原始流或可用画质' });
      alert('微信当前未提供原始视频流，无法下载');
      return;
    }
    __wx_log({ msg: '⚠️ 微信未提供原始流，改用最高可用画质<' + spec.fileFormat + '>' });
  }

  var filename = profile.title || profile.id || String(new Date().valueOf());
  var _profile = Object.assign({}, profile);
  var normalized = __wx_channels_normalize_video_download__(profile, spec);
  _profile.url = normalized.url;
  var qualitySuffix = '';

  if (normalized.qualityInfo) {
    qualitySuffix = "_" + __wx_channels_clean_download_filename__(normalized.qualityInfo);
    filename = filename + qualitySuffix;
  }

  __wx_log({ msg: '下载模式<' + normalized.mode + '>' });
  __wx_log({ msg: '视频链接<' + _profile.url + '>' });

  if (_profile.type === "picture") {
    filename = __wx_channels_prepare_download_filename__(filename, '', '');
    __wx_log({ msg: '下载文件名<' + filename + '>' });
    __wx_channels_download3(_profile, filename);
    return;
  }

  filename = __wx_channels_prepare_download_filename__(filename, qualitySuffix, '.mp4');
  __wx_log({ msg: '下载文件名<' + filename + '>' });

  if (!_profile.url) {
    alert("视频URL为空，无法下载");
    return;
  }

  try {
    await __wx_channels_download_via_backend__(_profile, filename, normalized);
  } catch (error) {
    __wx_log({ msg: '❌ 下载视频失败: ' + (error && error.message ? error.message : error) });
    alert('下载失败: ' + (error && error.message ? error.message : error));
  }
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
