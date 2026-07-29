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

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

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

function createMemoryCacheStorage(beforePut?: () => Promise<void>) {
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
          await beforePut?.();
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
    matchAll: vi.fn(async (): Promise<TestWindowClient[]> => []),
    openWindow: vi.fn(async () => null)
  };
  const setAppBadge = vi.fn(async () => {});
  const clearAppBadge = vi.fn(async () => {});
  const skipWaiting = vi.fn(async () => {});

  vi.stubGlobal('self', {
    location: { origin: 'https://chatto.example' },
    registration,
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

  const startFetch = (request: Pick<Request, 'method' | 'mode' | 'destination' | 'url'>) => {
    const responses: Promise<Response>[] = [];
    const { event, pending } = createWaitUntilEvent({
      request,
      respondWith: (response: Response | Promise<Response>) => {
        responses.push(Promise.resolve(response));
      }
    });
    for (const handler of handlers.get('fetch') ?? []) {
      handler(event);
    }
    if (responses.length !== 1) {
      throw new Error(`expected one service worker response, got ${responses.length}`);
    }
    return {
      response: responses[0],
      lifetime: Promise.all(pending).then(() => {})
    };
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
    skipWaiting,
    cacheStorage,
    startFetch,
    async dispatchFetch(request: Pick<Request, 'method' | 'mode' | 'destination' | 'url'>) {
      const fetch = startFetch(request);
      const response = await fetch.response;
      await fetch.lifetime;
      return response;
    }
  };
}

describe('service worker frontend caching', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not fetch build assets while installing', async () => {
    const worker = await importServiceWorker();
    const fetchMock = vi.fn();
    const waiting = deferred<void>();
    worker.skipWaiting.mockReturnValueOnce(waiting.promise);
    vi.stubGlobal('fetch', fetchMock);

    const install = worker.getPendingDispatch('install');
    for (const handler of worker.handlers.get('install') ?? []) {
      handler(install.event);
    }

    expect(fetchMock).not.toHaveBeenCalled();
    expect(worker.skipWaiting).toHaveBeenCalledOnce();
    expect(install.pending).toEqual([waiting.promise]);

    waiting.resolve();
    await Promise.all(install.pending);
  });

  it('caches known frontend assets only after they are requested', async () => {
    const worker = await importServiceWorker();
    const fetchMock = vi.fn(async () => new Response('app'));
    vi.stubGlobal('fetch', fetchMock);
    const request = {
      method: 'GET',
      mode: 'same-origin',
      destination: 'script',
      url: 'https://chatto.example/app.js'
    } as const;

    expect(await (await worker.dispatchFetch(request)).text()).toBe('app');
    expect(await (await worker.dispatchFetch(request)).text()).toBe('app');

    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('returns a fetched asset without waiting for a best-effort cache write', async () => {
    const cacheWrite = deferred<void>();
    const worker = await importServiceWorker(createMemoryCacheStorage(() => cacheWrite.promise));
    const fetchMock = vi.fn(async () => new Response('app'));
    vi.stubGlobal('fetch', fetchMock);
    const request = {
      method: 'GET',
      mode: 'same-origin',
      destination: 'script',
      url: 'https://chatto.example/app.js'
    } as const;

    const fetch = worker.startFetch(request);
    expect(await (await fetch.response).text()).toBe('app');

    cacheWrite.resolve();
    await fetch.lifetime;
  });

  it('keeps a fetched asset usable when the best-effort cache write fails', async () => {
    const worker = await importServiceWorker(
      createMemoryCacheStorage(async () => {
        throw new Error('cache quota exceeded');
      })
    );
    const fetchMock = vi.fn(async () => new Response('app'));
    vi.stubGlobal('fetch', fetchMock);
    const request = {
      method: 'GET',
      mode: 'same-origin',
      destination: 'script',
      url: 'https://chatto.example/app.js'
    } as const;

    const fetch = worker.startFetch(request);
    expect(await (await fetch.response).text()).toBe('app');
    await expect(fetch.lifetime).resolves.toBeUndefined();
  });

  it('does not retry a failed frontend asset request', async () => {
    const worker = await importServiceWorker();
    const fetchMock = vi.fn().mockRejectedValueOnce(new TypeError('offline'));
    vi.stubGlobal('fetch', fetchMock);
    const request = {
      method: 'GET',
      mode: 'same-origin',
      destination: 'script',
      url: 'https://chatto.example/app.js'
    } as const;

    const fetch = worker.startFetch(request);
    await expect(fetch.response).rejects.toThrow('offline');
    await expect(fetch.lifetime).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('reuses a successfully requested navigation as a best-effort shell fallback', async () => {
    const worker = await importServiceWorker();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('<main>Chatto</main>'))
      .mockRejectedValueOnce(new TypeError('offline'));
    vi.stubGlobal('fetch', fetchMock);
    const request = {
      method: 'GET',
      mode: 'navigate',
      destination: 'document',
      url: 'https://chatto.example/chat/server/room'
    } as const;

    expect(await (await worker.dispatchFetch(request)).text()).toBe('<main>Chatto</main>');
    expect(await (await worker.dispatchFetch(request)).text()).toBe('<main>Chatto</main>');

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

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
          notification: {
            title: 'Origin notification',
            navigate: 'https://chatto.example/chat/-/room-1'
          }
        })
      }
    });

    expect(visibleClient.postMessage).toHaveBeenCalledWith({ type: 'app-badge-refresh' });
    expect(worker.setAppBadge).not.toHaveBeenCalled();
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

  it('closes matching native notifications and updates the badge when the app is closed', async () => {
    const worker = await importServiceWorker();
    const staleNotification = { close: vi.fn() };
    worker.registration.getNotifications.mockResolvedValueOnce([staleNotification]);

    await worker.dispatch('push', {
      data: {
        json: () => ({
          action: 'dismiss',
          tag: 'notification-1',
          app_badge: '2'
        })
      }
    });

    expect(staleNotification.close).toHaveBeenCalledOnce();
    expect(worker.registration.getNotifications).toHaveBeenCalledOnce();
    expect(worker.clients.matchAll).toHaveBeenCalledWith({
      type: 'window',
      includeUncontrolled: true
    });
    expect(worker.setAppBadge).toHaveBeenCalledWith(2);
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });

  it('leaves dismiss badge updates to an open app client', async () => {
    const worker = await importServiceWorker();
    worker.clients.matchAll.mockResolvedValueOnce([
      { id: 'open-app', visibilityState: 'visible', postMessage: vi.fn() }
    ]);

    await worker.dispatch('push', {
      data: {
        json: () => ({
          action: 'dismiss',
          tag: 'notification-1',
          app_badge: 1
        })
      }
    });

    expect(worker.setAppBadge).not.toHaveBeenCalled();
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });

  it('updates a dismiss badge when the only app client is hidden', async () => {
    const worker = await importServiceWorker();
    worker.clients.matchAll.mockResolvedValueOnce([
      { id: 'background-app', visibilityState: 'hidden', postMessage: vi.fn() }
    ]);

    await worker.dispatch('push', {
      data: {
        json: () => ({
          action: 'dismiss',
          tag: 'notification-1',
          app_badge: 1
        })
      }
    });

    expect(worker.setAppBadge).toHaveBeenCalledWith(1);
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });

  it('ignores dismiss badge updates without a valid authoritative count', async () => {
    const worker = await importServiceWorker();

    await worker.dispatch('push', {
      data: {
        json: () => ({
          action: 'dismiss',
          tag: 'notification-1',
          app_badge: '-1'
        })
      }
    });

    expect(worker.clients.matchAll).not.toHaveBeenCalled();
    expect(worker.setAppBadge).not.toHaveBeenCalled();
    expect(worker.clearAppBadge).not.toHaveBeenCalled();
  });
});
