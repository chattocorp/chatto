import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test.describe('Recent reactions', () => {
  test('reacting with an emoji moves it to the front of the quick reactions', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    // Send two messages so we can check the toolbar on the second one
    await roomPage.sendMessage('Message 1');
    const message2 = await roomPage.sendMessage('Message 2');

    // Check default quick reactions on the toolbar:
    // first 4 slots are pinned, last 2 are fallback defaults
    const defaultReactions = await message2.getToolbarQuickReactions();
    expect(defaultReactions).toEqual(['👍', '👋', '🤣', '🙏', '❤️', '😂']);

    // React with a non-default emoji via the emoji picker
    const message1 = roomPage.getMessage('Message 1');
    await message1.reactViaEmojiPicker('rocket', 'rocket');
    await message1.expectReaction('🚀', 1);

    // Now hover message2 — rocket should be at the first non-pinned slot (4).
    // Pinned slots 0-3 are unchanged.
    const updatedReactions = await message2.getToolbarQuickReactions();
    expect(updatedReactions.slice(0, 4)).toEqual(['👍', '👋', '🤣', '🙏']);
    expect(updatedReactions[4]).toBe('🚀');
    expect(updatedReactions).toHaveLength(6);
    // The last fallback emoji should have been pushed off
    expect(updatedReactions).not.toContain('😂');
  });

  test('recent reactions persist across page reload', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const message1 = await roomPage.sendMessage('Persist test');

    // React with a non-default emoji
    await message1.reactViaEmojiPicker('fire', 'fire');
    await message1.expectReaction('🔥', 1);

    // Reload the page
    await page.reload();
    await expect(page.getByText('Persist test')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Send another message and check the toolbar:
    // pinned slots 0-3 unchanged, fire surfaced at slot 4
    const message2 = await roomPage.sendMessage('After reload');
    const reactions = await message2.getToolbarQuickReactions();
    expect(reactions.slice(0, 4)).toEqual(['👍', '👋', '🤣', '🙏']);
    expect(reactions[4]).toBe('🔥');
  });

  test('quick reaction buttons do not change order', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.sendMessage('First message');
    const message2 = await roomPage.sendMessage('Second message');

    // React with a quick reaction via the toolbar (not the emoji picker)
    const message1 = roomPage.getMessage('First message');
    await message1.reactViaToolbar('❤️');
    await message1.expectReaction('❤️', 1);

    // Order should remain unchanged — quick reactions don't update recency
    const reactions = await message2.getToolbarQuickReactions();
    expect(reactions).toEqual(['👍', '👋', '🤣', '🙏', '❤️', '😂']);
  });
});
