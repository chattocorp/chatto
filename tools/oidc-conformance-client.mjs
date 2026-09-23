// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later
import { randomBytes } from 'node:crypto';
import { writeFile } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import path from 'node:path';

const exec = promisify(execFile);

// The shared suite runner supplies API access and result handling. Chatto owns
// client registration and invokes its actual HTTP handlers in a Go test process.
export async function runClientChecks({ api, run, suite, state, env, signal, fail }) {
  const secret = randomBytes(32).toString('hex');
  const clientConfig = path.join(state, 'client-config.json');
  await writeFile(clientConfig, JSON.stringify({ alias: 'chatto', description: 'Chatto relying party checks',
    client: { client_id: 'chatto-conformance', client_secret: secret,
      redirect_uri: 'http://localhost:19410/auth/providers/conformance/callback' } }), { mode: 0o600 });
  const variant = { client_registration: 'static_client', request_type: 'plain_http_request' };
  const client = await api(`/api/plan?planName=oidcc-client-basic-certification-test-plan&variant=${encodeURIComponent(JSON.stringify(variant))}`, clientConfig);
  async function drive(name, reject, method = '') {
    try {
      await exec(env.CONFORMANCE_CLIENT_BINARY, ['-test.run=^TestOIDCConformance$', '-test.count=1', '-test.timeout=45s'], {
        env: { ...env, CHATTO_CONFORMANCE_ISSUER: `${suite}/test/a/chatto/`,
          CHATTO_CONFORMANCE_SECRET: secret, CHATTO_CONFORMANCE_REJECT: String(reject),
          CHATTO_CONFORMANCE_AUTH_METHOD: method }, timeout: 60_000, signal,
      });
    } catch (error) {
      // This test file emits fixed failure descriptions, never claim values.
      const assertion = error.stdout?.split('\n').find((line) => /^\s+oidc_conformance_test\.go:\d+: /.test(line));
      throw fail(`Chatto assertion failed for ${name}${assertion ? `: ${assertion.trim()}` : ''}`);
    }
  }
  for (const [name, reject] of [
    ['oidcc-client-test', false], ['oidcc-client-test-client-secret-basic', false],
    ['oidcc-client-test-invalid-iss', true], ['oidcc-client-test-invalid-aud', true],
    ['oidcc-client-test-invalid-sig-rs256', true], ['oidcc-client-test-idtoken-sig-none', true],
    ['oidcc-client-test-userinfo-invalid-sub', true], ['oidcc-client-test-scope-userinfo-claims', false]
  ]) {
    await run(client.id, name, { drive: () => drive(name, reject),
      expectedSkip: name === 'oidcc-client-test-idtoken-sig-none' });
  }
  // Basic RP fixes Basic authentication. A standalone official module also
  // exercises discovery-selected POST and an explicit POST override.
  for (const method of ['', 'client_secret_post']) {
    await run(undefined, 'oidcc-client-test', { drive: () => drive('oidcc-client-test POST', false, method),
      variant: { ...variant, response_type: 'code', response_mode: 'default', client_auth_type: 'client_secret_post' },
      configFile: clientConfig, scenario: method ? 'explicit POST' : 'discovered POST' });
  }
}
