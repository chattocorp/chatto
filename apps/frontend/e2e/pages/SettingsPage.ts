import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';

/**
 * Page object for the user settings/profile page.
 * Handles avatar upload, display name editing, and other profile settings.
 */
export class SettingsPage {
  constructor(readonly page: Page) {}

  // --- Navigation ---

  /**
   * Navigate to the settings page and wait for it to be interactive.
   */
  async goto(): Promise<void> {
    await this.page.goto(routes.settings);
    await this.page.waitForURL(routes.settings);
    // Wait for the form to be interactive (GraphQL connection established)
    await this.displayNameInput.waitFor({ state: 'visible' });
    // Ensure the WebSocket connection is established (no "Real-time updates paused" banner)
    await expect(this.page.getByText('Real-time updates paused')).not.toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  }

  // --- Locators: Avatar ---

  /** The file input for avatar upload (hidden) */
  get avatarFileInput(): Locator {
    return this.page.locator('input[type="file"]');
  }

  /** The avatar preview image */
  get avatarPreview(): Locator {
    return this.page.locator('img[alt="Avatar preview"]');
  }

  /** The remove avatar button */
  get removeAvatarButton(): Locator {
    return this.page.getByRole('button', { name: 'Remove' });
  }

  // --- Locators: Display Name ---

  /** The display name input field */
  get displayNameInput(): Locator {
    return this.page.getByPlaceholder('Enter your display name');
  }

  /** The save display name button */
  get saveDisplayNameButton(): Locator {
    return this.page.getByRole('button', { name: 'Save Changes' });
  }

  /** The form error message */
  get formError(): Locator {
    return this.page.locator('[class*="text-error"], [class*="text-danger"]').first();
  }

  // --- Actions: Avatar ---

  /**
   * Upload an avatar image.
   * Waits for success toast.
   */
  async uploadAvatar(filePath: string): Promise<void> {
    await this.avatarFileInput.setInputFiles(filePath);
    await expect(this.page.getByText('Avatar uploaded successfully')).toBeVisible({
      timeout: 10000
    });
  }

  /**
   * Remove the current avatar.
   * Waits for success toast.
   */
  async removeAvatar(): Promise<void> {
    await this.removeAvatarButton.click();
    await expect(this.page.getByText('Avatar removed')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  }

  // --- Actions: Display Name ---

  /**
   * Update the display name.
   * Waits for success message.
   */
  async updateDisplayName(name: string): Promise<void> {
    // Focus and select all, then type to ensure clean replacement
    await this.displayNameInput.click();
    await this.displayNameInput.selectText();
    await this.displayNameInput.fill(name);

    // Wait for button to be enabled (form recognizes the modified state)
    await expect(this.saveDisplayNameButton).toBeEnabled({ timeout: TIMEOUTS.UI_STANDARD });

    // Click and wait for success
    await this.saveDisplayNameButton.click();
    await expect(this.page.getByText('Profile updated')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  }

  /**
   * Fill and submit display name without expecting success.
   * Use this for testing validation errors.
   */
  async submitDisplayName(name: string): Promise<void> {
    await this.displayNameInput.fill(name);
    await this.saveDisplayNameButton.click();
  }

  // --- Assertions ---

  /**
   * Assert that the avatar preview shows an image.
   */
  async expectAvatarVisible(): Promise<void> {
    await expect(this.avatarPreview).toBeVisible();
  }

  /**
   * Assert that no avatar is set (remove button not visible).
   */
  async expectNoAvatar(): Promise<void> {
    await expect(this.removeAvatarButton).not.toBeVisible();
  }

  /**
   * Assert that the remove button is visible (avatar is set).
   */
  async expectRemoveButtonVisible(): Promise<void> {
    await expect(this.removeAvatarButton).toBeVisible();
  }

  /**
   * Assert that a form error message is visible.
   */
  async expectErrorVisible(errorText?: string): Promise<void> {
    if (errorText) {
      await expect(this.page.getByText(errorText)).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    } else {
      await expect(this.formError).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    }
  }

  /**
   * Assert that no form error is visible.
   */
  async expectNoError(): Promise<void> {
    await expect(this.formError).not.toBeVisible();
  }

  /**
   * Assert the current display name value.
   */
  async expectDisplayNameValue(value: string): Promise<void> {
    await expect(this.displayNameInput).toHaveValue(value);
  }
}
