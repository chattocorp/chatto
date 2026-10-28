import type { PublicRoom, RoomCommandAPI } from '@chatto/client/api/rooms';
import { normalizeRoomName } from '@chatto/client/util/roomName';
import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';

type RoomUpdateInput = Parameters<RoomCommandAPI['updateRoom']>[0];

/**
 * A sparse room settings update without the room ID. Every panel sends only
 * the fields it changed, so unchanged fields do not emit durable room events
 * or overwrite concurrent changes.
 */
export type RoomSettingsPatch = Omit<RoomUpdateInput, 'roomId'>;

/**
 * Apply only submitted fields from an accepted response. Concurrent panels can
 * receive whole-room responses in a different order from their saves.
 */
export function applyRoomSettingsResponse(
  current: AdminManagedRoom,
  updated: PublicRoom,
  patch: RoomSettingsPatch
): AdminManagedRoom {
  return {
    ...current,
    ...('name' in patch && { name: updated.name }),
    ...('description' in patch && { description: updated.description || null }),
    ...('universal' in patch && { isUniversal: updated.universal }),
    ...('slowModeSeconds' in patch && { slowModeSeconds: updated.slowModeSeconds }),
    ...('threadSlowModeSeconds' in patch && {
      threadSlowModeSeconds: updated.threadSlowModeSeconds
    }),
    ...('threadingMode' in patch && { threadingMode: updated.threadingMode }),
    ...('showMembershipEvents' in patch && {
      showMembershipEvents: updated.showMembershipEvents
    })
  };
}

/** The editable name and description of a room. */
export type RoomDetailsValues = { name: string; description: string };

/**
 * Builds the name and description patch. It normalizes the name, trims the
 * description, clears an empty description, and omits unchanged fields.
 */
export function buildRoomDetailsPatch(
  current: RoomDetailsValues,
  original: RoomDetailsValues
): RoomSettingsPatch {
  const patch: RoomSettingsPatch = {};
  const name = normalizeRoomName(current.name);
  const description = current.description.trim();
  if (name !== original.name) patch.name = name;
  if (description !== original.description) patch.description = description || null;
  return patch;
}
