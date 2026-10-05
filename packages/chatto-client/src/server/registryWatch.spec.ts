import { afterEach, describe, expect, it, vi } from 'vitest';
import { createClient, type ChattoClient } from '../client.js';

let client: ChattoClient;

afterEach(() => client.close());

function addServer(id: string) {
  client.registry.addServer({
    id,
    url: `https://${id}.example`,
    name: id,
    iconUrl: null,
    addedAt: 1
  });
  return client.registry.getStore(id);
}

describe('ServerRegistry.watchStores', () => {
  it('sets up current and later stores, and cleans up at disposal and stop', () => {
    client = createClient();
    const first = addServer('first');
    const cleanups: string[] = [];
    const setup = vi.fn((store: { serverId: string }) => () => cleanups.push(store.serverId));

    const stop = client.registry.watchStores(setup);
    expect(setup).toHaveBeenCalledWith(first);

    const second = addServer('second');
    expect(setup).toHaveBeenCalledWith(second);

    client.registry.removeServer('first');
    expect(cleanups).toEqual(['first']);

    stop();
    expect(cleanups).toEqual(['first', 'second']);
    addServer('third');
    expect(setup).toHaveBeenCalledTimes(2);
  });

  it('keeps watching when one setup throws', () => {
    client = createClient();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const later = vi.fn();
    client.registry.watchStores(() => {
      throw new Error('setup failed');
    });
    client.registry.watchStores(later);
    const store = addServer('only');
    expect(later).toHaveBeenCalledWith(store);
  });
});
