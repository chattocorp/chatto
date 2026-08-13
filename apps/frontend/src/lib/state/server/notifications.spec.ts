import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  NotificationStore,
  notificationTarget,
  type NotificationItem
} from './notifications.svelte';
import {
  NotificationItemKind,
  NotificationAttentionLevel,
  NotificationDeliveryIntensity,
  NotificationReason,
  type NotificationAPI,
  type NotificationOccurrenceItem,
  type NotificationOccurrencePage
} from '$lib/api-client/notifications';

type MockNotificationAPI = NotificationAPI & {
  listNotificationOccurrences: ReturnType<typeof vi.fn>;
  markNotificationRead: ReturnType<typeof vi.fn>;
  batchDeleteNotificationOccurrences: ReturnType<typeof vi.fn>;
  deleteAllNotificationOccurrences: ReturnType<typeof vi.fn>;
  getNotificationPolicy: ReturnType<typeof vi.fn>;
  setNotificationPolicyPreference: ReturnType<typeof vi.fn>;
};

type FlatNotificationPage = {
  items: NotificationItem[];
  totalCount: number;
  hasMore: boolean;
};

function page(items: NotificationItem[], totalCount = items.length): FlatNotificationPage {
  return {
    items,
    totalCount,
    hasMore: false
  };
}

