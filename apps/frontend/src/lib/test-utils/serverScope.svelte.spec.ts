import { describe, expect, it } from 'vitest';
import { createTestServerScope, serverScopeModule } from './serverScope.svelte';

describe('createTestServerScope', () => {
  it('provides a loaded viewer, permissions, and every feature by default', () => {
    const { scope } = createTestServerScope();

    expect(scope.serverId).toBe('server-1');
    expect(scope.store.accountId).toBe('viewer-1');
    expect(scope.store.viewerId).toBe('viewer-1');
    expect(scope.store.projectionViewerId).toBe('viewer-1');
    expect(scope.store.isAuthenticated).toBe(true);
    expect(scope.store.permissions.loaded).toBe(true);
    expect(scope.store.permissions.canManageServer).toBe(false);
    expect(scope.store.serverInfo.supportsFeature('botAccounts')).toBe(true);
    expect(scope.isCurrent()).toBe(true);
  });

  it('applies options and reflects later state changes', () => {
    const server = createTestServerScope({
      serverId: 'server-2',
      viewer: null,
      permissions: { canManageBots: true },
      features: { botAccounts: false }
    });
    const { scope } = server;

    expect(scope.store.accountId).toBeNull();
    expect(scope.store.permissions.canManageBots).toBe(true);
    expect(scope.store.serverInfo.supportsFeature('botAccounts')).toBe(false);
    expect(scope.store.serverInfo.supportsFeature('userAvatars')).toBe(true);

    server.permissions.canManageBots = false;
    server.current = false;
    server.serverId = 'server-3';

    expect(scope.store.permissions.canManageBots).toBe(false);
    expect(scope.isCurrent()).toBe(false);
    expect(scope.serverId).toBe('server-3');
    expect(scope.connection.queryScope).toBe('server-3-session');
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
        navigation: {
          get rooms() {
            return rooms;
          }
        }
      }
    });
    rooms = ['R2'];

    expect((scope.store.navigation as unknown as { rooms: string[] }).rooms).toEqual(['R2']);
  });

  it('serves the most recent scope through the module replacement', () => {
    createTestServerScope({ serverId: 'first' });
    const latest = createTestServerScope({ serverId: 'second' });

    expect(serverScopeModule.useServerScope()).toBe(latest.scope);
  });
});
