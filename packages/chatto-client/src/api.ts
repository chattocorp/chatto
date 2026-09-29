/**
 * Stateless, token-authenticated access to Chatto's public ConnectRPC API.
 *
 * Use it for short-lived work, such as a webhook handler, and for work that
 * can outlive a connection. It needs no retained state or realtime
 * connection, and it does not load the stores. For a live connection with
 * reactive stores, use `ChattoClient.connect`.
 *
 * Requests use Connect JSON and contact only the configured server, which
 * receives the host's IP address, the token, and the request data. Redirects
 * are rejected, so the token cannot reach another origin. The API never
 * retries requests.
 */

import type { ServiceType } from '@bufbuild/protobuf';
import { createClient, type Client } from '@connectrpc/connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { connectEndpoint, createChattoTransport } from './api/connect.js';
import { MessagingRequests } from './messaging/requests.js';
import type { RequestOptions } from './messaging/types.js';
import { parseServerUrl } from './util/serverUrl.js';

/** Settings for {@link createApi}. */
export interface ApiOptions {
  /** HTTP or HTTPS origin of the Chatto server, without credentials. */
  serverUrl: string;
  /** Bearer token, for example a bot API key. */
  apiKey: string;
  /** Fetch implementation for tests or host-specific networking. */
  fetch?: typeof globalThis.fetch;
  /**
   * The account of the key, when the host already knows it, for example
   * from `Connection.ready`. `ready()` then returns it without a request.
   */
  viewerId?: string;
}

/**
 * Typed access to Chatto's public services, with the same request helpers as
 * a connection: messages, threads, reactions, typing, and addressing.
 */
export class Api extends MessagingRequests {
  /** Origin of the server. */
  readonly serverUrl: string;
  readonly #service: <T extends ServiceType>(service: T) => Client<T>;
  readonly #viewerId: (options: RequestOptions) => Promise<string>;

  /** Use {@link createApi}. */
  constructor(
    serverUrl: string,
    service: <T extends ServiceType>(service: T) => Client<T>,
    knownViewerId?: string
  ) {
    const viewerId = knownViewerId
      ? async ({ signal }: RequestOptions) => {
          signal?.throwIfAborted();
          return knownViewerId;
        }
      : cachedViewerId(service);
    super({ service }, viewerId);
    this.serverUrl = serverUrl;
    this.#service = service;
    this.#viewerId = viewerId;
  }

  /** Create a typed Connect client for a public service. */
  service<T extends ServiceType>(service: T): Client<T> {
    return this.#service(service);
  }

  /**
   * Wait until the server accepted the key, and return its account, as
   * `Connection.ready` does. The first call reads the viewer from the
   * server; later calls reuse it. A failed read is not kept, so a later call
   * tries again.
   */
  async ready(options: RequestOptions = {}): Promise<{ viewerId: string }> {
    return { viewerId: await this.#viewerId(options) };
  }
}

/**
 * Read the viewer once and keep a successful result. Callers share one
 * request, so it has no caller's signal; each caller stops waiting when its
 * own signal aborts.
 */
function cachedViewerId(
  service: <T extends ServiceType>(service: T) => Client<T>
): (options: RequestOptions) => Promise<string> {
  let viewer: Promise<string> | undefined;
  return ({ signal }) => {
    signal?.throwIfAborted();
    viewer ??= service(ViewerService)
      .getViewer({}, { timeoutMs: 10_000 })
      .then((response) => {
        const id = response.user?.profile?.id;
        if (!id) throw new Error('Chatto did not return the viewer');
        return id;
      })
      .catch((error: unknown) => {
        viewer = undefined;
        throw error;
      });
    return untilAborted(viewer, signal);
  };
}

/** Settle like `promise`, or reject with the abort reason when `signal` aborts first. */
function untilAborted<T>(promise: Promise<T>, signal: AbortSignal | undefined): Promise<T> {
  if (!signal) return promise;
  return new Promise((resolve, reject) => {
    const abort = () => reject(signal.reason);
    signal.addEventListener('abort', abort, { once: true });
    promise.then(resolve, reject).finally(() => signal.removeEventListener('abort', abort));
  });
}

/** Create a stateless API client with a fixed bearer token. */
export function createApi(options: ApiOptions): Api {
  const url = parseServerUrl(options.serverUrl);
  if (!options.apiKey) throw new Error('A Chatto API key is required');
  const request = options.fetch ?? globalThis.fetch;
  const transport = createChattoTransport(
    { baseUrl: connectEndpoint(url.origin), bearerToken: options.apiKey },
    {
      useBinaryFormat: false,
      fetch: (input, init) => request(input, { ...init, redirect: 'error' })
    }
  );
  return new Api(url.origin, (service) => createClient(service, transport), options.viewerId);
}
