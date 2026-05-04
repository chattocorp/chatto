import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

// FIXME: "last visited space" semantics don't apply in a single-server world
// (#330 phase 2). These tests assume multiple spaces per instance and use the
// pre-collapse URL shape. Re-enable / rewrite after the next PR removes
// createSpace and narrows discovery to the primary space.
test.describe.skip('Last Space Navigation', () => {
  test('remembers and redirects to last visited space', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space (creates default "general" room and redirects there)
    await chatPage.createSpace('Last Space Test');

    // After creating space, user is redirected to their first room (general)
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByText('# general')).toBeVisible();

    // Get the space ID from the URL
    const currentUrl = page.url();
    const spaceId = currentUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)\//)?.[1];
    expect(spaceId).toBeTruthy();

    // Navigate to /chat
    await page.goto('/chat');

    // Should be redirected back to the last space (and then to the room within it)
    await page.waitForURL(new RegExp(routes.space()));
  });

  test('redirects to browse spaces when no last space is stored', async ({ browser }) => {
    // Create a fresh browser context to avoid localStorage from other tests
    const context = await browser.newContext();
    const freshPage = await context.newPage();
    await createAndLoginTestUser(freshPage);

    await freshPage.goto('/chat');

    // Should redirect to /chat/spaces (Browse Spaces directory)
    await freshPage.waitForURL(routes.spaces);
    await expect(freshPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();
    await context.close();
  });

  test('full navigation chain: /chat -> last space -> last room', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space and a room
    await chatPage.createSpace('Chain Test Space');
    const roomName = await chatPage.createRoom('chain-room');
    await chatPage.expectRoomHeaderVisible(roomName);

    // Get the room URL
    const roomUrl = page.url();
    expect(roomUrl).toMatch(routes.patterns.anyRoom);

    // Navigate to /chat
    await page.goto('/chat');

    // Should end up at the room (via space -> room redirects)
    await page.waitForURL(roomUrl);
    await chatPage.expectRoomHeaderVisible(roomName);
  });
});

// FIXME: see "Last Space Navigation" describe above — multi-space concepts.
test.describe.skip('Invalid Last Space Handling', () => {
  test('clears storage and redirects to browse spaces when last space does not exist', async ({
    browser
  }) => {
    // Create a fresh browser context
    const context = await browser.newContext();
    const freshPage = await context.newPage();
    await createAndLoginTestUser(freshPage);

    // Navigate directly to a non-existent space
    await freshPage.goto(routes.space());

    // Should be redirected to /chat, then to /chat/spaces (Browse Spaces)
    await freshPage.waitForURL(routes.spaces);
    await expect(freshPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();

    // Verify localStorage was cleared
    const lastSpace = await freshPage.evaluate(() => localStorage.getItem('chatto:lastSpace'));
    expect(lastSpace).toBeNull();

    await context.close();
  });

  test('clears storage and redirects to browse spaces when user is not a member of last space', async ({
    page,
    chatPage,
    browser
  }) => {
    // First user creates a space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Private Space');
    const spaceUrl = page.url();
    const spaceId = spaceUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)/)?.[1];
    expect(spaceId).toBeTruthy();

    // Second user tries to use that space as their last space
    const context = await browser.newContext();
    const otherPage = await context.newPage();
    await createAndLoginTestUser(otherPage);

    // Navigate directly to the space the user is not a member of
    // (bypassing /chat so we can observe the redirect behavior directly)
    await otherPage.goto(routes.space());

    // Should be redirected to /chat, then to /chat/spaces (Browse Spaces)
    await otherPage.waitForURL(routes.spaces);
    await expect(otherPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();

    // Verify localStorage was cleared
    const lastSpace = await otherPage.evaluate(() => localStorage.getItem('chatto:lastSpace'));
    expect(lastSpace).toBeNull();

    await context.close();
  });
});

