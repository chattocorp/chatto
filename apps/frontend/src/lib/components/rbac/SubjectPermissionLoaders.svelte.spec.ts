import '../../../app.css';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import RolePermissionsMatrix from './RolePermissionsMatrix.svelte';
import UserPermissionsMatrix from './UserPermissionsMatrix.svelte';
import { queryClient } from '$lib/query/client';
import { adminQueryKeys } from '$lib/query/admin';
import {
  refreshRegisteredAdminQueries,
  removeRegisteredAdminUserQueries
} from '$lib/query/cacheRegistry';

const viewerPermissions = vi.hoisted(() => ({ canAdminManageAccounts: true }));

const permissionMocks = vi.hoisted(() => ({
  getRolePermissionMatrix: vi.fn(),
  getUserPermissionMatrix: vi.fn(),
  setRolePermission: vi.fn(),
  setUserPermission: vi.fn(),
  addMember: vi.fn(),
  removeMember: vi.fn(),
  batchGetRooms: vi.fn(),
  batchGetRoomMembers: vi.fn()
}));

vi.mock('$lib/api-client/permissions', () => ({
  createPermissionAPI: () => permissionMocks
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: { permissions: viewerPermissions },
    connection: { queryScope: 'permission-loader-test', getAPI: () => permissionMocks },
    isCurrent: () => true
  })
}));

function matrix(subject: { roleName: string } | { userId: string }) {
  return {
    ...subject,
    page: { totalCount: 1, hasMore: false },
    applicablePermissions: ['message.post', 'room.manage'],
    scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
    cells: [
      {
        permission: 'message.post',
        scopeId: 'server',
        override: 'NONE',
        effective: 'NONE'
      },
      {
        permission: 'room.manage',
        scopeId: 'server',
        override: 'NONE',
        effective: 'NONE'
      }
    ]
  };
}

function cellButton(container: HTMLElement, permission: string): HTMLButtonElement {
  return container.querySelector(`td[data-permission="${permission}"] button`)!;
}

