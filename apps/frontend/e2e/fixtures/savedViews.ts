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

/** One persisted resource record, such as a room timeline or member list. */
export interface SavedResource {
  key: string;
  schemaVersion: number;
  data: {
    events?: {
      id: string;
      event: {
        kind?: string;
        body?: string;
        replyCount?: number;
        reactions?: unknown[];
      };
    }[];
  };
}

/** Read every saved resource record without changing the saved views. */
export async function readSavedResources(page: Page): Promise<SavedResource[]> {
  return page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('chatto-saved-views', 2);
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
    try {
      return await new Promise<SavedResource[]>((resolve, reject) => {
        const request = db.transaction('resources').objectStore('resources').getAll();
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      });
    } finally {
      db.close();
    }
  });
}

/**
 * Hold viewer verification requests until `release` is called, so a test can
 * observe what the saved view renders before verification.
 */
export async function holdViewerVerification(page: Page): Promise<{ release: () => void }> {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/chatto.api.v1.ViewerService/GetViewer', async (route) => {
    await gate;
    await route.continue();
  });
  return { release };
}
