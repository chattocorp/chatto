import { authHeaders, createChattoClient, handleAuthError } from "./connect.js";
import { MyAccountService } from "@chatto/api-types/api/v1/account_connect";
import { PresenceStatus } from "@chatto/api-types/api/v1/presence_pb";

export type PresenceAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

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
