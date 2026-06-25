import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import * as routes from './routes';

/**
 * Test for direct navigation to the Browse Rooms page.
 *
 * When navigating directly to the room directory URL (e.g., by typing the URL
 * or refreshing the page), the page should load correctly and NOT show
 * "Access Denied". Previously there was a race condition where the page
 * would read permissions before the parent layout had finished loading.
 */
test.describe('Browse Rooms direct navigation', () => {
  test('direct navigation to Browse Rooms shows room directory, not Access Denied', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create account (user gets default admin role with browse permission)

    // Navigate directly to the Browse Rooms page by URL
    // This is the key test - direct navigation should work
    await page.goto(routes.browseRooms);

    // Should show the Browse Rooms heading, NOT "Access Denied"
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
    await expect(page.getByText('Access Denied')).not.toBeVisible();
  });

  test('clicking Browse Rooms link also works (baseline)', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create account

    // Click the Overview link in the sidebar (which hosts the room
    // directory now that Browse Rooms has been folded in).
    await page.getByRole('link', { name: 'Overview' }).click();
    await page.waitForURL(/\/chat\/-\/overview$/);

    // Should show the Browse Rooms heading
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
    await expect(page.getByText('Access Denied')).not.toBeVisible();
  });

  test('refresh on Browse Rooms page does not show Access Denied', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create account and navigate to Browse Rooms via link
    await page.getByRole('link', { name: 'Overview' }).click();
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();

    // Refresh the page - should still work
    await page.reload();

    // Should still show the Browse Rooms heading after refresh
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
    await expect(page.getByText('Access Denied')).not.toBeVisible();
  });
});
