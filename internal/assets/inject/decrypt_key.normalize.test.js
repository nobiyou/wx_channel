const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function createElement() {
  const element = {
    style: {},
    children: [],
    appendChild(child) {
      this.children.push(child);
      child.parentNode = this;
    },
    remove() {},
    setAttribute() {},
    getAttribute() { return ''; },
    addEventListener() {},
    click() {},
    select() {},
    textContent: '',
    innerText: '',
  };

  let innerHTML = '';
  Object.defineProperty(element, 'innerHTML', {
    get() { return innerHTML; },
    set(value) {
      innerHTML = String(value);
      element.textContent = innerHTML.replace(/<[^>]*>/g, '');
      element.innerText = element.textContent;
    },
  });
  return element;
}

function loadScripts(names, fetchImpl) {
  const fetchCalls = [];
  const elements = new Map();
  const sandbox = {
    console: { log() {}, error() {}, warn() {} },
    window: {},
    document: {
      readyState: 'complete',
      body: { appendChild() {}, removeChild() {} },
      head: { appendChild() {} },
      createElement,
      querySelector() { return null; },
      getElementById(id) { return elements.get(id) || null; },
      addEventListener() {},
      removeEventListener() {},
      execCommand() {},
    },
    location: {
      href: 'https://channels.weixin.qq.com/web/pages/home',
      origin: 'https://channels.weixin.qq.com',
      pathname: '/web/pages/home',
    },
    navigator: { userAgent: 'node-test' },
    WXE: { onAPILoaded() {} },
    fetch: async (url, options) => {
      fetchCalls.push({ url, options });
      return fetchImpl ? fetchImpl(url, options) : {
        ok: true,
        status: 200,
        async json() { return { success: true, data: {} }; },
      };
    },
    URL,
    URLSearchParams,
    Blob,
    BigInt,
    Date,
    Number,
    JSON,
    Promise,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    encodeURIComponent,
    decodeURIComponent,
    formatFileSize(value) { return String(value); },
    alert() {},
    confirm() { return true; },
    MutationObserver: function MutationObserver() {},
  };
  sandbox.window = sandbox;

  vm.createContext(sandbox);
  for (const name of names) {
    const source = fs.readFileSync(path.resolve(__dirname, name), 'utf8');
    vm.runInContext(source, sandbox, { filename: name });
  }
  return { sandbox, fetchCalls, elements };
}

function jsonResponse(value) {
  return {
    ok: true,
    status: 200,
    async json() { return value; },
  };
}

test('normalizes every supported decrypt-key representation', () => {
  const { sandbox } = loadScripts(['utils.js']);
  const normalize = sandbox.__wx_channels_normalize_decrypt_key__;
  const select = sandbox.__wx_channels_select_decrypt_key__;

  assert.equal(normalize('  2136343393  '), '2136343393');
  assert.equal(normalize(2136343393), '2136343393');
  assert.equal(normalize(0), '0');
  assert.equal(normalize(BigInt('9007199254740993')), '9007199254740993');
  assert.equal(normalize(1.5), '');
  assert.equal(normalize(Number.NaN), '');
  assert.equal(normalize(Number.POSITIVE_INFINITY), '');
  assert.equal(normalize(null), '');
  assert.equal(normalize(undefined), '');
  assert.equal(normalize({ value: 2136343393 }), '');
  assert.equal(normalize(true), '');

  assert.equal(select('  ', 0, 2136343393), '0');
  assert.equal(select('', null, 2136343393), '2136343393');
  assert.equal(select({}, 1.5, undefined), '');
});

test('format_feed canonicalizes numeric media keys before persistence', () => {
  const { sandbox } = loadScripts(['utils.js']);
  const profile = sandbox.WXU.format_feed({
    id: 'feed-numeric-key',
    objectNonceId: 'nonce-1',
    contact: { username: 'author-1', nickname: '作者', headUrl: '' },
    objectDesc: {
      mediaType: 4,
      description: 'numeric key',
      media: [{
        url: 'https://cdn.example.test/video',
        urlToken: '?token=1',
        decodeKey: 2136343393,
        coverUrl: '',
        thumbUrl: '',
        spec: [],
      }],
    },
  });

  assert.equal(profile.key, '2136343393');
  assert.equal(profile.decryptKey, '2136343393');
  assert.equal(typeof profile.key, 'string');
  assert.equal(typeof profile.decryptKey, 'string');
});

test('single backend request always sends a canonical string key', async () => {
  const env = loadScripts(['utils.js', 'download.js'], () => jsonResponse({
    success: true,
    started: false,
  }));
  env.sandbox.__wx_log = function () {};

  await env.sandbox.__wx_channels_download_via_backend__({
    id: 'single-numeric-key',
    title: 'single',
    nickname: '作者',
    url: 'https://cdn.example.test/video?token=fresh',
    key: 2136343393,
    decryptKey: '',
  }, 'single.mp4', {
    mode: 'specific',
    url: 'https://cdn.example.test/video?token=fresh&X-snsvideoflag=xWT111',
    resolution: '720x1280',
    width: 720,
    height: 1280,
    fileFormat: 'xWT111',
  });

  const request = env.fetchCalls.find((call) => call.url === '/__wx_channels_api/download_video');
  assert.ok(request, 'single download should call the backend endpoint');
  const body = JSON.parse(request.options.body);
  assert.equal(body.key, '2136343393');
  assert.equal(typeof body.key, 'string');
  assert.equal(body.videoUrl, 'https://cdn.example.test/video?token=fresh&X-snsvideoflag=xWT111');
});

test('batch request always sends a canonical string key', async () => {
  const env = loadScripts(['utils.js', 'download.js', 'batch_download.js'], (url) => {
    if (url === '/__wx_channels_api/batch_start') {
      return jsonResponse({ success: true, data: { concurrency: 1 } });
    }
    if (url === '/__wx_channels_api/batch_progress') {
      return jsonResponse({ success: true, data: { total: 1, done: 1, failed: 0, running: 0 } });
    }
    return jsonResponse({ success: true });
  });
  env.sandbox.__wx_log = function () {};

  const manager = env.sandbox.__wx_batch_download_manager__;
  manager.videos = [{
    id: 'batch-numeric-key',
    type: 'media',
    canDownload: true,
    url: 'https://cdn.example.test/video?token=fresh',
    key: 2136343393,
    decryptKey: '',
    title: 'batch',
  }];
  manager.selectedItems = { 'batch-numeric-key': true };

  await env.sandbox.__batch_download_selected__();

  const request = env.fetchCalls.find((call) => call.url === '/__wx_channels_api/batch_start');
  assert.ok(request, 'batch download should call the backend endpoint');
  const body = JSON.parse(request.options.body);
  assert.equal(body.videos.length, 1);
  assert.equal(body.videos[0].key, '2136343393');
  assert.equal(typeof body.videos[0].key, 'string');
});
