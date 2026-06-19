import { beforeEach, describe, expect, it, vi } from 'vitest';

const { registry } = vi.hoisted(() => ({
  registry: (() => {
    const mock = {
      originServer: null as { id: string; token: string | null } | null,
      isOriginServer: vi.fn(),
      getServer: vi.fn()
    };
    mock.isOriginServer.mockImplementation((id: string) => id === mock.originServer?.id);
    return mock;
  })()
}));

vi.mock('./registry.svelte', () => ({
  serverRegistry: registry
}));

import { ServerConnection, httpToWireUrl, serverConnectionManager } from './serverConnection.svelte';

describe('server connection URL mapping', () => {
  it('maps instance base URLs to wire endpoints', () => {
    expect(httpToWireUrl('https://chat.example.com')).toBe('https://chat.example.com/api/wire');
    expect(httpToWireUrl('https://chat.example.com/')).toBe('https://chat.example.com/api/wire');
  });

  it('maps legacy GraphQL endpoints to wire endpoints', () => {
    expect(httpToWireUrl('https://chat.example.com/api/graphql')).toBe(
      'https://chat.example.com/api/wire'
    );
  });
});

describe('ServerConnection', () => {
  it('exposes wire endpoint metadata without opening a socket', () => {
    const connection = new ServerConnection({
      wireUrl: 'https://chat.example.com/api/wire',
      token: 'token-1',
      serverId: 'server-1',
      label: 'chat.example.com'
    });

    expect(connection.wireUrl).toBe('https://chat.example.com/api/wire');
    expect(connection.token).toBe('token-1');
    expect(connection.status).toBe('connecting');

    connection.dispose();
  });
});

describe('serverConnectionManager', () => {
  beforeEach(() => {
    registry.originServer = null;
    registry.getServer.mockReset();
    registry.isOriginServer.mockImplementation((id: string) => id === registry.originServer?.id);
  });

  it('creates the origin connection with the current origin token', () => {
    registry.originServer = { id: 'origin', token: 'origin-token' };

    const connection = serverConnectionManager.originClient;

    expect(connection.wireUrl).toBe('/api/wire');
    expect(connection.token).toBe('origin-token');
  });

  it('creates remote connections from registered server base URLs', () => {
    registry.originServer = { id: 'origin', token: null };
    registry.getServer.mockReturnValue({
      id: 'remote-1',
      url: 'https://remote.example.test',
      token: 'remote-token'
    });

    const connection = serverConnectionManager.getClient('remote-1');

    expect(connection.wireUrl).toBe('https://remote.example.test/api/wire');
    expect(connection.token).toBe('remote-token');
  });
});
