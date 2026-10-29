import { expect, test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';

test.use({ hasTouch: true, viewport: { width: 390, height: 800 } });

for (const help of [false, true]) {
  test(`mobile ${help ? 'shortcut help' : 'quick switcher'} allows header actions on the first tap`, async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const dialog = page.getByRole('dialog', {
      name: help ? 'Keyboard shortcuts' : 'Quick switcher',
      exact: true
    });
    const toggle = page
      .locator('.app-header')
      .getByRole('button', { name: 'Toggle sidebar', exact: true });
    const expanded = await toggle.getAttribute('aria-expanded');
    await page.keyboard.press(help ? 'ControlOrMeta+Shift+k' : 'ControlOrMeta+k');
    await expect(dialog).toBeVisible();
    expect(await dialog.evaluate((node) => node.matches(':modal'))).toBe(false);
    await toggle.tap();
    await expect(dialog).not.toBeVisible();
    await expect(toggle).toHaveAttribute('aria-expanded', expanded === 'true' ? 'false' : 'true');

    await page.keyboard.press(help ? 'ControlOrMeta+Shift+k' : 'ControlOrMeta+k');
    await expect(dialog).toBeVisible();
    await page
      .locator('.app-header')
      .getByRole('link', { name: 'Notifications', exact: true })
      .tap();
    await expect(dialog).not.toBeVisible();
    await expect(page).toHaveURL(/\/chat\/-\/notifications$/);
  });

  test(`mobile ${help ? 'shortcut help' : 'quick switcher'} fills the screen and returns focus`, async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const opener = page.getByRole('button', { name: 'Open quick switcher', exact: true });
    await opener.focus();
    await page.keyboard.press(help ? 'ControlOrMeta+Shift+k' : 'ControlOrMeta+k');
    const dialog = page.getByRole('dialog', {
      name: help ? 'Keyboard shortcuts' : 'Quick switcher',
      exact: true
    });
    await expect(dialog).toBeVisible();
    const search = dialog.getByRole('textbox');
    const back = dialog.getByRole('button', { name: 'Back', exact: true });
    await search.fill(help ? 'Open' : 'general');
    for (const width of [390, 767, 768, 1280, 390]) {
      await page.setViewportSize({ width, height: 800 });
      await expect(search).toHaveValue(help ? 'Open' : 'general');
      await expect(search).toBeFocused();
      if (width < 768) {
        await expect
          .poll(async () => {
            const box = (await dialog.boundingBox())!;
            return [
              Math.round(box.x),
              Math.round(box.y),
              Math.round(box.width),
              Math.round(box.height)
            ];
          })
          .toEqual([0, 56, width, 744]);
        const header = page.locator('.app-header');
        await expect(header).toBeVisible();
        expect((await header.boundingBox())!.y + (await header.boundingBox())!.height).toBe(56);
        expect(
          await dialog.evaluate(
            (element) => getComputedStyle(element, '::backdrop').backgroundColor
          )
        ).toBe('rgba(0, 0, 0, 0)');
        await expect(back).toBeVisible();
        await expect(back).toHaveCSS('width', '44px');
        await expect(back).toHaveCSS('height', '44px');
      } else {
        await expect.poll(() => dialog.evaluate((node) => node.matches(':modal'))).toBe(true);
        await expect(back).not.toBeVisible();
        expect((await dialog.boundingBox())!.width).toBeLessThan(width);
      }
    }
    await page.setViewportSize({ width: 390, height: 240 });
    await page.evaluate(() => document.body.setAttribute('data-keyboard-open', ''));
    await expect(page.locator('.app-header')).toBeVisible();
    await search.fill('');
    const list = help
      ? dialog.locator('.dialog-body')
      : dialog.locator('.command-palette > .menu-section').last();
    await expect
      .poll(() => list.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    await list.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await expect(back).toBeVisible();
    await expect(search).toBeInViewport();
    await page.setViewportSize({ width: 390, height: 800 });
    await page.evaluate(() => document.body.removeAttribute('data-keyboard-open'));
    await back.tap({ position: { x: 22, y: 42 } });
    await expect(dialog).not.toBeVisible();
    await expect(opener).toBeFocused();
    await page.keyboard.press(help ? 'ControlOrMeta+Shift+k' : 'ControlOrMeta+k');
    await expect(dialog).toBeVisible();
    await expect(search).toHaveValue('');
    if (!help) {
      await search.fill('general');
      await dialog.locator('.command-palette-result').filter({ hasText: 'general' }).tap();
      await expect(page.getByRole('heading', { name: '# general', exact: true })).toBeVisible();
    } else {
      await page.keyboard.press('Escape');
    }
    await expect(dialog).not.toBeVisible();
  });
}
