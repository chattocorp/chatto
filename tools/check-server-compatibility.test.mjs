/** Exercise label policy and the immutable Git revision boundary used by CI. */
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { checkThresholdChanges, readThresholds } from './check-server-compatibility.mjs';

const supportLabel = 'server-compatibility:full-support';
const floorLabel = 'server-compatibility:connection-floor';
const source = (support = '0.5.0-beta.16', floor = '0.5.0-beta.9') => `
export const MINIMUM_SUPPORTED_SERVER_VERSION = '${support}';
export const MINIMUM_CONNECTABLE_SERVER_VERSION = '${floor}';
`;

const featureSource = readFileSync(
  new URL('../packages/chatto-client/src/server/compatibility.ts', import.meta.url),
  'utf8'
);

test('derived full support preserves the threshold and requires labels when the maximum changes', () => {
  assert.deepEqual(checkThresholdChanges(source(), featureSource, []), []);
  const increased = featureSource.replace("threadViews: '0.5.0-beta.16'", "threadViews: '0.6.0'");
  assert.equal(checkThresholdChanges(featureSource, increased, []).length, 1);
  assert.deepEqual(checkThresholdChanges(featureSource, increased, [supportLabel]), []);
  const decreased = featureSource.replace(
    "threadViews: '0.5.0-beta.16'",
    "threadViews: '0.5.0-beta.10'"
  );
  assert.equal(checkThresholdChanges(featureSource, decreased, []).length, 1);
  const belowFloor = featureSource.replace("threadViews: '0.5.0-beta.16'", "threadViews: '0.4.0'");
  assert.equal(readThresholds(belowFloor).MINIMUM_SUPPORTED_SERVER_VERSION, '0.5.0-beta.9');
  const additional = featureSource.replace(
    "threadViews: '0.5.0-beta.16'",
    "threadViews: '0.5.0-beta.16', other: '0.7.0'"
  );
  assert.equal(readThresholds(additional).MINIMUM_SUPPORTED_SERVER_VERSION, '0.7.0');
});

test('unknown derivations and nonliteral feature requirements fail closed', () => {
  for (const invalid of [
    featureSource.replace('compare(minimum, required) < 0', 'compare(minimum, required) > 0'),
    featureSource.replace("threadViews: '0.5.0-beta.16'", 'threadViews: computeVersion()'),
    featureSource.replace(
      "threadViews: '0.5.0-beta.16'",
      "threadViews: '0.5.0-beta.16', threadViews: '0.6.0'"
    ),
    featureSource.replace("threadViews: '0.5.0-beta.16'", '...requirements')
  ]) {
    assert.throws(() => readThresholds(invalid));
  }
});

test('unchanged values allow no labels and unrelated labels', () => {
  for (const labels of [[], ['unrelated'], [supportLabel, floorLabel]]) {
    assert.deepEqual(checkThresholdChanges(source(), source(), labels), []);
  }
});

test('comments, quote style, and formatting do not change values', () => {
  const formatted = `/* export const MINIMUM_SUPPORTED_SERVER_VERSION = 'fake'; */
export const MINIMUM_SUPPORTED_SERVER_VERSION: string =\n "0.5.0-beta.16"
// A connection floor comment.
export const MINIMUM_CONNECTABLE_SERVER_VERSION = /* comment */ "0.5.0-beta.9"
`;
  assert.deepEqual(checkThresholdChanges(source(), formatted, []), []);
});

for (const [tier, changed, label, wrongLabel] of [
  ['support increase', source('0.5.0-beta.17'), supportLabel, floorLabel],
  ['support decrease', source('0.5.0-beta.15'), supportLabel, floorLabel],
  ['floor increase', source(undefined, '0.5.0-beta.10'), floorLabel, supportLabel],
  ['floor decrease', source(undefined, '0.5.0-beta.8'), floorLabel, supportLabel]
]) {
  test(`${tier} requires its exact label`, () => {
    for (const labels of [[], [wrongLabel], [`${label}-extra`], [label.toUpperCase()]]) {
      const errors = checkThresholdChanges(source(), changed, labels);
      assert.equal(errors.length, 1);
      assert.ok(errors[0].includes(label));
      assert.match(errors[0], /MINIMUM_.*_SERVER_VERSION: 0\.5\.0-beta\.\d+ -> 0\.5\.0-beta\.\d+/);
    }
    assert.deepEqual(checkThresholdChanges(source(), changed, [label]), []);
  });
}

