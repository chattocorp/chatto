import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Message Cache - Cross-Room and Cross-Space Scenarios', () => {
  test.describe('Thread replies from users in different rooms', () => {
    test('thread reply count updates when user is in different room of same space', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates space, posts root message, stays in general room
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();

      // User A posts root message
      const rootMessage = `Root message for cross-room test ${Date.now()}`;
      await roomPage.sendMessage(rootMessage);

      // User A creates and enters a second room
      const _secondRoomName = await chatPage.createRoom(`room-b-${Date.now()}`);

      // User B joins the space and enters the general room
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User B sees the root message
        await roomPage2.expectMessageVisible(rootMessage);

        // User B opens thread and posts a reply
        const message2 = roomPage2.getMessage(rootMessage);
        await message2.openThread();
        await roomPage2.expectThreadPaneVisible();

        const threadReply = `Reply from User B ${Date.now()}`;
        await roomPage2.postThreadReply(threadReply);
        await roomPage2.expectTextInThreadPane(threadReply);

        // User B closes thread
        await roomPage2.closeThread();

        // User A is in a DIFFERENT room (room-b)
        // Now User A navigates back to general room
        await chatPage.enterRoom('general');
        await waitForRoomReady(page, 'general');

        // User A should see the root message with "1 reply" indicator
        await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // User A opens the thread and sees the reply
        const message1 = roomPage.getMessage(rootMessage);
        await message1.openThread();
        await roomPage.expectThreadPaneVisible();
        await roomPage.expectTextInThreadPane(threadReply);
      } finally {
        await context2.close();
      }
    });

    test('thread reply appears in real-time when User A is in different room', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates space and root message
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();

      const rootMessage = `Cross-room realtime test ${Date.now()}`;
      const message1 = await roomPage.sendMessage(rootMessage);

      // User A opens thread (to load it into cache)
      await message1.openThread();
      await roomPage.expectThreadPaneVisible();
      await roomPage.expectTextInThreadPane(rootMessage);
      await roomPage.closeThread();

      // User A creates and moves to a second room
      const _secondRoomName = await chatPage.createRoom(`room-b-${Date.now()}`);

      // User B joins and enters general room
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(rootMessage);

        // User B opens thread and posts reply (while User A is in room-b)
        const message2 = roomPage2.getMessage(rootMessage);
        await message2.openThread();
        await roomPage2.expectThreadPaneVisible();

        const threadReply = `Reply while User A in different room ${Date.now()}`;
        await roomPage2.postThreadReply(threadReply);
        await roomPage2.expectTextInThreadPane(threadReply);

        // User A navigates back to general and opens the same thread
        await chatPage.enterRoom('general');
        await waitForRoomReady(page, 'general');

        // The reply count should show
        await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // User A opens thread - reply should be visible
        const message1Refetched = roomPage.getMessage(rootMessage);
        await message1Refetched.openThread();
        await roomPage.expectThreadPaneVisible();
        await roomPage.expectTextInThreadPane(threadReply);
      } finally {
        await context2.close();
      }
    });
  });

  // FIXME: tests cross-space switching, which doesn't apply in a
  // single-server world (#330 phase 2). The second-space flow also breaks
  // because the URL collapse can't represent a non-primary space. Re-enable
  // / rewrite in the next PR cycle once createSpace is removed.
  test.describe.skip('Messages when switching between spaces', () => {
    test('messages appear correctly after switching from another space', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates Space 1 and posts a message
      await createAndLoginTestUser(page);
      await chatPage.goto();
      const _space1Name = await chatPage.createSpace(`Space1-${Date.now()}`);
      await chatPage.enterRoom('general');

      const space1Id = await chatPage.getSpaceId();

      const space1Message = `Message in Space 1 ${Date.now()}`;
      await roomPage.sendMessage(space1Message);

      // User A creates Space 2
      await chatPage.goto();
      const _space2Name = await chatPage.createSpace(`Space2-${Date.now()}`);
      await chatPage.enterRoom('general');

      const _space2Id = await chatPage.getSpaceId();

      // User B joins Space 1 and posts a message while User A is in Space 2
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(space1Id));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User B posts message in Space 1 (while User A is in Space 2)
        const userBMessage = `Message from User B ${Date.now()}`;
        await roomPage2.sendMessage(userBMessage);
        await roomPage2.expectMessageVisible(userBMessage);

        // User A switches back to Space 1
        await page.goto(routes.space());
        await page.waitForURL(new RegExp(routes.space()));
        await chatPage.enterRoom('general');
        await waitForRoomReady(page, 'general');

        // User A should see BOTH messages (own message and User B's message)
        await roomPage.expectMessageVisible(space1Message);
        await roomPage.expectMessageVisible(userBMessage);
      } finally {
        await context2.close();
      }
    });

    test('thread in Space 1 receives replies when user switches back from Space 2', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates Space 1 with a thread
      await createAndLoginTestUser(page);
      await chatPage.goto();
      const _space1Name = await chatPage.createSpace(`ThreadSpace1-${Date.now()}`);
      await chatPage.enterRoom('general');

      const space1Id = await chatPage.getSpaceId();

      // User A posts root message and opens thread
      const rootMessage = `Thread root in Space 1 ${Date.now()}`;
      const message1 = await roomPage.sendMessage(rootMessage);
      await message1.openThread();
      await roomPage.expectThreadPaneVisible();

      // User A posts initial reply
      const initialReply = `Initial reply from User A ${Date.now()}`;
      await roomPage.postThreadReply(initialReply);
      await roomPage.expectTextInThreadPane(initialReply);
      await roomPage.closeThread();

      // User A creates and switches to Space 2
      await chatPage.goto();
      await chatPage.createSpace(`Space2-${Date.now()}`);
      await chatPage.enterRoom('general');

      const _space2Id = await chatPage.getSpaceId();

      // User B joins Space 1 and posts a thread reply
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(space1Id));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(rootMessage);

        // User B opens thread and posts reply (while User A is in Space 2)
        const message2 = roomPage2.getMessage(rootMessage);
        await message2.openThread();
        await roomPage2.expectThreadPaneVisible();

        const userBReply = `Reply from User B while User A in Space 2 ${Date.now()}`;
        await roomPage2.postThreadReply(userBReply);
        await roomPage2.expectTextInThreadPane(userBReply);

        // User A switches back to Space 1
        await page.goto(routes.space());
        await page.waitForURL(new RegExp(routes.space()));
        await chatPage.enterRoom('general');
        await waitForRoomReady(page, 'general');

        // Wait for the root message to render before checking reply count
        await expect(page.getByText(rootMessage)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // User A should see "2 replies" indicator (initial + User B's)
        await expect(page.getByText('2 replies')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // User A opens thread and sees both replies
        const message1Refetched = roomPage.getMessage(rootMessage);
        await message1Refetched.openThread();
        await roomPage.expectThreadPaneVisible();
        await roomPage.expectTextInThreadPane(initialReply);
        await roomPage.expectTextInThreadPane(userBReply);
      } finally {
        await context2.close();
      }
    });

    test('cache is cleared when entering a new space (no stale data)', async ({
      page,
      chatPage,
      roomPage
    }) => {
      // User creates Space 1 with messages
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace(`ClearCache1-${Date.now()}`);
      await chatPage.enterRoom('general');

      const space1Message = `Space 1 unique message ${Date.now()}`;
      await roomPage.sendMessage(space1Message);

      const space1Id = await chatPage.getSpaceId();

      // User creates Space 2
      await chatPage.goto();
      await chatPage.createSpace(`ClearCache2-${Date.now()}`);
      await chatPage.enterRoom('general');

      const space2Message = `Space 2 unique message ${Date.now()}`;
      await roomPage.sendMessage(space2Message);

      // Verify Space 2 shows only its messages
      await roomPage.expectMessageVisible(space2Message);
      await roomPage.expectMessageNotVisible(space1Message);

      // Switch back to Space 1
      await page.goto(routes.space());
      await page.waitForURL(new RegExp(routes.space()));
      await chatPage.enterRoom('general');
      await waitForRoomReady(page, 'general');

      // Verify Space 1 shows only its messages (cache was cleared)
      await roomPage.expectMessageVisible(space1Message);
      await roomPage.expectMessageNotVisible(space2Message);
    });
  });

  test.describe('Thread reply count updates', () => {
    test('reply count increments correctly when multiple replies are posted', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates space and root message
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();

      const rootMessage = `Multiple replies test ${Date.now()}`;
      await roomPage.sendMessage(rootMessage);

      // User B joins
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(rootMessage);

        // User B opens thread
        const message2 = roomPage2.getMessage(rootMessage);
        await message2.openThread();
        await roomPage2.expectThreadPaneVisible();

        // User B posts first reply
        const reply1 = `First reply ${Date.now()}`;
        await roomPage2.postThreadReply(reply1);
        await roomPage2.expectTextInThreadPane(reply1);

        // Close and verify count shows "1 reply" for User B
        await roomPage2.closeThread();
        await expect(page2.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

        // User B reopens thread and posts second reply
        await message2.openThread();
        await roomPage2.expectThreadPaneVisible();

        const reply2 = `Second reply ${Date.now()}`;
        await roomPage2.postThreadReply(reply2);
        await roomPage2.expectTextInThreadPane(reply2);

        await roomPage2.closeThread();

        // User B should see "2 replies"
        await expect(page2.getByText('2 replies')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

        // User A should also see "2 replies" (updates came via WebSocket)
        await expect(page.getByText('2 replies')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      } finally {
        await context2.close();
      }
    });
  });
});
