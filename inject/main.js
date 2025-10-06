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
    array = __wx_channels_video_decrypt(array, 0, profile);
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
function __wx_channels_download_cur__() {
  var profile = __wx_channels_store__.profile;
  if (!profile) {
    alert("检测不到视频，请将本工具更新到最新版");
    return;
  }
  if (__wx_channels_store__.buffers.length === 0) {
    alert("没有可下载的内容");
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
  profile.data = __wx_channels_store__.buffers;
  __wx_channels_download(profile, filename);
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
          __wx_channels_handle_click_download__(profile.spec[0]);
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

