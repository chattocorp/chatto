import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  NotificationStore,
  notificationTarget,
  type NotificationItem
} from './notifications.svelte';
import {
  NotificationItemKind,
  type NotificationAPI,
  type NotificationPage
} from '@chatto/api-client/notifications';

type MockNotificationAPI = NotificationAPI & {
  listNotifications: ReturnType<typeof vi.fn>;
  listRoomNotifications: ReturnType<typeof vi.fn>;
  hasNotifications: ReturnType<typeof vi.fn>;
  listNotificationCounts: ReturnType<typeof vi.fn>;
  dismissNotification: ReturnType<typeof vi.fn>;
  dismissAllNotifications: ReturnType<typeof vi.fn>;
};

function page(items: NotificationItem[], totalCount = items.length): NotificationPage {
  return {
    items,
    totalCount,
    hasMore: false,
    serverName: 'Chatto'
  };
}

function makeAPI(
  options: {
    notifications?: NotificationPage;
    roomNotifications?: NotificationPage;
    notificationsError?: Error;
    roomNotificationsError?: Error;
    dismissNotification?: (notificationId: string) => Promise<boolean> | boolean;
    dismissAllNotifications?: () => Promise<number> | number;
  } = {}
): MockNotificationAPI {
  return {
    listNotifications: vi.fn().mockImplementation(async () => {
      if (options.notificationsError) throw options.notificationsError;
      return options.notifications ?? page([]);
    }),
    listRoomNotifications: vi.fn().mockImplementation(async () => {
      if (options.roomNotificationsError) throw options.roomNotificationsError;
      return options.roomNotifications ?? page([]);
    }),
    hasNotifications: vi.fn().mockResolvedValue(false),
    listNotificationCounts: vi.fn().mockResolvedValue({}),
    dismissNotification: vi
      .fn()
      .mockImplementation(async (notificationId: string) =>
        options.dismissNotification ? options.dismissNotification(notificationId) : true
      ),
    dismissAllNotifications: vi
      .fn()
      .mockImplementation(async () =>
        options.dismissAllNotifications ? options.dismissAllNotifications() : 0
      )
  };
}

const mention = (id: string): NotificationItem =>
  ({
    kind: NotificationItemKind.Mention,
    id,
    createdAt: new Date('2026-04-29T12:00:00Z').toISOString(),
    actor: {
      id: 'a',
      login: 'tester',
      displayName: 'Tester',
      avatarUrl: null,
      presenceStatus: 'OFFLINE'
    },
    summary: 'mentioned you',
    mentionSpace: { id: 's1', name: 'Space' },
    mentionRoom: { id: 'r1', name: 'general' },
    mentionEventId: 'evt'
  }) as unknown as NotificationItem;

