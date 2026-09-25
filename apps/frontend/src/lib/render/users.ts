import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { formatAccountName } from './accountName';

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
  isBot?: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
  customStatus?: CustomUserStatusView | null;
};

type DirectMessageParticipant = Pick<
  UserAvatarUserView,
  'id' | 'login' | 'displayName' | 'isBot'
> & { deleted?: boolean };

/** Builds the shared label and avatar participants for a direct message. */
export function buildDirectMessagePresentation<T extends DirectMessageParticipant>(
  participants: readonly T[],
  currentUserId: string | null | undefined,
  currentUserLabel: string,
  getDisplayName: (userId: string, fallback: string) => string = (_userId, fallback) => fallback
) {
  const others = participants.filter((participant) => participant.id !== currentUserId);
  const self = participants[0];
  return {
    label:
      others.length > 0
        ? others
            .map((participant) =>
              formatAccountName(
                getDisplayName(participant.id, participant.displayName || participant.login),
                participant
              )
            )
            .join(', ')
        : self
          ? `${formatAccountName(getDisplayName(self.id, self.displayName || self.login) || self.login, self)} (${currentUserLabel})`
          : currentUserLabel,
    visibleParticipants: others.length > 0 ? others : participants.slice(0, 1)
  };
}
