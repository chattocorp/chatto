import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { CurrentUserState } from '$lib/auth/currentUser.svelte';
import {
  removeRegisteredAdminQueries,
  removeRegisteredServerQueries
} from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';
import { settingsQueryKeys } from '$lib/query/settings';
import ExternalIdentitySettings from './ExternalIdentitySettings.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    list: vi.fn(),
    startLink: vi.fn(),
    disconnect: vi.fn(),
    serverId: 'origin',
    scopeCurrent: true,
    beginExplicitSignOutRedirect: vi.fn(),
    cancelExplicitSignOutRedirect: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    clearCachedUser: vi.fn(),
    notifyLogout: vi.fn(),
    clearServerAuthentication: vi.fn()
  }
}));

const connection = {
  serverId: 'origin',
  queryScope: 'external-identities-test',
  getAPI: () => ({
    list: mocks.list,
    startLink: mocks.startLink,
    disconnect: mocks.disconnect
  })
};

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: mocks.serverId,
    connection,
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/auth/signOut', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/auth/signOut')>()),
  beginExplicitSignOutRedirect: mocks.beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect: mocks.cancelExplicitSignOutRedirect,
  hardRedirectAfterSignOut: mocks.hardRedirectAfterSignOut
}));

vi.mock('$lib/auth/loadAuth', () => ({ clearCachedUser: mocks.clearCachedUser }));
vi.mock('$lib/auth/sessionChannel', () => ({ notifyLogout: mocks.notifyLogout }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: (serverId: string) => serverId === 'origin',
    clearServerAuthentication: mocks.clearServerAuthentication
  }
}));

const currentUser = {
  user: { hasPassword: true }
} as unknown as CurrentUserState;

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function renderSettings() {
  return render(ExternalIdentitySettings, {
    props: { currentUser, accountSettingsPath: '/chat/-/settings/account' }
  });
}

function linkedIdentityList() {
  return {
    providers: [
      {
        id: 'github-main',
        type: 'github',
        label: 'GitHub',
        loginUrl: '/auth/github',
        linkUrl: '/auth/github?intent=link',
        linked: true,
        linkedIdentitySubjectHash: 'subject-1'
      }
    ],
    linkedIdentities: [
      {
        providerId: 'github-main',
        providerType: 'github',
        providerLabel: 'GitHub',
        subjectHash: 'subject-1'
      }
    ]
  };
}

describe('external identity settings query lifecycle', () => {
  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.scopeCurrent = true;
    mocks.serverId = 'origin';
    connection.serverId = 'origin';
    connection.queryScope = 'external-identities-test';
    mocks.list.mockResolvedValue(linkedIdentityList());
    mocks.startLink.mockResolvedValue('https://chat.example.test/link');
    mocks.disconnect.mockResolvedValue(undefined);
  });

  it('passes cancellation through and revalidates a cached callback snapshot', async () => {
    const first = renderSettings();
    await settle();

    expect(mocks.list).toHaveBeenCalledWith(
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(first.container.textContent).toContain('GitHub');
    first.unmount();

    const second = renderSettings();
    await settle();
    expect(second.container.textContent).toContain('GitHub');
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it('purges the private snapshot with the server session', async () => {
    const queryKey = settingsQueryKeys.externalIdentities('origin', connection);
    const view = renderSettings();
    await settle();
    view.unmount();

    removeRegisteredServerQueries('origin');

    expect(queryClient.getQueryData(queryKey)).toBeUndefined();
  });

  it('preserves the existing sign-out flow after a successful disconnect', async () => {
    const view = renderSettings();
    await settle();

    const disconnectButton = Array.from(view.container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Disconnect')
    );
    disconnectButton?.click();
    flushSync();
    const confirmButton = Array.from(document.querySelectorAll('button')).find(
      (button) => button !== disconnectButton && button.textContent?.includes('Disconnect')
    );
    confirmButton?.click();

    await vi.waitFor(() => expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin'));
    expect(mocks.clearCachedUser).toHaveBeenCalledOnce();
    expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
    expect(mocks.notifyLogout).toHaveBeenCalledOnce();
  });

  it('does not fence a successful disconnect when only admin queries are purged', async () => {
    let resolveDisconnect!: () => void;
    mocks.disconnect.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveDisconnect = resolve;
      })
    );
    const view = renderSettings();
    await settle();

    const disconnectButton = Array.from(view.container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Disconnect')
    );
    disconnectButton?.click();
    flushSync();
    const confirmButton = Array.from(document.querySelectorAll('button')).find(
      (button) => button !== disconnectButton && button.textContent?.includes('Disconnect')
    );
    confirmButton?.click();
    await vi.waitFor(() => expect(mocks.disconnect).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('origin');
    resolveDisconnect();
    await vi.waitFor(() => expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin'));
    expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
  });

  it('fences a late disconnect after the session is removed', async () => {
    let resolveDisconnect!: () => void;
    mocks.disconnect.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveDisconnect = resolve;
      })
    );
    const view = renderSettings();
    await settle();

    const disconnectButton = Array.from(view.container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Disconnect')
    );
    disconnectButton?.click();
    flushSync();
    const confirmButton = Array.from(document.querySelectorAll('button')).find(
      (button) => button !== disconnectButton && button.textContent?.includes('Disconnect')
    );
    confirmButton?.click();
    await vi.waitFor(() => expect(mocks.disconnect).toHaveBeenCalledOnce());

    removeRegisteredServerQueries('origin');
    view.unmount();
    resolveDisconnect();
    await settle();

    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.hardRedirectAfterSignOut).not.toHaveBeenCalled();
    expect(mocks.cancelExplicitSignOutRedirect).toHaveBeenCalledOnce();
  });

  it('finishes remote sign-out when authentication cleanup precedes an unauthenticated error', async () => {
    mocks.serverId = 'remote';
    connection.serverId = 'remote';
    connection.queryScope = 'remote-external-identities-test';
    let rejectDisconnect!: (error: Error) => void;
    mocks.disconnect.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectDisconnect = reject;
      })
    );
    const view = renderSettings();
    await settle();

    const disconnectButton = Array.from(view.container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Disconnect')
    );
    disconnectButton?.click();
    flushSync();
    const confirmButton = Array.from(document.querySelectorAll('button')).find(
      (button) => button !== disconnectButton && button.textContent?.includes('Disconnect')
    );
    confirmButton?.click();
    await vi.waitFor(() => expect(mocks.disconnect).toHaveBeenCalledOnce());

    removeRegisteredServerQueries('remote');
    rejectDisconnect(new ConnectError('expired', Code.Unauthenticated));

    await vi.waitFor(() => expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('remote'));
    expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
    expect(mocks.clearCachedUser).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
  });
});
