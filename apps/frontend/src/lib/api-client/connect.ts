import {
  Code,
  ConnectError,
  createClient,
  type Client,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import type { ServiceType } from "@bufbuild/protobuf";
import { notifyAuthenticationRequired } from "./hooks.js";

export type ConnectAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type PublicConnectAPIConfig = {
  baseUrl: string;
};

export function connectEndpoint(baseUrl: string): string {
  return new URL("/api/connect", baseUrl).toString();
}

export function createChattoTransport(
  config: { baseUrl: string },
  options: { useBinaryFormat?: boolean } = {},
): Transport {
  return createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: options.useBinaryFormat ?? true,
  });
}

export function createChattoClient<T extends ServiceType>(
  service: T,
  config: { baseUrl: string },
): Client<T> {
  return createClient(service, createChattoTransport(config));
}

export function createPublicChattoClient<T extends ServiceType>(
  service: T,
  baseUrl: string,
): Client<T> {
  return createClient(
    service,
    createChattoTransport(
      { baseUrl: connectEndpoint(baseUrl) },
      { useBinaryFormat: false },
    ),
  );
}

export function authHeaders(
  config: Pick<ConnectAPIConfig, "bearerToken">,
): HeadersInit | undefined {
  return config.bearerToken
    ? { Authorization: `Bearer ${config.bearerToken}` }
    : undefined;
}

export function handleAuthError(
  config: Pick<ConnectAPIConfig, "serverId" | "onAuthenticationRequired">,
  err: unknown,
): never {
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

export async function withAuth<T>(
  config: Pick<ConnectAPIConfig, "serverId" | "onAuthenticationRequired">,
  operation: () => Promise<T>,
): Promise<T> {
  try {
    return await operation();
  } catch (err) {
    handleAuthError(config, err);
  }
}

export function isConnectCode(err: unknown, code: Code): boolean {
  return err instanceof ConnectError && err.code === code;
}

export { Code, ConnectError };
