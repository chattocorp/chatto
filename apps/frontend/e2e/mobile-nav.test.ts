import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, loginAsAdmin, verifyAdminEmail } from './fixtures/testUser';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

async function touchDrag(
  page: import('@playwright/test').Page,
  fromX: number,
  toX: number,
  y: number
) {
  const client = await page.context().newCDPSession(page);
  const steps = 6;

  await client.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: fromX, y }]
  });

  for (let i = 1; i <= steps; i += 1) {
    const x = fromX + ((toX - fromX) * i) / steps;
    await client.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x, y }]
    });
  }

  await client.send('Input.dispatchTouchEvent', {
    type: 'touchEnd',
    touchPoints: []
  });
}

test.describe('Mobile Navigation', () => {
  // A narrow mouse window has compact chrome. These cases exercise touch chrome.
  test.use({ hasTouch: true });

  test('fills the bottom safe area with the chat background in both themes', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    const client = await page.context().newCDPSession(page);
    await client.send('Emulation.setSafeAreaInsetsOverride', {
      insets: { top: 0, left: 0, right: 0, bottom: 34 }
    });
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

    for (const colorScheme of ['dark', 'light'] as const) {
      await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' });
      await expect(page.locator('html')).toHaveAttribute('data-theme', colorScheme);
      await expect
        .poll(() =>
          page.evaluate(() => {
            const fill = document.querySelector<HTMLElement>('[data-safari-bottom-edge]')!;
            const frame = document.querySelector<HTMLElement>('[data-testid="app-frame"]')!;
            const bounds = fill.getBoundingClientRect();
            return {
              top: bounds.top,
              bottom: bounds.bottom,
              matchesBackground:
                getComputedStyle(fill).backgroundColor === getComputedStyle(frame).backgroundColor,
              pointerEvents: getComputedStyle(fill).pointerEvents
            };
          })
        )
        .toEqual({ top: 633, bottom: 667, matchesBackground: true, pointerEvents: 'none' });
    }
  });

  test('reserves the bottom safe area once on OAuth sign-in pages', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    const client = await page.context().newCDPSession(page);
    await client.send('Emulation.setSafeAreaInsetsOverride', {
      insets: { top: 0, left: 0, right: 0, bottom: 34 }
    });
    await page.goto('/login?redirect=%2Foauth%2Fauthorize');
    await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
    await expect(page.getByTestId('app-frame')).not.toBeAttached();
    await expect
      .poll(() =>
        page.getByTestId('oauth-page').evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          return bounds.bottom - Number.parseFloat(getComputedStyle(element).paddingBottom);
        })
      )
      .toBe(633);
  });

  test('hamburger menu toggles sidebar on mobile', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');

    // Verify we're in the room (use locator from RoomPage)
    await expect(roomPage.messageInput).toBeVisible();

    // Resize to mobile viewport (below md breakpoint of 768px)
    await page.setViewportSize({ width: 375, height: 667 });

    // Drawer backgrounds reach the viewport edge. Their controls and the
    // app content must stay above the gesture-navigation safe area.
    const client = await page.context().newCDPSession(page);
    await client.send('Emulation.setSafeAreaInsetsOverride', {
      insets: { top: 0, left: 0, right: 0, bottom: 34 }
    });
    expect(
      await page.evaluate(() => {
        const probe = document.createElement('div');
        probe.style.paddingBottom = 'env(safe-area-inset-bottom, 0px)';
        document.body.appendChild(probe);
        const inset = getComputedStyle(probe).paddingBottom;
        probe.remove();
        return inset;
      })
    ).toBe('34px');

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

    for (const testId of ['mobile-sidebar-panel', 'server-sidebar', 'mobile-sidebar-backdrop']) {
      await expect
        .poll(() =>
          page.getByTestId(testId).evaluate((element) => ({
            top: element.getBoundingClientRect().top,
            bottom: element.getBoundingClientRect().bottom
          }))
        )
        .toEqual({ top: 56, bottom: 667 });
    }
    for (const control of [
      page.getByTestId('current-user-presence-menu'),
      page.getByTestId('mobile-sidebar-panel').getByRole('link', { name: 'Add Server' })
    ]) {
      await expect(control).toBeVisible();
      const bounds = await control.boundingBox();
      expect(bounds).not.toBeNull();
      expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(633);
    }

    // Click hamburger again to close sidebar
    await hamburger.click();

    // Sidebar should be hidden again
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('keeps the composer above bottom system navigation as the viewport changes', async ({
    page,
    chatPage
  }) => {
    const pageErrors: string[] = [];
    page.on('pageerror', (error) => pageErrors.push(error.name));
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    const client = await page.context().newCDPSession(page);

    // Include a landscape inset, a reduced keyboard-sized viewport with no
    // navigation inset, and restoration. This does not emulate a real Android IME.
    for (const { width, height, bottom } of [
      { width: 375, height: 667, bottom: 34 },
      { width: 667, height: 375, bottom: 24 },
      { width: 375, height: 400, bottom: 0 },
      { width: 375, height: 667, bottom: 34 }
    ]) {
      await page.setViewportSize({ width, height });
      await client.send('Emulation.setSafeAreaInsetsOverride', {
        insets: { top: 0, left: 0, right: 0, bottom }
      });
      await expect
        .poll(() =>
          page
            .getByTestId('app-frame')
            .evaluate((element) => element.getBoundingClientRect().bottom)
        )
        .toBe(height - bottom);
      for (const control of [roomPage.messageInput, roomPage.sendButton]) {
        await expect(control).toBeVisible();
        const bounds = await control.boundingBox();
        expect(bounds).not.toBeNull();
        expect(bounds!.y).toBeGreaterThanOrEqual(0);
        expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(height - bottom);
      }
    }

    // The native iOS host already reserves the inset through body padding and
    // positions drawers above it. Do not reserve the same space a second time.
    await page.addStyleTag({
      content: `
        :root { --mobile-sidebar-safe-bottom: env(safe-area-inset-bottom, 0px); }
        body { padding-bottom: env(safe-area-inset-bottom, 0px); }
        [data-safari-bottom-edge] { display: none; }
      `
    });
    await expect(page.locator('[data-safari-bottom-edge]')).not.toBeVisible();
    await expect
      .poll(() =>
        page.getByTestId('app-frame').evaluate((element) => element.getBoundingClientRect().bottom)
      )
      .toBe(633);
    await page.locator('button[title="Toggle sidebar"]').click();
    await expect(page.locator('.room-list')).toBeVisible();
    for (const testId of ['mobile-sidebar-panel', 'server-sidebar']) {
      const panel = page.getByTestId(testId);
      await expect
        .poll(() => panel.evaluate((element) => element.getBoundingClientRect().bottom))
        .toBe(633);
      expect(await panel.evaluate((element) => getComputedStyle(element).paddingBottom)).toBe(
        '0px'
      );
    }
    expect(pageErrors).toEqual([]);
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

  test('sidebar closes on a leftward mobile swipe', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();

    await page.setViewportSize({ width: 375, height: 667 });

    const hamburger = page.locator('button[title="Toggle sidebar"]');
    const roomList = page.locator('.room-list');
    await expect(hamburger).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    await hamburger.click();
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    await touchDrag(page, 320, 20, 160);

    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('sidebar opens on a rightward mobile swipe from app content', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();

    await page.setViewportSize({ width: 375, height: 667 });

    const hamburger = page.locator('button[title="Toggle sidebar"]');
    const roomList = page.locator('.room-list');
    await expect(hamburger).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    await touchDrag(page, 100, 320, 160);

    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('sidebar ignores mouse drags in the narrow layout', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();

    await page.setViewportSize({ width: 375, height: 667 });

    const hamburger = page.locator('button[title="Toggle sidebar"]');
    const roomList = page.locator('.room-list');
    await expect(hamburger).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await hamburger.click();
    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    await page.mouse.move(320, 160);
    await page.mouse.down();
    await page.mouse.move(20, 160, { steps: 6 });
    await page.mouse.up();

    await expect(roomList).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await hamburger.click();
    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    await page.mouse.move(100, 160);
    await page.mouse.down();
    await page.mouse.move(320, 160, { steps: 6 });
    await page.mouse.up();

    await expect(roomList).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
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

  test('room header omits redundant server name on mobile', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const serverName = await chatPage.getServerName();

    await page.setViewportSize({ width: 375, height: 667 });

    const roomHeading = chatPage.getRoomHeader('general');
    await expect(roomHeading).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    const roomHeader = roomHeading.locator(
      'xpath=ancestor::div[contains(concat(" ", normalize-space(@class), " "), " border-b ")][1]'
    );
    await expect(roomHeader).not.toContainText(serverName);
  });

  test('hamburger menu works on admin pages', async ({ page }) => {
    // Login as admin (the bootstrap admin user)
    const adminUser = await loginAsAdmin(page);
    // Verify admin email to get config-based admin access
    await verifyAdminEmail(page, adminUser.id!);

    // Navigate to admin page and wait for it to load
    await page.goto(routes.serverAdminGeneral);
    await page.waitForURL(routes.serverAdminGeneral);

    // Wait for the page content to load (General heading visible)
    await expect(page.getByRole('heading', { name: 'General', level: 1 })).toBeVisible({
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });

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
