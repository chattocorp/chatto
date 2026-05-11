/**
 * Per-instance "last visited room" memory. Used to redirect users back to
 * the room they were last in when they return to an instance.
 *
 * Keys are namespaced by instance ID. There is no per-space dimension
 * because the deployment has one server (the instance) — see #330.
 */

import { resolve } from '$app/paths';
import { instanceIdToSegment } from '$lib/navigation';
import { instanceStorageKey } from './instanceStorage';

const LAST_ROOM_SUFFIX = 'lastRoom';

/** Get the last visited room for an instance, or null if none. */
export function getLastRoom(instanceId: string): string | null {
  try {
    return localStorage.getItem(instanceStorageKey(instanceId, LAST_ROOM_SUFFIX));
  } catch {
    return null;
  }
}

/** Save the last visited room for an instance. */
export function setLastRoom(instanceId: string, roomId: string): void {
  try {
    localStorage.setItem(instanceStorageKey(instanceId, LAST_ROOM_SUFFIX), roomId);
  } catch {
    // Ignore storage errors (quota exceeded, etc.)
  }
}

/** Clear the last visited room for an instance. */
export function clearLastRoom(instanceId: string): void {
  try {
    localStorage.removeItem(instanceStorageKey(instanceId, LAST_ROOM_SUFFIX));
  } catch {
    // Ignore storage errors
  }
}

/**
 * Resolve the last-visited path for an instance, or null if none.
 * Enables single-hop navigation from index pages to the user's last room.
 */
export function resolveLastPosition(instanceId: string): string | null {
  const lastRoom = getLastRoom(instanceId);
  if (!lastRoom) return null;
  return resolve('/chat/[instanceId]/(chrome)/[roomId]', {
    instanceId: instanceIdToSegment(instanceId),
    roomId: lastRoom
  });
}
