import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/**
 * Spread independent tests across shards in source order. Native sharding uses
 * contiguous groups, which puts many expensive threading tests on one runner.
 * Use Playwright's collected suite so new tests are included automatically.
 */
export function shardTestList(report, current, total) {
  if (
    !Number.isSafeInteger(total) ||
    total < 1 ||
    !Number.isSafeInteger(current) ||
    current < 1 ||
    current > total
  ) {
    throw new Error('Shard must be current/total with 1 <= current <= total.');
  }
  if (report.errors?.length) throw new Error('Cannot shard a suite with collection errors.');
  const tests = [];
  function visit(suite, parents) {
    for (const spec of suite.specs ?? []) {
      for (const test of spec.tests) {
        const titles = [...parents, spec.title];
        if (titles.some((title) => /[\r\n›]/u.test(title) || title.trim() !== title)) {
          throw new Error('Test titles must be representable in a Playwright test list.');
        }
        tests.push({
          file: spec.file,
          line: spec.line,
          list: `[${test.projectName}] › ${spec.file} › ${titles.join(' › ')}`
        });
      }
    }
    for (const child of suite.suites ?? []) visit(child, [...parents, child.title]);
  }
  for (const file of report.suites) visit(file, []);
  tests.sort((a, b) =>
    a.file < b.file
      ? -1
      : a.file > b.file
        ? 1
        : a.line - b.line || (a.list < b.list ? -1 : a.list > b.list ? 1 : 0)
  );
  if (tests.length < total) throw new Error('Each shard must contain at least one test.');
  if (new Set(tests.map((test) => test.list)).size !== tests.length) {
    throw new Error('Duplicate test list entries would run a test on multiple shards.');
  }
  return tests.filter((_, index) => index % total === current - 1).map((test) => test.list);
}

function main() {
  const [shard, ...args] = process.argv.slice(2);
  if (
    !/^\d+\/\d+$/u.test(shard ?? '') ||
    args.some((arg) => arg.startsWith('--shard') || arg.startsWith('--test-list'))
  ) {
    throw new Error('Usage: node scripts/run-e2e-shard.mjs current/total [Playwright options]');
  }
  const [current, total] = shard.split('/').map(Number);
  const directory = mkdtempSync(path.join(tmpdir(), 'chatto-e2e-shard-'));
  const require = createRequire(import.meta.url);
  const cli = require.resolve('@playwright/test/cli');
  const cwd = fileURLToPath(new URL('..', import.meta.url));
  function run(options, env = process.env, quiet = false) {
    const result = spawnSync(process.execPath, [cli, 'test', ...options], {
      cwd,
      env,
      stdio: quiet ? ['ignore', 'pipe', 'inherit'] : 'inherit',
      maxBuffer: 4 * 1024 * 1024
    });
    if (result.error) throw result.error;
    if (result.status !== 0)
      throw new Error(`Playwright exited with ${result.status ?? result.signal}.`);
  }
  try {
    const reportPath = path.join(directory, 'collection.json');
    run(
      [...args, '--list', '--reporter=json'],
      { ...process.env, PLAYWRIGHT_JSON_OUTPUT_FILE: reportPath },
      true
    );
    const report = JSON.parse(readFileSync(reportPath, 'utf8'));
    const list = shardTestList(report, current, total);
    const listPath = path.join(directory, 'tests.txt');
    writeFileSync(listPath, `${list.join('\n')}\n`);
    console.log(`Round-robin shard ${shard}: ${list.length} tests`);
    run([...args, `--test-list=${listPath}`]);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
