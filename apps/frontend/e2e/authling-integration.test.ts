import { test as base, expect } from '@playwright/test';
import { startStack, stopStack, type TestStack } from '../../../authling/e2e/fixtures/stack';
import { waitForVerificationCode } from '../../../authling/e2e/fixtures/mailpit';
import {
  serverBaseURLForTest,
  startServer,
  stopServer,
  type ServerInfo
} from './fixtures/server';
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
        CHATTO_AUTH_PROVIDERS_0_SCOPES: 'openid',
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
  await page.getByRole('link', { name: /Authling/ }).click();
  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Authorize' }).click();

  await expect(page.getByRole('heading', { name: 'Confirm Sign-In' })).toBeVisible();
  await expect(page.getByLabel('Username')).toHaveValue(preferredUsername);
  await expect(page.getByLabel('Display Name')).toHaveValue(authlingName);
  await page.getByLabel('Display Name').fill(chattoName);
  await page.getByRole('button', { name: 'Create Account' }).click();
  await page.waitForURL(routes.patterns.chatRedirect);

  await page.goto(routes.settings);
  await expect(page.getByPlaceholder('Enter your display name')).toHaveValue(chattoName);
  await expect(page.getByPlaceholder('Enter your username')).toHaveValue(preferredUsername);
});
