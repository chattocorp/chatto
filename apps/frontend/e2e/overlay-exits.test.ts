import { expect, test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

for (const input of ['mouse', 'touch'] as const) {
  test.describe(`Overlay exits with ${input} input`, () => {
    test.use({
      hasTouch: input === 'touch',
      viewport: { width: 1280, height: 800 }
    });

    test('keeps one narrow exit and preserves desktop controls across resize', async ({
      page,
      chatPage
    }) => {
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      await createAndLoginTestUser(page);
      await chatPage.goto();
      const room = await chatPage.enterRoom('general');
      expect(await page.evaluate(() => matchMedia('(any-pointer: coarse)').matches)).toBe(
        input !== 'mouse'
      );
      const targetSize = input === 'mouse' ? '40px' : '44px';

      for (const width of [390, 768, 1023]) {
        await page.setViewportSize({ width, height: 800 });
        await room.openMembersPanel();
        const overlay = page.getByTestId('room-sidebar-mobile-pane');
        const exit = overlay.getByRole('button', { name: 'Hide room extras', exact: true });
        await expect(exit).toHaveCount(1);
        await expect(exit).toHaveCSS('height', targetSize);
        await expect(exit).toHaveCSS('width', targetSize);
        await expect(exit.locator('[class~="icon-[uil--arrow-left]"]')).toBeVisible();
        await expect(overlay.locator('[class~="icon-[uil--times]"]')).toHaveCount(0);
        const heading = overlay.getByRole('heading', { name: /^Members/ });
        expect((await exit.boundingBox())!.x).toBeLessThan((await heading.boundingBox())!.x);
        if (input === 'touch') await exit.tap({ position: { x: 22, y: 42 } });
        else await exit.click();
        await expect(overlay).not.toBeVisible();
      }

      await page.setViewportSize({ width: 1280, height: 800 });
      await room.openMembersPanel();
      const desktop = page.getByTestId('room-sidebar-desktop-pane');
      const desktopExit = desktop.getByRole('button', { name: 'Hide room extras', exact: true });
      await expect(desktopExit.locator('[class~="icon-[uil--times]"]')).toBeVisible();
      await expect(desktopExit).toHaveCSS('height', targetSize);

      await room.sendMessage('Overlay exit thread');
      await room.getMessage('Overlay exit thread').openThread();
      const thread = page.getByTestId('thread-pane');
      const back = thread.getByRole('button', { name: 'Back to room', exact: true });
      const close = thread.getByRole('button', { name: 'Close thread', exact: true });
      for (const width of [1280, 390, 768, 1023, 1024, 390]) {
        await page.setViewportSize({ width, height: 800 });
        await expect(back).toBeVisible();
        await expect(back).toHaveCSS('height', targetSize);
        await expect(back).toHaveCSS('width', targetSize);
        if (width < 1024) await expect(close).not.toBeVisible();
        else await expect(close).toBeVisible();
      }
      if (input === 'touch') await back.tap({ position: { x: 22, y: 42 } });
      else await back.click();
      await expect(thread).not.toBeVisible();
      await room.expectThreadRouteClosed();
      expect(errors).toEqual([]);
    });
  });
}
