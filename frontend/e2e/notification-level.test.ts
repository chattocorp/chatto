import { expect } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import {
  graphqlQuery,
  waitForRoomUnread,
  waitForSpaceUnread,
  getRoomIdByName
} from './fixtures/graphqlHelpers';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

/**
 * Helper to set a space notification level via GraphQL mutation.
 */
async function setSpaceNotificationLevel(
  page: import('@playwright/test').Page,
  spaceId: string,
  level: string
): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: SetSpaceNotificationLevelInput!) {
				setSpaceNotificationLevel(input: $input) {
					level effectiveLevel
				}
			}`,
      variables: { input: { spaceId, level } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

/**
 * Helper to set a room notification level via GraphQL mutation.
 */
async function setRoomNotificationLevel(
  page: import('@playwright/test').Page,
  spaceId: string,
  roomId: string,
  level: string
): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: SetRoomNotificationLevelInput!) {
				setRoomNotificationLevel(input: $input) {
					level effectiveLevel
				}
			}`,
      variables: { input: { spaceId, roomId, level } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

test.describe('Notification Level - Preferences Page', () => {
  test('preferences page renders with space-level and room sections', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Navigate to preferences page
    await page.getByRole('link', { name: 'Preferences' }).click();
    await page.waitForURL(/\/preferences/);

    // Verify page heading
    await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible();

    // Verify space notification level section
    await expect(page.getByText('Space Notification Level')).toBeVisible();

    // Verify the three space-level option labels are visible
    await expect(page.getByText('No notifications or unread markers')).toBeVisible();
    await expect(
      page.getByText('Unread markers + mentions, DMs, and thread replies')
    ).toBeVisible();
    await expect(page.getByText('Normal + notification for every new message')).toBeVisible();

    // Verify room overrides section is visible
    await expect(page.getByText('Room Overrides')).toBeVisible();

    // The general room should be listed in the room overrides (use testid)
    await expect(page.getByTestId('room-notification-general')).toBeVisible();
  });

  test('can set space notification level via UI', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Navigate to preferences
    await page.getByRole('link', { name: 'Preferences' }).click();
    await page.waitForURL(/\/preferences/);

    // Normal should be selected by default (check for accent border on button)
    const normalButton = page.locator('button', { hasText: 'Normal' }).filter({
      hasText: 'Unread markers'
    });
    await expect(normalButton).toHaveClass(/border-accent/);

    // Click Muted button
    const mutedButton = page.locator('button', { hasText: 'Muted' }).filter({
      hasText: 'No notifications'
    });
    await mutedButton.click();

    // Wait for success toast
    await expect(page.getByText('Space notification level updated')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Verify Muted is now selected (has accent border)
    await expect(mutedButton).toHaveClass(/border-accent/);

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible();
    const mutedButtonReloaded = page.locator('button', { hasText: 'Muted' }).filter({
      hasText: 'No notifications'
    });
    await expect(mutedButtonReloaded).toHaveClass(/border-accent/);
  });

  test('can set room notification level via UI', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Navigate to preferences
    await page.getByRole('link', { name: 'Preferences' }).click();
    await page.waitForURL(/\/preferences/);

    // Find the room override row for "general" and change its select
    const generalRow = page.getByTestId('room-notification-general');
    const select = generalRow.locator('select');

    // Default should be selected initially
    await expect(select).toHaveValue('DEFAULT');

    // Change to MUTED
    await select.selectOption('MUTED');

    // Wait for success toast
    await expect(page.getByText('Room notification level updated')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Verify it persists after reload
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible();
    const generalRowAfterReload = page.getByTestId('room-notification-general');
    await expect(generalRowAfterReload.locator('select')).toHaveValue('MUTED');
  });

  test('preferences link is visible in space sidebar', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Verify Preferences link is visible in sidebar
    const preferencesLink = page.getByRole('link', { name: 'Preferences' });
    await expect(preferencesLink).toBeVisible();
  });
});

