import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

/**
 * Transient realtime signals delivered outside durable room timelines.
 */
export const TransientEventKind = {
  MentionNotification: 'mentionNotification',
  NewDirectMessageNotification: 'newDirectMessageNotification',
  PresenceChanged: 'presenceChanged',
  SessionTerminated: 'sessionTerminated',
  UserTyping: 'userTyping'
} as const;

export type TransientEventKind = (typeof TransientEventKind)[keyof typeof TransientEventKind];

export type TransientEventPayload =
  | {
      kind: typeof TransientEventKind.MentionNotification;
      roomId: string;
      actorUserId: string;
      actorDisplayName: string;
      roomName: string;
    }
  | {
      kind: typeof TransientEventKind.NewDirectMessageNotification;
      roomId: string;
      senderId: string;
      senderDisplayName: string;
      senderAvatarUrl: string;
      conversationName: string;
    }
  | { kind: typeof TransientEventKind.PresenceChanged; status: PresenceStatus }
  | { kind: typeof TransientEventKind.SessionTerminated; reason: string }
  | {
      kind: typeof TransientEventKind.UserTyping;
      roomId: string;
      typingThreadRootEventId?: string | null;
    };

export type TransientEventEnvelope = {
  id: string;
  createdAt: string;
  actorId?: string | null;
  event: TransientEventPayload;
};

export function transientEventKind(
  event: TransientEventPayload | object | null | undefined
): TransientEventKind | null {
  if (!event) return null;
  const kind = (event as { kind?: unknown }).kind;
  return typeof kind === 'string' &&
    Object.values(TransientEventKind).includes(kind as TransientEventKind)
    ? (kind as TransientEventKind)
    : null;
}
