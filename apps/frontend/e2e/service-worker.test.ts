import type { Page } from '@playwright/test';
import { expect, test } from './setup';

type CacheSnapshot = {
  cacheNames: string[];
  rootShellCached: boolean;
  fallbackShellCached: boolean;
  lazyStaticAssetCached: boolean;
  apiDiscoveryCached: boolean;
  apiConnectCached: boolean;
  uploadedAssetCached: boolean;
};

type ServiceWorkerRegistrationSnapshot = {
  scope: string;
  scriptURL: string;
};

type BadgeStateSnapshot = {
  cacheExists: boolean;
  notificationCount: number | null;
  serviceWorkerAppBadgeEnabled: boolean | null;
};

const BADGE_STATE_CACHE_NAME = 'chatto-badge-state-v1';
const BADGE_STATE_REQUEST = '/__chatto/foreground-notification-count';

test('service worker caches only the app shell and serves it offline', async ({
  page,
  context
}) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  const registration = await ensureServiceWorkerControlsPage(page);

  expect(registration.scope).toBe(`${new URL(page.url()).origin}/`);
  expect(registration.scriptURL).toBe(`${new URL(page.url()).origin}/service-worker.js`);

  await requestNetworkOnlyPaths(page);

  const onlineCacheSnapshot = await cacheSnapshot(page);
  expect(onlineCacheSnapshot.cacheNames.some((name) => name.startsWith('chatto-shell-'))).toBe(
    true
  );
  expect(onlineCacheSnapshot.rootShellCached).toBe(true);
  expect(onlineCacheSnapshot.fallbackShellCached).toBe(true);
  expect(onlineCacheSnapshot.lazyStaticAssetCached).toBe(false);
  expect(onlineCacheSnapshot.apiDiscoveryCached).toBe(false);
  expect(onlineCacheSnapshot.apiConnectCached).toBe(false);
  expect(onlineCacheSnapshot.uploadedAssetCached).toBe(false);

  await requestLazyStaticAsset(page);
  const lazyCacheSnapshot = await cacheSnapshot(page);
  expect(lazyCacheSnapshot.lazyStaticAssetCached).toBe(true);

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

test('browser-tab badge-state messages do not crash service worker badging', async ({ page }) => {
  const runtimeFailures: string[] = [];
  page.on('crash', () => runtimeFailures.push('page crashed'));
  page.on('pageerror', (error) => runtimeFailures.push(`page error: ${error.message}`));
  page.on('console', (message) => {
    if (message.type() !== 'error') return;
    if (/BadgeService|bad IPC|bad Mojo/i.test(message.text())) {
      runtimeFailures.push(`browser badging error: ${message.text()}`);
    }
  });

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  await ensureServiceWorkerControlsPage(page);
  await clearBadgeStateCache(page);

  const browserContext = await page.evaluate(() => {
    const standaloneDisplayModes = [
      'standalone',
      'fullscreen',
      'minimal-ui',
      'window-controls-overlay'
    ];
    return {
      controlled: Boolean(navigator.serviceWorker.controller),
      pageBadgingApiPresent: 'setAppBadge' in navigator,
      installedAppDisplayMode: standaloneDisplayModes.some((mode) =>
        window.matchMedia(`(display-mode: ${mode})`).matches
      )
    };
  });
  expect(browserContext.controlled).toBe(true);
  expect(browserContext.installedAppDisplayMode).toBe(false);
  if (!browserContext.pageBadgingApiPresent) {
    test.info().annotations.push({
      type: 'coverage-note',
      description: 'Page Badging API is not exposed in this browser run.'
    });
  }

  await postBadgeStateMessage(page, {
    type: 'chatto-badge-state',
    notificationCount: 3,
    serviceWorkerAppBadgeEnabled: false
  });
  await expectBadgeState(page, {
    cacheExists: true,
    notificationCount: 3,
    serviceWorkerAppBadgeEnabled: false
  });
  await expectPageStillResponsive(page);

  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  await ensureServiceWorkerControlsPage(page);
  await expectBadgeState(page, {
    cacheExists: true,
    notificationCount: 3,
    serviceWorkerAppBadgeEnabled: false
  });

  await postBadgeStateMessage(page, {
    type: 'chatto-badge-state',
    notificationCount: 2
  });
  await expectBadgeState(page, {
    cacheExists: true,
    notificationCount: 2,
    serviceWorkerAppBadgeEnabled: false
  });
  await expectPageStillResponsive(page);

  await postBadgeStateMessage(page, {
    type: 'chatto-badge-state',
    notificationCount: 0,
    serviceWorkerAppBadgeEnabled: false
  });
  await expectBadgeState(page, {
    cacheExists: true,
    notificationCount: 0,
    serviceWorkerAppBadgeEnabled: false
  });
  await expectPageStillResponsive(page);

  expect(runtimeFailures).toEqual([]);
});

async function ensureServiceWorkerControlsPage(
  page: Page
): Promise<ServiceWorkerRegistrationSnapshot> {
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

  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);

  return registration;
}

