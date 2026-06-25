import { isUserMentioned, type RoomMember } from '$lib/mentions';

export function shouldHighlightCurrentUserMention({
  actorId,
  body,
  currentUserId,
  currentUserLogin,
  members
}: {
  actorId: string | null | undefined;
  body: string | null | undefined;
  currentUserId: string | null | undefined;
  currentUserLogin: string | null | undefined;
  members: RoomMember[];
}): boolean {
  return Boolean(
    currentUserLogin &&
    body &&
    actorId !== currentUserId &&
    isUserMentioned(body, currentUserLogin, members)
  );
}
