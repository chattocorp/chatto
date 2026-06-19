import { Timestamp } from '@bufbuild/protobuf';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  DismissAllNotificationsResponse,
  DismissNotificationRequest,
  DismissNotificationResponse,
  ListNotificationsResponse,
  NotificationItemView,
  NotificationKind
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';
import {
  NotificationStore,
  type NotificationItem,
  type NotificationWireClient
} from './notifications.svelte';

function makeStore(client: NotificationWireClient): NotificationStore {
  return new NotificationStore('server_1', () => client);
}

function makeClient(overrides: Partial<NotificationWireClient> = {}): NotificationWireClient {
  return {
    listNotifications: vi.fn().mockResolvedValue(new ListNotificationsResponse()),
    hasNotifications: vi.fn().mockResolvedValue({ hasNotifications: false }),
    dismissNotification: vi
      .fn()
      .mockResolvedValue(new DismissNotificationResponse({ dismissed: true })),
    dismissAllNotifications: vi
      .fn()
      .mockResolvedValue(new DismissAllNotificationsResponse({ dismissedCount: 0 })),
    ...overrides
  };
}

const mention = (id: string): NotificationItem =>
  ({
    __typename: 'MentionNotificationItem',
    id,
    createdAt: new Date('2026-04-29T12:00:00Z').toISOString(),
    actor: {
      __typename: 'User',
      id: 'a',
      login: 'tester',
      displayName: 'Tester',
      avatarUrl: null,
      presenceStatus: 'OFFLINE'
    },
    summary: 'mentioned you',
    mentionRoom: { id: 'r1', name: 'general' },
    mentionEventId: 'evt',
    mentionInThread: null
  }) as unknown as NotificationItem;

function mentionView(id: string): NotificationItemView {
  return new NotificationItemView({
    id,
    createdAt: Timestamp.fromDate(new Date('2026-04-29T12:00:00Z')),
    kind: NotificationKind.MENTION,
    actor: new User({
      id: 'a',
      login: 'tester',
      displayName: 'Tester'
    }),
    summary: 'Tester mentioned you',
    roomId: 'r1',
    roomName: 'general',
    eventId: 'evt'
  });
}

