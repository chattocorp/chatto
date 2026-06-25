import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';
import { MessageComponent } from './MessageComponent';

/**
 * Page object for room interactions (message sending, attachments).
 */
export class RoomPage {
  constructor(readonly page: Page) {}

  /** The message input field (TipTap contenteditable editor) */
  get messageInput(): Locator {
    return this.page.getByTestId('message-input');
  }

  /** The composer (alias for messageInput - used for editing) */
  get composer(): Locator {
    return this.messageInput;
  }

  /** The "Editing message" indicator */
  get editingIndicator(): Locator {
    return this.page.getByText('Editing message');
  }

  /** The file input for attachments (hidden, use setInputFiles) */
  get fileInput(): Locator {
    return this.page.locator('input[type="file"]');
  }

  /** Attachment preview (shown when file is selected before sending) */
  get attachmentPreview(): Locator {
    return this.page.locator('img.h-16.w-16');
  }

  /** The attach file button */
  get attachButton(): Locator {
    return this.page.getByTitle('Attach file');
  }

  /** The send button */
  get sendButton(): Locator {
    return this.page.getByRole('button', { name: 'Send message' });
  }

  /** Video attachment preview in the composer (shown when a video file is staged) */
  get videoAttachmentPreview(): Locator {
    return this.page.getByTestId('video-attachment-preview');
  }

  /** Audio attachment preview in the composer (shown when an audio file is staged) */
  get audioAttachmentPreview(): Locator {
    return this.page.getByTestId('audio-attachment-preview');
  }

  /** Audio player in a posted message */
  get audioPlayer(): Locator {
    return this.page.getByTestId('audio-player');
  }

  /** Vidstack <media-player> element (visible after video processing completes) */
  get mediaPlayer(): Locator {
    return this.page.locator('media-player');
  }

  /** Vidstack media controls bar */
  get mediaControls(): Locator {
    return this.page.locator('media-player media-controls');
  }

  /** Vidstack settings menu (should be hidden via CSS for chat embeds) */
  get videoSettingsMenu(): Locator {
    return this.page.locator('media-player .vds-settings-menu');
  }

  /** All message avatars (for checking message grouping) - scoped to message articles */
  get avatars(): Locator {
    // Match the avatar button itself (not child elements like presence dots)
    return this.page.locator('[role="article"] button.absolute');
  }

  /** Image attachment in a posted message */
  get attachmentImage(): Locator {
    return this.page.locator('button[aria-label^="View"] img');
  }

  /** All message articles */
  get messages(): Locator {
    return this.page.locator('[role="article"]');
  }

  /** The member count header showing "Members (N)" in the room info pane */
  get memberCount(): Locator {
    return this.page.getByText(/^Members \(\d+\)$/);
  }

  /** The member list inside the room extras pane */
  get memberList(): Locator {
    return this.page.locator('aside[aria-label="Room extras"] nav[aria-label="Members"]');
  }

  /**
   * Get a member's list item by their display name or login.
   */
  getMember(name: string): Locator {
    return this.memberList.locator('button.sidebar-item', { hasText: name });
  }

  /**
   * Get a member's display name element.
   */
  getMemberDisplayName(name: string): Locator {
    return this.getMember(name).locator('.min-w-0 > div').first();
  }

  /**
   * Get a member's username element (the @username line).
   */
  getMemberUsername(name: string): Locator {
    return this.getMember(name).locator('.min-w-0 > div').nth(1);
  }

  /**
   * Get a member's avatar image element.
   */
  getMemberAvatarImage(name: string): Locator {
    return this.getMember(name).locator('img');
  }

  /**
   * Get a member's avatar initials element.
   * Initials can be 1-2 characters (e.g., "J" or "JD" for "John Doe").
   */
  getMemberAvatarInitials(name: string): Locator {
    return this.getMember(name)
      .locator('div')
      .filter({ hasText: /^[A-Z]{1,2}$/ });
  }

  /**
   * Wait for the message input (TipTap editor) to be editable.
   * TipTap starts with contenteditable="false" and switches to "true" once initialized.
   */
  async waitForInputEditable(): Promise<void> {
    await expect(this.messageInput).toHaveAttribute('contenteditable', 'true', {
      timeout: TIMEOUTS.UI_STANDARD
    });
  }

