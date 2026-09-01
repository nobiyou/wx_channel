const fs = require('fs');
const path = require('path');
const vm = require('vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

function loadDownloadModule() {
  const source = fs.readFileSync(path.resolve(__dirname, 'download.js'), 'utf8');
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    window: {},
    Date,
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'download.js' });
  return sandbox;
}

test('download filename cleaning uses the dynamic title budget', () => {
  const sandbox = loadDownloadModule();
  const title = 'a'.repeat(179) + '😀z';
  const filename = sandbox.__wx_channels_prepare_download_filename__(title, '', '.mp4');

  assert.equal(filename, 'a'.repeat(179));
  assert.equal(filename.length, 179);
});

test('download filename cleaning preserves a quality suffix', () => {
  const sandbox = loadDownloadModule();
  const suffix = '_xWT111_1920x1080';
  const title = '长标题'.repeat(100) + suffix;
  const filename = sandbox.__wx_channels_prepare_download_filename__(title, suffix, '.mp4');

  assert.ok(filename.endsWith(suffix));
  assert.ok(filename.startsWith('长标题'));
  assert.ok(filename.length <= 180 + suffix.length);
});

test('download filename cleaning applies the same illegal character rules', () => {
  const sandbox = loadDownloadModule();
  const filename = sandbox.__wx_channels_prepare_download_filename__(
    '<em>标题</em>:测试/文件\\名称?*',
    '',
    '.mp4'
  );

  assert.equal(filename, '标题_测试_文件_名称__');
});
