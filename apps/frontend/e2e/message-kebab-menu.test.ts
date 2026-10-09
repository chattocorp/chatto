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
    await expect(message.hoverToolbar.locator('[aria-label^="React with "]')).toHaveCount(4);
    for (const emoji of ['👍', '👋', '🤣', '🙏']) {
      await expect(message.hoverToolbar.getByLabel(`React with ${emoji}`)).toBeVisible();
    }
    await expect(message.hoverToolbar.getByLabel('More actions')).toBeVisible();
  });

  test('toolbar appears on hover in a narrow desktop window', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Narrow toolbar test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    await page.setViewportSize({ width: 640, height: 720 });
    await page.mouse.move(0, 0);
    await expect(message.hoverToolbar).not.toBeVisible();

    await message.locator.hover();
    await expect(message.hoverToolbar).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
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

  test('context menu thread reply focuses the composer without root attribution', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const target = await roomPage.sendMessage(`Thread reply target ${Date.now()}`);
    await target.replyInThread();

    await roomPage.expectThreadPaneVisible();
    await expect(roomPage.threadPane.getByTestId('reply-indicator')).not.toBeVisible();
    await expect(roomPage.threadReplyInput).toBeFocused({ timeout: TIMEOUTS.UI_STANDARD });

    const replyBody = `Plain thread reply ${Date.now()}`;
    await roomPage.postThreadReply(replyBody);
    await expect(
      roomPage.getThreadMessage(replyBody).locator.getByTestId('reply-attribution')
    ).toHaveCount(0);
  });
});
