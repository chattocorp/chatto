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
import { MethodIdempotency, type ServiceType } from '@bufbuild/protobuf';
import { presentableDisplayNamesInterceptor } from './presentableUsers.js';

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
  /**
   * Called when a request of a session that cannot renew itself is rejected
   * as unauthenticated. `source` names the request for diagnostics. The
   * error still reaches the caller.
   */
  onAuthenticationRequired?: (source: string) => void;
  /** Current private-data generation for this exact connection. */
  dataGeneration?: () => number;
  /**
   * Called with the final network outcome of each unary request: `true` when
   * the server answered, also with an error, and `false` when the request
   * failed on the network. A request that the caller canceled reports nothing.
   */
  onRequestOutcome?: (reachable: boolean) => void;
  /**
   * Build the transport from Chatto's interceptors instead of using HTTP.
   * Tests pass an in-memory fake server; see `@chatto/client/testing/fakeServer`.
   */
  transport?: (interceptors: Interceptor[]) => Transport;
};

/** An obsolete response was discarded. No response data escapes this boundary. */
export class StaleResponseError extends ConnectError {
  constructor(readonly mutationSucceeded: boolean) {
    super('Response discarded after a permission reset', Code.Canceled);
  }

  /**
   * `ConnectError` accepts any error that looks like a `ConnectError`, and a
   * subclass inherits that check. Restore the normal prototype check, so that
   * `instanceof StaleResponseError` does not match every `ConnectError`.
   */
  static override [Symbol.hasInstance](value: unknown): boolean {
    return Function.prototype[Symbol.hasInstance].call(StaleResponseError, value);
  }
}

/** Fence reads and mutation results, including responses from uncancellable requests. */
export function dataGenerationInterceptor(current: () => number): Interceptor {
  return (next) => async (request) => {
    const generation = current();
    const response = await next(request);
    if (generation !== current()) {
      throw new StaleResponseError(!isReadMethod(request.method));
    }
    return response;
  };
}

/** Whether a method only reads, so a repeated request cannot change server data. */
function isReadMethod(method: { name: string; idempotency?: MethodIdempotency }): boolean {
  return (
    method.idempotency === MethodIdempotency.NoSideEffects ||
    /^(Get|List|BatchGet|Search|Check|Fetch|Resolve|Find)/.test(method.name)
  );
}

/**
 * Whether a request failed on the network: the server was not reachable, a
 * proxy could not reach it, or the request timed out. Another attempt can
 * succeed without a change by the user.
 */
export function isNetworkError(error: unknown): boolean {
  // `fetch` rejects with a TypeError when the network fails.
  if (error instanceof TypeError) return true;
  if (!(error instanceof ConnectError)) return false;
  return (
    error.code === Code.Unavailable ||
    error.code === Code.DeadlineExceeded ||
    // Connect wraps a fetch network failure as Unknown with a TypeError cause.
    (error.code === Code.Unknown && error.cause instanceof TypeError)
  );
}

/**
 * Call-context flag for a caller that repeats its own failed requests.
 * Set it with {@link skipNetworkRetry}.
 */
const skipNetworkRetryKey = createContextKey(false, {
  description: 'skip the repeat of a read that failed on the network'
});

/**
 * Call options for a read that {@link networkRetryInterceptor} must not
 * repeat. Use it only when the caller repeats the request itself, so the two
 * retry limits do not multiply.
 */
export function skipNetworkRetry(): { contextValues: ContextValues } {
  return { contextValues: createContextValues().set(skipNetworkRetryKey, true) };
}

/** Delays before the second and third attempt of a read that failed on the network. */
const NETWORK_RETRY_DELAYS_MS = [500, 1_500];

/** Wait for `ms`, or until `signal` aborts. */
function delayOrAbort(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const finish = () => {
      clearTimeout(timer);
      signal.removeEventListener('abort', finish);
      resolve();
    };
    const timer = setTimeout(finish, ms);
    signal.addEventListener('abort', finish);
  });
}

