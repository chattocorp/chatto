import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import NotificationSync from './NotificationSync.svelte';
import type { ProjectionHandler } from '$lib/eventBus.svelte';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionNotificationAction,
  RealtimeProjectionNotificationChange,
  RealtimeProjectionNotificationsReplace,
  RealtimeProjectionOperation
} from '@chatto/api-types/realtime/v1/realtime_pb';

const { mocks } = vi.hoisted(() => {
  const bus = {
    projectionHandlers: new Set<ProjectionHandler>()
  };
  const createStore = () => ({
    isAuthenticated: true,
    notifications: {
      notifications: [] as Array<{ kind: string }>,
      groups: [] as Array<{ unread: boolean; reasons: number[] }>,
      count: 0,
      unreadNotificationCount: 0,
      hasLoaded: true,
      nextExpiryAt: null as string | null,
      fetch: vi.fn(async () => {})
    }
  });
  const stores = {
    origin: createStore(),
    remote: createStore()
  };

  return {
    mocks: {
      bus,
      servers: [{ id: 'origin' }],
      stores,
      badgeRefreshHandlers: new Set<() => void>(),
      playNotificationSound: vi.fn(),
      updateAppBadge: vi.fn(async () => {})
    }
  };
});

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    getStore: vi.fn((serverId: 'origin' | 'remote') => mocks.stores[serverId])
  }
}));

vi.mock('$lib/state/server/eventBus.svelte', () => ({
  eventBusManager: {
    getBus: vi.fn(() => mocks.bus)
  }
}));

vi.mock('$lib/state/userPreferences.svelte', () => ({
  userPreferences: {
    notificationSound: 'soft',
    notificationSoundFilters: {
      volume: 1,
      highPassHz: 20,
      lowPassHz: 20000,
      echo: 0,
      reverb: 0,
      crunch: 0
    }
  }
}));

vi.mock('$lib/audio/notificationSounds', () => ({
  playNotificationSound: mocks.playNotificationSound
}));

vi.mock('$lib/notifications/appBadge', () => ({
  listenForAppBadgeRefresh: vi.fn((handler: () => void) => {
    mocks.badgeRefreshHandlers.add(handler);
    return () => mocks.badgeRefreshHandlers.delete(handler);
  }),
  updateAppBadge: mocks.updateAppBadge
}));

function dispatch(change?: RealtimeProjectionNotificationChange) {
  const event = new RealtimeProjectionEvent({
    id: 'event-id',
    operations: [
      new RealtimeProjectionOperation({
        operation: {
          case: 'notificationsReplace',
          value: new RealtimeProjectionNotificationsReplace({ change })
        }
      })
    ]
  });

  for (const handler of mocks.bus.projectionHandlers) {
    handler(event);
  }
}

async function renderAndWaitForSubscription() {
  render(NotificationSync);
  const authenticatedServerCount = mocks.servers.filter(
    ({ id }) => mocks.stores[id as keyof typeof mocks.stores].isAuthenticated
  ).length;
  await vi.waitFor(() => expect(mocks.bus.projectionHandlers.size).toBe(authenticatedServerCount));
  await vi.waitFor(() => expect(mocks.badgeRefreshHandlers.size).toBe(1));
}

