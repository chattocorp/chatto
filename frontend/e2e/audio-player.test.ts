import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('audio player', () => {
	test('uploaded audio file renders inline audio player', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		await chatPage.enterRoom('general');

		// Upload a test audio file
		await roomPage.fileInput.setInputFiles('e2e/fixtures/test-audio.mp3');

		// Audio preview in composer shows music note icon
		await expect(roomPage.audioAttachmentPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

		// Send the message
		await roomPage.messageInput.press('Enter');

		// Wait for preview to clear (message sent)
		await expect(roomPage.audioAttachmentPreview).not.toBeVisible({
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});

		// Audio player should appear in the message
		await expect(roomPage.audioPlayer).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

		// Verify the audio element has a src attribute (pointing to the attachment URL)
		await expect(roomPage.audioPlayer).toHaveAttribute('src', /\/assets\/space\//);

		// Verify the filename is shown next to the player
		// Scope to <span> to avoid matching the <a> fallback inside <audio>
		await expect(page.locator('span', { hasText: 'test-audio.mp3' })).toBeVisible();
	});

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
