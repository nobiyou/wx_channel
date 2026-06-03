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
  assertEqual(shareEnv.fetchCalls[0].url, '/finder-preview/api/feed/get_feed_info', 'share link should hit feed info endpoint');
  assertEqual(sharePayload.scene, 141, 'share link should use encrypted export scene');
  assertEqual(sharePayload.encrypted_objectid, 'export-id-123', 'share link should use dynamic export id');
  assertEqual(sharePayload.objectid, undefined, 'share link should not set plain objectid');
  assertEqual(sharePayload.objectNonceId, undefined, 'share link should not set plain nonce id');

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
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
