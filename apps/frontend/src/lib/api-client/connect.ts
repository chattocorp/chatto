import {
  Code,
  ConnectError,
  createClient,
  type Client,
  type Interceptor,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import type { ServiceType } from "@bufbuild/protobuf";
import { notifyAuthenticationRequired } from "./hooks.js";

export type ConnectAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  /** Return the latest access token, rotating it when force is true or expiry is near. */
  renewBearerToken?: (force: boolean) => Promise<string | null>;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type PublicConnectAPIConfig = {
  baseUrl: string;
};

export function connectEndpoint(baseUrl: string): string {
  return new URL("/api/connect", baseUrl).toString();
}

export function createChattoTransport(
  config: { baseUrl: string } & Partial<ConnectAPIConfig>,
  options: { useBinaryFormat?: boolean } = {},
): Transport {
  return createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: options.useBinaryFormat ?? true,
    interceptors: config.renewBearerToken ? [bearerRenewalInterceptor(config)] : undefined,
  });
}

export function createChattoClient<T extends ServiceType>(
  service: T,
  config: { baseUrl: string } & Partial<ConnectAPIConfig>,
): Client<T> {
  return createClient(service, createChattoTransport(config));
}

export function bearerRenewalInterceptor(
  config: {
    serverId?: string;
    bearerToken?: string | null;
    renewBearerToken?: (force: boolean) => Promise<string | null>;
  }
): Interceptor {
  return (next) => async (request) => {
    const setAccessToken = (token: string | null) => {
      if (token) request.header.set('Authorization', `Bearer ${token}`);
      else request.header.delete('Authorization');
    };

    const currentToken = config.renewBearerToken
      ? await config.renewBearerToken(false)
      : config.bearerToken ?? null;
    setAccessToken(currentToken);
    try {
      return await next(request);
    } catch (error) {
      if (
        request.stream ||
        !(error instanceof ConnectError) ||
        error.code !== Code.Unauthenticated ||
        !config.renewBearerToken
      ) {
        throw error;
      }

      const renewedToken = await config.renewBearerToken(true);
      if (!renewedToken) throw error;
      setAccessToken(renewedToken);
      try {
        return await next(request);
      } catch (retryError) {
        if (
          retryError instanceof ConnectError &&
          retryError.code === Code.Unauthenticated &&
          config.serverId
        ) {
          notifyAuthenticationRequired(config.serverId);
        }
        throw retryError;
      }
    }
  };
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
