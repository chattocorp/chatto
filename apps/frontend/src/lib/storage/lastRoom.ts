/**
 * Per-server "last visited room" memory. Used to redirect users back to
 * the room they were last in when they return to a server.
 */

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
