import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';

/**
 * Page object for the Explore Spaces page (/chat/spaces).
 * Handles space discovery and joining.
 */
export class ExplorePage {
  constructor(readonly page: Page) {}

  /**
   * Navigate to the explore spaces page.
   */
  async goto(): Promise<void> {
    await this.page.goto(routes.spaces);
    await this.page.waitForURL(routes.spaces);
  }

  /**
   * Get the locator for a space card by name.
   */
  getSpaceItem(spaceName: string): Locator {
    return this.page.locator('[data-testid="space-card"]', {
      has: this.page.getByRole('heading', { name: spaceName })
    });
  }

  /**
   * Open a space from the directory: clicks "Join" if the user isn't yet a
   * member, or the "Joined" link if they already are. Issue #330 / ADR-027:
   * signup auto-joins the primary space, so the directory often shows a
   * Joined badge by the time tests interact with it. The `_spaceName` arg
   * is ignored — the directory has at most one card and we just open it,
   * because tests that pass custom names from `chatPage.createSpace(name)`
   * (now a no-op) won't find their card otherwise.
   */
  async joinSpace(_spaceName?: string): Promise<void> {
    const spaceCard = this.page.locator('[data-testid="space-card"]').first();
    await expect(spaceCard).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    const joinButton = spaceCard.getByRole('button', { name: 'Join', exact: true });
    if (await joinButton.isVisible().catch(() => false)) {
      await joinButton.click();
    } else {
      await spaceCard.getByRole('link', { name: 'Joined' }).click();
    }
    await this.page.waitForURL(routes.patterns.spaceOrRoom);
  }

  /**
   * Assert that a space is visible in the list.
   */
  async expectSpaceVisible(spaceName: string): Promise<void> {
    await expect(this.getSpaceItem(spaceName)).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }

  /**
   * Assert that a space is NOT visible in the list.
   */
  async expectSpaceNotVisible(spaceName: string): Promise<void> {
    await expect(this.getSpaceItem(spaceName)).not.toBeVisible();
  }

  /**
   * Assert that a space appears as "Joined" in the list.
   */
  async expectSpaceJoined(spaceName: string): Promise<void> {
    const spaceItem = this.getSpaceItem(spaceName);
    await expect(spaceItem).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(spaceItem.getByRole('link', { name: 'Joined' })).toBeVisible();
  }

  /**
   * Assert that a space shows a "Join" button (not yet joined).
   */
  async expectSpaceJoinable(spaceName: string): Promise<void> {
    const spaceItem = this.getSpaceItem(spaceName);
    await expect(spaceItem).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(spaceItem.getByRole('button', { name: 'Join', exact: true })).toBeVisible();
  }
}
