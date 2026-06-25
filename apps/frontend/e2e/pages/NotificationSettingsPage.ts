import { expect, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the notification settings page.
 * Handles push notification settings and notification sound settings.
 */
export class NotificationSettingsPage {
  constructor(readonly page: Page) {}

  // --- Navigation ---

  /**
   * Navigate to the notification settings page.
   */
  async goto(): Promise<void> {
    await this.page.goto(routes.settingsNotifications);
    await this.page.waitForURL(routes.settingsNotifications);
    // Wait for the page to be interactive
    await expect(
      this.page.getByRole('heading', { name: 'Notifications', exact: true })
    ).toBeVisible();
  }
}
