const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadOfficialAccount(pathname, options) {
  options = options || {};
  const source = fs.readFileSync(path.resolve(__dirname, 'officialaccount.js'), 'utf8');
  const intervals = [];
  const fetchCalls = [];
  const createdElements = [];
  const execCommandCalls = [];
  const documentHandlers = {};
  const openedWindows = [];

  function createElement() {
    const element = {
      id: '',
      className: '',
      textContent: '',
      style: {},
      value: '',
      innerHTML: '',
      href: '',
      target: '',
      rel: '',
      parentNode: null,
      children: [],
      attributes: {},
      eventHandlers: {},
      setAttribute(name, value) { this.attributes[name] = value; },
      appendChild(child) { child.parentNode = this; this.children.push(child); },
      insertBefore(child, reference) {
        child.parentNode = this;
        const index = this.children.indexOf(reference);
        if (index < 0) {
          this.children.push(child);
        } else {
          this.children.splice(index, 0, child);
        }
      },
      removeChild(child) {
        this.children = this.children.filter((item) => item !== child);
        child.parentNode = null;
      },
      addEventListener(name, handler) { this.eventHandlers[name] = handler; },
      querySelector(selector) {
        if (selector === '[data-status]') {
          return this.children.find((child) => child.attributes['data-status']) || null;
        }
        return null;
      },
      focus() {},
      select() { this.selected = true; },
      setSelectionRange() {},
      getBoundingClientRect() {
        return options.rect || { left: 600, right: 700, top: 720, bottom: 758 };
      },
    };
    Object.defineProperty(element, 'lastElementChild', {
      enumerable: true,
      get() { return this.children.length ? this.children[this.children.length - 1] : null; },
    });
    createdElements.push(element);
    return element;
  }

  let interactionBar = options.interactionBar || null;
  if (!interactionBar && options.withInteractionBar) {
    interactionBar = createElement();
    interactionBar.appendChild(createElement());
  }

  const body = {
    children: [],
    innerHTML: options.bodyHTML || '<main>page</main>',
    appendChild(child) { child.parentNode = this; this.children.push(child); },
    removeChild(child) {
      this.children = this.children.filter((item) => item !== child);
      child.parentNode = null;
    },
  };

  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    window: {},
    document: {
      readyState: 'complete',
      body,
      addEventListener(name, handler) { documentHandlers[name] = handler; },
      createElement,
      querySelector(selector) {
        if (selector === '#js_content') {
          return options.articleContent || null;
        }
        return options.elements && options.elements[selector] || null;
      },
      querySelectorAll(selector) {
        if (selector === '.interaction_bar') {
          return interactionBar ? [interactionBar] : [];
        }
        if (options.mediaElements) {
          return options.mediaElements;
        }
        return options.linkElements || [];
      },
      getElementById(id) {
        return createdElements.find((element) => element.id === id) || null;
      },
      execCommand(command) {
        execCommandCalls.push(command);
        return options.execCommandResult;
      },
    },
    location: {
      pathname,
      href: options.href || `https://mp.weixin.qq.com${pathname}?__biz=biz-1&key=key-1`,
      origin: 'https://mp.weixin.qq.com',
    },
    navigator: options.navigator || {},
    innerWidth: 1000,
    innerHeight: 800,
    open(url, target) {
      openedWindows.push({ url, target });
      return {};
    },
    fetch(url, requestOptions) {
      fetchCalls.push({ url, options: requestOptions });
      if (typeof options.fetchResponse === 'function') {
        return options.fetchResponse(url, requestOptions);
      }
      return Promise.resolve({ ok: true });
    },
    URL,
    URLSearchParams,
    setTimeout(fn) {
      fn();
      return 1;
    },
    clearTimeout() {},
    setInterval(fn, delay) {
      intervals.push({ fn, delay });
      return intervals.length;
    },
    clearInterval() {},
  };

  sandbox.window = sandbox;
  sandbox.window.__wx_channels_mp_config__ = {
    origin: 'http://127.0.0.1:2026',
    token: 'test-token',
  };
  Object.assign(sandbox.window, options.window || {});

  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'officialaccount.js' });
  return { intervals, fetchCalls, body, createdElements, execCommandCalls, openedWindows, documentHandlers, interactionBar, sandbox };
}

