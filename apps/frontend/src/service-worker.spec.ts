import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$service-worker', () => ({
  build: ['/_app/immutable/entry.js'],
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

type TestWindowClient = {
  id: string;
  visibilityState: 'hidden' | 'visible';
  postMessage: ReturnType<typeof vi.fn>;
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
  const caches = new Map<string, Map<string, { arrayBuffer: () => Promise<ArrayBuffer> }>>();
  return {
    open: vi.fn(async (name: string) => {
      let entries = caches.get(name);
      if (!entries) {
        entries = new Map();
        caches.set(name, entries);
      }
      const cache = entries;
      return {
        addAll: vi.fn(async (urls: string[]) => {
          for (const url of urls) cache.set(url, { arrayBuffer: async () => new ArrayBuffer(1) });
        }),
        keys: vi.fn(async () => [...cache.keys()]),
        match: vi.fn(async (request: string) => cache.get(request))
      };
    }),
    keys: vi.fn(async () => [...caches.keys()]),
    delete: vi.fn(async (name: string) => caches.delete(name))
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
    matchAll: vi.fn(async (): Promise<TestWindowClient[]> => []),
    openWindow: vi.fn(async () => null)
  };
  const setAppBadge = vi.fn(async () => {});
  const clearAppBadge = vi.fn(async () => {});
  const skipWaiting = vi.fn(async () => {});

  vi.stubGlobal('self', {
    location: { origin: 'https://chatto.example' },
    registration: { ...registration, scope: 'https://chatto.example/' },
    clients,
    skipWaiting,
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
    skipWaiting
  };
}

describe('service worker notifications', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('installs the versioned shell and its fetch handler', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('install');

    expect(worker.skipWaiting).toHaveBeenCalledOnce();
    expect(worker.handlers.has('fetch')).toBe(true);
  });

  it('serves a cached chat document without waiting for navigation fetch', async () => {
    const cacheStorage = createMemoryCacheStorage();
    const worker = await importServiceWorker(cacheStorage);
    await worker.dispatch('install');
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    let response: Promise<unknown> | undefined;

    const handler = worker.handlers.get('fetch')?.[0];
    handler?.({
      request: { url: 'https://chatto.example/chat/-/R1', method: 'GET', mode: 'navigate' },
      respondWith: (pending: Promise<unknown>) => {
        response = pending;
      }
    } as never);

    expect(response).toBeDefined();
    expect(await response).toBe(
      await (await cacheStorage.open('chatto-shell-test-version')).match('/login')
    );
    expect(fetch).not.toHaveBeenCalled();
  });

  it('deletes retired shell and foreground badge caches during activation', async () => {
    const cacheStorage = createMemoryCacheStorage();
    await cacheStorage.open('chatto-shell-old-version');
    await cacheStorage.open('chatto-badge-state-v1');
    await cacheStorage.open('chatto-badge-state-v2');
    await cacheStorage.open('unrelated-cache');
    const worker = await importServiceWorker(cacheStorage);

    await worker.dispatch('install');
    await worker.dispatch('activate');

    await expect(cacheStorage.keys()).resolves.toEqual([
      'unrelated-cache',
      'chatto-shell-test-version'
    ]);
    expect(worker.clients.claim).toHaveBeenCalledOnce();
  });

  it.each(['legacy', 'declarative', 'event'])(
    'preserves cleanup identity from %s push payloads',
    async (format) => {
      const worker = await importServiceWorker();
      const data = {
        notificationId: 'occurrence',
        serverOrigin: 'https://chat.example.com',
        recipientId: 'recipient'
      };
      const notification = { title: 'Push', data };
      await worker.dispatch(
        'push',
        format === 'event'
          ? { notification }
          : {
              data: {
                json: () => (format === 'legacy' ? { title: 'Push', ...data } : { notification })
              }
            }
      );
      expect(worker.registration.showNotification).toHaveBeenCalledWith(
        'Push',
        expect.objectContaining({
          data: expect.objectContaining(data)
        })
      );
    }
  );

  it('uses declarative push notification fields when legacy root fields are absent', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('push', {
      data: {
        json: () => ({
          web_push: 8030,
          app_badge: '5',
          notification: {
            title: 'Declarative notification',
            body: 'Opened by the browser or worker fallback',
            tag: 'notification-2',
            icon: 'https://chatto.example/icons/icon-192.png',
            badge: 'https://chatto.example/icons/icon-192.png',
            app_badge: '5',
            navigate: 'https://chatto.example/chat/-/room-2?highlight=event-2',
            data: {
              attentionLevel: 'important',
              notificationId: 'notif-2',
              url: 'https://chatto.example/chat/-/room-2?highlight=event-2'
            }
          }
        })
      }
    });

    expect(worker.setAppBadge).toHaveBeenCalledExactlyOnceWith();
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

  it.each(['legacy', 'declarative', 'event'])(
    'badges only explicit important attention from %s pushes',
    async (format) => {
      const worker = await importServiceWorker();
      for (const attentionLevel of ['important', 'ambient', undefined, 'future']) {
        worker.setAppBadge.mockClear();
        const notification = { title: 'Activity', data: { attentionLevel } };
        await worker.dispatch(
          'push',
          format === 'event'
            ? { notification }
            : {
                data: {
                  json: () =>
                    format === 'legacy' ? { title: 'Activity', attentionLevel } : { notification }
                }
              }
        );
        if (attentionLevel === 'important') {
          expect(worker.setAppBadge).toHaveBeenCalledExactlyOnceWith();
        } else {
          expect(worker.setAppBadge).not.toHaveBeenCalled();
        }
      }
      expect(worker.registration.showNotification).toHaveBeenCalledTimes(4);
      expect(worker.clearAppBadge).not.toHaveBeenCalled();
    }
  );

  it('asks a visible app to restore its aggregate badge after a regular push', async () => {
    const worker = await importServiceWorker();
    const visibleClient = {
      id: 'visible-app',
      visibilityState: 'visible' as const,
      postMessage: vi.fn()
    };
    worker.clients.matchAll.mockResolvedValueOnce([visibleClient]);

    await worker.dispatch('push', {
      data: {
        json: () => ({
          web_push: 8030,
          app_badge: '2',
          attentionLevel: 'important',
          notification: {
            title: 'Origin notification',
            navigate: 'https://chatto.example/chat/-/room-1'
          }
        })
      }
    });

    expect(visibleClient.postMessage).toHaveBeenCalledWith({ type: 'app-badge-refresh' });
    expect(worker.setAppBadge).toHaveBeenCalledExactlyOnceWith();
    expect(worker.setAppBadge.mock.invocationCallOrder[0]).toBeLessThan(
      visibleClient.postMessage.mock.invocationCallOrder[0]
    );
  });

  it.each(['unavailable', 'rejected'])(
    'still displays and reconciles a push when badging is %s',
    async (failure) => {
      const worker = await importServiceWorker();
      const visibleClient: TestWindowClient = {
        id: 'visible-app',
        visibilityState: 'visible',
        postMessage: vi.fn()
      };
      worker.clients.matchAll.mockResolvedValueOnce([visibleClient]);
      if (failure === 'unavailable') {
        vi.stubGlobal('navigator', {});
      } else {
        worker.setAppBadge.mockRejectedValueOnce(new Error('Badging unavailable'));
      }

      await worker.dispatch('push', {
        data: { json: () => ({ title: 'Important activity', attentionLevel: 'important' }) }
      });

      expect(worker.registration.showNotification).toHaveBeenCalledOnce();
      expect(visibleClient.postMessage).toHaveBeenCalledWith({ type: 'app-badge-refresh' });
    }
  );

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
});
