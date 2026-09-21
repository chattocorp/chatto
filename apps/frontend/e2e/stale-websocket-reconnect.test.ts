import { expect, type Page } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { postThreadReplyViaConnect, postMessagesViaConnect } from './fixtures/connectHelpers';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { RealtimeServerFrame, RealtimeSubscribe } from '@chatto/api-types/realtime/v1/realtime_pb';
import { GetRoomEventsAroundRequest } from '@chatto/api-types/api/v1/room_timeline_pb';

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

for (const mobile of [false, true]) {
  test(`keeps room and thread instances through a replacement snapshot (${mobile ? 'mobile' : 'desktop'})`, async ({ page, chatPage, roomPage }) => {
    let forceSnapshot = false;
    let releaseCatchUp: (() => void) | undefined;
    const restoredAnchorIds: string[] = [];
    page.on('request', (request) => {
      if (request.url().endsWith('/GetRoomEventsAround')) {
        restoredAnchorIds.push(GetRoomEventsAroundRequest.fromBinary(request.postDataBuffer()!).eventId);
      }
    });
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.routeWebSocket('**/api/realtime', (socket) => {
      const server = socket.connectToServer();
      let recovering = false;
      let held = false;
      const queued: Array<string | Buffer> = [];
      socket.onMessage((message) => {
        if (forceSnapshot && typeof message !== 'string') {
          forceSnapshot = false;
          recovering = true;
          const subscribe = RealtimeSubscribe.fromBinary(message);
          // Exercise the same fallback as cursor expiry without a 15-minute wait.
          subscribe.resumeCursor = 'expired-test-cursor';
          server.send(Buffer.from(subscribe.toBinary()));
        } else server.send(message);
      });
      server.onMessage((message) => {
        if (recovering && typeof message !== 'string' && RealtimeServerFrame.fromBinary(message).frame.case === 'caughtUp') {
          held = true;
          releaseCatchUp = () => {
            recovering = false;
            held = false;
            for (const frame of queued) socket.send(frame);
            queued.length = 0;
          };
        }
        if (held) queued.push(message);
        else socket.send(message);
      });
    });
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    const root = `snapshot-root-${Date.now()}`;
    await roomPage.sendMessage(root);
    await roomPage.getMessage(root).openThread();
    await roomPage.postThreadReply('snapshot reply');
    await roomPage.expectTextInThreadPane('snapshot reply');
    await roomPage.typeInThreadInput('unsent reply');
    const roomId = new URL(page.url()).pathname.split('/')[3];
    await postMessagesViaConnect(page, roomId, Array.from({ length: 30 }, (_, index) => `snapshot-row-${index}`));
    // Establish the conversation with the shared desktop navigation helper,
    // then exercise the complete suspend/recovery cycle at phone width.
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    const scroller = page.getByTestId('room-main-pane').getByTestId('messages-container');
    await expect.poll(() => scroller.evaluate((element) => element.scrollHeight > element.clientHeight + 400)).toBe(true);
    await scroller.evaluate(async (element) => {
      element.dispatchEvent(new WheelEvent('wheel', { deltaY: -200, bubbles: true }));
      element.scrollTop = element.scrollHeight - element.clientHeight - 200;
      // Let the real scroll event and virtualizer measurements reach the store.
      await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
    });
    const visibleAnchor = () => scroller.evaluate((element) => {
      const top = element.getBoundingClientRect().top;
      const rows = [...element.querySelectorAll<HTMLElement>('[data-event-id]')];
      const row = rows.find((row) => row.getBoundingClientRect().bottom > top);
      return row ? { id: row.dataset.eventId!, offset: row.getBoundingClientRect().top - top } : null;
    });
    await expect.poll(visibleAnchor).not.toBeNull();
    const anchor = (await visibleAnchor())!;
    const room = await page.getByTestId('room-main-pane').elementHandle();
    const thread = await page.getByTestId('thread-pane').elementHandle();
    const url = page.url();
    forceSnapshot = true;
    await simulateBackgroundResumeAndReconnect(page);
    await expect.poll(() => !!releaseCatchUp).toBe(true);
    await expect(page.getByTestId('room-main-pane')).toBeHidden();
    await expect(page.getByTestId('thread-pane').getByText('snapshot reply', { exact: true })).toHaveCount(0);
    expect(await room!.evaluate((element) => element.isConnected)).toBe(true);
    expect(await thread!.evaluate((element) => element.isConnected)).toBe(true);
    releaseCatchUp!();
    await roomPage.expectTextInThreadPane('snapshot reply');
    await expect(page.getByTestId('room-main-pane')).toBeVisible();
    expect(await room!.evaluate((element) => element.isConnected)).toBe(true);
    expect(await thread!.evaluate((element) => element.isConnected)).toBe(true);
    await expect(roomPage.threadReplyInput).toHaveText('unsent reply');
    expect(restoredAnchorIds).toContain(anchor.id);
    await expect.poll(async () => (await visibleAnchor())?.id).toBe(anchor.id);
    await expect.poll(async () => Math.abs(((await visibleAnchor())?.offset ?? Infinity) - anchor.offset)).toBeLessThan(5);
    expect(page.url()).toBe(url);
    expect(errors).toEqual([]);
  });
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
          // Reconnect invalidates the mounted canonical timeline. The client
          // recovers missed events through an explicit ConnectRPC read.
          expect(reconnectTimelineReads).toBeGreaterThanOrEqual(1);
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
