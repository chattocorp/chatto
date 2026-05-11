import { describe, it, expect, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Harness from './__permissionsTestHarness.svelte';
import { serverRegistry, type RegisteredInstance } from './registry.svelte';
import type { ServerPermissions, ViewerData } from './permissions.svelte';

const STORAGE_KEY = 'chatto:instances';

function makeInstance(overrides: Partial<RegisteredInstance> = {}): RegisteredInstance {
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
    canViewDMs: false,
    canWriteDMs: false,
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
  let perms: { readonly current: ServerPermissions } | undefined;
  render(Harness, {
    props: {
      serverId,
      expose: (p) => {
        perms = p;
      }
    }
  });
  if (!perms) throw new Error('harness did not expose perms');
  return perms;
}

describe('getServerPermissions', () => {
  beforeEach(() => {
    localStorage.removeItem(STORAGE_KEY);
    for (const instance of [...serverRegistry.instances]) {
      serverRegistry.removeInstance(instance.id);
    }
  });

  it('reads from the active instance store', () => {
    serverRegistry.addInstance(makeInstance({ id: 'remote' }));
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
    serverRegistry.addInstance(
      makeInstance({ id: 'origin', url: window.location.origin, name: 'Origin' })
    );
    serverRegistry.getStore('origin').setPermissions(makeViewer({ canAdminViewRoles: true }));

    serverRegistry.addInstance(makeInstance({ id: 'remote' }));
    serverRegistry.getStore('remote').setPermissions(makeViewer({ canAdminViewRoles: false }));

    const perms = mount('remote');

    // Bug we are guarding against: reading origin's permissions here would
    // wrongly return true.
    expect(perms.current.loaded).toBe(true);
    expect(perms.current.canAdminViewRoles).toBe(false);
  });
});
