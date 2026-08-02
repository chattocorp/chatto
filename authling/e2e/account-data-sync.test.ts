import { createHash, randomUUID } from 'node:crypto';
import path from 'node:path';
import { completeSignup } from './fixtures/signup';
import { restartAuthling } from './fixtures/stack';
import { expect, test } from './setup';

const password = 'correct horse battery staple';
const clientBundle = path.resolve('e2e/.generated/tinybase-client.js');

test('syncs account data across devices, offline edits, deletion, and restart', async ({
  browser,
  page,
  request,
  stack
}, testInfo) => {
  const email = `data-${randomUUID()}@example.invalid`;
  await page.context().addInitScript({ path: clientBundle });
  await completeSignup(page, request, stack, email, password);

  const secondContext = await browser.newContext({ baseURL: stack.baseURL });
  await secondContext.addInitScript({ path: clientBundle });
  const secondPage = await secondContext.newPage();
  try {
    await secondPage.goto('/login');
    await secondPage.getByLabel('Email address').fill(email);
    await secondPage.getByLabel('Password').fill(password);
    await secondPage.getByRole('button', { name: 'Sign in' }).click();
    await expect(secondPage.getByRole('heading', { name: 'Your account' })).toBeVisible();

    await page.evaluate(async () => {
      await authlingTinyBase.create('first', 'browser-device-a');
      authlingTinyBase.setRow('first', 'servers', 'one', {
        name: 'First server',
        url: 'https://one.example'
      });
      authlingTinyBase.setValue('first', 'preferences', {
        nested: { __authling_tinybase_undefined: true },
        reserved: '\uFFFC'
      });
      await authlingTinyBase.connect('first');
    });
    await secondPage.evaluate(async () => {
      await authlingTinyBase.create('second', 'browser-device-b');
      await authlingTinyBase.connect('second');
    });
    await expect
      .poll(() =>
        secondPage.evaluate(() => authlingTinyBase.getCell('second', 'servers', 'one', 'name'))
      )
      .toBe('First server');
    await expect
      .poll(() => secondPage.evaluate(() => authlingTinyBase.getValue('second', 'preferences')))
      .toEqual({ nested: { __authling_tinybase_undefined: true }, reserved: '\uFFFC' });

    await page.evaluate(async () => {
      await authlingTinyBase.disconnect('first');
      authlingTinyBase.setValue('first', 'theme', 'light');
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
    const beforeDark = await secondPage.evaluate(() => authlingTinyBase.syncStats('second'));
    await secondPage.evaluate(() => authlingTinyBase.setValue('second', 'theme', 'dark'));
    await expect
      .poll(async () => {
        const current = await secondPage.evaluate(() => authlingTinyBase.syncStats('second'));
        return current.sends > beforeDark.sends && current.receives > beforeDark.receives;
      })
      .toBe(true);
    await page.evaluate(() => authlingTinyBase.reconnect('first'));
    await expect
      .poll(async () => {
        const [firstTheme, secondTheme] = await Promise.all([
          page.evaluate(() => authlingTinyBase.getValue('first', 'theme')),
          secondPage.evaluate(() => authlingTinyBase.getValue('second', 'theme'))
        ]);
        return firstTheme === secondTheme && (firstTheme === 'light' || firstTheme === 'dark');
      })
      .toBe(true);

    await restartAuthling(stack, testInfo);
    await Promise.all([
      page.evaluate(() => authlingTinyBase.reconnect('first')),
      secondPage.evaluate(() => authlingTinyBase.reconnect('second'))
    ]);
    await expect
      .poll(() =>
        secondPage.evaluate(() => authlingTinyBase.getCell('second', 'servers', 'one', 'url'))
      )
      .toBe('https://one.example');
    await expect
      .poll(() => page.evaluate(() => authlingTinyBase.getValue('first', 'preferences')))
      .toEqual({ nested: { __authling_tinybase_undefined: true }, reserved: '\uFFFC' });

    await secondPage.evaluate(() => authlingTinyBase.delRow('second', 'servers', 'one'));
    await expect
      .poll(() => page.evaluate(() => authlingTinyBase.hasRow('first', 'servers', 'one')))
      .toBe(false);
  } finally {
    await secondContext.close();
  }
});

test('syncs global account data from an explicitly authorized OIDC client origin', async ({
  page,
  request,
  stack
}) => {
  await page.context().addInitScript({ path: clientBundle });
  await completeSignup(
    page,
    request,
    stack,
    `oidc-data-${randomUUID()}@example.invalid`,
    password
  );

  await page.evaluate(async () => {
    await authlingTinyBase.create('session', 'session-device');
    authlingTinyBase.setValue('session', 'theme', 'midnight');
    await authlingTinyBase.connect('session');
  });

  const clientPage = await page.context().newPage();
  const verifier = 'playwright-verifier-with-at-least-forty-three-characters';
  const challenge = createHash('sha256').update(verifier).digest('base64url');
  const authorize = new URL('/oauth/authorize', stack.baseURL);
  authorize.search = new URLSearchParams({
    client_id: 'authling-e2e',
    redirect_uri: stack.callbackURL,
    response_type: 'code',
    scope: 'openid account_data',
    state: 'account-data-state',
    nonce: 'account-data-nonce',
    code_challenge: challenge,
    code_challenge_method: 'S256'
  }).toString();

  await clientPage.goto(authorize.toString());
  await expect(
    clientPage.getByText('Read and change your private global account data', { exact: false })
  ).toBeVisible();
  await clientPage.getByRole('button', { name: 'Authorize' }).click();
  await clientPage.waitForURL(new RegExp(`^${stack.callbackURL.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\?`));
  const callback = new URL(clientPage.url());
  expect(callback.searchParams.get('state')).toBe('account-data-state');

  const tokenResponse = await request.post(`${stack.baseURL}/oauth/token`, {
    form: {
      grant_type: 'authorization_code',
      client_id: 'authling-e2e',
      redirect_uri: stack.callbackURL,
      code: callback.searchParams.get('code') ?? '',
      code_verifier: verifier
    }
  });
  expect(tokenResponse.status()).toBe(200);
  const tokens = (await tokenResponse.json()) as { access_token: string };

  await clientPage.evaluate(
    async ({ endpoint, accessToken }) => {
      await authlingTinyBase.create('oidc', 'oidc-device');
      await authlingTinyBase.connectWithAccessToken('oidc', endpoint, accessToken);
    },
    { endpoint: `${stack.baseURL}/data/sync`, accessToken: tokens.access_token }
  );
  await expect
    .poll(() => clientPage.evaluate(() => authlingTinyBase.getValue('oidc', 'theme')))
    .toBe('midnight');

  await clientPage.evaluate(() => authlingTinyBase.setValue('oidc', 'density', 'compact'));
  await expect
    .poll(() => page.evaluate(() => authlingTinyBase.getValue('session', 'density')))
    .toBe('compact');
});
