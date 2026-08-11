import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const comparedMetrics = [
  { key: 'seedDurationMs', label: 'Fixture generation', minimumRegressionMs: 1_000 },
  { key: 'memberListApiMs', label: 'Member list API', minimumRegressionMs: 25 },
  { key: 'memberSearchApiMs', label: 'Member search API', minimumRegressionMs: 25 },
  { key: 'membersPageMs', label: 'Members page', minimumRegressionMs: 100 },
  { key: 'roomPageMs', label: 'Large timeline page', minimumRegressionMs: 200 },
  { key: 'realtimeDeliveryMs', label: 'Realtime delivery', minimumRegressionMs: 100 }
];

export function comparePerformanceResults(baseResult, candidateResult, options = {}) {
  const maximumRegressionPercent = options.maximumRegressionPercent ?? 25;
  const base = readMeasurements(baseResult, 'base');
  const candidate = readMeasurements(candidateResult, 'candidate');

  for (const field of [
    'measurementVersion',
    'sampleCount',
    'fixtureVersion',
    'syntheticUsers',
    'messages'
  ]) {
    if (base[field] === undefined || candidate[field] === undefined) {
      throw new Error(`Cannot compare results without ${field}`);
    }
    if (base[field] !== candidate[field]) {
      throw new Error(
        `Cannot compare different fixtures: ${field} is ${JSON.stringify(base[field])} on the base and ${JSON.stringify(candidate[field])} on the candidate`
      );
    }
  }

  const rows = comparedMetrics.map((metric) => {
    const { key, label } = metric;
    const minimumRegressionMs = options.minimumRegressionMs ?? metric.minimumRegressionMs;
    const baseMs = positiveMeasurement(base, key, 'base');
    const candidateMs = positiveMeasurement(candidate, key, 'candidate');
    const deltaMs = candidateMs - baseMs;
    const deltaPercent = (deltaMs / baseMs) * 100;
    const regressed = deltaMs > minimumRegressionMs && deltaPercent > maximumRegressionPercent;
    return {
      key,
      label,
      baseMs,
      candidateMs,
      deltaMs,
      deltaPercent,
      minimumRegressionMs,
      regressed
    };
  });

  return {
    fixture: {
      measurementVersion: candidate.measurementVersion,
      sampleCount: candidate.sampleCount,
      version: candidate.fixtureVersion,
      syntheticUsers: candidate.syntheticUsers,
      messages: candidate.messages
    },
    maximumRegressionPercent,
    rows,
    regressed: rows.some((row) => row.regressed)
  };
}

export function formatPerformanceComparison(comparison) {
  const lines = [
    '## Large-server E2E performance comparison',
    '',
    `Fixture: ${comparison.fixture.syntheticUsers} users, ${comparison.fixture.messages} messages; ${comparison.fixture.sampleCount}-sample medians (${comparison.fixture.version}, ${comparison.fixture.measurementVersion})`,
    '',
    `Regression budget: more than ${comparison.maximumRegressionPercent}% and more than the metric's noise floor.`,
    '',
    '| Measurement | Base (ms) | Candidate (ms) | Change | Noise floor | Result |',
    '| --- | ---: | ---: | ---: | ---: | --- |'
  ];
  for (const row of comparison.rows) {
    const change = `${signedInteger(row.deltaMs)}ms (${signedDecimal(row.deltaPercent)}%)`;
    lines.push(
      `| ${row.label} | ${Math.round(row.baseMs)} | ${Math.round(row.candidateMs)} | ${change} | ${row.minimumRegressionMs}ms | ${row.regressed ? '❌ regression' : '✅ within budget'} |`
    );
  }
  lines.push('');
  lines.push(
    comparison.regressed
      ? '❌ One or more performance measurements exceeded the relative regression budget.'
      : '✅ No performance measurement exceeded the relative regression budget.'
  );
  return `${lines.join('\n')}\n`;
}

function readMeasurements(result, name) {
  const measurements = result?.measurements;
  if (!measurements || typeof measurements !== 'object') {
    throw new Error(`${name} result does not contain a measurements object`);
  }
  return measurements;
}

function positiveMeasurement(measurements, key, name) {
  const value = measurements[key];
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} measurement ${key} must be a positive finite number`);
  }
  return value;
}

function signedInteger(value) {
  const rounded = Math.round(value);
  return `${rounded >= 0 ? '+' : ''}${rounded}`;
}

function signedDecimal(value) {
  const rounded = value.toFixed(1);
  return `${value >= 0 ? '+' : ''}${rounded}`;
}

function positiveEnvironment(name, fallback) {
  const raw = process.env[name];
  if (!raw) return fallback;
  const value = Number(raw);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be a positive number, got ${JSON.stringify(raw)}`);
  }
  return value;
}

function optionalPositiveEnvironment(name) {
  const raw = process.env[name];
  if (!raw) return undefined;
  return positiveEnvironment(name, undefined);
}

function main() {
  const [basePath, candidatePath] = process.argv.slice(2);
  if (!basePath || !candidatePath) {
    throw new Error('usage: node compare-e2e-performance.mjs BASE.json CANDIDATE.json');
  }
  const comparison = comparePerformanceResults(
    JSON.parse(readFileSync(basePath, 'utf8')),
    JSON.parse(readFileSync(candidatePath, 'utf8')),
    {
      maximumRegressionPercent: positiveEnvironment('CHATTO_E2E_PERF_MAX_REGRESSION_PERCENT', 25),
      minimumRegressionMs: optionalPositiveEnvironment('CHATTO_E2E_PERF_MIN_REGRESSION_MS')
    }
  );
  process.stdout.write(formatPerformanceComparison(comparison));
  if (comparison.regressed) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main();
}
