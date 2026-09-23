import { test as base, expect } from '@playwright/test';
import { startStack, stopStack, type TestStack } from '../../../authling/e2e/fixtures/stack';
import { waitForVerificationCode } from '../../../authling/e2e/fixtures/mailpit';
import { serverBaseURLForTest, startServer, stopServer, type ServerInfo } from './fixtures/server';
import * as routes from './routes';

const test = base.extend<{ authling: TestStack; server: ServerInfo }>({
  authling: async ({}, use, testInfo) => {
    const chattoURL = serverBaseURLForTest(testInfo, { hostname: '127.0.0.1' });
    const stack = await startStack(testInfo, {
      additionalConfig: `\n[[oidc.clients]]\nid = '${chattoURL}/oauth/client-metadata.json'\nname = 'Chatto E2E'\nredirect_uris = ['${chattoURL}/auth/providers/authling/callback']\n`
    });
    try {
      await use(stack);
    } finally {
      await stopStack(stack, testInfo);
    }
  },
  server: async ({ authling }, use, testInfo) => {
    const chattoURL = serverBaseURLForTest(testInfo, { hostname: '127.0.0.1' });
    const server = await startServer(testInfo, {
      hostname: '127.0.0.1',
      env: {
        CHATTO_AUTH_PROVIDERS_0_ID: 'authling',
        CHATTO_AUTH_PROVIDERS_0_TYPE: 'oidc',
        CHATTO_AUTH_PROVIDERS_0_LABEL: 'Authling',
        CHATTO_AUTH_PROVIDERS_0_ISSUER_URL: authling.baseURL,
        CHATTO_AUTH_PROVIDERS_0_CLIENT_ID: `${chattoURL}/oauth/client-metadata.json`,
        CHATTO_AUTH_PROVIDERS_0_AUTO_PROVISION: 'true'
      }
    });
    try {
      await use(server);
    } finally {
      await stopServer(server, testInfo);
    }
  },
  baseURL: async ({ server }, use) => {
    await use(server.baseURL);
  }
});

test.setTimeout(60_000);

test('transfers an editable Authling profile into a new Chatto account', async ({
  page,
  request,
  authling
}) => {
  const email = `authling-chatto-${Date.now()}@example.invalid`;
  const password = 'correct horse battery staple';
  const preferredUsername = `authling-${Date.now()}`;
  const authlingName = 'Authling Profile Name';
  const chattoName = 'Chosen Chatto Name';

  await page.goto(new URL('/signup', authling.baseURL).toString());
  await page.getByLabel('Email address').fill(email);
  await page.getByRole('button', { name: 'Email me a code' }).click();
  const code = await waitForVerificationCode(request, authling.mailpitURL);
  await page.getByLabel('Verification code').fill(code);
  await page.getByRole('button', { name: 'Verify email' }).click();
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByLabel('Confirm password').fill(password);
  await page.getByRole('button', { name: 'Create account' }).click();
  await page.getByRole('link', { name: 'Edit profile' }).click();
  await page.getByLabel('Preferred username').fill(preferredUsername);
  await page.getByLabel('Full name').fill(authlingName);
  await page.getByRole('button', { name: 'Save profile' }).click();
  await page.getByRole('button', { name: 'Sign out' }).click();

  await page.goto(routes.login);
  const popupPromise = page.waitForEvent('popup');
  await page.getByRole('link', { name: /Authling/ }).click();
  const popup = await popupPromise;
  await popup.getByLabel('Email address').fill(email);
  await popup.getByLabel('Password').fill(password);
  await popup.getByRole('button', { name: 'Sign in' }).click();
  await popup.getByRole('button', { name: 'Authorize' }).click();

  await expect(popup.getByRole('heading', { name: 'Confirm Sign-In' })).toBeVisible();
  await expect(popup.getByLabel('Username')).toHaveValue(preferredUsername);
  await expect(popup.getByLabel('Display Name')).toHaveValue(authlingName);
  await popup.getByLabel('Display Name').fill(chattoName);
  const closed = popup.waitForEvent('close');
  await popup.getByRole('button', { name: 'Create Account' }).click();
  await closed;
  await page.waitForURL(routes.patterns.chatRedirect);

  await page.goto(routes.settings);
  await expect(page.getByPlaceholder('Enter your display name')).toHaveValue(chattoName);
  await expect(page.getByPlaceholder('Enter your username')).toHaveValue(preferredUsername);
});

