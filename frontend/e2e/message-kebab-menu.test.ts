import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test.describe('Message hover toolbar', () => {
  test('toolbar appears on hover with reaction and action buttons', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    await page.mouse.move(0, 0);
    await expect(message.hoverToolbar).not.toBeVisible();

    await message.locator.hover();
    await expect(message.hoverToolbar).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(message.hoverToolbar.getByLabel('React with 👍')).toBeVisible();
    await expect(message.hoverToolbar.getByLabel('React with ❤️')).toBeVisible();
    await expect(message.hoverToolbar.getByLabel('More actions')).toBeVisible();
  });

  test('can edit message directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar edit test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    await message.editViaToolbar();

    await roomPage.expectEditModeActive();
    await expect(roomPage.composer).toHaveText(testMessage);

    await page.keyboard.press('Escape');
  });

  test('can reply in thread directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar reply test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    await message.replyViaToolbar();

    await roomPage.expectThreadPaneVisible();
  });
});
