import { describe, expect, it } from 'vitest';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { isRootRoomEvent, isThreadEvent } from './filters';

function event(payload: TimelineEventView['event'], id = 'event-1'): TimelineEventView {
  return {
    id,
    createdAt: '2026-06-01T12:00:00.000Z',
    actorId: 'u1',
    actor: null,
    event: payload
  };
}

function messagePayload(overrides: Record<string, unknown> = {}): TimelineEventView['event'] {
  return {
    kind: TimelineEventKind.MessagePosted,
    roomId: 'room-1',
    body: 'hello',
    attachments: [],
    linkPreview: null,
    reactions: [],
    updatedAt: null,
    inReplyTo: null,
    threadRootEventId: null,
    echoOfEventId: null,
    echoFromThreadRootEventId: null,
    channelEchoEventId: null,
    replyCount: 0,
    lastReplyAt: null,
    threadParticipants: [],
    viewerIsFollowingThread: null,
    ...overrides
  } as TimelineEventView['event'];
}

describe('room message event filters', () => {
  it('uses local event kind for Connect-mapped root messages', () => {
    expect(
      isRootRoomEvent(
        event(
          messagePayload({
            kind: TimelineEventKind.MessagePosted
          })
        )
      )
    ).toBe(true);
  });

  it.each([TimelineEventKind.CallStarted, TimelineEventKind.CallEnded])(
    'keeps %s in the room timeline',
    (kind) => {
      expect(isRootRoomEvent(event({ kind, roomId: 'room-1', callId: 'call-1' } as never))).toBe(
        true
      );
    }
  );

  it('uses local event kind for room lifecycle events', () => {
    expect(
      isRootRoomEvent(
        event({
          kind: TimelineEventKind.RoomUpdated,
          roomId: 'room-1'
        } as never)
      )
    ).toBe(true);
  });

  it('ignores payloads without a local event kind', () => {
    expect(isRootRoomEvent(event({ roomId: 'room-1' } as never))).toBe(false);
  });

  it('uses local event kind for thread messages', () => {
    expect(
      isThreadEvent(
        event(
          messagePayload({
            kind: TimelineEventKind.MessagePosted,
            threadRootEventId: 'root-1'
          }),
          'reply-1'
        ),
        'room-1',
        'root-1'
      )
    ).toBe(true);
  });
});
