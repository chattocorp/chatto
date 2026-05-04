import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { ChatPage } from './pages';
import { TIMEOUTS } from './constants';
import { waitForRoomReady } from './fixtures/realtimeSync';
import * as routes from './routes';

/**
 * Post messages via GraphQL API (much faster than UI-based posting).
 */
async function postMessagesViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  messages: string[]
): Promise<void> {
  for (const body of messages) {
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
        variables: { input: { spaceId, roomId, body } }
      }
    });
  }
}

/**
 * Get the room ID from the current page URL.
 */
function getRoomIdFromUrl(page: Page): string {
  const url = page.url();
  const match = url.match(/\/chat\/-\/([^/?]+)/);
  if (!match) throw new Error(`Could not extract room ID from URL: ${url}`);
  return match[1];
}

test.describe('Virtualizer stability', () => {
  test('rapid room switching with different message counts does not cause JS errors', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    const spaceId = await chatPage.getSpaceId();

    // Enter the default "general" room and post many messages
    await chatPage.enterRoom('general');
    const generalRoomId = getRoomIdFromUrl(page);

    const messages = Array.from({ length: 20 }, (_, i) => `General message ${i + 1}`);
    await postMessagesViaAPI(page, spaceId, generalRoomId, messages);
    await expect(page.getByText('General message 20')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Create a second room with only a few messages
    const secondRoomName = await chatPage.createRoom(`sparse-room-${Date.now()}`);
    const sparseRoomId = getRoomIdFromUrl(page);

    const sparseMessages = Array.from({ length: 3 }, (_, i) => `Sparse message ${i + 1}`);
    await postMessagesViaAPI(page, spaceId, sparseRoomId, sparseMessages);
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
      await chatPage.enterRoom('general');
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
    // User 1: Create space with two rooms
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();

    await chatPage.enterRoom('general');
    const generalRoomId = getRoomIdFromUrl(page);

    // Seed general room with messages so it has scroll content
    const seedMessages = Array.from({ length: 15 }, (_, i) => `Seed message ${i + 1}`);
    await postMessagesViaAPI(page, spaceId, generalRoomId, seedMessages);
    await expect(page.getByText('Seed message 15')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Create a second room
    const secondRoomName = await chatPage.createRoom(`other-room-${Date.now()}`);

    // User 2: Join the space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      // Navigate to the space so the room list is visible
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
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
          await postMessagesViaAPI(page2, spaceId, generalRoomId, [`Live message ${i + 1}`]);
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
    } finally {
      await context2.close();
    }
  });
});
