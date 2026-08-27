import assert from 'node:assert/strict';
import test from 'node:test';
import {
  comparePerformanceResults,
  formatPerformanceComparison
} from './compare-e2e-performance.mjs';

function result(overrides = {}) {
  return {
    measurements: {
      measurementVersion: 'large-e2e-median-v1',
      sampleCount: 5,
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
      memberListApiMs: 130,
      memberSearchApiMs: 120,
      membersPageMs: 650,
      roomPageMs: 1_300
    })
  );

  assert.equal(comparison.rows.find((row) => row.key === 'memberListApiMs').regressed, true);
  assert.equal(comparison.rows.find((row) => row.key === 'memberSearchApiMs').regressed, false);
  assert.equal(comparison.rows.find((row) => row.key === 'membersPageMs').regressed, true);
  assert.equal(comparison.rows.find((row) => row.key === 'roomPageMs').regressed, true);
  assert.equal(comparison.regressed, true);
});

test('supports a global noise-floor override for unusually variable machines', () => {
  const comparison = comparePerformanceResults(result(), result({ memberListApiMs: 150 }), {
    minimumRegressionMs: 100
  });

  assert.equal(comparison.rows.find((row) => row.key === 'memberListApiMs').regressed, false);
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

test('rejects results produced by incompatible measurement semantics', () => {
  assert.throws(
    () =>
      comparePerformanceResults(result(), result({ measurementVersion: 'large-e2e-median-v2' })),
    /Cannot compare different fixtures: measurementVersion/
  );
  const legacy = result();
  delete legacy.measurements.sampleCount;
  assert.throws(() => comparePerformanceResults(legacy, result()), /without sampleCount/);
});
