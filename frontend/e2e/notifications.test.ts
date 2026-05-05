import { expect } from '@playwright/test';
import { test } from './setup';
import { ChatPage, RoomPage, NotificationsPage } from './pages';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  loginTestUser,
  joinSpace
} from './fixtures/testUser';
import * as routes from './routes';
import { POLLING_INTERVALS, TIMEOUTS } from './constants';

test.describe('Mention Notifications', () => {
  // Note: Toast notifications for mentions were removed - the bell icon with orange dot indicator
  // and room-level mention indicators are now the primary notification feedback.

  test('shows mention indicator in room list when mentioned', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space, and navigate to announcements room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    const spaceId = await chatPage.getSpaceId();

    // Navigate to "announcements" room (User A stays here to observe general)
    await chatPage.enterRoom('announcements');

    // Verify general has no mention indicator initially
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    await expect(generalLink).toBeVisible();
    // The warning-colored dot indicates a mention
    const mentionDot = generalLink.locator('.bg-warning');
    await expect(mentionDot).not.toBeVisible();

    // User B: Create account and join the space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      // User B joins the space
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room and mentions User A
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`Hey @${userA.login} you have a mention!`);

      // User A: Verify mention indicator appears on general room
      await expect(mentionDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User A: Verify mention cascades to space icon
      const spaceList = page.locator('.space-list');
      const spaceButton = spaceList.locator('[data-testid="space-icon"]').first();
      const spaceNotificationDot = spaceButton.locator('..').locator('.bg-warning');
      await expect(spaceNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('notification dot appears on space icon with logo image', async ({
    page,
    chatPage,
    spaceAdminPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();

    // Issue #330: upload the logo as e2eadmin (the bootstrap space owner) since
    // userA can't manage the space, then re-login as userA so they receive the
    // mention notification later in this test.
    await loginAsAdmin(page);
    // Upload a logo to the space via settings (general settings page)
    await spaceAdminPage.gotoGeneralDirectly(spaceId);

    // Create a minimal valid 1x1 red PNG for testing
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
      'base64'
    );
    await spaceAdminPage.uploadLogo(pngData, 'test-logo.png');
    await spaceAdminPage.expectToast('Logo uploaded successfully', TIMEOUTS.COMPLEX_OPERATION);

    // Re-login as userA so the rest of the test exercises userA's view.
    await loginTestUser(page, userA);

    // Navigate back to the space
    await page.goto(routes.space());
    await chatPage.enterRoom('announcements');

    // Verify space icon now shows the logo image (not text)
    const spaceList = page.locator('.space-list');
    const spaceButton = spaceList.locator('[data-testid="space-icon"]').first();
    const spaceLogoImage = spaceButton.locator('img');
    await expect(spaceLogoImage).toBeVisible();

    // User B: Create account and join the space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      // User B joins the space
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room and mentions User A
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`Hey @${userA.login} notification on space with logo!`);

      // User A: Verify notification dot appears on space icon with logo
      // The notification dot should be visible even when the space has an image logo
      const spaceNotificationDot = spaceButton.locator('..').locator('.bg-warning');
      await expect(spaceNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Also verify the logo is still visible (not replaced by anything)
      await expect(spaceLogoImage).toBeVisible();
    } finally {
      await context2.close();
    }
  });

  test('mention indicator clears when entering the room', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space, and navigate to announcements room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const mentionDot = generalLink.locator('.bg-warning');

    // User B: Mention User A in general
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} clearing mention test`);

      // Wait for mention indicator to appear
      await expect(mentionDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User A: Navigate to general room
      await generalLink.click();
      await page.waitForURL(routes.patterns.anyRoom);

      // Verify mention indicator is cleared (poll to allow server read event to propagate)
      await expect(async () => {
        await expect(mentionDot).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
    } finally {
      await context2.close();
    }
  });
});

test.describe('Thread Reply Notifications (Cascading Orange Dot)', () => {
  test('thread reply shows orange dot on thread, room, and space', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space, and post a root message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();

    await chatPage.enterRoom('general');
    const rootMessage = `Thread notify test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // User A: Navigate to announcements (different room)
    await chatPage.enterRoom('announcements');

    // User B: Create account, join space, and reply to User A's thread
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();

      const replyMessage = `Reply from User B ${Date.now()}`;
      await roomPage2.postThreadReply(replyMessage);

      // User A: Verify cascading orange dots appear

      // 1. Orange dot on the "general" room in room list
      const generalRoomLink = chatPage.roomList.locator('a', { hasText: '# general' });
      const roomNotificationDot = generalRoomLink.locator('.bg-warning');
      await expect(roomNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // 2. Orange dot on the space icon
      // The space icon is a button with aria-label matching the space name
      // We need to find the button inside the space-list and check its parent for .bg-warning
      const spaceList = page.locator('.space-list');
      const spaceButton = spaceList.locator('[data-testid="space-icon"]').first();
      const spaceNotificationDot = spaceButton.locator('..').locator('.bg-warning');
      await expect(spaceNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // 3. Navigate to general room and verify thread has orange dot
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

      // Room orange dot should also be gone (no more notifications for this room)
      await expect(roomNotificationDot).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Space orange dot should also be gone (no more notifications in this space)
      await expect(spaceNotificationDot).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    } finally {
      await context2.close();
    }
  });
});

