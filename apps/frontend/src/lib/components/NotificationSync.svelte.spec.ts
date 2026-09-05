import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { render } from 'vitest-browser-svelte';
import NotificationSync from './NotificationSync.svelte';
import type { ProjectionHandler } from '$lib/eventBus.svelte';
import { NotificationOccurrencesChangedEvent } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

const { mocks } = vi.hoisted(() => {
  const createBus = () => ({
    projectionHandlers: new Set<ProjectionHandler>()
  });
  const buses = {
    origin: createBus(),
    remote: createBus()
  };
  const createStore = () => ({
    isAuthenticated: true,
    waitForRealtimeResourceRefresh: vi.fn(async () => true),
    notifications: {
      occurrences: [] as Array<{ id?: string; unread: boolean }>,
      count: 0,
      unreadNotificationCount: 0,
      hasLoaded: true,
      nextExpiryAt: null as string | null,
      fetch: vi.fn(async () => {}),
      reconcile: vi.fn(async () => {})
    }
  });
  const stores = {
    origin: createStore(),
    remote: createStore()
  };

  return {
    mocks: {
      buses,
      servers: [{ id: 'origin' }],
      stores,
      badgeRefreshHandlers: new Set<() => void>(),
      playNotificationSound: vi.fn(),
      presencePreference: { effectiveStatus: 0 },
      updateAppBadge: vi.fn(async () => {}),
      soundPreferences: {
        origin: {
          notificationSound: 'soft',
          notificationSoundFilters: {
            volume: 1,
            highPassHz: 20,
            lowPassHz: 20000,
            echo: 0,
            reverb: 0,
            crunch: 0
          }
        },
        remote: {
          notificationSound: 'pop',
          notificationSoundFilters: {
            volume: 0.5,
            highPassHz: 20,
            lowPassHz: 20000,
            echo: 0,
            reverb: 0,
            crunch: 0
          }
        }
      }
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
    getBus: vi.fn((serverId: 'origin' | 'remote') => mocks.buses[serverId])
  }
}));

vi.mock('$lib/state/serverNotificationPreferences.svelte', () => ({
  getServerNotificationPreferences: (serverId: 'origin' | 'remote') =>
    mocks.soundPreferences[serverId]
}));

vi.mock('$lib/audio/notificationSounds', () => ({
  playNotificationSound: mocks.playNotificationSound
}));

vi.mock('$lib/state/presencePreference.svelte', () => ({
  presencePreference: mocks.presencePreference
}));

vi.mock('$lib/notifications/appBadge', () => ({
  listenForAppBadgeRefresh: vi.fn((handler: () => void) => {
    mocks.badgeRefreshHandlers.add(handler);
    return () => mocks.badgeRefreshHandlers.delete(handler);
  }),
  updateAppBadge: mocks.updateAppBadge
}));

function dispatch(
  playNotificationSound = false,
  eventId = 'event-id',
  serverId: 'origin' | 'remote' = 'origin',
  notificationId = 'notification-id'
) {
  const event = new RealtimeProjectionUpdate({
    event: new RealtimeEvent({
      id: eventId,
      event: {
        case: 'notificationOccurrencesChanged',
        value: new NotificationOccurrencesChangedEvent({
          createdNotificationId: playNotificationSound ? notificationId : undefined
        })
      }
    })
  });

  for (const handler of mocks.buses[serverId].projectionHandlers) {
    handler(event);
  }
}

async function renderAndWaitForSubscription() {
  const result = render(NotificationSync);
  const authenticatedServerCount = mocks.servers.filter(
    ({ id }) => mocks.stores[id as keyof typeof mocks.stores].isAuthenticated
  ).length;
  await vi.waitFor(() =>
    expect(
      Object.values(mocks.buses).reduce((count, bus) => count + bus.projectionHandlers.size, 0)
    ).toBe(authenticatedServerCount)
  );
  await vi.waitFor(() => expect(mocks.badgeRefreshHandlers.size).toBe(1));
  return result;
}

