import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { TIMEOUTS } from './constants';

test.describe('Emoji reactions', () => {
  test('shows reactor names by emoji from desktop and touch message menus', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60_000);
    const browserErrors: string[] = [];
    page.on('pageerror', (error) => browserErrors.push(error.message));
    page.on('console', (message) => {
      if (message.type() === 'error') browserErrors.push(message.text());
    });
    const firstUser = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const body = `Reaction details ${Date.now()}`;
    const message = await roomPage.sendMessage(body);
    await message.react('❤️');
    await message.expectReaction('❤️', 1);

    let secondName = '';
    await withServerUser(
      browser!,
      serverURL,
      async ({ user, chatPage: otherChat, roomPage: otherRoom }) => {
        secondName = user.displayName;
        await otherChat.goto();
        await otherChat.enterRoom('general');
        const otherMessage = otherRoom.getMessage(body);
        await otherMessage.react('👍');
        await otherMessage.expectReaction('👍', 1);
      },
      { viewport: { width: 1280, height: 720 } }
    );
    await message.expectReaction('👍', 1);

    await message.locator.locator('.message-content-stack').click({ button: 'right' });
    const desktopMenu = page.getByRole('menu');
    await desktopMenu.getByRole('menuitem', { name: 'Reactions', exact: true }).click();
    const details = page.getByRole('dialog', { name: 'Reactions' });
    await expect(details.getByTestId('reaction-details-panel')).toContainText(
      firstUser.displayName
    );
    await details.getByRole('tab', { name: /Thumbs up/ }).click();
    await expect(details.getByTestId('reaction-details-panel')).toContainText(secondName);
    await expect(details.getByTestId('reaction-details-panel')).not.toContainText(
      firstUser.displayName
    );
    await details.getByRole('button', { name: 'Close' }).click();

    await page.setViewportSize({ width: 390, height: 844 });
    const cdp = await page.context().newCDPSession(page);
    await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: true });
    await page.reload();
    await roomPage.expectMessageVisible(body);
    const touchMessage = roomPage.getMessage(body).locator.locator('.message-content-stack');
    await touchMessage.scrollIntoViewIfNeeded();
    const box = await touchMessage.boundingBox();
    if (!box) throw new Error('Message content is not visible');
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: box.x + box.width / 2, y: box.y + box.height / 2 }]
    });
    const actionSheet = page.getByRole('dialog', { name: 'Message actions' });
    await expect(actionSheet).toBeVisible({ timeout: 5000 });
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    const reactionsButton = actionSheet.getByRole('button', { name: 'Reactions', exact: true });
    const reactionsBox = await reactionsButton.boundingBox();
    if (!reactionsBox) throw new Error('Reactions action is not visible');
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [
        { x: reactionsBox.x + reactionsBox.width / 2, y: reactionsBox.y + reactionsBox.height / 2 }
      ]
    });
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    const touchDetails = page.getByRole('dialog', { name: 'Reactions' });
    await expect(touchDetails).toHaveClass(/bottom-sheet/);
    await touchDetails.getByRole('tab', { name: /Thumbs up/ }).click();
    await expect(touchDetails.getByTestId('reaction-details-panel')).toContainText(secondName);

    await touchDetails.getByRole('button', { name: 'Close' }).last().click();
    await expect(touchDetails).not.toBeVisible();
    const threadBox = await touchMessage.boundingBox();
    if (!threadBox) throw new Error('Message content is not visible');
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: threadBox.x + threadBox.width / 2, y: threadBox.y + threadBox.height / 2 }]
    });
    await expect(actionSheet).toBeVisible({ timeout: 5000 });
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    const replyInThread = actionSheet.getByRole('button', {
      name: 'Reply in thread',
      exact: true
    });
    const replyBox = await replyInThread.boundingBox();
    if (!replyBox) throw new Error('Reply in thread action is not visible');
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: replyBox.x + replyBox.width / 2, y: replyBox.y + replyBox.height / 2 }]
    });
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await expect(page.getByRole('heading', { name: 'Thread in #general' })).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Reply in thread...' })).toBeVisible();
    await cdp.detach();

    expect(browserErrors).toEqual([]);
  });

  test('add a reaction to a message', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Reaction test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a thumbs up reaction
    await message.react('👍');

    // Verify the reaction appears
    await message.expectReaction('👍', 1);
  });

  test('toggle reaction off by clicking it', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Toggle reaction test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a heart reaction
    await message.react('❤️');
    await message.expectReaction('❤️', 1);

    // Toggle it off
    await message.toggleReaction('❤️');
    await message.expectNoReaction('❤️');
  });

  test('real-time reaction sync via public event subscription', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create account and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Canonical event sync test ${Date.now()}`;
    const message1 = await roomPage.sendMessage(testMessage);

    // Verify no reactions yet (use expectNoReaction to check for reaction count buttons, not toolbar buttons)
    await message1.expectNoReaction('😂');

    // User 2: Create user and open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.goto();

        // Enter the general room
        await chatPage2.enterRoom('general');

        // Wait for messages to load
        await roomPage2.expectMessageVisible(testMessage);

        // User 2 adds a reaction
        const message2 = roomPage2.getMessage(testMessage);
        await message2.react('😂');
        await message2.expectReaction('😂', 1);

        // User 1 should see the reaction through public realtime delivery.
        await message1.expectReaction('😂', 1);
      },
      { viewport: { width: 1280, height: 720 } }
    );
  });

  test('hovering over a reaction shows tooltip with user name', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Tooltip test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a reaction
    await message.react('👍');
    await message.expectReaction('👍', 1);

    // Hover over the reaction and verify tooltip shows the user's display name
    await message.expectReactionTooltip('👍', user.displayName);
  });

  test('reaction tooltip shows multiple user names', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create account and post a message
    const user1 = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Multi-user tooltip test ${Date.now()}`;
    const message1 = await roomPage.sendMessage(testMessage);

    // User 1 adds a reaction
    await message1.react('❤️');
    await message1.expectReaction('❤️', 1);

    // User 2: Create user and open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ user: user2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.goto();
        await chatPage2.enterRoom('general');

        await roomPage2.expectMessageVisible(testMessage);

        // User 2 adds the same reaction
        const message2 = roomPage2.getMessage(testMessage);
        await message2.react('❤️');
        await message2.expectReaction('❤️', 2);

        // User 1 should see the updated count
        await message1.expectReaction('❤️', 2);

        // Verify tooltip shows both user names (separated by comma)
        await message1.expectReactionTooltipContains('❤️', user1.displayName);
        await message1.expectReactionTooltipContains('❤️', user2.displayName);
      },
      { viewport: { width: 1280, height: 720 } }
    );
  });

  test('add a reaction via emoji picker search', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Emoji picker test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Open emoji picker from context menu, search for "rocket", click it
    await message.reactViaEmojiPicker('rocket', 'rocket');

    // Verify the reaction appears
    await message.expectReaction('🚀', 1);
  });

  test('add a reaction with a Unicode 14.0+ emoji via picker', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Unicode 14 emoji test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Search for "melting" and react with 🫠 (melting_face, Unicode 14.0)
    await message.reactViaEmojiPicker('melting', 'melting_face');

    // Verify the reaction appears
    await message.expectReaction('🫠', 1);
  });

  test('add a reaction via the meta bar picker button', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Meta bar picker test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Add a quick reaction so the meta bar appears
    await message.react('👍');
    await message.expectReaction('👍', 1);

    // Use the "Add reaction" button in the meta bar to open the emoji picker
    await message.reactViaMetaBarPicker('rocket', 'rocket');

    // Verify both reactions appear
    await message.expectReaction('👍', 1);
    await message.expectReaction('🚀', 1);
  });

  test('add a reaction via meta bar picker when only thread replies are showing', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Thread-only meta bar test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    // Create a thread reply so the meta bar shows the replies button
    await message.openThread();
    await roomPage.postThreadReply(`Reply ${Date.now()}`);

    // Close the thread pane
    await page.keyboard.press('Escape');

    // The meta bar should now show "1 reply" — and the Add reaction button
    await message.reactViaMetaBarPicker('star', 'star');

    // Verify the reaction appears
    await message.expectReaction('⭐', 1);
  });

  test('emoji picker closes after selecting an emoji', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testMessage = `Picker close test ${Date.now()}`;
    const message = await roomPage.sendMessage(testMessage);

    await message.reactViaEmojiPicker('fire', 'fire');

    // Verify picker is dismissed
    const picker = page.locator('input[placeholder="Search emojis..."]');
    await expect(picker).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Verify the reaction was added
    await message.expectReaction('🔥', 1);
  });
});
