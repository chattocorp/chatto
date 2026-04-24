import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Thread Following', () => {
  test('follow bell is visible on messages with thread replies', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post root message and open thread
    const rootMsg = await roomPage.sendMessage(`Root ${Date.now()}`);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();

    // Post a reply to create the thread
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    // The root message should now show the follow bell
    await rootMsg.expectFollowingThread();
  });

  test('clicking follow bell toggles follow state', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Create a thread
    const rootMsg = await roomPage.sendMessage(`Root ${Date.now()}`);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    // Should be following (auto-followed by replying)
    await rootMsg.expectFollowingThread();

    // Click to unfollow
    await rootMsg.toggleThreadFollow();
    await rootMsg.expectNotFollowingThread();

    // Click to re-follow
    await rootMsg.toggleThreadFollow();
    await rootMsg.expectFollowingThread();
  });

  test('auto-follow on reply shows following state in thread pane', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post root message and open thread
    const rootMsg = await roomPage.sendMessage(`Root ${Date.now()}`);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();

    // Post a reply — should auto-follow
    await roomPage.postThreadReply(`Reply ${Date.now()}`);

    // Thread pane header should show following state
    await roomPage.expectThreadPaneFollowing();
  });

  test('thread pane follow button toggles follow state', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Create a thread and stay in the pane
    const rootMsg = await roomPage.sendMessage(`Root ${Date.now()}`);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);

    // Should be following
    await roomPage.expectThreadPaneFollowing();

    // Toggle to unfollow
    await roomPage.toggleThreadFollow();
    await roomPage.expectThreadPaneNotFollowing();

    // Toggle to re-follow
    await roomPage.toggleThreadFollow();
    await roomPage.expectThreadPaneFollowing();
  });

  test('follow state persists after navigating away and back', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Create a second room to navigate to
    const otherRoom = await chatPage.createRoom('other-room');

    // Go back to general
    await chatPage.enterRoom('general');

    // Post root message, create thread, and follow it
    const rootText = `Root ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);

    // Verify following state
    await roomPage.expectThreadPaneFollowing();
    await roomPage.closeThread();
    await rootMsg.expectFollowingThread();

    // Navigate away to the other room
    await chatPage.enterRoom(otherRoom);

    // Navigate back to general
    await chatPage.enterRoom('general');

    // The bell icon on the root message should still show as following
    const rootMsgAfter = roomPage.getMessage(rootText);
    await rootMsgAfter.expectFollowingThread();
  });

  test('root author sees follow bell active after another user starts a thread', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A (Alice) posts a root message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = chatPage.getSpaceId();

    const rootText = `Root ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);

    // User B (Bob) joins the space and replies to Alice's message in a thread
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await page2.goto(routes.joinSpace(spaceId));
      await page2.getByRole('button', { name: 'Join Space' }).click();
      await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Bob opens a thread on Alice's message and posts a reply
      const rootMsg2 = roomPage2.getMessage(rootText);
      await rootMsg2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply(`Reply from Bob ${Date.now()}`);
    } finally {
      await context2.close();
    }

    // Alice should see the thread reply count appear
    await expect(page.getByText('1 reply')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // Alice should see the follow bell as ACTIVE (auto-followed as root author)
    // This is the bug: the bell shows as disabled until a page reload
    await rootMsg.expectFollowingThread();
  });

  test('replier sees follow bell active in room view after starting a thread on another user message', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A (Alice) posts a root message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = chatPage.getSpaceId();

    const rootText = `Root ${Date.now()}`;
    await roomPage.sendMessage(rootText);

    // User B (Bob) joins the space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await page2.goto(routes.joinSpace(spaceId));
      await page2.getByRole('button', { name: 'Join Space' }).click();
      await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Bob opens a thread on Alice's message and posts a reply
      const rootMsg2 = roomPage2.getMessage(rootText);
      await rootMsg2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply(`Reply from Bob ${Date.now()}`);

      // Close the thread pane to see the room view
      await roomPage2.closeThread();

      // Bob should see the follow bell as ACTIVE in the room view
      // (auto-followed as the replier who started the thread)
      await rootMsg2.expectFollowingThread();
    } finally {
      await context2.close();
    }
  });

  test('unfollow persists when another user replies', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = chatPage.getSpaceId();

    // User A posts root message and creates a thread
    const rootText = `Root ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply from A ${Date.now()}`);

    // User A unfollows
    await roomPage.toggleThreadFollow();
    await roomPage.expectThreadPaneNotFollowing();
    await roomPage.closeThread();

    // User B joins and replies to the same thread
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await page2.goto(routes.joinSpace(spaceId));
      await page2.getByRole('button', { name: 'Join Space' }).click();
      await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // User B opens the thread and replies
      const rootMsg2 = roomPage2.getMessage(rootText);
      await rootMsg2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
    } finally {
      await context2.close();
    }

    // Wait for User B's reply to arrive — reply count on root message should increase to 2
    await expect(page.getByText('2 replies')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // User A should still be unfollowed — re-open thread to verify
    await rootMsg.openThread();
    await roomPage.expectThreadPaneNotFollowing();
  });

  test('thread bell works on messages with reply attribution (inReplyTo set)', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A posts two messages: a root and a reply-attributed reply (inReplyTo set)
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = chatPage.getSpaceId();

    const firstMsg = await roomPage.sendMessage(`First ${Date.now()}`);

    // Reply to it in the channel (sets inReplyTo, but NOT inThread — still a root message)
    await firstMsg.replyInRoom();
    await expect(page.getByText('Replying to')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    const replyText = `Reply-attributed ${Date.now()}`;
    await roomPage.sendMessage(replyText);

    // User B joins the space and starts a thread on User A's reply-attributed message
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await page2.goto(routes.joinSpace(spaceId));
      await page2.getByRole('button', { name: 'Join Space' }).click();
      await page2.waitForURL(routes.patterns.spaceOrRoomWithQuery);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      const targetMsg = roomPage2.getMessage(replyText);
      await targetMsg.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply(`Thread reply from B ${Date.now()}`);
    } finally {
      await context2.close();
    }

    // User A: wait for the thread reply count to appear via subscription
    await expect(page.getByText('1 reply')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // Reload to force a fresh GraphQL query — this directly tests the
    // ViewerIsFollowingThread resolver with InReplyTo set on the root message.
    // Before the fix, the resolver returned nil because it checked InReplyTo
    // instead of InThread, causing auto-follow to appear lost after reload.
    await page.reload();
    await page.waitForLoadState('networkidle');

    let replyMsg = roomPage.getMessage(replyText);
    await replyMsg.expectFollowingThread();

    // Unfollow and re-follow manually
    await replyMsg.toggleThreadFollow();
    await replyMsg.expectNotFollowingThread();
    await replyMsg.toggleThreadFollow();
    await replyMsg.expectFollowingThread();

    // Another reload — follow state should still persist
    await page.reload();
    await page.waitForLoadState('networkidle');
    replyMsg = roomPage.getMessage(replyText);
    await replyMsg.expectFollowingThread();
  });
});
