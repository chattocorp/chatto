import { expect } from '@playwright/test';
import { test } from './setup';
import { ChatPage } from './pages';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  denyUserInstancePermission,
  clearUserInstancePermissionOverride,
  type TestUser
} from './fixtures/testUser';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Helper to create a second user via the GraphQL API (without logging in).
 */
async function createSecondUser(page: import('@playwright/test').Page): Promise<TestUser> {
  const timestamp = Date.now();
  const user: TestUser = {
    email: `dmuser${timestamp}@example.com`,
    login: `dmuser${timestamp}`,
    displayName: `DM User ${timestamp}`,
    password: 'testpassword123'
  };

  const createUserResponse = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation CreateUser($input: CreateUserInput!) {
					createUser(input: $input) {
						id
					}
				}
			`,
      variables: {
        input: {
          login: user.login,
          displayName: user.displayName,
          password: user.password
        }
      }
    }
  });
  expect(createUserResponse.ok()).toBeTruthy();

  return user;
}

test.describe('Direct Messages', () => {
  test('can start DM with another user', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);

    // Should navigate to the conversation and show the other user's name
    await dmPage.expectConversationHeader(user2.displayName);
    await expect(roomPage.messageInput).toBeVisible();
  });

  test('can send message in DM conversation', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);

    const testMessage = `Hello from DM test! ${Date.now()}`;
    await roomPage.sendMessage(testMessage);
  });

  test('DM conversation appears in list after sending a message', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    // Start DM and send a message (empty DMs are hidden from the list)
    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);
    await roomPage.sendMessage(`Hello! ${Date.now()}`);

    // Go back to DM list
    await dmPage.goto();

    // Should see the conversation in the list
    await dmPage.expectConversationVisible(user2.displayName);
  });

  test('new DM conversation appears in sidebar immediately after first message', async ({
    page,
    dmPage
  }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    // Start DM - navigates to /chat/dm/-/{conversationId}
    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);

    // The sidebar should NOT show the conversation yet (empty DMs are filtered)
    await dmPage.expectConversationNotVisible(user2.displayName);

    // Send the first message
    await roomPage.sendMessage(`First DM message ${Date.now()}`);

    // The conversation should appear in the sidebar WITHOUT navigating away.
    // The MessagePostedEvent triggers a refetch that picks up the now-non-empty DM.
    await expect(async () => {
      await dmPage.expectConversationVisible(user2.displayName);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
  });

  test('can add reaction to DM message', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);

    const testMessage = `DM reaction test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    await message.react('👍');
    await message.expectReaction('👍', 1);
  });

  test('can reply to a DM message', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);

    // Send a message to reply to
    const originalMessage = `Original DM ${Date.now()}`;
    const message = await roomPage.sendMessage(originalMessage);

    // Reply to it via context menu
    await message.replyInRoom();

    // The composer should show reply indicator
    await expect(page.getByText('Replying to')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Send the reply
    const replyText = `Reply to DM ${Date.now()}`;
    await roomPage.sendMessage(replyText);

    // Verify reply attribution is visible on the reply message
    await expect(
      page.locator('[role="article"]', { hasText: replyText }).getByTestId('reply-attribution')
    ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('can delete attachment from DM message', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);

    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user2.login);

    // Upload an image attachment
    const fileChooserPromise = page.waitForEvent('filechooser');
    await page.getByTitle('Attach file').click();
    const fileChooser = await fileChooserPromise;

    // Create a 100x100 red PNG (must be large enough for the thumbnail to be visible)
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAABFUlEQVR4nO3OUQkAIABEsetfWiv4Nx4IC7Cd7XvkByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIX4Q4gchfhDiByF+EOIHIReeLesrH9s1agAAAABJRU5ErkJggg==',
      'base64'
    );
    await fileChooser.setFiles({
      name: 'test-image.png',
      mimeType: 'image/png',
      buffer: pngData
    });

    // Wait for attachment preview
    const attachmentPreview = page.locator('img[alt="test-image.png"]').first();
    await expect(attachmentPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Add message text and send
    const testMessage = `DM attachment test ${Date.now()}`;
    await roomPage.messageInput.fill(testMessage);
    await roomPage.messageInput.press('Enter');

    // Wait for message to appear
    await roomPage.expectMessageVisible(testMessage);

    // Get the message and verify attachment
    const message = roomPage.getMessage(testMessage);
    await message.expectAttachment();

    // Delete the attachment
    await message.deleteAttachment();

    // Verify the attachment is gone
    await message.expectNoAttachment();
  });
});

