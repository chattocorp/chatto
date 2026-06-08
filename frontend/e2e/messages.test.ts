import { expect } from '@playwright/test';
import { TIMEOUTS } from './constants';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ChatPage, RoomPage, ExplorePage } from './pages';
import * as routes from './routes';

test('consecutive messages from same user are grouped', async ({ page, chatPage, roomPage }) => {
  const testUser = await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const timestamp = Date.now();

  // Post three messages
  const message1 = `First message ${timestamp}`;
  const message2 = `Second message ${timestamp}`;
  const message3 = `Third message ${timestamp}`;

  await roomPage.sendMessage(message1);
  await roomPage.sendMessage(message2);
  await roomPage.sendMessage(message3);

  // Verify grouping: only ONE avatar should be visible for these messages
  await roomPage.expectAvatarCount(1);

  // Verify the display name appears only once (in the first message header)
  await roomPage.expectUserHeaderCount(testUser.displayName, 1);

  // Verify all three messages are visible
  await roomPage.expectMessageVisible(message1);
  await roomPage.expectMessageVisible(message2);
  await roomPage.expectMessageVisible(message3);
});

test('deleting first message in group leaves a tombstone as the group leader', async ({
  page,
  chatPage,
  roomPage
}) => {
  const testUser = await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const timestamp = Date.now();

  // Post three messages (they'll be grouped)
  const message1 = `First ${timestamp}`;
  const message2 = `Second ${timestamp}`;
  const message3 = `Third ${timestamp}`;

  const msg1 = await roomPage.sendMessage(message1);
  const msg1EventId = await msg1.getEventId();
  await roomPage.sendMessage(message2);
  await roomPage.sendMessage(message3);

  // Verify initial grouping: only ONE avatar
  await roomPage.expectAvatarCount(1);
  await roomPage.expectUserHeaderCount(testUser.displayName, 1);

  // Delete the first message (the group leader). The tombstone takes its place
  // and remains the group leader — avatar/header stay attached to it, so the
  // count is unchanged.
  await msg1.delete();
  if (msg1EventId) {
    await roomPage.getMessageByEventId(msg1EventId).expectDeleted();
  }

  await roomPage.expectAvatarCount(1);
  await roomPage.expectUserHeaderCount(testUser.displayName, 1);

  // Both remaining messages should still be visible
  await roomPage.expectMessageVisible(message2);
  await roomPage.expectMessageVisible(message3);
});

test('day separator appears for first message', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Test message ${Date.now()}`;
  await roomPage.sendMessage(testMessage);

  // Verify day separator is visible (should show "Today")
  await roomPage.expectDaySeparator('Today');
});

test('post message with image attachment', async ({ page, chatPage, roomPage }) => {
  const testUser = await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Image attachment test ${Date.now()}`;
  const message = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', testMessage);

  // Verify the message and attachment appear
  await roomPage.expectMessageVisible(testMessage);
  await message.expectAttachment();
  await expect(
    page.locator('[role="article"]').getByRole('button', { name: testUser.displayName })
  ).toBeVisible();
});

test('image attachment refreshes URL after an expired lazy-load request', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  let failedFirstAssetRequest = false;
  let refreshQueryCount = 0;

  await page.route('**/api/graphql', async (route) => {
    if (route.request().postData()?.includes('RefreshMessageAttachmentUrls')) {
      refreshQueryCount += 1;
    }
    await route.continue();
  });

  await page.route('**/assets/files/**', async (route) => {
    if (!failedFirstAssetRequest && route.request().resourceType() === 'image') {
      failedFirstAssetRequest = true;
      await route.fulfill({
        status: 403,
        contentType: 'text/plain',
        body: 'expired asset access ticket'
      });
      return;
    }
    await route.continue();
  });

  await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', 'Expired lazy image');

  await expect.poll(() => refreshQueryCount, { timeout: TIMEOUTS.UI_STANDARD }).toBeGreaterThan(0);
  await expect
    .poll(
      async () =>
        roomPage.attachmentImage
          .first()
          .evaluate((img) => img.complete && img.naturalWidth > 0),
      { timeout: TIMEOUTS.COMPLEX_OPERATION }
    )
    .toBe(true);
});

test('can post message with attachment but no text', async ({ page, chatPage, roomPage }) => {
  const testUser = await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Send attachment-only message
  const message = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');

  // The attachment should appear
  await message.expectAttachment();
  await expect(
    page.locator('[role="article"]').getByRole('button', { name: testUser.displayName })
  ).toBeVisible();
});