function findElementByText(element, text) {
  if (!element) {
    return null;
  }
  if (element.textContent === text || element.attributes && element.attributes['aria-label'] === text) {
    return element;
  }
  for (const child of element.children || []) {
    const found = findElementByText(child, text);
    if (found) {
      return found;
    }
  }
  return null;
}

function triggerSubmission(pathname) {
  const env = loadOfficialAccount(pathname);
  assert.equal(env.intervals.length, 1, `expected polling on ${pathname}`);
  env.intervals[0].fn();
  assert.equal(env.fetchCalls.length, 1, `expected metadata submission on ${pathname}`);
  return env.fetchCalls[0];
}

test('submits account metadata to the configured local API', () => {
  const call = triggerSubmission('/s/article-id');
  assert.equal(call.url, 'http://127.0.0.1:2026/api/mp/refresh?token=test-token');
  assert.equal(call.options.credentials, 'omit');
});

test('submits page metadata even when the page does not expose a key', () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-1',
    window: {
      nickname: '测试公众号',
      headimg: 'https://mmbiz.qpic.cn/avatar/1',
    },
  });
  assert.equal(env.intervals.length, 1);
  env.intervals[0].fn();
  assert.equal(env.fetchCalls.length, 1);
  assert.equal(env.fetchCalls[0].url, 'http://127.0.0.1:2026/api/mp/metadata?token=test-token');
  assert.equal(env.fetchCalls[0].options.credentials, 'omit');
  const body = JSON.parse(env.fetchCalls[0].options.body);
  assert.equal(body.biz, 'biz-1');
  assert.equal(body.nickname, '测试公众号');
  assert.equal(body.avatar_url, 'https://mmbiz.qpic.cn/avatar/1');
});

test('normalizes account fields from the upstream cgiDataNew shape', () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?mid=12&idx=2&sn=sn-cgi',
    window: {
      cgiDataNew: {
        bizuin: 'biz-cgi',
        user_name: 'gh-cgi',
        nick_name: '上游字段公众号',
        round_head_img: 'https://mmbiz.qpic.cn/avatar/cgi',
        user_uin: 'uin-cgi',
        key: 'key-cgi',
        pass_ticket: 'ticket-cgi',
        appmsg_token: 'token-cgi',
        mid: 12,
        idx: 2,
        sn: 'sn-cgi',
      },
    },
  });

  env.intervals[0].fn();

  assert.equal(env.fetchCalls.length, 1);
  const body = JSON.parse(env.fetchCalls[0].options.body);
  assert.equal(body.biz, 'biz-cgi');
  assert.equal(body.author_id, 'gh-cgi');
  assert.equal(body.uin, 'uin-cgi');
  assert.equal(body.refresh_uri, 'https://mp.weixin.qq.com/s?__biz=biz-cgi&mid=12&idx=2&sn=sn-cgi');
});

test('submits updated metadata when user_name arrives after the credential', () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-delayed&key=key-delayed',
    window: {
      cgiDataNew: { nick_name: '延迟公众号' },
    },
  });

  env.intervals[0].fn();
  assert.equal(env.fetchCalls.length, 1);
  assert.equal(JSON.parse(env.fetchCalls[0].options.body).author_id, '');

  env.sandbox.cgiDataNew.user_name = 'gh-delayed';
  env.intervals[0].fn();

  assert.equal(env.fetchCalls.length, 2);
  assert.equal(JSON.parse(env.fetchCalls[1].options.body).author_id, 'gh-delayed');
});

test('reads account fields from an official-account fetch response', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-network&key=key-network',
    window: { cgiDataNew: {} },
    fetchResponse(url) {
      if (url.indexOf('mp.weixin.qq.com') >= 0) {
        return Promise.resolve({
          ok: true,
          clone() {
            return {
              text() {
                return Promise.resolve(JSON.stringify({
                  data: {
                    bizuin: 'biz-network',
                    user_name: 'gh-network',
                    nick_name: '网络公众号',
                  },
                }));
              },
            };
          },
        });
      }
      return Promise.resolve({ ok: true });
    },
  });

  env.sandbox.fetch('https://mp.weixin.qq.com/mp/profile_ext?action=home');
  await new Promise((resolve) => setImmediate(resolve));
  env.intervals[0].fn();

  const submissions = env.fetchCalls.filter((call) => call.url.indexOf('/api/mp/') >= 0);
  assert.equal(submissions.length, 1);
  assert.equal(JSON.parse(submissions[0].options.body).author_id, 'gh-network');
});

