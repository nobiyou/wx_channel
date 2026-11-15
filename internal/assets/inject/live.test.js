const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadLive() {
  const requests = [];
  const elements = [];

  function createElement() {
    const element = {
      id: '',
      textContent: '',
      title: '',
      disabled: false,
      style: {},
      children: [],
      parentNode: null,
      offsetParent: {},
      listeners: {},
      setAttribute(name, value) {
        this[name] = String(value);
      },
      addEventListener(name, handler) {
        this.listeners[name] = handler;
      },
      click(event = {}) {
        return this.listeners.click(event);
      },
      appendChild(child) {
        child.parentNode = this;
        this.children.push(child);
      },
      insertBefore(child, before) {
        child.parentNode = this;
        const index = this.children.indexOf(before);
        this.children.splice(index < 0 ? this.children.length : index, 0, child);
      },
    };
    Object.defineProperty(element, 'nextSibling', {
      get() {
        if (!this.parentNode) return null;
        const index = this.parentNode.children.indexOf(this);
        return index >= 0 ? this.parentNode.children[index + 1] || null : null;
      },
    });
    elements.push(element);
    return element;
  }

  const container = createElement();
  const body = createElement();
  let currentContainer = container;
  let mutationCallback = null;
  const nativeFetch = async (url, options) => {
    requests.push({ url, options });
    if (url.endsWith('/download_live')) {
      return { ok: true, status: 200, async json() { return { success: true, path: 'downloads/test/live.flv' }; } };
    }
    return { ok: true, status: 200, async json() { return {}; } };
  };

  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    document: {
      readyState: 'complete',
      body,
      createElement,
      querySelector(selector) {
        return selector.indexOf('.host__info') >= 0 ? currentContainer : null;
      },
    },
    navigator: { userAgent: 'live-test' },
    location: { href: 'https://channels.weixin.qq.com/web/pages/live', origin: 'https://channels.weixin.qq.com' },
    fetch: nativeFetch,
    atob,
    TextDecoder,
    Uint8Array,
    dispatchEvent() {},
    setInterval() { return 1; },
    MutationObserver: class {
      constructor(callback) { mutationCallback = callback; }
      observe() {}
    },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init.detail; } },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'live.js'), 'utf8'), sandbox);
  return {
    sandbox,
    container,
    elements,
    requests,
    replaceContainer() { currentContainer = createElement(); return currentContainer; },
    remount() { if (mutationCallback) mutationCallback(); },
  };
}

test('live download keeps the original fetch when page bundles replace global fetch', async () => {
  const { sandbox, container, requests } = loadLive();
  sandbox.__wx_channels_capture_live_data__('finderGetLiveInfo', { liveCdnUrl: 'https://stream.example/live.flv' });

  const button = container.children.find(element => element.id === '__wx_channels_live_download_button__');
  assert.ok(button, 'live download button should be mounted');
  assert.equal(button.textContent, '下载直播');
  assert.match(button.style.cssText, /position:relative/);
  assert.match(button.style.cssText, /pointer-events:auto/);
  assert.equal(typeof button.listeners.pointerdown, 'function');
  assert.equal(typeof button.listeners.mousedown, 'function');

  sandbox.fetch = async () => { throw new Error('page fetch wrapper was used'); };
  const clickEvent = {
    preventDefault() { this.defaultPrevented = true; },
    stopPropagation() { this.propagationStopped = true; },
    stopImmediatePropagation() { this.immediatePropagationStopped = true; },
  };
  await button.click(clickEvent);
  assert.equal(clickEvent.defaultPrevented, true);
  assert.equal(clickEvent.propagationStopped, true);
  assert.equal(clickEvent.immediatePropagationStopped, true);

  const request = requests.find(item => item.url.endsWith('/download_live'));
  assert.ok(request, 'download_live request should reach the preserved native fetch');
  const payload = JSON.parse(request.options.body);
  assert.equal(payload.streamUrl, 'https://stream.example/live.flv');
  assert.equal(payload.title, '直播');
  assert.equal(payload.author, '未知作者');
  assert.equal(payload.sourceUrl, 'https://channels.weixin.qq.com/web/pages/live');
  assert.equal(payload.userAgent, 'live-test');
  assert.deepEqual(payload.headers, {
    Referer: 'https://channels.weixin.qq.com/web/pages/live',
    Origin: 'https://channels.weixin.qq.com',
    'User-Agent': 'live-test',
  });
  assert.match(button.parentNode.children.find(element => element.id === '__wx_channels_live_download_status__').textContent, /已启动/);
});

test('live SDK parameters supply the declared CDN quality', async () => {
  const { sandbox, container, requests } = loadLive();
  const sdkParams = Buffer.from(JSON.stringify({
    channel_params: {
      cdn_quality_svrcfg: 2,
      cdn_trans_info: [
        { quality_tag: 1, url: 'https://stream.example/low.flv' },
        { quality_tag: 2, url: 'https://stream.example/current.flv' },
      ],
    },
  })).toString('base64');
  sandbox.__wx_channels_capture_live_data__('joinLive', { data: { liveSdkInfo: { sdkParams } } });
  const button = container.children.find(element => element.id === '__wx_channels_live_download_button__');
  assert.equal(button.textContent, '下载直播');
  assert.equal(button.disabled, false);
  await button.click();
  const request = requests.find(item => item.url.endsWith('/download_live'));
  assert.equal(JSON.parse(request.options.body).streamUrl, 'https://stream.example/current.flv');
});

