import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { TIMEOUTS } from './constants';

/**
 * Player rendering for audio/image/video/generic attachments is unit-tested
 * in MessageAttachments.svelte.spec.ts. The upload pipeline is exercised by
 * the image-attachment cases in messages.test.ts. This e2e covers only the
 * subscription path: that an audio attachment posted by one user shows up in
 * a second user's room via the real-time event stream.
 */

test.describe('audio player', () => {
  test('second user sees audio player via real-time subscription', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    // Set up a second user
    await withServerUser(
      browser!,
      serverURL,
      async ({ chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');

        // User 1 uploads and sends an audio file
        await roomPage.fileInput.setInputFiles('e2e/fixtures/test-audio.mp3');
        await roomPage.messageInput.press('Control+Enter');

        // User 1 sees the audio player
        await expect(roomPage.audioPlayer).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

        // User 2 also sees the audio player via real-time subscription
        await expect(roomPage2.audioPlayer).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      }
    );
  });
});
