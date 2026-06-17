import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';

/**
 * E2E coverage focuses on the *highlight wiring* in MessageEvent —
 * specifically the actor-vs-current-user check that decides whether a
 * mention applies the bg-warning/10 background. The rest of mention
 * handling (span rendering, code/blockquote exclusion, case-insensitive
 * matching, valid-vs-invalid distinction) is exhaustively unit-tested in
 * src/lib/mentions.svelte.test.ts and src/lib/mentions.test.ts.
 */

test.describe('Mention highlighting', () => {
  test('message mentioning current user has highlight background', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1 loads the server
    const user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    const serverName = await chatPage.getServerName();
    await chatPage.enterRoom('general');

    // User 2 joins and mentions user 1
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      const user2 = await createAndLoginTestUser(page2);
      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.goto();
      await chatPage2.enterRoom('general');

      // Wait for both users to be visible
      await roomPage.expectMemberVisible(user2.login, { timeout: TIMEOUTS.UI_STANDARD });

      // User 2 sends a message mentioning user 1
      await roomPage2.sendMessage(`Hey @${user1.login}, check this out!`);

      // User 1 should see the message with highlight (bg-warning/10 class)
      const messageArticle = page
        .locator('[role="article"]')
        .filter({ hasText: `@${user1.login}` });
      await expect(messageArticle).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(messageArticle).toHaveClass(/bg-warning\/10/);
    } finally {
      await context2.close();
    }
  });

  test('self-authored message mentioning self does not highlight', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    // Send a message mentioning yourself
    await roomPage.sendMessage(`Note to myself @${user.login}`);

    // The message should NOT have the highlight background
    const messageArticle = page.locator('[role="article"]').filter({ hasText: `@${user.login}` });
    await expect(messageArticle).toBeVisible();
    await expect(messageArticle).not.toHaveClass(/bg-warning\/10/);
  });
});
