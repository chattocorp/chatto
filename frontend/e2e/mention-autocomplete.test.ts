import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

/**
 * E2E coverage for the TipTap-driven mention autocomplete integration keeps one
 * full app smoke: room members come from the real backend, Tab completes the
 * mention, and the completed mention can be sent. Local popup behaviour,
 * ranking, threshold show/hide, Escape, and cursor replacement are covered by
 * the browser/unit specs for MessageComposer, AutocompleteState, and
 * MentionAutocomplete.
 */
test.describe('Mention autocomplete', () => {
  test('Tab completes @mention with matching username and sends it', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.messageInput.click();
    const partialMention = `@${user.login.slice(0, 4)}`;
    await roomPage.messageInput.pressSequentially(partialMention);
    await roomPage.messageInput.press('Tab');

    await expect(roomPage.messageInput).toHaveText(`@${user.login} `);
    await roomPage.messageInput.pressSequentially('hello');
    await roomPage.messageInput.press('Enter');

    await expect(page.locator('[role="article"]', { hasText: `@${user.login} hello` })).toBeVisible(
      { timeout: TIMEOUTS.UI_STANDARD }
    );
  });
});