describe('NotificationStore', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(console, 'warn').mockImplementation(() => {});
  });

  it('populates notifications on success', async () => {
    const store = makeStore(
      makeClient({
        listNotifications: vi.fn().mockResolvedValue(
          new ListNotificationsResponse({
            items: [mentionView('n1'), mentionView('n2')],
            serverName: 'Test Server'
          })
        )
      })
    );
    await store.fetch();
    expect(store.notifications).toHaveLength(2);
    expect(store.serverName).toBe('Test Server');
    expect(store.error).toBeNull();
  });

  it('retains existing notifications when the server returns a wire error', async () => {
    const errClient = makeClient({
      listNotifications: vi.fn().mockRejectedValue(new Error('unknown method ListNotifications'))
    });
    const store = makeStore(errClient);
    store.notifications = [mention('original')];

    await store.fetch();

    expect(store.notifications).toHaveLength(1);
    expect(store.notifications[0].id).toBe('original');
    expect(store.error).toContain('unknown method');
    expect(consoleError).toHaveBeenCalled();
  });

  it('does not throw on wire error', async () => {
    const store = makeStore(
      makeClient({
        listNotifications: vi.fn().mockRejectedValue(new Error('something broke'))
      })
    );
    await expect(store.fetch()).resolves.toBeUndefined();
    expect(store.error).toBe('something broke');
  });

  it('does not throw on network/transport error', async () => {
    const client = makeClient({
      listNotifications: vi.fn().mockRejectedValue(new Error('network down'))
    });
    const store = makeStore(client);
    store.notifications = [mention('keepme')];
    await expect(store.fetch()).resolves.toBeUndefined();
    expect(store.notifications).toHaveLength(1);
    expect(store.error).toBe('network down');
  });

  it('checkHasNotifications calls the wire lightweight endpoint', async () => {
    const client = makeClient({
      hasNotifications: vi.fn().mockResolvedValue({ hasNotifications: true })
    });
    const store = makeStore(client);

    await expect(store.checkHasNotifications()).resolves.toBe(true);
    expect(client.hasNotifications).toHaveBeenCalledOnce();
  });

  // Mentions inside a thread must NOT be dismissed when the user enters the
  // parent room — they should only clear when the thread itself is opened
  // (via dismissThreadNotifications), mirroring how thread replies behave.
  it('dismissMentionNotifications skips mentions that are inside a thread', async () => {
    const roomMention = {
      __typename: 'MentionNotificationItem',
      id: 'room-mention',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'mentioned you',
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e1',
      mentionInThread: null
    } as NotificationItem;
    const threadMention = {
      __typename: 'MentionNotificationItem',
      id: 'thread-mention',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'mentioned you',
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e2',
      mentionInThread: 'thread-root'
    } as NotificationItem;

    const dismissedIds: string[] = [];
    const client = makeClient({
      dismissNotification: vi.fn().mockImplementation((request: DismissNotificationRequest) => {
        dismissedIds.push(request.notificationId);
        return Promise.resolve(new DismissNotificationResponse({ dismissed: true }));
      })
    });
    const store = makeStore(client);
    store.notifications = [roomMention, threadMention];

    await store.dismissMentionNotifications('r1');

    expect(dismissedIds).toEqual(['room-mention']);
    expect(store.notifications.map((n) => n.id)).toEqual(['thread-mention']);
  });

  // Opening the thread clears both thread-replies AND thread-mentions in one
  // pass (the code path called from ThreadPane).
  it('dismissThreadNotifications clears thread-scoped mentions too', async () => {
    const threadMention = {
      __typename: 'MentionNotificationItem',
      id: 'thread-mention',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'mentioned you',
      mentionRoom: { id: 'r1', name: 'r' },
      mentionEventId: 'e2',
      mentionInThread: 'thread-root'
    } as NotificationItem;

    const dismissedIds: string[] = [];
    const client = makeClient({
      dismissNotification: vi.fn().mockImplementation((request: DismissNotificationRequest) => {
        dismissedIds.push(request.notificationId);
        return Promise.resolve(new DismissNotificationResponse({ dismissed: true }));
      })
    });
    const store = makeStore(client);
    store.notifications = [threadMention];

    await store.dismissThreadNotifications('thread-root');

    expect(dismissedIds).toEqual(['thread-mention']);
    expect(store.notifications).toHaveLength(0);
  });

  it('restores a notification when optimistic single dismiss fails', async () => {
    const client = makeClient({
      dismissNotification: vi
        .fn()
        .mockResolvedValue(new DismissNotificationResponse({ dismissed: false }))
    });
    const store = makeStore(client);
    store.notifications = [mention('restore-me')];

    await expect(store.dismiss('restore-me')).resolves.toBe(false);
    expect(store.notifications.map((n) => n.id)).toEqual(['restore-me']);
  });

  it('restores notifications when dismissAll transport fails', async () => {
    const client = makeClient({
      dismissAllNotifications: vi.fn().mockRejectedValue(new Error('network down'))
    });
    const store = makeStore(client);
    store.notifications = [mention('keep-one'), mention('keep-two')];

    await expect(store.dismissAll()).resolves.toBe(0);
    expect(store.notifications.map((n) => n.id)).toEqual(['keep-one', 'keep-two']);
  });

  it('returns the wire dismissed count when dismissAll succeeds', async () => {
    const store = makeStore(
      makeClient({
        dismissAllNotifications: vi
          .fn()
          .mockResolvedValue(new DismissAllNotificationsResponse({ dismissedCount: 2 }))
      })
    );
    store.notifications = [mention('one'), mention('two')];

    await expect(store.dismissAll()).resolves.toBe(2);
    expect(store.notifications).toHaveLength(0);
  });

  // The DM list dot uses hasDMRoomNotification per conversation. It must
  // match DM notifications by room, and ignore non-DM notifications even if
  // they happen to share a room id.
  it('hasDMRoomNotification / getDMRoomNotification scope to DM notifications by room', () => {
    const dmA = {
      __typename: 'DMMessageNotificationItem',
      id: 'dm-a',
      createdAt: new Date('2026-04-29T12:00:00Z').toISOString(),
      actor: null,
      summary: 'hi',
      room: { id: 'roomA' }
    } as NotificationItem;
    const dmB = {
      __typename: 'DMMessageNotificationItem',
      id: 'dm-b',
      createdAt: new Date('2026-04-29T13:00:00Z').toISOString(),
      actor: null,
      summary: 'later',
      room: { id: 'roomA' }
    } as NotificationItem;
    const roomMention = {
      __typename: 'MentionNotificationItem',
      id: 'mention-same-id',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'mention',
      mentionRoom: { id: 'roomA', name: 'r' },
      mentionEventId: 'e',
      mentionInThread: null
    } as NotificationItem;

    const store = makeStore(makeClient());
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
    const homeStore = makeStore(
      makeClient({
        listNotifications: vi.fn().mockResolvedValue(
          new ListNotificationsResponse({
            items: [mentionView('h1')]
          })
        )
      })
    );
    const remoteStore = makeStore(
      makeClient({
        listNotifications: vi.fn().mockRejectedValue(new Error('unknown method ListNotifications'))
      })
    );

    await Promise.all([homeStore.fetch(), remoteStore.fetch()]);

    expect(homeStore.notifications).toHaveLength(1);
    expect(homeStore.error).toBeNull();
    expect(remoteStore.notifications).toHaveLength(0);
    expect(remoteStore.error).toContain('unknown method');
  });
});
