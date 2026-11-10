import type { RoomListItem } from '@chatto/client/server/rooms';
import { knownServers, serverRegistry } from '$lib/client';

/** One known server and its currently available rooms, without loading more data. */
export interface RoomLinkSource {
  origin: string;
  name: string;
  logoUrl: string | null;
  /** Whether this is the server where the message is being composed. */
  isCurrent: boolean;
  rooms: readonly RoomListItem[];
}

/** Read current client state so privacy resets and catalogue changes remove suggestions. */
export function roomLinkSources(currentServerId: string): RoomLinkSource[] {
  const sources = new Map<string, RoomLinkSource>();
  for (const server of knownServers.servers) {
    sources.set(server.origin, {
      origin: server.origin,
      name: server.profile.name,
      logoUrl: server.profile.iconUrl,
      isCurrent: false,
      rooms: []
    });
  }
  for (const server of serverRegistry.servers) {
    const origin = new URL(server.url).origin;
    const store = serverRegistry.tryGetStore(server.id);
    sources.set(origin, {
      origin,
      name: server.name,
      logoUrl: store?.serverInfo.iconUrl ?? server.iconUrl,
      isCurrent: server.id === currentServerId,
      rooms: store?.roomList.rooms ?? []
    });
  }
  return [...sources.values()];
}