  /**
   * Send a text message and wait for it to appear.
   * Returns a MessageComponent for the new message.
   */
  async sendMessage(text: string): Promise<MessageComponent> {
    await this.waitForInputEditable();
    await this.messageInput.fill(text);
    await this.dismissAutocompleteIfOpen(this.messageInput);
    await this.messageInput.press('Enter');
    const message = this.getMessage(text);
    await expect(message.locator).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    return message;
  }

  /**
   * Send a text message using the send button (instead of the keyboard shortcut).
   * Returns a MessageComponent for the new message.
   */
  async sendMessageWithButton(text: string): Promise<MessageComponent> {
    await this.waitForInputEditable();
    await this.messageInput.fill(text);
    await this.sendButton.click();
    const message = this.getMessage(text);
    await expect(message.locator).toBeVisible();
    return message;
  }

  /**
   * Select an attachment file (shows preview but doesn't send).
   */
  async selectAttachment(filePath: string): Promise<void> {
    await this.fileInput.setInputFiles(filePath);
    await expect(this.attachmentPreview).toBeVisible();
  }

  /**
   * Send an attachment with optional text.
   * Returns a MessageComponent for the new message.
   */
  async sendAttachment(filePath: string, text?: string): Promise<MessageComponent> {
    await this.selectAttachment(filePath);

    if (text) {
      await this.messageInput.fill(text);
    }
    await this.dismissAutocompleteIfOpen(this.messageInput);
    await this.messageInput.press('Enter');

    // Wait for attachment preview to clear (message sent)
    await expect(this.attachmentPreview).not.toBeVisible();

    // Wait for the message to appear with attachment
    await expect(this.attachmentImage).toBeVisible();

    // Return the message component
    if (text) {
      return this.getMessage(text);
    }
    // For attachment-only, find the message containing the attachment image
    const locator = this.page.locator('[role="article"]').filter({
      has: this.attachmentImage
    });
    return new MessageComponent(this.page, locator);
  }

  /**
   * Press the keyboard submit shortcut with empty input.
   */
  async submitEmpty(): Promise<void> {
    await this.messageInput.press('Control+Enter');
  }

  /**
   * Get a MessageComponent for a message containing the given text.
   */
  getMessage(text: string): MessageComponent {
    const locator = this.page.locator('[role="article"]', { hasText: text });
    return new MessageComponent(this.page, locator);
  }

  /**
   * Get a MessageComponent by its event ID attribute.
   */
  getMessageByEventId(eventId: string): MessageComponent {
    const locator = this.page.locator(`[data-event-id="${eventId}"]`);
    return new MessageComponent(this.page, locator);
  }

  /**
   * Get user header locators (for checking message grouping).
   */
  getUserHeaders(username: string): Locator {
    // Author names in message headers: <button> (active user) or <strong> (deleted user)
    return this.page.locator('[role="article"]').getByRole('button', { name: username });
  }

  // --- Assertions ---

