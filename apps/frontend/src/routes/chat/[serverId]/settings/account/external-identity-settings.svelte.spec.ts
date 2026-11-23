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
import { profileVerifiedIdentitiesKey } from '$lib/query/keys';
import { settingsQueryKeys } from '$lib/query/settings';
import ExternalIdentitySettings from './ExternalIdentitySettings.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    url: new URL('http://localhost/chat/-/settings/account'),
    replaceState: vi.fn(),
    leavePage: vi.fn(),
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
vi.mock('$lib/navigation/leavePage', () => ({ leavePage: mocks.leavePage }));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isOriginServer: (serverId: string) => serverId === 'origin',
    clearServerAuthentication: mocks.clearServerAuthentication
  }
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
    props: { currentUser: user }
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

  it('names the linked account when the server resolved one', async () => {
    const list = linkedIdentityList();
    Object.assign(list.linkedIdentities[0], { accountLabel: 'alice.example' });
    mocks.list.mockResolvedValue(list);
    renderSettings();
    await expect.element(browserPage.getByText('alice.example', { exact: true })).toBeVisible();
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
    const queryKey = settingsQueryKeys.externalIdentities(
      { serverId: 'origin', connection },
      'user-alice'
    );
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
    const profileKey = profileVerifiedIdentitiesKey(
      { serverId: 'origin', connection },
      'user-alice'
    );
    queryClient.setQueryData(profileKey, [
      { method: 'atproto', methodLabel: 'AT Protocol', value: 'alice.example' }
    ]);
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
    expect(queryClient.getQueryData(profileKey)).toBeUndefined();
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

describe('identity link popup', () => {
  beforeEach(() => {
    mocks.directLoginEnabled = true;
    queryClient.clear();
    vi.clearAllMocks();
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

  it('opens the provider directly when the origin session is fresh', async () => {
    const popup = { closed: false, opener: {} };
    const open = vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    mocks.startLink.mockResolvedValue('https://example.test/auth/start');
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    expect(open).toHaveBeenCalledWith(
      'https://example.test/auth/start',
      '_blank',
      expect.any(String)
    );
    expect(popup.opener).toBeNull();
    expect(mocks.startLink).toHaveBeenCalledWith({
      providerId: 'github-main',
      redirectPath:
        '/chat/-/settings/account?link_provider=github-main&link_user=user-alice&link_complete=1',
      currentPassword: undefined
    });
  });

  it('refreshes after an isolated provider window returns to the origin', async () => {
    const popup = { closed: false, opener: {} };
    vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    mocks.startLink.mockResolvedValue('https://example.test/auth/start');
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    popup.closed = true;
    await expect
      .element(browserPage.getByRole('button', { name: 'Link', exact: true }))
      .toBeEnabled();
    mocks.list.mockResolvedValue(linkedIdentityList());
    const profileKey = profileVerifiedIdentitiesKey(
      { serverId: 'origin', connection },
      'user-alice'
    );
    queryClient.setQueryData(profileKey, []);
    const callback = new BroadcastChannel('chatto-provider-linked');
    callback.postMessage('user-alice');
    callback.close();
    await expect
      .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }))
      .toBeVisible();
    expect(queryClient.getQueryData(profileKey)).toBeUndefined();
  });

  it('keeps password errors in the original page and retries a blocked provider window', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    const dialog = browserPage.getByRole('dialog', { name: 'Link GitHub?' });
    await dialog.getByLabelText('Chatto password').fill('wrong');
    mocks.startLink.mockRejectedValueOnce(
      new ConnectError('current password is invalid', Code.InvalidArgument)
    );
    await dialog.getByRole('button', { name: 'Continue with GitHub' }).click();
    await expect
      .element(dialog.getByText('current password is invalid', { exact: false }))
      .toBeVisible();
    expect(open).not.toHaveBeenCalled();
    await dialog.getByLabelText('Chatto password').fill('correct');
    mocks.startLink.mockResolvedValue('https://example.test/auth/start');
    await dialog.getByRole('button', { name: 'Continue with GitHub' }).click();
    await expect
      .element(browserPage.getByText('Allow pop-ups for this site, then try linking again.'))
      .toBeVisible();
    open.mockReturnValue({ closed: false, opener: {} } as unknown as Window);
    await browserPage.getByRole('button', { name: 'Continue with GitHub' }).click();
    expect(open).toHaveBeenCalledTimes(2);
    expect(mocks.startLink).toHaveBeenCalledTimes(3);
    await expect.element(browserPage.getByRole('dialog')).not.toBeInTheDocument();
  });

  it('cancels password verification without opening a window', async () => {
    const open = vi.spyOn(window, 'open');
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    await browserPage.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
    await expect.element(browserPage.getByRole('dialog')).not.toBeInTheDocument();
    expect(open).not.toHaveBeenCalled();
    expect(mocks.startLink).toHaveBeenCalledOnce();
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

  it('keeps monitoring a desktop window that an isolated provider page detached', async () => {
    const { popup } = remote();
    window.chattoDesktop = { onAuthorizationWindowClosed: () => () => {} };
    try {
      renderSettings();
      await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
      // The window handle reports closed while the native window stays open.
      popup.closed = true;
      mocks.list.mockClear();
      await vi.waitFor(() => expect(mocks.list).toHaveBeenCalled(), { timeout: 5000 });
      mocks.list.mockResolvedValue(linkedIdentityList());
      await expect
        .element(browserPage.getByRole('button', { name: 'Disconnect', exact: true }), {
          timeout: 5000
        })
        .toBeVisible();
    } finally {
      delete window.chattoDesktop;
    }
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
      renderSettings();
      await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
      await expect
        .element(browserPage.getByLabelText('Chatto password', { exact: true }))
        .toBeEnabled();
    } finally {
      observer.disconnect();
    }
    expect(disabledStates.length).toBeGreaterThan(0);
    expect(disabledStates).not.toContain(true);
  });

  it('shows a repeated freshness failure in the password dialog', async () => {
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    await browserPage.getByLabelText('Chatto password', { exact: true }).fill('test-password');
    await browserPage.getByRole('button', { name: 'Continue with GitHub', exact: true }).click();
    await expect
      .element(browserPage.getByText('fresh authentication is required', { exact: true }))
      .toBeVisible();
    expect(mocks.startLink).toHaveBeenLastCalledWith(
      expect.objectContaining({ currentPassword: 'test-password' })
    );
  });
});

/** The re-verification start URL that the page builds for a provider. */
function reverifyUrl(providerId: string, redirect: string) {
  const url = new URL(`/auth/providers/${providerId}/reverify`, window.location.origin);
  url.searchParams.set('redirect', redirect);
  return url.href;
}

function reverifiableIdentityList(extraProviders = 0) {
  const list = unlinkedIdentityList();
  const ssoProviders = Array.from({ length: 1 + extraProviders }, (_, index) => {
    const id = index === 0 ? 'sso' : `sso-${index}`;
    return {
      id,
      type: 'oidc',
      label: index === 0 ? 'Chatto ID' : `Other SSO ${index}`,
      loginUrl: `/auth/providers/${id}`,
      linkUrl: `/auth/providers/${id}?intent=link`,
      linked: true,
      linkedIdentitySubjectHash: `sso-subject-${index}`,
      reverificationAvailable: true
    };
  });
  return {
    providers: [...list.providers, ...ssoProviders],
    linkedIdentities: ssoProviders.map((provider) => ({
      providerId: provider.id,
      providerType: 'oidc',
      providerLabel: provider.label,
      subjectHash: provider.linkedIdentitySubjectHash,
      disconnectEligibility: DisconnectEligibility.ALLOWED,
      accountLabel: null
    }))
  };
}

describe('fresh sign-in through a linked provider', () => {
  beforeEach(() => {
    mocks.directLoginEnabled = false;
    queryClient.clear();
    vi.clearAllMocks();
    mocks.url = new URL('http://localhost/chat/-/settings/account');
    mocks.replaceState.mockImplementation((url: string | URL) => {
      mocks.url = new URL(url, mocks.url);
    });
    mocks.serverId = 'origin';
    mocks.scopeCurrent = true;
    connection.serverId = 'origin';
    mocks.list.mockResolvedValue(reverifiableIdentityList());
    mocks.startLink.mockRejectedValue(
      new ConnectError('fresh authentication is required', Code.FailedPrecondition)
    );
    mocks.disconnect.mockRejectedValue(
      new ConnectError('fresh authentication is required', Code.FailedPrecondition)
    );
  });
  afterEach(() => vi.restoreAllMocks());

  it('re-verifies in the provider window, which then continues in the link window', async () => {
    const popup = { closed: false, opener: {} };
    const open = vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    await vi.waitFor(() =>
      expect(open).toHaveBeenCalledWith(
        reverifyUrl('sso', '/sso/link?link_provider=github-main&link_user=user-alice'),
        '_blank',
        expect.any(String)
      )
    );
    expect(popup.opener).toBeNull();
    expect(mocks.leavePage).not.toHaveBeenCalled();
  });

  it('lets the user choose when several providers can re-verify', async () => {
    mocks.list.mockResolvedValue(reverifiableIdentityList(1));
    renderSettings();
    await browserPage.getByRole('button', { name: 'Link', exact: true }).click();
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const dialog = browserPage.getByRole('dialog', { name: 'Confirm it’s you' });
    await expect.element(dialog).toBeVisible();
    expect(open).not.toHaveBeenCalled();
    await dialog.getByRole('button', { name: 'Sign in again with Other SSO 1' }).click();
    await vi.waitFor(() =>
      expect(open).toHaveBeenCalledWith(
        reverifyUrl('sso-1', '/sso/link?link_provider=github-main&link_user=user-alice'),
        '_blank',
        expect.any(String)
      )
    );
  });

  it('re-verifies before a disconnect and returns to this page', async () => {
    renderSettings();
    await browserPage.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await browserPage
      .getByRole('dialog', { name: 'Disconnect provider' })
      .getByRole('button', { name: 'Disconnect', exact: true })
      .click();
    await vi.waitFor(() =>
      expect(mocks.leavePage).toHaveBeenCalledWith(
        reverifyUrl(
          'sso',
          '/chat/-/settings/account?disconnect_identity=sso-subject-0&disconnect_user=user-alice'
        )
      )
    );
  });

  it('asks for confirmation again on return and does not loop', async () => {
    mocks.url.searchParams.set('disconnect_identity', 'sso-subject-0');
    mocks.url.searchParams.set('disconnect_user', 'user-alice');
    mocks.url.searchParams.set('reverified', '1');
    renderSettings();
    const dialog = browserPage.getByRole('dialog', { name: 'Disconnect provider' });
    await expect.element(dialog).toBeVisible();
    expect(mocks.url.search).toBe('');
    expect(mocks.disconnect).not.toHaveBeenCalled();
    await dialog.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await vi.waitFor(() =>
      expect(mocks.disconnect).toHaveBeenCalledWith('sso-subject-0', undefined)
    );
    await expect
      .element(browserPage.getByText(/sign in again before you change your sign-in methods/))
      .toBeVisible();
    expect(mocks.leavePage).not.toHaveBeenCalled();
  });

  it('re-verifies again after a cancelled resumed disconnect', async () => {
    mocks.url.searchParams.set('disconnect_identity', 'sso-subject-0');
    mocks.url.searchParams.set('disconnect_user', 'user-alice');
    mocks.url.searchParams.set('reverified', '1');
    renderSettings();
    const dialog = browserPage.getByRole('dialog', { name: 'Disconnect provider' });
    await expect.element(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect.element(dialog).not.toBeInTheDocument();
    await browserPage.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await browserPage
      .getByRole('dialog', { name: 'Disconnect provider' })
      .getByRole('button', { name: 'Disconnect', exact: true })
      .click();
    await vi.waitFor(() =>
      expect(mocks.leavePage).toHaveBeenCalledWith(
        reverifyUrl(
          'sso',
          '/chat/-/settings/account?disconnect_identity=sso-subject-0&disconnect_user=user-alice'
        )
      )
    );
  });

  it('offers another attempt after an identity of another account', async () => {
    mocks.url.searchParams.set('disconnect_identity', 'sso-subject-0');
    mocks.url.searchParams.set('disconnect_user', 'user-alice');
    mocks.url.searchParams.set('error', 'reverification_mismatch');
    renderSettings();
    const dialog = browserPage.getByRole('dialog', { name: 'Confirm it’s you' });
    await expect
      .element(dialog.getByText(/identity that belongs to a different account/))
      .toBeVisible();
    await dialog.getByRole('button', { name: 'Sign in again with Chatto ID' }).click();
    await vi.waitFor(() =>
      expect(mocks.leavePage).toHaveBeenCalledWith(
        reverifyUrl(
          'sso',
          '/chat/-/settings/account?disconnect_identity=sso-subject-0&disconnect_user=user-alice'
        )
      )
    );
  });
});
