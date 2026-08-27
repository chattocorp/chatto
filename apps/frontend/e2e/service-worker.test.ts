import type { Page } from '@playwright/test';
import { expect, test } from './setup';

type CacheSnapshot = {
  cacheNames: string[];
};

type ServiceWorkerRegistrationSnapshot = {
  scope: string;
  scriptURL: string;
};

test('service worker leaves frontend and data requests to the network', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  const registration = await ensureServiceWorkerIsActive(page);

  expect(registration.scope).toBe(`${new URL(page.url()).origin}/`);
  expect(registration.scriptURL).toBe(`${new URL(page.url()).origin}/service-worker.js`);

  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);

  expect((await cacheSnapshot(page)).cacheNames).toEqual([]);

  await requestFrontendResource(page);
  await requestNetworkOnlyPaths(page);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  expect((await cacheSnapshot(page)).cacheNames).toEqual([]);
});

async function ensureServiceWorkerIsActive(page: Page): Promise<ServiceWorkerRegistrationSnapshot> {
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

  return registration;
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
      cacheNames: await caches.keys()
    };
  });
}

async function requestFrontendResource(page: Page) {
  await page.evaluate(async () => {
    const response = await fetch('/robots.txt');
    if (!response.ok) {
      throw new Error(`robots.txt request failed with ${response.status}`);
    }
  });
}