// FIXME: these tests use the multi-user joinSpace + post-message flow
// which races in CI under the URL-collapse transition (#330 phase 2).
// Re-enable when tests use the bootstrap space directly instead of
// creating + joining fresh spaces (next phase-2 PR removes createSpace).
test.describe.skip('Notification Level - Muted Room Hides Unread', () => {
  test('muted room does not show unread dot when messages are sent', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create space with a room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Mute Test');
    const spaceId = await chatPage.getSpaceId();

    // Create a second room to mute
    const mutedRoomName = await chatPage.createRoom('muted-room');

    // Enter general room so we can see the room list
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Get room IDs
    const mutedRoomId = await getRoomIdByName(page, spaceId, mutedRoomName);

    // Mute the second room via API
    await setRoomNotificationLevel(page, spaceId, mutedRoomId, 'MUTED');

    // User B: Join and post a message in the muted room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());

      const roomPage2 = new RoomPage(page2);

      // Join the muted room via Browse Rooms (stay on page, then navigate)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const mutedRoomItem = page2.locator('li', { hasText: `# ${mutedRoomName}` });
      await mutedRoomItem.getByRole('button', { name: 'Join' }).click();
      await expect(mutedRoomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(mutedRoomName);
      await waitForRoomReady(page2, mutedRoomName);

      // Post a message
      await roomPage2.sendMessage('Message in muted room');

      // Wait a bit for the event to propagate
      await page.waitForTimeout(TIMEOUTS.SERVER_MUTATION_SYNC);

      // User A: The muted room should NOT show an unread dot
      // (The room-unread-dot testid is used for unread indicators)
      await expect(page.locator('[data-testid="room-unread-dot"]')).not.toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // Also verify via GraphQL that HasUnread returns false for the muted room
      await expect(async () => {
        const data = await graphqlQuery<{ room: { hasUnread: boolean } }>(
          page,
          `query($spaceId: ID!, $roomId: ID!) { room(spaceId: $spaceId, roomId: $roomId) { hasUnread } }`,
          { spaceId, roomId: mutedRoomId }
        );
        expect(data.room.hasUnread).toBe(false);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    } finally {
      await context2.close();
    }
  });

  test('muted room does not show space-level unread dot', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create a space with a muted room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Mute Space Test');
    const muteSpaceId = await chatPage.getSpaceId();

    // Create a second room and mute it
    const mutedRoomName = await chatPage.createRoom('muted-channel');
    const mutedRoomId = await getRoomIdByName(page, muteSpaceId, mutedRoomName);
    await setRoomNotificationLevel(page, muteSpaceId, mutedRoomId, 'MUTED');

    // Navigate away from the space so the space icon can show the unread dot.
    // Use direct navigation to avoid dialog interception from the room creation modal.
    await page.goto(routes.spaces);
    await page.waitForURL(routes.spaces);

    // User B: Join the space and post in the muted room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, muteSpaceId);
      await page2.goto(routes.space());

      const roomPage2 = new RoomPage(page2);

      // Join the muted room via Browse Rooms (stay on page, then navigate)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const mutedRoomItem = page2.locator('li', { hasText: `# ${mutedRoomName}` });
      await mutedRoomItem.getByRole('button', { name: 'Join' }).click();
      await expect(mutedRoomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(mutedRoomName);
      await waitForRoomReady(page2, mutedRoomName);

      // Post a message
      await roomPage2.sendMessage('Message in muted channel');

      // Wait for the event to propagate
      await page.waitForTimeout(TIMEOUTS.SERVER_MUTATION_SYNC);

      // User A: The space with the muted room should NOT show a space-level unread dot.
      // Scope assertion to the specific space icon by its aria-label.
      const muteSpaceIcon = page.locator(
        'a[data-testid="space-icon"][aria-label="Mute Space Test"]'
      );
      await expect(muteSpaceIcon).toBeVisible();

      // The unread dot is a sibling of the link inside the same parent div
      const muteSpaceContainer = muteSpaceIcon.locator('..');
      await expect(muteSpaceContainer.locator('[data-testid="space-unread-dot"]')).not.toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    } finally {
      await context2.close();
    }
  });

  test('muting a room with existing unread clears the space unread dot', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create a space with two rooms
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Mute Clears Dot');
    const spaceId = await chatPage.getSpaceId();

    // Create a second room (the one we'll mute after it gets unread)
    const targetRoomName = await chatPage.createRoom('will-be-muted');
    const targetRoomId = await getRoomIdByName(page, spaceId, targetRoomName);

    // Enter general room to mark it as read
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Hello');

    // Navigate away from this space so the space icon dot can show
    await chatPage.createSpace('Other Space');
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Other space msg');

    // User B: Join the first space and post in the target room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      // User B joins and enters the target room
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const roomItem = page2.locator('li', { hasText: `# ${targetRoomName}` });
      await roomItem.getByRole('button', { name: 'Join' }).click();
      await expect(roomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      await chatPage2.enterRoom(targetRoomName);
      await waitForRoomReady(page2, targetRoomName);

      // User B posts a message → creates unread for User A
      await roomPage2.sendMessage('Message before muting');

      // Wait for server to register unread state for User A
      await waitForSpaceUnread(page, spaceId, true);

      // User A should see the space unread dot
      await expect(async () => {
        await chatPage.expectSpaceHasUnread('Mute Clears Dot');
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // User A mutes the target room via API
      await setRoomNotificationLevel(page, spaceId, targetRoomId, 'MUTED');

      // The space unread dot should clear since the only unread room is now muted
      await expect(async () => {
        await chatPage.expectSpaceHasNoUnread('Mute Clears Dot');
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
    } finally {
      await context2.close();
    }
  });

  test('unmuted room shows unread dot normally', async ({ page, chatPage, browser, serverURL }) => {
    test.setTimeout(60000);

    // User A: Create space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Unmute Test');
    const spaceId = await chatPage.getSpaceId();

    // Create a test room (not muted)
    const testRoomName = await chatPage.createRoom('test-room');

    // Stay in general room so unread dots for test-room are visible
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // User B: Join space and post in test-room
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());

      const roomPage2 = new RoomPage(page2);

      // Join the test room via Browse Rooms (stay on page, then navigate)
      await page2.getByRole('link', { name: 'Browse Rooms' }).click();
      const testRoomItem = page2.locator('li', { hasText: `# ${testRoomName}` });
      await testRoomItem.getByRole('button', { name: 'Join' }).click();
      await expect(testRoomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Navigate to the room via sidebar
      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom(testRoomName);
      await waitForRoomReady(page2, testRoomName);

      // Post a message
      await roomPage2.sendMessage('Message in unmuted room');

      // Get room ID for server-side verification
      const testRoomId = await getRoomIdByName(page, spaceId, testRoomName);

      // Wait for server to register unread
      await waitForRoomUnread(page, spaceId, testRoomId, true);

      // User A: The unmuted room SHOULD show an unread dot
      await expect(page.locator('[data-testid="room-unread-dot"]')).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    } finally {
      await context2.close();
    }
  });
});

test.describe('Notification Level - Server-Side Enforcement', () => {
  test('setting notification level persists via GraphQL roundtrip', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('API Test');
    const spaceId = await chatPage.getSpaceId();

    // Set space level to MUTED via API
    await setSpaceNotificationLevel(page, spaceId, 'MUTED');

    // Query it back
    const data = await graphqlQuery<{
      space: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($id: ID!) { space(id: $id) { viewerNotificationPreference { level effectiveLevel } } }`,
      { id: spaceId }
    );

    expect(data.space.viewerNotificationPreference.level).toBe('MUTED');
    expect(data.space.viewerNotificationPreference.effectiveLevel).toBe('MUTED');
  });

  test('room inherits space notification level when set to DEFAULT', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Inherit Test');
    const spaceId = await chatPage.getSpaceId();
    const roomId = await getRoomIdByName(page, spaceId, 'general');

    // Set space level to MUTED
    await setSpaceNotificationLevel(page, spaceId, 'MUTED');

    // Room (with DEFAULT) should inherit MUTED from space
    const data = await graphqlQuery<{
      room: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($spaceId: ID!, $roomId: ID!) {
				room(spaceId: $spaceId, roomId: $roomId) {
					viewerNotificationPreference { level effectiveLevel }
				}
			}`,
      { spaceId, roomId }
    );

    expect(data.room.viewerNotificationPreference.level).toBe('DEFAULT');
    expect(data.room.viewerNotificationPreference.effectiveLevel).toBe('MUTED');
  });

  test('room level overrides space level', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Override Test');
    const spaceId = await chatPage.getSpaceId();
    const roomId = await getRoomIdByName(page, spaceId, 'general');

    // Set space level to MUTED
    await setSpaceNotificationLevel(page, spaceId, 'MUTED');

    // Set room level to ALL_MESSAGES (overrides space MUTED)
    await setRoomNotificationLevel(page, spaceId, roomId, 'ALL_MESSAGES');

    // Room should show ALL_MESSAGES as effective level
    const data = await graphqlQuery<{
      room: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($spaceId: ID!, $roomId: ID!) {
				room(spaceId: $spaceId, roomId: $roomId) {
					viewerNotificationPreference { level effectiveLevel }
				}
			}`,
      { spaceId, roomId }
    );

    expect(data.room.viewerNotificationPreference.level).toBe('ALL_MESSAGES');
    expect(data.room.viewerNotificationPreference.effectiveLevel).toBe('ALL_MESSAGES');
  });
});
