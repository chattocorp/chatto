import { expect } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import {
  waitForSpaceUnread,
  waitForRoomUnread,
  waitForRoomRead,
  getRoomIdByName
} from './fixtures/graphqlHelpers';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';

test.describe('Multi-Tab Unread Sync', () => {
  test('entering room clears unread in other tabs via RoomMarkedAsReadEvent', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create space (auto-enters a room due to redirect behavior)
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();

    // Navigate User A to announcements room (not general) so general stays unread
    await chatPage.enterRoom('announcements');

    // Get room ID for general (the room that will have unread messages)
    const roomId = await getRoomIdByName(page, spaceId, 'general');

    // User B: Join space and send a message that creates unread state for User A
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await page2.goto(routes.joinSpace(spaceId));
      await page2.getByRole('button', { name: 'Join Space' }).click();
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // User B sends message - creates unread state for User A
      const testMessage = `Test message ${Date.now()}`;
      await roomPage2.sendMessage(testMessage);

      // Wait for server to register unread state for User A
      await waitForRoomUnread(page, spaceId, roomId, true);

      // User A: Open second tab (same account) - this tab will verify sync
      const context3 = await browser!.newContext({ baseURL: serverURL });
      const page3 = await context3.newPage();

      try {
        // Login as same user in Tab 2
        await page3.request.post('/auth/login', {
          data: { login: userA.login, password: userA.password }
        });

        // Tab 2 navigates to space and enters announcements room (not general)
        // This way Tab 2 can see general's unread indicator in the room list
        await page3.goto(routes.space());
        await page3.waitForURL(routes.patterns.anySpace);
        const chatPage3 = new ChatPage(page3);
        await chatPage3.enterRoom('announcements');

        // Wait for Tab 2 to show room-level unread indicator for general
        await expect(async () => {
          const roomUnreadDot = page3.locator('[data-testid="room-unread-dot"]');
          await expect(roomUnreadDot).toBeVisible();
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

        // Wait for WebSocket subscription to be established
        // networkidle waits until no network requests for 500ms, ensuring the
        // GraphQL subscription connection is established before we trigger events
        await page3.waitForLoadState('networkidle');

        // Tab 1: User A enters general room (this auto-marks room as read and emits RoomMarkedAsReadEvent)
        await chatPage.enterRoom('general');
        await waitForRoomReady(page, 'general');

        // Tab 2: Should receive RoomMarkedAsReadEvent and clear room-level unread indicator
        await expect(async () => {
          const roomUnreadDot = page3.locator('[data-testid="room-unread-dot"]');
          await expect(roomUnreadDot).not.toBeVisible();
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
      } finally {
        await context3.close();
      }
    } finally {
      await context2.close();
    }
  });
});

// FIXME: tests cross-space message visibility — explicitly multi-space.
// Doesn't apply post-collapse. Re-enable / remove in next phase-2 PR.
test.describe.skip('Cross-space message visibility', () => {
  test('message appears when returning to space after leaving', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create first space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Space One');
    const space1Id = await chatPage.getSpaceId();

    // Visit #announcements (this will be the "last room" for this space)
    // Both "general" and "announcements" are created automatically when a space is created
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');
    await roomPage.sendMessage('Initial message in announcements');

    // Create second space and go there (leaving space 1)
    await chatPage.createSpace('Space Two');
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Message in Space Two');

    // User B: Join space 1 and post in #general
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, space1Id);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters #general and posts a message
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      const testMessage = `Cross-space message ${Date.now()}`;
      await roomPage2.sendMessage(testMessage);

      // Wait for server to register the unread state for User A
      await waitForSpaceUnread(page, space1Id, true);

      // Verify User A sees unread indicator on Space One's icon
      await expect(async () => {
        await expect(page.locator('[data-testid="space-unread-dot"]')).toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // User A: Return to space 1 by clicking the space icon (not URL navigation)
      // This more closely matches the user's manual testing scenario
      await page.locator('.space-list').getByRole('link', { name: 'Space One' }).click();
      await page.waitForURL(routes.patterns.anySpace);

      // User should land in #announcements (last visited room in this space)
      // Wait for that room to load
      await waitForRoomReady(page, 'announcements');

      // Should see unread indicator on #general
      const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
      await expect(async () => {
        await expect(generalLink).toHaveClass(/font-semibold/);
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // Navigate to #general
      await chatPage.enterRoom('general');
      await waitForRoomReady(page, 'general');

      // Message should be visible after navigating to the room
      await expect(page.getByText(testMessage)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    } finally {
      await context2.close();
    }
  });
});

// FIXME: space icon unread dots across multiple spaces — multi-space.
test.describe.skip('Space-level unread indicators', () => {
  test('shows unread dot on space icon when another space has unread messages', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account, first space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Space One');
    const space1Id = await chatPage.getSpaceId();

    // Navigate to general room and send a message to mark it as read
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Hello from Space One');

    // User A: Create second space
    await chatPage.createSpace('Space Two');

    // Navigate to general room in Space Two
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Hello from Space Two');

    // User B: Create account and join Space One
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, space1Id);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room in Space One
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // User B sends a message - this should make Space One "unread" for User A
      const testMessage = `Unread message at ${Date.now()}`;
      await roomPage2.sendMessage(testMessage);

      // Wait for server to register the unread state for User A
      await waitForSpaceUnread(page, space1Id, true);

      // User A should see unread indicator on Space One's icon
      await expect(async () => {
        await expect(page.locator('[data-testid="space-unread-dot"]')).toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
    } finally {
      await context2.close();
    }
  });
});

test.describe('Space-level unread clearing', () => {
  // FIXME: multi-user multi-space flow. Re-enable / rewrite in next
  // phase-2 PR using bootstrap space.
  test.skip('space unread dot clears when all rooms are read', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Complex multi-user test with multiple navigation steps

    // User A: Create account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Main Space');
    const mainSpaceId = await chatPage.getSpaceId();

    // Navigate to general room and mark it as read
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial message');

    // Create a second space to navigate to (so we can see unread on first space)
    await chatPage.createSpace('Other Space');
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Other space message');

    // User B: Create account, join Main Space, post message in general
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, mainSpaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room and posts (announcements is read-only for non-creators)
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.sendMessage(`Unread message ${Date.now()}`);

      // Wait for server to register the unread state
      await waitForSpaceUnread(page, mainSpaceId, true);

      // User A: Should see unread dot on Main Space
      await expect(async () => {
        await expect(page.locator('[data-testid="space-unread-dot"]')).toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // Get the general room ID for polling
      const generalRoomId = await getRoomIdByName(page, mainSpaceId, 'general');

      // User A: Navigate to Main Space and read the general room
      await page.goto(routes.space());
      await page.waitForURL(routes.patterns.anySpace);
      await chatPage.enterRoom('general');
      await waitForRoomReady(page, 'general');

      // Wait for server to confirm the room is marked as read
      await waitForRoomRead(page, mainSpaceId, generalRoomId);

      // Navigate back to Other Space by clicking its icon in the sidebar
      await page.locator('.space-list').getByRole('link', { name: 'Other Space' }).click();
      await page.waitForURL(routes.patterns.anySpace);

      // Wait for server to confirm space has no unread rooms
      await waitForSpaceUnread(page, mainSpaceId, false);

      // The unread dot should now be gone
      await expect(async () => {
        await expect(page.locator('[data-testid="space-unread-dot"]')).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
    } finally {
      await context2.close();
    }
  });
});

test.describe('Multi-window unread sync', () => {
  test('unread indicator appears in second window when message posted by another user', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account and space
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Shared Space');
    const spaceId = await chatPage.getSpaceId();

    // User A visits general room then leaves to announcements
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('First window ready');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // Wait for server to confirm room is read
    await waitForRoomRead(page, spaceId, generalRoomId);

    // User A navigates away from general to announcements
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // User A opens second window (same account) - also in announcements
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      // Login as same user in second window
      const loginResponse = await page2.request.post('/auth/login', {
        data: { login: userA.login, password: userA.password }
      });
      expect(loginResponse.ok()).toBeTruthy();

      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);

      // Navigate to announcements in second window (not general)
      await chatPage2.enterRoom('announcements');
      await waitForRoomReady(page2, 'announcements');

      // User B: Create account, join space, post in general
      const context3 = await browser!.newContext({ baseURL: serverURL });
      const page3 = await context3.newPage();

      try {
        await createAndLoginTestUser(page3);
        await joinSpace(page3, spaceId);
        await page3.goto(routes.space());
        await page3.waitForURL(routes.patterns.anySpace);

        const chatPage3 = new ChatPage(page3);
        const roomPage3 = new RoomPage(page3);

        // User B posts in general room
        await chatPage3.enterRoom('general');
        await waitForRoomReady(page3, 'general');
        await roomPage3.sendMessage(`Message from User B at ${Date.now()}`);

        // Wait for server to register the unread state for User A
        await waitForRoomUnread(page2, spaceId, generalRoomId, true);

        // Both windows should see unread indicator on general room
        const generalLink1 = page.locator('nav').locator('a', { hasText: '# general' });
        const generalLink2 = page2.locator('nav').locator('a', { hasText: '# general' });

        await expect(async () => {
          await expect(generalLink1).toHaveClass(/font-semibold/);
          await expect(generalLink2).toHaveClass(/font-semibold/);
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
      } finally {
        await context3.close();
      }
    } finally {
      await context2.close();
    }
  });
});

test.describe('Unread indicators', () => {
  test('shows unread indicator when another user posts a message to a different room', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account, space, and navigate to announcements room
    // (User A stays in announcements while User B posts in general)
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    const spaceId = await chatPage.getSpaceId();

    // Navigate to "announcements" room (User A will observe from here)
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // Verify general has no unread indicator
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    await expect(generalLink).toBeVisible();
    await expect(generalLink).not.toHaveClass(/font-semibold/);

    // User B: Create account and join the space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      // User B joins the space via API helper
      await joinSpace(page2, spaceId);

      // Navigate to the space
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // User B sends a message
      const testMessage = `Hello from User B at ${Date.now()}`;
      await roomPage2.sendMessage(testMessage);

      // Wait for server to register the unread state
      await waitForRoomUnread(page, spaceId, generalRoomId, true);

      // User A: Verify unread indicator appears on "general"
      await expect(async () => {
        await expect(generalLink).toHaveClass(/font-semibold/);
        const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
        await expect(unreadDot).toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // User A: Navigate to general room
      await generalLink.click();
      await page.waitForURL(routes.patterns.anyRoom);

      // Verify the message is visible
      await expect(page.getByText(testMessage)).toBeVisible();

      // Wait for server to confirm room is read
      await waitForRoomRead(page, spaceId, generalRoomId);

      // Verify the unread indicator is now gone
      await expect(async () => {
        await expect(generalLink).not.toHaveClass(/font-semibold/);
        const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
        await expect(unreadDot).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
    } finally {
      await context2.close();
    }
  });

  test('unread indicator clears when navigating to room', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    // Navigate to general room first
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Send a message to mark general as "active"
    await roomPage.sendMessage('Hello from general');

    // Navigate to announcements
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // Both rooms should have no unread indicator since we've viewed them
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const announcementsLink = chatPage.roomList.locator('a', { hasText: '# announcements' });

    await expect(generalLink).not.toHaveClass(/font-semibold/);
    await expect(announcementsLink).not.toHaveClass(/font-semibold/);
  });
});

test.describe('Room unread separator', () => {
  test('shows unread separator when entering room with new messages', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Separator Test Space');
    const spaceId = await chatPage.getSpaceId();

    // User A enters general room and posts initial messages
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Message 1 from User A');
    await roomPage.sendMessage('Message 2 from User A');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // User B: Create account, join space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room (this records their last-read position)
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible('Message 2 from User A');

      // Wait for room to be fully loaded
      await expect(roomPage2.messageInput).toBeEnabled();

      // Wait for server to confirm room is read (replaces arbitrary timeout)
      await waitForRoomRead(page2, spaceId, generalRoomId);

      // User B leaves room by navigating to announcements
      await chatPage2.enterRoom('announcements');
      await waitForRoomReady(page2, 'announcements');

      // User A posts a new message while User B is away
      const newMessage = `New message ${Date.now()}`;
      await roomPage.sendMessage(newMessage);

      // Wait for server to register the unread state for User B
      await waitForRoomUnread(page2, spaceId, generalRoomId, true);

      // User B re-enters general room
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Wait for the message to arrive, then check separator
      await roomPage2.expectMessageVisible(newMessage);
      await roomPage2.expectUnreadSeparator();
    } finally {
      await context2.close();
    }
  });

  test('does not show unread separator when entering room for the first time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test needs more time

    // User A: Create account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('First Visit Test');
    const spaceId = await chatPage.getSpaceId();

    // User A enters general room and posts a message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Welcome message from creator');

    // User B: Create account, join space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room for the first time
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible('Welcome message from creator');

      // No unread separator should be shown - this is the first visit
      await roomPage2.expectNoUnreadSeparator();
    } finally {
      await context2.close();
    }
  });

  test('unread separator position stays fixed when new messages arrive', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Fixed Separator Test');
    const spaceId = await chatPage.getSpaceId();

    // User A enters general room and posts initial message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial message');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // User B: Create account, join space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general, then leaves
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible('Initial message');

      // Wait for server to confirm room is read
      await waitForRoomRead(page2, spaceId, generalRoomId);

      await chatPage2.enterRoom('announcements');
      await waitForRoomReady(page2, 'announcements');

      // User A posts first unread message
      const unreadMsg1 = `Unread 1 ${Date.now()}`;
      await roomPage.sendMessage(unreadMsg1);

      // Wait for server to register unread state
      await waitForRoomUnread(page2, spaceId, generalRoomId, true);

      // User B re-enters - should see separator before unreadMsg1
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible(unreadMsg1);
      await roomPage2.expectUnreadSeparator();

      // User A posts another message while User B is viewing
      const unreadMsg2 = `Unread 2 ${Date.now()}`;
      await roomPage.sendMessage(unreadMsg2);

      // Wait for message to arrive
      await roomPage2.expectMessageVisible(unreadMsg2);

      // Separator should still be visible (position doesn't change)
      await roomPage2.expectUnreadSeparator();
    } finally {
      await context2.close();
    }
  });

  test('no unread separator for own messages after posting and reloading', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // User creates account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Own Message Test');
    const spaceId = await chatPage.getSpaceId();

    // Enter room
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // Post a message (this marks the room as read)
    await roomPage.sendMessage('Initial message');

    // Wait for server to confirm room is read
    await waitForRoomRead(page, spaceId, generalRoomId);

    // Leave room by going to announcements
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // Go back and post another message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Wait for room to fully load
    await roomPage.expectMessageVisible('Initial message');

    // Post a second message (this should also update our last-read position)
    const ownMessage = `My own message ${Date.now()}`;
    await roomPage.sendMessage(ownMessage);

    // Reload the page
    await page.reload();
    await page.waitForURL(routes.patterns.anyRoom);

    // Wait for room to load
    await roomPage.expectMessageVisible(ownMessage);

    // The user's own message should NOT show the unread separator
    // (they clearly saw it since they posted it)
    await roomPage.expectNoUnreadSeparator();
  });
});