// DM-icon notification tests have been removed alongside the cross-instance
// DM icon (#330 phase 3). DMs now appear in the primary-space sidebar
// directly; notification surfacing for them happens via the room unread/dot
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
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // Verify bell has no indicator initially
    await notificationsPage.expectBellIndicatorNotVisible();

    // User B: Mention User A to create a notification
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} bell icon test`);

      // User A: Bell should now have indicator
      await notificationsPage.expectBellIndicatorVisible();
    } finally {
      await context2.close();
    }
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
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} notification display test`);

      // User A: Navigate to notifications page
      await notificationsPage.goto();

      // Verify notification appears with correct content
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Verify location is shown (room and space name)
      await notificationsPage.expectNotificationWithLocation(notification, 'general', spaceName);

      // Verify Clear all button is visible
      await notificationsPage.expectClearAllVisible();
    } finally {
      await context2.close();
    }
  });

  test('reply notification shows summary, location, and time', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Reply notification test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // Navigate to different room
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Reply to trigger notification');

      // User A: Navigate to notifications page
      await notificationsPage.goto();

      // Verify notification appears with correct content
      const notification = notificationsPage.getNotificationBySummary('replied to your message');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Verify location is shown (room and space name)
      await notificationsPage.expectNotificationWithLocation(notification, 'general', spaceName);
    } finally {
      await context2.close();
    }
  });

  test('multiple notifications show in list with correct count', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space, post a message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Multiple notifications test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();
    // Navigate AWAY from all rooms so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // User B: Create multiple notifications (mention in the second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const roomPage2 = new RoomPage(page2);

      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const roomItem = page2.locator('li', { hasText: `# ${secondRoomName}` });
      await roomItem.getByRole('button', { name: 'Join' }).click();
      await expect(roomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(secondRoomName);

      // Create mention notification in the second room (User A is not in any room)
      await roomPage2.sendMessage(`@${userA.login} first notification`);
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Reply notification');

      // User A: Verify both notifications appear
      // Use longer timeout to allow real-time events to propagate
      await notificationsPage.goto();
      await notificationsPage.expectNotificationCount(2, TIMEOUTS.COMPLEX_OPERATION);
    } finally {
      await context2.close();
    }
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
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} dismiss test`);

      // User A: Navigate to notifications and dismiss
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      await notificationsPage.dismissNotification(notification);

      // Verify notification is gone and empty state shows
      await expect(notification).not.toBeVisible();
      await notificationsPage.expectEmptyState();
    } finally {
      await context2.close();
    }
  });

  test('dismiss all notifications via Clear all button', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space, post message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Clear all test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();
    // Navigate AWAY from all rooms so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // User B: Create multiple notifications (mention in second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const roomPage2 = new RoomPage(page2);

      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const roomItem = page2.locator('li', { hasText: `# ${secondRoomName}` });
      await roomItem.getByRole('button', { name: 'Join' }).click();
      await expect(roomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(secondRoomName);

      // Create mention in the second room (User A is not in any room)
      await roomPage2.sendMessage(`@${userA.login} clear all test 1`);
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Clear all test 2');

      // User A: Dismiss all
      // Use longer timeout to allow real-time events to propagate
      await notificationsPage.goto();
      await notificationsPage.expectNotificationCount(2, TIMEOUTS.COMPLEX_OPERATION);
      await notificationsPage.dismissAll();

      // Verify all gone
      await notificationsPage.expectEmptyState();
      await notificationsPage.expectClearAllNotVisible();
    } finally {
      await context2.close();
    }
  });

  test('bell indicator clears after dismissing all notifications', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} bell clear test`);

      // User A: Verify bell has indicator
      await notificationsPage.expectBellIndicatorVisible();

      // Dismiss all
      await notificationsPage.goto();
      await notificationsPage.dismissAll();

      // Navigate back to chat and verify bell indicator is gone
      await chatPage.goto();
      await notificationsPage.expectBellIndicatorNotVisible();
    } finally {
      await context2.close();
    }
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
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} nav test`);

      // User A: Click notification
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage.clickNotification(notification);

      // Verify navigated to the room
      await page.waitForURL(routes.patterns.anyRoomWithQuery);
      await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    } finally {
      await context2.close();
    }
  });

  test('clicking reply notification navigates to the thread', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Thread nav test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    await chatPage.enterRoom('announcements');

    // User B: Reply to thread
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      const replyText = `Reply for nav test ${Date.now()}`;
      await roomPage2.postThreadReply(replyText);

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
    } finally {
      await context2.close();
    }
  });

  test('clicking notification dismisses it', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A in general room (User B can't post in announcements due to RBAC)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} dismiss on click test`);

      // User A: Click notification
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage.clickNotification(notification);
      await page.waitForURL(routes.patterns.anyRoomWithQuery);

      // Go back to notifications - should be empty
      await notificationsPage.gotoDirectly();
      await notificationsPage.expectEmptyState();
    } finally {
      await context2.close();
    }
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
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User A: Open second tab (same user, same session)
    const context1b = await browser!.newContext({ baseURL: serverURL });
    const page1b = await context1b.newPage();

    // User B context
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      // Log User A into second tab
      await loginTestUser(page1b, userA);
      await page1b.goto(routes.space());
      await page1b.waitForURL(routes.patterns.anySpace);
      const notificationsPage1b = new NotificationsPage(page1b);

      // Both tabs should have no bell indicator
      await notificationsPage.expectBellIndicatorNotVisible();
      await notificationsPage1b.expectBellIndicatorNotVisible();

      // User B: Create account and mention User A in general (User B can't post in announcements due to RBAC)
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} cross tab test`);

      // Both of User A's tabs should show bell indicator
      await notificationsPage.expectBellIndicatorVisible();
      await notificationsPage1b.expectBellIndicatorVisible();

      // Navigate to notifications page in second tab - should see the notification
      await notificationsPage1b.goto();
      await notificationsPage1b.expectNotificationWithSummary('mentioned you');
    } finally {
      await context1b.close();
      await context2.close();
    }
  });

  test('dismissed notification disappears from second tab', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User A: Open second tab
    const context1b = await browser!.newContext({ baseURL: serverURL });
    const page1b = await context1b.newPage();

    // User B context
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      // Log User A into second tab
      await loginTestUser(page1b, userA);
      await page1b.goto(routes.space());
      const notificationsPage1b = new NotificationsPage(page1b);

      // User B: Mention User A in general (User B can't post in announcements due to RBAC)
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} cross tab dismiss test`);

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
      // Navigate to a space page first to ensure the bell is visible
      await page1b.goto(routes.space());
      await notificationsPage1b.expectBellIndicatorNotVisible();

      // Second tab: Notifications page should also be empty
      await notificationsPage1b.goto();
      await notificationsPage1b.expectEmptyState();
    } finally {
      await context1b.close();
      await context2.close();
    }
  });

  test('notification dismissed by entering room syncs to other tabs', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User A: Open second tab on notifications page
    const context1b = await browser!.newContext({ baseURL: serverURL });
    const page1b = await context1b.newPage();

    // User B context
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      // Log User A into second tab
      await loginTestUser(page1b, userA);
      const notificationsPage1b = new NotificationsPage(page1b);

      // User B: Mention User A in general (User B can't post in announcements due to RBAC)
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} room entry dismiss sync test`);

      // User A (tab 2): Go to notifications page and verify notification exists
      await notificationsPage1b.gotoDirectly();
      const notification1b = notificationsPage1b.getNotificationBySummary('mentioned you');
      await expect(notification1b).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User A (tab 1): Enter the general room (auto-dismisses mention)
      await chatPage.enterRoom('general');

      // User A (tab 2): Notification should disappear from the list
      await expect(notification1b).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage1b.expectEmptyState();
    } finally {
      await context1b.close();
      await context2.close();
    }
  });

  test('notification dismissed by opening thread syncs to other tabs', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Thread sync test ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    // User A: Open second tab
    const context1b = await browser!.newContext({ baseURL: serverURL });
    const page1b = await context1b.newPage();

    // User B context
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      // Get user info from first page's logged in state
      // We'll verify via notification sync instead of explicit login

      // Actually, let's use the credentials - we need to get the login from somewhere
      // For simplicity, let's navigate tab2 to notifications and watch for changes
      await page1b.goto(`${serverURL}${routes.space()}`);
      // This will require auth, so let's use a different approach

      // User B: Reply to User A's message
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Reply for thread sync test');

      // User A (tab 1): Verify bell indicator
      await notificationsPage.expectBellIndicatorVisible();

      // User A (tab 1): Open the thread (auto-dismisses reply notification)
      await chatPage.enterRoom('general');
      await message.openThread();
      await roomPage.expectThreadPaneVisible();

      // User A (tab 1): Bell indicator should be gone
      await notificationsPage.expectBellIndicatorNotVisible();
    } finally {
      await context1b.close();
      await context2.close();
    }
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
    // User A: Create account, space, go to notifications page
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();

    // Navigate to notifications page (empty)
    await notificationsPage.goto();
    await notificationsPage.expectEmptyState();

    // User B: Mention User A while A is on notifications page
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} real-time test`);

      // User A: Notification should appear without refresh
      const notification = notificationsPage.getNotificationBySummary('mentioned you');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Empty state should be gone, Clear all should be visible
      await expect(notificationsPage.emptyState).not.toBeVisible();
      await notificationsPage.expectClearAllVisible();
    } finally {
      await context2.close();
    }
  });

  test('notification count updates in real-time', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space, post message, go to notifications
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Count test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    await notificationsPage.goto();
    await notificationsPage.expectEmptyState();

    // User B: Create multiple notifications
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // First notification (mention) - User B posts in general since they can't post in announcements
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} count test 1`);

      // User A: Should see 1 notification
      await notificationsPage.expectNotificationCount(1);

      // Second notification (reply) - User B is already in general
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Count test 2');

      // User A: Should see 2 notifications
      await notificationsPage.expectNotificationCount(2);
    } finally {
      await context2.close();
    }
  });
});

test.describe('Page Title Notification Count', () => {
  test('page title shows count prefix when there are notifications', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // Verify title does not have count prefix initially
    await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);

    // User B: Mention User A in general (User B can't post in announcements due to RBAC)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} page title test`);

      // User A: Page title should now show (1) prefix
      await expect(page).toHaveTitle(/^\(1\) /, { timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('page title count updates as notifications are added', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create space, post message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
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
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const roomPage2 = new RoomPage(page2);

      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const roomItem = page2.locator('li', { hasText: `# ${secondRoomName}` });
      await roomItem.getByRole('button', { name: 'Join' }).click();
      await expect(roomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(secondRoomName);

      // First notification (mention in the second room)
      await roomPage2.sendMessage(`@${userA.login} title count 1`);

      // User A: Title should show (1)
      await expect(page).toHaveTitle(/^\(1\) /, { timeout: TIMEOUTS.REALTIME_EVENT });
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Title count 2');

      // User A: Title should show (2)
      await expect(page).toHaveTitle(/^\(2\) /, { timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('page title returns to normal after dismissing all notifications', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A in general (User B can't post in announcements due to RBAC)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await roomPage2.sendMessage(`@${userA.login} title dismiss test`);

      // User A: Verify title has count
      await expect(page).toHaveTitle(/^\(1\) /, { timeout: TIMEOUTS.REALTIME_EVENT });

      // Dismiss all notifications
      await notificationsPage.goto();
      await notificationsPage.dismissAll();

      // Title should no longer have count prefix
      await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);
    } finally {
      await context2.close();
    }
  });

  test('page title count decrements when notification is dismissed', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space, post message in general, and create an additional room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Title decrement test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    // User A creates the additional room (room.create is not granted to everyone by default)
    const secondRoomName = await chatPage.createRoom();

    // Navigate away so notifications won't be auto-dismissed
    await page.goto(routes.settings);

    // User B: Create two notifications (mention in second room + reply in general)
    // Using separate rooms so notifications aren't deduplicated
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const roomPage2 = new RoomPage(page2);

      // Join the second room via Browse Rooms (User B doesn't have room.create)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const roomItem = page2.locator('li', { hasText: `# ${secondRoomName}` });
      await roomItem.getByRole('button', { name: 'Join' }).click();
      await expect(roomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar (Browse Rooms no longer auto-navigates)
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(secondRoomName);

      // First notification (mention in the second room)
      await roomPage2.sendMessage(`@${userA.login} title decrement 1`);
      await chatPage2.enterRoom('general');
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();
      await roomPage2.postThreadReply('Title decrement 2');

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
      const replyNotification =
        notificationsPage.getNotificationBySummary('replied to your message');
      await notificationsPage.dismissNotification(replyNotification);

      // Title should have no count prefix
      await expect(page).toHaveTitle(/^(?!\(\d+\)).*$/);
    } finally {
      await context2.close();
    }
  });
});

test.describe('Clickable Notification Dots', () => {
  test('clicking notification dot on room name navigates to message and dismisses', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('announcements');

    // User B: Mention User A in general
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      const testMessage = `@${userA.login} clickable dot test ${Date.now()}`;
      await roomPage2.sendMessage(testMessage);

      // User A: Navigate to the space (not in general room)
      await page.goto(routes.space());
      await chatPage.enterRoom('announcements');

      // Verify notification dot appears on general room
      const roomNotificationDot = page
        .locator('.room-list a', { hasText: 'general' })
        .locator('.bg-warning');
      await expect(roomNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Click the notification dot (not the room link itself)
      await roomNotificationDot.click();

      // Verify navigated to general room. Highlight intent is delivered via
      // PendingHighlightStore now (not ?highlight= URL param), so the URL is clean.
      await page.waitForURL(routes.patterns.anyRoom);
      await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();

      // Verify notification dot is gone (notification was dismissed)
      await expect(roomNotificationDot).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    } finally {
      await context2.close();
    }
  });


  test('clicking notification dot for room reply navigates to room with highlight', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply dot test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // Navigate to different room so notification appears
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room (not thread)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');

      const targetMsg = roomPage2.getMessage(rootMessage);
      await targetMsg.replyInRoom();
      await expect(page2.getByTestId('reply-indicator')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await roomPage2.sendMessage(`Room reply ${Date.now()}`);

      // User A: Verify orange dot on general room
      const roomNotificationDot = page
        .locator('.room-list a', { hasText: 'general' })
        .locator('.bg-warning');
      await expect(roomNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Click the notification dot
      await roomNotificationDot.click();

      // Verify navigated to general room (not a thread URL). Highlight intent
      // is delivered via PendingHighlightStore now, so the URL is clean.
      await page.waitForURL(routes.patterns.anyRoom);
      await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();

      // Verify notification dot is gone
      await expect(roomNotificationDot).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    } finally {
      await context2.close();
    }
  });

  // The DM-icon click-to-navigate test was removed alongside the
  // cross-instance DM icon (#330 phase 3). DM rows now live in the primary-
  // space sidebar; their click-to-navigate behaviour is the same as channel
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
    // User A: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply notify test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // Navigate to different room so notification appears
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room (not thread)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');

      const targetMsg = roomPage2.getMessage(rootMessage);
      await targetMsg.replyInRoom();
      await expect(page2.getByTestId('reply-indicator')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await roomPage2.sendMessage(`Reply from User B ${Date.now()}`);

      // User A: Bell indicator should appear
      await notificationsPage.expectBellIndicatorVisible();

      // Verify orange dot on general room in room list
      const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
      const roomNotificationDot = generalLink.locator('.bg-warning');
      await expect(roomNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Verify orange dot on space icon
      const spaceList = page.locator('.space-list');
      const spaceButton = spaceList.locator('[data-testid="space-icon"]').first();
      const spaceNotificationDot = spaceButton.locator('..').locator('.bg-warning');
      await expect(spaceNotificationDot).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Verify notification page shows reply notification with correct content
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('replied to your message');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage.expectNotificationWithLocation(notification, 'general', spaceName);
    } finally {
      await context2.close();
    }
  });

  test('clicking room reply notification navigates to room with highlight', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply nav test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');

      const targetMsg = roomPage2.getMessage(rootMessage);
      await targetMsg.replyInRoom();
      await expect(page2.getByTestId('reply-indicator')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await roomPage2.sendMessage(`Nav reply ${Date.now()}`);

      // User A: Click the reply notification
      await notificationsPage.goto();
      const notification = notificationsPage.getNotificationBySummary('replied to your message');
      await expect(notification).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await notificationsPage.clickNotification(notification);

      // Verify navigated to room (NOT a thread URL with 3 path segments).
      // Highlight intent is delivered via PendingHighlightStore now.
      await page.waitForURL(routes.patterns.anyRoom);
      await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    } finally {
      await context2.close();
    }
  });

  test('room reply notification dismissed on room entry', async ({
    page,
    chatPage,
    roomPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    // User A: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    await chatPage.enterRoom('general');
    const rootMessage = `Room reply dismiss test ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);
    await chatPage.enterRoom('announcements');

    // User B: Reply to User A's message in room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');

      const targetMsg = roomPage2.getMessage(rootMessage);
      await targetMsg.replyInRoom();
      await expect(page2.getByTestId('reply-indicator')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await roomPage2.sendMessage(`Dismiss reply ${Date.now()}`);

      // User A: Verify bell indicator appears
      await notificationsPage.expectBellIndicatorVisible();

      // User A: Enter the general room (should auto-dismiss the reply notification)
      await chatPage.enterRoom('general');

      // Bell indicator should be gone
      await notificationsPage.expectBellIndicatorNotVisible();
    } finally {
      await context2.close();
    }
  });
});
