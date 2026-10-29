import { expect, test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

// Measure real room actions and activate the added edge of each touch target.
for (const touch of [false, true]) {
  test.describe(`Pane header with ${touch ? 'touch' : 'mouse'} input`, () => {
    test.use({ hasTouch: touch, viewport: { width: 1280, height: 800 } });

    test('fits narrow headers and opens and closes room panels', async ({ page, chatPage }) => {
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.enterRoom('general');

      for (const width of [320, 390, 1024, 1920]) {
        await page.setViewportSize({ width, height: 800 });
        const disclosure = page.getByRole('button', { name: 'Actions for #general', exact: true });
        if (await disclosure.isVisible()) {
          await expect(disclosure).toHaveCSS('width', touch ? '44px' : '40px');
          await expect(disclosure).toHaveCSS('height', touch ? '44px' : '40px');
          if (touch) await disclosure.tap({ position: { x: 22, y: 42 } });
          else await disclosure.click();
          await expect(disclosure).toHaveAttribute('aria-expanded', 'true');
        }

        const files = page.getByRole('button', { name: 'Files', exact: true });
        await expect(files).toBeVisible();
        await expect(files).toHaveCSS('height', touch ? '44px' : '40px');
        expect((await files.boundingBox())!.width).toBeGreaterThanOrEqual(touch ? 44 : 40);
        // The expanded controls must fit without covering the room title.
        const title = chatPage.getRoomHeader('general');
        const titleBox = (await title.boundingBox())!;
        const firstAction = page.getByRole('button', { name: 'Room information', exact: true });
        expect(titleBox.width).toBeGreaterThan(0);
        expect(titleBox.x + titleBox.width).toBeLessThanOrEqual(
          (await firstAction.boundingBox())!.x
        );
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);

        if (touch) await files.tap({ position: { x: 22, y: 42 } });
        else await files.click();
        const close = page.getByRole('button', { name: 'Hide room extras', exact: true });
        await expect(close).toBeVisible();
        await expect(close).toHaveCSS('width', touch ? '44px' : '40px');
        await expect(close).toHaveCSS('height', touch ? '44px' : '40px');
        if (touch) await close.tap({ position: { x: 22, y: 42 } });
        else await close.click();
        await expect(close).not.toBeVisible();
        if (await disclosure.isVisible()) await disclosure.click();
      }
      expect(errors).toEqual([]);
    });
  });
}
