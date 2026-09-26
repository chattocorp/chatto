import { expect, test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { appPreferencesComposer } from './routes';

// Touch emulation replaces the mouse. Set Chromium's available devices to
// coarse + fine, with a fine primary pointer and hover, to test actual CSS.
test.use({
  launchOptions: {
    args: [
      '--blink-settings=availablePointerTypes=6,primaryPointerType=4,availableHoverTypes=2,primaryHoverType=2'
    ]
  },
  viewport: { width: 1024, height: 800 }
});

test('keeps touch presentation and mouse interaction on a hybrid device', async ({
  page,
  chatPage
}) => {
  await createAndLoginTestUser(page);
  expect(
    await page.evaluate(() => ({
      coarse: matchMedia('(any-pointer: coarse)').matches,
      fine: matchMedia('(any-pointer: fine)').matches,
      primaryFine: matchMedia('(pointer: fine)').matches,
      hover: matchMedia('(any-hover: hover)').matches
    }))
  ).toEqual({ coarse: true, fine: true, primaryFine: true, hover: true });
  // The shared account fixture opts into modifier-enter for other tests.
  await page.goto(appPreferencesComposer);
  await page.getByRole('radio', { name: /^Return/ }).click();
  await chatPage.goto();
  const room = await chatPage.enterRoom('general');
  // Enter must still submit when the primary pointer is a mouse.
  await room.waitForInputEditable();
  await room.messageInput.fill('Hybrid input presentation test');
  await room.messageInput.press('Enter');
  const message = room.getMessage('Hybrid input presentation test');
  await expect(message.locator).toBeVisible();
  const frame = page.getByTestId('app-frame');
  const composer = page.getByTestId('composer-input-surface');
  const send = composer.getByRole('button', { name: 'Send message', exact: true });
  for (const width of [320, 500, 767, 768, 1024, 500]) {
    await page.setViewportSize({ width, height: 800 });
    await expect(frame).toHaveCSS('border-radius', width < 768 ? '0px' : '16px');
    await expect(send).toHaveCSS('height', width < 768 ? '44px' : '28px');
    await message.locator.hover();
    await expect(message.hoverToolbar).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
    await page.getByRole('button', { name: 'About Chatto', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    if (width < 768) await expect(dialog).toHaveClass(/bottom-sheet/);
    else await expect(dialog).not.toHaveClass(/bottom-sheet/);
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'About Chatto', exact: true })).toBeFocused();
  }
});
