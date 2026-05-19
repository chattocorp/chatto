/**
 * Per-server "last visited room" memory. Used to redirect users back to
 * the room they were last in when they return to a server.
 */

import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { Codecs, serverSlot } from './slot';

const SUFFIX = 'lastRoom';

function slot(serverId: string) {
  return serverSlot(serverId, SUFFIX, '', Codecs.string);
}

export function getLastRoom(serverId: string): string | null {
  return slot(serverId).get() || null;
}

export function setLastRoom(serverId: string, roomId: string): void {
  slot(serverId).set(roomId);
}

export function clearLastRoom(serverId: string): void {
  slot(serverId).remove();
}

/**
 * Resolve the last-visited path for a server, or null if none.
 * Enables single-hop navigation from index pages to the user's last room.
 */
export function resolveLastPosition(serverId: string): string | null {
  const lastRoom = getLastRoom(serverId);
  if (!lastRoom) return null;
  return resolve('/chat/[serverId]/[roomId]', {
    serverId: serverIdToSegment(serverId),
    roomId: lastRoom
  });
}
