/** Exercise the release workflow's version resolution with isolated package metadata. */
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

const workflow = readFileSync(
  new URL('../../../.github/workflows/release.yml', import.meta.url),
  'utf8'
);
/** Extract the workflow step from both LF and Windows CRLF checkouts. */
function extractVersionScript(source) {
  const versionStep = source
    .replaceAll('\r\n', '\n')
    .split('      - name: Resolve desktop version\n')[1]
    ?.split('\n      - name:')[0];
  assert.ok(versionStep, 'The desktop release must resolve its version');
  return versionStep
    .split('        run: |\n')[1]
    .split('\n')
    .map((line) => line.slice(10))
    .join('\n');
}
const versionScript = extractVersionScript(workflow);
const sha = '1234567890abcdef1234567890abcdef12345678';

for (const [name, buildStatus, copyTestStatus] of [
  ['success', 0, 0],
  ['signing failure', 1, 0],
  ['mount or copy failure', 0, 1]
]) {
  test(`DMG publication: ${name}`, () => {
    const script = workflow
      .replaceAll('\r\n', '\n')
      .split('      - name: Build and test signed macOS DMG\n')[1]
      .split('\n      - name:')[0]
      .split('        run: |\n')[1]
      .split('\n')
      .map((line) => line.slice(10))
      .join('\n');
    const result = spawnSync(
      'bash',
      [
        '-e',
        '-o',
        'pipefail',
        '-c',
        `
mise() {
  [[ "$*" == 'x -- pnpm --filter chatto-desktop build:macos-dmg --release' ]] || return 99
  return ${buildStatus}
}
tee() { cat; }
bash() { return ${copyTestStatus}; }
cp() { echo published; }
${script}`
      ],
      { encoding: 'utf8' }
    );
    assert.equal(result.status, buildStatus || copyTestStatus, result.stderr);
    assert.equal(result.stdout.includes('published'), buildStatus === 0 && copyTestStatus === 0);
  });
}

// Ticket validation runs in desktop-build. Packaging must still stop on an
// invalid signature or Gatekeeper assessment before it creates the ZIP.
for (const [name, signatureStatus, assessmentStatus] of [
  ['valid app', 0, 0],
  ['invalid signature', 1, 0],
  ['Gatekeeper rejection', 0, 1]
]) {
  test(`macOS release packaging: ${name}`, () => {
    const script = workflow
      .replaceAll('\r\n', '\n')
      .split('      - name: Package macOS release\n')[1]
      .split('\n      - name:')[0]
      .split('        run: |\n')[1]
      .split('\n')
      .map((line) => line.slice(10))
      .join('\n');
    const result = spawnSync(
      'bash',
      [
        '-e',
        '-o',
        'pipefail',
        '-c',
        `
test() { return 0; }
codesign() { return ${signatureStatus}; }
spctl() { echo assess; return ${assessmentStatus}; }
mkdir() { return 0; }
ditto() { echo archive; }
${script}`
      ],
      { encoding: 'utf8' }
    );
    const expectedStatus = signatureStatus || assessmentStatus;
    assert.equal(result.status, expectedStatus, result.stderr);
    const lines = result.stdout.trim().split('\n');
    assert.equal(lines.includes('assess'), signatureStatus === 0);
    assert.equal(lines.includes('archive'), expectedStatus === 0);
  });
}

test('workflow extraction supports Windows checkout line endings', () => {
  const lf = workflow.replaceAll('\r\n', '\n');
  assert.equal(extractVersionScript(lf.replaceAll('\n', '\r\n')), extractVersionScript(lf));
});

/** Run the actual workflow shell step without a checkout, signing, or network access. */
function resolveVersion(
  t,
  {
    version,
    frontendVersion = version,
    event = 'push',
    tag = `v${version}`,
    developmentVersion = '0.5.0-dev'
  }
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
  const output = path.join(directory, 'output');
  writeFileSync(output, '');
  // Stub only the environment and checkout identity; jq and bash execute the
  // same package/tag validation used by the signing workflow on all platforms.
  execFileSync(
    'bash',
    [
      '-c',
      `git() { printf '%s\\n' "$TEST_SHA"; }
mise() { printf '%s\\n' "$TEST_MISE_ENV"; }
${versionScript}`
    ],
    {
      cwd: directory,
      env: {
        ...process.env,
        GITHUB_EVENT_NAME: event,
        GITHUB_REF_NAME: tag,
        GITHUB_OUTPUT: 'output',
        GITHUB_STEP_SUMMARY: 'summary',
        TEST_SHA: sha,
        TEST_MISE_ENV: JSON.stringify({ CHATTO_DEVELOPMENT_VERSION: developmentVersion })
      },
      stdio: 'pipe'
    }
  );
  return Object.fromEntries(
    readFileSync(output, 'utf8')
      .trim()
      .split('\n')
      .map((line) => line.split('='))
  );
}

for (const version of ['0.5.0', '0.5.0-beta.21']) {
  test(`tagged ${version} uses the exact shared frontend version`, (t) => {
    assert.deepEqual(resolveVersion(t, { version }), { version, sha, frontend_version: version });
  });
}

test('manual verification identifies unreleased source instead of the last release', (t) => {
  const result = resolveVersion(t, {
    version: '0.5.0-beta.20',
    event: 'workflow_dispatch',
    tag: 'main'
  });
  assert.equal(result.frontend_version, '0.5.0-dev+1234567890ab');
});

test('rejects a tag that differs from the package version', (t) => {
  assert.throws(
    () => resolveVersion(t, { version: '0.5.0-beta.20', tag: 'v0.5.0-beta.21' }),
    (error) => error.stdout.toString().includes('Chatto tag and package versions must match')
  );
});

test('rejects desktop and frontend version drift', (t) => {
  assert.throws(
    () => resolveVersion(t, { version: '0.5.0', frontendVersion: '0.4.26' }),
    (error) => error.stdout.toString().includes('Desktop and frontend package versions must match')
  );
});

test('manual verification fails without a development version', (t) => {
  assert.throws(
    () =>
      resolveVersion(t, { version: '0.5.0', event: 'workflow_dispatch', developmentVersion: '' }),
    (error) => error.stdout.toString().includes('CHATTO_DEVELOPMENT_VERSION is required')
  );
});

test('Sweet owns both version files through the Chatto package', () => {
  const root = new URL('../../../', import.meta.url);
  const manifest = readFileSync(new URL('.sweet.yml', root), 'utf8').replaceAll('\r\n', '\n');
  for (const name of ['desktop', 'frontend']) {
    const packagePath = `apps/${name}/package.json`;
    assert.ok(manifest.includes(`path: ${packagePath}\n    format: json\n    key: version`));
    assert.equal(
      JSON.parse(readFileSync(new URL(packagePath, root), 'utf8')).version,
      manifest.match(/^version: (.+)$/m)[1]
    );
  }
});
