export type RoomGroupSettingsValues = {
  name: string;
  description: string;
};

export function buildRoomGroupSettingsUpdate(
  groupId: string,
  current: RoomGroupSettingsValues,
  original: RoomGroupSettingsValues
): { groupId: string; name?: string; description?: string | null } {
  const name = current.name.trim();
  const description = current.description.trim();
  const update: { groupId: string; name?: string; description?: string | null } = { groupId };

  if (name !== original.name) update.name = name;
  if (description !== original.description) update.description = description || null;

  return update;
}
