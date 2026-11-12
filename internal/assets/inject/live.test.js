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
        return selector.indexOf('.host__info') >= 0 ? container : null;
      },
    },
    navigator: { userAgent: 'live-test' },
    location: { href: 'https://channels.weixin.qq.com/web/pages/live', origin: 'https://channels.weixin.qq.com' },
    fetch: nativeFetch,
    dispatchEvent() {},
    setInterval() { return 1; },
    MutationObserver: class { observe() {} },
    CustomEvent: class { constructor(type, init) { this.type = type; this.detail = init.detail; } },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'live.js'), 'utf8'), sandbox);
  return { sandbox, container, elements, requests };
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
