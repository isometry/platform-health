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
test('pathKey escapes %, / and # in that order and names an empty segment', () => {
  assert.equal(app.pathKey('', 'a/b'), 'a%2Fb');
  assert.equal(app.pathKey('', '100%'), '100%25');
  assert.equal(app.pathKey('', 'a#2'), 'a%232');
  assert.equal(app.pathKey('', ''), '%');
  assert.equal(app.pathKey('p', ''), 'p/%');
  assert.equal(app.pathKey('fluxcd', 'source-controller'), 'fluxcd/source-controller');
});

test('buildIndex keys same-named siblings with ordinals', () => {
  const index = app.buildIndex({
    components: [
      { name: 'db', type: 'tcp', status: 'HEALTHY' },
      { name: 'db', type: 'tcp', status: 'UNHEALTHY' },
      { name: 'db#2', type: 'tcp', status: 'HEALTHY' }
    ]
  });
  assert.deepEqual(index.get('/').childPaths, ['db', 'db#2', 'db%232']);
  assert.equal(index.get('db').node.status, 'HEALTHY');
  assert.equal(index.get('db#2').node.status, 'UNHEALTHY');
  assert.equal(index.size, 4);
});

test('buildIndex keeps an unnamed child as its own entry under the parent', () => {
  const index = app.buildIndex({
    components: [
      { name: 'sat', type: 'satellite', components: [{ type: 'tcp', status: 'HEALTHY' }] }
    ]
  });
  assert.deepEqual(index.get('sat').childPaths, ['sat/%']);
  assert.equal(index.get('sat/%').parentPath, 'sat');
  assert.equal(index.get('sat').parentPath, '/');
});

test('pathNames and pathCrumbs strip ordinals and un-escape', () => {
  assert.deepEqual(app.pathNames('a%2Fb/c%232#2/d%25'), ['a/b', 'c#2', 'd%']);
  assert.deepEqual(app.pathCrumbs('x/db#2'), [
    { key: 'x', name: 'x' },
    { key: 'x/db#2', name: 'db' }
  ]);
});
