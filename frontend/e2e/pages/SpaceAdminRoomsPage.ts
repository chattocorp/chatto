import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Space Admin Rooms page (/chat/-/{spaceId}/admin/rooms).
 * Covers room listing, archiving/unarchiving, global-room toggle, sets, and CRUD.
 */
export class SpaceAdminRoomsPage {
  constructor(readonly page: Page) {}

  // --- Page-level Locators ---

  /** The page heading (h1 from PaneHeader) */
  get pageHeading(): Locator {
    return this.page.locator('h1', { hasText: 'Rooms' });
  }

  /** The "New Set" button (page-level, in the PaneHeader). */
  get newSetButton(): Locator {
    return this.page.getByRole('button', { name: 'New Set' });
  }

  /** The "New Room" button on a specific set's header. */
  newRoomButton(setName: string): Locator {
    return this.setHeaderRow(setName).getByRole('button', { name: 'New Room' });
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
   * Get a set header locator by name.
   * Targets the `h2` that renders set names.
   */
  setHeader(name: string): Locator {
    return this.page.locator('h2', { hasText: name });
  }

  /**
   * Get the full set-header row for a given set name. Scopes the
   * per-set Rename / Delete buttons so they don't collide with the
   * seed "Rooms" set's buttons (post-ADR-031 there is always at least one
   * set present).
   */
  setHeaderRow(name: string): Locator {
    return this.page.locator('.set-header', {
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

  /** Click the global-room toggle chip on a room row (opens confirmation dialog). */
  async clickToggleGlobal(roomName: string): Promise<void> {
    const row = this.roomRow(roomName);
    const button = row.getByRole('button').filter({ has: this.page.locator('.uil--globe') });
    await expect(button).toBeVisible();
    await button.click();
    await expect(this.dialog).toBeVisible();
  }

  /**
   * Toggle the global flag on a room: clicks the chip, then confirms the
   * dialog. The confirm-button label switches with direction.
   */
  async toggleGlobal(roomName: string, becomingGlobal: boolean): Promise<void> {
    await this.clickToggleGlobal(roomName);
    const label = becomingGlobal ? 'Mark as Global' : 'Remove Global Flag';
    await this.dialog.getByRole('button', { name: label }).click();
  }

  // --- Set Actions ---

  /** Create a new set via the New Set modal. */
  async createSet(name: string): Promise<void> {
    await this.newSetButton.click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Set name').fill(name);
    await this.dialog.getByRole('button', { name: 'Create Set' }).click();
  }

  /**
   * Rename a set: clicks the rename icon on the named set's header
   * row, fills the new name, saves. Scoped to `currentName` because the
   * seed "Rooms" set always has its own Rename button.
   */
  async renameSet(currentName: string, newName: string): Promise<void> {
    await this.setHeaderRow(currentName).getByTitle('Rename set').click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Set name').clear();
    await this.dialog.getByLabel('Set name').fill(newName);
    await this.dialog.getByRole('button', { name: 'Save' }).click();
  }

  /**
   * Delete a set: clicks the delete icon on the named set's header
   * row, confirms the dialog. Scoped to `setName` for the same reason
   * as renameSet. The button is disabled while the set still has rooms,
   * so callers must move rooms out first.
   */
  async deleteSet(setName: string): Promise<void> {
    await this.setHeaderRow(setName).getByTitle('Delete set').click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByRole('button', { name: 'Delete Set' }).click();
  }

  // --- Room Creation ---

  /** Create a new room in the named set via the New Room modal. */
  async createRoom(setName: string, name: string): Promise<void> {
    await this.newRoomButton(setName).click();
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
    await expect(this.newSetButton).toBeVisible();
  }

  /** Assert a room is visible on the admin page. */
  async expectRoomVisible(name: string, timeout?: number): Promise<void> {
    await expect(this.roomRow(name)).toBeVisible({ timeout });
  }

  /** Assert a room is NOT visible on the admin page. */
  async expectRoomNotVisible(name: string): Promise<void> {
    await expect(this.roomRow(name)).not.toBeVisible();
  }

  /** Assert a set header is visible. */
  async expectSetVisible(name: string): Promise<void> {
    await expect(this.setHeader(name)).toBeVisible();
  }

  /** Assert a set header is NOT visible. */
  async expectSetNotVisible(name: string): Promise<void> {
    await expect(this.setHeader(name)).not.toBeVisible();
  }

  /** Assert the room is marked as global (chip title reflects "on" state). */
  async expectGlobalEnabled(roomName: string, timeout?: number): Promise<void> {
    const row = this.roomRow(roomName);
    await expect(
      row.getByTitle('Global room — all server members are members')
    ).toBeVisible({ timeout });
  }

  /** Assert the room is NOT marked as global (chip title reflects "off" state). */
  async expectGlobalDisabled(roomName: string, timeout?: number): Promise<void> {
    const row = this.roomRow(roomName);
    await expect(
      row.getByTitle('Make this room global (all server members get implicit membership)')
    ).toBeVisible({ timeout });
  }
}
