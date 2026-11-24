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

test('every Sweet reference uses the same release as mise.toml', () => {
  const [, miseVersion] = read('mise.toml').match(
    /"github:chattocorp\/sweet-release" = "(sweet@[^"]+)"/
  );
  const pins = new Set();
  const versions = new Set();
  // Jobs and steps that call a Sweet workflow or action, with their `with` inputs.
  const visit = (node) => {
    if (Array.isArray(node)) return node.forEach(visit);
    if (node === null || typeof node !== 'object') return;
    if (typeof node.uses === 'string' && node.uses.startsWith('chattocorp/sweet-release/')) {
      pins.add(node.uses.split('@')[1]);
      versions.add(node.with?.['sweet-version']);
    }
    Object.values(node).forEach(visit);
  };
  for (const file of readdirSync(workflowsDir)) visit(workflow(file));
  // The commit of the sweet@0.5.0 release; a tag in `uses:` could move.
  assert.deepEqual([...pins], ['70a402d3552871474f3a0f152fb2795592ec714a']);
  // Install the released CLI instead of building it from source.
  assert.deepEqual([...versions], [miseVersion]);
});

/** Every job of a workflow that calls a local reusable workflow, as `{job, target}`. */
const localCalls = (name) =>
  Object.entries(workflow(name).jobs)
    .filter(([, job]) => job.uses?.startsWith('./.github/workflows/'))
    .map(([, job]) => ({ job, target: path.basename(job.uses) }));

/** Every chain of local workflow calls from `name` and its prefixes, as lists of file names. */
function callChains(name, chain = [name]) {
  return [
    chain,
    ...localCalls(name).flatMap(({ target }) => callChains(target, [...chain, target]))
  ];
}

/** Evaluate a job condition like GitHub: without a status function, `success()` applies too. */
function evaluate(job, { github, inputs = {}, needs = {} }) {
  const results = Object.values(needs).map((need) => need.result);
  const condition = String(job.if ?? 'success()')
    .replace(/^\$\{\{|\}\}$/g, '')
    .replaceAll('needs.*.result', '__results')
    .replaceAll('needs.describe-tag', "needs['describe-tag']");
  const status = /always\(\)|cancelled\(\)|success\(\)|failure\(\)/.test(condition)
    ? condition
    : `success() && (${condition})`;
  return runInNewContext(status, {
    github,
    inputs,
    needs,
    __results: results,
    always: () => true,
    cancelled: () => false,
    success: () => results.every((result) => result === 'success'),
    failure: () => results.includes('failure'),
    contains: (values, value) => values.includes(value),
    startsWith: (value, prefix) => String(value ?? '').startsWith(prefix)
  });
}

/**
 * Simulate the job results of one workflow run in dependency order. Every job
 * that runs succeeds and gives the `outputs` of its entry, or `{ tags: '[{}]' }`.
 */
function simulate(name, context, outputs = {}) {
  const { jobs } = workflow(name);
  const results = {};
  const resultOf = (job) => {
    if (results[job]) return results[job];
    const needs = Object.fromEntries(
      [jobs[job].needs ?? []].flat().map((need) => {
        const result = resultOf(need);
        const out = result === 'success' ? (outputs[need] ?? { tags: '[{}]' }) : {};
        return [need, { result, outputs: out }];
      })
    );
    results[job] = evaluate(jobs[job], { ...context, needs }) ? 'success' : 'skipped';
    return results[job];
  };
  for (const job of Object.keys(jobs)) resultOf(job);
  return results;
}

const gate = workflow('release-gate.yml');
const pushRun = {
  github: { event_name: 'push', ref: 'refs/heads/main', event: { created: false } }
};
const manualRun = {
  github: { event_name: 'workflow_dispatch', ref: 'refs/heads/main', event: {} },
  inputs: { tag: 'v1.0.0', testflight: false }
};

