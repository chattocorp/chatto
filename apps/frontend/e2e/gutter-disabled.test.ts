// SPDX-License-Identifier: Apache-2.0

import { test, expect } from './setup';
import { startServer, stopServer, type ServerInfo } from './fixtures/server';
import { withServerUser } from './fixtures/serverUser';

test.describe('Bundled frontend without the Server Gutter', () => {
  test.setTimeout(90_000);

  let server: ServerInfo | undefined;

  test.beforeEach(async ({}, testInfo) => {
    server = await startServer(testInfo, {
      instanceId: 'single-server',
      portOffset: 5,
      env: { CHATTO_FRONTEND_GUTTER_ENABLED: 'false' }
    });
  });

  test.afterEach(async ({}, testInfo) => {
    if (server) await stopServer(server, testInfo);
  });

  test('hides the Server Gutter and the Server Directory', async ({ browser }) => {
    await withServerUser(browser, server!.baseURL, async ({ page }) => {
      await expect(page.getByTestId('server-sidebar')).toBeVisible();
      await expect(page.getByTestId('server-list')).toHaveCount(0);

      await page.goto('/chat/servers');
      await expect(page).not.toHaveURL(/\/chat\/servers/);
      await expect(page.getByTestId('server-sidebar')).toBeVisible();
    });
  });

  test('signs out from the server menu in the sidebar header', async ({ browser }) => {
    await withServerUser(browser, server!.baseURL, async ({ page }) => {
      const signOut = page.getByTestId('server-sign-out');
      await expect(async () => {
        if (!(await signOut.isVisible()))
          await page.getByRole('button', { name: /^Actions for / }).click();
        await expect(signOut).toBeVisible({ timeout: 1000 });
      }).toPass({ timeout: 15_000 });
      await signOut.click();
      await expect(page).toHaveURL(/\/login/);
    });
  });

  test('signs out from the settings navigation', async ({ browser }) => {
    await withServerUser(browser, server!.baseURL, async ({ page }) => {
      await page.goto('/chat/-/settings/account');
      await page.getByTestId('sidebar-nav-action-sign-out').click();
      await expect(page).toHaveURL(/\/login/);
    });
  });
});
