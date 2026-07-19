import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$service-worker', () => ({
  build: ['/app.js'],
  files: ['/manifest.webmanifest'],
  version: 'test-version'
}));

type ServiceWorkerHandler = (event: {
  data?: { json: () => unknown };
  notification?: {
    title?: string;
    body?: string;
    icon?: string;
    badge?: string;
    app_badge?: string | number;
    tag?: string;
    data?: { notificationId?: string; url?: string };
    close?: () => void;
  };
  waitUntil: (promise: Promise<unknown>) => void;
}) => void;

type TestNativeNotification = {
  close?: () => void;
};

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
        match: vi.fn(async (request: RequestInfo | URL) => cache.get(request.toString())?.clone()),
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
    showNotification: vi.fn(async (_title: string, _options?: NotificationOptions) => {})
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
    handlers,
    registration,
    setAppBadge,
    clearAppBadge,
    cacheStorage
  };
}

describe('service worker notifications', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('deletes retired foreground badge caches during activation', async () => {
    const cacheStorage = createMemoryCacheStorage();
    await cacheStorage.open('chatto-badge-state-v1');
    await cacheStorage.open('chatto-badge-state-v2');
    const worker = await importServiceWorker(cacheStorage);

    await worker.dispatch('activate');

    await expect(cacheStorage.keys()).resolves.not.toContain('chatto-badge-state-v1');
    await expect(cacheStorage.keys()).resolves.not.toContain('chatto-badge-state-v2');
  });

  it('uses declarative push notification fields when legacy root fields are absent', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('push', {
      data: {
        json: () => ({
          web_push: 8030,
          notification: {
            title: 'Declarative notification',
            body: 'Opened by the browser or worker fallback',
            tag: 'notification-2',
            icon: 'https://chatto.example/icons/icon-192.png',
            badge: 'https://chatto.example/icons/icon-192.png',
            app_badge: '5',
            navigate: 'https://chatto.example/chat/-/room-2?highlight=event-2',
            data: {
              notificationId: 'notif-2',
              url: 'https://chatto.example/chat/-/room-2?highlight=event-2'
            }
          }
        })
      }
    });

    expect(worker.setAppBadge).not.toHaveBeenCalled();
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
    expect(worker.registration.showNotification).toHaveBeenCalledWith('Declarative notification', {
      body: 'Opened by the browser or worker fallback',
      icon: 'https://chatto.example/icons/icon-192.png',
      badge: 'https://chatto.example/icons/icon-192.png',
      tag: 'notification-2',
      data: {
        notificationId: 'notif-2',
        url: 'https://chatto.example/chat/-/room-2?highlight=event-2'
      }
    });
  });

  it('handles mutable declarative push events with event.notification and no payload data', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('push', {
      notification: {
        title: 'Mutable declarative notification',
        body: 'Handled through PushEvent.notification',
        tag: 'notification-3',
        icon: 'https://chatto.example/icons/icon-192.png',
        badge: 'https://chatto.example/icons/icon-192.png',
        data: {
          notificationId: 'notif-3',
          url: 'https://chatto.example/chat/-/room-3?highlight=event-3'
        }
      }
    });

    expect(worker.registration.showNotification).toHaveBeenCalledWith(
      'Mutable declarative notification',
      {
        body: 'Handled through PushEvent.notification',
        icon: 'https://chatto.example/icons/icon-192.png',
        badge: 'https://chatto.example/icons/icon-192.png',
        tag: 'notification-3',
        data: {
          notificationId: 'notif-3',
          url: 'https://chatto.example/chat/-/room-3?highlight=event-3'
        }
      }
    );
  });

  it('uses declarative navigate as the fallback notification click URL', async () => {
    const worker = await importServiceWorker();
    const targetUrl = 'https://chatto.example/chat/-/room-2?highlight=event-2';

    await worker.dispatch('push', {
      data: {
        json: () => ({
          web_push: 8030,
          notification: {
            title: 'Declarative notification',
            navigate: targetUrl,
            data: {
              notificationId: 'notif-2'
            }
          }
        })
      }
    });

    const options = worker.registration.showNotification.mock.calls[0][1] as NotificationOptions;
    await worker.dispatch('notificationclick', {
      notification: {
        close: vi.fn(),
        data: options.data as { url?: string }
      }
    });

    expect(worker.clients.openWindow).toHaveBeenCalledWith(targetUrl);
  });

  it('does not write the app badge after a notification click', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('notificationclick', {
      notification: {
        close: vi.fn(),
        data: { url: 'https://chatto.example/chat/-/room-1' }
      }
    });

    expect(worker.registration.getNotifications).not.toHaveBeenCalled();
    expect(worker.setAppBadge).not.toHaveBeenCalled();
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });

  it('reports notification click routing failures', async () => {
    const worker = await importServiceWorker();
    worker.clients.openWindow.mockRejectedValueOnce(new Error('window activation failed'));
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});

    try {
      await worker.dispatch('notificationclick', {
        notification: {
          close: vi.fn(),
          data: { url: 'https://chatto.example/chat/-/room-1' }
        }
      });

      expect(consoleError).toHaveBeenCalledOnce();
    } finally {
      consoleError.mockRestore();
    }
  });

  it('closes matching native notifications after a dismiss push without writing a badge', async () => {
    const worker = await importServiceWorker();
    const staleNotification = { close: vi.fn() };
    worker.registration.getNotifications.mockResolvedValueOnce([staleNotification]);

    await worker.dispatch('push', {
      data: {
        json: () => ({
          action: 'dismiss',
          tag: 'notification-1'
        })
      }
    });

    expect(staleNotification.close).toHaveBeenCalledOnce();
    expect(worker.registration.getNotifications).toHaveBeenCalledOnce();
    expect(worker.setAppBadge).not.toHaveBeenCalled();
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });
});
