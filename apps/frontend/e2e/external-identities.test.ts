import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import * as routes from './routes';

test.describe('External identity confirmation flows', () => {
  test.use({
    serverOptions: {
      env: {
        CHATTO_AUTH_PROVIDERS_0_ID: 'github-main',
        CHATTO_AUTH_PROVIDERS_0_TYPE: 'github',
        CHATTO_AUTH_PROVIDERS_0_LABEL: 'GitHub',
        CHATTO_AUTH_PROVIDERS_0_CLIENT_ID: 'github-client',
        CHATTO_AUTH_PROVIDERS_0_CLIENT_SECRET: 'github-secret',
        CHATTO_AUTH_PROVIDERS_0_AUTO_PROVISION: 'true',
        CHATTO_AUTH_PROVIDERS_1_ID: 'discord-main',
        CHATTO_AUTH_PROVIDERS_1_TYPE: 'discord',
        CHATTO_AUTH_PROVIDERS_1_LABEL: 'Discord',
        CHATTO_AUTH_PROVIDERS_1_CLIENT_ID: 'discord-client',
        CHATTO_AUTH_PROVIDERS_1_CLIENT_SECRET: 'discord-secret'
      }
    }
  });

  test('creates an account from a pending provider identity after explicit confirmation', async ({
    page,
    authPage
  }) => {
    const stamp = Date.now();
    const login = `sso${stamp}`;
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'create',
      providerId: 'github-main',
      providerType: 'github',
      providerLabel: 'GitHub',
      subject: `github-${stamp}`,
      verifiedEmail: `${login}@example.test`,
      loginHint: login,
      displayNameHint: 'GitHub SSO User'
    });

    await page.goto(flow.confirmUrl);
    await expect(page.getByRole('heading', { name: 'Confirm Sign-In' })).toBeVisible();
    await expect(page.getByText('GitHub verified your identity')).toBeVisible();
    await expect(page.getByLabel('Username')).toHaveValue(login);

    await page.getByRole('button', { name: 'Create Account' }).click();
    await page.waitForURL(routes.patterns.chatRedirect);

    await page.goto(routes.settingsAccount);
    await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible();
    await expect(page.getByText('GitHub', { exact: true })).toBeVisible();
    await expect(page.getByText('Linked').first()).toBeVisible();
  });

  test('links a pending provider identity to the authenticated account', async ({
    page,
    authPage
  }) => {
    const user = await createAndLoginTestUser(page, { loginPrefix: 'ssolink' });
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'link',
      providerId: 'discord-main',
      providerType: 'discord',
      providerLabel: 'Discord',
      subject: `discord-${Date.now()}`,
      loginHint: user.login,
      displayNameHint: user.displayName,
      boundUserId: user.id
    });

    await page.goto(flow.confirmUrl);
    await expect(page.getByRole('heading', { name: 'Confirm Sign-In' })).toBeVisible();
    await expect(page.getByText('Discord verified your identity')).toBeVisible();

    await page.getByRole('button', { name: 'Link Account' }).click();
    await page.waitForURL(routes.patterns.chatRedirect);

    await page.goto(routes.settingsAccount);
    await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible();
    await expect(page.getByText('Discord', { exact: true })).toBeVisible();
    await expect(page.getByText('Linked').first()).toBeVisible();
  });

  test('rejects a link token bound to a different user', async ({ page, authPage }) => {
    const owner = await authPage.createUserViaApi(`ssoowner${Date.now()}`, 'testpassword123');
    await createAndLoginTestUser(page, { loginPrefix: 'ssowrong' });
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'link',
      providerId: 'discord-main',
      providerType: 'discord',
      providerLabel: 'Discord',
      subject: `discord-wrong-${Date.now()}`,
      boundUserId: owner.id
    });

    await page.goto(flow.confirmUrl);
    await page.getByRole('button', { name: 'Link Account' }).click();
    await expect(page.getByText('bound to a different user')).toBeVisible();
  });

  test('cancels a pending provider identity flow', async ({ page, authPage }) => {
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'create',
      providerId: 'github-main',
      providerType: 'github',
      providerLabel: 'GitHub',
      subject: `github-cancel-${Date.now()}`,
      loginHint: 'cancelled-sso'
    });

    await page.goto(flow.confirmUrl);
    await page.getByRole('button', { name: 'Cancel' }).click();
    await page.waitForURL(routes.login);

    await page.goto(flow.confirmUrl);
    await expect(page.getByText('This sign-in link is invalid or has expired.')).toBeVisible();
  });
});
