const fs = require('fs');
const path = require('path');
const vm = require('vm');

function loadClient() {
  const source = fs.readFileSync(path.resolve(__dirname, 'api_client.js'), 'utf8');
  const calls = { reload: 0, assigned: [], refresh: [], locked: false };
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
      href: 'https://channels.weixin.qq.com/web/pages/home',
      origin: 'https://channels.weixin.qq.com',
      reload() { calls.reload += 1; },
      assign(value) { calls.assigned.push(value); },
    },
    navigator: { userAgent: 'node-test' },
    URL,
    URLSearchParams,
    Date,
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
  sandbox.window.__wx_keep_alive = {
    isRefreshLocked() { return calls.locked; },
    performRefresh(reason) { calls.refresh.push(reason); },
  };
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'api_client.js' });
  return { api: sandbox.window.__wx_api_client, calls };
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function main() {
  const env = loadClient();
  env.api.handleCommand({ action: 'channel_reload', payload: { reason: 'test-reload' } });
  assert(env.calls.refresh.length === 1 && env.calls.refresh[0] === 'test-reload', 'valid reload must call keep-alive refresh');

  env.api.handleCommand({
    action: 'channel_navigate',
    payload: { url: 'https://channels.weixin.qq.com/web/pages/feed?source=test' },
  });
  assert(env.calls.assigned.length === 1, 'valid navigation must be assigned');

  env.api.handleCommand({ action: 'channel_navigate', payload: { url: 'https://example.com/web/pages/home' } });
  env.api.handleCommand({ action: 'channel_navigate', payload: { url: 'https://channels.weixin.qq.com/other' } });
  assert(env.calls.assigned.length === 1, 'invalid navigation must be rejected');

  env.calls.locked = true;
  env.api.handleCommand({ action: 'channel_reload', payload: { reason: 'locked' } });
  env.api.handleCommand({ action: 'channel_navigate', payload: { url: 'https://channels.weixin.qq.com/web/pages/home' } });
  assert(env.calls.refresh.length === 1 && env.calls.assigned.length === 1, 'refresh lock must block lifecycle commands');

  env.api.connected = true;
  env.api.ws = { send() {}, close() {} };
  env.api.startHeartbeat();
  env.api.sendHeartbeat();
  assert(env.api.heartbeatPending === true, 'heartbeat must wait for a pong acknowledgement');
  env.api.handleMessage({ type: 'pong' });
  assert(env.api.heartbeatPending === false && env.api.missedHeartbeats === 0, 'pong must acknowledge heartbeat');
  env.api.stopHeartbeat();
}

main();
console.log('api_client lifecycle tests passed');