test.describe('Self-DM', () => {
  test('can start a DM conversation with yourself', async ({ page, dmPage }) => {
    const user = await createAndLoginTestUser(page);
    await dmPage.goto();

    const roomPage = await dmPage.startConversation(user.login);

    // Self-DM header shows user's own display name
    await dmPage.expectConversationHeader(user.displayName);
    await expect(roomPage.messageInput).toBeVisible();
  });

  test('self-DM appears in conversation list after sending a message', async ({ page, dmPage }) => {
    const user = await createAndLoginTestUser(page);

    // Create self-DM and send a message (empty DMs are hidden from the list)
    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user.login);
    await roomPage.sendMessage(`Note to self ${Date.now()}`);

    // Go back to DM list
    await dmPage.goto();

    // Self-DM shows user's own display name
    await dmPage.expectConversationVisible(user.displayName);
  });

  test('can send message in self-DM', async ({ page, dmPage }) => {
    const user = await createAndLoginTestUser(page);

    await dmPage.goto();
    const roomPage = await dmPage.startConversation(user.login);

    const testMessage = `Note to self: ${Date.now()}`;
    await roomPage.sendMessage(testMessage);

    await roomPage.expectMessageVisible(testMessage);
  });

  test('starting self-DM twice returns the same conversation', async ({ page, dmPage }) => {
    const user = await createAndLoginTestUser(page);

    // Create self-DM first time
    await dmPage.startConversation(user.login);
    const firstUrl = page.url();

    // Start again via API — should return the same room
    await dmPage.startConversation(user.login);
    const secondUrl = page.url();

    expect(secondUrl).toBe(firstUrl);
  });
});

test.describe('DM Permissions', () => {
  test('user with denied dm.view cannot see DM icon in sidebar', async ({ page, browser }) => {
    // Login as admin user to make permission changes
    await loginAsAdmin(page);

    // Create regular user in separate browser context
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularChatPage = new ChatPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    // Navigate to chat - should see DM icon by default (everyone has dm.view)
    await regularChatPage.goto();
    await expect(regularPage.locator('[title="Direct Messages"]')).toBeVisible();

    // Deny dm.view for the regular user (using admin context)
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'dm.view');

    // Reload and verify DM icon is hidden
    await regularPage.reload();
    await expect(regularPage.locator('[title="Direct Messages"]')).not.toBeVisible();

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'dm.view', denyRoleName);
    await regularContext.close();
  });

  test('user with denied dm.view sees access denied on /chat/dm', async ({ page, browser }) => {
    // Login as admin user to make permission changes
    await loginAsAdmin(page);

    // Create regular user in separate browser context
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // Deny dm.view for the regular user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'dm.view');

    // Navigate directly to /chat/dm
    await regularPage.goto(routes.dm);

    // Should see access denied message
    await expect(regularPage.getByText('Access Denied', { exact: true })).toBeVisible();
    await expect(
      regularPage.getByText('You do not have permission to access Direct Messages.')
    ).toBeVisible();

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'dm.view', denyRoleName);
    await regularContext.close();
  });

  test('user with dm.view but denied dm.write can view DMs but not start new ones', async ({
    page,
    browser
  }) => {
    // Login as admin user to make permission changes
    const adminUser = await loginAsAdmin(page);

    // Create regular user in separate browser context
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const { DMPage } = await import('./pages');
    const regularDmPage = new DMPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    // Admin starts a DM with the regular user and sends a message
    // (empty DMs are hidden from the list, so a message is needed for visibility)
    const { DMPage: AdminDMPage } = await import('./pages');
    const adminDmPage = new AdminDMPage(page);
    await adminDmPage.goto();
    const adminRoomPage = await adminDmPage.startConversation(regularUser.login);
    await adminRoomPage.sendMessage(`Hello from admin ${Date.now()}`);

    // Now deny dm.write for the regular user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'dm.write');

    // Regular user navigates to DM page
    await regularDmPage.goto();

    // Should be able to view the DM list (has dm.view)
    await expect(regularPage.getByText('Access Denied')).not.toBeVisible();

    // Should see the existing conversation from admin
    await regularDmPage.expectConversationVisible(adminUser.displayName);

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'dm.write', denyRoleName);
    await regularContext.close();
  });

  test('user can access DMs by default', async ({ page, dmPage }) => {
    // Create a user (all users have dm.view and dm.write via everyone role)
    await createAndLoginTestUser(page);

    // Navigate to DM page
    await dmPage.goto();

    // Should NOT see access denied
    await expect(page.getByText('Access Denied')).not.toBeVisible();

    // Should see the DM interface (either empty state or conversation list)
    await expect(page.getByRole('heading', { name: 'Direct Messages' })).toBeVisible();
  });
});

