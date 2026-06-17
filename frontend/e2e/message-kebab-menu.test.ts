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

    // Move mouse away from the message so hover state clears
    await page.mouse.move(0, 0);

    // Toolbar should be hidden when not hovering
    await expect(message.hoverToolbar).not.toBeVisible();

    // Hover over the message to reveal the toolbar
    await message.locator.hover();
    await expect(message.hoverToolbar).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Verify toolbar has reaction buttons directly visible (no intermediate popup)
    await expect(message.hoverToolbar.getByLabel('React with 👍')).toBeVisible();
    await expect(message.hoverToolbar.getByLabel('React with ❤️')).toBeVisible();

    // Verify toolbar has action buttons
    await expect(message.hoverToolbar.getByLabel('More actions')).toBeVisible();
  });

  test('can add reaction directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar action test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a reaction directly via the toolbar (no context menu intermediate)
    await message.reactViaToolbar('👍');

    // Verify the reaction appears
    await message.expectReaction('👍', 1);
  });

  test('can edit message directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar edit test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Click edit directly in the toolbar (no context menu intermediate)
    await message.editViaToolbar();

    // Edit mode should be active with the message body pre-filled
    await roomPage.expectEditModeActive();
    await expect(roomPage.composer).toHaveText(testMessage);

    // Cancel the edit
    await page.keyboard.press('Escape');
  });

  test('can reply in thread directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar reply test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Click reply directly in the toolbar (no context menu intermediate)
    await message.replyViaToolbar();

    // Thread pane should open
    await roomPage.expectThreadPaneVisible();
  });
});
