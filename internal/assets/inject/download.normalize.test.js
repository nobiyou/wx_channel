const fs = require('fs');
const path = require('path');
const vm = require('vm');

function loadDownloadModule() {
  const file = path.resolve(__dirname, 'download.js');
  const source = fs.readFileSync(file, 'utf8');

  const sandbox = {
    console: { log() {}, error() {}, warn() {} },
    window: {},
    document: {
      querySelector() { return null; },
      createElement() {
        return {
          setAttribute() {},
          getAttribute() { return ''; },
          addEventListener() {},
          appendChild() {},
          style: {},
          click() {},
        };
      },
      head: { appendChild() {} },
      body: { appendChild() {} },
      getElementById() { return null; },
    },
    location: {
      href: 'https://channels.weixin.qq.com/web/pages/home',
      origin: 'https://channels.weixin.qq.com',
    },
    navigator: { userAgent: 'node-test' },
    fetch() {
      throw new Error('fetch should not be called in normalization tests');
    },
    alert() {
      throw new Error('alert should not be called in normalization tests');
    },
    Blob: function Blob() {},
    URL,
    URLSearchParams,
    setTimeout,
    clearTimeout,
    Date,
    encodeURIComponent,
    decodeURIComponent,
    __wx_log() {},
    formatFileSize(v) { return String(v); },
    __wx_channels_store__: {},
  };

  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: file });
  return sandbox;
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}\nactual:   ${actual}\nexpected: ${expected}`);
  }
}

function main() {
  const sandbox = loadDownloadModule();
  const normalize = sandbox.__wx_channels_normalize_video_download__;

  const profile = {
    url: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942',
    originalUrl: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac',
    urlToken: '&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942',
    media: {
      url: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac',
      urlToken: '&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942',
      width: 1080,
      height: 1920,
      fileSize: 24 * 1024 * 1024,
      fullUrl: '',
    },
  };

  assertEqual(
    sandbox.__wx_channels_has_true_original__(profile),
    false,
    'a source fileSize hint must not be treated as an original URL',
  );

  const availableProfile = Object.assign({}, profile, {
    spec: [
      { fileFormat: 'xWT113', bitRate: 1200 },
      { fileFormat: 'xWT111', bitRate: 4800 },
      { fileFormat: 'xWT127', bitRate: 2400 },
    ],
  });
  assertEqual(
    sandbox.__wx_channels_get_best_available_spec__(availableProfile).fileFormat,
    'xWT111',
    'highest available fallback should select the highest bitrate rendition',
  );
  assertEqual(
    sandbox.__wx_channels_primary_download_label__(availableProfile),
    '最高可用画质 (xWT111)',
    'primary label should disclose that the fallback is a rendition',
  );
  const batchFallback = sandbox.__wx_channels_normalize_batch_video_download__(availableProfile);
  assertEqual(batchFallback.mode, 'specific', 'batch mode should fall back to a specific rendition');
  assertEqual(batchFallback.fileFormat, 'xWT111', 'batch fallback should use the highest available rendition');
  assertEqual(
    batchFallback.url,
    profile.url + '&X-snsvideoflag=xWT111',
    'batch fallback should append the selected rendition without losing signed parameters',
  );

  const trueOriginalProfile = Object.assign({}, profile, {
    media: Object.assign({}, profile.media, {
      fullUrl: 'https://finder.video.qq.com/full/original.mp4',
    }),
  });
  assertEqual(
    sandbox.__wx_channels_has_true_original__(trueOriginalProfile),
    true,
    'a fullUrl supplied by the feed should enable original mode',
  );
  assertEqual(
    sandbox.__wx_channels_primary_download_label__(trueOriginalProfile),
    '原始视频',
    'original label should only be used when a fullUrl exists',
  );
  assertEqual(
    normalize(trueOriginalProfile, null).url,
    'https://finder.video.qq.com/full/original.mp4',
    'original mode should prefer the explicit fullUrl over the preview URL',
  );
  assertEqual(
    sandbox.__wx_channels_normalize_batch_video_download__(trueOriginalProfile).mode,
    'original',
    'batch mode should preserve a real original URL',
  );

  const original = normalize(profile, null);
  assertEqual(
    original.url,
    profile.url,
    'original mode should preserve the complete signed URL',
  );
  assertEqual(original.mode, 'original', 'original mode should be preserved');
  assertEqual(original.useDirectDownload, true, 'original mode should use the page session first');
  assertEqual(original.resolution, '1080x1920', 'original mode should preserve dimensions');

  const expectedSize = sandbox.__wx_channels_get_expected_video_size__(profile);
  assertEqual(expectedSize, 24 * 1024 * 1024, 'original mode should read the source size hint');
  sandbox.__wx_channels_validate_original_video_size__(expectedSize, expectedSize);
  let rejectedShrink = false;
  try {
    sandbox.__wx_channels_validate_original_video_size__(expectedSize, expectedSize * 0.5);
  } catch (err) {
    rejectedShrink = true;
  }
  assertEqual(rejectedShrink, true, 'original mode should reject an obviously shrunken stream');

  const markedOriginal = normalize({
    url: profile.url + '&X-snsvideoflag=original',
    media: profile.media,
  }, null);
  assertEqual(
    markedOriginal.url,
    profile.url,
    'original mode should remove the legacy marker before page-session download',
  );

  const specific = normalize(profile, {
    fileFormat: 'xWT111',
    width: 720,
    height: 1280,
  });
  assertEqual(
    specific.url,
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942&X-snsvideoflag=xWT111',
    'specific mode should preserve stream params and append explicit spec',
  );
  assertEqual(specific.useDirectDownload, false, 'specific mode should continue through the backend');

  const markedSpecific = normalize({
    url: profile.url + '&X-snsvideoflag=original',
    media: profile.media,
  }, {
    fileFormat: 'xWT111',
    width: 720,
    height: 1280,
  });
  assertEqual(
    markedSpecific.url,
    specific.url,
    'specific mode should replace a legacy original marker with the explicit spec',
  );

  const compactPrimary = {
    url: 'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456',
    originalUrl: profile.originalUrl,
    urlToken: profile.urlToken,
    media: profile.media,
  };
  const recovered = normalize(compactPrimary, null);
  assertEqual(
    recovered.url,
    profile.url,
    'original mode should recover the more complete signed URL when profile.url is compact',
  );

  const recoveredSpecific = normalize(compactPrimary, {
    fileFormat: 'xWT111',
    width: 720,
    height: 1280,
  });
  assertEqual(
    recoveredSpecific.url,
    specific.url,
    'specific mode should append the format to the recovered signed URL',
  );
}

main();
