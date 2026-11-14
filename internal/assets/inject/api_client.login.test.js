const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function load(href = 'https://channels.weixin.qq.com/platform/login') {
  const intervals = new Map();
  const messages = [];
  let timer = 0;
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    location: new URL(href), navigator: { userAgent: 'test' }, URL, URLSearchParams, Date,
    document: {
      readyState: 'loading', hidden: false, body: { innerText: '视频号ID：viewed-creator' },
      addEventListener() {}, dispatchEvent() {},
      querySelectorAll() { throw new Error('login state must not scan images, canvases or OAuth frames'); },
    },
    addEventListener() {}, removeEventListener() {},
    setInterval(fn) { const id = ++timer; intervals.set(id, fn); return id; },
    clearInterval(id) { intervals.delete(id); },
    setTimeout() { return ++timer; }, clearTimeout() {},
    WebSocket: Object.assign(function () {}, { OPEN: 1 }),
  };
  sandbox.window = sandbox;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'api_client.js'), 'utf8'), sandbox);
  const api = sandbox.__wx_api_client;
  api.connected = true;
  api.ws = { readyState: 1, send(raw) { messages.push(JSON.parse(raw)); } };
  return { api, sandbox, intervals, messages, tick() { for (const fn of [...intervals.values()]) fn(); } };
}

test('official login remains in the original browser without scraping or forwarding QR data', () => {
  const { api } = load();
  const state = api.collectClientState();
  assert.equal(state.finderUsername, '');
  assert.equal(state.href, 'https://channels.weixin.qq.com/platform/login');
  assert.ok(!('loginQRCode' in state));
  assert.ok(!('loginQRCodeURL' in state));
});

test('a viewed creator cannot authenticate the current account', () => {
  const { api } = load('https://channels.weixin.qq.com/web/pages/profile');
  assert.equal(api.collectClientState().finderUsername, '');
});

test('returning to login clears the previous username', () => {
  const env = load('https://channels.weixin.qq.com/web/pages/home');
  env.sandbox.__wx_username = 'own-account';
  assert.equal(env.api.collectClientState().finderUsername, 'own-account');
  env.sandbox.location = new URL('https://channels.weixin.qq.com/platform/login');
  assert.equal(env.api.collectClientState().finderUsername, '');
  assert.equal(env.sandbox.__wx_username, '');
});

test('monitor synchronizes login, account changes and logout without duplicate sends', () => {
  const env = load();
  env.api.startLoginStateMonitor();
  env.api.sendClientState();
  const initial = env.messages.length;
  env.tick();
  assert.equal(env.messages.length, initial);
  env.sandbox.location = new URL('https://channels.weixin.qq.com/web/pages/home');
  env.sandbox.__wx_username = 'own-account';
  env.tick();
  assert.equal(env.messages.at(-1).data.finderUsername, 'own-account');
  env.sandbox.__wx_username = 'new-account';
  env.tick();
  assert.equal(env.messages.at(-1).data.finderUsername, 'new-account');
  env.sandbox.location = new URL('https://channels.weixin.qq.com/platform/login');
  env.tick();
  assert.equal(env.messages.at(-1).data.finderUsername, '');
  env.api.stopHeartbeat();
  assert.equal(env.intervals.size, 0);
});

test('navigation rejects credentials, nonstandard ports and unofficial destinations', () => {
  const { api } = load();
  assert.equal(api.isAllowedChannelNavigation('https://channels.weixin.qq.com/platform/login'), true);
  for (const url of [
    'http://channels.weixin.qq.com/platform/login',
    'https://channels.weixin.qq.com.evil.example/platform/login',
    'https://user:pass@channels.weixin.qq.com/platform/login',
    'https://channels.weixin.qq.com:444/platform/login',
    'https://open.weixin.qq.com/connect/qrconnect?appid=test',
  ]) assert.equal(api.isAllowedChannelNavigation(url), false, url);
});
