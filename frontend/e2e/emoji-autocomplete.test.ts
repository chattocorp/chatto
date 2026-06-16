import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

/**
 * E2E coverage keeps one full app smoke: a selected emoji can be sent and
 * rendered as a posted message. Local trigger detection, cursor replacement,
 * and popup behavior are covered by MessageComposer, AutocompleteState,
 * AutocompletePopup, and EmojiAutocomplete browser/unit specs.
 */

function getPopup(page: import('@playwright/test').Page) {
  return page.locator('ul').filter({
    has: page.locator('button', { hasText: /^.+ :[^:]+:$/ })
  });
}

test.describe('Emoji autocomplete (composer integration)', () => {
  test('selected emoji round-trips through send and renders in the message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.messageInput.click();
    await roomPage.messageInput.pressSequentially('Hello :wave');

    const popup = getPopup(page);
    await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    await roomPage.messageInput.press('Enter');
    await expect(popup).not.toBeVisible();
    await expect(roomPage.messageInput).toHaveText(/👋/);

    await roomPage.messageInput.press('Enter');
    await expect(page.locator('[role="article"]', { hasText: '👋' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });
});