function occurrencePage(source: FlatNotificationPage): NotificationOccurrencePage {
  const occurrences = source.items.map((item) => {
    const target = notificationTarget(item);
    const occurrence = {
      id: item.id,
      sourceEventId: item.id,
      createdAt: item.createdAt,
      actor: item.actor ?? null,
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
      attentionLevel: NotificationAttentionLevel.IMPORTANT,
      unread: true,
      reactionEmoji: null,
      threadRootMessageExcerpt: null
    };
    return occurrence;
  });
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
    unreadCount: source.totalCount,
    importantUnreadCount: source.totalCount,
    roomUnreadCounts,
    roomImportantUnreadCounts,
    totalCount: source.totalCount,
    hasMore: source.hasMore
  };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function makeAPI(
  options: {
    notifications?: FlatNotificationPage;
    notificationsError?: Error;
  } = {}
): MockNotificationAPI {
  return {
    listNotificationOccurrences: vi.fn().mockImplementation(async () => {
      if (options.notificationsError) throw options.notificationsError;
      return occurrencePage(options.notifications ?? page([]));
    }),
    markNotificationRead: vi.fn().mockResolvedValue(undefined),
    deleteNotificationOccurrence: vi.fn().mockResolvedValue(false),
    batchDeleteNotificationOccurrences: vi.fn().mockResolvedValue(0),
    deleteAllNotificationOccurrences: vi.fn().mockResolvedValue(0),
    getNotificationPolicy: vi.fn().mockResolvedValue([]),
    setNotificationPolicyPreference: vi.fn().mockResolvedValue([])
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
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')], 3)));

    store.resetProjectionState();

    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.importantUnreadNotificationCount).toBe(0);
    expect(store.hasLoaded).toBe(true);
    expect(store.loading).toBe(true);
  });

  it('scrubs deleted notification actors and actor-derived summaries', () => {
    const store = new NotificationStore(makeAPI());
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    store.scrubUser('a');

    expect(store.notifications[0]?.actor).toBeNull();
    expect(store.notifications[0]?.summary).not.toContain('Tester');
    expect(store.occurrences[0]?.actor).toBeNull();
  });

  it('does not restore a scrubbed actor when a pending read fails', async () => {
    const mutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    store.scrubUser('a');
    mutation.reject(new Error('offline'));

    await expect(marking).resolves.toBe(false);
    expect(store.occurrences[0]?.actor).toBeNull();
    expect(store.notifications[0]?.actor).toBeNull();
  });

  it('clears room notification payloads at an authorization boundary', () => {
    const other = {
      ...mention('n2'),
      mentionRoom: { id: 'r2', name: 'other' }
    } as NotificationItem;
    const store = new NotificationStore(makeAPI());
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), other], 2)));

    store.clearRoom('r1');

    expect(store.notifications.map(({ id }) => id)).toEqual(['n2']);
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n2']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.importantUnreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r2: 1 });
    expect(store.roomImportantUnreadCounts).toEqual({ r2: 1 });
    expect(store.revokedRoomIds.has('r1')).toBe(true);
  });

  it('uses exact room totals when authorization loss affects rows outside the cached page', () => {
    const store = new NotificationStore(makeAPI());
    const response = occurrencePage(page([mention('n1')], 6));
    response.unreadCount = 6;
    response.importantUnreadCount = 5;
    response.roomUnreadCounts = { r1: 5, r2: 1 };
    response.roomImportantUnreadCounts = { r1: 4, r2: 1 };
    store.replaceOccurrenceProjection(response);

    store.clearRoom('r1');

    expect(store.occurrences).toEqual([]);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.importantUnreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r2: 1 });
    expect(store.roomImportantUnreadCounts).toEqual({ r2: 1 });

    const stale = occurrencePage(page([mention('stale')], 5));
    stale.roomUnreadCounts = { r1: 5 };
    stale.roomImportantUnreadCounts = { r1: 5 };
    store.replaceOccurrenceProjection(stale);
    expect(store.occurrences).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);

    store.restoreRoom('r1');
    expect(store.revokedRoomIds.has('r1')).toBe(false);
  });

  it('does not restore a failed read across an authorization boundary', async () => {
    const mutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    store.clearRoom('r1');
    mutation.reject(new Error('offline'));

    await expect(marking).resolves.toBe(false);
    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.roomUnreadCounts).toEqual({});
  });

  it('restores an unrelated failed read when another room loses authorization', async () => {
    const mutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const other = {
      ...mention('n2'),
      mentionRoom: { id: 'r2', name: 'other' }
    } as NotificationItem;
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), other], 2)));

    const marking = store.markRead('n2');
    store.clearRoom('r1');
    mutation.reject(new Error('offline'));

    await expect(marking).resolves.toBe(false);
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n2']);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n2']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r2: 1 });
  });

  it('restores an unrelated failed deletion when another room loses authorization', async () => {
    const mutation = deferred<number>();
    const api = makeAPI();
    api.batchDeleteNotificationOccurrences.mockReturnValueOnce(mutation.promise);
    const other = {
      ...mention('n2'),
      mentionRoom: { id: 'r2', name: 'other' }
    } as NotificationItem;
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), other], 2)));

    const deletion = store.deleteOccurrences(['n2']);
    store.clearRoom('r1');
    mutation.reject(new Error('offline'));

    await expect(deletion).rejects.toThrow('offline');
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n2']);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n2']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r2: 1 });
  });

  it('restores only still-visible rooms when delete-all fails after authorization loss', async () => {
    const mutation = deferred<number>();
    const api = makeAPI();
    api.deleteAllNotificationOccurrences.mockReturnValueOnce(mutation.promise);
    const other = {
      ...mention('n2'),
      mentionRoom: { id: 'r2', name: 'other' }
    } as NotificationItem;
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), other], 2)));

    const deletion = store.deleteAllOccurrences();
    store.clearRoom('r1');
    mutation.reject(new Error('offline'));

    await expect(deletion).rejects.toThrow('offline');
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n2']);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n2']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.importantUnreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r2: 1 });
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

  it('keeps read occurrences without exposing them as unread room indicators', () => {
    const store = new NotificationStore(makeAPI());
    const response = occurrencePage(page([mention('n1')]));
    response.occurrences[0]!.unread = false;
    response.unreadCount = 0;
    response.importantUnreadCount = 0;

    store.replaceOccurrenceProjection(response);

    expect(store.occurrences).toHaveLength(1);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.importantUnreadNotificationCount).toBe(0);
  });

  it('discards an older full-list response that arrives after a newer response', async () => {
    const older = deferred<NotificationOccurrencePage>();
    const newer = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences
      .mockReturnValueOnce(older.promise)
      .mockReturnValueOnce(newer.promise);
    const store = new NotificationStore(api);

    const olderFetch = store.fetch();
    const newerFetch = store.fetch();

    newer.resolve(occurrencePage(page([mention('newer')])));
    await newerFetch;
    older.resolve(occurrencePage(page([mention('older')])));
    await olderFetch;

    expect(store.notifications.map((notification) => notification.id)).toEqual(['newer']);
  });

  it('does not let an in-flight fetch restore an optimistically read notification', async () => {
    const response = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);
    store.notifications = [mention('dismiss-me')];
    store.unreadNotificationCount = 1;

    const fetch = store.fetch();
    await store.markRead('dismiss-me');
    response.resolve(occurrencePage(page([mention('dismiss-me')])));
    await fetch;

    expect(api.listNotificationOccurrences).toHaveBeenCalledTimes(1);
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
    expect(store.occurrences.map((occurrence) => occurrence.id)).toEqual(['room-mention']);
  });

  it('does not cache or return a room lookup completed after authorization loss', async () => {
    const response = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);

    const lookup = store.fetchRoomNotification('r1');
    store.clearRoom('r1');
    response.resolve(occurrencePage(page([mention('stale')])));

    await expect(lookup).resolves.toEqual({ ok: true, totalCount: 0, notification: null });
    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
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
    expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
  });

  it('returns one page for automatic UI pagination', async () => {
    const first = occurrencePage(page([mention('first')], 2));
    first.hasMore = true;
    const api = makeAPI();
    api.listNotificationOccurrences.mockResolvedValueOnce(first);
    const store = new NotificationStore(api);

    const result = await store.fetchPage(50);

    expect(result.occurrences.map(({ id }) => id)).toEqual(['first']);
    expect(result.hasMore).toBe(true);
    expect(api.listNotificationOccurrences).toHaveBeenCalledWith(50, 50);
  });

  it('retries an in-flight page response invalidated by a projection reset', async () => {
    const response = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);

    const fetch = store.fetchPage();
    store.resetProjectionState();
    response.resolve(occurrencePage(page([mention('stale')])));

    await expect(fetch).resolves.toMatchObject({ occurrences: [] });
    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
    expect(store.resetVersion).toBe(1);
  });

  it('retries an in-flight room lookup invalidated by a projection reset', async () => {
    const response = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);

    const fetch = store.fetchRoomNotification('r1');
    store.resetProjectionState();
    response.resolve(occurrencePage(page([mention('stale')])));

    await expect(fetch).resolves.toEqual({ ok: true, totalCount: 0, notification: null });
    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
  });

  it('retries an in-flight page response invalidated by an optimistic read', async () => {
    const response = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const fetch = store.fetchPage(50);
    await store.markRead('n1');
    response.resolve(occurrencePage(page([mention('stale')])));

    await expect(fetch).resolves.toMatchObject({ occurrences: [] });
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n1']);
    expect(store.occurrences[0]?.unread).toBe(false);
  });

  it('retries an in-flight room lookup invalidated by an optimistic delete', async () => {
    const response = deferred<NotificationOccurrencePage>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(response.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const fetch = store.fetchRoomNotification('r1');
    await store.deleteOccurrences(['n1']);
    response.resolve(occurrencePage(page([mention('stale')])));

    await expect(fetch).resolves.toEqual({ ok: true, totalCount: 0, notification: null });
    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
  });

  it('queues page reads started while a mutation is pending', async () => {
    const mutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    const pageFetch = store.fetchPage();

    expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
    expect(store.occurrences[0]?.unread).toBe(false);

    mutation.resolve(store.occurrences[0]!);
    await expect(marking).resolves.toBe(true);
    await expect(pageFetch).resolves.toBeDefined();
    expect(api.listNotificationOccurrences).toHaveBeenCalled();
  });

  it('queues room reads started while a mutation is pending', async () => {
    const mutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    const roomFetch = store.fetchRoomNotification('r1');

    expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
    mutation.resolve(store.occurrences[0]!);
    await expect(marking).resolves.toBe(true);
    await expect(roomFetch).resolves.toMatchObject({ ok: true });
    expect(api.listNotificationOccurrences).toHaveBeenCalled();
  });

  it('sanitizes revoked-room metadata while preserving the raw consumed offset', async () => {
    const api = makeAPI();
    const store = new NotificationStore(api);
    store.clearRoom('r1');
    const visible = {
      ...mention('visible'),
      mentionRoom: { id: 'r2', name: 'other' }
    } as NotificationItem;
    const response = occurrencePage(page([mention('revoked'), visible], 4));
    response.unreadCount = 4;
    response.importantUnreadCount = 3;
    response.roomUnreadCounts = { r1: 3, r2: 1 };
    response.roomImportantUnreadCounts = { r1: 2, r2: 1 };
    api.listNotificationOccurrences.mockResolvedValueOnce(response);

    const result = await store.fetchPage(50);

    expect(result.occurrences.map(({ id }) => id)).toEqual(['visible']);
    expect(result.consumedCount).toBe(2);
    expect(result.unreadCount).toBe(1);
    expect(result.importantUnreadCount).toBe(1);
    expect(result.roomUnreadCounts).toEqual({ r2: 1 });
    expect(result.roomImportantUnreadCounts).toEqual({ r2: 1 });
  });

  it('leaves post-mutation list reconciliation to the realtime replacement', async () => {
    const api = makeAPI();
    const store = new NotificationStore(api);

    await store.deleteOccurrences(['notification-1']);
    await store.markOccurrenceRead('notification-1');

    expect(api.batchDeleteNotificationOccurrences).toHaveBeenCalledWith(['notification-1']);
    expect(api.markNotificationRead).toHaveBeenCalledWith('notification-1');
    expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
  });

  it('deletes occurrences optimistically and restores them when the server rejects the mutation', async () => {
    const mutation = deferred<number>();
    const api = makeAPI();
    api.batchDeleteNotificationOccurrences.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const deletion = store.deleteOccurrences(['n1']);

    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.roomUnreadCounts).toEqual({});

    mutation.reject(new Error('offline'));
    await expect(deletion).rejects.toThrow('offline');
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n1']);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n1']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
  });

  it('restores both distinct optimistic deletions when concurrent requests fail', async () => {
    const firstMutation = deferred<number>();
    const secondMutation = deferred<number>();
    const api = makeAPI();
    api.batchDeleteNotificationOccurrences
      .mockReturnValueOnce(firstMutation.promise)
      .mockReturnValueOnce(secondMutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), mention('n2')], 2)));

    const firstDeletion = store.deleteOccurrences(['n1']);
    const secondDeletion = store.deleteOccurrences(['n2']);
    expect(store.occurrences).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);

    secondMutation.reject(new Error('second offline'));
    await expect(secondDeletion).rejects.toThrow('second offline');
    expect(store.occurrences.map(({ id }) => id)).toEqual(['n2']);
    expect(store.unreadNotificationCount).toBe(1);

    firstMutation.reject(new Error('first offline'));
    await expect(firstDeletion).rejects.toThrow('first offline');
    expect(store.occurrences.map(({ id }) => id).sort()).toEqual(['n1', 'n2']);
    expect(store.notifications.map(({ id }) => id).sort()).toEqual(['n1', 'n2']);
    expect(store.unreadNotificationCount).toBe(2);
  });

  it('restores only the failed one of two concurrent optimistic deletions', async () => {
    const firstMutation = deferred<number>();
    const secondMutation = deferred<number>();
    const api = makeAPI();
    api.batchDeleteNotificationOccurrences
      .mockReturnValueOnce(firstMutation.promise)
      .mockReturnValueOnce(secondMutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), mention('n2')], 2)));

    const firstDeletion = store.deleteOccurrences(['n1']);
    const secondDeletion = store.deleteOccurrences(['n2']);
    firstMutation.resolve(1);
    await firstDeletion;
    secondMutation.reject(new Error('offline'));
    await expect(secondDeletion).rejects.toThrow('offline');

    expect(store.occurrences.map(({ id }) => id)).toEqual(['n2']);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n2']);
    expect(store.unreadNotificationCount).toBe(1);
  });

  it('does not restore a deleted occurrence when an older read fails later', async () => {
    const readMutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(readMutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    const deletion = store.deleteOccurrences(['n1']);
    readMutation.reject(new Error('read offline'));

    await expect(marking).resolves.toBe(false);
    await expect(deletion).resolves.toBeUndefined();
    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
  });

  it('does not roll a failed deletion back over a newer authoritative projection', async () => {
    const mutation = deferred<number>();
    const api = makeAPI();
    api.batchDeleteNotificationOccurrences.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const deletion = store.deleteOccurrences(['n1']);
    store.replaceOccurrenceProjection(occurrencePage(page([], 0)));
    mutation.reject(new Error('offline'));
    await expect(deletion).rejects.toThrow('offline');

    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
  });

  it('optimistically decrements exact unread totals for occurrences outside the cached page', async () => {
    const api = makeAPI();
    const store = new NotificationStore(api);
    store.unreadNotificationCount = 5;
    store.importantUnreadNotificationCount = 4;
    store.roomUnreadCounts = { r1: 5 };
    store.roomImportantUnreadCounts = { r1: 4 };

    await store.deleteOccurrences(['older-1', 'older-2'], {
      unread: 2,
      importantUnread: 2,
      roomId: 'r1'
    });

    expect(store.unreadNotificationCount).toBe(3);
    expect(store.importantUnreadNotificationCount).toBe(2);
    expect(store.roomUnreadCounts).toEqual({ r1: 3 });
    expect(store.roomImportantUnreadCounts).toEqual({ r1: 2 });
  });

  it('updates exact room counts before a read request resolves and restores them on failure', async () => {
    const mutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');

    expect(store.unreadNotificationCount).toBe(0);
    expect(store.importantUnreadNotificationCount).toBe(0);
    expect(store.roomUnreadCounts).toEqual({});
    expect(store.roomImportantUnreadCounts).toEqual({});

    mutation.reject(new Error('offline'));
    await expect(marking).resolves.toBe(false);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.importantUnreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
    expect(store.roomImportantUnreadCounts).toEqual({ r1: 1 });
  });

  it('reads an occurrence found outside the cached page with exact room and attention updates', async () => {
    const api = makeAPI({ notifications: page([mention('older')]) });
    const mutation = deferred<NotificationOccurrenceItem>();
    api.markNotificationRead.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.unreadNotificationCount = 1;
    store.importantUnreadNotificationCount = 1;
    store.roomUnreadCounts = { r1: 1 };
    store.roomImportantUnreadCounts = { r1: 1 };

    await expect(store.fetchRoomNotification('r1')).resolves.toMatchObject({
      notification: { id: 'older' }
    });
    const marking = store.markRead('older');

    expect(store.unreadNotificationCount).toBe(0);
    expect(store.importantUnreadNotificationCount).toBe(0);
    expect(store.roomUnreadCounts).toEqual({});
    expect(store.roomImportantUnreadCounts).toEqual({});

    mutation.resolve(store.occurrences[0]!);
    await expect(marking).resolves.toBe(true);
  });

  it('rolls back only the failed one of two concurrent reads', async () => {
    const firstMutation = deferred<NotificationOccurrenceItem>();
    const secondMutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead
      .mockReturnValueOnce(firstMutation.promise)
      .mockReturnValueOnce(secondMutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), mention('n2')], 2)));

    const first = store.markRead('n1');
    const second = store.markRead('n2');
    secondMutation.resolve(store.occurrences.find(({ id }) => id === 'n2')!);
    await expect(second).resolves.toBe(true);
    firstMutation.reject(new Error('offline'));
    await expect(first).resolves.toBe(false);

    expect(store.occurrences.find(({ id }) => id === 'n1')?.unread).toBe(true);
    expect(store.occurrences.find(({ id }) => id === 'n2')?.unread).toBe(false);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n1']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
  });

  it('keeps a newer read request tracked when an older read finishes', async () => {
    const firstMutation = deferred<NotificationOccurrenceItem>();
    const secondMutation = deferred<NotificationOccurrenceItem>();
    const api = makeAPI();
    api.markNotificationRead
      .mockReturnValueOnce(firstMutation.promise)
      .mockReturnValueOnce(secondMutation.promise);
    const store = new NotificationStore(api);
    const response = occurrencePage(page([mention('n1')]));
    store.replaceOccurrenceProjection(response);

    const first = store.markRead('n1');
    store.replaceOccurrenceProjection(response);
    const second = store.markRead('n1');
    firstMutation.resolve(response.occurrences[0]!);
    await expect(first).resolves.toBe(true);

    const deletion = store.deleteOccurrences(['n1']);
    const settled = vi.fn();
    void deletion.then(settled);
    await Promise.resolve();
    await Promise.resolve();
    expect(settled).not.toHaveBeenCalled();

    secondMutation.resolve(response.occurrences[0]!);
    await expect(second).resolves.toBe(true);
    await expect(deletion).resolves.toBeUndefined();
  });

  it('reconciles authoritatively when a read and following delete both fail', async () => {
    const readMutation = deferred<NotificationOccurrenceItem>();
    const deleteMutation = deferred<number>();
    const api = makeAPI({ notifications: page([mention('n1')]) });
    api.markNotificationRead.mockReturnValueOnce(readMutation.promise);
    api.batchDeleteNotificationOccurrences.mockReturnValueOnce(deleteMutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    const deletion = store.deleteOccurrences(['n1']);
    readMutation.reject(new Error('read offline'));
    await expect(marking).resolves.toBe(false);
    deleteMutation.reject(new Error('delete offline'));
    await expect(deletion).rejects.toThrow('delete offline');

    expect(store.occurrences.find(({ id }) => id === 'n1')?.unread).toBe(true);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n1']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
  });

  it('reconciles authoritatively when a read and following delete-all both fail', async () => {
    const readMutation = deferred<NotificationOccurrenceItem>();
    const deleteMutation = deferred<number>();
    const api = makeAPI({ notifications: page([mention('n1')]) });
    api.markNotificationRead.mockReturnValueOnce(readMutation.promise);
    api.deleteAllNotificationOccurrences.mockReturnValueOnce(deleteMutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const marking = store.markRead('n1');
    const deletion = store.deleteAllOccurrences();
    readMutation.reject(new Error('read offline'));
    await expect(marking).resolves.toBe(false);
    deleteMutation.reject(new Error('delete offline'));
    await expect(deletion).rejects.toThrow('delete offline');

    expect(store.occurrences.find(({ id }) => id === 'n1')?.unread).toBe(true);
    expect(store.notifications.map(({ id }) => id)).toEqual(['n1']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
  });

  it('discards a list request invalidated by deleting occurrences', async () => {
    const list = deferred<NotificationOccurrencePage>();
    const mutation = deferred<number>();
    const api = makeAPI();
    api.listNotificationOccurrences.mockReturnValueOnce(list.promise);
    api.batchDeleteNotificationOccurrences.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1')])));

    const fetch = store.fetch();
    const deletion = store.deleteOccurrences(['n1']);
    list.resolve(occurrencePage(page([mention('n1')])));
    mutation.resolve(1);
    await Promise.all([fetch, deletion]);

    expect(api.listNotificationOccurrences).toHaveBeenCalledTimes(1);
    expect(store.occurrences).toEqual([]);
  });

  it('deletes all occurrences optimistically', async () => {
    const mutation = deferred<number>();
    const api = makeAPI();
    api.deleteAllNotificationOccurrences.mockReturnValueOnce(mutation.promise);
    const store = new NotificationStore(api);
    store.replaceOccurrenceProjection(occurrencePage(page([mention('n1'), mention('n2')], 2)));

    const deletion = store.deleteAllOccurrences();

    expect(store.occurrences).toEqual([]);
    expect(store.notifications).toEqual([]);
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.importantUnreadNotificationCount).toBe(0);

    mutation.resolve(2);
    await deletion;
    expect(api.deleteAllNotificationOccurrences).toHaveBeenCalledTimes(1);
  });

  it('updates ambient and important unread totals independently', async () => {
    const api = makeAPI();
    const store = new NotificationStore(api);
    const response = occurrencePage(page([mention('ambient'), mention('important')], 2));
    response.occurrences[0]!.attentionLevel = NotificationAttentionLevel.AMBIENT;
    response.importantUnreadCount = 1;
    response.roomImportantUnreadCounts = { r1: 1 };
    store.replaceOccurrenceProjection(response);

    await store.deleteOccurrences(['ambient']);
    expect(store.unreadNotificationCount).toBe(1);
    expect(store.importantUnreadNotificationCount).toBe(1);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
    expect(store.roomImportantUnreadCounts).toEqual({ r1: 1 });

    await store.markRead('important');
    expect(store.unreadNotificationCount).toBe(0);
    expect(store.importantUnreadNotificationCount).toBe(0);
    expect(store.roomUnreadCounts).toEqual({});
    expect(store.roomImportantUnreadCounts).toEqual({});
  });

  it('keeps an ambient-only room neutral after reading its last important occurrence', async () => {
    const api = makeAPI();
    const store = new NotificationStore(api);
    const response = occurrencePage(page([mention('ambient'), mention('important')], 2));
    response.occurrences[0]!.attentionLevel = NotificationAttentionLevel.AMBIENT;
    response.importantUnreadCount = 1;
    response.roomImportantUnreadCounts = { r1: 1 };
    store.replaceOccurrenceProjection(response);

    await store.markRead('important');

    expect(store.unreadNotificationCount).toBe(1);
    expect(store.importantUnreadNotificationCount).toBe(0);
    expect(store.roomUnreadCounts).toEqual({ r1: 1 });
    expect(store.roomImportantUnreadCounts).toEqual({});
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
      roomMsgEventId: 'room-event',
      roomMsgThreadRootId: 'thread-root'
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
      threadRootId: 'thread-root'
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
