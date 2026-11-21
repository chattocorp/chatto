/** Check the release workflows and run their scripts without publishing or accessing GitHub. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { runInNewContext } from 'node:vm';
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
  const others = Object.keys(ci.jobs).filter(
    (name) => !['tag', 'describe-tag', 'publish-releases'].includes(name)
  );
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
  const publish = ci.jobs['publish-releases'];
  assert.deepEqual(publish.needs, ['tag', 'describe-tag']);
  assert.equal(publish.uses, './.github/workflows/publish-releases.yml');
  // A condition without a status function also requires success() of every
  // job before `tag`. Skipped optional jobs there would skip publication.
  assert.ok(publish.if.includes('!cancelled()'), 'publish-releases needs a status function');
  assert.ok(
    publish.if.includes("needs.tag.result == 'success'"),
    'publish-releases checks the tag result'
  );
  assert.ok(
    publish.if.includes("needs.describe-tag.result == 'success'"),
    'publish-releases checks the describe-tag result'
  );
  assert.equal(
    publish.with.tags,
    '${{ needs.tag.outputs.tags || needs.describe-tag.outputs.tags }}'
  );
  assert.equal(
    publish.with.testflight,
    "${{ github.event_name != 'workflow_dispatch' || inputs.testflight }}"
  );
  assert.match(releases.jobs.testflight.if, /^inputs\.testflight && /);
  // Environment secrets (desktop signing) are empty in release.yml unless every
  // caller in the chain uses `secrets: inherit` (actions/runner#4453).
  assert.equal(publish.secrets, 'inherit', 'ci.yml passes every secret to publish-releases');
  assert.equal(
    releases.jobs.chatto.secrets,
    'inherit',
    'publish-releases passes every secret to release.yml'
  );
  // Push runs are never cancelled or replaced: each run owns its commit range.
  assert.match(ci.concurrency.group, /github\.ref, github\.sha/);
  assert.equal(ci.concurrency['cancel-in-progress'], "${{ github.event_name == 'pull_request' }}");
});

test('a manual CI run only describes one tag and publishes it', () => {
  const dispatch = ci.on.workflow_dispatch.inputs;
  assert.deepEqual(dispatch.tag, {
    description: dispatch.tag.description,
    required: true,
    type: 'string'
  });
  assert.equal(dispatch.testflight.type, 'boolean');
  assert.equal(dispatch.testflight.default, false);
  const describe = ci.jobs['describe-tag'];
  assert.equal(describe.if, "github.event_name == 'workflow_dispatch'");
  assert.deepEqual(describe.permissions, { contents: 'read' });
  const step = describe.steps.find((entry) => entry.id === 'describe');
  // The tag reaches the script only through the environment.
  assert.equal(step.env.TAG, '${{ inputs.tag }}');
  assert.ok(!step.run.includes('${{'), step.run);
  // A manual run has its own concurrency group, so it never replaces a push run.
  assert.match(
    ci.concurrency.group,
    /github\.event_name == 'workflow_dispatch' && format\('publish-\{0\}', inputs\.tag\)/
  );
  // Evaluate each job's condition for a manual run, in dependency order. No
  // job other than describe-tag and publish-releases may run.
  const results = manualRunResults();
  for (const [name, result] of Object.entries(results)) {
    if (name === 'describe-tag' || name === 'publish-releases')
      assert.equal(result, 'success', name);
    else assert.equal(result, 'skipped', `${name} runs in a manual publication run`);
  }
});

/** Simulate the job results of a manual ci.yml run; every job that runs succeeds. */
function manualRunResults() {
  const results = {};
  const resultOf = (name) => {
    if (results[name]) return results[name];
    const job = ci.jobs[name];
    const needs = [job.needs ?? []].flat();
    const needResults = needs.map(resultOf);
    const context = {
      github: { event_name: 'workflow_dispatch', ref: 'refs/heads/main', event: {} },
      inputs: { tag: 'v1.0.0', testflight: false },
      needs: Object.fromEntries(
        needs.map((need) => [
          need,
          { result: results[need], outputs: results[need] === 'success' ? { tags: '[{}]' } : {} }
        ])
      ),
      __results: needResults,
      always: () => true,
      cancelled: () => false,
      success: () => needResults.every((result) => result === 'success'),
      failure: () => needResults.includes('failure'),
      contains: (values, value) => values.includes(value),
      startsWith: (value, prefix) => String(value ?? '').startsWith(prefix)
    };
    const condition = String(job.if ?? 'success()')
      .replace(/^\$\{\{|\}\}$/g, '')
      .replaceAll('needs.*.result', '__results')
      .replaceAll('needs.describe-tag', "needs['describe-tag']");
    const status = /always\(\)|cancelled\(\)|success\(\)|failure\(\)/.test(condition)
      ? condition
      : `success() && (${condition})`;
    results[name] = runInNewContext(status, context) ? 'success' : 'skipped';
    return results[name];
  };
  for (const name of Object.keys(ci.jobs)) resultOf(name);
  return results;
}

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
  assert.match(
    command.run,
    /bash tools\/release\/npm-publish\.sh "\$\{packages\[0\]\}" "\$DIST_TAG"/
  );
  // The publication script comes from this run, also for an older tag.
  const checkout = publish.steps.find((entry) => entry.uses?.startsWith('actions/checkout@'));
  assert.equal(checkout.with.ref, '${{ github.sha }}');
  assert.match(read('tools/release/npm-publish.sh'), /--provenance --ignore-scripts/);
  const tools = release.jobs.release.steps.find(
    (entry) => entry.name === 'Check out the publication scripts'
  );
  assert.equal(tools.with.ref, '${{ github.sha }}');
  for (const name of ['Publish release images', 'Update Homebrew tap']) {
    const entry = release.jobs.release.steps.find((candidate) => candidate.name === name);
    assert.match(entry.run, /^bash \.context\/release-tools\/tools\/release\//, name);
  }
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

// No image is published yet: `imagetools inspect` fails.
const dockerWithoutImages = `docker() {
  if [[ "$1 \${2:-} \${3:-}" == 'buildx imagetools inspect' ]]; then return 1; fi
  record "$@"
}`;

for (const [version, prerelease, highest, serverTags, clientTags] of [
  ['0.5.0-beta.24', 'true', 'false', ['0.5.0-beta.24', 'next'], ['0.5.0-beta.24', 'next']],
  ['0.5.0', 'false', 'true', ['0.5.0', '0.5', 'latest'], ['0.5.0', 'latest']],
  ['0.4.27', 'false', 'false', ['0.4.27', '0.4'], ['0.4.27']]
]) {
  test(`publishes both architectures and correct container tags for ${version}`, (t) => {
    const result = run(t, script('publish-images.sh'), {
      stubs: dockerWithoutImages,
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

test('a published image stays unchanged when a tag is published again', (t) => {
  const result = run(t, script('publish-images.sh'), {
    stubs: `docker() {
  if [[ "$*" == 'buildx imagetools inspect ghcr.io/chattocorp/chatto-client:0.6.0' ]]; then return 0; fi
  if [[ "$1 \${2:-} \${3:-}" == 'buildx imagetools inspect' ]]; then return 1; fi
  record "$@"
}`,
    env: { VERSION: '0.6.0', PRERELEASE: 'false', HIGHEST: 'true' }
  });
  assert.ok(
    result.calls.every((call) => !call.includes('chatto-client')),
    result.calls
  );
  assert.ok(result.calls.includes('push ghcr.io/chattocorp/chatto:0.6.0-amd64'));
});

test('npm packaging checks the package identity, never the registry', (t) => {
  const stubs = `mise() { record "$@"; touch "$NPM_RELEASE_DIR/package.tgz"; }`;
  const releaseDir = mkdtempSync(path.join(os.tmpdir(), 'chatto-npm-release-'));
  t.after(() => rmSync(releaseDir, { recursive: true, force: true }));
  const { calls } = run(t, script('npm-package.sh'), {
    args: ['packages/chatto-client'],
    stubs,
    env: { NPM_RELEASE_DIR: releaseDir }
  });
  assert.equal(calls[0], 'x -- pnpm --filter @chatto/client check:release --verify-only');
  assert.equal(calls.at(-1), 'x -- pnpm --filter @chatto/client test:package');
  const runling = run(t, script('npm-package.sh'), {
    args: ['packages/runling'],
    stubs,
    env: { NPM_RELEASE_DIR: releaseDir }
  });
  assert.deepEqual(runling.calls, ['check-runling', 'test-runling', 'test-runling-package']);
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

const sweetPackages = JSON.stringify([
  { path: '.', tagPrefix: 'v', version: '0.6.0' },
  { path: 'pkg/jobs', tagPrefix: 'pkg/jobs/v', version: '0.1.0' },
  { path: 'packages/runling', tagPrefix: 'runling/v', version: '0.8.0' }
]);
const remoteTags = [
  ['a1', 'refs/tags/v0.4.27'],
  ['c1', 'refs/tags/v0.4.27^{}'],
  ['a2', 'refs/tags/v0.5.0'],
  ['c2', 'refs/tags/v0.5.0^{}'],
  ['a3', 'refs/tags/v0.6.0-beta.4'],
  ['c3', 'refs/tags/v0.6.0-beta.4^{}'],
  ['c4', 'refs/tags/pkg/jobs/v0.1.0']
]
  .map((line) => line.join('\t'))
  .join('\n');

/** Run describe-tag.sh with stubbed `sweet` and `git` against `remoteTags`. */
function describeTag(
  t,
  tag,
  { ref = 'refs/heads/main', trusted = true, packages = sweetPackages } = {}
) {
  return run(t, script('describe-tag.sh'), {
    stubs: `sweet() { printf '%s' "$SWEET_PACKAGES"; }
git() {
  case "$1" in
    ls-remote)
      if [[ "$3" == --refs ]]; then
        awk -v prefix="\${5%\\*}" 'index($2, prefix) == 1 && $2 !~ /\\^\\{\\}$/' <<< "$REMOTE_TAGS"
      else
        for ref in "\${@:4}"; do awk -v ref="$ref" '$2 == ref' <<< "$REMOTE_TAGS"; done
      fi ;;
    fetch | cat-file) ;;
    for-each-ref) echo refs/remotes/origin/main ;;
    merge-base) [[ "$TRUSTED" == true ]] ;;
    *) return 99 ;;
  esac
}`,
    env: {
      TAG: tag,
      REF: ref,
      SWEET_PACKAGES: packages,
      REMOTE_TAGS: remoteTags,
      TRUSTED: String(trusted)
    }
  });
}

/** Parse the JSON outputs and check that `packages` maps the package to the same entry. */
function described(result) {
  const entry = JSON.parse(result.output.tags)[0];
  assert.deepEqual(JSON.parse(result.output.packages), { [entry.package]: entry });
  return entry;
}

for (const [tag, expected] of [
  // A root prerelease: its commit comes from the annotated tag.
  [
    'v0.6.0-beta.4',
    { package: '.', version: '0.6.0-beta.4', commit: 'c3', prerelease: true, highest: false }
  ],
  // The highest stable root version.
  ['v0.5.0', { package: '.', version: '0.5.0', commit: 'c2', prerelease: false, highest: true }],
  // An older stable line is not the highest.
  ['v0.4.27', { package: '.', version: '0.4.27', commit: 'c1', prerelease: false, highest: false }],
  // A component with a lightweight tag.
  [
    'pkg/jobs/v0.1.0',
    { package: 'pkg/jobs', version: '0.1.0', commit: 'c4', prerelease: false, highest: true }
  ]
]) {
  test(`describes ${tag} in the shape of the Sweet tag outputs`, (t) => {
    assert.deepEqual(described(describeTag(t, tag)), { ...expected, tag });
  });
}

test('describes a tag from a release branch', (t) => {
  assert.equal(
    described(describeTag(t, 'v0.5.0', { ref: 'refs/heads/release-0.5' })).tag,
    'v0.5.0'
  );
});

for (const [name, tag, options, message] of [
  ['an unknown tag', 'v9.9.9', {}, 'does not exist on origin'],
  ['a tag of no package', 'other/v1.0.0', {}, 'does not match'],
  ['a tag without a valid version', 'v1.0', {}, 'does not match'],
  ['an empty tag', '', {}, 'must be a Sweet release tag'],
  ['shell syntax', 'v1.0.0;id', {}, 'must be a Sweet release tag'],
  ['an option', '-v1.0.0', {}, 'must be a Sweet release tag'],
  ['Git revision syntax', 'v1.0.0..main', {}, 'must be a Sweet release tag'],
  ['whitespace', 'v1.0.0 v0.5.0', {}, 'must be a Sweet release tag'],
  ['a run from another branch', 'v0.5.0', { ref: 'refs/heads/feature' }, 'Run this from main'],
  ['a run from a tag', 'v0.5.0', { ref: 'refs/tags/v0.5.0' }, 'Run this from main'],
  ['a commit outside the release branches', 'v0.5.0', { trusted: false }, 'is not on origin/main'],
  [
    'a tag of two packages',
    'v0.5.0',
    {
      packages: JSON.stringify([
        { path: '.', tagPrefix: 'v', version: '0.5.0' },
        { path: 'other', tagPrefix: 'v', version: '0.5.0' }
      ])
    },
    'more than one Sweet package'
  ]
]) {
  test(`describe-tag rejects ${name}`, (t) => {
    assert.throws(
      () => describeTag(t, tag, options),
      (error) => error.stdout.toString().includes(message)
    );
  });
}

/** Make a tarball fixture and run npm-publish.sh with `npm view` returning `published`. */
function publishTarball(t, published) {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'chatto-npm-publish-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  mkdirSync(path.join(directory, 'package'));
  writeFileSync(
    path.join(directory, 'package/package.json'),
    JSON.stringify({ name: '@chatto/client', version: '0.6.0-beta.4' })
  );
  const tarball = path.join(directory, 'client.tgz');
  execFileSync('tar', ['-czf', tarball, '-C', directory, 'package']);
  const integrity = `sha512-${createHash('sha512').update(readFileSync(tarball)).digest('base64')}`;
  const result = run(t, script('npm-publish.sh'), {
    args: [tarball, 'next'],
    stubs: `npm() { record "$@"; if [[ "$1" == view ]]; then printf '%s' "$PUBLISHED"; fi; }`,
    env: { PUBLISHED: published === 'same' ? integrity : published }
  });
  return { calls: result.calls, tarball };
}

test('npm publication publishes a new version', (t) => {
  const { calls, tarball } = publishTarball(t, '');
  assert.deepEqual(calls, [
    'view @chatto/client@0.6.0-beta.4 dist.integrity --registry=https://registry.npmjs.org',
    `publish ${tarball} --tag next --access public --provenance --ignore-scripts --registry=https://registry.npmjs.org`
  ]);
});

test('npm publication skips the same tarball that npm already has', (t) => {
  const { calls } = publishTarball(t, 'same');
  assert.equal(calls.length, 1);
  assert.match(calls[0], /^view /);
});

test('npm publication fails for other content with a published version', (t) => {
  assert.throws(
    () => publishTarball(t, 'sha512-other'),
    (error) => error.stdout.toString().includes('already on npm with other content')
  );
});