test('reads account fields from a JSON XMLHttpRequest response', () => {
  function MockXHR() {
    this.readyState = 0;
    this.responseType = 'json';
    this.response = {
      data: {
        bizuin: 'biz-xhr',
        user_name: 'gh-xhr',
        nick_name: 'XHR 公众号',
      },
    };
    this.listeners = {};
  }

  MockXHR.prototype.open = function (method, url) {
    this.method = method;
    this.url = url;
  };
  MockXHR.prototype.addEventListener = function (name, handler) {
    this.listeners[name] = handler;
  };
  MockXHR.prototype.send = function () {
    this.readyState = 4;
    if (this.listeners.load) {
      this.listeners.load();
    }
  };

  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-xhr&key=key-xhr',
    window: { cgiDataNew: {}, XMLHttpRequest: MockXHR },
  });
  const xhr = new env.sandbox.XMLHttpRequest();
  xhr.open('GET', 'https://mp.weixin.qq.com/mp/profile_ext?action=home');
  xhr.send();
  env.intervals[0].fn();

  const submissions = env.fetchCalls.filter((call) => call.url.indexOf('/api/mp/') >= 0);
  assert.equal(submissions.length, 1);
  assert.equal(JSON.parse(submissions[0].options.body).author_id, 'gh-xhr');
});

test('captures nested video metadata from an official-account network response', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-video-network&mid=9&idx=1',
    withInteractionBar: true,
    articleContent: {
      outerHTML: '<div id="js_content"><p>视频正文</p><iframe class="video_iframe" data-mpvid="video-network"></iframe></div>',
      innerHTML: '<p>视频正文</p>',
    },
    window: {
      cgiDataNew: {
        bizuin: 'biz-video-network',
        title: '网络视频文章',
      },
    },
    fetchResponse(url) {
      if (url.indexOf('mp.weixin.qq.com/s/') >= 0) {
        return Promise.resolve({
          ok: true,
          clone() {
            return {
              text() {
                return Promise.resolve(JSON.stringify({
                  data: {
                    video_page_info: {
                      subtype: 9,
                      mp_video_trans_info: [{
                        duration_ms: 125000,
                        url: 'https://vd.example.test/network.mp4?token=short-lived',
                      }],
                    },
                    video_page_infos: [{
                      video_id: 'video-network',
                      mp_video_trans_info: [{
                        duration_ms: 125000,
                        url: 'https://vd.example.test/network.mp4?token=short-lived',
                      }],
                    }],
                  },
                }));
              },
            };
          },
        });
      }
      if (url.indexOf('/api/mp/archive/download') >= 0) {
        return Promise.resolve({
          ok: true,
          json() { return Promise.resolve({ code: 0, data: { downloaded: 1, failed: 0 } }); },
        });
      }
      return Promise.resolve({ ok: true, json() { return Promise.resolve({ code: 0 }); } });
    },
  });

  env.sandbox.fetch('https://mp.weixin.qq.com/s/article-id?__biz=biz-video-network');
  await new Promise((resolve) => setImmediate(resolve));
  env.intervals[0].fn();

  const root = env.interactionBar.children[0];
  findElementByText(root.children[0], '下载').eventHandlers.click();
  findElementByText(root.children[1], '下载文章').eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  const call = env.fetchCalls.find((item) => item.url.indexOf('/api/mp/archive/download') >= 0);
  assert.ok(call);
  const request = JSON.parse(call.options.body);
  assert.equal(request.article.video_id, 'video-network');
  assert.equal(request.article.subtype, 9);
  assert.equal(request.article.duration, 125);
  assert.equal(request.article.play_url, 'https://vd.example.test/network.mp4?token=short-lived');
});

