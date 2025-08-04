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
var $icon = document.createElement("div");
$icon.innerHTML =
  '<div data-v-6548f11a="" data-v-132dee25="" class="click-box op-item item-gap-combine" role="button"  aria-label="下载" style="padding: 4px; --border-radius: 4px; --left: 0; --top: 0; --right: 0; --bottom: 0;"><svg data-v-132dee25 class="svg-icon icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" fill="currentColor" width="28" height="28"><path d="M213.333333 853.333333h597.333334v-85.333333H213.333333m597.333334-384h-170.666667V128H384v256H213.333333l298.666667 298.666667 298.666667-298.666667z"></path></svg><span data-v-132dee25="" class="text">下载</span></div>';
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
var __timer = setInterval(() => {
  count += 1;
  // const $wrap1 = document.getElementsByClassName("feed-card-wrap")[0];
  // const $wrap2 = document.getElementsByClassName(
  //   "operate-row transition-show"
  // )[0];
  const $wrap3 = document.getElementsByClassName("full-opr-wrp")[0];
  if (!$wrap3) {
    if (count >= 5) {
      clearInterval(__timer);
      __timer = null;
      fetch("/__wx_channels_api/tip", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          msg: "\n没有找到操作栏，添加下载按钮失败！\n请在打开「视频详情页」后，刷新试试！",
        }),
      });
    }
    return;
  }
  clearInterval(__timer);
  __timer = null;
  const relative_node = $wrap3.children[$wrap3.children.length - 1];
  if (!relative_node) {
    fetch("/__wx_channels_api/tip", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ msg: "添加下载按钮成功！" }),
    });
    $wrap3.appendChild(__wx_channels_video_download_btn__);
    return;
  }
  fetch("/__wx_channels_api/tip", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ msg: "添加下载按钮成功！" }),
  });
  $wrap3.insertBefore(__wx_channels_video_download_btn__, relative_node);
}, 1000);

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

