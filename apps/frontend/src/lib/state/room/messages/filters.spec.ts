import { describe, expect, it } from 'vitest';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { RoomEventView } from '$lib/render/types';
import { isRootRoomEvent, isThreadEvent } from './filters';

function event(payload: RoomEventView['event'], id = 'event-1'): RoomEventView {
  return {
    id,
    createdAt: '2026-06-01T12:00:00.000Z',
    actorId: 'u1',
    actor: null,
    event: payload
  };
}

function messagePayload(overrides: Record<string, unknown> = {}): RoomEventView['event'] {
  return {
    kind: RoomEventKind.MessagePosted,
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
  } as RoomEventView['event'];
}

describe('room message event filters', () => {
  it('uses local event kind for Connect-mapped root messages', () => {
    expect(
      isRootRoomEvent(
        event(
          messagePayload({
            kind: RoomEventKind.MessagePosted
          })
        )
      )
    ).toBe(true);
  });

  it('uses local event kind for room lifecycle events', () => {
    expect(
      isRootRoomEvent(
        event({
          kind: RoomEventKind.RoomUpdated,
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
            kind: RoomEventKind.MessagePosted,
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