test('falls back to video DOM metadata when page globals are incomplete', async () => {
  const video = {
    tagName: 'VIDEO',
    attributes: {
      'data-vid': 'video-dom',
      'data-duration-ms': '98000',
      'data-video-url': 'https://vd.example.test/dom.mp4?token=short-lived',
    },
    duration: 0,
    getAttribute(name) { return this.attributes[name] || null; },
  };
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-video-dom&mid=10&idx=1',
    withInteractionBar: true,
    mediaElements: [video],
    articleContent: {
      outerHTML: '<div id="js_content"><video data-vid="video-dom"></video></div>',
      innerHTML: '<video data-vid="video-dom"></video>',
    },
    window: {
      cgiDataNew: {
        bizuin: 'biz-video-dom',
        title: 'DOM 视频文章',
      },
    },
    fetchResponse(url) {
      if (url.indexOf('/api/mp/archive/download') >= 0) {
        return Promise.resolve({
          ok: true,
          json() { return Promise.resolve({ code: 0, data: { downloaded: 1, failed: 0 } }); },
        });
      }
      return Promise.resolve({ ok: true, json() { return Promise.resolve({ code: 0 }); } });
    },
  });

  env.intervals[0].fn();
  const root = env.interactionBar.children[0];
  findElementByText(root.children[0], '下载').eventHandlers.click();
  findElementByText(root.children[1], '下载文章').eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  const call = env.fetchCalls.find((item) => item.url.indexOf('/api/mp/archive/download') >= 0);
  assert.ok(call);
  const request = JSON.parse(call.options.body);
  assert.equal(request.article.video_id, 'video-dom');
  assert.equal(request.article.duration, 98);
  assert.equal(request.article.play_url, 'https://vd.example.test/dom.mp4?token=short-lived');
});

test('reads nested appmsg metrics from getappmsgext responses', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-getappmsgext&mid=7&idx=1',
    window: { cgiDataNew: {} },
    fetchResponse(url) {
      if (url.indexOf('getappmsgext') >= 0) {
        return Promise.resolve({
          ok: true,
          clone() {
            return {
              text() {
                return Promise.resolve(JSON.stringify({
                  data: { appmsg_stat: JSON.stringify({ read_num: 401, old_like_num: 23, comment_num: 9, share_num: 4, favorite_num: 2 }) },
                }));
              },
            };
          },
        });
      }
      return Promise.resolve({ ok: true });
    },
  });

  env.sandbox.fetch('https://mp.weixin.qq.com/mp/getappmsgext?__biz=biz-getappmsgext');
  await new Promise((resolve) => setImmediate(resolve));
  env.intervals[0].fn();

  const calls = env.fetchCalls.filter((call) => call.url.indexOf('/api/mp/metrics') >= 0);
  assert.equal(calls.length, 1);
  const request = JSON.parse(calls[0].options.body);
  assert.deepEqual(request.metrics, {
    view_count: 401,
    like_count: 23,
    comment_count: 9,
    share_count: 4,
    collect_count: 2,
  });
});

test('reads comment totals from appmsg_comment network responses', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-comment-network&mid=7&idx=1',
    window: { cgiDataNew: {} },
    fetchResponse(url) {
      if (url.indexOf('/mp/appmsg_comment') >= 0) {
        return Promise.resolve({
          ok: true,
          clone() {
            return {
              text() {
                return Promise.resolve(JSON.stringify({ elected_comment_total_cnt: 37 }));
              },
            };
          },
        });
      }
      return Promise.resolve({ ok: true });
    },
  });

  env.sandbox.fetch('https://mp.weixin.qq.com/mp/appmsg_comment?action=getcomment&__biz=biz-comment-network');
  await new Promise((resolve) => setImmediate(resolve));
  env.intervals[0].fn();

  const calls = env.fetchCalls.filter((call) => call.url.indexOf('/api/mp/metrics') >= 0);
  assert.equal(calls.length, 1);
  const request = JSON.parse(calls[0].options.body);
  assert.equal(request.metrics.comment_count, 37);
});

