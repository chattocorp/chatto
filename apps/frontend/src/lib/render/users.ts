import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

export type CustomUserStatusView = {
  emoji: string;
  text: string;
  expiresAt?: string | null;
};

/**
 * The narrow user shape shared by avatar-bearing chat surfaces.
 */
export type UserAvatarUserView = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
  customStatus?: CustomUserStatusView | null;
};

type DirectMessageParticipant = Pick<UserAvatarUserView, 'id' | 'login' | 'displayName'>;

/** Builds the shared label and avatar participants for a direct message. */
export function buildDirectMessagePresentation<T extends DirectMessageParticipant>(
  participants: readonly T[],
  currentUserId: string | null | undefined,
  currentUserLabel: string,
  getDisplayName: (userId: string, fallback: string) => string = (_userId, fallback) => fallback
) {
  const others = participants.filter((participant) => participant.id !== currentUserId);
  return {
    label:
      others.length > 0
        ? others
            .map((participant) =>
              getDisplayName(participant.id, participant.displayName || participant.login)
            )
            .join(', ')
        : currentUserLabel,
    visibleParticipants: others.length > 0 ? others : participants.slice(0, 1)
  };
}
