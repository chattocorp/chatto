import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { CustomUserStatus } from '$lib/api-client/userSummary';
import { formatAccountName } from './accountName';

/**
 * The narrow user shape shared by avatar-bearing chat surfaces.
 */
export type UserAvatarUserView = {
  id: string;
  login: string;
  displayName: string;
  deleted?: boolean;
  isBot?: boolean;
  /** Public human owner of an active bot; absent for other accounts. */
  bot?: { ownerUserId: string };
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
  customStatus?: CustomUserStatus | null;
};

type DirectMessageParticipant = Pick<
  UserAvatarUserView,
  'id' | 'login' | 'displayName' | 'isBot'
> & { deleted?: boolean };

/** Localized labels for DM participants who have no display name of their own. */
export type DirectMessageLabels = {
  /** Marks the viewer, for example "You". */
  currentUser: string;
  /** Replaces the name of a participant whose account was deleted. */
  deletedUser: string;
};

/**
 * Placeholder for a DM participant whose account was deleted. The server
 * reports these IDs separately because deleted accounts are no longer members.
 */
export function deletedDirectMessageParticipant(id: string): UserAvatarUserView {
  return {
    id,
    login: '',
    displayName: '',
    deleted: true,
    presenceStatus: PresenceStatus.UNSPECIFIED
  };
}

/**
 * Builds the shared label and avatar participants for a direct message.
 *
 * Deleted participants count as other participants, so a DM whose partner
 * deleted their account never looks like the viewer's self-DM.
 */
export function buildDirectMessagePresentation<T extends DirectMessageParticipant>(
  participants: readonly T[],
  currentUserId: string | null | undefined,
  labels: DirectMessageLabels,
  getDisplayName: (userId: string, fallback: string) => string = (_userId, fallback) => fallback
) {
  const others = participants.filter((participant) => participant.id !== currentUserId);
  const self = participants[0];
  return {
    label:
      others.length > 0
        ? others
            .map((participant) =>
              participant.deleted
                ? labels.deletedUser
                : formatAccountName(
                    getDisplayName(participant.id, participant.displayName || participant.login),
                    participant
                  )
            )
            .join(', ')
        : self
          ? `${formatAccountName(getDisplayName(self.id, self.displayName || self.login) || self.login, self)} (${labels.currentUser})`
          : labels.currentUser,
    visibleParticipants: others.length > 0 ? others : participants.slice(0, 1)
  };
}
