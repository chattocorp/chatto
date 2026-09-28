import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

export function createPresenceAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MyAccountService, config);
  return {
    async getPreference() {
      return (await client.getPresencePreference({})).preference;
    },
    async setPreference(status: PresenceStatus, expectedRevision: string) {
      return (await client.setPresencePreference({ status, expectedRevision })).preference;
    },
    async refreshPresence() {
      return (await client.refreshPresence({})).preference;
    }
  };
}

export type PresenceAPI = ReturnType<typeof createPresenceAPI>;
