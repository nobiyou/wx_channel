const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadParser() {
  const source = fs.readFileSync(path.resolve(__dirname, 'batch-input.js'), 'utf8');
  const sandbox = { console, Number, Object, String, JSON, encodeURIComponent };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'batch-input.js' });
  return sandbox;
}

test('normalizes profile batch exports with numeric duration and nickname', () => {
  const env = loadParser();
  const videos = env.parseBatchVideoJSON(JSON.stringify([{
    id: 'profile-video-1',
    title: '个人主页视频',
    url: 'https://cdn.example/video.mp4',
    nickname: '测试账号',
    duration: 12500,
    size: 1024,
    key: 'decode-key'
  }]));

  assert.equal(videos.length, 1);
  assert.equal(videos[0].author, '测试账号');
  assert.equal(videos[0].authorName, '测试账号');
  assert.equal(videos[0].durationMs, 12500);
  assert.equal(videos[0].key, 'decode-key');
});

test('accepts the single-video raw export envelope', () => {
  const env = loadParser();
  const videos = env.parseBatchVideoJSON(JSON.stringify({
    exportedAt: '2026-11-12T00:00:00Z',
    pageUrl: 'https://channels.weixin.qq.com/web/pages/feed',
    profile: {
      id: 'raw-video-1',
      title: '原始视频',
      url: 'https://cdn.example/raw.mp4',
      nickname: '原始作者',
      duration: '01:05'
    },
    rawFeed: { id: 'raw-video-1' }
  }));

  assert.equal(videos.length, 1);
  assert.equal(videos[0].id, 'raw-video-1');
  assert.equal(videos[0].durationMs, 65000);
  assert.equal(videos[0].sourceUrl, 'https://channels.weixin.qq.com/web/pages/feed');
});
