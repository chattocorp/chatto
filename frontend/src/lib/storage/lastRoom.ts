/**
 * Utilities for storing and retrieving the last visited space and room.
 * This allows auto-redirecting users to their last location when returning.
 *
 * Keys are namespaced by instance ID to prevent collisions in multi-instance mode.
 * All functions require an explicit instance ID — no implicit "active" instance.
 */

import { resolve } from '$app/paths';
import { DM_SPACE_ID } from '$lib/constants';
import { instanceIdToSegment } from '$lib/navigation';
import { instanceRegistry } from '$lib/state/instance/registry.svelte';
import { instanceStorageKey, migrateStorageKey } from './instanceStorage';

const LAST_ROOMS_SUFFIX = 'lastRooms';
const LAST_SPACE_SUFFIX = 'lastSpace';

// Legacy keys (pre-namespacing)
const LEGACY_LAST_ROOMS_KEY = 'chatto:lastRooms';
const LEGACY_LAST_SPACE_KEY = 'chatto:lastSpace';

/** Track which instance IDs have already been migrated this session. */
const migratedInstances = new Set<string>();

/**
 * Lazily migrate legacy keys for the given instance (once per session).
 */
function ensureMigrated(instanceId: string): void {
  if (migratedInstances.has(instanceId)) return;
  migratedInstances.add(instanceId);

  // Only migrate legacy keys for the origin instance
  if (!instanceRegistry.isOriginInstance(instanceId)) return;

  migrateStorageKey(instanceId, LEGACY_LAST_ROOMS_KEY, LAST_ROOMS_SUFFIX);
  migrateStorageKey(instanceId, LEGACY_LAST_SPACE_KEY, LAST_SPACE_SUFFIX);
}

type LastRoomsMap = Record<string, string>;

function getRoomsStorage(instanceId: string): LastRoomsMap {
  try {
    ensureMigrated(instanceId);
    const stored = localStorage.getItem(instanceStorageKey(instanceId, LAST_ROOMS_SUFFIX));
    return stored ? JSON.parse(stored) : {};
  } catch {
    return {};
  }
}

function setRoomsStorage(instanceId: string, map: LastRoomsMap): void {
  try {
    localStorage.setItem(instanceStorageKey(instanceId, LAST_ROOMS_SUFFIX), JSON.stringify(map));
  } catch {
    // Ignore storage errors (quota exceeded, etc.)
  }
}

/**
 * Get the last visited room for a space.
 */
export function getLastRoom(instanceId: string, spaceId: string): string | null {
  const map = getRoomsStorage(instanceId);
  return map[spaceId] ?? null;
}

/**
 * Save the last visited room for a space.
 */
export function setLastRoom(instanceId: string, spaceId: string, roomId: string): void {
  const map = getRoomsStorage(instanceId);
  map[spaceId] = roomId;
  setRoomsStorage(instanceId, map);
}

/**
 * Get the last visited space.
 */
export function getLastSpace(instanceId: string): string | null {
  try {
    ensureMigrated(instanceId);
    return localStorage.getItem(instanceStorageKey(instanceId, LAST_SPACE_SUFFIX));
  } catch {
    return null;
  }
}

/**
 * Save the last visited space.
 */
export function setLastSpace(instanceId: string, spaceId: string): void {
  try {
    localStorage.setItem(instanceStorageKey(instanceId, LAST_SPACE_SUFFIX), spaceId);
  } catch {
    // Ignore storage errors
  }
}

/**
 * Clear the last visited space.
 */
export function clearLastSpace(instanceId: string): void {
  try {
    localStorage.removeItem(instanceStorageKey(instanceId, LAST_SPACE_SUFFIX));
  } catch {
    // Ignore storage errors
  }
}

/**
 * Resolve the last visited position for an instance as a navigation path.
 * Returns the deepest known path (room > space), or null if no history.
 *
 * This enables single-hop navigation from index pages (/, /chat/[instanceId])
 * directly to the user's last location, avoiding multi-hop redirect chains.
 */
export function resolveLastPosition(instanceId: string): string | null {
  const lastSpace = getLastSpace(instanceId);
  if (!lastSpace || lastSpace === DM_SPACE_ID) return null;

  const lastRoom = getLastRoom(instanceId, lastSpace);
  return lastRoom
    ? resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceIdToSegment(instanceId), roomId: lastRoom })
    : resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(instanceId) });
}

/**
 * Clear the last visited room for a space.
 */
export function clearLastRoom(instanceId: string, spaceId: string): void {
  try {
    const map = getRoomsStorage(instanceId);
    delete map[spaceId];
    setRoomsStorage(instanceId, map);
  } catch {
    // Ignore storage errors
  }
}
