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

/** Extract a named shell step so tests exercise the actual publication commands. */
function shellStep(name) {
  const step = workflow.split(`      - name: ${name}\n`)[1]?.split('\n      - name:')[0];
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
    paths_released: JSON.stringify([
      '.',
      'packages/translatto',
      'packages/chatto-client',
      'pkg/events'
    ]),
    tag_name: 'v0.5.0',
    'packages/translatto--tag_name': 'translatto/v0.2.0',
    'packages/chatto-client--tag_name': 'chatto-client/v0.2.0',
    'pkg/events--tag_name': 'pkg/events/v0.1.0'
  };
  assert.deepEqual(
    runShell(t, shellStep('Publish component releases without Latest'), {
      RELEASE_OUTPUTS: JSON.stringify(outputs)
    }).calls,
    [
      'release edit translatto/v0.2.0 --draft=false --latest=false',
      'release edit chatto-client/v0.2.0 --draft=false --latest=false',
      'release edit pkg/events/v0.1.0 --draft=false --latest=false'
    ]
  );
});

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
