import type { Page } from '@playwright/test';
import { expect, test } from './setup';

/** Wait for a deliberately installed worker, without relying on app registration. */
async function registerWorker(page: Page, scope: string) {
  await page.evaluate(async (scope) => {
    await navigator.serviceWorker.register('/service-worker.js', { scope });
  }, scope);
  await expect
    .poll(() =>
      page.evaluate(async (scope) => {
        const registration = (await navigator.serviceWorker.getRegistrations()).find(
          (entry) => entry.scope === new URL(scope, location.origin).href
        );
        return registration?.active?.state;
      }, scope)
    )
    .toBe('activated');
}

test('fresh visits do not register an offline shell or precache the build', async ({ page }) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.name));
  const workerRequests: string[] = [];
  page.context().on('request', (request) => {
    if (request.serviceWorker()) workerRequests.push(new URL(request.url()).pathname);
  });
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  // This wall-clock observation must outlast the retired seven-second registration
  // timer. An immediate negative assertion would miss a background precache regression.
  await page.waitForTimeout(8_000);

  expect(
    await page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length)
  ).toBe(0);
  expect(await page.evaluate(() => caches.keys())).toEqual([]);
  expect(workerRequests).toEqual([]);
  expect(pageErrors).toEqual([]);
});

test('upgrades remove the root shell and retired caches while preserving scoped push workers', async ({
  page
}) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.name));
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  // Use real browser registrations and Cache Storage to reproduce an existing
  // installation. Push subscription retention is covered by the lifecycle unit tests.
  await registerWorker(page, '/');
  await registerWorker(page, '/__chatto/push/fixture/');
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);
  await page.evaluate(async () => {
    for (const name of [
      'chatto-shell-previous-build',
      'chatto-badge-state-v1',
      'chatto-badge-state-v2',
      'unrelated-cache'
    ]) {
      const cache = await caches.open(name);
      await cache.put('/login', new Response('Retired offline shell'));
    }
  });

  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  await expect.poll(() => page.evaluate(() => caches.keys())).toEqual(['unrelated-cache']);
  await expect
    .poll(() =>
      page.evaluate(async () =>
        (await navigator.serviceWorker.getRegistrations()).map(
          (entry) => new URL(entry.scope).pathname
        )
      )
    )
    .toEqual(['/__chatto/push/fixture/']);

  // An unregistered worker can still control the current document. A new
  // navigation releases that controller while keeping the scoped push worker.
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  expect(await page.evaluate(() => navigator.serviceWorker.controller === null)).toBe(true);

  await page.context().setOffline(true);
  try {
    const offlineResult = await page.evaluate(async () => {
      try {
        await fetch('/login', { cache: 'no-store' });
        return 'response';
      } catch {
        return 'network-error';
      }
    });
    expect(offlineResult).toBe('network-error');
  } finally {
    await page.context().setOffline(false);
  }
  expect(pageErrors).toEqual([]);
});
