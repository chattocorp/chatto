import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { NotificationStore } from '@chatto/client/server/notifications';
import {
  desktopNotifications,
  listenForDesktopNotificationClicks
} from '$lib/desktop/notifications.svelte';
import type { ServerStateStore } from '@chatto/client/server/store';
import {
  NotificationAttentionLevel,
  NotificationSignalKind,
  type NotificationAPI,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import DesktopNotificationSync from './DesktopNotificationSync.svelte';
vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => ({ disableRoomCallWideFor: vi.fn() })
}));

const mocks = vi.hoisted(() => ({
  show: vi.fn(async (_request: { id: string; title: string; body: string }) => {}),
  clear: vi.fn(),
  sound: vi.fn(),
  goto: vi.fn(async () => {}),
  highlight: vi.fn(),
  presence: { status: 0 },
  attention: vi.fn((row: NotificationOccurrenceItem) => row.unread)
}));
vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$lib/state/server/serverUi', () => ({
  serverUi: () => ({
    attention: { needsAttention: mocks.attention },
    pendingHighlights: { set: mocks.highlight }
  })
}));
vi.mock('$lib/state/server/presencePreference', () => ({
  presencePreferences: { get: () => mocks.presence }
}));
vi.mock('$lib/state/serverNotificationPreferences.svelte', () => ({
  getServerNotificationPreferences: () => ({
    notificationSound: 'soft',
    notificationSoundFilters: {}
  })
}));
vi.mock('$lib/audio/notificationSounds', () => ({ playNotificationSound: mocks.sound }));

function row(
  id: string,
  seconds: number,
  overrides: Partial<NotificationOccurrenceItem> = {}
): NotificationOccurrenceItem {
  return {
    id,
    createdAt: new Date(seconds * 1000).toISOString(),
    actor: null,
    signalKind: NotificationSignalKind.ROOM_MESSAGE,
    targetSupported: true,
    room: { id: 'room', name: 'private room' },
    eventId: id,
    threadRootId: null,
    attentionLevel: NotificationAttentionLevel.IMPORTANT,
    unread: true,
    ...overrides
  };
}
function fixture() {
  const notifications = new NotificationStore({} as NotificationAPI);
  const rows = {
    set: (items: NotificationOccurrenceItem[], hasMore = true) =>
      notifications.replaceOccurrenceProjection({
        occurrences: items,
        hasMore,
        totalCount: hasMore ? 100 : items.length,
        unreadCount: hasMore ? 100 : items.filter((row) => row.unread).length,
        importantUnreadCount: hasMore ? 100 : items.filter((row) => row.unread).length,
        roomUnreadCounts: {},
        roomImportantUnreadCounts: {}
      })
  };
  rows.set([row('old', 100)]);
  const resetListeners = new Set<(event: { privacy: boolean }) => void>();
  const store = {
    serverId: 'inactive-server',
    viewerId: 'viewer',
    isAuthenticated: true,
    notifications,
    onReset: (callback: (event: { privacy: boolean }) => void) => {
      resetListeners.add(callback);
      return () => resetListeners.delete(callback);
    },
    onRoomAccessLost: () => () => {},
    onPermissionsChanged: () => () => {},
    onSessionEnded: () => () => {}
  } as unknown as ServerStateStore;
  return { rows, store, resetListeners };
}

