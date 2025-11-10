const defaultRandomAlphabet =
  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
function __wx_uid__() {
  return random_string(12);
}
/**
 * 返回一个指定长度的随机字符串
 * @param length
 * @returns
 */
function random_string(length) {
  return random_string_with_alphabet(length, defaultRandomAlphabet);
}
function random_string_with_alphabet(length, alphabet) {
  let b = new Array(length);
  let max = alphabet.length;
  for (let i = 0; i < b.length; i++) {
    let n = Math.floor(Math.random() * max);
    b[i] = alphabet[n];
  }
  return b.join("");
}
function sleep() {
  return new Promise((resolve) => {
    setTimeout(() => {
      resolve();
    }, 1000);
  });
}
function __wx_channels_copy(text) {
  const textArea = document.createElement("textarea");
  textArea.value = text;
  textArea.style.cssText = "position: absolute; top: -999px; left: -999px;";
  document.body.appendChild(textArea);
  textArea.select();
  document.execCommand("copy");
  document.body.removeChild(textArea);
}
function __wx_channel_loading() {
  if (window.__wx_channels_tip__ && window.__wx_channels_tip__.loading) {
    return window.__wx_channels_tip__.loading("下载中");
  }
  return {
    hide() {},
  };
}
function __wx_log(msg) {
  fetch("/__wx_channels_api/tip", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(msg),
  });
}
function __wx_channels_video_decrypt(t, e, p) {
  for (
    var r = new Uint8Array(t), n = 0;
    n < t.byteLength && e + n < p.decryptor_array.length;
    n++
  )
    r[n] ^= p.decryptor_array[n];
  return r;
}
window.VTS_WASM_URL =
  "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/decrypt-video-core/1.3.0/wasm_video_decode.wasm";
window.MAX_HEAP_SIZE = 33554432;
var decryptor_array;
let decryptor;
/** t 是要解码的视频内容长度    e 是 decryptor_array 的长度 */
function wasm_isaac_generate(t, e) {
  decryptor_array = new Uint8Array(e);
  var r = new Uint8Array(Module.HEAPU8.buffer, t, e);
  decryptor_array.set(r.reverse());
  if (decryptor) {
    decryptor.delete();
  }
}
let loaded = false;
/** 获取 decrypt_array */
async function __wx_channels_decrypt(seed) {
  if (!loaded) {
    await __wx_load_script(
      "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/decrypt-video-core/1.3.0/wasm_video_decode.js"
    );
    loaded = true;
  }
  await sleep();
  decryptor = new Module.WxIsaac64(seed);
  // 调用该方法时，会调用 wasm_isaac_generate 方法
  // 131072 是 decryptor_array 的长度
  decryptor.generate(131072);
  // decryptor.delete();
  // const r = Uint8ArrayToBase64(decryptor_array);
  // decryptor_array = undefined;
  return decryptor_array;
}
async function show_progress_or_loaded_size(response) {
  const content_length = response.headers.get("Content-Length");
  const chunks = [];
  const total_size = content_length ? parseInt(content_length, 10) : 0;
  
  // Create a progress bar container with animated progress bar
  const progressBarId = `progress-${Date.now()}`;
  const progressBarHTML = `
    <div id="${progressBarId}" style="position: fixed; top: 20px; left: 50%; transform: translateX(-50%); z-index: 10000; background: rgba(0,0,0,0.7); border-radius: 8px; padding: 15px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); color: white; font-size: 14px; min-width: 280px; text-align: center;">
      <div style="margin-bottom: 12px; font-weight: bold;">视频下载中</div>
      <div class="progress-container" style="background: rgba(255,255,255,0.2); height: 10px; border-radius: 5px; overflow: hidden; margin-bottom: 10px; position: relative;">
        <div class="progress-bar" style="height: 100%; width: 100%; position: relative; overflow: hidden;">
          <div class="progress-bar-animation" style="position: absolute; height: 100%; width: 30%; background: #07c160; left: -30%; animation: progress-animation 1.5s infinite linear;"></div>
        </div>
      </div>
      <div class="progress-details" style="display: flex; justify-content: space-between; font-size: 12px; opacity: 0.8;">
        <span class="progress-size">准备下载...</span>
        <span class="progress-speed"></span>
      </div>
      <style>
        @keyframes progress-animation {
          0% { left: -30%; }
          100% { left: 100%; }
        }
      </style>
    </div>
  `;
  
  // Insert progress bar into DOM
  const progressBarContainer = document.createElement('div');
  progressBarContainer.innerHTML = progressBarHTML;
  document.body.appendChild(progressBarContainer.firstElementChild);
  
  const progressSize = document.querySelector(`#${progressBarId} .progress-size`);
  const progressSpeed = document.querySelector(`#${progressBarId} .progress-speed`);
  
  let loaded_size = 0;
  const reader = response.body.getReader();
  let startTime = Date.now();
  let lastUpdate = startTime;
  let lastLoaded = 0;
  
  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    
    chunks.push(value);
    loaded_size += value.length;
    
    // 更新下载信息，但不太频繁
    const currentTime = Date.now();
    if (currentTime - lastUpdate > 200) {
      // 显示已下载大小
      if (total_size) {
        progressSize.textContent = `${formatFileSize(loaded_size)} / ${formatFileSize(total_size)}`;
      } else {
        progressSize.textContent = `已下载: ${formatFileSize(loaded_size)}`;
      }
      
      // 计算并显示下载速度
      const timeElapsed = (currentTime - lastUpdate) / 1000;
      if (timeElapsed > 0) {
        const bytesReceived = loaded_size - lastLoaded;
        const currentSpeed = bytesReceived / timeElapsed;
        progressSpeed.textContent = `${formatFileSize(currentSpeed)}/s`;
      }
      
      lastLoaded = loaded_size;
      lastUpdate = currentTime;
    }
  }
  
  // 下载完成，显示成功通知
  const progressElement = document.getElementById(progressBarId);
  if (progressElement) {
    progressElement.innerHTML = `
      <div style="padding: 5px;">
        <div style="display: flex; align-items: center; justify-content: center; margin-bottom: 10px;">
          <svg viewBox="0 0 1024 1024" width="24" height="24" style="margin-right: 8px; fill: #07c160;">
            <path d="M512 64C264.6 64 64 264.6 64 512s200.6 448 448 448 448-200.6 448-448S759.4 64 512 64zm193.5 301.7l-210.6 292a31.8 31.8 0 0 1-51.7 0L318.5 484.9c-3.8-5.3 0-12.7 6.5-12.7h46.9c10.2 0 19.9 4.9 25.9 13.3l71.2 98.8 157.2-218c6-8.3 15.6-13.3 25.9-13.3H699c6.5 0 10.3 7.4 6.5 12.7z"></path>
          </svg>
          <span style="font-weight: bold; font-size: 16px;">下载完成</span>
        </div>
        <div style="font-size: 14px; margin-bottom: 5px;">总大小: ${formatFileSize(loaded_size)}</div>
        <div style="font-size: 12px; opacity: 0.8;">正在准备保存...</div>
      </div>
    `;
    
    // Auto remove after 2 seconds
    setTimeout(() => {
      progressElement.style.opacity = '0';
      progressElement.style.transition = 'opacity 0.5s';
      setTimeout(() => progressElement.remove(), 500);
    }, 1000);
  }
  
  // Log completion to console
  __wx_log({
    msg: `下载完成，文件总大小<${formatFileSize(loaded_size)}>`,
  });
  
  const blob = new Blob(chunks);
  return blob;
}

// Format file size to human-readable format
function formatFileSize(bytes) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/** 用于下载已经播放的视频内容 */
async function __wx_channels_download(profile, filename) {
  console.log("__wx_channels_download");
  const data = profile.data;
  const blob = new Blob(data, { type: "video/mp4" });
  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js"
  );
  saveAs(blob, filename + ".mp4");
}
/** 下载非加密视频 */
async function __wx_channels_download2(profile, filename) {
  console.log("__wx_channels_download2");
  const url = profile.url;

  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js"
  );
  const ins = __wx_channel_loading();
  ins.hide(); // Hide the default loader as we have our own progress UI
  
  const response = await fetch(url);
  const blob = await show_progress_or_loaded_size(response);
  saveAs(blob, filename + ".mp4");
}
/** 下载图片视频 */
async function __wx_channels_download3(profile, filename) {
  console.log("__wx_channels_download3");
  const files = profile.files;
  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js"
  );
  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/jszip.min.js"
  );
  const zip = new JSZip();
  zip.file("contact.txt", JSON.stringify(profile.contact, null, 2));
  const folder = zip.folder("images");
  console.log("files", files)
  const fetchPromises = files
    .map((f) => f.url)
    .map(async (url, index) => {
      const response = await fetch(url);
      const blob = await response.blob();
      folder.file(index + 1 + ".png", blob);
    });
  const ins = __wx_channel_loading();
  try {
    await Promise.all(fetchPromises);
    const content = await zip.generateAsync({ type: "blob" });
    ins.hide();
    saveAs(content, filename + ".zip");
  } catch (err) {
    __wx_log({
      msg: "下载失败\n" + err.message,
    });
  }
}
/** 下载加密视频 */
async function __wx_channels_download4(profile, filename) {
  console.log("__wx_channels_download4");
  const url = profile.url;

  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js"
  );
  const ins = __wx_channel_loading();
  ins.hide(); // Hide the default loader as we have our own progress UI
  
  // 如果有key但没有decryptor_array，先生成解密数组
  if (profile.key && !profile.decryptor_array) {
    console.log('🔑 检测到加密key，正在生成解密数组...');
    try {
      profile.decryptor_array = await __wx_channels_decrypt(profile.key);
      console.log('✓ 解密数组生成成功，长度:', profile.decryptor_array?.length);
    } catch (err) {
      console.error('✗ 解密数组生成失败:', err);
      throw new Error('解密数组生成失败: ' + err.message);
    }
  }
  
  const response = await fetch(url);
  const blob = await show_progress_or_loaded_size(response);
  
  // Show decryption progress
  const decryptProgressBarId = `decrypt-progress-${Date.now()}`;
  const decryptProgressHTML = `
    <div id="${decryptProgressBarId}" style="position: fixed; top: 20px; left: 50%; transform: translateX(-50%); z-index: 10000; background: rgba(0,0,0,0.7); border-radius: 8px; padding: 10px 15px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); color: white; font-size: 14px; min-width: 250px; text-align: center;">
      <div style="margin-bottom: 8px; font-weight: bold;">视频解密中</div>
      <div class="progress-container" style="background: rgba(255,255,255,0.2); height: 10px; border-radius: 5px; overflow: hidden; margin-bottom: 8px;">
        <div class="progress-bar" style="background: #07c160; height: 100%; width: 100%; animation: pulse 1.5s infinite linear;"></div>
      </div>
      <div class="progress-text">正在解密视频...</div>
      <style>
        @keyframes pulse {
          0% { opacity: 0.6; }
          50% { opacity: 1; }
          100% { opacity: 0.6; }
        }
      </style>
    </div>
  `;
  
  const decryptProgressContainer = document.createElement('div');
  decryptProgressContainer.innerHTML = decryptProgressHTML;
  document.body.appendChild(decryptProgressContainer.firstElementChild);
  
  let array = new Uint8Array(await blob.arrayBuffer());
  if (profile.decryptor_array) {
    console.log('🔐 开始解密视频，视频大小:', array.length, 'bytes');
    array = __wx_channels_video_decrypt(array, 0, profile);
    console.log('✓ 视频解密完成');
  } else {
    console.warn('⚠️ 没有解密数组，视频可能无法播放');
  }
  
  // Remove decrypt progress bar
  const decryptElement = document.getElementById(decryptProgressBarId);
  if (decryptElement) {
    decryptElement.remove();
  }
  
  // Show completion notification
  const completionNoticeId = `completion-${Date.now()}`;
  const completionHTML = `
    <div id="${completionNoticeId}" style="position: fixed; top: 20px; left: 50%; transform: translateX(-50%); z-index: 10000; background: rgba(0,0,0,0.7); border-radius: 8px; padding: 10px 15px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); color: white; font-size: 14px; text-align: center;">
      <div style="display: flex; align-items: center; justify-content: center; margin-bottom: 5px;">
        <svg viewBox="0 0 1024 1024" width="20" height="20" style="margin-right: 5px; fill: #07c160;">
          <path d="M512 64C264.6 64 64 264.6 64 512s200.6 448 448 448 448-200.6 448-448S759.4 64 512 64zm193.5 301.7l-210.6 292a31.8 31.8 0 0 1-51.7 0L318.5 484.9c-3.8-5.3 0-12.7 6.5-12.7h46.9c10.2 0 19.9 4.9 25.9 13.3l71.2 98.8 157.2-218c6-8.3 15.6-13.3 25.9-13.3H699c6.5 0 10.3 7.4 6.5 12.7z"></path>
        </svg>
        <span>视频已准备就绪</span>
      </div>
      <div style="font-size: 12px;">即将开始下载...</div>
    </div>
  `;
  
  const completionContainer = document.createElement('div');
  completionContainer.innerHTML = completionHTML;
  document.body.appendChild(completionContainer.firstElementChild);
  
  // Auto remove completion notice after 2 seconds
  setTimeout(() => {
    const notice = document.getElementById(completionNoticeId);
    if (notice) {
      notice.style.opacity = '0';
      notice.style.transition = 'opacity 0.5s';
      setTimeout(() => notice.remove(), 500);
    }
  }, 3000);
  
  const result = new Blob([array], { type: "video/mp4" });
  saveAs(result, filename + ".mp4");
}
function __wx_load_script(src) {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.type = "text/javascript";
    script.src = src;
    script.onload = resolve;
    script.onerror = reject;
    document.head.appendChild(script);
  });
}
function __wx_channels_handle_copy__() {
  __wx_channels_copy(location.href);
  if (window.__wx_channels_tip__ && window.__wx_channels_tip__.toast) {
    window.__wx_channels_tip__.toast("复制成功", 1e3);
  }
}
async function __wx_channels_handle_log__() {
  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js"
  );
  const content = document.body.innerHTML;
  const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
  saveAs(blob, "log.txt");
}
async function __wx_channels_handle_click_download__(spec) {
  var profile = __wx_channels_store__.profile;
  // profile = __wx_channels_store__.profiles.find((p) => p.id === profile.id);
  if (!profile) {
    alert("检测不到视频，请将本工具更新到最新版");
    return;
  }
  // console.log(__wx_channels_store__);
  var filename = (() => {
    if (profile.title) {
      return profile.title;
    }
    if (profile.id) {
      return profile.id;
    }
    return new Date().valueOf();
  })();
  const _profile = {
    ...profile,
  };
  if (spec) {
    _profile.url = profile.url + "&X-snsvideoflag=" + spec.fileFormat;
    // 添加分辨率信息到文件名中
    let qualityInfo = spec.fileFormat;
    if (spec.width && spec.height) {
      qualityInfo += `_${spec.width}x${spec.height}`;
    }
    filename = filename + "_" + qualityInfo;
  }
  // console.log("__wx_channels_handle_click_download__", url);
  __wx_log({
    msg: `下载文件名<${filename}>`,
  });
  __wx_log({
    msg: `页面链接<${location.href}>`,
  });
  __wx_log({
    msg: `视频链接<${_profile.url}>`,
  });
  __wx_log({
    msg: `视频密钥<${_profile.key || ""}>`,
  });
  if (_profile.type === "picture") {
    __wx_channels_download3(_profile, filename);
    return;
  }
  if (!_profile.key) {
    __wx_channels_download2(_profile, filename);
    return;
  }
  _profile.data = __wx_channels_store__.buffers;
  try {
    const r = await __wx_channels_decrypt(_profile.key);
    // console.log("[]after __wx_channels_decrypt", r);
    _profile.decryptor_array = r;
  } catch (err) {
    __wx_log({
      msg: `解密失败，停止下载`,
    });
    alert("解密失败，停止下载");
    return;
  }
  __wx_channels_download4(_profile, filename);
}
async function __wx_channels_download_cur__() {
  var profile = __wx_channels_store__.profile;
  if (!profile) {
    alert("检测不到视频，请将本工具更新到最新版");
    return;
  }
  
  var filename = (() => {
    if (profile.title) {
      return profile.title;
    }
    if (profile.id) {
      return profile.id;
    }
    return new Date().valueOf();
  })();
  
  // 使用当前视频的URL和规格信息下载，而不是缓存的buffers
  const _profile = {
    ...profile,
  };
  
  // 使用第一个可用的规格（通常是默认质量）
  if (profile.spec && profile.spec.length > 0) {
    _profile.url = profile.url + "&X-snsvideoflag=" + profile.spec[0].fileFormat;
    // 添加分辨率信息到文件名中
    let qualityInfo = profile.spec[0].fileFormat;
    if (profile.spec[0].width && profile.spec[0].height) {
      qualityInfo += `_${profile.spec[0].width}x${profile.spec[0].height}`;
    }
    filename = filename + "_" + qualityInfo;
  }
  
  __wx_log({
    msg: `下载当前视频<${filename}>`,
  });
  __wx_log({
    msg: `页面链接<${location.href}>`,
  });
  __wx_log({
    msg: `视频链接<${_profile.url}>`,
  });
  __wx_log({
    msg: `视频密钥<${_profile.key || ""}>`,
  });
  
  if (_profile.type === "picture") {
    __wx_channels_download3(_profile, filename);
    return;
  }
  if (!_profile.key) {
    __wx_channels_download2(_profile, filename);
    return;
  }
  _profile.data = __wx_channels_store__.buffers;
  try {
    const r = await __wx_channels_decrypt(_profile.key);
    _profile.decryptor_array = r;
  } catch (err) {
    __wx_log({
      msg: `解密失败，停止下载`,
    });
    alert("解密失败，停止下载");
    return;
  }
  __wx_channels_download4(_profile, filename);
}
async function __wx_channels_handle_download_cover() {
  var profile = __wx_channels_store__.profile;
  // profile = __wx_channels_store__.profiles.find((p) => p.id === profile.id);
  if (!profile) {
    alert("检测不到视频，请将本工具更新到最新版");
    return;
  }
  // console.log(__wx_channels_store__);
  var filename = (() => {
    if (profile.title) {
      return profile.title;
    }
    if (profile.id) {
      return profile.id;
    }
    return new Date().valueOf();
  })();
  const _profile = {
    ...profile,
  };
  await __wx_load_script(
    "https://res.wx.qq.com/t/wx_fed/cdn_libs/res/FileSaver.min.js"
  );
  __wx_log({
    msg: `下载封面\n${_profile.coverUrl}`,
  });
  const ins = __wx_channel_loading();
  try {
    const url = _profile.coverUrl.replace(/^http/, "https");
    const response = await fetch(url);
    const blob = await response.blob();
    saveAs(blob, filename + ".jpg");
  } catch (err) {
    alert(err.message);
  }
  ins.hide();
}
var __wx_channels_tip__ = {};
var __wx_channels_store__ = {
  profile: null,
  profiles: [],
  keys: {},
  buffers: [],
};

