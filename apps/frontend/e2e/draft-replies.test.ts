import { CreateMessageRequest } from '@chatto/api-types/api/v1/messages_pb';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  getIdsFromUrlViaConnect,
  postMessageViaConnect,
  postThreadReplyViaConnect
} from './fixtures/connectHelpers';
import { MessageComponent } from './pages/MessageComponent';

for (const destination of ['room', 'thread'] as const) {
  test(`restores a ${destination} draft reply target after leaving`, async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const { roomId } = await getIdsFromUrlViaConnect(page);
    const rootBody = 'Draft reply thread';
    const rootId = await postMessageViaConnect(page, roomId, rootBody);
    const targetBody = 'Original reply target';
    const inThread = destination === 'thread';
    const targetId = inThread
      ? await postThreadReplyViaConnect(page, roomId, targetBody, rootId)
      : rootId;
    if (inThread) await roomPage.getMessage(rootBody).openThread();
    const pane = inThread ? roomPage.threadPane : roomPage.roomDropZone;
    const input = inThread ? roomPage.threadReplyInput : roomPage.messageInput;
    const excerpt = inThread ? targetBody : rootBody;
    const target = new MessageComponent(
      page,
      pane.locator(`[role="article"][data-event-id="${targetId}"]`)
    );
    await target.replyInRoom();
    await input.fill('Unsent reply draft');
    await expect(pane.getByTestId('reply-indicator')).toContainText(excerpt);

    const leave = async () => {
      if (inThread) await roomPage.closeThread();
      else await page.getByRole('link', { name: 'Overview', exact: true }).click();
      await expect(pane).not.toBeVisible();
    };
    const returnToDraft = async () => {
      if (inThread) await roomPage.getMessage(rootBody).openThread();
      else await chatPage.enterRoom('general');
    };
    await leave();
    await returnToDraft();
    await expect(input).toHaveText('Unsent reply draft');
    await expect(pane.getByTestId('reply-indicator')).toContainText(excerpt);

    const posted = page.waitForRequest((request) => request.url().endsWith('/CreateMessage'));
    await pane.getByRole('button', { name: 'Send: Send message', exact: true }).click();
    const request = CreateMessageRequest.fromBinary((await posted).postDataBuffer()!);
    expect(request.inReplyTo).toBe(targetId);
    expect(request.threadRootEventId).toBe(inThread ? rootId : '');
    await expect(pane.getByTestId('reply-indicator')).toHaveCount(0);
    await leave();
    await returnToDraft();
    await expect(input).toBeEmpty();
    await expect(pane.getByTestId('reply-indicator')).toHaveCount(0);
  });
}
