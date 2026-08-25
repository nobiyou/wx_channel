const fs = require('fs');
const path = require('path');
const vm = require('vm');

function loadKeepAlive() {
  const source = fs.readFileSync(path.resolve(__dirname, 'keep_alive.js'), 'utf8');
  const intervals = [];
  let reloads = 0;
  const sandbox = {
    console: { log() {}, warn() {}, error() {} },
    window: {},
    document: {
      readyState: 'complete',
      hidden: false,
      body: { offsetHeight: 0, appendChild() {} },
      addEventListener() {},
      createElement() {
        return { id: '', style: {}, setAttribute() {}, remove() {} };
      },
      getElementById() { return null; },
    },
    navigator: {},
    location: { reload() { reloads += 1; } },
    sessionStorage: { getItem() { return null; }, setItem() {} },
    Date,
    JSON,
    setTimeout,
    clearTimeout,
    setInterval(fn, delay) {
      intervals.push({ fn, delay });
      return intervals.length;
    },
    clearInterval() {},
    CustomEvent: function CustomEvent(type, init) {
      return { type, detail: init ? init.detail : undefined };
    },
    MouseEvent: function MouseEvent() {},
  };
  sandbox.window = sandbox;
  sandbox.window.addEventListener = function () {};
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox, { filename: 'keep_alive.js' });
  return { keepAlive: sandbox.window.__wx_keep_alive, intervals, getReloads: () => reloads };
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function main() {
  const env = loadKeepAlive();
  const delays = env.intervals.map((entry) => entry.delay);
  assert(!delays.includes(15 * 60 * 1000), 'automatic 15-minute refresh timer must be removed');
  assert(typeof env.keepAlive.performRefresh === 'function', 'manual/server refresh must remain available');
  assert(typeof env.keepAlive.lockRefresh === 'function' && typeof env.keepAlive.unlockRefresh === 'function', 'refresh lock API must remain available');

  env.keepAlive.lockRefresh('export', 'test');
  env.keepAlive.performRefresh('locked');
  assert(env.getReloads() === 0, 'refresh lock must block refresh');
  env.keepAlive.unlockRefresh('export');
  env.keepAlive.performRefresh('unlocked');
  assert(env.getReloads() === 1, 'unlocked manual refresh must reload');
}

main();
console.log('keep_alive lifecycle tests passed');
