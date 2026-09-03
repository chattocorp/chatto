import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TransientEventKind, type TransientEventEnvelope } from '$lib/realtimeEvents';

function timestampToISO(value: { toDate(): Date } | undefined): string {
  return value?.toDate().toISOString() ?? new Date().toISOString();
}

export function realtimeEventToEventEnvelope(frame: RealtimeEvent): TransientEventEnvelope | null {
  const base = {
    id: frame.id,
    createdAt: timestampToISO(frame.createdAt),
    actorId: frame.actorId || null
  };

  switch (frame.event.case) {
    case 'userTypingSignal': {
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
    case 'presenceChangedSignal':
      return {
        ...base,
        event: {
          kind: TransientEventKind.PresenceChanged,
          status: presenceStatusFromSignal(frame.event.value.status)
        }
      };
    case 'sessionTerminatedSignal':
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
