import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
import { WireClient } from '$lib/wire/client';

export async function withActiveServerWireClient<T>(
  callback: (client: WireClient) => Promise<T>
): Promise<T> {
  const serverId = getActiveServer();
  const shared = wireEventBusManager.getClient(serverId);
  if (shared) return callback(shared);

  const server = serverRegistry.getServer(serverId);
  if (!server) {
    throw new Error('No active server connection');
  }

  const client = new WireClient({
    url: new URL('/api/wire', server.url).toString(),
    token: server.token
  });
  try {
    return await callback(client);
  } finally {
    client.dispose();
  }
}
