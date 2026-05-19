import { expect } from '@playwright/test';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, ExplorePage } from './pages';
import * as routes from './routes';

// Video processing (ffmpeg transcode) can take up to 60s for small test files on CI.
const VIDEO_PROCESSING_TIMEOUT = 60_000;

test.describe('animated GIF to video conversion', () => {
	test.setTimeout(120_000);

	test('animated GIF is converted to looping video player', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
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

		try {
			await createAndLoginTestUser(page2);
			await chatPage2.goto();

			const explorePage2 = new ExplorePage(page2);
			await page2.goto(routes.spaces);
			await page2.waitForURL(routes.patterns.spaceOrRoom);
			await explorePage2.joinSpace(testSpaceName);
			await chatPage2.enterRoom('general');

			// Upload an animated GIF (2 frames, 64x64px)
			await roomPage.fileInput.setInputFiles('e2e/fixtures/test-animation.gif');

			// GIF preview appears as an <img> tag (browser renders it as an image)
			await expect(roomPage.attachmentPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

			// Send the message
			await roomPage.messageInput.press('Enter');

			// Wait for preview to clear (message sent)
			await expect(roomPage.attachmentPreview).not.toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

			// The backend detects the animated GIF and routes it through video processing.
			// On fast machines, processing may complete before the subscription delivers
			// the initial event, so the processing indicator may never be visible.
			// Use data-autoloop attribute which is always set on converted GIF videos.
			const gifVideo = page.locator('video[data-autoloop]');
			const failedIndicator = page.getByText('Video processing failed');

			// Wait for either the completed video or a failure message.
			await expect(gifVideo.or(failedIndicator)).toBeVisible({
				timeout: VIDEO_PROCESSING_TIMEOUT
			});

			// If processing failed, surface the error details.
			if (await failedIndicator.isVisible()) {
				const errorDetail = await page.locator('.text-muted\\/70').textContent();
				throw new Error(`Video processing failed: ${errorDetail}`);
			}

			// Verify the video element has correct autoplay/loop attributes.
			await expect(gifVideo).toHaveAttribute('autoplay', '');
			await expect(gifVideo).toHaveAttribute('loop', '');

			// No Vidstack player or controls should be rendered for converted GIFs.
			await expect(roomPage.mediaPlayer).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });

			// User 2: Should also see the converted video via real-time subscription.
			const gifVideo2 = page2.locator('video[data-autoloop]');
			await expect(gifVideo2).toBeVisible({ timeout: VIDEO_PROCESSING_TIMEOUT });

			// Filter for critical errors
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

	test('static GIF renders as image, not video', async ({ page, chatPage, roomPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		await chatPage.enterRoom('general');

		// Upload a static (single-frame) GIF
		await roomPage.fileInput.setInputFiles('e2e/fixtures/test-static.gif');
		await expect(roomPage.attachmentPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

		// Send the message
		await roomPage.messageInput.press('Enter');
		await expect(roomPage.attachmentPreview).not.toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

		// The posted message should show an image, not a video player.
		await expect(roomPage.attachmentImage).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

		// No media player should appear — static GIFs skip video processing.
		await expect(roomPage.mediaPlayer).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
	});
});
