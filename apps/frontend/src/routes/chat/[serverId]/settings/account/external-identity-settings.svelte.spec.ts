import { Code, ConnectError } from '@connectrpc/connect';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page as browserPage } from 'vitest/browser';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { CurrentUserState } from '@chatto/client/auth/currentUser';
import {
  removeRegisteredAdminQueries,
  removeRegisteredServerQueries
} from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';
import { authorizationLaunchTarget } from '$lib/test-utils';
import { settingsQueryKeys } from '$lib/query/settings';
import ExternalIdentitySettings from './ExternalIdentitySettings.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    url: new URL('http://localhost/chat/-/settings/account'),
    replaceState: vi.fn(),
    list: vi.fn(),
    startLink: vi.fn(),
    disconnect: vi.fn(),
    serverId: 'origin',
    accountId: 'user-alice' as string | null,
    scopeCurrent: true,
    directLoginEnabled: true,
    beginExplicitSignOutRedirect: vi.fn(),
    cancelExplicitSignOutRedirect: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    notifyLogout: vi.fn(),
    clearServerAuthentication: vi.fn()
  }
}));

const connection = {
  serverId: 'origin',
  connectBaseUrl: 'https://remote.example.test/api/connect',
  queryScope: 'external-identities-test',
  getAPI: () => ({
    list: mocks.list,
    startLink: mocks.startLink,
    disconnect: mocks.disconnect
  })
};

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isOriginServer: (serverId: string) => serverId === 'origin',
    clearServerAuthentication: mocks.clearServerAuthentication
  }
}));

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return mocks.url;
    },
    state: {}
  }
}));
vi.mock('$app/navigation', () => ({
  replaceState: mocks.replaceState,
  pushState: vi.fn(),
  goto: vi.fn(),
  invalidateAll: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: mocks.serverId,
    connection,
    store: {
      serverInfo: {
        get directLoginEnabled() {
          return mocks.directLoginEnabled;
        }
      },
      get accountId() {
        return mocks.accountId;
      }
    },
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('@chatto/client/auth/signOut', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client/auth/signOut')>()),
  beginExplicitSignOutRedirect: mocks.beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect: mocks.cancelExplicitSignOutRedirect
}));

vi.mock('$lib/auth/signOutRedirect', () => ({
  hardRedirectAfterSignOut: mocks.hardRedirectAfterSignOut
}));

vi.mock('$lib/auth/sessionChannel', () => ({ notifyLogout: mocks.notifyLogout }));

const currentUser = {
  load: vi.fn().mockResolvedValue(undefined),
  user: { id: 'user-alice', hasPassword: true }
} as unknown as CurrentUserState;

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function renderSettings(user: CurrentUserState = currentUser) {
  mocks.accountId = user.user?.id ?? null;
  return render(ExternalIdentitySettings, {
    props: { currentUser: user, accountSettingsPath: '/chat/-/settings/account' }
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
        subjectHash: 'subject-1',
        disconnectEligibility: DisconnectEligibility.UNSPECIFIED
      }
    ]
  };
}

