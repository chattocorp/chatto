import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  NotificationStore,
  notificationTarget,
  type NotificationItem
} from './notifications.svelte';
import {
  NotificationItemKind,
  NotificationDeliveryIntensity,
  NotificationInboxState,
  NotificationReason,
  NotificationView,
  type NotificationAPI,
  type NotificationGroupPage,
  type NotificationPage
} from '$lib/api-client/notifications';

type MockNotificationAPI = NotificationAPI & {
  listNotificationGroups: ReturnType<typeof vi.fn>;
  listNotificationOccurrences: ReturnType<typeof vi.fn>;
  getNotificationOccurrence: ReturnType<typeof vi.fn>;
  updateNotificationOccurrence: ReturnType<typeof vi.fn>;
  updateNotificationGroup: ReturnType<typeof vi.fn>;
  deleteNotificationGroup: ReturnType<typeof vi.fn>;
  unsubscribeNotificationGroup: ReturnType<typeof vi.fn>;
  getNotificationPolicy: ReturnType<typeof vi.fn>;
  setNotificationPolicyPreference: ReturnType<typeof vi.fn>;
  listNotifications: ReturnType<typeof vi.fn>;
  listRoomNotifications: ReturnType<typeof vi.fn>;
  listRoomNotificationCounts: ReturnType<typeof vi.fn>;
  dismissNotification: ReturnType<typeof vi.fn>;
  dismissAllNotifications: ReturnType<typeof vi.fn>;
};

function page(items: NotificationItem[], totalCount = items.length): NotificationPage {
  return {
    items,
    totalCount,
    hasMore: false
  };
}

