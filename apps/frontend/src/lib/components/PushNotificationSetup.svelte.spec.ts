import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PushNotificationSetup from './PushNotificationSetup.svelte';

const mocks = vi.hoisted(() => ({
  refreshPushSubscriptions: vi.fn(),
  stores: {
    origin: {
      isAuthenticated: true,
      serverInfo: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'origin-vapid' as string | null
      }
    },
    remote: {
      isAuthenticated: true,
      serverInfo: {
        pushNotificationsEnabled: false,
        vapidPublicKey: null as string | null
      }
    }
  }
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  getPushRegistrationTargets: () => {
    const targets = [];
    for (const serverId of ['origin', 'remote'] as const) {
      const store = mocks.stores[serverId];
      if (
        !store.isAuthenticated ||
        !store.serverInfo.pushNotificationsEnabled ||
        !store.serverInfo.vapidPublicKey
      ) {
        continue;
      }
      targets.push({ serverId, vapidPublicKey: store.serverInfo.vapidPublicKey });
    }
    return targets;
  },
  refreshPushSubscriptions: mocks.refreshPushSubscriptions
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    servers: [{ id: 'origin' }, { id: 'remote' }],
    tryGetStore: (serverId: 'origin' | 'remote') => mocks.stores[serverId],
    isOriginServer: (serverId: string) => serverId === 'origin'
  }
}));

type ServiceWorkerListener = (event: Event) => void;

function installServiceWorkerStub() {
  const listeners = new Set<ServiceWorkerListener>();
  const serviceWorker = {
    addEventListener: vi.fn((type: string, listener: ServiceWorkerListener) => {
      if (type === 'controllerchange') listeners.add(listener);
    }),
    removeEventListener: vi.fn((type: string, listener: ServiceWorkerListener) => {
      if (type === 'controllerchange') listeners.delete(listener);
    }),
    dispatchControllerChange() {
      for (const listener of listeners) {
        listener(new Event('controllerchange'));
      }
    },
    listenerCount() {
      return listeners.size;
    }
  };

  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: serviceWorker
  });

  return serviceWorker;
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('PushNotificationSetup', () => {
  beforeEach(() => {
    mocks.refreshPushSubscriptions.mockReset();
    mocks.stores.origin.isAuthenticated = true;
    mocks.stores.origin.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.origin.serverInfo.vapidPublicKey = 'origin-vapid';
    mocks.stores.remote.isAuthenticated = true;
    mocks.stores.remote.serverInfo.pushNotificationsEnabled = false;
    mocks.stores.remote.serverInfo.vapidPublicKey = null;
  });

  it('refreshes granted-permission subscriptions on startup and service worker controller changes', async () => {
    const serviceWorker = installServiceWorkerStub();

    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith([
      { serverId: 'origin', vapidPublicKey: 'origin-vapid' }
    ]);
    expect(serviceWorker.addEventListener).toHaveBeenCalledWith(
      'controllerchange',
      expect.any(Function)
    );

    serviceWorker.dispatchControllerChange();
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledTimes(2);
  });

  it('does not reconcile when push is not configured', async () => {
    const serviceWorker = installServiceWorkerStub();
    mocks.stores.origin.serverInfo.pushNotificationsEnabled = false;

    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).not.toHaveBeenCalled();
    expect(serviceWorker.listenerCount()).toBe(0);
  });

  it('reconciles authenticated remote servers independently', async () => {
    installServiceWorkerStub();
    mocks.stores.origin.isAuthenticated = false;
    mocks.stores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.remote.serverInfo.vapidPublicKey = 'remote-vapid';

    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith([
      { serverId: 'remote', vapidPublicKey: 'remote-vapid' }
    ]);
  });
});