describe('external identity settings query lifecycle', () => {
  beforeEach(() => {
    mocks.directLoginEnabled = true;
    queryClient.clear();
    mocks.url = new URL('http://localhost/chat/-/settings/account');
    mocks.replaceState.mockImplementation((url: string | URL) => {
      mocks.url = new URL(url, mocks.url);
    });
    vi.clearAllMocks();
    mocks.scopeCurrent = true;
    mocks.serverId = 'origin';
    connection.serverId = 'origin';
    connection.queryScope = 'external-identities-test';
    mocks.list.mockResolvedValue(linkedIdentityList());
    mocks.startLink.mockResolvedValue('https://chat.example.test/link');
    mocks.disconnect.mockResolvedValue(undefined);
  });

  afterEach(() => vi.restoreAllMocks());

  it('blocks the last SSO provider despite a stored password on older servers', async () => {
    mocks.directLoginEnabled = false;
    renderSettings();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeDisabled();
    expect(mocks.disconnect).not.toHaveBeenCalled();
  });

  it('uses the server hint instead of counting unusable linked identities', async () => {
    mocks.directLoginEnabled = false;
    const list = linkedIdentityList();
    Object.assign(list.linkedIdentities[0], {
      disconnectEligibility: DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD
    });
    list.linkedIdentities.push({
      providerId: 'obsolete',
      providerType: 'oidc',
      providerLabel: 'Obsolete',
      subjectHash: 'obsolete-subject',
      disconnectEligibility: DisconnectEligibility.UNSPECIFIED
    });
    mocks.list.mockResolvedValue(list);
    renderSettings();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }).first())
      .toBeDisabled();
    expect(mocks.disconnect).not.toHaveBeenCalled();
  });

  it.each([
    { reason: 99 as DisconnectEligibility, notice: 'This action is currently unavailable.' },
    {
      reason: DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD,
      notice: 'Set up another sign-in method supported by this server before disconnecting GitHub.'
    }
  ])('shows the disconnect-specific notice for state $reason', async ({ reason, notice }) => {
    const list = linkedIdentityList();
    Object.assign(list.linkedIdentities[0], {
      disconnectEligibility: reason
    });
    mocks.list.mockResolvedValue(list);
    renderSettings();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeDisabled();
    await expect.element(browserPage.getByText(notice)).toBeVisible();
    expect(mocks.disconnect).not.toHaveBeenCalled();
  });

  it('disables unlinking when the operator forbids it', async () => {
    const list = linkedIdentityList();
    Object.assign(list.linkedIdentities[0], {
      disconnectEligibility: DisconnectEligibility.DISABLED_BY_OPERATOR
    });
    mocks.list.mockResolvedValue(list);
    renderSettings();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeDisabled();
    await expect
      .element(
        browserPage.getByText(
          'The server operator does not allow this provider to be disconnected.'
        )
      )
      .toBeVisible();
    expect(mocks.disconnect).not.toHaveBeenCalled();
  });

  it.each([DisconnectEligibility.ALLOWED])(
    'allows the server to check eligibility %s',
    async (disconnectEligibility) => {
      mocks.directLoginEnabled = false;
      const list = linkedIdentityList();
      Object.assign(list.linkedIdentities[0], { disconnectEligibility });
      mocks.list.mockResolvedValue(list);
      renderSettings();
      await browserPage.getByRole('button', { name: 'Disconnect', exact: true }).click();
      await expect
        .element(browserPage.getByRole('dialog', { name: 'Disconnect provider' }))
        .toBeVisible();
    }
  );

  it.each([
    { disconnectEligibility: DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD },
    { disconnectEligibility: DisconnectEligibility.DISABLED_BY_OPERATOR },
    {
      disconnectEligibility: 99 as DisconnectEligibility
    }
  ])(
    'refreshes a stale disconnect hint %s before asking for reauthentication',
    async (decision) => {
      const list = linkedIdentityList();
      Object.assign(list.linkedIdentities[0], {
        disconnectEligibility: DisconnectEligibility.ALLOWED
      });
      mocks.list.mockResolvedValue(list);
      renderSettings();
      await browserPage.getByRole('button', { name: 'Disconnect', exact: true }).click();
      const refreshed = linkedIdentityList();
      Object.assign(refreshed.linkedIdentities[0], decision);
      mocks.list.mockResolvedValue(refreshed);
      mocks.disconnect.mockRejectedValue(new ConnectError('last method', Code.FailedPrecondition));
      await browserPage
        .getByRole('dialog', { name: 'Disconnect provider' })
        .getByRole('button', { name: 'Disconnect', exact: true })
        .click();
      await expect
        .element(browserPage.getByRole('dialog', { name: 'Provider cannot be disconnected' }))
        .toBeVisible();
      await expect
        .element(browserPage.getByRole('dialog', { name: 'Confirm password' }))
        .not.toBeInTheDocument();
    }
  );

  it.each([
    DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD,
    DisconnectEligibility.DISABLED_BY_OPERATOR
  ])('closes password confirmation when disconnect becomes blocked (%s)', async (decision) => {
    mocks.list.mockResolvedValue(linkedIdentityList());
    mocks.disconnect.mockRejectedValue(
      new ConnectError('fresh authentication', Code.FailedPrecondition)
    );
    renderSettings();
    await browserPage.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await browserPage
      .getByRole('dialog', { name: 'Disconnect provider' })
      .getByRole('button', { name: 'Disconnect', exact: true })
      .click();
    const passwordDialog = browserPage.getByRole('dialog', { name: 'Confirm password' });
    await expect.element(passwordDialog).toBeVisible();
    await passwordDialog.getByLabelText('Current Password', { exact: true }).fill('test-password');
    const refreshed = linkedIdentityList();
    refreshed.linkedIdentities[0].disconnectEligibility = decision;
    mocks.list.mockResolvedValue(refreshed);
    await passwordDialog.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await expect
      .element(browserPage.getByRole('dialog', { name: 'Provider cannot be disconnected' }))
      .toBeVisible();
    await expect.element(passwordDialog).not.toBeInTheDocument();
    expect(document.querySelector('input[type="password"]')).toBeNull();
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
    const queryKey = settingsQueryKeys.externalIdentities('origin', connection, 'user-alice');
    const view = renderSettings();
    await settle();
    view.unmount();

    removeRegisteredServerQueries('origin');

    expect(queryClient.getQueryData(queryKey)).toBeUndefined();
  });

  it('does not reuse a private snapshot for another authenticated user', async () => {
    let resolveBob!: (result: ReturnType<typeof linkedIdentityList>) => void;
    mocks.list
      .mockResolvedValueOnce(linkedIdentityList())
      .mockImplementationOnce(
        () =>
          new Promise<ReturnType<typeof linkedIdentityList>>((resolve) => (resolveBob = resolve))
      );

    const aliceView = renderSettings();
    await settle();
    expect(aliceView.container.textContent).toContain('GitHub');
    aliceView.unmount();

    const bob = {
      user: { id: 'user-bob', hasPassword: true }
    } as unknown as CurrentUserState;
    const bobView = renderSettings(bob);
    await settle();

    expect(bobView.container.textContent).not.toContain('GitHub');
    resolveBob({ providers: [], linkedIdentities: [] });
    await settle();
  });

  it('refreshes identities without signing out after a successful disconnect', async () => {
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

    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.hardRedirectAfterSignOut).not.toHaveBeenCalled();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
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
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.hardRedirectAfterSignOut).not.toHaveBeenCalled();
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
    expect(mocks.list).toHaveBeenCalledOnce();
  });

  it('leaves expired-session cleanup to the API authentication handler', async () => {
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

    await settle();
    expect(mocks.clearServerAuthentication).not.toHaveBeenCalled();
    expect(mocks.hardRedirectAfterSignOut).not.toHaveBeenCalled();
    expect(mocks.list).toHaveBeenCalledOnce();
    expect(mocks.notifyLogout).not.toHaveBeenCalled();
  });
});

