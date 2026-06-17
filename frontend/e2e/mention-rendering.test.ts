import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
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
    const user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await withServerUser(
      browser!,
      serverURL,
      async ({ user: user2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');
        await roomPage.expectMemberVisible(user2.login, { timeout: TIMEOUTS.UI_STANDARD });

        await roomPage2.sendMessage(`Hey @${user1.login}, check this out!`);

        const messageArticle = page
          .locator('[role="article"]')
          .filter({ hasText: `@${user1.login}` });
        await expect(messageArticle).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
        await expect(messageArticle).toHaveClass(/bg-warning\/10/);
      },
      { viewport: { width: 1280, height: 720 } }
    );
  });

  test('self-authored message mentioning self does not highlight', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.sendMessage(`Note to myself @${user.login}`);

    const messageArticle = page.locator('[role="article"]').filter({ hasText: `@${user.login}` });
    await expect(messageArticle).toBeVisible();
    await expect(messageArticle).not.toHaveClass(/bg-warning\/10/);
  });
});
