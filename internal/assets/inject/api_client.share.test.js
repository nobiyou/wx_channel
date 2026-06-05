const fs = require('fs');
const path = require('path');
const vm = require('vm');

function loadAPIClientModule(overrides = {}) {
  const file = path.resolve(__dirname, 'api_client.js');
  const source = fs.readFileSync(file, 'utf8');

  const fetchCalls = [];
  const finderCalls = [];

  const sandbox = {
    console: { log() {}, error() {}, warn() {} },
    window: {},
    document: {
      readyState: 'loading',
      hidden: false,
      addEventListener() {},
      dispatchEvent() {},
      createElement() {
        return {
          setAttribute() {},
          getAttribute() { return ''; },
          addEventListener() {},
          appendChild() {},
          style: {},
          click() {},
        };
      },
      head: { appendChild() {} },
      body: { appendChild() {} },
    },
    location: {
      href: 'https://channels.weixin.qq.com/web/pages/home',
      origin: 'https://channels.weixin.qq.com',
    },
    navigator: { userAgent: 'node-test' },
    localStorage: {
      getItem() { return null; },
      setItem() {},
    },
    fetch: async (url, options) => {
      fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              sceneInfo: {
                dynamicExportId: 'export-id-123',
              },
            },
          };
        },
      };
    },
    URL,
    URLSearchParams,
    Date,
    encodeURIComponent,
    decodeURIComponent,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    CustomEvent: function CustomEvent(type, init) {
      return { type, detail: init ? init.detail : undefined };
    },
    WebSocket: function WebSocket() {},
  };

  sandbox.window = sandbox;
  sandbox.window.WXU = {
    API: {
      async finderGetCommentDetail(payload) {
        finderCalls.push(payload);
        return { errCode: 0, data: { object: { id: 'feed-1' } } };
      },
      decodeBase64ToUint64String(value) {
        return `decoded:${value}`;
      },
    },
    API2: {},
  };
  sandbox.window.WXE = undefined;

  Object.assign(sandbox, overrides);
  if (overrides.window) {
    Object.assign(sandbox.window, overrides.window);
  }

  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: file });

  return {
    api: sandbox.window.__wx_api_client,
    fetchCalls,
    finderCalls,
  };
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}\nactual:   ${actual}\nexpected: ${expected}`);
  }
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

async function main() {
  const shareEnv = loadAPIClientModule();
  const sharePayload = await shareEnv.api.buildFeedProfilePayload({
    url: 'https://weixin.qq.com/sph/A1b2C3d4',
  });

  assertEqual(shareEnv.fetchCalls.length, 1, 'share link should request feed info once');
  assertEqual(shareEnv.fetchCalls[0].url, '/finder-preview/api/feed/get_feed_info', 'share link should hit feed info endpoint directly');
  assertEqual(sharePayload.scene, 141, 'share link should use encrypted export scene');
  assertEqual(sharePayload.encrypted_objectid, 'export-id-123', 'share link should use dynamic export id');
  assertEqual(sharePayload.objectid, undefined, 'share link should not set plain objectid');
  assertEqual(sharePayload.objectNonceId, undefined, 'share link should not set plain nonce id');

  const shareFallbackEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      shareFallbackEnv.fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              exportId: 'export-id-fallback',
            },
          };
        },
      };
    },
  });
  const shareFallbackPayload = await shareFallbackEnv.api.buildFeedProfilePayload({
    url: 'https://weixin.qq.com/sph/A1b2C3d4',
  });

  assertEqual(shareFallbackPayload.scene, 141, 'share link fallback should still use encrypted export scene');
  assertEqual(shareFallbackPayload.encrypted_objectid, 'export-id-fallback', 'share link should fall back to alternate export id fields');

  const shareObjectFallbackEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      shareObjectFallbackEnv.fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              object: {
                id: 'export-id-from-object',
              },
            },
          };
        },
      };
    },
  });
  const shareObjectFallbackPayload = await shareObjectFallbackEnv.api.buildFeedProfilePayload({
    url: 'https://weixin.qq.com/sph/A1b2C3d4',
  });

  assertEqual(shareObjectFallbackPayload.encrypted_objectid, 'export-id-from-object', 'share link should fall back to object.id when sceneInfo.dynamicExportId is missing');

  const shareRequestFallbackEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      shareRequestFallbackEnv.fetchCalls.push({ url, options });
      return {
        ok: false,
        status: 500,
        async json() {
          return {};
        },
      };
    },
  });
  const shareRequestFallbackPayload = await shareRequestFallbackEnv.api.buildFeedProfilePayload({
    url: 'https://weixin.qq.com/sph/AI7ZDceho',
  });

  assertEqual(shareRequestFallbackPayload.scene, 141, 'share request fallback should still use encrypted export scene');
  assertEqual(shareRequestFallbackPayload.encrypted_objectid, 'AI7ZDceho', 'share request fallback should reuse shortUri as encrypted object id');

  const normalEnv = loadAPIClientModule();
  const normalPayload = await normalEnv.api.buildFeedProfilePayload({
    url: 'https://channels.weixin.qq.com/web/pages/feed?oid=Zm9v&nid=YmFy',
  });

  assertEqual(normalEnv.fetchCalls.length, 0, 'normal feed url should not request feed info');
  assertEqual(normalPayload.scene, 146, 'normal feed url should keep normal scene');
  assertEqual(normalPayload.objectid, 'decoded:Zm9v', 'normal feed url should decode oid');
  assertEqual(normalPayload.objectNonceId, 'decoded:YmFy', 'normal feed url should decode nid');
  assertEqual(normalPayload.encrypted_objectid, '', 'normal feed url should not set encrypted object id');

  const fetchEnv = loadAPIClientModule();
  const result = await fetchEnv.api.fetchFeedProfile({
    url: 'https://channels.weixin.qq.com/finder-preview/pages/sph?id=A1b2C3d4',
  });

  assertEqual(fetchEnv.finderCalls.length, 1, 'fetchFeedProfile should call finderGetCommentDetail once');
  assertEqual(result.payload.scene, 141, 'fetchFeedProfile should preserve share payload scene');
  assert(result.response && result.response.data && result.response.data.object, 'fetchFeedProfile should return response data');

  const directMediaEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      directMediaEnv.fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              feedInfo: {
                description: '短链直出视频',
                coverUrl: 'https://cdn.example.com/direct-cover.jpg',
                videoUrl: 'https://finder.video.qq.com/251/20302/stodownload?foo=1&encfilekey=abc123&token=tok456&bar=2',
              },
              authorInfo: {
                nickname: '直出作者',
              },
            },
          };
        },
      };
    },
  });
  const directMediaResult = await directMediaEnv.api.resolveSharedFeedProfile({
    url: 'https://weixin.qq.com/sph/A1b2C3d4',
  });

  assertEqual(directMediaEnv.finderCalls.length, 0, 'direct media share link should not call finderGetCommentDetail');
  assertEqual(directMediaResult.payload.source, 'short_uri_feed_info', 'direct media share link should be served from short-uri feed info');
  assertEqual(directMediaResult.response.data.object.id, 'shared_feed', 'direct media share link should fall back to shared_feed id');
  assertEqual(directMediaResult.response.data.object.objectDesc.description, '短链直出视频', 'direct media share link should preserve description');
  assertEqual(
    directMediaResult.response.data.object.objectDesc.media[0].url,
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456',
    'direct media share link should normalize media url to encfilekey+token'
  );

  const directObjectMediaEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      directObjectMediaEnv.fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              object: {
                objectDesc: {
                  description: '对象媒体视频',
                  media: [{
                    url: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=xyz789&token=tok000&junk=1',
                    thumbUrl: 'https://cdn.example.com/object-cover.jpg',
                  }],
                },
              },
            },
          };
        },
      };
    },
  });
  const directObjectMediaResult = await directObjectMediaEnv.api.resolveSharedFeedProfile({
    url: 'https://weixin.qq.com/sph/A1b2C3d4',
  });

  assertEqual(directObjectMediaEnv.finderCalls.length, 0, 'object media share link should not call finderGetCommentDetail');
  assertEqual(directObjectMediaResult.response.data.sceneInfo.dynamicExportId, 'shared_feed', 'object media share link should synthesize shared_feed export id');
  assertEqual(
    directObjectMediaResult.response.data.object.objectDesc.media[0].url,
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=xyz789&token=tok000',
    'object media share link should normalize object media url'
  );

  const sharedProfileEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      sharedProfileEnv.fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              sceneInfo: {
                dynamicExportId: 'profile-export-123',
              },
              feedInfo: {
                description: 'shared_profile 可用',
                videoUrl: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=profile123&token=profile456&foo=1',
              },
              authorInfo: {
                nickname: '老接口作者',
              },
            },
          };
        },
      };
    },
  });
  await sharedProfileEnv.api.handleAPICall({
    id: 'case-shared-profile',
    key: 'key:channels:shared_feed_profile',
    body: {
      url: 'https://weixin.qq.com/sph/A1b2C3d4',
    },
  });

  assertEqual(sharedProfileEnv.finderCalls.length, 1, 'shared_feed_profile should keep historical finderGetCommentDetail flow');

  const sharedResolveEnv = loadAPIClientModule({
    fetch: async (url, options) => {
      sharedResolveEnv.fetchCalls.push({ url, options });
      return {
        ok: true,
        async json() {
          return {
            errCode: 0,
            data: {
              feedInfo: {
                description: 'resolve 直出视频',
                videoUrl: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=resolve123&token=resolve456&foo=1',
              },
              authorInfo: {
                nickname: 'resolve 作者',
              },
            },
          };
        },
      };
    },
  });
  await sharedResolveEnv.api.handleAPICall({
    id: 'case-shared-resolve',
    key: 'key:channels:shared_feed_resolve',
    body: {
      url: 'https://weixin.qq.com/sph/A1b2C3d4',
    },
  });

  assertEqual(sharedResolveEnv.finderCalls.length, 0, 'shared_feed_resolve should use direct share resolver flow');
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