test('CI releases from ci.yml only after every other job', () => {
  const { release: job } = ci.jobs;
  assert.equal(job.uses, './.github/workflows/release-gate.yml');
  // The tags wait for every CI check job.
  const others = Object.keys(ci.jobs).filter((name) => name !== 'release');
  assert.deepEqual([...job.needs].sort(), others.sort());
  for (const guard of [
    'always() && !cancelled()',
    "!contains(needs.*.result, 'failure')",
    "!contains(needs.*.result, 'cancelled')",
    "github.ref == 'refs/heads/main'",
    "startsWith(github.ref, 'refs/heads/release-')",
    '!github.event.created',
    "github.event_name == 'workflow_dispatch'"
  ]) {
    assert.ok(job.if.includes(guard), guard);
  }
  // Skipped optional jobs do not block a release; failures and cancellations do.
  const needs = (...results) =>
    Object.fromEntries(results.map((result, index) => [`job${index}`, { result }]));
  const runs = (context, ...results) => evaluate(job, { ...context, needs: needs(...results) });
  assert.equal(runs(pushRun, 'success', 'skipped'), true);
  assert.equal(runs(pushRun, 'success', 'failure'), false);
  assert.equal(runs(pushRun, 'success', 'cancelled'), false);
  assert.equal(
    runs({ github: { ...pushRun.github, ref: 'refs/heads/release-1.2' } }, 'success'),
    true
  );
  assert.equal(
    runs({ github: { ...pushRun.github, event: { created: true } } }, 'success'),
    false,
    'a new branch tags nothing'
  );
  assert.equal(runs({ github: { ...pushRun.github, ref: 'refs/heads/other' } }, 'success'), false);
  assert.equal(runs({ github: { event_name: 'pull_request', event: {} } }, 'success'), false);
  // Only ci.yml starts the release: the gate cannot run on its own.
  assert.deepEqual(Object.keys(gate.on), ['workflow_call']);
  // ci.yml gives the job the union of the permissions in the gate.
  const union = {};
  for (const gateJob of Object.values(gate.jobs)) {
    for (const [scope, level] of Object.entries(gateJob.permissions)) {
      if (union[scope] !== 'write') union[scope] = level;
    }
  }
  assert.deepEqual(job.permissions, union);
  assert.equal(job.with.tag, '${{ inputs.tag }}');
  assert.equal(
    job.with.testflight,
    "${{ github.event_name != 'workflow_dispatch' || inputs.testflight }}"
  );
  // Push runs are never cancelled or replaced: each run owns its commit range.
  assert.match(ci.concurrency.group, /github\.ref, github\.sha/);
  assert.equal(ci.concurrency['cancel-in-progress'], "${{ github.event_name == 'pull_request' }}");
});

test('the publication call chain starts at ci.yml and passes every secret on', () => {
  const chains = callChains('ci.yml').filter((chain) => chain.includes('release-gate.yml'));
  // npm trusted publishing trusts the top-level workflow `ci.yml`.
  assert.ok(
    chains.some((chain) => chain.at(-1) === 'npm.yml'),
    'ci.yml reaches npm.yml through the gate'
  );
  // GitHub allows ten levels of workflows, including the top-level caller.
  const depth = Math.max(...chains.map((chain) => chain.length));
  assert.ok(depth <= 10, `publication chain has ${depth} levels`);
  // GitHub allows 50 unique reusable workflows from one workflow file.
  assert.ok(new Set(callChains('ci.yml').flatMap((chain) => chain.slice(1))).size <= 50);
  // Environment secrets (desktop signing) are empty in release.yml unless every
  // caller in the chain uses `secrets: inherit` (actions/runner#4453).
  const toRelease = chains.find((chain) => chain.at(-1) === 'release.yml');
  assert.deepEqual(toRelease, [
    'ci.yml',
    'release-gate.yml',
    'publish-releases.yml',
    'release.yml'
  ]);
  toRelease.slice(0, -1).forEach((caller, index) => {
    const call = localCalls(caller).find(({ target }) => target === toRelease[index + 1]);
    assert.equal(call.job.secrets, 'inherit', `${caller} passes every secret on`);
  });
  // A job that needs other jobs, but has no status function, also requires
  // success() of all of them. A skipped job would then skip publication.
  for (const name of ['release-gate.yml', 'publish-releases.yml']) {
    for (const [id, gateJob] of Object.entries(workflow(name).jobs)) {
      if (!gateJob.needs) continue;
      assert.match(String(gateJob.if), /!cancelled\(\)|always\(\)/, `${name} ${id}`);
    }
  }
});

