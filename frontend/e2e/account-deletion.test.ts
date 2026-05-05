import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { waitForUserDeleted } from './fixtures/graphqlHelpers';
import { test } from './setup';
import { AccountPage, ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Account Deletion', () => {
  test.describe('Account Settings Page', () => {
    test('can navigate to account settings page', async ({ page, accountPage }) => {
      await createAndLoginTestUser(page);
      await accountPage.goto();
      await accountPage.expectOnAccountPage();
    });

    test('displays username on account page', async ({ page, accountPage }) => {
      const user = await createAndLoginTestUser(page);
      await accountPage.goto();
      await accountPage.expectUsername(user.login);
    });

    test('can open and close delete confirmation modal', async ({ page, accountPage }) => {
      await createAndLoginTestUser(page);
      await accountPage.goto();

      // Open modal
      await accountPage.openDeleteModal();
      await expect(accountPage.deleteDialog).toBeVisible();

      // Cancel modal
      await accountPage.cancelDelete();
      await expect(accountPage.deleteDialog).not.toBeVisible();
    });

    test('delete button is disabled until DELETE is typed', async ({ page, accountPage }) => {
      await createAndLoginTestUser(page);
      await accountPage.goto();
      await accountPage.openDeleteModal();

      // Button should be disabled initially
      await accountPage.expectDeleteButtonDisabled();

      // Type partial text
      await accountPage.typeConfirmation('DEL');
      await accountPage.expectDeleteButtonDisabled();

      // Type wrong text
      await accountPage.typeConfirmation('delete');
      await accountPage.expectDeleteButtonDisabled();

      // Type correct text
      await accountPage.typeConfirmation('DELETE');
      await accountPage.expectDeleteButtonEnabled();
    });

    test('can delete own account', async ({ page, accountPage, authPage }) => {
      const user = await createAndLoginTestUser(page);
      await accountPage.goto();

      // Delete account
      await accountPage.deleteAccount();

      // Should be redirected to home page (logged out)
      await authPage.expectLoggedOut();

      // Trying to login with the old credentials should fail
      await authPage.gotoLogin();
      await authPage.fillLoginForm(user.login, user.password);
      await authPage.signInButton.click();

      // Should show error (user doesn't exist)
      await authPage.expectError(/invalid/i);
    });
  });

  test.describe('Deleted User Effects', () => {
    test('messages from deleted user show "Deleted User" and content is unavailable', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates a space and posts a message
      const userA = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();
      const messageText = `Hello from ${userA.login} at ${Date.now()}`;
      await roomPage.sendMessage(messageText);

      // User B joins the space
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);

        // User B joins the space
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        // User B enters the room and sees User A's message
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(messageText);

        // Verify User A's display name is shown (active users have a <button> for clickable name)
        await expect(
          page2.locator('[role="article"]').getByRole('button', { name: userA.displayName })
        ).toBeVisible();

        // User A deletes their account
        const accountPage = new AccountPage(page);
        await accountPage.goto();
        await accountPage.deleteAccount();

        // Wait for server to confirm user deletion (replaces arbitrary timeout)
        await waitForUserDeleted(page2, userA.id!);

        // User B refreshes the room to see updated state
        await page2.reload();
        // Wait for the page to load the chat (not redirect to login)
        await page2.waitForURL(routes.patterns.chatRedirect, { timeout: TIMEOUTS.UI_STANDARD });
        await waitForRoomReady(page2, 'general');

        // Message should be hidden entirely: body was crypto-shredded (no body, no reactions, no replies)
        await expect(page2.getByText(messageText)).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // User A's original display name should no longer be visible (message is hidden)
        await expect(
          page2.locator('[role="article"]').getByRole('button', { name: userA.displayName })
        ).not.toBeVisible();
      } finally {
        await context2.close();
      }
    });

    test('messages update in real-time when user deletes account (no refresh needed)', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates a space and posts a message
      const userA = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();
      const messageText = `Real-time test from ${userA.login} at ${Date.now()}`;
      await roomPage.sendMessage(messageText);

      // User B joins the space
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);

        // User B joins the space
        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        // User B enters the room and sees User A's message
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(messageText);

        // Verify User A's display name is shown (active users have a <button> for clickable name)
        await expect(
          page2.locator('[role="article"]').getByRole('button', { name: userA.displayName })
        ).toBeVisible();

        // User A deletes their account
        const accountPage = new AccountPage(page);
        await accountPage.goto();
        await accountPage.deleteAccount();

        // WITHOUT REFRESHING: User B should see the message disappear in real-time
        // SpaceMemberDeletedEvent triggers refetch → body is crypto-shredded → message hidden
        // (no body, no reactions, no replies)
        await expect(page2.getByText(messageText)).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // User A's original display name should no longer be visible (message is hidden)
        await expect(
          page2.locator('[role="article"]').getByRole('button', { name: userA.displayName })
        ).not.toBeVisible();

        // Message is hidden entirely (no body, no reactions, no replies) — no placeholder shown
        await expect(page2.getByText('[Message deleted]')).not.toBeVisible();
      } finally {
        await context2.close();
      }
    });

    test('system events from deleted user show "Deleted User"', async ({
      page,
      chatPage,
      browser,
      serverURL
    }) => {
      // User A creates a space
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();

      // User B joins the space (this creates a join event)
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        const userB = await createAndLoginTestUser(page2);

        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User A should see User B's join event
        await expect(
          page.getByText(new RegExp(`${userB.displayName} joined the room`))
        ).toBeVisible();

        // User B deletes their account
        const accountPage2 = new AccountPage(page2);
        await accountPage2.goto();
        await accountPage2.deleteAccount();

        // Wait for server to confirm user deletion (replaces arbitrary timeout)
        await waitForUserDeleted(page, userB.id!);

        // User A refreshes to see updated state
        await page.reload();
        await waitForRoomReady(page, 'general');

        // User A should now see "Deleted User joined the room"
        await expect(page.getByText(/Deleted User joined the room/)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });

        // User B's original display name should no longer be visible in system events
        await expect(page.getByText(new RegExp(`${userB.displayName} joined`))).not.toBeVisible();
      } finally {
        await context2.close();
      }
    });

    test('room member list shows correct members after user deletion', async ({
      page,
      chatPage,
      roomPage,
      browser,
      serverURL
    }) => {
      // User A creates a space
      const userA = await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.createSpace();
      await chatPage.enterRoom('general');

      const spaceId = await chatPage.getSpaceId();

      // User B joins the space and room
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        const userB = await createAndLoginTestUser(page2);

        await page2.goto(routes.joinSpace(spaceId));
        await page2.getByRole('button', { name: 'Join Space' }).click();
        await page2.waitForURL(routes.patterns.spaceOrRoom);

        const chatPage2 = new ChatPage(page2);
        const roomPage2 = new RoomPage(page2);

        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // Both users should see member list with 3 members initially: e2eadmin
        // (bootstrap space owner) + userA + userB. Issue #330 / ADR-027:
        // bootstrap creates the primary space owned by e2eadmin, so they
        // count among general's members.
        await page.reload();
        await waitForRoomReady(page, 'general');
        await expect(roomPage.memberCount).toHaveText('Members (3)');
        await expect(roomPage2.memberCount).toHaveText('Members (3)');

        // User A deletes their account
        const accountPage = new AccountPage(page);
        await accountPage.goto();
        await accountPage.deleteAccount();

        // Wait for server to confirm user deletion (replaces arbitrary timeout)
        await waitForUserDeleted(page2, userA.id!);

        // User B refreshes to see updated state
        await page2.reload();
        await waitForRoomReady(page2, 'general');

        // User B should see e2eadmin + themselves (not 0, not 3)
        await expect(roomPage2.memberCount).toHaveText('Members (2)', { timeout: TIMEOUTS.REALTIME_EVENT });

        // User B's name should still be visible in the member list
        await expect(page2.getByLabel('Room members').getByText(userB.login)).toBeVisible();

        // User C joins to verify new members can still join and be listed
        const context3 = await browser!.newContext({ baseURL: serverURL });
        const page3 = await context3.newPage();

        try {
          const userC = await createAndLoginTestUser(page3);

          await page3.goto(routes.joinSpace(spaceId));
          await page3.getByRole('button', { name: 'Join Space' }).click();
          await page3.waitForURL(routes.patterns.spaceOrRoom);

          const chatPage3 = new ChatPage(page3);
          const roomPage3 = new RoomPage(page3);

          await chatPage3.enterRoom('general');
          await waitForRoomReady(page3, 'general');

          // User C should see 3 members (e2eadmin + User B + themselves)
          await expect(roomPage3.memberCount).toHaveText('Members (3)');

          // User B refreshes and should also see 3 members
          await page2.reload();
          await waitForRoomReady(page2, 'general');
          await expect(roomPage2.memberCount).toHaveText('Members (3)');

          // Both User B and User C should be visible in the member list
          await expect(page2.getByLabel('Room members').getByText(userB.login)).toBeVisible();
          await expect(page2.getByLabel('Room members').getByText(userC.login)).toBeVisible();
        } finally {
          await context3.close();
        }
      } finally {
        await context2.close();
      }
    });
  });
});
