import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PushNotificationSetup from './PushNotificationSetup.svelte';

const mocks = vi.hoisted(() => ({
  ensureRegistered: vi.fn(),
  serverInfo: {
    pushNotificationsEnabled: true,
    vapidPublicKey: 'vapid-key' as string | null
  }
}));

vi.mock('$lib/notifications/pushNotifications', () => ({
  ensureRegistered: mocks.ensureRegistered
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    originServer: { id: 'origin' },
    getStore: () => ({
      serverInfo: mocks.serverInfo
    })
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
    mocks.ensureRegistered.mockReset();
    mocks.serverInfo.pushNotificationsEnabled = true;
    mocks.serverInfo.vapidPublicKey = 'vapid-key';
  });

  it('refreshes granted-permission subscriptions on startup and service worker controller changes', async () => {
    const serviceWorker = installServiceWorkerStub();

    render(PushNotificationSetup);
    await settle();

    expect(mocks.ensureRegistered).toHaveBeenCalledWith('vapid-key', { prompt: false });
    expect(serviceWorker.addEventListener).toHaveBeenCalledWith(
      'controllerchange',
      expect.any(Function)
    );

    serviceWorker.dispatchControllerChange();
    await settle();

    expect(mocks.ensureRegistered).toHaveBeenCalledTimes(2);
    expect(mocks.ensureRegistered).toHaveBeenLastCalledWith('vapid-key', { prompt: false });
  });

  it('does not reconcile when push is not configured', async () => {
    const serviceWorker = installServiceWorkerStub();
    mocks.serverInfo.pushNotificationsEnabled = false;

    render(PushNotificationSetup);
    await settle();

    expect(mocks.ensureRegistered).not.toHaveBeenCalled();
    expect(serviceWorker.listenerCount()).toBe(0);
  });
});
