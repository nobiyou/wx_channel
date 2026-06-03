const INDEX_HTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>wx_channel 分享链接解析 Worker</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 32px 20px;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #f6f8fb;
      color: #1f2937;
    }
    .wrap {
      max-width: 860px;
      margin: 0 auto;
      background: #fff;
      border-radius: 16px;
      padding: 24px;
      box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
    }
    h1 { margin: 0 0 12px; font-size: 24px; }
    p { margin: 0 0 16px; color: #4b5563; line-height: 1.7; }
    textarea {
      width: 100%;
      min-height: 88px;
      border: 1px solid #d1d5db;
      border-radius: 12px;
      padding: 12px 14px;
      font: inherit;
      resize: vertical;
      outline: none;
    }
    textarea:focus { border-color: #22c55e; }
    .actions {
      display: flex;
      gap: 12px;
      margin: 16px 0;
      flex-wrap: wrap;
    }
    button {
      border: none;
      border-radius: 10px;
      background: #16a34a;
      color: #fff;
      padding: 12px 20px;
      font: inherit;
      cursor: pointer;
    }
    button:disabled {
      opacity: 0.7;
      cursor: not-allowed;
    }
    .hint {
      font-size: 13px;
      color: #6b7280;
      margin-bottom: 16px;
    }
    pre {
      margin: 0;
      background: #0f172a;
      color: #e2e8f0;
      padding: 16px;
      border-radius: 12px;
      overflow: auto;
      font-size: 13px;
      line-height: 1.6;
      min-height: 120px;
    }
    .status {
      margin: 10px 0 16px;
      font-size: 14px;
      color: #6b7280;
    }
    .status.error { color: #dc2626; }
    .status.ok { color: #16a34a; }
  </style>
</head>
<body>
  <div class="wrap">
    <h1>wx_channel 分享链接解析 Worker</h1>
    <p>这个 Worker 用于把视频号分享链接解析成可用的视频详情，提供给 <code>wx_channel</code> 的 <code>cloudflare.sphHostname</code> 模式使用。</p>
    <div class="hint">支持的接口：<code>POST /api/fetch_video_profile</code>，请求体：<code>{"url":"https://weixin.qq.com/sph/xxx"}</code></div>
    <textarea id="urlInput" placeholder="粘贴视频号分享链接，例如 https://weixin.qq.com/sph/xxxx"></textarea>
    <div class="actions">
      <button id="fetchBtn">立即测试</button>
    </div>
    <div id="status" class="status"></div>
    <pre id="output">{}</pre>
  </div>
  <script>
    const urlInput = document.getElementById("urlInput");
    const fetchBtn = document.getElementById("fetchBtn");
    const statusEl = document.getElementById("status");
    const outputEl = document.getElementById("output");

    function setStatus(text, cls) {
      statusEl.textContent = text || "";
      statusEl.className = "status" + (cls ? " " + cls : "");
    }

    async function run() {
      const shareUrl = urlInput.value.trim();
      if (!shareUrl) {
        setStatus("请输入分享链接", "error");
        return;
      }
      fetchBtn.disabled = true;
      setStatus("查询中...", "");
      try {
        const resp = await fetch("/api/fetch_video_profile", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ url: shareUrl })
        });
        const data = await resp.json();
        if (!resp.ok) {
          throw new Error((data && (data.error || data.errMsg)) || ("HTTP " + resp.status));
        }
        outputEl.textContent = JSON.stringify(data, null, 2);
        setStatus("请求成功", "ok");
      } catch (err) {
        outputEl.textContent = JSON.stringify({ error: err.message }, null, 2);
        setStatus(err.message, "error");
      } finally {
        fetchBtn.disabled = false;
      }
    }

    fetchBtn.addEventListener("click", run);
    urlInput.addEventListener("keydown", function (event) {
      if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
        run();
      }
    });
  </script>
