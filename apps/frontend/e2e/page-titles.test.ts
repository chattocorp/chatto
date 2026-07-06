import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  verifyAdminEmail,
  type TestUser
} from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { AdminPage } from './pages/AdminPage';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Logs in as the admin user (created by server bootstrap) and verifies
 * the admin email to grant config-based admin access (for admin panel).
 */
async function createAndLoginAdminUser(page: Page): Promise<TestUser> {
  const adminUser = await loginAsAdmin(page);
  await verifyAdminEmail(page, adminUser.id!);
  return adminUser;
}

test.describe('Page titles', () => {
  test('room page has room and server name in title', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const serverName = await chatPage.getServerName();
    await chatPage.enterRoom('general');

    // Wait for room header to be visible (indicates room data is loaded)
    await expect(chatPage.getRoomHeader('general')).toBeVisible();

    // Title: "#room - <server> | <instance>". Post-PR(a) instance name falls
    // back to the server name when no runtime override is configured.
    await expect(page).toHaveTitle(`#general - ${serverName} | ${serverName}`);
  });

  test('room page title uses custom instance name', async ({ page, chatPage }) => {
    // Login as admin and set custom instance name
    await createAndLoginAdminUser(page);
    const adminPage = new AdminPage(page);

    await adminPage.gotoServerSettings();
    await adminPage.fillServerSettings({ serverName: 'Test Server' });
    await adminPage.saveServerSettings();

    // Create account and enter room
    await chatPage.goto();
    const serverName = await chatPage.getServerName();
    await chatPage.enterRoom('general');

    // Wait for room header to be visible
    await expect(chatPage.getRoomHeader('general')).toBeVisible();

    // Title should use custom instance name
    await expect(page).toHaveTitle(`#general - ${serverName} | Test Server`);
  });

  test('page title updates in real-time when instance name changes', async ({
    page,
    browser,
    serverURL
  }) => {
    // Setup: Admin in first browser, regular user in second browser
    await createAndLoginAdminUser(page);
    const adminPage = new AdminPage(page);

    // Set initial instance name
    await adminPage.gotoServerSettings();
    await adminPage.fillServerSettings({ serverName: 'Initial Server' });
    await adminPage.saveServerSettings();

    await withServerUser(browser, serverURL, async ({ page: page2 }) => {
      // Navigate second user to Browse Rooms (accessible to all users)
      await page2.goto(routes.browseRooms);
      // The instance name is fetched asynchronously via server discovery, so wait for it
      await expect(page2).toHaveTitle('Overview | Initial Server', {
        timeout: TIMEOUTS.UI_STANDARD
      });

      // Admin changes instance name
      await adminPage.gotoServerSettings();
      await adminPage.fillServerSettings({ serverName: 'Updated Server' });
      await adminPage.saveServerSettings();

      // Second user's page title should update via live events
      await expect(page2).toHaveTitle('Overview | Updated Server', {
        timeout: TIMEOUTS.UI_STANDARD
      });
    });
  });
});
