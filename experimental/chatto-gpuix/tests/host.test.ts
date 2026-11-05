import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
/** Host persistence and browser callback tests use synthetic credentials and loopback HTTP only. */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { randomBytes, createHash } from 'node:crypto';
import { openNativeState, memoryState } from '../src/state.ts';
import { browserSignIn } from '../src/browser-auth.ts';
import { emptyServerSession } from '@chatto/client/server/sessions';

const root = fileURLToPath(new URL('../../../.context/', import.meta.url));
mkdirSync(root, { recursive: true });

test('encrypted state survives restart, protects bytes, and excludes another process', async () => {
  const directory = mkdtempSync(join(root, 'native-vault-'));
  const key = randomBytes(32);
  const open = () => openNativeState({ directory, key: async () => Buffer.from(key) });
  try {
    const state = await open();
    const session = {
      ...emptyServerSession(),
      token: 'synthetic-access-token',
      refreshToken: 'synthetic-refresh-token',
      accessTokenExpiresAt: 123,
      refreshTokenExpiresAt: 456,
      oauthClientId: 'fixture-client',
      refreshRequestId: 'fixture-request-id'
    };
    const record = {
      ...session,
      id: 'fixture',
      url: 'https://fixture.example',
      name: 'Fixture',
      iconUrl: null,
      addedAt: 1
    };
    state.writeAuthentication('fixture', {
      token: session.token,
      refreshToken: session.refreshToken,
      accessTokenExpiresAt: 123,
      refreshTokenExpiresAt: 456,
      oauthClientId: 'fixture-client',
      refreshRequestId: 'fixture-request-id',
      reauthRequiredAt: null
    });
    state.writeServers([record]);
    state.setUi({
      uiScale: 1.37,
      activeServerId: 'fixture',
      rooms: { fixture: 'room-one' },
      order: ['fixture']
    });
    const bytes = readFileSync(join(directory, 'state.enc'));
    assert.equal(bytes.includes(Buffer.from(session.token)), false);
    assert.equal(bytes.includes(Buffer.from(session.refreshToken)), false);
    assert.equal(statSync(join(directory, 'state.enc')).mode & 0o777, 0o600);
    assert.equal(statSync(directory).mode & 0o777, 0o700);
    await assert.rejects(open(), /Another Chatto GPUix/);
    await state.close();
    const restored = await open();
    assert.equal(restored.readAuthentication('fixture')?.refreshRequestId, 'fixture-request-id');
    assert.equal(restored.ui.rooms.fixture, 'room-one');
    assert.equal(restored.ui.uiScale, 1.37);
    assert.equal(restored.readServers()[0]?.id, 'fixture');
    await restored.close();
    const corrupt = Buffer.from(bytes);
    corrupt[corrupt.length - 1] ^= 1;
    writeFileSync(join(directory, 'state.enc'), corrupt);
    await assert.rejects(open(), /Could not open saved sessions securely/);
    assert.deepEqual(readFileSync(join(directory, 'state.enc')), corrupt);
    writeFileSync(join(directory, 'state.enc'), bytes);
    const recovered = await open();
    await recovered.close();
  } finally {
    key.fill(0);
    rmSync(directory, { recursive: true, force: true });
  }
});

test('memory host stores do not share registrations or view state', async () => {
  const one = memoryState();
  const two = memoryState();
  one.setUi({ activeServerId: 'one', rooms: { one: 'room' }, order: ['one'] });
  assert.equal(two.ui.activeServerId, '');
  await one.close();
  await two.close();
  assert.throws(() => one.setUi({ activeServerId: 'late', rooms: {}, order: [] }), /closed/);
});

