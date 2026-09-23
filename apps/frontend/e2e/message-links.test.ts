import { expect } from '@playwright/test';
import { GetRoomEventsAroundRequest } from '@chatto/api-types/api/v1/room_timeline_pb';
import { seedData, loginSeededUser } from './fixtures/seed';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  postMessageViaConnect,
  postReplyViaConnect,
  postThreadReplyViaConnect,
  getIdsFromUrlViaConnect
} from './fixtures/connectHelpers';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';
import { MessageComponent } from './pages/MessageComponent';
import { withServerUser } from './fixtures/serverUser';

test.describe('Message links', () => {
  test.describe.configure({ timeout: 60_000 });

  test('thread message preview stays mounted across local and remote reactions', async ({
    page,
    chatPage,
    roomPage,
    serverURL,
    browser
  }, testInfo) => {
    const errors: string[] = [];
    const consoleErrors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    page.on('console', (message) => {
      if (message.type() === 'error') {
        consoleErrors.push(`${message.text()} (${message.location().url})`);
      }
    });
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const { roomId } = await getIdsFromUrlViaConnect(page);
    const targetBody = `Stable preview target - ${Date.now()}`;
    const targetId = await postMessageViaConnect(page, roomId, targetBody);
    const rootId = await postMessageViaConnect(page, roomId, 'Preview reaction thread');
    const linkedId = await postThreadReplyViaConnect(
      page,
      roomId,
      `${serverURL}${routes.messageLink(roomId, targetId)}`,
      rootId
    );
    const otherId = await postThreadReplyViaConnect(page, roomId, 'Another thread reply', rootId);
    await page.goto(routes.thread(roomId, rootId));
    const preview = roomPage.threadPane.getByTestId('message-preview-card');
    await expect(preview).toContainText(targetBody);
    await page.evaluate(() => document.fonts.ready.then(() => undefined));
    const card = await preview.elementHandle();
    const originalHeight = await preview.evaluate((node) => node.clientHeight);
    const previewRequests: string[] = [];
    page.on('request', (request) => {
      if (
        request.url().endsWith('/GetRoomEventsAround') &&
        GetRoomEventsAroundRequest.fromBinary(request.postDataBuffer()!).eventId === targetId
      ) {
        previewRequests.push(request.url());
      }
    });
    const assertStablePreview = async () => {
      expect(await card!.evaluate((node) => node.isConnected)).toBe(true);
      await expect(preview).toContainText(targetBody);
      expect(await preview.evaluate((node) => node.clientHeight)).toBe(originalHeight);
      expect(previewRequests).toEqual([]);
    };
    const message = (id: string) =>
      new MessageComponent(
        page,
        roomPage.threadPane.locator(`[role="article"][data-event-id="${id}"]`)
      );

    await withServerUser(browser, serverURL, async ({ page: remotePage, roomPage: remoteRoom }) => {
      await remotePage.goto(routes.thread(roomId, rootId));
      await expect(remoteRoom.threadPane.getByTestId('message-preview-card')).toContainText(
        targetBody
      );
      for (const id of [linkedId, otherId]) {
        const localMessage = message(id);
        const remoteMessage = new MessageComponent(
          remotePage,
          remoteRoom.threadPane.locator(`[role="article"][data-event-id="${id}"]`)
        );
        await localMessage.reactViaToolbar('👍');
        await remoteMessage.expectReaction('👍', 1);
        await assertStablePreview();
        await localMessage.toggleReaction('👍');
        await remoteMessage.expectNoReaction('👍');
        await assertStablePreview();

        await remoteMessage.reactViaToolbar('👍');
        await localMessage.expectReaction('👍', 1);
        await assertStablePreview();
        await remoteMessage.toggleReaction('👍');
        await localMessage.expectNoReaction('👍');
        await assertStablePreview();
      }
    });
    // Retain browser resource errors as evidence. Timeline reconciliation can
    // request anchors outside a timeline and receive handled 404 responses.
    await testInfo.attach('browser-console-errors', {
      body: JSON.stringify(consoleErrors, null, 2),
      contentType: 'application/json'
    });
    expect(errors).toEqual([]);
  });

  for (const entry of ['direct URL', 'body link', 'preview card'] as const) {
    test(`opening a root through a ${entry} opens and highlights its attached thread`, async ({
      page,
      chatPage,
      roomPage,
      serverURL
    }) => {
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      page.on('console', (message) => {
        if (message.type() === 'error') errors.push(message.text());
      });
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.enterRoom('general');

      const { roomId } = await getIdsFromUrlViaConnect(page);
      const rootBody = `Linked thread root - ${Date.now()}`;
      const rootId = await postMessageViaConnect(page, roomId, rootBody);
      const linkUrl = `${serverURL}${routes.messageLink(roomId, rootId)}`;

      // Create the link before the thread exists. Navigation must use current data.
      await roomPage.sendMessage(`Open this conversation: ${linkUrl}`);
      const linkedMessage = page.locator('[role="article"]', { hasText: linkUrl });
      await expect(linkedMessage.getByTestId('message-preview-card')).toBeVisible();
      const replyBody = `Attached thread reply - ${Date.now()}`;
      await postThreadReplyViaConnect(page, roomId, replyBody, rootId);

      const pageCount = page.context().pages().length;
      if (entry === 'direct URL') {
        await page.goto(routes.messageLink(roomId, rootId));
      } else if (entry === 'body link') {
        await linkedMessage.locator(`.prose a[href*="/m/${rootId}"]`).click();
      } else {
        await linkedMessage.getByTestId('message-preview-card').click();
      }

      const root = roomPage.threadPane.locator(`[data-event-id="${rootId}"]`);
      await expect(root).toHaveClass(/highlight-flash/, { timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(root).toBeVisible();
      await expect(root).toContainText(rootBody);
      await expect(roomPage.threadPane.getByText(replyBody, { exact: true })).toBeVisible();
      await expect(page).toHaveURL(new RegExp(`/chat/-/${roomId}/${rootId}$`));
      expect(page.context().pages()).toHaveLength(pageCount);
      expect(errors).toEqual([]);
    });
  }

  test('navigating to /m/ URL for a room message redirects to the room with highlight', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();
    const targetBody = `Target room message - ${timestamp}`;
    const eventId = await postMessageViaConnect(page, roomId, targetBody);

    // Navigate directly to the /m/ URL
    await page.goto(routes.messageLink(roomId, eventId));

    // Wait for the client-side redirect to the room URL (goto replaceState)
    await expect(async () => {
      expect(page.url()).not.toContain('/m/');
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The target message should be visible
    await expect(page.getByText(targetBody)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" should NOT appear — the linked message is already at
    // the end of the conversation, so we're already at the present.
    await expect(async () => {
      await expect(page.getByTestId('jump-to-present')).toHaveCount(0);
    }).toPass({
      timeout: TIMEOUTS.POLLING_EXTENDED,
      intervals: [...POLLING_INTERVALS]
    });
  });

  test('navigating to /m/ URL for a thread reply opens the thread pane', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Post root message + thread reply
    const rootBody = `Thread root - ${timestamp}`;
    const rootEventId = await postMessageViaConnect(page, roomId, rootBody);

    const replyBody = `Thread reply - ${timestamp}`;
    const replyEventId = await postThreadReplyViaConnect(page, roomId, replyBody, rootEventId);

    // Navigate directly to the reply's /m/ URL
    await page.goto(routes.messageLink(roomId, replyEventId));

    // Wait for the client-side redirect to the thread URL
    await expect(async () => {
      expect(page.url()).not.toContain('/m/');
      expect(page.url()).toContain(rootEventId);
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Thread pane should be open
    await expect(roomPage.threadPane).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The reply should be visible in the thread pane
    await expect(roomPage.threadPane.getByText(replyBody)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });

  test('message link pasted in a posted message shows a preview card', async ({
    page,
    chatPage,
    roomPage,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Post the target message
    const targetBody = `Preview target - ${timestamp}`;
    const targetEventId = await postMessageViaConnect(page, roomId, targetBody);

    // Post a message containing the target's message link URL
    const linkUrl = `${serverURL}${routes.messageLink(roomId, targetEventId)}`;
    await roomPage.sendMessage(linkUrl);

    // Wait for the embedded preview card to appear
    const previewCard = page.getByTestId('message-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Preview should contain the target message body
    await expect(previewCard).toContainText(targetBody);
  });

  test('message link preview works for image-only messages', async ({
    page,
    chatPage,
    roomPage,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);

    // Post an image-only message (no body text)
    const imageMessage = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');
    const imageEventId = await imageMessage.getEventId();
    expect(imageEventId).toBeTruthy();

    // Post a message containing the image message's link
    const linkUrl = `${serverURL}${routes.messageLink(roomId, imageEventId!)}`;
    await roomPage.sendMessage(linkUrl);

    // The preview card should appear for the image-only message
    const previewCard = page.getByTestId('message-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Preview should show attachment info (image indicator)
    await expect(previewCard).toContainText('Image');
  });

  test('Jump to Present dismisses after jumping to old message and returning', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    const scene = await seedData(page.request, { seed: 42, users: 1, rooms: 1, messages: 61 });
    await loginSeededUser(page.request, scene.users[0]);
    await chatPage.goto();
    await chatPage.enterRoom(scene.rooms[0].name);
    const roomId = scene.rooms[0].id;
    const targetBody = scene.messages[0].body;
    const targetEventId = scene.messages[0].id;
    const timestamp = Date.now();

    // Post a reply referencing the old target (same pattern as jump-to-message tests)
    const replyBody = `Reply to old target - ${timestamp}`;
    await postReplyViaConnect(page, roomId, replyBody, targetEventId);

    // Reload for clean state, wait for reply to be visible
    await page.reload();
    await page.waitForURL(routes.patterns.anyRoomWithQuery);
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Jump to the old message via the reply link
    const replyAttribution = page
      .locator('[role="article"]', { hasText: replyBody })
      .getByTestId('reply-attribution');
    await replyAttribution.click({ position: { x: 8, y: 8 } });

    // The old target should be visible after jump
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" SHOULD appear (we jumped to an old message)
    await expect(page.getByTestId('jump-to-present')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Activate "Jump to Present" to return to the latest messages. The floating
    // button can sit over a moving scroll layer, so avoid pointer interception
    // from timeline content while still exercising the button's click handler.
    await page.getByTestId('jump-to-present').evaluate((button: HTMLElement) => button.click());

    // The latest filler should become visible
    await expect(page.getByText(scene.messages[60].body)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" button should disappear after returning to present
    await expect(page.getByTestId('jump-to-present')).not.toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('clicking a message link in body navigates in-app without opening a new window', async ({
    page,
    chatPage,
    roomPage,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Post the target message
    const targetBody = `Navigation target - ${timestamp}`;
    const targetEventId = await postMessageViaConnect(page, roomId, targetBody);

    // Post a message containing the message link
    const linkUrl = `${serverURL}${routes.messageLink(roomId, targetEventId)}`;
    await roomPage.sendMessage(`Go to ${linkUrl}`);

    // Wait for the link to render in the message body (inside .prose, not the preview card)
    const message = page.locator('[role="article"]', { hasText: linkUrl });
    const link = message.locator(`.prose a[href*="/m/${targetEventId}"]`);
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Count pages (tabs) before clicking
    const pageCountBefore = page.context().pages().length;

    // Click the link
    await link.click();

    // Should navigate within the same tab — no new pages opened
    expect(page.context().pages().length).toBe(pageCountBefore);

    // URL should have changed (redirect from /m/ route)
    await page.waitForURL(routes.patterns.anyRoomWithQuery, {
      timeout: TIMEOUTS.UI_STANDARD
    });

    // The target message should be visible. The same text can also appear in
    // the link preview card, so avoid a strict locator over all matching <p>s.
    await expect(
      page.locator('[role="article"] .prose p', { hasText: targetBody }).first()
    ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  });
});