function scopedCellButton(
  container: HTMLElement,
  scopeId: string,
  permission: string
): HTMLButtonElement {
  return container.querySelector(
    `td[data-scope="${scopeId}"][data-permission="${permission}"] button`
  )!;
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

beforeEach(() => {
  vi.clearAllMocks();
  viewerPermissions.canAdminManageAccounts = true;
  permissionMocks.batchGetRooms.mockResolvedValue([]);
  permissionMocks.batchGetRoomMembers.mockResolvedValue([]);
  permissionMocks.getRolePermissionMatrix.mockImplementation((roleName: string) =>
    Promise.resolve(matrix({ roleName }))
  );
  permissionMocks.getUserPermissionMatrix.mockImplementation((userId: string) => {
    if (userId === 'bot-a') {
      return Promise.resolve({
        userId,
        page: { totalCount: 1, hasMore: false },
        applicablePermissions: ['message.post'],
        scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
        cells: [
          {
            permission: 'message.post',
            scopeId: 'server',
            override: 'ALLOW',
            effective: 'DENY',
            allowPermitted: false
          }
        ]
      });
    }
    return Promise.resolve(matrix({ userId }));
  });
  permissionMocks.setRolePermission.mockResolvedValue({});
  permissionMocks.setUserPermission.mockResolvedValue({});
});

afterEach(() => queryClient.clear());

describe('subject permission loaders', () => {
  it('keeps the same matrix elements mounted while refreshed data replaces their cells', async () => {
    let resolveRefresh: ((value: ReturnType<typeof matrix>) => void) | undefined;
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    const originalTable = rendered.container.querySelector('table');
    const originalCell = rendered.container.querySelector(
      'td[data-scope="server"][data-permission="message.post"]'
    );
    permissionMocks.getRolePermissionMatrix.mockImplementationOnce(
      () => new Promise((resolve) => (resolveRefresh = resolve))
    );

    refreshRegisteredAdminQueries('origin');
    await settle();

    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(
      rendered.container.querySelector('td[data-scope="server"][data-permission="message.post"]')
    ).toBe(originalCell);

    const refreshed = matrix({ roleName: 'role-a' });
    refreshed.cells[0].override = 'ALLOW';
    resolveRefresh?.(refreshed);
    await vi.waitFor(() => {
      expect(cellButton(rendered.container, 'message.post').getAttribute('aria-label')).toContain(
        'Override allow'
      );
    });
    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(
      rendered.container.querySelector('td[data-scope="server"][data-permission="message.post"]')
    ).toBe(originalCell);
  });

  it('isolates pending role mutation state after route reuse', async () => {
    const mutations: Array<{
      resolve: (value: object) => void;
      reject: (error: Error) => void;
    }> = [];
    permissionMocks.setRolePermission.mockImplementation(
      () =>
        new Promise<object>((resolve, reject) => {
          mutations.push({ resolve, reject });
        })
    );
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    cellButton(rendered.container, 'message.post').click();
    await rendered.rerender({ roleName: 'role-b' });
    await settle();

    const replacementButton = cellButton(rendered.container, 'message.post');
    expect(replacementButton.disabled).toBe(false);
    replacementButton.click();
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[0].reject(new Error('stale role failure'));
    await settle();

    expect(permissionMocks.getRolePermissionMatrix).toHaveBeenCalledWith(
      'role-b',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(rendered.container.textContent).not.toContain('stale role failure');
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[1].resolve({});
    await vi.waitFor(() =>
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false)
    );
  });

  it('isolates pending user mutation state after route reuse', async () => {
    const mutations: Array<{
      resolve: (value: object) => void;
      reject: (error: Error) => void;
    }> = [];
    permissionMocks.setUserPermission.mockImplementation(
      () =>
        new Promise<object>((resolve, reject) => {
          mutations.push({ resolve, reject });
        })
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    cellButton(rendered.container, 'message.post').click();
    await rendered.rerender({ userId: 'user-b' });
    await settle();

    const replacementButton = cellButton(rendered.container, 'message.post');
    expect(replacementButton.disabled).toBe(false);
    replacementButton.click();
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[0].reject(new Error('stale user failure'));
    await settle();

    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledWith(
      'user-b',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(rendered.container.textContent).not.toContain('stale user failure');
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[1].resolve({});
    await vi.waitFor(() =>
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false)
    );
  });

  it('scrubs a mounted user matrix without refetching after realtime user removal', async () => {
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    expect(rendered.container.querySelector('table')).not.toBeNull();

    removeRegisteredAdminUserQueries('origin', 'user-a');
    await settle();

    expect(rendered.container.querySelector('table')).toBeNull();
    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledOnce();
  });

  it('updates a user permission without remounting the matrix or changing page scroll', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    rendered.container.style.height = '80px';
    rendered.container.style.overflowY = 'auto';
    rendered.container.scrollTop = 60;
    const originalScrollTop = rendered.container.scrollTop;
    const originalTable = rendered.container.querySelector('table');

    cellButton(rendered.container, 'message.post').click();
    await settle();
    resolveMutation?.({ decision: 'ALLOW' });

    await vi.waitFor(() =>
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false)
    );
    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(rendered.container.scrollTop).toBe(originalScrollTop);
  });

  it('updates only the active cell in a binary user matrix', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'user-a', decisionMode: 'binary' }
    });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    const originalTable = rendered.container.querySelector('table');
    const originalTarget = cellButton(rendered.container, 'message.post');
    const originalOther = cellButton(rendered.container, 'room.manage');
    const otherClassName = originalOther.className;

    originalTarget.click();
    await settle();

    expect(cellButton(rendered.container, 'message.post')).toBe(originalTarget);
    expect(originalTarget.disabled).toBe(false);
    expect(originalTarget.getAttribute('aria-disabled')).toBe('true');
    expect(cellButton(rendered.container, 'room.manage')).toBe(originalOther);
    expect(originalOther.disabled).toBe(false);
    expect(originalOther.className).toBe(otherClassName);
    originalOther.click();
    expect(permissionMocks.setUserPermission).toHaveBeenCalledOnce();

    resolveMutation?.({ decision: 'ALLOW' });
    await vi.waitFor(() => expect(originalTarget.getAttribute('aria-disabled')).toBeNull());

    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(cellButton(rendered.container, 'room.manage')).toBe(originalOther);
    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledOnce();
    expect(
      queryClient.getQueryState(
        adminQueryKeys.userPermissions('origin', { queryScope: 'permission-loader-test' }, 'user-a')
      )?.isInvalidated
    ).toBe(true);
  });

  it('keeps a ceiling-blocked inherited room inert without writing a denial', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue({
      userId: 'bot-inheritance',
      page: { totalCount: 1, hasMore: false },
      applicablePermissions: ['message.post'],
      scopes: [
        { id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' },
        { id: 'group:general', label: 'General', kind: 'GROUP', parentGroupId: '' },
        { id: 'room:lobby', label: 'Lobby', kind: 'ROOM', parentGroupId: 'general' }
      ],
      cells: [
        {
          permission: 'message.post',
          scopeId: 'server',
          override: 'NONE',
          effective: 'NONE'
        },
        {
          permission: 'message.post',
          scopeId: 'group:general',
          override: 'ALLOW',
          effective: 'ALLOW'
        },
        {
          permission: 'message.post',
          scopeId: 'room:lobby',
          override: 'NONE',
          effective: 'ALLOW',
          allowPermitted: false
        }
      ]
    });
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'bot-inheritance', decisionMode: 'binary', ownerCapped: true }
    });
    await expect
      .poll(() => scopedCellButton(rendered.container, 'group:general', 'message.post'))
      .toBeTruthy();
    const table = rendered.container.querySelector('table');
    const group = scopedCellButton(rendered.container, 'group:general', 'message.post');
    const room = scopedCellButton(rendered.container, 'room:lobby', 'message.post');
    const groupClassName = group.className;

    expect(room.title).toContain('Enabled · Inherited from a broader scope');
    expect(room.querySelector('[class~="bg-warning/20"]')).not.toBeNull();
    expect(room.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(room.disabled).toBe(true);

    room.click();
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    expect(permissionMocks.setUserPermission).not.toHaveBeenCalled();
    expect(rendered.container.querySelector('table')).toBe(table);
    expect(scopedCellButton(rendered.container, 'group:general', 'message.post')).toBe(group);
    expect(group.className).toBe(groupClassName);
    expect(room.querySelector('[class~="bg-warning/20"]')).not.toBeNull();
    expect(room.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(room.disabled).toBe(true);
    expect(rendered.container.querySelector('table')).toBe(table);
    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledOnce();
  });

  it('shows a bot narrow permission as enabled when message.read includes it', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue({
      userId: 'bot-read',
      page: { totalCount: 1, hasMore: false },
      applicablePermissions: ['message.read', 'message.read-interactions'],
      scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
      cells: [
        {
          permission: 'message.read',
          scopeId: 'server',
          override: 'ALLOW',
          effective: 'ALLOW',
          allowPermitted: true
        },
        {
          permission: 'message.read-interactions',
          scopeId: 'server',
          override: 'NONE',
          effective: 'ALLOW',
          allowPermitted: true
        }
      ]
    });
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'bot-read', decisionMode: 'binary', ownerCapped: true }
    });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    const child = scopedCellButton(rendered.container, 'server', 'message.read-interactions');
    expect(child.title).toContain('Included by message.read');
    expect(child.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(child.disabled).toBe(true);
  });

  it('serializes role mutations within one resource', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setRolePermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    cellButton(rendered.container, 'message.post').click();
    await settle();
    expect(cellButton(rendered.container, 'room.manage').disabled).toBe(true);

    cellButton(rendered.container, 'room.manage').click();
    cellButton(rendered.container, 'message.post').click();
    expect(permissionMocks.setRolePermission).toHaveBeenCalledOnce();

    resolveMutation?.({});
    await vi.waitFor(() => {
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
      expect(cellButton(rendered.container, 'room.manage').disabled).toBe(false);
    });
  });

  it('invalidates cached user matrices after a role permission changes', async () => {
    const connection = { queryScope: 'permission-loader-test' };
    const userPermissionKey = adminQueryKeys.userPermissions('origin', connection, 'user-a');
    queryClient.setQueryData(userPermissionKey, {
      pages: [matrix({ userId: 'user-a' })],
      pageParams: [0]
    });
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    cellButton(rendered.container, 'message.post').click();

    await vi.waitFor(() =>
      expect(queryClient.getQueryState(userPermissionKey)?.isInvalidated).toBe(true)
    );
  });

  it('serializes user mutations within one resource', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    cellButton(rendered.container, 'message.post').click();
    await settle();
    expect(cellButton(rendered.container, 'room.manage').disabled).toBe(true);

    cellButton(rendered.container, 'room.manage').click();
    cellButton(rendered.container, 'message.post').click();
    expect(permissionMocks.setUserPermission).toHaveBeenCalledOnce();

    resolveMutation?.({});
    await vi.waitFor(() => {
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
      expect(cellButton(rendered.container, 'room.manage').disabled).toBe(false);
    });
  });

  it('shows the owner ceiling and writes bot decisions through the user permission API', async () => {
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'bot-a', subjectKind: 'bot', ownerCapped: true }
    });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());

    const button = cellButton(rendered.container, 'message.post');
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).toBeNull();
    expect(rendered.container.textContent).toContain('your bot');

    button.click();
    await settle();
    expect(permissionMocks.setUserPermission).toHaveBeenCalledWith({
      userId: 'bot-a',
      permission: 'message.post',
      scope: { tier: 'server' },
      state: 'deny'
    });
  });
});

