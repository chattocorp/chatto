import { describe, expect, it, vi } from 'vitest';
import { outranksAccount, RoleCatalogStore, roleRanksBelow } from './roleCatalog.js';

function role(name: string) {
  return {
    name,
    displayName: name,
    description: '',
    permissions: [],
    permissionDenials: [],
    isSystem: false,
    pingable: true
  };
}

describe('RoleCatalogStore', () => {
  it('waits for verified startup authority before loading roles', async () => {
    let canLoad = false;
    const listRoles = vi.fn().mockResolvedValue(catalog());
    const store = new RoleCatalogStore({ listRoles }, () => canLoad);

    await expect(store.load()).resolves.toBe(false);
    expect(listRoles).not.toHaveBeenCalled();
    expect(store.status).toBe('idle');

    canLoad = true;
    await expect(store.load()).resolves.toBe(true);
    expect(listRoles).toHaveBeenCalledOnce();
  });

  it('maps and caches the public role catalogue', async () => {
    const listRoles = vi.fn().mockResolvedValue({
      roles: [role('moderator'), role('everyone')],
      viewerHighestRole: 'moderator'
    });
    const store = new RoleCatalogStore({ listRoles });

    await expect(store.load()).resolves.toBe(true);
    await expect(store.load()).resolves.toBe(true);

    expect(listRoles).toHaveBeenCalledOnce();
    expect(store.status).toBe('ready');
    expect(store.roles).toEqual([
      {
        name: 'moderator',
        isSystem: false,
        pingable: true
      }
    ]);
    expect(store.viewerHighestRole).toBe('moderator');
  });

  it('coalesces concurrent loads', async () => {
    let resolveList: ((value: ReturnType<typeof catalog>) => void) | undefined;
    const listRoles = vi.fn(
      () =>
        new Promise<ReturnType<typeof catalog>>((resolve) => {
          resolveList = resolve;
        })
    );
    const store = new RoleCatalogStore({ listRoles });

    const first = store.load();
    const second = store.refresh();
    resolveList?.(catalog());

    await expect(Promise.all([first, second])).resolves.toEqual([true, true]);
    expect(listRoles).toHaveBeenCalledOnce();
  });

  it('clears failed data and retries on the next load', async () => {
    const listRoles = vi
      .fn()
      .mockResolvedValueOnce(catalog('moderator'))
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValueOnce(catalog('support'));
    const store = new RoleCatalogStore({ listRoles });

    await store.load();
    await expect(store.refresh()).resolves.toBe(false);
    expect(store.status).toBe('failed');
    expect(store.roles).toEqual([]);
    expect(store.viewerHighestRole).toBeNull();

    await expect(store.load()).resolves.toBe(true);
    expect(store.status).toBe('ready');
    expect(store.roles.map(({ name }) => name)).toEqual(['support']);
  });
});

function catalog(name = 'moderator') {
  return {
    roles: [role(name)],
    viewerHighestRole: 'everyone'
  };
}

describe('role order helpers', () => {
  // Highest first, like the public role catalogue, without everyone.
  const order = ['owner', 'admin', 'moderator', 'helper'];

  it('lets accounts manage only roles below their highest role', () => {
    expect(roleRanksBelow(order, 'admin', 'moderator')).toBe(true);
    expect(roleRanksBelow(order, 'admin', 'admin')).toBe(false);
    expect(roleRanksBelow(order, 'moderator', 'admin')).toBe(false);
    expect(roleRanksBelow(order, 'admin', 'unknown')).toBe(false);
  });

  it('keeps everyone manageable and owner owner-only', () => {
    expect(roleRanksBelow(order, 'everyone', 'everyone')).toBe(true);
    expect(roleRanksBelow(order, 'admin', 'owner')).toBe(false);
    expect(roleRanksBelow(order, 'owner', 'owner')).toBe(false);
    expect(roleRanksBelow(order, 'owner', 'admin')).toBe(true);
  });

  it('requires a strictly higher rank to act on accounts', () => {
    expect(outranksAccount(order, 'admin', ['helper', 'moderator'])).toBe(true);
    expect(outranksAccount(order, 'moderator', ['helper', 'admin'])).toBe(false);
    expect(outranksAccount(order, 'admin', ['admin'])).toBe(false);
    expect(outranksAccount(order, 'helper', [])).toBe(true);
    expect(outranksAccount(order, 'everyone', [])).toBe(false);
  });

  it('lets only owners act on owners', () => {
    expect(outranksAccount(order, 'admin', ['owner'])).toBe(false);
    expect(outranksAccount(order, 'owner', ['owner'])).toBe(true);
    expect(outranksAccount(order, 'owner', ['admin'])).toBe(true);
  });

  it('answers from the loaded catalogue', async () => {
    const store = new RoleCatalogStore({
      listRoles: vi.fn().mockResolvedValue({
        roles: [role('owner'), role('admin'), role('moderator'), role('everyone')],
        viewerHighestRole: 'admin'
      })
    });
    // An unknown rank locks nothing; the server still checks every action.
    expect(store.ranksBelowViewer('admin')).toBe(true);
    expect(store.viewerOutranks(['admin'])).toBe(true);
    await store.load();
    expect(store.ranksBelowViewer('moderator')).toBe(true);
    expect(store.ranksBelowViewer('admin')).toBe(false);
    expect(store.viewerOutranks(['moderator'])).toBe(true);
    expect(store.viewerOutranks(['admin'])).toBe(false);
  });

  it('keeps the catalogue while a reload runs and replaces it afterwards', async () => {
    let resolveReload: ((value: unknown) => void) | undefined;
    const listRoles = vi
      .fn()
      .mockResolvedValueOnce({ roles: [role('admin'), role('helper')], viewerHighestRole: 'admin' })
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveReload = resolve;
          })
      );
    const store = new RoleCatalogStore({ listRoles });
    await store.load();

    const reload = store.reload();
    expect(store.status).toBe('loading');
    expect(store.viewerHighestRole).toBe('admin');
    expect(store.ranksBelowViewer('admin')).toBe(false);

    resolveReload?.({ roles: [role('helper'), role('admin')], viewerHighestRole: 'helper' });
    await expect(reload).resolves.toBe(true);
    expect(store.roles.map(({ name }) => name)).toEqual(['helper', 'admin']);
    expect(store.ranksBelowViewer('admin')).toBe(true);
  });

  it('forgets the rank after a reset', async () => {
    const store = new RoleCatalogStore({
      listRoles: vi.fn().mockResolvedValue({ roles: [role('admin')], viewerHighestRole: 'helper' })
    });
    await store.load();
    expect(store.ranksBelowViewer('admin')).toBe(false);

    store.invalidate();

    expect(store.viewerHighestRole).toBeNull();
    expect(store.ranksBelowViewer('admin')).toBe(true);
  });
});
