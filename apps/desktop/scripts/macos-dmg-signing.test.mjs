/** Verify that release failures cannot proceed to a trusted distribution artifact. */
import assert from 'node:assert/strict';
import test from 'node:test';
import { signMacOSDmg } from './macos-dmg-signing.mjs';

const environment = {
  CHATTO_MACOS_SIGN_IDENTITY: 'developer-id-hash',
  CHATTO_MACOS_NOTARY_API_KEY: '/key.p8',
  CHATTO_MACOS_NOTARY_API_KEY_ID: 'key-id',
  CHATTO_MACOS_NOTARY_API_ISSUER_ID: 'issuer'
};
const file = '/output/Chatto Desktop.dmg';

test('signs, submits, staples, and verifies the same disk image in order', () => {
  const calls = [];
  signMacOSDmg(file, environment, (command, args) => {
    calls.push([command, ...args]);
    return JSON.stringify({ id: 'submission', status: 'Accepted' });
  });
  assert.deepEqual(calls, [
    ['codesign', '--sign', 'developer-id-hash', '--timestamp', file],
    [
      'xcrun',
      'notarytool',
      'submit',
      file,
      '--key',
      '/key.p8',
      '--key-id',
      'key-id',
      '--issuer',
      'issuer',
      '--wait',
      '--output-format',
      'json'
    ],
    ['xcrun', 'stapler', 'staple', file],
    ['xcrun', 'stapler', 'validate', file],
    ['codesign', '--verify', '--strict', '--verbose=2', file],
    [
      'spctl',
      '--assess',
      '--type',
      'open',
      '--context',
      'context:primary-signature',
      '--verbose=4',
      file
    ]
  ]);
});

for (const missing of Object.keys(environment)) {
  test(`rejects missing ${missing} before invoking signing tools`, () => {
    assert.throws(
      () =>
        signMacOSDmg(file, { ...environment, [missing]: '' }, () => {
          assert.fail('must reject credentials before signing');
        }),
      /required|must be set together/
    );
  });
}

test('rejects unsigned mode when release signing was requested', () => {
  assert.throws(() => signMacOSDmg(file, {}), /requires all/);
});

for (const status of ['Invalid', 'Rejected', 'In Progress', undefined]) {
  test(`does not staple a submission with status ${status}`, () => {
    let calls = 0;
    assert.throws(
      () =>
        signMacOSDmg(file, environment, () => {
          calls++;
          return JSON.stringify({ id: 'submission', status });
        }),
      /not accepted/
    );
    assert.equal(calls, 2);
  });
}

for (let failure = 1; failure <= 6; failure++) {
  test(`stops immediately when release command ${failure} fails`, () => {
    let calls = 0;
    assert.throws(
      () =>
        signMacOSDmg(file, environment, () => {
          if (++calls === failure) throw new Error('command failed');
          return JSON.stringify({ id: 'submission', status: 'Accepted' });
        }),
      /command failed/
    );
    assert.equal(calls, failure);
  });
}
