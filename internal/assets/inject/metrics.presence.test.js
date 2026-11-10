const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

const metricKeys = ['playCount', 'likeCount', 'commentCount', 'favCount', 'forwardCount'];
const firstObservedAt = '2026-11-06T10:00:00.000Z';
const laterObservedAt = '2026-11-06T12:00:00.000Z';
const unknownMetrics = Object.fromEntries(metricKeys.map(key => [key, null]));
const domMetrics = {
  playCount: null,
  likeCount: 0,
  commentCount: null,
  favCount: null,
  forwardCount: 1234,
};

function loadScripts(page, countTexts = []) {
  const requests = [];
  let clockTime = Date.parse(firstObservedAt);
  class ClockDate extends Date {
    constructor(...args) { super(...(args.length ? args : [clockTime])); }
    static now() { return clockTime; }
  }
  const counts = countTexts.map(text => text == null ? null : { textContent: text });
  const feedNode = {
    id: 'flow-feed-metrics-video',
    getBoundingClientRect() { return { top: 0, left: 0, bottom: 800, right: 1000 }; },
    querySelector(selector) {
      if (selector === '.feed-video video, video') {
        return { currentSrc: 'https://video.example.test/metrics.mp4' };
      }
      if (selector.includes('.ctn')) return { textContent: 'Metrics video' };
      return null;
    },
    querySelectorAll() { return counts; },
  };
  const sandbox = {
    Date: ClockDate,
    console: { log() {}, warn() {}, error() {} },
    document: {
      documentElement: { clientHeight: 800, clientWidth: 1000 },
      getElementById() { return null; },
      querySelector() { return null; },
      querySelectorAll(selector) {
        return selector === '[id^="flow-feed-"]' ? [feedNode] : counts;
      },
      createElement() {
        return { set innerHTML(value) { this.textContent = value; } };
      },
    },
    location: {
      pathname: '/web/pages/' + (page || 'home'),
      href: 'https://channels.weixin.qq.com/web/pages/' + (page || 'home'),
      origin: 'https://channels.weixin.qq.com',
    },
    navigator: { userAgent: 'metrics-test' },
    innerHeight: 800,
    innerWidth: 1000,
    URL,
    URLSearchParams,
    setTimeout() { return 1; },
    clearTimeout() {},
    __wx_log() {},
    alert(message) { assert.fail('Unexpected alert: ' + message); },
    WXE: {
      onAPILoaded() {},
      onPCFlowLoaded() {},
      onGotoNextFeed() {},
      onGotoPrevFeed() {},
      onFetchFeedProfile() {},
      onFeed() {},
    },
    async fetch(url, options) {
      requests.push({ url, options, body: JSON.parse(options.body) });
      return { ok: true, async json() { return { success: true }; } };
    },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  const files = ['utils.js', 'download.js'];
  if (page) files.push(page + '.js');
  for (const file of files) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, file), 'utf8'), sandbox, { filename: file });
  }
  return { sandbox, requests, setTime(iso) { clockTime = Date.parse(iso); } };
}

function apiFeed(extra = {}) {
  return {
    id: 'metrics-video',
    objectDesc: {
      mediaType: 4,
      description: 'Metrics video',
      media: [{
        url: 'https://video.example.test/metrics.mp4',
        fullUrl: 'https://video.example.test/original.mp4',
      }],
    },
    ...extra,
  };
}

function extractDomProfile(sandbox, page) {
  return page === 'feed'
    ? sandbox.__extract_profile_from_feed_dom_fallback('metrics-video')
    : sandbox.__extract_profile_from_dom_fallback('metrics-video');
}

function values(profile) {
  return Object.fromEntries(metricKeys.map(key => [key, profile[key]]));
}

test('DOM count parser accepts complete nonnegative integers including real zero and comma groups', () => {
  const { sandbox } = loadScripts();
  const parse = sandbox.WXU.parse_dom_metric_count;
  assert.equal(typeof parse, 'function');
  for (const [input, expected] of [
    ['0', 0], [' 0 ', 0], ['1234', 1234], ['1,234', 1234],
    [' 12,345,678 ', 12345678], ['9007199254740991', Number.MAX_SAFE_INTEGER], [0, 0],
  ]) {
    assert.equal(parse(input), expected, String(input));
  }
});

test('DOM count parser keeps missing, abbreviated, malformed and inexact counts unknown', () => {
  const { sandbox } = loadScripts();
  const parse = sandbox.WXU.parse_dom_metric_count;
  assert.equal(typeof parse, 'function');
  for (const input of [
    undefined, null, '', ' ', '赞', '--', '1.2万', '12万', '1.2亿', '1.2K',
    '1.2', '-1', '+1', '1,23', '1,,234', '12 345', '123人', '1e3',
    '9007199254740992', -1, 1.2, Infinity, NaN, true, {},
  ]) {
    assert.equal(parse(input), null, String(input));
  }
});

