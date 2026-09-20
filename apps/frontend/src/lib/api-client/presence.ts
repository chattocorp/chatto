import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig
} from './connect.js';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

export type PresenceAPIConfig = ConnectAPIConfig;

export function createPresenceAPI(config: PresenceAPIConfig) {
  const client = createChattoClient(MyAccountService, config);
  const headers = () => authHeaders(config);
  return {
    async getPreference() {
      try {
        return (await client.getPresencePreference({}, { headers: headers() })).preference;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async setPreference(status: PresenceStatus, expectedRevision: string) {
      try {
        return (
          await client.setPresencePreference({ status, expectedRevision }, { headers: headers() })
        ).preference;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async refreshPresence() {
      try {
        return (await client.refreshPresence({}, { headers: headers() })).preference;
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

export type PresenceAPI = ReturnType<typeof createPresenceAPI>;
