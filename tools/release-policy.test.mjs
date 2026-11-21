/** Check the release workflows and run their scripts without publishing or accessing GitHub. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'yaml';

const root = path.resolve(import.meta.dirname, '..');
const workflowsDir = path.join(root, '.github/workflows');
const read = (file) => readFileSync(path.join(root, file), 'utf8').replaceAll('\r\n', '\n');
const workflow = (name) => parse(read(`.github/workflows/${name}`));
const ci = workflow('ci.yml');
const releases = workflow('publish-releases.yml');
const release = workflow('release.yml');
const npm = workflow('npm.yml');
const script = (name) => path.join(root, 'tools/release', name);

test('every Sweet reference uses the same revision', () => {
  const pins = new Set();
  for (const file of readdirSync(workflowsDir)) {
    for (const [, revision] of read(`.github/workflows/${file}`).matchAll(
      /uses: chattocorp\/sweet-release\/\S+@(\S+)/g
    )) {
      pins.add(revision);
    }
  }
  assert.deepEqual([...pins], ['7e618be0b38acb105fe128f030f2f9b71527a0f6']);
});

test('CI tags only after every other job and then publishes from the tag outputs', () => {
  const { tag } = ci.jobs;
  assert.match(tag.uses, /^chattocorp\/sweet-release\/\.github\/workflows\/tag\.yml@/);
  assert.deepEqual(tag.permissions, { contents: 'write' });
  assert.equal(tag.secrets, undefined, 'GITHUB_TOKEN tags start no other workflow');
  const others = Object.keys(ci.jobs).filter((name) => !['tag', 'publish-releases'].includes(name));
  assert.deepEqual([...tag.needs].sort(), others.sort());
  for (const guard of [
    "!contains(needs.*.result, 'failure')",
    "!contains(needs.*.result, 'cancelled')",
    "github.ref == 'refs/heads/main'",
    "startsWith(github.ref, 'refs/heads/release-')",
    '!github.event.created'
  ]) {
    assert.ok(tag.if.includes(guard), guard);
  }
  assert.equal(ci.jobs['publish-releases'].needs, 'tag');
  assert.equal(ci.jobs['publish-releases'].uses, './.github/workflows/publish-releases.yml');
  // Push runs are never cancelled or replaced: each run owns its commit range.
  assert.match(ci.concurrency.group, /github\.ref, github\.sha/);
  assert.equal(ci.concurrency['cancel-in-progress'], "${{ github.event_name == 'pull_request' }}");
});

test('each release unit has one publication job', () => {
  const { chatto, runling, translatto, components } = releases.jobs;
  assert.equal(chatto.if, "fromJSON(inputs.packages)['.'] != null");
  assert.equal(chatto.uses, './.github/workflows/release.yml');
  for (const [job, packagePath] of [
    [runling, 'packages/runling'],
    [translatto, 'packages/translatto']
  ]) {
    assert.equal(job.if, `fromJSON(inputs.packages)['${packagePath}'] != null`);
    assert.equal(job.uses, './.github/workflows/npm.yml');
    assert.equal(job.with.package, packagePath);
  }
  const select = components.steps.find((step) => step.id === 'select').run;
  // Only the newest tag of each non-root package, as for the artifact jobs.
  assert.match(select, /select\(\.key != "\." and \.value != null\)/);
  assert.match(select, /::warning::/);
  assert.equal(
    releases.jobs.authling.with.highest,
    "${{ fromJSON(inputs.packages)['authling'].highest }}"
  );
  const publish = components.steps.at(-1).run;
  assert.match(publish, /sweet github release "\$tag"\n/);
  assert.doesNotMatch(publish, /--latest/);
});

test('release approvals stay together and publication waits for all release artifacts', () => {
  const jobs = release.jobs;
  const text = read('.github/workflows/release.yml');
  assert.equal(release.on.push, undefined, 'a tag push must not start a release');
  assert.equal(jobs['approve-release'].environment, 'chatto-release');
  assert.equal(jobs['approve-release'].needs, undefined);
  assert.equal(text.match(/environment: chatto-release/g)?.length, 1);
  // Only these two independent jobs may request an environment approval.
  assert.equal(text.match(/^    environment:/gm)?.length, 2);
  const desktop = jobs['build-desktop-release'];
  assert.equal(desktop.needs, undefined);
  assert.equal(desktop.environment, '${{ matrix.environment }}');
  assert.deepEqual(
    [...new Set(desktop.strategy.matrix.include.map((entry) => entry.environment))],
    ['desktop-signing', 'desktop-windows-signing']
  );
  const steps = desktop.steps.map((step) => step.name);
  assert.ok(steps.indexOf('Verify trusted desktop ref') < steps.indexOf('Configure macOS signing'));
  assert.ok(
    steps.indexOf('Verify immutable GitHub OIDC subject') <
      steps.indexOf('Log in to Azure for Windows signing')
  );
  assert.deepEqual(jobs['publish-chatto-client'].needs, [
    'approve-release',
    'build-server-release',
    'assemble-desktop-release'
  ]);
  assert.equal(jobs['publish-chatto-client'].uses, './.github/workflows/npm.yml');
  assert.equal(jobs['publish-chatto-client'].with.package, 'packages/chatto-client');
  assert.equal(jobs['build-server-release'].needs, 'approve-release');
  assert.deepEqual(jobs.release.needs, [
    'approve-release',
    'build-server-release',
    'assemble-desktop-release',
    'publish-chatto-client'
  ]);
  assert.equal(jobs['publish-stable-docs'].needs, 'release');
  assert.equal(jobs['publish-stable-docs'].if, '${{ inputs.highest }}');
  // Builds run before approval completes publication, so they cannot write.
  for (const name of ['build-server-release', 'assemble-desktop-release']) {
    const job = jobs[name];
    assert.equal(job.environment, undefined, name);
    assert.ok(!Object.values(job.permissions).includes('write'), name);
    assert.ok(!JSON.stringify(job.steps).includes('"push":true'), name);
  }
  for (const [name, job] of Object.entries(jobs)) assert.ok(!job.if?.includes('always()'), name);
  const goreleaser = jobs['build-server-release'].steps.find(
    (step) => step.name === 'Run GoReleaser'
  );
  assert.equal(goreleaser.with.args, 'release --clean --skip=publish');
  assert.equal(goreleaser.env.GORELEASER_CURRENT_TAG, '${{ inputs.tag }}');
});

test('the GitHub release goes public only after its assets and images', () => {
  const steps = release.jobs.release.steps;
  const names = steps.map((step) => step.name);
  const order = [
    'Verify release archives',
    'Upload assets to the draft release',
    'Publish release images',
    'Publish GitHub Release',
    'Update Homebrew tap'
  ].map((name) => names.indexOf(name));
  assert.ok(
    order.every((index, i) => index >= 0 && (i === 0 || order[i - 1] < index)),
    names
  );
  const step = (name) => steps.find((entry) => entry.name === name);
  assert.match(
    step('Upload assets to the draft release').run,
    /^sweet github release "\$TAG" --draft /
  );
  // Sweet decides prerelease and Latest; the root package is Chatto.
  assert.equal(step('Publish GitHub Release').run, 'sweet github release "$TAG"');
  assert.equal(step('Update Homebrew tap').if, '${{ inputs.highest }}');
});

test('npm publishes only the tested tarball through trusted publishing', () => {
  const { package: pack, publish } = npm.jobs;
  assert.equal(publish.needs, 'package');
  assert.equal(publish.if, "inputs.commit != ''");
  assert.equal(publish.environment, 'npm');
  assert.deepEqual(publish.permissions, { contents: 'read', 'id-token': 'write' });
  assert.equal(pack.permissions, undefined);
  const command = publish.steps.at(-1);
  assert.equal(
    command.env.DIST_TAG,
    "${{ inputs.prerelease && 'next' || (inputs.highest && 'latest' || 'maintenance') }}"
  );
  assert.match(command.run, /npm publish "\$\{packages\[0\]\}" --tag "\$DIST_TAG" /);
  assert.match(command.run, /--provenance --ignore-scripts/);
});

/** Run a release script with stubbed commands; return the calls and GITHUB_OUTPUT lines. */
function run(t, file, { args = [], env = {}, stubs = '', cwd } = {}) {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'chatto-release-policy-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const calls = path.join(directory, 'calls');
  const output = path.join(directory, 'output');
  writeFileSync(calls, '');
  writeFileSync(output, '');
  try {
    execFileSync(
      'bash',
      [
        '-c',
        `record() { printf '%s\\n' "$*" >> "$CALLS"; }\n${stubs}\nsource "$0" "$@"`,
        file,
        ...args
      ],
      {
        cwd: cwd ?? directory,
        env: {
          ...process.env,
          CALLS: calls,
          GITHUB_OUTPUT: output,
          GITHUB_STEP_SUMMARY: path.join(directory, 'summary'),
          ...env
        },
        stdio: 'pipe'
      }
    );
  } catch (error) {
    t.diagnostic(readFileSync(calls, 'utf8'));
    throw error;
  }
  return {
    directory,
    calls: readFileSync(calls, 'utf8').trim().split('\n').filter(Boolean),
    output: Object.fromEntries(
      readFileSync(output, 'utf8')
        .trim()
        .split('\n')
        .filter(Boolean)
        .map((line) => line.split('='))
    )
  };
}

