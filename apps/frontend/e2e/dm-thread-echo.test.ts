import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { DMPage } from './pages/DMPage';
import { RoomPage } from './pages/RoomPage';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Direct-message thread echoes', () => {
  test('echoes a reply to the conversation for both participants', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: pageB }) => {
      const roomB = await new DMPage(pageB).startConversation(userA.login);
      const roomID = pageB.url().split('/').pop()!;
      const rootText = `DM echo root ${Date.now()}`;
      const replyText = `DM echo reply ${Date.now()}`;
      const root = await roomB.sendMessage(rootText);

      await page.goto(routes.room(roomID));
      await page.waitForURL(routes.patterns.anyRoom);
      const roomA = new RoomPage(page);
      await roomA.expectMessageVisible(rootText);

      await root.openThread();
      const echoToggle = pageB.getByRole('button', { name: 'Also send to conversation' });
      await expect(echoToggle).toBeVisible();
      await echoToggle.click();
      await roomB.postThreadReply(replyText);
      await roomB.expectTextInThreadPane(replyText);
      await roomB.closeThread();

      const senderEcho = pageB
        .getByTestId('room-main-pane')
        .locator('[role="article"]', { hasText: replyText });
      await expect(senderEcho).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(senderEcho.getByText('Thread')).toBeVisible();

      const receiverEcho = page
        .getByTestId('room-main-pane')
        .locator('[role="article"]', { hasText: replyText });
      await expect(receiverEcho).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(receiverEcho.getByText('Thread')).toBeVisible();
    });
  });
});
