import { describe, expect, it, vi } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  NotificationAttentionLevel,
  NotificationSignalKind,
  type NotificationAPI,
  type NotificationOccurrenceItem,
  type NotificationOccurrencePage
} from '@chatto/client/api/notifications';
import { NotificationStore } from '@chatto/client/server/notifications';
import { NotificationAttention } from './notificationAttention';
import { ReadViewRegistry } from './readViews';

function snapshotOf(occurrences: NotificationOccurrenceItem[]): NotificationOccurrencePage {
  const roomUnreadCounts: Record<string, number> = {};
  const roomImportantUnreadCounts: Record<string, number> = {};
  for (const occurrence of occurrences) {
    const roomId = occurrence.room?.id;
    if (!roomId || !occurrence.unread) continue;
    roomUnreadCounts[roomId] = (roomUnreadCounts[roomId] ?? 0) + 1;
    if (occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT) {
      roomImportantUnreadCounts[roomId] = (roomImportantUnreadCounts[roomId] ?? 0) + 1;
    }
  }
  return {
    occurrences,
    unreadCount: occurrences.length,
    importantUnreadCount: occurrences.length,
    roomUnreadCounts,
    roomImportantUnreadCounts,
    totalCount: occurrences.length,
    hasMore: false
  };
}

function makeAPI(remote: NotificationOccurrenceItem[] = []) {
  return {
    listNotificationOccurrences: vi.fn(async () => snapshotOf(remote))
  } as unknown as NotificationAPI & { listNotificationOccurrences: ReturnType<typeof vi.fn> };
}

const mention = (id: string): NotificationOccurrenceItem => ({
  id,
  createdAt: new Date('2026-04-29T12:00:00Z').toISOString(),
  actor: {
    id: 'a',
    login: 'tester',
    displayName: 'Tester',
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE,
    customStatus: null
  },
  room: { id: 'r1', name: 'general' },
  eventId: 'evt',
  threadRootId: null,
  signalKind: NotificationSignalKind.DIRECT_MENTION,
  targetSupported: true,
  attentionLevel: NotificationAttentionLevel.IMPORTANT,
  unread: true,
  reactionEmoji: null
});

function attentionFor(api = makeAPI()) {
  const views = new ReadViewRegistry();
  const notifications = new NotificationStore(api);
  return { views, notifications, attention: new NotificationAttention(notifications, views) };
}

