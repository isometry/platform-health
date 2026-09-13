import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const app = require('../assets/app.js');

test('effectiveStatus coerces an unknown enum number to a string', () => {
  assert.equal(app.effectiveStatus({ status: 4 }), '4');
  assert.equal(app.statusClass(app.effectiveStatus({ status: 4 })), 'unknown');
});

test('effectiveStatus maps missing and zero status to UNKNOWN', () => {
  assert.equal(app.effectiveStatus({}), 'UNKNOWN');
  assert.equal(app.effectiveStatus({ status: 0 }), 'UNKNOWN');
  assert.equal(app.effectiveStatus(null), 'UNKNOWN');
  assert.equal(app.effectiveStatus({ status: 'HEALTHY' }), 'HEALTHY');
});