test('image attachment respects container width on narrow viewport', async ({
  page,
  chatPage,
  roomPage
}) => {
  // Setup at normal viewport (sidebar needs space to be visible)
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Upload an image (1024px wide, should be constrained)
  await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');

  // Now resize to mobile viewport to test responsive behavior
  await page.setViewportSize({ width: 375, height: 667 });

  // Get the image and its container
  const image = roomPage.attachmentImage;
  const container = page.locator('[role="article"]').filter({ has: image });

  // Verify image width doesn't exceed container
  const imageBox = await image.boundingBox();
  const containerBox = await container.boundingBox();

  expect(imageBox).not.toBeNull();
  expect(containerBox).not.toBeNull();
  expect(imageBox!.width).toBeLessThanOrEqual(containerBox!.width);
});

test('room scrolls to bottom on load even with slow-loading images', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Send a message with an image attachment
  await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', 'Test image for scroll');

  // Send a few more text messages so the image isn't the last message
  await roomPage.sendMessage('Message after image 1');
  await roomPage.sendMessage('Message after image 2');
  await roomPage.sendMessage('Final message');

  // Set up route handler to delay image loading significantly
  await page.route('**/assets/**', async (route) => {
    const request = route.request();
    if (request.resourceType() === 'image') {
      // Delay image responses by 2 seconds
      await new Promise((r) => setTimeout(r, 2000));
    }
    await route.continue();
  });

  // Reload the page to test initial scroll behavior
  await page.reload();

  // Wait for messages to render (but images should still be loading due to delay)
  await roomPage.expectMessageVisible('Final message');

  // Check scroll position immediately - should be at bottom due to aspect-ratio reserving space
  const isAtBottom = await page.evaluate(() => {
    const container = document.querySelector('[data-testid="messages-container"]');
    if (!container) return false;
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight;
    return distanceFromBottom < 50; // Allow small margin
  });

  expect(isAtBottom).toBe(true);
});

test('cannot post message with neither text nor attachments', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Try to submit with empty text and no attachments
  await roomPage.submitEmpty();

  // Verify that the input is still focused (submit was rejected)
  await expect(roomPage.messageInput).toBeFocused();

  // No message should appear in the room (check over time to be sure)
  await expect.poll(async () => await roomPage.messages.count(), { timeout: TIMEOUTS.UI_FAST }).toBe(0);
});

test('send button is visible', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  await expect(roomPage.sendButton).toBeVisible();
});

test('send button is disabled when input is empty', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Ensure input is empty
  await expect(roomPage.messageInput).toHaveText('');

  // Send button should be disabled
  await expect(roomPage.sendButton).toBeDisabled();
});

test('send button is enabled when input has text', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Type some text
  await roomPage.messageInput.fill('Hello world');

  // Send button should be enabled
  await expect(roomPage.sendButton).toBeEnabled();
});

test('can send message by clicking send button', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Send button test ${Date.now()}`;
  await roomPage.sendMessageWithButton(testMessage);

  // Verify the message appears
  await roomPage.expectMessageVisible(testMessage);
});

test('user can delete their own message', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Delete test ${Date.now()}`;
  const message = await roomPage.sendMessage(testMessage);

  // Get event ID for stable lookup after deletion
  const eventId = await message.getEventId();

  // Delete the message
  await message.delete();

  // Deleted message should show the tombstone (original text gone, placeholder in place)
  await roomPage.expectMessageNotVisible(testMessage);
  if (eventId) {
    const deletedMessage = roomPage.getMessageByEventId(eventId);
    await deletedMessage.expectDeleted();
  }
});