// 添加CSS样式确保下载按钮在Home页面正确显示
const downloadButtonStyles = `
  <style>
    .feed-download-icon {
      width: 28px;
      height: 28px;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    
    .feed-download-icon svg {
      width: 28px;
      height: 28px;
    }
    
    .op-text {
      font-size: 12px;
      margin-top: 6px;
    }
    
    /* 确保下载按钮在Home页面中的样式与其他操作按钮一致 */
    .click-box.op-item[aria-label="下载"] {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-width: 28px;
      cursor: pointer;
      transition: opacity 0.2s ease;
    }
    
    .click-box.op-item[aria-label="下载"]:hover {
      opacity: 0.8;
    }
  </style>
`;

// 将样式添加到页面头部
if (document.head) {
  document.head.insertAdjacentHTML('beforeend', downloadButtonStyles);
}
var $icon = document.createElement("div");
var $svg = `<svg data-v-132dee25 class="svg-icon icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" fill="currentColor" width="28" height="28"><path d="M213.333333 853.333333h597.333334v-85.333333H213.333333m597.333334-384h-170.666667V128H384v256H213.333333l298.666667 298.666667 298.666667-298.666667z"></path></svg>`;
$icon.innerHTML = `<div class=""><div data-v-6548f11a data-v-1fe2ed37 class="click-box op-item download-icon" role="button" aria-label="下载" style="padding: 4px 4px 4px 4px; --border-radius: 4px; --left: 0; --top: 0; --right: 0; --bottom: 0;">${$svg}<div data-v-1fe2ed37 class="op-text">下载</div></div></div>`;
var __wx_channels_video_download_btn__ = $icon.firstChild;
__wx_channels_video_download_btn__.onclick = () => {
  if (!window.__wx_channels_store__.profile) {
    return;
  }
  __wx_channels_handle_click_download__(
    window.__wx_channels_store__.profile.spec[0]
  );
};
var count = 0;
fetch("/__wx_channels_api/tip", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    msg: "等待添加下载按钮",
  }),
});
// 等待元素加载的辅助函数
function findElm(fn, timeout = 5000) {
  return new Promise((resolve) => {
    const startTime = Date.now();
    const check = () => {
      const elm = fn();
      if (elm) {
        resolve(elm);
      } else if (Date.now() - startTime > timeout) {
        resolve(null);
      } else {
        setTimeout(check, 100);
      }
    };
    check();
  });
}

// 专门针对Home页面的下载按钮插入函数（参考GitHub原项目实现）
async function __insert_download_btn_to_home_page() {
  var $container = await findElm(function () {
    return document.querySelector(".slides-scroll");
  });
  if (!$container) {
    return false;
  }
  var cssText = $container.style.cssText;
  var re = /translate3d\([0-9]{1,}px, {0,1}-{0,1}([0-9]{1,})%/;
  var matched = cssText.match(re);
  var idx = matched ? Number(matched[1]) / 100 : 0;
  console.log('[]idx', idx);
  var $item = document.querySelectorAll(".slides-item")[idx];
  if (!$item) {
    return false;
  }
  var $existing_download_btn = $item.querySelector(".download-icon");
  if ($existing_download_btn) {
    return true;
  }
  var $elm3 = await findElm(function () {
    return $item.getElementsByClassName("click-box op-item")[0];
  });
  if (!$elm3) {
    return false;
  }
  const $parent = $elm3.parentElement;
  if ($parent) {
    // 使用SVG图标而不是Base64图标
    var $icon = document.createElement("div");
    var $svg = `<svg data-v-132dee25 class="svg-icon icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" fill="currentColor" width="28" height="28"><path d="M213.333333 853.333333h597.333334v-85.333333H213.333333m597.333334-384h-170.666667V128H384v256H213.333333l298.666667 298.666667 298.666667-298.666667z"></path></svg>`;
    $icon.innerHTML = `<div class=""><div data-v-6548f11a data-v-1fe2ed37 class="click-box op-item download-icon" role="button" aria-label="下载" style="padding: 4px 4px 4px 4px; --border-radius: 4px; --left: 0; --top: 0; --right: 0; --bottom: 0;">${$svg}<div data-v-1fe2ed37 class="op-text">下载</div></div></div>`;
    __wx_channels_video_download_btn__ = $icon.firstChild;
    __wx_channels_video_download_btn__.onclick = () => {
      // 等待数据采集完成（最多等待3秒，每100ms检查一次）
      var checkCount = 0;
      var maxChecks = 30;
      
      var checkData = () => {
        if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
          var profile = window.__wx_channels_store__.profile;
          if (profile.key && window.__wx_channels_store__.buffers.length === 0) {
            __wx_log({
              msg: '⏳ 视频尚未缓存完成\n请等待视频播放一段时间后再下载\n或者切换到视频详情页进行下载',
            });
            return;
          }
          
          // 显示下载选项菜单
          __show_home_download_options(profile);
        } else {
          checkCount++;
          if (checkCount < maxChecks) {
            // 继续等待
            setTimeout(checkData, 100);
            if (checkCount === 1) {
              __wx_log({
                msg: '⏳ 正在获取视频数据，请稍候...',
              });
            }
          } else {
            // 超时
            __wx_log({
              msg: '❌ 获取视频数据超时\n请重新滑动视频或刷新页面',
            });
          }
        }
      };
      
      checkData();
    };
    $parent.appendChild(__wx_channels_video_download_btn__);
    __wx_log({
      msg: "注入下载按钮成功!",
    });
    return true;
  }
  return false;
}

// 全局变量：记录上次的幻灯片索引
var __last_slide_index__ = -1;
var __home_slide_observer__ = null;
// 全局变量：标记首次加载状态
var __home_first_load__ = true;

