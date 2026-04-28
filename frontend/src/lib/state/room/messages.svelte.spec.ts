import { describe, it, expect, beforeEach, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import {
  RoomMessagesStore,
  ThreadMessagesStore,
  isRootRoomEvent,
  isThreadEvent
} from './messages.svelte';

// ---------------------------------------------------------------------------
// Test fixtures and helpers
// ---------------------------------------------------------------------------

const ROOM_ID = 'r_main';
const SPACE_ID = 's_main';

function rootMessage(
  id: string,
  overrides: {
    roomId?: string;
    actorId?: string;
    createdAt?: string;
    replyCount?: number;
    lastReplyAt?: string | null;
    threadParticipants?: { id: string }[];
    viewerIsFollowingThread?: boolean;
    echoOfEventId?: string | null;
    inThread?: string | null;
  } = {}
): RoomEventViewFragment {
  return {
    id,
    createdAt: overrides.createdAt ?? '2024-01-01T00:00:00Z',
    actorId: overrides.actorId ?? 'u_other',
    actor: { id: overrides.actorId ?? 'u_other' },
    event: {
      __typename: 'MessagePostedEvent',
      roomId: overrides.roomId ?? ROOM_ID,
      body: 'hello',
      attachments: [],
      linkPreview: null,
      reactions: [],
      updatedAt: null,
      inReplyTo: null,
      inThread: overrides.inThread ?? null,
      echoOfEventId: overrides.echoOfEventId ?? null,
      echoFromThreadRootEventId: null,
      replyCount: overrides.replyCount ?? 0,
      lastReplyAt: overrides.lastReplyAt ?? null,
      threadParticipants: overrides.threadParticipants ?? [],
      viewerIsFollowingThread: overrides.viewerIsFollowingThread ?? false
    }
  } as unknown as RoomEventViewFragment;
}

function threadReply(
  id: string,
  threadRootId: string,
  overrides: { actorId?: string; createdAt?: string; roomId?: string } = {}
): RoomEventViewFragment {
  return rootMessage(id, {
    ...overrides,
    inThread: threadRootId,
    echoOfEventId: null
  });
}

function simpleEvent(
  typename: string,
  fields: Record<string, unknown> = {}
): RoomEventViewFragment {
  return {
    id: `evt_${typename}_${Math.random().toString(36).slice(2, 8)}`,
    createdAt: '2024-01-01T00:00:00Z',
    actorId: 'u_actor',
    actor: { id: 'u_actor' },
    event: { __typename: typename, ...fields }
  } as unknown as RoomEventViewFragment;
}

/**
 * Minimal urql client mock — returns a benign result for every query so that
 * the store's setRoom/loadInitial/refetch paths complete without populating
 * state. Tests that want to assert on query calls inspect `queryMock.mock.calls`.
 */
function makeMockClient() {
  const queryMock = vi.fn(
    (_query: unknown, _variables?: unknown, _options?: unknown) => ({
      toPromise: () => Promise.resolve({ data: null, error: null })
    })
  );
  const client = { query: queryMock } as unknown as Client;
  return { client, queryMock };
}

/**
 * Construct a RoomMessagesStore wired to mock client, with spaceId/roomId
 * already set without going through the async setRoom flow.
 */
function makeRoomStore(currentUserId: string | null = 'u_me') {
  const { client, queryMock } = makeMockClient();
  const store = new RoomMessagesStore(client, () => currentUserId);
  // setRoom triggers an initial fetch (which the mock returns null for —
  // leaving events empty). After flushSync, isInitialLoading is still true
  // because the promise hasn't resolved yet, but spaceId/roomId are set.
  store.setRoom(SPACE_ID, ROOM_ID, 'reset');
  return { store, client, queryMock };
}

function makeThreadStore(currentUserId: string | null = 'u_me') {
  const { client, queryMock } = makeMockClient();
  const store = new ThreadMessagesStore(client, () => currentUserId);
  store.setThread(SPACE_ID, ROOM_ID, 'm_root');
  return { store, client, queryMock };
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

describe('isRootRoomEvent', () => {
  it('keeps a root message', () => {
    expect(isRootRoomEvent(rootMessage('m1'))).toBe(true);
  });

  it('drops thread replies', () => {
    expect(isRootRoomEvent(rootMessage('m1', { inThread: 'root' }))).toBe(false);
  });

  it('keeps echoes (which carry both inThread and echoOfEventId)', () => {
    expect(
      isRootRoomEvent(rootMessage('m1', { inThread: 'root', echoOfEventId: 'src' }))
    ).toBe(true);
  });

  it('keeps system events visible in the room', () => {
    expect(isRootRoomEvent(simpleEvent('UserJoinedRoomEvent', { roomId: ROOM_ID }))).toBe(true);
    expect(isRootRoomEvent(simpleEvent('RoomUpdatedEvent', { roomId: ROOM_ID }))).toBe(true);
    expect(isRootRoomEvent(simpleEvent('MessageDeletedEvent', { roomId: ROOM_ID }))).toBe(true);
  });

  it('drops events the room view does not show', () => {
    expect(isRootRoomEvent(simpleEvent('PresenceChangedEvent', { status: 'ONLINE' }))).toBe(false);
    expect(isRootRoomEvent(simpleEvent('UserTypingEvent', { roomId: ROOM_ID }))).toBe(false);
    expect(isRootRoomEvent(simpleEvent('SpaceMemberDeletedEvent', { spaceId: SPACE_ID }))).toBe(
      false
    );
  });
});

describe('isThreadEvent', () => {
  const threadRootId = 'm_root';

  it('keeps the root message of the thread', () => {
    const evt = rootMessage(threadRootId);
    expect(isThreadEvent(evt, ROOM_ID, threadRootId)).toBe(true);
  });

  it('keeps replies in this thread', () => {
    expect(isThreadEvent(threadReply('r1', threadRootId), ROOM_ID, threadRootId)).toBe(true);
  });

  it('drops replies from a different thread', () => {
    expect(isThreadEvent(threadReply('r1', 'other_root'), ROOM_ID, threadRootId)).toBe(false);
  });

  it('drops messages from a different room', () => {
    const evt = threadReply('r1', threadRootId, { roomId: 'r_other' });
    expect(isThreadEvent(evt, ROOM_ID, threadRootId)).toBe(false);
  });

  it('drops non-message events (system events stay out of threads)', () => {
    const evt = simpleEvent('UserJoinedRoomEvent', { roomId: ROOM_ID });
    expect(isThreadEvent(evt, ROOM_ID, threadRootId)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// RoomMessagesStore.ingestSpaceEvent
// ---------------------------------------------------------------------------

describe('RoomMessagesStore.ingestSpaceEvent', () => {
  let store: RoomMessagesStore;
  let queryMock: ReturnType<typeof makeMockClient>['queryMock'];

  beforeEach(() => {
    ({ store, queryMock } = makeRoomStore());
    queryMock.mockClear();
  });

  it('appends root MessagePostedEvent for the current room', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();
    expect(store.events.map((e) => e.id)).toEqual(['m1']);
  });

  it('ignores MessagePostedEvent for a different room', () => {
    store.ingestSpaceEvent(rootMessage('m1', { roomId: 'r_other' }));
    flushSync();
    expect(store.events).toEqual([]);
  });

  it('dedups repeated events by id', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();
    expect(store.events.map((e) => e.id)).toEqual(['m1']);
  });

  it('does not append a thread reply to the room timeline', () => {
    store.ingestSpaceEvent(rootMessage('root'));
    store.ingestSpaceEvent(threadReply('reply1', 'root'));
    flushSync();
    expect(store.events.map((e) => e.id)).toEqual(['root']);
  });

  it('appends room system events', () => {
    store.ingestSpaceEvent(simpleEvent('UserJoinedRoomEvent', { roomId: ROOM_ID }));
    flushSync();
    expect(store.events).toHaveLength(1);
    expect(store.events[0].event?.__typename).toBe('UserJoinedRoomEvent');
  });

  it('clears state on RoomDeletedEvent for the current room', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();
    expect(store.events).toHaveLength(1);

    store.ingestSpaceEvent(simpleEvent('RoomDeletedEvent', { roomId: ROOM_ID }));
    flushSync();
    expect(store.events).toEqual([]);
  });

  it('leaves state alone on RoomDeletedEvent for a different room', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();

    store.ingestSpaceEvent(simpleEvent('RoomDeletedEvent', { roomId: 'r_other' }));
    flushSync();
    expect(store.events).toHaveLength(1);
  });

  it('triggers per-message refetch on MessageUpdatedEvent', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageUpdatedEvent', { roomId: ROOM_ID, messageEventId: 'm1' })
    );
    expect(queryMock).toHaveBeenCalledTimes(1);
    const args = queryMock.mock.calls[0];
    expect(args[1]).toMatchObject({ eventId: 'm1', roomId: ROOM_ID });
  });

  it('matches echoes by their echoOfEventId on edit/delete', () => {
    // An echo lives at the room level (inThread: null) and points at the
    // original thread reply 't1' via echoOfEventId. When 't1' is edited,
    // the echo should also be refetched.
    store.ingestSpaceEvent(rootMessage('echo1', { echoOfEventId: 't1' }));
    flushSync();
    expect(store.events.map((e) => e.id)).toEqual(['echo1']);
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageUpdatedEvent', { roomId: ROOM_ID, messageEventId: 't1' })
    );
    // We refetch by the echo's own id (that's what's in the array).
    expect(queryMock).toHaveBeenCalledTimes(1);
    expect(queryMock.mock.calls[0][1]).toMatchObject({ eventId: 'echo1' });
  });

  it('clears body and attachments locally on MessageDeletedEvent (no refetch)', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageDeletedEvent', { roomId: ROOM_ID, messageEventId: 'm1' })
    );
    flushSync();

    // No round-trip — applied synchronously.
    expect(queryMock).not.toHaveBeenCalled();
    const m1 = store.events.find((e) => e.id === 'm1');
    if (m1?.event?.__typename !== 'MessagePostedEvent') throw new Error('expected MessagePostedEvent');
    expect(m1.event.body).toBeNull();
    expect(m1.event.attachments).toEqual([]);
  });

  it('preserves reactions and reply metadata on MessageDeletedEvent (placeholder case)', () => {
    const withEngagement = rootMessage('m1', { replyCount: 2, lastReplyAt: '2024-06-01T12:00:00Z' });
    // Inject a reaction directly — the rootMessage helper defaults to none.
    if (withEngagement.event?.__typename === 'MessagePostedEvent') {
      (withEngagement.event as { reactions: unknown[] }).reactions = [
        { emoji: 'thumbsup', count: 1, hasReacted: false, users: [] }
      ];
    }
    store.ingestSpaceEvent(withEngagement);
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageDeletedEvent', { roomId: ROOM_ID, messageEventId: 'm1' })
    );
    flushSync();

    const m1 = store.events.find((e) => e.id === 'm1');
    if (m1?.event?.__typename !== 'MessagePostedEvent') throw new Error('expected MessagePostedEvent');
    expect(m1.event.body).toBeNull();
    expect(m1.event.attachments).toEqual([]);
    // Engagement preserved so MessageEvent.svelte renders the [Message deleted] stub.
    expect(m1.event.reactions).toHaveLength(1);
    expect(m1.event.replyCount).toBe(2);
    expect(m1.event.lastReplyAt).toBe('2024-06-01T12:00:00Z');
  });

  it('clears body on a matching echo when its original is deleted', () => {
    // Echo at the room level pointing at thread reply 't1'.
    store.ingestSpaceEvent(rootMessage('echo1', { echoOfEventId: 't1' }));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageDeletedEvent', { roomId: ROOM_ID, messageEventId: 't1' })
    );
    flushSync();

    expect(queryMock).not.toHaveBeenCalled();
    const echo = store.events.find((e) => e.id === 'echo1');
    if (echo?.event?.__typename !== 'MessagePostedEvent') throw new Error('expected MessagePostedEvent');
    expect(echo.event.body).toBeNull();
  });

  it('leaves unrelated messages alone on MessageDeletedEvent', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    store.ingestSpaceEvent(rootMessage('m2'));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageDeletedEvent', { roomId: ROOM_ID, messageEventId: 'm1' })
    );
    flushSync();

    const m2 = store.events.find((e) => e.id === 'm2');
    if (m2?.event?.__typename !== 'MessagePostedEvent') throw new Error('expected MessagePostedEvent');
    expect(m2.event.body).toBe('hello');
  });

  it('triggers refetch on ReactionAddedEvent and ReactionRemovedEvent', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('ReactionAddedEvent', { roomId: ROOM_ID, messageEventId: 'm1' })
    );
    store.ingestSpaceEvent(
      simpleEvent('ReactionRemovedEvent', { roomId: ROOM_ID, messageEventId: 'm1' })
    );
    expect(queryMock).toHaveBeenCalledTimes(2);
  });

  it('triggers full refetch on SpaceMemberDeletedEvent', () => {
    store.ingestSpaceEvent(rootMessage('m1'));
    store.ingestSpaceEvent(rootMessage('m2'));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(simpleEvent('SpaceMemberDeletedEvent', { spaceId: SPACE_ID }));
    // refetchAll iterates over rootEvents and calls refetchOne for each.
    // The iteration is async-sequential so we just assert the count after a tick.
    return Promise.resolve().then(() => {
      expect(queryMock).toHaveBeenCalledTimes(1); // first iteration in flight
    });
  });
});

