import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { RoomTimelineAPI } from '$lib/api-client/roomTimeline';
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
    resolveMessageLinkTarget: vi.fn(async () => ({
      event: null,
      threadRootEventId: null
    })),
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
    async resolveMessageLinkTarget(input) {
      const data = await resolveFakeResult(fake, 'timeline:message-link', input, {
        requestPolicy: 'network-only'
      });
      return {
        event: (data.room?.event as never) ?? null,
        threadRootEventId: null
      };
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

    store.ingestServerEvent({
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

    store.ingestServerEvent({
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
      attachments: []
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

    store.ingestServerEvent(callEvent(RoomEventKind.CallStarted, 'call-started') as never);
    store.ingestServerEvent(callEvent(RoomEventKind.CallParticipantJoined, 'call-joined') as never);
    store.ingestServerEvent(callEvent(RoomEventKind.CallParticipantLeft, 'call-left') as never);
    store.ingestServerEvent(callEvent(RoomEventKind.CallEnded, 'call-ended') as never);

    expect(store.rootEvents).toEqual([]);
    expect(fake.queryMock).not.toHaveBeenCalled();
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

    store.ingestServerEvent({
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

    store.ingestServerEvent({
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

    store.ingestServerEvent({
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

    store.ingestServerEvent({
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
      attachments: []
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

    store.ingestServerEvent(returnedPost as never);
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
    store.ingestServerEvent(returnedReply as never);

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
    store.ingestServerEvent(threadMessageEvent('m8') as never);
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

    store.ingestServerEvent(returnedReply as never);
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

    store.ingestServerEvent({
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

    store.ingestServerEvent({
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
