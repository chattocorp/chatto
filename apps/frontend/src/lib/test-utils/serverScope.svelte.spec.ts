import { describe, expect, it } from 'vitest';
import { createTestServerScope, serverScopeModule } from './serverScope.svelte';

describe('createTestServerScope', () => {
  it('provides a loaded viewer, permissions, and a supported version by default', () => {
    const { scope } = createTestServerScope();

    expect(scope.serverId).toBe('server-1');
    expect(scope.store.accountId).toBe('viewer-1');
    expect(scope.store.viewerId).toBe('viewer-1');
    expect(scope.store.projectionViewerId).toBe('viewer-1');
    expect(scope.store.isAuthenticated).toBe(true);
    expect(scope.store.permissions.loaded).toBe(true);
    expect(scope.store.permissions.canManageServer).toBe(false);
    expect(scope.store.serverInfo.isSupportedVersion).toBe(true);
    expect(scope.isCurrent()).toBe(true);
  });

  it('applies options and reflects later state changes', () => {
    const server = createTestServerScope({
      serverId: 'server-2',
      queryScope: 'session-2',
      viewer: null,
      permissions: { canManageBots: true },
      isSupportedVersion: false
    });
    const { scope } = server;

    expect(scope.store.accountId).toBeNull();
    expect(scope.store.permissions.canManageBots).toBe(true);
    expect(scope.store.serverInfo.isSupportedVersion).toBe(false);

    expect(scope.serverId).toBe('server-2');
    expect(scope.store.serverId).toBe('server-2');
    expect(scope.connection.queryScope).toBe('session-2');
    expect(scope.connection.apiConfig.queryScope).toBe('session-2');

    server.permissions.canManageBots = false;
    server.current = false;

    expect(scope.store.permissions.canManageBots).toBe(false);
    expect(scope.isCurrent()).toBe(false);
  });

  it('returns the given API object, or runs the real factory with a stub config', () => {
    const api = { listBots: () => [] };
    expect(createTestServerScope({ api }).scope.connection.getAPI(() => 'real')).toBe(api);

    const { scope } = createTestServerScope();
    expect(scope.connection.getAPI((config) => config)).toMatchObject({
      serverId: 'server-1',
      queryScope: 'server-1-session',
      bearerToken: null
    });
  });

  it('keeps getters of extra store members', () => {
    let rooms = ['R1'];
    const { scope } = createTestServerScope({
      store: {
        roomList: {
          get rooms() {
            return rooms;
          }
        }
      }
    });
    rooms = ['R2'];

    expect((scope.store.roomList as unknown as { rooms: string[] }).rooms).toEqual(['R2']);
  });

  it('adds server info members and keeps the version check on the fixture', () => {
    let livekitUrl: string | null = null;
    const server = createTestServerScope({
      isSupportedVersion: false,
      serverInfo: {
        get livekitUrl() {
          return livekitUrl;
        },
        isSupportedVersion: true
      }
    });
    const { serverInfo } = server.scope.store;
    livekitUrl = 'wss://livekit.example.test';

    expect(serverInfo.livekitUrl).toBe('wss://livekit.example.test');
    expect(serverInfo.isSupportedVersion).toBe(false);
    server.isSupportedVersion = true;
    expect(serverInfo.isSupportedVersion).toBe(true);
  });

  it('serves the most recent scope through the module replacement', () => {
    createTestServerScope({ serverId: 'first' });
    const latest = createTestServerScope({ serverId: 'second' });

    expect(serverScopeModule.useServerScope()).toBe(latest.scope);
  });
});
