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
      fullUrl: '',
    },
  };

  const original = normalize(profile, null);
  assertEqual(
    original.url,
    'https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456',
    'original mode should keep only encfilekey and token',
  );
  assertEqual(original.mode, 'original', 'original mode should be preserved');
  assertEqual(original.resolution, '1080x1920', 'original mode should preserve dimensions');

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
}

main();
