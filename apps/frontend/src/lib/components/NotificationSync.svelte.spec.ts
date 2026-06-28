import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import NotificationSync from './NotificationSync.svelte';
import type { EventEnvelope, EventHandler } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';

const { mocks } = vi.hoisted(() => {
  const bus = {
    handlers: new Set<EventHandler>(),
    catchUpHandlers: new Set()
  };
  const store = {
    isAuthenticated: true,
    notifications: {
      count: 0,
      unreadNotificationCount: 0,
      addNotification: vi.fn(() => Promise.resolve()),
      removeNotification: vi.fn(),
      consumeLocalDismissal: vi.fn(),
      fetch: vi.fn(() => Promise.resolve())
    },
    rooms: {
      refreshNotificationCounts: vi.fn(() => Promise.resolve()),
      incrementUnreadNotification: vi.fn(),
      decrementUnreadNotification: vi.fn(),
      refresh: vi.fn()
    },
    roomUnread: {
      hasAnyUnread: false
    }
  };

  return {
    mocks: {
      bus,
      store,
      playNotificationSound: vi.fn(),
      updateBadge: vi.fn(() => Promise.resolve()),
      setFlagBadge: vi.fn(() => Promise.resolve()),
      clearBadge: vi.fn(() => Promise.resolve()),
      syncServiceWorkerUnreadBadgeState: vi.fn()
    }
  };
});

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    servers: [{ id: 'origin' }],
    getStore: vi.fn(() => mocks.store)
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
  updateBadge: mocks.updateBadge,
  setFlagBadge: mocks.setFlagBadge,
  clearBadge: mocks.clearBadge,
  syncServiceWorkerUnreadBadgeState: mocks.syncServiceWorkerUnreadBadgeState
}));

function dispatch(event: Record<string, unknown>) {
  const envelope = {
    id: 'event-id',
    createdAt: new Date().toISOString(),
    actorId: 'actor-id',
    actor: null,
    event
  } as EventEnvelope;

  for (const handler of mocks.bus.handlers) {
    handler(envelope);
  }
}

async function renderAndWaitForSubscription() {
  render(NotificationSync);
  await vi.waitFor(() => expect(mocks.bus.handlers.size).toBe(1));
}

describe('NotificationSync', () => {
  beforeEach(() => {
    mocks.bus.handlers.clear();
    mocks.bus.catchUpHandlers.clear();
    vi.clearAllMocks();

    mocks.store.isAuthenticated = true;
    mocks.store.notifications.count = 0;
    mocks.store.notifications.unreadNotificationCount = 0;
    mocks.store.roomUnread.hasAnyUnread = false;
    mocks.store.notifications.addNotification.mockResolvedValue(undefined);
    mocks.store.notifications.removeNotification.mockReturnValue(null);
    mocks.store.notifications.consumeLocalDismissal.mockReturnValue(false);
    mocks.store.notifications.fetch.mockResolvedValue(undefined);
    mocks.store.rooms.refreshNotificationCounts.mockResolvedValue(undefined);
  });

  it('reconciles authoritative counts on notification creation instead of incrementing locally', async () => {
    await renderAndWaitForSubscription();

    dispatch({
      kind: RoomEventKind.NotificationCreated,
      notificationId: 'n1',
      roomId: 'room-1',
      eventId: 'event-1',
      inReplyToId: null,
      silent: false
    });

    expect(mocks.store.notifications.addNotification).toHaveBeenCalledOnce();
    expect(mocks.store.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    expect(mocks.store.rooms.incrementUnreadNotification).not.toHaveBeenCalled();
    expect(mocks.playNotificationSound).toHaveBeenCalledOnce();
  });

  it('reconciles silent notification creation without playing a sound', async () => {
    await renderAndWaitForSubscription();

    dispatch({
      kind: RoomEventKind.NotificationCreated,
      notificationId: 'n1',
      roomId: 'room-1',
      eventId: 'event-1',
      inReplyToId: null,
      silent: true
    });

    expect(mocks.store.notifications.addNotification).toHaveBeenCalledOnce();
    expect(mocks.store.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('reconciles counts when a cached notification is dismissed elsewhere', async () => {
    mocks.store.notifications.removeNotification.mockReturnValue('room-1');
    await renderAndWaitForSubscription();

    dispatch({
      kind: RoomEventKind.NotificationDismissed,
      notificationId: 'n1'
    });

    expect(mocks.store.notifications.removeNotification).toHaveBeenCalledWith('n1');
    expect(mocks.store.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    expect(mocks.store.rooms.decrementUnreadNotification).not.toHaveBeenCalled();
    expect(mocks.store.notifications.fetch).not.toHaveBeenCalled();
  });

  it('refetches notification state and counts when an uncached remote dismissal arrives', async () => {
    mocks.store.notifications.removeNotification.mockReturnValue(null);
    mocks.store.notifications.consumeLocalDismissal.mockReturnValue(false);
    await renderAndWaitForSubscription();

    dispatch({
      kind: RoomEventKind.NotificationDismissed,
      notificationId: 'unknown-notification'
    });

    expect(mocks.store.notifications.consumeLocalDismissal).toHaveBeenCalledWith(
      'unknown-notification'
    );
    expect(mocks.store.notifications.fetch).toHaveBeenCalledOnce();
    expect(mocks.store.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    expect(mocks.store.rooms.refresh).not.toHaveBeenCalled();
  });

  it('uses the authoritative notification count when the cached list is lower', async () => {
    mocks.store.notifications.count = 1;
    mocks.store.notifications.unreadNotificationCount = 3;

    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.updateBadge).toHaveBeenCalledWith(3));
    expect(mocks.syncServiceWorkerUnreadBadgeState).toHaveBeenCalledWith(false);
    expect(mocks.setFlagBadge).not.toHaveBeenCalled();
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });

  it('clears the app badge when there are no notifications or unread rooms', async () => {
    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.clearBadge).toHaveBeenCalledOnce());
    expect(mocks.syncServiceWorkerUnreadBadgeState).toHaveBeenCalledWith(false);
    expect(mocks.updateBadge).not.toHaveBeenCalled();
    expect(mocks.setFlagBadge).not.toHaveBeenCalled();
  });

  it('shares unread-only badge state with the service worker', async () => {
    mocks.store.roomUnread.hasAnyUnread = true;

    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.setFlagBadge).toHaveBeenCalledOnce());
    expect(mocks.syncServiceWorkerUnreadBadgeState).toHaveBeenCalledWith(true);
    expect(mocks.updateBadge).not.toHaveBeenCalled();
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });
});