// ---------------------------------------------------------------------------
// applyThreadReplyToRoot — auto-follow heuristics + metadata fan-out
// ---------------------------------------------------------------------------

describe('RoomMessagesStore: thread reply metadata fan-out', () => {
  function setupRoot(opts: {
    rootAuthorId: string;
    replyCount?: number;
    viewerIsFollowingThread?: boolean;
    threadParticipants?: { id: string }[];
    currentUserId?: string | null;
  }) {
    const { store, queryMock } = makeRoomStore(opts.currentUserId ?? 'u_me');
    store.ingestSpaceEvent(
      rootMessage('root', {
        actorId: opts.rootAuthorId,
        replyCount: opts.replyCount ?? 0,
        viewerIsFollowingThread: opts.viewerIsFollowingThread ?? false,
        threadParticipants: opts.threadParticipants ?? []
      })
    );
    flushSync();
    queryMock.mockClear();
    return store;
  }

  function getRoot(store: RoomMessagesStore) {
    const root = store.events.find((e) => e.id === 'root');
    if (!root || root.event?.__typename !== 'MessagePostedEvent') {
      throw new Error('root not found');
    }
    return root.event;
  }

  it('increments replyCount and updates lastReplyAt on a thread reply', () => {
    const store = setupRoot({ rootAuthorId: 'u_other' });

    store.ingestSpaceEvent(
      threadReply('r1', 'root', { actorId: 'u_other', createdAt: '2024-06-01T12:00:00Z' })
    );
    flushSync();

    const root = getRoot(store);
    expect(root.replyCount).toBe(1);
    expect(root.lastReplyAt).toBe('2024-06-01T12:00:00Z');
  });

  it('appends a new participant to threadParticipants', () => {
    const store = setupRoot({ rootAuthorId: 'u_a' });

    store.ingestSpaceEvent(threadReply('r1', 'root', { actorId: 'u_b' }));
    flushSync();

    const root = getRoot(store);
    expect(root.threadParticipants.map((p: unknown) => (p as { id: string }).id)).toEqual(['u_b']);
  });

  it('does not duplicate an existing participant', () => {
    const store = setupRoot({
      rootAuthorId: 'u_a',
      threadParticipants: [{ id: 'u_b' }]
    });

    store.ingestSpaceEvent(threadReply('r1', 'root', { actorId: 'u_b' }));
    flushSync();

    const root = getRoot(store);
    expect(root.threadParticipants).toHaveLength(1);
  });

  it('auto-follows when the current user posts the reply', () => {
    const store = setupRoot({ rootAuthorId: 'u_other', currentUserId: 'u_me' });

    store.ingestSpaceEvent(threadReply('r1', 'root', { actorId: 'u_me' }));
    flushSync();

    expect(getRoot(store).viewerIsFollowingThread).toBe(true);
  });

  it('auto-follows the root author on the FIRST reply only', () => {
    const store = setupRoot({
      rootAuthorId: 'u_me',
      currentUserId: 'u_me',
      replyCount: 0
    });

    store.ingestSpaceEvent(threadReply('r1', 'root', { actorId: 'u_other' }));
    flushSync();

    expect(getRoot(store).viewerIsFollowingThread).toBe(true);
  });

  it('does NOT auto-follow the root author on subsequent replies', () => {
    const store = setupRoot({
      rootAuthorId: 'u_me',
      currentUserId: 'u_me',
      replyCount: 3, // already replies — not the first
      viewerIsFollowingThread: false
    });

    store.ingestSpaceEvent(threadReply('r1', 'root', { actorId: 'u_other' }));
    flushSync();

    expect(getRoot(store).viewerIsFollowingThread).toBe(false);
  });

  it('preserves an existing viewerIsFollowingThread = true', () => {
    const store = setupRoot({
      rootAuthorId: 'u_other',
      currentUserId: 'u_me',
      viewerIsFollowingThread: true
    });

    store.ingestSpaceEvent(threadReply('r1', 'root', { actorId: 'u_other' }));
    flushSync();

    expect(getRoot(store).viewerIsFollowingThread).toBe(true);
  });

  it('does not update if the root message is not in the timeline', () => {
    const { store, queryMock } = makeRoomStore();
    queryMock.mockClear();

    // No root message — reply arrives in a vacuum.
    store.ingestSpaceEvent(threadReply('r1', 'missing_root'));
    flushSync();

    expect(store.events).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// ThreadMessagesStore
// ---------------------------------------------------------------------------

describe('ThreadMessagesStore.ingestSpaceEvent', () => {
  it('appends a reply targeting the current thread', () => {
    const { store } = makeThreadStore();
    store.ingestSpaceEvent(threadReply('r1', 'm_root'));
    flushSync();
    expect(store.events.map((e) => e.id)).toEqual(['r1']);
  });

  it('ignores replies for a different thread', () => {
    const { store } = makeThreadStore();
    store.ingestSpaceEvent(threadReply('r1', 'other_root'));
    flushSync();
    expect(store.events).toEqual([]);
  });

  it('ignores room system events (threads only show messages)', () => {
    const { store } = makeThreadStore();
    store.ingestSpaceEvent(simpleEvent('UserJoinedRoomEvent', { roomId: ROOM_ID }));
    flushSync();
    expect(store.events).toEqual([]);
  });

  it('triggers refetch on MessageUpdatedEvent for events in the buffer', () => {
    const { store, queryMock } = makeThreadStore();
    store.ingestSpaceEvent(threadReply('r1', 'm_root'));
    flushSync();
    queryMock.mockClear();

    store.ingestSpaceEvent(
      simpleEvent('MessageUpdatedEvent', { roomId: ROOM_ID, messageEventId: 'r1' })
    );
    expect(queryMock).toHaveBeenCalledTimes(1);
  });
});
