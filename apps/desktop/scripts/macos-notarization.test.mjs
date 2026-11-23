/** Check recovery without repeated submissions or bypassed ticket validation. */
import assert from 'node:assert/strict';
import test from 'node:test';
import { setImmediate } from 'node:timers/promises';
import { stapleMacOSArtifact, submitMacOSArtifact } from './macos-notarization.mjs';
import { signMacOSDmg } from './macos-dmg-signing.mjs';

const environment = {
  CHATTO_MACOS_SIGN_IDENTITY: 'developer-id-hash',
  CHATTO_MACOS_NOTARY_API_KEY: '/key.p8',
  CHATTO_MACOS_NOTARY_API_KEY_ID: 'key-id',
  CHATTO_MACOS_NOTARY_API_ISSUER_ID: 'issuer'
};
const credentials = {
  appleApiKey: '/key.p8',
  appleApiKeyId: 'key-id',
  appleApiIssuer: 'issuer'
};

for (const status of ['Accepted', 'Invalid']) {
  test(`logs the complete ${status} submission response without authentication arguments`, (t) => {
    const logs = t.mock.method(console, 'log', () => {});
    const submission = {
      id: 'submission',
      status,
      message: 'Processing complete',
      details: { warnings: ['Example warning from Apple'] }
    };
    const submit = () =>
      submitMacOSArtifact('/output/Chatto.dmg', credentials, () => JSON.stringify(submission));
    if (status === 'Accepted') {
      submit();
    } else {
      assert.throws(submit, /not accepted: submission/);
    }
    assert.deepEqual(JSON.parse(logs.mock.calls[1].arguments[0]), submission);
    const output = logs.mock.calls.map((call) => call.arguments[0]).join('\n');
    for (const value of Object.values(credentials)) {
      assert.ok(!output.includes(value));
    }
  });
}

for (const operation of ['staple', 'validate']) {
  for (const artifact of ['app', 'DMG']) {
    test(`${artifact} retries ${operation} without signing or submitting again`, async (t) => {
      t.mock.timers.enable({ apis: ['setTimeout'] });
      const warnings = t.mock.method(console, 'warn', () => {});
      const calls = [];
      let failed = false;
      const dmg = '/output/Chatto.dmg';
      const app = '/output/Chatto.app';
      const file = artifact === 'app' ? app : dmg;
      const run = (command, args) => {
        calls.push([command, ...args]);
        if (args[0] === 'stapler' && args[1] === operation && args[2] === file && !failed) {
          failed = true;
          throw Object.assign(new Error('connection lost'), { status: 68 });
        }
        return JSON.stringify({ id: 'submission', status: 'Accepted' });
      };
      const result = signMacOSDmg(dmg, app, environment, run);
      const attempts = () =>
        calls.filter((call) => call[2] === operation && call[3] === file).length;

      await setImmediate();
      assert.equal(attempts(), 1);
      t.mock.timers.tick(14_999);
      await setImmediate();
      assert.equal(attempts(), 1);
      t.mock.timers.tick(1);
      await result;

      assert.equal(calls.filter((call) => call[1] === 'notarytool').length, 1);
      assert.equal(attempts(), 2);
      assert.equal(calls.filter((call) => call[1] === 'stapler').length, 5);
      assert.equal(
        calls.filter((call) => call[0] === 'codesign' && call[1] === '--sign').length,
        1
      );
      assert.match(warnings.mock.calls[0].arguments[0], /retrying in 15s/);
    });
  }

  test(`${operation} stops after five network failures with increasing delays`, async (t) => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const warnings = t.mock.method(console, 'warn', () => {});
    const failure = Object.assign(new Error('connection lost'), { status: 68 });
    let attempts = 0;
    const calls = [];
    const result = stapleMacOSArtifact('/output/Chatto.app', (command, args) => {
      calls.push(args[1]);
      if (args[1] === operation) {
        attempts++;
        throw failure;
      }
    });
    const rejected = assert.rejects(result, (error) => error === failure);
    for (const [index, delay] of [15_000, 30_000, 60_000, 120_000].entries()) {
      await setImmediate();
      assert.equal(attempts, index + 1);
      t.mock.timers.tick(delay);
    }
    await rejected;
    assert.equal(attempts, 5);
    assert.equal(warnings.mock.callCount(), 4);
    assert.deepEqual(
      warnings.mock.calls.map((call) => call.arguments[0].match(/retrying in (\d+)s/)[1]),
      ['15', '30', '60', '120']
    );
    if (operation === 'staple') assert.ok(!calls.includes('validate'));
  });

  for (const status of [65, 1, null]) {
    test(`${operation} does not retry exit status ${status}`, async () => {
      let attempts = 0;
      const failure = Object.assign(new Error('permanent failure'), { status });
      await assert.rejects(
        stapleMacOSArtifact('/output/Chatto.app', (command, args) => {
          if (args[1] === operation) {
            attempts++;
            throw failure;
          }
        }),
        (error) => error === failure
      );
      assert.equal(attempts, 1);
    });
  }
}
