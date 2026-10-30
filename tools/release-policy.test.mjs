/** Exercise the workflow shell steps without publishing or accessing GitHub. */
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

const workflow = readFileSync(
  new URL('../.github/workflows/release.yml', import.meta.url),
  'utf8'
).replaceAll('\r\n', '\n');

test('all release approvals are available together before builds, with no publication approval', () => {
  const approval = workflow
    .split('\n  approve-release:\n')[1]
    .split('\n  build-desktop-release:\n')[0];
  const desktop = workflow
    .split('\n  build-desktop-release:\n')[1]
    .split('\n  assemble-desktop-release:\n')[0];
  const build = workflow.split('\n  build-server-release:\n')[1].split('\n  release:\n')[0];
  const publish = workflow.split('\n  release:\n')[1].split('\n  publish-stable-docs:\n')[0];
  const assemble = workflow
    .split('\n  assemble-desktop-release:\n')[1]
    .split('\n  build-server-release:\n')[0];
  assert.equal(workflow.match(/environment: chatto-release/g)?.length, 1);
  assert.match(approval, /environment: chatto-release/);
  assert.match(
    approval,
    /github.event_name == 'push' && startsWith\(github.ref, 'refs\/tags\/v'\)/
  );
  assert.ok(!approval.includes('needs:'));
  assert.ok(!desktop.includes('needs:'));
  assert.match(desktop, /environment: \$\{\{ matrix.environment \}\}/);
  assert.match(desktop, /environment: desktop-signing/);
  assert.match(desktop, /environment: desktop-windows-signing/);
  assert.match(desktop, /github.event_name == 'workflow_dispatch' && inputs.target == 'desktop'/);
  // Only these two independent jobs may request an environment approval.
  assert.equal(workflow.match(/^    environment:/gm)?.length, 2);
  assert.match(build, /needs: approve-release/);
  assert.ok(!build.includes('always()'));
  assert.ok(!publish.includes('environment:'));
  assert.match(
    publish,
    /needs: \[approve-release, build-server-release, assemble-desktop-release\]/
  );
  assert.ok(!publish.includes('always()'));
  assert.match(build, /args: release --clean --skip=publish/);
  for (const source of [build, assemble]) {
    assert.ok(!source.includes('environment:'));
    assert.ok(!source.includes('contents: write'));
    assert.ok(!source.includes('packages: write'));
    assert.ok(!source.includes('push: true'));
    assert.ok(!source.includes('gh release upload'));
    assert.ok(!source.includes('--draft=false'));
  }
  assert.match(assemble, /needs: build-desktop-release/);
  assert.ok(!assemble.includes('always()'));
  assert.ok(
    assemble.indexOf('Generate desktop update manifests') <
      assemble.indexOf('Upload desktop release verification')
  );
  assert.ok(publish.indexOf('Verify release archives') < publish.indexOf('Upload release assets'));
  assert.ok(publish.indexOf('Upload release assets') < publish.indexOf('Publish release images'));
  assert.ok(publish.indexOf('Publish release images') < publish.indexOf('Publish GitHub Release'));
  assert.match(workflow, /publish-stable-docs:\n    needs: release\n/);
});

