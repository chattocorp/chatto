import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import { TIMEOUTS } from './constants';

test.describe('Up arrow to edit last message', () => {
  test('pressing up arrow on empty input edits last message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Edit me with up arrow ${Date.now()}`;
    await roomPage.sendMessage(originalMessage);

    // Make sure input is empty and focused
    await roomPage.messageInput.clear();
    await roomPage.messageInput.focus();

    // Press up arrow
    await roomPage.pressUpArrow();

    // Should enter edit mode with the message pre-filled
    await roomPage.expectEditModeActive();
    await expect(roomPage.composer).toHaveText(originalMessage);
  });

  test('pressing up arrow with text in input does NOT start editing', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Should not edit ${Date.now()}`;
    await roomPage.sendMessage(originalMessage);

    // Type something in the input (not empty)
    const typedText = 'Some text in progress';
    await roomPage.messageInput.fill(typedText);

    // Press up arrow
    await roomPage.pressUpArrow();

    // Should NOT enter edit mode - input should still have the typed text
    await roomPage.expectEditModeInactive();
    await expect(roomPage.messageInput).toHaveText(typedText);
  });

  test('up arrow only edits own messages, not other users messages', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    const user1Message = `User1 message ${Date.now()}`;
    await roomPage.sendMessage(user1Message);

    // User 2: Join and post their own message
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // User 2 posts a message
      const user2Message = `User2 message ${Date.now()}`;
      await roomPage2.sendMessage(user2Message);

      // User 1 should see both messages
      await roomPage.expectMessageVisible(user2Message);

      // User 1 presses up arrow - should edit THEIR message, not User 2's
      await roomPage.messageInput.clear();
      await roomPage.messageInput.focus();
      await roomPage.pressUpArrow();

      await roomPage.expectEditModeActive();
      // Should have User 1's message, not User 2's
      await expect(roomPage.composer).toHaveText(user1Message);
    } finally {
      await context2.close();
    }
  });

  test('pressing up arrow in thread input edits last thread reply', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a root message and open its thread
    const rootMessage = `Thread root ${Date.now()}`;
    const message = await roomPage.sendMessage(rootMessage);
    await message.openThread();
    await roomPage.expectThreadPaneVisible();

    // Post a reply in the thread
    const replyMessage = `Thread reply to edit ${Date.now()}`;
    await roomPage.postThreadReply(replyMessage);

    // Wait for thread input to be editable, then clear and focus
    await expect(roomPage.threadReplyInput).toHaveAttribute('contenteditable', 'true', {
      timeout: TIMEOUTS.UI_STANDARD
    });
    await roomPage.threadReplyInput.clear();
    await roomPage.threadReplyInput.focus();

    // Press up arrow in the thread input
    await roomPage.pressThreadUpArrow();

    // Should enter edit mode in the thread pane with the reply pre-filled
    await roomPage.expectThreadEditModeActive();
    await expect(roomPage.threadReplyInput).toHaveText(replyMessage);
  });
});

test.describe('Message editing', () => {
  test('chat input auto-resizes immediately when entering edit mode', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a multi-line message that will require the editor to expand
    const multilineMessage = `Line 1\nLine 2\nLine 3\nLine 4\nLine 5`;
    await roomPage.sendMessage(multilineMessage);

    // Clear input and get baseline height
    await roomPage.messageInput.clear();
    const baselineHeight = await roomPage.getComposerHeight();

    // Enter edit mode via up arrow
    await roomPage.messageInput.focus();
    await roomPage.pressUpArrow();
    await roomPage.expectEditModeActive();

    // The editor should immediately resize to fit the multi-line content
    await roomPage.expectComposerResized();

    // Verify the height is greater than baseline
    const editHeight = await roomPage.getComposerHeight();
    expect(editHeight).toBeGreaterThan(baselineHeight);
  });

  test('chat input auto-resizes when editing via edit button', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a multi-line message
    const multilineMessage = `First line\nSecond line\nThird line\nFourth line`;
    const message = await roomPage.sendMessage(multilineMessage);

    // Get baseline height with empty input
    await roomPage.messageInput.clear();
    const baselineHeight = await roomPage.getComposerHeight();

    // Enter edit mode via edit button
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // The editor should immediately resize
    await roomPage.expectComposerResized();
    const editHeight = await roomPage.getComposerHeight();
    expect(editHeight).toBeGreaterThan(baselineHeight);
  });

  test('user can edit their own message', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Original message ${Date.now()}`;
    const message = await roomPage.sendMessage(originalMessage);

    // Click edit button
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // The input should be pre-filled with the original message
    await expect(roomPage.composer).toHaveText(originalMessage);

    // Edit the message
    const editedMessage = `Edited message ${Date.now()}`;
    await roomPage.completeEdit(editedMessage);

    // Verify the edited message appears and original is gone
    await roomPage.expectMessageVisible(editedMessage);
    await roomPage.expectMessageNotVisible(originalMessage);

    // Verify "(edited)" indicator appears
    const editedMsg = roomPage.getMessage(editedMessage);
    await editedMsg.expectEdited();
  });

  test('user can cancel editing with Escape', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Cancel edit test ${Date.now()}`;
    const message = await roomPage.sendMessage(originalMessage);

    // Click edit button
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // Press Escape to cancel
    await roomPage.cancelEditWithEscape();
    await roomPage.expectEditModeInactive();

    // Original message should still be there
    await roomPage.expectMessageVisible(originalMessage);
  });

  test('user can cancel editing with Cancel button', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Cancel button test ${Date.now()}`;
    const message = await roomPage.sendMessage(originalMessage);

    // Click edit button
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // Click Cancel button
    await roomPage.cancelEditWithButton();
    await roomPage.expectEditModeInactive();
  });

  test('edited message updates for other connected clients in real-time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // User 1: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Real-time edit test ${Date.now()}`;
    const message1 = await roomPage.sendMessage(originalMessage);
    const eventId = await message1.getEventId();

    // User 2: Create user and join space
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // User 2 should see the original message
      await roomPage2.expectMessageVisible(originalMessage);

      // User 1: Edit the message
      await message1.startEdit();
      await roomPage.expectEditModeActive();

      const editedMessage = `Edited by user1 ${Date.now()}`;
      await roomPage.completeEdit(editedMessage);

      // User 1 should see the edited message
      await roomPage.expectMessageVisible(editedMessage);
      await roomPage.expectMessageNotVisible(originalMessage);

      // User 2 should also see the edited message via LiveEvent
      if (eventId) {
        const message2 = roomPage2.getMessageByEventId(eventId);
        await expect(message2.locator.getByText(editedMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
        await message2.expectEdited();
      }

      // User 1's message should also show edited indicator
      if (eventId) {
        const message1BySeq = roomPage.getMessageByEventId(eventId);
        await message1BySeq.expectEdited();
      }
    } finally {
      await context2.close();
    }
  });

  test('new messages continue to appear after editing a message', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // This test verifies the fix for a bug where the MessageUpdatedEvent was being
    // added to the room cache, which could corrupt the event list and break
    // the subscription for subsequent events.

    // User 1: Create space and post a message
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const originalMessage = `Message to edit ${Date.now()}`;
    const message = await roomPage.sendMessage(originalMessage);

    // User 2: Create user and join space
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);

      const chatPage2 = new ChatPage(page2);
      const roomPage2 = new RoomPage(page2);
      const explorePage2 = new ExplorePage(page2);

      await chatPage2.goto();
      await chatPage2.goToExploreSpaces();
      await explorePage2.joinSpace(spaceName);
      await chatPage2.enterRoom('general');

      // User 2 should see the original message
      await roomPage2.expectMessageVisible(originalMessage);

      // User 1: Edit the message
      await message.startEdit();
      await roomPage.expectEditModeActive();

      const editedMessage = `Edited message ${Date.now()}`;
      await roomPage.completeEdit(editedMessage);

      // Both users should see the edited message
      await roomPage.expectMessageVisible(editedMessage);
      await roomPage2.expectMessageVisible(editedMessage);

      // KEY TEST: User 2 sends a new message AFTER the edit
      const newMessage = `New message after edit ${Date.now()}`;
      await roomPage2.sendMessage(newMessage);

      // User 1 should see the new message in real-time (without page reload)
      // This is the bug we're testing for - previously the subscription would
      // stop working after receiving a MessageUpdatedEvent
      await roomPage.expectMessageVisible(newMessage);
    } finally {
      await context2.close();
    }
  });

  test('edit mode clears when deleting the message being edited', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message
    const messageText = `Delete while editing ${Date.now()}`;
    const message = await roomPage.sendMessage(messageText);

    // Start editing it
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // Delete the same message
    await message.delete();

    // Edit mode should be cleared, not stuck
    await roomPage.expectEditModeInactive();

    // Input should be empty (not still containing the deleted message text)
    await roomPage.expectMainInputEmpty();

    // Original message text should no longer be visible (deleted)
    await roomPage.expectMessageNotVisible(messageText);
  });

  test('attachment button is hidden during edit mode', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Attach button should be visible in normal mode
    await expect(roomPage.attachButton).toBeVisible();

    // Post a message, then enter edit mode
    const messageText = `Edit no attach ${Date.now()}`;
    const message = await roomPage.sendMessage(messageText);
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // Attach button should be hidden in edit mode
    await expect(roomPage.attachButton).not.toBeVisible();

    // Cancel edit — attach button should reappear
    await roomPage.cancelEditWithEscape();
    await roomPage.expectEditModeInactive();
    await expect(roomPage.attachButton).toBeVisible();
  });

  test('editing a multi-line message does not double blank lines', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Regression test: blank lines used to double on each edit-save cycle
    // because plainTextToHtml emitted <p><br></p> for empty lines, and the
    // HardBreak's renderText contributed an extra '\n' on top of the block
    // separator when getText() serialized the doc back to plain text.

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Compose "line1" + blank line + "line2". Shift+Enter inserts a hard
    // break in the composer (Enter alone submits the message).
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.click();
    await page.keyboard.type('line1');
    await page.keyboard.press('Shift+Enter');
    await page.keyboard.press('Shift+Enter');
    await page.keyboard.type('line2');
    await roomPage.messageInput.press('Enter');

    const message = roomPage.getMessage('line1');
    await expect(message.locator).toBeVisible();

    // Snapshot the composer's visible state: rendered text, rendered height,
    // and paragraph count. With the Markdown-backed composer, a blank line may
    // be represented as two paragraphs rather than an empty paragraph between
    // them; stability across edit cycles is the regression signal here.
    const composerSnapshot = async () => ({
      text: await roomPage.composer.evaluate((el: HTMLElement) => el.innerText),
      height: await roomPage.getComposerHeight(),
      paragraphs: await roomPage.composer.locator('p').count()
    });

    // Each edit cycle: open the editor, snapshot it, then save. The no-op
    // change (Ctrl/Cmd+End to ensure cursor is at end of doc, then type a
    // char and delete it) is essential: it forces the composer's `message`
    // state to refresh from the editor's getText() output, which is the
    // round-trip the bug lived in. Saving without any change would send the
    // original body verbatim and bypass the bug.
    const isMac = process.platform === 'darwin';
    const docEnd = isMac ? 'Meta+ArrowDown' : 'Control+End';
    const cycleAndSnapshot = async () => {
      await message.startEdit();
      await roomPage.expectEditModeActive();
      const snapshot = await composerSnapshot();
      await page.keyboard.press(docEnd);
      await page.keyboard.type('x');
      await page.keyboard.press('Backspace');
      await roomPage.composer.press('Enter');
      await roomPage.expectEditModeInactive();
      return snapshot;
    };

    const firstEditState = await cycleAndSnapshot();
    // Sanity-check the initial render preserves the blank line semantically.
    expect(firstEditState.text.replace(/[ \t]+\n/g, '\n').replace(/\n+$/, '')).toBe(
      'line1\n\nline2'
    );
    expect(firstEditState.paragraphs).toBeGreaterThanOrEqual(2);

    // Re-enter edit mode twice more. Pre-fix, each cycle would grow the
    // composer state (paragraph count, height, rendered text) because every
    // round-trip through plainTextToHtml + getText doubled the blank lines.
    for (let i = 0; i < 2; i++) {
      const state = await cycleAndSnapshot();
      expect(state).toEqual(firstEditState);
    }
  });

  test('pending file attachments are cleared when entering edit mode', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Post a message first
    const messageText = `Edit clears attach ${Date.now()}`;
    const message = await roomPage.sendMessage(messageText);

    // Stage a file attachment via the hidden file input (after sending)
    await roomPage.selectAttachment('e2e/fixtures/brighton.jpg');
    await expect(roomPage.attachmentPreview).toBeVisible();

    // Enter edit mode on the message we sent
    await message.startEdit();
    await roomPage.expectEditModeActive();

    // Attachment preview should be gone (cleared on edit mode entry)
    await expect(roomPage.attachmentPreview).not.toBeVisible();
  });
});
