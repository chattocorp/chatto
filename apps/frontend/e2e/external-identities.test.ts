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

  test('shows immediate feedback while starting provider sign-in', async ({ page, context }) => {
    let releaseProviderRequest: (() => void) | undefined;
    const providerRequestStarted = new Promise<void>((resolve) => {
      context.route('**/auth/providers/github-main?**', async (route) => {
        resolve();
        await new Promise<void>((release) => {
          releaseProviderRequest = release;
        });
        await route.fulfill({ status: 204, body: '' });
      });
    });

    await page.goto(routes.login);

    const githubButton = page.locator('a[href^="/auth/providers/github-main"]').first();
    const discordButton = page.locator('a[href^="/auth/providers/discord-main"]').first();
    await githubButton.click();

    await expect(githubButton).toHaveAttribute('aria-busy', 'true');
    await expect(githubButton).toContainText('Connecting to GitHub...');
    await expect(discordButton).toHaveAttribute('aria-disabled', 'true');
    await expect(page.getByLabel('Username or Email')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeDisabled();

    await providerRequestStarted;
    releaseProviderRequest?.();
  });

  test('signs in through a popup from the origin login page and resumes its destination', async ({
    page,
    context,
    serverURL
  }) => {
    const destination = '/chat/-/settings/account';
    await context.route('**/auth/providers/github-main?**', async (route) => {
      const response = await context.request.post(`${serverURL}/auth/browser/login`, {
        headers: { Origin: serverURL, 'X-Chatto-Authentication-Mode': 'cookie' },
        data: { identifier: 'e2eadmin', password: 'adminpassword123' }
      });
      expect(response.ok()).toBe(true);
      const callback = new URL(route.request().url()).searchParams.get('redirect');
      expect(callback).toContain('/servers/callback?mode=provider&state=');
      await route.fulfill({ status: 302, headers: { Location: callback! } });
    });
    await page.goto(`/login?redirect=${encodeURIComponent(destination)}`);
    const popupPromise = page.waitForEvent('popup');
    await page.getByRole('link', { name: 'Continue with GitHub' }).click();
    const popup = await popupPromise;
    await expect.poll(() => popup.isClosed()).toBe(true);
    await expect(page).toHaveURL(new URL(destination, serverURL).href);
    await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible();
  });

  test('links a provider in a popup from origin account settings without reloading', async ({
    page,
    context,
    authPage
  }) => {
    const user = await createAndLoginTestUser(page, { loginPrefix: 'originpopup' });
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'link',
      providerId: 'discord-main',
      providerType: 'discord',
      providerLabel: 'Discord',
      subject: `discord-popup-${Date.now()}`,
      boundUserId: user.id,
      redirectPath: `/chat/-/settings/account?link_provider=discord-main&link_user=${user.id}&link_complete=1`
    });
    // Model a verified callback that commits the link before returning to settings.
    // The real provider callback is covered by the backend OIDC integration test.
    await context.route('**/auth/providers/discord-main?**', async (route) => {
      const response = await context.request.post(
        '/api/connect/chatto.auth.v1.ExternalIdentityAuthService/ConfirmExternalIdentityLink',
        { data: { token: flow.token } }
      );
      expect(response.ok()).toBe(true);
      await route.fulfill({
        status: 302,
        headers: {
          Location: `/chat/-/settings/account?link_provider=discord-main&link_user=${user.id}&link_complete=1`
        }
      });
    });
    await page.goto(routes.settingsAccount);
    const row = page.locator('div.rounded.border').filter({ hasText: 'Discord' });
    const popupPromise = page.waitForEvent('popup');
    await row.getByRole('button', { name: 'Link', exact: true }).click();
    const popup = await popupPromise;
    await expect.poll(() => popup.isClosed()).toBe(true);
    await expect(row.getByRole('button', { name: 'Disconnect', exact: true })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(routes.settingsAccount + '$'));
  });

  test('creates a provider account in a popup from registration', async ({
    page,
    context,
    authPage
  }) => {
    await context.route('**/auth/providers/github-main?**', async (route) => {
      const redirectPath = new URL(route.request().url()).searchParams.get('redirect')!;
      const flow = await authPage.createExternalIdentityFlow({
        kind: 'create',
        providerId: 'github-main',
        providerType: 'github',
        providerLabel: 'GitHub',
        subject: `create-popup-${Date.now()}`,
        loginHint: 'popupsignup',
        displayNameHint: 'Popup Signup',
        redirectPath
      });
      await route.fulfill({ status: 302, headers: { Location: flow.confirmUrl } });
    });
    await page.goto('/register');
    const popupPromise = page.waitForEvent('popup');
    await page.getByRole('link', { name: 'Continue with GitHub' }).click();
    const popup = await popupPromise;
    await popup.getByRole('button', { name: 'Create Account', exact: true }).click();
    await expect.poll(() => popup.isClosed()).toBe(true);
    await expect(page).toHaveURL(routes.patterns.chatRedirect);
  });

  test('recovers when the user closes the provider sign-in popup', async ({ page, context }) => {
    await context.route('**/auth/providers/github-main?**', (route) =>
      route.fulfill({ contentType: 'text/html', body: '<p>Test provider consent</p>' })
    );
    await page.goto(routes.login);
    const popupPromise = page.waitForEvent('popup');
    await page.getByRole('link', { name: 'Continue with GitHub' }).click();
    const popup = await popupPromise;
    await popup.close();
    await expect(page.getByRole('link', { name: 'Continue with GitHub' })).not.toHaveAttribute(
      'aria-busy',
      'true'
    );
    await expect(page.getByLabel('Username or Email')).toBeEnabled();
    await expect(page).toHaveURL(/\/login$/);
  });

  test('explains unlinked provider sign-in when account creation is disabled', async ({ page }) => {
    await page.goto('/login?error=external_identity_unlinked');

    await expect(
      page.getByText('No account is linked to that provider identity yet.')
    ).toBeVisible();
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
    await expect(page.getByLabel('Display Name')).toHaveValue('GitHub SSO User');
    await page.getByLabel('Display Name').fill('Chosen SSO Name');
    await expect(page.getByRole('link', { name: 'Sign in with existing account' })).toHaveAttribute(
      'href',
      '/login'
    );

    await page.getByRole('button', { name: 'Create Account' }).click();
    await page.waitForURL(routes.patterns.chatRedirect);

    await page.goto(routes.settingsAccount);
    await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible();

    await page.goto(routes.settings);
    await expect(page.getByPlaceholder('Enter your display name')).toHaveValue('Chosen SSO Name');

    await page.goto(routes.settingsAccount);
    await expect(page.getByText('GitHub', { exact: true })).toBeVisible();
    const githubRow = page.locator('div.rounded.border').filter({ hasText: 'GitHub' });
    await expect(githubRow.getByText('Linked')).toBeVisible();

    let disconnectRequestCount = 0;
    await page.route(
      '**/api/connect/chatto.api.v1.MyAccountService/DisconnectExternalIdentity',
      async (route) => {
        disconnectRequestCount++;
        await route.continue();
      }
    );

    await githubRow.getByRole('button', { name: 'Disconnect' }).click();
    await expect(
      page.getByRole('dialog', { name: 'Provider cannot be disconnected' })
    ).toBeVisible();
    await expect(
      page.getByText(
        'GitHub is your only sign-in method. Add a password or link another provider first.'
      )
    ).toBeVisible();
    expect(disconnectRequestCount).toBe(0);
    await page.getByRole('dialog').getByText('Close').click();
    await expect(githubRow.getByText('Linked')).toBeVisible();
    await expect(githubRow.locator('[class~="icon-[uil--link-broken]"]')).toBeVisible();

    await page.getByLabel('New Password').fill('newpassword456');
    await page.getByLabel('Confirm Password').fill('newpassword456');
    await page.getByRole('button', { name: 'Add password' }).click();
    await expect(page.getByLabel('Current Password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Change password' })).toBeVisible();

    await githubRow.getByRole('button', { name: 'Disconnect' }).click();
    await expect(page.getByRole('dialog', { name: 'Disconnect provider' })).toBeVisible();
    await expect(page.getByText('Disconnect GitHub from this account?')).toBeVisible();
    await page.getByRole('dialog').getByRole('button', { name: 'Disconnect' }).click();
    await expect(githubRow.getByRole('button', { name: 'Link', exact: true })).toBeVisible();
    await page.reload();
    await expect(githubRow.getByRole('button', { name: 'Link', exact: true })).toBeVisible();
  });

  test('can leave create confirmation to sign in without cancelling the pending flow', async ({
    page,
    authPage
  }) => {
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'create',
      providerId: 'github-main',
      providerType: 'github',
      providerLabel: 'GitHub',
      subject: `github-existing-${Date.now()}`,
      loginHint: 'existing-sso'
    });

    await page.goto(flow.confirmUrl);
    await page.getByRole('link', { name: 'Sign in with existing account' }).click();
    await page.waitForURL(routes.login);

    await page.goto(flow.confirmUrl);
    await expect(page.getByText('GitHub verified your identity')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Account' })).toBeVisible();
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
    const discordRow = page.locator('div.rounded.border').filter({ hasText: 'Discord' });
    const githubRow = page.locator('div.rounded.border').filter({ hasText: 'GitHub' });
    await expect(discordRow.getByText('Linked')).toBeVisible();

    let releaseDisconnectRequest: (() => void) | undefined;
    const disconnectRequestStarted = new Promise<void>((resolve) => {
      page.route(
        '**/api/connect/chatto.api.v1.MyAccountService/DisconnectExternalIdentity',
        async (route) => {
          resolve();
          await new Promise<void>((release) => {
            releaseDisconnectRequest = release;
          });
          await route.continue();
        }
      );
    });

    const disconnectButton = discordRow.getByRole('button', { name: 'Disconnect' });
    await expect(disconnectButton).toHaveClass(/btn-danger-secondary/);
    await disconnectButton.click();
    await expect(page.getByRole('dialog', { name: 'Disconnect provider' })).toBeVisible();
    await page.getByRole('dialog').getByRole('button', { name: 'Disconnect' }).click();
    await disconnectRequestStarted;
    const disconnectingButton = discordRow.getByRole('button', { name: 'Disconnecting...' });
    await expect(disconnectingButton).toBeDisabled();
    await expect(disconnectingButton).toHaveAttribute('aria-busy', 'true');
    await expect(githubRow.getByRole('button', { name: 'Link' })).toBeDisabled();
    releaseDisconnectRequest?.();

    await expect(discordRow.getByRole('button', { name: 'Link', exact: true })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(routes.settingsAccount + '$'));
    await page.reload();
    await expect(discordRow.getByRole('button', { name: 'Link', exact: true })).toBeVisible();
  });

  test('disconnects a linked identity for an unconfigured provider', async ({ page, authPage }) => {
    const user = await createAndLoginTestUser(page, { loginPrefix: 'ssoretired' });
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'link',
      providerId: 'retired-provider',
      providerType: 'github',
      providerLabel: 'Retired Provider',
      subject: `retired-${Date.now()}`,
      boundUserId: user.id
    });

    await page.goto(flow.confirmUrl);
    await page.getByRole('button', { name: 'Link Account' }).click();
    await page.waitForURL(routes.patterns.chatRedirect);

    await page.goto(routes.settingsAccount);
    const retiredRow = page.locator('div.rounded.border').filter({ hasText: 'retired-provider' });
    await expect(retiredRow.getByText('Provider no longer configured')).toBeVisible();

    await retiredRow.getByRole('button', { name: 'Disconnect' }).click();
    await expect(page.getByRole('dialog', { name: 'Disconnect provider' })).toBeVisible();
    await expect(page.getByText('Disconnect retired-provider from this account?')).toBeVisible();
    await page.getByRole('dialog').getByRole('button', { name: 'Disconnect' }).click();
    await expect(retiredRow).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(routes.settingsAccount + '$'));
  });

  test('confirms a link token through the public flow service', async ({ page, authPage }) => {
    const owner = await authPage.createUserViaApi(`ssoowner${Date.now()}`, 'testpassword123');
    const flow = await authPage.createExternalIdentityFlow({
      kind: 'link',
      providerId: 'discord-main',
      providerType: 'discord',
      providerLabel: 'Discord',
      subject: `discord-public-${Date.now()}`,
      boundUserId: owner.id
    });

    const confirmRequestStarted = new Promise<void>((resolve) => {
      page.route(
        '**/api/connect/chatto.auth.v1.ExternalIdentityAuthService/ConfirmExternalIdentityLink',
        async (route) => {
          resolve();
          await route.continue();
        }
      );
    });
    await page.goto(flow.confirmUrl);
    await page.getByRole('button', { name: 'Link Account' }).click();
    await confirmRequestStarted;
    await page.waitForURL(routes.login);
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
