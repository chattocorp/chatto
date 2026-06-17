import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  postMessageViaAPI,
  postMessagesViaAPI,
  postReplyViaAPI,
  postThreadReplyViaAPI,
  getIdsFromUrl
} from './fixtures/graphqlHelpers';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';

test.describe('Message links', () => {
  test.describe.configure({ timeout: 60_000 });

  test('navigating to /m/ URL for a room message redirects to the room with highlight', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();
    const targetBody = `Target room message - ${timestamp}`;
    const eventId = await postMessageViaAPI(page, roomId, targetBody);

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

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post root message + thread reply
    const rootBody = `Thread root - ${timestamp}`;
    const rootEventId = await postMessageViaAPI(page, roomId, rootBody);

    const replyBody = `Thread reply - ${timestamp}`;
    const replyEventId = await postThreadReplyViaAPI(
      page,
      roomId,
      replyBody,
      rootEventId
    );

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

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post the target message
    const targetBody = `Preview target - ${timestamp}`;
    const targetEventId = await postMessageViaAPI(page, roomId, targetBody);

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

    const { roomId } = await getIdsFromUrl(page);

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
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post an old target message, then fill to push it out of view
    const targetBody = `Old target - ${timestamp}`;
    const targetEventId = await postMessageViaAPI(page, roomId, targetBody);

    const fillerMessages = Array.from({ length: 60 }, (_, i) => `Filler ${i + 1} - ${timestamp}`);
    await postMessagesViaAPI(page, roomId, fillerMessages);

    // Post a reply referencing the old target (same pattern as jump-to-message tests)
    const replyBody = `Reply to old target - ${timestamp}`;
    await postReplyViaAPI(page, roomId, replyBody, targetEventId);

    // Reload for clean state, wait for reply to be visible
    await page.reload();
    await page.waitForURL(routes.patterns.anyRoomWithQuery);
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Jump to the old message via the reply link
    const replyAttribution = page
      .locator('[role="article"]', { hasText: replyBody })
      .getByTestId('reply-attribution');
    await replyAttribution.getByText('in reply to').click();

    // The old target should be visible after jump
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" SHOULD appear (we jumped to an old message)
    await expect(page.getByTestId('jump-to-present')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Click "Jump to Present" to return to the latest messages
    await page.getByTestId('jump-to-present').click();

    // The latest filler should become visible
    await expect(page.getByText(`Filler 60 - ${timestamp}`)).toBeVisible({
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

    const { roomId } = await getIdsFromUrl(page);
    const timestamp = Date.now();

    // Post the target message
    const targetBody = `Navigation target - ${timestamp}`;
    const targetEventId = await postMessageViaAPI(page, roomId, targetBody);

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

    // The target message should be visible (scope to <p> to avoid matching the preview card snippet)
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });
});
