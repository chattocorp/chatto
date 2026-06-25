import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the notifications page and bell icon.
 * Handles notification bell, notification list, and dismiss actions.
 */
export class NotificationsPage {
  constructor(readonly page: Page) {}

  // --- Bell Icon ---

  /** The notification bell link in the header */
  get bellButton(): Locator {
    return this.page.locator('a[title="Notifications"]');
  }

  /** The orange indicator dot on the bell icon (visible when there are notifications) */
  get bellIndicator(): Locator {
    // bg-warning class is on a span inside the link when there are notifications
    return this.page.locator('a[title="Notifications"] .bg-warning');
  }

  // --- Notifications Page ---

  /** The page header */
  get pageHeader(): Locator {
    return this.page.getByRole('heading', { name: 'Notifications' });
  }

  /** The "Clear all" button */
  get clearAllButton(): Locator {
    return this.page.getByRole('button', { name: 'Clear all' });
  }

  /** The empty state message */
  get emptyState(): Locator {
    return this.page.getByText("You're all caught up!");
  }

  /** Get all notification items on the page */
  get notificationItems(): Locator {
    return this.page.locator('[data-testid="notification-item"]');
  }

  /**
   * Navigate to the notifications page by clicking the bell.
   */
  async goto(): Promise<void> {
    await this.bellButton.click();
    await this.page.waitForURL(routes.notifications);
    await expect(this.pageHeader).toBeVisible();
  }

  /**
   * Navigate directly to notifications page via URL.
   */
  async gotoDirectly(): Promise<void> {
    await this.page.goto(routes.notifications);
    await expect(this.pageHeader).toBeVisible();
  }

  /**
   * Get a notification item by the actor's display name.
   */
  getNotificationByActor(displayName: string): Locator {
    // Notifications show the actor's avatar, so we look for items containing the name
    return this.notificationItems.filter({ hasText: displayName });
  }

  /**
   * Get a notification item by its summary text.
   */
  getNotificationBySummary(summaryText: string): Locator {
    return this.notificationItems.filter({ hasText: summaryText });
  }

  /**
   * Get the dismiss button (X) for a specific notification.
   */
  getDismissButton(notification: Locator): Locator {
    return notification.locator('button[title="Dismiss"]');
  }

  /**
   * Click on a notification to navigate to it.
   */
  async clickNotification(notification: Locator): Promise<void> {
    await notification.click();
  }

  /**
   * Dismiss a specific notification.
   */
  async dismissNotification(notification: Locator): Promise<void> {
    await this.getDismissButton(notification).click();
  }

  /**
   * Dismiss all notifications.
   */
  async dismissAll(): Promise<void> {
    await this.clearAllButton.click();
  }

  // --- Assertions ---

  /**
   * Assert that the bell has an indicator (notifications exist).
   */
  async expectBellIndicatorVisible(): Promise<void> {
    await expect(this.bellIndicator).toBeVisible();
  }

  /**
   * Assert that the bell does NOT have an indicator (no notifications).
   */
  async expectBellIndicatorNotVisible(): Promise<void> {
    await expect(this.bellIndicator).not.toBeVisible();
  }

  /**
   * Assert that the empty state is shown.
   */
  async expectEmptyState(): Promise<void> {
    await expect(this.emptyState).toBeVisible();
  }

  /**
   * Assert that a notification with specific summary text exists.
   */
  async expectNotificationWithSummary(summaryText: string): Promise<void> {
    await expect(this.getNotificationBySummary(summaryText)).toBeVisible();
  }

  /**
   * Assert that a notification shows the correct location (e.g., "#general in My Space").
   */
  async expectNotificationWithLocation(
    notification: Locator,
    roomName: string,
    serverName: string
  ): Promise<void> {
    const locationText = `#${roomName} in ${serverName}`;
    await expect(notification.getByText(locationText)).toBeVisible();
  }

  /**
   * Assert notification count.
   * @param count Expected number of notifications
   * @param timeout Optional timeout in ms (default 5000). Use longer timeout for real-time updates.
   */
  async expectNotificationCount(count: number, timeout?: number): Promise<void> {
    await expect(this.notificationItems).toHaveCount(count, { timeout: timeout ?? 5000 });
  }

  /**
   * Assert that Clear all button is visible (only when there are notifications).
   */
  async expectClearAllVisible(): Promise<void> {
    await expect(this.clearAllButton).toBeVisible();
  }

  /**
   * Assert that Clear all button is NOT visible (empty state).
   */
  async expectClearAllNotVisible(): Promise<void> {
    await expect(this.clearAllButton).not.toBeVisible();
  }
}
