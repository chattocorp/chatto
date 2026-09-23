import { expect, test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

for (const input of ['mouse', 'touch'] as const) {
	const touch = input !== 'mouse';
	test.describe(`App presentation with ${input} input`, () => {
		test.use({
			hasTouch: input === 'touch',
			viewport: { width: 500, height: 800 }
		});

		test('fits the frame, drawers, and task dialog across viewport sizes', async ({ page, chatPage }) => {
			const errors: string[] = [];
			page.on('pageerror', (error) => errors.push(error.message));
			await createAndLoginTestUser(page);
			expect(await page.evaluate(() => ({
				coarse: matchMedia('(any-pointer: coarse)').matches,
				fine: matchMedia('(any-pointer: fine)').matches,
				hover: matchMedia('(any-hover: hover)').matches
			}))).toEqual({ coarse: touch, fine: input !== 'touch', hover: input !== 'touch' });
			await chatPage.goto();
			const toggle = page.getByRole('button', { name: 'Toggle sidebar', exact: true });
			await expect(chatPage.roomList).not.toBeVisible();
			await toggle.click();
			await chatPage.enterRoom('general');
			const header = page.locator('.app-header');
			const frame = page.getByTestId('app-frame');
			for (const width of [375, 500, 767, 768, 1024, 500]) {
				await page.setViewportSize({ width, height: 800 });
				const mobile = touch && width < 768;
				await expect(frame).toHaveCSS('border-radius', mobile ? '0px' : '16px');
				await expect(header).toHaveCSS('height', mobile ? '56px' : '44px');
				await expect.poll(async () => (await frame.boundingBox())?.x).toBe(mobile ? 0 : 12);
				expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
				if (width < 768) {
					await expect(chatPage.roomList).not.toBeVisible();
					await toggle.click();
					await expect(chatPage.roomList).toBeVisible();
					const headerBox = await header.boundingBox();
					const frameBox = (await frame.boundingBox())!;
					const bottom = touch ? 800 : frameBox.y + frameBox.height;
					for (const id of ['mobile-sidebar-panel', 'server-sidebar', 'mobile-sidebar-backdrop']) {
						const panel = page.getByTestId(id);
						await expect.poll(async () => (await panel.boundingBox())?.y)
							.toBe(headerBox!.y + headerBox!.height);
						await expect.poll(async () => {
							const box = await panel.boundingBox();
							return box!.y + box!.height;
						}).toBe(bottom);
					}
					await expect.poll(async () =>
						(await page.getByTestId('mobile-sidebar-panel').boundingBox())?.x
					).toBe(frameBox.x);
					const gutter = (await page.getByTestId('mobile-sidebar-panel').boundingBox())!;
					const sidebar = (await page.getByTestId('server-sidebar').boundingBox())!;
					expect(gutter.x).toBe(frameBox.x);
					expect(gutter.width + sidebar.width).toBe(frameBox.width);
					expect(sidebar.x + sidebar.width).toBe(frameBox.x + frameBox.width);
					if (!touch) {
						await expect(frame).toHaveCSS('contain', 'layout paint');
						// Freeze a real pointer drag halfway. The drawer must stay inside
						// the rounded work plane while it moves, leaving the rim exposed.
						await page.mouse.move(width - 45, 160);
						await page.mouse.down();
						await page.mouse.move(width / 2, 160, { steps: 4 });
						expect(await page.evaluate(() =>
							document.elementFromPoint(6, 160)?.closest('[data-app-sidebar]') != null
						)).toBe(false);
						await page.mouse.move(20, 160, { steps: 4 });
						await page.mouse.up();
						await expect(chatPage.roomList).not.toBeVisible();
						await toggle.click();
						await expect(chatPage.roomList).toBeVisible();
					}
					await toggle.click();
					await expect(chatPage.roomList).not.toBeVisible();
				}
				await page.getByRole('button', { name: 'About Chatto', exact: true }).click();
				const dialog = page.getByRole('dialog');
				await expect(dialog).toBeVisible();
				if (mobile) await expect(dialog).toHaveClass(/bottom-sheet/);
				else await expect(dialog).not.toHaveClass(/bottom-sheet/);
				expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(width);
				await dialog.getByRole('button', { name: 'Close', exact: true }).first().click();
				await expect(dialog).not.toBeVisible();
			}
			// Change hardware capability with the drawer open: frame containment,
			// safe-area positioning, and keyboard focus must update in place.
			await toggle.click();
			const roomLink = chatPage.getRoomLink('general');
			await roomLink.focus();
			const cdp = await page.context().newCDPSession(page);
			for (const coarse of [!touch, touch]) {
				await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: coarse });
				await expect(frame).toHaveCSS('border-radius', coarse ? '0px' : '16px');
				await expect(roomLink).toBeFocused();
				await expect.poll(async () =>
					(await page.getByTestId('mobile-sidebar-panel').boundingBox())?.x
				).toBe(coarse ? 0 : 12);
				const side = (await page.getByTestId('server-sidebar').boundingBox())!;
				expect(side.y).toBe(coarse ? 56 : 44);
			}
			await cdp.detach();
			await toggle.click();
			await expect(chatPage.roomList).not.toBeVisible();
			expect(errors).toEqual([]);
		});
	});
}
