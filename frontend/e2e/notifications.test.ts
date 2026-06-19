import { expect } from '@playwright/test';
import { test } from './setup';
import { ChatPage, NotificationsPage } from './pages';
import { createAndLoginTestUser, loginAsAdmin, loginTestUser } from './fixtures/testUser';
import {
  joinRoomFromOverview,
  postMentionFromServerUser,
  postRoomReplyFromServerUser,
  postThreadReplyFromServerUser,
  serverNotificationBadge,
  withLoggedInServerWindow,
  withServerUser
} from './fixtures/serverUser';
import * as routes from './routes';
import { POLLING_INTERVALS, TIMEOUTS } from './constants';

test.describe('Mention Notifications', () => {
  // Note: Toast notifications for mentions were removed - the bell icon with notification badge
  // and room-level mention indicators are now the primary notification feedback.

  test('shows mention indicator in room list when mentioned', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and navigate to announcements room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to "announcements" room (User A stays here to observe general)
    await chatPage.enterRoom('announcements');

    // Verify general has no mention indicator initially
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    await expect(generalLink).toBeVisible();
    // The warning-colored badge indicates a mention notification.
    const mentionBadge = generalLink.getByTestId('room-notification-badge');
    await expect(mentionBadge).not.toBeVisible();

    // User B enters general room and mentions User A
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'you have a mention!');

    // User A: Verify mention indicator appears on general room
    await expect(mentionBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(mentionBadge).toHaveText('1');

    // User A: Verify mention cascades to server icon
    const notificationBadge = serverNotificationBadge(page);
    await expect(notificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(notificationBadge).toHaveText('1');
  });

  test('notification badge appears on server icon with logo image', async ({
    page,
    chatPage,
    serverAdminPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceId = await chatPage.getServerScopeId();

    // Issue #330: upload the logo as e2eadmin (the bootstrap server owner) since
    // userA can't manage the server, then re-login as userA so they receive the
    // mention notification later in this test.
    await loginAsAdmin(page);
    // Upload a logo to the server via settings (general settings page)
    await serverAdminPage.gotoGeneralDirectly(spaceId);

    // Create a minimal valid 1x1 red PNG for testing
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
      'base64'
    );
    await serverAdminPage.uploadLogo(pngData, 'test-logo.png');
    await serverAdminPage.expectToast('Logo uploaded successfully', TIMEOUTS.COMPLEX_OPERATION);

    // Re-login as userA so the rest of the test exercises userA's view.
    await loginTestUser(page, userA);

    // Navigate back to the server
    await page.goto(routes.space());
    await chatPage.enterRoom('announcements');

    // Verify server icon now shows the logo image (not text)
    const serverGutter = page.locator('.server-gutter');
    const spaceButton = serverGutter.locator('[data-testid="server-icon"]').first();
    const spaceLogoImage = spaceButton.locator('img');
    await expect(spaceLogoImage).toBeVisible();

    // User B enters general room and mentions User A
    await postMentionFromServerUser(
      browser!,
      serverURL,
      userA.login,
      'notification on server with logo!'
    );

    // User A: Verify notification badge appears on server icon with logo.
    // The badge should be visible even when the server has an image logo.
    const notificationBadge = serverNotificationBadge(page);
    await expect(notificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(notificationBadge).toHaveText('1');

    // Also verify the logo is still visible (not replaced by anything)
    await expect(spaceLogoImage).toBeVisible();
  });

  test('mention indicator clears when entering the room', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and navigate to announcements room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('announcements');

    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const mentionBadge = generalLink.getByTestId('room-notification-badge');

    // User B: Mention User A in general
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'clearing mention test');

    // Wait for mention indicator to appear.
    await expect(mentionBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(mentionBadge).toHaveText('1');

    // User A: Navigate to general room
    await generalLink.click();
    await page.waitForURL(routes.patterns.anyRoom);

    // Verify mention indicator is cleared (poll to allow server read event to propagate)
    await expect(async () => {
      await expect(mentionBadge).not.toBeVisible();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
  });
});

test.describe('All Messages Notifications', () => {
  test('plain room messages show room and server notification badges for ALL_MESSAGES subscribers', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await page.goto(routes.settingsNotifications);
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();

    const generalNotificationRow = page.getByTestId('room-notification-general');
    await expect(generalNotificationRow).toBeVisible();
    await generalNotificationRow.locator('select').selectOption('ALL_MESSAGES');
    await expect(page.getByText('Room notification level updated')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const roomNotificationBadge = generalLink.getByTestId('room-notification-badge');
    await expect(roomNotificationBadge).not.toBeVisible();

    await withServerUser(browser!, serverURL, async ({ chatPage, roomPage }) => {
      await chatPage.enterRoom('general');
      await roomPage.sendMessage(`plain all-messages notification ${Date.now()}`);
    });

    await expect(roomNotificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(roomNotificationBadge).toHaveText('1');

    const notificationBadge = serverNotificationBadge(page);
    await expect(notificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(notificationBadge).toHaveText('1');

    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('posted a message');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  });
});

test.describe('Thread Reply Notifications (Cascading Indicators)', () => {
  test('thread reply shows indicators on thread, room, and server', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a root message
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    const rootMessage = `Thread notify test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // User A: Navigate to announcements (different room)
    await chatPage.enterRoom('announcements');

    // User B: Create account, open the server, and reply to User A's thread
    const replyMessage = `Reply from User B ${Date.now()}`;
    await postThreadReplyFromServerUser(browser!, serverURL, rootMessage, replyMessage);

    // User A: Verify cascading notification indicators appear.

    // 1. Notification badge on the "general" room in room list.
    const generalRoomLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const roomNotificationBadge = generalRoomLink.getByTestId('room-notification-badge');
    await expect(roomNotificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(roomNotificationBadge).toHaveText('1');

    // 2. Notification badge on the server icon.
    const notificationBadge = serverNotificationBadge(page);
    await expect(notificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(notificationBadge).toHaveText('1');

    // 3. Navigate to general room and verify thread has notification indicator
    await chatPage.enterRoom('general');

    // The thread indicator button shows "1 reply" and should have orange dot
    const threadButton = page.getByRole('button', { name: /1 reply/i });
    await expect(threadButton).toBeVisible();
    const threadNotificationDot = threadButton.locator('.bg-warning');
    await expect(threadNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // 4. Open the thread - notification should be dismissed
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Thread orange dot should be gone
    await expect(threadNotificationDot).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Room notification badge should also be gone (no more notifications for this room).
    await expect(roomNotificationBadge).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Server notification badge should also be gone (no more notifications on this server).
    await expect(notificationBadge).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });
});

// DM-icon notification tests have been removed alongside the cross-instance
// DM icon (#330 phase 3). DMs now appear in the primary-server sidebar
// directly; notification surfacing for them happens via the room unread/badge
// machinery shared with channels and is covered by other tests in this file.
// Cross-server consolidated DM notifications will be re-tested when that view
// is reintroduced.

test.describe('Notification Bell & Page', () => {
  test('bell icon shows indicator when there are notifications', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // Verify bell has no indicator initially
    await notificationsPage.expectBellIndicatorNotVisible();

    // User B: Mention User A to create a notification
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'bell icon test');

    // User A: Bell should now have indicator
    await notificationsPage.expectBellIndicatorVisible();
  });

  test('clicking bell navigates to notifications page', async ({
    page,
    chatPage,
    notificationsPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await notificationsPage.goto();
    await expect(notificationsPage.pageHeader).toBeVisible();
  });

  test('notifications page shows empty state when no notifications', async ({
    page,
    chatPage,
    notificationsPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await notificationsPage.goto();

    await notificationsPage.expectEmptyState();
    await notificationsPage.expectClearAllNotVisible();
  });
});

test.describe('Notification Page Display', () => {
  test('mention notification shows summary, location, and time', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    const serverName = await chatPage.getServerName();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'notification display test');

    // User A: Navigate to notifications page
    await notificationsPage.goto();

    // Verify notification appears with correct content
    const notification = notificationsPage.getNotificationBySummary('mentioned you');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Verify location is shown (room and server name)
    await notificationsPage.expectNotificationWithLocation(notification, 'general', serverName);

    // Verify Clear all button is visible
    await notificationsPage.expectClearAllVisible();
  });

  test('reply notification shows summary, location, and time', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const serverName = await chatPage.getServerName();
    await chatPage.enterRoom('general');
    const rootMessage = `Reply notification test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // Navigate to different room
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message
    await postThreadReplyFromServerUser(
      browser!,
      serverURL,
      rootMessage,
      'Reply to trigger notification'
    );

    // User A: Navigate to notifications page
    await notificationsPage.goto();

    // Verify notification appears with correct content
    const notification = notificationsPage.getNotificationBySummary('replied to your message');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Verify location is shown (room and server name)
    await notificationsPage.expectNotificationWithLocation(notification, 'general', serverName);
  });

  test('multiple notifications show in list with correct count', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, post a message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Multiple notifications test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();
    // Navigate AWAY from all rooms so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // User B: Create multiple notifications (mention in the second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    await withServerUser(browser!, serverURL, async ({ page: page2, chatPage, roomPage }) => {
      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await joinRoomFromOverview(page2, secondRoomName);

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      await chatPage.enterRoom(secondRoomName);

      // Create mention notification in the second room (User A is not in any room)
      await roomPage.sendMessage(`@${userA.login} first notification`);
      await chatPage.enterRoom('general');
      const message2 = roomPage.getMessage(rootMessage);
      await message2.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReply('Reply notification');
    });

    // User A: Verify both notifications appear
    // Use longer timeout to allow real-time events to propagate
    await notificationsPage.goto();
    await notificationsPage.expectNotificationCount(2, TIMEOUTS.COMPLEX_OPERATION);
  });
});

