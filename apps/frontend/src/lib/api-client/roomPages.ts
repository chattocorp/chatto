import type { ListRoomsResponse, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';

/** Collect a complete directory before replacing local state. Pages are live reads. */
export async function listAllDirectoryRooms(
  readPage: (page: { limit: number; offset: number }) => Promise<ListRoomsResponse>
): Promise<RoomWithViewerState[]> {
  const rooms = new Map<string, RoomWithViewerState>();
  let offset = 0;
  for (;;) {
    const response = await readPage({ limit: 100, offset });
    for (const entry of response.rooms) {
      if (entry.room?.id) rooms.set(entry.room.id, entry);
    }
    if (!response.page?.hasMore) return [...rooms.values()];
    if (response.rooms.length === 0) throw new Error('Room directory returned an empty continuation page');
    offset += response.rooms.length;
  }
}
