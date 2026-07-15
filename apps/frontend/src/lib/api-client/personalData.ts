import { FieldMask } from '@bufbuild/protobuf';
import { PersonalDataService } from '@chatto/api-types/chatto/personaldata/api/v1/personal_data_connect';
import {
  KnownServer,
  Preferences,
  TimeFormat
} from '@chatto/api-types/chatto/personaldata/api/v1/personal_data_pb';
import { authHeaders, createChattoClient, type ConnectAPIConfig, withAuth } from './connect';

export { TimeFormat as PersonalTimeFormat };

export type PersonalPreferences = {
  locale?: string;
  timezone?: string;
  timeFormat?: TimeFormat;
};

export type PersonalKnownServer = {
  id: string;
  url: string;
  name: string;
  iconUrl?: string;
};

export function createPersonalDataAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(PersonalDataService, { baseUrl: config.baseUrl });
  const headers = authHeaders(config);

  return {
    async getPreferences(): Promise<PersonalPreferences> {
      const response = await withAuth(config, () => client.getPreferences({}, { headers }));
      return response.preferences ?? {};
    },

    async updatePreferences(
      preferences: PersonalPreferences,
      paths: Array<'locale' | 'timezone' | 'time_format'>
    ): Promise<PersonalPreferences> {
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
      servers: PersonalKnownServer[];
      homeServerId?: string;
    }> {
      const response = await withAuth(config, () => client.listKnownServers({}, { headers }));
      return {
        servers: response.servers.map(serverFromAPI),
        homeServerId: response.homeServerId
      };
    },

    async createKnownServer(server: PersonalKnownServer): Promise<void> {
      await withAuth(config, () =>
        client.createKnownServer({ server: new KnownServer(server) }, { headers })
      );
    },

    async updateKnownServer(server: PersonalKnownServer): Promise<void> {
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

function serverFromAPI(server: KnownServer): PersonalKnownServer {
  return {
    id: server.id,
    url: server.url,
    name: server.name,
    iconUrl: server.iconUrl
  };
}
