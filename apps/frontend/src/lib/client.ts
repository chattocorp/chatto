/**
 * The frontend's Chatto client: the servers in device storage, the origin
 * server with its cookie session, and one live server at a time, the server
 * in the URL (see `ServerRuntimeCoordinator`).
 *
 * The frontend has exactly one client. Import it, or its parts, from here.
 */

import { createClient } from '@chatto/client/client';
import { voiceCallFactory } from '$lib/state/server/voiceCallRegistration';

export const client = createClient({
  storage: 'device',
  originServer: true,
  liveServers: 'selected',
  voiceCall: voiceCallFactory
});

/** The client's servers, sessions, and stores. */
export const serverRegistry = client.registry;
/** The client's server connections. */
export const serverConnectionManager = client.connections;
/** The client's event buses and realtime transports. */
export const eventBusManager = client.realtime;