test.describe('Notification Dismissal', () => {
  test('dismiss single notification via X button', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'dismiss test');

    // User A: Navigate to notifications and dismiss
    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('mentioned you');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    await notificationsPage.dismissNotification(notification);

    // Verify notification is gone and empty state shows
    await expect(notification).not.toBeVisible();
    await notificationsPage.expectEmptyState();
  });

  test('dismiss all notifications via Clear all button', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, post message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Clear all test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();
    // Navigate AWAY from all rooms so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // User B: Create multiple notifications (mention in second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    await withServerUser(browser!, serverURL, async ({ page: page2, chatPage, roomPage }) => {
      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await joinRoomFromOverview(page2, secondRoomName);

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      await chatPage.enterRoom(secondRoomName);

      // Create mention in the second room (User A is not in any room)
      await roomPage.sendMessage(`@${userA.login} clear all test 1`);
      await chatPage.enterRoom('general');
      const message2 = roomPage.getMessage(rootMessage);
      await message2.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReply('Clear all test 2');
    });

    // User A: Dismiss all
    // Use longer timeout to allow real-time events to propagate
    await notificationsPage.goto();
    await notificationsPage.expectNotificationCount(2, TIMEOUTS.COMPLEX_OPERATION);
    await notificationsPage.dismissAll();

    // Verify all gone
    await notificationsPage.expectEmptyState();
    await notificationsPage.expectClearAllNotVisible();
  });

  test('bell indicator clears after dismissing all notifications', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'bell clear test');

    // User A: Verify bell has indicator
    await notificationsPage.expectBellIndicatorVisible();

    // Dismiss all
    await notificationsPage.goto();
    await notificationsPage.dismissAll();

    // Navigate back to chat and verify bell indicator is gone
    await chatPage.goto();
    await notificationsPage.expectBellIndicatorNotVisible();
  });
});

