import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';

/**
 * Page object for the notifications page and bell icon.
 * Handles the notification bell, persistent notification list, and dismissal.
 */
export class NotificationsPage {
  constructor(readonly page: Page) {}

  // --- Bell Icon ---

  /** The notification bell link in the header */
  get bellButton(): Locator {
    return this.page.locator('a[title="Notifications"]');
  }

  /** The ambient or important indicator dot shown when notifications are unread. */
  get bellIndicator(): Locator {
    return this.bellButton.getByTestId('notifications-unread-dot');
  }

  // --- Notifications Page ---

  /** The page header */
  get pageHeader(): Locator {
    return this.page.getByRole('heading', { name: 'Notifications' });
  }

  /** The empty state message */
  get emptyState(): Locator {
    return this.page.getByText("You're all caught up!");
  }

  /** Get all notification items on the page */
  get notificationItems(): Locator {
    return this.page.locator('[data-testid="notification-group"]');
  }

  /** Notification groups that still need attention. */
  get unreadItems(): Locator {
    return this.page.locator(
      '[data-testid="notification-group"][data-notification-state="unread"]'
    );
  }

  /** Notification groups that have already been read. */
  get readItems(): Locator {
    return this.page.locator('[data-testid="notification-group"][data-notification-state="read"]');
  }

  /**
   * Navigate to the notifications page by clicking the bell.
   */
  async goto(): Promise<void> {
    await expect(this.bellButton).toBeVisible();
    await expect(async () => {
      await Promise.all([
        this.page.waitForURL(routes.notifications, { timeout: TIMEOUTS.UI_STANDARD }),
        this.bellButton.click()
      ]);
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
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
   * Get a notification item by text in its localised activity summary.
   */
  getNotificationBySummary(summaryText: string): Locator {
    return this.notificationItems.filter({ hasText: summaryText });
  }

  /**
   * Get the Delete button for a specific notification group.
   */
  getDeleteButton(notification: Locator): Locator {
    return notification.locator('button[title="Delete"]');
  }

  /**
   * Click on a notification to navigate to it.
   */
  async clickNotification(notification: Locator): Promise<void> {
    await notification.click();
  }

  /**
   * Permanently dismiss a specific notification group.
   */
  async dismiss(notification: Locator): Promise<void> {
    await this.getDeleteButton(notification).click();
  }

  /**
   * Permanently dismiss every currently visible notification group.
   */
  async dismissAll(): Promise<void> {
    await expect(this.notificationItems.first()).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await this.page.getByRole('button', { name: 'Dismiss all' }).click();
    await expect(this.notificationItems).toHaveCount(0);
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
   * Assert that a notification with specific localised summary text exists.
   */
  async expectNotificationWithSummary(summaryText: string): Promise<void> {
    await expect(this.getNotificationBySummary(summaryText)).toBeVisible();
  }

  /**
   * Assert that a notification shows the correct location (e.g., "#general in My Space").
   */
  async expectNotificationWithLocation(notification: Locator, roomName: string): Promise<void> {
    await expect(notification.getByText(`#${roomName}`, { exact: false })).toBeVisible();
  }

  /**
   * Assert notification count.
   * @param count Expected number of notifications
   * @param timeout Optional timeout in ms (default 5000). Use longer timeout for real-time updates.
   */
  async expectNotificationCount(count: number, timeout?: number): Promise<void> {
    await expect(this.notificationItems).toHaveCount(count, { timeout: timeout ?? 5000 });
  }

  /** Assert that the list contains at least one unread notification group. */
  async expectUnreadNotEmpty(): Promise<void> {
    await expect(this.unreadItems.first()).toBeVisible();
  }

  /** Assert that the list contains no unread notification groups. */
  async expectUnreadEmpty(): Promise<void> {
    await expect(this.unreadItems).toHaveCount(0);
  }

  /** Assert that the list contains at least one read notification group. */
  async expectReadNotEmpty(): Promise<void> {
    await expect(this.readItems.first()).toBeVisible();
  }
}
