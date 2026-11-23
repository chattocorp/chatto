// Tests CI job selection: the path filters in .github/workflows/ci.yml together
// with the combination rules in tools/ci-select-jobs.mjs.
//
// A pull request that changes the workflow selects all jobs, so its own CI run
// cannot show a filter that skips too much. These tests are that check.

import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { runInNewContext } from 'node:vm';
import picomatch from 'picomatch';
import { parse } from 'yaml';
import { desktopMatrix, jobOutputs, selectJobs } from './ci-select-jobs.mjs';

const root = path.resolve(import.meta.dirname, '..');
const workflowSource = readFileSync(path.join(root, '.github/workflows/ci.yml'), 'utf8');
const workflow = parse(workflowSource);
const filterStep = workflow.jobs.changes.steps.find((step) => step.id === 'filter');

// Match as dorny/paths-filter does with `predicate-quantifier: some-with-excludes`:
// a file matches when one pattern includes it and no negated pattern excludes it.
assert.equal(filterStep.with['predicate-quantifier'], 'some-with-excludes');
const filters = Object.fromEntries(
  Object.entries(parse(filterStep.with.filters)).map(([name, patterns]) => {
    const includes = patterns
      .filter((p) => !p.startsWith('!'))
      .map((p) => picomatch(p, { dot: true }));
    const excludes = patterns
      .filter((p) => p.startsWith('!'))
      .map((p) => picomatch(p.slice(1), { dot: true }));
    return [name, (file) => includes.some((m) => m(file)) && !excludes.some((m) => m(file))];
  })
);

const pullRequest = { event: 'pull_request', headRef: 'feature', sameRepository: true };
const releasePullRequest = { ...pullRequest, headRef: 'sweet/release-package-root-0123' };
const push = { event: 'push' };

/** Returns the sorted names of the job groups that the changed files select. */
function selected(files, context = pullRequest) {
  const changed = Object.fromEntries(
    Object.entries(filters).map(([name, matches]) => [name, files.some(matches)])
  );
  return Object.entries(selectJobs(changed, context))
    .filter(([, value]) => value)
    .map(([name]) => name)
    .sort();
}

const everything = [
  'authling',
  'botto',
  'chatto',
  'chattobot',
  'desktop',
  'desktop-capture',
  'desktop-full',
  'docs',
  'go',
  'knip',
  'runling',
  'translatto',
  'web'
];

const releaseFiles = [
  '.sweet.yml',
  '.sweet/notes/fix-something.md',
  'CHANGELOG.md',
  'cli/version.go',
  'apps/frontend/package.json',
  'apps/desktop/package.json',
  'packages/chatto-client/package.json'
];

test('frontend changes skip the Go and desktop jobs', () => {
  assert.deepEqual(selected(['apps/frontend/src/lib/Message.svelte']), ['chatto', 'web']);
  assert.deepEqual(selected(['apps/frontend/src/lib/Message.svelte', 'docs/fdr/FDR-001.md']), [
    'chatto',
    'web'
  ]);
});

test('Go changes skip the workspace and desktop jobs', () => {
  assert.deepEqual(selected(['cli/internal/core/message.go']), ['chatto', 'go']);
  assert.deepEqual(selected(['pkg/events/store.go']), ['authling', 'chatto', 'go']);
  assert.deepEqual(selected(['authling/internal/server.go']), ['authling', 'chatto', 'go']);
});

/**
 * Evaluates the publishing condition's JavaScript-compatible expression subset.
 * Model the implicit success() guard too: without a status function, GitHub
 * skips a job when any dependency was skipped. actionlint checks the syntax.
 */
