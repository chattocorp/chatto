import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { test } from './setup';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import { TIMEOUTS } from './constants';

test.describe('Typing indicators', () => {
  // Typing indicator tests need longer timeout because:
  // - Setup creates users, spaces, rooms
  // - Typing indicator timeout is 6 seconds
  // - Need time for cleanup interval to run
  test.setTimeout(TIMEOUTS.POLLING_EXTENDED);

  test('user sees typing indicator when another user is typing', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and enter room
    const user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // User 2: Join the same space and room
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      const user2 = await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // Wait for both users to be in the room
      await roomPage.expectMemberVisible(user2.displayName, { timeout: TIMEOUTS.REALTIME_EVENT });
      await roomPage2.expectMemberVisible(user1.displayName, { timeout: TIMEOUTS.REALTIME_EVENT });

      // Verify no typing indicator initially (no avatar for user2 in typing indicator)
      await expect(page.locator('.typing-dots')).not.toBeVisible();

      // User 2: Start typing (without sending) - use type() to simulate keystrokes
      await roomPage2.messageInput.click();
      await roomPage2.messageInput.pressSequentially('Hello', { delay: 50 });

      // User 1: Should see typing indicator appear
      await expect(page.locator('.typing-dots')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User 2: Clear the input (stop typing)
      await roomPage2.messageInput.fill('');

      // User 1: Typing indicator should disappear after timeout (~6 seconds)
      await expect(page.locator('.typing-dots')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('typing indicator disappears after timeout when user stops typing', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and enter room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // User 2: Join the same space and room
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // User 2: Start typing
      await roomPage2.messageInput.fill('Typing something...');

      // User 1: Should see typing indicator appear
      await expect(page.locator('.typing-dots')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Wait for the typing indicator to disappear (timeout is 6 seconds)
      // We don't send any more typing events, so it should auto-clear
      await expect(page.locator('.typing-dots')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('thread typing indicator is scoped to thread only', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space, post a message to start a thread
    const _user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const rootMessage = `Thread root ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // User 2: Join the same space and room
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // Wait for both users to be in the room and message to be visible
      await roomPage2.expectMessageVisible(rootMessage, { timeout: TIMEOUTS.REALTIME_EVENT });

      // User 2: Open the thread
      const message2 = roomPage2.getMessage(rootMessage);
      await message2.openThread();
      await roomPage2.expectThreadPaneVisible();

      // User 1: Open the same thread
      const message1 = roomPage.getMessage(rootMessage);
      await message1.openThread();
      await roomPage.expectThreadPaneVisible();

      // User 2: Start typing in the thread
      await roomPage2.threadReplyInput.fill('Thread reply...');

      // User 1: Should see typing indicator in the THREAD pane (avatar visible)
      const threadTypingDots = roomPage.threadPane.locator('.typing-dots');
      await expect(threadTypingDots).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User 1: Should NOT see typing indicator in the MAIN room
      // The typing dots should only appear once (in the thread pane)
      const allTypingDots = await page.locator('.typing-dots').count();
      expect(allTypingDots).toBe(1);
    } finally {
      await context2.close();
    }
  });

  test('main room typing does not show in thread pane', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // Use wide viewport so both room and thread panes are visible (needed for @3xl container query)
    await page.setViewportSize({ width: 1400, height: 900 });

    // User 1: Create space, post a message to start a thread
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const rootMessage = `Thread root ${Date.now()}`;
    await roomPage.sendMessage(rootMessage);

    // User 2: Join the same space and room
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // Wait for message to be visible
      await roomPage2.expectMessageVisible(rootMessage, { timeout: TIMEOUTS.REALTIME_EVENT });

      // User 1: Open the thread
      const message1 = roomPage.getMessage(rootMessage);
      await message1.openThread();
      await roomPage.expectThreadPaneVisible();

      // User 2: Type in the MAIN room (not in thread)
      await roomPage2.messageInput.fill('Main room typing...');

      // User 1: Should NOT see typing indicator in the thread pane
      // Use toPass() to give the typing event time to propagate, then verify absence
      const threadTypingDots = roomPage.threadPane.locator('.typing-dots');
      await expect(async () => {
        await expect(threadTypingDots).not.toBeVisible();
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [500, 1000, 2000] });

      // But should see it somewhere on the page (main room area)
      await expect(page.locator('.typing-dots')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('multiple users typing shows combined indicator', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and enter room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // User 2 and User 3: Join the same space and room
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();
    const context3 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page3 = await context3.newPage();

    try {
      const user2 = await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      const user3 = await createAndLoginTestUser(page3);
      const chatPage3 = new ChatPage(page3);
      const roomPage3 = new RoomPage(page3);
      const explorePage3 = new ExplorePage(page3);

      // User 2 joins
      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // User 3 joins
      await chatPage3.goto();
      await chatPage3.goToExploreSpaces();
      await explorePage3.joinSpace(spaceName);
      await chatPage3.enterRoom('general');

      // Wait for all users to be visible
      await roomPage.expectMemberVisible(user2.displayName, { timeout: TIMEOUTS.REALTIME_EVENT });
      await roomPage.expectMemberVisible(user3.displayName, { timeout: TIMEOUTS.REALTIME_EVENT });

      // User 2: Start typing
      await roomPage2.messageInput.fill('User 2 typing...');

      // User 1: Should see typing indicator appear
      await expect(page.locator('.typing-dots')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // User 3: Also start typing
      await roomPage3.messageInput.fill('User 3 typing...');

      // User 1: Should still see typing indicator (now with two user initials)
      await expect(page.locator('.typing-dots')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
      await context3.close();
    }
  });

  test('typing indicator does not cause JavaScript errors', async ({
    page,
    chatPage,
    roomPage: _roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and enter room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // User 2: Join and set up error capture
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    const pageErrors: string[] = [];
    page.on('pageerror', (err) => {
      pageErrors.push(err.message);
    });

    try {
      await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // User 2: Type multiple times to trigger typing events.
      // Delays between keystrokes simulate real user typing cadence and
      // ensure each typing event is published as a separate update.
      await roomPage2.messageInput.fill('First');
      await roomPage2.messageInput.pressSequentially(' Second', { delay: 100 });
      await roomPage2.messageInput.pressSequentially(' Third', { delay: 100 });

      // Wait for typing indicator to appear (avatar visible)
      await expect(page.locator('.typing-dots')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Wait for it to disappear (6 second timeout + buffer)
      await expect(page.locator('.typing-dots')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Check for critical errors
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
});