test.describe('Clickable unread dots', () => {
  // FIXME: clicking unread dot on space icon — multi-space flow.
  test.skip('clicking unread dot on space icon navigates to the unread room', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create account and first space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Space Alpha');
    const space1Id = await chatPage.getSpaceId();

    // User A enters general room and sends a message to mark it as read
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Hello from Space Alpha');

    // Get the general room ID for later assertions
    const generalRoomId = await getRoomIdByName(page, space1Id, 'general');

    // User A: Create second space and stay there
    await chatPage.createSpace('Space Beta');
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Hello from Space Beta');

    // User B: Create account, join Space Alpha, post a message
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, space1Id);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      const testMessage = `Clickable unread test ${Date.now()}`;
      await roomPage2.sendMessage(testMessage);

      // Wait for server to register the unread state for User A
      await waitForSpaceUnread(page, space1Id, true);

      // User A: Wait for unread dot to appear on Space Alpha's icon, then click it
      await expect(async () => {
        await chatPage.expectSpaceHasUnread('Space Alpha');
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      await chatPage.clickSpaceUnreadDot('Space Alpha');

      // User A should be navigated to the unread room in Space Alpha
      await expect(async () => {
        const url = page.url();
        expect(url).toContain(space1Id);
        expect(url).toContain(generalRoomId);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

      // The message should be visible
      await expect(page.getByText(testMessage)).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    } finally {
      await context2.close();
    }
  });
});

test.describe('Unread dot stability after loadRooms refresh', () => {
  test('room unread dot does not reappear after clearing when loadRooms is triggered', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Sticky Dot Test');
    const spaceId = await chatPage.getSpaceId();

    // User A enters general room and posts a message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial message from User A');

    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // User B: Create account, join space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room (marks as read)
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible('Initial message from User A');
      await waitForRoomRead(page2, spaceId, generalRoomId);

      // User B navigates to announcements
      await chatPage2.enterRoom('announcements');
      await waitForRoomReady(page2, 'announcements');

      // User A posts a new message → User B should see unread dot on general
      const testMessage = `Trigger unread ${Date.now()}`;
      await roomPage.sendMessage(testMessage);

      // Wait for server to register unread state
      await waitForRoomUnread(page2, spaceId, generalRoomId, true);

      // User B should see unread dot
      const generalLink = chatPage2.roomList.locator('a', { hasText: '# general' });
      await expect(async () => {
        await expect(generalLink).toHaveClass(/font-semibold/);
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // User B enters general room → dot should clear
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible(testMessage);

      // Wait for server to confirm room is read
      await waitForRoomRead(page2, spaceId, generalRoomId);

      // Verify the unread dot is gone
      await expect(async () => {
        await expect(generalLink).not.toHaveClass(/font-semibold/);
        const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
        await expect(unreadDot).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // User B navigates to announcements (so general is not active)
      await chatPage2.enterRoom('announcements');
      await waitForRoomReady(page2, 'announcements');

      // User A renames the general room → triggers RoomUpdatedEvent → loadRooms() in User B
      await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: UpdateRoomInput!) {
						updateRoom(input: $input) { id name }
					}`,
          variables: { input: { spaceId, roomId: generalRoomId, name: 'general-renamed' } }
        }
      });

      // Wait for the rename to be visible in User B's room list
      const renamedLink = chatPage2.roomList.locator('a', { hasText: '# general-renamed' });
      await expect(renamedLink).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // The renamed room should NOT show an unread dot (the loadRooms refresh
      // should not have restored the stale unread state)
      await expect(renamedLink).not.toHaveClass(/font-semibold/);
      const unreadDot = renamedLink.locator('[data-testid="room-unread-dot"]');
      await expect(unreadDot).not.toBeVisible();
    } finally {
      await context2.close();
    }
  });
});

test.describe('Thread reply unread behavior', () => {
  test('thread reply does not cause unread dot on room or space', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create account and space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Thread Unread Test');
    const spaceId = await chatPage.getSpaceId();

    // User A enters general room and posts a root message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    const rootMessage = `Root message ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootMessage);

    const generalRoomId = await getRoomIdByName(page, spaceId, 'general');

    // User B: Create account, join space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B enters general room (marks as read)
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');
      await roomPage2.expectMessageVisible(rootMessage);

      // Wait for server to confirm room is read for User B
      await waitForRoomRead(page2, spaceId, generalRoomId);

      // User B navigates to announcements (so general is not active)
      await chatPage2.enterRoom('announcements');
      await waitForRoomReady(page2, 'announcements');

      // User A posts a thread reply to the root message
      await rootMsg.openThread();
      await roomPage.expectThreadPaneVisible();
      const threadReply = `Thread reply ${Date.now()}`;
      await roomPage.postThreadReply(threadReply);

      // Verify server-side: room should still be read for User B
      // (waitForRoomRead polls the server, giving events time to propagate)
      await waitForRoomRead(page2, spaceId, generalRoomId);

      // Verify UI: no unread dot on room — use toPass() to allow events to settle
      // before asserting absence (negative assertions need extra care)
      const generalLink = chatPage2.roomList.locator('a', { hasText: '# general' });
      const roomUnreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
      await expect(async () => {
        await expect(generalLink).not.toHaveClass(/font-semibold/);
        await expect(roomUnreadDot).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

      // Now User A posts a new ROOT message — this SHOULD cause unread
      await roomPage.closeThread();
      const newRootMessage = `New root message ${Date.now()}`;
      await roomPage.sendMessage(newRootMessage);

      // Wait for server to register unread state
      await waitForRoomUnread(page2, spaceId, generalRoomId, true);

      // User B should see unread dot on general room
      await expect(async () => {
        await expect(generalLink).toHaveClass(/font-semibold/);
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
    } finally {
      await context2.close();
    }
  });
});
