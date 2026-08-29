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

function loadReconnectClient() {
  const source = fs.readFileSync(path.resolve(__dirname, 'api_client.js'), 'utf8');
  const sockets = [];

  function FakeWebSocket(url) {
    this.url = url;
    this.readyState = FakeWebSocket.CONNECTING;
    this.send = function () {};
    this.close = function () {
      this.readyState = FakeWebSocket.CLOSED;
    };
    sockets.push(this);
  }
  FakeWebSocket.CONNECTING = 0;
  FakeWebSocket.OPEN = 1;
  FakeWebSocket.CLOSING = 2;
  FakeWebSocket.CLOSED = 3;

  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    window: {},
    document: {
      readyState: 'complete',
      hidden: false,
      addEventListener() {},
      dispatchEvent() {},
    },
    location: {
      href: 'https://channels.weixin.qq.com/web/pages/home',
      origin: 'https://channels.weixin.qq.com',
      reload() {},
      assign() {},
    },
    navigator: { userAgent: 'node-test' },
    localStorage: { getItem() { return null; }, setItem() {} },
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
    WebSocket: FakeWebSocket,
    fetch() { return Promise.resolve({}); },
  };
  sandbox.window = sandbox;
  sandbox.window.addEventListener = function () {};
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'api_client.js' });
  return { api: sandbox.window.__wx_api_client, sockets };
}

function clearReconnectTimer(api) {
  if (api.reconnectTimer) {
    clearTimeout(api.reconnectTimer);
    api.reconnectTimer = null;
  }
  api.stopHeartbeat();
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

  const reconnectEnv = loadReconnectClient();
  const reconnectAPI = reconnectEnv.api;
  const socket = reconnectEnv.sockets[0];
  assert(socket, 'client init must create a WebSocket');
  socket.readyState = 1;
  socket.onopen();
  assert(reconnectAPI.connected === true, 'open socket must mark client connected');
  reconnectAPI.reconnectDelay = 60000;
  socket.readyState = 3;
  socket.onclose({ code: 1000, reason: 'server idle close' });
  assert(reconnectAPI.connected === false, 'closed socket must mark client disconnected');
  assert(reconnectAPI.reconnectTimer !== null, 'closed socket must schedule reconnect');
  clearReconnectTimer(reconnectAPI);
}

main();
console.log('api_client lifecycle tests passed');
