/** Verify push ranges and migration handling with real Git tags and Sweet. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const root = path.resolve(import.meta.dirname, '..');
const script = path.join(root, 'tools/tag-chatto-release.sh');

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
  const tag = ci.split('\n  tag-chatto:\n')[1];
  const jobs = [...ci.split('jobs:\n')[1].matchAll(/^  ([\w-]+):$/gm)]
    .map((m) => m[1])
    .filter((name) => name !== 'tag-chatto');
  for (const job of jobs) assert.ok(tag.includes(`      - ${job}\n`), `Missing dependency: ${job}`);
  assert.ok(tag.includes('PUSH_BEFORE: ${{ github.event.before }}'));
  assert.ok(tag.includes('PUSH_AFTER: ${{ github.sha }}'));
  assert.ok(tag.includes("!contains(needs.*.result, 'failure')"));
  assert.ok(tag.includes("!contains(needs.*.result, 'cancelled')"));
});
