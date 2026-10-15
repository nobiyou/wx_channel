const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function video(id, type = 'media', extra = {}) {
  return { id, type, title: id, url: 'https://example.test/' + id, key: '123', ...extra };
}

function loadBatch(pathname = '/web/pages/profile') {
  const requests = [];
  let submittedCount = 0;
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    location: { pathname, href: 'https://channels.weixin.qq.com' + pathname, origin: 'https://channels.weixin.qq.com' },
    navigator: { userAgent: 'batch-test' },
    document: { getElementById() { return null; } },
    __wx_log() {},
    setTimeout(fn, delay) { if (delay === 100) queueMicrotask(fn); return 1; },
    setInterval() { throw new Error('completed downloads should not start polling'); },
    clearInterval() {},
    async fetch(url, options) {
      if (url.endsWith('/batch_start')) {
        const body = JSON.parse(options.body);
        requests.push(body);
        submittedCount = body.videos.length;
        return { ok: true, async json() { return { success: true, data: { concurrency: 5 } }; } };
      }
      assert.ok(url.endsWith('/batch_progress'), 'unexpected API: ' + url);
      return { ok: true, async json() { return { success: true, data: { total: submittedCount, done: submittedCount } }; } };
    },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'batch_download.js'), 'utf8'), sandbox);
  return { manager: sandbox.__wx_batch_download_manager__, sandbox, requests };
}

const ids = items => Array.from(items, item => item.id);

test('profile batch download defaults to videos and separates live replays', () => {
  const { manager } = loadBatch();
  manager.setVideos([video('replay', 'live_replay'), video('short'), video('legacy', 'media', { type: undefined }), video('live', 'live')]);
  assert.equal(manager.activeTab, 'video');
  assert.deepEqual(ids(manager.getCurrentPageVideos()), ['short', 'legacy']);
  assert.equal(manager.setActiveTab('live_replay'), true);
  assert.deepEqual(ids(manager.getCurrentPageVideos()), ['replay']);
});

test('tab pagination and select-all use only the current category and preserve selections', () => {
  const { manager } = loadBatch();
  manager.pageSize = 2;
  manager.setVideos([video('v1'), video('r1', 'live_replay'), video('v2'), video('r2', 'live_replay'), video('v3')]);
  assert.equal(manager.getTotalPages(), 2);
  manager.currentPage = 2;
  manager.selectAllCurrentPage(true);
  assert.deepEqual(ids(manager.getSelectedVideos()), ['v3']);
  manager.setActiveTab('live_replay');
  assert.equal(manager.currentPage, 1);
  assert.equal(manager.getTotalPages(), 1);
  assert.deepEqual(ids(manager.getSelectedVideos()), []);
  manager.selectAllCurrentPage(true);
  assert.deepEqual(ids(manager.getSelectedVideos()), ['r1', 'r2']);
  manager.setActiveTab('video');
  assert.deepEqual(ids(manager.getSelectedVideos()), ['v3']);
  manager.selectAllCurrentPage(true);
  manager.selectAllCurrentPage(false);
  assert.deepEqual(ids(manager.getSelectedVideos()), ['v3']);
});

test('newly collected entries update categories without switching tabs or losing selections', () => {
  const { manager } = loadBatch();
  manager.setVideos([video('v1'), video('r1', 'live_replay')]);
  manager.toggleSelect('v1', true);
  assert.equal(manager.appendVideos([video('r2', 'live_replay'), video('v2'), video('r1', 'live_replay')]), 2);
  assert.equal(manager.activeTab, 'video');
  assert.deepEqual(ids(manager.getCurrentPageVideos()), ['v1', 'v2']);
  assert.deepEqual(ids(manager.getSelectedVideos()), ['v1']);
  assert.equal(manager.getVideosForTab('live_replay').length, 2);
});

test('clearing a category preserves the other category and its selections', () => {
  const { manager } = loadBatch();
  manager.setVideos([video('v1'), video('r1', 'live_replay')]);
  manager.toggleSelect('v1', true);
  manager.toggleSelect('r1', true);
  manager.clearCurrentTab();
  assert.deepEqual(ids(manager.videos), ['r1']);
  assert.deepEqual(ids(manager.getSelectedVideos()), []);
  assert.equal(manager.currentPage, 1);
  assert.equal(manager.getTotalPages(), 1);
  manager.setActiveTab('live_replay');
  assert.deepEqual(ids(manager.getSelectedVideos()), ['r1']);
});

