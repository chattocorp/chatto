import type { RoomListItem } from '@chatto/client/server/rooms';
import { knownServers, serverRegistry } from '$lib/client';

/** One known server and its currently available rooms, without loading more data. */
export interface RoomLinkSource {
  origin: string;
  name: string;
  rooms: readonly RoomListItem[];
}

/** Read current client state so privacy resets and catalogue changes remove suggestions. */
export function roomLinkSources(): RoomLinkSource[] {
  const sources = new Map<string, RoomLinkSource>();
  for (const server of knownServers.servers) {
    sources.set(server.origin, { origin: server.origin, name: server.profile.name, rooms: [] });
  }
  for (const server of serverRegistry.servers) {
    const origin = new URL(server.url).origin;
    sources.set(origin, {
      origin,
      name: server.name,
      rooms: serverRegistry.tryGetStore(server.id)?.roomList.rooms ?? []
    });
  }
  return [...sources.values()];
}
