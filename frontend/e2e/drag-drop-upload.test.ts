import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

test.describe('drag and drop image upload', () => {
  test('can send message after dropping image', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.dropFile('e2e/fixtures/brighton.jpg');

    const testMessage = `Drag drop test ${Date.now()}`;
    await roomPage.messageInput.fill(testMessage);
    await roomPage.messageInput.press('Enter');

    await expect(roomPage.attachmentPreview).not.toBeVisible();
    await roomPage.expectMessageVisible(testMessage);
    await expect(roomPage.attachmentImage).toBeVisible();
  });
});
