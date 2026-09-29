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