// 监听幻灯片切换，自动重新注入下载按钮
function __start_home_slide_monitor() {
  var $container = document.querySelector(".slides-scroll");
  if (!$container) {
    console.log("未找到slides-scroll容器，无法启动监听");
    return;
  }
  
  console.log("✅ 启动Home页面幻灯片切换监听器");
  
  // 使用MutationObserver监听style属性变化
  __home_slide_observer__ = new MutationObserver(function(mutations) {
    mutations.forEach(function(mutation) {
      if (mutation.type === 'attributes' && mutation.attributeName === 'style') {
        var cssText = $container.style.cssText;
        var re = /translate3d\([0-9]{1,}px, {0,1}-{0,1}([0-9]{1,})%/;
        var matched = cssText.match(re);
        var idx = matched ? Number(matched[1]) / 100 : 0;
        
        // 如果索引变化，说明切换了幻灯片
        if (idx !== __last_slide_index__) {
          console.log('检测到幻灯片切换:', __last_slide_index__, '->', idx);
          
          // 🎯 首次滑动特殊处理：触发首屏数据采集
          if (__home_first_load__) {
            __home_first_load__ = false;
            console.log('🎯 检测到首次滑动，触发首屏数据采集...');
            
            // 如果用户向下滑动（从0到1），先采集首屏数据
            if (__last_slide_index__ === 0 && idx === 1) {
              console.log('📹 用户向下滑动，将在返回时采集首屏数据');
              // 提示用户可以返回首屏
              setTimeout(function() {
                if (idx === 1 && !window.__wx_channels_store__.profile) {
                  console.log('💡 提示：向上滑动可返回首屏并采集数据');
                }
              }, 1000);
            }
            // 如果用户向上滑动（从0到-1），说明从首屏向上
            else if (__last_slide_index__ === 0 && idx === -1) {
              console.log('📹 用户向上滑动，将在返回时采集首屏数据');
            }
          }
          
          __last_slide_index__ = idx;
          
          // 缩短延迟到200ms，加快按钮注入速度
          setTimeout(() => {
            __insert_download_btn_to_home_page();
          }, 200);
        }
      }
    });
  });
  
  // 开始观察
  __home_slide_observer__.observe($container, {
    attributes: true,
    attributeFilter: ['style']
  });
  
  // 记录初始索引
  var cssText = $container.style.cssText;
  var re = /translate3d\([0-9]{1,}px, {0,1}-{0,1}([0-9]{1,})%/;
  var matched = cssText.match(re);
  __last_slide_index__ = matched ? Number(matched[1]) / 100 : 0;
}

// 统一的按钮插入函数（参考GitHub原项目实现）
async function insert_download_btn() {
  __wx_log({
    msg: "等待注入下载按钮",
  });
  
  // 1. 尝试Feed页面的横向布局
  var $elm1 = await findElm(function () {
    return document.getElementsByClassName("full-opr-wrp layout-row")[0];
  });
  if ($elm1) {
    var relative_node = $elm1.children[$elm1.children.length - 1];
    if (!relative_node) {
      __wx_log({
        msg: "注入下载按钮成功1!",
      });
      $elm1.appendChild(__wx_channels_video_download_btn__);
      return;
    }
    __wx_log({
      msg: "注入下载按钮成功2!",
    });
    $elm1.insertBefore(__wx_channels_video_download_btn__, relative_node);
    return;
  }
  
  // 2. 尝试Feed页面的纵向布局
  var $elm2 = await findElm(function () {
    return document.getElementsByClassName("full-opr-wrp layout-col")[0];
  });
  if ($elm2) {
    // 使用与Home页和横向布局相同的样式
    var $icon2 = document.createElement("div");
    var $svg2 = `<svg data-v-132dee25 class="svg-icon icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" fill="currentColor" width="28" height="28"><path d="M213.333333 853.333333h597.333334v-85.333333H213.333333m597.333334-384h-170.666667V128H384v256H213.333333l298.666667 298.666667 298.666667-298.666667z"></path></svg>`;
    $icon2.innerHTML = `<div class=""><div data-v-6548f11a data-v-1fe2ed37 class="click-box op-item download-icon" role="button" aria-label="下载" style="padding: 4px 4px 4px 4px; --border-radius: 4px; --left: 0; --top: 0; --right: 0; --bottom: 0;">${$svg2}<div data-v-1fe2ed37 class="op-text">下载</div></div></div>`;
    __wx_channels_video_download_btn__ = $icon2.firstChild;
    __wx_channels_video_download_btn__.onclick = () => {
      if (!window.__wx_channels_store__.profile) {
        return;
      }
      __wx_channels_handle_click_download__(
        window.__wx_channels_store__.profile.spec[0]
      );
    };
    var relative_node = $elm2.children[$elm2.children.length - 1];
  if (!relative_node) {
      __wx_log({
        msg: "注入下载按钮成功3!",
      });
      $elm2.appendChild(__wx_channels_video_download_btn__);
      return;
    }
    __wx_log({
      msg: "注入下载按钮成功4!",
    });
    $elm2.insertBefore(__wx_channels_video_download_btn__, relative_node);
    return;
  }
  
  // 3. 尝试Home页面的幻灯片布局
  var success = await __insert_download_btn_to_home_page();
  if (success) {
    // 启动幻灯片切换监听器
    setTimeout(() => {
      __start_home_slide_monitor();
      
      // 下载按钮注入成功后，延迟1秒执行首屏数据自动采集
      // console.log("✅ 下载按钮注入成功，准备自动采集首屏数据...");
      setTimeout(function() {
        __try_capture_initial_home_data();
      }, 1000);
    }, 500);
    return;
  }
  
  __wx_log({
    msg: "没有找到操作栏，注入下载按钮失败\n",
  });
}

// Home页面首次加载自动采集（由按钮注入成功后调用）
function __try_capture_initial_home_data() {
  try {
    var isHomePage = window.location.pathname.includes('/pages/home');
    if (!isHomePage) return;
    
    // 检查是否还是首次加载状态
    if (!__home_first_load__ || !window.__wx_channels_store__ || window.__wx_channels_store__.profile) {
      return;
    }
    
    // __wx_log({ msg: "🎯 [静默采集] 开始首屏视频数据采集（无感模式）..." });
    
    var container = document.querySelector('.slides-scroll');
    if (!container) {
      // __wx_log({ msg: "⚠️  未找到容器，1秒后重试..." });
      setTimeout(__try_capture_initial_home_data, 1000);
      return;
    }
    
    // 保存原始样式
    var originalTransform = container.style.transform;
    var originalTransition = container.style.transition;
    var originalVisibility = container.style.visibility;
    
    // 临时隐藏容器（用户看不见）
    container.style.visibility = 'hidden';
    container.style.transition = 'none';
    
    // __wx_log({ msg: "⬇️  [无感模式] 触发数据请求（用户不可见）..." });
    
    // 创建键盘事件触发数据请求
    var downEvent = new KeyboardEvent('keydown', {
      key: 'ArrowDown',
      code: 'ArrowDown',
      keyCode: 40,
      which: 40,
      bubbles: true,
      cancelable: true,
      view: window
    });
    
    // 触发事件（触发数据请求，但视觉上不可见）
    document.dispatchEvent(downEvent);
    
    // 等待数据请求完成
    setTimeout(function() {
      // 触发返回事件
      var upEvent = new KeyboardEvent('keydown', {
        key: 'ArrowUp',
        code: 'ArrowUp',
        keyCode: 38,
        which: 38,
        bubbles: true,
        cancelable: true,
        view: window
      });
      
      document.dispatchEvent(upEvent);
      
      // 再等待数据采集
      setTimeout(function() {
        // 恢复原始样式（用户完全无感知）
        container.style.transform = originalTransform;
        container.style.transition = originalTransition;
        container.style.visibility = originalVisibility;
        
        // 验证结果
        if (window.__wx_channels_store__.profile) {
          // __wx_log({ msg: "✅ [无感采集成功] 首屏数据已静默采集完成！" });
        } else {
          // __wx_log({ msg: "⚠️  [无感采集失败] 尝试备用方案..." });
          // 恢复显示后再试
          setTimeout(__try_capture_by_dom_silent, 500);
        }
      }, 1000);
}, 1000);
    
  } catch (e) {
    // __wx_log({ msg: "❌ [自动采集失败] " + e.message });
    console.error("[自动采集失败]", e);
  }
}

// 备用方法：静默DOM操作
function __try_capture_by_dom_silent() {
  var container = document.querySelector('.slides-scroll');
  if (!container) {
    __wx_log({ msg: "⚠️  容器不存在" });
    return;
  }
  
  __wx_log({ msg: "🔄 [备用方案] 使用DOM静默操作..." });
  
  // 保存原始样式
  var originalTransform = container.style.transform;
  var originalTransition = container.style.transition;
  var originalPointerEvents = container.style.pointerEvents;
  
  // 禁用交互和动画
  container.style.pointerEvents = 'none';
  container.style.transition = 'none';
  
  // 快速切换（用户几乎看不到，只有1帧）
  container.style.transform = 'translate3d(0px, -100%, 0px)';
  
  // 立即返回（20ms）
  setTimeout(function() {
    container.style.transform = originalTransform;
    
    // 恢复原始状态
    setTimeout(function() {
      container.style.transition = originalTransition;
      container.style.pointerEvents = originalPointerEvents;
      
      if (window.__wx_channels_store__.profile) {
        // __wx_log({ msg: "✅ [备用方案成功] 静默采集完成！" });
      } else {
        // __wx_log({ msg: "⚠️  静默采集失败，建议手动滑动一次" });
      }
    }, 100);
  }, 20);
}

// 旧的DOM方法（保留用于非静默场景）
function __try_capture_by_dom() {
  var container = document.querySelector('.slides-scroll');
  if (!container) {
    __wx_log({ msg: "⚠️  未找到幻灯片容器，1秒后重试..." });
    setTimeout(__try_capture_initial_home_data, 1000);
    return;
  }
  
  // 修改为下一页
  container.style.transform = 'translate3d(0px, -100%, 0px)';
  container.style.transitionDuration = '300ms';
  
  // 等待1500ms返回
  setTimeout(function() {
    container.style.transform = 'translate3d(0px, 0%, 0px)';
    container.style.transitionDuration = '300ms';
    
    // 验证结果
    setTimeout(function() {
      if (window.__wx_channels_store__.profile) {
        // __wx_log({ msg: "✅ [方法2成功] DOM操作方式采集首屏数据完成！" });
      } else {
        // __wx_log({ msg: "⚠️  [方法2失败] 请手动向下滑动一次，再返回首页" });
      }
    }, 1500);
  }, 1500);
}

// 调试：检测页面事件监听器
function __debug_event_listeners() {
  setTimeout(function() {
    try {
      var container = document.querySelector('.slides-scroll');
      if (!container) return;
      
      console.log("=== 页面原生事件监听器分析 ===");
      
      // 检测各种事件监听
      var events = ['keydown', 'keyup', 'wheel', 'touchstart', 'touchmove', 'touchend'];
      
      // 尝试触发并监听事件
      var detectedEvents = [];
      events.forEach(function(eventType) {
        var hasListener = false;
        try {
          var testEvent = new Event(eventType, { bubbles: true, cancelable: true });
          var originalPrevent = testEvent.preventDefault;
          testEvent.preventDefault = function() {
            hasListener = true;
            originalPrevent.call(this);
          };
          container.dispatchEvent(testEvent);
          document.dispatchEvent(testEvent);
          if (hasListener) {
            detectedEvents.push(eventType);
          }
        } catch(e) {}
      });
      
      if (detectedEvents.length > 0) {
        console.log("✅ 检测到的事件监听器:", detectedEvents.join(', '));
        __wx_log({ msg: "📊 [页面分析] 检测到事件监听: " + detectedEvents.join(', ') });
      }
      
      // 查找Vue组件实例
      var vueInstance = container.__vnode;
      if (vueInstance) {
        console.log("✅ 找到Vue实例");
        __wx_log({ msg: "📊 [页面分析] 使用Vue 3框架，通过响应式系统管理状态" });
      }
      
      // 检测transform变化监听
      var hasObserver = container.__vue_observer__ || container.__ob__;
      if (hasObserver) {
        console.log("✅ 检测到响应式观察器");
      }
      
    } catch (e) {
      console.error("调试失败:", e);
    }
  }, 3000);
}

// 使用setTimeout延迟执行，而不是setInterval
setTimeout(async () => {
  insert_download_btn();
  // __try_capture_initial_home_data 将在按钮注入成功后自动调用
  
  // 启用调试（仅在开发时）
  // __debug_event_listeners();
}, 800);

// 修改FeedDetail.publish的注入代码，在main.go中需要更新以下内容:
// 原来的:
// return f("div",{class:"context-item",role:"button",onClick:() => __wx_channels_handle_click_download__(sp)},sp.fileFormat);
// 修改为:
// 添加一个函数来格式化显示质量选项
function __wx_format_quality_option(spec) {
  let label = spec.fileFormat;
  
  // 显示分辨率信息（如果可用）
  if (spec.width && spec.height) {
    label += ` (${spec.width}×${spec.height})`;
  }
  
  // 显示文件大小信息（如果可用）
  if (spec.fileSize) {
    const sizeMB = (spec.fileSize / (1024 * 1024)).toFixed(1);
    label += ` - ${sizeMB}MB`;
  }
  
  return label;
}

// Home页面下载选项菜单显示函数
function __show_home_download_options(profile) {
  // 移除已存在的菜单
  var existingMenu = document.querySelector('.home-download-menu');
  if (existingMenu) {
    existingMenu.remove();
  }
  
  // 创建菜单容器
  var menu = document.createElement('div');
  menu.className = 'home-download-menu';
  menu.style.cssText = `
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: rgba(0, 0, 0, 0.92);
    border-radius: 8px;
    padding: 12px;
    z-index: 10000;
    min-width: 200px;
    max-width: 85vw;
    color: white;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);
    backdrop-filter: blur(10px);
  `;
  
  // 创建菜单标题
  var title = document.createElement('div');
  title.style.cssText = `
    font-size: 13px;
    font-weight: bold;
    margin-bottom: 10px;
    text-align: center;
    color: #07c160;
  `;
  title.textContent = '选择下载选项';
  menu.appendChild(title);
  
  // 创建选项列表
  var optionsList = document.createElement('div');
  optionsList.style.cssText = `
    display: flex;
    flex-direction: column;
    gap: 5px;
  `;
  
  // 添加各种视频格式选项
  if (profile.spec && profile.spec.length > 0) {
    profile.spec.forEach(function(spec, index) {
      var option = document.createElement('div');
      option.style.cssText = `
        padding: 8px 12px;
        background: rgba(255, 255, 255, 0.1);
        border-radius: 6px;
        cursor: pointer;
        transition: all 0.2s ease;
        border: 1px solid rgba(255, 255, 255, 0.2);
        font-size: 12px;
      `;
      option.textContent = __wx_format_quality_option(spec);
      
      option.addEventListener('mouseenter', function() {
        this.style.background = 'rgba(7, 193, 96, 0.2)';
        this.style.borderColor = '#07c160';
      });
      
      option.addEventListener('mouseleave', function() {
        this.style.background = 'rgba(255, 255, 255, 0.1)';
        this.style.borderColor = 'rgba(255, 255, 255, 0.2)';
      });
      
      option.addEventListener('click', function() {
        __wx_channels_handle_click_download__(spec);
        menu.remove();
      });
      
      optionsList.appendChild(option);
    });
  }
  
  // 添加原始视频选项
  var originalOption = document.createElement('div');
  originalOption.style.cssText = `
    padding: 8px 12px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s ease;
    border: 1px solid rgba(255, 255, 255, 0.2);
    font-size: 12px;
  `;
  originalOption.textContent = '原始视频';
  
  originalOption.addEventListener('mouseenter', function() {
    this.style.background = 'rgba(7, 193, 96, 0.2)';
    this.style.borderColor = '#07c160';
  });
  
  originalOption.addEventListener('mouseleave', function() {
    this.style.background = 'rgba(255, 255, 255, 0.1)';
    this.style.borderColor = 'rgba(255, 255, 255, 0.2)';
  });
  
  originalOption.addEventListener('click', function() {
    __wx_channels_handle_click_download__();
    menu.remove();
  });
  
  optionsList.appendChild(originalOption);
  
  // 添加当前视频选项
  var currentOption = document.createElement('div');
  currentOption.style.cssText = `
    padding: 8px 12px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s ease;
    border: 1px solid rgba(255, 255, 255, 0.2);
    font-size: 12px;
  `;
  currentOption.textContent = '当前视频';
  
  currentOption.addEventListener('mouseenter', function() {
    this.style.background = 'rgba(7, 193, 96, 0.2)';
    this.style.borderColor = '#07c160';
  });
  
  currentOption.addEventListener('mouseleave', function() {
    this.style.background = 'rgba(255, 255, 255, 0.1)';
    this.style.borderColor = 'rgba(255, 255, 255, 0.2)';
  });
  
  currentOption.addEventListener('click', function() {
    __wx_channels_download_cur__();
    menu.remove();
  });
  
  optionsList.appendChild(currentOption);
  
  // 添加下载封面选项
  var coverOption = document.createElement('div');
  coverOption.style.cssText = `
    padding: 8px 12px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s ease;
    border: 1px solid rgba(255, 255, 255, 0.2);
    font-size: 12px;
  `;
  coverOption.textContent = '下载封面';
  
  coverOption.addEventListener('mouseenter', function() {
    this.style.background = 'rgba(7, 193, 96, 0.2)';
    this.style.borderColor = '#07c160';
  });
  
  coverOption.addEventListener('mouseleave', function() {
    this.style.background = 'rgba(255, 255, 255, 0.1)';
    this.style.borderColor = 'rgba(255, 255, 255, 0.2)';
  });
  
  coverOption.addEventListener('click', function() {
    __wx_channels_handle_download_cover();
    menu.remove();
  });
  
  optionsList.appendChild(coverOption);
  
  // 添加关闭按钮
  var closeButton = document.createElement('div');
  closeButton.style.cssText = `
    margin-top: 8px;
    padding: 7px 12px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    cursor: pointer;
    text-align: center;
    transition: all 0.2s ease;
    border: 1px solid rgba(255, 255, 255, 0.2);
    font-size: 12px;
  `;
  closeButton.textContent = '取消';
  
  closeButton.addEventListener('mouseenter', function() {
    this.style.background = 'rgba(255, 0, 0, 0.2)';
    this.style.borderColor = '#ff4444';
  });
  
  closeButton.addEventListener('mouseleave', function() {
    this.style.background = 'rgba(255, 255, 255, 0.1)';
    this.style.borderColor = 'rgba(255, 255, 255, 0.2)';
  });
  
  closeButton.addEventListener('click', function() {
    menu.remove();
  });
  
  // 组装菜单
  menu.appendChild(optionsList);
  menu.appendChild(closeButton);
  
  // 添加背景遮罩
  var overlay = document.createElement('div');
  overlay.style.cssText = `
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.5);
    z-index: 9999;
  `;
  
  overlay.addEventListener('click', function() {
    menu.remove();
    overlay.remove();
  });
  
  // 添加到页面
  document.body.appendChild(overlay);
  document.body.appendChild(menu);
  
  // 添加ESC键关闭功能
  var escHandler = function(e) {
    if (e.key === 'Escape') {
      menu.remove();
      overlay.remove();
      document.removeEventListener('keydown', escHandler);
    }
  };
  document.addEventListener('keydown', escHandler);
}

// ==================== Profile页面视频列表批量下载功能 ====================

// 检测是否为profile页面
function is_profile_page() {
  return window.location.pathname.includes('/pages/profile');
}

// Profile页面视频列表采集器
window.__wx_channels_profile_collector = {
  videos: [],
  isCollecting: false,
  batchDownloading: false,
  downloadProgress: { current: 0, total: 0 },
  _serverProgressInterval: null, // 后端下载进度轮询定时器
  _forceRedownload: false, // 是否强制重新下载（取消后自动启用）
  _statusMessageTimeout: null, // 状态信息自动隐藏定时器
  
  // 初始化profile页面功能
  init: function() {
    if (!is_profile_page()) return;
    
    // 发送初始化日志到后端
    fetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({msg: '🎯 [主页页面] 初始化视频列表采集器'})
    }).catch(() => {});
    
    // 检查并加载临时存储的视频数据
    if (window.__wx_channels_temp_profiles && window.__wx_channels_temp_profiles.length > 0) {
      const tempCount = window.__wx_channels_temp_profiles.length;
      console.log('📦 发现临时存储的视频数据，数量:', tempCount);
      
      // 直接批量添加，不触发每次的UI更新（提高性能）
      window.__wx_channels_temp_profiles.forEach(profile => {
        if (profile && profile.id && !this.videos.some(v => v.id === profile.id)) {
          this.videos.push(profile);
        }
      });
      
      // 清空临时存储
      window.__wx_channels_temp_profiles = [];
      
      const msg = `📦 [主页采集] 从临时存储加载了 ${this.videos.length} 个视频`;
      console.log(msg);
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: msg})
      }).catch(() => {});
    }
    
    // 延迟启动，等待页面加载完成
    setTimeout(() => {
      this.collectVideosFromPage();
      this.addBatchDownloadUI();
      this.setupScrollListener();
      
      // UI创建后立即更新显示（如果之前已有采集到的视频）
      if (this.videos.length > 0) {
        console.log(`📊 UI创建完成，立即更新显示 ${this.videos.length} 个已采集视频`);
        setTimeout(() => {
          this.updateBatchDownloadUI();
        }, 100);
      }
    }, 2000);
  },
  
  // 分片上传实现
  uploadInChunks: async function(videoData, finalFilename, authorName) {
    const CHUNK_SIZE = 2 * 1024 * 1024; // 2MB
    const total = Math.ceil(videoData.byteLength / CHUNK_SIZE);
    const sizeMB = (videoData.byteLength / 1024 / 1024).toFixed(2);
    
    // 发送到后端显示
    fetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ msg: `📦 [分片上传] ${finalFilename.substring(0, 30)}... | 总大小: ${sizeMB}MB, 分片数: ${total}` })
    }).catch(() => {});

    // 初始化（带重试与错误输出）
    let uploadId = '';
    for (let attempt = 1; attempt <= 3 && !uploadId; attempt++) {
      try {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 30000);
        const initResp = await fetch('/__wx_channels_api/init_upload', { method: 'POST', signal: controller.signal });
        clearTimeout(timeout);
        const text = await initResp.text();
        
        if (!initResp.ok) {
          throw new Error(`HTTP ${initResp.status}: ${text}`);
        }
        
        let initJson;
        try { 
          initJson = JSON.parse(text); 
        } catch (parseError) {
          throw new Error(`JSON解析失败: ${parseError.message}`);
        }
        
        if (initJson && initJson.success && initJson.uploadId) {
          uploadId = initJson.uploadId;
          break;
        }
        
        const msg = initJson && initJson.error ? initJson.error : `响应格式错误`;
        if (attempt === 3) throw new Error(`init_upload 失败: ${msg}`);
        await new Promise(r => setTimeout(r, attempt * 1000));
      } catch (e) {
        if (attempt === 3) throw new Error(`init_upload 失败: ${e && e.message ? e.message : e}`);
        await new Promise(r => setTimeout(r, attempt * 1000));
      }
    }

    // 逐片上传（每上传5片报告一次进度）
    for (let i = 0; i < total; i++) {
      const start = i * CHUNK_SIZE;
      const end = Math.min(videoData.byteLength, start + CHUNK_SIZE);
      const chunk = videoData.slice(start, end);
      
      // 每5片或最后一片报告进度到后端
      if ((i + 1) % 5 === 0 || i === total - 1) {
        const progress = ((i + 1) / total * 100).toFixed(0);
        fetch('/__wx_channels_api/tip', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ msg: `📤 [上传进度] ${finalFilename.substring(0, 25)}... | ${i + 1}/${total} (${progress}%)` })
        }).catch(() => {});
      }

      // 每片重试最多3次
      let ok = false;
      for (let attempt = 1; attempt <= 3 && !ok; attempt++) {
        try {
          // 重要：每次重试都要重新构建 FormData（避免复用已消费的流）
          const form = new FormData();
          form.append('uploadId', uploadId);
          form.append('index', String(i));
          form.append('total', String(total));
          form.append('chunk', new Blob([chunk], { type: 'application/octet-stream' }));

          const controller = new AbortController();
          const timeout = setTimeout(() => controller.abort(), 60000); // 60s/片
          const r = await fetch('/__wx_channels_api/upload_chunk', { method: 'POST', body: form, signal: controller.signal });
          clearTimeout(timeout);
          const j = await r.json();
          if (!j.success) throw new Error('chunk 返回失败');
          ok = true;
        } catch (e) {
          if (attempt === 3) {
            fetch('/__wx_channels_api/tip', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ msg: `❌ [上传失败] 分片 ${i + 1}/${total} 失败` })
            }).catch(() => {});
            throw e;
          }
          await new Promise(r => setTimeout(r, attempt * 1000));
        }
      }
    }

    // 完成合并
    fetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ msg: `🔗 [合并中] ${finalFilename.substring(0, 30)}... | 正在合并 ${total} 个分片` })
    }).catch(() => {});
    
    const complete = await fetch('/__wx_channels_api/complete_upload', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ uploadId, total, filename: finalFilename, authorName })
    });
    const cj = await complete.json();
    if (!cj.success) throw new Error(cj.error || 'complete_upload 失败');
    
    // 成功完成，发送完成通知
    fetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ msg: `✅ [上传完成] ${finalFilename} | ${sizeMB}MB` })
    }).catch(() => {});
    
    return cj.path;
  },

  // 从页面采集所有视频信息
  collectVideosFromPage: function() {
    if (this.isCollecting) return;
    this.isCollecting = true;
    
    console.log('📋 [Profile页面] 开始采集视频列表...');
    
    // 尝试多种选择器来找到视频列表
    const selectors = [
      '.video-list .video-item',
      '.profile-video-list .video-card', 
      '.author-videos .video-item',
      '[class*="video"][class*="item"]',
      '[class*="video"][class*="card"]',
      '.slides-item',
      '.feed-item'
    ];
    
    let videoElements = [];
    for (const selector of selectors) {
      const elements = document.querySelectorAll(selector);
      if (elements.length > 0) {
        console.log(`✅ 找到视频元素: ${selector} (${elements.length}个)`);
        videoElements = elements;
        break;
      }
    }
    
    if (videoElements.length === 0) {
      console.log('⚠️ 未找到视频列表元素，尝试从API数据中获取');
      this.collectFromAPI();
      this.isCollecting = false;
      return;
    }
    
    // 从DOM元素提取视频信息
    this.videos = [];
    videoElements.forEach((element, index) => {
      const videoInfo = this.extractVideoInfoFromElement(element, index);
      if (videoInfo) {
        this.videos.push(videoInfo);
      }
    });
    
    console.log(`📊 [Profile页面] 采集到 ${this.videos.length} 个视频`);
    this.updateBatchDownloadUI();
    this.isCollecting = false;
  },
  
  // 从DOM元素提取视频信息
  extractVideoInfoFromElement: function(element, index) {
    try {
      // 尝试从元素中提取视频ID、标题等信息
      const titleElement = element.querySelector('[class*="title"], [class*="desc"], .video-title, .video-desc');
      const title = titleElement ? titleElement.textContent.trim() : `视频 ${index + 1}`;
      
      // 尝试从data属性或href中获取视频ID
      const videoId = element.dataset.videoId || 
                     element.dataset.id || 
                     element.querySelector('a')?.href?.match(/[?&]id=([^&]+)/)?.[1] ||
                     `profile_video_${index}`;
      
      // 尝试获取封面图片
      const coverElement = element.querySelector('img, [class*="cover"], [class*="thumb"]');
      const coverUrl = coverElement ? (coverElement.src || coverElement.dataset.src) : '';
      
      return {
        id: videoId,
        title: title,
        coverUrl: coverUrl,
        element: element,
        index: index,
        collected: false
      };
    } catch (error) {
      console.error('提取视频信息失败:', error);
      return null;
    }
  },
  
  // 从API数据中采集（备用方案）
  collectFromAPI: function() {
    // 监听网络请求，尝试从API响应中获取视频列表
    const originalFetch = window.fetch;
    window.fetch = function(...args) {
      return originalFetch.apply(this, args).then(response => {
        if (response.url.includes('author_page') || response.url.includes('profile')) {
          response.clone().json().then(data => {
            if (data && data.data && data.data.videos) {
              console.log('📡 从API获取到视频列表数据');
              window.__wx_channels_profile_collector.processAPIData(data.data.videos);
            }
          }).catch(() => {});
        }
        return response;
      });
    };
  },
  
  // 从API添加单个视频（由main.go注入的代码调用）
  addVideoFromAPI: function(videoData) {
    if (!videoData || !videoData.id) return;
    
    // 检查是否已存在
    const exists = this.videos.some(v => v.id === videoData.id);
    if (!exists) {
      this.videos.push(videoData);
      console.log(`✅ [Profile API] 新增视频: ${videoData.title?.substring(0, 30)}...`);
      
      // 每10个视频发送一次日志到后端
      if (this.videos.length % 10 === 0) {
        fetch('/__wx_channels_api/tip', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({msg: `📊 [主页采集器] 当前已采集 ${this.videos.length} 个视频`})
        }).catch(() => {});
      }
      
      // 尝试立即更新UI
      this.updateBatchDownloadUI();
      
      // 如果是第一个视频，启动周期性UI刷新（确保UI能及时显示）
      if (this.videos.length === 1 && !this._uiRefreshInterval) {
        console.log('🔄 启动周期性UI刷新');
        this._uiRefreshInterval = setInterval(() => {
          const countElement = document.getElementById('video-count');
          if (countElement && this.videos.length > 0) {
            const currentText = countElement.textContent;
            const expectedText = `已采集: ${this.videos.length} 个视频`;
            if (currentText !== expectedText) {
              countElement.textContent = expectedText;
              console.log('🔄 周期性刷新UI:', expectedText);
            }
          }
          
          // 如果采集完成（5秒内没有新视频），停止刷新
          if (this._lastVideoTime && Date.now() - this._lastVideoTime > 5000) {
            clearInterval(this._uiRefreshInterval);
            this._uiRefreshInterval = null;
            console.log('✓ 停止周期性UI刷新');
          }
        }, 500);
      }
      
      // 记录最后一次添加视频的时间
      this._lastVideoTime = Date.now();
    }
  },
  
  // 处理API数据
  processAPIData: function(videosData) {
    this.videos = videosData.map((video, index) => ({
      id: video.id || `api_video_${index}`,
      title: video.title || video.desc || `视频 ${index + 1}`,
      coverUrl: video.coverUrl || video.thumbUrl || '',
      element: null,
      index: index,
      collected: false,
      apiData: video
    }));
    
    console.log(`📊 [API采集] 获取到 ${this.videos.length} 个视频`);
    this.updateBatchDownloadUI();
  },
  
  // 添加批量下载UI
  addBatchDownloadUI: function() {
    // 移除现有UI
    const existingUI = document.getElementById('wx-channels-batch-download-ui');
    if (existingUI) {
      existingUI.remove();
    }
    
    // 创建浮动UI
    const ui = document.createElement('div');
    ui.id = 'wx-channels-batch-download-ui';
    ui.style.cssText = `
      position: fixed;
      top: 20px;
      right: 20px;
      background: rgba(0, 0, 0, 0.9);
      color: white;
      padding: 15px;
      border-radius: 8px;
      z-index: 99999;
      font-family: Arial, sans-serif;
      font-size: 14px;
      min-width: 200px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    `;
    
    ui.innerHTML = `
      <div style="margin-bottom: 10px; font-weight: bold;">主页页面视频采集</div>
      <div id="video-count">已采集: 0 个视频</div>
      <div id="status-message" style="
        display: none;
        margin-top: 8px;
        padding: 8px 12px;
        border-radius: 4px;
        font-size: 13px;
        line-height: 1.4;
        word-wrap: break-word;
        max-height: 100px;
        overflow-y: auto;
      "></div>
      <div style="margin: 10px 0;">
        <button id="batch-download-btn" style="
          background: #ff6b35;
          color: white;
          border: none;
          padding: 6px 10px;
          border-radius: 4px;
          cursor: pointer;
          margin-right: 6px;
        ">前端批量下载</button>
        <button id="server-batch-start" style="
          background: #722ed1;
          color: white;
          border: none;
          padding: 6px 10px;
          border-radius: 4px;
          cursor: pointer;
          margin-right: 6px;
        ">后端批量下载</button>
        <button id="server-batch-cancel" style="
          background: #faad14;
          color: white;
          border: none;
          padding: 6px 10px;
          border-radius: 4px;
          cursor: pointer;
          margin-right: 6px;
        ">取消</button>
        <button id="export-links-btn" style="
          background: #1890ff;
          color: white;
          border: none;
          padding: 6px 10px;
          border-radius: 4px;
          cursor: pointer;
          margin-right: 6px;
        ">导出链接</button>
        <button id="server-batch-failed" style="
          background: #f5222d;
          color: white;
          border: none;
          padding: 6px 10px;
          border-radius: 4px;
          cursor: pointer;
        ">导出失败</button>
      </div>
      <div style="margin-top: 6px; padding-top: 6px; border-top: 1px solid rgba(255,255,255,0.1);">
        <label style="display: flex; align-items: center; color: white; font-size: 13px; cursor: pointer;">
          <input type="checkbox" id="force-redownload-checkbox" style="margin-right: 6px; cursor: pointer;" />
          <span>强制重新下载（覆盖已存在的文件，只有后端下载生效）</span>
        </label>
      </div>
      <div style="margin-top:8px;">
        <button id="toggle-select-list" style="
          background:#595959;color:#fff;border:none;padding:6px 10px;border-radius:4px;cursor:pointer;margin-right:6px;">编辑选择</button>
        <button id="selected-frontend" style="
          background:#13c2c2;color:#fff;border:none;padding:6px 10px;border-radius:4px;cursor:pointer;margin-right:6px;">仅选中-前端下载</button>
        <button id="selected-backend" style="
          background:#531dab;color:#fff;border:none;padding:6px 10px;border-radius:4px;cursor:pointer;">仅选中-后端下载</button>
      </div>
      <div id="select-list" style="display:none;max-height:240px;overflow:auto;margin-top:8px;border:1px solid rgba(255,255,255,0.15);padding:6px;border-radius:4px;"></div>
      <div id="download-progress" style="display: none; margin-top: 10px;">
        <div>下载进度: <span id="progress-text">0/0</span></div>
        <div style="background: #333; height: 4px; border-radius: 2px; margin-top: 5px;">
          <div id="progress-bar" style="background: #07c160; height: 100%; width: 0%; border-radius: 2px; transition: width 0.3s;"></div>
        </div>
      </div>
      <div id="server-download-progress" style="display: none; margin-top: 10px;">
        <div>后端下载进度: <span id="server-progress-text">0/0</span> (进行中: <span id="server-progress-running">0</span>, 失败: <span id="server-progress-failed">0</span>)</div>
        <div style="background: #333; height: 4px; border-radius: 2px; margin-top: 5px;">
          <div id="server-progress-bar" style="background: #722ed1; height: 100%; width: 0%; border-radius: 2px; transition: width 0.3s;"></div>
        </div>
      </div>
    `;
    
    document.body.appendChild(ui);
    
    // 等待DOM更新后再绑定事件
    setTimeout(() => {
      // 绑定事件
      const batchBtn = document.getElementById('batch-download-btn');
      if (batchBtn) {
        batchBtn.onclick = () => {
          this.startBatchDownload();
        };
      }
    }, 0);
    
    // 导出菜单
    let exportMenu = document.getElementById('wx-export-menu');
    if (!exportMenu) {
      exportMenu = document.createElement('div');
      exportMenu.id = 'wx-export-menu';
      exportMenu.style.cssText = `
        position:absolute; right:20px; margin-top:4px; background:#111; color:#fff; border:1px solid rgba(255,255,255,.15);
        border-radius:4px; z-index:100000; display:none;
      `;
      exportMenu.innerHTML = `
        <div style="display:flex;">
          <button data-fmt="txt" style="background:#1890ff;border:none;color:#fff;padding:6px 10px;margin:6px;border-radius:4px;cursor:pointer;">导出 TXT</button>
          <button data-fmt="json" style="background:#13c2c2;border:none;color:#fff;padding:6px 10px;margin:6px;border-radius:4px;cursor:pointer;">导出 JSON</button>
          <button data-fmt="md" style="background:#722ed1;border:none;color:#fff;padding:6px 10px;margin:6px;border-radius:4px;cursor:pointer;">导出 Markdown</button>
        </div>`;
      ui.appendChild(exportMenu);
      exportMenu.querySelectorAll('button').forEach(btn => {
        btn.onclick = () => {
          const fmt = btn.getAttribute('data-fmt');
          this.exportVideoLinks(fmt);
          exportMenu.style.display = 'none';
        };
      });
      document.addEventListener('click', (e)=>{
        const target = e.target;
        const within = target && (target.id === 'export-links-btn' || target.closest('#wx-export-menu'));
        if (!within) exportMenu.style.display = 'none';
      });
    }

    document.getElementById('export-links-btn').onclick = (ev) => {
      ev.stopPropagation();
      exportMenu.style.display = exportMenu.style.display === 'none' ? 'block' : 'none';
    };

    // 后端批量按钮
    const addAuthHeader = (headers) => {
      try {
        if (window.__WX_LOCAL_TOKEN__) headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
      } catch(_) {}
      return headers;
    };
    const toBase64 = (u8) => { let s=''; for (let i=0;i<u8.length;i++) s += String.fromCharCode(u8[i]); return btoa(s); };
    const buildBatchPayload = async (list, forceRedownload = false) => {
      const items = (list || this.videos || []).filter(v => v && v.url);
      const out = [];
      for (const v of items) {
        const rec = {
          id: String(v.id || ''),
          url: String(v.url || ''),
          title: String(v.title || ''),
          filename: String(v.title || ''),
          authorName: String(v.nickname || (v.contact && v.contact.nickname) || '')
        };
        try {
          if (v.key && v.key.length > 0 && typeof __wx_channels_decrypt === 'function') {
            const dec = await __wx_channels_decrypt(v.key);
            const prefixLen = Math.min(dec.length, 131072); // 128 KiB 前缀
            rec.decryptorPrefix = toBase64(dec.slice(0, prefixLen));
            rec.prefixLen = prefixLen;
          }
        } catch(_) {}
        out.push(rec);
      }
      // 优先使用传入的参数，如果没有则使用自动设置的标志
      const finalForceRedownload = forceRedownload !== undefined ? forceRedownload : this._forceRedownload;
      console.log('[构建payload] forceRedownload参数:', forceRedownload, '自动标志:', this._forceRedownload, '最终值:', finalForceRedownload);
      return { videos: out, forceRedownload: finalForceRedownload };
    };
    const safeFetch = (url, opt) => fetch(url, opt).catch(() => ({ ok:false }));

    // 等待DOM更新后再获取按钮元素
    const getButtons = () => {
      return {
        btnStart: document.getElementById('server-batch-start'),
        btnCancel: document.getElementById('server-batch-cancel'),
        btnFailed: document.getElementById('server-batch-failed'),
        btnToggleSelect: document.getElementById('toggle-select-list'),
        btnSelFrontend: document.getElementById('selected-frontend'),
        btnSelBackend: document.getElementById('selected-backend'),
        selList: document.getElementById('select-list'),
        forceRedownloadCheckbox: document.getElementById('force-redownload-checkbox')
      };
    };
    
    // 选择集合
    this._selectedIds = this._selectedIds || new Set();
    
    // 延迟绑定按钮事件，确保DOM已完全渲染
    setTimeout(() => {
      const buttons = getButtons();
      const btnStart = buttons.btnStart;
      const btnCancel = buttons.btnCancel;
      const btnFailed = buttons.btnFailed;
      const btnToggleSelect = buttons.btnToggleSelect;
      const btnSelFrontend = buttons.btnSelFrontend;
      const btnSelBackend = buttons.btnSelBackend;
      const selList = buttons.selList;
      const forceRedownloadCheckbox = buttons.forceRedownloadCheckbox;

      const renderSelectList = () => {
        if (!selList) return;
      const items = (this.videos || []).slice(0, 200);
      const fmtTs = (ts) => {
        let n = Number(ts); if (!Number.isFinite(n) || n <= 0) return '时间未知';
        if (n < 1e12) n = n * 1000; const d = new Date(n);
        const p = (x)=>String(x).padStart(2,'0');
        return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
      };
      const fmtDur = (ms) => {
        let s = Math.floor((Number(ms)||0)/1000); const m = Math.floor(s/60); s = s%60;
        return `${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`;
      };
      const fmtMB = (b) => {
        const x = Number(b)||0; if (x<=0) return '未知'; return (x/1024/1024).toFixed(2)+'MB';
      };
      selList.innerHTML = items.map(v => {
        const id = String(v.id || '');
        const checked = this._selectedIds.has(id) ? 'checked' : '';
        const title = String(v.title || '').slice(0, 40).replace(/</g,'&lt;');
        const cover = v.coverUrl || (v.cover && v.cover.url) || '';
        const ctime = fmtTs(v.createtime);
        const dur = fmtDur(v.duration);
        const size = fmtMB(v.size);
        return `<label style="display:flex;align-items:center;gap:8px;margin:6px 0;">
          <input type="checkbox" data-id="${id}" ${checked}/>
          <img src="${cover}" onerror="this.style.display='none'" style="width:64px;height:36px;object-fit:cover;border-radius:4px;border:1px solid rgba(255,255,255,0.15)"/>
          <div style="display:flex;flex-direction:column;gap:2px;min-width:0;">
            <div style="opacity:.95;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:360px;">${title || '(无标题)'}</div>
            <div style="opacity:.65;font-size:12px;">${ctime} · 时长 ${dur} · ${size}</div>
          </div>
        </label>`;
      }).join('');
      selList.querySelectorAll('input[type="checkbox"]').forEach(cb => {
        cb.onchange = (e) => {
          const id = cb.getAttribute('data-id');
          if (!id) return;
          if (cb.checked) this._selectedIds.add(id); else this._selectedIds.delete(id);
        };
      });
      };

      // 后端下载进度轮询
      const startServerProgressPolling = () => {
        // 清除之前的轮询
        if (this._serverProgressInterval) {
          clearInterval(this._serverProgressInterval);
          this._serverProgressInterval = null;
        }
      // 显示进度条
      const serverProgressEl = document.getElementById('server-download-progress');
      if (serverProgressEl) {
        serverProgressEl.style.display = 'block';
      }
      // 开始轮询
      const pollProgress = async () => {
        const headers = addAuthHeader({'Content-Type':'application/json'});
        const res = await safeFetch('/__wx_channels_api/batch_progress', { method:'POST', headers });
        if (res && res.ok) {
          const data = await res.json().catch(()=>null);
          if (data) {
            const total = data.total || 0;
            const done = data.done || 0;
            const running = data.running || 0;
            const failed = data.failed || 0;
            const percentage = total > 0 ? (done / total * 100) : 0;
            // 更新进度显示
            const textEl = document.getElementById('server-progress-text');
            const runningEl = document.getElementById('server-progress-running');
            const failedEl = document.getElementById('server-progress-failed');
            const barEl = document.getElementById('server-progress-bar');
            if (textEl) textEl.textContent = `${done}/${total}`;
            if (runningEl) runningEl.textContent = running;
            if (failedEl) failedEl.textContent = failed;
            if (barEl) barEl.style.width = `${percentage}%`;
            // 如果全部完成，停止轮询
            if (total > 0 && done + failed >= total && running === 0) {
              if (this._serverProgressInterval) {
                clearInterval(this._serverProgressInterval);
                this._serverProgressInterval = null;
              }
              // 3秒后隐藏进度条
              setTimeout(() => {
                if (serverProgressEl) serverProgressEl.style.display = 'none';
              }, 3000);
            }
          }
        }
      };
      // 立即查询一次
      pollProgress();
      // 每2秒轮询一次
      this._serverProgressInterval = setInterval(pollProgress, 2000);
    };
      const stopServerProgressPolling = () => {
        if (this._serverProgressInterval) {
          clearInterval(this._serverProgressInterval);
          this._serverProgressInterval = null;
        }
        const serverProgressEl = document.getElementById('server-download-progress');
        if (serverProgressEl) {
          serverProgressEl.style.display = 'none';
        }
      };

      if (btnStart) {
        btnStart.onclick = async () => {
          try {
            console.log('[后端批量] 开始构建payload...');
            // 先停止之前的轮询（如果有）
            stopServerProgressPolling();
            // 从复选框获取强制重新下载选项，或使用自动设置的标志
            const forceRedownload = forceRedownloadCheckbox ? forceRedownloadCheckbox.checked : this._forceRedownload;
            const payload = await buildBatchPayload(null, forceRedownload);
            console.log('[后端批量] payload构建完成，视频数量:', payload.videos.length, '强制重新下载:', payload.forceRedownload);
            if (!payload.videos.length) { 
              this.showStatusMessage('没有可用视频', 'warning');
              return; 
            }
            // 下载开始后，清除自动设置的强制重新下载标志（但保留用户手动选择的复选框状态）
            this._forceRedownload = false;
            const headers = addAuthHeader({'Content-Type':'application/json'});
            console.log('[后端批量] 发送请求到后端...');
            const res = await safeFetch('/__wx_channels_api/batch_start', { method:'POST', headers, body: JSON.stringify(payload) });
            if (res && res.ok) {
              this.showStatusMessage('已提交到后端下载队列' + (forceRedownload ? '（将重新下载已存在的文件）' : ''), 'success');
              // 自动开始显示进度并轮询
              startServerProgressPolling();
            } else {
              console.error('[后端批量] 提交失败，响应:', res);
              this.showStatusMessage('提交失败，请检查控制台', 'error');
            }
        } catch (error) {
          console.error('[后端批量] 错误:', error);
          this.showStatusMessage('发生错误: ' + error.message, 'error');
        }
      };
      } else {
        console.error('[后端批量] 按钮未找到: server-batch-start');
      }
      
      if (btnCancel) {
        btnCancel.onclick = async () => {
          console.log('[后端批量] 收到取消请求');
          // 先取消前端批量（无需刷新）
          try { this.cancelBatchDownload(); } catch(_) {}
          // 停止后端进度轮询
          stopServerProgressPolling();
          // 设置强制重新下载标志和复选框，下次下载时将重新下载已存在的文件
          this._forceRedownload = true;
          if (forceRedownloadCheckbox) {
            forceRedownloadCheckbox.checked = true;
          }
          // 同时尝试通知后端（容错）
          const headers = addAuthHeader({'Content-Type':'application/json'});
          await safeFetch('/__wx_channels_api/batch_cancel', { method:'POST', headers });
          this.showStatusMessage('已请求取消，已自动勾选"强制重新下载"选项', 'info');
        };
      }
      
      if (btnFailed) {
        btnFailed.onclick = async () => {
          const headers = addAuthHeader({'Content-Type':'application/json'});
          const res = await safeFetch('/__wx_channels_api/batch_failed', { method:'POST', headers });
          if (res && res.ok) {
            const data = await res.json().catch(()=>null);
            if (data) {
              this.showStatusMessage(`失败: ${data.failed} 个\n清单: ${data.json}`, 'warning', 8000);
            } else {
              this.showStatusMessage('导出失败', 'error');
            }
          } else {
            this.showStatusMessage('导出失败', 'error');
          }
        };
      }

      if (btnToggleSelect) btnToggleSelect.onclick = () => {
      if (!selList) return;
        if (selList.style.display === 'none') { renderSelectList(); selList.style.display = 'block'; }
        else { selList.style.display = 'none'; }
      };

      // 仅选中下载（公共获取函数）
      const getSelectedVideos = () => {
      const ids = this._selectedIds || new Set();
      const all = this.videos || [];
      if (!ids.size) return [];
        return all.filter(v => ids.has(String(v.id || '')) && v.url);
      };

      if (btnSelFrontend) {
        btnSelFrontend.onclick = async () => {
          const list = getSelectedVideos();
          if (!list.length) { 
            this.showStatusMessage('未选择任何视频', 'warning');
            return; 
          }
          const confirmed = await this.showConfirmDialog(`仅选中-前端下载：${list.length} 个，开始？`, '确认下载');
          if (!confirmed) return;
          // 按现有前端流程串行下载
          this.batchDownloading = true;
          this.batchCancelRequested = false;
          this.currentAbortController = null;
          this.downloadProgress = { current: 0, total: list.length, failedCount: 0 };
          this.showDownloadProgress();
          const runNext = () => {
            if (this.batchCancelRequested || this.downloadProgress.current >= this.downloadProgress.total) {
              this.batchDownloading = false;
              this.hideDownloadProgress();
              if (this.batchCancelRequested) {
                this.showStatusMessage('已取消前端批量下载', 'info');
              } else {
                const successCount = this.downloadProgress.total - (this.downloadProgress.failedCount || 0);
                const failedCount = this.downloadProgress.failedCount || 0;
                this.showStatusMessage(`前端批量下载完成！共处理 ${this.downloadProgress.total} 个视频，成功: ${successCount} 个，失败: ${failedCount} 个`, 'success', 8000);
              }
              return;
            }
            const v = list[this.downloadProgress.current];
            this.silentDownload(v).then(()=>{
              this.downloadProgress.current++; this.updateDownloadProgress(); setTimeout(runNext, 800);
            }).catch(()=>{ this.downloadProgress.failedCount=(this.downloadProgress.failedCount||0)+1; this.downloadProgress.current++; this.updateDownloadProgress(); setTimeout(runNext, 800); });
          };
          runNext();
        };
      }

      if (btnSelBackend) {
        btnSelBackend.onclick = async () => {
          try {
            console.log('[仅选中-后端] 获取选中的视频...');
            // 先停止之前的轮询（如果有）
            stopServerProgressPolling();
            // 从复选框获取强制重新下载选项，或使用自动设置的标志
            const forceRedownload = forceRedownloadCheckbox ? forceRedownloadCheckbox.checked : this._forceRedownload;
            const list = getSelectedVideos();
            console.log('[仅选中-后端] 选中视频数量:', list.length);
            if (!list.length) { 
              this.showStatusMessage('未选择任何视频', 'warning');
              return; 
            }
            const headers = addAuthHeader({'Content-Type':'application/json'});
            console.log('[仅选中-后端] 构建payload...');
            const payload = await buildBatchPayload(list, forceRedownload);
            console.log('[仅选中-后端] payload构建完成，视频数量:', payload.videos.length, '强制重新下载:', payload.forceRedownload);
            // 下载开始后，清除自动设置的强制重新下载标志（但保留用户手动选择的复选框状态）
            this._forceRedownload = false;
            const res = await safeFetch('/__wx_channels_api/batch_start', { method:'POST', headers, body: JSON.stringify(payload) });
            if (res && res.ok) {
              this.showStatusMessage('选中清单已提交后端' + (forceRedownload ? '（将重新下载已存在的文件）' : ''), 'success');
              // 自动开始显示进度并轮询
              startServerProgressPolling();
            } else {
              console.error('[仅选中-后端] 提交失败，响应:', res);
              this.showStatusMessage('提交失败，请检查控制台', 'error');
            }
          } catch (error) {
            console.error('[仅选中-后端] 错误:', error);
            this.showStatusMessage('发生错误: ' + error.message, 'error');
          }
        };
      } else {
        console.error('[仅选中-后端] 按钮未找到: selected-backend');
      }
    }, 100); // 延迟100ms确保DOM完全渲染
  },
  
  // 显示状态信息
  showStatusMessage: function(message, type = 'info', duration = 5000) {
    const statusEl = document.getElementById('status-message');
    if (!statusEl) return;
    
    // 清除之前的定时器
    if (this._statusMessageTimeout) {
      clearTimeout(this._statusMessageTimeout);
      this._statusMessageTimeout = null;
    }
    
    // 设置消息内容和样式
    statusEl.textContent = message;
    statusEl.style.display = 'block';
    
    // 根据类型设置颜色（使用半透明背景，更柔和）
    const colors = {
      'info': { bg: 'rgba(24, 144, 255, 0.15)', border: 'rgba(64, 169, 255, 0.4)', text: '#69b7ff' },
      'success': { bg: 'rgba(82, 196, 26, 0.15)', border: 'rgba(115, 209, 61, 0.4)', text: '#95de64' },
      'warning': { bg: 'rgba(250, 173, 20, 0.15)', border: 'rgba(255, 197, 61, 0.4)', text: '#ffd666' },
      'error': { bg: 'rgba(245, 34, 45, 0.15)', border: 'rgba(255, 77, 79, 0.4)', text: '#ff7875' }
    };
    const color = colors[type] || colors.info;
    statusEl.style.background = color.bg;
    statusEl.style.border = `1px solid ${color.border}`;
    statusEl.style.color = color.text;
    
    // 自动隐藏
    if (duration > 0) {
      this._statusMessageTimeout = setTimeout(() => {
        statusEl.style.opacity = '0';
        statusEl.style.transition = 'opacity 0.3s';
        setTimeout(() => {
          statusEl.style.display = 'none';
          statusEl.style.opacity = '1';
          statusEl.style.transition = '';
        }, 300);
      }, duration);
    }
  },
  
  // 隐藏状态信息
  hideStatusMessage: function() {
    const statusEl = document.getElementById('status-message');
    if (statusEl) {
      statusEl.style.display = 'none';
    }
    if (this._statusMessageTimeout) {
      clearTimeout(this._statusMessageTimeout);
      this._statusMessageTimeout = null;
    }
  },
  
  // 显示自定义确认对话框
  showConfirmDialog: function(message, title = '确认') {
    return new Promise((resolve) => {
      // 创建遮罩层
      const overlay = document.createElement('div');
      overlay.id = 'wx-confirm-overlay';
      overlay.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        background: rgba(0, 0, 0, 0.5);
        z-index: 999999;
        display: flex;
        align-items: center;
        justify-content: center;
      `;
      
      // 创建对话框
      const dialog = document.createElement('div');
      dialog.style.cssText = `
        background: #1f1f1f;
        border: 1px solid rgba(255, 255, 255, 0.2);
        border-radius: 8px;
        padding: 20px;
        min-width: 300px;
        max-width: 500px;
        box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);
        color: white;
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      `;
      
      dialog.innerHTML = `
        <div style="font-size: 16px; font-weight: bold; margin-bottom: 12px; color: #fff;">${title}</div>
        <div style="font-size: 14px; line-height: 1.6; margin-bottom: 20px; color: rgba(255, 255, 255, 0.9); white-space: pre-line;">${message}</div>
        <div style="display: flex; justify-content: flex-end; gap: 10px;">
          <button id="wx-confirm-cancel" style="
            background: #595959;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
          ">取消</button>
          <button id="wx-confirm-ok" style="
            background: #1890ff;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
          ">确定</button>
        </div>
      `;
      
      overlay.appendChild(dialog);
      document.body.appendChild(overlay);
      
      // 清理函数
      const cleanup = () => {
        document.body.removeChild(overlay);
      };
      
      // 绑定事件
      const okBtn = dialog.querySelector('#wx-confirm-ok');
      const cancelBtn = dialog.querySelector('#wx-confirm-cancel');
      
      okBtn.onclick = () => {
        cleanup();
        resolve(true);
      };
      
      cancelBtn.onclick = () => {
        cleanup();
        resolve(false);
      };
      
      overlay.onclick = (e) => {
        if (e.target === overlay) {
          cleanup();
          resolve(false);
        }
      };
      
      // ESC键关闭
      const escHandler = (e) => {
        if (e.key === 'Escape') {
          cleanup();
          document.removeEventListener('keydown', escHandler);
          resolve(false);
        }
      };
      document.addEventListener('keydown', escHandler);
    });
  },
  
  // 更新批量下载UI
  updateBatchDownloadUI: function() {
    const countElement = document.getElementById('video-count');
    if (countElement) {
      countElement.textContent = `已采集: ${this.videos.length} 个视频`;
      console.log('✓ UI已更新，当前视频数:', this.videos.length);
    } else {
      console.log('⚠️ UI元素未找到，将在下次尝试更新');
      // UI还未创建，等待一下再更新
      setTimeout(() => {
        const el = document.getElementById('video-count');
        if (el) {
          el.textContent = `已采集: ${this.videos.length} 个视频`;
          console.log('✓ 延迟更新UI成功，当前视频数:', this.videos.length);
        }
      }, 500);
    }
  },
  
  // 设置滚动监听器
  setupScrollListener: function() {
    let scrollTimeout;
    window.addEventListener('scroll', () => {
      clearTimeout(scrollTimeout);
      scrollTimeout = setTimeout(() => {
        // 滚动到底部时自动采集新加载的视频
        if ((window.innerHeight + window.scrollY) >= document.body.offsetHeight - 1000) {
          console.log('📜 检测到滚动到底部，重新采集视频列表');
          this.collectVideosFromPage();
        }
      }, 500);
    });
  },
  
  // 开始手动下载（浏览器下载对话框）
  startManualDownload: async function() {
    if (this.batchDownloading) {
      console.log('⚠️ 批量下载已在进行中，请等待完成后再进行手动下载');
      this.showStatusMessage('批量下载进行中，请等待完成后再进行手动下载', 'warning');
      return;
    }
    
    if (this.videos.length === 0) {
      this.showStatusMessage('没有找到可下载的视频，请先刷新页面让系统自动采集视频列表', 'warning');
      return;
    }
    
    // 检查视频URL有效性
    const validVideos = this.videos.filter(video => {
      if (!video.url || video.url.trim() === '') {
        console.warn('⚠️ 跳过无效URL的视频:', video.title);
        return false;
      }
      return true;
    });
    
    if (validVideos.length === 0) {
      this.showStatusMessage('没有找到有效的视频URL，请刷新页面重新采集', 'warning');
      return;
    }
    
    // 显示选择对话框
    const message = `找到 ${validVideos.length} 个视频\n\n手动下载会逐个弹出浏览器下载对话框，您可以选择保存位置。\n\n是否继续？`;
    const confirmed = await this.showConfirmDialog(message, '确认下载');
    if (!confirmed) {
      return;
    }
    
    this.batchDownloading = true;
    this.downloadProgress = { current: 0, total: validVideos.length, failedCount: 0 };
    
    console.log(`🚀 开始手动下载 ${validVideos.length} 个有效视频`);
    
    // 发送手动下载开始日志到后端
    fetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({msg: `🚀 [Profile手动下载] 开始手动下载 ${validVideos.length} 个视频`})
    }).catch(() => {});
    
    this.showDownloadProgress();
    this.downloadNextManual();
  },
  
  // 手动下载下一个视频
  downloadNextManual: function() {
    if (this.downloadProgress.current >= this.downloadProgress.total) {
      this.batchDownloading = false;
      console.log('✅ 手动下载完成');
      
      const successCount = this.downloadProgress.total - (this.downloadProgress.failedCount || 0);
      const failedCount = this.downloadProgress.failedCount || 0;
      
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `✅ [Profile手动下载] 完成！共处理 ${this.downloadProgress.total} 个视频，成功 ${successCount} 个，失败 ${failedCount} 个`})
      }).catch(() => {});
      
      this.hideDownloadProgress();
      this.showStatusMessage(`手动下载完成！共处理 ${this.downloadProgress.total} 个视频，成功: ${successCount} 个，失败: ${failedCount} 个`, 'success', 8000);
      return;
    }
    
    const video = this.videos[this.downloadProgress.current];
    console.log(`📥 手动下载视频 ${this.downloadProgress.current + 1}/${this.downloadProgress.total}: ${video.title}`);
    
    // 使用浏览器下载（弹出保存对话框）
    this.simulateDownload(video).then(() => {
      this.downloadProgress.current++;
      this.updateDownloadProgress();
      
      // 延迟2秒后下载下一个（给用户时间处理对话框）
      setTimeout(() => {
        this.downloadNextManual();
      }, 2000);
    }).catch(error => {
      console.error('下载失败:', error);
      
      this.downloadProgress.failedCount = (this.downloadProgress.failedCount || 0) + 1;
      
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `⚠️ [Profile手动下载] 下载失败: ${video.title?.substring(0, 30)}...`})
      }).catch(() => {});
      
      this.downloadProgress.current++;
      this.updateDownloadProgress();
      setTimeout(() => {
        this.downloadNextManual();
      }, 2000);
    });
  },
  
  // 开始批量下载（自动下载到服务器）
  startBatchDownload: async function() {
    if (this.batchDownloading) {
      console.log('⚠️ 自动下载已在进行中');
      this.showStatusMessage('自动下载进行中，请等待完成', 'warning');
      return;
    }
    
    if (this.videos.length === 0) {
      this.showStatusMessage('没有找到可下载的视频，请先刷新页面让系统自动采集视频列表', 'warning');
      return;
    }
    
    // 检查视频URL有效性
    const validVideos = this.videos.filter(video => {
      if (!video.url || video.url.trim() === '') {
        console.warn('⚠️ 跳过无效URL的视频:', video.title);
        return false;
      }
      return true;
    });
    
    if (validVideos.length === 0) {
      this.showStatusMessage('没有找到有效的视频URL，请刷新页面重新采集', 'warning');
      return;
    }
    
    if (validVideos.length < this.videos.length) {
      console.warn(`⚠️ 过滤掉 ${this.videos.length - validVideos.length} 个无效URL的视频`);
    }
    
    // 显示确认对话框
    const message = `找到 ${validVideos.length} 个视频\n\n自动下载会将视频保存到软件的 downloads/<作者名称>/ 目录。\n\n是否继续？`;
    const confirmed = await this.showConfirmDialog(message, '确认下载');
    if (!confirmed) {
      return;
    }
    
    this.batchDownloading = true;
    this.batchCancelRequested = false;
    this.currentAbortController = null;
    this.downloadProgress = { current: 0, total: validVideos.length, failedCount: 0 };
    
    console.log(`🚀 开始自动下载 ${validVideos.length} 个有效视频`);
    
    // 发送批量下载开始日志到后端
    fetch('/__wx_channels_api/tip', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({msg: `🚀 [Profile自动下载] 开始自动下载 ${validVideos.length} 个视频`})
    }).catch(() => {});
    
    this.showDownloadProgress();
    this.downloadNext();
  },
  
  // 下载下一个视频（自动下载）
  downloadNext: function() {
    if (this.batchCancelRequested) {
      this.batchDownloading = false;
      this.hideDownloadProgress();
      this.showStatusMessage('已取消批量下载', 'info');
      return;
    }
    if (this.downloadProgress.current >= this.downloadProgress.total) {
      this.batchDownloading = false;
      console.log('✅ 自动下载完成');
      
      // 统计实际成功下载的数量
      const successCount = this.downloadProgress.total - (this.downloadProgress.failedCount || 0);
      const failedCount = this.downloadProgress.failedCount || 0;
      
      // 发送完成日志到后端
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `✅ [Profile自动下载] 完成！共处理 ${this.downloadProgress.total} 个视频，成功 ${successCount} 个，失败 ${failedCount} 个`})
      }).catch(() => {});
      
      this.hideDownloadProgress();
      this.showStatusMessage(`自动下载完成！共处理 ${this.downloadProgress.total} 个视频，成功: ${successCount} 个，失败: ${failedCount} 个`, 'success', 8000);
      return;
    }
    
    const video = this.videos[this.downloadProgress.current];
    console.log(`📥 自动下载视频 ${this.downloadProgress.current + 1}/${this.downloadProgress.total}: ${video.title}`);
    
    // 每5个视频发送一次进度日志
    if ((this.downloadProgress.current + 1) % 5 === 0) {
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `📥 [Profile自动下载] 进度: ${this.downloadProgress.current + 1}/${this.downloadProgress.total}`})
      }).catch(() => {});
    }
    
    // 使用静默下载（保存到服务器）
    this.silentDownload(video).then(() => {
      this.downloadProgress.current++;
      this.updateDownloadProgress();
      
      // 延迟1秒后下载下一个
      setTimeout(() => {
        this.downloadNext();
      }, 1000);
    }).catch(error => {
      console.error('下载失败:', error);
      
      // 增加失败计数
      this.downloadProgress.failedCount = (this.downloadProgress.failedCount || 0) + 1;
      
      // 发送错误日志
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `⚠️ [Profile自动下载] 下载失败: ${video.title?.substring(0, 30)}...`})
      }).catch(() => {});
      
      this.downloadProgress.current++;
      this.updateDownloadProgress();
      setTimeout(() => {
        this.downloadNext();
      }, 1000);
    });
  },
  
  // 取消批量下载（前端）
  cancelBatchDownload: function() {
    this.batchCancelRequested = true;
    try { if (this.currentAbortController) this.currentAbortController.abort(); } catch(_) {}
  },

  // 静默下载视频（保存到服务器）
  silentDownload: async function(video) {
    try {
      console.log(`📥 静默下载: ${video.title}`);
      // 在下载前打印关键视频信息，便于排查
      try {
        const fmtTs = (ts) => {
          let n = Number(ts);
          if (!Number.isFinite(n) || n <= 0) return 'N/A';
          if (n < 1e12) n = n * 1000;
          const d = new Date(n);
          const pad = (x) => String(x).padStart(2, '0');
          return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
        };
        const debugInfo = {
          id: video.id,
          title: (video.title || '').substring(0, 60),
          urlHead: video.url ? video.url.substring(0, 80) : null,
          hasUrl: !!video.url,
          keyLen: video.key ? String(video.key).length : 0,
          hasDecryptor: !!video.decryptor_array,
          type: video.type,
          specCount: Array.isArray(video.spec) ? video.spec.length : 0,
          size: video.size || 0,
          nickname: video.nickname || (video.contact && video.contact.nickname) || '',
          createtime: video.createtime || null,
          createtimeFmt: fmtTs(video.createtime)
        };
      } catch(_) {}
      
      // 简化的开始日志
      const shortTitle = (video.title || '未命名').substring(0, 35);
      const sizeMB = video.size ? (video.size / 1024 / 1024).toFixed(2) : '未知';
      
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `🎬 [开始下载] ${shortTitle}... | ${sizeMB}MB`})
      }).catch(() => {});
      
      if (video.type === 'media') {
        // 视频下载
        if (!video.url) {
          throw new Error('视频URL为空');
        }
        
        // 判断是否需要解密
        const hasKey = !!(video.key && video.key.length > 0);
        
        console.log('静默下载方法选择:', {
          hasKey: hasKey,
          keyValue: video.key ? (video.key.substring(0, 20) + '...') : 'null'
        });
        
        // 下载视频数据（添加缓存控制和重试机制）
        let response;
        let retryCount = 0;
        const maxRetries = 3;
        
        // 可取消控制器
        this.currentAbortController = new AbortController();
        const signal = this.currentAbortController.signal;
        while (retryCount < maxRetries && !this.batchCancelRequested) {
          try {
            response = await fetch(video.url, {
              cache: 'no-cache',
              headers: {
                'Cache-Control': 'no-cache, no-store, must-revalidate',
                'Pragma': 'no-cache',
                'Expires': '0'
              },
              signal
            });
            
            if (response.ok) {
              break; // 成功，跳出重试循环
            } else {
              throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
          } catch (error) {
            retryCount++;
            console.warn(`⚠️ 下载失败，第${retryCount}次重试: ${error.message}`);
            
            if (retryCount < maxRetries && !this.batchCancelRequested) {
              // 等待1-3秒后重试
              const delay = retryCount * 1000;
              console.log(`⏳ 等待${delay}ms后重试...`);
              await new Promise(resolve => setTimeout(resolve, delay));
            } else {
              throw new Error(`下载失败，已重试${maxRetries}次: ${error.message}`);
            }
          }
        }
        if (this.batchCancelRequested) { throw new Error('已取消'); }
        
        const blob = await response.blob();
        let videoData = new Uint8Array(await blob.arrayBuffer());
        
        // 如果需要解密
        if (hasKey) {
          console.log('🔐 视频需要解密');
          
          // 生成解密数组
          if (!video.decryptor_array) {
            console.log('🔑 生成解密数组...');
            video.decryptor_array = await __wx_channels_decrypt(video.key);
            console.log('✓ 解密数组生成成功');
          }
          
          // 解密视频
          console.log('🔐 开始解密视频...');
          videoData = __wx_channels_video_decrypt(videoData, 0, video);
          console.log('✓ 视频解密完成');
        }
        
        // 获取作者名称
        const authorName = video.nickname || video.contact?.nickname || '未知作者';
        
        // 清理文件名：去除标签（#开头的内容）和多余空白
        let cleanTitle = (video.title || 'video')
          .split('\n')[0]  // 只取第一行
          .replace(/#[^\s#]+/g, '')  // 去除所有标签
          .replace(/\s+/g, ' ')  // 多个空格合并为一个
          .trim();  // 去除首尾空格
        
        // 计算发布时间前缀: 优先使用 video.createtime (秒或毫秒), 否则当前时间
        let ts = Number(video.createtime);
        if (!Number.isFinite(ts) || ts <= 0) {
          ts = Date.now();
        } else {
          // 如果是秒级时间戳，转换为毫秒
          if (ts < 1e12) ts = ts * 1000;
        }
        const d = new Date(ts);
        const pad = (n) => String(n).padStart(2, '0');
        const yyyy = d.getFullYear();
        const MM = pad(d.getMonth() + 1);
        const dd = pad(d.getDate());
        const HH = pad(d.getHours());
        const mm = pad(d.getMinutes());
        const ss = pad(d.getSeconds());
        const timePrefix = `${yyyy}${MM}${dd}_${HH}${mm}${ss}`;
        
        // 如果清理后为空，使用默认名称
        if (!cleanTitle) {
          cleanTitle = 'video_' + timePrefix;
        }
        
        // 最终文件名: 时间前缀_标题
        const finalFilename = `${timePrefix}_${cleanTitle}`;
        
        // 全部使用分片上传（更稳定）
        const sizeMB = videoData.byteLength / 1024 / 1024;
        console.log(`📦 使用分片上传: ${sizeMB.toFixed(2)}MB`);
        try {
          const path = await this.uploadInChunks(videoData, finalFilename, authorName);
          console.log('✓ 静默下载成功(分片):', path);
          if (window.__wx_channels_record_download) {
            window.__wx_channels_record_download(video);
          }
          return { path };
        } catch (e) {
          console.error('❌ 分片上传失败:', e && e.message ? e.message : e);
          fetch('/__wx_channels_api/tip', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ msg: `❌ [分片上传失败] ${finalFilename}: ${e && e.message ? e.message : e}` })
          }).catch(() => {});
          throw e; // 直接抛出错误，不再回退到直传
        }
      } else if (video.type === 'picture') {
        // 图片暂不支持静默下载
        console.warn('⚠️ 图片暂不支持静默下载，跳过');
        return null;
      }
    } catch (error) {
      console.error('✗ 静默下载失败:', error);
      fetch('/__wx_channels_api/tip', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({msg: `❌ [静默下载] 失败: ${error.message}`})
      }).catch(() => {});
      throw error;
    }
  },
  
  // 实际下载视频（浏览器下载）
  simulateDownload: function(video) {
    return new Promise((resolve, reject) => {
      try {
        console.log(`📥 开始下载: ${video.title}`);
        console.log('视频数据:', {
          type: video.type,
          hasUrl: !!video.url,
          hasKey: !!video.key,
          hasSpec: !!(video.spec && video.spec.length > 0),
          url: video.url?.substring(0, 100) + '...'
        });
        
        // 发送下载日志
        fetch('/__wx_channels_api/tip', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({msg: `📥 [下载] ${video.title?.substring(0, 40)}... (type: ${video.type}, hasUrl: ${!!video.url}, hasKey: ${!!video.key})`})
        }).catch(() => {});
        
        // 根据视频类型调用相应的下载函数
        if (video.type === 'picture') {
          // 图片下载
          console.log('调用图片下载函数 __wx_channels_download3');
          if (typeof __wx_channels_download3 === 'function') {
            __wx_channels_download3(video, video.title || 'picture')
              .then(() => {
                console.log('✓ 图片下载成功');
                resolve();
              })
              .catch(err => {
                console.error('✗ 图片下载失败:', err);
                reject(err);
              });
          } else {
            console.warn('图片下载函数不可用');
            resolve();
          }
        } else if (video.type === 'media') {
          // 视频下载
          console.log('准备下载视频，URL:', video.url ? '有' : '无');
          
          if (!video.url) {
            const msg = '视频URL为空，无法下载';
            console.error(msg);
            fetch('/__wx_channels_api/tip', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({msg: `❌ [下载] ${msg}: ${video.title?.substring(0, 30)}...`})
            }).catch(() => {});
            resolve(); // 跳过这个视频
            return;
          }
          
          // 先将视频设置为当前profile（这样下载函数可以访问到完整数据）
          if (window.__wx_channels_store__) {
            window.__wx_channels_store__.profile = video;
          }
          
          // 判断使用哪个下载函数
          // 注意：微信视频号的视频通常都是加密的，即使没有明确的key也可能需要特殊处理
          const hasKey = !!(video.key && video.key.length > 0);
          const download4Available = typeof __wx_channels_download4 === 'function';
          const download2Available = typeof __wx_channels_download2 === 'function';
          
          console.log('下载方法选择:', {
            hasKey: hasKey,
            keyValue: video.key ? (video.key.substring(0, 20) + '...') : 'null',
            download4Available: download4Available,
            download2Available: download2Available
          });
          
          // 优先使用加密下载（如果有key）
          if (hasKey && download4Available) {
            // 加密视频下载
            console.log('✓ 检测到解密key，使用方法4下载（加密视频）');
            fetch('/__wx_channels_api/tip', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({msg: `🔐 [下载] 使用加密下载方法（含解密）: ${video.title?.substring(0, 30)}...`})
            }).catch(() => {});
            
            __wx_channels_download4(video, video.title || 'video')
              .then(() => {
                console.log('✓ 加密视频下载并解密成功');
                if (window.__wx_channels_record_download) {
                  window.__wx_channels_record_download(video);
                }
                resolve();
              })
              .catch(err => {
                console.error('✗ 加密视频下载失败:', err);
                fetch('/__wx_channels_api/tip', {
                  method: 'POST',
                  headers: {'Content-Type': 'application/json'},
                  body: JSON.stringify({msg: `❌ [下载] 加密下载失败: ${err.message}`})
                }).catch(() => {});
                
                // 尝试降级到普通下载
                if (download2Available) {
                  console.log('⚠️ 尝试降级到普通下载方法');
                  __wx_channels_download2(video, video.title || 'video')
                    .then(() => {
                      console.warn('⚠️ 使用普通方法下载成功，但视频可能无法播放（需要解密）');
                      resolve();
                    })
                    .catch(err2 => {
                      console.error('✗ 降级下载也失败:', err2);
                      reject(err2);
                    });
                } else {
                  reject(err);
                }
              });
          } else if (download2Available) {
            // 普通视频下载（无key或无加密下载函数）
            const warningMsg = hasKey ? '⚠️ 视频有解密key但加密下载函数不可用，使用普通下载（可能无法播放）' : '使用普通下载方法';
            console.log(warningMsg);
            
            fetch('/__wx_channels_api/tip', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({msg: `📹 [下载] ${warningMsg}: ${video.title?.substring(0, 30)}...`})
            }).catch(() => {});
            
            __wx_channels_download2(video, video.title || 'video')
              .then(() => {
                console.log('✓ 普通视频下载成功');
                if (hasKey) {
                  console.warn('⚠️ 警告：该视频需要解密才能播放！');
                }
                if (window.__wx_channels_record_download) {
                  window.__wx_channels_record_download(video);
                }
                resolve();
              })
              .catch(err => {
                console.error('✗ 普通视频下载失败:', err);
                fetch('/__wx_channels_api/tip', {
                  method: 'POST',
                  headers: {'Content-Type': 'application/json'},
                  body: JSON.stringify({msg: `❌ [下载] 普通下载失败: ${err.message}`})
                }).catch(() => {});
                reject(err);
              });
          } else {
            const msg = '没有可用的视频下载函数';
            console.warn(msg);
            fetch('/__wx_channels_api/tip', {
              method: 'POST',
              headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({msg: `⚠️ [下载] ${msg}: ${video.title?.substring(0, 30)}...`})
            }).catch(() => {});
            resolve(); // 跳过
          }
        } else {
          console.warn('未知的视频类型:', video.type);
          resolve();
        }
      } catch (error) {
        console.error('下载失败:', error);
        fetch('/__wx_channels_api/tip', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({msg: `❌ [下载] 异常: ${error.message}`})
        }).catch(() => {});
        reject(error);
      }
    });
  },
  
  // 显示下载进度
  showDownloadProgress: function() {
    const progressElement = document.getElementById('download-progress');
    if (progressElement) {
      progressElement.style.display = 'block';
    }
  },
  
  // 隐藏下载进度
  hideDownloadProgress: function() {
    const progressElement = document.getElementById('download-progress');
    if (progressElement) {
      progressElement.style.display = 'none';
    }
  },
  
  // 更新下载进度
  updateDownloadProgress: function() {
    const progressText = document.getElementById('progress-text');
    const progressBar = document.getElementById('progress-bar');
    
    if (progressText && progressBar) {
      const percentage = (this.downloadProgress.current / this.downloadProgress.total) * 100;
      progressText.textContent = `${this.downloadProgress.current}/${this.downloadProgress.total}`;
      progressBar.style.width = `${percentage}%`;
    }
  },
  
  // 导出视频链接
  exportVideoLinks: function(format) {
    if (this.videos.length === 0) {
      this.showStatusMessage('没有找到可导出的视频', 'warning');
      return;
    }
    
    const nowStr = new Date().toLocaleString();
    // 不再导出作者主页链接（pageUrl），仅导出视频直链等关键信息
    const fmtTs = (ts) => {
      let n = Number(ts); if (!Number.isFinite(n) || n <= 0) return '时间未知';
      if (n < 1e12) n = n * 1000; const d = new Date(n);
      const p = (x)=>String(x).padStart(2,'0');
      return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
    };
    const fmtDur = (ms) => {
      let s = Math.floor((Number(ms)||0)/1000); const m = Math.floor(s/60); s = s%60;
      return `${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`;
    };
    const fmtMB = (b) => { const x = Number(b)||0; if (x<=0) return '未知'; return (x/1024/1024).toFixed(2)+'MB'; };
    const rows = this.videos.map((video, index) => {
      const key = (video && video.key) ? String(video.key) : 'N/A';
      const url = (video && video.url) ? String(video.url) : 'N/A';
      const title = String(video.title || '');
      const id = String(video.id || '');
      const author = String(video.nickname || (video.contact && video.contact.nickname) || '');
      const like = Number(video.likeCount||0);
      const comment = Number(video.commentCount||0);
      const fav = Number(video.favCount||0);
      const forward = Number(video.forwardCount||0);
      const sizeMB = fmtMB(video.size);
      const duration = fmtDur(video.duration);
      const created = fmtTs(video.createtime);
      const cover = String(video.coverUrl || (video.cover && video.cover.url) || '');
      return { index: index+1, title, id, url, key, author, duration, sizeMB, like, comment, fav, forward, created, cover };
    });

    const download = (filename, mime, content) => {
      const blob = new Blob([content], { type: mime });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a'); a.href = url; a.download = filename; document.body.appendChild(a); a.click(); document.body.removeChild(a); URL.revokeObjectURL(url);
    };

    const fmt = (format||'txt').toLowerCase();
    if (fmt === 'json') {
      const payload = { generated_at: nowStr, count: rows.length, videos: rows };
      download(`profile_videos_${Date.now()}.json`, 'application/json', JSON.stringify(payload, null, 2));
    } else if (fmt === 'md') {
      const md = [
        `# 主页页面视频列表导出`,
        `生成时间: ${nowStr}`,
        `总计: ${rows.length} 个视频`,
        ''
      ].concat(rows.map(r => `${r.index}. [${r.title || '(无标题)'}](${r.url})  \n   作者: ${r.author}  ·  ID: ${r.id}  ·  时长: ${r.duration}  ·  大小: ${r.sizeMB}  \n   👍 ${r.like}  ·  💬 ${r.comment}  ·  🔖 ${r.fav}  ·  🔄 ${r.forward}  \n   创建时间: ${r.created}  \n   封面: ${r.cover}`)).join('\n');
      download(`profile_videos_${Date.now()}.md`, 'text/markdown;charset=utf-8', md);
    } else {
      const txt = [
        `主页页面视频列表导出`,
        `生成时间: ${nowStr}`,
        `总计: ${rows.length} 个视频`,
        ''
      ].concat(rows.map(r => `${r.index}. ${r.title}\n   作者: ${r.author}\n   ID: ${r.id}\n   URL: ${r.url}\n   KEY: ${r.key}\n   时长: ${r.duration}\n   大小: ${r.sizeMB}\n   点赞: ${r.like}  评论: ${r.comment}  收藏: ${r.fav}  转发: ${r.forward}\n   创建时间: ${r.created}\n   封面: ${r.cover}`)).join('\n');
      download(`profile_videos_${Date.now()}.txt`, 'text/plain;charset=utf-8', txt);
    }
    console.log(`📄 已导出 ${this.videos.length} 个视频（格式: ${fmt}）`);
  }
};

// 立即初始化profile采集器（供API拦截代码调用）
if (is_profile_page()) {
  console.log('🎯 [主页页面] 检测到主页页面，立即初始化采集器对象');
  
  // 立即暴露采集器对象，这样API拦截代码可以立即使用
  // init()会在页面加载后调用，用于添加UI
  
  // 等待页面完全加载后再调用init()添加UI
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      console.log('🎯 [Profile页面] DOM加载完成，准备添加UI');
      setTimeout(() => {
        if (window.__wx_channels_profile_collector) {
          window.__wx_channels_profile_collector.init();
        }
      }, 1000);
    });
  } else {
    console.log('🎯 [Profile页面] DOM已就绪，准备添加UI');
    setTimeout(() => {
      if (window.__wx_channels_profile_collector) {
        window.__wx_channels_profile_collector.init();
      }
    }, 1000);
  }
}

