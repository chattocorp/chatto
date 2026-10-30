import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { DMPage } from './pages/DMPage';
import { MyThreadsPage } from './pages/MyThreadsPage';
import { TIMEOUTS } from './constants';
import * as routes from './routes';
import { getIdsFromUrlViaConnect } from './fixtures/connectHelpers';

test.describe('Direct-message threads', () => {
  test('creates, replies to, follows, and lists a DM thread', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: pageB }) => {
      const room = await new DMPage(pageB).startConversation(userA.login);
      let creations = 0;
      pageB.on('request', (request) => {
        if (request.url().endsWith('/chatto.api.v1.RoomService/StartDM')) creations++;
      });
      const rootText = `DM thread root ${Date.now()}`;
      const replyText = `DM thread reply ${Date.now()}`;

      const root = await room.sendMessage(rootText);
      const rootId = await root.getEventId();
      const draft = 'Keep this unsent root draft';
      await room.messageInput.fill(draft);
      const composer = await room.messageInput.elementHandle();
      await root.openThread();
      await room.expectThreadPaneVisible();
      await expect(room.messageInput).toHaveText(draft);
      expect(await composer!.evaluate((element) => element.isConnected)).toBe(true);
      await pageB.goBack();
      await expect(pageB).toHaveURL(new RegExp(`${routes.user(userA.id!)}$`));
      await expect(room.messageInput).toHaveText(draft);
      await pageB.goForward();
      await room.expectThreadPaneVisible();
      expect(await composer!.evaluate((element) => element.isConnected)).toBe(true);
      await room.postThreadReply(replyText);
      await room.expectTextInThreadPane(replyText);
      expect(creations).toBe(0);
      await pageB.goto(`${routes.user(userA.id!)}/m/${rootId}`);
      await room.expectThreadPaneVisible();
      await room.expectTextInThreadPane(rootText);
      await expect(pageB).not.toHaveURL(/\/m\//);
      await room.expectThreadPaneFollowing();

      // Old room-based DM links must go straight to the canonical thread URL.
      const { roomId } = await getIdsFromUrlViaConnect(pageB);
      const visits: string[] = [];
      pageB.on('framenavigated', (frame) => {
        if (frame === pageB.mainFrame()) visits.push(new URL(frame.url()).pathname);
      });
      await pageB.goto(routes.messageLink(roomId, rootId));
      await expect(pageB).toHaveURL(new RegExp(`${routes.user(userA.id!)}/${rootId}$`));
      await room.expectTextInThreadPane(rootText);
      expect(visits).not.toContain(`${routes.user(userA.id!)}/m/${rootId}`);

      const myThreads = new MyThreadsPage(pageB);
      await myThreads.goto();
      await expect(myThreads.threadItems).toHaveCount(0);
      await pageB.getByRole('radio', { name: 'Mine', exact: true }).check();
      const item = myThreads.threadItems.filter({ hasText: rootText });
      await expect(item).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(item).toContainText(userA.displayName);
      await expect(item.getByText('1 reply')).toHaveCount(0);
      await expect(item).not.toContainText(replyText);

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
        notificationsPage.getNotificationBySummary('replied in a thread you follow')
      ).toBeVisible();
    });
  });
});
