const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

test('download results remain visible and update the UI with the original payload', () => {
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
    const payload = { videoId: 'video-1', title: 'Result video', percentage: 42, error: 'download error', decrypted: true, url: 'https://example.test/video?token=private' };
    sandbox.__wx_api_client.handleCommand({ action, payload });
    assert.equal(events.at(-1).type, 'wx_' + action);
    assert.equal(events.at(-1).detail, payload);
  }
  assert.equal(tips.length, 2);
  assert.equal(tips[0].level, 'info');
  assert.equal(tips[1].level, 'error');
  assert.match(tips[0].msg, /Result video/);
  assert.match(tips[0].msg, /并解密/);
  assert.match(tips[1].msg, /Result video/);
  assert.match(tips[1].msg, /download error/);
  assert.ok(tips.every(tip => !tip.msg.includes('token=private')));

  sandbox.__wx_api_client.handleCommand({ action: 'download_complete' });
  sandbox.__wx_api_client.handleCommand({ action: 'download_failed', payload: {} });
  assert.equal(tips[2].level, 'info');
  assert.equal(tips[2].msg, '✓ 视频已下载');
  assert.equal(tips[3].level, 'error');
  assert.equal(tips[3].msg, '❌ 下载视频失败: 未知错误');
});
