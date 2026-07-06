import type { Page } from '@playwright/test';

export async function unloadPageForIdentitySwitch(page: Page): Promise<void> {
  try {
    await page.goto('about:blank');
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (
      !message.includes('net::ERR_ABORTED') &&
      !message.includes('interrupted by another navigation')
    ) {
      throw error;
    }
  }
}
