import { expect } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('User context menu', () => {
  test.describe('from message avatar', () => {
    test('right-clicking a message avatar shows user profile dialog', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A: Create space and send a message
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      const spaceId = await chatPage.getSpaceId();
      await chatPage.enterRoom('general');
      await roomPage.sendMessage('Hello from User A');

      // User B: Join and send a message
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        const userB = await createAndLoginTestUser(page2);
        await joinSpace(page2, spaceId);
        await page2.goto(routes.space());
        await page2.waitForURL(routes.patterns.anySpace);

        const chatPage2 = new ChatPage(page2);
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        const roomPage2 = new RoomPage(page2);
        await roomPage2.sendMessage('Hello from User B');

        // Wait for User A to see User B's message
        await roomPage.expectMessageVisible('Hello from User B', {
          timeout: TIMEOUTS.REALTIME_EVENT
        });

        // Right-click on User B's avatar in User A's view
        const messageArticle = page.locator('[role="article"]', {
          hasText: 'Hello from User B'
        });
        const avatarButton = messageArticle.locator('button').first();
        await avatarButton.click({ button: 'right' });

        // Verify user profile dialog appears with correct content
        const profileDialog = page.getByRole('dialog', { name: 'User profile' });
        await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
        await expect(profileDialog.getByText(userB.displayName)).toBeVisible();
        await expect(profileDialog.getByText(`@${userB.login}`)).toBeVisible();
      } finally {
        await context2.close();
      }
    });

    test('right-clicking avatar does not open message context menu', async ({
      page,
      chatPage,
      roomPage
    }) => {
      // Create space and send a message
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      await roomPage.sendMessage('Test message');

      // Right-click on own avatar
      const messageArticle = page.locator('[role="article"]', {
        hasText: 'Test message'
      });
      const avatarButton = messageArticle.locator('button').first();
      await avatarButton.click({ button: 'right' });

      // Should see user profile dialog, NOT the message context menu
      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // The message context menu (role="menu") should NOT be visible
      await expect(page.locator('[role="menu"]')).not.toBeVisible();
    });

    test('clicking display name shows user profile dialog', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      await roomPage.sendMessage('Test message');

      // Click on the display name button in the message header
      const messageArticle = page.locator('[role="article"]', {
        hasText: 'Test message'
      });
      const nameButton = messageArticle.getByRole('button', { name: user.displayName });
      await nameButton.click();

      // Verify user profile dialog appears
      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(profileDialog.getByText(user.displayName)).toBeVisible();
      await expect(profileDialog.getByText(`@${user.login}`)).toBeVisible();
    });
  });

  test.describe('from member list', () => {
    test('right-clicking a member shows user profile dialog', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      // Wait for the member to be visible in the member list
      await roomPage.expectMemberVisible(user.login, { timeout: TIMEOUTS.UI_STANDARD });

      // Right-click on the member in the member list
      const memberButton = roomPage.memberList.getByRole('button', {
        name: new RegExp(user.displayName)
      });
      await memberButton.click({ button: 'right' });

      // Verify user profile dialog appears
      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(profileDialog.getByText(user.displayName)).toBeVisible();
      await expect(profileDialog.getByText(`@${user.login}`)).toBeVisible();
    });

    test('left-clicking a member also shows user profile dialog', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      await roomPage.expectMemberVisible(user.login, { timeout: TIMEOUTS.UI_STANDARD });

      // Left-click on the member
      const memberButton = roomPage.memberList.getByRole('button', {
        name: new RegExp(user.displayName)
      });
      await memberButton.click();

      // Verify user profile dialog appears
      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(profileDialog.getByText(user.displayName)).toBeVisible();
    });
  });

  test.describe('dismiss behavior', () => {
    test('pressing Escape closes the user profile dialog', async ({ page, chatPage, roomPage }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      await roomPage.sendMessage('Test message');

      // Open user profile by clicking display name
      const messageArticle = page.locator('[role="article"]', {
        hasText: 'Test message'
      });
      const nameButton = messageArticle.getByRole('button', { name: user.displayName });
      await nameButton.click();

      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Press Escape to dismiss
      await page.keyboard.press('Escape');
      await expect(profileDialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    });

    test('clicking outside closes the user profile dialog', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const user = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');
      await roomPage.sendMessage('Test message');

      // Open user profile by clicking display name
      const messageArticle = page.locator('[role="article"]', {
        hasText: 'Test message'
      });
      const nameButton = messageArticle.getByRole('button', { name: user.displayName });
      await nameButton.click();

      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Click outside the dialog (on the message area)
      await page.locator('body').click({ position: { x: 10, y: 10 } });
      await expect(profileDialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    });
  });

  test.describe('Send Message button', () => {
    test('user profile dialog shows Send Message button', async ({ page, chatPage, roomPage }) => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      // Send a message so we have an avatar to click
      await roomPage.sendMessage('Test message');

      // Open user profile from member list
      const user = await roomPage.getMemberDisplayNamesInOrder();
      const memberButton = roomPage.memberList.getByRole('button', {
        name: new RegExp(user[0])
      });
      await memberButton.click();

      const profileDialog = page.getByRole('dialog', { name: 'User profile' });
      await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Verify "Send Message" button is visible
      await expect(profileDialog.getByRole('button', { name: 'Send Message' })).toBeVisible();
    });
  });
});