</body>
</html>`;

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
  };
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      ...corsHeaders(),
      "Content-Type": "application/json",
    },
  });
}

function log(...args) {
  console.log(`[${new Date().toISOString()}]`, ...args);
}

const PARSE_URL = "https://yuanbao.tencent.com/api/weixin/get_parse_result";
const FEED_INFO_URL = "https://channels.weixin.qq.com/finder-preview/api/feed/get_feed_info";

const PARSE_HEADERS = {
  "accept": "application/json, text/plain, */*",
  "accept-language": "zh-CN,zh;q=0.9,en;q=0.8",
  "content-type": "application/json",
  "origin": "https://yuanbao.tencent.com",
  "referer": "https://yuanbao.tencent.com/chat/naQivTmsDa/cf4d0079-ed1b-4c55-a3f3-2ca1379727d1",
  "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
  "sec-ch-ua": `"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`,
  "sec-ch-ua-mobile": "?0",
  "sec-ch-ua-platform": `"macOS"`,
  "sec-fetch-dest": "empty",
  "sec-fetch-mode": "cors",
  "sec-fetch-site": "same-origin",
  "t-userid": "b9575f6b0a8c4a55a08096904a5ef20a",
  "x-agentid": "naQivTmsDa/cf4d0079-ed1b-4c55-a3f3-2ca1379727d1",
  "x-commit-tag": "72282a0d",
  "x-device-id": "1921b001708100d7fa31002b9646bd0cc15a3e2e1f",
  "x-hy106": "",
  "x-hy92": "e963067ffa31002b9646bd0c03000008b1951a",
  "x-hy93": "1921b001708100d7fa31002b9646bd0cc15a3e2e1f",
  "x-id": "b9575f6b0a8c4a55a08096904a5ef20a",
  "x-instance-id": "5",
  "x-language": "zh-CN",
  "x-os_version": "Mac OS(10.15.7)-Blink",
  "x-platform": "mac",
  "x-requested-with": "XMLHttpRequest",
  "x-source": "web",
  "x-web-third-source": "main",
  "x-webdriver": "0",
  "x-webversion": "2.69.0",
  "x-ybuitest": "0",
};

const FEED_INFO_HEADERS = {
  "accept": "application/json, text/plain, */*",
  "accept-language": "zh-CN,zh;q=0.9,en;q=0.8",
  "connection": "keep-alive",
  "content-type": "application/json",
  "origin": "https://channels.weixin.qq.com",
  "sec-fetch-dest": "empty",
  "sec-fetch-mode": "cors",
  "sec-fetch-site": "same-origin",
  "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
  "sec-ch-ua": `"Chromium";v="148", "Google Chrome";v="148", "Not/A)Brand";v="99"`,
  "sec-ch-ua-mobile": "?0",
  "sec-ch-ua-platform": `"macOS"`,
};

function generateRid() {
  const timestampHex = Math.floor(Date.now() / 1000).toString(16);
  let randomHex = "";
  const chars = "0123456789abcdef";
  for (let i = 0; i < 8; i++) {
    randomHex += chars[Math.floor(Math.random() * 16)];
  }
  return `${timestampHex}-${randomHex}`;
}

function cleanVideoURL(videoUrl) {
  try {
    const u = new URL(videoUrl);
    const encfilekey = u.searchParams.get("encfilekey") || "";
    const token = u.searchParams.get("token") || "";
    if (!encfilekey || !token) {
      return "";
    }
    return `${u.origin}${u.pathname}?encfilekey=${encodeURIComponent(encfilekey)}&token=${encodeURIComponent(token)}`;
  } catch (_) {
    return "";
  }
}