describe('NotificationAttention', () => {
  it.each(['before', 'after'])(
    'suppresses viewed thread attention when notifications arrive %s registration',
    (order) => {
      const { views, notifications, attention } = attentionFor();
      const rows = [
        { ...mention('thread-a'), threadRootId: 'a' },
        { ...mention('thread-b'), threadRootId: 'b' },
        mention('room')
      ];
      const snapshot = snapshotOf(rows);
      // Include unread activity beyond the retained page. It must not be subtracted.
      snapshot.unreadCount += 4;
      snapshot.importantUnreadCount += 4;
      snapshot.roomUnreadCounts.r1 += 4;
      snapshot.roomImportantUnreadCounts.r1 += 4;
      if (order === 'before') notifications.replaceOccurrenceProjection(snapshot);
      const closeA = views.register({ roomId: 'r1', threadRootId: 'a' });
      if (order === 'after') notifications.replaceOccurrenceProjection(snapshot);

      expect(attention.counts.unreadNotificationCount).toBe(6);
      expect(attention.counts.importantUnreadNotificationCount).toBe(6);
      expect(attention.counts.roomUnreadCounts.r1).toBe(6);
      expect(attention.counts.roomImportantUnreadCounts.r1).toBe(6);
      expect(attention.occurrences.map((row) => row.id)).toEqual(['thread-b', 'room']);
      expect(attention.hasThreadNotification('a')).toBe(false);
      expect(attention.needsAttention(rows[0])).toBe(false);
      expect(notifications.unreadNotificationCount).toBe(7);
      expect(notifications.occurrences.every((row) => row.unread)).toBe(true);

      const closeB = views.register({ roomId: 'r1', threadRootId: 'b' });
      expect(attention.counts.unreadNotificationCount).toBe(5);
      // A stale authoritative snapshot must still pass through the same local rule.
      notifications.replaceOccurrenceProjection(snapshot);
      expect(attention.counts.unreadNotificationCount).toBe(5);
      closeA();
      closeB();
      expect(attention.counts.unreadNotificationCount).toBe(7);
      expect(attention.hasThreadNotification('a')).toBe(true);
    }
  );

  it('resolves a room notification from the loaded occurrences before querying', async () => {
    const api = makeAPI([mention('remote')]);
    const { notifications, attention } = attentionFor(api);
    const cached = mention('cached');
    notifications.occurrences = [cached];

    await expect(attention.resolveRoomNotification('r1')).resolves.toEqual({
      ok: true,
      totalCount: null,
      notification: cached
    });
    expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
  });

  it('filters loaded occurrences by attention level', async () => {
    const api = makeAPI();
    const { notifications, attention } = attentionFor(api);
    const reaction = { ...mention('reaction'), attentionLevel: NotificationAttentionLevel.AMBIENT };
    notifications.occurrences = [reaction, mention('mention')];

    const important = await attention.resolveRoomNotification('r1', {
      attentionLevel: NotificationAttentionLevel.IMPORTANT
    });
    const ambient = await attention.resolveRoomNotification('r1', {
      attentionLevel: NotificationAttentionLevel.AMBIENT
    });
    expect(important.notification?.id).toBe('mention');
    expect(ambient.notification?.id).toBe('reaction');
    expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
  });

  it('asks the server when no loaded occurrence of the room needs attention', async () => {
    const api = makeAPI([mention('remote')]);
    const { views, notifications, attention } = attentionFor(api);
    notifications.occurrences = [mention('viewed')];
    views.register({ roomId: 'r1' });

    const result = await attention.resolveRoomNotification('r1');
    expect(api.listNotificationOccurrences).toHaveBeenCalled();
    expect(result.notification?.id).toBe('remote');
  });

  it('finds thread and direct-message notifications by their targets', () => {
    const { notifications, attention } = attentionFor();
    notifications.occurrences = [
      {
        ...mention('thread-reply'),
        signalKind: NotificationSignalKind.REPLY,
        room: { id: 'room-kind', name: 'general' },
        threadRootId: 'thread-root'
      },
      {
        ...mention('dm-kind'),
        signalKind: NotificationSignalKind.DIRECT_MESSAGE,
        actor: null,
        room: { id: 'dm-room', name: '' }
      }
    ];
    expect(attention.hasThreadNotification('thread-root')).toBe(true);
    expect(attention.hasDMRoomNotification('dm-room')).toBe(true);
    expect(attention.hasDMNotifications()).toBe(true);
    expect(attention.hasNonDMNotifications()).toBe(true);
  });

  it('does not choose an unsupported future target as a server-badge destination', () => {
    const { notifications, attention } = attentionFor();
    const unsupported = {
      ...mention('future-target'),
      createdAt: new Date('2026-04-29T13:00:00Z').toISOString(),
      actor: null,
      targetSupported: false
    };
    const supported = mention('supported-mention');

    notifications.occurrences = [unsupported, supported];
    expect(attention.getNonDMNotification()).toBe(supported);
    notifications.occurrences = [unsupported];
    expect(attention.getNonDMNotification()).toBeUndefined();
  });

  it('keeps direct-message and room lookups apart when they share a room ID', () => {
    const { notifications, attention } = attentionFor();
    const dmA = {
      ...mention('dm-a'),
      signalKind: NotificationSignalKind.DIRECT_MESSAGE,
      createdAt: new Date('2026-04-29T12:00:00Z').toISOString(),
      room: { id: 'roomA', name: '' }
    };
    const dmB = {
      ...dmA,
      id: 'dm-b',
      createdAt: new Date('2026-04-29T13:00:00Z').toISOString()
    };
    const roomMention = { ...mention('mention-same-id'), room: { id: 'roomA', name: 'r' } };
    // Most-recent-first ordering, as a fetch produces.
    notifications.occurrences = [dmB, dmA, roomMention];

    expect(attention.hasDMRoomNotification('roomA')).toBe(true);
    expect(attention.hasDMRoomNotification('roomB')).toBe(false);
    expect(attention.getCachedRoomNotification('roomA', { isDM: true })?.id).toBe('dm-b');
    expect(attention.hasRoomNotification('roomA')).toBe(true);
    notifications.occurrences = [dmB, dmA];
    expect(attention.hasRoomNotification('roomA')).toBe(false);
    expect(attention.hasDMRoomNotification('roomA')).toBe(true);
  });
});