describe('NotificationSync', () => {
  beforeEach(() => {
    mocks.bus.projectionHandlers.clear();
    mocks.badgeRefreshHandlers.clear();
    vi.clearAllMocks();

    mocks.servers.splice(0, mocks.servers.length, { id: 'origin' });
    for (const store of Object.values(mocks.stores)) {
      store.isAuthenticated = true;
      store.notifications.notifications = [];
      store.notifications.groups = [];
      store.notifications.count = 0;
      store.notifications.unreadNotificationCount = 0;
      store.notifications.hasLoaded = true;
      store.notifications.nextExpiryAt = null;
      store.notifications.fetch.mockClear();
    }
  });

  it('plays a sound for a live non-silent notification creation', async () => {
    await renderAndWaitForSubscription();

    dispatch(
      new RealtimeProjectionNotificationChange({
        action: RealtimeProjectionNotificationAction.CREATED,
        notificationId: 'n1',
        silent: false
      })
    );

    expect(mocks.playNotificationSound).toHaveBeenCalledOnce();
  });

  it('does not play a sound for a silent notification creation', async () => {
    await renderAndWaitForSubscription();

    dispatch(
      new RealtimeProjectionNotificationChange({
        action: RealtimeProjectionNotificationAction.CREATED,
        notificationId: 'n1',
        silent: true
      })
    );

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('does not play a sound for reconciliation or occurrence updates', async () => {
    await renderAndWaitForSubscription();

    dispatch();
    dispatch(
      new RealtimeProjectionNotificationChange({
        action: RealtimeProjectionNotificationAction.UPDATED,
        notificationId: 'n1'
      })
    );

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('refreshes authoritative notification state at the next expiry boundary', async () => {
    mocks.stores.origin.notifications.nextExpiryAt = new Date().toISOString();
    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.stores.origin.notifications.fetch).toHaveBeenCalledOnce());
  });

  it('uses the exact unread-occurrence count for the installed-app badge', async () => {
    mocks.stores.origin.notifications.unreadNotificationCount = 2;

    await renderAndWaitForSubscription();

    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 2 })
    );
  });

  it('uses a numeric badge for every notification cause', async () => {
    mocks.stores.origin.notifications.unreadNotificationCount = 1;

    await renderAndWaitForSubscription();

    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 1 })
    );
  });

  it('uses the server aggregate independently of the bounded group page', async () => {
    mocks.stores.origin.notifications.unreadNotificationCount = 3;

    await renderAndWaitForSubscription();

    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 3 })
    );
  });

  it('aggregates exact unread-occurrence counts across authenticated servers', async () => {
    mocks.servers.push({ id: 'remote' });
    mocks.stores.origin.notifications.unreadNotificationCount = 1;
    mocks.stores.remote.notifications.unreadNotificationCount = 2;

    await renderAndWaitForSubscription();

    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 3 })
    );
  });

  it('keeps an exact numeric badge when the group page is truncated', async () => {
    mocks.stores.origin.notifications.unreadNotificationCount = 3;

    await renderAndWaitForSubscription();

    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 3 })
    );
  });

  it('reasserts the unchanged aggregate badge after a regular push', async () => {
    mocks.stores.origin.notifications.unreadNotificationCount = 1;
    await renderAndWaitForSubscription();
    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 1 })
    );
    mocks.updateAppBadge.mockClear();

    for (const refresh of mocks.badgeRefreshHandlers) refresh();

    await vi.waitFor(() =>
      expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'count', count: 1 })
    );
  });

  it('clears an existing app badge once empty notification stores have loaded', async () => {
    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'clear' }));
  });

  it('clears the app badge when Inbox contains only read notifications', async () => {
    mocks.stores.origin.notifications.notifications = [{ kind: 'directMessage' }];

    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'clear' }));
  });

  it('owns a zero badge while signed out and reasserts it after a push', async () => {
    mocks.stores.origin.isAuthenticated = false;
    render(NotificationSync);
    await vi.waitFor(() => expect(mocks.badgeRefreshHandlers.size).toBe(1));
    await vi.waitFor(() => expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'clear' }));
    mocks.updateAppBadge.mockClear();

    for (const refresh of mocks.badgeRefreshHandlers) refresh();

    await vi.waitFor(() => expect(mocks.updateAppBadge).toHaveBeenCalledWith({ kind: 'clear' }));
  });

  it('does not clear the app badge before notifications have loaded', async () => {
    mocks.stores.origin.notifications.hasLoaded = false;

    await renderAndWaitForSubscription();

    expect(mocks.updateAppBadge).not.toHaveBeenCalled();
  });
});