test.describe('DM List Sorting', () => {
  // These tests create multiple conversations and send messages, which takes time
  test.setTimeout(30000);

  test('sending a message bumps conversation to top of list', async ({ page, dmPage }) => {
    await createAndLoginTestUser(page);
    const user2 = await createSecondUser(page);
    const user3 = await createSecondUser(page);

    // Create conversations and send messages to establish order
    // (DM list is sorted by last message time, not creation time)
    await dmPage.goto();
    let roomPage = await dmPage.startConversation(user2.login);
    await roomPage.sendMessage(`First message to user2 - ${Date.now()}`);

    await dmPage.goto();
    await dmPage.expectConversationVisible(user2.displayName);

    roomPage = await dmPage.startConversation(user3.login);
    await roomPage.sendMessage(`First message to user3 - ${Date.now()}`);

    await dmPage.goto();
    await dmPage.expectConversationVisible(user2.displayName);
    await dmPage.expectConversationVisible(user3.displayName);

    // User3 should be at top (most recent message)
    await dmPage.expectConversationAtTop(user3.displayName);

    // Send a message to user2's conversation (not at top)
    roomPage = await dmPage.openConversation(user2.displayName);
    // Verify we're in the right conversation (header shows the user's name)
    await dmPage.expectConversationHeader(user2.displayName);
    await roomPage.sendMessage(`Second message to user2 - ${Date.now()}`);

    // Go back to DM list
    await dmPage.goto();

    // User2's conversation should now be at the top
    await dmPage.expectConversationAtTop(user2.displayName);
  });
});

