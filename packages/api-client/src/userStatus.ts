import { notifyAuthenticationRequired } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { Timestamp } from "@bufbuild/protobuf";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AccountService } from "@chatto/api-types/api/v1/account_connect";

export type CustomUserStatusAPIConfig = {
  serverId: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type CustomUserStatus = {
  emoji: string;
  text: string;
  expiresAt: string | null;
};

export async function setCustomStatus(
  config: CustomUserStatusAPIConfig,
  input: {
    emoji: string;
    text: string;
    expiresAt?: string | null;
  },
): Promise<CustomUserStatus | null> {
  const client = createUserStatusClient(config);
  try {
    const response = await client.setCustomStatus(
      {
        emoji: input.emoji,
        text: input.text,
        expiresAt: input.expiresAt
          ? Timestamp.fromDate(new Date(input.expiresAt))
          : undefined,
      },
      { headers: headers(config) },
    );
    return apiStatus(response.status);
  } catch (err) {
    handleAuthError(config, err);
  }
}

export async function clearCustomStatus(
  config: CustomUserStatusAPIConfig,
): Promise<CustomUserStatus | null> {
  const client = createUserStatusClient(config);
  try {
    const response = await client.clearCustomStatus(
      {},
      { headers: headers(config) },
    );
    return apiStatus(response.status);
  } catch (err) {
    handleAuthError(config, err);
  }
}

function createUserStatusClient(config: CustomUserStatusAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  return createClient(AccountService, transport);
}

function headers(config: CustomUserStatusAPIConfig) {
  return config.bearerToken
    ? { Authorization: `Bearer ${config.bearerToken}` }
    : undefined;
}

function handleAuthError(
  config: CustomUserStatusAPIConfig,
  err: unknown,
): never {
  if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
    notifyAuthenticationRequired(
      config.serverId,
      config.onAuthenticationRequired,
    );
  }
  throw err;
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