describe('NotificationSync', () => {
  beforeEach(() => {
    for (const bus of Object.values(mocks.buses)) bus.projectionHandlers.clear();
    mocks.badgeRefreshHandlers.clear();
    vi.clearAllMocks();
    mocks.presencePreference.effectiveStatus = PresenceStatus.ONLINE;

    mocks.servers.splice(0, mocks.servers.length, { id: 'origin' });
    for (const store of Object.values(mocks.stores)) {
      store.isAuthenticated = true;
      store.notifications.occurrences = [{ id: 'notification-id', unread: true }];
      store.waitForRealtimeResourceRefresh.mockReset().mockResolvedValue(true);
      store.notifications.count = 0;
      store.notifications.unreadNotificationCount = 0;
      store.notifications.hasLoaded = true;
      store.notifications.nextExpiryAt = null;
      store.notifications.fetch.mockClear();
      store.notifications.reconcile.mockClear();
    }
  });

  it('plays a sound for an eligible live in-app notification creation', async () => {
    await renderAndWaitForSubscription();

    dispatch(true);

    await vi.waitFor(() => expect(mocks.playNotificationSound).toHaveBeenCalledOnce());
  });

  it('uses the sound preference for the server that produced the event', async () => {
    mocks.servers.push({ id: 'remote' });
    await renderAndWaitForSubscription();

    dispatch(true, 'origin-event', 'origin');
    dispatch(true, 'remote-event', 'remote');

    await vi.waitFor(() => expect(mocks.playNotificationSound).toHaveBeenCalledTimes(2));

    expect(mocks.playNotificationSound).toHaveBeenNthCalledWith(
      1,
      'soft',
      mocks.soundPreferences.origin.notificationSoundFilters
    );
    expect(mocks.playNotificationSound).toHaveBeenNthCalledWith(
      2,
      'pop',
      mocks.soundPreferences.remote.notificationSoundFilters
    );
  });

  it('plays a repeated creation only once, even with different event IDs', async () => {
    await renderAndWaitForSubscription();

    dispatch(true, 'duplicate-sound-event');
    dispatch(true, 'another-envelope-for-the-same-notification');

    await vi.waitFor(() => expect(mocks.playNotificationSound).toHaveBeenCalledOnce());
  });

  it('waits for the resource read before it checks unread state', async () => {
    const read = Promise.withResolvers<boolean>();
    mocks.stores.origin.waitForRealtimeResourceRefresh.mockReturnValue(read.promise);
    mocks.stores.origin.notifications.occurrences = [];
    await renderAndWaitForSubscription();
    dispatch(true);
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
    mocks.stores.origin.notifications.occurrences = [{ id: 'notification-id', unread: true }];
    read.resolve(true);
    await vi.waitFor(() => expect(mocks.playNotificationSound).toHaveBeenCalledOnce());
  });

  it.each(['read', 'missing', 'failed', 'dnd', 'signed-out'])(
    'does not sound when the resource read completes with %s state',
    async (state) => {
      const read = Promise.withResolvers<boolean>();
      mocks.stores.origin.waitForRealtimeResourceRefresh.mockReturnValue(read.promise);
      await renderAndWaitForSubscription();
      dispatch(true);
      if (state === 'read') mocks.stores.origin.notifications.occurrences[0].unread = false;
      if (state === 'missing') mocks.stores.origin.notifications.occurrences = [];
      if (state === 'dnd') mocks.presencePreference.effectiveStatus = PresenceStatus.DO_NOT_DISTURB;
      if (state === 'signed-out') mocks.stores.origin.isAuthenticated = false;
      if (state === 'failed') read.reject(new Error('Read failed'));
      else read.resolve(true);
      await read.promise.catch(() => {});
      await Promise.resolve();
      expect(mocks.playNotificationSound).not.toHaveBeenCalled();
    }
  );

  it('does not play suppressed creations when DND ends', async () => {
    await renderAndWaitForSubscription();
    mocks.presencePreference.effectiveStatus = PresenceStatus.DO_NOT_DISTURB;
    dispatch(true);
    mocks.presencePreference.effectiveStatus = PresenceStatus.ONLINE;
    dispatch(true, 'duplicate-after-dnd');
    await Promise.resolve();
    expect(mocks.stores.origin.waitForRealtimeResourceRefresh).not.toHaveBeenCalled();
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('plays once for several creations in the same reconciliation batch', async () => {
    const read = Promise.withResolvers<boolean>();
    mocks.stores.origin.waitForRealtimeResourceRefresh.mockReturnValue(read.promise);
    mocks.stores.origin.notifications.occurrences.push({ id: 'second-notification', unread: true });
    await renderAndWaitForSubscription();
    dispatch(true);
    dispatch(true, 'second-event', 'origin', 'second-notification');
    read.resolve(true);
    await vi.waitFor(() => expect(mocks.playNotificationSound).toHaveBeenCalledOnce());
    expect(mocks.stores.origin.waitForRealtimeResourceRefresh).toHaveBeenCalledOnce();
  });

  it('stays silent when a queued resource read failed', async () => {
    mocks.stores.origin.waitForRealtimeResourceRefresh.mockResolvedValue(false);
    await renderAndWaitForSubscription();
    dispatch(true);
    await Promise.resolve();
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('does not sound after the component is removed', async () => {
    const read = Promise.withResolvers<boolean>();
    mocks.stores.origin.waitForRealtimeResourceRefresh.mockReturnValue(read.promise);
    const component = await renderAndWaitForSubscription();
    dispatch(true);
    await component.unmount();
    read.resolve(true);
    await read.promise;
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('does not borrow notification state from another server', async () => {
    mocks.servers.push({ id: 'remote' });
    mocks.stores.remote.notifications.occurrences = [];
    await renderAndWaitForSubscription();
    dispatch(true, 'remote-event', 'remote');
    await Promise.resolve();
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('periodically reconciles notification state after missed live hints', async () => {
    vi.useFakeTimers();
    try {
      await renderAndWaitForSubscription();
      await vi.advanceTimersByTimeAsync(60_000);
      expect(mocks.stores.origin.notifications.reconcile).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not play a sound when the replacement does not request one', async () => {
    await renderAndWaitForSubscription();

    dispatch(false);

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('does not play a sound for reconciliation', async () => {
    await renderAndWaitForSubscription();

    dispatch();

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

  it('clears the app badge when the notification list contains only read notifications', async () => {
    mocks.stores.origin.notifications.occurrences = [{ unread: false }];

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
