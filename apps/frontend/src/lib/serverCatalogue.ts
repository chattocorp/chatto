/**
 * The frontend's server catalogue policy: how servers join the catalogue,
 * which server is the origin, and which server to show. The client keeps the
 * servers, their sessions, and their stores; this module adds the frontend's
 * policy on top of the client's public registry API (ADR-112). Until phase 2
 * of ADR-112, the registry still stores the device-local server list.
 */

import { getPublicServerInfo, type PublicServerInfo } from '@chatto/client/api/server';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { generateServerId } from '@chatto/client/server/serverIds';
import { isBackendCapableOrigin } from '@chatto/client/util/runtimeOrigin';
import { serverRegistry } from '$lib/client';
import { canonicalServerOrigin } from '$lib/serverUrl';

/** Public data that names a server in the catalogue. */
type ServerProfile = Pick<PublicServerInfo, 'name'> & { iconUrl?: string | null };

/** Discovery of the origin that runs, so concurrent callers share it. */
let originDiscovery: Promise<void> | null = null;

/**
 * Add a server without a session and return its ID, or `undefined` when the
 * registry refused it, for example after the client closed.
 */
function addServer(url: string, profile: ServerProfile): string | undefined {
  const id = generateServerId(
    url,
    serverRegistry.servers.map((server) => server.id)
  );
  serverRegistry.addServer({
    id,
    url,
    name: profile.name || 'Chatto',
    iconUrl: profile.iconUrl ?? null,
    addedAt: Date.now()
  });
  return serverRegistry.getServer(id) ? id : undefined;
}

/**
 * Register the server that serves this page as the origin server, unless it
 * is registered or the page has no Chatto backend.
 *
 * - `signedIn`: the origin has a signed-in user, so it is a Chatto server. The
 *   store's discovery loads its name later.
 * - `serverInfo`: public data that the caller already loaded from the origin.
 *
 * Without either, this asks the origin for its public data and registers the
 * origin only when it answers as a Chatto server.
 */
export async function registerOriginServer({
  signedIn = false,
  serverInfo,
  protocol
}: {
  signedIn?: boolean;
  serverInfo?: PublicServerInfo | null;
  /** The page's protocol; the default is `window.location.protocol`. */
  protocol?: string;
} = {}): Promise<void> {
  // Without a page, for example in Node tests, there is no origin server.
  if (typeof window === 'undefined') return;
  // The registry finds the origin server by `window.location.origin`.
  const { origin } = window.location;
  if (!isBackendCapableOrigin({ protocol: protocol ?? window.location.protocol })) return;
  if (serverRegistry.originServer) return;

  if (signedIn) {
    addServer(origin, { name: 'Chatto' });
    return;
  }
  if (serverInfo) {
    addServer(origin, serverInfo);
    serverRegistry.settleOriginUnauthenticated();
    return;
  }

  originDiscovery ??= getPublicServerInfo(origin)
    .then((info) => {
      // Another caller can register the origin while the request runs.
      if (serverRegistry.originServer) return;
      addServer(origin, info);
      serverRegistry.settleOriginUnauthenticated();
    })
    .catch(() => {
      // The origin is not a Chatto server, for example static hosting.
    })
    .finally(() => {
      originDiscovery = null;
    });
  await originDiscovery;
}

/**
 * The registered server that `url` addresses, compared by canonical origin
 * (see `canonicalServerOrigin`). `undefined` for a URL that is not an HTTP(S)
 * server URL.
 */
export function findServerByUrl(url: string): RegisteredServer | undefined {
  const origin = canonicalServerOrigin(url);
  if (!origin) return undefined;
  return serverRegistry.servers.find((server) => canonicalServerOrigin(server.url) === origin);
}

/**
 * Add a remote server without a session and return its ID, for the Server
 * Directory's join. Returns the ID of a registered server with the same URL
 * and leaves it unchanged. The server stays signed out until the user signs
 * in from its signed-out view; a later sign-in adds its session to this
 * registration.
 *
 * @throws Error when the registry refuses the server.
 */
export function addSignedOutServer(url: string, profile: ServerProfile): string {
  const existing = findServerByUrl(url);
  if (existing) return existing.id;
  const id = addServer(url, profile);
  if (!id) throw new Error('The server could not be registered.');
  return id;
}

/**
 * Choose a server to show after sign-out or from chat-wide pages: the origin
 * when it has a signed-in user, otherwise the first server with one.
 * `excludedId` skips a server, such as the one that is signing out.
 */
export function firstAuthenticatedServerId(excludedId?: string): string | undefined {
  const originId = serverRegistry.originServer?.id;
  if (originId && originId !== excludedId && serverRegistry.isAuthenticated(originId)) {
    return originId;
  }
  return serverRegistry.servers.find(
    (server) => server.id !== excludedId && serverRegistry.isAuthenticated(server.id)
  )?.id;
}
