/**
 * Per-server "last visited room" memory. Used to redirect users back to
 * the room they were last in when they return to a server.
 */

import { serverIdToSegment } from '$lib/navigation';
import { roomPathForSegment } from '$lib/roomUrls';
import { Codecs, serverSlot } from './slot';
import type { ResolvedPathname } from '$app/types';

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
export function resolveLastPosition(serverId: string): ResolvedPathname | null {
  const lastRoom = getLastRoom(serverId);
  if (!lastRoom) return null;
  return roomPathForSegment(serverIdToSegment(serverId), lastRoom);
}
