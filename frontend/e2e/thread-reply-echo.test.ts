import { expect, type Page } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { waitForSpaceUnread, getRoomIdByName, waitForRoomRead } from './fixtures/graphqlHelpers';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

async function getIdsFromUrl(page: Page): Promise<{ spaceId: string; roomId: string }> {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  const roomId = match[1];
  const data = await page.evaluate(async () => {
    const r = await fetch('/api/graphql', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ query: `query { instance { primarySpaceId } }` })
    });
    return r.json();
  });
  return { spaceId: data.data.instance.primarySpaceId, roomId };
}

/** Post a message via API and return its event ID. */
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

/** Post a thread reply via API and return its event ID. */
async function postThreadReplyViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string,
  inThread: string,
  inReplyTo?: string
): Promise<string> {
  const input: Record<string, unknown> = { spaceId, roomId, body, inThread };
  if (inReplyTo) input.inReplyTo = inReplyTo;
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/** Post a thread reply with echo via API and return its event ID. */
async function postThreadReplyWithEchoViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string,
  inThread: string,
  inReplyTo: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: {
        input: { spaceId, roomId, body, inThread, inReplyTo, alsoSendToChannel: true }
      }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

test.describe('Thread Reply Echo ("Also send to channel")', () => {
  test('"Also send to channel" checkbox is visible in thread composer', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for checkbox test ${Date.now()}`;
    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Open thread and verify checkbox is visible', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      // Verify the checkbox is visible
      const checkbox = page.getByLabel('Also send to channel');
      await expect(checkbox).toBeVisible();
    });
  });

  test('echo appears in main channel when checkbox is checked', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for echo check ${Date.now()}`;
    const replyMessage = `Reply with echo ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Open thread, check checkbox, and post reply', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      // Check the checkbox
      const checkbox = page.getByLabel('Also send to channel');
      await checkbox.check();
      await expect(checkbox).toBeChecked();

      // Post the reply
      await roomPage.postThreadReply(replyMessage);

      // Verify reply appears in thread
      await roomPage.expectTextInThreadPane(replyMessage);
    });

    await test.step('Close thread and verify echo appears in main room', async () => {
      await roomPage.closeThread();

      // Wait for echo to arrive via WebSocket and become visible
      const echoArticle = page.locator('[role="article"]', { hasText: replyMessage });
      await expect(echoArticle.first()).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    });

    await test.step('Verify echo has "Thread" badge', async () => {
      const echoArticle = page.locator('[role="article"]', { hasText: replyMessage });
      const badge = echoArticle.getByText('Thread');
      await expect(badge).toBeVisible();
    });
  });

  test('reply without checkbox does not echo to channel', async ({ page, chatPage, roomPage }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for no-echo test ${Date.now()}`;
    const replyMessage = `Reply without echo ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Open thread and post reply WITHOUT checking checkbox', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      // Make sure checkbox is NOT checked
      const checkbox = page.getByLabel('Also send to channel');
      if (await checkbox.isChecked()) {
        await checkbox.uncheck();
      }

      // Post the reply
      await roomPage.postThreadReply(replyMessage);

      // Verify reply appears in thread
      await roomPage.expectTextInThreadPane(replyMessage);
    });

    await test.step('Close thread and verify NO echo in main room', async () => {
      await roomPage.closeThread();

      // Assert no echo appeared — use toHaveCount with retry to allow settlement
      const replies = page.locator('[role="article"]', { hasText: replyMessage });
      await expect(replies).toHaveCount(0, { timeout: TIMEOUTS.UI_STANDARD });
    });
  });

  test('user B sees echo from user A in real-time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    let spaceId: string;

    await test.step('User A creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      spaceId = await chatPage.getSpaceId();
    });

    const rootMessage = `Root for realtime echo ${Date.now()}`;
    const replyMessage = `Reply with realtime echo ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await test.step('User B joins the space', async () => {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);
      });

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await test.step('User B enters general room and sees root message', async () => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(rootMessage);
      });

      await test.step('User A posts reply with echo', async () => {
        await rootMessageComponent.openThread();
        await roomPage.expectThreadPaneVisible();

        // Check the checkbox and post
        const checkbox = page.getByLabel('Also send to channel');
        await checkbox.check();
        await roomPage.postThreadReply(replyMessage);
      });

      await test.step('User B receives echo in real-time', async () => {
        // Close thread on User A's side
        await roomPage.closeThread();

        // User B should see the echo appear in their main room
        const replies = page2.locator('[role="article"]', { hasText: replyMessage });
        await expect(replies.first()).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

        // Verify it has the thread badge
        const echoArticle = page2.locator('[role="article"]', { hasText: replyMessage });
        const badge = echoArticle.getByText('Thread');
        await expect(badge).toBeVisible();
      });
    } finally {
      await context2.close();
    }
  });

  test('clicking "Thread" on echo opens original thread', async ({ page, chatPage, roomPage }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for thread nav ${Date.now()}`;
    const replyMessage = `Reply for nav test ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      // Check checkbox and post
      const checkbox = page.getByLabel('Also send to channel');
      await checkbox.check();
      await roomPage.postThreadReply(replyMessage);
    });

    await test.step('Close thread and click "Thread" badge on echo', async () => {
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();

      // Wait for echo to appear before clicking its badge
      const echoArticle = page.locator('[role="article"]', { hasText: replyMessage });
      await expect(echoArticle).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      const threadLink = echoArticle.getByText('Thread');
      await threadLink.click();
    });

    await test.step('Thread pane opens showing original thread', async () => {
      await roomPage.expectThreadPaneVisible();
      await roomPage.expectThreadRouteActive();

      // Root message and reply should be visible in thread
      await roomPage.expectTextInThreadPane(rootMessage);
      await roomPage.expectTextInThreadPane(replyMessage);
    });
  });

  test('echo and original share reactions', async ({ page, chatPage, roomPage }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root ${Date.now()}`;
    const replyMessage = `Reply for reactions ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReplyWithEcho(replyMessage);
    });

    await test.step('React to original in thread pane', async () => {
      const threadReply = roomPage.getThreadMessage(replyMessage);
      await threadReply.react('👍');
      await threadReply.expectReaction('👍', 1);
    });

    await test.step('Close thread and verify echo ALSO has the reaction', async () => {
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();

      // Wait for echo to be visible in main room
      const echo = roomPage.getMessage(replyMessage);
      await expect(echo.locator).toBeVisible();

      // The echo should have the 👍 reaction (reactions are shared)
      await echo.expectReaction('👍', 1);
    });

    await test.step('React to echo and verify thread original also has it', async () => {
      const echo = roomPage.getMessage(replyMessage);
      await echo.react('❤️');
      await echo.expectReaction('❤️', 1);

      // Open thread and verify original ALSO has ❤️
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      const threadReply = roomPage.getThreadMessage(replyMessage);
      // Original should have both 👍 and ❤️ (shared reactions)
      await threadReply.expectReaction('👍', 1);
      await threadReply.expectReaction('❤️', 1);
    });
  });

  test('editing echo in main room propagates to thread original', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for edit-echo test ${Date.now()}`;
    const replyMessage = `Reply to edit via echo ${Date.now()}`;
    const editedMessage = `Edited via echo ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    let echoEventId: string | null;

    await test.step('Post reply with echo and close thread', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReplyWithEcho(replyMessage);
      await roomPage.closeThread();

      // Wait for echo to arrive and become visible
      const echo = roomPage.getMessage(replyMessage);
      await expect(echo.locator).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      echoEventId = await echo.getEventId();
    });

    await test.step('Edit the echo in main room', async () => {
      const echo = roomPage.getMessage(replyMessage);
      await echo.startEdit();
      await roomPage.expectEditModeActive();
      await roomPage.completeEdit(editedMessage);
    });

    await test.step('Verify echo shows edited content', async () => {
      // Use event ID for stable lookup since text changed
      const echo = roomPage.getMessageByEventId(echoEventId!);
      await expect(echo.locator.getByText(editedMessage)).toBeVisible();
      await echo.expectEdited();
    });

    await test.step('Open thread and verify original also shows edited content', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      // The thread reply should show the edited text
      await roomPage.expectTextInThreadPane(editedMessage);

      const threadReply = roomPage.getThreadMessage(editedMessage);
      await threadReply.expectEdited();
    });
  });

  test('editing original in thread propagates to echo in main room', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for edit-thread test ${Date.now()}`;
    const replyMessage = `Reply to edit via thread ${Date.now()}`;
    const editedMessage = `Edited via thread ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReplyWithEcho(replyMessage);
    });

    await test.step('Edit the reply in the thread pane', async () => {
      const threadReply = roomPage.getThreadMessage(replyMessage);
      await threadReply.startEdit();
      await roomPage.expectThreadEditModeActive();
      await roomPage.completeThreadEdit(editedMessage);
    });

    await test.step('Verify thread reply shows edited content', async () => {
      const threadReply = roomPage.getThreadMessage(editedMessage);
      await expect(threadReply.locator).toBeVisible();
      await threadReply.expectEdited();
    });

    await test.step('Close thread and verify echo in main room also shows edited content', async () => {
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();

      // The echo in the main room should show the edited text
      await roomPage.expectMessageVisible(editedMessage);
      const echo = roomPage.getMessage(editedMessage);
      await echo.expectEdited();
    });
  });

  test('deleting echo deletes both echo and thread original', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for delete-echo test ${Date.now()}`;
    const replyMessage = `Reply to delete via echo ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    let echoEventId: string | null;

    await test.step('Post reply with echo and close thread', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReplyWithEcho(replyMessage);
      await roomPage.closeThread();

      // Wait for echo to arrive and become visible
      const echo = roomPage.getMessage(replyMessage);
      await expect(echo.locator).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      echoEventId = await echo.getEventId();
    });

    await test.step('Delete the echo in main room', async () => {
      const echo = roomPage.getMessage(replyMessage);
      await echo.delete();
    });

    await test.step('Verify echo is hidden (no body, no reactions, no replies)', async () => {
      const echo = roomPage.getMessageByEventId(echoEventId!);
      await echo.expectHidden();
    });

    await test.step('Open thread and verify original is also hidden', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      // The thread reply should also be hidden (no body, no reactions, no replies)
      await expect(roomPage.threadPane.getByText(replyMessage)).not.toBeVisible();
      await expect(roomPage.threadPane.getByText('[Message deleted]')).not.toBeVisible();
    });
  });

  test('deleting thread original deletes both thread original and echo', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for delete-thread test ${Date.now()}`;
    const replyMessage = `Reply to delete via thread ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReplyWithEcho(replyMessage);
    });

    await test.step('Delete the reply in the thread pane', async () => {
      const threadReply = roomPage.getThreadMessage(replyMessage);
      await threadReply.delete();
    });

    await test.step('Verify thread reply is hidden (no body, no reactions, no replies)', async () => {
      await expect(roomPage.threadPane.getByText(replyMessage)).not.toBeVisible();
      await expect(roomPage.threadPane.getByText('[Message deleted]')).not.toBeVisible();
    });

    await test.step('Close thread and verify echo in main room is also hidden', async () => {
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();

      // The echo should also be hidden (no body, no reactions, no replies)
      await expect(page.getByText(replyMessage)).not.toBeVisible();
    });
  });

  test('only root messages show reply count, not echoes', async ({ page, chatPage, roomPage }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for count test ${Date.now()}`;
    const replyMessage = `Reply for count test ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo', async () => {
      await rootMessageComponent.openThread();
      await roomPage.postThreadReplyWithEcho(replyMessage);
    });

    await test.step('Close thread and verify reply count appears on root only', async () => {
      await roomPage.closeThread();

      // Reload to get a clean state with server-side reply count
      await page.reload();
      await roomPage.expectMessageVisible(rootMessage);

      // Wait for "1 reply" to be visible on the root message
      const replyCountText = page.getByText('1 reply');
      await expect(replyCountText.first()).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Check that "Thread" badge appears (on the echo)
      const echoArticle = page.locator('[role="article"]', { hasText: replyMessage });
      const threadBadge = echoArticle.getByText('Thread');
      await expect(threadBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    });
  });

  test('echo with inReplyTo shows reply attribution and clicking it opens thread at referenced message', async ({
    page,
    chatPage
  }) => {
    const timestamp = Date.now();

    await test.step('Setup: User creates space and enters room', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const { spaceId, roomId } = await getIdsFromUrl(page);

    // Post a root message that starts the thread
    const rootBody = `Thread root ${timestamp}`;
    const rootEventId = await postMessageAndGetId(page, spaceId, roomId, rootBody);

    // Post a first thread reply — this is the message we'll reply to
    const targetBody = `Target reply in thread ${timestamp}`;
    const targetEventId = await postThreadReplyViaAPI(
      page,
      spaceId,
      roomId,
      targetBody,
      rootEventId
    );

    // Post filler thread replies to push the target off-screen
    for (let i = 0; i < 15; i++) {
      await postThreadReplyViaAPI(
        page,
        spaceId,
        roomId,
        `Filler thread reply ${i + 1} - ${timestamp}`,
        rootEventId
      );
    }

    // Post a thread reply that references targetEventId AND echoes to channel
    const echoReplyBody = `Echo reply pointing to target ${timestamp}`;
    await postThreadReplyWithEchoViaAPI(
      page,
      spaceId,
      roomId,
      echoReplyBody,
      rootEventId,
      targetEventId
    );

    await test.step('Reload and verify echo shows reply attribution', async () => {
      await page.reload();
      await expect(page.getByText(echoReplyBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // The echo should have reply attribution
      const echoArticle = page.locator('[role="article"]', { hasText: echoReplyBody });
      const attribution = echoArticle.getByTestId('reply-attribution');
      await expect(attribution).toBeVisible();
      await expect(attribution.getByText('in reply to')).toBeVisible();
    });

    await test.step('Click reply attribution and verify thread opens with target visible', async () => {
      const echoArticle = page.locator('[role="article"]', { hasText: echoReplyBody });
      const attribution = echoArticle.getByTestId('reply-attribution');

      // Click "in reply to" (not the author button which has stopPropagation)
      await attribution.getByText('in reply to').click();

      // Thread pane should open
      await expect(page.getByTestId('thread-pane')).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // The target message should be scrolled into view and visible in the thread
      await expect(
        page.getByTestId('thread-pane').locator('p', { hasText: targetBody })
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // The target message should have the highlight-flash animation (1.5s duration)
      const targetEvent = page
        .getByTestId('thread-pane')
        .locator(`[data-event-id="${targetEventId}"]`);
      await expect(targetEvent).toHaveClass(/highlight-flash/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });
  });

  test('reacting to echo via emoji picker works', async ({ page, chatPage, roomPage }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for emoji test ${Date.now()}`;
    const replyMessage = `Reply for emoji test ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo and close thread', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReplyWithEcho(replyMessage);
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();
    });

    await test.step('React to echo in main room and verify reaction sticks', async () => {
      const echo = roomPage.getMessage(replyMessage);
      await expect(echo.locator).toBeVisible();
      await echo.react('👍');
      await echo.expectReaction('👍', 1);
    });
  });

  test('checkbox resets to unchecked after posting with echo', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await test.step('Setup: User creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const rootMessage = `Root for reset test ${Date.now()}`;
    const firstReply = `First reply with echo ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    await test.step('Post reply with echo checked', async () => {
      await rootMessageComponent.openThread();
      await roomPage.expectThreadPaneVisible();

      const checkbox = page.getByLabel('Also send to channel');
      await checkbox.check();
      await expect(checkbox).toBeChecked();

      await roomPage.postThreadReply(firstReply);
      await roomPage.expectTextInThreadPane(firstReply);
    });

    await test.step('Verify checkbox is unchecked for the next reply', async () => {
      const checkbox = page.getByLabel('Also send to channel');
      await expect(checkbox).not.toBeChecked();
    });
  });

  test('checkbox is hidden when message.echo permission is denied', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    let spaceId: string;

    await test.step('User A creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      spaceId = await chatPage.getSpaceId();
    });

    const rootMessage = `Root for permission test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    await test.step('Deny message.echo on everyone role', async () => {
      const resp = await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: DenySpacePermissionInput!) { denySpacePermission(input: $input)
					}`,
          variables: { input: { spaceId, role: 'everyone', permission: 'message.echo' } }
        }
      });
      expect(resp.ok()).toBeTruthy();
    });

    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await test.step('User B joins space and enters room', async () => {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);

        const chatPage2 = new ChatPage(page2);
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
      });

      await test.step('User B opens thread and checkbox is NOT visible', async () => {
        const roomPage2 = new RoomPage(page2);
        await roomPage2.expectMessageVisible(rootMessage);
        const rootMsg = roomPage2.getMessage(rootMessage);
        await rootMsg.openThread();
        await roomPage2.expectThreadPaneVisible();

        const checkbox = page2.getByLabel('Also send to channel');
        await expect(checkbox).not.toBeVisible();
      });
    } finally {
      await context2.close();
    }
  });

  test('echo triggers unread indicator for other users', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    let spaceId: string;

    await test.step('User A creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      spaceId = await chatPage.getSpaceId();
    });

    const rootMessage = `Root for unread test ${Date.now()}`;
    const replyMessage = `Echo for unread test ${Date.now()}`;

    const rootMessageComponent = await roomPage.sendMessage(rootMessage);

    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await test.step('User B joins space and marks room as read', async () => {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);

        const chatPage2 = new ChatPage(page2);
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // Wait for markRoomAsRead to complete on server before navigating away.
        const roomId = await getRoomIdByName(page2, spaceId, 'general');
        await waitForRoomRead(page2, spaceId, roomId);

        // Navigate to announcements so general is no longer active.
        // Note: navigating to /chat/-/${spaceId} doesn't work because the route
        // auto-redirects to the last visited room (general), which re-mounts the
        // room component and auto-marks it as read — preventing unread detection.
        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');
      });

      await test.step('User A posts reply with echo', async () => {
        await rootMessageComponent.openThread();
        await roomPage.expectThreadPaneVisible();
        await roomPage.postThreadReplyWithEcho(replyMessage);
      });

      await test.step('User B sees unread indicator from echo', async () => {
        // Wait for server to register unread state
        await waitForSpaceUnread(page2, spaceId, true);

        // Verify UI shows the unread dot
        await expect(async () => {
          const generalLink = page2.locator('nav').locator('a', { hasText: '# general' });
          const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
          await expect(unreadDot).toBeVisible();
        }).toPass({
          timeout: TIMEOUTS.REALTIME_EVENT,
          intervals: [100, 250, 500, 1000]
        });
      });
    } finally {
      await context2.close();
    }
  });

  test('echo reply attribution highlight works for non-root target after thread was already opened', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const timestamp = Date.now();

    await test.step('Setup: User creates space and enters room', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const { spaceId, roomId } = await getIdsFromUrl(page);

    // Post a root message that starts the thread
    const rootBody = `Thread root ${timestamp}`;
    const rootEventId = await postMessageAndGetId(page, spaceId, roomId, rootBody);

    // Post a target reply in the thread
    const targetBody = `Target reply ${timestamp}`;
    const targetEventId = await postThreadReplyViaAPI(
      page,
      spaceId,
      roomId,
      targetBody,
      rootEventId
    );

    // Echo #1: replies to the root message
    const echo1Body = `Echo replying to root ${timestamp}`;
    await postThreadReplyWithEchoViaAPI(
      page,
      spaceId,
      roomId,
      echo1Body,
      rootEventId,
      rootEventId // inReplyTo = root
    );

    // Echo #2: replies to the non-root target message
    const echo2Body = `Echo replying to target ${timestamp}`;
    await postThreadReplyWithEchoViaAPI(
      page,
      spaceId,
      roomId,
      echo2Body,
      rootEventId,
      targetEventId // inReplyTo = non-root
    );

    // Reload to get all echoes rendered
    await page.reload();
    await expect(page.getByText(echo1Body)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(page.getByText(echo2Body)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    await test.step('Click echo #1 reply attribution (reply to root) — should highlight root', async () => {
      const echo1Article = page.locator('[role="article"]', { hasText: echo1Body });
      const attribution1 = echo1Article.getByTestId('reply-attribution');
      await attribution1.getByText('in reply to').click();

      // Thread pane should open
      await expect(page.getByTestId('thread-pane')).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Root message should be visible and highlighted
      const rootEvent = page.getByTestId('thread-pane').locator(`[data-event-id="${rootEventId}"]`);
      await expect(rootEvent).toHaveClass(/highlight-flash/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });

    await test.step('Close thread', async () => {
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();
    });

    await test.step('Click echo #2 reply attribution (reply to non-root) — should highlight target', async () => {
      const echo2Article = page.locator('[role="article"]', { hasText: echo2Body });
      const attribution2 = echo2Article.getByTestId('reply-attribution');
      await attribution2.getByText('in reply to').click();

      // Thread pane should open
      await expect(page.getByTestId('thread-pane')).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Target message should be visible in the thread
      await expect(
        page.getByTestId('thread-pane').locator('p', { hasText: targetBody })
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Target message should be highlighted
      const targetEvent = page
        .getByTestId('thread-pane')
        .locator(`[data-event-id="${targetEventId}"]`);
      await expect(targetEvent).toHaveClass(/highlight-flash/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });
  });

  test('echo reply attribution highlight works for non-root target in scrollable cached thread', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const timestamp = Date.now();

    await test.step('Setup: User creates space and enters room', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const { spaceId, roomId } = await getIdsFromUrl(page);

    // Post root message
    const rootBody = `Thread root ${timestamp}`;
    const rootEventId = await postMessageAndGetId(page, spaceId, roomId, rootBody);

    // Post target reply early in the thread (2nd message)
    const targetBody = `Target reply ${timestamp}`;
    const targetEventId = await postThreadReplyViaAPI(
      page,
      spaceId,
      roomId,
      targetBody,
      rootEventId
    );

    // Post enough thread replies to make the thread pane scrollable.
    // The target is near the top, so auto-scroll-to-bottom would scroll past it.
    for (let i = 0; i < 25; i++) {
      await postThreadReplyViaAPI(
        page,
        spaceId,
        roomId,
        `Filler reply ${i} ${timestamp}`,
        rootEventId
      );
    }

    // Post an echo that replies to the non-root target
    const echoBody = `Echo to target in long thread ${timestamp}`;
    await postThreadReplyWithEchoViaAPI(
      page,
      spaceId,
      roomId,
      echoBody,
      rootEventId,
      targetEventId
    );

    // Wait for echo to appear in main room
    await expect(page.getByText(echoBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    await test.step('Open thread first to cache data, then close', async () => {
      // Navigate directly to thread URL to cache the data
      await page.goto(routes.thread(roomId, rootEventId));
      await roomPage.expectThreadPaneVisible();

      // Wait for thread data to finish loading (last filler should be visible)
      await expect(
        page.getByTestId('thread-pane').getByText(`Filler reply 24 ${timestamp}`)
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Close thread
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();
    });

    await test.step('Click echo reply attribution — should highlight target in cached thread', async () => {
      const echoArticle = page.locator('[role="article"]', { hasText: echoBody });
      const attribution = echoArticle.getByTestId('reply-attribution');
      await attribution.getByText('in reply to').click();

      // Thread pane should open
      await expect(page.getByTestId('thread-pane')).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Target message should be highlighted (proving it was scrolled into view).
      // The bug: auto-scroll fires during the fly transition (data is cached),
      // scrolling to bottom before the highlight can scroll to the target.
      const targetEvent = page
        .getByTestId('thread-pane')
        .locator(`[data-event-id="${targetEventId}"]`);
      await expect(targetEvent).toHaveClass(/highlight-flash/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });
  });

  test('in-thread reply highlight works for non-root target via UI', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const timestamp = Date.now();

    await test.step('Setup: User creates space and enters room', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const { spaceId, roomId } = await getIdsFromUrl(page);

    // Post root message
    const rootBody = `Thread root ${timestamp}`;
    const rootEventId = await postMessageAndGetId(page, spaceId, roomId, rootBody);

    // Post target reply (this is what we'll reply to)
    const targetBody = `Target reply ${timestamp}`;
    const targetEventId = await postThreadReplyViaAPI(
      page,
      spaceId,
      roomId,
      targetBody,
      rootEventId
    );

    // Reload to see root message
    await page.reload();
    await expect(page.getByText(rootBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    await test.step('Open thread and use UI to reply to target message', async () => {
      // Open thread
      const rootMessage = roomPage.getMessage(rootBody);
      await rootMessage.openThread();
      await roomPage.expectThreadPaneVisible();

      // Wait for target reply to appear
      await expect(page.getByTestId('thread-pane').getByText(targetBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Right-click target reply and click "Reply" to set inReplyTo
      const targetMessage = new (await import('./pages')).MessageComponent(
        page,
        page.getByTestId('thread-pane').locator('[role="article"]', { hasText: targetBody })
      );
      await targetMessage.replyInRoom();

      // Verify reply attribution preview
      await expect(page.getByTestId('thread-pane').getByText('Replying to')).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // Post the reply
      const replyBody = `Reply to target ${timestamp}`;
      await page.getByTestId('thread-reply-input').fill(replyBody);
      await page.getByTestId('thread-reply-input').press('Enter');

      // Wait for reply to appear
      await expect(page.getByTestId('thread-pane').getByText(replyBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });

    await test.step('Click in-reply-to on new reply — should highlight target', async () => {
      const replyBody = `Reply to target ${timestamp}`;
      const replyArticle = page
        .getByTestId('thread-pane')
        .locator('[role="article"]', { hasText: replyBody });
      const attribution = replyArticle.getByTestId('reply-attribution');
      await expect(attribution).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await attribution.getByText('in reply to').click();

      // Target message should be highlighted
      const targetEvent = page
        .getByTestId('thread-pane')
        .locator(`[data-event-id="${targetEventId}"]`);
      await expect(targetEvent).toHaveClass(/highlight-flash/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });
  });

  test('echo reply attribution highlight works when echo created via UI reply flow', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const timestamp = Date.now();

    await test.step('Setup: User creates space and enters room', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const { spaceId, roomId } = await getIdsFromUrl(page);

    // Post root message via API
    const rootBody = `Thread root ${timestamp}`;
    const rootEventId = await postMessageAndGetId(page, spaceId, roomId, rootBody);

    // Post target reply via API (so we know its event ID for verification)
    const targetBody = `Target for UI reply ${timestamp}`;
    const targetEventId = await postThreadReplyViaAPI(
      page,
      spaceId,
      roomId,
      targetBody,
      rootEventId
    );

    // Reload to see the root message with thread indicator
    await page.reload();
    await expect(page.getByText(rootBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    await test.step('Open thread and use UI to reply to non-root target with echo', async () => {
      // Open thread by clicking the root message's thread indicator
      const rootMessage = roomPage.getMessage(rootBody);
      await rootMessage.openThread();
      await roomPage.expectThreadPaneVisible();

      // Wait for the target reply to appear in the thread pane
      await expect(page.getByTestId('thread-pane').getByText(targetBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Right-click the target reply and click "Reply" to set inReplyTo
      const targetMessage = new (await import('./pages')).MessageComponent(
        page,
        page.getByTestId('thread-pane').locator('[role="article"]', { hasText: targetBody })
      );
      await targetMessage.replyInRoom();

      // Verify the reply attribution preview appears in the composer
      await expect(page.getByTestId('thread-pane').getByText('Replying to')).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // Type the echo reply and check "Also send to channel"
      const echoBody = `UI echo reply ${timestamp}`;
      const checkbox = page.getByLabel('Also send to channel');
      await expect(checkbox).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await checkbox.check();

      // Post the reply
      await page.getByTestId('thread-reply-input').fill(echoBody);
      await page.getByTestId('thread-reply-input').press('Enter');

      // Wait for the echo reply to appear in the thread
      await expect(page.getByTestId('thread-pane').getByText(echoBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });

    await test.step('Close thread', async () => {
      await roomPage.closeThread();
      await roomPage.expectThreadRouteClosed();
    });

    const echoBody = `UI echo reply ${timestamp}`;

    await test.step('Echo should be visible in room with reply attribution', async () => {
      await expect(page.getByText(echoBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      const echoArticle = page.locator('[role="article"]', { hasText: echoBody });
      await expect(echoArticle.getByTestId('reply-attribution')).toBeVisible();
    });

    await test.step('Click echo reply attribution — should highlight target in thread', async () => {
      const echoArticle = page.locator('[role="article"]', { hasText: echoBody });
      const attribution = echoArticle.getByTestId('reply-attribution');
      await attribution.getByText('in reply to').click();

      // Thread pane should open
      await expect(page.getByTestId('thread-pane')).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Target message should be highlighted
      const targetEvent = page
        .getByTestId('thread-pane')
        .locator(`[data-event-id="${targetEventId}"]`);
      await expect(targetEvent).toHaveClass(/highlight-flash/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });
  });
});
