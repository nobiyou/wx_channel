const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadHome() {
  const source = fs.readFileSync(path.resolve(__dirname, 'home.js'), 'utf8');
  const createdElements = [];
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
  };

  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'home.js' });
  return { sandbox, body, createdElements, logMessages };
}

test('injects only the home download icon', async () => {
  const env = loadHome();
  await env.sandbox.__ensure_home_download_button(false);

  const downloadButton = env.sandbox.document.getElementById('wx-home-download-icon');
  const consoleButton = env.sandbox.document.getElementById('wx-home-console-icon');
  assert.ok(downloadButton);
  assert.equal(consoleButton, null);
  assert.equal(downloadButton.style.left, '872px');
});

test('does not duplicate the home download icon during reinjection', async () => {
  const env = loadHome();
  await env.sandbox.__ensure_home_download_button(false);
  await env.sandbox.__ensure_home_download_button(false);

  assert.equal(env.body.children.filter((element) => element.id === 'wx-home-download-icon').length, 1);
  assert.equal(env.body.children.filter((element) => element.id === 'wx-home-console-icon').length, 0);
});
