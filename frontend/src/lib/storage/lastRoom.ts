/**
 * Per-instance "last visited room" memory. Used to redirect users back to
 * the room they were last in when they return to an instance.
 *
 * Keys are namespaced by instance ID. There is no per-space dimension
 * because the deployment has one server (the instance) — see #330.
 */

import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { serverStorageKey } from './serverStorage';

const LAST_ROOM_SUFFIX = 'lastRoom';

/** Get the last visited room for an instance, or null if none. */
export function getLastRoom(serverId: string): string | null {
  try {
    return localStorage.getItem(serverStorageKey(serverId, LAST_ROOM_SUFFIX));
  } catch {
    return null;
  }
}

/** Save the last visited room for an instance. */
export function setLastRoom(serverId: string, roomId: string): void {
  try {
    localStorage.setItem(serverStorageKey(serverId, LAST_ROOM_SUFFIX), roomId);
  } catch {
    // Ignore storage errors (quota exceeded, etc.)
  }
}

/** Clear the last visited room for an instance. */
export function clearLastRoom(serverId: string): void {
  try {
    localStorage.removeItem(serverStorageKey(serverId, LAST_ROOM_SUFFIX));
  } catch {
    // Ignore storage errors
  }
}

/**
 * Resolve the last-visited path for an instance, or null if none.
 * Enables single-hop navigation from index pages to the user's last room.
 */
export function resolveLastPosition(serverId: string): string | null {
  const lastRoom = getLastRoom(serverId);
  if (!lastRoom) return null;
  return resolve('/chat/[serverId]/(chrome)/[roomId]', {
    serverId: serverIdToSegment(serverId),
    roomId: lastRoom
  });
}
