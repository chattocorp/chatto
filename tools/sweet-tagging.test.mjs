/** Verify push ranges and migration handling with real Git tags and Sweet. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const root = path.resolve(import.meta.dirname, '..');
const script = path.join(root, 'tools/tag-releases.sh');

/** Make an isolated repository and bare remote; never use the project remote. */
function fixture(t) {
  mkdirSync(path.join(root, '.context'), { recursive: true });
  const dir = mkdtempSync(path.join(root, '.context/sweet-tag-test-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const repo = path.join(dir, 'repo');
  mkdirSync(repo);
  const git = (...args) =>
    execFileSync('git', args, {
      cwd: repo,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe']
    }).trim();
  git('init');
  git('config', 'user.name', 'Fixture');
  git('config', 'user.email', 'fixture@example.invalid');
  git('init', '--bare', path.join(dir, 'remote'));
  git('remote', 'add', 'origin', path.join(dir, 'remote'));
  function commit(file, content) {
    writeFileSync(path.join(repo, file), content);
    git('add', '.');
    git('commit', '-m', 'fixture');
    return git('rev-parse', 'HEAD');
  }
  const baseline = () => commit('.sweet.yml', 'version: 0.5.0\ntag-prefix: v\n');
  function tag(before, after) {
    execFileSync('bash', [script], {
      cwd: repo,
      env: { ...process.env, PUSH_BEFORE: before, PUSH_AFTER: after, RUNNER_TEMP: dir },
      stdio: 'pipe'
    });
  }
  return { git, commit, baseline, tag };
}

test('full push tags a release before the tip and supports retries', (t) => {
  const f = fixture(t);
  const before = f.baseline();
  const release = f.commit('.sweet.yml', 'version: 0.5.1\ntag-prefix: v\n');
  const after = f.commit('later.txt', 'Later non-release change');
  f.tag(before, after);
  assert.equal(f.git('rev-parse', 'v0.5.1^{}'), release);
  assert.match(f.git('ls-remote', 'origin', 'refs/tags/v0.5.1'), /refs\/tags\/v0.5.1/);
  f.tag(before, after);
  assert.equal(f.git('tag', '--list'), 'v0.5.1');
});

test('migration preserves the historical tag and includes later releases in the push', (t) => {
  const f = fixture(t);
  const before = f.commit('old.txt', 'Existing release');
  f.git('tag', 'v0.5.0');
  const migration = f.baseline();
  f.tag(before, migration);
  assert.equal(f.git('rev-parse', 'v0.5.0'), before);
  const release = f.commit('.sweet.yml', 'version: 0.5.1\ntag-prefix: v\n');
  const after = f.commit('later.txt', 'Later change');
  f.tag(before, after);
  assert.equal(f.git('rev-parse', 'v0.5.1^{}'), release);
  assert.equal(f.git('rev-parse', 'v0.5.0'), before);
});

test('CI waits for every other job and uses the push boundaries', () => {
  const ci = readFileSync(path.join(root, '.github/workflows/ci.yml'), 'utf8');
  const tag = ci.split('\n  tag-releases:\n')[1];
  const jobs = [...ci.split('jobs:\n')[1].matchAll(/^  ([\w-]+):$/gm)]
    .map((m) => m[1])
    .filter((name) => name !== 'tag-releases');
  for (const job of jobs) assert.ok(tag.includes(`      - ${job}\n`), `Missing dependency: ${job}`);
  assert.ok(tag.includes('PUSH_BEFORE: ${{ github.event.before }}'));
  assert.ok(tag.includes('PUSH_AFTER: ${{ github.sha }}'));
  assert.ok(tag.includes("!contains(needs.*.result, 'failure')"));
  assert.ok(tag.includes("!contains(needs.*.result, 'cancelled')"));
});

test('all release units preserve migration tags and bump independently configured version files', (t) => {
  const f = fixture(t);
  const before = f.commit('old.txt', 'Historical release');
  f.git('tag', 'v0.5.0');
  f.git('tag', 'authling/v0.1.0-alpha.3');
  f.git('tag', 'runling/v0.7.0');
  f.git('tag', 'translatto/v0.2.0');
  f.git('tag', 'pkg/events/v0.1.0');
  const repo = f.git('rev-parse', '--show-toplevel');
  // Keep the migration baseline fixed when a release PR updates the source versions.
  // Read the real manifests so changes to version-file configuration remain covered.
  const units = {
    '.': '0.5.0',
    authling: '0.1.0-alpha.3',
    'packages/runling': '0.7.0',
    'packages/translatto': '0.2.0',
    'pkg/appconfig': '0.0.0',
    'pkg/datacrypto': '0.0.0',
    'pkg/events': '0.1.0',
    'pkg/jetstreamutil': '0.0.0',
    'pkg/natsruntime': '0.0.0'
  };
  const sourceVersions = {};
  for (const [unit, version] of Object.entries(units)) {
    const manifest = readFileSync(path.join(root, unit, '.sweet.yml'), 'utf8');
    sourceVersions[unit] = manifest.match(/^version: (.+)$/m)[1];
    mkdirSync(path.join(repo, unit), { recursive: true });
    writeFileSync(
      path.join(repo, unit, '.sweet.yml'),
      manifest.replace(/^version: .+$/m, `version: ${version}`)
    );
  }
  for (const file of [
    '.sweet.config.yml',
    'cli/version.go',
    'authling/version.go',
    ...['packages/runling', 'packages/translatto'].flatMap((p) => [
      `${p}/package.json`,
      `${p}/version.txt`
    ]),
    ...['packages/chatto-client', 'apps/frontend', 'apps/desktop'].map((p) => `${p}/package.json`)
  ]) {
    mkdirSync(path.dirname(path.join(repo, file)), { recursive: true });
    const unit = Object.keys(units).find((unit) => file.startsWith(`${unit}/`)) ?? '.';
    writeFileSync(
      path.join(repo, file),
      readFileSync(path.join(root, file), 'utf8').replaceAll(sourceVersions[unit], units[unit])
    );
  }
  const migration = f.commit('migration.txt', 'Baseline');
  f.tag(before, migration);
  assert.equal(f.git('tag', '--list').split('\n').length, 5);
  for (const unit of Object.keys(units)) {
    execFileSync('mise', ['x', '--', 'sweet', 'change', '--type', 'fix', '--stdin', unit], {
      cwd: repo,
      input: 'Fix a release fixture.\n',
      stdio: ['pipe', 'pipe', 'pipe']
    });
  }
  execFileSync('mise', ['x', '--', 'sweet', 'bump', '--all'], { cwd: repo, stdio: 'pipe' });
  const released = f.commit('release.txt', 'Prepared all packages');
  const after = f.commit('later.txt', 'After release');
  f.tag(migration, after);
  for (const tag of [
    'v0.5.1',
    'authling/v0.1.1',
    'runling/v0.7.1',
    'translatto/v0.2.1',
    ...['appconfig', 'datacrypto', 'jetstreamutil', 'natsruntime'].map((p) => `pkg/${p}/v0.0.1`),
    'pkg/events/v0.1.1'
  ]) {
    assert.equal(f.git('rev-parse', `${tag}^{}`), released);
  }
  assert.match(readFileSync(path.join(repo, 'authling/version.go'), 'utf8'), /"0\.1\.1"/);
  assert.equal(
    readFileSync(path.join(repo, 'packages/runling/version.txt'), 'utf8').trim(),
    '0.7.1'
  );
  assert.equal(
    JSON.parse(readFileSync(path.join(repo, 'apps/desktop/package.json'))).version,
    '0.5.1'
  );
});

test('Authling image metadata checks its Sweet manifest against the source and tag', (t) => {
  const f = fixture(t);
  f.commit('old.txt', 'Fixture');
  const repo = f.git('rev-parse', '--show-toplevel');
  mkdirSync(path.join(repo, 'authling'));
  writeFileSync(path.join(repo, 'authling/version.go'), 'package main\nconst Version = "0.1.1"\n');
  writeFileSync(path.join(repo, 'authling/.sweet.yml'), 'version: 0.1.1\n');
  const workflow = readFileSync(
    path.join(root, '.github/workflows/build-authling-image.yml'),
    'utf8'
  );
  const step = workflow
    .split('      - name: Resolve image metadata\n')[1]
    .split('\n      - name:')[0];
  const script = step
    .split('        run: |\n')[1]
    .split('\n')
    .filter((line) => line.startsWith('          '))
    .map((line) => line.slice(10))
    .join('\n');
  const output = path.join(repo, 'output');
  const run = () =>
    execFileSync('python3', ['-c', script], {
      cwd: repo,
      env: { ...process.env, GITHUB_REF: 'refs/tags/authling/v0.1.1', GITHUB_OUTPUT: output },
      stdio: 'pipe'
    });
  run();
  assert.match(readFileSync(output, 'utf8'), /tag=0\.1\.1/);
  writeFileSync(path.join(repo, 'authling/.sweet.yml'), 'version: 0.1.2\n');
  assert.throws(run, (error) => error.stderr.toString().includes('must agree'));
});
