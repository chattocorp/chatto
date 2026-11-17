import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

/** Event evidence survives same-document room navigation. */
declare global {
  interface Window {
    sidebarTapObservations: { messageEvents: string[]; openedOverlay: boolean };
  }
}

test.describe('Mobile sidebar click-through', () => {
  // Firefox does not support Playwright's isMobile option. Both engines use
  // the same viewport and touch input; this is not Firefox for Android.
  test.use({ hasTouch: true });
  test.setTimeout(60_000);

  test('channel taps do not reach an overlapping message image', async ({
    page,
    chatPage,
    roomPage,
    browserName
  }, testInfo) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createRoom('overlap-a');
    await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', 'Image in overlap-a');
    // Later messages move the image up to the channel rows without CSS or DOM changes.
    for (let i = 0; i < 8; i++) await roomPage.sendMessage('After image a ' + i);
    const firstURL = page.url();
    await chatPage.createRoom('overlap-b');
    await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', 'Image in overlap-b');
    for (let i = 0; i < 8; i++) await roomPage.sendMessage('After image b ' + i);
    const secondURL = page.url();
    await page.setViewportSize({ width: 390, height: 844 });

    const toggle = page.getByTitle('Toggle sidebar', { exact: true });
    const sidebar = page.getByTestId('server-sidebar');
    const backdrop = page.getByTestId('mobile-sidebar-backdrop');
    await expect(sidebar).not.toBeInViewport();
    await expect(roomPage.attachmentImage).toBeVisible();

    // Record even transient interactions: navigation can remove a viewer or
    // message menu before a final visibility assertion sees it. Capture does
    // not cancel events or replace any application handlers.
    await page.evaluate(() => {
      const observations = { messageEvents: [] as string[], openedOverlay: false };
      window.sidebarTapObservations = observations;
      for (const type of ['pointerdown', 'touchstart', 'mousedown', 'click', 'contextmenu']) {
        document.addEventListener(
          type,
          (event) => {
            if (event.target instanceof Element && event.target.closest('[role="article"]')) {
              observations.messageEvents.push(type);
            }
          },
          true
        );
      }
      new MutationObserver(() => {
        if (document.querySelector('dialog[open], [role="menu"]')) {
          observations.openedOverlay = true;
        }
      }).observe(document.body, { subtree: true, childList: true, attributes: true });
    });

    let lastPoint = { x: 0, y: 0 };
    for (let attempt = 0; attempt < 4; attempt++) {
      const destination = attempt % 2 === 0 ? 'overlap-a' : 'overlap-b';
      await toggle.tap();
      const entry = chatPage.getRoomLink(destination);
      await expect(entry).toBeVisible();
      // Wait for the drawer transform to finish before measuring the tap.
      await expect
        .poll(() => sidebar.evaluate((node) => node.getBoundingClientRect().left))
        .toBe(68);
      const entryBox = await entry.boundingBox();
      const imageBox = await roomPage.attachmentImage.boundingBox();
      expect(entryBox).not.toBeNull();
      expect(imageBox).not.toBeNull();
      const left = Math.max(entryBox!.x, imageBox!.x);
      const right = Math.min(entryBox!.x + entryBox!.width, imageBox!.x + imageBox!.width);
      const top = Math.max(entryBox!.y, imageBox!.y);
      const bottom = Math.min(entryBox!.y + entryBox!.height, imageBox!.y + imageBox!.height);
      expect(
        right - left,
        'channel entry must overlap the real image horizontally'
      ).toBeGreaterThan(4);
      expect(bottom - top, 'channel entry must overlap the real image vertically').toBeGreaterThan(
        4
      );
      const point = { x: (left + right) / 2, y: (top + bottom) / 2 };
      lastPoint = point;
      expect(
        await entry.evaluate(
          (node, point) => node.contains(document.elementFromPoint(point.x, point.y)),
          point
        )
      ).toBe(true);

      // The app header is outside the drawer by design. Check the complete
      // content plane, including the bottom and exposed right-hand strip.
      const coverage = await backdrop.evaluate((node) => {
        const rect = node.getBoundingClientRect();
        return {
          left: rect.left,
          right: rect.right,
          top: rect.top,
          bottom: rect.bottom,
          exposedStripBlocked: [57, 422, 843].every(
            (y) => document.elementFromPoint(389, y) === node
          )
        };
      });
      expect(coverage).toEqual({
        left: 0,
        right: 390,
        top: 56,
        bottom: 844,
        exposedStripBlocked: true
      });
      await page.touchscreen.tap(point.x, point.y);
      // Allow the legacy 300 ms compatibility-click window to expire. This
      // wall-clock observation is intentional: an immediate negative check
      // cannot detect a delayed click after the drawer has disappeared.
      await page.waitForTimeout(500);
      const observations = await page.evaluate(() => window.sidebarTapObservations);
      await testInfo.attach(`tap-${attempt + 1}`, {
        body: JSON.stringify({ browserName, point, entryBox, imageBox, coverage, observations }),
        contentType: 'application/json'
      });
      console.log(
        `${browserName}: channel tap ${attempt + 1}: click-through ${observations.messageEvents.length || observations.openedOverlay ? 'REPRODUCED' : 'not reproduced'}`
      );
      expect(observations.messageEvents).toEqual([]);
      expect(observations.openedOverlay).toBe(false);
      await expect(page.locator('dialog[open], [role="menu"]')).toHaveCount(0);
      await expect(page).toHaveURL(attempt % 2 === 0 ? firstURL : secondURL);
      await expect(chatPage.getRoomHeader(destination)).toBeVisible();
      await expect(sidebar).not.toBeInViewport();
      await expect(roomPage.attachmentImage).toBeVisible();
      // The destination also has a real image at the tap coordinates. This
      // covers a delayed click retargeted after the room switch.
      await expect
        .poll(() =>
          roomPage.attachmentImage.evaluate(
            (image, point) => document.elementFromPoint(point.x, point.y) === image,
            point
          )
        )
        .toBe(true);
    }
    // Positive control: the same coordinates must open the viewer with the
    // drawer closed, and the event probe must detect that image interaction.
    await page.touchscreen.tap(lastPoint.x, lastPoint.y);
    await expect(page.locator('dialog[open]')).toBeVisible();
    const control = await page.evaluate(() => window.sidebarTapObservations);
    expect(control.messageEvents).toContain('click');
    expect(control.openedOverlay).toBe(true);
    await page.keyboard.press('Escape');
    await expect(page.locator('dialog[open]')).toHaveCount(0);
    expect(errors).toEqual([]);
  });
});
