import { expect } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import {
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
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());

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

      // Rename the general room → triggers RoomUpdatedEvent → loadRooms() in
      // User B. Issue #330: regular members can't manage rooms on the
      // bootstrap space, so do the rename as e2eadmin in a side context (so
      // user A's page session stays intact).
      const adminCtx = await browser!.newContext({ baseURL: serverURL });
      try {
        const adminPage = await adminCtx.newPage();
        await adminPage.request.post('/auth/login', {
          data: { login: 'e2eadmin', password: 'adminpassword123' }
        });
        await adminPage.request.post('/api/graphql', {
          headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
          data: {
            query: `mutation($input: UpdateRoomInput!) { updateRoom(input: $input) { id name } }`,
            variables: { input: { spaceId, roomId: generalRoomId, name: 'general-renamed' } }
          }
        });
      } finally {
        await adminCtx.close();
      }

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
