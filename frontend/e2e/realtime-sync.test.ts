import { expect } from '@playwright/test';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, ExplorePage, SettingsPage } from './pages';
import * as routes from './routes';

test.describe('Real-time synchronization', () => {

  test('room list updates when user joins a room from another session', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // Session 1: Create user, space
    const user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace(`Room Sync Test ${Date.now()}`);

    // Session 1: Create a new room via API (creator is auto-joined)
    const testRoomName = await chatPage.createRoom();

    // Room should be visible in session 1's room list
    await expect(chatPage.roomList.getByText(`# ${testRoomName}`)).toBeVisible();

    // Session 2: Same user in a different browser context (simulating second tab/device)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      // Login as same user in session 2
      const loginResponse = await page2.request.post('/auth/login', {
        data: {
          login: user1.login,
          password: user1.password
        }
      });
      expect(loginResponse.ok()).toBeTruthy();

      // Navigate to the space. Session 2 should already have the room
      // since it's the same user and the room was created with auto-join
      // for the creator. Allow a generous timeout because the new
      // context boots its own WebSocket subscription and rooms store
      // refresh, which races the initial sidebar render.
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      await expect(
        chatPage2.roomList.getByRole('link', { name: `# ${testRoomName}` })
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('user sees leave event when another user leaves the room', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and room, stay in it
    const _user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = `Leave Event Test ${Date.now()}`;
    await chatPage.createSpace(spaceName);
    await chatPage.createRoom('leave-test');
    await chatPage.expectRoomHeaderVisible('leave-test');

    // User 2: Join the same space and room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      const user2 = await createAndLoginTestUser(page2);
      const explorePage2 = new ExplorePage(page2);
      await explorePage2.goto();

      // Join the space
      await explorePage2.joinSpace(spaceName);

      // Join the room (via the Overview directory). Playwright leaves the
      // cursor over the Join button after click, which keeps the row in
      // :hover and swaps the button label to "Leave" — move the mouse
      // away first so the visible state is the stable "Joined" pill.
      await page2.getByRole('link', { name: 'Overview' }).click();
      const leaveTestItem = page2.locator('li', { hasText: '# leave-test' });
      await leaveTestItem.getByRole('button', { name: 'Join' }).click();
      await page2.mouse.move(0, 0);
      await expect(
        leaveTestItem.getByRole('button', { name: /^Joined$|Joined #leave-test/i })
      ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('leave-test');

      // User 1 should see User 2's join event
      await expect(page.getByText(`${user2.displayName} joined the room`)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // User 2: Leave the room
      await page2.getByTitle('Leave room').click();
      await page2.getByRole('dialog').getByRole('button', { name: 'Leave Room' }).click();

      // User 1 should see User 2's leave event in the room
      await expect(page.getByText(`${user2.displayName} left the room`)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    } finally {
      await context2.close();
    }
  });

  test('room membership events update room list in real-time', async ({ page, chatPage }) => {
    // This test verifies that when a user joins a room via the room directory,
    // the room list updates immediately

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace(`Room Join Test ${Date.now()}`);

    // User is auto-joined to general and announcements
    await expect(chatPage.roomList.getByText('# general')).toBeVisible();
    await expect(chatPage.roomList.getByText('# announcements')).toBeVisible();

    // Create a room (user is auto-joined as creator)
    const testRoomName = `testing-${Date.now()}`;
    await chatPage.createRoom(testRoomName);
    await expect(chatPage.roomList.getByText(`# ${testRoomName}`)).toBeVisible();

    // Leave the room so we can test joining it
    await page.getByTitle('Leave room').click();
    await page.getByRole('dialog').getByRole('button', { name: 'Leave Room' }).click();

    // After leaving, room should NOT be in the room list
    await expect(chatPage.roomList.getByText(`# ${testRoomName}`)).not.toBeVisible();

    // Open room directory and join the room
    await page.getByRole('link', { name: 'Overview' }).click();

    // Click the Join button for the test room
    const testRoomItem = page.locator('li', { hasText: `# ${testRoomName}` });
    await testRoomItem.getByRole('button', { name: 'Join' }).click();
    // Hover-stable: the button swaps visible text to "Leave" on hover.
    await expect(testRoomItem.locator('button[title^="Joined "]')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The room should now appear in the room list (real-time update)
    await expect(chatPage.roomList.getByText(`# ${testRoomName}`)).toBeVisible();
  });

  test('display name updates propagate to other users in real-time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and room
    const user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = `Display Name Test ${Date.now()}`;
    await chatPage.createSpace(spaceName);
    await chatPage.createRoom('test-room');
    await chatPage.expectRoomHeaderVisible('test-room');

    // User 2: Join the space and room first (so they can see messages)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      const _user2 = await createAndLoginTestUser(page2);
      const explorePage2 = new ExplorePage(page2);
      await explorePage2.goto();
      await explorePage2.joinSpace(spaceName);

      // Join the room via Browse Rooms, then navigate to it
      await page2.getByRole('link', { name: 'Overview' }).click();
      const testRoomItem2 = page2.locator('li', { hasText: '# test-room' });
      await testRoomItem2.getByRole('button', { name: 'Join' }).click();
      await expect(testRoomItem2.locator('button[title^="Joined "]')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('test-room');

      // User 1: Send a message now that both users are in the room
      await roomPage.sendMessage('Hello from User 1');

      // User 2 should see the message appear (wait for real-time delivery)
      await expect(page2.getByText('Hello from User 1')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Locate the message author element on User 2's view
      // The message article contains a <button> with the author name (clickable for profile popover)
      const messageArticle = page2.locator('[role="article"]', { hasText: 'Hello from User 1' });
      await expect(messageArticle.getByRole('button', { name: user1.displayName })).toBeVisible();

      // User 1: Change display name via settings
      const settingsPage = new SettingsPage(page);
      await settingsPage.goto();
      const newDisplayName = `Updated Name ${Date.now()}`;
      await settingsPage.updateDisplayName(newDisplayName);

      // User 2: Should see updated display name in the member list (without refresh)
      // This tests the RoomSidebar component's use of getLiveDisplayName
      // Note: Live events may take a few seconds to propagate across users
      // Poll for the update with a longer timeout since WebSocket events can be delayed
      await expect(async () => {
        const memberListText = await page2.locator('[aria-label="Member list"]').textContent();
        expect(memberListText).toContain(newDisplayName);
      }).toPass({ timeout: TIMEOUTS.POLLING_EXTENDED, intervals: [...POLLING_INTERVALS] });

      // User 2: Should also see updated display name on User 1's message
      await expect(messageArticle.getByRole('button', { name: newDisplayName })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    } finally {
      await context2.close();
    }
  });

  test('display name update does not cause JavaScript errors on receiving clients', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // This test specifically checks that receiving display name updates doesn't crash
    // the frontend (regression test for lifecycle_outside_component bug)

    // User 1: Create space and room
    const _user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = `Error Check Test ${Date.now()}`;
    await chatPage.createSpace(spaceName);
    await chatPage.createRoom('error-test');
    await chatPage.expectRoomHeaderVisible('error-test');

    // User 2: Join the space and room, capture console errors
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    const consoleErrors: string[] = [];
    page2.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    // Also capture page errors (uncaught exceptions)
    const pageErrors: string[] = [];
    page2.on('pageerror', (err) => {
      pageErrors.push(err.message);
    });

    try {
      await createAndLoginTestUser(page2);
      const explorePage2 = new ExplorePage(page2);
      await explorePage2.goto();
      await explorePage2.joinSpace(spaceName);

      // Join the room via Browse Rooms, then navigate to it
      await page2.getByRole('link', { name: 'Overview' }).click();
      const errorTestItem = page2.locator('li', { hasText: '# error-test' });
      await errorTestItem.getByRole('button', { name: 'Join' }).click();
      await expect(errorTestItem.locator('button[title^="Joined "]')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('error-test');

      // Wait for room to be ready (connection established)
      await expect(page2.getByText('Real-time updates paused')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User 1: Change display name
      const settingsPage = new SettingsPage(page);
      await settingsPage.goto();
      const newDisplayName = `Updated ${Date.now()}`;
      await settingsPage.updateDisplayName(newDisplayName);

      // Wait for the event to propagate to User 2
      // Check member list for the update
      await expect(async () => {
        const memberListText = await page2.locator('[aria-label="Member list"]').textContent();
        expect(memberListText).toContain(newDisplayName);
      }).toPass({ timeout: TIMEOUTS.POLLING_EXTENDED, intervals: [...POLLING_INTERVALS] });

      // Check for any JavaScript errors that occurred during the update
      // Filter out non-critical errors (like favicon 404s)
      const criticalErrors = [
        ...consoleErrors.filter(
          (e) => e.includes('lifecycle_outside_component') || e.includes('getContext')
        ),
        ...pageErrors.filter(
          (e) => e.includes('lifecycle_outside_component') || e.includes('getContext')
        )
      ];

      expect(criticalErrors).toEqual([]);
    } finally {
      await context2.close();
    }
  });

  test('avatar updates are visible to other users in real-time', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account, space, and navigate to general room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    const spaceId = await chatPage.getSpaceId();

    // Navigate to "general" room to see member list
    const roomPage = await chatPage.enterRoom('general');

    // User A should see themselves in the member list
    await expect(roomPage.memberList).toBeVisible();
    await roomPage.expectMemberVisible(userA.login);

    // User B: Create account and join the space
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      const userB = await createAndLoginTestUser(page2);

      // User B joins the space via API helper
      await joinSpace(page2);

      // Navigate to the space
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      // User B clicks on general room
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Wait for User B to be visible in User A's member list
      await roomPage.expectMemberVisible(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });

      // User A: Verify User B's avatar shows initials (no avatar yet)
      await roomPage.expectMemberHasInitials(userB.login);

      // User B: Navigate to settings and upload an avatar
      const settingsPage2 = new SettingsPage(page2);
      await settingsPage2.goto();
      await settingsPage2.uploadAvatar('e2e/fixtures/brighton.jpg');

      // User A: Verify User B's avatar now shows an image instead of initials
      // The avatar should update in real-time via the UserProfileUpdatedEvent
      await roomPage.expectMemberHasAvatar(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });

      // User B: Remove the avatar
      await settingsPage2.removeAvatar();

      // User A: Verify User B's avatar goes back to initials
      await roomPage.expectMemberHasInitials(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });
});