  /**
   * Assert that a message with the given text is visible.
   */
  async expectMessageVisible(text: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.page.getByText(text)).toBeVisible(options);
  }

  /**
   * Assert that a message with the given text is NOT visible.
   */
  async expectMessageNotVisible(text: string): Promise<void> {
    await expect(this.page.getByText(text)).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Assert that the day separator (e.g., "Today") is visible.
   */
  async expectDaySeparator(text: string = 'Today'): Promise<void> {
    await expect(this.page.getByText(text)).toBeVisible();
  }

  /**
   * Assert that the "New messages" unread separator is visible.
   * Uses REALTIME_EVENT timeout by default since this is typically checked
   * after a message from another user arrives.
   */
  async expectUnreadSeparator(options?: { timeout?: number }): Promise<void> {
    await expect(this.page.getByText('New messages', { exact: true })).toBeVisible({
      timeout: options?.timeout ?? TIMEOUTS.REALTIME_EVENT
    });
  }

  /**
   * Assert that the "New messages" unread separator is NOT visible.
   */
  async expectNoUnreadSeparator(): Promise<void> {
    await expect(this.page.getByText('New messages', { exact: true })).not.toBeVisible();
  }

  /**
   * Assert that the "New messages" unread separator is visible in the thread pane.
   * Uses REALTIME_EVENT timeout by default since this is typically checked
   * after a message from another user arrives.
   */
  async expectUnreadSeparatorInThreadPane(options?: { timeout?: number }): Promise<void> {
    await expect(this.threadPane.getByText('New messages', { exact: true })).toBeVisible({
      timeout: options?.timeout ?? TIMEOUTS.REALTIME_EVENT
    });
  }

  /**
   * Assert that the "New messages" unread separator is NOT visible in the thread pane.
   */
  async expectNoUnreadSeparatorInThreadPane(): Promise<void> {
    await expect(this.threadPane.getByText('New messages', { exact: true })).not.toBeVisible();
  }

  /**
   * Assert the number of message avatars (for grouping tests).
   * Grouped messages share one avatar.
   */
  async expectAvatarCount(count: number): Promise<void> {
    await expect(this.avatars).toHaveCount(count);
  }

  /**
   * Assert the number of user headers (for grouping tests).
   * Grouped messages share one header.
   */
  async expectUserHeaderCount(username: string, count: number): Promise<void> {
    await expect(this.getUserHeaders(username)).toHaveCount(count);
  }

  /**
   * Assert that no messages are visible in the room.
   */
  async expectNoMessages(): Promise<void> {
    await expect(this.messages).not.toBeVisible();
  }

  // --- Member List Assertions ---

  /**
   * Get the "Online (N)" section header in the member list.
   * Uses getByRole so the accessible-name regex matches normalized text
   * (the surrounding whitespace from the chevron span isn't included).
   */
  get onlineSectionHeader(): Locator {
    return this.memberList.getByRole('button', { name: /^Online \(\d+\)$/ });
  }

  /**
   * Get the "Offline (N)" section header in the member list.
   */
  get offlineSectionHeader(): Locator {
    return this.memberList.getByRole('button', { name: /^Offline \(\d+\)$/ });
  }

  /**
   * Assert that a member is visible in the member list.
   */
  async expectMemberVisible(name: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getMember(name)).toBeVisible(options);
  }

  /**
   * Assert that a member has an avatar image (not initials).
   */
  async expectMemberHasAvatar(name: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getMemberAvatarImage(name)).toBeVisible(options);
  }

  /**
   * Assert that a member has initials (no avatar image).
   */
  async expectMemberHasInitials(name: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getMemberAvatarInitials(name)).toBeVisible(options);
    await expect(this.getMemberAvatarImage(name)).not.toBeVisible();
  }

  /**
   * Assert that a member's display name is shown correctly.
   */
  async expectMemberDisplayName(
    memberIdentifier: string,
    expectedDisplayName: string,
    options?: { timeout?: number }
  ): Promise<void> {
    await expect(this.getMemberDisplayName(memberIdentifier)).toHaveText(
      expectedDisplayName,
      options
    );
  }

  /**
   * Assert that a member's username is shown correctly (with @ prefix and muted style).
   */
  async expectMemberUsernameFormat(
    memberIdentifier: string,
    expectedLogin: string,
    options?: { timeout?: number }
  ): Promise<void> {
    const usernameElement = this.getMemberUsername(memberIdentifier);
    await expect(usernameElement).toHaveText(`@${expectedLogin}`, options);
    await expect(usernameElement).toHaveClass(/text-muted/);
  }

  /**
   * Get all member display names in the order they appear in the list.
   * Returns an array of display name strings.
   */
  async getMemberDisplayNamesInOrder(): Promise<string[]> {
    const memberItems = this.memberList.locator('button.sidebar-item');
    const count = await memberItems.count();
    const displayNames: string[] = [];

    for (let i = 0; i < count; i++) {
      const displayNameElement = memberItems.nth(i).locator('.min-w-0 > div').first();
      const text = await displayNameElement.textContent();
      if (text) {
        displayNames.push(text);
      }
    }

    return displayNames;
  }

  /**
   * Assert that members within each section (online/offline) are sorted alphabetically by display name.
   */
  async expectMembersSortedAlphabetically(): Promise<void> {
    const displayNames = await this.getMemberDisplayNamesInOrder();

    // Members are grouped by online/offline status, so we need to verify
    // alphabetical order within each group. Since online comes first,
    // we compare each adjacent pair within the same status group.
    // For simplicity, we verify the entire list is alphabetically sorted
    // since that's the expected behavior when all members have the same status.
    const sorted = [...displayNames].sort((a, b) =>
      a.localeCompare(b, undefined, { sensitivity: 'base' })
    );

    expect(displayNames).toEqual(sorted);
  }

  // --- Message Editing ---

  /**
   * Complete editing a message (fill new text and press the keyboard submit shortcut).
   * Assumes edit mode is already active.
   */
  async completeEdit(newText: string): Promise<void> {
    await this.composer.fill(newText);
    await this.composer.press('Enter');
    await expect(this.editingIndicator).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Press up arrow in the message input (for edit-last-message feature).
   */
  async pressUpArrow(): Promise<void> {
    await this.messageInput.press('ArrowUp');
  }

  /**
   * Press up arrow in the thread reply input (for edit-last-message feature).
   */
  async pressThreadUpArrow(): Promise<void> {
    await this.threadReplyInput.press('ArrowUp');
  }

  /**
   * Cancel editing by clicking the cancel button.
   * On desktop, this is the "Esc to cancel" button; on mobile, "Cancel".
   */
  async cancelEditWithButton(): Promise<void> {
    const desktopCancel = this.page.getByRole('button', { name: 'Esc to cancel' });
    const mobileCancel = this.page.getByRole('button', { name: 'Cancel', exact: true });
    if (await desktopCancel.isVisible()) {
      await desktopCancel.click();
    } else {
      await mobileCancel.click();
    }
    await expect(this.editingIndicator).not.toBeVisible();
  }

  /**
   * Assert that edit mode is active.
   */
  async expectEditModeActive(): Promise<void> {
    await expect(this.editingIndicator).toBeVisible();
  }

  /**
   * Assert that edit mode is not active.
   */
  async expectEditModeInactive(): Promise<void> {
    await expect(this.editingIndicator).not.toBeVisible();
  }

  /**
   * Get the current height of the composer in pixels.
   */
  async getComposerHeight(): Promise<number> {
    const height = await this.composer.evaluate((el) => el.getBoundingClientRect().height);
    return height;
  }

  /**
   * Assert that the composer has auto-resized to fit multi-line content.
   * The minimum height is 48px (min-h-12), so we check it's larger.
   * Uses polling to wait for the resize to complete.
   */
  async expectComposerResized(): Promise<void> {
    await expect
      .poll(() => this.getComposerHeight(), {
        message: 'Expected composer to resize above 48px',
        timeout: TIMEOUTS.UI_STANDARD
      })
      .toBeGreaterThan(48);
  }

  // --- Thread Pane ---

  /** The thread pane container */
  get threadPane(): Locator {
    return this.page.getByTestId('thread-pane');
  }

  /** The follow/unfollow bell button in the thread pane header */
  get threadFollowButton(): Locator {
    return this.threadPane.getByTitle(/(?:Un)?[Ff]ollow thread/);
  }

  /**
   * Toggle thread follow/unfollow in the thread pane header.
   */
  async toggleThreadFollow(): Promise<void> {
    await this.threadFollowButton.click();
  }

  /**
   * Assert the thread pane header shows "following" state.
   */
  async expectThreadPaneFollowing(): Promise<void> {
    await expect(this.threadPane.getByTitle('Unfollow thread')).toBeVisible();
  }

  /**
   * Assert the thread pane header shows "not following" state.
   */
  async expectThreadPaneNotFollowing(): Promise<void> {
    await expect(this.threadPane.getByTitle('Follow thread')).toBeVisible();
  }

  /** The thread reply input (TipTap contenteditable editor) */
  get threadReplyInput(): Locator {
    return this.page.getByTestId('thread-reply-input');
  }

  /**
   * Post a reply in the currently open thread.
   */
  async postThreadReply(text: string): Promise<void> {
    // Wait for thread reply input to be editable (TipTap may not be ready yet,
    // especially in multi-user tests where the user just navigated to the thread)
    await expect(this.threadReplyInput).toHaveAttribute('contenteditable', 'true', {
      timeout: TIMEOUTS.UI_STANDARD
    });
    await this.threadReplyInput.fill(text);
    await this.dismissAutocompleteIfOpen(this.threadReplyInput);
    await this.threadReplyInput.press('Enter');
    await expect(this.threadPane.getByText(text)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  }

  /**
   * Post a reply in the currently open thread with "Also send to channel" checkbox enabled.
   */
  async postThreadReplyWithEcho(text: string): Promise<void> {
    // Wait for thread reply input to be editable
    await expect(this.threadReplyInput).toHaveAttribute('contenteditable', 'true', {
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Check the "Also send to channel" checkbox
    const checkbox = this.page.getByLabel('Also send to channel');
    await expect(checkbox).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await checkbox.check();
    await expect(checkbox).toBeChecked();

    // Post the reply
    await this.threadReplyInput.fill(text);
    await this.dismissAutocompleteIfOpen(this.threadReplyInput);
    await this.threadReplyInput.press('Enter');
    // Wait for message to appear in thread pane specifically
    await expect(this.threadPane.getByText(text)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  }

  private async dismissAutocompleteIfOpen(input: Locator): Promise<void> {
    const popup = this.page
      .locator('[data-testid="mention-autocomplete"], [data-testid="emoji-autocomplete"]')
      .first();
    await popup.waitFor({ state: 'visible', timeout: 250 }).catch(() => undefined);
    if (await popup.isVisible()) {
      await input.press('Escape');
    }
  }

  /**
   * Get a MessageComponent for a message in the thread pane by text.
   */
  getThreadMessage(text: string): MessageComponent {
    const locator = this.threadPane.locator('[role="article"]', { hasText: text });
    return new MessageComponent(this.page, locator);
  }

  /**
   * The thread pane's edit input field (scoped to thread pane).
   */
  get threadEditingInput(): Locator {
    return this.threadPane.locator('[contenteditable="true"]').first();
  }

  /**
   * Close the thread pane using the back button.
   */
  async closeThread(): Promise<void> {
    await this.page.getByTitle('Back to room').click();
    await expect(this.page.getByRole('heading', { name: /^Thread in #/ })).not.toBeVisible();
  }

  /**
   * Assert that the thread pane is visible.
   */
  async expectThreadPaneVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: /^Thread in #/ })).toBeVisible();
  }

  /**
   * Assert that the URL contains a thread route.
   */
  async expectThreadRouteActive(threadId?: string): Promise<void> {
    if (threadId) {
      await this.page.waitForURL(new RegExp(`/[^/]+/${threadId}$`));
    } else {
      await this.page.waitForURL(routes.patterns.anyThread);
    }
  }

  /**
   * Assert that the URL does NOT contain a thread route (just room).
   */
  async expectThreadRouteClosed(): Promise<void> {
    // Room URLs have exactly 2 path segments after /chat/-/ (spaceId/roomId)
    // Thread URLs have 3 segments (spaceId/roomId/threadId)
    await expect(this.page).toHaveURL(routes.patterns.anyRoom);
  }

  /**
   * Navigate directly to a thread URL.
   */
  async gotoThread(roomId: string, threadId: string): Promise<void> {
    await this.page.goto(routes.thread(roomId, threadId));
  }

  /**
   * Assert that text is visible in the thread pane.
   */
  async expectTextInThreadPane(text: string): Promise<void> {
    await expect(this.threadPane.getByText(text)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Assert that text is NOT visible in the thread pane.
   */
  async expectTextNotInThreadPane(text: string): Promise<void> {
    await expect(this.threadPane.getByText(text)).not.toBeVisible();
  }

  // --- Thread Edit State (scoped to thread pane only) ---

  /** All editing indicators on the page (should be 0 or 1 when properly isolated) */
  get allEditingIndicators(): Locator {
    return this.page.getByText('Editing message');
  }

  /** The thread pane's editing indicator */
  get threadEditingIndicator(): Locator {
    return this.threadPane.getByText('Editing message');
  }

  /**
   * Assert that exactly one editing indicator is visible.
   * This is the key test for edit state isolation.
   */
  async expectExactlyOneEditIndicator(): Promise<void> {
    await expect(this.allEditingIndicators).toHaveCount(1, { timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Complete editing a message in the thread pane (fill new text and press the keyboard submit shortcut).
   * Assumes edit mode is already active in the thread pane.
   */
  async completeThreadEdit(newText: string): Promise<void> {
    await this.threadReplyInput.fill(newText);
    await this.threadReplyInput.press('Enter');
    await expect(this.threadEditingIndicator).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Assert that the thread pane's edit indicator is visible.
   */
  async expectThreadEditModeActive(): Promise<void> {
    await expect(this.threadEditingIndicator).toBeVisible();
  }

  /**
   * Assert that the thread pane's edit indicator is NOT visible.
   */
  async expectThreadEditModeInactive(): Promise<void> {
    await expect(this.threadEditingIndicator).not.toBeVisible();
  }

  /**
   * Cancel editing by pressing Escape.
   * Main room and thread pane have separate edit state contexts, so we check
   * which pane has the editing indicator and click its input to ensure focus
   * before pressing Escape.
   */
  async cancelEditWithEscape(): Promise<void> {
    if (await this.threadEditingIndicator.isVisible()) {
      await this.threadReplyInput.click();
    } else {
      await this.messageInput.click();
    }
    await this.page.keyboard.press('Escape');
    await expect(this.editingIndicator).not.toBeVisible();
  }

  /**
   * Get a MessageComponent for a message in the thread pane.
   */
  getThreadMessage(text: string): MessageComponent {
    const locator = this.threadPane.locator('[role="article"]', { hasText: text });
    return new MessageComponent(this.page, locator);
  }

  // --- Thread Responsive Layout ---

  /** The back-to-room button in the thread pane (visible on small screens) */
  get threadBackButton(): Locator {
    return this.page.getByTitle('Back to room');
  }

  /** The close thread button (visible on md+ screens) */
  get threadCloseButton(): Locator {
    return this.page.getByTitle('Close thread');
  }

  /** The main room content area (events + input) */
  get mainRoomContent(): Locator {
    // The main content contains the room events pane and chat input
    return this.page.locator('div').filter({
      has: this.page.getByTestId('message-input')
    });
  }

  /**
   * Assert that the thread reply input is focused.
   */
  async expectThreadInputFocused(): Promise<void> {
    await expect(this.threadReplyInput).toBeFocused({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Assert that the back button is visible in the thread pane (small screens).
   */
  async expectThreadBackButtonVisible(): Promise<void> {
    await expect(this.threadBackButton).toBeVisible();
  }

  /**
   * Assert that the back button is NOT visible in the thread pane (md+ screens).
   */
  async expectThreadBackButtonNotVisible(): Promise<void> {
    await expect(this.threadBackButton).not.toBeVisible();
  }

  /**
   * Assert that the close (X) button is visible in the thread pane (md+ screens).
   */
  async expectThreadCloseButtonVisible(): Promise<void> {
    await expect(this.threadCloseButton).toBeVisible();
  }

  /**
   * Assert that the close (X) button is NOT visible in the thread pane (small screens).
   */
  async expectThreadCloseButtonNotVisible(): Promise<void> {
    await expect(this.threadCloseButton).not.toBeVisible();
  }

  /**
   * Close the thread using the back button (for small screens).
   */
  async closeThreadWithBackButton(): Promise<void> {
    await this.threadBackButton.click();
    await expect(this.page.getByRole('heading', { name: /^Thread in #/ })).not.toBeVisible();
  }

  /**
   * Close the thread using the close (X) button.
   */
  async closeThreadWithCloseButton(): Promise<void> {
    await this.threadCloseButton.click();
    await expect(this.page.getByRole('heading', { name: /^Thread in #/ })).not.toBeVisible();
  }

  /**
   * Close the thread by pressing Escape.
   */
  async closeThreadWithEscape(): Promise<void> {
    await this.page.keyboard.press('Escape');
    await expect(this.page.getByRole('heading', { name: /^Thread in #/ })).not.toBeVisible();
  }

  // --- Draft Testing ---

  /**
   * Type text in the main room input without sending.
   */
  async typeInMainInput(text: string): Promise<void> {
    await this.messageInput.fill(text);
  }

  /**
   * Type text in the thread reply input without sending.
   */
  async typeInThreadInput(text: string): Promise<void> {
    await this.threadReplyInput.fill(text);
  }

  /**
   * Get the current text of the main room input.
   */
  async getMainInputValue(): Promise<string> {
    return (await this.messageInput.textContent()) ?? '';
  }

  /**
   * Get the current text of the thread reply input.
   */
  async getThreadInputValue(): Promise<string> {
    return (await this.threadReplyInput.textContent()) ?? '';
  }

  /**
   * Assert that the main room input has specific text.
   */
  async expectMainInputValue(expected: string): Promise<void> {
    await expect(this.messageInput).toHaveText(expected);
  }

  /**
   * Assert that the thread reply input has specific text.
   */
  async expectThreadInputValue(expected: string): Promise<void> {
    await expect(this.threadReplyInput).toHaveText(expected);
  }

  /**
   * Assert that the main room input is empty.
   */
  async expectMainInputEmpty(): Promise<void> {
    await expect(this.messageInput).toHaveText('');
  }

  /**
   * Assert that the thread reply input is empty.
   */
  async expectThreadInputEmpty(): Promise<void> {
    await expect(this.threadReplyInput).toHaveText('');
  }

  // --- Drag and Drop ---

  /** The drop zone overlay that appears when dragging files */
  get dropZoneOverlay(): Locator {
    return this.page.getByText('Drop files here');
  }

  /** The main room content div (where the drop zone is attached) */
  get roomDropZone(): Locator {
    // Target the room content area that contains both the message input and the room header
    return this.page.locator('div.relative.flex.min-h-0.min-w-0.flex-1.flex-col').filter({
      has: this.page.getByTestId('message-input')
    });
  }

  /**
   * Simulate dragging files over the room drop zone.
   * This triggers the dragenter event with file types.
   */
  async simulateDragEnter(): Promise<void> {
    await this.roomDropZone.evaluate((el) => {
      const dataTransfer = new DataTransfer();
      // Add a dummy file to simulate file drag
      dataTransfer.items.add(new File([''], 'test.png', { type: 'image/png' }));

      const dragEnterEvent = new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        dataTransfer
      });
      el.dispatchEvent(dragEnterEvent);
    });
  }

  /**
   * Simulate dragging files away from the room drop zone.
   * This triggers the dragleave event.
   */
  async simulateDragLeave(): Promise<void> {
    await this.roomDropZone.evaluate((el) => {
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(new File([''], 'test.png', { type: 'image/png' }));

      const dragLeaveEvent = new DragEvent('dragleave', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
        relatedTarget: null // Leaving the element entirely
      });
      el.dispatchEvent(dragLeaveEvent);
    });
  }

  /**
   * Simulate dropping a file on the room drop zone.
   * Uses a real file from the e2e fixtures.
   */
  async simulateFileDrop(filePath: string): Promise<void> {
    // Read the file and convert to base64
    const fs = await import('fs/promises');
    const path = await import('path');

    const absolutePath = path.resolve(filePath);
    const fileBuffer = await fs.readFile(absolutePath);
    const fileName = path.basename(absolutePath);
    const base64 = fileBuffer.toString('base64');

    // Determine MIME type from extension
    const ext = path.extname(fileName).toLowerCase();
    const mimeTypes: Record<string, string> = {
      '.png': 'image/png',
      '.jpg': 'image/jpeg',
      '.jpeg': 'image/jpeg',
      '.gif': 'image/gif',
      '.webp': 'image/webp',
      '.mp3': 'audio/mpeg',
      '.wav': 'audio/wav',
      '.ogg': 'audio/ogg',
      '.m4a': 'audio/mp4',
      '.flac': 'audio/flac',
      '.webm': 'audio/webm',
      '.mp4': 'video/mp4',
      '.mov': 'video/quicktime'
    };
    const mimeType = mimeTypes[ext] || 'application/octet-stream';

    await this.roomDropZone.evaluate(
      (el, { base64Data, fileName, mimeType }) => {
        // Convert base64 to Uint8Array
        const binaryString = atob(base64Data);
        const bytes = new Uint8Array(binaryString.length);
        for (let i = 0; i < binaryString.length; i++) {
          bytes[i] = binaryString.charCodeAt(i);
        }

        // Create a File object
        const file = new File([bytes], fileName, { type: mimeType });

        // Create DataTransfer with the file
        const dataTransfer = new DataTransfer();
        dataTransfer.items.add(file);

        // Dispatch drop event
        const dropEvent = new DragEvent('drop', {
          bubbles: true,
          cancelable: true,
          dataTransfer
        });
        el.dispatchEvent(dropEvent);
      },
      { base64Data: base64, fileName, mimeType }
    );
  }

  /**
   * Assert that the drop zone overlay is visible.
   */
  async expectDropZoneOverlayVisible(): Promise<void> {
    await expect(this.dropZoneOverlay).toBeVisible();
  }

  /**
   * Assert that the drop zone overlay is NOT visible.
   */
  async expectDropZoneOverlayNotVisible(): Promise<void> {
    await expect(this.dropZoneOverlay).not.toBeVisible();
  }

  /**
   * Drag and drop a file onto the room, then verify an attachment preview appears.
   * Waits for the count to increase rather than checking visibility (handles multiple files).
   */
  async dropFile(filePath: string): Promise<void> {
    const currentCount = await this.attachmentPreview.count();
    await this.simulateFileDrop(filePath);
    await expect(this.attachmentPreview).toHaveCount(currentCount + 1);
  }
}