async function clearBadgeStateCache(page: Page) {
  await page.evaluate(
    async ({ cacheName, request }) => {
      const cache = await caches.open(cacheName);
      await cache.delete(request);
    },
    { cacheName: BADGE_STATE_CACHE_NAME, request: BADGE_STATE_REQUEST }
  );
}

async function postBadgeStateMessage(
  page: Page,
  message: {
    type: 'chatto-badge-state';
    notificationCount: number;
    serviceWorkerAppBadgeEnabled?: boolean;
  }
) {
  await page.evaluate((message) => {
    const controller = navigator.serviceWorker.controller;
    if (!controller) {
      throw new Error('Service worker controller is missing');
    }
    controller.postMessage(message);
  }, message);
}

async function readBadgeState(page: Page): Promise<BadgeStateSnapshot> {
  return page.evaluate(
    async ({ cacheName, request }) => {
      const cache = await caches.open(cacheName);
      const response = await cache.match(request);
      if (!response) {
        return {
          cacheExists: false,
          notificationCount: null,
          serviceWorkerAppBadgeEnabled: null
        };
      }

      const payload = (await response.json()) as {
        notificationCount?: unknown;
        serviceWorkerAppBadgeEnabled?: unknown;
      };
      return {
        cacheExists: true,
        notificationCount:
          typeof payload.notificationCount === 'number' ? payload.notificationCount : null,
        serviceWorkerAppBadgeEnabled:
          typeof payload.serviceWorkerAppBadgeEnabled === 'boolean'
            ? payload.serviceWorkerAppBadgeEnabled
            : null
      };
    },
    { cacheName: BADGE_STATE_CACHE_NAME, request: BADGE_STATE_REQUEST }
  );
}

async function expectBadgeState(page: Page, expected: BadgeStateSnapshot) {
  await expect.poll(() => readBadgeState(page)).toEqual(expected);
}

async function expectPageStillResponsive(page: Page) {
  await expect(page.locator('body')).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.readyState)).toBe('complete');
}

async function requestNetworkOnlyPaths(page: Page) {
  await page.evaluate(async () => {
    await Promise.allSettled([
      fetch('/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer', {
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
      lazyStaticAssetCached: Boolean(await caches.match('/robots.txt')),
      apiDiscoveryCached: Boolean(
        await caches.match('/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer')
      ),
      apiConnectCached: Boolean(await caches.match('/api/connect')),
      uploadedAssetCached: Boolean(await caches.match('/assets/example.png'))
    };
  });
}

async function requestLazyStaticAsset(page: Page) {
  await page.evaluate(async () => {
    const response = await fetch('/robots.txt');
    if (!response.ok) {
      throw new Error(`robots.txt request failed with ${response.status}`);
    }
  });
}
