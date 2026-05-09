import { expect, type Locator, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import { waitForRoomReady } from './fixtures/realtimeSync';

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
 * Scroll a container to the top using native mouse wheel events.
 * Uses multiple smaller wheel events with pauses between them,
 * giving virtua time to process item measurements and scroll corrections.
 * This avoids $fixScrollJump corrections undoing the entire scroll.
 */
async function scrollContainerToTop(page: Page, container: Locator) {
  const box = await container.boundingBox();
  if (!box) throw new Error('Container not visible');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let i = 0; i < 15; i++) {
    await page.mouse.wheel(0, -800);
    await page.waitForTimeout(TIMEOUTS.SCROLL_SETTLE);
  }
}

test.describe('Message pane auto-scroll', () => {
  test('auto-scrolls to new messages after scrolling back down to bottom', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    // User 1: Create space and post enough messages to make container scrollable
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const testSpaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post 20 messages via API (much faster than UI-based posting)
    const longText = 'Lorem ipsum dolor sit amet, consectetur adipiscing elit.';
    const messages = Array.from(
      { length: 20 },
      (_, i) => `Message ${i + 1} - ${timestamp} - ${longText}`
    );
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI and scroll position to stabilize at bottom
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Get the messages container
    const messagesContainer = page.getByTestId('messages-container');

    // Wait for auto-scroll to complete after message loading
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Scroll up to the top (this should set shouldScrollToBottom = false)
    await scrollContainerToTop(page, messagesContainer);

    // Wait for scroll position to stabilize away from the bottom
    // Pagination may adjust position, but key is we're NOT at the bottom
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Scroll back down to the bottom using mouse wheel (this should re-enable auto-scroll).
    // Programmatic scrollTop assignment doesn't reliably work with virtua's scroll handling.
    const box2 = await messagesContainer.boundingBox();
    if (!box2) throw new Error('Container not visible');
    await page.mouse.move(box2.x + box2.width / 2, box2.y + box2.height / 2);
    for (let i = 0; i < 15; i++) {
      await page.mouse.wheel(0, 800);
      await page.waitForTimeout(TIMEOUTS.SCROLL_SETTLE);
    }

    // Wait for scroll position to reach the bottom
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // User 2: Create user, join space, and post a message
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();
    const chatPage2 = new ChatPage(page2);
    const roomPage2 = new RoomPage(page2);
    const explorePage2 = new ExplorePage(page2);

    try {
      await createAndLoginTestUser(page2);
      await chatPage2.goto();

      // Join the space via Explore Spaces
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(testSpaceName);

      // User 2 is auto-joined to "general" room - enter it
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // User 2 posts a new message
      const newMessage = `New message from User 2 - ${Date.now()}`;
      await roomPage2.sendMessage(newMessage);

      // User 1 should see the new message (auto-scrolled into view)
      // The key assertion: if auto-scroll is working, the message will be visible
      await expect(page.getByText(newMessage)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('does not auto-scroll when user is scrolled up viewing history', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    // User 1: Create space and post enough messages to make container scrollable
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const testSpaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post 20 messages via API (much faster than UI-based posting)
    const longText =
      'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor.';
    const messages = Array.from(
      { length: 20 },
      (_, i) => `Message ${i + 1} - ${timestamp} - ${longText}`
    );
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Get the messages container
    const messagesContainer = page.getByTestId('messages-container');

    // Scroll to top using native mouse wheel events (programmatic scrollTop
    // doesn't work reliably with virtua's scroll correction mechanism)
    await scrollContainerToTop(page, messagesContainer);

    // Wait for scroll position to stabilize away from the bottom
    // Pagination may trigger when near top, which adjusts scroll position
    // The key assertion is that we're NOT at the bottom (no auto-scroll happened)
    await expect(async () => {
      const scrollInfo = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom =
        scrollInfo.scrollHeight - scrollInfo.scrollTop - scrollInfo.clientHeight;
      // Should NOT be at the bottom - at least 100px away indicates no auto-scroll
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // User 2: Create user, join space, and post a message
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();
    const chatPage2 = new ChatPage(page2);
    const roomPage2 = new RoomPage(page2);
    const explorePage2 = new ExplorePage(page2);

    try {
      await createAndLoginTestUser(page2);
      await chatPage2.goto();

      // Join the space via Explore Spaces
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(testSpaceName);

      // User 2 is auto-joined to "general" room - enter it
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // User 2 posts a new message
      const newMessage = `New message while scrolled up - ${Date.now()}`;
      await roomPage2.sendMessage(newMessage);

      // Wait for the "New messages" indicator button which confirms:
      // (a) WebSocket event arrived, (b) component decided NOT to auto-scroll,
      // (c) the indicator appeared.
      await expect(page.getByRole('button', { name: /new messages/i })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // User 1 should NOT have been auto-scrolled to the bottom
      // The key behavior is: user stays scrolled up (viewing history), not at bottom
      // Use toPass() to wait for UI to stabilize after message render
      await expect(async () => {
        const scrollInfo = await messagesContainer.evaluate((el) => ({
          scrollTop: el.scrollTop,
          scrollHeight: el.scrollHeight,
          clientHeight: el.clientHeight
        }));
        const distanceFromBottom =
          scrollInfo.scrollHeight - scrollInfo.scrollTop - scrollInfo.clientHeight;
        // Should NOT be at the bottom - at least 100px away indicates no auto-scroll happened
        expect(distanceFromBottom).toBeGreaterThan(100);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
    } finally {
      await context2.close();
    }
  });

  test('scrolls to bottom when entering a room with existing messages', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and post enough messages to fill the screen
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const testSpaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post 20 messages via API (much faster than UI-based posting)
    const messages = Array.from({ length: 20 }, (_, i) => `Message ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Remember the last message text
    const lastMessage = `Message 20 - ${timestamp}`;

    // User 2: Join the space and enter the room - should auto-scroll to bottom
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();
    const chatPage2 = new ChatPage(page2);
    const explorePage2 = new ExplorePage(page2);

    try {
      await createAndLoginTestUser(page2);
      await chatPage2.goto();

      // Join the space via Explore Spaces
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(testSpaceName);

      // Enter the general room
      await chatPage2.enterRoom('general');

      // The last message should be visible (auto-scrolled to bottom)
      await expect(page2.getByText(lastMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Verify we're actually at the bottom by checking scroll position
      const messagesContainer = page2.getByTestId('messages-container');
      await expect(async () => {
        const scrollInfo = await messagesContainer.evaluate((el) => ({
          scrollTop: el.scrollTop,
          scrollHeight: el.scrollHeight,
          clientHeight: el.clientHeight
        }));
        const distanceFromBottom =
          scrollInfo.scrollHeight - scrollInfo.scrollTop - scrollInfo.clientHeight;
        // Should be at or very near the bottom (within 50px tolerance)
        expect(distanceFromBottom).toBeLessThan(50);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
    } finally {
      await context2.close();
    }
  });


  test('does not show new messages indicator when reaction is added while scrolled up', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    // User 1: Create space and post enough messages to make container scrollable
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const testSpaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post 20 messages via API (much faster than UI-based posting)
    const longText =
      'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor.';
    const messages = Array.from(
      { length: 20 },
      (_, i) => `Message ${i + 1} - ${timestamp} - ${longText}`
    );
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // User 2: Join the space and room BEFORE user 1 scrolls up
    // This ensures the "user joined" event doesn't trigger the indicator later
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 500 }
    });
    const page2 = await context2.newPage();
    const chatPage2 = new ChatPage(page2);
    const roomPage2 = new RoomPage(page2);
    const explorePage2 = new ExplorePage(page2);

    try {
      const user2 = await createAndLoginTestUser(page2);
      await chatPage2.goto();

      // Join the space
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(testSpaceName);

      await chatPage2.enterRoom('general');

      // Wait for messages to load on user 2's side - use partial match for longer message
      await expect(page2.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Wait for user 1 to see user 2's join event (auto-scroll should still be enabled)
      await expect(page.getByText(`${user2.displayName} joined the room`)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // NOW user 1 scrolls up (after user 2 has already joined)
      const messagesContainer = page.getByTestId('messages-container');

      // Scroll to top using native mouse wheel events
      await scrollContainerToTop(page, messagesContainer);

      // Wait for scroll position to stabilize away from the bottom
      await expect(async () => {
        const info = await messagesContainer.evaluate((el) => ({
          scrollTop: el.scrollTop,
          scrollHeight: el.scrollHeight,
          clientHeight: el.clientHeight
        }));
        const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
        expect(distanceFromBottom).toBeGreaterThan(100);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

      // Verify the new messages indicator is NOT visible initially
      await expect(page.getByRole('button', { name: /new messages/i })).not.toBeVisible();

      // User 2 adds a reaction to message 20
      const message2 = roomPage2.getMessage(`Message 20 - ${timestamp}`);
      await message2.react('👍');
      await message2.expectReaction('👍', 1);

      // The new messages indicator should NOT appear for reactions.
      // Use toPass() to give the WebSocket event time to propagate, then verify absence.
      await expect(async () => {
        await expect(page.getByRole('button', { name: /new messages/i })).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: POLLING_INTERVALS });
    } finally {
      await context2.close();
    }
  });

  test('does not show new messages indicator when loading older messages via pagination', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    // Create user and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Extract roomId from URL; resolve spaceId via GraphQL (post-ADR-027 the
    // URL no longer carries spaceId).
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];
    const spaceId = await chatPage.getSpaceId();

    const timestamp = Date.now();

    // Post 60 messages via API (much faster than UI-based posting)
    const messages = Array.from({ length: 60 }, (_, i) => `Message ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Reload so messages are loaded via the initial query (last 50) rather than
    // waiting for 60 subscription events to arrive and render through virtua.
    // This is much faster and more deterministic.
    await page.reload();

    // Wait for the last message to appear (it's in the initial load of 50 newest messages)
    await expect(page.getByText(`Message 60 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Get the messages container
    const messagesContainer = page.getByTestId('messages-container');

    // Scroll to top to trigger pagination (loading older messages)
    await scrollContainerToTop(page, messagesContainer);

    // Wait for scroll position to stabilize away from the bottom
    // Pagination will adjust position as older messages load
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Wait for pagination to complete and scroll restoration to settle
    await expect(async () => {
      const scrollTop1 = await messagesContainer.evaluate((el) => el.scrollTop);
      await page.waitForTimeout(TIMEOUTS.LAYOUT_SETTLE);
      const scrollTop2 = await messagesContainer.evaluate((el) => el.scrollTop);
      expect(Math.abs(scrollTop2 - scrollTop1)).toBeLessThan(5);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // The new messages indicator should NOT appear when loading older messages
    // This was a bug: the indicator would show because event count increased,
    // even though the new events were historical (older), not new messages
    // Use toPass() to verify the indicator stays hidden (gives time for any incorrect trigger)
    await expect(async () => {
      await expect(page.getByRole('button', { name: /new messages/i })).not.toBeVisible();
    }).toPass({ timeout: TIMEOUTS.UI_FAST, intervals: POLLING_INTERVALS });

    // Verify pagination actually loaded older messages by scrolling to the top
    await scrollContainerToTop(page, messagesContainer);
    await expect(page.getByText(`Message 1 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
  });

  test('stays at bottom when window is resized narrower', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Setup: Create user, space, and navigate to room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post messages via API with long text that will wrap when window narrows
    const messages = Array.from(
      { length: 10 },
      (_, i) =>
        `Message ${i + 1} - ${timestamp} - This is a longer message that will wrap to multiple lines when the window becomes narrower, causing content height to increase.`
    );
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI
    await expect(page.getByText(`Message 10 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Verify we're at the bottom
    const messagesContainer = page.getByTestId('messages-container');
    await expect(async () => {
      const scrollInfo = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom =
        scrollInfo.scrollHeight - scrollInfo.scrollTop - scrollInfo.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Resize the window to be narrower (this will cause text to wrap)
    const originalSize = page.viewportSize();
    await page.setViewportSize({ width: 600, height: originalSize!.height });

    // Wait for layout to stabilize after resize and verify we're still at the bottom.
    // Use polling approach with toPass for CI reliability.
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distance = info.scrollHeight - info.scrollTop - info.clientHeight;
      // Use 100px tolerance - the auto-scroll behavior should keep us near the bottom
      expect(distance).toBeLessThan(100);
    }).toPass({ timeout: TIMEOUTS.COMPLEX_OPERATION, intervals: POLLING_INTERVALS });

    // Restore original size
    await page.setViewportSize(originalSize!);
  });

  test('scrolls to bottom when user posts a message while scrolled up', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    // Create user, space, and enter room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post enough messages via API to make the container scrollable
    const longText =
      'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor.';
    const messages = Array.from(
      { length: 20 },
      (_, i) => `Message ${i + 1} - ${timestamp} - ${longText}`
    );
    await postMessagesViaAPI(page, roomId, messages);

    // Reload so messages are loaded via initial query instead of waiting for
    // 20 subscription events to arrive and render through virtua
    await page.reload();

    // Wait for the last message to be visible
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Get the messages container
    const messagesContainer = page.getByTestId('messages-container');

    // Scroll to top (this should set shouldScrollToBottom = false)
    await scrollContainerToTop(page, messagesContainer);

    // Wait for scroll position to stabilize away from the bottom
    // Pagination may adjust position, but key is we're NOT at the bottom
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // User posts a NEW message while scrolled up
    const newMessage = `New message posted while scrolled up - ${Date.now()}`;
    await roomPage.sendMessage(newMessage);

    // The new message should be visible (meaning we scrolled to bottom)
    await expect(page.getByText(newMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Verify we're at the bottom
    await expect(async () => {
      const scrollInfoAfter = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom =
        scrollInfoAfter.scrollHeight - scrollInfoAfter.scrollTop - scrollInfoAfter.clientHeight;
      // Should be at or very near the bottom (within 50px tolerance)
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
  });

  test('thread pane scrolls to bottom when user posts a reply while scrolled up', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Smaller viewport to make the thread pane scrollable with a reasonable
    // number of replies.
    await page.setViewportSize({ width: 1280, height: 500 });

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post a root message and open its thread.
    const rootMessage = await roomPage.sendMessage(`Thread root ${timestamp}`);
    const rootEventId = await rootMessage.getEventId();
    if (!rootEventId) throw new Error('Could not read root event id');
    await rootMessage.openThread();
    await expect(roomPage.threadPane).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Post enough thread replies via API to make the thread scrollable.
    const longText =
      'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor.';
    const replies = Array.from(
      { length: 20 },
      (_, i) => `Reply ${i + 1} - ${timestamp} - ${longText}`
    );
    for (const body of replies) {
      await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
          variables: { input: { roomId, body, inThread: rootEventId } }
        }
      });
    }

    // Reload so the thread pane loads via initial query (URL-driven).
    await page.reload();
    await expect(roomPage.threadPane).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
    await expect(
      roomPage.threadPane.getByText(`Reply 20 - ${timestamp}`)
    ).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // The thread pane has its own messages-container; scope to it.
    const threadContainer = roomPage.threadPane.getByTestId('messages-container');

    // Scroll the thread to the top.
    await scrollContainerToTop(page, threadContainer);

    await expect(async () => {
      const info = await threadContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Post a new reply while scrolled up.
    const newReply = `Reply posted while scrolled up - ${Date.now()}`;
    await roomPage.postThreadReply(newReply);

    // The new reply should be at the bottom of the thread.
    await expect(async () => {
      const info = await threadContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
  });

  test('stays at bottom when font size increases', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Setup: Create user, space, and navigate to room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post messages via API to make the container scrollable
    const messages = Array.from({ length: 15 }, (_, i) => `Message ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI
    await expect(page.getByText(`Message 15 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Verify we're at the bottom
    const messagesContainer = page.getByTestId('messages-container');
    await expect(async () => {
      const scrollInfo = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom =
        scrollInfo.scrollHeight - scrollInfo.scrollTop - scrollInfo.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Increase font size by changing the root font size (simulates Cmd+)
    await page.evaluate(() => {
      document.documentElement.style.fontSize = '24px';
    });

    // Wait for layout to stabilize and auto-scroll to kick in
    // CI runners are slower, so use longer timeout and larger tolerance
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distance = info.scrollHeight - info.scrollTop - info.clientHeight;
      // Should still be near the bottom after font size change
      // Use 150px tolerance for CI where layout timing varies
      expect(distance).toBeLessThan(150);
    }).toPass({ timeout: TIMEOUTS.COMPLEX_OPERATION, intervals: POLLING_INTERVALS });

    // Restore font size
    await page.evaluate(() => {
      document.documentElement.style.fontSize = '';
    });
  });

  test('scrolls to show full content when posting a long multi-line message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Setup: Create user, space, and navigate to room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post initial messages via API to make the container scrollable
    const messages = Array.from({ length: 10 }, (_, i) => `Message ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for messages to appear in UI
    await expect(page.getByText(`Message 10 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Now post a long multi-line message (via UI since this tests the user posting behavior)
    const longMessage = `This is a very long message that spans multiple lines - ${timestamp}

Line 2: Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Line 3: Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
Line 4: Ut enim ad minim veniam, quis nostrud exercitation ullamco.
Line 5: Duis aute irure dolor in reprehenderit in voluptate velit.
Line 6: Excepteur sint occaecat cupidatat non proident.
Line 7: Sunt in culpa qui officia deserunt mollit anim id est laborum.
Line 8: This is the last line of this long message.`;

    await roomPage.messageInput.fill(longMessage);
    await roomPage.messageInput.press('Enter');

    // Wait for the message to appear
    await expect(page.getByText('Line 8: This is the last line of this long message.')).toBeVisible(
      {
        timeout: TIMEOUTS.UI_STANDARD
      }
    );

    // Verify we're at the bottom
    const messagesContainer = page.getByTestId('messages-container');
    await expect(async () => {
      const scrollInfo = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom =
        scrollInfo.scrollHeight - scrollInfo.scrollTop - scrollInfo.clientHeight;
      // Should be at or very near the bottom (within 50px tolerance)
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
  });

  test('bottom-aligns messages when conversation is shorter than viewport', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Create user, space, and enter room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Send just 2 messages — not enough to fill the viewport
    await postMessagesViaAPI(page, roomId, [
      `First message - ${timestamp}`,
      `Second message - ${timestamp}`
    ]);

    // Wait for messages to appear
    await expect(page.getByText(`Second message - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    const messagesContainer = page.getByTestId('messages-container');

    // Verify: content does NOT fill the viewport (no scrollbar)
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      expect(info.scrollHeight).toBeLessThanOrEqual(info.clientHeight + 5);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Verify: messages are bottom-aligned, not top-aligned
    const lastMessage = page.getByText(`Second message - ${timestamp}`);
    const containerBox = await messagesContainer.boundingBox();
    const messageBox = await lastMessage.boundingBox();

    expect(containerBox).not.toBeNull();
    expect(messageBox).not.toBeNull();

    // The last message's bottom edge should be near the container's bottom
    const containerBottom = containerBox!.y + containerBox!.height;
    const messageBottom = messageBox!.y + messageBox!.height;
    const distanceFromBottom = containerBottom - messageBottom;
    expect(distanceFromBottom).toBeLessThan(150);

    // Messages should NOT be stuck at the top (significant gap above them)
    const distanceFromTop = messageBox!.y - containerBox!.y;
    expect(distanceFromTop).toBeGreaterThan(100);
  });

  test('does not show false new messages indicator when returning to a room after navigating away', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    // Create space and enter general room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');

    // Extract roomId from URL for API-based message posting
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    const timestamp = Date.now();

    // Post enough messages via API to make container scrollable
    const longText =
      'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt.';
    const messages = Array.from(
      { length: 20 },
      (_, i) => `Message ${i + 1} - ${timestamp} - ${longText}`
    );
    await postMessagesViaAPI(page, roomId, messages);

    // Wait for the last message to be visible
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Get the messages container
    const messagesContainer = page.getByTestId('messages-container');

    // Scroll to top
    await scrollContainerToTop(page, messagesContainer);

    // Wait for scroll position to stabilize away from the bottom
    // Pagination may adjust position, but key is we're NOT at the bottom
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    // Verify no indicator initially
    await expect(page.getByRole('button', { name: /new messages/i })).not.toBeVisible();

    // Create a second room and navigate to it
    const secondRoomName = await chatPage.createRoom(`room-b-${timestamp}`);
    await chatPage.enterRoom(secondRoomName);

    // Verify we're in the second room
    await expect(page.getByRole('heading', { name: `# ${secondRoomName}` })).toBeVisible();

    // Navigate back to general room
    await chatPage.enterRoom('general');

    // Verify we're back in general room
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();

    // Wait for messages to load
    await expect(page.getByText(`Message 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The "New Messages" indicator should NOT appear (no new messages arrived).
    // Verify scrolled to bottom and indicator stays hidden using polling for reliability.
    await expect(async () => {
      // Verify we're at the bottom (fresh room entry should scroll to bottom)
      const info = await messagesContainer.evaluate((el) => ({
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(100);
      // Indicator should not be visible
      await expect(page.getByRole('button', { name: /new messages/i })).not.toBeVisible();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
  });
});
