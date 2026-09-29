import { afterEach, expect, it, vi } from 'vitest';
import type { PublicServerInfo } from './api/server.js';

const mocks = vi.hoisted(() => ({
  discovery: vi.fn<(url: string) => Promise<PublicServerInfo>>()
}));
vi.mock('./api/server.js', async (original) => ({
  ...(await original<typeof import('./api/server.js')>()),
  getPublicServerInfo: mocks.discovery
}));

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

it('lets no late work of a closed client register servers or write device storage', async () => {
  let respond!: (info: PublicServerInfo) => void;
  mocks.discovery.mockReturnValueOnce(new Promise((resolve) => (respond = resolve)));
  const client = createAppClient();
  const probe = client.registry.probeOrigin();
  await vi.waitFor(() => expect(mocks.discovery).toHaveBeenCalled());
  client.close();
  const before = stored();

  respond({ name: 'Origin', version: '0.5.0' } as PublicServerInfo);
  await probe;

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