test('live capture reports one visible detail summary without exposing the stream URL', async () => {
  const { sandbox, container, requests } = loadLive();
  sandbox.__wx_channels_capture_live_data__('finderGetLiveInfo', {
    liveInfo: { liveId: 'live-42' },
    liveDescription: '春日直播',
    bizUserInfo: { bizNickname: '主播A' },
    onlineCount: 128,
    liveCdnUrl: 'https://stream.example/private-token.flv',
  });
  sandbox.__wx_channels_capture_live_data__('joinLive', {
    liveInfo: { liveId: 'live-42' },
    liveDescription: '春日直播',
    bizUserInfo: { bizNickname: '主播A' },
    liveCdnUrl: 'https://stream.example/private-token.flv',
  });

  const summaries = requests
    .filter((item) => item.url.endsWith('/tip'))
    .map((item) => JSON.parse(item.options.body))
    .filter((item) => item.level === 'info' && item.msg.includes('直播信息'));
  assert.equal(summaries.length, 1);
  assert.match(summaries[0].msg, /ID=live-42/);
  assert.match(summaries[0].msg, /标题=春日直播/);
  assert.match(summaries[0].msg, /主播=主播A/);
  assert.match(summaries[0].msg, /在线人数=128/);
  assert.match(summaries[0].msg, /状态=已捕获直播流/);
  assert.doesNotMatch(summaries[0].msg, /private-token/);
  const button = container.children.find((element) => element.id === '__wx_channels_live_download_button__');
  await button.click();
  const request = requests.find((item) => item.url.endsWith('/download_live'));
  assert.equal(JSON.parse(request.options.body).author, '主播A');
});

test('switching to the next live refreshes the stream, detail summary, and button', async () => {
  const { sandbox, container, requests, replaceContainer, remount } = loadLive();
  sandbox.__wx_channels_capture_live_data__('finderGetLiveInfo', {
    liveId: 'live-first',
    title: '第一场直播',
    contact: { nickname: '主播一' },
    liveCdnUrl: 'https://stream.example/first.flv',
  });
  const oldButton = container.children.find((element) => element.id === '__wx_channels_live_download_button__');
  assert.ok(oldButton);
  await oldButton.click();
  assert.equal(sandbox.__wx_channels_live_runtime__.downloading, true);
  assert.match(sandbox.__wx_channels_live_runtime__.statusText, /已启动/);

  const nextContainer = replaceContainer();
  oldButton.isConnected = false;
  remount();
  sandbox.__wx_channels_capture_live_data__('joinLive', {
    title: '第二场直播',
    contact: { nickname: '主播二' },
    liveCdnUrl: 'https://stream.example/second.flv',
  });

  const nextButton = nextContainer.children.find((element) => element.id === '__wx_channels_live_download_button__');
  assert.ok(nextButton, 'next live should remount the download button');
  assert.equal(nextButton.textContent, '下载直播');
  assert.equal(sandbox.__wx_channels_live_runtime__.streamUrl, 'https://stream.example/second.flv');
  assert.equal(sandbox.__wx_channels_live_runtime__.downloading, false);
  const summaries = requests
    .filter((item) => item.url.endsWith('/tip'))
    .map((item) => JSON.parse(item.options.body))
    .filter((item) => item.level === 'info' && item.msg.includes('直播信息'));
  assert.equal(summaries.length, 2);
  assert.match(summaries[1].msg, /标题=第二场直播/);
  assert.match(summaries[1].msg, /主播=主播二/);

  sandbox.__wx_channels_capture_live_data__('finderGetLiveInfo', {
    liveId: 'live-first',
    title: '第一场直播',
    contact: { nickname: '主播一' },
    liveCdnUrl: 'https://stream.example/first.flv',
  });
  assert.equal(sandbox.__wx_channels_live_runtime__.downloading, true);
  assert.match(sandbox.__wx_channels_live_runtime__.statusText, /已启动/);
});

test('multi-view SDK parameters skip streams without a usable HTTP URL', () => {
  const { sandbox } = loadLive();
  const sdkParams = Buffer.from(JSON.stringify({
    channel_params_desc: [
      { stream_id: 'pending', channel_params: { cdn_trans_info: [{ url: 'blob:temporary' }] } },
      { stream_id: 'main', view_desc: '主画面', channel_params: { cdn_trans_info: [{ url: 'https://stream.example/main.flv' }] } },
    ],
  })).toString('base64');
  sandbox.__wx_channels_capture_live_data__('joinLive', { liveSdkInfo: { sdkParams } });
  assert.equal(sandbox.__wx_channels_live_runtime__.streamUrl, 'https://stream.example/main.flv');
});

test('invalid SDK data remains waiting and a later valid update enables recording', () => {
  const { sandbox, container } = loadLive();
  const button = container.children.find(element => element.id === '__wx_channels_live_download_button__');
  for (const sdkParams of ['not base64!', Buffer.from('not JSON').toString('base64')]) {
    assert.doesNotThrow(() => sandbox.__wx_channels_capture_live_data__('joinLive', { liveSdkInfo: { sdkParams } }));
    assert.equal(button.textContent, '等待直播流');
    assert.equal(button.disabled, true);
  }
  sandbox.__wx_channels_capture_live_data__('joinLive', {
    payload: { channelParams: { cdn_trans_info: [{ url: 'https://stream.example/recovered.flv' }] } },
  });
  assert.equal(button.textContent, '下载直播');
  assert.equal(sandbox.__wx_channels_live_runtime__.streamUrl, 'https://stream.example/recovered.flv');
});