test('user can cancel deleting a message', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Cancel delete test ${Date.now()}`;
  const message = await roomPage.sendMessage(testMessage);

  // Try to delete but cancel
  await message.cancelDelete();

  // Message should still be visible
  await roomPage.expectMessageVisible(testMessage);
});

test('deleted message disappears for other connected clients in real-time', async ({
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

  const testMessage = `Real-time delete test ${Date.now()}`;
  const message1 = await roomPage.sendMessage(testMessage);
  const eventId = await message1.getEventId();

  // User 2: Create user and join space
  const context2 = await browser!.newContext({
    baseURL: serverURL,
    viewport: { width: 1280, height: 720 }
  });
  const page2 = await context2.newPage();

  try {
    await createAndLoginTestUser(page2);

    // Post-#330 PR(a): signup auto-joins the server space; the Browse Spaces
    // UI is gone, so user 2 just navigates to the chat root and clicks into
    // the room.
    void spaceName;
    await page2.goto(routes.chat);
    await page2.waitForURL(routes.patterns.spaceOrRoom);

    // User 2 enters the general room (may already be there due to redirect)
    const generalRoomLink = page2.locator('.room-list').getByRole('link', { name: '# general' });
    await expect(generalRoomLink).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await generalRoomLink.click();
    await page2.waitForURL(routes.patterns.anyRoom);

    // User 2 should see the message
    await expect(page2.getByText(testMessage)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // User 1: Delete the message
    await message1.delete();

    // User 1: deleted message should show the tombstone
    await roomPage.expectMessageNotVisible(testMessage);
    if (eventId) {
      const message1AfterDelete = roomPage.getMessageByEventId(eventId);
      await message1AfterDelete.expectDeleted();
    }

    // User 2: should also see the tombstone arrive via LiveEvent
    if (eventId) {
      const message2AfterDelete = page2.locator(`[data-event-id="${eventId}"]`);
      await expect(
        message2AfterDelete.getByText('This message has been deleted')
      ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(page2.getByText(testMessage)).not.toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    }
  } finally {
    await context2.close();
  }
});

test('deleted attachment-only message shows placeholder', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Send attachment-only message
  const message = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');
  const eventId = await message.getEventId();

  // Delete the message
  await message.delete();

  // Deleted attachment-only message should show the tombstone
  if (eventId) {
    const messageAfterDelete = roomPage.getMessageByEventId(eventId);
    await messageAfterDelete.expectDeleted();
  }
});

test('deleting attachment-only message in group does not mark text message as edited', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const timestamp = Date.now();

  // Post a text message first
  const textMessage = `Text message ${timestamp}`;
  await roomPage.sendMessage(textMessage);

  // Post an attachment-only message (should be grouped with text message)
  const attachmentMsg = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');
  const attachmentEventId = await attachmentMsg.getEventId();

  // Delete the attachment-only message
  await attachmentMsg.delete();

  // Deleted attachment-only message should show the tombstone
  if (attachmentEventId) {
    const messageAfterDelete = roomPage.getMessageByEventId(attachmentEventId);
    await messageAfterDelete.expectDeleted();
  }

  // Verify the text message still exists and is NOT marked as edited
  // Re-fetch the message locator after DOM updates from deletion
  await roomPage.expectMessageVisible(textMessage);
  const textMsgAfterDelete = roomPage.getMessage(textMessage);
  await textMsgAfterDelete.expectNotEdited();
});

test('removing attachment from attachment-only message hides it', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Send attachment-only message (no text body)
  const message = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');
  const eventId = await message.getEventId();

  // Verify attachment is visible
  await message.expectAttachment();

  // Remove the attachment (not delete the whole message)
  await message.deleteAttachment();

  // Message with no body and no attachments should show the deleted-tombstone
  if (eventId) {
    const messageAfterRemove = roomPage.getMessageByEventId(eventId);
    await messageAfterRemove.expectDeleted();
  }
});

test('deleted message with reactions remains visible', async ({ page, chatPage, roomPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Delete with reaction ${Date.now()}`;
  const message = await roomPage.sendMessage(testMessage);
  const eventId = await message.getEventId();

  // Add a reaction before deleting
  await message.reactViaToolbar('👍');
  await message.expectReaction('👍', 1);

  // Delete the message
  await message.delete();

  // Message should still be visible with "This message has been deleted" because it has a reaction
  await roomPage.expectMessageNotVisible(testMessage);
  if (eventId) {
    const deletedMessage = roomPage.getMessageByEventId(eventId);
    await deletedMessage.expectDeleted();
    await deletedMessage.expectReaction('👍', 1);
  }
});

