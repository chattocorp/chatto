import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Client } from '@urql/svelte';
import { NotificationStore, type NotificationItem } from './notifications.svelte';

/**
 * Build a minimal mock urql Client whose `query` resolves with a controllable
 * `{ data, error }` pair. This is intentionally simpler than
 * `test-utils/mockGraphqlClient` — we want full control over the query result
 * so we can simulate schema-mismatch and network-error cases.
 */
function makeClient(result: { data?: unknown; error?: { message: string } | null }): Client {
  return {
    query: vi.fn().mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: result.data ?? null,
        error: result.error ?? null
      })
    }),
    mutation: vi.fn(),
    subscription: vi.fn()
  } as unknown as Client;
}

const mention = (id: string): NotificationItem =>
  ({
    __typename: 'MentionNotificationItem',
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
      makeClient({ data: { notifications: [mention('n1'), mention('n2')] } })
    );
    await store.fetch();
    expect(store.notifications).toHaveLength(2);
    expect(store.error).toBeNull();
  });

  // The motivating bug: a remote instance running an older backend rejects
  // the entire query when the frontend asks for a field it doesn't have.
  // Before the resilience contract this caused fetch() to throw and the
  // remote's NotificationStore to lock up — symptom was the orange DM dot
  // disappearing for cross-instance DMs. The contract is: a server error
  // records the error message, but does NOT replace existing notifications.
  it('retains existing notifications when the server returns a GraphQL error', async () => {
    const errClient = makeClient({
      error: { message: 'Cannot query field "inThread" on type "MentionNotificationItem".' }
    });
    const store = new NotificationStore(errClient);
    // Pre-populate as if a previous fetch had succeeded.
    store.notifications = [mention('original')];

    await store.fetch();

    expect(store.notifications).toHaveLength(1);
    expect(store.notifications[0].id).toBe('original');
    expect(store.error).toContain('Cannot query field');
    expect(consoleError).toHaveBeenCalled();
  });

  it('does not throw on GraphQL error', async () => {
    const store = new NotificationStore(
      makeClient({ error: { message: 'something broke' } })
    );
    await expect(store.fetch()).resolves.toBeUndefined();
    expect(store.error).toBe('something broke');
  });

  it('does not throw on network/transport error', async () => {
    const client = {
      query: vi.fn().mockReturnValue({
        toPromise: vi.fn().mockRejectedValue(new Error('network down'))
      }),
      mutation: vi.fn(),
      subscription: vi.fn()
    } as unknown as Client;
    const store = new NotificationStore(client);
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
      __typename: 'MentionNotificationItem',
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
      __typename: 'MentionNotificationItem',
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
    const client = {
      query: vi.fn(),
      mutation: vi.fn().mockImplementation((_doc, vars: { input: { notificationId: string } }) => ({
        toPromise: vi.fn().mockImplementation(() => {
          dismissedIds.push(vars.input.notificationId);
          return Promise.resolve({ data: { dismissNotification: true }, error: null });
        })
      })),
      subscription: vi.fn()
    } as unknown as Client;
    const store = new NotificationStore(client);
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
    const client = {
      query: vi.fn(),
      mutation: vi.fn().mockImplementation((_doc, vars: { input: { notificationId: string } }) => ({
        toPromise: vi.fn().mockImplementation(() => {
          dismissedIds.push(vars.input.notificationId);
          return Promise.resolve({ data: { dismissNotification: true }, error: null });
        })
      })),
      subscription: vi.fn()
    } as unknown as Client;
    const store = new NotificationStore(client);
    store.notifications = [threadMention];

    await store.dismissThreadNotifications('thread-root');

    expect(dismissedIds).toEqual(['thread-mention']);
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
      __typename: 'DMMessageNotificationItem',
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
      __typename: 'MentionNotificationItem',
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

    const store = new NotificationStore(makeClient({}));
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
    const homeStore = new NotificationStore(
      makeClient({ data: { notifications: [mention('h1')] } })
    );
    const remoteStore = new NotificationStore(
      makeClient({ error: { message: 'Cannot query field "inThread"' } })
    );

    await Promise.all([homeStore.fetch(), remoteStore.fetch()]);

    expect(homeStore.notifications).toHaveLength(1);
    expect(homeStore.error).toBeNull();
    expect(remoteStore.notifications).toHaveLength(0);
    expect(remoteStore.error).toContain('Cannot query field');
  });
});
