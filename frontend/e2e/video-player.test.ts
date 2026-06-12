import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

// Video processing (ffmpeg transcode) can take up to 45s for small test videos.
const VIDEO_PROCESSING_TIMEOUT = 45_000;

test.use({ serverOptions: { env: { CHATTO_VIDEO_ENABLED: 'true' } } });

test.describe('video player @ffmpeg', () => {
	test.setTimeout(90_000);

	test('uploaded video renders Vidstack player without settings menu', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		// Track JS errors so that subscription callback failures are caught.
		const consoleErrors: string[] = [];
		const pageErrors: string[] = [];
		page.on('console', (msg) => {
			if (msg.type() === 'error') consoleErrors.push(msg.text());
		});
		page.on('pageerror', (err) => pageErrors.push(err.message));

		await createAndLoginTestUser(page);
		await chatPage.goto();
		const testSpaceName = await chatPage.createSpace();
		await chatPage.enterRoom('general');

		// Set up a second user who will observe the real-time processing event.
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();
		const chatPage2 = new ChatPage(page2);
		const roomPage2 = new RoomPage(page2);

		try {
			await createAndLoginTestUser(page2);
			await chatPage2.goto();

			// User 2 joins the space via Explore, then enters the room.
			const explorePage2 = new ExplorePage(page2);
			await page2.goto(routes.spaces);
			await page2.waitForURL(routes.patterns.spaceOrRoom);
			await explorePage2.joinSpace(testSpaceName);
			await chatPage2.enterRoom('general');

			// Upload a small test video
			await roomPage.fileInput.setInputFiles('e2e/fixtures/test-video.mp4');

			// Video preview in composer shows a thumbnail frame, via data-testid.
			await expect(roomPage.videoAttachmentPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

			// Send the message
			await roomPage.messageInput.press('Enter');

			// Wait for preview to clear (message sent)
			await expect(roomPage.videoAttachmentPreview).not.toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

			// User 1: The Vidstack <media-player> should appear once video processing
			// completes and the custom elements are registered.
			await expect(roomPage.mediaPlayer).toBeVisible({ timeout: VIDEO_PROCESSING_TIMEOUT });

			// Verify Vidstack rendered its default video layout with controls.
			await expect(roomPage.mediaControls).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

			// The settings menu should be hidden (via CSS).
			if ((await roomPage.videoSettingsMenu.count()) > 0) {
				const computedDisplay = await roomPage.videoSettingsMenu.evaluate(
					(el) => window.getComputedStyle(el).display
				);
				expect(computedDisplay).toBe('none');
			}

			// User 2: the asset processing completion event must also be delivered
			// via the subscription so that the second user sees the player without reloading.
			await expect(roomPage2.mediaPlayer).toBeVisible({ timeout: VIDEO_PROCESSING_TIMEOUT });

			// Filter for critical errors (ignore noise like favicon 404s)
			const criticalErrors = [
				...consoleErrors.filter(
					(e) =>
						e.includes('lifecycle_outside_component') ||
						e.includes('Cannot read properties of undefined')
				),
				...pageErrors.filter(
					(e) =>
						e.includes('lifecycle_outside_component') ||
						e.includes('Cannot read properties of undefined')
				)
			];
			expect(criticalErrors).toEqual([]);
		} finally {
			await context2.close();
		}
	});

	test('a video that fails to process shows the failure indicator (both users)', async ({
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

		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();
		const chatPage2 = new ChatPage(page2);

		try {
			await createAndLoginTestUser(page2);
			await chatPage2.goto();
			const explorePage2 = new ExplorePage(page2);
			await page2.goto(routes.spaces);
			await page2.waitForURL(routes.patterns.spaceOrRoom);
			await explorePage2.joinSpace(testSpaceName);
			await chatPage2.enterRoom('general');

			// Upload bytes that claim to be a video but aren't — ffprobe rejects
			// them, so the worker emits a PROCESSING_FAILED outcome. This drives
			// the failure branch the success tests never exercise.
			await roomPage.fileInput.setInputFiles({
				name: 'broken.mp4',
				mimeType: 'video/mp4',
				buffer: Buffer.from('this is not a valid video file')
			});
			await expect(roomPage.videoAttachmentPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

			await roomPage.messageInput.press('Enter');
			await expect(roomPage.videoAttachmentPreview).not.toBeVisible({
				timeout: TIMEOUTS.COMPLEX_OPERATION
			});

			// User 1 (poster): the failure message renders once the worker gives up.
			await expect(page.getByText('Video processing failed')).toBeVisible({
				timeout: VIDEO_PROCESSING_TIMEOUT
			});

			// User 2: the AssetProcessingFailed event must also arrive over the
			// subscription so the failure shows without a reload — the live
			// failure path that carries the owning message id.
			await expect(page2.getByText('Video processing failed')).toBeVisible({
				timeout: VIDEO_PROCESSING_TIMEOUT
			});

			// A Vidstack player must NOT render for a failed video.
			await expect(roomPage.mediaPlayer).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		} finally {
			await context2.close();
		}
	});
});