function unlinkedIdentityList() {
  const list = linkedIdentityList();
  list.providers[0].linked = false;
  list.providers[0].linkedIdentitySubjectHash = '';
  list.linkedIdentities = [];
  return list;
}

describe('identity link popup and continuation', () => {
  beforeEach(() => {
    mocks.directLoginEnabled = true;
    queryClient.clear();
    vi.clearAllMocks();
    mocks.url = new URL('http://localhost/chat/-/settings/account');
    mocks.replaceState.mockImplementation((url: string | URL) => {
      mocks.url = new URL(url, mocks.url);
    });
    mocks.serverId = 'origin';
    mocks.scopeCurrent = true;
    connection.serverId = 'origin';
    mocks.list.mockResolvedValue(unlinkedIdentityList());
    mocks.startLink.mockRejectedValue(
      new ConnectError('fresh authentication is required', Code.FailedPrecondition)
    );
  });
  afterEach(() => vi.restoreAllMocks());

  function remote() {
    mocks.serverId = 'remote';
    connection.serverId = 'remote';
    const popup = { closed: false, opener: {}, close: vi.fn() };
    const open = vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    return { popup, open };
  }

  function continuation(userId = 'user-alice', providerId = 'github-main') {
    mocks.url.searchParams.set('link_provider', providerId);
    mocks.url.searchParams.set('link_user', userId);
  }

  it('opens remote settings synchronously without submitting credentials to the remote API', async () => {
    const { popup, open } = remote();
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    expect(open).toHaveBeenCalledOnce();
    expect(popup.opener).toBeNull();
    const url = new URL(authorizationLaunchTarget(open)!);
    expect(url.origin).toBe('https://remote.example.test');
    expect(url.pathname).toBe('/chat/-/settings/account');
    expect([...url.searchParams]).toEqual([
      ['link_provider', 'github-main'],
      ['link_user', 'user-alice']
    ]);
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  it('also opens a popup when linking on the origin server', async () => {
    const { popup, open } = remote();
    mocks.serverId = 'origin';
    connection.serverId = 'origin';
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    expect(open).toHaveBeenCalledOnce();
    expect(popup.opener).toBeNull();
    expect(new URL(authorizationLaunchTarget(open)!).searchParams.get('link_provider')).toBe(
      'github-main'
    );
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  it('shows a blocked popup error', async () => {
    const { open } = remote();
    open.mockReturnValue(null);
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    await expect
      .element(browserPage.getByText('Allow pop-ups for this site, then try linking again.'))
      .toBeVisible();
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  it('updates settings after linking without a focus event', async () => {
    const { popup } = remote();
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    mocks.list.mockResolvedValue(linkedIdentityList());

    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeVisible();
    expect(popup.close).not.toHaveBeenCalled();
    mocks.list.mockClear();
    window.dispatchEvent(new Event('focus'));
    await settle();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('keeps the popup open when only another provider is linked', async () => {
    const { popup } = remote();
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    const other = linkedIdentityList().providers[0];
    mocks.list.mockResolvedValue({
      providers: [...unlinkedIdentityList().providers, { ...other, id: 'other-provider' }],
      linkedIdentities: []
    });
    mocks.list.mockClear();
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalled(), { timeout: 5000 });
    await settle();
    expect(popup.close).not.toHaveBeenCalled();
  });

  it('keeps monitoring after a failed read until a successful link read', async () => {
    const { popup } = remote();
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    mocks.list.mockRejectedValue(new Error('Temporarily unavailable'));
    await expect.element(browserPage.getByText('Temporarily unavailable')).toBeVisible();
    expect(popup.close).not.toHaveBeenCalled();
    mocks.list.mockResolvedValue(linkedIdentityList());
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeVisible();
  });

  it('refreshes on focus and closure, then stops monitoring', async () => {
    const { popup } = remote();
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    mocks.list.mockClear();
    window.dispatchEvent(new Event('focus'));
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledOnce());
    await settle();
    mocks.list.mockClear();
    popup.closed = true;
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledOnce());
    await settle();
    window.dispatchEvent(new Event('focus'));
    await settle();
    expect(mocks.list).toHaveBeenCalledOnce();
  });

  it('replaces an unfinished focus refresh when the popup closes', async () => {
    const { popup } = remote();
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    let finishStaleRead!: (value: ReturnType<typeof linkedIdentityList>) => void;
    mocks.list.mockClear();
    mocks.list
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishStaleRead = resolve;
          })
      )
      .mockResolvedValueOnce(linkedIdentityList());
    window.dispatchEvent(new Event('focus'));
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledOnce());
    popup.closed = true;
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeVisible();
    finishStaleRead(unlinkedIdentityList());
    await settle();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeVisible();
  });

  it.each(['session', 'server', 'unmount'])(
    'does not refresh after %s changes',
    async (boundary) => {
      const { popup } = remote();
      const view = renderSettings();
      await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
      if (boundary === 'session') removeRegisteredServerQueries('remote');
      if (boundary === 'server') mocks.scopeCurrent = false;
      if (boundary === 'unmount') view.unmount();
      mocks.list.mockClear();
      popup.closed = true;
      window.dispatchEvent(new Event('focus'));
      await settle();
      expect(mocks.list).not.toHaveBeenCalled();
    }
  );

  it('consumes a resumed continuation once and keeps password confirmation on the origin', async () => {
    continuation();
    const view = renderSettings();
    await vi.waitFor(() => expect(mocks.startLink).toHaveBeenCalledOnce());
    expect(mocks.replaceState).toHaveBeenCalledOnce();
    expect(mocks.url.search).toBe('');
    expect(mocks.startLink).toHaveBeenCalledWith({
      providerId: 'github-main',
      redirectPath:
        '/chat/-/settings/account?link_provider=github-main&link_user=user-alice&link_complete=1',
      currentPassword: undefined
    });
    await expect
      .element(browserPage.getByRole('dialog', { name: 'Confirm password' }))
      .toBeVisible();
    view.unmount();
    renderSettings();
    await settle();
    expect(mocks.startLink).toHaveBeenCalledOnce();
  });

  it.each([
    ['other-user', 'github-main', 'Sign in to the correct account on this server'],
    ['user-alice', 'missing-provider', 'This sign-in provider is no longer available.'],
    ['', 'github-main', 'Sign in to the correct account on this server']
  ])('rejects a continuation for %s / %s', async (userId, providerId, error) => {
    continuation(userId, providerId);
    renderSettings();
    await expect.element(browserPage.getByText(error, { exact: false })).toBeVisible();
    expect(mocks.startLink).not.toHaveBeenCalled();
    expect(mocks.url.search).toBe('');
  });

  it('does not restart an already linked provider', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    continuation();
    mocks.list.mockResolvedValue(linkedIdentityList());
    renderSettings();
    await vi.waitFor(() => expect(mocks.replaceState).toHaveBeenCalledOnce());
    expect(mocks.startLink).not.toHaveBeenCalled();
    expect(close).toHaveBeenCalledOnce();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeVisible();
  });

  it.each([
    ['user-alice', true, true],
    ['other-user', true, false],
    ['user-alice', false, false]
  ])(
    'closes a completed popup only for its linked account: %s / %s',
    async (userId, linked, shouldClose) => {
      const close = vi.spyOn(window, 'close').mockImplementation(() => {});
      continuation(userId);
      mocks.url.searchParams.set('link_complete', '1');
      mocks.list.mockResolvedValue(linked ? linkedIdentityList() : unlinkedIdentityList());
      renderSettings();
      await vi.waitFor(() => expect(mocks.replaceState).toHaveBeenCalledOnce());
      expect(close).toHaveBeenCalledTimes(shouldClose ? 1 : 0);
      expect(mocks.startLink).not.toHaveBeenCalled();
      expect(mocks.url.search).toBe('');
    }
  );

  it.each([
    [
      'provider_access_denied',
      'Your account at that sign-in provider is not allowed to sign in to this server.'
    ],
    [
      'external_identity_conflict',
      'That provider identity is already linked to a different account.'
    ],
    ['provider_failed', 'Failed to link sign-on provider']
  ])('shows a link callback error in the popup: %s', async (errorCode, message) => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    continuation();
    mocks.url.searchParams.set('link_complete', '1');
    mocks.url.searchParams.set('error', errorCode);
    mocks.list.mockResolvedValue(unlinkedIdentityList());
    renderSettings();
    await expect.element(browserPage.getByText(message, { exact: false })).toBeVisible();
    expect(close).not.toHaveBeenCalled();
    expect(mocks.startLink).not.toHaveBeenCalled();
    expect(mocks.url.search).toBe('');
  });

  it('requires SSO reauthentication instead of a stored password on SSO-only servers', async () => {
    mocks.directLoginEnabled = false;
    continuation();
    renderSettings();
    await expect
      .element(browserPage.getByText(/Open account settings on the server that owns this account/))
      .toBeVisible();
    await expect
      .element(browserPage.getByRole('dialog', { name: 'Confirm password' }))
      .not.toBeInTheDocument();
  });

  it('opens the password dialog ready for input', async () => {
    // A field that renders disabled and enables a tick later can lose typing
    // that starts in between.
    const disabledStates: boolean[] = [];
    const observer = new MutationObserver(() => {
      const input = document.getElementById('sso-link-current-password');
      if (input instanceof HTMLInputElement) disabledStates.push(input.disabled);
    });
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['disabled']
    });
    try {
      continuation();
      renderSettings();
      await expect
        .element(browserPage.getByLabelText('Current Password', { exact: true }))
        .toBeEnabled();
    } finally {
      observer.disconnect();
    }
    expect(disabledStates.length).toBeGreaterThan(0);
    expect(disabledStates).not.toContain(true);
  });

  it('shows a repeated freshness failure in the password dialog', async () => {
    continuation();
    renderSettings();
    await browserPage.getByLabelText('Current Password', { exact: true }).fill('test-password');
    await browserPage.getByRole('button', { name: 'Continue', exact: true }).click();
    await expect
      .element(browserPage.getByText('fresh authentication is required', { exact: true }))
      .toBeVisible();
    expect(mocks.startLink).toHaveBeenLastCalledWith(
      expect.objectContaining({ currentPassword: 'test-password' })
    );
  });
});
