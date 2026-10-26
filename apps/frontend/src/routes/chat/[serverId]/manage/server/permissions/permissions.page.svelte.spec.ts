import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { queryClient } from '$lib/query/client';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: { listAdminRoles: vi.fn(), getRolePermissionTierMatrix: vi.fn() }
}));

vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn(),
  beforeNavigate: vi.fn(),
  afterNavigate: vi.fn()
}));
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string, params?: Record<string, string>) =>
    Object.entries(params ?? {}).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));
vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (id: string) => id,
  segmentToServerId: (id: string) => id
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

import PermissionsPage from './+page.svelte';

beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  queryClient.clear();
  vi.clearAllMocks();
  createTestServerScope({
    serverId: 'origin',
    permissions: { canAdminManageRoles: false, canAssignRoles: true },
    api: mocks
  });
});

describe('role roster navigation', () => {
  it('lists role links without fetching an editor matrix or showing creation controls', async () => {
    mocks.listAdminRoles.mockResolvedValue({
      roles: [{ name: 'moderator', displayName: 'Moderator' }],
      viewerCanManageRoles: false,
      viewerCanAssignRoles: true
    });
    const { container } = render(PermissionsPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Moderator (@moderator)'));
    expect(
      container.querySelector('a[href="/chat/origin/manage/server/permissions/moderator"]')
    ).not.toBeNull();
    expect(container.querySelector('a[href$="/new"]')).toBeNull();
    expect(mocks.getRolePermissionTierMatrix).not.toHaveBeenCalled();
    expect(mocks.listAdminRoles).toHaveBeenCalledWith(
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it('shows a failed catalog read with a retry action', async () => {
    mocks.listAdminRoles.mockRejectedValue(new ConnectError('read denied', Code.PermissionDenied));
    const { container } = render(PermissionsPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Try Again'));
    expect(container.querySelector('a[href$="/moderator"]')).toBeNull();
    expect(mocks.getRolePermissionTierMatrix).not.toHaveBeenCalled();
  });
});