test('changing both thresholds requires both labels', () => {
  const changed = source('0.6.0', '0.5.0');
  assert.equal(checkThresholdChanges(source(), changed, []).length, 2);
  assert.equal(checkThresholdChanges(source(), changed, [supportLabel]).length, 1);
  assert.equal(checkThresholdChanges(source(), changed, [floorLabel]).length, 1);
  assert.deepEqual(checkThresholdChanges(source(), changed, [supportLabel, floorLabel]), []);
});

for (const [name, invalid] of [
  ['missing', ''],
  ['duplicate', source() + source()],
  ['computed', source().replace("'0.5.0-beta.16'", "'0.5.0-' + 'beta.16'")],
  ['template', source().replace("'0.5.0-beta.16'", '`0.5.0-beta.16`')],
  ['mutable', source().replace('const', 'let')],
  ['unexported', source().replace('export ', '')],
  ['syntax error', source() + 'const = ;']
]) {
  test(`${name} source fails even with both labels`, () => {
    assert.throws(() => readThresholds(invalid));
    assert.throws(() => checkThresholdChanges(source(), invalid, [supportLabel, floorLabel]));
    assert.throws(() => checkThresholdChanges(invalid, source(), [supportLabel, floorLabel]));
  });
}

test('CLI uses the merge base and head, and label addition/removal changes its exit status', () => {
  const root = fileURLToPath(new URL('../', import.meta.url));
  mkdirSync(path.join(root, '.context'), { recursive: true });
  const cwd = mkdtempSync(path.join(root, '.context/compatibility-test-'));
  const git = (...args) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();
  const target = path.join(cwd, 'packages/chatto-client/src/server/compatibility.ts');
  try {
    git('init', '--quiet');
    git('config', 'user.email', 'test@example.invalid');
    git('config', 'user.name', 'Test');
    git('config', 'commit.gpgsign', 'false');
    mkdirSync(path.dirname(target), { recursive: true });
    const commit = (content) => {
      writeFileSync(target, content);
      git('add', '.');
      git('commit', '--quiet', '-m', 'test: fixture');
      return git('rev-parse', 'HEAD');
    };
    const ancestor = commit(source());
    const head = commit(source('0.6.0'));
    git('checkout', '--quiet', '--detach', ancestor);
    // The target branch moves independently to the same values as the PR.
    // Comparing its tip instead of the merge base would miss the PR change.
    writeFileSync(path.join(cwd, 'base-only'), 'base');
    const base = commit(source('0.6.0'));
    // Working tree contents must not participate in the comparison either.
    writeFileSync(target, 'invalid working tree');
    const eventPath = path.join(cwd, 'event.json');
    const run = (labels) => {
      writeFileSync(
        eventPath,
        JSON.stringify({
          pull_request: {
            base: { sha: base },
            head: { sha: head },
            labels: labels.map((name) => ({ name }))
          }
        })
      );
      return spawnSync(
        process.execPath,
        [path.join(root, 'tools/check-server-compatibility.mjs')],
        {
          cwd,
          encoding: 'utf8',
          env: { ...process.env, GITHUB_EVENT_PATH: eventPath }
        }
      );
    };
    const missing = run([]);
    assert.equal(missing.status, 1, missing.stderr);
    assert.match(missing.stderr, /0\.5\.0-beta\.16 -> 0\.6\.0/);
    assert.equal(run([supportLabel]).status, 0);
    assert.equal(run([]).status, 1);
  } finally {
    rmSync(cwd, { recursive: true, force: true });
  }
});
