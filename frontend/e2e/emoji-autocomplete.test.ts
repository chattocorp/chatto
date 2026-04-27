import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

/**
 * E2E coverage for the TipTap-driven emoji autocomplete *integration*:
 * trigger detection, mid-message position, threshold-based show/hide,
 * and the full send-flow round-trip.
 *
 * Pure popup behavior (search results, keyboard nav, Enter/Tab/click select,
 * Escape close, top-10 cap, exact-match ranking) is covered by the unit specs
 * for AutocompletePopup.svelte and EmojiAutocomplete.svelte — don't add it here.
 */

function getPopup(page: import('@playwright/test').Page) {
  return page.locator('ul').filter({
    has: page.locator('button', { hasText: /^.+ :[^:]+:$/ })
  });
}

test.describe('Emoji autocomplete (composer integration)', () => {
  test('popup appears at threshold and hides when below it', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const popup = getPopup(page);

    await roomPage.messageInput.click();

    // ":" alone — no popup
    await roomPage.messageInput.pressSequentially(':');
    await expect(popup).not.toBeVisible();

    // ":h" — still below threshold
    await roomPage.messageInput.pressSequentially('h');
    await expect(popup).not.toBeVisible();

    // ":he" — popup appears
    await roomPage.messageInput.pressSequentially('e');
    await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Backspace back to ":h" — popup hides
    await roomPage.messageInput.press('Backspace');
    await expect(popup).not.toBeVisible();
  });

  test('triggers mid-message and inserts the emoji at the cursor', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.messageInput.click();
    await roomPage.messageInput.pressSequentially('I feel :jo');

    const popup = getPopup(page);
    await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    await roomPage.messageInput.press('Enter');
    await expect(popup).not.toBeVisible();

    // The shortcode is replaced by the emoji + trailing space, and we can keep typing
    await expect(roomPage.messageInput).toHaveText(/^I feel 😂 $/);
    await roomPage.messageInput.pressSequentially('today!');
    await expect(roomPage.messageInput).toHaveText('I feel 😂 today!');
  });

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
