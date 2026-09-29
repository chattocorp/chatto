import { createClient, type ClientOptions } from '../client.js';

/**
 * Create a client configured like the bundled frontend: device storage, the
 * origin server, and one live server. A test file can create one such client.
 */
export function createAppClient(options: ClientOptions = {}) {
  return createClient({
    storage: 'device',
    originServer: true,
    liveServers: 'selected',
    ...options
  });
}
