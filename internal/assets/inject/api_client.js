/**
 * @file API 客户端 - 通过 WebSocket 与后端通信
 */
console.log('[api_client.js] 加载 API 客户端模块');

window.__wx_api_client = {
  ws: null,
  connected: false,
  connecting: false,
  initialized: false,
  connectToken: 0,
  reconnectTimer: null,
  reconnectDelay: 3000,
  requests: {},
  heartbeatTimer: null,
  lastHeartbeatTime: 0,
  missedHeartbeats: 0,
  apiMethods: {},

  // 初始化
  init: function () {
    if (this.initialized) {
      console.log('[API客户端] 已初始化，跳过重复启动');
      return;
    }
    this.initialized = true;
    this.connect();
    this.setupVisibilityHandler();
    this.setupBeforeUnloadHandler();
    this.scheduleInjectHealthReports('init');
  },

  decodeFeedProfileURL: function (rawURL) {
    if (!rawURL) {
      return '';
    }
    try {
      return decodeURIComponent(rawURL);
    } catch (err) {
      return rawURL;
    }
  },

  isSharedFeedURL: function (rawURL) {
    var decoded = this.decodeFeedProfileURL(rawURL);
    if (!decoded) {
      return false;
    }

    try {
      var u = new URL(decoded, window.location.origin);
      return (u.hostname === 'weixin.qq.com' && u.pathname.indexOf('/sph/') >= 0) ||
        (u.hostname === 'channels.weixin.qq.com' && u.pathname.indexOf('/finder-preview/pages/sph') >= 0);
    } catch (err) {
      return decoded.indexOf('weixin.qq.com/sph/') >= 0 ||
        decoded.indexOf('channels.weixin.qq.com/finder-preview/pages/sph') >= 0;
    }
  },

  extractSharedFeedShortUri: function (rawURL) {
    var decoded = this.decodeFeedProfileURL(rawURL);
    if (!decoded) {
      return '';
    }

    var u = new URL(decoded, window.location.origin);
    var match = u.pathname.match(/\/sph\/([a-zA-Z0-9_-]+)/);
    if (match) {
      return match[1];
    }
    return u.searchParams.get('id') || '';
  },

  extractSharedFeedFallbackEID: function (rawURL) {
    var shortUri = this.extractSharedFeedShortUri(rawURL);
    if (shortUri) {
      return shortUri;
    }

    var decoded = this.decodeFeedProfileURL(rawURL);
    if (!decoded) {
      return '';
    }

    var match = decoded.match(/\/([a-zA-Z0-9_-]{1,})$/);
    if (match && match[1]) {
      return match[1];
    }

    return '';
  },

  extractSharedFeedExportID: function (data, rawURL) {
    var payload = data && data.data ? data.data : {};
    var sceneInfo = payload && payload.sceneInfo ? payload.sceneInfo : {};
    var object = payload && payload.object ? payload.object : {};
    var exportID = sceneInfo.dynamicExportId ||
      sceneInfo.exportId ||
      payload.exportId ||
      payload.eid ||
      object.id ||
      '';

    if (!exportID && rawURL) {
      try {
        var u = new URL(this.decodeFeedProfileURL(rawURL), window.location.origin);
        exportID = u.searchParams.get('eid') || '';
      } catch (err) {
        exportID = '';
      }
    }

    return exportID ? String(exportID).trim() : '';
  },

  cleanSharedFeedMediaURL: function (rawURL) {
    var trimmed = rawURL ? String(rawURL).trim() : '';
    if (!trimmed) {
      return '';
    }

    try {
      var parsed = new URL(trimmed, window.location.origin);
      var fileKey = (parsed.searchParams.get('encfilekey') || '').trim();
      var token = (parsed.searchParams.get('token') || '').trim();
      if (!fileKey || !token) {
        return trimmed;
      }

      var clean = new URL(parsed.origin + parsed.pathname);
      clean.searchParams.set('encfilekey', fileKey);
      clean.searchParams.set('token', token);
      return clean.toString();
    } catch (err) {
      return trimmed;
    }
  },

  fetchSharedFeedInfo: async function (rawURL) {
    var shortUri = this.extractSharedFeedShortUri(rawURL);
    if (!shortUri) {
      throw new Error('无法从分享链接中解析 shortUri');
    }

    var resp = await fetch('/finder-preview/api/feed/get_feed_info', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      credentials: 'include',
      body: JSON.stringify({
        baseReq: {
          generalToken: ''
        },
        shortUri: shortUri
      })
    });

    if (!resp.ok) {
      throw new Error('获取分享视频信息失败: HTTP ' + resp.status);
    }

    var data = await resp.json();
    if (data && typeof data.errCode === 'number' && data.errCode !== 0) {
      throw new Error(data.errMsg || ('获取分享视频信息失败: ' + data.errCode));
    }

    return data;
  },

  fetchSharedFeedExportID: async function (rawURL) {
    var data = await this.fetchSharedFeedInfo(rawURL);
    var exportID = this.extractSharedFeedExportID(data, rawURL);
    if (!exportID) {
      throw new Error("获取分享视频信息失败: 缺少可用的 exportId");
    }

    return exportID;
  },

  resolveSharedFeedExportID: async function (rawURL) {
    try {
      var exportID = await this.fetchSharedFeedExportID(rawURL);
      if (exportID) {
        return exportID;
      }
    } catch (err) {
      var fallbackEID = this.extractSharedFeedFallbackEID(rawURL);
      if (fallbackEID) {
        return fallbackEID;
      }
      throw err;
    }

    var fallback = this.extractSharedFeedFallbackEID(rawURL);
    if (fallback) {
      return fallback;
    }

    throw new Error('获取分享视频信息失败: 缺少可用的 exportId');
  },

  buildSharedFeedCompatResponse: function (rawURL, data) {
    var payload = data && data.data ? data.data : {};
    var object = payload && payload.object ? payload.object : {};
    var objectDesc = object && object.objectDesc ? object.objectDesc : {};
    var mediaList = objectDesc && Array.isArray(objectDesc.media) ? objectDesc.media : [];
    var sceneInfo = payload && payload.sceneInfo ? payload.sceneInfo : {};

    if (mediaList.length > 0) {
      var mediaCopy = Object.assign({}, mediaList[0]);
      if (!mediaCopy.url && mediaCopy.urlToken) {
        mediaCopy.url = mediaCopy.urlToken;
        mediaCopy.urlToken = '';
      }
      if (mediaCopy.url) {
        mediaCopy.url = this.cleanSharedFeedMediaURL(mediaCopy.url);
      }
      objectDesc.media = [mediaCopy];
      object.objectDesc = objectDesc;
      payload.object = object;
      if (!sceneInfo.dynamicExportId) {
        sceneInfo.dynamicExportId = this.extractSharedFeedExportID(data, rawURL) || object.id || 'shared_feed';
        payload.sceneInfo = sceneInfo;
      }
      return {
        errCode: typeof data.errCode === 'number' ? data.errCode : 0,
        errMsg: data && data.errMsg ? data.errMsg : 'ok',
        data: payload
      };
    }

    var feedInfo = payload && payload.feedInfo ? payload.feedInfo : {};
    var authorInfo = payload && payload.authorInfo ? payload.authorInfo : {};
    var exportID = this.extractSharedFeedExportID(data, rawURL) || 'shared_feed';
    var mediaURL = this.cleanSharedFeedMediaURL(
      feedInfo.originVideoUrl ||
      feedInfo.videoUrl ||
      ((feedInfo.h264VideoInfo && feedInfo.h264VideoInfo.videoUrl) || '') ||
      ((feedInfo.h265VideoInfo && feedInfo.h265VideoInfo.videoUrl) || '')
    );
    var coverURL = feedInfo.coverUrl || feedInfo.thumbUrl || '';

    return {
      errCode: typeof data.errCode === 'number' ? data.errCode : 0,
      errMsg: data && data.errMsg ? data.errMsg : 'ok',
      data: {
        object: {
          id: exportID,
          nickname: authorInfo.nickname || '',
          headUrl: authorInfo.headImgUrl || '',
          contact: {
            nickname: authorInfo.nickname || '',
            headImgUrl: authorInfo.headImgUrl || '',
            authIconUrl: authorInfo.authIconUrl || ''
          },
          objectDesc: {
            description: feedInfo.description || '',
            mediaType: feedInfo.mediaType || 4,
            media: mediaURL ? [{
              url: mediaURL,
              urlToken: '',
              decodeKey: feedInfo.decodeKey || '',
              decryptKey: feedInfo.decryptKey || '',
              thumbUrl: coverURL,
              coverUrl: coverURL,
              fullThumbUrl: coverURL,
              fileSize: feedInfo.fileSize || 0,
              durationMs: feedInfo.durationMs || 0,
              videoDuration: feedInfo.videoDuration || 0,
              videoPlayLen: feedInfo.videoPlayLen || 0,
              videoResolution: feedInfo.videoResolution || ''
            }] : []
          }
        },
        sceneInfo: {
          dynamicExportId: exportID
        },
        feedInfo: feedInfo,
        authorInfo: authorInfo
      }
    };
  },

  hasSharedFeedMedia: function (responseData) {
    var payload = responseData && responseData.data ? responseData.data : {};
    var object = payload && payload.object ? payload.object : {};
    var objectDesc = object && object.objectDesc ? object.objectDesc : {};
    var media = objectDesc && Array.isArray(objectDesc.media) ? objectDesc.media : [];
    if (!media.length) {
      return false;
    }
    return !!((media[0] && media[0].url) || '');
  },

  fetchSharedFeedProfile: async function (body, rawURL) {
    var sharedInfo = await this.fetchSharedFeedInfo(rawURL);
    var compatResponse = this.buildSharedFeedCompatResponse(rawURL, sharedInfo);
    if (this.hasSharedFeedMedia(compatResponse)) {
      return {
        payload: {
          shortUri: this.extractSharedFeedShortUri(rawURL),
          source: 'short_uri_feed_info'
        },
        response: compatResponse
      };
    }

    var exportID = this.extractSharedFeedExportID(sharedInfo, rawURL) || this.extractSharedFeedFallbackEID(rawURL);
    if (!exportID) {
      throw new Error('获取分享视频信息失败: 页面接口未返回可用媒体地址或 exportId');
    }

    var payload = await this.buildFeedProfilePayload({
      objectId: body.objectId || body.object_id || body.oid || '',
      nonceId: body.nonceId || body.nonce_id || body.nid || '',
      eid: exportID
    });
    var response = await window.WXU.API.finderGetCommentDetail(payload);
    return {
      payload: payload,
      response: response
    };
  },

  buildFeedProfilePayload: async function (body) {
    body = body || {};

    var oid = body.objectId || body.object_id || body.oid || '';
    var nid = body.nonceId || body.nonce_id || body.nid || '';
    var eid = body.eid || body.encryptedObjectId || body.encrypted_objectid || '';
    var rawURL = body.url ? this.decodeFeedProfileURL(body.url) : '';

    if (rawURL && !eid) {
      if (this.isSharedFeedURL(rawURL)) {
        eid = await this.resolveSharedFeedExportID(rawURL);
      } else {
        var u = new URL(rawURL, window.location.origin);
        var encodedOID = u.searchParams.get('oid');
        var encodedNID = u.searchParams.get('nid');
        if (encodedOID) {
          oid = window.WXU.API.decodeBase64ToUint64String(encodedOID);
        }
        if (encodedNID) {
          nid = window.WXU.API.decodeBase64ToUint64String(encodedNID);
        }
      }
    }

    if (!eid && (!oid || !nid)) {
      throw new Error('缺失 object_id 或 nonce_id');
    }

    return {
      needObject: 1,
      lastBuffer: '',
      scene: eid ? 141 : 146,
      direction: 2,
      identityScene: 2,
      pullScene: 6,
      objectid: eid ? undefined : (String(oid).indexOf('_') >= 0 ? String(oid).split('_')[0] : String(oid)),
      objectNonceId: eid ? undefined : nid,
      encrypted_objectid: eid || ''
    };
  },

  resolveSharedFeedProfile: async function (body) {
    body = body || {};
    var rawURL = body.url ? this.decodeFeedProfileURL(body.url) : '';
    if (rawURL && this.isSharedFeedURL(rawURL)) {
      return this.fetchSharedFeedProfile(body, rawURL);
    }

    return this.fetchFeedProfile(body);
  },

  fetchFeedProfile: async function (body) {
    body = body || {};
    var payload = await this.buildFeedProfilePayload(body);
    var response = await window.WXU.API.finderGetCommentDetail(payload);
    return {
      payload: payload,
      response: response
    };
  },

  // 设置页面可见性监听
  setupVisibilityHandler: function () {
    var self = this;

    document.addEventListener('visibilitychange', function () {
      if (!document.hidden) {
        // 页面变为可见
        console.log('[API客户端] 📱 页面激活，检查连接状态...');

        if (!self.connected) {
          console.log('[API客户端] 连接已断开，立即重连...');
          // 清除现有的重连定时器
          if (self.reconnectTimer) {
            clearTimeout(self.reconnectTimer);
            self.reconnectTimer = null;
          }
          // 立即重连
          self.connect();
        } else {
          // 连接还在，发送一个心跳测试
          self.sendHeartbeat();
        }
      } else {
        // 页面变为隐藏
        console.log('[API客户端] 📴 页面进入后台');
      }
    });

    console.log('[API客户端] ✅ 页面可见性监听已启动');
  },

  // 设置页面关闭前的处理
  setupBeforeUnloadHandler: function () {
    var self = this;

    window.addEventListener('beforeunload', function () {
      // 页面即将关闭，清理资源
      if (self.ws && self.connected) {
        self.ws.close(1000, 'Page unloading');
      }

      if (self.heartbeatTimer) {
        clearInterval(self.heartbeatTimer);
      }

      if (self.reconnectTimer) {
        clearTimeout(self.reconnectTimer);
      }
    });
  },

  // 连接 WebSocket
  connect: function () {
    if (this.connected) {
      return;
    }
    if (this.ws && this.ws.readyState === WebSocket.CONNECTING) {
      console.log('[API客户端] 连接已在进行中，跳过重复 connect');
      return;
    }
    this.connecting = true;
    this.connectToken += 1;
    var token = this.connectToken;

    // 检测代理端口
    // 方法1: 尝试从 /__wx_channels_api 端点获取端口信息
    // 方法2: 使用默认端口 2026
    var wsPort = 2026; // 默认端口

    // 尝试多个可能的端口
    var possiblePorts = [2026, 9527, 8081, 3001];

    // 从 localStorage 获取上次成功的端口
    try {
      var lastPort = localStorage.getItem('__wx_api_ws_port');
      if (lastPort) {
        possiblePorts.unshift(parseInt(lastPort));
      }
    } catch (e) {
      // ignore
    }

    // 尝试连接
    this.tryConnect(possiblePorts, 0, token);
  },

  // 尝试连接到指定端口
  tryConnect: function (ports, index, token) {
    var self = this;

    if (token !== this.connectToken) {
      return;
    }

    if (index >= ports.length) {
      this.connecting = false;
      console.error('[API客户端] 所有端口都连接失败，3秒后重试...');
      this.reconnectTimer = setTimeout(function () {
        self.connect();
      }, this.reconnectDelay);
      return;
    }

    var wsPort = ports[index];
    var wsUrl = 'ws://127.0.0.1:' + wsPort + '/ws/api';
    if (window.__WX_LOCAL_TOKEN__) {
      wsUrl += '?token=' + encodeURIComponent(window.__WX_LOCAL_TOKEN__);
    }

    console.log('[API客户端] 尝试连接:', wsUrl);

    // 标记当前尝试的端口索引
    this.currentPortIndex = index;
    this.currentPorts = ports;

    try {
      var ws = new WebSocket(wsUrl);
      this.ws = ws;

      // 设置连接超时（5秒）
      var connectTimeout = setTimeout(function () {
        if (token !== self.connectToken) return;
        if (!self.connected && self.ws === ws && ws.readyState !== WebSocket.OPEN) {
          console.log('[API客户端] 连接超时，尝试下一个端口...');
          ws.close();
          self.tryConnect(ports, index + 1, token);
        }
      }, 5000);

      ws.onopen = function () {
        if (token !== self.connectToken || self.ws !== ws) {
          try { ws.close(); } catch (e) {}
          return;
        }
        clearTimeout(connectTimeout);
        self.connected = true;
        self.connecting = false;
        console.log('[API客户端] ✅ 已连接到后端: ws://127.0.0.1:' + wsPort + '/ws/api');

        // 保存成功的端口
        try {
          localStorage.setItem('__wx_api_ws_port', wsPort);
        } catch (e) {
          // ignore
        }

        // 清除重连定时器
        if (self.reconnectTimer) {
          clearTimeout(self.reconnectTimer);
          self.reconnectTimer = null;
        }

        // 启动心跳
        self.startHeartbeat();
        self.sendClientState();
        self.scheduleInjectHealthReports('ws_open');
      };

      ws.onmessage = function (event) {
        if (token !== self.connectToken || self.ws !== ws) return;
        try {
          var msg = JSON.parse(event.data);
          self.handleMessage(msg);
        } catch (err) {
          console.error('[API客户端] 解析消息失败:', err);
        }
      };

      ws.onerror = function (error) {
        if (token !== self.connectToken || self.ws !== ws) return;
        clearTimeout(connectTimeout);
        console.error('[API客户端] ❌ WebSocket 错误:', error);
        // 如果还没有连接成功，尝试下一个端口
        if (!self.connected) {
          self.tryConnect(ports, index + 1, token);
        }
      };

      ws.onclose = function (event) {
        if (token !== self.connectToken || self.ws !== ws) return;
        clearTimeout(connectTimeout);
        console.log('[API客户端] 🔌 连接关闭:', event.code, event.reason);

        // 停止心跳
        self.stopHeartbeat();
        self.connecting = false;

        if (self.connected) {
          // 之前连接成功过，现在断开了，需要重连
          self.connected = false;
          console.log('[API客户端] 连接已关闭，3秒后重连...');

          // 自动重连（使用之前成功的端口）
          self.reconnectTimer = setTimeout(function () {
            self.connect();
          }, self.reconnectDelay);
        } else {
          // 连接从未成功，尝试下一个端口
          self.tryConnect(ports, index + 1, token);
        }
      };
    } catch (err) {
      this.connecting = false;
      console.error('[API客户端] ❌ 连接失败:', err);
      // 尝试下一个端口
      this.tryConnect(ports, index + 1, token);
    }
  },

  // 处理消息
  handleMessage: function (msg) {
    if (msg.type === 'api_call') {
      this.handleAPICall(msg.data);
    } else if (msg.type === 'cmd') {
      this.handleCommand(msg.data);
    } else if (msg.type === 'pong') {
      this.lastHeartbeatTime = Date.now();
    }
  },

  collectClientState: function () {
    var methods = {};
    if (window.WXU) {
      methods.finderGetCommentDetail = !!(window.WXU.API && typeof window.WXU.API.finderGetCommentDetail === 'function');
      methods.finderGetCommentList = !!(window.WXU.API && typeof window.WXU.API.finderGetCommentList === 'function');
      methods.finderUserPage = !!(window.WXU.API && typeof window.WXU.API.finderUserPage === 'function');
      methods.finderSearch = !!(window.WXU.API2 && typeof window.WXU.API2.finderSearch === 'function');
      methods.finderGetInteractionedFeedList = !!(window.WXU.API4 && typeof window.WXU.API4.finderGetInteractionedFeedList === 'function');
    }
    this.apiMethods = methods;
    return {
      pagePath: window.location.pathname,
      href: window.location.href,
      apiReady: !!(methods.finderGetCommentDetail || methods.finderGetCommentList || methods.finderUserPage || methods.finderSearch || methods.finderGetInteractionedFeedList),
      methods: methods,
      injectHealth: this.collectInjectHealth(),
      timestamp: Date.now(),
      userAgent: navigator.userAgent,
      visible: !document.hidden
    };
  },

  collectInjectHealth: function () {
    var health = this.collectInjectHealthSnapshot();
    var self = this;
    window.__wx_channels_inject_health_last__ = health;
    window.__wx_channels_inject_health__ = function () {
      return self.collectInjectHealthSnapshot();
    };
    return health;
  },

  collectInjectHealthSnapshot: function () {
    var store = window.__wx_channels_store__ || null;
    var profile = store && store.profile ? store.profile : null;
    var now = Date.now();
    return {
      wxu: !!window.WXU,
      wxe: !!window.WXE,
      store: !!store,
      profile: !!profile,
      hasUrl: !!(profile && (profile.url || profile.originalUrl || (profile.media && profile.media.url))),
      hasKey: !!(profile && profile.key),
      title: (profile && profile.title) || '',
      id: (profile && profile.id) || '',
      pagePath: window.location.pathname,
      href: window.location.href,
      timestamp: now,
      ts: now
    };
  },

  reportInjectHealth: function (reason) {
    var payload = this.collectInjectHealth();
    payload.reason = reason || '';

    var headers = { 'Content-Type': 'application/json' };
    if (window.__WX_LOCAL_TOKEN__) {
      headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
    }

    return fetch('/__wx_channels_api/inject_health', {
      method: 'POST',
      headers: headers,
      body: JSON.stringify(payload)
    }).catch(function (err) {
      console.warn('[API客户端] 上报注入健康失败:', err);
    });
  },

  scheduleInjectHealthReports: function (reason) {
    var self = this;
    self.reportInjectHealth(reason);
    setTimeout(function () {
      self.reportInjectHealth(reason + ':delayed');
    }, 1500);
  },

  sendClientState: function () {
    if (!this.connected || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    try {
      this.ws.send(JSON.stringify({
        type: 'client_state',
        data: this.collectClientState()
      }));
    } catch (err) {
      console.error('[API客户端] 发送客户端状态失败:', err);
    }
  },

  // 处理指令
  handleCommand: function (data) {
    console.log('[API客户端] 收到指令:', data);

    if (data.action === 'download_progress') {
      // 派发自定义事件，供 UI 组件消费
      var event = new CustomEvent('wx_download_progress', { detail: data.payload });
      document.dispatchEvent(event);
    }

    if (data.action === 'download_complete') {
      if (typeof __wx_log === 'function') {
        __wx_log({ msg: '✓ 视频已下载' + (data.payload && data.payload.decrypted ? '并解密' : '') });
      }
      var completeEvent = new CustomEvent('wx_download_complete', { detail: data.payload });
      document.dispatchEvent(completeEvent);
    }

    if (data.action === 'download_failed') {
      if (typeof __wx_log === 'function') {
        __wx_log({ msg: '❌ 下载视频失败: ' + ((data.payload && data.payload.error) || '未知错误') });
      }
      var failedEvent = new CustomEvent('wx_download_failed', { detail: data.payload });
      document.dispatchEvent(failedEvent);
    }
  },

  // 处理 API 调用请求
  handleAPICall: async function (data) {
    var id = data.id;
    var key = data.key;
    var body = data.body;

    // 响应函数
    var self = this;
    function resp(responseData) {
      self.sendResponse(id, responseData);
    }

    try {
      if (key === 'key:channels:download_video') {
        var headers = { 'Content-Type': 'application/json' };
        if (window.__WX_LOCAL_TOKEN__) {
          headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
        }

        try {
          var downloadResp = await fetch('/__wx_channels_api/download_video', {
            method: 'POST',
            headers: headers,
            body: JSON.stringify(body || {})
          });
          var downloadData = await downloadResp.json().catch(function () { return {}; });

          if (!downloadResp.ok || !downloadData || downloadData.success === false) {
            resp({
              errCode: downloadResp.status || 1011,
              errMsg: (downloadData && (downloadData.error || downloadData.message)) || '下载视频失败',
              payload: body,
              response: downloadData
            });
            return;
          }

          resp(downloadData);
          return;
        } catch (err) {
          resp({
            errCode: 1011,
            errMsg: err.message || '下载视频失败',
            payload: body
          });
          return;
        }
      }

      // 等待 WXU.API 和 WXU.API2 初始化
      var maxWait = 10000; // 最多等待10秒
      var startTime = Date.now();

      while ((!window.WXU || !window.WXU.API || !window.WXU.API2) && (Date.now() - startTime < maxWait)) {
        console.log('[API客户端] 等待 WXU.API 初始化...');
        await new Promise(function (resolve) { setTimeout(resolve, 500); });
      }

      if (!window.WXU || !window.WXU.API || !window.WXU.API2) {
        resp({
          errCode: 1,
          errMsg: 'WXU.API 未初始化，请刷新页面重试'
        });
        return;
      }

      if (key === 'key:channels:contact_list') {
        // Correct Scene Mapping:
        // Type 1 (User): Scene 13 → infoList (supports pagination)
        // Type 2 (Live): Scene 13 → objectList (NO pagination support)
        // Type 3 (Video): Scene 19 → objectList (supports pagination)
        var scene = 13; // Default to Scene 13 for Type 1 and Type 2
        if (body.type == 3) {
          scene = 19; // Only Type 3 (Video) uses Scene 19
        }

        var payload = {
          query: body.keyword,
          scene: scene,
          requestId: String(new Date().valueOf()), // Unique request ID for every page
          lastBuffer: body.next_marker ? decodeURIComponent(body.next_marker) : '',
          lastBuff: body.next_marker ? decodeURIComponent(body.next_marker) : '', // Try alias
        };
        var r = await window.WXU.API2.finderSearch(payload);
        console.log('[API客户端] finderSearch 结果:', r);
        resp({
          ...r,
          payload: payload
        });
        return;
      }

      // 获取账号视频列表
      if (key === 'key:channels:feed_list') {
        var payload = {
          username: body.username,
          finderUsername: window.__wx_username || '',
          lastBuffer: body.next_marker ? decodeURIComponent(body.next_marker) : '',
          needFansCount: 0,
          objectId: '0'
        };
        var r = await window.WXU.API.finderUserPage(payload);
        console.log('[API客户端] finderUserPage 结果:', r);
        resp({
          ...r,
          payload: payload
        });
        return;
      }

      // 获取视频详情
      if (key === 'key:channels:feed_profile' || key === 'key:channels:shared_feed_profile' || key === 'key:channels:shared_feed_resolve') {
        console.log('[API客户端] 获取视频详情:', body);

        try {
          var profileResult = key === 'key:channels:shared_feed_resolve'
            ? await this.resolveSharedFeedProfile(body)
            : await this.fetchFeedProfile(body);
          console.log('[API客户端] finderGetCommentDetail 结果:', profileResult.response);
          resp({
            ...profileResult.response,
            payload: profileResult.payload
          });
          return;
        } catch (err) {
          console.error('[API客户端] 获取视频详情失败:', err);
          resp({
            errCode: 1011,
            errMsg: err.message,
            payload: body
          });
          return;
        }
      }

      if (key === 'key:channels:fetch_feed_comment_list') {
        if (!body.object_id) {
          resp({
            errCode: 1011,
            errMsg: '缺失 object_id',
            payload: body
          });
          return;
        }

        if (!body.nonce_id && !body.comment_id) {
          resp({
            errCode: 1011,
            errMsg: '缺失 nonce_id 或 comment_id',
            payload: body
          });
          return;
        }

        var payload = body.comment_id ? {
          direction: 2,
          identityScene: 2,
          objectId: body.object_id,
          rootCommentId: body.comment_id,
          lastBuffer: body.next_marker ? decodeURIComponent(body.next_marker) : undefined
        } : {
          finderBasereq: {
            scene: 140,
            ctxInfo: {
              clientReportBuff: '{"entranceId":"1002"}'
            },
            objectBaseInfos: []
          },
          objectId: body.object_id,
          objectNonceId: body.nonce_id,
          direction: 2,
          identityScene: 2,
          lastBuffer: body.next_marker ? decodeURIComponent(body.next_marker) : undefined,
          enterSessionId: String(Date.now())
        };

        try {
          var commentResp = await window.WXU.API.finderGetCommentList(payload);
          console.log('[API客户端] finderGetCommentList 结果:', commentResp);
          resp({
            ...commentResp,
            payload: payload
          });
        } catch (err) {
          console.error('[API客户端] 获取评论列表失败:', err);
          resp({
            errCode: 1011,
            errMsg: err.message,
            payload: payload
          });
        }
        return;
      }

      // 未匹配的 key
      resp({
        errCode: 1000,
        errMsg: '未匹配的key: ' + key,
        payload: data
      });

    } catch (err) {
      console.error('[API客户端] API 调用失败:', err);
      resp({
        errCode: 1,
        errMsg: err.message || 'API 调用失败',
        payload: data
      });
    }
  },

  // 发送响应
  sendResponse: function (id, responseData) {
    if (!this.connected || !this.ws) {
      console.error('[API客户端] WebSocket 未连接');
      return;
    }

    // 构建响应消息
    // 后端期望的格式: {type: "api_response", data: {id: "xxx", data: {...}, errCode: 0, errMsg: "ok"}}
    var msg = {
      type: 'api_response',
      data: {
        id: id,
        data: responseData,  // 整个 responseData 作为 data 字段
        errCode: responseData.errCode || 0,
        errMsg: responseData.errMsg || 'ok'
      }
    };

    try {
      var msgStr = JSON.stringify(msg);
      this.ws.send(msgStr);
    } catch (err) {
      console.error('[API客户端] 发送响应失败:', err);
    }
  },

  // 启动心跳
  startHeartbeat: function () {
    var self = this;

    // 清除旧的心跳定时器
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
    }

    // 重置心跳计数
    this.missedHeartbeats = 0;
    this.lastHeartbeatTime = Date.now();

    // 每 30 秒发送一次心跳
    this.heartbeatTimer = setInterval(function () {
      self.sendHeartbeat();
    }, 30000);

    console.log('[API客户端] ✅ 心跳已启动 (30秒间隔)');
  },

  // 停止心跳
  stopHeartbeat: function () {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
      console.log('[API客户端] ⏹️ 心跳已停止');
    }
  },

  // 发送心跳
  sendHeartbeat: function () {
    if (!this.connected || !this.ws) {
      console.warn('[API客户端] 无法发送心跳：未连接');
      this.missedHeartbeats++;

      // 连续 3 次心跳失败，触发重连
      if (this.missedHeartbeats >= 3) {
        console.error('[API客户端] 心跳连续失败，触发重连...');
        this.stopHeartbeat();

        // 关闭当前连接
        if (this.ws) {
          try {
            this.ws.close();
          } catch (e) {
            // ignore
          }
        }

        // 立即重连
        this.connected = false;
        this.connect();
      }
      return;
    }

    try {
      var heartbeat = {
        type: 'ping',
        timestamp: Date.now()
      };

      this.ws.send(JSON.stringify(heartbeat));
      this.lastHeartbeatTime = Date.now();
      this.missedHeartbeats = 0;

      console.log('[API客户端] 💓 心跳已发送');
    } catch (err) {
      console.error('[API客户端] 发送心跳失败:', err);
      this.missedHeartbeats++;
    }
  }
};

// 自动初始化
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', function () {
    window.__wx_api_client.init();
  });
} else {
  window.__wx_api_client.init();
}

// 监听初始化事件，获取用户名
if (window.WXE && window.WXE.onInit) {
  window.WXE.onInit(function (data) {
    if (data && data.mainFinderUsername) {
      window.__wx_username = data.mainFinderUsername;
      console.log('[API客户端] 已获取用户名:', window.__wx_username);
    }
  });
}

if (window.WXE && window.WXE.onAPILoaded) {
  window.WXE.onAPILoaded(function () {
    if (window.__wx_api_client) {
      window.__wx_api_client.sendClientState();
    }
  });
}

console.log('[api_client.js] API 客户端模块加载完成');
