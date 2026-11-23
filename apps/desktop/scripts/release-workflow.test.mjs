/** Exercise release workflow shell steps without signing or publishing artifacts. */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const workflow = readFileSync(
  new URL('../../../.github/workflows/release.yml', import.meta.url),
  'utf8'
);
/** Extract a named shell step from both LF and Windows CRLF checkouts. */
function workflowScript(name, source = workflow) {
  const step = source
    .replaceAll('\r\n', '\n')
    .split(`      - name: ${name}\n`)[1]
    ?.split('\n      - name:')[0];
  const script = step?.split('        run: |\n')[1];
  assert.ok(script, `Missing shell script for workflow step: ${name}`);
  return script
    .split('\n')
    .map((line) => line.slice(10))
    .join('\n');
}
const macOSPackageScript = workflowScript('Package macOS release');

for (const [name, buildStatus, copyTestStatus] of [
  ['success', 0, 0],
  ['signing failure', 1, 0],
  ['mount or copy failure', 0, 1]
]) {
  test(`DMG publication: ${name}`, () => {
    const script = workflowScript('Build and test signed macOS DMG');
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
mkdir() { return 0; }
cp() { echo published; }
${script}`
      ],
      { encoding: 'utf8' }
    );
    assert.equal(result.status, buildStatus || copyTestStatus, result.stderr);
    assert.equal(result.stdout.includes('published'), buildStatus === 0 && copyTestStatus === 0);
  });
}

test('macOS notarises the DMG before it packages the stapled app as a ZIP', () => {
  const steps = workflow.replaceAll('\r\n', '\n');
  const dmg = steps.indexOf('      - name: Build and test signed macOS DMG\n');
  const zip = steps.indexOf('      - name: Package macOS release\n');
  assert.ok(dmg > 0 && zip > dmg, 'The DMG step must run before the ZIP step.');
});

// Ticket validation runs in build:macos-dmg --release. Packaging must still stop
// on an invalid signature or Gatekeeper assessment before it creates the ZIP.
for (const [name, signatureStatus, assessmentStatus] of [
  ['valid app', 0, 0],
  ['invalid signature', 1, 0],
  ['Gatekeeper rejection', 0, 1]
]) {
  test(`macOS release packaging: ${name}`, () => {
    const script = macOSPackageScript;
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

for (const [target, runner, expected] of [
  ['', 'ARM64', 'ARM64'],
  ['', 'X64', 'X64'],
  ['universal', 'ARM64', 'UNIVERSAL']
]) {
  test(`macOS ZIP uses the ${expected} target name`, () => {
    const script = macOSPackageScript;
    const result = spawnSync(
      'bash',
      [
        '-e',
        '-o',
        'pipefail',
        '-c',
        `
      test() { return 0; }
      codesign() { return 0; }
      spctl() { return 0; }
      mkdir() { return 0; }
      ditto() { printf '%s\\n' "$@"; }
      ${script}
    `
      ],
      {
        encoding: 'utf8',
        env: { ...process.env, CHATTO_DESKTOP_ARCH: target, RUNNER_ARCH: runner, VERSION: '0.5.0' }
      }
    );
    assert.equal(result.status, 0, result.stderr);
    assert.ok(result.stdout.includes(`chatto-desktop_0.5.0_macOS_${expected}.zip`));
  });
}

test('workflow extraction supports Windows checkout line endings', () => {
  const lf = workflow.replaceAll('\r\n', '\n');
  assert.equal(
    workflowScript('Package macOS release', lf.replaceAll('\n', '\r\n')),
    macOSPackageScript
  );
});
