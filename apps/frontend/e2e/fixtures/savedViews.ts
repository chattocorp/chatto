import type { Page } from '@playwright/test';

/**
 * Delete the device's saved views so the next app load uses live startup.
 * The page first leaves the app, so no open connection blocks the deletion.
 */
export async function clearSavedViews(page: Page): Promise<void> {
  await page.goto('/robots.txt');
  await page.evaluate(
    () =>
      new Promise<void>((resolve, reject) => {
        const request = indexedDB.deleteDatabase('chatto-saved-views');
        request.onsuccess = () => resolve();
        request.onerror = () => reject(request.error);
        request.onblocked = () =>
          reject(new Error('An open connection blocks saved-view deletion'));
      })
  );
}
