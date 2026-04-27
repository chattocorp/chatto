import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

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
    const testSpaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Set up a second user
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();
    const chatPage2 = new ChatPage(page2);
    const roomPage2 = new RoomPage(page2);

    try {
      await createAndLoginTestUser(page2);
      await chatPage2.goto();

      // User 2 joins the space via Explore, then enters the room
      const explorePage2 = new ExplorePage(page2);
      await page2.goto(routes.spaces);
      await page2.waitForURL(routes.spaces);
      await explorePage2.joinSpace(testSpaceName);
      await chatPage2.enterRoom('general');

      // User 1 uploads and sends an audio file
      await roomPage.fileInput.setInputFiles('e2e/fixtures/test-audio.mp3');
      await roomPage.messageInput.press('Enter');

      // User 1 sees the audio player
      await expect(roomPage.audioPlayer).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

      // User 2 also sees the audio player via real-time subscription
      await expect(roomPage2.audioPlayer).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });
});
