import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { flushSync } from 'svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import { PUSH_PROMPT_SNOOZE_MS, isPushPromptSnoozed } from '$lib/notifications/pushPrompt';
import { toast } from '$lib/ui/toast';
import PushNotificationSetup from './PushNotificationSetup.svelte';

const mocks = vi.hoisted(() => ({
  getPermission: vi.fn(),
  getPushCapability: vi.fn(),
  enablePushOnAllServers: vi.fn(),
  servers: [] as { id: string; reauthRequiredAt: number | null }[],
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

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return mocks.servers;
    }
  }
}));

vi.mock('$lib/notifications/pushNotifications', async () => {
  const { userPreferences: reactivePreferences } =
    await import('$lib/state/userPreferences.svelte');
  return {
    enablePushOnAllServers: mocks.enablePushOnAllServers,
    getPermission: mocks.getPermission,
    getPushCapability: mocks.getPushCapability,
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
    mocks.enablePushOnAllServers.mockReset();
    mocks.enablePushOnAllServers.mockResolvedValue({ permission: 'granted', registrations: [] });
    mocks.getPushCapability.mockReset();
    mocks.getPushCapability.mockReturnValue('supported');
    mocks.servers = [{ id: 'origin', reauthRequiredAt: null }];
    window.localStorage.removeItem('chatto:pushPromptSnoozedUntil');
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

  describe('invitation to enable push notifications', () => {
    const invitation = () => page.getByText('Enable push notifications');

    beforeEach(() => {
      mocks.getPermission.mockReturnValue('default');
    });

    afterEach(() => {
      window.localStorage.removeItem('chatto:pushPromptSnoozedUntil');
    });

    it('never asks the browser without a click on Enable', async () => {
      render(PushNotificationSetup);
      await settle();
      window.dispatchEvent(new MouseEvent('click'));
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }));

      await expect.element(invitation()).toBeVisible();
      expect(mocks.enablePushOnAllServers).not.toHaveBeenCalled();
    });

    it('asks the browser when the user selects Enable', async () => {
      render(PushNotificationSetup);
      await page.getByRole('button', { name: 'Enable', exact: true }).click();

      expect(mocks.enablePushOnAllServers).toHaveBeenCalledOnce();
      expect(isPushPromptSnoozed()).toBe(false);
    });

    it('confirms only when every server saved the subscription', async () => {
      const success = vi.spyOn(toast, 'success');
      mocks.enablePushOnAllServers.mockResolvedValue({
        permission: 'granted',
        registrations: [
          { serverId: 'origin', registered: true },
          { serverId: 'remote', registered: false }
        ]
      });
      render(PushNotificationSetup);
      await page.getByRole('button', { name: 'Enable', exact: true }).click();
      await settle();
      expect(success).not.toHaveBeenCalled();

      mocks.enablePushOnAllServers.mockResolvedValue({
        permission: 'granted',
        registrations: [{ serverId: 'origin', registered: true }]
      });
      await page.getByRole('button', { name: 'Enable', exact: true }).click();
      await expect.poll(() => success).toHaveBeenCalledWith('Push notifications enabled');
      success.mockRestore();
    });

    it('waits 14 days after Not now', async () => {
      render(PushNotificationSetup);
      await page.getByRole('button', { name: 'Not now' }).click();

      await expect.element(invitation()).not.toBeInTheDocument();
      expect(isPushPromptSnoozed()).toBe(true);
      expect(isPushPromptSnoozed(Date.now() + PUSH_PROMPT_SNOOZE_MS)).toBe(false);
    });

    it('treats a dismissed browser prompt as Not now', async () => {
      mocks.enablePushOnAllServers.mockResolvedValue({ permission: 'default', registrations: [] });
      render(PushNotificationSetup);
      await page.getByRole('button', { name: 'Enable', exact: true }).click();

      await expect.element(invitation()).not.toBeInTheDocument();
      expect(isPushPromptSnoozed()).toBe(true);
    });

    it('stays hidden while an earlier Not now is in effect', async () => {
      window.localStorage.setItem('chatto:pushPromptSnoozedUntil', String(Date.now() + 60_000));
      render(PushNotificationSetup);
      await settle();

      await expect.element(invitation()).not.toBeInTheDocument();
    });

    it.each(['granted', 'denied', null] as const)(
      'does not invite while permission is %s',
      async (permission) => {
        mocks.getPermission.mockReturnValue(permission);
        render(PushNotificationSetup);
        await settle();

        await expect.element(invitation()).not.toBeInTheDocument();
      }
    );

    it('does not invite where this browser cannot use push', async () => {
      mocks.getPushCapability.mockReturnValue('ios_home_screen_required');
      render(PushNotificationSetup);
      await settle();

      await expect.element(invitation()).not.toBeInTheDocument();
    });

    it('waits while a server needs sign-in', async () => {
      mocks.servers = [{ id: 'origin', reauthRequiredAt: Date.now() }];
      render(PushNotificationSetup);
      await settle();

      await expect.element(invitation()).not.toBeInTheDocument();
    });

    it('waits for a server that supports push', async () => {
      mocks.stores.origin.serverInfo.pushNotificationsEnabled = false;
      render(PushNotificationSetup);
      await settle();
      await expect.element(invitation()).not.toBeInTheDocument();

      mocks.stores.origin.serverInfo.pushNotificationsEnabled = true;
      userPreferences.composerEditor = 'visual';
      await settle();

      await expect.element(invitation()).toBeVisible();
    });
  });
});
