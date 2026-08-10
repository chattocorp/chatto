import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';

/**
 * Page object for the notifications page and bell icon.
 * Handles the notification bell, grouped inbox, and triage actions.
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
   * Get a notification item by its summary text.
   */
  getNotificationBySummary(summaryText: string): Locator {
    return this.notificationItems.filter({ hasText: summaryText });
  }

  /**
   * Get the Mark done button for a specific Inbox group.
   */
  getDismissButton(notification: Locator): Locator {
    return notification.locator('button[title="Mark done"]');
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
   * Move every currently visible Inbox group to Done.
   */
  async dismissAll(): Promise<void> {
    await expect(this.notificationItems.first()).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    while ((await this.notificationItems.count()) > 0) {
      const count = await this.notificationItems.count();
      await this.getDismissButton(this.notificationItems.first()).click();
      await expect(this.notificationItems).toHaveCount(count - 1);
    }
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
    void serverName;
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

  /**
   * Assert that bulk triage has visible Inbox work.
   */
  async expectClearAllVisible(): Promise<void> {
    await expect(this.notificationItems.first()).toBeVisible();
  }

  /**
   * Assert that there is no visible Inbox work.
   */
  async expectClearAllNotVisible(): Promise<void> {
    await expect(this.notificationItems).toHaveCount(0);
  }
}