for (const [version, prerelease, highest, serverTags, clientTags] of [
  ['0.5.0-beta.24', 'true', 'false', ['0.5.0-beta.24', 'next'], ['0.5.0-beta.24', 'next']],
  ['0.5.0', 'false', 'true', ['0.5.0', '0.5', 'latest'], ['0.5.0', 'latest']],
  ['0.4.27', 'false', 'false', ['0.4.27', '0.4'], ['0.4.27']]
]) {
  test(`publishes both architectures and correct container tags for ${version}`, (t) => {
    const result = run(t, script('publish-images.sh'), {
      stubs: 'docker() { record "$@"; }',
      env: { VERSION: version, PRERELEASE: prerelease, HIGHEST: highest }
    });
    const expected = [];
    for (const [repository, tags] of [
      // The shared stream must not advance before the frontend is available.
      ['chatto-client', clientTags],
      ['chatto', serverTags]
    ]) {
      const image = `ghcr.io/chattocorp/${repository}`;
      expected.push(
        `push ${image}:${version}-amd64`,
        `push ${image}:${version}-arm64`,
        `buildx imagetools create ${tags.map((tag) => `--tag ${image}:${tag}`).join(' ')} ${image}:${version}-amd64 ${image}:${version}-arm64`
      );
    }
    assert.deepEqual(result.calls, expected);
  });
}

