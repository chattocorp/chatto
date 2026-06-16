import { expect } from '@playwright/test';
import { createAndLoginTestUser, openServer } from './fixtures/testUser';
import { test } from './setup';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

test.describe('Room auto-join', () => {
  test('user is auto-joined to default rooms when opening the server', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const spaceId = await chatPage.getServerScopeId();

    // User B: Create account and open the server
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      // User B opens the server
      await openServer(page2);
      await page2.goto(routes.space());

      // Verify User B sees both default auto-join rooms in the sidebar
      const roomList = page2.locator('.room-list');

      // "general" should be visible (auto-joined)
      const generalRoom = roomList.getByRole('link', { name: '# general' });
      await expect(generalRoom).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // "announcements" should be visible (auto-joined)
      const announcementsRoom = roomList.getByRole('link', { name: '# announcements' });
      await expect(announcementsRoom).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // User B can click on a room and see its content (confirming they're a member)
      await generalRoom.click();
      await page2.waitForURL(routes.patterns.anyRoom);

      // Room header should be visible
      await expect(page2.getByRole('heading', { name: '# general' })).toBeVisible();

      // Message input should be available (confirming room access)
      await expect(page2.getByTestId('message-input')).toBeVisible();
    } finally {
      await context2.close();
    }
  });

  test('user can see messages posted before they joined', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const spaceId = await chatPage.getServerScopeId();

    // User A enters general room and posts a message
    await chatPage.enterRoom('general');

    const testMessage = `Message before join ${Date.now()}`;
    await roomPage.sendMessage(testMessage);

    // User B: Create account and open the server
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      // User B joins via the join page
      await openServer(page2);
      await page2.goto(routes.space());

      // User B clicks on general room (auto-joined)
      const generalRoom = page2.locator('.room-list').getByRole('link', { name: '# general' });
      await expect(generalRoom).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await generalRoom.click();
      await page2.waitForURL(routes.patterns.anyRoom);

      // User B should see the message posted by User A before they joined
      await expect(page2.getByText(testMessage)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });
});
