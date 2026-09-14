import assert from 'node:assert/strict';
import test from 'node:test';
import { shardTestList } from './run-e2e-shard.mjs';

function spec(title, line, file = 'example.test.ts', projects = ['']) {
  return { title, line, file, tests: projects.map((projectName) => ({ projectName })) };
}

const report = {
  suites: [
    {
      title: 'example.test.ts',
      specs: [spec('first', 1), spec('last', 30)],
      suites: [{ title: 'nested', specs: [spec('middle A', 10), spec('middle B', 20)] }]
    }
  ]
};

test('interleaves source order and preserves nested test titles', () => {
  assert.deepEqual(shardTestList(report, 1, 2), [
    '[] › example.test.ts › first',
    '[] › example.test.ts › nested › middle B'
  ]);
  assert.deepEqual(shardTestList(report, 2, 2), [
    '[] › example.test.ts › nested › middle A',
    '[] › example.test.ts › last'
  ]);
});

test('selects every test exactly once, including generated tests and projects', () => {
  const generated = {
    suites: [
      {
        specs: Array.from({ length: 11 }, (_, index) =>
          spec(`case ${index}`, 1, 'generated.test.ts', ['desktop', 'mobile'])
        )
      }
    ]
  };
  const all = shardTestList(generated, 1, 1);
  const shards = Array.from({ length: 4 }, (_, index) => shardTestList(generated, index + 1, 4));
  assert.deepEqual(
    shards.map((shard) => shard.length),
    [6, 6, 5, 5]
  );
  assert.equal(new Set(shards.flat()).size, all.length);
  assert.deepEqual(shards.flat().sort(), all.sort());
});

test('rejects invalid partitions and collection errors instead of silently dropping tests', () => {
  for (const [current, total] of [
    [0, 4],
    [5, 4],
    [1, 0],
    [1.5, 4],
    [1, 5]
  ]) {
    assert.throws(() => shardTestList(report, current, total));
  }
  assert.throws(() => shardTestList({ ...report, errors: [{}] }, 1, 2));
  for (const title of ['line\nbreak', 'separator › title', ' whitespace ']) {
    assert.throws(() => shardTestList({ suites: [{ specs: [spec(title, 1)] }] }, 1, 1));
  }
  assert.throws(() =>
    shardTestList({ suites: [{ specs: [spec('duplicate', 1), spec('duplicate', 2)] }] }, 1, 1)
  );
});