test('a failed image push stops the image script', (t) => {
  assert.throws(() =>
    run(t, script('publish-images.sh'), {
      stubs: 'docker() { return 1; }',
      env: { VERSION: '0.5.0', PRERELEASE: 'false', HIGHEST: 'true' }
    })
  );
});

test('npm packaging checks the registry history only for publication', (t) => {
  const stubs = `mise() { record "$@"; touch "$NPM_RELEASE_DIR/package.tgz"; }`;
  for (const [publish, check] of [
    ['true', 'x -- pnpm --filter @chatto/client check:release'],
    ['false', 'x -- pnpm --filter @chatto/client check:release --verify-only']
  ]) {
    const releaseDir = mkdtempSync(path.join(os.tmpdir(), 'chatto-npm-release-'));
    t.after(() => rmSync(releaseDir, { recursive: true, force: true }));
    const { calls } = run(t, script('npm-package.sh'), {
      args: ['packages/chatto-client'],
      stubs,
      env: { NPM_PUBLISH: publish, NPM_RELEASE_DIR: releaseDir }
    });
    assert.equal(calls[0], check);
    assert.equal(calls.at(-1), 'x -- pnpm --filter @chatto/client test:package');
  }
  const releaseDir = mkdtempSync(path.join(os.tmpdir(), 'chatto-npm-release-'));
  t.after(() => rmSync(releaseDir, { recursive: true, force: true }));
  const runling = run(t, script('npm-package.sh'), {
    args: ['packages/runling'],
    stubs,
    env: { NPM_PUBLISH: 'true', NPM_RELEASE_DIR: releaseDir }
  });
  assert.deepEqual(runling.calls, [
    'x -- node packages/runling/scripts/check-release.mjs',
    'check-runling',
    'test-runling',
    'test-runling-package'
  ]);
  assert.throws(() =>
    run(t, script('npm-package.sh'), {
      args: ['packages/api-types'],
      stubs,
      env: { NPM_RELEASE_DIR: releaseDir }
    })
  );
});

const sha = '1234567890abcdef1234567890abcdef12345678';

/** Run desktop-version.sh in a fixture with the given package versions. */
function resolveVersion(
  t,
  { version, frontendVersion = version, tag = `v${version}`, developmentVersion = '0.5.0-dev' }
) {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'chatto-desktop-release-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  for (const [name, packageVersion] of [
    ['desktop', version],
    ['frontend', frontendVersion]
  ]) {
    mkdirSync(path.join(directory, 'apps', name), { recursive: true });
    writeFileSync(
      path.join(directory, 'apps', name, 'package.json'),
      JSON.stringify({ version: packageVersion })
    );
  }
  return run(t, script('desktop-version.sh'), {
    cwd: directory,
    stubs: `git() { printf '%s\\n' "${sha}"; }\nmise() { printf '%s\\n' '${JSON.stringify({ CHATTO_DEVELOPMENT_VERSION: developmentVersion })}'; }`,
    env: { RELEASE_TAG: tag }
  }).output;
}

for (const version of ['0.5.0', '0.5.0-beta.21']) {
  test(`release ${version} bundles the exact shared frontend version`, (t) => {
    assert.deepEqual(resolveVersion(t, { version }), { version, sha, frontend_version: version });
  });
}

test('manual verification identifies unreleased source instead of the last release', (t) => {
  const result = resolveVersion(t, { version: '0.5.0-beta.20', tag: '' });
  assert.equal(result.frontend_version, '0.5.0-dev+1234567890ab');
});

test('rejects desktop and frontend version drift', (t) => {
  assert.throws(
    () => resolveVersion(t, { version: '0.5.0', frontendVersion: '0.4.26' }),
    (error) => error.stdout.toString().includes('Desktop and frontend package versions must match')
  );
});

test('manual verification fails without a development version', (t) => {
  assert.throws(
    () => resolveVersion(t, { version: '0.5.0', tag: '', developmentVersion: '' }),
    (error) => error.stdout.toString().includes('CHATTO_DEVELOPMENT_VERSION is required')
  );
});

test('Sweet owns the Chatto client, desktop, and frontend versions', () => {
  const manifest = read('.sweet.yml');
  const version = manifest.match(/^version: (.+)$/m)[1];
  for (const packagePath of [
    'packages/chatto-client/package.json',
    'apps/desktop/package.json',
    'apps/frontend/package.json'
  ]) {
    assert.ok(manifest.includes(`path: ${packagePath}\n    format: json\n    key: version`));
    assert.equal(JSON.parse(read(packagePath)).version, version);
  }
});
