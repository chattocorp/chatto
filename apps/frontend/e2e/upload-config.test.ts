import { expect } from '@playwright/test';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { createAndLoginTestUser } from './fixtures/testUser';

test.use({
  serverOptions: {
    env: {
      CHATTO_CORE_ASSETS_MAX_UPLOAD_SIZE: '1KB',
      CHATTO_VIDEO_ENABLED: 'false'
    }
  }
});

test.describe('upload configuration', () => {
  test('server upload size limit reaches the composer', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.fileInput.setInputFiles('e2e/fixtures/brighton.jpg');

    await expect(page.getByText('too large')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(roomPage.attachmentPreview).not.toBeVisible();
  });

  test('server video-disabled setting reaches the composer', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await expect(roomPage.fileInput).toHaveAttribute('accept', 'image/*,audio/*');
    await roomPage.fileInput.setInputFiles('e2e/fixtures/test-video.mp4');

    await expect(page.getByText('Video uploads are disabled on this server.')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(roomPage.videoAttachmentPreview).not.toBeVisible();
  });
});