describe('account membership mutations', () => {
  async function confirmMembership() {
    await expect.poll(() => document.querySelector('dialog[open]')).toBeTruthy();
    expect(document.querySelector('dialog[open]')?.textContent).toContain(
      'Confirm to apply this membership change immediately.'
    );
    document.querySelector<HTMLButtonElement>('dialog[open] button[type="submit"]')!.click();
    await settle();
  }

  beforeEach(() => {
    permissionMocks.batchGetRooms.mockResolvedValue([
      { id: 'work', isUniversal: false, archived: false, canManageRoom: false }
    ]);
  });
  function botMatrix() {
    return {
      ...matrix({ userId: 'membership-bot' }),
      scopes: [
        {
          id: 'room:work',
          label: 'work',
          kind: 'ROOM',
          parentGroupId: ''
        }
      ],
      cells: [
        { permission: 'room.manage', scopeId: 'room:work', override: 'NONE', effective: 'NONE' }
      ]
    };
  }

  it('confirms membership before saving immediately without writing permissions', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue(botMatrix());
    permissionMocks.addMember.mockImplementation(async () => {
      permissionMocks.batchGetRoomMembers.mockResolvedValue([{ id: 'membership-bot' }]);
      return {};
    });
    permissionMocks.removeMember.mockImplementation(async () => {
      permissionMocks.batchGetRoomMembers.mockResolvedValue([]);
      return true;
    });
    const { container } = render(UserPermissionsMatrix, {
      props: { userId: 'membership-bot', ownerCapped: false, decisionMode: 'tri-state' }
    });
    await expect
      .poll(() => container.querySelector('button[aria-label="Add account to #work"]'))
      .toBeTruthy();
    (
      container.querySelector('button[aria-label="Add account to #work"]') as HTMLButtonElement
    ).click();
    expect(permissionMocks.addMember).not.toHaveBeenCalled();
    await confirmMembership();
    await expect.poll(() => permissionMocks.addMember.mock.calls.length).toBe(1);
    expect(permissionMocks.addMember).toHaveBeenCalledWith({
      roomId: 'work',
      userId: 'membership-bot'
    });
    await expect
      .poll(
        () =>
          container.querySelector<HTMLButtonElement>(
            'button[aria-label="Remove account from #work"]'
          )?.disabled
      )
      .toBe(false);
    (
      container.querySelector('button[aria-label="Remove account from #work"]') as HTMLButtonElement
    ).click();
    expect(permissionMocks.removeMember).not.toHaveBeenCalled();
    await confirmMembership();
    await expect.poll(() => permissionMocks.removeMember.mock.calls.length).toBe(1);
    await expect
      .poll(() => container.querySelector('button[aria-label="Add account to #work"]'))
      .toBeTruthy();
    expect(permissionMocks.setUserPermission).not.toHaveBeenCalled();
  });

  it('lets a scoped room manager add a human without room.join', async () => {
    viewerPermissions.canAdminManageAccounts = false;
    permissionMocks.batchGetRooms.mockResolvedValue([
      { id: 'work', isUniversal: false, archived: false, canManageRoom: true }
    ]);
    permissionMocks.getUserPermissionMatrix.mockResolvedValue({ ...botMatrix(), userId: 'human' });
    permissionMocks.addMember.mockResolvedValue({});
    const { container } = render(UserPermissionsMatrix, { props: { userId: 'human' } });
    await expect
      .poll(
        () =>
          container.querySelector<HTMLButtonElement>('button[aria-label="Add account to #work"]')
            ?.disabled
      )
      .toBe(false);
    container
      .querySelector<HTMLButtonElement>('button[aria-label="Add account to #work"]')!
      .click();
    expect(permissionMocks.addMember).not.toHaveBeenCalled();
    await confirmMembership();
    await expect.poll(() => permissionMocks.addMember.mock.calls.length).toBe(1);
    expect(permissionMocks.addMember).toHaveBeenCalledWith({ roomId: 'work', userId: 'human' });
    expect(permissionMocks.setUserPermission).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    'cancels a membership change without writing, joined=%s',
    async (joined) => {
      permissionMocks.getUserPermissionMatrix.mockResolvedValue(botMatrix());
      permissionMocks.batchGetRoomMembers.mockResolvedValue(
        joined ? [{ id: 'membership-bot' }] : []
      );
      const { container } = render(UserPermissionsMatrix, { props: { userId: 'membership-bot' } });
      const label = joined ? 'Remove account from #work' : 'Add account to #work';
      await expect
        .poll(() => container.querySelector(`button[aria-label="${label}"]`))
        .toBeTruthy();
      container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!.click();
      await expect.poll(() => document.querySelector('dialog[open]')).toBeTruthy();
      const cancel = [...document.querySelectorAll<HTMLButtonElement>('dialog[open] button')].find(
        (button) => button.textContent?.trim() === 'Cancel'
      );
      cancel!.click();
      await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
      expect(permissionMocks.addMember).not.toHaveBeenCalled();
      expect(permissionMocks.removeMember).not.toHaveBeenCalled();
      expect(permissionMocks.setUserPermission).not.toHaveBeenCalled();
    }
  );

  it('discards an unconfirmed action when the account changes', async () => {
    permissionMocks.getUserPermissionMatrix.mockImplementation((userId: string) =>
      Promise.resolve({ ...botMatrix(), userId })
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'membership-bot' } });
    await expect
      .poll(() => rendered.container.querySelector('button[aria-label="Add account to #work"]'))
      .toBeTruthy();
    rendered.container
      .querySelector<HTMLButtonElement>('button[aria-label="Add account to #work"]')!
      .click();
    await expect.poll(() => document.querySelector('dialog[open]')).toBeTruthy();
    await rendered.rerender({ userId: 'other-account' });
    await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
    await rendered.rerender({ userId: 'membership-bot' });
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    expect(document.querySelector('dialog[open]')).toBeNull();
    expect(permissionMocks.addMember).not.toHaveBeenCalled();
  });

  it('rechecks membership availability when confirming', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue(botMatrix());
    const { container } = render(UserPermissionsMatrix, { props: { userId: 'membership-bot' } });
    await expect
      .poll(() => container.querySelector('button[aria-label="Add account to #work"]'))
      .toBeTruthy();
    container
      .querySelector<HTMLButtonElement>('button[aria-label="Add account to #work"]')!
      .click();
    await expect.poll(() => document.querySelector('dialog[open]')).toBeTruthy();
    permissionMocks.batchGetRooms.mockResolvedValue([
      { id: 'work', isUniversal: false, archived: true, canManageRoom: false }
    ]);
    refreshRegisteredAdminQueries('origin');
    const key = adminQueryKeys.userPermissions(
      'origin',
      { queryScope: 'permission-loader-test' },
      'membership-bot'
    );
    await expect
      .poll(
        () =>
          queryClient.getQueryData<{
            pages: { scopes: { membership?: { canJoin: boolean } }[] }[];
          }>(key)?.pages[0]?.scopes[0]?.membership?.canJoin
      )
      .toBe(false);
    await confirmMembership();
    expect(permissionMocks.addMember).not.toHaveBeenCalled();
  });

  it('keeps a bot join locked without effective room.join or a manager override', async () => {
    viewerPermissions.canAdminManageAccounts = false;
    permissionMocks.getUserPermissionMatrix.mockResolvedValue(botMatrix());
    const { container } = render(UserPermissionsMatrix, {
      props: { userId: 'membership-bot', ownerCapped: true }
    });
    await expect
      .poll(
        () =>
          container.querySelector<HTMLButtonElement>('button[aria-label="Add account to #work"]')
            ?.disabled
      )
      .toBe(true);
    expect(permissionMocks.addMember).not.toHaveBeenCalled();
  });

  it('retains authoritative membership after a failed join and allows retry', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue(botMatrix());
    permissionMocks.addMember.mockRejectedValue(new Error('denied'));
    const { container } = render(UserPermissionsMatrix, {
      props: { userId: 'membership-bot', ownerCapped: true, decisionMode: 'binary' }
    });
    await expect
      .poll(() => container.querySelector('button[aria-label="Add account to #work"]'))
      .toBeTruthy();
    (
      container.querySelector('button[aria-label="Add account to #work"]') as HTMLButtonElement
    ).click();
    await confirmMembership();
    await expect.poll(() => container.textContent).toContain('Failed to join room');
    await expect
      .poll(
        () =>
          container.querySelector<HTMLButtonElement>('button[aria-label="Add account to #work"]')
            ?.disabled
      )
      .toBe(false);
    expect(permissionMocks.getUserPermissionMatrix.mock.calls.length).toBeGreaterThan(1);
    expect(permissionMocks.setUserPermission).not.toHaveBeenCalled();
  });
  it('ignores a late membership failure after switching bots', async () => {
    let rejectJoin: ((error: Error) => void) | undefined;
    permissionMocks.getUserPermissionMatrix.mockImplementation((userId: string) =>
      Promise.resolve({ ...botMatrix(), userId })
    );
    permissionMocks.addMember.mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectJoin = reject;
        })
    );
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'membership-bot', ownerCapped: true, decisionMode: 'binary' }
    });
    await expect
      .poll(() => rendered.container.querySelector('button[aria-label="Add account to #work"]'))
      .toBeTruthy();
    (
      rendered.container.querySelector(
        'button[aria-label="Add account to #work"]'
      ) as HTMLButtonElement
    ).click();
    await confirmMembership();
    await expect.poll(() => rejectJoin).toBeTruthy();
    await rendered.rerender({ userId: 'replacement-bot' });
    await expect
      .poll(() =>
        permissionMocks.getUserPermissionMatrix.mock.calls.some(([id]) => id === 'replacement-bot')
      )
      .toBe(true);
    rejectJoin!(new Error('stale failure'));
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    expect(rendered.container.textContent).not.toContain('Failed to join room');
    expect(
      rendered.container.querySelector<HTMLButtonElement>(
        'button[aria-label="Add account to #work"]'
      )?.disabled
    ).toBe(false);
  });
});

