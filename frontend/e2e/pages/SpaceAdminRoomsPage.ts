import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Space Admin Rooms page (/chat/-/{spaceId}/admin/rooms).
 * Covers room listing, archiving/unarchiving, global-room toggle, groups, and CRUD.
 */
export class SpaceAdminRoomsPage {
  constructor(readonly page: Page) {}

  // --- Page-level Locators ---

  /** The page heading (h1 from PaneHeader) */
  get pageHeading(): Locator {
    return this.page.locator('h1', { hasText: 'Rooms' });
  }

  /** The "New Group" button (page-level). */
  get newGroupButton(): Locator {
    return this.page.getByRole('button', { name: 'New Group' });
  }

  /** The "New Room" button on a specific group's header. */
  newRoomButton(groupName: string): Locator {
    return this.groupHeaderRow(groupName).getByRole('button', { name: 'New Room' });
  }

  /** The dialog element (used for create/edit/archive/delete modals) */
  get dialog(): Locator {
    return this.page.getByRole('dialog');
  }

  // --- Room Row Helpers ---

  /**
   * Get the room row locator for a given room name.
   * Targets the draggable row div that contains the room name.
   */
  roomRow(name: string): Locator {
    return this.page.locator('.cursor-grab', { hasText: name });
  }

  /**
   * Get a group header locator by name.
   * Targets the `h2` that renders group names.
   */
  groupHeader(name: string): Locator {
    return this.page.locator('h2', { hasText: name });
  }

  /**
   * Get the full group-header row for a given group name. Scopes the
   * per-group Rename / Delete buttons so they don't collide with the
   * seed "Lobby" group's buttons (post-ADR-031 there is always at
   * least one group present).
   */
  groupHeaderRow(name: string): Locator {
    return this.page.locator('.group-header', {
      has: this.page.locator('h2', { hasText: name })
    });
  }

  // --- Navigation ---

  /** Navigate directly to the rooms admin page. */
  async goto(spaceId: string): Promise<void> {
    await this.page.goto(routes.serverAdminRooms);
    await expect(this.pageHeading).toBeVisible();
  }

  // --- Room Actions ---

  /** Click the Archive button on a room row (opens confirmation dialog). */
  async clickArchive(roomName: string): Promise<void> {
    const row = this.roomRow(roomName);
    await row.getByTitle('Archive room').click();
    await expect(this.dialog).toBeVisible();
  }

  /** Archive a room via admin UI: clicks Archive, then confirms the dialog. */
  async archiveRoom(roomName: string): Promise<void> {
    await this.clickArchive(roomName);
    await this.dialog.getByRole('button', { name: 'Archive Room' }).click();
  }

  /** Click the Unarchive button on an archived room row (opens confirmation dialog). */
  async clickUnarchive(roomName: string): Promise<void> {
    const row = this.roomRow(roomName);
    await row.getByTitle('Unarchive room').click();
    await expect(this.dialog).toBeVisible();
  }

  /** Unarchive a room via admin UI: clicks Unarchive, then confirms the dialog. */
  async unarchiveRoom(roomName: string): Promise<void> {
    await this.clickUnarchive(roomName);
    await this.dialog.getByRole('button', { name: 'Unarchive Room' }).click();
  }

  /** Click the Edit button on a room row (opens edit dialog). */
  async clickEdit(roomName: string): Promise<void> {
    const row = this.roomRow(roomName);
    await row.getByTitle('Edit room').click();
    await expect(this.dialog).toBeVisible();
  }

  /**
   * Edit a room's name and/or description via the edit dialog.
   * Opens the dialog, fills fields, and saves.
   */
  async editRoom(currentName: string, newName: string, description?: string): Promise<void> {
    await this.clickEdit(currentName);

    const nameInput = this.dialog.getByLabel('Name');
    await nameInput.clear();
    await nameInput.fill(newName);

    if (description !== undefined) {
      const descInput = this.dialog.getByLabel('Description');
      await descInput.fill(description);
    }

    await this.dialog.getByRole('button', { name: 'Save Changes' }).click();
  }

  // --- Group Actions ---

  /** Create a new group via the New Group modal. */
  async createGroup(name: string): Promise<void> {
    await this.newGroupButton.click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Group name').fill(name);
    await this.dialog.getByRole('button', { name: 'Create Group' }).click();
  }

  /**
   * Rename a group: clicks the rename icon on the named group's header
   * row, fills the new name, saves. Scoped to `currentName` because the
   * seed "Lobby" group always has its own Rename button.
   */
  async renameGroup(currentName: string, newName: string): Promise<void> {
    await this.groupHeaderRow(currentName).getByTitle('Rename group').click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Group name').clear();
    await this.dialog.getByLabel('Group name').fill(newName);
    await this.dialog.getByRole('button', { name: 'Save' }).click();
  }

  /**
   * Delete a group: clicks the delete icon on the named group's header
   * row, confirms the dialog. Scoped to `groupName` for the same reason
   * as renameGroup. The button is disabled while the group still has
   * rooms, so callers must move rooms out first.
   */
  async deleteGroup(groupName: string): Promise<void> {
    await this.groupHeaderRow(groupName).getByTitle('Delete group').click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByRole('button', { name: 'Delete Group' }).click();
  }

  // --- Room Creation ---

  /** Create a new room in the named group via the New Room modal. */
  async createRoom(groupName: string, name: string): Promise<void> {
    await this.newRoomButton(groupName).click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Room Name').fill(name);
    await this.dialog.getByRole('button', { name: 'Create Room' }).click();
  }

  // --- Dialog Actions ---

  /** Cancel the currently open dialog. */
  async cancelDialog(): Promise<void> {
    await this.dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(this.dialog).not.toBeVisible();
  }

  // --- Assertions ---

  /** Assert the rooms admin page is visible. */
  async expectVisible(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.newGroupButton).toBeVisible();
  }

  /** Assert a room is visible on the admin page. */
  async expectRoomVisible(name: string, timeout?: number): Promise<void> {
    await expect(this.roomRow(name)).toBeVisible({ timeout });
  }

  /** Assert a room is NOT visible on the admin page. */
  async expectRoomNotVisible(name: string): Promise<void> {
    await expect(this.roomRow(name)).not.toBeVisible();
  }

  /** Assert a group header is visible. */
  async expectGroupVisible(name: string): Promise<void> {
    await expect(this.groupHeader(name)).toBeVisible();
  }

  /** Assert a group header is NOT visible. */
  async expectGroupNotVisible(name: string): Promise<void> {
    await expect(this.groupHeader(name)).not.toBeVisible();
  }

}
