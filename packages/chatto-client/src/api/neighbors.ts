import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import type { Neighbor as APINeighbor } from '@chatto/api-types/admin/v1/server_pb';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';

export type Neighbor = {
  id: string;
  origin: string;
  revision: string;
};

export function createNeighborAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminServerService, config);

  return {
    async list(options: { signal?: AbortSignal } = {}): Promise<Neighbor[]> {
      const response = await client.listNeighbors({}, { signal: options.signal });
      return response.neighbors.map(mapNeighbor);
    },

    async create(origin: string): Promise<Neighbor> {
      const response = await client.createNeighbor({ origin });
      if (!response.neighbor) throw new Error('Neighbor response was incomplete.');
      return mapNeighbor(response.neighbor);
    },

    async update(neighbor: Neighbor, origin: string): Promise<Neighbor> {
      const response = await client.updateNeighbor({
        neighborId: neighbor.id,
        origin,
        revision: neighbor.revision,
        updateMask: { paths: ['origin'] }
      });
      if (!response.neighbor) throw new Error('Neighbor response was incomplete.');
      return mapNeighbor(response.neighbor);
    },

    async delete(neighbor: Neighbor): Promise<void> {
      await client.deleteNeighbor({ neighborId: neighbor.id, revision: neighbor.revision });
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