async function parseShareUrl(shareUrl, cookie) {
  const resp = await fetch(PARSE_URL, {
    method: "POST",
    headers: {
      ...PARSE_HEADERS,
      cookie,
    },
    body: JSON.stringify({
      type: "video_channel_url",
      url: shareUrl,
      scene: 1,
    }),
  });

  if (!resp.ok) {
    throw new Error(`parseShareUrl: http ${resp.status}`);
  }

  const result = await resp.json();
  if (typeof result.code === "number" && result.code !== 0) {
    throw new Error(result.msg || `parseShareUrl: code=${result.code}`);
  }
  if (!result.data || (!result.data.playable_url && !result.data.wx_export_id)) {
    throw new Error("parseShareUrl: missing playable_url and wx_export_id");
  }

  return result.data;
}

async function getFeedInfo(exportId, generalToken) {
  const rid = generateRid();
  const referer =
    `https://channels.weixin.qq.com/finder-preview/pages/feed` +
    `?entry_card_type=48&comment_scene=39&appid=0` +
    `&token=${encodeURIComponent(generalToken)}` +
    `&entry_scene=0&eid=${encodeURIComponent(exportId)}`;

  const resp = await fetch(
    `${FEED_INFO_URL}?_rid=${rid}&_pageUrl=https:%2F%2Fchannels.weixin.qq.com%2Ffinder-preview%2Fpages%2Ffeed`,
    {
      method: "POST",
      headers: {
        ...FEED_INFO_HEADERS,
        referer,
      },
      body: JSON.stringify({
        baseReq: { generalToken },
        exportId,
      }),
    }
  );

  if (!resp.ok) {
    throw new Error(`getFeedInfo: http ${resp.status}`);
  }

  const result = await resp.json();
  if (typeof result.errCode === "number" && result.errCode !== 0) {
    throw new Error(result.errMsg || `getFeedInfo: errCode=${result.errCode}`);
  }

  return result;
}

async function fetchVideoProfile(shareUrl, cookie) {
  const parseData = await parseShareUrl(shareUrl, cookie);

  let generalToken = "";
  let exportId = "";
  try {
    const playableUrl = new URL(parseData.playable_url || "");
    generalToken = playableUrl.searchParams.get("token") || "";
    exportId = playableUrl.searchParams.get("eid") || "";
  } catch (_) {
  }

  if (!exportId) {
    exportId = parseData.wx_export_id || "";
  }
  if (!exportId) {
    throw new Error("parse share url: missing export id");
  }
  if (!generalToken) {
    throw new Error("parse share url: missing general token");
  }

  const result = await getFeedInfo(exportId, generalToken);
  if (result && result.data && result.data.feedInfo) {
    const cleaned = cleanVideoURL(result.data.feedInfo.originVideoUrl || result.data.feedInfo.videoUrl || "");
    if (cleaned) {
      result.data.feedInfo.originVideoUrl = cleaned;
    }
    if (result.data.sceneInfo && !result.data.sceneInfo.dynamicExportId) {
      result.data.sceneInfo.dynamicExportId = exportId;
    }
  }
  return result;
}

async function handleFetchVideoProfile(request, env) {
  try {
    const cookie = (env && env.COOKIE ? String(env.COOKIE) : "").trim();
    if (!cookie) {
      return jsonResponse({ error: "missing worker env COOKIE" }, 500);
    }

    const body = await request.json();
    const shareUrl = body && typeof body.url === "string" ? body.url.trim() : "";
    if (!shareUrl) {
      return jsonResponse({ error: "missing url" }, 400);
    }

    log("[sph-worker] fetch video profile", shareUrl);
    const result = await fetchVideoProfile(shareUrl, cookie);
    return jsonResponse(result, 200);
  } catch (err) {
    log("[sph-worker] error", err && err.message ? err.message : err);
    return jsonResponse({ error: err && err.message ? err.message : "internal error" }, 500);
  }
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (request.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders() });
    }

    if (request.method === "GET" && url.pathname === "/") {
      return new Response(INDEX_HTML, {
        headers: { "Content-Type": "text/html; charset=utf-8" },
      });
    }

    if (request.method === "POST" && url.pathname === "/api/fetch_video_profile") {
      return handleFetchVideoProfile(request, env);
    }

    return new Response("not found", { status: 404 });
  },
};