test('deletion of a reacted message shows placeholder for other connected clients in real-time', async ({
  page,
  chatPage,
  roomPage,
  browser,
  serverURL
}) => {
  // User 1 posts a message; User 2 reacts to it; User 1 deletes it.
  // User 2 (already viewing the room) should see the original body replaced
  // by the "This message has been deleted" placeholder while the reaction stays visible —
  // this is the propagation case the single-user delete-with-reaction test
  // doesn't cover and that the previous refetch-only path failed silently on.
  await createAndLoginTestUser(page);
  await chatPage.goto();
  const spaceName = await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Real-time delete with reaction ${Date.now()}`;
  const message1 = await roomPage.sendMessage(testMessage);
  const eventId = await message1.getEventId();
  if (!eventId) throw new Error('expected eventId from sent message');

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
    await roomPage2.expectMessageVisible(testMessage);

    // User 2 reacts so the deletion can't take the "fully hidden" path.
    const message2 = roomPage2.getMessageByEventId(eventId);
    await message2.react('👍');
    await message2.expectReaction('👍', 1);

    // User 1 deletes their own message.
    await message1.delete();

    // User 2 must see the placeholder + reaction without a refresh.
    await message2.expectDeleted();
    await message2.expectReaction('👍', 1);
    await expect(message2.locator.getByText(testMessage)).not.toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  } finally {
    await context2.close();
  }
});

test('deleted message with thread replies remains visible', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  const testMessage = `Delete with thread ${Date.now()}`;
  const message = await roomPage.sendMessage(testMessage);
  const eventId = await message.getEventId();

  // Open thread and post a reply
  await message.openThread();
  await roomPage.expectThreadRouteActive();
  const replyText = `Thread reply ${Date.now()}`;
  await roomPage.postThreadReply(replyText);
  await roomPage.expectTextInThreadPane(replyText);
  await roomPage.closeThread();
  await roomPage.expectThreadRouteClosed();

  // Wait for thread indicator to appear
  await expect(page.getByText('1 reply')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

  // Delete the root message
  await message.delete();

  // Message should still be visible with "This message has been deleted" because it has thread replies
  await roomPage.expectMessageNotVisible(testMessage);
  if (eventId) {
    const deletedMessage = roomPage.getMessageByEventId(eventId);
    await deletedMessage.expectDeleted();
  }
});

test('image lightbox supports keyboard navigation with multiple images', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Upload two images in a single message
  await roomPage.fileInput.setInputFiles([
    'e2e/fixtures/brighton.jpg',
    'e2e/fixtures/brighton2.jpg'
  ]);

  // Wait for both attachment previews to appear
  await expect(roomPage.attachmentPreview).toHaveCount(2);

  // Send the message
  await roomPage.messageInput.press('Enter');

  // Wait for both attachment images to appear in the message
  await expect(roomPage.attachmentImage).toHaveCount(2, { timeout: TIMEOUTS.COMPLEX_OPERATION });

  // Click the first image to open the lightbox
  await roomPage.attachmentImage.first().click();

  // Verify lightbox is open with counter showing "1 / 2"
  const dialog = page.locator('dialog[open]');
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText('1 / 2')).toBeVisible();

  // Verify the "brighton.jpg" filename is shown
  await expect(dialog.getByText('brighton.jpg')).toBeVisible();

  // Press ArrowRight to go to the next image
  await page.keyboard.press('ArrowRight');
  await expect(dialog.getByText('2 / 2')).toBeVisible();
  await expect(dialog.getByText('brighton2.jpg')).toBeVisible();

  // Press ArrowRight again to wrap around to the first image
  await page.keyboard.press('ArrowRight');
  await expect(dialog.getByText('1 / 2')).toBeVisible();
  await expect(dialog.getByText('brighton.jpg')).toBeVisible();

  // Press ArrowLeft to wrap backwards to the last image
  await page.keyboard.press('ArrowLeft');
  await expect(dialog.getByText('2 / 2')).toBeVisible();
  await expect(dialog.getByText('brighton2.jpg')).toBeVisible();

  // Verify navigation buttons are present
  await expect(dialog.getByRole('button', { name: 'Previous image' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Next image' })).toBeVisible();

  // Click the "Next image" button
  await dialog.getByRole('button', { name: 'Next image' }).click();
  await expect(dialog.getByText('1 / 2')).toBeVisible();

  // Close with Escape
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
});

test('image lightbox does not show navigation for single image', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.createSpace();
  await chatPage.enterRoom('general');

  // Upload a single image
  await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');

  // Click the image to open the lightbox
  await roomPage.attachmentImage.click();

  // Verify lightbox is open
  const dialog = page.locator('dialog[open]');
  await expect(dialog).toBeVisible();

  // Verify no navigation controls are shown
  await expect(dialog.getByRole('button', { name: 'Previous image' })).not.toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Next image' })).not.toBeVisible();

  // Verify no counter is shown
  await expect(dialog.getByText(/\d+ \/ \d+/)).not.toBeVisible();

  // Close with Escape
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
});

test.describe('image lightbox back button and tap behavior', () => {
  test('closes lightbox with browser back and stays on room page', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');

    // Remember the URL before opening lightbox
    const roomUrl = page.url();

    await roomPage.attachmentImage.click();
    const dialog = page.locator('dialog[open]');
    await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Press browser back
    await page.goBack();

    // Lightbox should close
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Should still be on the same room page
    expect(page.url()).toBe(roomUrl);
  });

  test('closes lightbox by clicking backdrop', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');
    await roomPage.attachmentImage.click();

    const dialog = page.locator('dialog[open]');
    await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Click the dialog backdrop (top-left corner, outside the image content)
    await dialog.click({ position: { x: 5, y: 5 } });
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  });
});

test.describe('Message link rendering', () => {
  test('long URLs do not overflow the message container', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Send a message with a very long URL that would overflow without wrapping
    const longUrl =
      'https://example.com/very/long/path/that/keeps/going/and/going/with/many/segments/to/ensure/it/exceeds/the/container/width/completely';
    await roomPage.messageInput.fill(longUrl);
    await roomPage.sendButton.click();

    // Wait for the link to render
    const link = page.locator('.prose a[href]').first();
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The link should not overflow its prose container
    const overflows = await link.evaluate((el) => {
      const prose = el.closest('.prose');
      if (!prose) return true;
      const proseRect = prose.getBoundingClientRect();
      const linkRect = el.getBoundingClientRect();
      return linkRect.right > proseRect.right + 1; // 1px tolerance
    });

    expect(overflows).toBe(false);
  });
});