test.describe('Navigation from Notifications', () => {
  test('clicking mention notification navigates to the room', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'nav test');

    // User A: Click notification
    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('mentioned you');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await notificationsPage.clickNotification(notification);

    // Verify navigated to the room
    await page.waitForURL(routes.patterns.anyRoomWithQuery);
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
  });

  test('clicking reply notification navigates to the thread', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Thread nav test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    await chatPage.enterRoom('announcements');

    // User B: Reply to thread
    const replyText = `Reply for nav test ${Date.now()}`;
    await postThreadReplyFromServerUser(browser!, serverURL, rootMessage, replyText);

    // User A: Click notification
    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('replied to your message');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await notificationsPage.clickNotification(notification);

    // Verify navigated to the thread URL. Highlight intent is delivered via
    // PendingHighlightStore now (not ?highlight= URL param), so the URL is clean.
    await page.waitForURL(routes.patterns.anyThread);
    // Thread pane should be visible and scrolled to the new reply
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(replyText);
    await expect(roomPage.threadPane.getByText(replyText)).toBeInViewport();
  });

  test('clicking notification dismisses it', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A in general room (User B can't post in announcements due to RBAC)
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'dismiss on click test');

    // User A: Click notification
    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('mentioned you');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await notificationsPage.clickNotification(notification);
    await page.waitForURL(routes.patterns.anyRoomWithQuery);

    // Go back to notifications - should be empty
    await notificationsPage.gotoDirectly();
    await notificationsPage.expectEmptyState();
  });
});

