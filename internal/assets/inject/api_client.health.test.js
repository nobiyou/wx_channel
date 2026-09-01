const fs = require('fs');
const path = require('path');
const vm = require('vm');

function loadClient() {
  const source = fs.readFileSync(path.resolve(__dirname, 'api_client.js'), 'utf8');
  const messages = [];
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    window: {},
    document: {
      readyState: 'loading',
      hidden: false,
      addEventListener() {},
      dispatchEvent() {},
    },
    location: {
      href: 'https://channels.weixin.qq.com/web/pages/feed',
      origin: 'https://channels.weixin.qq.com',
      reload() {},
      assign() {},
    },
    navigator: { userAgent: 'node-test' },
    localStorage: { getItem() { return null; }, setItem() {} },
    URL,
    URLSearchParams,
    Date,
    Promise,
    JSON,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    CustomEvent: function CustomEvent(type, init) {
      return { type, detail: init ? init.detail : undefined };
    },
    WebSocket: function WebSocket() {},
    fetch() { return Promise.resolve({}); },
  };
  sandbox.WebSocket.OPEN = 1;
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'api_client.js' });
  return { api: sandbox.window.__wx_api_client, sandbox, messages };
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

async function main() {
  const successEnv = loadClient();
  const successCalls = [];
  successEnv.sandbox.window.__wx_channels_store__ = {
    profile: { id: 'feed-1', nonce_id: 'nonce-1' },
  };
  successEnv.sandbox.window.WXU = {
    API: {
      finderGetCommentDetail(payload) {
        successCalls.push(payload);
        return Promise.resolve({ errCode: 0 });
      },
    },
  };
  successEnv.api.connected = true;
  successEnv.api.ws = {
    readyState: 1,
    send(raw) { successEnv.messages.push(JSON.parse(raw)); },
  };

  const success = await successEnv.api.runFunctionalProbe('test-success');
  assert(success === true, 'successful functional probe must resolve true');
  assert(successCalls.length === 1, 'successful probe must call the detail API once');
  assert(successEnv.api.apiProbeStatus === 'ok', 'successful probe must set ok status');
  assert(successEnv.api.apiFunctional === true, 'successful probe must set apiFunctional');
  assert(successEnv.messages.some((message) => message.type === 'client_state'), 'probe result must send client state');

  const failureEnv = loadClient();
  failureEnv.sandbox.window.__wx_channels_store__ = {
    profile: { id: 'feed-2', nonce_id: 'nonce-2' },
  };
  failureEnv.sandbox.window.WXU = {
    API: {
      finderGetCommentDetail() {
        return new Promise(() => {});
      },
    },
  };
  failureEnv.api.connected = true;
  failureEnv.api.functionalProbeTimeout = 5;
  failureEnv.api.ws = {
    readyState: 1,
    send(raw) { failureEnv.messages.push(JSON.parse(raw)); },
  };

  const failure = await failureEnv.api.runFunctionalProbe('test-timeout');
  assert(failure === false, 'timed out functional probe must resolve false');
  assert(failureEnv.api.apiProbeStatus === 'failed', 'timed out probe must set failed status');
  assert(failureEnv.api.apiFunctional === false, 'timed out probe must clear apiFunctional');
  assert(failureEnv.api.apiProbeError.includes('超时'), 'timed out probe must preserve error context');

  console.log('api_client health tests passed');
}

main().catch((err) => {
  console.error(err.stack || err);
  process.exitCode = 1;
});
