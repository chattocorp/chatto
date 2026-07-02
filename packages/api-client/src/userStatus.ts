import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig,
} from "./connect.js";
import { Timestamp } from "@bufbuild/protobuf";
import { MyAccountService } from "@chatto/api-types/api/v1/account_connect";

export type CustomUserStatusAPIConfig = ConnectAPIConfig & {
  serverId: string;
};

export type CustomUserStatus = {
  emoji: string;
  text: string;
  expiresAt: string | null;
};

export async function updateCustomStatus(
  config: CustomUserStatusAPIConfig,
  input: {
    emoji: string;
    text: string;
    expiresAt?: string | null;
  },
): Promise<CustomUserStatus | null> {
  const client = createUserStatusClient(config);
  try {
    const response = await client.updateCustomStatus(
      {
        emoji: input.emoji,
        text: input.text,
        expiresAt: input.expiresAt
          ? Timestamp.fromDate(new Date(input.expiresAt))
          : undefined,
      },
      { headers: authHeaders(config) },
    );
    return apiStatus(response.status);
  } catch (err) {
    handleAuthError(config, err);
  }
}

export async function deleteCustomStatus(
  config: CustomUserStatusAPIConfig,
): Promise<CustomUserStatus | null> {
  const client = createUserStatusClient(config);
  try {
    const response = await client.deleteCustomStatus(
      {},
      { headers: authHeaders(config) },
    );
    return apiStatus(response.status);
  } catch (err) {
    handleAuthError(config, err);
  }
}

function createUserStatusClient(config: CustomUserStatusAPIConfig) {
  return createChattoClient(MyAccountService, config);
}

function apiStatus(
  status:
    | {
        emoji: string;
        text: string;
        expiresAt?: { toDate(): Date };
      }
    | undefined,
): CustomUserStatus | null {
  if (!status) return null;
  return {
    emoji: status.emoji,
    text: status.text,
    expiresAt: status.expiresAt
      ? status.expiresAt.toDate().toISOString()
      : null,
  };
}
