import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Space Admin Rooms page (/chat/-/{spaceId}/admin/rooms).
 * Covers room listing, archiving/unarchiving, auto-join, sections, and CRUD.
 */
export class SpaceAdminRoomsPage {
  constructor(readonly page: Page) {}

  // --- Page-level Locators ---

  /** The page heading (h1 from PaneHeader) */
  get pageHeading(): Locator {
    return this.page.locator('h1', { hasText: 'Rooms' });
  }

  /** The "New Room" button */
  get newRoomButton(): Locator {
    return this.page.getByRole('button', { name: 'New Room' });
  }

  /** The "New Section" button */
  get newSectionButton(): Locator {
    return this.page.getByRole('button', { name: 'New Section' });
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
   * Get a section header locator by name.
   * Targets the `span.font-semibold` that renders section names.
   */
  sectionHeader(name: string): Locator {
    return this.page.locator('span.font-semibold', { hasText: name });
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

  /** Click the Unarchive button on an archived room row. */
  async unarchiveRoom(roomName: string): Promise<void> {
    const row = this.roomRow(roomName);
    await expect(row.getByTitle('Unarchive room')).toBeVisible();
    await row.getByRole('button', { name: 'Unarchive' }).click();
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

  /** Click the auto-join toggle button on a room row. */
  async toggleAutoJoin(roomName: string): Promise<void> {
    const row = this.roomRow(roomName);
    const button = row.getByTitle(/auto-join/);
    await expect(button).toBeVisible();
    await button.click();
  }

  // --- Section Actions ---

  /** Create a new section via the New Section modal. */
  async createSection(name: string): Promise<void> {
    await this.newSectionButton.click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Section name').fill(name);
    await this.dialog.getByRole('button', { name: 'Create Section' }).click();
  }

  /** Rename a section: clicks the rename icon, fills new name, saves. */
  async renameSection(newName: string): Promise<void> {
    await this.page.getByTitle('Rename section').click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Section name').clear();
    await this.dialog.getByLabel('Section name').fill(newName);
    await this.dialog.getByRole('button', { name: 'Save' }).click();
  }

  /** Delete a section: clicks the delete icon, confirms the dialog. */
  async deleteSection(): Promise<void> {
    await this.page.getByTitle('Delete section (rooms move to Unsorted)').click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByRole('button', { name: 'Delete Section' }).click();
  }

  // --- Room Creation ---

  /** Create a new room via the New Room modal. */
  async createRoom(name: string): Promise<void> {
    await this.newRoomButton.click();
    await expect(this.dialog).toBeVisible();
    await this.dialog.getByLabel('Name').fill(name);
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
    await expect(this.newRoomButton).toBeVisible();
    await expect(this.newSectionButton).toBeVisible();
  }

  /** Assert a room is visible on the admin page. */
  async expectRoomVisible(name: string, timeout?: number): Promise<void> {
    await expect(this.page.locator('.truncate.text-sm', { hasText: name })).toBeVisible({
      timeout
    });
  }

  /** Assert a room is NOT visible on the admin page. */
  async expectRoomNotVisible(name: string): Promise<void> {
    await expect(this.page.locator('.truncate.text-sm', { hasText: name })).not.toBeVisible();
  }

  /** Assert a section header is visible. */
  async expectSectionVisible(name: string): Promise<void> {
    await expect(this.sectionHeader(name)).toBeVisible();
  }

  /** Assert a section header is NOT visible. */
  async expectSectionNotVisible(name: string): Promise<void> {
    await expect(this.sectionHeader(name)).not.toBeVisible();
  }

  /** Assert auto-join is enabled on a room (button title reflects "on" state). */
  async expectAutoJoinEnabled(roomName: string, timeout?: number): Promise<void> {
    const row = this.roomRow(roomName);
    await expect(row.getByTitle('New members auto-join this room')).toBeVisible({ timeout });
  }

  /** Assert auto-join is disabled on a room (button title reflects "off" state). */
  async expectAutoJoinDisabled(roomName: string, timeout?: number): Promise<void> {
    const row = this.roomRow(roomName);
    await expect(row.getByTitle('New members do not auto-join this room')).toBeVisible({
      timeout
    });
  }
}
