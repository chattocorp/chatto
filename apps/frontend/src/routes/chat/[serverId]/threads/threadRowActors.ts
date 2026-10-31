import type { UserAvatarUserView } from '@chatto/client/timeline/users';

/**
 * Choose the participant preview for a My Threads row.
 *
 * The root author comes first, so the row avatar identifies the author of the
 * original message. The second entry, shown as the avatar badge,
 * is the first other person among `others` (DM partners or repliers in reply
 * order) and then the latest reply author.
 */
export function threadRowActors(
  rootAuthor: UserAvatarUserView | null | undefined,
  others: readonly UserAvatarUserView[],
  latestAuthor: UserAvatarUserView | null | undefined
): UserAvatarUserView[] {
  const candidates = [rootAuthor, ...others, latestAuthor].filter(
    (actor): actor is UserAvatarUserView => Boolean(actor)
  );
  const [first] = candidates;
  if (!first) return [];
  const second = candidates.find((actor) => actor.id !== first.id);
  return second ? [first, second] : [first];
}