test('proactively requests article comments with page comment identity', () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-comment-active&mid=7&idx=2&key=key-active',
    window: {
      cgiDataNew: {
        bizuin: 'biz-comment-active',
        comment_id: 'comment-active',
        mid: 7,
        idx: 2,
      },
    },
  });

  env.intervals[0].fn();
  const call = env.fetchCalls.find((item) => item.url.indexOf('/mp/appmsg_comment') >= 0);
  assert.ok(call);
  const query = new URL(call.url).searchParams;
  assert.equal(query.get('action'), 'getcomment');
  assert.equal(query.get('__biz'), 'biz-comment-active');
  assert.equal(query.get('appmsgid'), '7');
  assert.equal(query.get('idx'), '2');
  assert.equal(query.get('comment_id'), 'comment-active');
  assert.equal(call.options.credentials, 'include');
});

test('reads preload comment totals from article page data', () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-comment-preload&mid=8&idx=1',
    window: {
      cgiDataNew: {
        bizuin: 'biz-comment-preload',
        preload_comment_total_cnt: 19,
      },
    },
  });

  env.intervals[0].fn();
  const call = env.fetchCalls.find((item) => item.url.indexOf('/api/mp/metrics') >= 0);
  assert.ok(call);
  assert.equal(JSON.parse(call.options.body).metrics.comment_count, 19);
});

test('prefers current interaction fields over legacy metric aliases', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-metric-priority&mid=8&idx=1',
    window: { cgiDataNew: {} },
    fetchResponse(url) {
      if (url.indexOf('getappmsgext') >= 0) {
        return Promise.resolve({
          ok: true,
          clone() {
            return {
              text() {
                return Promise.resolve(JSON.stringify({
                  data: { appmsg_stat: JSON.stringify({ old_like_num: 630, like_num: 426, favorite_num: 240, collect_num: 174 }) }
                }));
              }
            };
          }
        });
      }
      return Promise.resolve({ ok: true });
    }
  });

  env.sandbox.fetch('https://mp.weixin.qq.com/mp/getappmsgext?__biz=biz-metric-priority');
  await new Promise((resolve) => setImmediate(resolve));
  env.intervals[0].fn();

  const calls = env.fetchCalls.filter((call) => call.url.indexOf('/api/mp/metrics') >= 0);
  assert.equal(calls.length, 1);
  const request = JSON.parse(calls[0].options.body);
  assert.equal(request.metrics.like_count, 426);
  assert.equal(request.metrics.collect_count, 174);
});

test('submits article interaction metrics with nullable counter semantics', () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-metric&mid=7&idx=1',
    window: {
      cgiDataNew: {
        bizuin: 'biz-metric',
        nick_name: '指标公众号',
        read_num: '12,345',
        like_num: '1.2万',
        comment_count: 8,
        share_count: 3,
      },
    },
    fetchResponse(url) {
      if (url.indexOf('/api/mp/metrics') >= 0) {
        return Promise.resolve({
          ok: true,
          json() { return Promise.resolve({ code: 0, data: { stored: true } }); },
        });
      }
      return Promise.resolve({ ok: true });
    },
  });

  env.intervals[0].fn();
  const calls = env.fetchCalls.filter((call) => call.url.indexOf('/api/mp/metrics') >= 0);
  assert.equal(calls.length, 1);
  const request = JSON.parse(calls[0].options.body);
  assert.equal(request.biz, 'biz-metric');
  assert.equal(request.article.content_url, 'https://mp.weixin.qq.com/s/article-id?__biz=biz-metric&mid=7&idx=1');
  assert.deepEqual(request.metrics, {
    view_count: 12345,
    like_count: 12000,
    comment_count: 8,
    share_count: 3,
  });
  assert.equal(request.metrics.collect_count, undefined);
  assert.equal(request.metrics.reward_count, undefined);
});

test('also collects metadata on public-account and author pages', () => {
  for (const pathname of ['/s', '/mp/profile_ext', '/mp/author']) {
    triggerSubmission(pathname);
  }
});

test('does not run on unrelated pages', () => {
  const env = loadOfficialAccount('/other');
  assert.equal(env.intervals.length, 0);
  assert.equal(env.fetchCalls.length, 0);
});

test('falls back to selection copy when the WebView clipboard API rejects', async () => {
  let nativeCalls = 0;
  const env = loadOfficialAccount('/mp/profile_ext', {
    navigator: {
      clipboard: {
        writeText() {
          nativeCalls += 1;
          return Promise.reject(new Error('clipboard permission denied'));
        },
      },
    },
    execCommandResult: true,
  });
  const panel = env.body.children[0];
  const button = panel.children.find((child) => child.eventHandlers.click);
  assert.ok(button);

  button.eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  assert.equal(nativeCalls, 1);
  assert.deepEqual(env.execCommandCalls, ['copy']);
  assert.equal(panel.querySelector('[data-status]').textContent, 'RSS 地址已复制');
});

