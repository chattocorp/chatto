import { afterAll, afterEach, beforeAll, expect, it, vi } from 'vitest';
import type { CurrentUser } from './api/viewer.js';

vi.mock('./api/server.js', async (original) => ({
  ...(await original<typeof import('./api/server.js')>()),
  getPublicServerInfo: vi.fn(async () => ({ name: 'Bot server', version: '0.5.0' }))
}));
vi.mock('./api/viewer.js', async (original) => ({
  ...(await original<typeof import('./api/viewer.js')>()),
  getCurrentUserViaConnect: vi.fn(async () => ({ id: 'bot', login: 'bot' }) as CurrentUser)
}));

import { connectChatto } from './connect.js';
import { serverRegistry } from './server/registry.js';
import { setRealtimeSocketFactoryForTests } from './server/realtimeTransport.js';
import { inertRealtimeSocket } from './testing/inertSocket.js';

beforeAll(() => setRealtimeSocketFactoryForTests(inertRealtimeSocket));
afterAll(() => setRealtimeSocketFactoryForTests(null));

afterEach(() => localStorage.clear());

function storedValues(): string {
  return Array.from({ length: localStorage.length }, (_, index) => {
    const key = localStorage.key(index)!;
    return `${key}=${localStorage.getItem(key)}`;
  }).join('\n');
}

it('never writes a fixed token to device storage', async () => {
  const connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'secret-key' });
  await connection.ready();
  serverRegistry.handleAuthenticationRequired(connection.serverId);
  expect(storedValues()).not.toContain('secret-key');
  connection.close();
  expect(storedValues()).not.toContain('secret-key');
});

function storedKeys(): string[] {
  return Array.from({ length: localStorage.length }, (_, index) => localStorage.key(index)!);
}

it('leaves no per-server records behind after close', async () => {
  const before = new Set(storedKeys());
  for (let index = 0; index < 3; index++) {
    const connection = connectChatto({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await connection.ready();
    connection.close();
  }
  const added = storedKeys().filter((key) => !before.has(key) && key.includes('authentication'));
  expect(added).toEqual([]);
});

it("refuses the page's own origin, which uses its cookie session", () => {
  expect(() => connectChatto({ serverUrl: location.origin, apiKey: 'key' })).toThrow(
    "page's own origin"
  );
  expect(storedKeys()).toEqual([]);
});