test.describe('Cross-Tab Sync', () => {
  test('new notification appears in second tab without refresh', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    await withLoggedInServerWindow(browser!, serverURL, userA, async ({ page: page1b }) => {
      await page1b.goto(routes.space());
      await page1b.waitForURL(routes.patterns.anySpace);
      const notificationsPage1b = new NotificationsPage(page1b);

      // Both tabs should have no bell indicator
      await notificationsPage.expectBellIndicatorNotVisible();
      await notificationsPage1b.expectBellIndicatorNotVisible();

      // User B: Create account and mention User A in general (User B can't post in announcements due to RBAC)
      await postMentionFromServerUser(browser!, serverURL, userA.login, 'cross tab test');

      // Both of User A's tabs should show bell indicator
      await notificationsPage.expectBellIndicatorVisible();
      await notificationsPage1b.expectBellIndicatorVisible();

      // Navigate to notifications page in second tab - should see the notification
      await notificationsPage1b.goto();
      await notificationsPage1b.expectNotificationWithSummary('mentioned you');
    });
  });

  test('dismissed notification disappears from second tab', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    await withLoggedInServerWindow(browser!, serverURL, userA, async ({ page: page1b }) => {
      await page1b.goto(routes.space());
      const notificationsPage1b = new NotificationsPage(page1b);

      // User B: Mention User A in general (User B can't post in announcements due to RBAC)
      await postMentionFromServerUser(browser!, serverURL, userA.login, 'cross tab dismiss test');

      // Both tabs should show bell indicator
      await notificationsPage.expectBellIndicatorVisible();
      await notificationsPage1b.expectBellIndicatorVisible();

      // User A: Dismiss notification in first tab
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage.dismissNotification(notification);
      await notificationsPage.expectEmptyState();

      // Second tab: Bell indicator should also be gone
      // Navigate to a server page first to ensure the bell is visible
      await page1b.goto(routes.space());
      await notificationsPage1b.expectBellIndicatorNotVisible();

      // Second tab: Notifications page should also be empty
      await notificationsPage1b.goto();
      await notificationsPage1b.expectEmptyState();
    });
  });

  test('notification dismissed by entering room syncs to other tabs', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    await withLoggedInServerWindow(browser!, serverURL, userA, async ({ page: page1b }) => {
      const notificationsPage1b = new NotificationsPage(page1b);

      // User B: Mention User A in general (User B can't post in announcements due to RBAC)
      await postMentionFromServerUser(
        browser!,
        serverURL,
        userA.login,
        'room entry dismiss sync test'
      );

      // User A (tab 2): Go to notifications page and verify notification exists
      await notificationsPage1b.gotoDirectly();
      const notification1b = notificationsPage1b.getNotificationBySummary('mentioned you');
      await expect(notification1b).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User A (tab 1): Enter the general room (auto-dismisses mention)
      await chatPage.enterRoom('general');

      // User A (tab 2): Notification should disappear from the list
      await expect(notification1b).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage1b.expectEmptyState();
    });
  });

  test('reply notification is dismissed by opening the thread', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Thread sync test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // User B: Reply to User A's message
    await postThreadReplyFromServerUser(
      browser!,
      serverURL,
      rootMessage,
      'Reply for thread sync test'
    );

    // User A: Verify bell indicator
    await notificationsPage.expectBellIndicatorVisible();

    // User A: Open the thread (auto-dismisses reply notification)
    await chatPage.enterRoom('general');
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // User A: Bell indicator should be gone
    await notificationsPage.expectBellIndicatorNotVisible();
  });

  test('dismissing a mention notification clears the room badge on other tabs and survives reload', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // Verifies the cross-device fix: dismissing a mention notification on
    // Tab 1 not only removes the notification on Tab 2, but also clears the
    // room-level mention indicator (notification badge in the room list). The reload
    // step proves the server-side pending notification was cleared — not just the
    // local frontend state — by hitting the GraphQL room notification count resolver
    // on a fresh load.

    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // The room-level notification badge on #general. Scoped to #general's room
    // link so we don't catch the bell or other rooms.
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const generalMentionBadge = generalLink.getByTestId('room-notification-badge');

    await withLoggedInServerWindow(browser!, serverURL, userA, async ({ page: page1b }) => {
      // User A: second tab, also navigated to announcements so #general's
      // mention badge is visible in the sidebar.
      await page1b.goto(routes.space());
      const chatPage1b = new ChatPage(page1b);
      await chatPage1b.enterRoom('announcements');
      const generalLink1b = chatPage1b.roomList.locator('a', { hasText: '# general' });
      const generalMentionBadge1b = generalLink1b.getByTestId('room-notification-badge');

      // User B: mention User A in #general.
      await postMentionFromServerUser(
        browser!,
        serverURL,
        userA.login,
        'cross-device mention sync test'
      );

      // Both tabs show the room-level mention badge and the bell.
      await expect(generalMentionBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(generalMentionBadge).toHaveText('1');
      await expect(generalMentionBadge1b).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(generalMentionBadge1b).toHaveText('1');
      await notificationsPage.expectBellIndicatorVisible();

      // Tab 1: dismiss the mention via the bell panel — does NOT enter the
      // room. Pre-fix this would only sync the bell across tabs; the
      // room-level badge would linger on Tab 2 and re-appear after reload.
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage.dismissNotification(notification);
      await notificationsPage.expectEmptyState();

      // Tab 2: the badge disappears via live notification dismissal sync.
      await expect(generalMentionBadge1b).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Tab 2: hard reload. If the badge reappears, the pending notification wasn't
      // cleared server-side — local state covered for it transiently. The
      // badge staying gone proves Room.viewerNotifications.totalCount returns 0.
      await page1b.reload();
      await page1b.waitForURL(routes.patterns.anySpace);
      const chatPage1bAfter = new ChatPage(page1b);
      await chatPage1bAfter.enterRoom('announcements');
      const generalLink1bAfter = chatPage1bAfter.roomList.locator('a', { hasText: '# general' });
      // Use toPass so we tolerate the brief window during reload where the
      // sidebar is still hydrating and the badge might briefly flash.
      await expect(async () => {
        await expect(generalLink1bAfter.getByTestId('room-notification-badge')).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
    });
  });
});

