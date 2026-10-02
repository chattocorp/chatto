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

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

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
      mocks.getPermission.mockReturnValue(permission);
      render(PushNotificationSetup);
      await settle();

      expect(mocks.refreshPushSubscriptions).not.toHaveBeenCalled();
    }
  );

  it('refreshes granted-permission subscriptions on startup', async () => {
    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledOnce();
    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith();
  });

  it('checks for subscriptions that are due for a refresh when the window gets focus', async () => {
    render(PushNotificationSetup);
    await settle();
    mocks.refreshPushSubscriptions.mockClear();

    window.dispatchEvent(new FocusEvent('focus'));
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledOnce();
    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledWith();
  });

  it('does not reconcile when push is not configured', async () => {
    mocks.stores.origin.serverInfo.pushNotificationsEnabled = false;
    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).not.toHaveBeenCalled();
  });

  it('reconciles a server that becomes eligible after mount', async () => {
    mocks.stores.origin.serverInfo.pushNotificationsEnabled = false;
    render(PushNotificationSetup);
    await settle();
    expect(mocks.refreshPushSubscriptions).not.toHaveBeenCalled();

    mocks.stores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.remote.serverInfo.vapidPublicKey = 'remote-vapid';
    userPreferences.composerEditor = 'visual';
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledOnce();
  });

  it('reconciles authenticated remote servers independently', async () => {
    mocks.stores.origin.isAuthenticated = false;
    mocks.stores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.stores.remote.serverInfo.vapidPublicKey = 'remote-vapid';
    render(PushNotificationSetup);
    await settle();

    expect(mocks.refreshPushSubscriptions).toHaveBeenCalledOnce();
  });
});
