import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

// theme.js runs synchronously in <head>; a minimal DOM stub lets it load
// here. Nothing is stored, so every read falls back to its default.
function loadTheme(innerWidth) {
  const rootProps = {};
  globalThis.window = { innerWidth: innerWidth };
  globalThis.document = {
    documentElement: {
      setAttribute() {},
      removeAttribute() {},
      style: { setProperty(name, value) { rootProps[name] = value; } }
    },
    readyState: 'loading',
    addEventListener() {},
    getElementById() { return null; }
  };
  delete require.cache[require.resolve('../assets/theme.js')];
  require('../assets/theme.js');
  return rootProps;
}

test('theme.js publishes the rail width policy for app.js', () => {
  loadTheme(1000);
  const rail = globalThis.window.phRail;
  assert.equal(rail.key, 'ph-ui-rail-width');
  assert.equal(rail.defaultWidth, 280);
  assert.equal(rail.min, 200);
  assert.equal(rail.maxRatio, 0.45);
  assert.equal(rail.clamp(100), 200);
  assert.equal(rail.clamp(300), 300);
  assert.equal(rail.clamp(900), 450);
});

test('theme.js restores the default rail width before paint', () => {
  const rootProps = loadTheme(1000);
  assert.equal(rootProps['--rail-width'], '280px');
});
