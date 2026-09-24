import type { Page } from '@playwright/test';
import { expect, test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { getRoomIdByNameViaConnect, postMessageViaConnect } from './fixtures/connectHelpers';

type CacheSnapshot = {
  cacheNames: string[];
};

type ServiceWorkerRegistrationSnapshot = {
  scope: string;
  scriptURL: string;
};

test('service worker caches the shell while leaving private requests on the network', async ({
  page
}) => {
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

  expect(
    (await cacheSnapshot(page)).cacheNames.some((name) => name.startsWith('chatto-shell-'))
  ).toBe(true);

  await requestFrontendResource(page);
  await requestNetworkOnlyPaths(page);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  const cachedRequests = await page.evaluate(async () => {
    const cache = await caches.open(
      (await caches.keys()).find((name) => name.startsWith('chatto-shell-'))!
    );
    return (await cache.keys()).map((request) => new URL(request.url).pathname);
  });
  expect(cachedRequests).toContain('/login');
  expect(
    cachedRequests.some((path) => path.startsWith('/api/') || path.startsWith('/assets/'))
  ).toBe(false);

  await page.context().setOffline(true);
  try {
    await page.goto('/login');
    await expect(
      page.getByRole('heading', { name: 'Choose a server to get started' })
    ).toBeVisible();
  } finally {
    await page.context().setOffline(false);
  }
});

test('offline reload restores saved text in the normal chat view', async ({ page }) => {
  await createAndLoginTestUser(page);
  await page.goto('/chat/-/overview');
  const roomId = await getRoomIdByNameViaConnect(page, 'general');
  await page.goto(`/chat/-/${roomId}`);
  await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
  const message = `Offline saved message ${Date.now()}`;
  await postMessageViaConnect(page, roomId, message);
  await expect(page.getByText(message)).toBeVisible();
  await ensureServiceWorkerIsActive(page);
  const manifest = await page.evaluate(
    async () => (await (await fetch('/manifest.webmanifest')).json()) as { start_url?: string }
  );
  expect(manifest.start_url).toBe('/chat/-');
  await expect
    .poll(() =>
      page.evaluate(async (body) => {
        const db = await new Promise<IDBDatabase>((resolve, reject) => {
          const request = indexedDB.open('chatto-saved-views', 1);
          request.onsuccess = () => resolve(request.result);
          request.onerror = () => reject(request.error);
        });
        try {
          const views = await new Promise<
            Array<{ rooms?: Array<{ messages?: Array<{ body?: string }> }> }>
          >((resolve, reject) => {
            const request = db.transaction('views').objectStore('views').getAll();
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
          });
          return views.some((view) =>
            view.rooms?.some((room) => room.messages?.some((entry) => entry.body === body))
          );
        } finally {
          db.close();
        }
      }, message)
    )
    .toBe(true);

  let releaseViewer: () => void = () => {};
  const viewerHeld = new Promise<void>((resolve) => {
    releaseViewer = resolve;
  });
  const viewerRoute = '**/chatto.api.v1.ViewerService/GetViewer';
  const viewerRequests: Promise<void>[] = [];
  let releaseTimeline: () => void = () => {};
  const timelineHeld = new Promise<void>((resolve) => {
    releaseTimeline = resolve;
  });
  const timelineRoute = '**/chatto.api.v1.RoomService/GetRoomEvents';
  const timelineRequests: Promise<void>[] = [];
  const realtimeSockets: string[] = [];
  const privateRequests: string[] = [];
  page.on('websocket', (socket) => {
    if (socket.url().includes('/api/realtime')) realtimeSockets.push(socket.url());
  });
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname;
    if (
      path.startsWith('/api/connect/') &&
      !path.endsWith('/chatto.api.v1.ViewerService/GetViewer') &&
      !path.endsWith('/chatto.discovery.v1.ServerDiscoveryService/GetServer')
    )
      privateRequests.push(path);
  });
  await page.route(viewerRoute, (route) => {
    const resumed = viewerHeld.then(() => route.continue());
    viewerRequests.push(resumed);
    return resumed;
  });
  await page.route(timelineRoute, (route) => {
    const resumed = timelineHeld.then(() => route.continue());
    timelineRequests.push(resumed);
    return resumed;
  });
  try {
    await page.goto('/chat/-');
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    await expect(page.getByText(message)).toBeVisible();
    await expect.poll(() => viewerRequests.length).toBeGreaterThan(0);
    expect(realtimeSockets).toHaveLength(0);
    expect(privateRequests).toHaveLength(0);
    const timeline = await page.getByTestId('messages-container').elementHandle();
    expect(timeline).not.toBeNull();
    releaseViewer();
    // The live room includes permissions that the saved display data omits.
    // Keep its text mounted while the verified snapshot's timeline is pending.
    await expect.poll(() => timelineRequests.length).toBeGreaterThan(0);
    await expect(page.getByText(message)).toBeVisible();
    expect(await timeline!.evaluate((element) => element.isConnected)).toBe(true);
    releaseTimeline();
    await expect
      .poll(() =>
        page
          .getByTestId('messages-container')
          .evaluate((element) => element.closest('[aria-busy]')?.getAttribute('aria-busy'))
      )
      .toBe('false');
    expect(await timeline!.evaluate((element) => element.isConnected)).toBe(true);
    await timeline!.dispose();
  } finally {
    releaseViewer();
    releaseTimeline();
    await Promise.all(viewerRequests);
    await Promise.all(timelineRequests);
    await page.unroute(viewerRoute);
    await page.unroute(timelineRoute);
  }

  await page.context().setOffline(true);
  try {
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    await expect(page.getByText(message)).toBeVisible();
    await expect(page.getByTestId('saved-view-overlay')).toHaveCount(0);
  } finally {
    await page.context().setOffline(false);
  }

  await page.reload();
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);
  await page.context().setOffline(true);
  try {
    await page.reload();
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    await expect(page.getByText(message)).toBeVisible();
    await expect(page.getByTestId('saved-view-overlay')).toHaveCount(0);

    await page.goto('/chat/-');
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    await expect(page.getByText(message)).toBeVisible();
  } finally {
    await page.context().setOffline(false);
  }
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
          reject(new Error('Chatto did not register the service worker'));
        }, 20_000);

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
