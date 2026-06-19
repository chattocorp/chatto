import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import { Timestamp } from '@bufbuild/protobuf';
import { MessagesStore } from './messages.svelte';
import { JumpToMessageState } from './composerContext.svelte';
import { StreamEvent } from '$lib/pb/chatto/wire/v1/protocol_pb';
import { Event as DurableEvent } from '$lib/pb/chatto/core/v1/event_pb';
import {
  CallEventView,
  GetRoomEventResponse,
  GetRoomTimelineAfterResponse,
  GetRoomTimelineAroundResponse,
  GetRoomTimelineResponse,
  GetThreadEventsAroundResponse,
  GetThreadEventsResponse,
  MessageEditedView,
  MessagePostedView,
  MessageRetractedView,
  ReactionSummaryView,
  RoomEventPayload,
  RoomEventsPage,
  RoomEventView,
  RoomScopedEventView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';
import {
  MessagePostedEvent,
  MessageRetractedEvent
} from '$lib/pb/chatto/core/v1/message_events_pb';

type QueryResult = { data: Record<string, unknown> | null; error: unknown };
type QueryFn = (
  operation: string,
  variables: Record<string, unknown>,
  options?: unknown
) => Promise<QueryResult>;

class FakeWireClient {
  queryMock: ReturnType<typeof vi.fn>;
  #queryDataQueue: unknown[] | null;
  #queryData: unknown;

  constructor(queryData: unknown = null) {
    this.#queryData = queryData;
    this.#queryDataQueue = Array.isArray(queryData) ? [...queryData] : null;
    this.queryMock = vi.fn(
      async (_operation: string, _variables: Record<string, unknown>, _options?: unknown) => {
        const data =
          this.#queryDataQueue === null
            ? this.#queryData
            : this.#queryDataQueue.length > 1
              ? this.#queryDataQueue.shift()
              : this.#queryDataQueue[0];
        const resolvedData = await Promise.resolve(data);
        if (isOperationResult(resolvedData)) return resolvedData;
        return { data: resolvedData, error: null };
      }
    );
  }

  runQuery(operation: string, variables: Record<string, unknown>, options?: unknown) {
    return (this.queryMock as unknown as QueryFn)(operation, variables, options);
  }

  async getRoomTimeline(request: { roomId: string; limit: number; beforeSequence: bigint }) {
    const variables: Record<string, unknown> = { roomId: request.roomId, limit: request.limit };
    const before = sequenceToTestCursor(request.beforeSequence);
    if (before) variables.before = before;
    const result = await this.runQuery(
      'GetRoomTimeline',
      variables,
      before ? undefined : { requestPolicy: 'network-only' }
    );
    if (result.error) throw result.error;
    const page = asRecord(asRecord(result.data)?.room)?.events ?? null;
    return roomTimelineResponseFromPage(page);
  }

  async getRoomTimelineAfter(request: { roomId: string; limit: number; afterSequence: bigint }) {
    const variables: Record<string, unknown> = { roomId: request.roomId, limit: request.limit };
    const after = sequenceToTestCursor(request.afterSequence);
    if (after) variables.after = after;
    const result = await this.runQuery('GetRoomTimelineAfter', variables);
    if (result.error) throw result.error;
    return new GetRoomTimelineAfterResponse({
      page: roomEventsPageFromConnection(asRecord(asRecord(result.data)?.room)?.events ?? null)
    });
  }

  async getRoomTimelineAround(request: { roomId: string; eventId: string; limit: number }) {
    const result = await this.runQuery(
      'GetRoomTimelineAround',
      {
        roomId: request.roomId,
        eventId: request.eventId,
        limit: request.limit
      },
      { requestPolicy: 'network-only' }
    );
    if (result.error) throw result.error;
    return new GetRoomTimelineAroundResponse({
      page: roomEventsPageFromConnection(asRecord(asRecord(result.data)?.room)?.eventsAround ?? null)
    });
  }

  async getRoomEvent(request: { roomId: string; eventId: string }) {
    const result = await this.runQuery(
      'GetRoomEvent',
      { roomId: request.roomId, eventId: request.eventId },
      { requestPolicy: 'network-only' }
    );
    if (result.error) throw result.error;
    return new GetRoomEventResponse({
      event: eventFragmentToWire(asRecord(asRecord(result.data)?.room)?.event ?? null)
    });
  }

  async getThreadEvents(request: {
    roomId: string;
    threadRootEventId: string;
    limit: number;
    beforeSequence: bigint;
    afterSequence: bigint;
  }) {
    const variables: Record<string, unknown> = {
      roomId: request.roomId,
      threadRootEventId: request.threadRootEventId,
      limit: request.limit
    };
    const before = sequenceToTestCursor(request.beforeSequence);
    if (before) variables.before = before;
    const after = sequenceToTestCursor(request.afterSequence);
    if (after) variables.after = after;
    const result = await this.runQuery(
      'GetThreadEvents',
      variables,
      before || after ? undefined : { requestPolicy: 'network-only' }
    );
    if (result.error) throw result.error;
    return threadEventsResponseFromRoot(asRecord(asRecord(result.data)?.room)?.event ?? null);
  }

  async getThreadEventsAround(request: {
    roomId: string;
    threadRootEventId: string;
    anchorEventId: string;
    limit: number;
  }) {
    const result = await this.runQuery(
      'GetThreadEventsAround',
      {
        roomId: request.roomId,
        threadRootEventId: request.threadRootEventId,
        anchorEventId: request.anchorEventId,
        limit: request.limit
      },
      { requestPolicy: 'network-only' }
    );
    if (result.error) throw result.error;
    const response = threadEventsResponseFromRoot(asRecord(asRecord(result.data)?.room)?.event ?? null);
    return new GetThreadEventsAroundResponse({
      rootEvent: response.rootEvent,
      replies: response.replies
    });
  }
}

function isOperationResult(value: unknown): value is QueryResult {
  return typeof value === 'object' && value !== null && ('data' in value || 'error' in value);
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : null;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function optionalString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

function numberValue(value: unknown): number {
  return typeof value === 'number' ? value : 0;
}

function booleanOrUndefined(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function timestampFrom(value: unknown): Timestamp | undefined {
  if (typeof value !== 'string' || value === '') return undefined;
  return Timestamp.fromDate(new Date(value));
}

function sequenceToTestCursor(sequence: unknown): string | undefined {
  if (sequence === undefined || sequence === null || sequence === '') return undefined;
  try {
    const value = BigInt(sequence as bigint | number | string);
    return value > 0n ? `seq:${value}` : undefined;
  } catch {
    return undefined;
  }
}

function testCursorToSequence(cursor: unknown): bigint {
  if (typeof cursor !== 'string') return 0n;
  const value = cursor.startsWith('seq:') ? cursor.slice(4) : cursor;
  return /^\d+$/.test(value) ? BigInt(value) : 0n;
}

function roomTimelineResponseFromPage(value: unknown): GetRoomTimelineResponse {
  const page = asRecord(value);
  return new GetRoomTimelineResponse({
    eventViews: arrayValue(page?.events).map(eventFragmentToWire).filter(isWireEvent),
    startSequence: testCursorToSequence(page?.startCursor),
    endSequence: testCursorToSequence(page?.endCursor),
    hasOlder: Boolean(page?.hasOlder),
    hasNewer: Boolean(page?.hasNewer)
  });
}

function roomEventsPageFromConnection(value: unknown): RoomEventsPage {
  const page = asRecord(value);
  return new RoomEventsPage({
    events: arrayValue(page?.events).map(eventFragmentToWire).filter(isWireEvent),
    startSequence: testCursorToSequence(page?.startCursor),
    endSequence: testCursorToSequence(page?.endCursor),
    hasOlder: Boolean(page?.hasOlder),
    hasNewer: Boolean(page?.hasNewer),
    targetIndex: numberValue(page?.targetIndex)
  });
}

function threadEventsResponseFromRoot(value: unknown): GetThreadEventsResponse {
  const root = asRecord(value);
  const payload = asRecord(root?.event);
  return new GetThreadEventsResponse({
    rootEvent: eventFragmentToWire(root),
    replies: roomEventsPageFromConnection(payload?.threadReplies ?? null)
  });
}

function eventFragmentToWire(value: unknown): RoomEventView | undefined {
  const event = asRecord(value);
  const payload = asRecord(event?.event);
  const wirePayload: RoomEventPayload['payload'] = payload
    ? eventPayloadToWire(payload)
    : { case: undefined };
  return new RoomEventView({
    id: stringValue(event?.id),
    createdAt: timestampFrom(event?.createdAt),
    actorId: stringValue(event?.actorId),
    actor: userFragmentToWire(event?.actor),
    event: new RoomEventPayload({ payload: wirePayload })
  });
}

function eventPayloadToWire(payload: Record<string, unknown>): RoomEventPayload['payload'] {
  switch (payload.__typename) {
    case 'MessagePostedEvent':
      return {
        case: 'messagePosted',
        value: new MessagePostedView({
          roomId: stringValue(payload.roomId),
          body: optionalString(payload.body),
          attachments: [],
          linkPreview: undefined,
          updatedAt: timestampFrom(payload.updatedAt),
          inReplyTo: optionalString(payload.inReplyTo),
          threadRootEventId: optionalString(payload.threadRootEventId),
          echoOfEventId: optionalString(payload.echoOfEventId),
          echoFromThreadRootEventId: optionalString(payload.echoFromThreadRootEventId),
          channelEchoEventId: optionalString(payload.channelEchoEventId),
          replyCount: numberValue(payload.replyCount),
          lastReplyAt: timestampFrom(payload.lastReplyAt),
          threadParticipants: arrayValue(payload.threadParticipants).map(userFragmentToWire).filter(isUser),
          viewerIsFollowingThread: booleanOrUndefined(payload.viewerIsFollowingThread),
          reactions: arrayValue(payload.reactions).map(reactionToWire)
        })
      };
    case 'MessageEditedEvent':
      return {
        case: 'messageEdited',
        value: new MessageEditedView({
          roomId: stringValue(payload.roomId),
          messageEventId: stringValue(payload.messageEventId),
          body: optionalString(payload.body),
          attachments: [],
          linkPreview: undefined,
          updatedAt: timestampFrom(payload.updatedAt)
        })
      };
    case 'MessageRetractedEvent':
      return {
        case: 'messageRetracted',
        value: new MessageRetractedView({
          roomId: stringValue(payload.roomId),
          messageEventId: stringValue(payload.messageEventId),
          reason: optionalString(payload.retractedReason)
        })
      };
    case 'CallStartedEvent':
      return { case: 'callStarted', value: callEventToWire(payload) };
    case 'CallParticipantJoinedEvent':
      return { case: 'callParticipantJoined', value: callEventToWire(payload) };
    case 'CallParticipantLeftEvent':
      return { case: 'callParticipantLeft', value: callEventToWire(payload) };
    case 'CallEndedEvent':
      return { case: 'callEnded', value: callEventToWire(payload) };
    case 'RoomUpdatedEvent':
      return { case: 'roomUpdated', value: new RoomScopedEventView({ roomId: stringValue(payload.roomId) }) };
    case 'RoomDeletedEvent':
      return { case: 'roomDeleted', value: new RoomScopedEventView({ roomId: stringValue(payload.roomId) }) };
    case 'RoomArchivedEvent':
      return { case: 'roomArchived', value: new RoomScopedEventView({ roomId: stringValue(payload.roomId) }) };
    case 'RoomUnarchivedEvent':
      return { case: 'roomUnarchived', value: new RoomScopedEventView({ roomId: stringValue(payload.roomId) }) };
    default:
      return { case: undefined };
  }
}

function reactionToWire(value: unknown): ReactionSummaryView {
  const reaction = asRecord(value);
  return new ReactionSummaryView({
    emoji: stringValue(reaction?.emoji),
    count: numberValue(reaction?.count),
    hasReacted: Boolean(reaction?.hasReacted),
    users: arrayValue(reaction?.users).map(userFragmentToWire).filter(isUser)
  });
}

function callEventToWire(value: Record<string, unknown>): CallEventView {
  return new CallEventView({
    roomId: stringValue(value.roomId),
    callId: stringValue(value.callId)
  });
}

function userFragmentToWire(value: unknown): User | undefined {
  const user = asRecord(value);
  if (!user) return undefined;
  return new User({
    id: stringValue(user.id),
    login: stringValue(user.login),
    displayName: stringValue(user.displayName)
  });
}

function isWireEvent(value: RoomEventView | undefined): value is RoomEventView {
  return value !== undefined;
}

function isUser(value: User | undefined): value is User {
  return value !== undefined;
}

async function settle() {
  for (let i = 0; i < 8; i++) {
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
      __typename: 'MessagePostedEvent',
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
          __typename: 'ReactionSummary',
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
          __typename: 'ReactionSummary',
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
  typename:
    | 'CallStartedEvent'
    | 'CallEndedEvent'
    | 'CallParticipantJoinedEvent'
    | 'CallParticipantLeftEvent',
  id: string,
  roomId = 'room-1'
) {
  return {
    id,
    createdAt: '2026-05-27T00:00:01Z',
    actorId: 'u1',
    actor: null,
    event: {
      __typename: typename,
      roomId,
      callId: 'call-1'
    }
  };
}

function durableWireEvent(id: string, event: DurableEvent['event'], actorId = 'u1'): StreamEvent {
  return new StreamEvent({
    eventId: id,
    payload: {
      case: 'durableEvent',
      value: new DurableEvent({
        id,
        actorId,
        event
      })
    }
  });
}

function wireMessagePostedEvent(
  id: string,
  options: {
    roomId?: string;
    threadRootEventId?: string | null;
    echoOfEventId?: string;
    echoFromThreadRootEventId?: string;
  } = {}
): StreamEvent {
  return durableWireEvent(id, {
    case: 'messagePosted',
    value: new MessagePostedEvent({
      roomId: options.roomId ?? 'room-1',
      inThread: options.threadRootEventId ?? '',
      echoOfEventId: options.echoOfEventId ?? '',
      echoFromThreadRootEventId: options.echoFromThreadRootEventId ?? ''
    })
  });
}

function wireMessageRetractedEvent(id: string, messageEventId: string): StreamEvent {
  return durableWireEvent(id, {
    case: 'messageRetracted',
    value: new MessageRetractedEvent({
      roomId: 'room-1',
      eventId: messageEventId
    })
  });
}

function threadQueryResult({
  replies,
  startCursor,
  endCursor,
  hasOlder,
  hasNewer
}: {
  replies: unknown[];
  startCursor: string | null;
  endCursor: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
}) {
  return {
    room: {
      event: {
        ...threadMessageEvent('t1'),
        event: {
          ...threadMessageEvent('t1').event,
          threadReplies: {
            events: replies,
            startCursor,
            endCursor,
            hasOlder,
            hasNewer
          }
        }
      }
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

function wireRoomMessageEvent(id: string, body: string, sequence: bigint): RoomEventView {
  return new RoomEventView({
    id,
    createdAt: Timestamp.fromDate(new Date(Date.UTC(2026, 4, 27, 0, 0, Number(sequence)))),
    actorId: 'u1',
    sequence,
    event: new RoomEventPayload({
      payload: {
        case: 'messagePosted',
        value: new MessagePostedView({
          roomId: 'room-1',
          body
        })
      }
    })
  });
}

describe('MessagesStore — room lifecycle ownership', () => {
  it('loads the room timeline through the protobuf wire client when available', async () => {
    const fake = new FakeWireClient(
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const getRoomTimeline = vi.fn(async () => {
      return new GetRoomTimelineResponse({
        eventViews: [wireRoomMessageEvent('wire-1', 'from wire', 41n)],
        startSequence: 41n,
        endSequence: 41n,
        hasOlder: false,
        hasNewer: false
      });
    });
    const store = new MessagesStore(() => null, {
      wireClient: { getRoomTimeline } as never
    });

    store.setRoom('room-1');
    await settle();

    expect(getRoomTimeline).toHaveBeenCalledOnce();
    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.rootEvents.map((event) => event.id)).toEqual(['wire-1']);
    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      body: 'from wire'
    });
    store.dispose();
  });

  it('does not refetch or clear events when setRoom is called for the current room', async () => {
    const loaded = threadMessageEvent('m1');
    const fake = new FakeWireClient(
      roomEventsResult({
        events: [loaded],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(() => null, { wireClient: fake });

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

  it('serves already-loaded events by id without refetching', async () => {
    const loaded = threadMessageEvent('m1');
    const fake = new FakeWireClient(
      roomEventsResult({
        events: [loaded],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(() => null, { wireClient: fake });

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
    const fake = new FakeWireClient([
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: target } }
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

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
    const fake = new FakeWireClient([
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
    const store = new MessagesStore(() => null, { wireClient: fake });

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
    const fake = new FakeWireClient({
      room: {
        events: {
          events: [
            {
              id: 'm1',
              createdAt: '2026-05-27T00:00:00Z',
              actorId: 'u1',
              actor: null,
              event: {
                __typename: 'MessagePostedEvent',
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
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestServerEvent({
      id: 'edit-1',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u1',
      actor: null,
      event: {
        __typename: 'MessageEditedEvent',
        roomId: 'room-1',
        messageEventId: 'm1',
        body: 'after',
        attachments: [],
        linkPreview: null,
        updatedAt: '2026-05-27T00:00:01Z'
      }
    } as never);

    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      body: 'after',
      updatedAt: '2026-05-27T00:00:01Z'
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('applies MessageRetractedEvent payloads inline without refetching', async () => {
    const fake = new FakeWireClient({
      room: {
        events: {
          events: [
            {
              id: 'm1',
              createdAt: '2026-05-27T00:00:00Z',
              actorId: 'u1',
              actor: null,
              event: {
                __typename: 'MessagePostedEvent',
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
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestServerEvent({
      id: 'retract-1',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u1',
      actor: null,
      event: {
        __typename: 'MessageRetractedEvent',
        roomId: 'room-1',
        messageEventId: 'm1',
        retractedReason: null
      }
    } as never);

    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      body: null,
      attachments: []
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('uses protobuf MessagePostedEvent as live signal and fetches the rich wire row', async () => {
    const posted = threadMessageEvent('m1');
    const fake = new FakeWireClient([
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: posted } }
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.ingestWireEvent(wireMessagePostedEvent('m1'));
    await settle();

    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({ roomId: 'room-1', eventId: 'm1' });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1']);
    store.dispose();
  });

  it('applies protobuf MessageRetractedEvent payloads inline without refetching', async () => {
    const fake = new FakeWireClient(
      roomEventsResult({
        events: [threadMessageEvent('m1')],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.ingestWireEvent(wireMessageRetractedEvent('retract-1', 'm1'));

    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      body: null,
      attachments: []
    });
    store.dispose();
  });

  it('treats call start and end as root timeline system events', async () => {
    const fake = new FakeWireClient(
      roomEventsResult({
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestServerEvent(callEvent('CallStartedEvent', 'call-started') as never);
    store.ingestServerEvent(callEvent('CallParticipantJoinedEvent', 'call-joined') as never);
    store.ingestServerEvent(callEvent('CallParticipantLeftEvent', 'call-left') as never);
    store.ingestServerEvent(callEvent('CallEndedEvent', 'call-ended') as never);

    expect(store.rootEvents.map((event) => event.id)).toEqual(['call-started', 'call-ended']);
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('refetches a loaded message when a replayed reaction event arrives', async () => {
    const fake = new FakeWireClient([
      roomEventsResult({
        events: [threadMessageEvent('m1')],
        startCursor: 'seq:1',
        endCursor: 'seq:1',
        hasOlder: false,
        hasNewer: false
      }),
      { room: { event: messageWithReaction('m1', 'heart') } }
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestServerEvent({
      id: 'reaction-1',
      createdAt: '2026-05-27T00:00:01Z',
      actorId: 'u2',
      actor: null,
      event: {
        __typename: 'ReactionAddedEvent',
        roomId: 'room-1',
        messageEventId: 'm1',
        emoji: 'heart'
      }
    } as never);
    await settle();

    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({ roomId: 'room-1', eventId: 'm1' });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      reactions: [{ emoji: 'heart', count: 1 }]
    });
    store.dispose();
  });

  it('hides only the echo when an echo is retracted', async () => {
    const fake = new FakeWireClient({
      room: {
        events: {
          events: [
            {
              id: 'root',
              createdAt: '2026-05-27T00:00:00Z',
              actorId: 'u1',
              actor: null,
              event: {
                __typename: 'MessagePostedEvent',
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
                __typename: 'MessagePostedEvent',
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
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestServerEvent({
      id: 'retract-echo',
      createdAt: '2026-05-27T00:00:02Z',
      actorId: 'u1',
      actor: null,
      event: {
        __typename: 'MessageRetractedEvent',
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
    const fake = new FakeWireClient({
      room: {
        events: {
          events: [
            {
              id: 'echo',
              createdAt: '2026-05-27T00:00:01Z',
              actorId: 'u1',
              actor: null,
              event: {
                __typename: 'MessagePostedEvent',
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
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    store.ingestServerEvent({
      id: 'retract-original',
      createdAt: '2026-05-27T00:00:02Z',
      actorId: 'u1',
      actor: null,
      event: {
        __typename: 'MessageRetractedEvent',
        roomId: 'room-1',
        messageEventId: 'reply',
        retractedReason: null
      }
    } as never);

    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      body: null,
      attachments: []
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('runs an initial fetch on setRoom', async () => {
    const fake = new FakeWireClient({
      room: { events: { events: [], hasOlder: false, hasNewer: false } }
    });
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();

    expect(fake.queryMock).toHaveBeenCalledTimes(1);
    store.dispose();
  });

  it('soft-refreshes the latest room window without entering initial loading', async () => {
    const fake = new FakeWireClient([
      roomEventsResult({
        events: [threadMessageEvent('m1')],
        startCursor: 'seq:1',
        endCursor: 'seq:1',
        hasOlder: false,
        hasNewer: false
      }),
      roomEventsResult({
        events: [messageWithReaction('m1', 'heart'), threadMessageEvent('m2')],
        startCursor: 'seq:1',
        endCursor: 'seq:2',
        hasOlder: false,
        hasNewer: false
      })
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow();
    await settle();

    expect(store.isInitialLoading).toBe(false);
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2']);
    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
      reactions: [{ emoji: 'heart', count: 1 }]
    });
    expect(fake.queryMock).toHaveBeenCalledOnce();
    expect(fake.queryMock.mock.calls[0][1]).toEqual({ roomId: 'room-1', limit: 50 });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    store.dispose();
  });

  it('soft-refreshes around an anchor event when one is provided', async () => {
    const fake = new FakeWireClient([
      roomEventsResult({
        events: [threadMessageEvent('m1'), threadMessageEvent('m2'), threadMessageEvent('m3')],
        startCursor: 'seq:1',
        endCursor: 'seq:3',
        hasOlder: false,
        hasNewer: false
      }),
      {
        room: {
          eventsAround: {
            events: [messageWithReaction('m2', 'thumbsup')],
            targetIndex: 0,
            startCursor: 'seq:2',
            endCursor: 'seq:2',
            hasOlder: true,
            hasNewer: true
          }
        }
      }
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setRoom('room-1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('m2');
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual(['m2']);
    expect(store.hasReachedStart).toBe(false);
    expect(store.rootEvents[0].event).toMatchObject({
      __typename: 'MessagePostedEvent',
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
    const fake = new FakeWireClient([
      roomEventsResult({
        events: [
          threadMessageEvent('m1'),
          threadMessageEvent('m2'),
          threadMessageEvent('m3'),
          threadMessageEvent('m4'),
          threadMessageEvent('m5')
        ],
        startCursor: 'seq:1',
        endCursor: 'seq:5',
        hasOlder: false,
        hasNewer: true
      }),
      anchoredRefresh,
      roomEventsResult({
        events: [threadMessageEvent('m6'), threadMessageEvent('m7')],
        startCursor: 'seq:6',
        endCursor: 'seq:7',
        hasOlder: true,
        hasNewer: true
      })
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

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
          startCursor: 'seq:3',
          endCursor: 'seq:5',
          hasOlder: true,
          hasNewer: true
        }
      }
    });

    await refresh;
    await settle();
    expect(store.rootEvents.map((event) => event.id)).toEqual(['m3', 'm4', 'm5', 'm8']);

    const jumpState = new JumpToMessageState();
    jumpState.isJumpedMode = true;
    await store.loadNewer(jumpState);
    await settle();

    expect(store.rootEvents.map((event) => event.id)).toEqual(['m3', 'm4', 'm5', 'm6', 'm7', 'm8']);
    store.dispose();
  });

  it('soft-refreshes a thread around an anchored reply', async () => {
    const fake = new FakeWireClient([
      threadQueryResult({
        replies: [
          threadMessageEvent('r18', 't1'),
          threadMessageEvent('r19', 't1'),
          threadMessageEvent('r20', 't1')
        ],
        startCursor: 'seq:18',
        endCursor: 'seq:20',
        hasOlder: true,
        hasNewer: true
      }),
      threadQueryResult({
        replies: [
          threadMessageEvent('r19', 't1'),
          threadMessageWithReaction('r20', 't1', 'thumbsup'),
          threadMessageEvent('r21', 't1')
        ],
        startCursor: 'seq:19',
        endCursor: 'seq:21',
        hasOlder: true,
        hasNewer: true
      })
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('r20');
    await settle();

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r19', 'r20', 'r21']);
    expect(store.hasReachedStart).toBe(false);
    expect(store.threadEvents.find((event) => event.id === 'r20')?.event).toMatchObject({
      __typename: 'MessagePostedEvent',
      reactions: [{ emoji: 'thumbsup', count: 1 }]
    });
    expect(fake.queryMock.mock.calls[0][1]).toEqual({
      roomId: 'room-1',
      threadRootEventId: 't1',
      anchorEventId: 'r20',
      limit: 50
    });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    store.dispose();
  });

  it('soft-refreshes a thread around the root anchor without jumping to latest replies', async () => {
    const fake = new FakeWireClient([
      threadQueryResult({
        replies: [
          threadMessageEvent('r18', 't1'),
          threadMessageEvent('r19', 't1'),
          threadMessageEvent('r20', 't1')
        ],
        startCursor: 'seq:18',
        endCursor: 'seq:20',
        hasOlder: true,
        hasNewer: false
      }),
      threadQueryResult({
        replies: [
          threadMessageEvent('r1', 't1'),
          threadMessageEvent('r2', 't1'),
          threadMessageEvent('r3', 't1')
        ],
        startCursor: 'seq:1',
        endCursor: 'seq:3',
        hasOlder: false,
        hasNewer: true
      })
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    await store.refreshCurrentWindow('t1');
    await settle();

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1', 'r2', 'r3']);
    expect(store.hasReachedStart).toBe(true);
    expect(fake.queryMock.mock.calls[0][1]).toEqual({
      roomId: 'room-1',
      threadRootEventId: 't1',
      anchorEventId: 't1',
      limit: 50
    });
    expect(fake.queryMock.mock.calls[0][2]).toEqual({ requestPolicy: 'network-only' });
    store.dispose();
  });

  it('dispose() is idempotent', () => {
    const fake = new FakeWireClient();
    const store = new MessagesStore(() => null, { wireClient: fake });
    store.dispose();
    expect(() => store.dispose()).not.toThrow();
  });
});

describe('MessagesStore — thread lifecycle ownership', () => {
  it('does not refetch or clear events when setThread is called for the current thread', async () => {
    const fake = new FakeWireClient(
      threadQueryResult({
        replies: [threadMessageEvent('r1', 't1')],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setThread('room-1', 't1');
    await settle();
    fake.queryMock.mockClear();

    store.setThread('room-1', 't1');
    await settle();

    expect(fake.queryMock).not.toHaveBeenCalled();
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r1']);
    expect(store.isInitialLoading).toBe(false);
    store.dispose();
  });

  it('links and unlinks visible echoes for thread replies from live events', async () => {
    const fake = new FakeWireClient(
      threadQueryResult({
        replies: [threadMessageEvent('reply1', 't1')],
        startCursor: 'seq:1',
        endCursor: 'seq:1',
        hasOlder: false,
        hasNewer: false
      })
    );
    const store = new MessagesStore(() => null, { wireClient: fake });

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
      __typename: 'MessagePostedEvent',
      channelEchoEventId: 'echo1'
    });

    store.ingestServerEvent({
      id: 'retract-echo1',
      createdAt: '2026-05-27T00:00:03Z',
      actorId: 'u1',
      actor: null,
      event: {
        __typename: 'MessageRetractedEvent',
        roomId: 'room-1',
        messageEventId: 'echo1',
        retractedReason: null
      }
    } as never);

    expect(store.threadEvents.find((event) => event.id === 'reply1')?.event).toMatchObject({
      __typename: 'MessagePostedEvent',
      channelEchoEventId: null
    });
    expect(fake.queryMock).not.toHaveBeenCalled();
    store.dispose();
  });

  it('loads older reply pages when the first thread page is not complete', async () => {
    const fake = new FakeWireClient([
      threadQueryResult({
        replies: [threadMessageEvent('r51', 't1'), threadMessageEvent('r52', 't1')],
        startCursor: 'seq:51',
        endCursor: 'seq:52',
        hasOlder: true,
        hasNewer: false
      }),
      threadQueryResult({
        replies: [threadMessageEvent('r49', 't1'), threadMessageEvent('r50', 't1')],
        startCursor: 'seq:49',
        endCursor: 'seq:50',
        hasOlder: false,
        hasNewer: true
      })
    ]);
    const store = new MessagesStore(() => null, { wireClient: fake });

    store.setThread('room-1', 't1');
    await settle();

    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r51', 'r52']);
    expect(store.hasReachedStart).toBe(false);

    await store.loadMore();
    await settle();

    expect(fake.queryMock).toHaveBeenCalledTimes(2);
    expect(fake.queryMock.mock.calls[1][1]).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 't1',
      limit: 50,
      before: 'seq:51'
    });
    expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r49', 'r50', 'r51', 'r52']);
    expect(store.hasReachedStart).toBe(true);

    store.dispose();
  });
});