for (const source of [undefined, '', 'wechat_api', 'wechat_dom', 'other']) {
  test('video format_feed sets API source unless upstream explicitly marks DOM: ' + String(source), () => {
    const { sandbox } = loadScripts();
    const input = apiFeed({ ...domMetrics, metricsSource: source });
    const profile = sandbox.WXU.format_feed(input);
    assert.equal(profile.metricsSource, source === 'wechat_dom' ? 'wechat_dom' : 'wechat_api');
    assert.deepEqual(values(profile), domMetrics);
    assert.equal(input.metricsSource, source, 'formatting must not mutate the upstream feed');
  });
}

test('format_feed preserves absence of API counts instead of manufacturing zero', () => {
  const { sandbox } = loadScripts();
  const profile = sandbox.WXU.format_feed(apiFeed());
  for (const key of metricKeys) assert.equal(profile[key], undefined, key);
});

test('format_feed retains the first observation time for unchanged counts on the same feed object', () => {
  const { sandbox, setTime } = loadScripts();
  const feed = apiFeed(domMetrics);
  const first = sandbox.WXU.format_feed(feed);
  assert.equal(first.metricsObservedAt, firstObservedAt);
  setTime(laterObservedAt);
  feed.objectDesc.description = 'Updated title';
  assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, firstObservedAt);
  assert.equal(first.metricsObservedAt, firstObservedAt);
  assert.equal(feed.metricsObservedAt, undefined, 'observation cache must not mutate the raw feed');
});

for (const key of metricKeys) {
  test('format_feed refreshes the observation when ' + key + ' changes on the same object', () => {
    const { sandbox, setTime } = loadScripts();
    const feed = apiFeed(unknownMetrics);
    const first = sandbox.WXU.format_feed(feed);
    assert.equal(first.metricsObservedAt, firstObservedAt);
    setTime(laterObservedAt);
    feed[key] = 0;
    const updated = sandbox.WXU.format_feed(feed);
    assert.equal(updated.metricsObservedAt, laterObservedAt);
    assert.equal(updated[key], 0);
    setTime('2026-11-06T14:00:00.000Z');
    assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, laterObservedAt);
    assert.equal(first.metricsObservedAt, firstObservedAt, 'an old profile keeps its own observation time');
  });
}

test('format_feed treats absent and null counters as different observations', () => {
  const { sandbox, setTime } = loadScripts();
  const feed = apiFeed();
  assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, firstObservedAt);
  setTime(laterObservedAt);
  feed.likeCount = null;
  assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, laterObservedAt);
});

test('format_feed records a new observation for a new API response object with identical counts', () => {
  const { sandbox, setTime } = loadScripts();
  const feed = apiFeed(domMetrics);
  assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, firstObservedAt);
  setTime(laterObservedAt);
  assert.equal(sandbox.WXU.format_feed({ ...feed }).metricsObservedAt, laterObservedAt);
  assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, firstObservedAt);
});

test('readCount changes do not refresh the observation of unchanged playCount metrics', () => {
  const { sandbox, setTime } = loadScripts();
  const feed = apiFeed({ ...domMetrics, playCount: 0, readCount: 1234 });
  assert.equal(sandbox.WXU.format_feed(feed).metricsObservedAt, firstObservedAt);
  setTime(laterObservedAt);
  feed.readCount = 5678;
  const profile = sandbox.WXU.format_feed(feed);
  assert.equal(profile.metricsObservedAt, firstObservedAt);
  assert.equal(profile.playCount, 0);
  assert.equal(profile.readCount, 5678, 'legacy UI still receives readCount');
});

