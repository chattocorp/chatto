import { test, expect } from './setup';
import { startServer, stopServer } from './fixtures/server';
import { connectRemoteInstance, createUserOnRemote } from './fixtures/multiServer';
import { createAndLoginTestUser } from './fixtures/testUser';

for (const signedInAtOrigin of [false, true]) {
  test(`remote identity linking closes its popup with origin signed in: ${signedInAtOrigin}`, async ({
    page,
    context
  }, testInfo) => {
    const remote = await startServer(testInfo, {
      instanceId: 'identity-link',
      portOffset: 5,
      hostname: '127.0.0.1',
      env: {
        CHATTO_AUTH_PROVIDERS_0_ID: 'discord-main',
        CHATTO_AUTH_PROVIDERS_0_TYPE: 'discord',
        CHATTO_AUTH_PROVIDERS_0_LABEL: 'Discord',
        CHATTO_AUTH_PROVIDERS_0_CLIENT_ID: 'test-client',
        CHATTO_AUTH_PROVIDERS_0_CLIENT_SECRET: 'test-secret'
      }
    });
    try {
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      if (signedInAtOrigin) await createAndLoginTestUser(page);
      const user = await createUserOnRemote(remote.baseURL, 'link-user', 'testpassword123');
      await connectRemoteInstance(page, remote, user.userId);
      await page.goto('/chat/127.0.0.1/settings/account');
      const mainURL = page.url();

      // Model provider authorization and callback completion. The link write and
      // subsequent account reads use the real remote server and its event log.
      await context.route(`${remote.baseURL}/auth/providers/discord-main?**`, async (route) => {
        const response = await context.request.post(
          `${remote.baseURL}/auth/test/external-identity-flow`,
          {
            data: {
              kind: 'link',
              providerId: 'discord-main',
              providerType: 'discord',
              providerLabel: 'Discord',
              subject: 'test-discord-subject',
              boundUserId: user.userId,
              redirectPath: `/chat/-/settings/account?link_provider=discord-main&link_user=${user.userId}&link_complete=1`
            }
          }
        );
        expect(response.ok()).toBe(true);
        const flow = await response.json();
        const confirmation = await context.request.post(
          `${remote.baseURL}/api/connect/chatto.auth.v1.ExternalIdentityAuthService/ConfirmExternalIdentityLink`,
          { data: { token: flow.token } }
        );
        expect(confirmation.ok()).toBe(true);
        await route.fulfill({
          status: 302,
          headers: {
            Location: `/chat/-/settings/account?link_provider=discord-main&link_user=${user.userId}&link_complete=1`
          }
        });
      });

      const popupPromise = page.waitForEvent('popup');
      await page.getByRole('button', { name: 'Link', exact: true }).click();
      const popup = await popupPromise;
      popup.on('pageerror', (error) => errors.push(error.message));
      await popup.getByLabel('Username or Email').fill('link-user');
      await popup.getByLabel('Password', { exact: true }).fill('testpassword123');
      await popup.getByRole('button', { name: 'Sign In', exact: true }).click();

      // Do not focus, reload, or navigate the main page to trigger reconciliation.
      await expect(page.getByRole('button', { name: 'Disconnect', exact: true })).toBeVisible();
      expect(errors).toEqual([]);
      await expect.poll(() => popup.isClosed()).toBe(true);
      expect(page.url()).toBe(mainURL);

      await page.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await page
        .getByRole('dialog')
        .getByRole('button', { name: 'Disconnect', exact: true })
        .click();
      await page.locator('#sso-disconnect-current-password').fill('testpassword123');
      await page
        .getByRole('dialog')
        .getByRole('button', { name: 'Disconnect', exact: true })
        .click();
      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Link', exact: true })).toBeVisible();
      await page.reload();
      await expect(page.getByRole('button', { name: 'Link', exact: true })).toBeVisible();
      expect(page.url()).toBe(mainURL);
    } finally {
      await stopServer(remote, testInfo);
    }
  });
}
