import assert from 'node:assert/strict';
import test from 'node:test';
import {
  comparePerformanceResults,
  formatPerformanceComparison
} from './compare-e2e-performance.mjs';

function result(overrides = {}) {
  return {
    measurements: {
      fixtureVersion: 'large-e2e-v1',
      syntheticUsers: 2048,
      messages: 50_000,
      seedDurationMs: 10_000,
      memberListApiMs: 100,
      memberSearchApiMs: 100,
      membersPageMs: 500,
      roomPageMs: 1_000,
      realtimeDeliveryMs: 500,
      ...overrides
    }
  };
}

test('requires both the relative and absolute regression thresholds', () => {
  const comparison = comparePerformanceResults(
    result(),
    result({
      memberListApiMs: 250,
      membersPageMs: 750,
      roomPageMs: 1_300
    })
  );

  assert.equal(comparison.rows.find((row) => row.key === 'memberListApiMs').regressed, false);
  assert.equal(comparison.rows.find((row) => row.key === 'membersPageMs').regressed, true);
  assert.equal(comparison.rows.find((row) => row.key === 'roomPageMs').regressed, true);
  assert.equal(comparison.regressed, true);
});

test('reports improvements without failing the comparison', () => {
  const comparison = comparePerformanceResults(result(), result({ roomPageMs: 700 }));

  assert.equal(comparison.regressed, false);
  assert.match(formatPerformanceComparison(comparison), /-300ms \(-30\.0%\)/);
});

test('rejects comparisons made with different fixture sizes', () => {
  assert.throws(
    () => comparePerformanceResults(result(), result({ messages: 60_000 })),
    /Cannot compare different fixtures/
  );
});