for (const page of ['feed', 'home']) {
  test(page + ' DOM fallback preserves real zero, comma counts, abbreviations and absent nodes', () => {
    const { sandbox } = loadScripts(page, ['0', '1,234', '1.2万', null]);
    const profile = extractDomProfile(sandbox, page);
    assert.equal(profile.metricsSource, 'wechat_dom');
    assert.deepEqual(values(profile), domMetrics);
  });

  test(page + ' DOM fallback marks all missing metrics as null', () => {
    const { sandbox } = loadScripts(page);
    const profile = extractDomProfile(sandbox, page);
    assert.deepEqual(values(profile), unknownMetrics);
  });

  test(page + ' DOM fallback rejects labels and malformed counts', () => {
    const { sandbox } = loadScripts(page, ['赞', '', '-1', '123人']);
    const profile = extractDomProfile(sandbox, page);
    assert.deepEqual(values(profile), unknownMetrics);
  });

  test(page + ' DOM fallback timestamps each fresh read even when counts are unchanged', () => {
    const { sandbox, setTime } = loadScripts(page, ['0', '1,234', '1.2万', null]);
    const first = extractDomProfile(sandbox, page);
    assert.equal(first.metricsObservedAt, firstObservedAt);
    setTime(laterObservedAt);
    const second = extractDomProfile(sandbox, page);
    assert.equal(second.metricsObservedAt, laterObservedAt);
    assert.deepEqual(values(second), values(first));
    assert.equal(first.metricsObservedAt, firstObservedAt);
  });

  test(page + ' DOM profile reaches the download request without losing nulls or provenance', async () => {
    const { sandbox, requests, setTime } = loadScripts(page, ['0', '1,234', '1.2万', null]);
    const profile = page === 'feed'
      ? sandbox.__sync_feed_profile_with_runtime(false)
      : sandbox.__sync_home_profile_with_runtime(false);
    assert.ok(profile);
    assert.equal(profile.metricsObservedAt, firstObservedAt);
    setTime(laterObservedAt);
    await sandbox.__wx_channels_handle_click_download__({ fileFormat: 'xWT111', width: 720, height: 1280 });
    const request = requests.find(item => item.url === '/__wx_channels_api/download_video');
    assert.ok(request, 'click must submit a backend download');
    assert.equal(request.body.metricsSource, 'wechat_dom');
    assert.equal(request.body.metricsObservedAt, firstObservedAt, '12:00 download retains the 10:00 observation');
    assert.deepEqual(request.body.metricValues, domMetrics);
    assert.equal(request.body.likeCount, 0, 'legacy GUI count remains an integer');
    assert.equal(request.body.favCount, 0, 'unknown legacy GUI count remains compatible');
  });
}

test('API feed publication and original download preserve source, real zero and explicit nulls', async () => {
  const { sandbox, requests, setTime } = loadScripts();
  const apiMetrics = { ...domMetrics, playCount: 0 };
  sandbox.WXU.set_feed(apiFeed({ ...apiMetrics, readCount: 1234 }));
  const published = requests.find(item => item.url === '/__wx_channels_api/profile');
  assert.ok(published);
  assert.equal(published.body.metricsSource, 'wechat_api');
  assert.equal(published.body.metricsObservedAt, firstObservedAt);
  assert.deepEqual(values(published.body), apiMetrics);
  assert.equal(published.body.readCount, 1234, 'readCount remains available to the legacy UI');

  setTime(laterObservedAt);
  await sandbox.__wx_channels_handle_click_download__(null);
  const download = requests.find(item => item.url === '/__wx_channels_api/download_video');
  assert.ok(download);
  assert.equal(download.body.metricsSource, 'wechat_api');
  assert.equal(download.body.metricsObservedAt, firstObservedAt, '12:00 download retains the 10:00 observation');
  assert.deepEqual(download.body.metricValues, apiMetrics);
});

test('readCount alone never manufactures a play count in profiles or download metrics', async () => {
  const { sandbox, requests } = loadScripts();
  sandbox.WXU.set_feed(apiFeed({ readCount: 1234 }));
  const profile = sandbox.__wx_channels_store__.profile;
  assert.equal(profile.readCount, 1234);
  assert.equal(profile.playCount, undefined);
  const published = requests.find(item => item.url === '/__wx_channels_api/profile');
  assert.equal(Object.hasOwn(published.body, 'playCount'), false);
  await sandbox.__wx_channels_handle_click_download__(null);
  const download = requests.find(item => item.url === '/__wx_channels_api/download_video');
  assert.ok(download);
  assert.deepEqual(download.body.metricValues, {});
});

test('backend request uses empty provenance for an unmarked profile', async () => {
  const { sandbox, requests } = loadScripts();
  await sandbox.__wx_channels_download_via_backend__(
    { id: 'unmarked', ...unknownMetrics },
    'unmarked',
    { mode: 'original', url: 'https://video.example.test/unmarked.mp4' },
  );
  assert.equal(requests[0].body.metricsSource, '');
  assert.equal(requests[0].body.metricsObservedAt, '', 'download must not invent an observation time');
  assert.deepEqual(requests[0].body.metricValues, unknownMetrics);
});

test('download request keeps absent API counts absent in metricValues', async () => {
  const { sandbox, requests } = loadScripts();
  sandbox.WXU.set_feed(apiFeed({ likeCount: 0 }));
  await sandbox.__wx_channels_handle_click_download__(null);
  const request = requests.find(item => item.url === '/__wx_channels_api/download_video');
  assert.ok(request);
  assert.deepEqual(request.body.metricValues, { likeCount: 0 });
});
