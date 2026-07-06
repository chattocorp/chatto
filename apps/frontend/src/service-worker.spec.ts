import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$service-worker', () => ({
  build: ['/app.js'],
  files: ['/manifest.webmanifest'],
  version: 'test-version'
}));

type ServiceWorkerHandler = (event: {
  data?: { json: () => unknown };
  notification?: { close: () => void; data?: { url?: string } };
  waitUntil: (promise: Promise<unknown>) => void;
}) => void;

type TestNativeNotification = {
  close?: () => void;
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function createWaitUntilEvent(extra: Record<string, unknown> = {}) {
  const pending: Promise<unknown>[] = [];
  return {
    event: {
      ...extra,
      waitUntil: (promise: Promise<unknown>) => pending.push(promise)
    },
    pending
  };
}

function createMemoryCacheStorage() {
  const cachesByName = new Map<string, Map<string, Response>>();
  return {
    open: vi.fn(async (name: string) => {
      let cache = cachesByName.get(name);
      if (!cache) {
        cache = new Map();
        cachesByName.set(name, cache);
      }

      return {
        match: vi.fn(async (request: RequestInfo | URL) =>
          cache.get(request.toString())?.clone()
        ),
        put: vi.fn(async (request: RequestInfo | URL, response: Response) => {
          cache.set(request.toString(), response.clone());
        }),
        delete: vi.fn(async (request: RequestInfo | URL) => cache.delete(request.toString()))
      };
    }),
    keys: vi.fn(async () => Array.from(cachesByName.keys())),
    delete: vi.fn(async (name: string) => cachesByName.delete(name))
  };
}

async function importServiceWorker(cacheStorage = createMemoryCacheStorage()) {
  const handlers = new Map<string, ServiceWorkerHandler[]>();
  const registration = {
    getNotifications: vi.fn(
      async (_options?: { tag?: string }): Promise<TestNativeNotification[]> => []
    ),
    showNotification: vi.fn(async () => {})
  };
  const clients = {
    claim: vi.fn(async () => {}),
    matchAll: vi.fn(async () => []),
    openWindow: vi.fn(async () => null)
  };
  const setAppBadge = vi.fn(async () => {});
  const clearAppBadge = vi.fn(async () => {});

  vi.stubGlobal('self', {
    location: { origin: 'https://chatto.example' },
    registration,
    clients,
    skipWaiting: vi.fn(),
    addEventListener: vi.fn((type: string, handler: ServiceWorkerHandler) => {
      const list = handlers.get(type) ?? [];
      list.push(handler);
      handlers.set(type, list);
    })
  });
  vi.stubGlobal('navigator', { setAppBadge, clearAppBadge });
  vi.stubGlobal('caches', cacheStorage);

  await import('./service-worker');

  const dispatch = async (type: string, extra: Record<string, unknown> = {}) => {
    const { event, pending } = createWaitUntilEvent(extra);
    for (const handler of handlers.get(type) ?? []) {
      handler(event);
    }
    await Promise.all(pending);
  };

  return {
    clients,
    dispatch,
    getPendingDispatch(type: string, extra: Record<string, unknown> = {}) {
      return createWaitUntilEvent(extra);
    },
    handlers,
    registration,
    setAppBadge,
    clearAppBadge,
    cacheStorage
  };
}

describe('service worker badge orchestration', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not let a stale foreground zero clear a regular pushed notification', async () => {
    const worker = await importServiceWorker();
    const nativeNotification = { close: vi.fn() };
    const listing = deferred<Array<typeof nativeNotification>>();
    worker.registration.getNotifications.mockReturnValueOnce(listing.promise);

    const messageDispatch = worker.getPendingDispatch('message', {
      data: {
        type: 'chatto-badge-state',
        notificationCount: 0,
        serviceWorkerAppBadgeEnabled: true
      }
    });
    for (const handler of worker.handlers.get('message') ?? []) {
      handler(messageDispatch.event);
    }

    await worker.dispatch('push', {
      data: {
        json: () => ({
          title: 'New notification',
          body: 'Hello',
          tag: 'notification-1',
          url: 'https://chatto.example/chat/-/room-1'
        })
      }
    });

    listing.resolve([nativeNotification]);
    await Promise.all(messageDispatch.pending);

    expect(worker.registration.showNotification).toHaveBeenCalledOnce();
    expect(nativeNotification.close).not.toHaveBeenCalled();
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });

  it('preserves a foreground authoritative count after clicking the only native notification', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('message', {
      data: {
        type: 'chatto-badge-state',
        notificationCount: 3,
        serviceWorkerAppBadgeEnabled: true
      }
    });
    worker.registration.getNotifications.mockResolvedValueOnce([]);

    await worker.dispatch('notificationclick', {
      notification: {
        close: vi.fn(),
        data: { url: 'https://chatto.example/chat/-/room-1' }
      }
    });

    expect(worker.clearAppBadge).not.toHaveBeenCalled();
    expect(worker.setAppBadge).toHaveBeenLastCalledWith(3);
  });

  it('preserves a foreground authoritative count after a service worker restart', async () => {
    const cacheStorage = createMemoryCacheStorage();
    const firstWorker = await importServiceWorker(cacheStorage);

    await firstWorker.dispatch('message', {
      data: {
        type: 'chatto-badge-state',
        notificationCount: 3,
        serviceWorkerAppBadgeEnabled: true
      }
    });

    vi.resetModules();
    const restartedWorker = await importServiceWorker(cacheStorage);
    restartedWorker.registration.getNotifications.mockResolvedValueOnce([]);

    await restartedWorker.dispatch('notificationclick', {
      notification: {
        close: vi.fn(),
        data: { url: 'https://chatto.example/chat/-/room-1' }
      }
    });

    expect(restartedWorker.clearAppBadge).not.toHaveBeenCalled();
    expect(restartedWorker.setAppBadge).toHaveBeenLastCalledWith(3);
  });

  it('does not call the worker Badging API for a foreground browser tab', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('message', {
      data: {
        type: 'chatto-badge-state',
        notificationCount: 3,
        serviceWorkerAppBadgeEnabled: false
      }
    });
    worker.registration.getNotifications.mockResolvedValueOnce([]);

    await worker.dispatch('notificationclick', {
      notification: {
        close: vi.fn(),
        data: { url: 'https://chatto.example/chat/-/room-1' }
      }
    });

    expect(worker.clearAppBadge).not.toHaveBeenCalled();
    expect(worker.setAppBadge).not.toHaveBeenCalled();
  });

  it('does not preserve a foreground count after a dismiss push without a fresh count', async () => {
    const worker = await importServiceWorker();
    const staleNotification = { close: vi.fn() };

    await worker.dispatch('message', {
      data: {
        type: 'chatto-badge-state',
        notificationCount: 1,
        serviceWorkerAppBadgeEnabled: true
      }
    });
    worker.registration.getNotifications
      .mockResolvedValueOnce([staleNotification])
      .mockResolvedValueOnce([]);

    await worker.dispatch('push', {
      data: {
        json: () => ({
          action: 'dismiss',
          tag: 'notification-1'
        })
      }
    });

    expect(staleNotification.close).toHaveBeenCalledOnce();
    expect(worker.clearAppBadge).toHaveBeenCalledOnce();
    expect(worker.setAppBadge).toHaveBeenCalledTimes(1);
  });
});
