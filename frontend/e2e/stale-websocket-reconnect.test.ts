import { expect, type Page } from '@playwright/test';
import { createAndLoginTestUser, openServer } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

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

    const spaceId = await chatPage.getServerScopeId();
    const baselineMessage = `baseline-${Date.now()}`;
    await roomPage.sendMessage(baselineMessage);
    await roomPage.expectMessageVisible(baselineMessage);

    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();
    const chatPage2 = new ChatPage(page2);
    const roomPage2 = new RoomPage(page2);

    try {
      await createAndLoginTestUser(page2);
      await openServer(page2);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.spaceOrRoom);
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

      await expect(page.getByText(missedMessage)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await page.context().setOffline(false);
      await context2.close();
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

    const spaceId = await chatPage.getServerScopeId();

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

    // Extract room ID and thread root event ID from the URL
    // URL format: /chat/-/{spaceId}/{roomId}/{threadId}
    const urlParts = page.url().split('/');
    const roomId = urlParts[urlParts.length - 2];
    const threadRootEventId = urlParts[urlParts.length - 1];

    // Set up User 2
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await openServer(page2);

      // Go offline to simulate tab suspension
      await page.context().setOffline(true);
      await page.waitForTimeout(TIMEOUTS.NETWORK_OFFLINE);

      // User 2 posts a thread reply via API while User 1 is disconnected
      const missedReply = `missed-thread-reply-${Date.now()}`;
      await page2.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
          variables: {
            input: { roomId, body: missedReply, threadRootEventId: threadRootEventId }
          }
        }
      });

      // Verify User 1 doesn't see it yet (offline)
      await roomPage.expectTextNotInThreadPane(missedReply);

      // Come back online and simulate background resume
      await page.context().setOffline(false);
      await simulateBackgroundResumeAndReconnect(page);

      // Verify User 1 sees the missed thread reply
      await expect(page.getByTestId('thread-pane').getByText(missedReply)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    } finally {
      await page.context().setOffline(false);
      await context2.close();
    }
  });
});
