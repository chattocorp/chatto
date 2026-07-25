import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig,
} from "./connect.js";
import { MyAccountService } from "@chatto/api-types/api/v1/account_connect";
import { PresenceStatus } from "@chatto/api-types/api/v1/presence_pb";

export type PresenceAPIConfig = ConnectAPIConfig;

export { PresenceStatus as APIPresenceStatus };

export function createPresenceAPI(config: PresenceAPIConfig) {
  const client = createChattoClient(MyAccountService, config);
  const headers = () => authHeaders(config);
  return {
    async updatePresence(
      status: PresenceStatus,
      userSelected = false,
    ): Promise<PresenceStatus> {
      try {
        const response = await client.updatePresence(
          { status, userSelected },
          { headers: headers() },
        );
        return response.status;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
  };
}

export type PresenceAPI = ReturnType<typeof createPresenceAPI>;
