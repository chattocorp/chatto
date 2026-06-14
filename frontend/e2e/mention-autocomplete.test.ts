import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, ExplorePage } from './pages';
import { TIMEOUTS } from './constants';

/**
 * Locator helper for the mention autocomplete popup.
 */
function getMentionPopup(page: import('@playwright/test').Page) {
  return page.getByTestId('mention-autocomplete');
}

test.describe('Mention autocomplete', () => {
  test('Tab completes @mention with matching username', async ({ page, chatPage, roomPage }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Type @partial and press Tab (use click + type to ensure proper events)
    await roomPage.messageInput.click();
    const partialMention = `@${user.login.slice(0, 4)}`;
    await roomPage.messageInput.pressSequentially(partialMention);
    await roomPage.messageInput.press('Tab');

    // Should complete to full username + space
    await expect(roomPage.messageInput).toHaveText(`@${user.login} `);
  });

  test('Tab cycles through multiple matching usernames', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1 creates space
    const user1 = await createAndLoginTestUser(page, { loginPrefix: 'alice' });
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // User 2 joins the space (with similar prefix)
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      const user2 = await createAndLoginTestUser(page2, { loginPrefix: 'alfred' });

      const chatPage2 = new ChatPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // Wait for BOTH users to be visible in member list
      await roomPage.expectMemberVisible(user1.login, { timeout: TIMEOUTS.UI_STANDARD });
      await roomPage.expectMemberVisible(user2.login, { timeout: TIMEOUTS.UI_STANDARD });

      // Now user1 types @al which should match both alice* and alfred*
      await roomPage.messageInput.click();
      await roomPage.messageInput.pressSequentially('@al');
      await roomPage.messageInput.press('Tab');

      // Should complete to first match (alphabetically)
      const inputValue1 = (await roomPage.messageInput.textContent()) ?? '';
      expect(inputValue1).toMatch(/^@al(ice|fred)[0-9]+ $/);

      // Press Tab again to cycle
      await roomPage.messageInput.press('Tab');
      const inputValue2 = (await roomPage.messageInput.textContent()) ?? '';

      // Should now show the other user
      expect(inputValue2).not.toBe(inputValue1);
      expect(inputValue2).toMatch(/^@al(ice|fred)[0-9]+ $/);

      // Press Tab again to cycle to the virtual @all mention, which also
      // matches "al" and is ordered after matching users.
      await roomPage.messageInput.press('Tab');
      const inputValue3 = (await roomPage.messageInput.textContent()) ?? '';
      expect(inputValue3).toBe('@all ');

      // Press Tab once more to cycle back to the first user match.
      await roomPage.messageInput.press('Tab');
      const inputValue4 = (await roomPage.messageInput.textContent()) ?? '';
      expect(inputValue4).toBe(inputValue1);
    } finally {
      await context2.close();
    }
  });

  test('typing any character resets autocomplete state', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1 creates space
    await createAndLoginTestUser(page, { loginPrefix: 'bob' });
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // User 2 joins
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      const user2 = await createAndLoginTestUser(page2, { loginPrefix: 'bobby' });

      const chatPage2 = new ChatPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // Wait for member list to update
      await roomPage.expectMemberVisible(user2.login, { timeout: TIMEOUTS.UI_STANDARD });

      // Type @bo and Tab to complete
      await roomPage.messageInput.click();
      await roomPage.messageInput.pressSequentially('@bo');
      await roomPage.messageInput.press('Tab');
      const firstCompletion = (await roomPage.messageInput.textContent()) ?? '';

      // Type something (this should reset state)
      await roomPage.messageInput.press('End');
      await roomPage.messageInput.type('hello ');

      // Now start a new @mention
      await roomPage.messageInput.type('@bo');
      await roomPage.messageInput.press('Tab');

      // Should start fresh from first match again
      const newCompletion = (await roomPage.messageInput.textContent()) ?? '';
      expect(newCompletion).toContain(firstCompletion.trim());
    } finally {
      await context2.close();
    }
  });

  test('Tab does nothing when no @mention partial at cursor', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Type regular text without @
    await roomPage.messageInput.click();
    await roomPage.messageInput.pressSequentially('hello world');

    // Tab should move focus (default behavior), not complete anything
    await roomPage.messageInput.press('Tab');

    // Input should still have the same value (Tab moved focus away)
    // The value stays the same, but focus moved to send button
    await expect(roomPage.sendButton).toBeFocused();
  });

  test('@mention requires at least one character after @', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Type just @ with no characters after
    await roomPage.messageInput.click();
    await roomPage.messageInput.pressSequentially('@');
    await roomPage.messageInput.press('Tab');

    // Should not complete (Tab moves focus to send button)
    await expect(roomPage.messageInput).toHaveText('@');
    await expect(roomPage.sendButton).toBeFocused();
  });

  test('completing mention in middle of message', async ({ page, chatPage, roomPage }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Type "Hey @partial" with cursor after partial
    await roomPage.messageInput.click();
    const partial = user.login.slice(0, 4);
    await roomPage.messageInput.pressSequentially(`Hey @${partial}`);
    await roomPage.messageInput.press('Tab');

    // Should complete the mention (wait ensures cursor repositioning via tick() completes)
    await expect(roomPage.messageInput).toHaveText(`Hey @${user.login} `);

    // The cursor should be positioned at the end after Tab completion
    // Verify by typing more text - it should appear after the completed mention
    await roomPage.messageInput.pressSequentially('how are you?');
    await expect(roomPage.messageInput).toHaveText(`Hey @${user.login} how are you?`);
  });

  test.describe('Popup', () => {
    test('popup appears when typing @ followed by matching characters', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      await roomPage.messageInput.click();
      await roomPage.messageInput.pressSequentially('@');

      // Popup should NOT appear with just @
      const popup = getMentionPopup(page);
      await expect(popup).not.toBeVisible();

      // Type first char of login — popup should appear (matches the user)
      await roomPage.messageInput.pressSequentially(user.login[0]);
      await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    });

    test('Escape closes popup without completing', async ({ page, chatPage, roomPage }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const partial = user.login.slice(0, 4);
      await roomPage.messageInput.click();
      await roomPage.messageInput.pressSequentially(`@${partial}`);

      const popup = getMentionPopup(page);
      await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

      // Press Escape
      await roomPage.messageInput.press('Escape');
      await expect(popup).not.toBeVisible();

      // Input should still have the partial text
      await expect(roomPage.messageInput).toHaveText(`@${partial}`);
    });

    test('Enter sends message instead of selecting from popup', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const partial = user.login.slice(0, 4);
      await roomPage.messageInput.click();
      await roomPage.messageInput.pressSequentially(`Hello @${partial}`);

      const popup = getMentionPopup(page);
      await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

      // Press Enter — should send the message, not select from popup
      await roomPage.messageInput.press('Enter');

      // Message should appear in the room
      await expect(page.locator('[role="article"]', { hasText: `@${partial}` })).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    });

    test('popup disappears when deleting below threshold', async ({ page, chatPage, roomPage }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const partial = user.login.slice(0, 3);
      await roomPage.messageInput.click();
      await roomPage.messageInput.pressSequentially(`@${partial}`);

      const popup = getMentionPopup(page);
      await expect(popup).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

      // Delete characters until just "@" remains
      for (let i = 0; i < partial.length; i++) {
        await roomPage.messageInput.press('Backspace');
      }
      await expect(popup).not.toBeVisible();
    });
  });

  // Fuzzy-matching coverage moved to unit specs:
  //   src/lib/fuzzyMatch.test.ts                          — algorithm
  //   src/lib/components/composer/MentionAutocomplete.svelte.spec.ts — wrapper + ranking
});
