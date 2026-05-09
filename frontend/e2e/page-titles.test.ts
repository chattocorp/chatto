import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  verifyAdminEmail,
  type TestUser
} from './fixtures/testUser';
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
  // 'Browse Spaces page has correct title' was retired with the Browse
  // Spaces UI in PR(a).

  test('browse rooms page has correct title', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();

    // Navigate to Browse Rooms page (space context but no specific room)
    await page.getByRole('link', { name: 'Browse Rooms' }).click();
    await page.waitForURL(routes.patterns.browseRooms);

    // Post-#330 PR(a) the instance name falls back to the bootstrap space's
    // name when no runtime override is configured.
    await expect(page).toHaveTitle(`Browse Rooms | ${spaceName}`);
  });

  test('room page has room and space name in title', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Wait for room header to be visible (indicates room data is loaded)
    await expect(chatPage.getRoomHeader('general')).toBeVisible();

    // Title: "#room - <space> | <instance>". Post-PR(a) instance name falls
    // back to the space name when no runtime override is configured.
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);
  });

  test('room title updates when switching pages', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();

    // Enter general room
    await chatPage.enterRoom('general');
    await expect(chatPage.getRoomHeader('general')).toBeVisible();
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);

    // Go back to space index by navigating to browse rooms and back
    await page.getByRole('link', { name: 'Browse Rooms' }).click();
    await expect(page).toHaveTitle(`Browse Rooms | ${spaceName}`);

    // Re-enter general room and verify title is set again
    await chatPage.enterRoom('general');
    await expect(chatPage.getRoomHeader('general')).toBeVisible();
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);
  });

  test('room page title uses custom instance name', async ({ page, chatPage }) => {
    // Login as admin and set custom instance name
    await createAndLoginAdminUser(page);
    const adminPage = new AdminPage(page);

    await adminPage.gotoInstanceSettings();
    await adminPage.fillInstanceSettings({ instanceName: 'Test Server' });
    await adminPage.saveInstanceSettings();

    // Create space and enter room
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Wait for room header to be visible
    await expect(chatPage.getRoomHeader('general')).toBeVisible();

    // Title should use custom instance name
    await expect(page).toHaveTitle(`#general - ${spaceName} | Test Server`);
  });

  test('title reverts cleanly when leaving a room for another page', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();

    // Enter room — title should include room and space name
    await chatPage.enterRoom('general');
    await expect(chatPage.getRoomHeader('general')).toBeVisible();
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);

    // Navigate to Browse Rooms — title should switch to that page's title, not blank
    await page.goto(routes.browseRooms);
    await page.waitForURL(routes.browseRooms);
    await expect(page).toHaveTitle(`Browse Rooms | ${spaceName}`);
  });

  test('title stays correct during rapid navigation between rooms and pages', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();

    // Enter room
    await chatPage.enterRoom('general');
    await expect(chatPage.getRoomHeader('general')).toBeVisible();
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);

    // Navigate to Browse Rooms via sidebar link (SPA navigation)
    await page.getByRole('link', { name: 'Browse Rooms' }).click();
    await expect(page).toHaveTitle(`Browse Rooms | ${spaceName}`);

    // Back to room
    await chatPage.enterRoom('general');
    await expect(chatPage.getRoomHeader('general')).toBeVisible();
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);

    // Full navigation to Browse Rooms verifies title after page load
    await page.goto(routes.browseRooms);
    await expect(page).toHaveTitle(`Browse Rooms | ${spaceName}`);

    // Back to room again
    await page.goBack();
    await expect(chatPage.getRoomHeader('general')).toBeVisible();
    await expect(page).toHaveTitle(`#general - ${spaceName} | ${spaceName}`);
  });

  test('page title updates in real-time when instance name changes', async ({ page, browser }) => {
    // Setup: Admin in first browser, regular user in second browser
    await createAndLoginAdminUser(page);
    const adminPage = new AdminPage(page);

    // Set initial instance name
    await adminPage.gotoInstanceSettings();
    await adminPage.fillInstanceSettings({ instanceName: 'Initial Server' });
    await adminPage.saveInstanceSettings();

    // Create a second browser context for a regular user
    const context2 = await browser.newContext();
    const page2 = await context2.newPage();
    await createAndLoginTestUser(page2);

    // Navigate second user to Browse Rooms (accessible to all users)
    await page2.goto(routes.browseRooms);
    // The instance name is fetched asynchronously via /api/instance, so wait for it
    await expect(page2).toHaveTitle('Browse Rooms | Initial Server', { timeout: TIMEOUTS.UI_STANDARD });

    // Admin changes instance name
    await adminPage.gotoInstanceSettings();
    await adminPage.fillInstanceSettings({ instanceName: 'Updated Server' });
    await adminPage.saveInstanceSettings();

    // Second user's page title should update via live events
    await expect(page2).toHaveTitle('Browse Rooms | Updated Server', { timeout: TIMEOUTS.UI_STANDARD });

    // Clean up
    await context2.close();
  });
});

test.describe('PWA theme-color meta tags', () => {
  test('light mode theme-color matches outer frame background (gray-200)', async ({ page }) => {
    await page.goto('/chat');

    const lightMeta = page.locator('meta[name="theme-color"][media*="light"]');
    await expect(lightMeta).toHaveAttribute('content', '#e5e7eb');
  });

  test('dark mode theme-color matches outer frame background (neutral-800)', async ({ page }) => {
    await page.goto('/chat');

    const darkMeta = page.locator('meta[name="theme-color"][media*="dark"]');
    await expect(darkMeta).toHaveAttribute('content', '#262626');
  });
});