test('browser OAuth validates callback state and route, and binds PKCE exchange', async () => {
  const original = globalThis.fetch;
  let verifier = '';
  let challenge = '';
  let endpoint: URL | null = null;
  globalThis.fetch = async (url, options) => {
    if (String(url).endsWith('/oauth/token')) {
      const body = JSON.parse(String(options?.body));
      verifier = body.code_verifier;
      assert.equal(body.client_id, 'fixture-client');
      assert.equal(body.code, 'synthetic-code');
      assert.equal(options?.redirect, 'error');
      assert.equal(options?.credentials, 'omit');
      return Response.json({
        access_token: 'fixture-access',
        refresh_token: 'fixture-refresh',
        expires_in: 300,
        refresh_token_expires_in: 600
      });
    }
    return Response.json(
      {
        profile: { name: 'Fixture', version: '0.5.0-rc.4' },
        login: { authorizeUrl: '/oauth/authorize' }
      },
      { headers: { 'Content-Type': 'application/json' } }
    );
  };
  try {
    const result = await browserSignIn(
      'https://fixture.example',
      'fixture-client',
      new AbortController().signal,
      async (url) => {
        endpoint = new URL(url.searchParams.get('redirect_uri')!);
        challenge = url.searchParams.get('code_challenge')!;
        const bad = new URL(endpoint);
        bad.searchParams.set('code', 'synthetic-code');
        bad.searchParams.set('state', 'wrong');
        assert.equal((await original(bad)).status, 400);
        const callback = new URL(endpoint);
        callback.searchParams.set('code', 'synthetic-code');
        callback.searchParams.set('state', url.searchParams.get('state')!);
        const duplicate = new URL(callback);
        duplicate.searchParams.append('code', 'other');
        assert.equal((await original(duplicate)).status, 400);
        assert.equal((await original(callback)).status, 200);
      }
    );
    assert.equal(result.credentials.token, 'fixture-access');
    assert.equal(result.credentials.oauthClientId, 'fixture-client');
    assert.equal(createHash('sha256').update(verifier).digest('base64url'), challenge);
    await assert.rejects(original(endpoint!, { signal: AbortSignal.timeout(1000) }));
  } finally {
    globalThis.fetch = original;
  }
});

test('browser cancellation closes its callback listener', async () => {
  const original = globalThis.fetch;
  const controller = new AbortController();
  let callback: URL | null = null;
  globalThis.fetch = async () =>
    Response.json(
      {
        profile: { name: 'Fixture', version: '0.5.0-rc.4' },
        login: { authorizeUrl: '/oauth/authorize' }
      },
      { headers: { 'Content-Type': 'application/json' } }
    );
  try {
    await assert.rejects(
      browserSignIn('https://fixture.example', 'fixture-client', controller.signal, async (url) => {
        callback = new URL(url.searchParams.get('redirect_uri')!);
        controller.abort();
      }),
      /cancelled/
    );
    await assert.rejects(original(callback!, { signal: AbortSignal.timeout(1000) }));
  } finally {
    globalThis.fetch = original;
  }
});

// This intentionally exercises LaunchServices, not just an injected URL receiver.
test(
  'desktop OAuth receives an actual macOS custom-scheme callback',
  { skip: process.platform !== 'darwin' },
  async () => {
    const original = globalThis.fetch;
    globalThis.fetch = async (url, options) => {
      if (String(url).endsWith('/oauth/token')) {
        const body = JSON.parse(String(options?.body));
        assert.equal(body.client_id, 'chatto://desktop');
        assert.equal(body.redirect_uri, 'chatto://desktop/servers/callback?mode=popup');
        assert.equal(body.code, 'synthetic-desktop-code');
        assert.ok(body.code_verifier);
        return Response.json({
          access_token: 'fixture-access',
          refresh_token: 'fixture-refresh',
          expires_in: 300,
          refresh_token_expires_in: 600
        });
      }
      return Response.json({
        profile: { name: 'Fixture', version: '0.5.0-rc.4' },
        login: { authorizeUrl: '/oauth/authorize' }
      });
    };
    try {
      const result = await browserSignIn(
        'https://fixture.example',
        'chatto://desktop',
        AbortSignal.timeout(20_000),
        async (authorization) => {
          const callback = new URL(authorization.searchParams.get('redirect_uri')!);
          callback.searchParams.set('code', 'synthetic-desktop-code');
          callback.searchParams.set('state', authorization.searchParams.get('state')!);
          await promisify(execFile)('/usr/bin/open', [callback.href]);
        }
      );
      assert.equal(result.credentials.oauthClientId, 'chatto://desktop');
      assert.equal(result.credentials.token, 'fixture-access');
    } finally {
      globalThis.fetch = original;
    }
  }
);
