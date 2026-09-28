import type { RoomCommandAPI } from '$lib/api-client/rooms';
import { normalizeRoomName } from '$lib/utils/roomName';

type RoomUpdateInput = Parameters<RoomCommandAPI['updateRoom']>[0];

/**
 * A sparse room settings update without the room ID. Every panel sends only
 * the fields it changed, so unchanged fields do not emit durable room events
 * or overwrite concurrent changes.
 */
export type RoomSettingsPatch = Omit<RoomUpdateInput, 'roomId'>;

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