test.describe('DM from Member List', () => {
  // Use larger viewport so member list is visible (lg breakpoint is 1024px)
  test.use({ viewport: { width: 1280, height: 720 } });

  test('can start DM by clicking member in room member list', async ({ page, chatPage }) => {
    const { RoomPage } = await import('./pages');

    // Create a user and create a space/room
    const user = await createAndLoginTestUser(page);

    await chatPage.goto();
    await chatPage.createSpace('Test Space');
    await chatPage.createRoom('test-room');

    // Create a RoomPage to interact with the room
    const roomPage = new RoomPage(page);

    // Wait for member list to show our name
    await roomPage.expectMemberVisible(user.displayName, { timeout: TIMEOUTS.REALTIME_EVENT });

    // Click on ourselves in the member list (opens profile popover)
    const memberButton = page.locator('aside[aria-label="Room members"] button', {
      hasText: user.displayName
    });
    await memberButton.click();

    // Profile popover should appear with "Send Message" button
    const sendMessageButton = page.getByRole('button', { name: 'Send Message', exact: true });
    await expect(sendMessageButton).toBeVisible();

    // Click "Send Message" to start the DM
    await sendMessageButton.click();

    // Should navigate to DM conversation
    await page.waitForURL(routes.patterns.anyDmConversation);

    // Should show user's own display name in the conversation header (self-DM)
    await expect(page.getByRole('heading', { name: user.displayName })).toBeVisible();
  });

  test('can start DM by clicking author name in a message', async ({ page, chatPage }) => {
    const { RoomPage } = await import('./pages');

    // Create a user and a space/room
    const user = await createAndLoginTestUser(page);

    await chatPage.goto();
    await chatPage.createSpace('Test Space');
    await chatPage.createRoom('test-room');

    const roomPage = new RoomPage(page);
    const testMessage = `Clickable name test ${Date.now()}`;
    await roomPage.sendMessage(testMessage);

    // Click the author's display name in the message
    const messageAuthor = page
      .locator('[role="article"]', { hasText: testMessage })
      .getByRole('button', { name: user.displayName });
    await messageAuthor.click();

    // Profile popover should appear with "Send Message" button
    const popover = page.getByRole('dialog', { name: 'User profile' });
    await expect(popover).toBeVisible();
    const sendMessageButton = popover.getByRole('button', { name: 'Send Message' });
    await expect(sendMessageButton).toBeVisible();

    // Click "Send Message" to start the DM
    await sendMessageButton.click();

    // Should navigate to DM conversation
    await page.waitForURL(routes.patterns.anyDmConversation);
    await expect(page.getByRole('heading', { name: user.displayName })).toBeVisible();
  });

  test('can open profile popover by clicking @mention in a message', async ({ page, chatPage }) => {
    const { RoomPage } = await import('./pages');

    // Create a user and a space/room
    const user = await createAndLoginTestUser(page);

    await chatPage.goto();
    await chatPage.createSpace('Test Space');
    await chatPage.createRoom('test-room');

    const roomPage = new RoomPage(page);

    // Send a message that mentions ourselves
    const testMessage = `Hey @${user.login} check this out`;
    await roomPage.sendMessage(testMessage);

    // Click the @mention in the message body
    const mention = page.locator('.mention', { hasText: `@${user.login}` }).first();
    await expect(mention).toBeVisible();
    await mention.click();

    // Profile popover should appear
    const popover = page.getByRole('dialog', { name: 'User profile' });
    await expect(popover).toBeVisible();
    await expect(popover.getByText(`@${user.login}`)).toBeVisible();

    // Should show "Send Message" button
    await expect(popover.getByRole('button', { name: 'Send Message' })).toBeVisible();
  });

  test('popover closes when clicking outside', async ({ page, chatPage }) => {
    const { RoomPage } = await import('./pages');

    const user = await createAndLoginTestUser(page);

    await chatPage.goto();
    await chatPage.createSpace('Test Space');
    await chatPage.createRoom('test-room');

    const roomPage = new RoomPage(page);
    const testMessage = `Click outside test ${Date.now()}`;
    await roomPage.sendMessage(testMessage);

    // Open the popover by clicking the author name
    const messageAuthor = page
      .locator('[role="article"]', { hasText: testMessage })
      .getByRole('button', { name: user.displayName });
    await messageAuthor.click();

    const popover = page.getByRole('dialog', { name: 'User profile' });
    await expect(popover).toBeVisible();

    // Click outside the popover (on the message area)
    await page
      .locator('[role="article"]', { hasText: testMessage })
      .click({ position: { x: 5, y: 5 } });

    // Popover should close
    await expect(popover).not.toBeVisible();
  });

  test('popover closes when pressing Escape', async ({ page, chatPage }) => {
    const { RoomPage } = await import('./pages');

    const user = await createAndLoginTestUser(page);

    await chatPage.goto();
    await chatPage.createSpace('Test Space');
    await chatPage.createRoom('test-room');

    const roomPage = new RoomPage(page);
    const testMessage = `Escape test ${Date.now()}`;
    await roomPage.sendMessage(testMessage);

    // Open the popover by clicking the author name
    const messageAuthor = page
      .locator('[role="article"]', { hasText: testMessage })
      .getByRole('button', { name: user.displayName });
    await messageAuthor.click();

    const popover = page.getByRole('dialog', { name: 'User profile' });
    await expect(popover).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');

    // Popover should close
    await expect(popover).not.toBeVisible();
  });

  test('popover hides Send Message when dm.write is denied', async ({
    page,
    browser
  }) => {
    // Login as admin to manage permissions
    const adminPage = page;
    await loginAsAdmin(adminPage);

    // Create regular user in separate browser context
    const regularContext = await browser!.newContext();
    const regularPage = await regularContext.newPage();
    const regularChatPage = new ChatPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    try {
      // Deny dm.write for the regular user
      const denyRoleName = await denyUserInstancePermission(adminPage, regularUser.id!, 'dm.write');

      // Regular user creates a space and room
      await regularChatPage.goto();
      await regularChatPage.createSpace('Popover Perm Test');
      await regularChatPage.createRoom('test-room');

      const { RoomPage } = await import('./pages');
      const roomPage = new RoomPage(regularPage);
      const testMessage = `DM perm test ${Date.now()}`;
      await roomPage.sendMessage(testMessage);

      // Reload to pick up permission changes
      await regularPage.reload();
      await roomPage.expectMessageVisible(testMessage, { timeout: TIMEOUTS.REALTIME_EVENT });

      // Click the author name to open popover
      const messageAuthor = regularPage
        .locator('[role="article"]', { hasText: testMessage })
        .getByRole('button', { name: regularUser.displayName });
      await messageAuthor.click();

      // Popover should appear but WITHOUT "Send Message" button
      const popover = regularPage.getByRole('dialog', { name: 'User profile' });
      await expect(popover).toBeVisible();
      await expect(popover.getByText(`@${regularUser.login}`)).toBeVisible();
      await expect(popover.getByRole('button', { name: 'Send Message' })).not.toBeVisible();

      // Clean up permission
      await clearUserInstancePermissionOverride(
        adminPage,
        regularUser.id!,
        'dm.write',
        denyRoleName
      );
    } finally {
      await regularContext.close();
    }
  });
});
