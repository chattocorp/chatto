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

test('release approvals stay together and publication waits for all release artifacts', () => {
  const approval = workflow
    .split('\n  approve-release:\n')[1]
    .split('\n  publish-chatto-client:\n')[0];
  const client = workflow
    .split('\n  publish-chatto-client:\n')[1]
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
  assert.match(
    client,
    /needs: \[approve-release, build-server-release, assemble-desktop-release\]/
  );
  assert.match(client, /uses: \.\/\.github\/workflows\/publish-chatto-client.yml/);
  assert.match(client, /github.event_name == 'push' && startsWith\(github.ref, 'refs\/tags\/v'\)/);
  assert.match(client, /id-token: write/);
  assert.ok(!client.includes('always()'));
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
    /needs: \[approve-release, build-server-release, assemble-desktop-release, publish-chatto-client\]/
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

test('Sweet owns the Chatto client version', () => {
  const root = new URL('../', import.meta.url);
  const manifest = readFileSync(new URL('.sweet.yml', root), 'utf8');
  const packagePath = 'packages/chatto-client/package.json';
  assert.ok(manifest.includes(`path: ${packagePath}\n    format: json\n    key: version`));
  assert.equal(
    JSON.parse(readFileSync(new URL(packagePath, root), 'utf8')).version,
    manifest.match(/^version: (.+)$/m)[1]
  );
  assert.ok(shellStep('Verify release tag').includes(`${packagePath})`));
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
    assert.match(source, /^  workflow_call:/m);
    assert.match(source, /^  workflow_dispatch:/m);
    assert.doesNotMatch(source, /^  push:/m);
    assert.match(source, /publish:\n    if: github.event_name == 'push'\n    needs: package/);
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

// Use the actual selection block from the existing trusted-tag validation step.
const validation = shellStep('Verify release tag');
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

const componentScript = readFileSync(
  new URL('./publish-component-release.sh', import.meta.url),
  'utf8'
);

test('component notes start as drafts and never claim Latest, including retries', (t) => {
  for (const exists of ['true', 'false']) {
    const result = runShell(
      t,
      `
      export RUNNER_TEMP="$PWD"
      gh() {
        if [[ "$1 $2" == 'release view' ]]; then ${exists}; return; fi
        printf '%s\\n' "$*" >> calls
      }
      mise() {
        if [[ "$*" == 'x -- sweet status --json' ]]; then
          printf '%s' '[{"path":"authling","name":"authling","tagPrefix":"authling/v","version":"0.1.1"}]'
        else
          printf 'Release notes\\n'
        fi
      }
      ${componentScript}
    `,
      { GITHUB_REF_NAME: 'authling/v0.1.1' }
    );
    assert.equal(result.calls.at(-1), 'release edit authling/v0.1.1 --draft=false --latest=false');
    if (exists === 'false')
      assert.match(
        result.calls[0],
        /release create authling\/v0\.1\.1 --verify-tag --draft --latest=false/
      );
    else assert.equal(result.calls.length, 1);
  }
});

test('component publication rejects a root Chatto tag', (t) => {
  assert.throws(() =>
    runShell(
      t,
      `
    export RUNNER_TEMP="$PWD"
    mise() { printf '%s' '[{"path":".","tagPrefix":"v","version":"0.5.0"}]'; }
    ${componentScript}
  `,
      { GITHUB_REF_NAME: 'v0.5.0' }
    )
  );
});
