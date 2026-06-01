import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test.use({
  serverOptions: {
    env: {
      CHATTO_VIDEO_ENABLED: 'false'
    }
  }
});

test.describe('video uploads disabled', () => {
  test('file picker rejects video attachments with a toast', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await expect(roomPage.fileInput).toHaveAttribute('accept', 'image/*,audio/*');
    await roomPage.fileInput.setInputFiles('e2e/fixtures/test-video.mp4');

    await expect(page.getByText('Video uploads are disabled on this server.')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(roomPage.videoAttachmentPreview).not.toBeVisible();
  });

  test('drop zone rejects video attachments with a toast', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.simulateFileDrop('e2e/fixtures/test-video.mp4');

    await expect(page.getByText('Video uploads are disabled on this server.')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(roomPage.videoAttachmentPreview).not.toBeVisible();
  });
});