test('links a remote identity in one popup and refreshes the original client', async ({
  browser,
  page,
  request,
  authling,
  server
}, testInfo) => {
  const { createUserOnRemote } = await import('./fixtures/multiServer');
  const clientServer = await startServer(testInfo, { instanceId: 'link-client', portOffset: 5 });
  const clientContext = await browser.newContext({ baseURL: clientServer.baseURL });
  const clientPage = await clientContext.newPage();
  const errors: string[] = [];
  clientPage.on('pageerror', (error) => errors.push(error.message));
  try {
    const password = 'correct horse battery staple';
    const email = `remote-link-${Date.now()}@example.invalid`;
    await page.goto(new URL('/signup', authling.baseURL).href);
    await page.getByLabel('Email address').fill(email);
    await page.getByRole('button', { name: 'Email me a code' }).click();
    await page
      .getByLabel('Verification code')
      .fill(await waitForVerificationCode(request, authling.mailpitURL));
    await page.getByRole('button', { name: 'Verify email' }).click();
    await page.getByLabel('Password', { exact: true }).fill(password);
    await page.getByLabel('Confirm password').fill(password);
    await page.getByRole('button', { name: 'Create account' }).click();
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();

    await createUserOnRemote(clientServer.baseURL, 'local-link-user', password);
    await createUserOnRemote(server.baseURL, 'remote-link-user', password);
    await clientPage.goto('/login');
    await clientPage.getByLabel('Username or Email').fill('local-link-user');
    await clientPage.getByLabel('Password', { exact: true }).fill(password);
    await clientPage.getByRole('button', { name: 'Sign In', exact: true }).click();
    await expect(clientPage).toHaveURL(/\/chat\//);

    // Establish a real delegated OAuth session for the remote account.
    await clientPage.getByTitle('Add Server').click();
    await clientPage.getByLabel('Server URL').fill(server.baseURL);
    await clientPage.getByRole('button', { name: 'Find server' }).click();
    const authPopupPromise = clientPage.waitForEvent('popup');
    await clientPage.getByRole('button', { name: 'Join', exact: true }).click();
    const authPopup = await authPopupPromise;
    await authPopup.getByLabel('Username or Email').fill('remote-link-user');
    await authPopup.getByLabel('Password', { exact: true }).fill(password);
    await authPopup.getByRole('button', { name: 'Sign In', exact: true }).click();
    const authClosed = authPopup.waitForEvent('close');
    await authPopup.getByRole('button', { name: 'Allow Access' }).click();
    await authClosed;
    await expect(clientPage).toHaveURL(/\/chat\/127\.0\.0\.1/);

    // Remove only the remote browser cookie, leaving the main client's OAuth
    // session intact. This exercises continuation through first-party sign-in.
    await clientContext.clearCookies({ domain: '127.0.0.1' });
    await clientPage.goto('/chat/127.0.0.1/settings/account');
    const remoteRow = clientPage.locator('div.rounded.border').filter({ hasText: 'Authling' });
    const linkPopupPromise = clientPage.waitForEvent('popup');
    await remoteRow.getByRole('button', { name: 'Link', exact: true }).click();
    const linkPopup = await linkPopupPromise;
    linkPopup.on('pageerror', (error) => errors.push(error.message));
    await expect(linkPopup).toHaveURL(new RegExp(`${new URL(server.baseURL).host}/login`));
    expect(await linkPopup.evaluate(() => window.opener === null)).toBe(true);
    await linkPopup.getByLabel('Username or Email').fill('remote-link-user');
    await linkPopup.getByLabel('Password', { exact: true }).fill(password);
    await linkPopup.getByRole('button', { name: 'Sign In', exact: true }).click();

    // The same popup completes real provider authorization and closes itself.
    await linkPopup.getByLabel('Email address').fill(email);
    await linkPopup.getByLabel('Password', { exact: true }).fill(password);
    await linkPopup.getByRole('button', { name: 'Sign in', exact: true }).click();
    const linkClosed = linkPopup.waitForEvent('close');
    await linkPopup.getByRole('button', { name: 'Authorize' }).click();
    await linkClosed;
    await expect(remoteRow.getByRole('button', { name: 'Disconnect' })).toBeVisible();
    await expect(clientPage).toHaveURL(/\/chat\/127\.0\.0\.1\/settings\/account$/);
    expect(errors).toEqual([]);
  } finally {
    await clientContext.close();
    await stopServer(clientServer, testInfo);
  }
});
