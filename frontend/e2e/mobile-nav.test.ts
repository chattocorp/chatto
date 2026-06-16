import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, loginAsAdmin, verifyAdminEmail } from './fixtures/testUser';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

test.describe('Mobile Navigation', () => {
  test('hamburger menu toggles sidebar on mobile', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');

    // Verify we're in the room (use locator from RoomPage)
    await expect(roomPage.messageInput).toBeVisible();

    // Resize to mobile viewport (below md breakpoint of 768px)
    await page.setViewportSize({ width: 375, height: 667 });

    // Hamburger menu should be visible in the app header (also proves layout settled)
    const hamburger = page.locator('button[title="Toggle sidebar"]');
    await expect(hamburger).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Sidebar should be closed after resizing to mobile
    const roomList = page.locator('.room-list');
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Click hamburger to open sidebar
    await hamburger.click();

    // Sidebar should now be visible
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Click hamburger again to close sidebar
    await hamburger.click();

    // Sidebar should be hidden again
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('sidebar closes when navigating to a room on mobile', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a second room so we can navigate between rooms
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();
    await chatPage.createRoom('second-room');

    // Resize to mobile viewport (below md breakpoint of 768px)
    await page.setViewportSize({ width: 375, height: 667 });

    // Open sidebar using hamburger menu
    const hamburger = page.locator('button[title="Toggle sidebar"]');
    await expect(hamburger).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await hamburger.click();

    // Sidebar should now be visible
    const roomList = page.locator('.room-list');
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Click on the second room to navigate
    await page.locator('.room-list').getByRole('link', { name: 'second-room' }).click();

    // After navigation, sidebar should automatically close on mobile
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The new room should be active (verify we navigated)
    await expect(roomPage.messageInput).toBeVisible();
  });

  test('sidebar is visible by default on desktop', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');

    // Verify we're in the room first
    await expect(roomPage.messageInput).toBeVisible();

    // Ensure desktop viewport (above md breakpoint)
    await page.setViewportSize({ width: 1280, height: 720 });

    // Both sidebar and content should be visible on desktop (also proves layout settled)
    const roomList = page.locator('.room-list');
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(roomPage.messageInput).toBeVisible();
  });

  test('sidebar reappears when resizing back to desktop', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();

    const roomList = page.locator('.room-list');

    // Start at desktop — sidebar visible
    await page.setViewportSize({ width: 1280, height: 720 });
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Shrink to mobile — sidebar hidden
    await page.setViewportSize({ width: 375, height: 667 });
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Grow back to desktop — sidebar should reappear
    await page.setViewportSize({ width: 1280, height: 720 });
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('manually closed sidebar stays closed after resize round-trip', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();

    const roomList = page.locator('.room-list');
    const hamburger = page.locator('button[title="Toggle sidebar"]');

    // Start at desktop — sidebar visible
    await page.setViewportSize({ width: 1280, height: 720 });
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Manually close sidebar on desktop
    await hamburger.click();
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Shrink to mobile and back to desktop
    await page.setViewportSize({ width: 375, height: 667 });
    await page.setViewportSize({ width: 1280, height: 720 });

    // Sidebar should still be closed — user preference sticks
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Manually reopen on desktop
    await hamburger.click();
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('subtitle is hidden on mobile, visible on desktop', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to a page with a subtitle (notifications page)
    await page.goto(routes.notifications);
    await page.waitForURL(routes.notifications);

    // Ensure desktop viewport first
    await page.setViewportSize({ width: 1280, height: 720 });

    // Subtitle should be visible on desktop
    const subtitle = page.locator("text=Here's what's new");
    await expect(subtitle).toBeVisible();

    // Resize to mobile
    await page.setViewportSize({ width: 375, height: 667 });

    // Subtitle should be hidden on mobile
    await expect(subtitle).not.toBeVisible();

    // Title should still be visible
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
  });

  test('hamburger menu works on admin pages', async ({ page }) => {
    // Login as admin (the bootstrap admin user)
    const adminUser = await loginAsAdmin(page);
    // Verify admin email to get config-based admin access
    await verifyAdminEmail(page, adminUser.id!);

    // Navigate to admin page and wait for it to load
    await page.goto(routes.admin);
    await page.waitForURL(routes.admin);

    // Wait for the page content to load (General heading visible)
    await expect(page.getByRole('heading', { name: 'General', level: 1 })).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Resize to mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });

    // Hamburger should be visible (also proves layout settled after resize)
    const hamburger = page.locator('button[title="Toggle sidebar"]');
    await expect(hamburger).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Sidebar should be closed after resizing to mobile
    const generalLink = page.locator('nav').getByRole('link', { name: 'General', exact: true });
    await expect(generalLink).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Click hamburger to open sidebar
    await hamburger.click();

    // Dedicated admin sidebar should now be visible.
    await expect(page.getByRole('link', { name: 'Back to Server' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(generalLink).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Click hamburger to close sidebar again
    await hamburger.click();

    // Sidebar should be hidden
    await expect(generalLink).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });
});
