import { Event } from '@chatto/api-types/core/evt/v1/event_pb';
import {
  type PublicEvent,
  RealtimeEvent
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TransientEventKind, type TransientEventEnvelope } from '$lib/realtimeEvents';

function timestampToISO(value: { toDate(): Date } | undefined): string {
  return value?.toDate().toISOString() ?? new Date().toISOString();
}

export function realtimeEventToEventEnvelope(frame: RealtimeEvent): TransientEventEnvelope | null {
  const source = frame.event;
  if (!source) return null;
  const base = {
    id: source.id,
    createdAt: timestampToISO(source.createdAt),
    actorId: source.actorId || null
  };

  switch (source.event.case) {
    case 'userTypingSignal': {
      const value = source.event.value;
      return {
        ...base,
        event: {
          kind: TransientEventKind.UserTyping,
          roomId: value.roomId,
          typingThreadRootEventId: value.threadRootEventId ?? null
        }
      };
    }
    case 'presenceChangedSignal':
      return {
        ...base,
        event: {
          kind: TransientEventKind.PresenceChanged,
          status: presenceStatusFromSignal(source.event.value.status)
        }
      };
    case 'sessionTerminatedSignal':
      return {
        ...base,
        event: {
          kind: TransientEventKind.SessionTerminated,
          reason: source.event.value.reason
        }
      };
    default:
      return null;
  }
}

/** Convert the public wire union to the canonical shape used by local reducers. */
export function publicEventToCanonicalEvent(source: PublicEvent): Event {
  return new Event({
    id: source.id,
    createdAt: source.createdAt,
    actorId: source.actorId,
    event: source.event
  });
}

function presenceStatusFromSignal(status: string): PresenceStatus {
  switch (status) {
    case 'ONLINE':
      return PresenceStatus.ONLINE;
    case 'AWAY':
      return PresenceStatus.AWAY;
    case 'DO_NOT_DISTURB':
      return PresenceStatus.DO_NOT_DISTURB;
    case 'OFFLINE':
    default:
      return PresenceStatus.OFFLINE;
  }
}