test('empty video category stays empty until the user chooses live replays', () => {
  const { manager } = loadBatch();
  manager.setVideos([video('r1', 'live_replay')]);
  assert.deepEqual(ids(manager.getCurrentPageVideos()), []);
  assert.equal(manager.getTotalPages(), 1);
  manager.setActiveTab('live_replay');
  assert.deepEqual(ids(manager.getCurrentPageVideos()), ['r1']);
  manager.isDownloading = true;
  assert.equal(manager.setActiveTab('video'), false);
  assert.equal(manager.activeTab, 'live_replay');
});

test('closing and reopening a running replay download preserves its view and selections', () => {
  const { manager, sandbox } = loadBatch();
  manager.pageSize = 1;
  manager.setVideos([video('v1'), video('r1', 'live_replay'), video('r2', 'live_replay')], 'Original profile');
  manager.setActiveTab('live_replay');
  manager.currentPage = 2;
  manager.toggleSelect('r2', true);
  manager.isDownloading = true;
  manager.isVisible = true;
  const ui = { style: {}, removed: false, remove() { this.removed = true; } };
  sandbox.document.getElementById = id => id === 'wx-batch-download-ui' && !ui.removed ? ui : null;

  sandbox.__close_batch_download_ui__();
  assert.equal(manager.isVisible, false);
  assert.equal(ui.removed, false, 'running download must retain its progress controls');
  assert.equal(ui.style.display, 'none');
  sandbox.__show_batch_download_ui__([video('v1'), video('r1', 'live_replay'), video('r2', 'live_replay'), video('r3', 'live_replay')], 'Original profile');
  assert.equal(manager.isVisible, true);
  assert.equal(ui.style.display, '');
  assert.equal(manager.isDownloading, true);
  assert.equal(manager.activeTab, 'live_replay');
  assert.equal(manager.currentPage, 2);
  assert.equal(manager.title, 'Original profile');
  assert.deepEqual(ids(manager.getSelectedVideos()), ['r2']);
  assert.deepEqual(ids(manager.getVideosForTab('live_replay')), ['r1', 'r2', 'r3'], 'reopen includes entries collected while hidden');
  assert.equal(manager.setActiveTab('video'), false);

  manager.isDownloading = false;
  assert.equal(manager.setActiveTab('video'), true);
  sandbox.__close_batch_download_ui__();
  assert.equal(ui.removed, true, 'completed dialog closes normally');
  manager.setVideos([video('new')]);
  assert.equal(manager.activeTab, 'video');
  assert.equal(manager.currentPage, 1);
  assert.deepEqual(ids(manager.getSelectedVideos()), []);
});

test('home and likes pages keep their existing combined list', () => {
  for (const pathname of ['/web/pages/home', '/web/pages/account/like']) {
    const { manager } = loadBatch(pathname);
    manager.setVideos([video('v1'), video('r1', 'live_replay')]);
    assert.equal(manager.activeTab, 'all');
    assert.deepEqual(ids(manager.getCurrentPageVideos()), ['v1', 'r1']);
  }
});

test('select-all skips entries that cannot be downloaded', () => {
  const { manager } = loadBatch('/web/pages/home');
  manager.setVideos([video('v1'), video('blocked', 'media', { canDownload: false }), video('live', 'live')]);
  manager.selectAllCurrentPage(true);
  assert.deepEqual(ids(manager.getSelectedVideos()), ['v1']);
});

test('batch API receives only selections in the active tab', async () => {
  const { manager, sandbox, requests } = loadBatch();
  manager.setVideos([video('v1'), video('r1', 'live_replay')]);
  manager.toggleSelect('v1', true);
  manager.toggleSelect('r1', true);
  await sandbox.__batch_download_selected__();
  assert.deepEqual(ids(requests[0].videos), ['v1']);
  assert.equal(manager.isDownloading, false);
  manager.setActiveTab('live_replay');
  await sandbox.__batch_download_selected__();
  assert.deepEqual(ids(requests[1].videos), ['r1']);
  assert.equal(manager.isDownloading, false);
});
