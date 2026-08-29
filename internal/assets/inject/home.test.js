const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadHome() {
  const source = fs.readFileSync(path.resolve(__dirname, 'home.js'), 'utf8');
  const createdElements = [];
  const openedWindows = [];
  const logMessages = [];

  function createElement() {
    const element = {
      id: '',
      className: '',
      title: '',
      innerHTML: '',
      style: {},
      children: [],
      attributes: {},
      parentNode: null,
      appendChild(child) {
        child.parentNode = this;
        this.children.push(child);
      },
      remove() {
        if (this.parentNode && this.parentNode.children) {
          this.parentNode.children = this.parentNode.children.filter((child) => child !== this);
        }
        this.parentNode = null;
      },
      setAttribute(name, value) {
        this.attributes[name] = String(value);
      },
      getAttribute(name) {
        return this.attributes[name] || null;
      },
      querySelector(selector) {
        if (selector === '.wx-home-download-icon') {
          return this.children.find((child) => child.className === 'h-full w-full wx-home-download-icon') || null;
        }
        if (selector === '.wx-home-download-label') return null;
        return null;
      },
      getBoundingClientRect() {
        if (this.rect) return this.rect;
        const left = parseFloat(this.style.left) || 0;
        const top = parseFloat(this.style.top) || 0;
        return { left, top, right: left + 20, bottom: top + 20 };
      },
    };
    createdElements.push(element);
    return element;
  }

  const body = {
    children: [],
    appendChild(child) {
      child.parentNode = this;
      this.children.push(child);
    },
  };

  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    window: {},
    document: {
      body,
      documentElement: { clientHeight: 800, clientWidth: 1000 },
      createElement,
      getElementById(id) {
        return createdElements.find((element) => element.id === id) || null;
      },
      querySelector(selector) {
        if (selector.indexOf('.home-header .search-bar') === 0) {
          return { getBoundingClientRect() { return { left: 900, top: 16, right: 920, bottom: 36 }; } };
        }
        return null;
      },
      querySelectorAll(selector) {
        if (selector === '[role="tab"]') return [];
        if (selector.indexOf('[id^="flow-feed-"]') === 0) return [];
        return [];
      },
      addEventListener() {},
    },
    location: {
      pathname: '/web/pages/home',
      href: 'https://channels.weixin.qq.com/web/pages/home',
    },
    innerWidth: 1000,
    innerHeight: 800,
    URL,
    URLSearchParams,
    setTimeout() { return 1; },
    clearTimeout() {},
    setInterval() { return 1; },
    clearInterval() {},
    __wx_channels_store__: { profile: null },
    __wx_log(payload) { logMessages.push(payload); },
    WXE: {
      onPCFlowLoaded() {},
      onGotoNextFeed() {},
      onGotoPrevFeed() {},
      onFetchFeedProfile() {},
      onFeed() {},
    },
    WXU: {
      set_feed() {},
      set_cur_video() {},
    },
    open(url, target) {
      openedWindows.push({ url, target });
      return {};
    },
  };

  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'home.js' });
  return { sandbox, body, createdElements, openedWindows, logMessages };
}

test('adds a non-download management console icon beside the home download icon', async () => {
  const env = loadHome();
  await env.sandbox.__ensure_home_download_button(false);

  const downloadButton = env.sandbox.document.getElementById('wx-home-download-icon');
  const consoleButton = env.sandbox.document.getElementById('wx-home-console-icon');
  assert.ok(downloadButton);
  assert.ok(consoleButton);
  assert.equal(consoleButton.attributes['aria-label'], '管理控制台');
  assert.equal(consoleButton.title, '管理控制台');
  assert.equal(consoleButton.style.left, '844px');
  assert.equal(downloadButton.style.left, '872px');
  assert.match(consoleButton.innerHTML, /<rect x="3" y="4"/);
  assert.doesNotMatch(consoleButton.innerHTML, /M12 3v12/);

  consoleButton.onclick({ preventDefault() {}, stopPropagation() {} });
  assert.deepEqual(env.openedWindows, [{ url: 'http://127.0.0.1:2025/console', target: '_blank' }]);
  assert.equal(env.logMessages.length, 1);
  assert.equal(env.logMessages[0].msg, '已打开管理控制台');
});

test('does not duplicate the home console icon during reinjection', async () => {
  const env = loadHome();
  await env.sandbox.__ensure_home_download_button(false);
  await env.sandbox.__ensure_home_download_button(false);

  assert.equal(env.body.children.filter((element) => element.id === 'wx-home-download-icon').length, 1);
  assert.equal(env.body.children.filter((element) => element.id === 'wx-home-console-icon').length, 1);
});
