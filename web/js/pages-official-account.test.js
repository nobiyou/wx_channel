const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadOfficialAccountPage(options = {}) {
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    URL,
    document: {
      readyState: 'complete',
      getElementById() { return null; },
      addEventListener() {}
    },
    window: {
      location: { href: 'http://127.0.0.1:2025/console' }
    },
    escapeHtml(value) {
      return String(value || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;');
    },
    ConnectionManager: options.connectionManager || {
      getServiceUrl() { return 'http://127.0.0.1:2026'; }
    }
  };
  vm.createContext(sandbox);
  const source = fs.readFileSync(path.resolve(__dirname, 'pages-official-account.js'), 'utf8');
  vm.runInContext(source, sandbox, { filename: 'pages-official-account.js' });
  return sandbox;
}

test('routes qpic account avatars through the configured service', () => {
  const env = loadOfficialAccountPage();
  const markup = env.officialAccountAvatarMarkup({
    nickname: 'Account',
    biz: 'biz-avatar',
    avatar_url: 'https://mmbiz.qpic.cn/avatar/1?wx_fmt=jpeg'
  });

  assert.match(markup, /src="http:\/\/127\.0\.0\.1:2026\/mp\/proxy\?url=https%3A%2F%2Fmmbiz\.qpic\.cn%2Favatar%2F1%3Fwx_fmt%3Djpeg"/);
});

test('keeps unsupported avatar hosts as direct URLs', () => {
  const env = loadOfficialAccountPage();
  const markup = env.officialAccountAvatarMarkup({
    nickname: 'Account',
    biz: 'biz-avatar',
    avatar_url: 'https://example.com/avatar.png'
  });

  assert.match(markup, /src="https:\/\/example\.com\/avatar\.png"/);
});

test('keeps account identity and biz readable in the compact account card', () => {
  const env = loadOfficialAccountPage();
  const markup = env.renderOfficialAccount({
    nickname: '一个较长的公众号名称',
    biz: 'MzI1NjA0MDg2Mw==',
    is_effective: true,
    article_count: 21,
    archived_count: 4,
    sync_status: 'completed'
  });

  assert.match(markup, /official-account-item-head/);
  assert.match(markup, /official-account-biz-line/);
  assert.match(markup, /<code class="official-account-biz"[^>]*>MzI1NjA0MDg2Mw==<\/code>/);
  assert.match(markup, /official-account-item-footer/);
  assert.match(markup, /official-account-copy/);
  assert.doesNotMatch(markup, /official-account-item-side/);
});

test('renders complete video metadata in the article row', () => {
  const env = loadOfficialAccountPage();
  const markup = env.renderOfficialArticle({
    key: 'article-video',
    title: '视频文章',
    content_url: 'https://mp.weixin.qq.com/s/video',
    publish_time: 1710000000,
    last_seen_at: 1710000000,
    archive_status: 'not_archived',
    video_id: 'video-17',
    duration: 125,
    audio_fileid: 17,
    play_url: 'https://vd.example.test/video.mp4'
  });

  assert.match(markup, /official-article-type-badge video/);
  assert.match(markup, />视频<\/span>/);
  assert.match(markup, />02:05<\/span>/);
  assert.match(markup, /视频 ID <code>video-17<\/code>/);
  assert.match(markup, /音频 ID 17/);
  assert.match(markup, /媒体地址已记录/);
});

test('uses audio identity before duration when classifying media', () => {
  const env = loadOfficialAccountPage();
  const media = env.officialAccountMediaInfo({
    duration: 125,
    audio_fileid: 17,
    play_url: 'https://vd.example.test/audio.mp3'
  });

  assert.equal(media.kind, 'audio');
  assert.equal(media.videoID, '');
});

test('does not submit duplicate metric sync starts while the first request is pending', async () => {
  const env = loadOfficialAccountPage();
  vm.runInContext("officialAccountConsoleState.accounts = [{ biz: 'biz-lock', is_effective: true }]; officialAccountConsoleState.selectedBiz = 'biz-lock';", env);
  let calls = 0;
  let release;
  const pending = new Promise((resolve) => { release = resolve; });
  env.ApiClient = {
    startOfficialMetricSync() {
      calls += 1;
      return pending;
    },
  };
  env.showMessage = () => {};

  const first = env.startOfficialAccountMetricSync(false);
  const second = env.startOfficialAccountMetricSync(true);
  assert.equal(calls, 1);
  release({ code: 0, data: { id: 'metric-run-lock', status: 'queued', biz: 'biz-lock', total: 0 } });
  await Promise.all([first, second]);
  assert.equal(vm.runInContext('officialAccountConsoleState.metricSyncStartPromise', env), null);
});
