import { expect } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
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
        await joinSpace(page2);
        await page2.goto(routes.space());

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
        await joinSpace(page2);
        await page2.goto(routes.space());

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
        await joinSpace(page2);
        await page2.goto(routes.space());

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
