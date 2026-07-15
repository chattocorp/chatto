import { FieldMask } from '@bufbuild/protobuf';
import { ClientSyncService } from '@chatto/api-types/chatto/clientsync/api/v1/client_sync_connect';
import {
  KnownServer,
  Preferences,
  TimeFormat
} from '@chatto/api-types/chatto/clientsync/api/v1/client_sync_pb';
import { authHeaders, createChattoClient, type ConnectAPIConfig, withAuth } from './connect';

export { TimeFormat as ClientSyncTimeFormat };

export type SyncedPreferences = {
  locale?: string;
  timezone?: string;
  timeFormat?: TimeFormat;
};

export type SyncedKnownServer = {
  id: string;
  url: string;
  name: string;
  iconUrl?: string;
};

export function createClientSyncAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(ClientSyncService, { baseUrl: config.baseUrl });
  const headers = authHeaders(config);

  return {
    async getPreferences(): Promise<SyncedPreferences> {
      const response = await withAuth(config, () => client.getPreferences({}, { headers }));
      return response.preferences ?? {};
    },

    async updatePreferences(
      preferences: SyncedPreferences,
      paths: Array<'locale' | 'timezone' | 'time_format'>
    ): Promise<SyncedPreferences> {
      const response = await withAuth(config, () =>
        client.updatePreferences(
          {
            preferences: new Preferences(preferences),
            updateMask: new FieldMask({ paths })
          },
          { headers }
        )
      );
      return response.preferences ?? {};
    },

    async listKnownServers(): Promise<{
      servers: SyncedKnownServer[];
      homeServerId?: string;
    }> {
      const response = await withAuth(config, () => client.listKnownServers({}, { headers }));
      return {
        servers: response.servers.map(serverFromAPI),
        homeServerId: response.homeServerId
      };
    },

    async createKnownServer(server: SyncedKnownServer): Promise<void> {
      await withAuth(config, () =>
        client.createKnownServer({ server: new KnownServer(server) }, { headers })
      );
    },

    async updateKnownServer(server: SyncedKnownServer): Promise<void> {
      await withAuth(config, () =>
        client.updateKnownServer(
          {
            server: new KnownServer(server),
            updateMask: new FieldMask({ paths: ['url', 'name', 'icon_url'] })
          },
          { headers }
        )
      );
    },

    async deleteKnownServer(id: string): Promise<void> {
      await withAuth(config, () => client.deleteKnownServer({ id }, { headers }));
    },

    async setHomeServer(id: string): Promise<void> {
      await withAuth(config, () => client.setHomeServer({ id }, { headers }));
    }
  };
}

function serverFromAPI(server: KnownServer): SyncedKnownServer {
  return {
    id: server.id,
    url: server.url,
    name: server.name,
    iconUrl: server.iconUrl
  };
}
