import { Timestamp } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { UserTypingEvent } from '@chatto/api-types/core/live/v1/live_events_pb';
import { PublicEvent, RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { TransientEventKind } from '$lib/realtimeEvents';
import {
  publicEventToCanonicalEvent,
  realtimeEventToEventEnvelope
} from './realtimeEventMapper';

describe('realtime event mapping', () => {
  it('keeps public metadata and the canonical payload for local reducers', () => {
    const payload = new UserTypingEvent({ roomId: 'room-1' });
    const source = new PublicEvent({
      id: 'event-1',
      createdAt: Timestamp.fromDate(new Date('2026-09-03T10:00:00Z')),
      actorId: 'user-1',
      event: { case: 'userTypingSignal', value: payload }
    });

    const canonical = publicEventToCanonicalEvent(source);

    expect(canonical.id).toBe(source.id);
    expect(canonical.createdAt).toBe(source.createdAt);
    expect(canonical.actorId).toBe(source.actorId);
    expect(canonical.event.case).toBe('userTypingSignal');
    expect(canonical.event.value).toBe(payload);
  });

  it('maps a public transient signal to the legacy local event bus', () => {
    const frame = new RealtimeEvent({
      event: new PublicEvent({
        id: 'event-1',
        actorId: 'user-1',
        event: {
          case: 'userTypingSignal',
          value: new UserTypingEvent({ roomId: 'room-1' })
        }
      })
    });

    expect(realtimeEventToEventEnvelope(frame)).toMatchObject({
      id: 'event-1',
      actorId: 'user-1',
      event: { kind: TransientEventKind.UserTyping, roomId: 'room-1' }
    });
  });
});
