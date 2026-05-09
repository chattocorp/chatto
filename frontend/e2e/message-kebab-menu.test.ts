import { test, expect } from './setup';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { ChatPage, RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Message hover toolbar', () => {
  test('toolbar appears on hover with reaction and action buttons', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Move mouse away from the message so hover state clears
    await page.mouse.move(0, 0);

    // Toolbar should be hidden when not hovering
    await expect(message.hoverToolbar).not.toBeVisible();

    // Hover over the message to reveal the toolbar
    await message.locator.hover();
    await expect(message.hoverToolbar).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Verify toolbar has reaction buttons directly visible (no intermediate popup)
    await expect(message.hoverToolbar.getByLabel('React with 👍')).toBeVisible();
    await expect(message.hoverToolbar.getByLabel('React with ❤️')).toBeVisible();

    // Verify toolbar has action buttons
    await expect(message.hoverToolbar.getByLabel('More actions')).toBeVisible();
  });

  test('toolbar stays visible while emoji picker is open', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar visibility test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Reveal the toolbar
    await message.revealHoverToolbar();

    // Click "More reactions" to open the emoji picker
    await message.hoverToolbar.getByLabel('More reactions').click();

    // Emoji picker should be visible
    const picker = page.locator('input[placeholder="Search emojis..."]');
    await expect(picker).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Move mouse to the emoji picker (away from the message)
    await picker.click();

    // Toolbar should still be visible (forceVisible while picker is open)
    await expect(message.hoverToolbar).toBeVisible();

    // Close the emoji picker
    await page.keyboard.press('Escape');
  });

  test('can add reaction directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar action test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a reaction directly via the toolbar (no context menu intermediate)
    await message.reactViaToolbar('👍');

    // Verify the reaction appears
    await message.expectReaction('👍', 1);
  });

  test('clicking a reaction again in toolbar removes it', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar toggle test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a reaction via the toolbar
    await message.reactViaToolbar('👍');
    await message.expectReaction('👍', 1);

    // Click the same reaction again — should remove it
    await message.reactViaToolbar('👍');
    await message.expectNoReaction('👍');
  });

  test('can edit message directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar edit test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Click edit directly in the toolbar (no context menu intermediate)
    await message.editViaToolbar();

    // Edit mode should be active with the message body pre-filled
    await roomPage.expectEditModeActive();
    await expect(roomPage.composer).toHaveText(testMessage);

    // Cancel the edit
    await page.keyboard.press('Escape');
  });

  test('can reply in thread directly through toolbar', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Toolbar reply test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Click reply directly in the toolbar (no context menu intermediate)
    await message.replyViaToolbar();

    // Thread pane should open
    await roomPage.expectThreadPaneVisible();
  });

  test('toolbar stays visible while context menu is open', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Context menu visibility test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Reveal the toolbar and open the context menu via "More actions"
    await message.revealHoverToolbar();
    await message.hoverToolbar.getByLabel('More actions').click();
    await expect(message.contextMenu).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Move mouse to the context menu (away from the message)
    await message.contextMenu.hover();

    // Toolbar should still be visible (forceVisible while context menu is open)
    await expect(message.hoverToolbar).toBeVisible();

    // Close the context menu
    await page.keyboard.press('Escape');
  });

  test('context menu has no empty actions section for non-author in thread pane', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: create space, post message, open thread, post a reply
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = await chatPage.getSpaceId();

    const rootMessage = `Thread root ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);

    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    const replyText = `Thread reply by author ${Date.now()}`;
    await roomPage.postThreadReply(replyText);

    // User B: join space via /join URL, enter room, open thread
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      await joinSpace(page2);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Open the thread pane via the root message
      const rootMsg2 = roomPage2.getMessage(rootMessage);
      await rootMsg2.openThread();
      await roomPage2.expectThreadPaneVisible();

      // Open context menu on User A's thread reply via toolbar (User B is not the author)
      const threadReply = roomPage2.getThreadMessage(replyText);
      await threadReply.revealHoverToolbar();
      await threadReply.hoverToolbar.getByLabel('More actions').click();
      await expect(threadReply.contextMenu).toBeVisible({
        timeout: TIMEOUTS.UI_FAST
      });

      // Reactions should be visible (User B can react)
      await expect(threadReply.contextMenu.getByLabel(/React with/).first()).toBeVisible();

      // Non-author should see "Reply" (reply-in-room) but NOT "Reply in thread" / Edit / Delete
      await expect(
        threadReply.contextMenu.getByRole('menuitem', { name: 'Reply', exact: true })
      ).toBeVisible();
      await expect(
        threadReply.contextMenu.getByRole('menuitem', { name: /Reply in thread/ })
      ).not.toBeVisible();
      await expect(
        threadReply.contextMenu.getByRole('menuitem', { name: 'Edit' })
      ).not.toBeVisible();
      await expect(
        threadReply.contextMenu.getByRole('menuitem', { name: 'Delete' })
      ).not.toBeVisible();
    } finally {
      await context2.close();
    }
  });

  test('toolbar shows edit and reply actions for message author', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const testMessage = `Author actions test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Reveal the toolbar
    await message.revealHoverToolbar();

    // Author should see edit and more actions menu
    await expect(message.hoverToolbar.getByLabel('Edit message')).toBeVisible();
    await expect(message.hoverToolbar.getByLabel('More actions')).toBeVisible();

    // Reply in thread should be visible (we're in the main room, not thread pane)
    await expect(message.hoverToolbar.getByLabel('Reply in thread')).toBeVisible();
  });

  test('toolbar hides edit for non-author messages', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User A: create space and post message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const spaceId = await chatPage.getSpaceId();
    const testMessage = `Non-author test ${Date.now()}`;
    await roomPage.sendMessage(testMessage);

    // User B: join and check toolbar on User A's message
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      await joinSpace(page2);
      await page2.goto(routes.space());

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);

      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Hover over User A's message
      const message2 = roomPage2.getMessage(testMessage);
      await message2.revealHoverToolbar();

      // Non-author should see reactions, reply, and more actions, but NOT edit
      await expect(message2.hoverToolbar.getByLabel('React with 👍')).toBeVisible();
      await expect(message2.hoverToolbar.getByLabel('Reply in thread')).toBeVisible();
      await expect(message2.hoverToolbar.getByLabel('More actions')).toBeVisible();
      await expect(message2.hoverToolbar.getByLabel('Edit message')).not.toBeVisible();
    } finally {
      await context2.close();
    }
  });
});
