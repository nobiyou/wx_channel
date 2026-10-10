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

async function main() {
  const sandbox = loadDownloadModule();
  const normalize = sandbox.__wx_channels_normalize_video_download__;

  assertEqual(
    sandbox.__wx_channels_join_video_url_parts__('https://video.example.test/file', '&token=tok'),
    'https://video.example.test/file?token=tok',
    'URL token joining should add a question mark when the base has no query',
  );
  assertEqual(
    sandbox.__wx_channels_join_video_url_parts__('https://video.example.test/file?encfilekey=abc', '%26token%3Dtok%2Bplus'),
    'https://video.example.test/file?encfilekey=abc&token=tok%2Bplus',
    'URL token joining should decode query structure without changing escaped signature bytes',
  );
  assertEqual(
    sandbox.__wx_channels_join_video_url_parts__('https://video.example.test/file', 'https://cdn.example.test/file.mp4?token=tok'),
    'https://cdn.example.test/file.mp4?token=tok',
    'URL token joining should accept a complete replacement URL',
  );

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
    'a signed playable URL without fullUrl should use a concrete media spec',
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
    'primary label should expose the highest available media spec',
  );
  assertEqual(
    sandbox.__wx_channels_get_best_available_spec__({
      spec: [{ fileFormat: 'xWT111' }, { fileFormat: 'xWT112' }],
    }).fileFormat,
    'xWT111',
    'when bitrate metadata is absent, the upstream spec order should select the first rendition',
  );

  const noOriginalProfile = Object.assign({}, availableProfile, {
    url: '',
    originalUrl: '',
    urlToken: '',
    media: Object.assign({}, availableProfile.media, { url: '', urlToken: '' }),
  });
  const batchFallback = sandbox.__wx_channels_normalize_batch_video_download__(noOriginalProfile);
  assertEqual(batchFallback.mode, 'specific', 'batch mode should fall back when no URL is available');
  assertEqual(batchFallback.fileFormat, 'xWT111', 'batch fallback should use the highest available rendition');
  assertEqual(
    batchFallback.url,
    '',
    'batch fallback without a source URL should not invent a download URL',
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
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942',
    'original mode should keep the complete original-resource signature',
  );
  assertEqual(original.mode, 'original', 'original mode should be preserved');
  assertEqual(original.resolution, '1080x1920', 'original mode should preserve dimensions');

  const expectedSize = sandbox.__wx_channels_get_expected_video_size__(profile);
  assertEqual(expectedSize, 24 * 1024 * 1024, 'original mode should read the source size hint');

  const markedOriginal = normalize({
    url: profile.url + '&X-snsvideoflag=original',
    media: profile.media,
  }, null);
  assertEqual(
    markedOriginal.url,
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942',
    'original mode should preserve the signed query and remove only the legacy marker',
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
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&hy=SH&idx=1&m=compressed&uzid=7a1ac&token=tok456&basedata=CAMSBnhXVDEyOCJa&sign=sig789&web=1&extg=10f0000&svrbypass=AAuL%2FQsF&svrnonce=1778655942',
    'original mode should preserve a recovered signed URL',
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

  let backendRequest = null;
  sandbox.fetch = async function (url, options) {
    backendRequest = JSON.parse(options.body);
    return {
      ok: true,
      async json() { return { success: true }; },
    };
  };
  await sandbox.__wx_channels_download_via_backend__(profile, 'video', specific);
  assertEqual(
    backendRequest.videoUrl,
    specific.url,
    'backend fallback should receive the normalized URL rather than profile.url',
  );

  const backendRequests = [];
  sandbox.fetch = async function (url, options) {
    backendRequests.push({ url, options });
    return {
      ok: true,
      async json() { return { success: true }; },
    };
  };
  sandbox.__wx_channels_store__.profile = availableProfile;
  await sandbox.__wx_channels_handle_click_download__();
  assertEqual(backendRequests.length, 1, 'primary click should use the backend once without a page-direct request');
  assertEqual(
    backendRequests[0].url,
    '/__wx_channels_api/download_video',
    'original click should submit the download to the local backend',
  );
  const originalBackendRequest = JSON.parse(backendRequests[0].options.body);
  assertEqual(
    originalBackendRequest.videoUrl,
    specific.url,
    'primary click should submit the highest-quality spec URL to Gopeed',
  );
  assertEqual(
    originalBackendRequest.fileFormat,
    'xWT111',
    'primary click should send the selected media spec to the backend',
  );
}

main().catch(function (error) {
  console.error(error);
  process.exitCode = 1;
});
