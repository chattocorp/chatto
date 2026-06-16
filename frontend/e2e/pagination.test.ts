import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';

/**
 * Post messages via GraphQL API (much faster than UI-based posting).
 * Use this for test setup when you need many messages quickly.
 */
async function postMessagesViaAPI(
  page: Page,
  roomId: string,
  messages: string[]
): Promise<void> {
  for (const body of messages) {
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
        variables: { input: { roomId, body } }
      }
    });
  }
}

/**
 * Extract roomId from the current URL (`/chat/-/{roomId}`). Post-ADR-030
 * the spaceId is just the kind discriminator constant.
 */
async function getIdsFromUrl(page: Page): Promise<{ spaceId: string; roomId: string }> {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  return { spaceId: 'server', roomId: match[1] };
}

test.describe('message pagination', () => {
  test('newest message is visible after posting many messages and reloading', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post 60 messages via API (more than default limit of 50)
    const messages = Array.from({ length: 60 }, (_, i) => `Message ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    const lastMessage = `Message 60 - ${timestamp}`;

    // Reload so messages are loaded via the initial query (last 50) rather than
    // waiting for 60 subscription events to arrive and render through virtua.
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);

    // The newest message should still be visible after reload
    await expect(page.getByText(lastMessage)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  });

  test('scroll position remains stable when loading older messages', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post 70 messages via API (well over the 50-message page size)
    const messages = Array.from({ length: 70 }, (_, i) => `Scroll-test ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Intercept the initial GraphQL query to limit it to 25 events.
    // The subscription uses DeliverNewPolicy (no replay), so after reload only
    // the initial query populates the cache. By limiting to 25 we create a
    // clear pagination gap (messages 1-45 are not loaded initially).
    await page.route('**/api/graphql', async (route, request) => {
      const postData = request.postDataJSON();
      const queryName = postData?.query?.match(/query\s+(\w+)/)?.[1] ?? '';
      if (queryName === 'RoomEventsQuery' && !postData?.variables?.before) {
        await route.continue({
          postData: JSON.stringify({
            ...postData,
            variables: { ...postData.variables, limit: 25 }
          })
        });
      } else {
        await route.continue();
      }
    });

    // Reload the page — the intercepted query returns only ~25 events
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);
    await expect(page.getByText(`Scroll-test 70 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Remove the intercept so pagination queries go through unmodified
    await page.unroute('**/api/graphql');

    const messagesContainer = page.getByTestId('messages-container');

    // Wait for auto-scroll to stabilize at the bottom
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Pick a message near the top of the initially loaded batch as anchor.
    // With 25 events returned, the earliest message is around Scroll-test 46.
    const anchorMessage = `Scroll-test 48 - ${timestamp}`;

    // Scroll up incrementally until the anchor message is visible.
    // Important: stop scrolling as soon as the anchor appears to avoid
    // over-scrolling past it. Continuous wheel events during pagination
    // would undo virtua's shift-based scroll restoration.
    const box = await messagesContainer.boundingBox();
    if (!box) throw new Error('Container not visible');
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    for (let i = 0; i < 30; i++) {
      await page.mouse.wheel(0, -200);
      await page.waitForTimeout(TIMEOUTS.SCROLL_SETTLE);
      const isVisible = await page
        .getByText(anchorMessage)
        .isVisible()
        .catch(() => false);
      if (isVisible) break;
    }

    // Wait for the anchor to be visible (should already be, but verify)
    await expect(page.getByText(anchorMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Wait for scroll to fully settle before recording position
    await expect(async () => {
      const scrollTop1 = await messagesContainer.evaluate((el) => el.scrollTop);
      await page.waitForTimeout(TIMEOUTS.SCROLL_SETTLE);
      const scrollTop2 = await messagesContainer.evaluate((el) => el.scrollTop);
      expect(Math.abs(scrollTop2 - scrollTop1)).toBeLessThan(5);
    }).toPass({ timeout: TIMEOUTS.UI_FAST, intervals: POLLING_INTERVALS });

    // Get the anchor message's position relative to the viewport BEFORE pagination
    const anchorTopBefore = await page.getByText(anchorMessage).evaluate((el) => {
      return el.getBoundingClientRect().top;
    });

    // Wait for pagination to complete — scrollTop increases as older messages
    // are prepended and virtua's shift mode adjusts the position.
    await expect(async () => {
      const scrollTop = await messagesContainer.evaluate((el) => el.scrollTop);
      expect(scrollTop).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: POLLING_INTERVALS });

    // Wait for scroll position to settle after pagination
    await expect(async () => {
      const scrollTop1 = await messagesContainer.evaluate((el) => el.scrollTop);
      await page.waitForTimeout(TIMEOUTS.LAYOUT_SETTLE);
      const scrollTop2 = await messagesContainer.evaluate((el) => el.scrollTop);
      expect(Math.abs(scrollTop2 - scrollTop1)).toBeLessThan(5);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Get the anchor message's position AFTER pagination
    const anchorTopAfter = await page.getByText(anchorMessage).evaluate((el) => {
      return el.getBoundingClientRect().top;
    });

    // The anchor message should remain at approximately the same viewport position.
    // virtua's shift={isLoadingMore} handles scroll restoration when items are
    // prepended. Small drift can occur from measurement adjustments.
    const drift = Math.abs(anchorTopAfter - anchorTopBefore);
    expect(drift).toBeLessThan(200);
  });

  test('backward pagination reaches the very beginning of conversation', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Use a tall viewport (realistic desktop size) to ensure pagination triggers
    // even when short messages don't fill much vertical space.
    // Previously this used height: 500 which masked a bug where the pagination
    // guard (distanceFromBottom > viewportSize) was too strict for tall viewports.
    await page.setViewportSize({ width: 1280, height: 900 });

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post 150 messages (3 full pages of 50)
    const messages = Array.from({ length: 150 }, (_, i) => `Paginate ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Reload for clean state (loads last ~50)
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);
    await expect(page.getByText(`Paginate 150 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The first message should NOT be visible yet
    await expect(page.getByText(`Paginate 1 - ${timestamp}`)).not.toBeVisible();

    const messagesContainer = page.getByTestId('messages-container');

    // Wait for scroll to stabilize at bottom
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Repeatedly scroll up to trigger pagination until the "beginning"
    // marker becomes visible. Stopping earlier (e.g. as soon as Paginate 1
    // is visible) is flaky because virtua's shift={isLoadingMore} places the
    // marker above the current viewport post-pagination — the user has to
    // keep scrolling for it to render. Looking for the marker directly is
    // the authoritative signal that the start has been reached.
    const box = await messagesContainer.boundingBox();
    if (!box) throw new Error('Container not visible');
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);

    const startMarker = page.getByText('This is the beginning of this conversation.');

    let markerVisible = false;
    for (let i = 0; i < 60; i++) {
      markerVisible = await startMarker.isVisible().catch(() => false);
      if (markerVisible) break;

      await page.mouse.wheel(0, -1000);
      await page.waitForTimeout(TIMEOUTS.SCROLL_SETTLE);
    }

    // Sanity check: the first message must be in the loaded timeline too.
    await expect(page.getByText(`Paginate 1 - ${timestamp}`)).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
    await expect(startMarker).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('messages in other rooms are not affected by room with many messages', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const timestamp = Date.now();

    // Create a second room and post 5 messages via API
    const secondRoomName = await chatPage.createRoom(`room-b-${timestamp}`);
    const roomBIds = await getIdsFromUrl(page);
    const roomBMessages = Array.from(
      { length: 5 },
      (_, i) => `Room B Message ${i + 1} - ${timestamp}`
    );
    await postMessagesViaAPI(page, roomBIds.roomId, roomBMessages);
    const lastRoomBMessage = `Room B Message 5 - ${timestamp}`;
    await expect(page.getByText(lastRoomBMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Go to room A (general) and post 60 messages via API
    await chatPage.enterRoom('general');
    const roomAIds = await getIdsFromUrl(page);
    const roomAMessages = Array.from(
      { length: 60 },
      (_, i) => `Room A Message ${i + 1} - ${timestamp}`
    );
    await postMessagesViaAPI(page, roomAIds.roomId, roomAMessages);
    const lastRoomAMessage = `Room A Message 60 - ${timestamp}`;

    // Reload so room A messages are loaded via initial query (last 50) rather than
    // waiting for 60 subscription events to arrive and render through virtua.
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);

    // Room A should show its newest message
    await expect(page.getByText(lastRoomAMessage)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Go to room B - all messages should still be visible
    await chatPage.enterRoom(secondRoomName);
    await expect(page.getByText(lastRoomBMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Verify ALL room B messages are visible (may need time to load)
    for (let i = 1; i <= 5; i++) {
      await expect(page.getByText(`Room B Message ${i} - ${timestamp}`)).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    }
  });
});