function groupPage(source: NotificationPage): NotificationGroupPage {
  const groups = source.items.map((item) => {
    const target = notificationTarget(item);
    const occurrence = {
      id: item.id,
      sourceEventId: item.id,
      createdAt: item.createdAt,
      actor: item.actor ?? null,
      summary: item.summary,
      room: target.roomId ? { id: target.roomId, name: target.roomName ?? '' } : null,
      eventId: target.eventId ?? '',
      threadRootId: target.threadRootId,
      parentEventId: null,
      reasons: [NotificationReason.DIRECT_MENTION],
      reasonMatches: [
        {
          reason: NotificationReason.DIRECT_MENTION,
          intensity: NotificationDeliveryIntensity.ALERT
        }
      ],
      inboxState: NotificationInboxState.UNREAD,
      saved: false
    };
    return {
      id: `group-${item.id}`,
      occurrences: [occurrence],
      openTarget: occurrence,
      unread: true,
      occurrenceCount: 1,
      latestAt: item.createdAt,
      reasons: [NotificationReason.DIRECT_MENTION]
    };
  });
  return {
    groups,
    unreadGroupCount: source.totalCount,
    totalCount: source.totalCount,
    hasMore: source.hasMore
  };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
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
    listNotificationGroups: vi.fn().mockImplementation(async () => {
      if (options.notificationsError) throw options.notificationsError;
      return groupPage(options.notifications ?? page([]));
    }),
    listNotificationOccurrences: vi.fn().mockResolvedValue({
      notifications: [],
      totalCount: 0,
      hasMore: false
    }),
    getNotificationOccurrence: vi.fn<NotificationAPI['getNotificationOccurrence']>(),
    updateNotificationOccurrence: vi.fn().mockResolvedValue(undefined),
    updateNotificationGroup: vi.fn().mockResolvedValue(undefined),
    deleteNotificationGroup: vi.fn().mockResolvedValue(0),
    unsubscribeNotificationGroup: vi.fn().mockResolvedValue(undefined),
    getNotificationPolicy: vi.fn().mockResolvedValue([]),
    setNotificationPolicyPreference: vi.fn().mockResolvedValue([]),
    listNotifications: vi.fn().mockImplementation(async () => {
      if (options.notificationsError) throw options.notificationsError;
      return options.notifications ?? page([]);
    }),
    listRoomNotifications: vi.fn().mockImplementation(async () => {
      if (options.roomNotificationsError) throw options.roomNotificationsError;
      return options.roomNotifications ?? page([]);
    }),
    listRoomNotificationCounts: vi.fn().mockResolvedValue({}),
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

  it('invalidates projection state during reset', () => {
    const store = new NotificationStore(makeAPI());
    store.replaceProjection(page([mention('n1')], 3));

    store.resetProjectionState();

    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.hasLoaded).toBe(true);
    expect(store.loading).toBe(true);
  });

  it('scrubs deleted notification actors and actor-derived summaries', () => {
    const store = new NotificationStore(makeAPI());
    store.replaceGroupProjection(groupPage(page([mention('n1')])));

    store.scrubUser('a');

    expect(store.notifications[0]?.actor).toBeNull();
    expect(store.notifications[0]?.summary).not.toContain('Tester');
    expect(store.groups[0]?.occurrences[0]?.actor).toBeNull();
    expect(store.groups[0]?.occurrences[0]?.summary).not.toContain('Tester');
    expect(store.groups[0]?.openTarget?.actor).toBeNull();
  });

  it('clears room notification payloads at an authorization boundary', () => {
    const other = {
      ...mention('n2'),
      mentionRoom: { id: 'r2', name: 'other' }
    } as NotificationItem;
    const store = new NotificationStore(makeAPI());
    store.replaceGroupProjection(groupPage(page([mention('n1'), other], 2)));

    store.clearRoom('r1');

    expect(store.notifications.map(({ id }) => id)).toEqual(['n2']);
    expect(store.groups.map(({ id }) => id)).toEqual(['group-n2']);
    expect(store.unreadNotificationCount).toBe(1);
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

  it('keeps read Inbox groups without exposing them as unread room indicators', () => {
    const store = new NotificationStore(makeAPI());
    const response = groupPage(page([mention('n1')]));
    response.groups[0]!.unread = false;
    response.groups[0]!.occurrences[0]!.inboxState = NotificationInboxState.READ;
    response.unreadGroupCount = 0;

    store.replaceGroupProjection(response);

    expect(store.groups).toHaveLength(1);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
  });

  it('discards an older full-list response that arrives after a newer response', async () => {
    const older = deferred<NotificationGroupPage>();
    const newer = deferred<NotificationGroupPage>();
    const api = makeAPI();
    api.listNotificationGroups
      .mockReturnValueOnce(older.promise)
      .mockReturnValueOnce(newer.promise);
    const store = new NotificationStore(api);

    const olderFetch = store.fetch();
    const newerFetch = store.fetch();

    newer.resolve(groupPage(page([mention('newer')])));
    await newerFetch;
    older.resolve(groupPage(page([mention('older')])));
    await olderFetch;

    expect(store.notifications.map((notification) => notification.id)).toEqual(['newer']);
  });

  it('does not let an in-flight fetch restore an optimistically dismissed notification', async () => {
    const response = deferred<NotificationGroupPage>();
    const api = makeAPI();
    api.listNotificationGroups.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);
    store.notifications = [mention('dismiss-me')];
    store.unreadNotificationCount = 1;

    const fetch = store.fetch();
    await store.dismiss('dismiss-me');
    response.resolve(groupPage(page([mention('dismiss-me')])));
    await fetch;

    expect(api.listNotificationGroups).toHaveBeenCalledTimes(2);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
  });

  it('fetchRoomNotification returns the newest room-scoped notification and caches it', async () => {
    const roomMention = mention('room-mention');
    const store = new NotificationStore(makeAPI({ notifications: page([roomMention]) }));

    const result = await store.fetchRoomNotification('r1');

    expect(result).toMatchObject({
      ok: true,
      totalCount: 1,
      notification: { id: roomMention.id }
    });
    expect(store.notifications.map((n) => n.id)).toEqual(['room-mention']);
  });

  it('fetchRoomNotification reports an empty room-scoped notification result', async () => {
    const store = new NotificationStore(makeAPI({ notifications: page([], 0) }));

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
    const api = makeAPI({ notifications: page([mention('remote')], 1) });
    const store = new NotificationStore(api);
    store.notifications = [cached];

    const result = await store.resolveRoomNotification('r1');

    expect(result).toEqual({
      ok: true,
      totalCount: null,
      notification: cached
    });
    expect(api.listNotificationGroups).not.toHaveBeenCalled();
  });

  it('returns one selected-view page for automatic UI pagination', async () => {
    const first = groupPage(page([mention('first')], 2));
    first.hasMore = true;
    const api = makeAPI();
    api.listNotificationGroups.mockResolvedValueOnce(first);
    const store = new NotificationStore(api);

    const result = await store.fetchView(NotificationView.DONE, 50);

    expect(result.groups.map(({ id }) => id)).toEqual(['group-first']);
    expect(result.hasMore).toBe(true);
    expect(api.listNotificationGroups).toHaveBeenCalledWith(NotificationView.DONE, 50, 50);
  });

  it('normalizes the room, thread, and event used by push payloads', () => {
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
    expect(notificationTarget(threadReply)).toMatchObject({
      roomId: 'room-2',
      eventId: 'reply-event',
      threadRootId: 'thread-root'
    });

    expect(notificationTarget(roomMessage)).toMatchObject({
      roomId: 'room-news',
      eventId: 'room-event',
      threadRootId: null
    });
  });

  it('routes notifications using notification item kind', () => {
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

    const store = new NotificationStore(makeAPI());
    store.notifications = [threadReply, dm];

    expect(notificationTarget(threadReply)).toMatchObject({
      isDM: false,
      roomId: 'room-kind',
      eventId: 'reply-event',
      threadRootId: 'thread-root'
    });
    expect(store.hasThreadNotification('thread-root')).toBe(true);
    expect(store.hasDMRoomNotification('dm-room')).toBe(true);
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
