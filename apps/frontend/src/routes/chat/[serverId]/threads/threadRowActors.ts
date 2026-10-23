import type { UserAvatarUserView } from '@chatto/client/timeline/users';

/**
 * Choose the participant preview for a My Threads row.
 *
 * The author of the latest message comes first, so the row avatar matches the
 * name on the row's first line. The second entry, shown as the avatar badge,
 * is the first other person among `others` (DM partners or repliers in reply
 * order) and then the root author.
 */
export function threadRowActors(
  latestAuthor: UserAvatarUserView | null | undefined,
  others: readonly UserAvatarUserView[],
  rootAuthor: UserAvatarUserView | null | undefined
): UserAvatarUserView[] {
  const candidates = [latestAuthor, ...others, rootAuthor].filter(
    (actor): actor is UserAvatarUserView => Boolean(actor)
  );
  const [first] = candidates;
  if (!first) return [];
  const second = candidates.find((actor) => actor.id !== first.id);
  return second ? [first, second] : [first];
}
