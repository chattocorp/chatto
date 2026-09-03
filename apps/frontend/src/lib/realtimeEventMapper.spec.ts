import { describe, expect, it } from 'vitest';
import { UserTypingSignalEvent } from '@chatto/api-types/realtime/v1/transient_events_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { TransientEventKind } from '$lib/realtimeEvents';
import { realtimeEventToEventEnvelope } from './realtimeEventMapper';

describe('realtime event mapping', () => {
  it('maps a public transient signal to the legacy local event bus', () => {
    const frame = new RealtimeEvent({
      id: 'event-1',
      actorId: 'user-1',
      event: {
        case: 'userTypingSignal',
        value: new UserTypingSignalEvent({ roomId: 'room-1' })
      }
    });

    expect(realtimeEventToEventEnvelope(frame)).toMatchObject({
      id: 'event-1',
      actorId: 'user-1',
      event: { kind: TransientEventKind.UserTyping, roomId: 'room-1' }
    });
  });
});
