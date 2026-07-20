import { resolve } from '$app/paths';

const serverPages = new Set([
  'general',
  'members',
  'moderation',
  'permissions',
  'security',
  'system',
  'event-log'
]);

/** Maps a legacy server-admin suffix to its permanent management destination. */
export function legacyServerAdminDestination(
  serverId: string,
  legacyPath: string,
  search = ''
): string | null {
  const segments = legacyPath.split('/').filter(Boolean);
  let destination: string | null = null;

  if (segments.length === 0) {
    destination = resolve('/chat/[serverId]/manage/server/general', { serverId });
  } else if (segments.length === 1 && segments[0] === 'rooms') {
    destination = resolve('/chat/[serverId]/manage/rooms', { serverId });
  } else if (segments.length === 3 && segments[0] === 'rooms' && segments[1] === 'room') {
    destination = resolve('/chat/[serverId]/manage/rooms/[roomId]', {
      serverId,
      roomId: segments[2]
    });
  } else if (
    segments.length === 3 &&
    segments[0] === 'rooms' &&
    segments[1] === 'group'
  ) {
    destination = resolve('/chat/[serverId]/manage/room-groups/[groupId]', {
      serverId,
      groupId: segments[2]
    });
  } else if (
    serverPages.has(segments[0] ?? '') &&
    (segments.length === 1 ||
      (segments.length === 2 &&
        ['members', 'permissions', 'event-log'].includes(segments[0] ?? '')))
  ) {
    const suffix = segments.map(encodeURIComponent).join('/');
    destination = `/chat/${encodeURIComponent(serverId)}/manage/server/${suffix}`;
  }

  return destination ? `${destination}${search}` : null;
}
