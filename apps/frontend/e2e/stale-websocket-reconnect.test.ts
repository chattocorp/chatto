import { expect, type Page } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { postThreadReplyViaConnect } from './fixtures/connectHelpers';
import { test } from './setup';
import { TIMEOUTS } from './constants';

async function simulateBackgroundResumeAndReconnect(page: Page, hiddenMs = 31_000) {
  await page.evaluate((durationMs: number) => {
    const originalNow = Date.now;
    let now = originalNow();

    Date.now = () => now;

    Object.defineProperty(document, 'visibilityState', {
      value: 'hidden',
      writable: true,
      configurable: true
    });
    document.dispatchEvent(new Event('visibilitychange'));

    now += durationMs;
    Object.defineProperty(document, 'visibilityState', {
      value: 'visible',
      writable: true,
      configurable: true
    });
    document.dispatchEvent(new Event('visibilitychange'));

    window.dispatchEvent(new Event('online'));

    Date.now = originalNow;
  }, hiddenMs);
}

test.describe('WebSocket reconnect recovery', () => {
  test('recovers messages posted while disconnected after reconnecting', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    const baselineMessage = `baseline-${Date.now()}`;
    await roomPage.sendMessage(baselineMessage);
    await roomPage.expectMessageVisible(baselineMessage);
    let reconnectTimelineReads = 0;
    page.on('request', (request) => {
      if (request.url().includes('/chatto.api.v1.RoomService/GetRoomEvents')) {
        reconnectTimelineReads++;
      }
    });

    try {
      await withServerUser(
        browser!,
        serverURL,
        async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
          await chatPage2.enterRoom('general');
          await waitForRoomReady(page2, 'general');
          await roomPage2.expectMessageVisible(baselineMessage);

          await page.context().setOffline(true);
          await page.waitForTimeout(TIMEOUTS.NETWORK_OFFLINE);

          const missedMessage = `missed-while-disconnected-${Date.now()}`;
          await roomPage2.sendMessage(missedMessage);
          await roomPage.expectMessageNotVisible(missedMessage);

          await page.context().setOffline(false);
          await simulateBackgroundResumeAndReconnect(page);

          await expect(page.getByText(missedMessage)).toBeVisible({
            timeout: TIMEOUTS.REALTIME_EVENT
          });
          expect(reconnectTimelineReads).toBe(0);
        }
      );
    } finally {
      await page.context().setOffline(false);
    }
  });

  test('recovers thread replies posted while disconnected after reconnecting', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Post a message that will become the thread root
    const threadRoot = `thread-root-${Date.now()}`;
    await roomPage.sendMessage(threadRoot);
    await roomPage.expectMessageVisible(threadRoot);

    // Open the thread pane
    const threadRootMessage = roomPage.getMessage(threadRoot);
    await threadRootMessage.openThread();
    await roomPage.expectThreadPaneVisible();

    // Post a baseline thread reply so we know the thread is working
    const baselineReply = `baseline-reply-${Date.now()}`;
    await roomPage.postThreadReply(baselineReply);
    await roomPage.expectTextInThreadPane(baselineReply);

    // Extract room ID and thread root event ID from the URL.
    // URL format: /chat/-/{roomId}/{threadId}
    const urlParts = page.url().split('/');
    const roomId = urlParts[urlParts.length - 2];
    const threadRootEventId = urlParts[urlParts.length - 1];

    try {
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        // Go offline to simulate tab suspension
        await page.context().setOffline(true);
        await page.waitForTimeout(TIMEOUTS.NETWORK_OFFLINE);

        // User 2 posts a thread reply via Connect while User 1 is disconnected
        const missedReply = `missed-thread-reply-${Date.now()}`;
        await postThreadReplyViaConnect(page2, roomId, missedReply, threadRootEventId);

        // Verify User 1 doesn't see it yet (offline)
        await roomPage.expectTextNotInThreadPane(missedReply);

        // Come back online and simulate background resume
        await page.context().setOffline(false);
        await simulateBackgroundResumeAndReconnect(page);

        // Verify User 1 sees the missed thread reply
        await expect(page.getByTestId('thread-pane').getByText(missedReply)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
      });
    } finally {
      await page.context().setOffline(false);
    }
  });
});
