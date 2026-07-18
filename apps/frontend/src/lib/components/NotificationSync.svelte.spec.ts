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
  const store = {
    isAuthenticated: true,
    notifications: {
      notifications: [] as Array<{ kind: string }>,
      count: 0,
      unreadNotificationCount: 0,
      hasLoaded: true
    }
  };

  return {
    mocks: {
      bus,
      store,
      playNotificationSound: vi.fn(),
      updateBadge: vi.fn(() => Promise.resolve()),
      clearBadge: vi.fn(() => Promise.resolve()),
      syncServiceWorkerNotificationBadgeState: vi.fn()
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
  clearBadge: mocks.clearBadge,
  syncServiceWorkerNotificationBadgeState: mocks.syncServiceWorkerNotificationBadgeState
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
  await vi.waitFor(() => expect(mocks.bus.projectionHandlers.size).toBe(1));
}

describe('NotificationSync', () => {
  beforeEach(() => {
    mocks.bus.projectionHandlers.clear();
    vi.clearAllMocks();

    mocks.store.isAuthenticated = true;
    mocks.store.notifications.notifications = [];
    mocks.store.notifications.count = 0;
    mocks.store.notifications.unreadNotificationCount = 0;
    mocks.store.notifications.hasLoaded = true;
  });

  it('plays a sound for a live non-silent notification creation', async () => {
    await renderAndWaitForSubscription();

    dispatch(new RealtimeProjectionNotificationChange({
      action: RealtimeProjectionNotificationAction.CREATED,
      notificationId: 'n1',
      silent: false
    }));

    expect(mocks.playNotificationSound).toHaveBeenCalledOnce();
  });

  it('does not play a sound for a silent notification creation', async () => {
    await renderAndWaitForSubscription();

    dispatch(new RealtimeProjectionNotificationChange({
      action: RealtimeProjectionNotificationAction.CREATED,
      notificationId: 'n1',
      silent: true
    }));

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('does not play a sound for reconciliation or dismissal replacements', async () => {
    await renderAndWaitForSubscription();

    dispatch();
    dispatch(new RealtimeProjectionNotificationChange({
      action: RealtimeProjectionNotificationAction.DISMISSED,
      notificationId: 'n1'
    }));

    expect(mocks.playNotificationSound).not.toHaveBeenCalled();
  });

  it('uses a numeric app badge for loaded DM notifications', async () => {
    mocks.store.notifications.notifications = [{ kind: 'directMessage' }];
    mocks.store.notifications.count = 1;
    mocks.store.notifications.unreadNotificationCount = 1;

    await renderAndWaitForSubscription();

    await vi.waitFor(() =>
      expect(mocks.updateBadge).toHaveBeenCalledWith({ kind: 'count', count: 1 })
    );
    expect(mocks.syncServiceWorkerNotificationBadgeState).toHaveBeenCalledWith({
      kind: 'count',
      count: 1
    });
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });

  it('uses a flag instead of a capped DM count when notifications are not fully loaded', async () => {
    mocks.store.notifications.notifications = [{ kind: 'directMessage' }];
    mocks.store.notifications.count = 1;
    mocks.store.notifications.unreadNotificationCount = 3;

    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.updateBadge).toHaveBeenCalledWith({ kind: 'flag' }));
    expect(mocks.syncServiceWorkerNotificationBadgeState).toHaveBeenCalledWith({ kind: 'flag' });
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });

  it('uses a flag app badge for channel notifications', async () => {
    mocks.store.notifications.notifications = [{ kind: 'mention' }];
    mocks.store.notifications.count = 1;
    mocks.store.notifications.unreadNotificationCount = 1;

    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.updateBadge).toHaveBeenCalledWith({ kind: 'flag' }));
    expect(mocks.syncServiceWorkerNotificationBadgeState).toHaveBeenCalledWith({ kind: 'flag' });
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });

  it('clears the app badge when there are no notifications or unread rooms', async () => {
    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.clearBadge).toHaveBeenCalledOnce());
    expect(mocks.syncServiceWorkerNotificationBadgeState).toHaveBeenCalledWith({ kind: 'clear' });
    expect(mocks.updateBadge).not.toHaveBeenCalled();
  });

  it('does not treat startup zero as authoritative before notifications load', async () => {
    mocks.store.notifications.hasLoaded = false;

    await renderAndWaitForSubscription();

    expect(mocks.syncServiceWorkerNotificationBadgeState).not.toHaveBeenCalled();
    expect(mocks.updateBadge).not.toHaveBeenCalled();
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });

  it('still publishes a positive count before all stores are loaded', async () => {
    mocks.store.notifications.hasLoaded = false;
    mocks.store.notifications.unreadNotificationCount = 2;

    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.updateBadge).toHaveBeenCalledWith({ kind: 'flag' }));
    expect(mocks.syncServiceWorkerNotificationBadgeState).toHaveBeenCalledWith({ kind: 'flag' });
    expect(mocks.clearBadge).not.toHaveBeenCalled();
  });

  it('clears the dock badge when there are no notifications', async () => {
    await renderAndWaitForSubscription();

    await vi.waitFor(() => expect(mocks.clearBadge).toHaveBeenCalledOnce());
    expect(mocks.syncServiceWorkerNotificationBadgeState).toHaveBeenCalledWith({ kind: 'clear' });
    expect(mocks.updateBadge).not.toHaveBeenCalled();
  });
});
