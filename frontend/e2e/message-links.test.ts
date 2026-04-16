import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';
import { ChatPage, RoomPage } from './pages';

// ---------------------------------------------------------------------------
// GraphQL helpers (per-file convention — each test file redeclares these)
// ---------------------------------------------------------------------------

/** Post a message via GraphQL API and return the event ID. */
async function postMessageAndGetId(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { spaceId, roomId, body } }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/** Post a thread reply via GraphQL API and return the event ID. */
async function postThreadReplyViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string,
  inThread: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { spaceId, roomId, body, inThread } }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/** Extract spaceId and roomId from the current URL. */
function getIdsFromUrl(page: Page): { spaceId: string; roomId: string } {
  const match = page.url().match(/\/chat\/-\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)/);
  if (!match) throw new Error(`Could not extract IDs from URL: ${page.url()}`);
  return { spaceId: match[1], roomId: match[2] };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Message links', () => {
  test.describe.configure({ timeout: 60_000 });

  test('navigating to /m/ URL for a room message redirects to the room with highlight', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { spaceId, roomId } = getIdsFromUrl(page);
    const timestamp = Date.now();
    const targetBody = `Target room message - ${timestamp}`;
    const eventId = await postMessageAndGetId(page, spaceId, roomId, targetBody);

    // Navigate directly to the /m/ URL
    await page.goto(routes.messageLink(spaceId, roomId, eventId));

    // Wait for the client-side redirect to the room URL (goto replaceState)
    await expect(async () => {
      expect(page.url()).not.toContain('/m/');
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The target message should be visible
    await expect(page.getByText(targetBody)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" should NOT appear — the linked message is already at
    // the end of the conversation, so we're already at the present.
    await expect(async () => {
      await expect(page.getByTestId('jump-to-present')).toHaveCount(0);
    }).toPass({
      timeout: TIMEOUTS.POLLING_EXTENDED,
      intervals: [...POLLING_INTERVALS]
    });
  });

  test('navigating to /m/ URL for a thread reply opens the thread pane', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { spaceId, roomId } = getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post root message + thread reply
    const rootBody = `Thread root - ${timestamp}`;
    const rootEventId = await postMessageAndGetId(page, spaceId, roomId, rootBody);

    const replyBody = `Thread reply - ${timestamp}`;
    const replyEventId = await postThreadReplyViaAPI(
      page,
      spaceId,
      roomId,
      replyBody,
      rootEventId
    );

    // Navigate directly to the reply's /m/ URL
    await page.goto(routes.messageLink(spaceId, roomId, replyEventId));

    // Wait for the client-side redirect to the thread URL
    await expect(async () => {
      expect(page.url()).not.toContain('/m/');
      expect(page.url()).toContain(rootEventId);
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Thread pane should be open
    await expect(roomPage.threadPane).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The reply should be visible in the thread pane
    await expect(roomPage.threadPane.getByText(replyBody)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });

  test('message link pasted in a posted message shows a preview card', async ({
    page,
    chatPage,
    roomPage,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { spaceId, roomId } = getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post the target message
    const targetBody = `Preview target - ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, spaceId, roomId, targetBody);

    // Post a message containing the target's message link URL
    const linkUrl = `${serverURL}${routes.messageLink(spaceId, roomId, targetEventId)}`;
    await roomPage.sendMessage(linkUrl);

    // Wait for the embedded preview card to appear
    const previewCard = page.getByTestId('message-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Preview should contain the target message body
    await expect(previewCard).toContainText(targetBody);
  });

  test('message link preview does not appear when viewer lacks permission', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // --- User A: create space + room + message ---
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { spaceId: spaceA, roomId: roomA } = getIdsFromUrl(page);
    const timestamp = Date.now();
    const secretBody = `Secret message - ${timestamp}`;
    const secretEventId = await postMessageAndGetId(page, spaceA, roomA, secretBody);
    const secretLinkUrl = `${serverURL}${routes.messageLink(spaceA, roomA, secretEventId)}`;

    // --- User B: separate space, NOT a member of User A's space ---
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();
    await createAndLoginTestUser(page2);

    const chatPage2 = new ChatPage(page2);
    const roomPage2 = new RoomPage(page2);

    await chatPage2.goto();
    await chatPage2.createSpace();
    await chatPage2.enterRoom('general');
    await waitForRoomReady(page2, 'general');

    // User B posts a message containing User A's message link
    await roomPage2.sendMessage(`Check this: ${secretLinkUrl}`);

    // The message text should appear
    await expect(page2.getByText(secretLinkUrl, { exact: false })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // No preview card should appear (User B has no access to User A's room).
    // Use toPass with polling to give it time to NOT appear.
    await expect(async () => {
      await expect(page2.getByTestId('message-preview-card')).toHaveCount(0);
    }).toPass({
      timeout: TIMEOUTS.POLLING_EXTENDED,
      intervals: [...POLLING_INTERVALS]
    });

    await context2.close();
  });

  test('Jump to Present dismisses when scrolling to end after jumping to an old message', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { spaceId, roomId } = getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post an old target message, then fill with enough messages to push it out of view
    const targetBody = `Old target - ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, spaceId, roomId, targetBody);

    const fillerMessages = Array.from({ length: 60 }, (_, i) => `Filler ${i + 1} - ${timestamp}`);
    for (const body of fillerMessages) {
      await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
          variables: { input: { spaceId, roomId, body } }
        }
      });
    }

    // Navigate to the old message's link
    await page.goto(routes.messageLink(spaceId, roomId, targetEventId));

    // Wait for redirect and old message to be visible
    await expect(async () => {
      expect(page.url()).not.toContain('/m/');
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });

    await expect(page.getByText(targetBody)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" SHOULD appear (we jumped to an old message)
    await expect(page.getByTestId('jump-to-present')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Click "Jump to Present" to return to the latest messages
    await page.getByTestId('jump-to-present').click();

    // The latest filler should become visible
    await expect(page.getByText(`Filler 60 - ${timestamp}`)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" button should disappear after returning to present
    await expect(page.getByTestId('jump-to-present')).not.toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('clicking a message link in body navigates in-app without opening a new window', async ({
    page,
    chatPage,
    roomPage,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { spaceId, roomId } = getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post the target message
    const targetBody = `Navigation target - ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, spaceId, roomId, targetBody);

    // Post a message containing the message link
    const linkUrl = `${serverURL}${routes.messageLink(spaceId, roomId, targetEventId)}`;
    await roomPage.sendMessage(`Go to ${linkUrl}`);

    // Wait for the link to render in the message body (inside .prose, not the preview card)
    const message = page.locator('[role="article"]', { hasText: linkUrl });
    const link = message.locator(`.prose a[href*="/m/${targetEventId}"]`);
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Count pages (tabs) before clicking
    const pageCountBefore = page.context().pages().length;

    // Click the link
    await link.click();

    // Should navigate within the same tab — no new pages opened
    expect(page.context().pages().length).toBe(pageCountBefore);

    // URL should have changed (redirect from /m/ route)
    await page.waitForURL(routes.patterns.anyRoomWithQuery, {
      timeout: TIMEOUTS.UI_STANDARD
    });

    // The target message should be visible (scope to <p> to avoid matching the preview card snippet)
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });
});
