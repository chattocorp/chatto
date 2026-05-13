import { expect, type Page } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { ChatPage, RoomPage } from './pages';
import * as routes from './routes';

/**
 * Post a message and return its event ID.
 */
async function postMessageAndGetId(
  page: Page,
  roomId: string,
  body: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { roomId, body } }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/**
 * Post a message with inReplyTo attribution via GraphQL API.
 */
async function postReplyViaAPI(
  page: Page,
  roomId: string,
  body: string,
  inReplyTo: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { roomId, body, inReplyTo } }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

async function getIdsFromUrl(page: Page): Promise<{ spaceId: string; roomId: string }> {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  return { spaceId: 'server', roomId: match[1] };
}

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

test.describe('Message Threading', () => {
  test('thread reply from another user appears in real-time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await test.step('User A creates space and posts root message', async () => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
    });

    const spaceId = await chatPage.getSpaceId();

    const rootMessage = `Root message ${Date.now()}`;
    let message1: Awaited<ReturnType<typeof roomPage.sendMessage>>;
    await test.step('User A posts root message', async () => {
      message1 = await roomPage.sendMessage(rootMessage);
    });

    // User B: Create a second context and login
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await test.step('User B creates account and logs in', async () => {
        await createAndLoginTestUser(page2);
      });

      await test.step('User B joins the space', async () => {
        await joinSpace(page2);
        await page2.goto(routes.space());
      });

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await test.step('User B enters the general room (auto-joined)', async () => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
      });

      await test.step('User A opens thread pane', async () => {
        await message1.openThread();
        await roomPage.expectThreadPaneVisible();
      });

      await test.step('User B opens thread pane', async () => {
        const message2 = roomPage2.getMessage(rootMessage);
        await message2.openThread();
        await roomPage2.expectThreadPaneVisible();
      });

      const replyMessage = `Reply from User B ${Date.now()}`;
      await test.step('User B posts reply', async () => {
        await roomPage2.postThreadReply(replyMessage);
      });

      await test.step('User A receives reply in real-time', async () => {
        await roomPage.expectTextInThreadPane(replyMessage);
      });
    } finally {
      await context2.close();
    }
  });

  test('thread reply deletion propagates to other connected clients in real-time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // Reproduces Felix's bug in the thread case: with the thread pane open on
    // user B, user A deletes their own thread reply and B should see it
    // disappear without a refresh. The store-level fix applies to threads
    // because ThreadMessagesStore inherits ingestSpaceEvent from MessageListStore.
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');
    const spaceId = await chatPage.getSpaceId();

    const rootMessage = `Thread root ${Date.now()}`;
    const message1 = await roomPage.sendMessage(rootMessage);

    await message1.openThread();
    await roomPage.expectThreadPaneVisible();

    const replyText = `Reply to delete ${Date.now()}`;
    await roomPage.postThreadReply(replyText);

    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      const rootForB = roomPage2.getMessage(rootMessage);
      await rootForB.openThread();
      await roomPage2.expectThreadPaneVisible();

      // User B sees the reply that user A posted.
      await roomPage2.expectTextInThreadPane(replyText);

      // User A deletes the reply.
      const replyForA = roomPage.getThreadMessage(replyText);
      await replyForA.delete();

      // User B should see the reply replaced by the tombstone — without refresh.
      await expect(roomPage2.threadPane.getByText(replyText)).not.toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await expect(
        roomPage2.threadPane.getByText('This message has been deleted').first()
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('thread reply edit propagates to other connected clients in real-time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // Edits use the refetch path, which is the same chain as deletion before
    // the fix. Locking it in for threads so a future regression in the
    // refetchByMessageEventId branch surfaces here.
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');
    const spaceId = await chatPage.getSpaceId();

    const rootMessage = `Thread root for edit ${Date.now()}`;
    const message1 = await roomPage.sendMessage(rootMessage);

    await message1.openThread();
    await roomPage.expectThreadPaneVisible();

    const originalReply = `Original reply ${Date.now()}`;
    await roomPage.postThreadReply(originalReply);

    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      const rootForB = roomPage2.getMessage(rootMessage);
      await rootForB.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.expectTextInThreadPane(originalReply);

      // User A edits the reply.
      const replyForA = roomPage.getThreadMessage(originalReply);
      await replyForA.startEdit();
      await roomPage.expectThreadEditModeActive();
      const editedReply = `Edited reply ${Date.now()}`;
      await roomPage.threadReplyInput.fill(editedReply);
      await roomPage.threadReplyInput.press('Enter');

      // User B should see the new content and the (edited) marker.
      await expect(roomPage2.threadPane.getByText(editedReply)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await expect(roomPage2.threadPane.getByText(originalReply)).not.toBeVisible();
      const editedForB = roomPage2.getThreadMessage(editedReply);
      await editedForB.expectEdited();
    } finally {
      await context2.close();
    }
  });

  test('thread indicator shows reply count and participant avatars', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Thread count test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread and post a reply
    await message.openThread();
    await roomPage.expectThreadRouteActive();

    const replyMessage = `Reply 1 ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);

    // Wait for the reply to appear in the thread pane before closing
    await roomPage.expectTextInThreadPane(replyMessage);

    // Close the thread pane
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();

    // The message in the main view should show a thread indicator with "1 reply"
    await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The thread indicator button should contain at least one avatar
    const threadButton = page.getByRole('button', { name: /1 reply/i });
    await expect(threadButton).toBeVisible();
    const avatarContainer = threadButton.locator('div.-space-x-1\\.5');
    await expect(avatarContainer).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    const avatarElement = avatarContainer.locator('[aria-label]').first();
    await expect(avatarElement).toBeVisible();
  });

  test('room reply with thread replies shows thread indicator after reload', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Step 1: Post a root message
    const rootMessage = `Root ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootMessage);

    // Step 2: Reply to the root message in the room (sets inReplyTo)
    await rootMsg.replyInRoom();

    // The composer should show reply indicator
    await expect(page.getByText(`Replying to`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Send the room reply
    const roomReplyText = `Room reply ${Date.now()}`;
    await roomPage.sendMessage(roomReplyText);

    // Verify reply attribution is visible on the room reply
    await expect(
      page.locator('[role="article"]', { hasText: roomReplyText }).getByTestId('reply-attribution')
    ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Step 3: Open a thread on the room reply and post a thread reply
    const roomReply = roomPage.getMessage(roomReplyText);
    await roomReply.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(roomReplyText);

    const threadReplyText = `Thread reply ${Date.now()}`;
    await roomPage.postThreadReply(threadReplyText);
    await roomPage.expectTextInThreadPane(threadReplyText);

    // Step 4: Close thread and verify the indicator shows
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();
    await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Step 5: Reload the page and verify the indicator persists
    await page.reload();
    await waitForRoomReady(page);
    await expect(page.getByText(roomReplyText)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('switching threads clears previous thread messages', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post first root message
    const rootMessage1 = `First root message ${Date.now()}`;
    const message1 = await roomPage.sendMessage(rootMessage1);

    // Post second root message
    const rootMessage2 = `Second root message ${Date.now()}`;
    const message2 = await roomPage.sendMessage(rootMessage2);

    // Open thread 1 and post a reply
    await message1.openThread();
    await roomPage.expectThreadPaneVisible();

    const reply1 = `Reply to first thread ${Date.now()}`;
    await roomPage.postThreadReply(reply1);
    await roomPage.expectTextInThreadPane(reply1);

    // Close thread 1 and open thread 2
    // (thread is a slideover so main room is not interactive while open)
    await roomPage.closeThread();
    await message2.openThread();

    // Wait for thread 1's content to clear before checking thread 2's content
    // This handles the transition timing on slow CI
    await roomPage.expectTextNotInThreadPane(reply1);

    // Thread pane should now show thread 2's root message
    await roomPage.expectTextInThreadPane(rootMessage2);

    // First thread's messages should NOT be visible in second thread's pane
    await roomPage.expectTextNotInThreadPane(reply1);
    await roomPage.expectTextNotInThreadPane(rootMessage1);
  });

  test('thread replies are filtered from main room view', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Filter test root ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread and post a reply
    await message.openThread();

    // Post a reply (should only appear in thread pane, NOT in main view)
    const replyMessage = `This is a thread reply ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);

    // Close the thread pane
    await roomPage.closeThread();

    // The reply should NOT appear in the main room view
    await roomPage.expectMessageNotVisible(replyMessage);

    // But the root message should still be visible
    await roomPage.expectMessageVisible(rootMessage);
  });

  test('opening a thread shows the root message and updates URL', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Thread root test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open the thread pane
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // URL should contain thread ID
    await roomPage.expectThreadRouteActive();

    // The root message should appear in the thread pane BEFORE any replies are posted
    // This is the core issue: user reports thread pane stays empty and shows "Thread not found"
    await roomPage.expectTextInThreadPane(rootMessage);
  });

  test('re-opening a thread shows both root and replies', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Thread reopen test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread and post a reply
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(rootMessage);

    const replyMessage = `Reply in thread ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);
    await roomPage.expectTextInThreadPane(replyMessage);

    // Close the thread
    await roomPage.closeThread();

    // Re-open the same thread - both root and reply should appear
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(rootMessage);
    await roomPage.expectTextInThreadPane(replyMessage);
  });

  test('browser back button closes thread', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Back button test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open the thread pane
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectThreadRouteActive();

    // Go back
    await page.goBack();

    // Thread should be closed and URL should not contain thread
    await roomPage.expectThreadRouteClosed();
    await expect(page.getByRole('heading', { name: /^Thread in #/ })).not.toBeVisible();
  });

  test('direct thread URL navigation opens the thread pane', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message and get its event ID
    const rootMessage = `Direct URL test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread and add a reply
    await message.openThread();
    await roomPage.expectThreadRouteActive();

    // Get the thread ID from current URL
    const currentUrl = page.url();
    const threadId = currentUrl.split('/').pop();

    const replyMessage = `Reply for direct URL test ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);

    // Close the thread
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();

    // Resolve roomId from URL and spaceId from the GraphQL primary-space
    // field — post ADR-027 the URL no longer carries spaceId.
    const { roomId } = await getIdsFromUrl(page);

    // Navigate directly to thread URL
    await roomPage.gotoThread(roomId, threadId!);

    // Verify thread pane shows with content
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(rootMessage);
    await roomPage.expectTextInThreadPane(replyMessage);
  });

  test('invalid thread URL shows thread not found message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Resolve roomId from URL and spaceId from the GraphQL primary-space
    // field — post ADR-027 the URL no longer carries spaceId.
    const { roomId } = await getIdsFromUrl(page);

    // Navigate to a non-existent thread
    await page.goto(routes.thread(roomId, 'nonexistent123'));

    // Thread pane should show "Thread not found" message
    await expect(page.getByText('Thread not found')).toBeVisible();

    // User can close the thread pane to go back
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();
  });

  // Note: "editing message in main room does not affect thread pane input" test was removed
  // because the thread is now always a slideover with the main room marked inert,
  // so users cannot interact with the main room while a thread is open.

  test('editing message in thread pane does not affect main room input', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Thread edit isolation ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread and post a reply
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    const replyMessage = `Thread reply to edit ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);
    await roomPage.expectTextInThreadPane(replyMessage);

    // Click edit on a message in the THREAD PANE (reply only exists there)
    const threadMessage = roomPage.getThreadMessage(replyMessage);
    await threadMessage.startEdit();

    // There should be exactly ONE editing indicator (in thread pane only)
    await roomPage.expectExactlyOneEditIndicator();
    await roomPage.expectThreadEditModeActive();

    // Cancel the edit
    await roomPage.cancelEditWithEscape();
    await roomPage.expectEditModeInactive();
  });

  test('thread pane messages do not show reply count indicator', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Reply indicator test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread and post a reply
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(rootMessage);

    const replyMessage = `Reply in thread ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);
    await roomPage.expectTextInThreadPane(replyMessage);

    // Close the thread pane and verify the main room shows "1 reply"
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();
    await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Re-open the thread pane
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // The thread pane should NOT show any "N reply/replies" indicator
    // because you're already viewing the thread
    const threadPane = page.locator('[class*="w-full"]'); // Thread pane has responsive width
    await expect(threadPane.getByText(/\d+ repl(y|ies)/)).not.toBeVisible();
  });

  test('opening thread auto-focuses the reply input', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Auto-focus test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // The thread reply input should be focused
    await roomPage.expectThreadInputFocused();
  });

  test('on small screens, thread slideover has back button and covers room', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Responsive test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread at desktop size (toolbar requires pointer-fine / md breakpoint)
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Resize to narrow viewport — thread pane switches to slideover mode
    await page.setViewportSize({ width: 375, height: 667 });

    // The thread slideover should show back button (always present)
    await roomPage.expectThreadBackButtonVisible();

    // The thread input should be visible
    await expect(page.getByTestId('thread-reply-input')).toBeVisible();
  });

  test('back button on small screens closes thread and shows room', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Back button test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread at desktop size (toolbar requires pointer-fine / md breakpoint)
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Resize to mobile viewport — thread pane switches to slideover mode
    await page.setViewportSize({ width: 375, height: 667 });

    // Close thread using back button
    await roomPage.closeThreadWithBackButton();
    await roomPage.expectThreadRouteClosed();

    // Room view should be visible again
    await expect(page.getByTestId('message-input')).toBeVisible();
  });

  test('thread slideover shows back button and dimmed room underneath', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Slideover layout test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // The thread slideover should show the back button
    await roomPage.expectThreadBackButtonVisible();

    // The thread input should be visible
    await expect(page.getByTestId('thread-reply-input')).toBeVisible();

    // The room input is still in the DOM (dimmed underneath), but the room is inert
    // so clicking it should not be possible — we verify this via the close overlay instead
    await expect(page.getByTestId('message-input')).toBeVisible();
  });

  test('close button in thread pane header closes the thread', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message and open thread
    const rootMessage = `Close button test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // The close button should be visible
    await roomPage.expectThreadCloseButtonVisible();

    // Close thread using the close button
    await roomPage.closeThreadWithCloseButton();
    await roomPage.expectThreadRouteClosed();

    // Room view should be visible
    await expect(page.getByTestId('message-input')).toBeVisible();
  });

  test('main room draft does not prefill thread input when opening thread', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message to create a thread target
    const rootMessage = `Draft isolation test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Type some text in the main room input (don't send)
    const mainDraft = `Main room draft ${Date.now()}`;
    await roomPage.typeInMainInput(mainDraft);
    await roomPage.expectMainInputValue(mainDraft);

    // Open the thread
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // The thread input should be empty (not prefilled with main draft)
    await roomPage.expectThreadInputEmpty();

    // The main room input should still have its draft
    await roomPage.expectMainInputValue(mainDraft);
  });

  test('thread draft persists independently from main room draft', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Draft persistence test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Type a draft in the main room
    const mainDraft = `Main draft ${Date.now()}`;
    await roomPage.typeInMainInput(mainDraft);

    // Open thread and type a different draft
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    const threadDraft = `Thread draft ${Date.now()}`;
    await roomPage.typeInThreadInput(threadDraft);
    await roomPage.expectThreadInputValue(threadDraft);

    // Close the thread
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();

    // Main room input should still have its original draft
    await roomPage.expectMainInputValue(mainDraft);

    // Reopen the thread - it should still have its draft
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectThreadInputValue(threadDraft);
  });

  test('does not show unread separator when opening thread for the first time', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `First open test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread for the first time
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(rootMessage);

    // No unread separator should be shown - this is the first time opening
    // Use toPass() to wait for markThreadAsRead mutation to complete and UI to stabilize
    await expect(async () => {
      await roomPage.expectNoUnreadSeparatorInThreadPane();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
  });

  test('shows unread separator when opening thread with new messages', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post root message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = await chatPage.getSpaceId();

    const rootMessage = `Unread separator test ${Date.now()}`;
    const message1 = await roomPage.sendMessage(rootMessage);

    // User B: Create account, join space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible(rootMessage);

      // User B: Open thread (this records the "last opened" timestamp)
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.expectTextInThreadPane(rootMessage);

      // Wait for markThreadAsRead mutation to complete and UI to stabilize
      // Use toPass() to ensure the thread state is recorded before closing
      await expect(async () => {
        await roomPage2.expectNoUnreadSeparatorInThreadPane();
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

      // User B: Close thread
      await roomPage2.closeThread();
      await roomPage2.expectThreadRouteClosed();

      // User A: Post a reply to the thread
      await message1.openThread();
      await roomPage.expectThreadPaneVisible();

      const replyMessage = `New reply from User A ${Date.now()}`;
      await roomPage.postThreadReply(replyMessage);
      await roomPage.expectTextInThreadPane(replyMessage);

      // User B: Re-open the thread (no arbitrary wait needed - subsequent
      // expectUnreadSeparatorInThreadPane has built-in polling timeout)
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();

      // User B should see the "New messages" separator before User A's reply
      await roomPage2.expectUnreadSeparatorInThreadPane();
      await roomPage2.expectTextInThreadPane(replyMessage);
    } finally {
      await context2.close();
    }
  });

  test('no unread separator after posting a message and reloading', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message
    const rootMessage = `Reload test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // Open thread
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(rootMessage);

    // Wait for markThreadAsRead mutation to complete
    await expect(async () => {
      await roomPage.expectNoUnreadSeparatorInThreadPane();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

    // Post a reply in the thread
    const replyMessage = `My reply ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);
    await roomPage.expectTextInThreadPane(replyMessage);

    // Reload the page (stays on thread URL)
    await page.reload();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(replyMessage);

    // The user's own message should NOT show the unread separator
    // (they clearly saw it since they posted it)
    // Use toPass() to wait for mutation to complete after reload
    await expect(async () => {
      await roomPage.expectNoUnreadSeparatorInThreadPane();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
  });

  test('different threads have separate draft storage', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post two root messages for two threads
    const rootMessage1 = `Thread 1 root ${Date.now()}`;
    const message1 = await roomPage.sendMessage(rootMessage1);

    const rootMessage2 = `Thread 2 root ${Date.now()}`;
    const message2 = await roomPage.sendMessage(rootMessage2);

    // Open thread 1 and type a draft
    await message1.openThread();
    await roomPage.expectThreadPaneVisible();

    const thread1Draft = `Thread 1 draft ${Date.now()}`;
    await roomPage.typeInThreadInput(thread1Draft);
    await roomPage.expectThreadInputValue(thread1Draft);

    // Close thread 1, then open thread 2
    // (thread is a slideover so main room is not interactive while open)
    await roomPage.closeThread();
    await message2.openThread();
    await roomPage.expectTextInThreadPane(rootMessage2);

    // Thread 2 input should be empty (not have thread 1's draft)
    await roomPage.expectThreadInputEmpty();

    // Type a different draft in thread 2
    const thread2Draft = `Thread 2 draft ${Date.now()}`;
    await roomPage.typeInThreadInput(thread2Draft);
    await roomPage.expectThreadInputValue(thread2Draft);

    // Close thread 2, open thread 1 - it should still have its draft
    await roomPage.closeThread();
    await message1.openThread();
    await roomPage.expectTextInThreadPane(rootMessage1);
    await roomPage.expectThreadInputValue(thread1Draft);

    // Close thread 1, open thread 2 - it should still have its draft
    await roomPage.closeThread();
    await message2.openThread();
    await roomPage.expectTextInThreadPane(rootMessage2);
    await roomPage.expectThreadInputValue(thread2Draft);
  });

  test('Escape closes thread when reply input is focused', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message and open its thread
    const rootMessage = `Escape from input test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Thread reply input should be auto-focused
    await roomPage.expectThreadInputFocused();

    // Press Escape while focus is in the thread reply input
    await roomPage.closeThreadWithEscape();
    await roomPage.expectThreadRouteClosed();
  });

  test('Escape closes thread when focus is not on reply input', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message and open its thread
    const rootMessage = `Escape from pane test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Click on the thread pane heading to move focus away from the input
    await page.getByRole('heading', { name: /^Thread in #/ }).click();

    // Press Escape while focus is NOT on the reply input
    await roomPage.closeThreadWithEscape();
    await roomPage.expectThreadRouteClosed();
  });

  test('Escape closes image modal without closing thread pane', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message with an image attachment and open its thread
    const message = await roomPage.sendAttachment(
      'e2e/fixtures/brighton.jpg',
      `Image escape test ${Date.now()}`
    );
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Click the image thumbnail inside the thread pane to open the image modal
    const imageButton = roomPage.threadPane.getByRole('button', { name: /^View brighton\.jpg$/ });
    await imageButton.click();

    // The image modal dialog should be open
    const dialog = page.locator('dialog[open]');
    await expect(dialog).toBeVisible();

    // Press Escape — should close only the image modal, not the thread
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await roomPage.expectThreadPaneVisible();

    // Press Escape again — now the thread pane should close
    await roomPage.closeThreadWithEscape();
    await roomPage.expectThreadRouteClosed();
  });

  test('clicking outside thread pane closes it (sidebar area)', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message and open thread
    const rootMessage = `Click outside test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Click on the room list sidebar area (not on a link or button)
    // The sidebar container is always visible on desktop viewports
    const sidebar = page.locator('.room-list');
    await sidebar.click({ position: { x: 10, y: 10 } });

    // Thread should close
    await roomPage.expectThreadRouteClosed();
    await expect(page.getByRole('heading', { name: /^Thread in #/ })).not.toBeVisible();
  });

  test('thread reply does not scroll main chat to bottom', async ({ page, chatPage, roomPage }) => {
    // Use smaller viewport to ensure content is scrollable
    await page.setViewportSize({ width: 1280, height: 500 });

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = await chatPage.getSpaceId();

    // Extract roomId from URL
    const url = page.url();
    const match = url.match(/\/chat\/-\/([^/]+)/);
    const roomId = match![1];

    // Post enough messages to make the container scrollable
    const timestamp = Date.now();
    const messages = Array.from({ length: 20 }, (_, i) => `Scroll test ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, messages);

    // Reload so messages are loaded via initial query instead of waiting for
    // 20 subscription events to arrive and render through virtua
    await page.reload();

    // Wait for messages to appear and scroll to stabilize at bottom
    await expect(page.getByText(`Scroll test 20 - ${timestamp}`)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    const messagesContainer = page.getByTestId('messages-container').first();

    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeLessThan(50);
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [TIMEOUTS.SCROLL_SETTLE, 300, 750, 1500] });

    // Scroll the main chat to the top using native mouse wheel events.
    // Programmatic scrollTop assignment doesn't work reliably with virtua.
    const box = await messagesContainer.boundingBox();
    if (!box) throw new Error('Messages container not visible');
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    for (let i = 0; i < 15; i++) {
      await page.mouse.wheel(0, -800);
      await page.waitForTimeout(TIMEOUTS.SCROLL_SETTLE);
    }

    // Verify we're scrolled up (not at bottom)
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500] });

    // Open a thread on the first visible message and post a reply
    const rootMessage = roomPage.getMessage(`Scroll test 1 - ${timestamp}`);
    await rootMessage.openThread();
    await roomPage.expectThreadPaneVisible();

    const replyMessage = `Thread reply ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);
    await roomPage.expectTextInThreadPane(replyMessage);

    // Close the thread
    await roomPage.closeThread();
    await roomPage.expectThreadRouteClosed();

    // Main chat should still be scrolled up (NOT at the bottom)
    await expect(async () => {
      const info = await messagesContainer.evaluate((el) => ({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight
      }));
      const distanceFromBottom = info.scrollHeight - info.scrollTop - info.clientHeight;
      expect(distanceFromBottom).toBeGreaterThan(100);
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [TIMEOUTS.SCROLL_SETTLE, 300, 750, 1500] });
  });

  test('reply attribution shows avatar for the replied-to author', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post a root message and a reply via API
    const targetBody = `Target ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, roomId, targetBody);
    const replyBody = `Reply to target ${timestamp}`;
    await postReplyViaAPI(page, roomId, replyBody, targetEventId);

    // Reload to see the reply with attribution
    await page.reload();
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The reply attribution should contain an avatar image (or initials div)
    const attribution = page
      .locator('[role="article"]', { hasText: replyBody })
      .getByTestId('reply-attribution');
    await expect(attribution).toBeVisible();

    // The author section should be present with an avatar
    const authorButton = attribution.getByTestId('reply-attribution-author');
    await expect(authorButton).toBeVisible();
    // Avatar is either an <img> (custom avatar) or a <div> with initials
    await expect(authorButton.locator('img, div.rounded-full')).toBeVisible();
  });

  test('clicking reply attribution author opens user context menu', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post a root message and a reply via API
    const targetBody = `Target ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, roomId, targetBody);
    const replyBody = `Reply to target ${timestamp}`;
    await postReplyViaAPI(page, roomId, replyBody, targetEventId);

    // Reload to see the reply with attribution
    await page.reload();
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Click the author avatar/name in the attribution
    const attribution = page
      .locator('[role="article"]', { hasText: replyBody })
      .getByTestId('reply-attribution');
    const authorButton = attribution.getByTestId('reply-attribution-author');
    await authorButton.click();

    // The user context menu should appear
    await expect(page.getByRole('dialog', { name: 'User profile' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  // Posts 60+ messages via API — needs more time than the default
  test('clicking reply attribution excerpt scrolls to target message', async ({
    page,
    chatPage
  }) => {
    test.setTimeout(60_000);
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post a target message, then enough filler to push it outside the initial
    // 50-message load window, then a reply referencing the target.
    const targetBody = `Scroll target ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, roomId, targetBody);

    const fillerMessages = Array.from({ length: 60 }, (_, i) => `Filler ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, fillerMessages);

    const replyBody = `Reply pointing to target ${timestamp}`;
    await postReplyViaAPI(page, roomId, replyBody, targetEventId);

    // Reload so only the latest ~50 messages are loaded (target is outside this window)
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Target should NOT be visible (outside the loaded message window)
    await expect(page.locator('p', { hasText: targetBody })).not.toBeVisible();

    // Click the "in reply to" text specifically to avoid the nested author button,
    // which has stopPropagation and opens a user popover instead of jumping.
    const attribution = page
      .locator('[role="article"]', { hasText: replyBody })
      .getByTestId('reply-attribution');
    await attribution.getByText('in reply to').click();

    // Target message should now be visible (fetched via roomEventsAround and scrolled into view)
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });

  test('multi-user reply attribution shows correct author name and avatar', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A creates space and posts a message
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    const targetBody = `User A says hello ${timestamp}`;
    const targetEventId = await postMessageAndGetId(page, roomId, targetBody);

    // User B: join space, reply to User A's message
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('general');

      // User B posts a reply to User A's message via API
      const replyBody = `User B replies ${timestamp}`;
      await postReplyViaAPI(page2, roomId, replyBody, targetEventId);

      // User B should see the reply attribution with User A's name
      await expect(page2.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      const attribution2 = page2
        .locator('[role="article"]', { hasText: replyBody })
        .getByTestId('reply-attribution');
      await expect(attribution2).toBeVisible();

      // Attribution should show User A's display name
      const authorButton2 = attribution2.getByTestId('reply-attribution-author');
      await expect(authorButton2).toBeVisible();
      await expect(authorButton2).toContainText(userA.displayName);

      // User A should also see User B's reply with correct attribution
      await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      const attribution1 = page
        .locator('[role="article"]', { hasText: replyBody })
        .getByTestId('reply-attribution');
      await expect(attribution1).toBeVisible();
      await expect(attribution1.getByTestId('reply-attribution-author')).toContainText(
        userA.displayName
      );
    } finally {
      await context2.close();
    }
  });

  test('reply-in-room via hover bar sets attribution on sent message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const timestamp = Date.now();

    // Post a root message
    const rootBody = `Original message ${timestamp}`;
    const rootMsg = await roomPage.sendMessage(rootBody);

    // Use "Reply" from the context menu (replyInRoom)
    await rootMsg.replyInRoom();

    // Composer should show "Replying to {display name}" indicator
    await expect(page.getByText(`Replying to`)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(page.getByText(`Replying to`).locator('strong')).toContainText(user.displayName);

    // Send the reply
    const replyBody = `Hover bar reply ${timestamp}`;
    await roomPage.sendMessage(replyBody);

    // The sent message should have a reply attribution
    const attribution = page
      .locator('[role="article"]', { hasText: replyBody })
      .getByTestId('reply-attribution');
    await expect(attribution).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Attribution should show the root message author's name
    await expect(attribution.getByTestId('reply-attribution-author')).toContainText(
      user.displayName
    );

    // Attribution should show a preview of the root message body
    await expect(attribution).toContainText(rootBody.slice(0, 30));
  });
});