test('a push run tags and publishes from the tag outputs', () => {
  const { tag, 'publish-releases': publish } = gate.jobs;
  assert.match(tag.uses, /^chattocorp\/sweet-release\/\.github\/workflows\/tag\.yml@/);
  assert.deepEqual(tag.permissions, { contents: 'write' });
  assert.equal(tag.secrets, undefined, 'GITHUB_TOKEN tags start no other workflow');
  assert.deepEqual(publish.needs, ['tag', 'describe-tag']);
  assert.equal(publish.uses, './.github/workflows/publish-releases.yml');
  assert.equal(
    publish.with.tags,
    '${{ needs.tag.outputs.tags || needs.describe-tag.outputs.tags }}'
  );
  assert.equal(publish.with.testflight, '${{ inputs.testflight }}');
  assert.match(releases.jobs.testflight.if, /^inputs\.testflight && /);
  const push = { ...pushRun, inputs: { tag: '', testflight: true } };
  assert.deepEqual(simulate('release-gate.yml', push), {
    tag: 'success',
    'describe-tag': 'skipped',
    'publish-releases': 'success'
  });
  // A push without release versions publishes nothing.
  assert.equal(
    simulate('release-gate.yml', push, { tag: { tags: '[]' } })['publish-releases'],
    'skipped'
  );
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
  const describe = gate.jobs['describe-tag'];
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
  // No CI job other than the release runs in a manual run.
  for (const [name, result] of Object.entries(simulate('ci.yml', manualRun))) {
    assert.equal(result, name === 'release' ? 'success' : 'skipped', name);
  }
  assert.deepEqual(simulate('release-gate.yml', manualRun), {
    tag: 'skipped',
    'describe-tag': 'success',
    'publish-releases': 'success'
  });
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
  // CI's desktop-macos-capture job tests the helper; a release does not again.
  assert.ok(!JSON.stringify(desktop.steps).includes('test-desktop-macos-capture'));
  // Two macOS builds run on arm64, so the runner architecture is no unique name.
  const notarisationLog = desktop.steps.find(
    (step) => step.name === 'Upload macOS notarisation log'
  );
  assert.match(notarisationLog.with.name, /\$\{\{ matrix\.platform \}\}$/);
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
  // The published client image keeps its digest; only its other tags are set again.
  assert.deepEqual(
    result.calls.filter((call) => call.includes('chatto-client')),
    [
      'buildx imagetools create --tag ghcr.io/chattocorp/chatto-client:latest ghcr.io/chattocorp/chatto-client:0.6.0'
    ]
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

test('the Authling image builds in CI before tags and publishes from one workflow', () => {
  const image = workflow('build-authling-image.yml');
  // Only the callers start it: no path-filtered run, and no manual run that
  // could move `latest`. ci.yml publishes a release tag again.
  assert.deepEqual(Object.keys(image.on), ['workflow_call']);
  assert.deepEqual(Object.keys(image.on.workflow_call.inputs).sort(), ['highest', 'release_tag']);
  const job = ci.jobs['authling-image'];
  assert.equal(job.uses, './.github/workflows/build-authling-image.yml');
  assert.equal(job.with, undefined, 'CI builds the commit of its run');
  assert.equal(job.if, "${{ needs.changes.outputs.authling == 'true' }}");
  assert.ok(ci.jobs.release.needs.includes('authling-image'));
  assert.deepEqual(releases.jobs.authling.with, {
    release_tag: "${{ fromJSON(inputs.packages)['authling'].tag }}",
    highest: "${{ fromJSON(inputs.packages)['authling'].highest }}"
  });
  // Each platform builds and runs its check on a runner of its architecture.
  const { build, metadata, publish } = image.jobs;
  assert.deepEqual(
    build.strategy.matrix.include.map(({ arch, runner }) => `${arch} ${runner}`),
    ['amd64 ubuntu-latest', 'arm64 ubuntu-24.04-arm']
  );
  assert.equal(build.if, "needs.metadata.outputs.published != 'true'");
  const check = build.steps.find((step) => step.name === 'Check executable');
  assert.match(check.run, /--network none --read-only authling:check version/);
  const push = build.steps.find((step) => step.id === 'push');
  assert.match(push.with.outputs, /push-by-digest=true/);
  assert.doesNotMatch(push.with.outputs, /:/, 'the platform images get no tag');
  // The release tag reaches the scripts only through the environment, and the
  // scripts come from this run, also for an older tag.
  const resolve = metadata.steps.find((step) => step.id === 'metadata');
  assert.equal(resolve.env.RELEASE_TAG, '${{ inputs.release_tag }}');
  for (const step of [resolve, publish.steps.at(-1)]) {
    assert.ok(!step.run.includes('${{'), step.run);
  }
  for (const job of [metadata, publish]) {
    const tools = job.steps.find((step) => step.with?.['sparse-checkout'] === 'tools/release');
    assert.equal(tools.with.ref, '${{ github.sha }}');
  }
  assert.equal(publish.concurrency['cancel-in-progress'], false);
  assert.match(publish.if, /!cancelled\(\)/);
});

/** Run authling-image-metadata.sh on a source tree with the given versions. */
function authlingMetadata(
  t,
  { version = '0.3.0', manifest = version, published = false, ...env } = {}
) {
  const source = mkdtempSync(path.join(os.tmpdir(), 'chatto-authling-image-'));
  t.after(() => rmSync(source, { recursive: true, force: true }));
  mkdirSync(path.join(source, 'authling'));
  writeFileSync(
    path.join(source, 'authling/version.go'),
    `package authling\n\n// Version is the Authling release version.\nconst Version = "${version}" // sweet-version\n`
  );
  if (manifest !== null) {
    writeFileSync(
      path.join(source, 'authling/.sweet.yml'),
      `name: authling\nversion: ${manifest}\ntag-prefix: authling/v\n`
    );
  }
  return run(t, script('authling-image-metadata.sh'), {
    cwd: source,
    stubs: `git() { echo 0123456789abcdef0123456789abcdef01234567; }
docker() { record "$@"; [[ "$PUBLISHED" == true ]]; }`,
    env: {
      RELEASE_TAG: '',
      HIGHEST: 'false',
      GITHUB_EVENT_NAME: 'pull_request',
      GITHUB_REF: 'refs/pull/1/merge',
      PUBLISHED: String(published),
      ...env
    }
  });
}

const authlingSha = '0123456789abcdef0123456789abcdef01234567';

for (const [name, env, expected] of [
  [
    'the highest stable release moves latest',
    { RELEASE_TAG: 'authling/v0.3.0', HIGHEST: 'true' },
    { tag: '0.3.0', stream_tag: 'latest', publish: 'true' }
  ],
  [
    'an older release keeps latest',
    { RELEASE_TAG: 'authling/v0.3.0', HIGHEST: 'false' },
    { tag: '0.3.0', stream_tag: '', publish: 'true' }
  ],
  [
    'a prerelease keeps latest',
    { RELEASE_TAG: 'authling/v0.3.0-beta.1', version: '0.3.0-beta.1', HIGHEST: 'false' },
    { tag: '0.3.0-beta.1', stream_tag: '', publish: 'true' }
  ],
  [
    'a push to main moves dev',
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/heads/main' },
    { tag: authlingSha, stream_tag: 'dev', publish: 'true' }
  ],
  [
    'a push to a release branch publishes nothing',
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/heads/release-0.5' },
    { tag: authlingSha, stream_tag: '', publish: 'false' }
  ],
  ['a pull request publishes nothing', {}, { tag: authlingSha, stream_tag: '', publish: 'false' }]
]) {
  test(`Authling image metadata: ${name}`, (t) => {
    const { output } = authlingMetadata(t, env);
    assert.deepEqual(output, {
      sha: authlingSha,
      version: env.version ?? '0.3.0',
      published: 'false',
      ...expected
    });
  });
}

test('Authling image metadata reports a published release image', (t) => {
  const { output, calls } = authlingMetadata(t, {
    RELEASE_TAG: 'authling/v0.3.0',
    HIGHEST: 'true',
    published: true
  });
  assert.equal(output.published, 'true');
  assert.deepEqual(calls, ['buildx imagetools inspect ghcr.io/chattocorp/authling:0.3.0']);
});

for (const [name, options, message] of [
  ['a tag of another package', { RELEASE_TAG: 'v0.3.0' }, 'Only authling/v release tags'],
  ['a tag without a version', { RELEASE_TAG: 'authling/v' }, 'not a supported SemVer'],
  ['an invalid version', { RELEASE_TAG: 'authling/v0.3' }, 'not a supported SemVer'],
  ['a leading zero', { RELEASE_TAG: 'authling/v0.03.0' }, 'not a supported SemVer'],
  ['build metadata', { RELEASE_TAG: 'authling/v0.3.0+abc' }, 'not a supported SemVer'],
  ['shell syntax', { RELEASE_TAG: 'authling/v0.3.0;id' }, 'not a supported SemVer'],
  [
    'a different version.go',
    { RELEASE_TAG: 'authling/v0.3.0', version: '0.2.0', manifest: '0.3.0' },
    'must agree'
  ],
  [
    'a tag without a Sweet manifest',
    { RELEASE_TAG: 'authling/v0.3.0', manifest: null },
    'older than Sweet Release'
  ],
  ['a different manifest', { RELEASE_TAG: 'authling/v0.3.0', manifest: '0.2.0' }, 'must agree']
]) {
  test(`Authling image metadata rejects ${name}`, (t) => {
    assert.throws(
      () => authlingMetadata(t, options),
      (error) => error.stdout.toString().includes(message)
    );
  });
}

const amd64Digest = `sha256:${'a'.repeat(64)}`;
const arm64Digest = `sha256:${'b'.repeat(64)}`;

/** Run publish-authling-image.sh; `published` lists the tags in the registry. */
function publishAuthling(t, env, published = []) {
  return run(t, script('publish-authling-image.sh'), {
    stubs: `docker() {
  if [[ "$1 \${2:-} \${3:-}" == 'buildx imagetools inspect' ]]; then
    # The summary reads the digest of the published index.
    if [[ "\${5:-}" == --format ]]; then echo "sha256:\${4##*:}"; return 0; fi
    [[ " $PUBLISHED_TAGS " == *" \${4##*:} "* ]]
    return
  fi
  record "$@"
}`,
    env: {
      STREAM_TAG: '',
      DIGEST_AMD64: amd64Digest,
      DIGEST_ARM64: arm64Digest,
      PUBLISHED_TAGS: published.join(' '),
      ...env
    }
  });
}

test('publishes the Authling release index and latest from both platforms', (t) => {
  const image = 'ghcr.io/chattocorp/authling';
  const { calls } = publishAuthling(t, { TAG: '0.3.0', STREAM_TAG: 'latest', RELEASE: 'true' });
  assert.deepEqual(calls, [
    `buildx imagetools create --tag ${image}:0.3.0 --tag ${image}:latest ${image}@${amd64Digest} ${image}@${arm64Digest}`
  ]);
});

test('publishes the Authling commit image and dev from main', (t) => {
  const image = 'ghcr.io/chattocorp/authling';
  const { calls } = publishAuthling(t, { TAG: authlingSha, STREAM_TAG: 'dev', RELEASE: 'false' });
  assert.deepEqual(calls, [
    `buildx imagetools create --tag ${image}:${authlingSha} --tag ${image}:dev ${image}@${amd64Digest} ${image}@${arm64Digest}`
  ]);
});

test('a published Authling release stays unchanged and sets latest again', (t) => {
  const image = 'ghcr.io/chattocorp/authling';
  const { calls } = publishAuthling(
    t,
    { TAG: '0.3.0', STREAM_TAG: 'latest', RELEASE: 'true', DIGEST_AMD64: '', DIGEST_ARM64: '' },
    ['0.3.0']
  );
  assert.deepEqual(calls, [`buildx imagetools create --tag ${image}:latest ${image}:0.3.0`]);
});

test('Authling publication needs the digests of both platforms', (t) => {
  for (const digests of [
    { DIGEST_ARM64: '' },
    { DIGEST_AMD64: 'sha256:short' },
    { DIGEST_AMD64: `${amd64Digest} --tag x` }
  ]) {
    assert.throws(
      () => publishAuthling(t, { TAG: '0.3.0', RELEASE: 'true', ...digests }),
      (error) => error.stdout.toString().includes('digest is missing or not valid')
    );
  }
});
