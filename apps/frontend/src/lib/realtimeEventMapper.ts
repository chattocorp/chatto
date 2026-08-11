import { RealtimeEventEnvelope } from '@chatto/api-types/realtime/v1/realtime_pb';
import { presenceStatusOrOffline } from '$lib/api-client/enumDefaults';
import { TransientEventKind, type TransientEventEnvelope } from '$lib/realtimeEvents';

function timestampToISO(value: { toDate(): Date } | undefined): string {
  return value?.toDate().toISOString() ?? new Date().toISOString();
}

export function realtimeEventToEventEnvelope(
  frame: RealtimeEventEnvelope
): TransientEventEnvelope | null {
  const base = {
    id: frame.id,
    createdAt: timestampToISO(frame.createdAt),
    actorId: frame.actorId ?? null
  };

  switch (frame.event.case) {
    case 'userTyping': {
      const value = frame.event.value;
      return {
        ...base,
        event: {
          kind: TransientEventKind.UserTyping,
          roomId: value.roomId,
          typingThreadRootEventId: value.threadRootEventId ?? null
        }
      };
    }
    case 'presenceChanged':
      return {
        ...base,
        actorId: frame.event.value.userId || base.actorId,
        event: {
          kind: TransientEventKind.PresenceChanged,
          status: presenceStatusOrOffline(frame.event.value.status)
        }
      };
    case 'sessionTerminated':
      return {
        ...base,
        event: {
          kind: TransientEventKind.SessionTerminated,
          reason: frame.event.value.reason
        }
      };
    default:
      return null;
  }
}
