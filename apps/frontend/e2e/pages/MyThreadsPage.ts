import { type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the My Threads view.
 * Handles navigation and common selectors for the followed threads list.
 */
export class MyThreadsPage {
  constructor(readonly page: Page) {}

  /** The My Threads sidebar navigation link */
  get myThreadsLink(): Locator {
    return this.page.getByRole('link', { name: 'My Threads' });
  }

  /** All thread items in the list */
  get threadItems(): Locator {
    return this.page.getByTestId('my-thread-item');
  }

  /** The sidebar unread indicator: a notification count badge or an unread dot */
  get sidebarUnreadIndicator(): Locator {
    return this.page.locator(
      '[data-testid="my-threads-notification-badge"], [data-testid="my-threads-unread-dot"]'
    );
  }

  /** Navigate to My Threads and wait for the page to load */
  async goto(): Promise<void> {
    await this.myThreadsLink.click();
    await this.page.waitForURL(routes.threads);
  }
}