describe('DesktopNotificationSync', () => {
  let stopClicks: () => void;
  let click: (id: string) => void;
  beforeEach(() => {
    vi.clearAllMocks();
    window.chattoDesktop = {
      notifications: {
        show: mocks.show,
        clear: mocks.clear,
        onClick: (listener) => {
          click = listener;
          return () => {};
        }
      }
    };
    stopClicks = listenForDesktopNotificationClicks();
    desktopNotifications.enabled = true;
    mocks.presence.status = PresenceStatus.ONLINE;
    mocks.attention.mockImplementation((row) => row.unread);
  });
  afterEach(() => {
    stopClicks();
    desktopNotifications.enabled = false;
    delete window.chattoDesktop;
  });

  it('alerts from a polled resource update without a bus or a transport, then clears read alerts', async () => {
    const f = fixture();
    const view = render(DesktopNotificationSync, { store: f.store });
    await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
    expect(mocks.show).not.toHaveBeenCalled();
    f.rows.set([row('new', 101), row('old', 100)]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledOnce());
    expect(mocks.sound).toHaveBeenCalledOnce();
    expect(mocks.show.mock.calls[0][0]).toMatchObject({ title: 'Chatto', body: 'Notifications' });
    f.rows.set([row('new', 101, { unread: false })]);
    await vi.waitFor(() =>
      expect(mocks.clear).toHaveBeenCalledWith([mocks.show.mock.calls[0][0].id])
    );
    expect(mocks.show).toHaveBeenCalledOnce();
    await view.unmount();
    expect(f.resetListeners.size).toBe(0);
  });

  it('delivers the server Web Push title and message preview unchanged', async () => {
    const f = fixture();
    render(DesktopNotificationSync, { store: f.store });
    await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
    const content = {
      title: '@Alice mentioned you in #general',
      body: 'Could you review the release notes?'
    };
    f.rows.set([row('new', 101, { content })]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledOnce());
    expect(mocks.show.mock.calls[0][0]).toMatchObject(content);
    // Repeated projections must not repeat the alert, even if its content changes.
    f.rows.set([row('new', 101, { content: { ...content, body: 'Edited message' } })]);
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    expect(mocks.show).toHaveBeenCalledOnce();
    for (const reset of f.resetListeners) reset({ privacy: true });
    expect(mocks.clear).toHaveBeenCalledWith([mocks.show.mock.calls[0][0].id]);
  });

  it.each(['disabled', 'dnd', 'viewed', 'ambient'] as const)(
    'suppresses %s arrivals without replaying them later',
    async (reason) => {
      const f = fixture();
      render(DesktopNotificationSync, { store: f.store });
      await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
      if (reason === 'disabled') desktopNotifications.enabled = false;
      if (reason === 'dnd') mocks.presence.status = PresenceStatus.DO_NOT_DISTURB;
      if (reason === 'viewed') mocks.attention.mockReturnValue(false);
      const fresh = row(
        'new',
        101,
        reason === 'ambient' ? { attentionLevel: NotificationAttentionLevel.AMBIENT } : {}
      );
      f.rows.set([fresh]);
      // A browser frame lets the store subscriber process the suppressed arrival.
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      );
      desktopNotifications.enabled = true;
      mocks.presence.status = PresenceStatus.ONLINE;
      mocks.attention.mockReturnValue(true);
      f.rows.set([{ ...fresh }]);
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      );
      expect(mocks.show).not.toHaveBeenCalled();
      expect(mocks.sound).not.toHaveBeenCalled();
    }
  );

  it('does not lose its baseline during an ordinary reconnect snapshot', async () => {
    const f = fixture();
    render(DesktopNotificationSync, { store: f.store });
    await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
    for (const reset of f.resetListeners) reset({ privacy: false });
    f.rows.set([row('new', 101), row('old', 100)]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledOnce());
    for (const reset of f.resetListeners) reset({ privacy: true });
    f.rows.set([row('other-account', 200)]);
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
    );
    expect(mocks.show).toHaveBeenCalledOnce();
  });

  it('retains and opens an alert that falls outside the loaded page', async () => {
    const f = fixture();
    render(DesktopNotificationSync, { store: f.store });
    await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
    f.rows.set([row('new', 101)]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledOnce());
    const id = mocks.show.mock.calls[0][0].id;
    mocks.clear.mockClear();
    f.rows.set([row('later', 102)]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledTimes(2));
    expect(mocks.clear).not.toHaveBeenCalledWith([id]);
    click(id);
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledOnce());
    expect(mocks.highlight).toHaveBeenCalledWith('room', null, 'new', 'new');
  });

  it('clears a missing alert when a complete page confirms removal', async () => {
    const f = fixture();
    render(DesktopNotificationSync, { store: f.store });
    await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
    f.rows.set([row('new', 101)]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledOnce());
    const id = mocks.show.mock.calls[0][0].id;
    f.rows.set([], false);
    await vi.waitFor(() => expect(mocks.clear).toHaveBeenCalledWith([id]));
    click(id);
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
