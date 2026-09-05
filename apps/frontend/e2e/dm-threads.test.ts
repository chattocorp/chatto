import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { DMPage } from './pages/DMPage';
import { MyThreadsPage } from './pages/MyThreadsPage';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Direct-message threads', () => {
  test('creates, replies to, follows, and lists a DM thread', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: pageB }) => {
      const room = await new DMPage(pageB).startConversation(userA.login);
      const rootText = `DM thread root ${Date.now()}`;
      const replyText = `DM thread reply ${Date.now()}`;

      const root = await room.sendMessage(rootText);
      await root.openThread();
      await room.expectThreadPaneVisible();
      await room.postThreadReply(replyText);
      await room.expectTextInThreadPane(replyText);
      await room.expectThreadPaneFollowing();

      const myThreads = new MyThreadsPage(pageB);
      await myThreads.goto();
      const item = myThreads.threadItems.filter({ hasText: rootText });
      await expect(item).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(item).toContainText(userA.displayName);
      await expect(item.getByText('1 reply')).toBeVisible();

      await item.click();
      await room.expectThreadPaneVisible();
      await room.expectTextInThreadPane(rootText);
      await room.expectTextInThreadPane(replyText);
    });
  });

  test('creates one notification for a DM thread reply', async ({
    page,
    browser,
    serverURL,
    notificationsPage
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: pageB, user: userB }) => {
      const roomA = await new DMPage(page).startConversation(userB.login);
      const roomB = await new DMPage(pageB).startConversation(userA.login);
      const rootText = `DM notification root ${Date.now()}`;
      const replyText = `DM notification reply ${Date.now()}`;

      await roomA.sendMessage(rootText);
      await roomB.expectMessageVisible(rootText, { timeout: TIMEOUTS.REALTIME_EVENT });
      await roomB.getMessage(rootText).openThread();
      await roomB.expectThreadPaneVisible();

      await page.goto(routes.settings);
      await roomB.postThreadReply(replyText);

      await notificationsPage.goto();
      await notificationsPage.expectNotificationCount(1, TIMEOUTS.REALTIME_EVENT);
      await expect(
        notificationsPage.getNotificationBySummary('replied in a thread you follow.')
      ).toBeVisible();
    });
  });
});
