import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { RoomTimelineAPI } from '$lib/api-client/roomTimeline';
import { Timestamp } from '@bufbuild/protobuf';
import {
  RoomMessagePosted,
  RoomTimelineEvent,
  RoomTimelinePage
} from '@chatto/api-types/api/v1/room_timeline_pb';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { EventConnectionPage } from './messages/helpers';
import { MessagesStore } from './messages.svelte';
import { JumpToMessageState } from './composerContext.svelte';

class FakeQueryClient {
  reconnectCount = 0;
  queryMock: ReturnType<typeof vi.fn>;

  constructor(queryData: unknown = null) {
    const queryDataQueue = Array.isArray(queryData) ? [...queryData] : null;
    this.queryMock = vi.fn(() => ({
      toPromise: async () => {
        const data =
          queryDataQueue === null
            ? queryData
            : queryDataQueue.length > 1
              ? queryDataQueue.shift()
              : queryDataQueue[0];
        const resolvedData = await Promise.resolve(data);
        if (isLegacyAsyncResult(resolvedData)) return resolvedData;
        return { data: resolvedData, error: null };
      }
    }));
  }
}

function isLegacyAsyncResult(value: unknown): value is { data: unknown; error: unknown } {
  return typeof value === 'object' && value !== null && ('data' in value || 'error' in value);
}

async function settle() {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve();
  }
  flushSync();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function threadMessageEvent(id: string, threadRootEventId: string | null = null) {
  const offsetSeconds = Number(id.replace(/\D/g, '')) || 0;
  return {
    id,
    createdAt: new Date(Date.UTC(2026, 4, 27, 0, 0, offsetSeconds)).toISOString(),
    actorId: 'u1',
    actor: null,
    event: {
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      body: id,
      attachments: [],
      linkPreview: null,
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId,
      echoOfEventId: null,
      echoFromThreadRootEventId: null,
      channelEchoEventId: null,
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null,
      reactions: []
    }
  };
}

function messageRetraction(messageEventId: string, createdAt = '2026-05-27T00:00:02Z') {
  return {
    id: `retract-${messageEventId}`,
    createdAt,
    actorId: 'u1',
    actor: null,
    event: {
      kind: RoomEventKind.MessageRetracted,
      roomId: 'room-1',
      messageEventId,
      retractedReason: null
    }
  };
}

function messageWithReaction(id: string, emoji: string) {
  const event = threadMessageEvent(id);
  return {
    ...event,
    event: {
      ...event.event,
      reactions: [
        {
          emoji,
          count: 1,
          hasReacted: false,
          users: []
        }
      ]
    }
  };
}

function messageWithReactionState(
  id: string,
  reaction: {
    emoji: string;
    count: number;
    hasReacted: boolean;
    users?: { id: string; displayName: string }[];
  }
) {
  const event = threadMessageEvent(id);
  return {
    ...event,
    event: {
      ...event.event,
      reactions: [
        {
          users: [],
          ...reaction
        }
      ]
    }
  };
}

function reactionsOf(event: { event?: { kind?: string; reactions?: unknown[] } | null }) {
  if (event.event?.kind !== RoomEventKind.MessagePosted) throw new Error('expected message event');
  return event.event.reactions ?? [];
}

function threadMessageWithReaction(id: string, threadRootEventId: string, emoji: string) {
  const event = threadMessageEvent(id, threadRootEventId);
  return {
    ...event,
    event: {
      ...event.event,
      reactions: [
        {
          emoji,
          count: 1,
          hasReacted: false,
          users: []
        }
      ]
    }
  };
}

function callEvent(
  kind:
    | typeof RoomEventKind.CallStarted
    | typeof RoomEventKind.CallEnded
    | typeof RoomEventKind.CallParticipantJoined
    | typeof RoomEventKind.CallParticipantLeft,
  id: string,
  roomId = 'room-1'
) {
  return {
    id,
    createdAt: '2026-05-27T00:00:01Z',
    actorId: 'u1',
    actor: null,
    event: {
      kind,
      roomId,
      callId: 'call-1'
    }
  };
}

function roomSystemEvent(
  id: string,
  kind:
    | typeof RoomEventKind.UserJoinedRoom
    | typeof RoomEventKind.UserLeftRoom
    | typeof RoomEventKind.RoomUpdated
    | typeof RoomEventKind.RoomArchived
    | typeof RoomEventKind.RoomUnarchived,
  actor: unknown = null
) {
  return {
    id,
    createdAt: '2026-05-27T00:00:01Z',
    actorId: 'u1',
    actor,
    event: {
      kind,
      roomId: 'room-1'
    }
  };
}

function roomEventsResult({
  events,
  startCursor,
  endCursor,
  hasOlder,
  hasNewer
}: {
  events: unknown[];
  startCursor: string | null;
  endCursor: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
}) {
  return {
    room: {
      events: {
        events,
        startCursor,
        endCursor,
        hasOlder,
        hasNewer
      }
    }
  };
}

function fakeTimelineAPI(overrides: Partial<RoomTimelineAPI> = {}): RoomTimelineAPI {
  return {
    getRoomEvents: vi.fn(async () => ({
      events: [],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    })),
    getRoomEventsAround: vi.fn(async () => ({
      events: [],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    })),
    getMessage: vi.fn(async () => null),
    getThreadEvents: vi.fn(async () => ({
      events: [],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    })),
    getThreadEventsAround: vi.fn(async () => ({
      events: [],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    })),
    ...overrides
  };
}

function emptyPage(): EventConnectionPage {
  return {
    events: [],
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false
  };
}

function pageFromEvent(event: unknown): EventConnectionPage {
  return {
    events: event ? [event as never] : [],
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false
  };
}

function projectedMessagePage(id: string): RoomTimelinePage {
  return new RoomTimelinePage({
    events: [
      new RoomTimelineEvent({
        id,
        actorId: 'u1',
        createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
        event: {
          case: 'messagePosted',
          value: new RoomMessagePosted({
            message: new Message({
              id,
              roomId: 'room-1',
              actorId: 'u1',
              createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
              body: id
            })
          })
        }
      })
    ]
  });
}

async function resolveFakeResult(
  fake: FakeQueryClient,
  label: string,
  variables: unknown,
  options?: unknown
) {
  const query = fake.queryMock as unknown as (
    label: string,
    variables: unknown,
    options?: unknown
  ) => {
    toPromise(): Promise<{ data: unknown; error: unknown }>;
  };
  const result = await query(label, variables, options).toPromise();
  if (result.error) throw result.error;
  return result.data as {
    room?: {
      events?: EventConnectionPage;
      eventsAround?: EventConnectionPage;
      event?: unknown;
    };
  };
}

function timelineFromFixtures(fake: FakeQueryClient): RoomTimelineAPI {
  return {
    async getRoomEvents(input) {
      const label = input.before
        ? 'timeline:before'
        : input.after
          ? 'timeline:after'
          : 'timeline:latest';
      const data = await resolveFakeResult(fake, label, input, { requestPolicy: 'network-only' });
      return data.room?.events ?? emptyPage();
    },
    async getRoomEventsAround(input) {
      const data = await resolveFakeResult(fake, 'timeline:around', input, {
        requestPolicy: 'network-only'
      });
      return data.room?.eventsAround ?? pageFromEvent(data.room?.event);
    },
    async getMessage(input) {
      const data = await resolveFakeResult(fake, 'timeline:message-link', input, {
        requestPolicy: 'network-only'
      });
      return (data.room?.event as never) ?? null;
    },
    async getThreadEvents(input) {
      const data = await resolveFakeResult(fake, 'timeline:thread-latest', input);
      return data.room?.events ?? emptyPage();
    },
    async getThreadEventsAround(input) {
      const data = await resolveFakeResult(fake, 'timeline:thread-around', input, {
        requestPolicy: 'network-only'
      });
      return data.room?.eventsAround ?? pageFromEvent(data.room?.event);
    }
  };
}

