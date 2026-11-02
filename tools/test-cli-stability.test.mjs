/** Check the stability runner's budget and failure handling without running Go tests. */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const script = path.join(root, 'tools/test-cli-stability.sh');

/** Replace only the Go executable; run the real shell entry point. */
function runner(t) {
  mkdirSync(path.join(root, '.context'), { recursive: true });
  const directory = mkdtempSync(path.join(root, '.context/stability-runner-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  writeFileSync(
    path.join(directory, 'go'),
    '#!/bin/sh\nprintf "GO_ARG:%s\\n" "$@"\nexit "${FAKE_GO_STATUS:-0}"\n',
    { mode: 0o755 }
  );
  return (overrides = {}) =>
    spawnSync('bash', [script], {
      cwd: path.join(root, 'cli'),
      encoding: 'utf8',
      env: {
        ...process.env,
        PATH: `${directory}${path.delimiter}${process.env.PATH}`,
        CHATTO_GO_STABILITY_COUNT: '',
        CHATTO_GO_TEST_SHUFFLE: '',
        FAKE_GO_STATUS: '0',
        ...overrides
      }
    });
}

test('keeps all repetitions in one serial race run with a scaled package timeout', (t) => {
  const run = runner(t);
  for (const [count, expectedCount, timeout] of [
    ['', '3', '45m'],
    ['1', '1', '15m'],
    ['5', '5', '75m']
  ]) {
    const result = run({ CHATTO_GO_STABILITY_COUNT: count });
    assert.equal(result.status, 0, result.stderr);
    const args = result.stdout.split('\n').filter((line) => line.startsWith('GO_ARG:'));
    assert.deepEqual(
      args.map((line) => line.slice(7)),
      [
        'test',
        '-trimpath',
        '-race',
        '-tags',
        'test_endpoints',
        '-p',
        '1',
        '-parallel',
        '1',
        '-shuffle=on',
        `-count=${expectedCount}`,
        `-timeout=${timeout}`,
        './internal/testutil',
        './internal/core',
        './internal/connectapi',
        './internal/http_server',
        './internal/mcpserver'
      ]
    );
  }
});

test('preserves an explicit shuffle seed and the Go failure status', (t) => {
  const result = runner(t)({ CHATTO_GO_TEST_SHUFFLE: '1793615000972846000', FAKE_GO_STATUS: '7' });
  assert.equal(result.status, 7);
  assert.match(result.stdout, /GO_ARG:-shuffle=1793615000972846000/);
});

test('rejects counts that could skip tests or produce an invalid budget', (t) => {
  const run = runner(t);
  for (const count of ['0', '-1', '1.5', 'abc', '03', '1+2', '10000000']) {
    const result = run({ CHATTO_GO_STABILITY_COUNT: count });
    assert.equal(result.status, 2, count);
    assert.match(result.stderr, /CHATTO_GO_STABILITY_COUNT must be a positive integer/);
    assert.doesNotMatch(result.stdout, /GO_ARG:/);
  }
});