it.each(['role', 'user'] as const)(
  'fences a delayed next scope page after %s navigation',
  async (kind) => {
    const read =
      kind === 'role'
        ? permissionMocks.getRolePermissionMatrix
        : permissionMocks.getUserPermissionMatrix;
    const subject = (id: string) => (kind === 'role' ? { roleName: id } : { userId: id });
    let resolveNext!: (value: ReturnType<typeof matrix>) => void;
    read.mockImplementation((id: string, options: { page: { offset: number } }) => {
      if (id === 'first' && options.page.offset > 0)
        return new Promise((resolve) => {
          resolveNext = resolve;
        });
      return Promise.resolve({
        ...matrix(subject(id)),
        page: { totalCount: id === 'first' ? 2 : 1, hasMore: id === 'first' }
      });
    });
    const rendered =
      kind === 'role'
        ? render(RolePermissionsMatrix, { props: { roleName: 'first' } })
        : render(UserPermissionsMatrix, { props: { userId: 'first' } });
    await vi.waitFor(() => expect(resolveNext).toBeTypeOf('function'));
    const signal = read.mock.calls.find(
      ([id, options]) => id === 'first' && options.page.offset === 1
    )?.[1].signal as AbortSignal;
    await rendered.rerender(kind === 'role' ? { roleName: 'second' } : { userId: 'second' });
    await vi.waitFor(() => expect(signal.aborted).toBe(true));
    const late = matrix(subject('first'));
    late.scopes = [{ id: 'room:late', label: 'Late scope', kind: 'ROOM', parentGroupId: '' }];
    resolveNext(late);
    await settle();
    await vi.waitFor(() => expect(rendered.container.querySelector('table')).not.toBeNull());
    expect(rendered.container.querySelector('[data-scope="room:late"]')).toBeNull();
  }
);
