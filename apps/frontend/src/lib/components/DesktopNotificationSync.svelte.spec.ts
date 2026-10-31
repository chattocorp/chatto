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
import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import { desktopNotificationContent } from '$lib/desktop/notificationContent';
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
  const getEventById = vi.fn<(_id: string) => TimelineEventView | null>(() => null);
  const store = {
    serverId: 'inactive-server',
    viewerId: 'viewer',
    isAuthenticated: true,
    notifications,
    knownMessage: (_roomId: string, eventId: string) => getEventById(eventId),
    onReset: (callback: (event: { privacy: boolean }) => void) => {
      resetListeners.add(callback);
      return () => resetListeners.delete(callback);
    },
    onRoomAccessLost: () => () => {},
    onUserDeleted: () => () => {},
    onPermissionsChanged: () => () => {},
    onSessionEnded: () => () => {}
  } as unknown as ServerStateStore;
  return { rows, store, resetListeners, getEventById };
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
    expect(mocks.show.mock.calls[0][0].body).toBe('#private room');
    expect(mocks.show.mock.calls[0][0].title).toBe('New activity');
    f.rows.set([row('new', 101, { unread: false })]);
    await vi.waitFor(() =>
      expect(mocks.clear).toHaveBeenCalledWith([mocks.show.mock.calls[0][0].id])
    );
    expect(mocks.show).toHaveBeenCalledOnce();
    await view.unmount();
    expect(f.resetListeners.size).toBe(0);
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

  it.each([
    [NotificationSignalKind.DIRECT_MESSAGE, 'Alice sent you a DM'],
    [NotificationSignalKind.DIRECT_MENTION, 'Alice mentioned you'],
    [NotificationSignalKind.ROLE_MENTION, 'Alice mentioned you'],
    [NotificationSignalKind.HERE, 'Alice mentioned you'],
    [NotificationSignalKind.ALL, 'Alice mentioned you'],
    [NotificationSignalKind.REPLY, 'Alice replied to you'],
    [NotificationSignalKind.FOLLOWED_THREAD, 'Alice replied in a thread you follow'],
    [NotificationSignalKind.ROOM_MESSAGE, 'Alice posted a new message'],
    [NotificationSignalKind.FOLLOWED_ROOM, 'Alice posted a new message'],
    [NotificationSignalKind.ROOM_INVITATION, 'Alice invited you to a room'],
    [NotificationSignalKind.REACTION, 'Alice reacted with 👍']
  ] as const)('formats %s from occurrence data without loading a message', (signalKind, title) => {
    const f = fixture();
    const item = row('new', 101, {
      signalKind,
      actor: {
        id: 'alice',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        avatarUrl: null,
        bio: null,
        timezone: null,
        presenceStatus: PresenceStatus.ONLINE,
        customStatus: null
      },
      reactionEmoji: 'thumbsup',
      eventId: signalKind === NotificationSignalKind.ROOM_INVITATION ? '' : 'new'
    });
    expect(desktopNotificationContent(f.store, item)).toEqual({
      title,
      body: signalKind === NotificationSignalKind.DIRECT_MESSAGE ? '' : '#private room'
    });
    // The fixture exposes no request API or store-creation method.
  });

  it('uses a loaded message immediately and does not repeat an alert when content changes', async () => {
    const f = fixture();
    const message = {
      id: 'new',
      createdAt: new Date().toISOString(),
      event: {
        kind: 'messagePosted',
        roomId: 'room',
        body: 'Ready for review',
        attachments: [],
        reactions: [],
        replyCount: 0,
        threadParticipants: []
      }
    } satisfies TimelineEventView;
    f.getEventById.mockReturnValue(message);
    render(DesktopNotificationSync, { store: f.store });
    await vi.waitFor(() => expect(f.resetListeners.size).toBe(1));
    f.rows.set([row('new', 101)]);
    await vi.waitFor(() => expect(mocks.show).toHaveBeenCalledOnce());
    expect(mocks.show.mock.calls[0][0].body).toBe('#private room\nReady for review');
    f.getEventById.mockReturnValue(null);
    f.rows.set([row('new', 101)]);
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    expect(mocks.show).toHaveBeenCalledOnce();
    expect(mocks.sound).toHaveBeenCalledOnce();

    f.getEventById.mockReturnValue({ ...message, actorResolution: 'deleted' });
    expect(desktopNotificationContent(f.store, row('new', 101)).body).toBe('#private room');
    f.getEventById.mockReturnValue({
      ...message,
      event: { ...message.event, deletedAt: new Date().toISOString() }
    });
    expect(desktopNotificationContent(f.store, row('new', 101)).body).toBe('#private room');
    f.store.notifications.revokedRoomIds.add('room');
    f.getEventById.mockClear();
    expect(desktopNotificationContent(f.store, row('new', 101)).body).toBe('#private room');
    expect(f.getEventById).not.toHaveBeenCalled();
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
