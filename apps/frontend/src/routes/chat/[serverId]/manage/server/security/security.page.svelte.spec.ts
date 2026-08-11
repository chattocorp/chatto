import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';

const mocks = vi.hoisted(() => ({
  getServerSecurityConfig: vi.fn(),
  updateBlockedUsernames: vi.fn(),
  listOAuthClients: vi.fn(),
  updateOAuthClientPolicy: vi.fn(),
  success: vi.fn(),
  error: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: { currentUser: { user: null } },
    connection: {
      queryScope: 'security-test',
      apiConfig: { baseUrl: '/api/connect', bearerToken: 'token' },
      getAPI: () => ({
        list: mocks.listOAuthClients,
        updatePolicy: mocks.updateOAuthClientPolicy
      })
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/api-client/serverState', async () => {
  const actual = await vi.importActual<typeof import('$lib/api-client/serverState')>(
    '$lib/api-client/serverState'
  );
  return {
    ...actual,
    getServerSecurityConfig: mocks.getServerSecurityConfig,
    updateBlockedUsernames: mocks.updateBlockedUsernames
  };
});

vi.mock('$lib/components/admin', async () => ({
  Panel: (await import('../permissions/[name]/RolePageSnippetMock.svelte')).default,
  DataTable: (await import('./DataTableMock.svelte')).default
}));
vi.mock('$lib/ui', async () => ({
  Hint: (await import('../permissions/[name]/RolePageSnippetMock.svelte')).default,
  PaneContent: (await import('../permissions/[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/PaneHeader.svelte', async () => ({
  default: (await import('../permissions/[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/PageTitle.svelte', async () => ({
  default: (await import('../permissions/[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.success, error: mocks.error }
}));

import SecurityPage from './+page.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('server security query lifecycle', () => {
  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.getServerSecurityConfig.mockResolvedValue({ blockedUsernames: 'root\nadmin' });
    mocks.updateBlockedUsernames.mockResolvedValue({
      blockedUsernames: 'root\nadmin\nreserved'
    });
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [],
      totalCount: 0,
      hasMore: false
    });
    mocks.updateOAuthClientPolicy.mockResolvedValue({
      clientId: 'https://remote.example/oauth/client-metadata.json',
      clientName: 'Remote Chatto',
      clientUri: 'https://remote.example',
      source: 'cimd',
      policy: 'blocked',
      firstObservedAt: '2026-08-10T12:00:00.000Z',
      lastObservedAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://remote.example'],
      authorizedUserCount: 2
    });
  });

  it('passes cancellation through and reuses a fresh cached snapshot', async () => {
    const first = render(SecurityPage);
    await settle();

    expect(mocks.getServerSecurityConfig).toHaveBeenCalledWith(
      { baseUrl: '/api/connect', bearerToken: 'token' },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect((first.container.querySelector('textarea') as HTMLTextAreaElement).value).toBe(
      'root\nadmin'
    );
    first.unmount();

    const second = render(SecurityPage);
    await settle();
    expect((second.container.querySelector('textarea') as HTMLTextAreaElement).value).toBe(
      'root\nadmin'
    );
    expect(mocks.getServerSecurityConfig).toHaveBeenCalledOnce();
  });

  it('saves changed values and replaces the exact cached snapshot', async () => {
    const connection = { queryScope: 'security-test' };
    const queryKey = adminQueryKeys.securityConfig('origin', connection);
    const { container } = render(SecurityPage);
    await settle();

    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    const save = container.querySelector('button[type="submit"]') as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    textarea.value = 'root\nadmin\nreserved';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(save.disabled).toBe(false);
    save.click();

    await vi.waitFor(() =>
      expect(mocks.updateBlockedUsernames).toHaveBeenCalledWith(
        { baseUrl: '/api/connect', bearerToken: 'token' },
        'root\nadmin\nreserved'
      )
    );
    await vi.waitFor(() =>
      expect(queryClient.getQueryData(queryKey)).toEqual({
        blockedUsernames: 'root\nadmin\nreserved'
      })
    );
    expect(save.disabled).toBe(true);
    expect(mocks.success).toHaveBeenCalledOnce();
  });

  it('does not restore private data after an admin cache privacy boundary', async () => {
    const saveResult = deferred<{ blockedUsernames: string }>();
    mocks.updateBlockedUsernames.mockReturnValue(saveResult.promise);
    const connection = { queryScope: 'security-test' };
    const queryKey = adminQueryKeys.securityConfig('origin', connection);
    const view = render(SecurityPage);
    await settle();

    const textarea = view.container.querySelector('textarea') as HTMLTextAreaElement;
    textarea.value = 'private-pattern';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (view.container.querySelector('button[type="submit"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(mocks.updateBlockedUsernames).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('origin');
    view.unmount();
    saveResult.resolve({ blockedUsernames: 'private-pattern' });
    await settle();

    expect(queryClient.getQueryData(queryKey)).toBeUndefined();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it('lists observed OAuth clients and saves policy changes immediately', async () => {
    const client = {
      clientId: 'https://remote.example/oauth/client-metadata.json',
      clientName: 'Remote Chatto',
      clientUri: 'https://remote.example',
      source: 'cimd' as const,
      policy: 'default' as const,
      firstObservedAt: '2026-08-10T12:00:00.000Z',
      lastObservedAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://remote.example'],
      authorizedUserCount: 2
    };
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [client],
      totalCount: 1,
      hasMore: false
    });

    const { container } = render(SecurityPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Remote Chatto'));

    const policy = container.querySelector('select') as HTMLSelectElement;
    policy.value = 'blocked';
    policy.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() =>
      expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledWith(client.clientId, 'blocked')
    );
    await vi.waitFor(() => expect(mocks.listOAuthClients).toHaveBeenCalledTimes(2));
    expect(mocks.success).toHaveBeenCalledWith('OAuth client policy saved');
  });
});
