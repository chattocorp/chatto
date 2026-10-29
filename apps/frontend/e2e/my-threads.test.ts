import { expect } from '@playwright/test';
import { Timestamp } from '@bufbuild/protobuf';
import { ListFollowedThreadsResponse } from '@chatto/api-types/api/v1/threads_pb';
import { createAndLoginTestUser } from './fixtures/testUser';
import { postThreadReplyFromServerUser, withServerUser } from './fixtures/serverUser';
import {
  getRoomIdByNameViaConnect,
  postMessageViaConnect,
  postThreadReplyViaConnect,
  updateScopedNotificationPolicy
} from './fixtures/connectHelpers';
import { test } from './setup';
import { MyThreadsPage } from './pages';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';

test.describe('My Threads', () => {
  test('uses Today and relative time for activity, and the root day and clock time for Age', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const root = await postMessageViaConnect(page, roomId, 'Calendar day thread');
    await postThreadReplyViaConnect(page, roomId, 'Recent reply', root);
    const rootDate = await page.evaluate(() => {
      const date = new Date();
      date.setDate(date.getDate() - 3);
      date.setHours(13, 45, 0, 0);
      return date.toISOString();
    });
    await page.route('**/chatto.api.v1.ThreadService/ListFollowedThreads', async (route) => {
      const response = await route.fetch();
      const body = ListFollowedThreadsResponse.fromBinary(await response.body());
      for (const item of body.threads) {
        if (item.thread?.threadRootEventId === root && item.rootMessage)
          item.rootMessage.createdAt = Timestamp.fromDate(new Date(rootDate));
      }
      await route.fulfill({ response, body: Buffer.from(body.toBinary()) });
    });
    const myThreads = new MyThreadsPage(page);
    await myThreads.goto();
    await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible();
    await expect(myThreads.threadItems).toContainText(/Just now|\d+[sm] ago/);
    await page.getByRole('radio', { name: 'Age', exact: true }).check();
    await expect(page.getByRole('heading', { name: 'Today', exact: true })).toHaveCount(0);
    await expect(myThreads.threadItems).toContainText(/(?:13|01):45/);
    await expect(myThreads.threadItems).not.toContainText(/Just now|ago/);
  });

  test('refreshes cached threads when switching tabs and returning to the page', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const root = await postMessageViaConnect(page, roomId, 'Refresh cached thread');
    await postThreadReplyViaConnect(page, roomId, 'First reply', root);

    // Simulate server snapshots changing without a realtime notification.
    // Returning to a cached view must still read the new snapshot.
    let rootBody = 'Snapshot one';
    await page.route('**/chatto.api.v1.ThreadService/ListFollowedThreads', async (route) => {
      const response = await route.fetch();
      const body = ListFollowedThreadsResponse.fromBinary(await response.body());
      for (const thread of body.threads ?? []) {
        if (thread.thread?.threadRootEventId === root && thread.rootMessage)
          thread.rootMessage.body = rootBody;
      }
      await route.fulfill({ response, body: Buffer.from(body.toBinary()) });
    });

    const myThreads = new MyThreadsPage(page);
    await myThreads.goto();
    await expect(myThreads.threadItems).toContainText('Snapshot one');
    await page.getByRole('radio', { name: 'Mine', exact: true }).check();
    await expect(myThreads.threadItems).toContainText('Snapshot one');

    rootBody = 'Snapshot two';
    await page.getByRole('radio', { name: 'Followed', exact: true }).check();
    await expect(myThreads.threadItems).toContainText('Snapshot two');
    rootBody = 'Snapshot three';
    await page.getByRole('radio', { name: 'Mine', exact: true }).check();
    await expect(myThreads.threadItems).toContainText('Snapshot three');

    await page.getByRole('link', { name: 'Overview', exact: true }).click();
    rootBody = 'Snapshot four';
    await myThreads.goto();
    await expect(myThreads.threadItems).toContainText('Snapshot four');
  });

  test('includes own threads in Followed and narrows Mine while retaining unread selection', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const mineText = `My conversation ${Date.now()}`;
    const theirsText = `Their conversation ${Date.now()}`;
    const mine = await postMessageViaConnect(page, roomId, mineText);
    await withServerUser(browser, serverURL, async (other) => {
      await other.chatPage.enterRoom('general');
      const theirs = await postMessageViaConnect(other.page, roomId, theirsText);
      await postThreadReplyViaConnect(page, roomId, 'Join their conversation', theirs);
      await postThreadReplyViaConnect(other.page, roomId, 'Reply to my conversation', mine);
      await postThreadReplyViaConnect(
        other.page,
        roomId,
        'Unread reply in their conversation',
        theirs
      );
    });
    const myThreads = new MyThreadsPage(page);
    await myThreads.goto();
    await expect(page.getByRole('radio', { name: 'Followed', exact: true })).toBeChecked();
    await expect(page.getByRole('radio').first()).toHaveAccessibleName('Followed');
    await expect(myThreads.threadItems).toHaveCount(2);
    await expect(myThreads.threadItems.filter({ hasText: mineText })).toBeVisible();
    await expect(myThreads.threadItems.filter({ hasText: theirsText })).toBeVisible();
    await page.getByRole('radio', { name: 'Mine', exact: true }).check();
    await expect(myThreads.threadItems).toHaveCount(1);
    await expect(myThreads.threadItems).toContainText(mineText);
    await page.getByRole('radio', { name: 'Unread', exact: true }).check();
    const mineTab = page.getByRole('radio', { name: 'Mine', exact: true });
    await mineTab.focus();
    await page.keyboard.press('ArrowLeft');
    await expect(page.getByRole('radio', { name: 'Followed', exact: true })).toBeChecked();
    await expect(page.getByRole('radio', { name: 'Unread', exact: true })).toBeChecked();
    await expect(myThreads.threadItems).toHaveCount(2);
    await expect(myThreads.threadItems.filter({ hasText: mineText })).toBeVisible();
    const theirThread = myThreads.threadItems.filter({ hasText: theirsText });
    await theirThread.hover();
    await theirThread.getByRole('button', { name: 'Unfollow thread', exact: true }).click();
    await expect(myThreads.threadItems).toHaveCount(1);
    await expect(myThreads.threadItems).toContainText(mineText);
    await mineTab.check();
    await expect(myThreads.threadItems).toHaveCount(1);
    await expect(myThreads.threadItems).toContainText(mineText);
  });

  test('shows empty state when no threads are followed', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const myThreads = new MyThreadsPage(page);

    await myThreads.goto('mine');

    // Should show empty state
    await expect(page.getByText('No threads started by you')).toBeVisible();
    await expect(page.getByText('Threads you start and follow will appear here.')).toBeVisible();
    await page.getByRole('radio', { name: 'Followed', exact: true }).check();
    await expect(page.getByText('No followed threads')).toBeVisible();
    await expect(
      page.getByText(
        'Threads you follow will appear here. You automatically follow threads you participate in.'
      )
    ).toBeVisible();
  });

  test('followed thread appears in My Threads list', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Post a message, open thread, reply (auto-follows)
    const rootText = `Thread root ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    // Thread shows its room and root context without a reply count.
    await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Today', exact: true })).toBeVisible();
    const threadItem = myThreads.threadItems;
    await expect(threadItem).toBeVisible();
    await expect(threadItem.getByText('#general')).toBeVisible();
    await expect(threadItem.getByText(rootText)).toBeVisible();
    await expect(threadItem.getByText('1 reply')).toHaveCount(0);
  });

  test('followed thread without a visible latest reply uses the root as its activity', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const rootText = `Followed root ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    const replyText = `Temporary reply ${Date.now()}`;
    await roomPage.postThreadReply(replyText);
    await roomPage.getThreadMessage(replyText).delete();
    await roomPage.closeThread();

    const myThreads = new MyThreadsPage(page);
    await myThreads.goto('mine');

    const threadItem = myThreads.threadItems;
    await expect(threadItem).toBeVisible();
    await expect(threadItem.getByText(rootText)).toBeVisible();
    // Removing the final reply preserves the original-message preview.
    await expect(threadItem.getByText(replyText)).toHaveCount(0);
    await expect(threadItem.getByText('0 replies')).toHaveCount(0);
    await expect(threadItem.getByText('Message no longer available')).not.toBeVisible();
  });

  test('clicking a thread navigates to the room with thread pane open', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create a thread
    const rootText = `Clickable thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    // Click the thread entry
    await myThreads.threadItems.click();

    // Should navigate to the room with thread pane open
    await expect(page).toHaveURL(routes.patterns.anyThread);
    await roomPage.expectThreadPaneVisible();
  });

  test('unfollowing a thread removes it from the list', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create a thread (auto-followed)
    const rootText = `Unfollow me ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    // Unfollow the thread
    await rootMsg.toggleThreadFollow();
    await rootMsg.expectNotFollowingThread();

    await myThreads.goto('mine');

    // Thread should not appear
    await expect(page.getByText('No threads started by you')).toBeVisible();
  });

  test('new reply updates unread state while keeping the root preview', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // User A creates a thread
    const rootText = `Realtime thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    const firstReply = `Reply from A ${Date.now()}`;
    await roomPage.postThreadReply(firstReply);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    await expect(myThreads.threadItems).toContainText(rootText);
    await expect(myThreads.threadItems).not.toContainText(firstReply);

    // Reload on My Threads so the source room timeline is not retained. Reply
    // activity must still refresh the query-backed conversation summary.
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Threads', exact: true })).toBeVisible();
    await page.getByRole('radio', { name: 'Mine', exact: true }).check();
    await expect(myThreads.threadItems).toContainText(rootText);
    await expect(myThreads.threadItems).toHaveAttribute('data-thread-state', 'read');

    // User B opens the thread and posts a reply
    const nextReply = `Reply from B ${Date.now()}`;
    await postThreadReplyFromServerUser(browser!, serverURL, rootText, nextReply);

    // Reply activity updates the row without replacing the original message.
    await expect(myThreads.threadItems).toHaveAttribute('data-thread-state', 'unread', {
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(myThreads.threadItems).toContainText(rootText);
    await expect(myThreads.threadItems).not.toContainText(nextReply);
  });

  test('unread indicator appears when new replies arrive', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // User A creates a thread and navigates to My Threads
    const rootText = `Unread thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    // Initially there is no mark-read action (we just replied, so it is read).
    // Use toPass() to allow subscriptions to settle before asserting absence
    const threadItem = myThreads.threadItems;
    const markReadButton = threadItem.getByRole('button', { name: 'Mark as read' });
    await expect(async () => {
      await expect(markReadButton).not.toBeVisible();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [500, 1000, 2000] });

    // User B replies to the thread
    await postThreadReplyFromServerUser(
      browser!,
      serverURL,
      rootText,
      `Reply from B ${Date.now()}`
    );

    // User A should see the action that is available for unread replies.
    await expect(markReadButton).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(threadItem).toHaveAttribute('data-thread-attention', 'important');
  });

  test('sidebar notification badge appears when another user replies', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // User A creates a thread
    const rootText = `Sidebar dot thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
    await roomPage.closeThread();

    // Stay on the room page (not My Threads) — sidebar indicator should not be visible yet
    // Use toPass() to allow subscriptions to settle before asserting absence
    await expect(async () => {
      await expect(myThreads.sidebarUnreadIndicator).not.toBeVisible();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [500, 1000, 2000] });

    // User B replies to the thread
    await postThreadReplyFromServerUser(
      browser!,
      serverURL,
      rootText,
      `Reply from B ${Date.now()}`
    );

    // Sidebar unread indicator should appear (User A is still on room page)
    await expect(myThreads.sidebarUnreadIndicator).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // Navigate to My Threads — sidebar indicator should still be visible (visiting the list
    // doesn't mark threads as read; you need to open each thread individually)
    await myThreads.goto('mine');
    await expect(myThreads.sidebarUnreadIndicator).toBeVisible();
  });

  test('sidebar notification badge clears after opening the thread', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // User A creates a thread
    const rootText = `Dot clears thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
    await roomPage.closeThread();

    // User B replies to the thread
    await postThreadReplyFromServerUser(
      browser!,
      serverURL,
      rootText,
      `Reply from B ${Date.now()}`
    );

    // Sidebar unread indicator should appear
    await expect(myThreads.sidebarUnreadIndicator).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // Open the thread that has unread replies
    const rootMsgAgain = roomPage.getMessage(rootText);
    await rootMsgAgain.openThread();
    await roomPage.expectThreadPaneVisible();

    // Sidebar unread indicator should clear after opening the thread
    await expect(async () => {
      await expect(myThreads.sidebarUnreadIndicator).not.toBeVisible();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

    await myThreads.goto('mine');
    await page.getByRole('radio', { name: 'Unread' }).click();
    await expect(page.getByText('All caught up')).toBeVisible();
    await expect(myThreads.myThreadsLink.locator('[data-testid]')).toHaveCount(0);
  });

  for (const mode of ['OFF', 'UNREAD_BADGE'] as const) {
    test(`unread replies with ${mode} delivery do not show a sidebar indicator`, async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      const pageErrors: string[] = [];
      const consoleErrors: string[] = [];
      page.on('pageerror', (error) => pageErrors.push(error.message));
      page.on('console', (message) => {
        if (message.type() === 'error') consoleErrors.push(message.text());
      });

      await createAndLoginTestUser(page);
      await chatPage.goto();
      await updateScopedNotificationPolicy(
        page,
        { server: {} },
        { followedThreads: mode, replies: 'OFF' }
      );
      await chatPage.enterRoom('general');

      const rootText = `Thread without notifications ${mode} ${Date.now()}`;
      const roomId = await getRoomIdByNameViaConnect(page, 'general');
      const rootId = await postMessageViaConnect(page, roomId, rootText);
      await postThreadReplyViaConnect(page, roomId, `Initial reply ${Date.now()}`, rootId);
      await roomPage.getMessage(rootText).expectFollowingThread();

      const replyText = `Unread reply ${Date.now()}`;
      await postThreadReplyFromServerUser(browser!, serverURL, rootText, replyText);

      const myThreads = new MyThreadsPage(page);
      await myThreads.goto('mine');
      await page.getByRole('radio', { name: 'Unread' }).click();
      const threadItem = myThreads.threadItems;
      await expect(threadItem).toHaveCount(1);
      await expect(threadItem).toHaveAttribute('data-thread-state', 'unread');
      await expect(threadItem.getByText(rootText)).toBeVisible();
      await expect(threadItem.getByText(replyText)).toHaveCount(0);
      await expect(threadItem).toHaveAttribute('data-thread-attention', 'none');
      await expect(myThreads.myThreadsLink.locator('[data-testid]')).toHaveCount(0);

      const markReadButton = threadItem.getByRole('button', { name: 'Mark as read' });
      await markReadButton.focus();
      await markReadButton.press('Enter');
      await expect(page.getByText('All caught up')).toBeVisible();
      await expect(myThreads.threadItems).toHaveCount(0);
      await expect(myThreads.myThreadsLink.locator('[data-testid]')).toHaveCount(0);
      expect(pageErrors).toEqual([]);
      expect(consoleErrors).toEqual([]);
    });
  }

  test('manually following another author shows the thread in Followed', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    const rootText = `Manual follow thread ${Date.now()}`;

    // User B creates a thread (so User A doesn't auto-follow)
    await withServerUser(
      browser!,
      serverURL,
      async ({ chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');

        // User B posts root message and a reply to create a thread
        const rootMsg2 = await roomPage2.sendMessage(rootText);
        await rootMsg2.openThread();
        await roomPage2.expectThreadPaneVisible();
        await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
        await roomPage2.closeThread();
      }
    );

    // Wait for User B's messages to appear for User A
    await expect(page.getByText(rootText)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // User A manually follows the thread (bell icon on the message)
    const rootMsg = roomPage.getMessage(rootText);
    await rootMsg.expectNotFollowingThread();
    await rootMsg.toggleThreadFollow();
    await rootMsg.expectFollowingThread();

    // Other authors appear only in Followed.
    await myThreads.goto('mine');
    await expect(myThreads.threadItems).toHaveCount(0);
    await page.getByRole('radio', { name: 'Followed', exact: true }).check();

    const threadItem = myThreads.threadItems;
    await expect(threadItem).toBeVisible();
    await expect(threadItem.getByText(rootText)).toBeVisible();
  });

  test('unread filter hides read threads and shows "all caught up" when none are unread', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create a thread (we just replied, so it's "read")
    const rootText = `Read thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    // Default filter is "All" — thread is visible
    await expect(myThreads.threadItems).toBeVisible();

    // Switch to "Unread" filter
    await page.getByRole('radio', { name: 'Unread' }).click();

    // Thread should be hidden (it's read), show "all caught up"
    await expect(myThreads.threadItems).not.toBeVisible();
    await expect(page.getByText('All caught up')).toBeVisible();

    // Switch back to "All" — thread reappears
    await page.getByRole('radio', { name: 'All' }).click();
    await expect(myThreads.threadItems).toBeVisible();
  });

  test('unread filter shows only threads with unread replies', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create two threads
    const readRoot = `Read thread ${Date.now()}`;
    const readMsg = await roomPage.sendMessage(readRoot);
    await readMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    const unreadRoot = `Unread thread ${Date.now()}`;
    const unreadMsg = await roomPage.sendNewRootAfterThread(unreadRoot);
    await unreadMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    // User B replies to only the second thread (making it unread for User A)
    await postThreadReplyFromServerUser(
      browser!,
      serverURL,
      unreadRoot,
      `Reply from B ${Date.now()}`
    );

    await myThreads.goto('mine');

    // "All" shows both threads
    await expect(myThreads.threadItems).toHaveCount(2, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // Switch to "Unread" — only the unread thread is visible
    await page.getByRole('radio', { name: 'Unread' }).click();
    await expect(myThreads.threadItems).toHaveCount(1);
    await expect(page.getByText(unreadRoot)).toBeVisible();
    await expect(page.getByText(readRoot)).not.toBeVisible();
  });

  test('filter selection persists across back navigation', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create a thread
    const rootText = `Sticky filter thread ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    // Switch to "Unread" filter
    await page.getByRole('radio', { name: 'Unread' }).click();
    await expect(page.getByRole('radio', { name: 'Unread' })).toBeChecked();

    // Click the thread to navigate to it
    // (thread is read, so switch back to All first to see it)
    await page.getByRole('radio', { name: 'All' }).click();
    await myThreads.threadItems.click();
    await expect(page).toHaveURL(routes.patterns.anyThread);

    // Go back — should return to My Threads
    await page.goBack();
    await page.waitForURL(routes.threads);

    // The "All" filter should still be selected (was last set before navigating)
    await expect(page.getByRole('radio', { name: 'All' })).toBeChecked();
  });

  test('navigating from My Threads thread to a different room does not crash', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create a second room to navigate to
    const secondRoom = `other-${Date.now()}`;
    await chatPage.createRoom(secondRoom);

    // Navigate back to general (createRoom navigates to the new room)
    await chatPage.enterRoom('general');

    // Create a thread in general
    const rootText = `Nav test ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootText);
    await rootMsg.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);
    await roomPage.closeThread();

    // Track page errors (the bug causes TypeError: Cannot read properties of undefined)
    const pageErrors: string[] = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));

    // Navigate to My Threads
    await myThreads.goto('mine');
    await expect(myThreads.threadItems).toBeVisible();

    // Click the thread — opens room with thread pane
    await myThreads.threadItems.click();
    await roomPage.expectThreadPaneVisible();

    // Click a different room in the sidebar — this is the trigger for the bug
    await chatPage.enterRoom(secondRoom);

    // Navigate back to the original room to confirm no deadlock
    await chatPage.enterRoom('general');
    // ThreadPane's exit `transition:fly` keeps it in the DOM for ~200ms after
    // the URL drops the thread suffix. Wait for it to actually unmount before
    // asserting rootText is uniquely visible — otherwise the still-fading
    // pane and the main timeline both match (strict-mode violation).
    await expect(page.getByTestId('thread-pane')).toBeHidden();
    await expect(page.getByText(rootText)).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // No TypeError should have occurred
    const criticalErrors = pageErrors.filter((e) =>
      e.includes('Cannot read properties of undefined')
    );
    expect(criticalErrors).toEqual([]);
  });

  test('multiple followed threads are sorted by last activity', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const myThreads = new MyThreadsPage(page);

    // Create first thread
    const root1 = `First thread ${Date.now()}`;
    const msg1 = await roomPage.sendMessage(root1);
    await msg1.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply to first ${Date.now()}`);
    await roomPage.closeThread();

    // Create second thread (more recent activity)
    const root2 = `Second thread ${Date.now()}`;
    const msg2 = await roomPage.sendNewRootAfterThread(root2);
    await msg2.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(`Reply to second ${Date.now()}`);
    await roomPage.closeThread();

    await myThreads.goto('mine');

    // Both threads should be visible
    const items = myThreads.threadItems;
    await expect(items).toHaveCount(2);

    // Most recent thread should be first (second thread has more recent activity)
    const firstItem = items.nth(0);
    const secondItem = items.nth(1);
    await expect(firstItem.getByText(root2)).toBeVisible();
    await expect(secondItem.getByText(root1)).toBeVisible();
  });
});
