import { beforeAll, beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { AdminRoleService } from '@chatto/api-types/admin/v1/roles_connect';
import { AdminPermissionService } from '@chatto/api-types/admin/v1/permissions_connect';
import {
  PermissionDecision as Decision,
  PermissionScopeKind as Kind
} from '@chatto/api-types/admin/v1/permissions_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { mockService } from '@chatto/client/testing/fakeServer';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { queryClient } from '$lib/query/client';
import Page from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string, params: Record<string, string> = {}) =>
    Object.entries(params).reduce(
      (url, [key, value]) => url.replace(`[${key}]`, encodeURIComponent(value)),
      path
    )
}));
vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (id: string) => id,
  segmentToServerId: (id: string) => id
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const roles = mockService(AdminRoleService);
const permissions = mockService(AdminPermissionService);
function response(read = Decision.NONE) {
  return {
    matrix: {
      roleName: 'feedback',
      applicablePermissions: ['message.post', 'message.read'],
      scopes: [{ id: 'room:feedback', label: 'Feedback room', kind: Kind.ROOM }],
      cells: [
        { scopeId: 'room:feedback', permission: 'message.post', effective: Decision.ALLOW },
        { scopeId: 'room:feedback', permission: 'message.read', effective: read }
      ]
    }
  };
}
beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  queryClient.clear();
  vi.resetAllMocks();
  roles.listRoles.mockReturnValue({
    roles: [{ role: { name: 'feedback', displayName: 'Feedback role' } }]
  });
  permissions.getRolePermissionMatrix.mockReturnValue(response());
  createTestServerScope({
    routes: (router) => {
      router.service(AdminRoleService, roles);
      router.service(AdminPermissionService, permissions);
    }
  });
});
afterEach(() => queryClient.clear());

describe('configuration warnings page', () => {
  it('shows an advisory with role and room names and a permission settings link', async () => {
    const view = render(Page);
    await expect.element(view.getByText('Feedback room', { exact: true })).toBeVisible();
    await expect.element(view.getByText('Feedback role', { exact: true })).toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Review role permissions' }))
      .toHaveAttribute('href', '/chat/server-1/manage/server/permissions/feedback');
    await expect.element(view.getByText(/These settings can be intentional/)).toBeVisible();
    await expect.element(view.getByText(/Interaction-only read access/)).toBeVisible();
    expect(permissions.setRolePermission).not.toHaveBeenCalled();
  });
  it('does not show all-clear before the matrix finishes', async () => {
    let finish!: (value: ReturnType<typeof response>) => void;
    permissions.getRolePermissionMatrix.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const view = render(Page);
    await expect.poll(() => permissions.getRolePermissionMatrix.mock.calls.length).toBe(1);
    await expect.element(view.getByRole('status')).toBeInTheDocument();
    await expect
      .element(view.getByText('No warnings found in the checked rooms.'))
      .not.toBeInTheDocument();
    finish(response(Decision.ALLOW));
    await expect.element(view.getByText('No warnings found in the checked rooms.')).toBeVisible();
  });
  it('shows failed reads and supports retry instead of claiming all-clear', async () => {
    permissions.getRolePermissionMatrix.mockRejectedValue(
      new ConnectError('unavailable', Code.Unavailable)
    );
    const view = render(Page);
    await expect.element(view.getByRole('button', { name: 'Try Again' })).toBeVisible();
    await expect
      .element(view.getByText('No warnings found in the checked rooms.'))
      .not.toBeInTheDocument();
    permissions.getRolePermissionMatrix.mockReturnValue(response());
    await view.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(view.getByText('Feedback room', { exact: true })).toBeVisible();
  });
  it('treats a missing matrix as an error', async () => {
    permissions.getRolePermissionMatrix.mockReturnValue({});
    const view = render(Page);
    await expect.element(view.getByRole('button', { name: 'Try Again' })).toBeVisible();
    await expect
      .element(view.getByText('No warnings found in the checked rooms.'))
      .not.toBeInTheDocument();
  });
});
