import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { goto } from '$app/navigation';
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

import RolesPage from './+page.svelte';

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

describe('role catalog navigation', () => {
  it('lists role links without fetching an editor matrix or showing creation controls', async () => {
    mocks.listAdminRoles.mockResolvedValue({
      roles: [
        {
          name: 'moderator',
          displayName: 'Moderator',
          description: 'Helps members',
          position: 100,
          isSystem: true
        }
      ],
      viewerCanManageRoles: false,
      viewerCanAssignRoles: true
    });
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Moderator'));
    expect(
      container.querySelector('a[href="/chat/origin/manage/server/roles/moderator"]')
    ).not.toBeNull();
    expect(container.querySelector('a[href$="/new"]')).toBeNull();
    expect(container.querySelector('[data-role-member-count]')?.textContent).toBe('—');
    expect(container.querySelector('button[title="Edit Role"]')).toBeNull();
    expect(container.querySelector('button[title="Permissions"]')).toBeNull();
    expect(mocks.getRolePermissionTierMatrix).not.toHaveBeenCalled();
    expect(mocks.listAdminRoles).toHaveBeenCalledWith(
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it('filters role names and descriptions and exposes creation only to managers', async () => {
    mocks.listAdminRoles.mockResolvedValue({
      roles: [
        {
          name: 'moderator',
          displayName: 'Moderator',
          description: 'Helps members',
          position: 100,
          isSystem: true,
          memberCount: 12,
          permissionGrantCount: 3,
          permissionDenialCount: 0
        },
        {
          name: 'support',
          displayName: 'Support team',
          description: 'Answers questions',
          position: 1,
          isSystem: false
        }
      ],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(container.querySelectorAll('tbody tr')).toHaveLength(2));
    expect(container.querySelector('a[href$="/roles/new"]')).not.toBeNull();
    const row = container.querySelector('tbody tr')!;
    expect(row.textContent).toContain('System');
    expect(row.querySelector('[data-role-member-count]')?.textContent).toBe('12');
    expect(row.querySelector('[data-role-grant-count]')?.textContent).toBe('3');
    expect(row.querySelector('[data-role-denial-count]')?.textContent).toBe('');
    (row.querySelector('button[title="Edit Role"]') as HTMLButtonElement).click();
    expect(goto).toHaveBeenLastCalledWith('/chat/origin/manage/server/roles/moderator');
    (row.querySelector('button[title="Permissions"]') as HTMLButtonElement).click();
    expect(goto).toHaveBeenLastCalledWith('/chat/origin/manage/server/roles/moderator/permissions');
    const search = container.querySelector('input')!;
    search.value = 'questions';
    search.dispatchEvent(new Event('input', { bubbles: true }));
    await vi.waitFor(() => expect(container.querySelectorAll('tbody tr')).toHaveLength(1));
    expect(container.querySelector('tbody')?.textContent).toContain('Support team');
    expect(container.querySelector('tbody')?.textContent).not.toContain('Moderator');
    search.value = '@moderator';
    search.dispatchEvent(new Event('input', { bubbles: true }));
    await vi.waitFor(() =>
      expect(container.querySelector('tbody')?.textContent).toContain('Moderator')
    );
    search.value = '';
    search.dispatchEvent(new Event('input', { bubbles: true }));
    await vi.waitFor(() => expect(container.querySelectorAll('tbody tr')).toHaveLength(2));
    expect(mocks.listAdminRoles).toHaveBeenCalledOnce();
  });

  it('shows a failed catalog read with a retry action', async () => {
    mocks.listAdminRoles.mockRejectedValue(new ConnectError('read denied', Code.PermissionDenied));
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Try Again'));
    expect(container.querySelector('a[href$="/moderator"]')).toBeNull();
    expect(mocks.getRolePermissionTierMatrix).not.toHaveBeenCalled();
  });
});
