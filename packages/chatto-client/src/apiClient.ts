/**
 * Stateless, token-authenticated access to Chatto's public ConnectRPC API.
 *
 * Use it for short-lived work, such as a webhook handler, that needs typed
 * requests but no retained state or realtime connection. For a live
 * connection with reactive stores, use `connectChatto`.
 *
 * Requests use Connect JSON and contact only the configured server, which
 * receives the host's IP address, the token, and the request data. Redirects
 * are rejected, so the token cannot reach another origin. The client never
 * retries requests.
 */

import type { ServiceType } from '@bufbuild/protobuf';
import { createClient, type Client } from '@connectrpc/connect';
import { connectEndpoint, createChattoTransport } from './api/connect.js';

/** Settings for {@link createChattoApi}. */
export interface ChattoApiOptions {
  /** HTTP or HTTPS origin of the Chatto server, without credentials. */
  serverUrl: string;
  /** Bearer token, for example a bot API key. */
  apiKey: string;
  /** Fetch implementation for tests or host-specific networking. */
  fetch?: typeof globalThis.fetch;
}

/** Typed access to Chatto's public services. */
export interface ChattoApi {
  /** Origin of the server. */
  readonly serverUrl: string;
  /** Create a typed Connect client for a public service. */
  service<T extends ServiceType>(service: T): Client<T>;
}

/** Create a stateless API client with a fixed bearer token. */
export function createChattoApi(options: ChattoApiOptions): ChattoApi {
  let url: URL;
  try {
    url = new URL(options.serverUrl);
  } catch {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  if (!options.apiKey) throw new Error('A Chatto API key is required');
  const request = options.fetch ?? globalThis.fetch;
  const transport = createChattoTransport(
    { baseUrl: connectEndpoint(url.origin), bearerToken: options.apiKey },
    {
      useBinaryFormat: false,
      fetch: (input, init) => request(input, { ...init, redirect: 'error' })
    }
  );
  return {
    serverUrl: url.origin,
    service: (service) => createClient(service, transport)
  };
}
