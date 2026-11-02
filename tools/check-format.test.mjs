/** Exercise the formatting gate against real Git histories and Prettier configurations. */
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const script = fileURLToPath(new URL('./check-format.mjs', import.meta.url));

/** Each fixture owns an isolated repository under the worktree's scratch directory. */
function fixture(t) {
  mkdirSync(path.join(root, '.context'), { recursive: true });
  const directory = mkdtempSync(path.join(root, '.context/format-test-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync('git', args, {
      cwd: directory,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe']
    });
  const write = (file, content) => {
    mkdirSync(path.dirname(path.join(directory, file)), { recursive: true });
    writeFileSync(path.join(directory, file), content);
  };
  git('init', '-b', 'main');
  git('config', 'user.name', 'Formatting test');
  git('config', 'user.email', 'format-test@example.invalid');
  write('.prettierrc.json', '{"singleQuote":true,"plugins":[]}\n');
  write('.prettierignore', 'ignored/\n');
  write('old.js', 'const old=1');
  write('deleted.js', 'const deleted=1');
  git('add', '.');
  git('commit', '-qm', 'test: create baseline');
  git('branch', 'baseline');
  const run = (base = 'baseline', separator = false) =>
    spawnSync(process.execPath, [script, ...(separator ? ['--'] : []), base], {
      cwd: directory,
      encoding: 'utf8'
    });
  return { directory, git, write, run };
}

test('an empty diff does not check unformatted baseline files', (t) => {
  const { run } = fixture(t);
  const result = run();
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /0 changed files checked/);
  assert.equal(run('baseline', true).status, 0);
});

test('checks staged and unstaged files literally, and uses nested configuration', (t) => {
  const { write, git, run } = fixture(t);
  write('nested/.prettierrc.json', '{"singleQuote":false,"plugins":[]}\n');
  write('nested/[route] with spaces.js', 'const value = "formatted";\n');
  write('line\nbreak.js', 'const value=1');
  git('add', '.');
  write('old.js', 'const old=2');
  const failed = run();
  assert.equal(failed.status, 1);
  assert.match(failed.stderr, /Formatting required: "old.js"/);
  assert.match(failed.stderr, /Formatting required: "line\\nbreak.js"/);
  assert.doesNotMatch(failed.stderr, /Formatting required: "nested\/\[route\]/);
  write('old.js', 'const old = 2;\n');
  write('line\nbreak.js', 'const value = 1;\n');
  write('nested/.prettierrc.json', '{ "singleQuote": false, "plugins": [] }\n');
  const passed = run();
  assert.equal(passed.status, 0, passed.stderr);
});

test('skips deleted, ignored, unsupported, and symlink files but checks rename destinations', (t) => {
  const { directory, write, git, run } = fixture(t);
  git('rm', 'deleted.js');
  git('mv', 'old.js', 'renamed.js');
  write('ignored/bad.js', 'not valid javascript');
  write('unsupported.go', 'not valid Go');
  symlinkSync('missing.js', path.join(directory, 'symlink.js'));
  git('add', '.');
  const failed = run();
  assert.equal(failed.status, 1);
  assert.match(failed.stderr, /Formatting required: "renamed.js"/);
  assert.match(failed.stdout, /1 changed files checked/);
  write('renamed.js', 'const old = 1;\n');
  assert.equal(run().status, 0);
});

test('uses the merge base when the target branch advances independently', (t) => {
  const { write, git, run } = fixture(t);
  git('switch', '-c', 'feature');
  write('feature.js', 'const feature = true;\n');
  git('add', '.');
  git('commit', '-qm', 'test: add feature');
  git('switch', 'baseline');
  write('target.js', 'const target=1');
  git('add', '.');
  git('commit', '-qm', 'test: advance target');
  git('switch', 'feature');
  const result = run();
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /1 changed files checked/);
});

test('invalid bases and parser errors fail the check', (t) => {
  const { write, git, run } = fixture(t);
  assert.notEqual(run('missing-ref').status, 0);
  assert.notEqual(run('--help').status, 0);
  write('broken.js', 'const = ;');
  git('add', '.');
  assert.notEqual(run().status, 0);
});