test('uses selection copy when the WebView has no clipboard API', async () => {
  const env = loadOfficialAccount('/mp/profile_ext', { execCommandResult: true });
  const panel = env.body.children[0];
  const button = panel.children.find((child) => child.eventHandlers.click);
  assert.ok(button);

  button.eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  assert.deepEqual(env.execCommandCalls, ['copy']);
  assert.equal(panel.querySelector('[data-status]').textContent, 'RSS 地址已复制');
});

test('mounts the article menu and copies article and RSS content', async () => {
  const copied = [];
  const env = loadOfficialAccount('/s/article-id', {
    withInteractionBar: true,
    navigator: {
      clipboard: {
        writeText(value) {
          copied.push(value);
          return Promise.resolve();
        },
      },
    },
    window: {
      cgiDataNew: {
        bizuin: 'biz-menu',
        nick_name: '菜单公众号',
        content_noencode: '<p>article body</p>',
      },
    },
  });

  const root = env.interactionBar.children[0];
  assert.equal(root.id, '__wx_channels_mp_tools__');
  const trigger = root.children[0];
  const menu = root.children[1];
  assert.equal(trigger.attributes['aria-label'], '下载');
  assert.equal(trigger.children[0].className, 'wx-channels-mp-tools-icon');
  assert.match(trigger.children[0].innerHTML, /<svg/);
  assert.match(trigger.style.cssText, /background:transparent/);
  trigger.eventHandlers.mouseenter();
  assert.equal(trigger.style.background, 'rgba(7,193,96,.10)');
  trigger.eventHandlers.mouseleave();
  assert.equal(trigger.style.background, 'transparent');

  trigger.eventHandlers.click();
  assert.equal(menu.style.display, 'block');
  assert.equal(menu.attributes['aria-hidden'], 'false');
  assert.deepEqual(menu.children.map((item) => item.attributes['aria-label']), [
    '复制文章HTML',
    '复制RSS地址',
    '下载文章',
    '推送列表',
    '管理控制台',
  ]);
  assert.match(menu.style.cssText, /padding:6px/);

  const consoleItem = findElementByText(menu, '管理控制台');
  assert.match(consoleItem.children[0].innerHTML, /<rect x="3" y="4"/);
  assert.doesNotMatch(consoleItem.children[0].innerHTML, /M12 3v12/);

  const copyItem = findElementByText(menu, '复制文章HTML');
  assert.equal(copyItem.children[0].className, 'wx-channels-mp-icon');
  assert.match(copyItem.children[0].innerHTML, /<svg/);
  copyItem.eventHandlers.mouseenter();
  assert.equal(copyItem.style.background, '#ecfdf3');
  assert.equal(copyItem.style.color, '#067647');
  copyItem.eventHandlers.mouseleave();
  assert.equal(copyItem.style.background, 'transparent');
  copyItem.eventHandlers.click();
  findElementByText(menu, '复制RSS地址').eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  assert.deepEqual(copied, [
    '<p>article body</p>',
    'http://127.0.0.1:2026/rss/mp?biz=biz-menu&proxy=1',
  ]);
  assert.equal(menu.children.some((child) => child.textContent === '复制页面HTML'), false);
});

