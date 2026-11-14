const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

test('quiet backend notifications still update download UI with the original payload', () => {
  const events = [];
  const tips = [];
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    location: new URL('https://channels.weixin.qq.com/web/pages/home'),
    URL, URLSearchParams, Date,
    document: {
      readyState: 'loading', addEventListener() {},
      dispatchEvent(event) { events.push(event); },
    },
    CustomEvent: function (type, options) { this.type = type; this.detail = options.detail; },
    __wx_log(tip) { tips.push(tip); },
    setTimeout() {}, clearTimeout() {}, setInterval() {}, clearInterval() {},
  };
  sandbox.window = sandbox;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'api_client.js'), 'utf8'), sandbox);
  for (const action of ['download_progress', 'download_complete', 'download_failed']) {
    const payload = { videoId: 'video-1', percentage: 42, error: 'download error', decrypted: true };
    sandbox.__wx_api_client.handleCommand({ action, payload });
    assert.equal(events.at(-1).type, 'wx_' + action);
    assert.equal(events.at(-1).detail, payload);
  }
  assert.equal(tips.length, 2);
  assert.ok(tips.every(tip => tip.level === 'debug'));
  assert.match(tips[1].msg, /download error/);
});
