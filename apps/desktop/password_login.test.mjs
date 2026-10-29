// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { passwordLogin } from './password_login.mjs';

const request = {
  serverUrl: 'https://chat.example',
  identifier: 'alice',
  password: 'example-password'
};

test('password submission uses a fixed endpoint without cookies or redirects', async () => {
  const result = await passwordLogin(request, async (url, options) => {
    assert.equal(url, 'https://chat.example/auth/login');
    assert.equal(options.redirect, 'error');
    assert.equal(options.credentials, 'omit');
    assert.deepEqual(JSON.parse(options.body), {
      identifier: request.identifier,
      password: request.password
    });
    assert.ok(options.signal instanceof AbortSignal);
    return Response.json({ token: 'access' });
  });
  assert.deepEqual(result, { ok: true, session: { token: 'access' } });
});

test('rejects unsafe destinations before sending a password', async () => {
  for (const serverUrl of [
    'http://chat.example',
    'https://user:pass@chat.example',
    'https://chat.example/path',
    'file:///secret',
    'https://chat.example?token=x'
  ]) {
    await assert.rejects(
      passwordLogin({ ...request, serverUrl }, () => assert.fail('must not send'))
    );
  }
});

test('permits local development and never exposes server or network errors', async () => {
  const local = { ...request, serverUrl: 'http://chat.philadelphia.localhost:1234' };
  assert.deepEqual(
    await passwordLogin(local, async () => new Response(request.password, { status: 401 })),
    { ok: false }
  );
  assert.deepEqual(
    await passwordLogin(local, async () => {
      throw new Error(request.password);
    }),
    { ok: false }
  );
});
