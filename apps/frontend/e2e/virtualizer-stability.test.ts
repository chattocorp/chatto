import { expect, type Page } from '@playwright/test';
import { seedData, loginSeededUser } from './fixtures/seed';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { postMessagesViaConnect } from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';
import { waitForRoomReady } from './fixtures/realtimeSync';

/**
 * Get the room ID from the current page URL.
 */
function getRoomIdFromUrl(page: Page): string {
  const url = page.url();
  const match = url.match(/\/chat\/-\/([^/?]+)/);
  if (!match) throw new Error(`Could not extract room ID from URL: ${url}`);
  return match[1];
}

async function getScrollFadeOpacities(page: Page): Promise<{ top: number; bottom: number }> {
  return page.getByTestId('messages-container').evaluate((el) => {
    const fades = Array.from(el.parentElement?.querySelectorAll('[aria-hidden="true"]') ?? []);
    const topFade = fades[0];
    const bottomFade = fades[1];
    if (!(topFade instanceof HTMLElement) || !(bottomFade instanceof HTMLElement)) {
      throw new Error('Scroll fades not found');
    }
    return {
      top: Number(getComputedStyle(topFade).opacity),
      bottom: Number(getComputedStyle(bottomFade).opacity)
    };
  });
}

test.describe('Virtualizer stability', () => {
  test('scroll fades reset when switching from overflowing room to sparse room', async ({
    page,
    chatPage
  }) => {
    await page.setViewportSize({ width: 1280, height: 500 });
    const scene = await seedData(page.request, { seed: 42, users: 1, rooms: 1, messages: 25 });
    await loginSeededUser(page.request, scene.users[0]);
    await chatPage.goto();
    await chatPage.enterRoom(scene.rooms[0].name);
    const timestamp = Date.now();

    await expect(page.getByText(scene.messages[24].body)).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    await expect
      .poll(() => getScrollFadeOpacities(page), { timeout: TIMEOUTS.UI_STANDARD })
      .toMatchObject({ top: 1 });

    const sparseRoomName = await chatPage.createRoom(`fade-sparse-${timestamp}`);
    await expect(chatPage.getRoomHeader(sparseRoomName)).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    await expect
      .poll(
        () =>
          page.getByTestId('messages-container').evaluate((el) => ({
            overflows: el.scrollHeight > el.clientHeight + 1,
            fades: Array.from(el.parentElement?.querySelectorAll('[aria-hidden="true"]') ?? []).map(
              (fade) => Number(getComputedStyle(fade).opacity)
            )
          })),
        { timeout: TIMEOUTS.UI_STANDARD }
      )
      .toEqual({ overflows: false, fades: [0, 0] });
  });

  test('rapid room switching with different message counts does not cause JS errors', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    const scene = await seedData(page.request, { seed: 42, users: 1, rooms: 1, messages: 20 });
    await loginSeededUser(page.request, scene.users[0]);
    await chatPage.goto();
    await chatPage.enterRoom(scene.rooms[0].name);

    await expect(page.getByText(scene.messages[19].body)).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Create a second room with only a few messages
    const secondRoomName = await chatPage.createRoom(`sparse-room-${Date.now()}`);
    const sparseRoomId = getRoomIdFromUrl(page);

    const sparseMessages = Array.from({ length: 3 }, (_, i) => `Sparse message ${i + 1}`);
    await postMessagesViaConnect(page, sparseRoomId, sparseMessages);
    await expect(page.getByText('Sparse message 3')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Set up error capture
    const pageErrors: string[] = [];
    const consoleErrors: string[] = [];

    page.on('pageerror', (err) => {
      pageErrors.push(err.message);
    });
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    // Rapidly switch between rooms 6 times
    for (let i = 0; i < 6; i++) {
      await chatPage.enterRoom(scene.rooms[0].name);
      await chatPage.enterRoom(secondRoomName);
    }

    // Wait for any deferred errors to surface by verifying the page is stable
    await expect(page.getByTestId('message-input')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Filter for the specific crash signature
    const criticalErrors = [
      ...pageErrors.filter(
        (e) =>
          e.includes('Cannot read properties of undefined') ||
          e.includes('lifecycle_outside_component')
      ),
      ...consoleErrors.filter(
        (e) =>
          e.includes('Cannot read properties of undefined') ||
          e.includes('lifecycle_outside_component')
      )
    ];

    expect(criticalErrors).toEqual([]);
  });

  test('real-time messages from another user during room switching do not cause JS errors', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create account with two rooms
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    const generalRoomId = getRoomIdFromUrl(page);

    // Seed general room with messages so it has scroll content
    const seedMessages = Array.from({ length: 15 }, (_, i) => `Seed message ${i + 1}`);
    await postMessagesViaConnect(page, generalRoomId, seedMessages);
    await expect(page.getByText('Seed message 15')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Create a second room
    const secondRoomName = await chatPage.createRoom(`other-room-${Date.now()}`);

    // User 2: Open the server
    await withServerUser(browser!, serverURL, async ({ page: page2, chatPage: chatPage2 }) => {
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Set up error capture on User 1's page
      const pageErrors: string[] = [];
      const consoleErrors: string[] = [];

      page.on('pageerror', (err) => {
        pageErrors.push(err.message);
      });
      page.on('console', (msg) => {
        if (msg.type() === 'error') {
          consoleErrors.push(msg.text());
        }
      });

      // User 2 posts messages while User 1 switches rooms
      const postPromise = (async () => {
        for (let i = 0; i < 10; i++) {
          await postMessagesViaConnect(page2, generalRoomId, [`Live message ${i + 1}`]);
        }
      })();

      // User 1 switches rooms while messages arrive
      for (let i = 0; i < 4; i++) {
        await chatPage.enterRoom('general');
        await chatPage.enterRoom(secondRoomName);
      }

      // Wait for all messages to be posted
      await postPromise;

      // Wait for any deferred errors to surface by verifying the page is stable
      await expect(page.getByTestId('message-input')).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // Filter for the specific crash signature
      const criticalErrors = [
        ...pageErrors.filter(
          (e) =>
            e.includes('Cannot read properties of undefined') ||
            e.includes('lifecycle_outside_component')
        ),
        ...consoleErrors.filter(
          (e) =>
            e.includes('Cannot read properties of undefined') ||
            e.includes('lifecycle_outside_component')
        )
      ];

      expect(criticalErrors).toEqual([]);

      // Verify User 1 can still see messages after all the switching
      await chatPage.enterRoom('general');
      await expect(page.getByText('Seed message 15')).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    });
  });
});
