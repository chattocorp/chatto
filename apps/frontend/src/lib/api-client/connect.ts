import {
  Code,
  ConnectError,
  createClient,
  type Client,
  type Interceptor,
  type Transport
} from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { ServiceType } from '@bufbuild/protobuf';
import { notifyAuthenticationRequired } from './hooks.js';

/** Request header for a read that must include an accepted realtime boundary. */
export const REALTIME_MINIMUM_CURSOR_HEADER = 'Chatto-Realtime-Minimum-Cursor';

export type ConnectAPIConfig = {
  serverId?: string;
  /** Opaque connection scope for session-owned resource queries. */
  queryScope?: string;
  baseUrl: string;
  bearerToken: string | null;
  /** Return the latest access token, rotating it when force is true or expiry is near. */
  renewBearerToken?: (force: boolean) => Promise<string | null>;
  onAuthenticationRequired?: (serverId: string) => void;
  /** Current private-data generation for this exact connection. */
  dataGeneration?: () => number;
  /** Hold private reads and reject private actions until a restored viewer is verified. */
  beforePrivateRequest?: (methodName: string, signal: AbortSignal) => Promise<void>;
};

/** An obsolete response was discarded. No response data escapes this boundary. */
export class StaleResponseError extends ConnectError {
  constructor(readonly mutationSucceeded: boolean) {
    super('Response discarded after a permission reset', Code.Canceled);
  }
}

/** Fence reads and mutation results, including responses from uncancellable requests. */
export function dataGenerationInterceptor(current: () => number): Interceptor {
  return (next) => async (request) => {
    const generation = current();
    const response = await next(request);
    if (generation !== current()) {
      const read = /^(Get|List|BatchGet|Search|Check|Fetch|Resolve|Find)/.test(request.method.name);
      throw new StaleResponseError(!read);
    }
    return response;
  };
}

/** Let viewer verification through while a restored chat view holds other private calls. */
export function privateRequestInterceptor(
  beforeRequest: NonNullable<ConnectAPIConfig['beforePrivateRequest']>
): Interceptor {
  return (next) => async (request) => {
    if (request.service.typeName !== 'chatto.api.v1.ViewerService' ||
      request.method.name !== 'GetViewer') {
      await beforeRequest(request.method.name, request.signal);
    }
    return next(request);
  };
}

export type PublicConnectAPIConfig = {
  baseUrl: string;
};

export function connectEndpoint(baseUrl: string): string {
  return new URL('/api/connect', baseUrl).toString();
}

export function createChattoTransport(
  config: { baseUrl: string } & Partial<ConnectAPIConfig>,
  options: { useBinaryFormat?: boolean } = {}
): Transport {
  return createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: options.useBinaryFormat ?? true,
    interceptors:
      config.dataGeneration || config.renewBearerToken || config.beforePrivateRequest
        ? [
            // The verification gate must run before the response guard captures its generation.
            ...(config.beforePrivateRequest ? [privateRequestInterceptor(config.beforePrivateRequest)] : []),
            ...(config.dataGeneration ? [dataGenerationInterceptor(config.dataGeneration)] : []),
            ...(config.renewBearerToken ? [bearerRenewalInterceptor(config)] : [])
          ]
        : undefined
  });
}

export function createChattoClient<T extends ServiceType>(
  service: T,
  config: { baseUrl: string } & Partial<ConnectAPIConfig>
): Client<T> {
  return createClient(service, createChattoTransport(config));
}

/** Refresh a bearer credential for unary requests without treating a later API 401 as revocation. */
export function bearerRenewalInterceptor(config: {
  serverId?: string;
  bearerToken?: string | null;
  renewBearerToken?: (force: boolean) => Promise<string | null>;
}): Interceptor {
  return (next) => async (request) => {
    const setAccessToken = (token: string | null) => {
      if (token) request.header.set('Authorization', `Bearer ${token}`);
      else request.header.delete('Authorization');
    };

    const currentToken = config.renewBearerToken
      ? await config.renewBearerToken(false)
      : (config.bearerToken ?? null);
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
      // A successful refresh proves the renewable session was accepted.
      // A second API rejection can be transient or method-specific; only a
      // rejected refresh grant proves that this pair needs a new sign-in.
      return next(request);
    }
  };
}

export function createPublicChattoClient<T extends ServiceType>(
  service: T,
  baseUrl: string
): Client<T> {
  return createClient(
    service,
    createConnectTransport({
      baseUrl: connectEndpoint(baseUrl),
      useBinaryFormat: false,
      fetch: (input, init) =>
        fetch(input, {
          ...init,
          credentials: 'omit',
          redirect: 'error',
          referrerPolicy: 'no-referrer'
        })
    })
  );
}

export function authHeaders(
  config: Pick<ConnectAPIConfig, 'bearerToken'>
): HeadersInit | undefined {
  return config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;
}

/** Request sign-in for cookie or missing bearer credentials; renewable grants decide their own validity. */
export function handleAuthError(
  config: Pick<ConnectAPIConfig, 'serverId' | 'onAuthenticationRequired' | 'renewBearerToken'>,
  err: unknown
): never {
  if (err instanceof ConnectError && err.code === Code.Unauthenticated &&
    config.serverId && !config.renewBearerToken) {
    notifyAuthenticationRequired(config.serverId, config.onAuthenticationRequired);
  }
  throw err;
}

export function isConnectCode(err: unknown, code: Code): boolean {
  return err instanceof ConnectError && err.code === code;
}

export { Code, ConnectError };
