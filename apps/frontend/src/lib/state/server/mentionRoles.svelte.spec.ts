import { describe, expect, it, vi } from 'vitest';
import { MentionRolesStore } from './mentionRoles.svelte';

function role(name: string) {
  return {
    name,
    displayName: name,
    description: '',
    permissions: [],
    permissionDenials: [],
    isSystem: false,
    position: 1,
    pingable: true
  };
}

describe('MentionRolesStore', () => {
  it('maps and caches the public role catalogue', async () => {
    const listRoles = vi.fn().mockResolvedValue({
      roles: [role('everyone'), role('moderator')],
      viewerCanManageRoles: false,
      viewerCanAssignRoles: false
    });
    const store = new MentionRolesStore({ listRoles });

    await expect(store.load()).resolves.toBe(true);
    await expect(store.load()).resolves.toBe(true);

    expect(listRoles).toHaveBeenCalledOnce();
    expect(store.status).toBe('ready');
    expect(store.roles).toEqual([
      {
        name: 'moderator',
        isSystem: false,
        position: 1,
        pingable: true
      }
    ]);
  });

  it('coalesces concurrent loads', async () => {
    let resolveList: ((value: ReturnType<typeof catalog>) => void) | undefined;
    const listRoles = vi.fn(
      () =>
        new Promise<ReturnType<typeof catalog>>((resolve) => {
          resolveList = resolve;
        })
    );
    const store = new MentionRolesStore({ listRoles });

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
    const store = new MentionRolesStore({ listRoles });

    await store.load();
    await expect(store.refresh()).resolves.toBe(false);
    expect(store.status).toBe('failed');
    expect(store.roles).toEqual([]);

    await expect(store.load()).resolves.toBe(true);
    expect(store.status).toBe('ready');
    expect(store.roles.map(({ name }) => name)).toEqual(['support']);
  });
});

function catalog(name = 'moderator') {
  return {
    roles: [role(name)],
    viewerCanManageRoles: false,
    viewerCanAssignRoles: false
  };
}
