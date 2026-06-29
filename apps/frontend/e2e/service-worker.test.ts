import type { Page } from '@playwright/test';
import { expect, test } from './setup';

type CacheSnapshot = {
  cacheNames: string[];
  rootShellCached: boolean;
  fallbackShellCached: boolean;
  manifestCached: boolean;
  iconCached: boolean;
  apiDiscoveryCached: boolean;
  apiConnectCached: boolean;
  uploadedAssetCached: boolean;
};

test('service worker caches only the app shell and serves it offline', async ({
  page,
  context
}) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  const registration = await page.evaluate(async () => {
    if (!('serviceWorker' in navigator)) {
      throw new Error('Service workers are not available in this browser');
    }

    const registered = await waitForRegistration();
    const active = registered.active ?? registered.waiting ?? registered.installing;
    if (!active) {
      throw new Error('Service worker registration did not expose a worker');
    }

    if (active.state !== 'activated') {
      await new Promise<void>((resolve, reject) => {
        const timeout = window.setTimeout(() => {
          active.removeEventListener('statechange', onStateChange);
          reject(new Error(`Service worker did not activate; final state: ${active.state}`));
        }, 10_000);

        function onStateChange() {
          if (active.state === 'activated') {
            window.clearTimeout(timeout);
            active.removeEventListener('statechange', onStateChange);
            resolve();
          }
        }

        active.addEventListener('statechange', onStateChange);
      });
    }

    return {
      scope: registered.scope,
      scriptURL: (registered.active ?? active).scriptURL
    };

    async function waitForRegistration(): Promise<ServiceWorkerRegistration> {
      const existing = await navigator.serviceWorker.getRegistration('/');
      if (existing) return existing;

      return new Promise((resolve, reject) => {
        const timeout = window.setTimeout(() => {
          navigator.serviceWorker.removeEventListener('controllerchange', onControllerChange);
          reject(new Error('SvelteKit did not register the service worker'));
        }, 10_000);

        async function onControllerChange() {
          const changed = await navigator.serviceWorker.getRegistration('/');
          if (!changed) return;
          window.clearTimeout(timeout);
          navigator.serviceWorker.removeEventListener('controllerchange', onControllerChange);
          resolve(changed);
        }

        navigator.serviceWorker.addEventListener('controllerchange', onControllerChange);
      });
    }
  });

  expect(registration.scope).toBe(`${new URL(page.url()).origin}/`);
  expect(registration.scriptURL).toBe(`${new URL(page.url()).origin}/service-worker.js`);

  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);

  await requestNetworkOnlyPaths(page);

  const onlineCacheSnapshot = await cacheSnapshot(page);
  expect(onlineCacheSnapshot.cacheNames.some((name) => name.startsWith('chatto-shell-'))).toBe(
    true
  );
  expect(onlineCacheSnapshot.rootShellCached).toBe(true);
  expect(onlineCacheSnapshot.fallbackShellCached).toBe(true);
  expect(onlineCacheSnapshot.manifestCached).toBe(true);
  expect(onlineCacheSnapshot.iconCached).toBe(true);
  expect(onlineCacheSnapshot.apiDiscoveryCached).toBe(false);
  expect(onlineCacheSnapshot.apiConnectCached).toBe(false);
  expect(onlineCacheSnapshot.uploadedAssetCached).toBe(false);

  await context.setOffline(true);
  try {
    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(page.getByRole('heading', { name: 'Welcome to Chatto' })).toBeVisible();

    const offlineCacheSnapshot = await cacheSnapshot(page);
    expect(offlineCacheSnapshot.apiDiscoveryCached).toBe(false);
    expect(offlineCacheSnapshot.apiConnectCached).toBe(false);
    expect(offlineCacheSnapshot.uploadedAssetCached).toBe(false);
  } finally {
    await context.setOffline(false);
  }
});

async function requestNetworkOnlyPaths(page: Page) {
  await page.evaluate(async () => {
    await Promise.allSettled([
      fetch('/api/connect/chatto.api.v1.ServerDiscoveryService/GetServer', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Connect-Protocol-Version': '1'
        },
        body: '{}'
      }),
      fetch('/api/connect'),
      fetch('/assets/example.png')
    ]);
  });
}

async function cacheSnapshot(page: Page) {
  return page.evaluate<CacheSnapshot>(async () => {
    return {
      cacheNames: await caches.keys(),
      rootShellCached: Boolean(await caches.match('/')),
      fallbackShellCached: Boolean(await caches.match('/200.html')),
      manifestCached: Boolean(await caches.match('/manifest.webmanifest')),
      iconCached: Boolean(await caches.match('/icons/icon-192.png')),
      apiDiscoveryCached: Boolean(
        await caches.match('/api/connect/chatto.api.v1.ServerDiscoveryService/GetServer')
      ),
      apiConnectCached: Boolean(await caches.match('/api/connect')),
      uploadedAssetCached: Boolean(await caches.match('/assets/example.png'))
    };
  });
}
