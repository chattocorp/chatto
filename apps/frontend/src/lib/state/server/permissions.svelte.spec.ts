import { describe, it, expect, beforeEach } from 'vitest';
import { serverRegistry, type RegisteredServer } from './registry.svelte';
import { getServerPermissions, type ServerPermissions, type ViewerData } from './permissions.svelte';

const STORAGE_KEY = 'chatto:instances';

function makeServer(overrides: Partial<RegisteredServer> = {}): RegisteredServer {
  return {
    id: 'remote',
    url: 'https://remote.example.com',
    name: 'Remote',
    iconUrl: null,
    token: null,
    userId: null,
    userLogin: null,
    userDisplayName: null,
    userAvatarUrl: null,
    addedAt: 0,
    ...overrides
  };
}

function makeViewer(overrides: Partial<ViewerData> = {}): ViewerData {
  return {
    canViewAdmin: false,
    canStartDMs: false,
    canAdminViewUsers: false,
    canAdminManageUsers: false,
    canAdminViewRoles: false,
    canAdminManageRoles: false,
    canAdminViewSystem: false,
    canAdminViewAudit: false,
    ...overrides
  };
}

function mount(serverId: string): { readonly current: ServerPermissions } {
  return getServerPermissions(serverId);
}

describe('getServerPermissions', () => {
  beforeEach(() => {
    localStorage.removeItem(STORAGE_KEY);
    for (const server of [...serverRegistry.servers]) {
      serverRegistry.removeServer(server.id);
    }
  });

  it('reads from the active instance store', () => {
    serverRegistry.addServer(makeServer({ id: 'remote' }));
    serverRegistry.getStore('remote').setPermissions(makeViewer({ canAdminViewRoles: true }));

    const perms = mount('remote');

    expect(perms.current.loaded).toBe(true);
    expect(perms.current.canAdminViewRoles).toBe(true);
  });

  it('returns unloaded defaults when no store exists for the active instance', () => {
    const perms = mount('not-registered');

    expect(perms.current.loaded).toBe(false);
    expect(perms.current.canAdminViewRoles).toBe(false);
  });

  it('reflects the active instance, not the origin', () => {
    // Origin grants admin; remote does not.
    serverRegistry.addServer(
      makeServer({ id: 'origin', url: window.location.origin, name: 'Origin' })
    );
    serverRegistry.getStore('origin').setPermissions(makeViewer({ canAdminViewRoles: true }));

    serverRegistry.addServer(makeServer({ id: 'remote' }));
    serverRegistry.getStore('remote').setPermissions(makeViewer({ canAdminViewRoles: false }));

    const perms = mount('remote');

    // Bug we are guarding against: reading origin's permissions here would
    // wrongly return true.
    expect(perms.current.loaded).toBe(true);
    expect(perms.current.canAdminViewRoles).toBe(false);
  });
});
