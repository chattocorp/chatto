import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import type { Neighbor as APINeighbor } from '@chatto/api-types/admin/v1/server_pb';
import { authHeaders, createChattoClient } from './connect.js';

export type Neighbor = {
  id: string;
  origin: string;
  revision: string;
};

export type NeighborAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export function createNeighborAPI(config: NeighborAPIConfig) {
  const client = createChattoClient(AdminServerService, config);
  const headers = () => authHeaders(config);

  return {
    async list(options: { signal?: AbortSignal } = {}): Promise<Neighbor[]> {
      const response = await client.listNeighbors(
        {},
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return response.neighbors.map(mapNeighbor);
    },

    async create(origin: string): Promise<Neighbor> {
      const response = await client.createNeighbor({ origin }, { headers: headers() });
      if (!response.neighbor) throw new Error('Neighbor response was incomplete.');
      return mapNeighbor(response.neighbor);
    },

    async update(neighbor: Neighbor, origin: string): Promise<Neighbor> {
      const response = await client.updateNeighbor(
        { neighborId: neighbor.id, origin, revision: neighbor.revision },
        { headers: headers() }
      );
      if (!response.neighbor) throw new Error('Neighbor response was incomplete.');
      return mapNeighbor(response.neighbor);
    },

    async delete(neighbor: Neighbor): Promise<void> {
      await client.deleteNeighbor(
        { neighborId: neighbor.id, revision: neighbor.revision },
        { headers: headers() }
      );
    }
  };
}

function mapNeighbor(neighbor: APINeighbor): Neighbor {
  return {
    id: neighbor.id,
    origin: neighbor.origin,
    revision: neighbor.revision
  };
}
