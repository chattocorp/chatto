import assert from 'node:assert/strict';
import test from 'node:test';
import { createBrowserAuthorization } from './browser_authorization.mjs';

const redirect = 'chatto://desktop/servers/callback?mode=popup';
const origin = 'https://chat.example';
function authorize(state) {
  return (
    origin +
    '/oauth/authorize?' +
    new URLSearchParams({
      client_id: 'chatto://desktop',
      redirect_uri: redirect,
      state
    })
  );
}

test('routes each OS callback once to the corresponding browser request', async () => {
  const opened = [];
  const auth = createBrowserAuthorization({
    register: () => true,
    openExternal: async (url) => opened.push(url)
  });
  const first = auth.start(origin, authorize('one'));
  const second = auth.start(origin, authorize('two'));
  await new Promise(setImmediate);
  assert.deepEqual(opened, [authorize('one'), authorize('two')]);
  const callback = redirect + '&state=two&code=sample';
  for (const invalid of [
    callback.replace('/callback', '/other'),
    callback.replace('desktop', 'other'),
    callback + '&state=one',
    callback + '&mode=popup',
    callback.replace('two', 'unknown')
  ])
    assert.equal(auth.accept(invalid), false);
  assert.equal(auth.accept(callback), true);
  assert.equal(await second, callback);
  assert.equal(auth.accept(callback), false);
  auth.cancel('one');
  await assert.rejects(first);
  assert.equal(auth.accept(redirect + '&state=one&code=late'), false);
});

test('rejects unsafe launch targets before contacting the OS', async () => {
  const auth = createBrowserAuthorization({ register: assert.fail, openExternal: assert.fail });
  for (const url of [
    'file:///oauth',
    authorize('one').replace('https:', 'http:'),
    authorize('one').replace('chat.example', 'elsewhere.example')
  ])
    await assert.rejects(auth.start(origin, url), /Invalid browser sign-in request/);
});

test('returns a safe registration reason without opening the browser', async () => {
  for (const [platform, registration, expected] of [
    ['win32', () => false, 'callback-registration'],
    ['darwin', () => false, 'install-in-applications'],
    [
      'linux',
      () => {
        throw new Error('private path');
      },
      'callback-registration'
    ],
    [
      'linux',
      () => {
        throw Object.assign(new Error('private path'), {
          code: 'ENOENT',
          syscall: 'spawn xdg-mime'
        });
      },
      'missing-xdg-mime'
    ]
  ]) {
    const auth = createBrowserAuthorization({
      platform,
      register: registration,
      openExternal: assert.fail
    });
    assert.deepEqual(await auth.start(origin, authorize('pending')), { error: expected });
    assert.equal(auth.accept(redirect + '&state=pending&code=late'), false);
  }
});

test('cancellation during registration prevents a late browser launch', async () => {
  const registration = Promise.withResolvers();
  const opened = [];
  const auth = createBrowserAuthorization({
    register: () => registration.promise,
    openExternal: (url) => opened.push(url)
  });
  const waiting = auth.start(origin, authorize('cancelled'));
  auth.cancel('cancelled');
  await assert.rejects(waiting);
  registration.resolve(true);
  await new Promise(setImmediate);
  assert.deepEqual(opened, []);
});