describe('MessagesStore — room lifecycle ownership', () => {
  it('scrubs deleted-user actors, thread participants, and reaction previews', () => {
    const fake = new FakeQueryClient();
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );
    const message = threadMessageEvent('m1');
    store.events = [
      {
        ...message,
        actorId: 'deleted-user',
        actor: { id: 'deleted-user', displayName: 'Alice' },
        event: {
          ...message.event,
          threadParticipants: [
            { id: 'deleted-user', displayName: 'Alice' },
            { id: 'remaining-user', displayName: 'Bob' }
          ],
          reactions: [
            {
              emoji: 'heart',
              count: 2,
              hasReacted: false,
              users: [
                { id: 'deleted-user', displayName: 'Alice' },
                { id: 'remaining-user', displayName: 'Bob' }
              ]
            }
          ]
        }
      } as never
    ];

    store.scrubUserReferences('deleted-user');

    expect(store.events[0]).toMatchObject({
      actorId: 'deleted-user',
      actor: null,
      event: {
        threadParticipants: [{ id: 'remaining-user', displayName: 'Bob' }],
        reactions: [
          {
            count: 2,
            users: [{ id: 'remaining-user', displayName: 'Bob' }]
          }
        ]
      }
    });
    store.dispose();
  });

  it('scrubs deleted-user render data from off-window preview events', async () => {
    const preview = threadMessageEvent('preview');
    const deletedUserPreview = {
      ...preview,
      actorId: 'deleted-user',
      actor: { id: 'deleted-user', displayName: 'Alice' },
      event: {
        ...preview.event,
        threadParticipants: [
          { id: 'deleted-user', displayName: 'Alice' },
          { id: 'remaining-user', displayName: 'Bob' }
        ],
        reactions: [
          {
            emoji: 'heart',
            count: 2,
            hasReacted: false,
            users: [
              { id: 'deleted-user', displayName: 'Alice' },
              { id: 'remaining-user', displayName: 'Bob' }
            ]
          }
        ]
      }
    };
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async () => pageFromEvent(deletedUserPreview))
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();
    await store.ensureEvent('preview');

    store.scrubUserReferences('deleted-user');

    expect(store.getEventById('preview')).toMatchObject({
      actorId: 'deleted-user',
      actor: null,
      event: {
        threadParticipants: [{ id: 'remaining-user', displayName: 'Bob' }],
        reactions: [
          {
            count: 2,
            users: [{ id: 'remaining-user', displayName: 'Bob' }]
          }
        ]
      }
    });
    store.dispose();
  });

  it('rejects an in-flight preview captured before deleted-user scrubbing', async () => {
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const pendingRead = deferred<AroundPage>();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(() => pendingRead.promise)
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();

    const loading = store.ensureEvent('preview');
    store.scrubUserReferences('deleted-user');
    pendingRead.resolve(
      pageFromEvent({
        ...threadMessageEvent('preview'),
        actorId: 'deleted-user',
        actor: { id: 'deleted-user', displayName: 'Alice' }
      })
    );
    await loading;

    expect(store.getEventById('preview')).toBeUndefined();
    store.dispose();
  });

  it('does not let optimistic reaction rollbacks restore scrubbed users', async () => {
    const deletedReactionUser = { id: 'deleted-user', displayName: 'Alice' };
    const mainEvent = messageWithReactionState('main', {
      emoji: 'heart',
      count: 1,
      hasReacted: false,
      users: [deletedReactionUser]
    });
    const previewEvent = messageWithReactionState('preview', {
      emoji: 'heart',
      count: 1,
      hasReacted: false,
      users: [deletedReactionUser]
    });
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async () => pageFromEvent(previewEvent))
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();
    store.events = [mainEvent as never];
    await store.ensureEvent('preview');

    const mainOptimistic = store.beginOptimisticReaction({
      messageEventId: 'main',
      emoji: 'heart',
      action: 'add'
    });
    const previewOptimistic = store.beginOptimisticReaction({
      messageEventId: 'preview',
      emoji: 'heart',
      action: 'add'
    });
    store.scrubUserReferences('deleted-user');

    mainOptimistic.rollback();
    previewOptimistic.rollback();

    expect(reactionsOf(store.getEventById('main')!)).toMatchObject([{ users: [] }]);
    expect(reactionsOf(store.getEventById('preview')!)).toMatchObject([{ users: [] }]);
    store.dispose();
  });

  it('scrubs deleted users from a primary timeline response that resolves later', async () => {
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const pendingRead = deferred<AroundPage>();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(() => pendingRead.promise)
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();
    store.events = [threadMessageEvent('main') as never];
    const refreshing = store.refreshCurrentWindow('main');

    store.scrubUserReferences('deleted-user');
    const stale = threadMessageEvent('main');
    pendingRead.resolve(
      pageFromEvent({
        ...stale,
        actorId: 'deleted-user',
        actor: { id: 'deleted-user', displayName: 'Alice' },
        event: {
          ...stale.event,
          threadParticipants: [{ id: 'deleted-user', displayName: 'Alice' }],
          reactions: [
            {
              emoji: 'heart',
              count: 1,
              hasReacted: false,
              users: [{ id: 'deleted-user', displayName: 'Alice' }]
            }
          ]
        }
      })
    );
    await refreshing;

    expect(store.getEventById('main')).toMatchObject({
      actor: null,
      event: { threadParticipants: [], reactions: [{ users: [] }] }
    });
    store.dispose();
  });

  it('reports a successful jump when the target is already loaded', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI();
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();
    store.events = [threadMessageEvent('m1') as never];

    const jumpState = new JumpToMessageState();
    const jumped = await store.jumpToMessage('m1', jumpState);

    expect(jumped).toBe(true);
    expect(jumpState.scrollToEventId).toBe('m1');
    expect(timeline.getRoomEventsAround).not.toHaveBeenCalled();
    store.dispose();
  });

  it('reports a successful jump after loading a room window around the target', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async () => ({
        events: [
          threadMessageEvent('m1') as never,
          threadMessageEvent('m2') as never,
          threadMessageEvent('m3') as never
        ],
        startCursor: 'tl:start',
        endCursor: 'tl:end',
        hasOlder: true,
        hasNewer: true
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const jumpState = new JumpToMessageState();
    const jumped = await store.jumpToMessage('m2', jumpState);

    expect(jumped).toBe(true);
    expect(timeline.getRoomEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      eventId: 'm2',
      limit: 50
    });
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2', 'm3']);
    expect(jumpState.isJumpedMode).toBe(true);
    expect(jumpState.hasReachedEnd).toBe(false);
    expect(jumpState.hasOlderMessages).toBe(true);
    expect(jumpState.scrollToEventId).toBe('m2');
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('keeps an in-flight message jump when lazy latest-page hydration arrives', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    let resolveAround: ((value: AroundPage) => void) | undefined;
    const aroundPage = new Promise<AroundPage>((resolve) => {
      resolveAround = resolve;
    });
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => aroundPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.awaitRoomProjection('room-1');

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('historical-target', jumpState);
    store.replaceRoomProjectionPage('room-1', new RoomTimelinePage());
    expect(store.isInitialLoading).toBe(true);
    resolveAround?.({
      events: [threadMessageEvent('historical-target') as never],
      startCursor: 'tl:historical',
      endCursor: 'tl:historical',
      hasOlder: true,
      hasNewer: true
    });

    await expect(jumping).resolves.toBe(true);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['historical-target']);
    expect(jumpState.scrollToEventId).toBe('historical-target');
    expect(jumpState.isJumpedMode).toBe(true);
    store.dispose();
  });

  it('cancels an in-flight historical jump at a room lifecycle boundary', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    let resolveAround: ((value: AroundPage) => void) | undefined;
    const aroundPage = new Promise<AroundPage>((resolve) => {
      resolveAround = resolve;
    });
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => aroundPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('latest-message'));

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('historical-target', jumpState);
    store.restoreRoomProjectionPage('room-1', projectedMessagePage('latest-message'));
    resolveAround?.({
      events: [threadMessageEvent('historical-target') as never],
      startCursor: 'tl:historical',
      endCursor: 'tl:historical',
      hasOlder: true,
      hasNewer: true
    });

    await expect(jumping).resolves.toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['latest-message']);
    expect(jumpState.isJumpedMode).toBe(false);
    store.dispose();
  });

  it('scrubs an open thread, rejects its in-flight plaintext, and reloads only after access returns', async () => {
    type ThreadPage = Awaited<ReturnType<RoomTimelineAPI['getThreadEvents']>>;
    let resolveRevokedRead: ((value: ThreadPage) => void) | undefined;
    const revokedRead = new Promise<ThreadPage>((resolve) => {
      resolveRevokedRead = resolve;
    });
    const getThreadEvents = vi
      .fn<RoomTimelineAPI['getThreadEvents']>()
      .mockImplementationOnce(() => revokedRead)
      .mockResolvedValueOnce({
        events: [threadMessageEvent('restored-thread-message', 'thread-root') as never],
        startCursor: 'thread:restored',
        endCursor: 'thread:restored',
        hasOlder: false,
        hasNewer: false
      });
    const timeline = fakeTimelineAPI({ getThreadEvents });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );

    store.setThread('room-1', 'thread-root');
    store.events = [threadMessageEvent('cached-thread-plaintext', 'thread-root') as never];
    store.clearForAccessRevocation();
    expect(store.events).toEqual([]);
    expect(store.isInitialLoading).toBe(false);
    expect(getThreadEvents).toHaveBeenCalledTimes(1);

    resolveRevokedRead?.({
      events: [threadMessageEvent('late-revoked-plaintext', 'thread-root') as never],
      startCursor: 'thread:revoked',
      endCursor: 'thread:revoked',
      hasOlder: false,
      hasNewer: false
    });
    await settle();
    expect(store.events).toEqual([]);
    expect(getThreadEvents).toHaveBeenCalledTimes(1);

    store.restoreAfterAccessGrant();
    await settle();
    expect(getThreadEvents).toHaveBeenCalledTimes(2);
    expect(store.events.map(({ id }) => id)).toEqual(['restored-thread-message']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('keeps the revocation fence closed across late projection and command rows', () => {
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      fakeTimelineAPI()
    );
    store.awaitRoomProjection('room-1');
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('secret'));
    store.clearForAccessRevocation();

    store.replaceRoomProjectionPage('room-1', projectedMessagePage('late-projection'));
    store.ingestEvent(threadMessageEvent('late-command') as never);
    expect(store.events).toEqual([]);
    expect(store.ensureEvent('late-preview')).toBeUndefined();

    store.restoreAfterAccessGrant();
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('authorised-again'));
    expect(store.rootEvents.map((event) => event.id)).toEqual(['authorised-again']);
    store.dispose();
  });

  it('rejects delayed room pagination after a projection reset', async () => {
    type RoomPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEvents']>>;
    const older = deferred<RoomPage>();
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      fakeTimelineAPI({ getRoomEvents: vi.fn(() => older.promise) })
    );
    const current = projectedMessagePage('current');
    current.startCursor = 'before-current';
    current.hasOlder = true;
    store.replaceRoomProjectionPage('room-1', current);

    const loading = store.loadMore();
    store.resetProjectionState();
    older.resolve({
      events: [threadMessageEvent('stale-older') as never],
      startCursor: 'before-stale',
      endCursor: 'stale',
      hasOlder: false,
      hasNewer: true
    });
    await loading;

    expect(store.events).toEqual([]);
    expect(store.isLoadingMore).toBe(false);
    store.dispose();
  });

  it('rejects delayed thread pagination after access revocation', async () => {
    type ThreadPage = Awaited<ReturnType<RoomTimelineAPI['getThreadEvents']>>;
    const older = deferred<ThreadPage>();
    const getThreadEvents = vi
      .fn<RoomTimelineAPI['getThreadEvents']>()
      .mockResolvedValueOnce({
        events: [threadMessageEvent('thread-root') as never],
        startCursor: 'before-thread',
        endCursor: 'thread',
        hasOlder: true,
        hasNewer: false
      })
      .mockImplementationOnce(() => older.promise);
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      fakeTimelineAPI({ getThreadEvents })
    );
    store.setThread('room-1', 'thread-root');
    await settle();

    const loading = store.loadMore();
    store.clearForAccessRevocation();
    older.resolve({
      events: [threadMessageEvent('stale-reply', 'thread-root') as never],
      startCursor: 'before-stale',
      endCursor: 'stale',
      hasOlder: false,
      hasNewer: true
    });
    await loading;

    expect(store.events).toEqual([]);
    expect(store.isLoadingMore).toBe(false);
    store.dispose();
  });

  it('keeps the late latest-page fallback when an in-flight jump omits its target', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    let resolveAround: ((value: AroundPage) => void) | undefined;
    const aroundPage = new Promise<AroundPage>((resolve) => {
      resolveAround = resolve;
    });
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => aroundPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.awaitRoomProjection('room-1');

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('missing-target', jumpState);
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('latest-message'));
    resolveAround?.({
      events: [threadMessageEvent('other-message') as never],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    });

    await expect(jumping).resolves.toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['latest-message']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('keeps the late latest-page fallback when an in-flight jump fails', async () => {
    const fake = new FakeQueryClient();
    let rejectAround: ((reason: Error) => void) | undefined;
    const aroundPage = new Promise<never>((_resolve, reject) => {
      rejectAround = reject;
    });
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => aroundPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.awaitRoomProjection('room-1');

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('failed-target', jumpState);
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('latest-message'));
    rejectAround?.(new Error('network failed'));

    await expect(jumping).resolves.toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['latest-message']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('completes a jump when late hydration contains a target omitted by the around page', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    let resolveAround: ((value: AroundPage) => void) | undefined;
    const aroundPage = new Promise<AroundPage>((resolve) => {
      resolveAround = resolve;
    });
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => aroundPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.awaitRoomProjection('room-1');

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('hydrated-target', jumpState);
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('hydrated-target'));
    resolveAround?.({
      events: [threadMessageEvent('other-message') as never],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    });

    await expect(jumping).resolves.toBe(true);
    expect(jumpState.scrollToEventId).toBe('hydrated-target');
    expect(store.rootEvents.map((event) => event.id)).toEqual(['hydrated-target']);
    store.dispose();
  });

  it('completes a jump when late hydration contains the target after the around read fails', async () => {
    const fake = new FakeQueryClient();
    let rejectAround: ((reason: Error) => void) | undefined;
    const aroundPage = new Promise<never>((_resolve, reject) => {
      rejectAround = reject;
    });
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => aroundPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.awaitRoomProjection('room-1');

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('hydrated-target', jumpState);
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('hydrated-target'));
    rejectAround?.(new Error('network failed'));

    await expect(jumping).resolves.toBe(true);
    expect(jumpState.scrollToEventId).toBe('hydrated-target');
    expect(store.rootEvents.map((event) => event.id)).toEqual(['hydrated-target']);
    store.dispose();
  });

  it('reports a failed jump when the around page omits the target', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async () => ({
        events: [threadMessageEvent('m3') as never],
        startCursor: 'tl:start',
        endCursor: 'tl:end',
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();
    store.events = [threadMessageEvent('m1') as never];

    const jumpState = new JumpToMessageState();
    jumpState.isJumpedMode = true;
    jumpState.scrollToEventId = 'previous';
    const jumped = await store.jumpToMessage('m2', jumpState);

    expect(jumped).toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1']);
    expect(jumpState.isJumpedMode).toBe(false);
    expect(jumpState.scrollToEventId).toBeNull();
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('reports a failed jump when loading the target window rejects', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async () => {
        throw new Error('network failed');
      })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const jumpState = new JumpToMessageState();
    const jumped = await store.jumpToMessage('m2', jumpState);

    expect(jumped).toBe(false);
    expect(jumpState.scrollToEventId).toBeNull();
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('discards a jump response superseded by a newer jump', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    let resolveFirst: ((value: AroundPage) => void) | undefined;
    const firstPage = new Promise<AroundPage>((resolve) => {
      resolveFirst = resolve;
    });
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi
        .fn()
        .mockReturnValueOnce(firstPage)
        .mockResolvedValueOnce({
          events: [threadMessageEvent('new-target') as never],
          startCursor: 'tl:new',
          endCursor: 'tl:new',
          hasOlder: false,
          hasNewer: false
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const jumpState = new JumpToMessageState();
    const firstJump = store.jumpToMessage('old-target', jumpState);
    const secondJump = store.jumpToMessage('new-target', jumpState);
    await expect(secondJump).resolves.toBe(true);

    resolveFirst?.({
      events: [threadMessageEvent('old-target') as never],
      startCursor: 'tl:old',
      endCursor: 'tl:old',
      hasOlder: false,
      hasNewer: false
    });

    await expect(firstJump).resolves.toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['new-target']);
    expect(jumpState.scrollToEventId).toBe('new-target');
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('does not cancel the initial room load when jumping to an already-loaded event', async () => {
    const fake = new FakeQueryClient();
    type RoomPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEvents']>>;
    let resolveInitial: ((value: RoomPage) => void) | undefined;
    const initialPage = new Promise<RoomPage>((resolve) => {
      resolveInitial = resolve;
    });
    const timeline = fakeTimelineAPI({ getRoomEvents: vi.fn(() => initialPage) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    store.events = [threadMessageEvent('linked-realtime') as never];

    const jumpState = new JumpToMessageState();
    await expect(store.jumpToMessage('linked-realtime', jumpState)).resolves.toBe(true);
    expect(store.isInitialLoading).toBe(true);

    resolveInitial?.({
      events: [threadMessageEvent('authoritative') as never],
      startCursor: null,
      endCursor: null,
      hasOlder: false,
      hasNewer: false
    });
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual(['authoritative', 'linked-realtime']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('discards load-newer results from a replaced jump window', async () => {
    const fake = new FakeQueryClient();
    type RoomPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEvents']>>;
    let resolveNewer: ((value: RoomPage) => void) | undefined;
    const newerPage = new Promise<RoomPage>((resolve) => {
      resolveNewer = resolve;
    });
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi
        .fn()
        .mockResolvedValueOnce({
          events: [],
          startCursor: null,
          endCursor: null,
          hasOlder: false,
          hasNewer: false
        })
        .mockReturnValueOnce(newerPage),
      getRoomEventsAround: vi
        .fn()
        .mockResolvedValueOnce({
          events: [threadMessageEvent('first-target') as never],
          startCursor: 'tl:first',
          endCursor: 'tl:first',
          hasOlder: true,
          hasNewer: true
        })
        .mockResolvedValueOnce({
          events: [threadMessageEvent('second-target') as never],
          startCursor: 'tl:second',
          endCursor: 'tl:second',
          hasOlder: true,
          hasNewer: true
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const jumpState = new JumpToMessageState();
    await store.jumpToMessage('first-target', jumpState);
    const loadingNewer = store.loadNewer(jumpState);
    await store.jumpToMessage('second-target', jumpState);

    resolveNewer?.({
      events: [threadMessageEvent('stale-newer') as never],
      startCursor: 'tl:stale',
      endCursor: 'tl:stale',
      hasOlder: false,
      hasNewer: false
    });
    await loadingNewer;

    expect(store.rootEvents.map((event) => event.id)).toEqual(['second-target']);
    expect(jumpState.scrollToEventId).toBe('second-target');
    expect(jumpState.isLoadingNewer).toBe(false);
    store.dispose();
  });

  it('discards load-newer results after a room lifecycle restoration', async () => {
    const fake = new FakeQueryClient();
    type RoomPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEvents']>>;
    let resolveNewer!: (value: RoomPage) => void;
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi.fn(
        () =>
          new Promise<RoomPage>((resolve) => {
            resolveNewer = resolve;
          })
      ),
      getRoomEventsAround: vi.fn(async () => ({
        events: [threadMessageEvent('historical-target') as never],
        startCursor: 'tl:historical',
        endCursor: 'tl:historical',
        hasOlder: true,
        hasNewer: true
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.replaceRoomProjectionPage('room-1', projectedMessagePage('latest-message'));

    const jumpState = new JumpToMessageState();
    await store.jumpToMessage('historical-target', jumpState);
    const loadingNewer = store.loadNewer(jumpState);
    store.restoreRoomProjectionPage('room-1', projectedMessagePage('latest-message'));
    resolveNewer({
      events: [threadMessageEvent('stale-newer') as never],
      startCursor: 'tl:stale',
      endCursor: 'tl:stale',
      hasOlder: false,
      hasNewer: false
    });
    await loadingNewer;

    expect(store.rootEvents.map((event) => event.id)).toEqual(['latest-message']);
    store.dispose();
  });

  it('discards an in-flight jump after returning to the present', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    let resolveAround: ((value: AroundPage) => void) | undefined;
    const aroundPage = new Promise<AroundPage>((resolve) => {
      resolveAround = resolve;
    });
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi
        .fn()
        .mockResolvedValueOnce({
          events: [],
          startCursor: null,
          endCursor: null,
          hasOlder: false,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [threadMessageEvent('present') as never],
          startCursor: 'tl:present',
          endCursor: 'tl:present',
          hasOlder: false,
          hasNewer: false
        }),
      getRoomEventsAround: vi.fn(() => aroundPage)
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const jumpState = new JumpToMessageState();
    const jumping = store.jumpToMessage('historical', jumpState);
    const returningToPresent = store.jumpToPresent(jumpState);
    await settle();
    await expect(returningToPresent).resolves.toBe(true);

    resolveAround?.({
      events: [threadMessageEvent('historical') as never],
      startCursor: 'tl:historical',
      endCursor: 'tl:historical',
      hasOlder: true,
      hasNewer: true
    });

    await expect(jumping).resolves.toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['present']);
    expect(jumpState.isJumpedMode).toBe(false);
    expect(jumpState.scrollToEventId).toBeNull();
    store.dispose();
  });

  it('resolves returning to present only after initial backfill completes', async () => {
    const fake = new FakeQueryClient();
    type RoomPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEvents']>>;
    let resolveOlder: ((page: RoomPage) => void) | undefined;
    const olderPage = new Promise<RoomPage>((resolve) => {
      resolveOlder = resolve;
    });
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi
        .fn()
        .mockResolvedValueOnce(emptyPage())
        .mockResolvedValueOnce({
          events: [threadMessageEvent('present') as never],
          startCursor: 'tl:present',
          endCursor: 'tl:present',
          hasOlder: true,
          hasNewer: false
        })
        .mockImplementationOnce(() => olderPage)
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    let completed = false;
    const returningToPresent = store.jumpToPresent(new JumpToMessageState()).then((loaded) => {
      completed = true;
      return loaded;
    });
    await settle();
    expect(completed).toBe(false);

    resolveOlder?.({
      events: [threadMessageEvent('older') as never],
      startCursor: 'tl:older',
      endCursor: 'tl:older',
      hasOlder: false,
      hasNewer: true
    });

    await expect(returningToPresent).resolves.toBe(true);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['older', 'present']);
    store.dispose();
  });

  it('clears jump loading when an in-flight jump is superseded by a loaded target', async () => {
    const fake = new FakeQueryClient();
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const unresolvedAround = new Promise<AroundPage>(() => {});
    const timeline = fakeTimelineAPI({ getRoomEventsAround: vi.fn(() => unresolvedAround) });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();
    store.events = [threadMessageEvent('loaded-target') as never];

    const jumpState = new JumpToMessageState();
    void store.jumpToMessage('missing-target', jumpState);
    expect(store.isInitialLoading).toBe(true);

    await expect(store.jumpToMessage('loaded-target', jumpState)).resolves.toBe(true);
    expect(store.isInitialLoading).toBe(false);
    expect(jumpState.scrollToEventId).toBe('loaded-target');
    store.dispose();
  });

  it('releases initial-load ownership when a refresh supersedes it', async () => {
    const fake = new FakeQueryClient();
    type RoomPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEvents']>>;
    const unresolvedInitial = new Promise<RoomPage>(() => {});
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi
        .fn()
        .mockReturnValueOnce(unresolvedInitial)
        .mockResolvedValueOnce({
          events: [threadMessageEvent('refreshed') as never],
          startCursor: 'tl:refreshed',
          endCursor: 'tl:refreshed',
          hasOlder: false,
          hasNewer: false
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    expect(store.isInitialLoading).toBe(true);

    await store.refreshCurrentWindow();

    expect(store.rootEvents.map((event) => event.id)).toEqual(['refreshed']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('loads room history through the injected timeline API', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi.fn(async () => ({
        events: [threadMessageEvent('m1') as never],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setRoom('room-1');
    await settle();

    expect(timeline.getRoomEvents).toHaveBeenCalledWith({ roomId: 'room-1', limit: 50 });
    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1']);
    store.dispose();
  });

  it('backfills the initial room window when the latest page has too few messages', async () => {
    const join = roomSystemEvent('join-1', RoomEventKind.UserJoinedRoom);
    const firstMessage = threadMessageEvent('m2');
    const olderMessage = threadMessageEvent('m1');
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi
        .fn()
        .mockResolvedValueOnce({
          events: [firstMessage as never, join as never],
          startCursor: 'tl:cursor-join',
          endCursor: 'tl:cursor-join',
          hasOlder: true,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [olderMessage as never],
          startCursor: 'tl:cursor-message',
          endCursor: 'tl:cursor-message',
          hasOlder: false,
          hasNewer: true
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setRoom('room-1');

    await vi.waitFor(() => {
      expect(store.isInitialLoading).toBe(false);
      expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'join-1', 'm2']);
    });
    expect(timeline.getRoomEvents).toHaveBeenNthCalledWith(1, { roomId: 'room-1', limit: 50 });
    expect(timeline.getRoomEvents).toHaveBeenNthCalledWith(2, {
      roomId: 'room-1',
      limit: 50,
      before: 'tl:cursor-join'
    });
    store.dispose();
  });

  it('does not refetch or clear events when setRoom is called for the current room', async () => {
    const loaded = threadMessageEvent('m1');
    const fake = new FakeQueryClient(
      roomEventsResult({
        events: [loaded],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.setRoom('room-1');
    await settle();

    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('serves already-loaded events by id without querying the timeline API', async () => {
    const loaded = threadMessageEvent('m1');
    const fake = new FakeQueryClient(
      roomEventsResult({
        events: [loaded],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    expect(store.getEventById('m1')?.id).toBe(loaded.id);
    await store.ensureEvent('m1');

    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('deduplicates concurrent off-window event fetches', async () => {
    const target = threadMessageEvent('target');
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: target } }
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await Promise.all([store.ensureEvent('target'), store.ensureEvent('target')]);

    expect(store.getEventById('target')?.id).toBe('target');
    expect(fake.queryMock).toHaveBeenCalledOnce();
    store.dispose();
  });

  it('rejects an in-flight preview from before reset without losing its replacement request', async () => {
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const staleRead = deferred<AroundPage>();
    const replacementRead = deferred<AroundPage>();
    const getRoomEventsAround = vi
      .fn<RoomTimelineAPI['getRoomEventsAround']>()
      .mockImplementationOnce(() => staleRead.promise)
      .mockImplementationOnce(() => replacementRead.promise);
    const timeline = fakeTimelineAPI({ getRoomEventsAround });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();

    const stale = store.ensureEvent('preview');
    store.resetProjectionState();
    const replacement = store.ensureEvent('preview');
    staleRead.resolve(pageFromEvent(threadMessageEvent('preview')));
    await stale;

    expect(store.getEventById('preview')).toBeUndefined();
    expect(store.ensureEvent('preview')).toBe(replacement);

    replacementRead.resolve(pageFromEvent({ ...threadMessageEvent('preview'), actorId: 'current' }));
    await replacement;
    expect(store.getEventById('preview')).toMatchObject({ id: 'preview', actorId: 'current' });
    expect(getRoomEventsAround).toHaveBeenCalledTimes(2);
    store.dispose();
  });

  it('rejects an in-flight preview after access revocation and blocks new preview reads', async () => {
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const revokedRead = deferred<AroundPage>();
    const getRoomEventsAround = vi.fn<RoomTimelineAPI['getRoomEventsAround']>(() =>
      revokedRead.promise
    );
    const timeline = fakeTimelineAPI({ getRoomEventsAround });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();

    const loading = store.ensureEvent('preview');
    store.clearForAccessRevocation();
    expect(store.ensureEvent('another-preview')).toBeUndefined();
    revokedRead.resolve(pageFromEvent(threadMessageEvent('preview')));
    await loading;

    expect(store.getEventById('preview')).toBeUndefined();
    expect(getRoomEventsAround).toHaveBeenCalledOnce();
    store.dispose();
  });

  it('rejects an in-flight preview after its message is deleted', async () => {
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const pendingRead = deferred<AroundPage>();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(() => pendingRead.promise)
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();

    const loading = store.ensureEvent('preview');
    store.ingestEvent(messageRetraction('preview') as never);
    pendingRead.resolve(pageFromEvent(threadMessageEvent('preview')));
    await loading;

    expect(store.getEventById('preview')).toBeUndefined();
    store.dispose();
  });

  it('keeps a delayed primary timeline response behind its message tombstone', async () => {
    type AroundPage = Awaited<ReturnType<RoomTimelineAPI['getRoomEventsAround']>>;
    const pendingRead = deferred<AroundPage>();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(() => pendingRead.promise)
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();
    store.events = [threadMessageEvent('main') as never];
    const refreshing = store.refreshCurrentWindow('main');

    store.ingestEvent(messageRetraction('main') as never);
    pendingRead.resolve(pageFromEvent(threadMessageEvent('main')));
    await refreshing;

    expect(store.getEventById('main')?.event).toMatchObject({
      body: null,
      attachments: [],
      deletedAt: '2026-05-27T00:00:02Z'
    });
    store.dispose();
  });

  it('tombstones linked and retained previews at deletion boundaries', async () => {
    const linkedEcho = {
      ...threadMessageEvent('echo'),
      event: { ...threadMessageEvent('echo').event, echoOfEventId: 'original' }
    };
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async ({ eventId }) =>
        pageFromEvent(eventId === 'echo' ? linkedEcho : threadMessageEvent(eventId))
      )
    });
    const store = new MessagesStore(
      new FakeQueryClient() as unknown as ServerConnection,
      () => null,
      timeline
    );
    store.setRoom('room-1');
    await settle();
    await store.ensureEvent('echo');
    await store.ensureEvent('retained');

    store.ingestEvent(messageRetraction('original') as never);
    store.upsertRoomProjectionEvent(
      'room-1',
      new RoomTimelineEvent({
        id: 'retained',
        event: {
          case: 'messagePosted',
          value: new RoomMessagePosted({
            message: new Message({
              id: 'retained',
              roomId: 'room-1',
              deletedAt: Timestamp.fromDate(new Date('2026-05-27T00:00:03Z'))
            })
          })
        }
      }),
      undefined,
      true
    );

    expect(store.getEventById('echo')?.event).toMatchObject({
      body: null,
      attachments: [],
      deletedAt: '2026-05-27T00:00:02Z'
    });
    expect(store.getEventById('retained')?.event).toMatchObject({
      body: null,
      attachments: [],
      deletedAt: '2026-05-27T00:00:03.000Z'
    });
    store.dispose();
  });

  it('does not cache transient off-window event fetch errors as missing', async () => {
    const target = threadMessageEvent('target');
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }),
      { data: null, error: new Error('temporary failure') },
      { room: { event: target } }
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.ensureEvent('target');
    expect(store.getEventById('target')).toBeUndefined();

    await store.ensureEvent('target');

    expect(store.getEventById('target')?.id).toBe('target');
    expect(fake.queryMock).toHaveBeenCalledTimes(2);
    errorSpy.mockRestore();
    store.dispose();
  });

  it('applies MessageEditedEvent payloads inline without refetching', async () => {
    const fake = new FakeQueryClient({
      room: {
        events: {
          events: [
            {
              id: 'm1',
              createdAt: '2026-05-27T00:00:00Z',
              actorId: 'u1',
              actor: null,
              event: {
                kind: RoomEventKind.MessagePosted,
                roomId: 'room-1',
                body: 'before',
                attachments: [],
                linkPreview: null,
                updatedAt: null,
                inReplyTo: null,
                threadRootEventId: null,
                echoOfEventId: null,
                echoFromThreadRootEventId: null,
                replyCount: 0,
                lastReplyAt: null,
                threadParticipants: [],
                viewerIsFollowingThread: null
              }
            }
          ],
          hasOlder: false,
          hasNewer: false
        }
      }
    });
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent({
      id: 'edit-1',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u1',
      actor: null,
      event: {
        kind: RoomEventKind.MessageEdited,
        roomId: 'room-1',
        messageEventId: 'm1',
        body: 'after',
        attachments: [],
        linkPreview: null,
        updatedAt: '2026-05-27T00:00:01Z'
      }
    } as never);

    expect(store.rootEvents[0].event).toMatchObject({
      body: 'after',
      updatedAt: '2026-05-27T00:00:01Z'
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('applies local-kind message retraction payloads inline without refetching', async () => {
    const fake = new FakeQueryClient({
      room: {
        events: {
          events: [
            {
              id: 'm1',
              createdAt: '2026-05-27T00:00:00Z',
              actorId: 'u1',
              actor: null,
              event: {
                kind: RoomEventKind.MessagePosted,
                roomId: 'room-1',
                body: 'before',
                attachments: [{ id: 'a1' }],
                linkPreview: null,
                updatedAt: null,
                inReplyTo: null,
                threadRootEventId: null,
                echoOfEventId: null,
                echoFromThreadRootEventId: null,
                replyCount: 0,
                lastReplyAt: null,
                threadParticipants: [],
                viewerIsFollowingThread: null
              }
            }
          ],
          hasOlder: false,
          hasNewer: false
        }
      }
    });
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.applyLocalMessageDeletion('m1');
    const provisionalMessage = store.rootEvents[0].event;
    if (!provisionalMessage) throw new Error('expected provisional message payload');
    expect(provisionalMessage).toMatchObject({ body: null, attachments: [] });
    expect(
      Number.isFinite(
        Date.parse('deletedAt' in provisionalMessage ? (provisionalMessage.deletedAt ?? '') : '')
      )
    ).toBe(true);

    store.ingestEvent({
      id: 'retract-1',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u1',
      actor: null,
      event: {
        kind: RoomEventKind.MessageRetracted,
        roomId: 'room-1',
        messageEventId: 'm1',
        retractedReason: null
      }
    } as never);

    expect(store.rootEvents[0].event).toMatchObject({
      body: null,
      attachments: [],
      deletedAt: '2026-05-27T00:00:01Z'
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('ignores call lifecycle and participant events in the room timeline', async () => {
    const fake = new FakeQueryClient(
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent(callEvent(RoomEventKind.CallStarted, 'call-started') as never);
    store.ingestEvent(callEvent(RoomEventKind.CallParticipantJoined, 'call-joined') as never);
    store.ingestEvent(callEvent(RoomEventKind.CallParticipantLeft, 'call-left') as never);
    store.ingestEvent(callEvent(RoomEventKind.CallEnded, 'call-ended') as never);

    expect(store.rootEvents).toEqual([]);
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('hydrates actorless live room lifecycle events before inserting them', async () => {
    const hydratedArchive = roomSystemEvent('archive-1', RoomEventKind.RoomArchived, {
      id: 'u1',
      displayName: 'Alice'
    });
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: hydratedArchive } }
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent(roomSystemEvent('archive-1', RoomEventKind.RoomArchived) as never);
    await settle();

    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({
      roomId: 'room-1',
      eventId: 'archive-1',
      limit: 1
    });
    expect(store.rootEvents).toHaveLength(1);
    expect(store.rootEvents[0]).toMatchObject({
      id: 'archive-1',
      actor: { id: 'u1', displayName: 'Alice' },
      event: { kind: RoomEventKind.RoomArchived, roomId: 'room-1' }
    });
    store.dispose();
  });

  it('refetches a loaded message when a replayed reaction event arrives', async () => {
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [threadMessageEvent('m1')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: messageWithReaction('m1', 'heart') } }
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent({
      id: 'reaction-1',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u2',
      actor: null,
      event: {
        kind: RoomEventKind.ReactionAdded,
        roomId: 'room-1',
        messageEventId: 'm1',
        emoji: 'heart'
      }
    } as never);
    await settle();

    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({ roomId: 'room-1', eventId: 'm1', limit: 1 });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    expect(store.rootEvents[0].event).toMatchObject({
      reactions: [{ emoji: 'heart', count: 1 }]
    });
    store.dispose();
  });

  it('refetches a visible echo when a reaction event targets the original reply', async () => {
    const baseEcho = threadMessageEvent('echo');
    const echo = {
      ...baseEcho,
      event: {
        ...baseEcho.event,
        body: 'reply',
        echoOfEventId: 'reply',
        echoFromThreadRootEventId: 'root'
      }
    };
    const updatedEcho = {
      ...echo,
      event: {
        ...echo.event,
        reactions: [{ emoji: 'heart', count: 1, hasReacted: false, users: [] }]
      }
    };
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [echo],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: updatedEcho } }
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent({
      id: 'reaction-echo',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u2',
      actor: null,
      event: {
        kind: RoomEventKind.ReactionAdded,
        roomId: 'room-1',
        messageEventId: 'reply',
        emoji: 'heart'
      }
    } as never);
    await settle();

    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({
      roomId: 'room-1',
      eventId: 'echo',
      limit: 1
    });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    expect(store.rootEvents[0].event).toMatchObject({
      reactions: [{ emoji: 'heart', count: 1 }]
    });
    store.dispose();
  });

  it('does not let a stale rollback overwrite an authoritative reaction refetch', async () => {
    const updated = messageWithReactionState('m1', {
      emoji: 'heart',
      count: 7,
      hasReacted: true
    });
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi.fn(async () => ({
        events: [messageWithReaction('m1', 'heart') as never],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      })),
      getRoomEventsAround: vi.fn(async () => pageFromEvent(updated))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const optimistic = store.beginOptimisticReaction({
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'add'
    });

    store.ingestEvent({
      id: 'reaction-authoritative',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u2',
      actor: null,
      event: {
        kind: RoomEventKind.ReactionAdded,
        roomId: 'room-1',
        messageEventId: 'm1',
        emoji: 'heart'
      }
    } as never);
    await settle();
    optimistic.rollback();

    expect(reactionsOf(store.rootEvents[0])).toMatchObject([
      { emoji: 'heart', count: 7, hasReacted: true }
    ]);
    store.dispose();
  });

  it('does not let a stale thread-follow rollback overwrite an authoritative refetch', async () => {
    const event = threadMessageEvent('m1');
    const updated = {
      ...event,
      event: {
        ...event.event,
        viewerIsFollowingThread: true
      }
    };
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi.fn(async () => ({
        events: [threadMessageEvent('m1') as never],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      })),
      getRoomEventsAround: vi.fn(async () => pageFromEvent(updated))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();

    const optimistic = store.beginOptimisticThreadFollow('m1', true);

    await store.refreshCurrentWindow('m1');
    optimistic.rollback();

    expect(store.rootEvents[0].event).toMatchObject({
      viewerIsFollowingThread: true
    });
    store.dispose();
  });

  it('patches and rolls back optimistic reactions in the preview cache', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEventsAround: vi.fn(async () => pageFromEvent(messageWithReaction('preview', 'heart')))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setRoom('room-1');
    await settle();
    await store.ensureEvent('preview');

    const optimistic = store.beginOptimisticReaction({
      messageEventId: 'preview',
      emoji: 'heart',
      action: 'add'
    });

    expect(reactionsOf(store.getEventById('preview')!)).toMatchObject([
      { emoji: 'heart', count: 2, hasReacted: true }
    ]);

    optimistic.rollback();

    expect(reactionsOf(store.getEventById('preview')!)).toMatchObject([
      { emoji: 'heart', count: 1, hasReacted: false }
    ]);
    store.dispose();
  });

  it('hides only the echo when an echo is retracted', async () => {
    const fake = new FakeQueryClient({
      room: {
        events: {
          events: [
            {
              id: 'root',
              createdAt: '2026-05-27T00:00:00Z',
              actorId: 'u1',
              actor: null,
              event: {
                kind: RoomEventKind.MessagePosted,
                roomId: 'room-1',
                body: 'root',
                attachments: [],
                linkPreview: null,
                updatedAt: null,
                inReplyTo: null,
                threadRootEventId: null,
                echoOfEventId: null,
                echoFromThreadRootEventId: null,
                replyCount: 1,
                lastReplyAt: null,
                threadParticipants: [],
                viewerIsFollowingThread: null
              }
            },
            {
              id: 'echo',
              createdAt: '2026-05-27T00:00:01Z',
              actorId: 'u1',
              actor: null,
              event: {
                kind: RoomEventKind.MessagePosted,
                roomId: 'room-1',
                body: 'reply',
                attachments: [],
                linkPreview: null,
                updatedAt: null,
                inReplyTo: null,
                threadRootEventId: null,
                echoOfEventId: 'reply',
                echoFromThreadRootEventId: 'root',
                replyCount: 0,
                lastReplyAt: null,
                threadParticipants: [],
                viewerIsFollowingThread: null
              }
            }
          ],
          hasOlder: false,
          hasNewer: false
        }
      }
    });
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent({
      id: 'retract-echo',
      createdAt: '2026-05-27T00:00:02Z',
      actorId: 'u1',
      actor: null,
      event: {
        kind: RoomEventKind.MessageRetracted,
        roomId: 'room-1',
        messageEventId: 'echo',
        retractedReason: null
      }
    } as never);

    expect(store.rootEvents.map((event) => event.id)).toEqual(['root']);
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('tombstones visible echoes when the original reply is retracted', async () => {
    const fake = new FakeQueryClient({
      room: {
        events: {
          events: [
            {
              id: 'echo',
              createdAt: '2026-05-27T00:00:01Z',
              actorId: 'u1',
              actor: null,
              event: {
                kind: RoomEventKind.MessagePosted,
                roomId: 'room-1',
                body: 'reply',
                attachments: [{ id: 'a1' }],
                linkPreview: null,
                updatedAt: null,
                inReplyTo: null,
                threadRootEventId: null,
                echoOfEventId: 'reply',
                echoFromThreadRootEventId: 'root',
                replyCount: 0,
                lastReplyAt: null,
                threadParticipants: [],
                viewerIsFollowingThread: null
              }
            }
          ],
          hasOlder: false,
          hasNewer: false
        }
      }
    });
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent({
      id: 'retract-original',
      createdAt: '2026-05-27T00:00:02Z',
      actorId: 'u1',
      actor: null,
      event: {
        kind: RoomEventKind.MessageRetracted,
        roomId: 'room-1',
        messageEventId: 'reply',
        retractedReason: null
      }
    } as never);

    expect(store.rootEvents[0].event).toMatchObject({
      body: null,
      attachments: [],
      deletedAt: '2026-05-27T00:00:02Z'
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('runs an initial fetch on setRoom', async () => {
    const fake = new FakeQueryClient({
      room: { events: { events: [], hasOlder: false, hasNewer: false } }
    });
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();

    expect(fake.queryMock).toHaveBeenCalledTimes(1);
    store.dispose();
  });

  it('ingests a returned root room message immediately and dedupes later subscription delivery', async () => {
    const fake = new FakeQueryClient(
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );
    const returnedPost = threadMessageEvent('m-local');

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent(returnedPost as never);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m-local']);

    store.ingestEvent(returnedPost as never);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m-local']);
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('applies a returned thread reply to the room root only once when subscription delivery follows', async () => {
    const fake = new FakeQueryClient(
      roomEventsResult({
        events: [threadMessageEvent('t1')],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );
    const returnedReply = threadMessageEvent('r1', 't1');

    store.setRoom('room-1');
    await settle();

    store.ingestEvent(returnedReply as never);
    store.ingestEvent(returnedReply as never);

    expect(store.rootEvents[0].event).toMatchObject({ replyCount: 1 });
    store.dispose();
  });

  it('soft-refreshes the latest room window without entering initial loading', async () => {
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [threadMessageEvent('m1')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      }),
      roomEventsResult({
        events: [messageWithReaction('m1', 'heart'), threadMessageEvent('m2')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-2',
        hasOlder: false,
        hasNewer: false
      })
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow();
    await settle();

    expect(store.isInitialLoading).toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2']);
    expect(store.rootEvents[0].event).toMatchObject({
      reactions: [{ emoji: 'heart', count: 1 }]
    });
    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({ roomId: 'room-1', limit: 50 });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    store.dispose();
  });

  it('keeps the event array stable when a latest soft-refresh is unchanged', async () => {
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [threadMessageEvent('m1'), threadMessageEvent('m2')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-2',
        hasOlder: false,
        hasNewer: false
      }),
      roomEventsResult({
        events: [threadMessageEvent('m1'), threadMessageEvent('m2')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-2',
        hasOlder: false,
        hasNewer: false
      })
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    const previousEvents = store.events;

    const result = await store.refreshCurrentWindow();
    await settle();

    expect(result).toMatchObject({ refreshed: true, changed: false });
    expect(store.events).toBe(previousEvents);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2']);
    store.dispose();
  });

  it('preserves the loaded room window when a latest soft-refresh adds newer events', async () => {
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [threadMessageEvent('m1'), threadMessageEvent('m2'), threadMessageEvent('m3')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-3',
        hasOlder: false,
        hasNewer: false
      }),
      roomEventsResult({
        events: [threadMessageEvent('m3'), threadMessageEvent('m4')],
        startCursor: 'tl:cursor-3',
        endCursor: 'tl:cursor-4',
        hasOlder: true,
        hasNewer: false
      })
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();

    const result = await store.refreshCurrentWindow();
    await settle();

    expect(result).toMatchObject({ refreshed: true, changed: true });
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2', 'm3', 'm4']);
    store.dispose();
  });

  it('replaces a disjoint latest room refresh so older pagination can bridge gaps', async () => {
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [threadMessageEvent('m1'), threadMessageEvent('m2')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-2',
        hasOlder: false,
        hasNewer: false
      }),
      roomEventsResult({
        events: [threadMessageEvent('m5'), threadMessageEvent('m6')],
        startCursor: 'tl:cursor-5',
        endCursor: 'tl:cursor-6',
        hasOlder: true,
        hasNewer: false
      }),
      roomEventsResult({
        events: [threadMessageEvent('m3'), threadMessageEvent('m4')],
        startCursor: 'tl:cursor-3',
        endCursor: 'tl:cursor-4',
        hasOlder: false,
        hasNewer: true
      })
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    const result = await store.refreshCurrentWindow();
    await settle();

    expect(result).toMatchObject({ refreshed: true, changed: true });
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m5', 'm6']);
    expect(store.hasReachedStart).toBe(false);

    await store.loadMore();
    await settle();

    expect(fake.queryMock.mock.calls[1][0]).toBe('timeline:before');
    expect(fake.queryMock.mock.calls[1][1]).toEqual({
      roomId: 'room-1',
      limit: 50,
      before: 'tl:cursor-5'
    });
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m3', 'm4', 'm5', 'm6']);
    expect(store.hasReachedStart).toBe(true);
    store.dispose();
  });

  it('soft-refreshes around an anchor event when one is provided', async () => {
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [threadMessageEvent('m1'), threadMessageEvent('m2'), threadMessageEvent('m3')],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-3',
        hasOlder: false,
        hasNewer: false
      }),
      {
        room: {
          eventsAround: {
            events: [messageWithReaction('m2', 'thumbsup')],
            targetIndex: 0,
            startCursor: 'tl:cursor-2',
            endCursor: 'tl:cursor-2',
            hasOlder: true,
            hasNewer: true
          }
        }
      }
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('m2');
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2', 'm3']);
    expect(store.hasReachedStart).toBe(true);
    expect(store.rootEvents.find((event) => event.id === 'm2')?.event).toMatchObject({
      reactions: [{ emoji: 'thumbsup', count: 1 }]
    });
    expect(fake.queryMock.mock.calls[0][1]).toEqual({
      roomId: 'room-1',
      eventId: 'm2',
      limit: 50
    });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    store.dispose();
  });

  it('keeps live events ordered when anchored refresh races forward pagination', async () => {
    let resolveAnchoredRefresh!: (value: unknown) => void;
    const anchoredRefresh = new Promise((resolve) => {
      resolveAnchoredRefresh = resolve;
    });
    const fake = new FakeQueryClient([
      roomEventsResult({
        events: [
          threadMessageEvent('m1'),
          threadMessageEvent('m2'),
          threadMessageEvent('m3'),
          threadMessageEvent('m4'),
          threadMessageEvent('m5')
        ],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-5',
        hasOlder: false,
        hasNewer: true
      }),
      anchoredRefresh,
      roomEventsResult({
        events: [threadMessageEvent('m6'), threadMessageEvent('m7')],
        startCursor: 'tl:cursor-6',
        endCursor: 'tl:cursor-7',
        hasOlder: true,
        hasNewer: true
      })
    ]);
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    const refresh = store.refreshCurrentWindow('m3');
    store.ingestEvent(threadMessageEvent('m8') as never);
    resolveAnchoredRefresh({
      room: {
        eventsAround: {
          events: [threadMessageEvent('m3'), threadMessageEvent('m4'), threadMessageEvent('m5')],
          targetIndex: 0,
          startCursor: 'tl:cursor-3',
          endCursor: 'tl:cursor-5',
          hasOlder: true,
          hasNewer: true
        }
      }
    });

    await refresh;
    await settle();
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2', 'm3', 'm4', 'm5', 'm8']);

    const jumpState = new JumpToMessageState();
    jumpState.isJumpedMode = true;
    await store.loadNewer(jumpState);
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual([
      'm1',
      'm2',
      'm3',
      'm4',
      'm5',
      'm6',
      'm7',
      'm8'
    ]);
    store.dispose();
  });

  it('keeps backward pagination alive when an older page adds no new rows but has more history', async () => {
    const currentWindow = Array.from({ length: 10 }, (_, index) =>
      threadMessageEvent(`m${index + 10}`)
    );
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getRoomEvents: vi
        .fn()
        .mockResolvedValueOnce({
          events: currentWindow as never[],
          startCursor: 'tl:cursor-3',
          endCursor: 'tl:cursor-3',
          hasOlder: true,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [threadMessageEvent('m10') as never],
          startCursor: 'tl:cursor-2',
          endCursor: 'tl:cursor-2',
          hasOlder: true,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [threadMessageEvent('m1') as never],
          startCursor: 'tl:cursor-1',
          endCursor: 'tl:cursor-1',
          hasOlder: false,
          hasNewer: true
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setRoom('room-1');
    await settle();

    await store.loadMore();
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual(
      currentWindow.map((event) => event.id)
    );
    expect(store.hasReachedStart).toBe(false);

    await store.loadMore();
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual([
      'm1',
      ...currentWindow.map((event) => event.id)
    ]);
    expect(store.hasReachedStart).toBe(true);
    expect(timeline.getRoomEvents).toHaveBeenNthCalledWith(2, {
      roomId: 'room-1',
      limit: 50,
      before: 'tl:cursor-3'
    });
    expect(timeline.getRoomEvents).toHaveBeenNthCalledWith(3, {
      roomId: 'room-1',
      limit: 50,
      before: 'tl:cursor-2'
    });
    store.dispose();
  });

  it('soft-refreshes a thread around an anchored reply', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [
          threadMessageEvent('t1') as never,
          threadMessageEvent('r18', 't1') as never,
          threadMessageEvent('r19', 't1') as never,
          threadMessageEvent('r20', 't1') as never
        ],
        startCursor: 'tl:cursor-18',
        endCursor: 'tl:cursor-20',
        hasOlder: true,
        hasNewer: true
      })),
      getThreadEventsAround: vi.fn(async () => ({
        events: [
          threadMessageEvent('t1') as never,
          threadMessageEvent('r19', 't1') as never,
          threadMessageWithReaction('r20', 't1', 'thumbsup') as never,
          threadMessageEvent('r21', 't1') as never
        ],
        startCursor: 'tl:cursor-19',
        endCursor: 'tl:cursor-21',
        hasOlder: true,
        hasNewer: true
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('r20');
    await settle();

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r18', 'r19', 'r20', 'r21']);
    expect(store.hasReachedStart).toBe(false);
    expect(store.threadEvents.find((event) => event.id === 'r20')?.event).toMatchObject({
      reactions: [{ emoji: 'thumbsup', count: 1 }]
    });
    expect(timeline.getThreadEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 't1',
      eventId: 'r20',
      limit: 50
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('keeps a projection-updated thread root over an older in-flight query row', async () => {
    const fake = new FakeQueryClient();
    type ThreadPage = Awaited<ReturnType<RoomTimelineAPI['getThreadEvents']>>;
    let resolveThread!: (value: ThreadPage) => void;
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(
        () =>
          new Promise<ThreadPage>((resolve) => {
            resolveThread = resolve;
          })
      )
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    const root = threadMessageEvent('t1');
    const updatedRoot = {
      ...root,
      event: { ...root.event, viewerIsFollowingThread: true }
    };
    store.events = [updatedRoot as never];
    resolveThread({
      events: [threadMessageEvent('t1') as never],
      startCursor: 'tl:t1',
      endCursor: 'tl:t1',
      hasOlder: false,
      hasNewer: false
    });
    await settle();

    expect(store.threadEvents[0].event).toMatchObject({ viewerIsFollowingThread: true });
    store.dispose();
  });

  it('keeps a changed thread root over an older in-flight refresh row', async () => {
    const fake = new FakeQueryClient();
    type ThreadPage = Awaited<ReturnType<RoomTimelineAPI['getThreadEventsAround']>>;
    let resolveRefresh!: (value: ThreadPage) => void;
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never],
        startCursor: 'tl:t1',
        endCursor: 'tl:t1',
        hasOlder: false,
        hasNewer: false
      })),
      getThreadEventsAround: vi.fn(
        () =>
          new Promise<ThreadPage>((resolve) => {
            resolveRefresh = resolve;
          })
      )
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    store.setThread('room-1', 't1');
    await settle();

    const refreshing = store.refreshCurrentWindow('t1');
    store.setThreadRootFollowState('t1', true);
    resolveRefresh({
      events: [threadMessageEvent('t1') as never],
      startCursor: 'tl:t1',
      endCursor: 'tl:t1',
      hasOlder: false,
      hasNewer: false
    });
    await refreshing;

    expect(store.threadEvents[0].event).toMatchObject({ viewerIsFollowingThread: true });
    store.dispose();
  });

  it('soft-refreshes a thread around the root anchor without jumping to latest replies', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [
          threadMessageEvent('t1') as never,
          threadMessageEvent('r18', 't1') as never,
          threadMessageEvent('r19', 't1') as never,
          threadMessageEvent('r20', 't1') as never
        ],
        startCursor: 'tl:cursor-18',
        endCursor: 'tl:cursor-20',
        hasOlder: true,
        hasNewer: false
      })),
      getThreadEventsAround: vi.fn(async () => ({
        events: [
          threadMessageEvent('t1') as never,
          threadMessageEvent('r1', 't1') as never,
          threadMessageEvent('r2', 't1') as never,
          threadMessageEvent('r3', 't1') as never
        ],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-3',
        hasOlder: false,
        hasNewer: true
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('t1');
    await settle();

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1', 'r2', 'r3']);
    expect(store.hasReachedStart).toBe(true);
    expect(timeline.getThreadEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 't1',
      eventId: 't1',
      limit: 50
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('replaces a disjoint latest thread refresh so older pagination can bridge gaps', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi
        .fn()
        .mockResolvedValueOnce({
          events: [
            threadMessageEvent('t1') as never,
            threadMessageEvent('r1', 't1') as never,
            threadMessageEvent('r2', 't1') as never
          ],
          startCursor: 'tl:cursor-1',
          endCursor: 'tl:cursor-2',
          hasOlder: false,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [
            threadMessageEvent('t1') as never,
            threadMessageEvent('r5', 't1') as never,
            threadMessageEvent('r6', 't1') as never
          ],
          startCursor: 'tl:cursor-5',
          endCursor: 'tl:cursor-6',
          hasOlder: true,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [
            threadMessageEvent('r3', 't1') as never,
            threadMessageEvent('r4', 't1') as never
          ],
          startCursor: 'tl:cursor-3',
          endCursor: 'tl:cursor-4',
          hasOlder: false,
          hasNewer: true
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();

    const result = await store.refreshCurrentWindow(null);
    await settle();

    expect(result).toMatchObject({ refreshed: true, changed: true });
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r5', 'r6']);
    expect(store.hasReachedStart).toBe(false);

    await store.loadMore();
    await settle();

    expect(timeline.getThreadEvents).toHaveBeenCalledTimes(3);
    expect(timeline.getThreadEvents).toHaveBeenLastCalledWith({
      roomId: 'room-1',
      threadRootEventId: 't1',
      limit: 50,
      before: 'tl:cursor-5'
    });
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r3', 'r4', 'r5', 'r6']);
    expect(store.hasReachedStart).toBe(true);
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('dispose() is idempotent', () => {
    const fake = new FakeQueryClient();
    const store = new MessagesStore(
      fake as unknown as ServerConnection,
      () => null,
      timelineFromFixtures(fake)
    );
    store.dispose();
    expect(() => store.dispose()).not.toThrow();
  });
});

describe('MessagesStore — thread lifecycle ownership', () => {
  it('loads thread history through the injected timeline API', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never, threadMessageEvent('r1', 't1') as never],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();

    expect(timeline.getThreadEvents).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 't1',
      limit: 50
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1']);
    store.dispose();
  });

  it('soft-refreshes a thread around an anchor through the injected timeline API', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never, threadMessageEvent('r18', 't1') as never],
        startCursor: 'tl:cursor-18',
        endCursor: 'tl:cursor-18',
        hasOlder: true,
        hasNewer: true
      })),
      getThreadEventsAround: vi.fn(async () => ({
        events: [
          threadMessageEvent('t1') as never,
          threadMessageEvent('r19', 't1') as never,
          threadMessageWithReaction('r20', 't1', 'thumbsup') as never,
          threadMessageEvent('r21', 't1') as never
        ],
        startCursor: 'tl:cursor-19',
        endCursor: 'tl:cursor-21',
        hasOlder: true,
        hasNewer: true
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('r20');
    await settle();

    expect(timeline.getThreadEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 't1',
      eventId: 'r20',
      limit: 50
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r18', 'r19', 'r20', 'r21']);
    expect(store.threadEvents.find((event) => event.id === 'r20')?.event).toMatchObject({
      reactions: [{ emoji: 'thumbsup', count: 1 }]
    });
    store.dispose();
  });

  it('does not refetch or clear events when setThread is called for the current thread', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never, threadMessageEvent('r1', 't1') as never],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();
    vi.mocked(timeline.getThreadEvents).mockClear();

    store.setThread('room-1', 't1');
    await settle();

    expect(timeline.getThreadEvents).not.toHaveBeenCalled();
    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('ingests a returned thread reply immediately and dedupes later subscription delivery', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    const returnedReply = threadMessageEvent('r1', 't1');

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent(returnedReply as never);
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1']);

    store.ingestEvent(returnedReply as never);
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1']);
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('ignores returned thread replies outside the active thread scope', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);
    const otherThreadReply = threadMessageEvent('r-other-thread', 'other-thread');
    const otherRoomReplyBase = threadMessageEvent('r-other-room', 't1');
    const otherRoomReply = {
      ...otherRoomReplyBase,
      event: {
        ...otherRoomReplyBase.event,
        roomId: 'room-2'
      }
    };

    store.setThread('room-1', 't1');
    await settle();

    store.ingestEvent(otherThreadReply as never);
    store.ingestEvent(otherRoomReply as never);

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1']);
    store.dispose();
  });

  it('links and unlinks visible echoes for thread replies from live events', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi.fn(async () => ({
        events: [threadMessageEvent('t1') as never, threadMessageEvent('reply1', 't1') as never],
        startCursor: 'tl:cursor-1',
        endCursor: 'tl:cursor-1',
        hasOlder: false,
        hasNewer: false
      }))
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestEvent({
      id: 'echo1',
      createdAt: '2026-05-27T00:00:02Z',
      actorId: 'u1',
      actor: null,
      event: {
        ...threadMessageEvent('echo1').event,
        echoOfEventId: 'reply1',
        echoFromThreadRootEventId: 't1'
      }
    } as never);

    expect(store.threadEvents.find((event) => event.id === 'reply1')?.event).toMatchObject({
      channelEchoEventId: 'echo1'
    });
    expect(store.refreshAnchorForMessageMutation('echo1')).toBe('reply1');

    store.ingestEvent({
      id: 'retract-echo1',
      createdAt: '2026-05-27T00:00:03Z',
      actorId: 'u1',
      actor: null,
      event: {
        kind: RoomEventKind.MessageRetracted,
        roomId: 'room-1',
        messageEventId: 'echo1',
        retractedReason: null
      }
    } as never);

    expect(store.threadEvents.find((event) => event.id === 'reply1')?.event).toMatchObject({
      channelEchoEventId: null
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('loads older reply pages when the first thread page is not complete', async () => {
    const fake = new FakeQueryClient();
    const timeline = fakeTimelineAPI({
      getThreadEvents: vi
        .fn()
        .mockResolvedValueOnce({
          events: [
            threadMessageEvent('t1') as never,
            threadMessageEvent('r51', 't1') as never,
            threadMessageEvent('r52', 't1') as never
          ],
          startCursor: 'tl:cursor-51',
          endCursor: 'tl:cursor-52',
          hasOlder: true,
          hasNewer: false
        })
        .mockResolvedValueOnce({
          events: [
            threadMessageEvent('r49', 't1') as never,
            threadMessageEvent('r50', 't1') as never
          ],
          startCursor: 'tl:cursor-49',
          endCursor: 'tl:cursor-50',
          hasOlder: false,
          hasNewer: true
        })
    });
    const store = new MessagesStore(fake as unknown as ServerConnection, () => null, timeline);

    store.setThread('room-1', 't1');
    await settle();

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r51', 'r52']);
    expect(store.hasReachedStart).toBe(false);

    await store.loadMore();
    await settle();

    expect(timeline.getThreadEvents).toHaveBeenCalledTimes(2);
    expect(timeline.getThreadEvents).toHaveBeenLastCalledWith({
      roomId: 'room-1',
      threadRootEventId: 't1',
      limit: 50,
      before: 'tl:cursor-51'
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r49', 'r50', 'r51', 'r52']);
    expect(store.hasReachedStart).toBe(true);

    store.dispose();
  });
});
