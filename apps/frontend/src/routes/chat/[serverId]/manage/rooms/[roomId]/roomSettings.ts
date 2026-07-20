import type { RoomCommandAPI } from '$lib/api-client/rooms';

export type RoomSettingsValues = {
  name: string;
  description: string;
  universal: boolean;
};

type RoomUpdateInput = Parameters<RoomCommandAPI['updateRoom']>[0];

/** Builds a sparse update so unchanged fields do not emit durable room events. */
export function buildRoomSettingsUpdate(
  roomId: string,
  current: RoomSettingsValues,
  original: RoomSettingsValues
): RoomUpdateInput {
  const input: RoomUpdateInput = { roomId };
  const name = current.name.trim();
  const description = current.description.trim();

  if (name !== original.name) input.name = name;
  if (description !== original.description) input.description = description || null;
  if (current.universal !== original.universal) input.universal = current.universal;

  return input;
}
