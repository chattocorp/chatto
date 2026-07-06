import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Account settings page.
 * Handles account information display and account deletion.
 */
export class AccountPage {
  constructor(readonly page: Page) {}

  // --- Navigation ---

  /**
   * Navigate to the account settings page.
   */
  async goto(): Promise<void> {
    await this.page.goto(routes.settingsAccount);
    await this.page.waitForURL(routes.settingsAccount);
  }

  // --- Locators: Account Info ---

  /** The Account header (h1, exact match to avoid "Account Information") */
  get accountHeader(): Locator {
    return this.page.getByRole('heading', { name: 'Account', exact: true }).first();
  }

  /** The username display */
  get usernameDisplay(): Locator {
    return this.page.locator('dd.font-mono');
  }

  // --- Locators: Delete Account ---

  /** The Delete Account button in the Danger Zone */
  get deleteAccountButton(): Locator {
    return this.page.getByRole('button', { name: 'Delete Account' });
  }

  /** The delete confirmation modal dialog */
  get deleteDialog(): Locator {
    return this.page.getByRole('dialog');
  }

  /** The confirmation text input in the delete modal */
  get confirmInput(): Locator {
    return this.page.getByLabel('Type DELETE to confirm');
  }

  /** The confirm delete button in the modal */
  get confirmDeleteButton(): Locator {
    return this.deleteDialog.getByRole('button', { name: 'Delete Account' });
  }

  /** The cancel button in the delete modal */
  get cancelButton(): Locator {
    return this.deleteDialog.getByRole('button', { name: 'Cancel' });
  }

  // --- Actions ---

  /**
   * Open the delete account confirmation modal.
   */
  async openDeleteModal(): Promise<void> {
    await this.deleteAccountButton.click();
    await expect(this.deleteDialog).toBeVisible();
  }

  /**
   * Type the confirmation text in the delete modal.
   */
  async typeConfirmation(text: string): Promise<void> {
    await this.confirmInput.fill(text);
  }

  /**
   * Confirm account deletion.
   * Assumes the modal is open and confirmation text is entered.
   */
  async confirmDelete(): Promise<void> {
    await this.confirmDeleteButton.click();
  }

  /**
   * Cancel the delete modal.
   */
  async cancelDelete(): Promise<void> {
    await this.cancelButton.click();
    await expect(this.deleteDialog).not.toBeVisible();
  }

  /**
   * Complete the full account deletion flow.
   * Opens modal, types DELETE, confirms, and waits for redirect.
   */
  async deleteAccount(): Promise<void> {
    await this.openDeleteModal();
    await this.typeConfirmation('DELETE');
    await this.confirmDelete();
    // Wait for redirect to landing page after deletion
    await this.page.waitForURL('/');
  }

  // --- Assertions ---

  /**
   * Assert that the account settings page is visible.
   */
  async expectOnAccountPage(): Promise<void> {
    await expect(this.accountHeader).toBeVisible();
  }

  /**
   * Assert that the delete button is disabled (confirmation not complete).
   */
  async expectDeleteButtonDisabled(): Promise<void> {
    await expect(this.confirmDeleteButton).toBeDisabled();
  }

  /**
   * Assert that the delete button is enabled (confirmation complete).
   */
  async expectDeleteButtonEnabled(): Promise<void> {
    await expect(this.confirmDeleteButton).toBeEnabled();
  }

  /**
   * Assert that the username is displayed.
   */
  async expectUsername(username: string): Promise<void> {
    await expect(this.usernameDisplay).toHaveText(username);
  }
}