// FIXME: relies on the pre-collapse two-segment URL shape
// (/chat/-/<spaceId>/<roomId>) and on creating multiple rooms via the
// createSpace flow. Re-write in the next phase-2 PR (#330) once the URL
// shape is settled and tests can use the bootstrap space directly.
test.describe.skip('Invalid Last Room Handling', () => {
  test('redirects to first room when last room does not exist', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space (has default "general" room)
    await chatPage.createSpace('Test Space');
    const currentUrl = page.url();
    const spaceId = currentUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)/)?.[1];
    expect(spaceId).toBeTruthy();

    // Navigate directly to a non-existent room in the space
    await page.goto(routes.room('nonexistent-room-id'));

    // Should be redirected to the first available room (general)
    // Room.svelte detects invalid room, clears localStorage, redirects to space page
    // Space page load function then redirects to first available room
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByText('# general')).toBeVisible();
  });

  test('redirects to another room when user left the last room', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space (has default "general" room) and another room
    await chatPage.createSpace('Leave Room Test');
    const roomName = await chatPage.createRoom('temp-room');
    await chatPage.expectRoomHeaderVisible(roomName);

    const roomUrl = page.url();
    const spaceId = roomUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)\//)?.[1];
    const roomId = roomUrl.match(/\/chat\/-\/[a-zA-Z0-9]+\/([a-zA-Z0-9]+)/)?.[1];
    expect(spaceId).toBeTruthy();
    expect(roomId).toBeTruthy();

    // Leave the room via the leave button and confirmation dialog
    await page.getByTitle('Leave room').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Leave Room' }).click();

    // Should be redirected to the other room (general) since user still has rooms in space
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByText('# general')).toBeVisible();

    // Now try to navigate directly to the room we left
    await page.goto(routes.room(roomId!));

    // Should redirect to another room since the left room is invalid
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByText('# general')).toBeVisible();
  });
});

// FIXME: see "Invalid Last Room Handling" describe — same two-segment URL
// regex + multi-space assumptions. The "last room is space-specific" test
// is also intrinsically multi-space and goes away in the next PR.
test.describe.skip('Last Room Navigation', () => {
  test('sidebar highlights the correct room after auto-redirect from last room', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space and room
    await chatPage.createSpace('Highlight Test');
    const roomName = await chatPage.createRoom('highlight-room');
    await chatPage.expectRoomHeaderVisible(roomName);

    // Get URLs for later verification
    const roomUrl = page.url();
    const spaceId = roomUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)\//)?.[1];
    expect(spaceId).toBeTruthy();

    // Navigate away (use explore page to ensure clean navigation)
    await chatPage.goToExploreSpaces();
    await expect(page).toHaveURL(routes.spaces);

    // Navigate to space root (not directly to room)
    await page.goto(routes.space());

    // Wait for redirect to the last room
    await page.waitForURL(roomUrl, { timeout: TIMEOUTS.REALTIME_EVENT });

    // Verify sidebar item has active state (aria-current and highlight class)
    // Use toPass() to handle timing issues with reactivity updates
    const roomLink = page.locator('.room-list a', { hasText: `# ${roomName}` });
    await expect(async () => {
      await expect(roomLink).toHaveAttribute('aria-current', 'page');
      await expect(roomLink).toHaveClass(/bg-surface-100/);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
  });

  test('remembers and redirects to last visited room in a space', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space and a room
    await chatPage.createSpace('Last Room Test Space');
    const roomName = await chatPage.createRoom('test-room');

    // Verify we're in the room
    await chatPage.expectRoomHeaderVisible(roomName);

    // Get the current URL (should be /chat/-/[spaceId]/[roomId])
    const roomUrl = page.url();
    expect(roomUrl).toMatch(routes.patterns.anyRoom);

    // Navigate to a different space (e.g., explore)
    await chatPage.goToExploreSpaces();
    await expect(page).toHaveURL(routes.spaces);

    // Navigate back to the space by clicking on it in the sidebar
    // First, get the space ID from the URL
    const spaceId = roomUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)\//)?.[1];
    expect(spaceId).toBeTruthy();

    // Navigate to the space root (not the room)
    await page.goto(routes.space());

    // Should be redirected to the last room
    await page.waitForURL(roomUrl);
    await chatPage.expectRoomHeaderVisible(roomName);
  });

  test('redirects to first room when no last room is stored', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space - user gets redirected to the default "general" room
    await chatPage.createSpace('Auto Room Space');

    // Should be redirected to the first room (general) since no last room is stored
    await page.waitForURL(routes.patterns.anyRoom);
    await expect(page.getByText('# general')).toBeVisible();
  });

  test('last room is space-specific', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create first space with a room
    await chatPage.createSpace('Space One');
    const room1 = await chatPage.createRoom('room-one');
    await chatPage.expectRoomHeaderVisible(room1);
    const space1RoomUrl = page.url();
    const space1Id = space1RoomUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)\//)?.[1];

    // Create second space with a different room
    await chatPage.createSpace('Space Two');
    const room2 = await chatPage.createRoom('room-two');
    await chatPage.expectRoomHeaderVisible(room2);
    const space2RoomUrl = page.url();
    const space2Id = space2RoomUrl.match(/\/chat\/-\/([a-zA-Z0-9]+)\//)?.[1];

    // Navigate to space 1 root - should redirect to room-one
    await page.goto(routes.space());
    await page.waitForURL(space1RoomUrl);
    await chatPage.expectRoomHeaderVisible(room1);

    // Navigate to space 2 root - should redirect to room-two
    await page.goto(routes.space());
    await page.waitForURL(space2RoomUrl);
    await chatPage.expectRoomHeaderVisible(room2);
  });
});