function publishesImage(
  needs,
  { event = 'push', ref = 'refs/heads/main', cancelled = false } = {}
) {
  const condition = workflow.jobs['publish-main-image'].if.replace(/^\$\{\{|\}\}$/g, '').trim();
  const results = Object.values(needs).map(({ result }) => result);
  if (!/\b(always|cancelled|failure|success)\s*\(/.test(condition)) {
    if (!results.every((result) => result === 'success')) return false;
  }
  const expression = condition
    .replaceAll('needs.*.result', 'results')
    .replace(/needs\.([\w-]+)/g, (_, name) => `needs['${name}']`);
  return runInNewContext(expression, {
    needs,
    results,
    github: { event_name: event, ref },
    cancelled: () => cancelled,
    always: () => true,
    success: () => results.every((result) => result === 'success'),
    failure: () => results.includes('failure'),
    contains: (values, value) => values.includes(value)
  });
}

/** Creates successful or intentionally skipped dependencies from the path selection. */
function imageDependencies(files) {
  const groups = selected(files, push);
  const outputs = Object.fromEntries(
    Object.keys(workflow.jobs.changes.outputs).map((name) => [name, String(groups.includes(name))])
  );
  const needs = Object.fromEntries(
    workflow.jobs['publish-main-image'].needs.map((name) => [name, { result: 'success' }])
  );
  needs.changes.outputs = outputs;
  // Apply each dependency's actual selection condition, with successful ancestors.
  for (const [name, dependency] of Object.entries(needs)) {
    const condition = workflow.jobs[name].if;
    if (!condition) continue;
    const expression = condition.replace(/^\$\{\{|\}\}$/g, '').trim();
    const runs = runInNewContext(expression, {
      needs: { changes: needs.changes },
      github: { event_name: 'push', ref: 'refs/heads/main' },
      always: () => true,
      cancelled: () => false
    });
    dependency.result = runs ? 'success' : 'skipped';
  }
  return needs;
}

for (const file of ['apps/frontend/src/lib/Message.svelte', 'cli/internal/core/message.go']) {
  test(`main images publish with optional checks skipped for ${file}`, () => {
    const needs = imageDependencies([file]);
    assert.ok(Object.values(needs).some(({ result }) => result === 'skipped'));
    assert.equal(publishesImage(needs), true);

    for (const name of Object.keys(needs)) {
      for (const result of ['failure', 'cancelled']) {
        assert.equal(
          publishesImage({ ...needs, [name]: { ...needs[name], result } }),
          false,
          `${name}: ${result}`
        );
      }
    }
    assert.equal(
      publishesImage({ ...needs, 'build-main-image-contexts': { result: 'skipped' } }),
      false
    );
    assert.equal(publishesImage(needs, { cancelled: true }), false);
    assert.equal(publishesImage(needs, { event: 'pull_request' }), false);
    assert.equal(publishesImage(needs, { ref: 'refs/heads/release-0.5' }), false);
  });
}

test('main images publish for shared changes but not documentation-only changes', () => {
  assert.equal(publishesImage(imageDependencies(['mise.toml'])), true);
  assert.equal(publishesImage(imageDependencies(['README.md'])), false);
});

test('changes that affect both toolchains select both', () => {
  for (const file of ['proto/chatto/api/v1/message.proto', 'docker/Dockerfile.goreleaser']) {
    const jobs = selected([file]);
    assert.ok(jobs.includes('go') && jobs.includes('web'), file);
  }
  // The server tests build the frontend and test its build tooling.
  for (const file of ['apps/frontend/vite.config.ts', 'apps/frontend/scripts/build.mjs']) {
    assert.deepEqual(selected([file]), ['chatto', 'desktop', 'go', 'web'], file);
  }
});

test('shared desktop inputs build one platform for each operating system', () => {
  assert.deepEqual(selected(['packages/chatto-client/src/client.ts']), [
    'botto',
    'chatto',
    'chattobot',
    'desktop',
    'runling',
    'web'
  ]);
  assert.deepEqual(
    desktopMatrix(false).map(({ platform }) => platform),
    ['linux', 'linux-arm64', 'macos', 'windows']
  );
});

test('desktop changes and pushes build all desktop platforms', () => {
  assert.deepEqual(selected(['apps/desktop/main.mjs']), ['desktop', 'desktop-full']);
  assert.deepEqual(selected(['apps/frontend/src/lib/desktop/bridge.ts']), [
    'chatto',
    'desktop',
    'desktop-full',
    'web'
  ]);
  assert.deepEqual(selected(['tools/test-desktop-linux.sh']), ['desktop', 'desktop-full']);
  assert.ok(selected(['packages/chatto-client/src/client.ts'], push).includes('desktop-full'));
  assert.equal(desktopMatrix(true).length, 6);
  // Only the helper's own files run its Swift tests, also on a push.
  assert.ok(!selected(['apps/desktop/main.mjs'], push).includes('desktop-capture'));
  assert.ok(!selected(['apps/desktop/package.json'], push).includes('desktop-capture'));
  for (const file of [
    'apps/desktop/native/macos-capture-probe/Sources/Probe/Capture.swift',
    'tools/test-desktop-macos-capture.sh',
    '.github/actions/cache-macos-capture-probe/action.yml'
  ]) {
    assert.ok(selected([file]).includes('desktop-capture'), file);
  }
  for (const entry of desktopMatrix(true)) {
    assert.deepEqual(Object.keys(entry), ['platform', 'os', 'artifact-path']);
  }
});

test('Authling image inputs select the Authling jobs, which build the image', () => {
  for (const file of [
    'authling/Dockerfile',
    '.github/workflows/build-authling-image.yml',
    'tools/release/authling-image-metadata.sh',
    'tools/release/publish-authling-image.sh'
  ]) {
    assert.ok(selected([file]).includes('authling'), file);
  }
  // Image inputs at the root are common files.
  for (const file of ['go.work', 'go.work.sum', 'LICENSE']) {
    assert.deepEqual(selected([file]), everything, file);
  }
  assert.equal(
    workflow.jobs['authling-image'].if,
    "${{ needs.changes.outputs.authling == 'true' }}"
  );
});

test('shared build and dependency configuration selects all jobs', () => {
  for (const file of [
    'mise.toml',
    'pnpm-lock.yaml',
    'pnpm-workspace.yaml',
    'package.json',
    'turbo.json',
    '.github/workflows/ci.yml',
    '.github/actions/setup/action.yml',
    'tools/ci-select-jobs.mjs',
    '.github/workflows/a-new-workflow.yml'
  ]) {
    assert.deepEqual(selected([file]), everything, file);
  }
});

test('files that have unconditional checks select no product jobs', () => {
  for (const file of [
    'README.md',
    'docs/adr/ADR-001.md',
    'NOTICE',
    'LICENSES/MIT.txt',
    '.sweet.config.yml',
    '.sweet/notes/fix-something.md',
    'cli/CHANGELOG.md',
    'tools/ci-selection.test.mjs',
    '.github/workflows/release.yml',
    '.github/workflows/npm.yml',
    '.github/workflows/publish-releases.yml',
    'tools/release/publish-images.sh',
    '.github/workflows/build-docs-image.yml',
    'experimental/gobotto/main.go'
  ]) {
    assert.deepEqual(selected([file]), [], file);
  }
  assert.deepEqual(selected(['knip.ts']), ['knip']);
});

test('a release pull request selects no product jobs', () => {
  assert.deepEqual(selected(releaseFiles, releasePullRequest), []);
  assert.equal(
    jobOutputs({ common: true, go: true }, releasePullRequest)['desktop-matrix'],
    JSON.stringify(desktopMatrix(false))
  );
});

test('release files select their products outside a release pull request', () => {
  const jobs = ['botto', 'chatto', 'chattobot', 'desktop', 'desktop-full', 'go', 'runling', 'web'];
  // An ordinary branch, a fork that uses the release branch name, and the
  // push to main after the merge.
  assert.deepEqual(selected(releaseFiles), jobs);
  assert.deepEqual(selected(releaseFiles, { ...releasePullRequest, sameRepository: false }), jobs);
  assert.deepEqual(selected(releaseFiles, push), jobs);
});

test('a release pull request with another change selects jobs as an ordinary pull request', () => {
  const files = [...releaseFiles, 'docs/RELEASING.md', 'cli/internal/core/message.go'];
  assert.deepEqual(selected(files, releasePullRequest), selected(files));
  assert.ok(selected(files, releasePullRequest).includes('go'));
});

test('the release filter covers the version files of every Sweet manifest', () => {
  const manifests = execFileSync('git', ['ls-files', ':(glob)**/.sweet.yml'], {
    cwd: root,
    encoding: 'utf8'
  })
    .trim()
    .split('\n');
  assert.ok(manifests.includes('.sweet.yml') && manifests.includes('authling/.sweet.yml'));
  for (const manifest of manifests) {
    const versionFiles = parse(readFileSync(path.join(root, manifest), 'utf8'))['version-files'];
    const files = [
      manifest,
      ...(versionFiles ?? []).map((file) =>
        path.posix.join(path.posix.dirname(manifest), file.path)
      )
    ];
    for (const file of files) {
      assert.equal(filters['non-release'](file), false, `${file} is not a release file`);
    }
  }
});

test('every selection output that a job reads exists', () => {
  const produced = Object.keys(jobOutputs({}, pullRequest)).sort();
  assert.deepEqual(Object.keys(workflow.jobs.changes.outputs).sort(), produced);
  for (const [name, expression] of Object.entries(workflow.jobs.changes.outputs)) {
    assert.equal(expression, `\${{ steps.select.outputs.${name} }}`);
  }
  const read = new Set(
    [...workflowSource.matchAll(/needs\.changes\.outputs\.([\w-]+)/g)].map((match) => match[1])
  );
  for (const name of read) assert.ok(produced.includes(name), `Unknown output: ${name}`);
  for (const name of produced) assert.ok(read.has(name), `Unused output: ${name}`);
});