test('downloads the visible article through the archive API', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    href: 'https://mp.weixin.qq.com/s/article-id?__biz=biz-archive&mid=9&idx=1',
    withInteractionBar: true,
    articleContent: {
      outerHTML: '<div id="js_content"><p>正文内容</p><img data-src="//mmbiz.qpic.cn/image/1?wx_fmt=jpeg"></div>',
      innerHTML: '<p>正文内容</p>'
    },
    window: {
      cgiDataNew: {
        bizuin: 'biz-archive',
        title: '归档标题',
        digest: '归档摘要',
        author: '文章作者',
        audio_fileid: 17,
        video_page_infos: [{
          video_id: 'video-archive',
          mp_video_trans_info: [{
            duration_ms: 125000,
            url: 'https://vd.example.test/video.mp4?token=short-lived'
          }]
        }]
      }
    },
    fetchResponse(url) {
      if (url.indexOf('/api/mp/archive/download?token=test-token') >= 0) {
        return Promise.resolve({
          ok: true,
          json() {
            return Promise.resolve({ code: 0, data: { downloaded: 1, failed: 0 } });
          }
        });
      }
      return Promise.resolve({ ok: true, json() { return Promise.resolve({ code: 0 }); } });
    }
  });

  const root = env.interactionBar.children[0];
  findElementByText(root.children[0], '下载').eventHandlers.click();
  findElementByText(root.children[1], '下载文章').eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  const call = env.fetchCalls.find((item) => item.url.indexOf('/api/mp/archive/download') >= 0);
  assert.ok(call);
  assert.equal(call.options.method, 'POST');
  assert.equal(call.options.credentials, 'omit');
  const request = JSON.parse(call.options.body);
  assert.equal(request.biz, 'biz-archive');
  assert.equal(request.article.title, '归档标题');
  assert.equal(request.article.author, '文章作者');
  assert.equal(request.article.duration, 125);
  assert.equal(request.article.audio_fileid, 17);
  assert.equal(request.article.play_url, 'https://vd.example.test/video.mp4?token=short-lived');
  assert.equal(request.article.content_url, 'https://mp.weixin.qq.com/s/article-id?__biz=biz-archive&mid=9&idx=1');
  assert.match(request.html, /id="js_content"/);
  assert.match(request.html, /正文内容/);
});

test('loads the push list into an article dialog', async () => {
  const env = loadOfficialAccount('/s/article-id', {
    withInteractionBar: true,
    window: {
      cgiDataNew: { bizuin: 'biz-list', nick_name: '列表公众号' },
    },
    fetchResponse(url) {
      if (url.indexOf('/api/mp/msg/list?biz=biz-list&token=test-token') >= 0) {
        return Promise.resolve({
          ok: true,
          json() {
            return Promise.resolve({
              code: 0,
              data: {
                articles: [
                  { title: '第一篇', content_url: 'https://mp.weixin.qq.com/s/one', publish_time: 1710000000 },
                  { title: '第二篇', url: 'https://mp.weixin.qq.com/s/two', publish_time: 1710000600 },
                ],
              },
            });
          },
        });
      }
      return Promise.resolve({ ok: true, json() { return Promise.resolve({ code: 0 }); } });
    },
  });

  const root = env.interactionBar.children[0];
  findElementByText(root.children[0], '下载').eventHandlers.click();
  findElementByText(root.children[1], '推送列表').eventHandlers.click();
  await new Promise((resolve) => setImmediate(resolve));

  assert.equal(env.fetchCalls[0].url, 'http://127.0.0.1:2026/api/mp/msg/list?biz=biz-list&token=test-token');
  const overlay = env.sandbox.document.getElementById('__wx_channels_mp_message_list__');
  assert.ok(overlay);
  const dialog = overlay.children[0];
  assert.equal(dialog.attributes.role, 'dialog');
  assert.equal(dialog.attributes['aria-modal'], 'true');
  const closeButton = dialog.children[0].children[1];
  assert.equal(closeButton.attributes['aria-label'], '关闭');
  assert.match(closeButton.children[0].innerHTML, /<svg/);
  const rows = dialog.children[1].children;
  rows[0].eventHandlers.mouseenter();
  assert.equal(rows[0].style.background, '#f6fef9');
  assert.equal(rows[1].style.background, undefined);
  rows[0].eventHandlers.mouseleave();
  assert.equal(rows[0].style.background, 'transparent');
  assert.ok(findElementByText(dialog, '第一篇'));
  assert.ok(findElementByText(dialog, '第二篇'));
});

test('opens the local console from the article menu', () => {
  const env = loadOfficialAccount('/s/article-id', { withInteractionBar: true });
  const root = env.interactionBar.children[0];
  findElementByText(root.children[0], '下载').eventHandlers.click();
  findElementByText(root.children[1], '管理控制台').eventHandlers.click();

  assert.deepEqual(env.openedWindows, [{ url: 'http://127.0.0.1:2025/console', target: '_blank' }]);
});
