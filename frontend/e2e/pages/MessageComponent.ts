import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';

/**
 * Component for interacting with a single message in the chat.
 * Uses hover toolbar for quick actions and "more actions" menu for full context menu.
 */
export class MessageComponent {
  constructor(
    readonly page: Page,
    readonly locator: Locator
  ) {}

  /** The context menu (rendered at page level via popover) */
  get contextMenu(): Locator {
    return this.page.locator('[role="menu"]');
  }

  /** The quick actions hover toolbar (visible on hover, desktop only) */
  get hoverToolbar(): Locator {
    return this.locator.getByRole('toolbar', { name: 'Message actions' });
  }

  /** Image attachment within this message */
  get attachmentImage(): Locator {
    return this.locator.locator('button[aria-label^="View"] img');
  }

  /** Get the event ID attribute for stable lookups */
  async getEventId(): Promise<string | null> {
    return this.locator.getAttribute('data-event-id');
  }

  /**
   * Open the context menu via the toolbar's "More actions" button.
   */
  private async openContextMenu(): Promise<void> {
    await this.revealHoverToolbar();
    await this.hoverToolbar.getByLabel('More actions').click();
    await expect(this.contextMenu).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  }

  /**
   * Hover over the message to reveal the quick actions toolbar.
   */
  async revealHoverToolbar(): Promise<void> {
    await this.locator.hover();
    await expect(this.hoverToolbar).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Toggle a reaction via the hover toolbar (direct click, no context menu).
   * Adds the reaction if not yet reacted, removes it if already reacted.
   */
  async reactViaToolbar(emoji: string): Promise<void> {
    await this.revealHoverToolbar();
    // Label changes based on whether user already reacted
    const button = this.hoverToolbar.getByLabel(new RegExp(`(React with|Remove) ${emoji}`));
    await button.click();
  }

  /**
   * Get the quick reaction emojis currently shown in the hover toolbar.
   */
  async getToolbarQuickReactions(): Promise<string[]> {
    await this.revealHoverToolbar();
    const buttons = this.hoverToolbar.getByLabel(/React with|Remove/);
    const count = await buttons.count();
    const emojis: string[] = [];
    for (let i = 0; i < count; i++) {
      const text = await buttons.nth(i).textContent();
      if (text) emojis.push(text.trim());
    }
    return emojis;
  }

  /**
   * Start editing the message via the toolbar's direct edit button.
   */
  async editViaToolbar(): Promise<void> {
    await this.revealHoverToolbar();
    await this.hoverToolbar.getByLabel('Edit message').click();
  }

  /**
   * Open the thread pane via the toolbar's direct reply button.
   */
  async replyViaToolbar(): Promise<void> {
    await this.revealHoverToolbar();
    await this.hoverToolbar.getByLabel('Reply in thread').click();
  }

  /**
   * Add a reaction to the message via the context menu.
   */
  async react(emoji: string): Promise<void> {
    await this.openContextMenu();
    await this.contextMenu
      .getByLabel(`React with ${emoji}`)
      .click({ timeout: TIMEOUTS.REALTIME_EVENT });
  }

  /**
   * Add a reaction via the emoji picker in the context menu.
   */
  async reactViaEmojiPicker(search: string, emojiTitle: string): Promise<void> {
    await this.openContextMenu();
    await this.contextMenu.getByLabel('More reactions').click({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The emoji picker opens in a new ContextMenu
    const picker = this.page.locator('input[placeholder="Search emojis..."]');
    await expect(picker).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await picker.fill(search);

    // Click the emoji button matching the title (shortcode name)
    await this.page.locator(`button[title="${emojiTitle}"]`).click();
  }

  /**
   * Add a reaction via the emoji picker in the meta bar.
   * Clicks the "Add reaction" button in the meta bar (visible when reactions exist),
   * searches for the emoji, then clicks the result.
   */
  async reactViaMetaBarPicker(search: string, emojiTitle: string): Promise<void> {
    const addReactionButton = this.locator.getByLabel('Add reaction');
    await addReactionButton.click({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The emoji picker opens in a ContextMenu
    const picker = this.page.locator('input[placeholder="Search emojis..."]');
    await expect(picker).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await picker.fill(search);

    // Click the emoji button matching the title (shortcode name)
    await this.page.locator(`button[title="${emojiTitle}"]`).click();
  }

  /**
   * Toggle a reaction on/off by clicking the reaction button.
   * If the reaction exists, removes it. If not, this will fail.
   */
  async toggleReaction(emoji: string): Promise<void> {
    const reactionButton = this.locator.getByRole('button', {
      name: new RegExp(`${emoji} reaction`)
    });
    await reactionButton.click();
  }

  /**
   * Delete the message via the context menu.
   */
  async delete(): Promise<void> {
    await this.openContextMenu();
    await this.contextMenu
      .getByRole('menuitem', { name: 'Delete' })
      .click({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Confirm in the modal dialog
    const dialog = this.page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Delete' }).click();
  }

  /**
   * Click delete but cancel the confirmation.
   */
  async cancelDelete(): Promise<void> {
    await this.openContextMenu();
    await this.contextMenu
      .getByRole('menuitem', { name: 'Delete' })
      .click({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Cancel in the modal dialog
    const dialog = this.page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(dialog).not.toBeVisible();
  }

  /**
   * Start editing the message via the context menu.
   */
  async startEdit(): Promise<void> {
    await this.openContextMenu();
    await this.contextMenu
      .getByRole('menuitem', { name: 'Edit' })
      .click({ timeout: TIMEOUTS.REALTIME_EVENT });
  }

  /**
   * Reply to this message in the room (sets inReplyTo attribution).
   * Right-clicks to open context menu, then clicks Reply.
   */
  async replyInRoom(): Promise<void> {
    await this.openContextMenu();
    // "Reply" (exact) is the room reply; "Reply in thread" is the thread reply
    await this.contextMenu
      .getByRole('menuitem', { name: 'Reply', exact: true })
      .click({ timeout: TIMEOUTS.REALTIME_EVENT });
  }

  /**
   * Open the thread pane for this message.
   * Right-clicks to open context menu, then clicks Reply in thread.
   */
  async openThread(): Promise<void> {
    await this.openContextMenu();
    await this.contextMenu
      .getByRole('menuitem', { name: /Reply in thread/ })
      .click({ timeout: TIMEOUTS.REALTIME_EVENT });
  }

  /**
   * Delete an attachment from the message.
   * Hovers over the attachment image first and confirms in the modal dialog.
   */
  async deleteAttachment(): Promise<void> {
    await this.attachmentImage.hover();
    // Use fresh locator query - Playwright will retry if element is detached during re-render
    await this.locator.getByLabel('Delete attachment').click({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Confirm in the modal dialog
    const dialog = this.page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Delete' }).click();
  }

  /**
   * Toggle thread follow/unfollow by clicking the bell button on the thread preview.
   */
  async toggleThreadFollow(): Promise<void> {
    await this.locator.getByTitle(/(?:Un)?[Ff]ollow thread/).click();
  }

  /**
   * Assert that the thread follow button shows "following" state.
   */
  async expectFollowingThread(): Promise<void> {
    await expect(this.locator.getByTitle('Unfollow thread')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  }

  /**
   * Assert that the thread follow button shows "not following" state.
   */
  async expectNotFollowingThread(): Promise<void> {
    await expect(this.locator.getByTitle('Follow thread')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  }

  /**
   * Close the context menu by pressing Escape.
   */
  async closeContextMenu(): Promise<void> {
    await this.page.keyboard.press('Escape');
    await expect(this.contextMenu).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  // --- Context Menu Assertions ---

  /**
   * Assert that the context menu contains a reaction button for the given emoji.
   * Opens the context menu if not already open.
   */
  async expectContextMenuHasReaction(): Promise<void> {
    await this.openContextMenu();
    await expect(this.contextMenu.getByLabel(/React with/).first()).toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu does NOT contain reaction buttons.
   */
  async expectContextMenuNoReaction(): Promise<void> {
    await this.openContextMenu();
    await expect(this.contextMenu.getByLabel(/React with/)).not.toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu contains the Edit action.
   */
  async expectContextMenuHasEdit(): Promise<void> {
    await this.openContextMenu();
    await expect(this.contextMenu.getByRole('menuitem', { name: 'Edit' })).toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu does NOT contain the Edit action.
   */
  async expectContextMenuNoEdit(): Promise<void> {
    await this.openContextMenu();
    await expect(this.contextMenu.getByRole('menuitem', { name: 'Edit' })).not.toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu contains the Reply action.
   */
  async expectContextMenuHasReply(): Promise<void> {
    await this.openContextMenu();
    await expect(
      this.contextMenu.getByRole('menuitem', { name: 'Reply', exact: true })
    ).toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu does NOT contain the Reply action.
   */
  async expectContextMenuNoReply(): Promise<void> {
    await this.openContextMenu();
    await expect(
      this.contextMenu.getByRole('menuitem', { name: 'Reply', exact: true })
    ).not.toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu contains the Delete action.
   */
  async expectContextMenuHasDelete(): Promise<void> {
    await this.openContextMenu();
    await expect(this.contextMenu.getByRole('menuitem', { name: 'Delete' })).toBeVisible();
    await this.closeContextMenu();
  }

  /**
   * Assert that the context menu does NOT contain the Delete action.
   */
  async expectContextMenuNoDelete(): Promise<void> {
    await this.openContextMenu();
    await expect(this.contextMenu.getByRole('menuitem', { name: 'Delete' })).not.toBeVisible();
    await this.closeContextMenu();
  }

  // --- Assertions ---

  /**
   * Assert that a reaction with the given emoji and count is visible.
   */
  async expectReaction(emoji: string, count: number): Promise<void> {
    const reactionButton = this.locator.getByRole('button', {
      name: new RegExp(`${emoji} reaction \\(${count}\\)`)
    });
    await expect(reactionButton).toBeVisible();
  }

  /**
   * Assert that a reaction is NOT visible (count went to 0).
   */
  async expectNoReaction(emoji: string): Promise<void> {
    const reactionButton = this.locator.getByRole('button', {
      name: new RegExp(`${emoji} reaction`)
    });
    await expect(reactionButton).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Get the reaction group container for a specific emoji.
   * The container holds both the button and its tooltip.
   */
  private getReactionGroup(emoji: string): Locator {
    // Find the button, then go up to its parent group container
    return this.locator
      .getByRole('button', { name: new RegExp(`${emoji} reaction`) })
      .locator('..');
  }

  /**
   * Hover over a reaction button and verify the tooltip shows the expected text.
   */
  async expectReactionTooltip(emoji: string, expectedText: string | RegExp): Promise<void> {
    const group = this.getReactionGroup(emoji);
    const button = group.getByRole('button');
    await button.hover();

    // Find the tooltip within the same group container (sibling of button)
    const tooltip = group.getByRole('tooltip');
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toHaveText(expectedText);
  }

  /**
   * Hover over a reaction and verify tooltip contains specific user name(s).
   */
  async expectReactionTooltipContains(emoji: string, userName: string): Promise<void> {
    const group = this.getReactionGroup(emoji);
    const button = group.getByRole('button');
    await button.hover();

    // Find the tooltip within the same group container (sibling of button)
    const tooltip = group.getByRole('tooltip');
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toContainText(userName);
  }

  /**
   * Assert that the message is no longer visible (deleted).
   */
  async expectNotVisible(): Promise<void> {
    await expect(this.locator).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Assert that the message shows the "This message has been deleted" tombstone.
   */
  async expectDeleted(): Promise<void> {
    await expect(this.locator.getByText('This message has been deleted.')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  }

  /**
   * Assert that the message shows (edited) indicator.
   */
  async expectEdited(): Promise<void> {
    await expect(this.locator.getByText('(edited)')).toBeVisible();
  }

  /**
   * Assert that the message does NOT show (edited) indicator.
   */
  async expectNotEdited(): Promise<void> {
    await expect(this.locator.getByText('(edited)')).not.toBeVisible();
  }

  /**
   * Assert that the message has an image attachment.
   */
  async expectAttachment(): Promise<void> {
    await expect(this.attachmentImage).toBeVisible();
  }

  /**
   * Assert that the message has no image attachment.
   */
  async expectNoAttachment(): Promise<void> {
    await expect(this.attachmentImage).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }
}
