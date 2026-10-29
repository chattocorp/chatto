import { beforeEach, expect, it, vi } from 'vitest';
import { KnownServers } from './knownServers.js';
import { createClient } from '../client.js';

const profile = {
  name: 'Community',
  version: '0.5.0',
  description: 'Hello',
  iconUrl: null,
  bannerUrl: null
};

beforeEach(() => localStorage.clear());

it('persists only public fields, canonicalises addresses, and deduplicates', () => {
  const known = new KnownServers(localStorage);
  const full = { ...profile, token: 'not-persisted' };
  known.remember('https://community.example/path', full);
  known.remember('https://community.example/', { ...profile, name: 'Updated' });
  const restored = new KnownServers(localStorage);
  expect(restored.servers).toEqual([
    { origin: 'https://community.example', profile: { ...profile, name: 'Updated' } }
  ]);
  expect(localStorage.getItem('chatto:known-servers')).not.toContain('token');
  restored.forget('https://community.example/');
  expect(new KnownServers(localStorage).servers).toEqual([]);
});

it('ignores malformed stored entries without losing valid entries', () => {
  localStorage.setItem(
    'chatto:known-servers',
    JSON.stringify([
      null,
      {},
      { origin: 'javascript:alert(1)', profile },
      { origin: 'https://valid.example', profile }
    ])
  );
  expect(new KnownServers(localStorage).servers.map((server) => server.origin)).toEqual([
    'https://valid.example'
  ]);
  localStorage.setItem('chatto:known-servers', '{');
  expect(new KnownServers(localStorage).servers).toEqual([]);
  expect(() => new KnownServers().remember('https://user:secret@example.test', profile)).toThrow();
});

it('keeps discovery isolated from registrations and other client instances', () => {
  const first = createClient({});
  const second = createClient({});
  try {
    first.knownServers.remember('https://community.example', profile);
    expect(first.registry.servers).toEqual([]);
    expect(second.knownServers.servers).toEqual([]);
  } finally {
    first.close();
    second.close();
  }
});

it('keeps working when storage writes fail', () => {
  const storage = {
    getItem: () => null,
    setItem: vi.fn(() => {
      throw new Error('Full');
    })
  } as unknown as Storage;
  const known = new KnownServers(storage);
  known.remember('https://community.example', profile);
  expect(known.servers).toHaveLength(1);
});

it('restores known servers in a device client without restoring registrations', () => {
  const first = createClient({ storage: 'device' });
  first.knownServers.remember('https://community.example', profile);
  first.close();
  const restored = createClient({ storage: 'device' });
  try {
    expect(restored.knownServers.servers).toHaveLength(1);
    expect(restored.registry.servers).toEqual([]);
  } finally {
    restored.close();
  }
});