test.describe('Real-time Notification Updates', () => {
  test('notification appears in real-time while on notifications page', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and go to notifications page
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to notifications page (empty)
    await notificationsPage.goto();
    await notificationsPage.expectEmptyState();

    // User B: Mention User A while A is on notifications page
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'real-time test');

    // User A: Notification should appear without refresh
    const notification = notificationsPage.getNotificationBySummary('mentioned you');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Empty state should be gone, Clear all should be visible
    await expect(notificationsPage.emptyState).not.toBeVisible();
    await notificationsPage.expectClearAllVisible();
  });

  test('notification count updates in real-time', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, post message, go to notifications
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Count test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    await notificationsPage.goto();
    await notificationsPage.expectEmptyState();

    // User B: Create multiple notifications
    await withServerUser(browser!, serverURL, async ({ chatPage, roomPage }) => {
      // First notification (mention) - User B posts in general since they can't post in announcements
      await chatPage.enterRoom('general');
      await roomPage.sendMessage(`@${userA.login} count test 1`);

      // User A: Should see 1 notification
      await notificationsPage.expectNotificationCount(1);

      // Second notification (reply) - User B is already in general
      const message2 = roomPage.getMessage(rootMessage);
      await message2.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReply('Count test 2');
    });

    // User A: Should see 2 notifications
    await notificationsPage.expectNotificationCount(2);
  });
});

