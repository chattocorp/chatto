import { notifyAuthenticationRequired } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
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
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(MyAccountService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      config.serverId
    ) {
      notifyAuthenticationRequired(
        config.serverId,
        config.onAuthenticationRequired,
      );
    }
    throw err;
  }

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
        return handleAuthError(err);
      }
    },
  };
}