/**
 * Repeat a unary read that failed on the network, and report the final
 * network outcome of each unary request to `onRequestOutcome`.
 *
 * - Only reads repeat. A repeated mutation could apply twice, for example as
 *   a second message. {@link skipNetworkRetry} stops the repeat of a read.
 * - A request with `timeoutMs` has one deadline for all attempts. A request
 *   that reached its deadline does not repeat.
 * - Each delay has jitter, so clients do not repeat in step after an outage.
 *
 * Install it as the innermost interceptor, so each attempt is one transport
 * request and the outer interceptors see only the final result.
 */
export function networkRetryInterceptor(
  config: Pick<ConnectAPIConfig, 'onRequestOutcome'>,
  retryDelaysMs: readonly number[] = NETWORK_RETRY_DELAYS_MS
): Interceptor {
  return (next) => async (request) => {
    if (request.stream) return next(request);
    for (let attempt = 0; ; attempt++) {
      try {
        const response = await next(request);
        config.onRequestOutcome?.(true);
        return response;
      } catch (error) {
        const { signal } = request;
        // `fetch` rejects an aborted request with an AbortError. The signal's
        // reason tells a reached deadline from a cancel by the caller.
        if (!isNetworkError(signal.aborted ? signal.reason : error)) {
          // A server error is still an answer. A cancel proves nothing.
          if (!signal.aborted) config.onRequestOutcome?.(true);
          throw error;
        }
        const delay = retryDelaysMs[attempt];
        if (
          signal.aborted ||
          delay === undefined ||
          !isReadMethod(request.method) ||
          request.contextValues.get(skipNetworkRetryKey)
        ) {
          config.onRequestOutcome?.(false);
          throw error;
        }
        await delayOrAbort(delay * (0.75 + Math.random() * 0.5), signal);
        if (signal.aborted) {
          if (isNetworkError(signal.reason)) config.onRequestOutcome?.(false);
          throw signal.reason;
        }
      }
    }
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
 * Report a rejected session that cannot renew itself.
 *
 * Cookie and fixed-token sessions have no other way to recover from
 * `Unauthenticated`. The hook decides whether the session needs a new
 * sign-in; the registry first confirms a rejected cookie session. A renewable bearer session is different: the bearer
 * interceptor refreshes it, and only a rejected refresh grant marks it for
 * reauthentication. The error always reaches the caller.
 */
export function authenticationRequiredInterceptor(
  config: Pick<ConnectAPIConfig, 'onAuthenticationRequired' | 'renewBearerToken'>
): Interceptor {
  return (next) => async (request) => {
    try {
      return await next(request);
    } catch (err) {
      if (
        err instanceof ConnectError &&
        err.code === Code.Unauthenticated &&
        config.onAuthenticationRequired &&
        !config.renewBearerToken &&
        !request.contextValues.get(skipAuthenticationRequiredKey)
      ) {
        config.onAuthenticationRequired(`${request.service.typeName}/${request.method.name}`);
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
  options: {
    useBinaryFormat?: boolean;
    /** Fetch implementation for hosts that supply their own networking. */
    fetch?: typeof globalThis.fetch;
  } = {}
): Transport {
  const interceptors = [
    // Outermost, so it sees errors from every inner interceptor.
    authenticationRequiredInterceptor(config),
    ...(config.dataGeneration ? [dataGenerationInterceptor(config.dataGeneration)] : []),
    bearerRenewalInterceptor(config),
    presentableDisplayNamesInterceptor,
    // Innermost, so each attempt is one transport request.
    networkRetryInterceptor(config)
  ];
  if (config.transport) return config.transport(interceptors);
  // Connect sends every request with `redirect: 'error'`, so a token and
  // request data reach only the configured server.
  return createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: options.useBinaryFormat ?? true,
    interceptors,
    ...(options.fetch ? { fetch: options.fetch } : {})
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

/** Create a credential-free public client with GET for side-effect-free RPCs. */
export function createPublicChattoClient<T extends ServiceType>(
  service: T,
  baseUrl: string
): Client<T> {
  return createClient(
    service,
    createConnectTransport({
      baseUrl: connectEndpoint(baseUrl),
      useBinaryFormat: false,
      // The connection floor already excludes 0.4 servers, which reject GET discovery.
      useHttpGet: true,
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
