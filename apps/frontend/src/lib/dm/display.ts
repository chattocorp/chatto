export type DMDisplayParticipant = {
  id: string;
  login: string;
  displayName: string;
  deleted?: boolean | null;
};

type ParticipantFormatter<T extends DMDisplayParticipant> = (participant: T) => string;

function defaultParticipantLabel(participant: DMDisplayParticipant): string {
  return participant.displayName || participant.login;
}

/**
 * Derive the visible label for a DM conversation from the members returned by
 * the room member endpoint.
 */
export function getDMConversationLabel<T extends DMDisplayParticipant>(
  participants: readonly T[],
  currentUserId: string | null | undefined,
  deletedUserLabel: string,
  formatParticipant: ParticipantFormatter<T> = defaultParticipantLabel
): string {
  const otherParticipants = participants.filter((participant) => participant.id !== currentUserId);

  if (otherParticipants.length === 0) {
    return deletedUserLabel;
  }

  return otherParticipants
    .map((participant) => {
      if (participant.deleted) return deletedUserLabel;
      return formatParticipant(participant);
    })
    .join(', ');
}

export function getDMAvatarParticipants<T extends { id: string }>(
  participants: readonly T[] | null | undefined,
  currentUserId: string | null | undefined,
  limit: number
): T[] {
  const availableParticipants = participants ?? [];
  const otherParticipants = availableParticipants.filter(
    (participant) => participant.id !== currentUserId
  );
  const visibleParticipants =
    otherParticipants.length === 0 ? availableParticipants : otherParticipants;

  return visibleParticipants.slice(0, limit);
}
