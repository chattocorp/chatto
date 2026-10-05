/**
 * The frontend's Chatto client: the servers in device storage, the origin
 * server with its cookie session, and one live server at a time, the server
 * in the URL (see `ServerRuntimeCoordinator`).
 *
 * The frontend has exactly one client. Import it, or its parts, from here.
 */

import { createClient } from '@chatto/client';
import { connectQueryCaches } from '$lib/query/cacheRegistry';
import { serverUi } from '$lib/state/server/serverUi';

export const client = createClient({
  storage: 'device',
  originServer: true,
  liveServers: 'selected'
});

// Create each store's UI state when the registry creates the store, outside
// reactive reads, so that Svelte tracks the voice call's `$state`. Keep the
// cached reads of each store within its privacy boundaries.
client.registry.watchStores((store) => {
  serverUi(store);
  return connectQueryCaches(store);
});

// A module replacement creates a new client; release the old one first, so
// that it gives up the device storage and the origin server.
if (import.meta.hot) import.meta.hot.dispose(() => client.close());

/** The client's servers, sessions, and stores. */
export const serverRegistry = client.registry;
/** The client's server connections. */
export const serverConnectionManager = client.connections;
/** The client's event buses and realtime transports. */
export const eventBusManager = client.realtime;
