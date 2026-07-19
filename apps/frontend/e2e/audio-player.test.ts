import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { TIMEOUTS } from './constants';
import { MessageComponent } from './pages/MessageComponent';

/**
 * Player rendering for audio/image/video/generic attachments is unit-tested
 * in MessageAttachments.svelte.spec.ts. The upload pipeline is exercised by
 * the image-attachment cases in messages.test.ts. This e2e covers the
 * subscription path and verifies that reaction updates do not interrupt
 * active playback by replacing the media element.
 */

test.describe('audio player', () => {
  test('preserves playback across reactions and syncs to a second user', async ({
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
        await roomPage.messageInput.press('Enter');

        // User 1 sees the audio player
        await expect(roomPage.audioPlayer).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

        const audioMessageLocator = page
          .locator('[role="article"]')
          .filter({ has: roomPage.audioPlayer });
        const audioMessage = new MessageComponent(page, audioMessageLocator);
        const inlineAudio = audioMessageLocator.getByTestId('audio-player');
        await inlineAudio.evaluate(async (audio) => {
          audio.dataset.reactionPlaybackMarker = 'preserve-me';
          audio.muted = true;
          audio.loop = true;
          await audio.play();
        });
        await expect(inlineAudio).not.toHaveJSProperty('paused', true);

        await audioMessage.reactViaToolbar('👍');
        await audioMessage.expectReaction('👍', 1);
        await expect(inlineAudio).toHaveAttribute('data-reaction-playback-marker', 'preserve-me');
        await expect(inlineAudio).not.toHaveJSProperty('paused', true);

        await audioMessage.toggleReaction('👍');
        await audioMessage.expectNoReaction('👍');
        await expect(inlineAudio).toHaveAttribute('data-reaction-playback-marker', 'preserve-me');
        await expect(inlineAudio).not.toHaveJSProperty('paused', true);

        // User 2 also sees the audio player via real-time subscription
        await expect(roomPage2.audioPlayer).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      }
    );
  });
});
