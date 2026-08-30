import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import PushNotificationSetup from './PushNotificationSetup.svelte';

const mocks = vi.hoisted(() => ({
  getPermission: vi.fn(),
  refreshPushSubscriptions: vi.fn(),
  stores: {
    origin: {
      isAuthenticated: true,
      currentUser: { user: { id: 'origin-user' } },
      serverInfo: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'origin-vapid' as string | null
      }
    },
    remote: {
      isAuthenticated: true,
      currentUser: { user: { id: 'remote-user' } },
      serverInfo: {
        pushNotificationsEnabled: false,
        vapidPublicKey: null as string | null
      }
    }
  }
}));

vi.mock('$lib/notifications/pushNotifications', async () => {
  const { userPreferences: reactivePreferences } =
    await import('$lib/state/userPreferences.svelte');
  return {
    getPermission: mocks.getPermission,
    getPushRegistrationTargets: () => {
      // Give the mutable fixture the same reactive invalidation behaviour as
      // the real server registry.
      void reactivePreferences.composerEditor;
      const targets = [];
      for (const serverId of ['origin', 'remote'] as const) {
        const store = mocks.stores[serverId];
        if (
          !store.isAuthenticated ||
          !store.currentUser.user.id ||
          !store.serverInfo.pushNotificationsEnabled ||
          !store.serverInfo.vapidPublicKey
        ) {
          continue;
        }
        targets.push({
          serverId,
          userId: store.currentUser.user.id,
          vapidPublicKey: store.serverInfo.vapidPublicKey
        });
      }
      return targets;
    },
    refreshPushSubscriptions: mocks.refreshPushSubscriptions
  };
});

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
      for (const listener of listeners) listener(new Event('controllerchange'));
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

const originTarget = {
  serverId: 'origin',
  userId: 'origin-user',
  vapidPublicKey: 'origin-vapid'
};

describe('PushNotificationSetup', () => {
  beforeEach(() => {
    mocks.getPermission.mockReset();
    mocks.getPermission.mockReturnValue('granted');
    mocks.refreshPushSubscriptions.mockReset();
    userPreferences.composerEditor = 'markdown';
    mocks.stores.origin.isAuthenticated = true;
    mocks.stores.origin.currentUser.user.id = 'origin-user';
    mocks.stores.origin.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.origin.serverInfo.vapidPublicKey = 'origin-vapid';
    mocks.stores.remote.isAuthenticated = true;
    mocks.stores.remote.currentUser.user.id = 'remote-user';
    mocks.stores.remote.serverInfo.pushNotificationsEnabled = false;
    mocks.stores.remote.serverInfo.vapidPublicKey = null;
  });

  it.each(['default', 'denied'] as const)(
    'does not reconcile while browser permission is %s',
    async (permission) => {
      installServiceWorkerStub();
      mocks.getPermission.mockReturnValue(permission);
      render(PushNotificationSetup);
      await settle();

      expect(mocks.refreshPushSubscriptions).not.toHaveBeenCalled();
    }
  );

  it('refreshes granted-permission subscriptions on startup and service worker changes', async () => {
    const serviceWorker = installServiceWorkerStub();
    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith([originTarget]);
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
    expect(serviceWorker.listenerCount()).toBe(1);
  });

  it('reconciles a server that becomes eligible after mount', async () => {
    installServiceWorkerStub();
    mocks.stores.origin.serverInfo.pushNotificationsEnabled = false;
    render(PushNotificationSetup);
    await settle();
    expect(mocks.refreshPushSubscriptions).not.toHaveBeenCalled();

    mocks.stores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.remote.serverInfo.vapidPublicKey = 'remote-vapid';
    userPreferences.composerEditor = 'visual';
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith([
      {
        serverId: 'remote',
        userId: 'remote-user',
        vapidPublicKey: 'remote-vapid'
      }
    ]);
  });

  it('reconciles authenticated remote servers independently', async () => {
    installServiceWorkerStub();
    mocks.stores.origin.isAuthenticated = false;
    mocks.stores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.remote.serverInfo.vapidPublicKey = 'remote-vapid';
    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith([
      {
        serverId: 'remote',
        userId: 'remote-user',
        vapidPublicKey: 'remote-vapid'
      }
    ]);
  });
});
