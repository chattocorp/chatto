import { afterEach, expect, it } from 'vitest';
import { isServerIdClaimed } from './server/serverIds.js';
import { createAppClient } from './testing/appClient.js';

afterEach(() => localStorage.clear());

/** Every stored item, as `key=value` lines. */
function stored(): string {
  return Array.from({ length: localStorage.length }, (_, index) => {
    const key = localStorage.key(index)!;
    return `${key}=${localStorage.getItem(key)}`;
  }).join('\n');
}

it('lets no late work of a closed client register servers or write device storage', () => {
  const client = createAppClient();
  client.close();
  const before = stored();

  // Work that was still running, such as a host's origin discovery, adds the
  // server after the client closed.
  client.registry.addServer({
    id: 'localhost',
    url: 'http://localhost:3000',
    name: 'Origin',
    iconUrl: null,
    addedAt: 1
  });

  expect(client.registry.servers).toEqual([]);
  expect(stored()).toBe(before);
  expect(isServerIdClaimed('localhost')).toBe(false);
});

it('claims no server ID when restoring device storage fails', async () => {
  const { claimServerId, releaseServerId } = await import('./server/serverIds.js');
  localStorage.setItem(
    'chatto:instances',
    JSON.stringify([
      { id: 'first', url: 'https://first.example', name: 'First', iconUrl: null, addedAt: 1 },
      { id: 'second', url: 'https://second.example', name: 'Second', iconUrl: null, addedAt: 2 }
    ])
  );
  const holder = { getServer: () => undefined };
  claimServerId('second', holder);
  try {
    expect(() => createAppClient()).toThrow('belongs to another Chatto client');
    expect(isServerIdClaimed('first')).toBe(false);
  } finally {
    releaseServerId('second', holder);
  }
  const restored = createAppClient();
  expect(restored.registry.servers.map((server) => server.id)).toEqual(['first', 'second']);
  restored.close();
});