test.describe('Page Title Notification Count', () => {
  test('page title shows count prefix when there are notifications', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // Verify title does not have count prefix initially
    await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);

    // User B: Mention User A in general (User B can't post in announcements due to RBAC)
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'page title test');

    // User A: Page title should now show (1) prefix
    await expect(page).toHaveTitle(/^\(1\) /, { timeout: TIMEOUTS.REALTIME_EVENT });
  });

  test('page title count updates as notifications are added', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, post message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Title count test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();

    // Navigate away so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // Verify no count prefix
    await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);

    // User B: Create multiple notifications (mention in second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    await withServerUser(browser!, serverURL, async ({ page: page2, chatPage, roomPage }) => {
      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await joinRoomFromOverview(page2, secondRoomName);

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      await chatPage.enterRoom(secondRoomName);

      // First notification (mention in the second room)
      await roomPage.sendMessage(`@${userA.login} title count 1`);

      // User A: Title should show (1)
      await expect(page).toHaveTitle(/^\(1\) /, { timeout: TIMEOUTS.REALTIME_EVENT });
      await chatPage.enterRoom('general');
      const message2 = roomPage.getMessage(rootMessage);
      await message2.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReply('Title count 2');
    });

    // User A: Title should show (2)
    await expect(page).toHaveTitle(/^\(2\) /, { timeout: TIMEOUTS.REALTIME_EVENT });
  });

  test('page title returns to normal after dismissing all notifications', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A in general (User B can't post in announcements due to RBAC)
    await postMentionFromServerUser(browser!, serverURL, userA.login, 'title dismiss test');

    // User A: Verify title has count
    await expect(page).toHaveTitle(/^\(1\) /, { timeout: TIMEOUTS.REALTIME_EVENT });

    // Dismiss all notifications
    await notificationsPage.goto();
    await notificationsPage.dismissAll();

    // Title should no longer have count prefix
    await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);
  });

  test('page title count decrements when notification is dismissed', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, post message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Title decrement test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();

    // Navigate away so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // User B: Create two notifications (mention in second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    await withServerUser(browser!, serverURL, async ({ page: page2, chatPage, roomPage }) => {
      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await joinRoomFromOverview(page2, secondRoomName);

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      await chatPage.enterRoom(secondRoomName);

      // First notification (mention in the second room)
      await roomPage.sendMessage(`@${userA.login} title decrement 1`);
      await chatPage.enterRoom('general');
      const message2 = roomPage.getMessage(rootMessage);
      await message2.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.postThreadReply('Title decrement 2');
    });

    // User A: Verify title shows (2)
    await expect(page).toHaveTitle(/^\(2\) /, { timeout: TIMEOUTS.REALTIME_EVENT });

    // Dismiss one notification
    await notificationsPage.goto();
    const mentionNotification = notificationsPage.getNotificationBySummary('mentioned you');
    await expect(mentionNotification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await notificationsPage.dismissNotification(mentionNotification);

    // Title should now show (1)
    await expect(page).toHaveTitle(/^\(1\) /);

    // Dismiss the remaining notification
    const replyNotification = notificationsPage.getNotificationBySummary('replied to your message');
    await notificationsPage.dismissNotification(replyNotification);

    // Title should have no count prefix
    await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);
  });
});

