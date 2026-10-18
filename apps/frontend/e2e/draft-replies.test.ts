import { CreateMessageRequest } from '@chatto/api-types/api/v1/messages_pb';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  getIdsFromUrlViaConnect,
  postMessageViaConnect,
  postThreadReplyViaConnect
} from './fixtures/connectHelpers';
import { MessageComponent } from './pages/MessageComponent';

test('restores a thread draft reply target after leaving the thread', async ({
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
  const targetId = await postThreadReplyViaConnect(page, roomId, targetBody, rootId);
  await roomPage.getMessage(rootBody).openThread();
  const target = new MessageComponent(
    page,
    roomPage.threadPane.locator(`[role="article"][data-event-id="${targetId}"]`)
  );
  await target.replyInRoom();
  await roomPage.threadReplyInput.fill('Unsent reply draft');
  await expect(roomPage.threadPane.getByTestId('reply-indicator')).toContainText(targetBody);

  await roomPage.closeThread();
  await roomPage.getMessage(rootBody).openThread();
  await expect(roomPage.threadReplyInput).toHaveText('Unsent reply draft');
  await expect(roomPage.threadPane.getByTestId('reply-indicator')).toContainText(targetBody);

  const posted = page.waitForRequest((request) => request.url().endsWith('/CreateMessage'));
  await roomPage.threadPane.getByRole('button', { name: 'Send message', exact: true }).click();
  const request = CreateMessageRequest.fromBinary((await posted).postDataBuffer()!);
  expect(request.inReplyTo).toBe(targetId);
  expect(request.threadRootEventId).toBe(rootId);
  await expect(roomPage.threadPane.getByTestId('reply-indicator')).toHaveCount(0);
  await roomPage.closeThread();
  await roomPage.getMessage(rootBody).openThread();
  await expect(roomPage.threadReplyInput).toBeEmpty();
  await expect(roomPage.threadPane.getByTestId('reply-indicator')).toHaveCount(0);
});
