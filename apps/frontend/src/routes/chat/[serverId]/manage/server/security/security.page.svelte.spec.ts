import '../../../../../../app.css';
import { beforeAll } from 'vitest';
import { loadLocaleMessages } from '$lib/i18n/messages';

// Standalone management components do not run the route catalog loader.
beforeAll(() => loadLocaleMessages('en-GB'));

import { beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { OAuthClient } from '$lib/api/oauthClients';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';
import { serverRegistry } from '$lib/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  getServerSecurityConfig: vi.fn(),
  updateBlockedUsernames: vi.fn(),
  listOAuthClients: vi.fn(),
  updateOAuthClientPolicy: vi.fn(),
  success: vi.fn(),
  error: vi.fn()
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

let server: TestServerScope;

vi.mock('@chatto/client/api/serverState', async () => {
  const actual = await vi.importActual<typeof import('@chatto/client/api/serverState')>(
    '@chatto/client/api/serverState'
  );
  return {
    ...actual,
    getServerSecurityConfig: mocks.getServerSecurityConfig,
    updateBlockedUsernames: mocks.updateBlockedUsernames
  };
});

vi.mock('$lib/ui/Panel.svelte', async () => ({
  default: (await import('../roles/[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui', async () => ({
  Panel: (await import('$lib/ui/Panel.svelte')).default,
  PaneHeader: (await import('$lib/ui/PaneHeader.svelte')).default,
  PageTitle: (await import('$lib/ui/PageTitle.svelte')).default,
  LoadingFog: (await import('$lib/ui/LoadingFog.svelte')).default,
  EmptyState: (await import('$lib/ui/EmptyState.svelte')).default,
  Hint: (await import('../roles/[name]/RolePageSnippetMock.svelte')).default,
  PaneContent: (await import('../roles/[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/PaneHeader.svelte', async () => ({
  default: (await import('../roles/[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/PageTitle.svelte', async () => ({
  default: (await import('../roles/[name]/RolePageSnippetMock.svelte')).default
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
    server = createTestServerScope({
      serverId: 'origin',
      viewer: null,
      api: {
        list: mocks.listOAuthClients,
        updatePolicy: mocks.updateOAuthClientPolicy
      }
    });
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
      clientOrigin: 'https://remote.example',
      source: 'cimd',
      policy: 'blocked',
      firstAuthorizationAt: '2026-08-10T12:00:00.000Z',
      lastAuthorizationAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://remote.example'],
      authorizedUserCount: 2
    });
  });

  it('passes cancellation through and reuses a fresh cached snapshot', async () => {
    const first = render(SecurityPage);
    await settle();

    expect(mocks.getServerSecurityConfig).toHaveBeenCalledWith(
      server.scope.connection.apiConfig,
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
    const connection = server.scope.connection;
    const queryKey = adminQueryKeys.securityConfig('origin', connection);
    const { container } = render(SecurityPage);
    await settle();

    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
    const save = container
      .querySelector('textarea')!
      .closest('form')!
      .querySelector('button[type="submit"]') as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    textarea.value = 'root\nadmin\nreserved';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(save.disabled).toBe(false);
    save.click();

    await vi.waitFor(() =>
      expect(mocks.updateBlockedUsernames).toHaveBeenCalledWith(
        server.scope.connection.apiConfig,
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
    const connection = server.scope.connection;
    const queryKey = adminQueryKeys.securityConfig('origin', connection);
    const view = render(SecurityPage);
    await settle();

    const textarea = view.container.querySelector('textarea') as HTMLTextAreaElement;
    textarea.value = 'private-pattern';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (
      view.container
        .querySelector('textarea')!
        .closest('form')!
        .querySelector('button[type="submit"]') as HTMLButtonElement
    ).click();
    await vi.waitFor(() => expect(mocks.updateBlockedUsernames).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('origin');
    view.unmount();
    saveResult.resolve({ blockedUsernames: 'private-pattern' });
    await settle();

    expect(queryClient.getQueryData(queryKey)).toBeUndefined();
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it('does not show the OAuth client empty state while the initial list is loading', async () => {
    const listResult = deferred<{
      oauthClients: never[];
      totalCount: number;
      hasMore: boolean;
    }>();
    mocks.listOAuthClients.mockReturnValue(listResult.promise);

    const view = render(SecurityPage);
    await settle();

    expect(view.container.querySelector('[data-loading-fog][aria-busy="true"]')).not.toBeNull();
    expect(view.container.textContent).not.toContain('No OAuth clients have been authorised');

    listResult.resolve({ oauthClients: [], totalCount: 0, hasMore: false });
    await vi.waitFor(() =>
      expect(view.container.textContent).toContain('No OAuth clients have been authorised')
    );
  });

  it('does not show the OAuth client empty state when the initial list fails', async () => {
    mocks.listOAuthClients.mockRejectedValue(new Error('Inventory unavailable'));

    const view = render(SecurityPage);

    await vi.waitFor(
      () => expect(view.container.textContent).toContain('Inventory unavailable'),
      5_000
    );
    expect(view.container.textContent).not.toContain('No OAuth clients have been authorised');
  });

  it('lists authorized OAuth clients and saves policy changes immediately', async () => {
    const client = {
      clientId: 'https://remote.example/oauth/client-metadata.json',
      clientName: 'Remote Chatto',
      clientOrigin: 'https://remote.example',
      source: 'cimd' as const,
      policy: 'default' as const,
      firstAuthorizationAt: '2026-08-10T12:00:00.000Z',
      lastAuthorizationAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://remote.example'],
      authorizedUserCount: 2
    };
    mocks.listOAuthClients
      .mockResolvedValueOnce({ oauthClients: [client], totalCount: 1, hasMore: false })
      .mockResolvedValue({
        oauthClients: [{ ...client, policy: 'blocked' as const }],
        totalCount: 1,
        hasMore: false
      });

    const { container } = render(SecurityPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Remote Chatto'));
    expect(container.textContent).not.toContain('Last authorisation');
    expect(container.textContent).toContain('Users');
    expect(container.textContent).not.toContain(client.clientId);
    const metadataLink = container.querySelector('a')!;
    expect(metadataLink.textContent).toBe(client.redirectOrigins[0]);
    expect(metadataLink.getAttribute('href')).toBe(client.clientId);
    expect(metadataLink.getAttribute('target')).toBe('_blank');
    expect(metadataLink.getAttribute('rel')).toBe('noopener noreferrer');
    expect(
      Array.from(container.querySelectorAll('bdi[dir="ltr"]'), (element) => element.textContent)
    ).toEqual([client.redirectOrigins[0]]);

    const policy = container.querySelector(
      'select:not(#registration-admission)'
    ) as HTMLSelectElement;
    policy.value = 'blocked';
    policy.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() =>
      expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledWith(client.clientId, 'blocked')
    );
    await vi.waitFor(() => expect(mocks.listOAuthClients).toHaveBeenCalledTimes(2));
    expect(mocks.success).toHaveBeenCalledWith('OAuth client policy saved');
    expect(policy.value).toBe('blocked');
  });

  it('keeps a CIMD URL unchanged when its path looks like a Chatto route', async () => {
    const originSpy = vi.spyOn(serverRegistry, 'originServer', 'get').mockReturnValue({
      id: 'origin',
      url: window.location.origin,
      name: 'Local server',
      iconUrl: null,
      token: null,
      userId: null,
      userLogin: null,
      userDisplayName: null,
      userAvatarUrl: null,
      reauthRequiredAt: null,
      addedAt: 1
    });
    onTestFinished(() => originSpy.mockRestore());
    const clientId = `${window.location.origin}/chat/-/client-metadata.json`;
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [
        {
          clientId,
          clientName: 'Local metadata client',
          policy: 'default',
          redirectOrigins: [window.location.origin],
          authorizedUserCount: 1
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    const { container } = render(SecurityPage);
    await vi.waitFor(() => expect(container.querySelector('a')).not.toBeNull());
    expect(container.querySelector('a')!.getAttribute('href')).toBe(clientId);
  });

  it.each(['chatto://desktop', 'javascript:alert(1)'])(
    'does not link a non-web client identifier: %s',
    async (clientId) => {
      mocks.listOAuthClients.mockResolvedValue({
        oauthClients: [
          {
            clientId,
            clientName: 'Native client',
            policy: 'default',
            redirectOrigins: ['chatto://desktop'],
            authorizedUserCount: 1
          }
        ],
        totalCount: 1,
        hasMore: false
      });
      const { container } = render(SecurityPage);
      await vi.waitFor(() => expect(container.textContent).toContain('Native client'));
      expect(container.textContent).toContain('chatto://desktop');
      expect(container.querySelector('a')).toBeNull();
    }
  );

  it.each([320, 480, 960])('keeps client details and policy within a %ipx pane', async (width) => {
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [
        {
          clientId: 'https://remote.example/oauth/client-metadata.json',
          clientName: 'Remote Chatto',
          policy: 'default',
          redirectOrigins: ['https://remote.example'],
          authorizedUserCount: 2,
          lastAuthorizationAt: '2026-08-11T12:00:00.000Z'
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    const host = document.createElement('div');
    host.style.width = `${width}px`;
    document.body.append(host);
    const view = render(SecurityPage, { target: host });
    try {
      await vi.waitFor(() => expect(host.querySelector('select')).not.toBeNull());
      const list = host.querySelector<HTMLUListElement>('ul[aria-label="OAuth clients"]')!;
      const policy = host.querySelector('select')!;
      const listBounds = list.getBoundingClientRect();
      const policyBounds = policy.getBoundingClientRect();

      expect(list.scrollWidth).toBeLessThanOrEqual(list.clientWidth);
      expect(policyBounds.left).toBeGreaterThanOrEqual(listBounds.left);
      expect(policyBounds.right).toBeLessThanOrEqual(listBounds.right);
      expect(policyBounds.width).toBeGreaterThanOrEqual(144);
      expect(list.textContent).toContain('https://remote.example');
      expect(list.textContent).not.toContain('Last authorisation');
    } finally {
      view.unmount();
      host.remove();
    }
  });

  it('tracks pending policy saves independently for each OAuth client', async () => {
    const alpha: OAuthClient = {
      clientId: 'https://alpha.example/oauth/client-metadata.json',
      clientName: 'Alpha',
      clientOrigin: 'https://alpha.example',
      source: 'cimd' as const,
      sourceCode: 1,
      policy: 'default' as const,
      policyCode: 1,
      firstAuthorizationAt: '2026-08-10T12:00:00.000Z',
      lastAuthorizationAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://alpha.example'],
      authorizedUserCount: 1
    };
    const bravo: OAuthClient = {
      ...alpha,
      clientId: 'https://bravo.example/oauth/client-metadata.json',
      clientName: 'Bravo',
      clientOrigin: 'https://bravo.example',
      redirectOrigins: ['https://bravo.example']
    };
    const alphaSave = deferred<OAuthClient>();
    const bravoSave = deferred<OAuthClient>();
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [alpha, bravo],
      totalCount: 2,
      hasMore: false
    });
    mocks.updateOAuthClientPolicy.mockImplementation((clientId: string) =>
      clientId === alpha.clientId ? alphaSave.promise : bravoSave.promise
    );

    const { container } = render(SecurityPage);
    await vi.waitFor(() =>
      expect(container.querySelectorAll('select:not(#registration-admission)')).toHaveLength(2)
    );
    const [alphaPolicy, bravoPolicy] = Array.from(
      container.querySelectorAll('select:not(#registration-admission)')
    ) as HTMLSelectElement[];

    alphaPolicy.value = 'blocked';
    alphaPolicy.dispatchEvent(new Event('change', { bubbles: true }));
    await vi.waitFor(() => expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledOnce());
    expect(alphaPolicy.disabled).toBe(true);
    expect(bravoPolicy.disabled).toBe(false);

    bravoPolicy.value = 'trusted';
    bravoPolicy.dispatchEvent(new Event('change', { bubbles: true }));
    await vi.waitFor(() => expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledTimes(2));
    expect(alphaPolicy.disabled).toBe(true);
    expect(bravoPolicy.disabled).toBe(true);

    alphaPolicy.value = 'trusted';
    alphaPolicy.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledTimes(2);

    alphaSave.resolve({ ...alpha, policy: 'blocked' });
    await vi.waitFor(() => expect(alphaPolicy.disabled).toBe(false));
    expect(bravoPolicy.disabled).toBe(true);

    bravoSave.resolve({ ...bravo, policy: 'trusted' });
    await vi.waitFor(() => expect(bravoPolicy.disabled).toBe(false));
  });

  it('keeps the confirmed policy visible when an update is rejected', async () => {
    const client = {
      clientId: 'https://remote.example/oauth/client-metadata.json',
      clientName: 'Remote Chatto',
      clientOrigin: 'https://remote.example',
      source: 'cimd' as const,
      policy: 'default' as const,
      firstAuthorizationAt: '2026-08-10T12:00:00.000Z',
      lastAuthorizationAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://remote.example'],
      authorizedUserCount: 2
    };
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [client],
      totalCount: 1,
      hasMore: false
    });
    mocks.updateOAuthClientPolicy.mockRejectedValue(new Error('Policy update rejected'));

    const { container } = render(SecurityPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Remote Chatto'));

    const policy = container.querySelector(
      'select:not(#registration-admission)'
    ) as HTMLSelectElement;
    policy.value = 'blocked';
    policy.dispatchEvent(new Event('change', { bubbles: true }));

    expect(policy.value).toBe('default');
    await vi.waitFor(() =>
      expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledWith(client.clientId, 'blocked')
    );
    await vi.waitFor(() => expect(mocks.error).toHaveBeenCalledWith('Policy update rejected'));
    expect(policy.value).toBe('default');
  });

  it('shows unknown future policies without allowing an overwrite', async () => {
    const client: OAuthClient = {
      clientId: 'https://future.example/oauth/client-metadata.json',
      clientName: 'Future Client',
      clientOrigin: 'https://future.example',
      source: 'unknown',
      sourceCode: 99,
      policy: 'unknown',
      policyCode: 101,
      firstAuthorizationAt: '2026-08-10T12:00:00.000Z',
      lastAuthorizationAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://future.example'],
      authorizedUserCount: 1
    };
    mocks.listOAuthClients.mockResolvedValue({
      oauthClients: [client],
      totalCount: 1,
      hasMore: false
    });

    const { container } = render(SecurityPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Future Client'));

    const policy = container.querySelector(
      'select:not(#registration-admission)'
    ) as HTMLSelectElement;
    expect(policy.value).toBe('unknown');
    expect(policy.disabled).toBe(true);
    expect(policy.selectedOptions[0]?.textContent?.trim()).toBe('Unknown (101)');

    policy.value = 'blocked';
    policy.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    expect(mocks.updateOAuthClientPolicy).not.toHaveBeenCalled();
    expect(policy.value).toBe('unknown');
  });

  it('shows a confirmed policy when the background list refresh fails', async () => {
    const client = {
      clientId: 'https://remote.example/oauth/client-metadata.json',
      clientName: 'Remote Chatto',
      clientOrigin: 'https://remote.example',
      source: 'cimd' as const,
      policy: 'default' as const,
      firstAuthorizationAt: '2026-08-10T12:00:00.000Z',
      lastAuthorizationAt: '2026-08-11T12:00:00.000Z',
      redirectOrigins: ['https://remote.example'],
      authorizedUserCount: 2
    };
    mocks.listOAuthClients
      .mockResolvedValueOnce({ oauthClients: [client], totalCount: 1, hasMore: false })
      .mockRejectedValue(new Error('List refresh failed'));

    const { container } = render(SecurityPage);
    await vi.waitFor(() => expect(container.textContent).toContain('Remote Chatto'));

    const policy = container.querySelector(
      'select:not(#registration-admission)'
    ) as HTMLSelectElement;
    policy.value = 'blocked';
    policy.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => expect(mocks.updateOAuthClientPolicy).toHaveBeenCalledOnce());
    await vi.waitFor(() => expect(mocks.listOAuthClients).toHaveBeenCalledTimes(2));
    expect(mocks.success).toHaveBeenCalledWith('OAuth client policy saved');
    expect(policy.value).toBe('blocked');
  });
});