for (const [version, latest, serverTags, clientTags] of [
  ['0.5.0-beta.24', 'false', ['0.5.0-beta.24', 'next'], ['0.5.0-beta.24', 'next']],
  ['0.5.0', 'true', ['0.5.0', '0.5', 'latest'], ['0.5.0', 'latest']],
  ['0.4.27', 'false', ['0.4.27', '0.4'], ['0.4.27']]
]) {
  test(`publishes both architectures and correct container tags for ${version}`, (t) => {
    const result = runShell(t, `docker() { gh "$@"; }\n${shellStep('Publish release images')}`, {
      VERSION: version,
      PUSH_LATEST: latest
    });
    const expected = [];
    for (const [repository, tags] of [
      ['chatto', serverTags],
      ['chatto-client', clientTags]
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

test('a failed image push stops publication before GitHub release visibility changes', (t) => {
  assert.throws(() =>
    runShell(
      t,
      `docker() { return 1; }\n${shellStep('Publish release images')}\n${shellStep('Publish GitHub Release')}`,
      { VERSION: '0.5.0', PUSH_LATEST: 'true', MAKE_LATEST: 'true' }
    )
  );
});

/** Extract a named shell step so tests exercise the actual publication commands. */
function shellStep(name, source = workflow) {
  const step = source.split(`      - name: ${name}\n`)[1]?.split('\n      - name:')[0];
  assert.ok(step, `Missing workflow step: ${name}`);
  const lines = step.split('        run: |\n')[1].split('\n');
  const end = lines.findIndex((line) => line.trim() && !line.startsWith('          '));
  return lines
    .slice(0, end < 0 ? undefined : end)
    .map((line) => line.slice(10))
    .join('\n');
}

/** Capture gh arguments and supply git tags while running real bash and jq. */
function runShell(t, script, env = {}) {
  const directory = mkdtempSync(path.join(os.tmpdir(), 'chatto-release-policy-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  execFileSync(
    'bash',
    [
      '-c',
      `
gh() { printf '%s\\n' "$*" >> calls; }
git() { printf '%s\\n' "$TEST_TAGS"; }
: > calls
: > output
${script}`
    ],
    {
      cwd: directory,
      env: { ...process.env, GITHUB_OUTPUT: 'output', ...env },
      stdio: 'pipe'
    }
  );
  return {
    calls: readFileSync(path.join(directory, 'calls'), 'utf8').trim().split('\n').filter(Boolean),
    output: Object.fromEntries(
      readFileSync(path.join(directory, 'output'), 'utf8')
        .trim()
        .split('\n')
        .filter(Boolean)
        .map((line) => line.split('='))
    )
  };
}

test('all release-please components are drafts until publication explicitly sets Latest', () => {
  const config = JSON.parse(
    readFileSync(new URL('../.release-please-config.json', import.meta.url), 'utf8')
  );
  assert.equal(config['force-tag-creation'], true);
  for (const [name, component] of Object.entries(config.packages)) {
    assert.equal(component.draft ?? config.draft, true, `${name} must start as a draft`);
  }
});

test('component publication leaves Chatto draft and publishes all components without Latest', (t) => {
  const outputs = {
    paths_released: JSON.stringify(['.', 'packages/translatto', 'pkg/events']),
    tag_name: 'v0.5.0',
    'packages/translatto--tag_name': 'translatto/v0.2.0',
    'pkg/events--tag_name': 'pkg/events/v0.1.0'
  };
  assert.deepEqual(
    runShell(t, shellStep('Publish component releases without Latest'), {
      RELEASE_OUTPUTS: JSON.stringify(outputs)
    }).calls,
    [
      'release edit translatto/v0.2.0 --draft=false --latest=false',
      'release edit pkg/events/v0.1.0 --draft=false --latest=false'
    ]
  );
});

test('the root release owns the client version and includes client changes', () => {
  const root = new URL('../', import.meta.url);
  const config = JSON.parse(readFileSync(new URL('.release-please-config.json', root), 'utf8'));
  const manifest = JSON.parse(readFileSync(new URL('.release-please-manifest.json', root), 'utf8'));
  const packagePath = 'packages/chatto-client/package.json';
  assert.equal(config.packages['packages/chatto-client'], undefined);
  assert.equal(manifest['packages/chatto-client'], undefined);
  assert.ok(!config.packages['.']['exclude-paths'].includes('packages/chatto-client'));
  assert.ok(
    config.packages['.']['extra-files'].some(
      (file) => file.path === packagePath && file.jsonpath === '$.version'
    )
  );
  assert.equal(JSON.parse(readFileSync(new URL(packagePath, root), 'utf8')).version, manifest['.']);
  assert.ok(shellStep('Verify release-please tag').includes(`${packagePath})`));
});

for (const [tag, expected] of [
  ['v0.5.0', 'latest'],
  ['v0.5.0-beta.22', 'next']
]) {
  test(`client publication from ${tag} uses npm ${expected}`, (t) => {
    const source = readFileSync(
      new URL('../.github/workflows/publish-chatto-client.yml', import.meta.url),
      'utf8'
    ).replaceAll('\r\n', '\n');
    assert.match(source, /tags:\n\s+- 'v\*'/);
    const script = shellStep('Publish the tested tarball with trusted publishing', source);
    const result = runShell(
      t,
      `
mkdir release
touch release/client.tgz
npm() { gh "$@"; }
${script}`,
      { RELEASE_TAG: tag }
    );
    assert.deepEqual(result.calls, [
      `publish ./release/client.tgz --tag ${expected} --access public --provenance --ignore-scripts --registry=https://registry.npmjs.org`
    ]);
  });
}

test('a root-only release never publishes through the component step', (t) => {
  assert.deepEqual(
    runShell(t, shellStep('Publish component releases without Latest'), {
      RELEASE_OUTPUTS: JSON.stringify({ paths_released: '["."]', tag_name: 'v0.5.0' })
    }).calls,
    []
  );
});

test('component outputs cannot accidentally publish a root Chatto tag', (t) => {
  assert.throws(() =>
    runShell(t, shellStep('Publish component releases without Latest'), {
      RELEASE_OUTPUTS: JSON.stringify({
        paths_released: '["packages/translatto"]',
        'packages/translatto--tag_name': 'v0.5.0'
      })
    })
  );
});

// Use the actual selection block from the existing trusted-tag validation step.
const validation = shellStep('Verify release-please tag');
const selectionStart = validation.indexOf('version="${GITHUB_REF_NAME#v}"');
assert.ok(selectionStart >= 0);
const selection = validation.slice(selectionStart);

for (const [version, tags, expected] of [
  ['0.5.0', 'v0.4.26\nv0.5.0', 'true'],
  ['0.4.27', 'v0.4.27\nv0.5.0', 'false'],
  ['0.5.0-beta.22', 'v0.4.26\nv0.5.0-beta.22', 'false'],
  ['0.10.0', 'v0.9.0\nv0.10.0', 'true']
]) {
  test(`Chatto ${version} explicitly publishes with latest=${expected}`, (t) => {
    const { output } = runShell(t, selection, { GITHUB_REF_NAME: `v${version}`, TEST_TAGS: tags });
    assert.equal(output.push_latest, expected);
    assert.deepEqual(
      runShell(t, shellStep('Publish GitHub Release'), {
        VERSION: version,
        MAKE_LATEST: output.push_latest
      }).calls,
      [`release edit v${version} --repo chattocorp/chatto --draft=false --latest=${expected}`]
    );
  });
}