test.describe('Clickable Notification Badges', () => {
  test('clicking notification badge on room name navigates to message and dismisses', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A in general
    await postMentionFromServerUser(
      browser!,
      serverURL,
      userA.login,
      `clickable dot test ${Date.now()}`
    );

    // User A: Navigate to the server (not in general room)
    await page.goto(routes.space());
    await chatPage.enterRoom('announcements');

    // Verify notification badge appears on general room.
    const roomNotificationBadge = page
      .locator('.room-list a', { hasText: 'general' })
      .getByTestId('room-notification-badge');
    await expect(roomNotificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(roomNotificationBadge).toHaveText('1');

    // Click the notification badge (not the room link itself).
    await roomNotificationBadge.click();

    // Verify navigated to general room. Highlight intent is delivered via
    // PendingHighlightStore now (not ?highlight= URL param), so the URL is clean.
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();

    // Verify notification badge is gone (notification was dismissed).
    await expect(roomNotificationBadge).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('clicking notification badge for room reply navigates to room with highlight', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply dot test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // Navigate to different room so notification appears
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room (not thread)
    await postRoomReplyFromServerUser(browser!, serverURL, rootMessage, `Room reply ${Date.now()}`);

    // User A: Verify notification badge on general room.
    const roomNotificationBadge = page
      .locator('.room-list a', { hasText: 'general' })
      .getByTestId('room-notification-badge');
    await expect(roomNotificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(roomNotificationBadge).toHaveText('1');

    // Click the notification badge.
    await roomNotificationBadge.click();

    // Verify navigated to general room (not a thread URL). Highlight intent
    // is delivered via PendingHighlightStore now, so the URL is clean.
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();

    // Verify notification badge is gone.
    await expect(roomNotificationBadge).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  // The DM-icon click-to-navigate test was removed alongside the
  // cross-instance DM icon (#330 phase 3). DM rows now live in the primary-
  // server sidebar; their click-to-navigate behaviour is the same as channel
  // rooms and is exercised by sidebar/notification tests above.
});

test.describe('Room Reply Notifications', () => {
  test('room reply creates bell indicator and notification page entry', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const serverName = await chatPage.getServerName();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply notify test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // Navigate to different room so notification appears
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room (not thread)
    await postRoomReplyFromServerUser(
      browser!,
      serverURL,
      rootMessage,
      `Reply from User B ${Date.now()}`
    );

    // User A: Bell indicator should appear
    await notificationsPage.expectBellIndicatorVisible();

    // Verify notification badge on general room in room list.
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const roomNotificationBadge = generalLink.getByTestId('room-notification-badge');
    await expect(roomNotificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(roomNotificationBadge).toHaveText('1');

    // Verify notification badge on server icon.
    const notificationBadge = serverNotificationBadge(page);
    await expect(notificationBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(notificationBadge).toHaveText('1');

    // Verify notification page shows reply notification with correct content
    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('replied to your message');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await notificationsPage.expectNotificationWithLocation(notification, 'general', serverName);
  });

  test('clicking room reply notification navigates to room with highlight', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply nav test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room
    await postRoomReplyFromServerUser(browser!, serverURL, rootMessage, `Nav reply ${Date.now()}`);

    // User A: Click the reply notification
    await notificationsPage.goto();
    const notification = notificationsPage.getNotificationBySummary('replied to your message');
    await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await notificationsPage.clickNotification(notification);

    // Verify navigated to room (NOT a thread URL with 3 path segments).
    // Highlight intent is delivered via PendingHighlightStore now.
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
  });

  test('room reply notification dismissed on room entry', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply dismiss test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room
    await postRoomReplyFromServerUser(
      browser!,
      serverURL,
      rootMessage,
      `Dismiss reply ${Date.now()}`
    );

    // User A: Verify bell indicator appears
    await notificationsPage.expectBellIndicatorVisible();

    // User A: Enter the general room (should auto-dismiss the reply notification)
    await chatPage.enterRoom('general');

    // Bell indicator should be gone
    await notificationsPage.expectBellIndicatorNotVisible();
  });
});
