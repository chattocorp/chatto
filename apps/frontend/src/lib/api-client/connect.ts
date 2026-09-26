import {
  Code,
  ConnectError,
  createClient,
  createContextKey,
  createContextValues,
  type Client,
  type ContextValues,
  type Interceptor,
  type Transport
} from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { ServiceType } from '@bufbuild/protobuf';
import { notifyAuthenticationRequired } from './hooks.js';

/** Request header for a read that must include an accepted realtime boundary. */
export const REALTIME_MINIMUM_CURSOR_HEADER = 'Chatto-Realtime-Minimum-Cursor';

/** Request headers that make a read include the given realtime cursor, if any. */
export function minimumCursorHeaders(minimumCursor?: string): Headers | undefined {
  return minimumCursor
    ? new Headers({ [REALTIME_MINIMUM_CURSOR_HEADER]: minimumCursor })
    : undefined;
}

export type ConnectAPIConfig = {
  serverId?: string;
  /** Opaque connection scope for session-owned resource queries. */
  queryScope?: string;
  baseUrl: string;
  bearerToken: string | null;
  /** Return the latest access token, rotating it when force is true or expiry is near. */
  renewBearerToken?: (force: boolean) => Promise<string | null>;
  /** Current private-data generation for this exact connection. */
  dataGeneration?: () => number;
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

/**
 * Call-context flag for a caller that owns its own reaction to `Unauthenticated`.
 * Set it with {@link skipAuthenticationRequired}.
 */
const skipAuthenticationRequiredKey = createContextKey(false, {
  description: 'skip the authentication-required notification'
});

/**
 * Call options for a request whose `Unauthenticated` result must not request a
 * new sign-in. Use it only when the caller makes that decision itself.
 */
export function skipAuthenticationRequired(): { contextValues: ContextValues } {
  return { contextValues: createContextValues().set(skipAuthenticationRequiredKey, true) };
}

/**
 * Request a new sign-in when a session that cannot renew itself is rejected.
 *
 * Cookie and fixed-token sessions have no other way to recover from
 * `Unauthenticated`. A renewable bearer session is different: the bearer
 * interceptor refreshes it, and only a rejected refresh grant marks it for
 * reauthentication. The error always reaches the caller.
 */
export function authenticationRequiredInterceptor(
  config: Pick<ConnectAPIConfig, 'serverId' | 'renewBearerToken'>
): Interceptor {
  return (next) => async (request) => {
    try {
      return await next(request);
    } catch (err) {
      if (
        err instanceof ConnectError &&
        err.code === Code.Unauthenticated &&
        config.serverId &&
        !config.renewBearerToken &&
        !request.contextValues.get(skipAuthenticationRequiredKey)
      ) {
        notifyAuthenticationRequired(config.serverId);
      }
      throw err;
    }
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
    interceptors: [
      // Outermost, so it sees errors from every inner interceptor.
      authenticationRequiredInterceptor(config),
      ...(config.dataGeneration ? [dataGenerationInterceptor(config.dataGeneration)] : []),
      bearerRenewalInterceptor(config)
    ]
  });
}

export function createChattoClient<T extends ServiceType>(
  service: T,
  config: { baseUrl: string } & Partial<ConnectAPIConfig>
): Client<T> {
  return createClient(service, createChattoTransport(config));
}

/**
 * Set the request's bearer credential. The transport owns the Authorization
 * header: it replaces or removes any value that a caller sets. A cookie session
 * sends none. A renewable session gets a current token before each request, and
 * a unary request retries once after a forced renewal. A later API 401 is not
 * treated as revocation.
 */
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

export function isConnectCode(err: unknown, code: Code): boolean {
  return err instanceof ConnectError && err.code === code;
}

export { Code, ConnectError };
