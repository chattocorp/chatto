import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

test.describe('drag and drop image upload', () => {
  test('can send message after dropping image', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.dropFile('e2e/fixtures/brighton.jpg');

    const testMessage = `Drag drop test ${Date.now()}`;
    await roomPage.messageInput.fill(testMessage);
    await roomPage.messageInput.press('Enter');

    await expect(roomPage.attachmentPreview).not.toBeVisible();
    await roomPage.expectMessageVisible(testMessage);
    await expect(roomPage.attachmentImage).toBeVisible();
  });

  test('drops an image into the thread composer', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const rootText = `Thread drag drop root ${Date.now()}`;
    const rootMessage = await roomPage.sendMessage(rootText);
    await rootMessage.openThread();
    await roomPage.expectThreadPaneVisible();

    await roomPage.dropFileInThread('e2e/fixtures/brighton.jpg');
    await expect(roomPage.roomAttachmentPreview).toHaveCount(0);

    const replyText = `Thread drag drop reply ${Date.now()}`;
    await roomPage.threadReplyInput.fill(replyText);
    await roomPage.threadReplyInput.press('Enter');

    await expect(roomPage.threadAttachmentPreview).toHaveCount(0);
    const reply = roomPage.getMessage(replyText);
    await expect(reply.locator).toBeVisible();
    await expect(reply.attachmentImage).toBeVisible();
  });
});