describe('NotificationStore', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('populates notifications on success', async () => {
    const store = new NotificationStore(
      makeAPI({ notifications: page([mention('n1'), mention('n2')]) })
    );
    await store.fetch();
    expect(store.notifications).toHaveLength(2);
    expect(store.error).toBeNull();
    expect(store.hasLoaded).toBe(true);
  });

  it('fetchRoomNotification returns the newest room-scoped notification and caches it', async () => {
    const roomMention = mention('room-mention');
    const store = new NotificationStore(makeAPI({ roomNotifications: page([roomMention], 4) }));

    const result = await store.fetchRoomNotification('r1');

    expect(result).toEqual({
      ok: true,
      totalCount: 4,
      notification: roomMention
    });
    expect(store.notifications.map((n) => n.id)).toEqual(['room-mention']);
  });

  it('fetchRoomNotification reports an empty room-scoped notification result', async () => {
    const store = new NotificationStore(makeAPI({ roomNotifications: page([], 0) }));

    const result = await store.fetchRoomNotification('r1');

    expect(result).toEqual({
      ok: true,
      totalCount: 0,
      notification: null
    });
    expect(store.notifications).toHaveLength(0);
  });

  it('resolveRoomNotification uses the cached room notification before querying', async () => {
    const cached = mention('cached');
    const api = makeAPI({ roomNotifications: page([mention('remote')], 1) });
    const store = new NotificationStore(api);
    store.notifications = [cached];

    const result = await store.resolveRoomNotification('r1');

    expect(result).toEqual({
      ok: true,
      totalCount: null,
      notification: cached
    });
    expect(api.listRoomNotifications).not.toHaveBeenCalled();
  });

  it('routes notification targets to the same room/thread/event used by push payloads', () => {
    const store = new NotificationStore(makeAPI());
    const threadMention = {
      kind: NotificationItemKind.Mention,
      id: 'thread-mention',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'mentioned you',
      mentionRoom: { id: 'room-2', name: 'general' },
      mentionEventId: 'mention-event',
      mentionInThread: 'thread-root'
    } as unknown as NotificationItem;
    const threadReply = {
      kind: NotificationItemKind.Reply,
      id: 'thread-reply',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'replied to you',
      replyRoom: { id: 'room-2', name: 'general' },
      replyEventId: 'reply-event',
      inReplyToId: 'mid-thread-msg',
      replyInThread: 'thread-root'
    } as unknown as NotificationItem;
    const roomMessage = {
      kind: NotificationItemKind.RoomMessage,
      id: 'room-message',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'posted a message',
      roomMsgRoom: { id: 'room-news', name: 'news' },
      roomMsgEventId: 'room-event'
    } as unknown as NotificationItem;

    expect(notificationTarget(threadMention)).toMatchObject({
      roomId: 'room-2',
      eventId: 'mention-event',
      threadRootId: 'thread-root'
    });
    expect(store.getNavigationPath('origin', threadMention)).toBe(
      '/chat/-/room-2/thread-root?highlight=mention-event'
    );

    expect(notificationTarget(threadReply)).toMatchObject({
      roomId: 'room-2',
      eventId: 'reply-event',
      threadRootId: 'thread-root'
    });
    expect(store.getNavigationPath('origin', threadReply)).toBe(
      '/chat/-/room-2/thread-root?highlight=reply-event'
    );

    expect(notificationTarget(roomMessage)).toMatchObject({
      roomId: 'room-news',
      eventId: 'room-event',
      threadRootId: null
    });
    expect(store.getNavigationPath('origin', roomMessage)).toBe(
      '/chat/-/room-news?highlight=room-event'
    );
  });

  it('routes and dismisses notifications using notification item kind', async () => {
    const threadReply = {
      kind: NotificationItemKind.Reply,
      id: 'thread-reply-kind',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'replied to you',
      replyRoom: { id: 'room-kind', name: 'general' },
      replyEventId: 'reply-event',
      inReplyToId: 'parent-message',
      replyInThread: 'thread-root'
    } as unknown as NotificationItem;
    const dm = {
      kind: NotificationItemKind.DirectMessage,
      id: 'dm-kind',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'sent you a message',
      room: { id: 'dm-room' }
    } as unknown as NotificationItem;

    const dismissedIds: string[] = [];
    const store = new NotificationStore(
      makeAPI({
        dismissNotification: (notificationId) => {
          dismissedIds.push(notificationId);
          return true;
        }
      })
    );
    store.notifications = [threadReply, dm];

    expect(notificationTarget(threadReply)).toMatchObject({
      isDM: false,
      roomId: 'room-kind',
      eventId: 'reply-event',
      threadRootId: 'thread-root'
    });
    expect(store.hasThreadNotification('thread-root')).toBe(true);
    expect(store.hasDMRoomNotification('dm-room')).toBe(true);

    await store.dismissThreadNotifications('thread-root');

    expect(dismissedIds).toEqual(['thread-reply-kind']);
    expect(store.notifications.map((n) => n.id)).toEqual(['dm-kind']);
  });

  it('retains existing notifications when the server returns an API error', async () => {
    const store = new NotificationStore(
      makeAPI({
        notificationsError: new Error('Cannot query field "threadRootEventId"')
      })
    );
    // Pre-populate as if a previous fetch had succeeded.
    store.notifications = [mention('original')];

    await store.fetch();

    expect(store.notifications).toHaveLength(1);
    expect(store.notifications[0].id).toBe('original');
    expect(store.error).toContain('Cannot query field');
    expect(store.hasLoaded).toBe(false);
    expect(consoleError).toHaveBeenCalled();
  });

  it('does not throw on API error', async () => {
    const store = new NotificationStore(
      makeAPI({ notificationsError: new Error('something broke') })
    );
    await expect(store.fetch()).resolves.toBeUndefined();
    expect(store.error).toBe('something broke');
  });

  it('does not throw on network/transport error', async () => {
    const store = new NotificationStore(makeAPI({ notificationsError: new Error('network down') }));
    store.notifications = [mention('keepme')];
    await expect(store.fetch()).resolves.toBeUndefined();
    // Existing notifications survive a network blip too.
    expect(store.notifications).toHaveLength(1);
    expect(store.error).toBe('network down');
  });

  // Mentions inside a thread must NOT be dismissed when the user enters the
  // parent room — they should only clear when the thread itself is opened
  // (via dismissThreadNotifications), mirroring how thread replies behave.
  it('dismissMentionNotifications skips mentions that are inside a thread', async () => {
    const roomMention = {
      kind: NotificationItemKind.Mention,
      id: 'room-mention',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'mentioned you',
      mentionSpace: { id: 's1', name: 'S' },
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e1',
      mentionInThread: null
    } as unknown as NotificationItem;
    const threadMention = {
      kind: NotificationItemKind.Mention,
      id: 'thread-mention',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'mentioned you',
      mentionSpace: { id: 's1', name: 'S' },
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e2',
      mentionInThread: 'thread-root'
    } as unknown as NotificationItem;

    const dismissedIds: string[] = [];
    const store = new NotificationStore(
      makeAPI({
        dismissNotification: (notificationId) => {
          dismissedIds.push(notificationId);
          return true;
        }
      })
    );
    store.notifications = [roomMention, threadMention];

    await store.dismissMentionNotifications('r1');

    expect(dismissedIds).toEqual(['room-mention']);
    expect(store.notifications.map((n) => n.id)).toEqual(['thread-mention']);
  });

  it('dismissMentionNotifications reports dismissed counts by room', async () => {
    const roomMentionA = {
      kind: NotificationItemKind.Mention,
      id: 'room-mention-a',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'mentioned you',
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e1',
      mentionInThread: null
    } as unknown as NotificationItem;
    const roomMentionB = {
      ...roomMentionA,
      id: 'room-mention-b',
      mentionEventId: 'e2'
    } as unknown as NotificationItem;

    const store = new NotificationStore(makeAPI());
    store.notifications = [roomMentionA, roomMentionB];
    store.setUnreadNotificationCount(2);

    const counts = await store.dismissMentionNotifications('r1');

    expect(counts).toEqual({ total: 2, byRoom: { r1: 2 } });
    expect(store.unreadNotificationCount).toBe(0);
  });

  // Opening the thread clears both thread-replies AND thread-mentions in one
  // pass (the code path called from ThreadPane).
  it('dismissThreadNotifications clears thread-scoped mentions too', async () => {
    const threadMention = {
      kind: NotificationItemKind.Mention,
      id: 'thread-mention',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'a',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'mentioned you',
      mentionSpace: { id: 's1', name: 'S' },
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e2',
      mentionInThread: 'thread-root'
    } as unknown as NotificationItem;

    const dismissedIds: string[] = [];
    const store = new NotificationStore(
      makeAPI({
        dismissNotification: (notificationId) => {
          dismissedIds.push(notificationId);
          return true;
        }
      })
    );
    store.notifications = [threadMention];

    await store.dismissThreadNotifications('thread-root');

    expect(dismissedIds).toEqual(['thread-mention']);
    expect(store.notifications).toHaveLength(0);
  });

  it('suppresses live echo refreshes for locally dismissed notifications', async () => {
    const store = new NotificationStore(makeAPI());
    store.notifications = [mention('local')];

    await store.dismiss('local');

    expect(store.consumeLocalDismissal('local')).toBe(true);
    expect(store.consumeLocalDismissal('local')).toBe(false);
  });

  // The DM list dot uses hasDMRoomNotification per conversation. It must
  // match DM notifications by room, and ignore non-DM notifications even if
  // they happen to share a room id.
  it('hasDMRoomNotification / getDMRoomNotification scope to DM notifications by room', () => {
    const dmA = {
      kind: NotificationItemKind.DirectMessage,
      id: 'dm-a',
      createdAt: new Date('2026-04-29T12:00:00Z').toISOString(),
      actor: {
        id: 'u',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'hi',
      room: { id: 'roomA' }
    } as unknown as NotificationItem;
    const dmB = {
      kind: NotificationItemKind.DirectMessage,
      id: 'dm-b',
      createdAt: new Date('2026-04-29T13:00:00Z').toISOString(),
      actor: {
        id: 'u',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'later',
      room: { id: 'roomA' }
    } as unknown as NotificationItem;
    const roomMention = {
      kind: NotificationItemKind.Mention,
      id: 'mention-same-id',
      createdAt: new Date().toISOString(),
      actor: {
        id: 'u',
        login: 't',
        displayName: 't',
        avatarUrl: null,
        presenceStatus: 'OFFLINE'
      },
      summary: 'mention',
      mentionSpace: { id: 's', name: 'S' },
      mentionRoom: { id: 'roomA', name: 'r' },
      mentionEventId: 'e'
    } as unknown as NotificationItem;

    const store = new NotificationStore(makeAPI());
    // Most-recent-first ordering, as fetch() would produce.
    store.notifications = [dmB, dmA, roomMention];

    expect(store.hasDMRoomNotification('roomA')).toBe(true);
    expect(store.hasDMRoomNotification('roomB')).toBe(false);

    // getDMRoomNotification returns the freshest DM, not the mention,
    // even when the mention's roomId matches.
    expect(store.getDMRoomNotification('roomA')?.id).toBe('dm-b');

    // hasRoomNotification (the non-DM variant) must NOT see DM notifications
    // — that's how the regular sidebar dot stays orthogonal to the DM dot.
    expect(store.hasRoomNotification('roomA')).toBe(true); // matched by mention
    // If we drop the mention, hasRoomNotification goes false even though
    // DMs still target that room id.
    store.notifications = [dmB, dmA];
    expect(store.hasRoomNotification('roomA')).toBe(false);
    expect(store.hasDMRoomNotification('roomA')).toBe(true);
  });

  // Per-instance isolation: each instance has its own NotificationStore, and
  // an error in one must not affect notifications loaded on another.
  it('one store failing does not affect a sibling store', async () => {
    const homeStore = new NotificationStore(makeAPI({ notifications: page([mention('h1')]) }));
    const remoteStore = new NotificationStore(
      makeAPI({ notificationsError: new Error('Cannot query field "threadRootEventId"') })
    );

    await Promise.all([homeStore.fetch(), remoteStore.fetch()]);

    expect(homeStore.notifications).toHaveLength(1);
    expect(homeStore.error).toBeNull();
    expect(remoteStore.notifications).toHaveLength(0);
    expect(remoteStore.error).toContain('Cannot query field');
  });
});
