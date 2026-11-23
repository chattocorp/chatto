import { Code, ConnectError } from '@connectrpc/connect';
import { StaleResponseError } from '@chatto/client/api/connect';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  url: new URL('https://chat.example/sso/link'),
  list: vi.fn(),
  startLink: vi.fn(),
  startReverification: vi.fn(),
  leavePage: vi.fn()
}));

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return mocks.url;
    }
  }
}));
vi.mock('$lib/navigation/leavePage', () => ({ leavePage: mocks.leavePage }));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: { originServer: { id: 'origin' } },
  serverConnectionManager: {
    getClient: () => ({
      getAPI: () => ({
        list: mocks.list,
        startLink: mocks.startLink,
        startReverification: mocks.startReverification
      })
    })
  }
}));

import LinkPage from './+page.svelte';

function provider(linked: boolean) {
  return {
    id: 'github-main',
    type: 'github',
    label: 'GitHub',
    loginUrl: '/auth/github',
    linkUrl: '/auth/github?intent=link',
    linked,
    linkedIdentitySubjectHash: linked ? 'subject-1' : ''
  };
}

function open(
  query: Record<string, string>,
  { hasPassword = true, directLoginEnabled = true } = {}
) {
  mocks.url = new URL(`${window.location.origin}/sso/link?${new URLSearchParams(query)}`);
  return render(LinkPage, {
    props: {
      data: { user: { id: 'user-alice', hasPassword }, directLoginEnabled }
    } as never
  });
}

const link = { link_provider: 'github-main', link_user: 'user-alice' };
const freshAuthRequired = new ConnectError(
  'fresh authentication is required',
  Code.FailedPrecondition
);

describe('identity-link window page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockResolvedValue({ providers: [provider(false)], linkedIdentities: [] });
    mocks.startLink.mockRejectedValue(freshAuthRequired);
  });
  afterEach(() => vi.restoreAllMocks());

  async function confirm(screen: ReturnType<typeof open>) {
    await expect
      .element(screen.getByText('Once linked, you can use GitHub to sign in to Chatto.'))
      .toBeVisible();
    await screen.getByRole('button', { name: 'Continue with GitHub' }).click();
  }

  it('waits for a confirmation before it starts the link', async () => {
    const screen = open(link);
    await expect.element(screen.getByRole('heading', { name: 'Link GitHub?' })).toBeVisible();
    await expect
      .element(screen.getByRole('button', { name: 'Continue with GitHub' }))
      .toBeEnabled();
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  it('cancels by closing the window', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const screen = open(link);
    await screen.getByRole('button', { name: 'Cancel' }).click();
    expect(close).toHaveBeenCalledOnce();
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  it('sends the window to the provider when the session is fresh', async () => {
    mocks.startLink.mockResolvedValue('https://provider.example/authorize');
    const screen = open(link);
    await confirm(screen);
    await vi.waitFor(() =>
      expect(mocks.leavePage).toHaveBeenCalledWith('https://provider.example/authorize')
    );
    expect(mocks.startLink).toHaveBeenCalledWith({
      providerId: 'github-main',
      redirectPath:
        '/chat/-/settings/account?link_provider=github-main&link_user=user-alice&link_complete=1',
      currentPassword: undefined
    });
  });

  it('asks for the password, then sends the window to the provider', async () => {
    const screen = open(link);
    await confirm(screen);
    const field = screen.getByLabelText('Chatto password');
    await expect.element(field).toBeEnabled();
    await field.fill('correct');
    mocks.startLink.mockResolvedValueOnce('https://provider.example/authorize');
    await screen.getByRole('button', { name: 'Continue with GitHub' }).click();
    await vi.waitFor(() =>
      expect(mocks.leavePage).toHaveBeenCalledWith('https://provider.example/authorize')
    );
    expect(mocks.startLink).toHaveBeenLastCalledWith(
      expect.objectContaining({ currentPassword: 'correct' })
    );
  });

  it('shows a password error on the page', async () => {
    const screen = open(link);
    await confirm(screen);
    await screen.getByLabelText('Chatto password').fill('wrong');
    mocks.startLink.mockRejectedValueOnce(
      new ConnectError('current password is invalid', Code.InvalidArgument)
    );
    await screen.getByRole('button', { name: 'Continue with GitHub' }).click();
    await expect
      .element(screen.getByText('current password is invalid', { exact: false }))
      .toBeVisible();
    await expect.element(screen.getByLabelText('Chatto password')).toBeEnabled();
    expect(mocks.leavePage).not.toHaveBeenCalled();
  });

  it('repeats a provider read that a privacy reset discarded', async () => {
    mocks.list.mockRejectedValueOnce(new StaleResponseError(false));
    const screen = open(link);
    await expect
      .element(screen.getByRole('button', { name: 'Continue with GitHub' }))
      .toBeEnabled();
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it('requires a provider sign-in without a retry on SSO-only servers', async () => {
    const screen = open(link, { directLoginEnabled: false });
    await confirm(screen);
    await expect
      .element(screen.getByText(/sign in again before you change your sign-in methods/))
      .toBeVisible();
    expect(screen.getByLabelText('Chatto password').query()).toBeNull();
    expect(screen.getByRole('button', { name: 'Try Again' }).query()).toBeNull();
    await expect.element(screen.getByRole('button', { name: 'Close window' })).toBeVisible();
  });

  it.each([
    [{ link_provider: 'github-main', link_user: '' }, 'This sign-in link is invalid'],
    [{ link_provider: '', link_user: 'user-alice' }, 'This sign-in link is invalid'],
    [{ link_provider: 'github-main', link_user: 'other-user' }, 'Sign in to the correct account'],
    [
      { link_provider: 'missing-provider', link_user: 'user-alice' },
      'This sign-in provider is no longer available.'
    ]
  ])('refuses %o without a retry', async (query, message) => {
    const screen = open(query);
    await expect.element(screen.getByText(message, { exact: false })).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Close window' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Try Again' }).query()).toBeNull();
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  it.each([
    ['after a completed link', { ...link, link_complete: '1' }],
    ['when the provider was already linked', link]
  ])('tells account settings and closes %s', async (_case, query) => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const messages: unknown[] = [];
    const channel = new BroadcastChannel('chatto-provider-linked');
    channel.onmessage = ({ data }) => messages.push(data);
    mocks.list.mockResolvedValue({ providers: [provider(true)], linkedIdentities: [] });
    try {
      const screen = open(query);
      await expect
        .element(screen.getByText('GitHub is now linked. You can close this window.'))
        .toBeVisible();
      expect(close).toHaveBeenCalledOnce();
      await vi.waitFor(() => expect(messages).toEqual(['user-alice']));
      expect(mocks.startLink).not.toHaveBeenCalled();
    } finally {
      channel.close();
    }
  });

  it.each([
    [
      'a callback error',
      { ...link, link_complete: '1', error: 'provider_access_denied' },
      'Your account at that sign-in provider is not allowed to sign in to this server.'
    ],
    [
      'a conflict',
      { ...link, link_complete: '1', error: 'external_identity_conflict' },
      'That provider identity is already linked to a different account.'
    ],
    [
      'a callback without a link',
      { ...link, link_complete: '1' },
      'Failed to link sign-on provider'
    ]
  ])('offers to start again after %s', async (_case, query, message) => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const screen = open(query);
    await expect.element(screen.getByText(message, { exact: false })).toBeVisible();
    await expect.element(screen.getByRole('heading', { name: 'Link GitHub?' })).toBeVisible();
    expect(close).not.toHaveBeenCalled();
    await screen.getByRole('button', { name: 'Try Again' }).click();
    expect(mocks.leavePage).toHaveBeenCalledWith(
      '/sso/link?link_provider=github-main&link_user=user-alice'
    );
    await screen.getByRole('button', { name: 'Close window' }).click();
    expect(close).toHaveBeenCalledOnce();
  });

  it('shows a callback error even when the providers cannot load', async () => {
    mocks.list.mockRejectedValue(new ConnectError('unavailable', Code.Unavailable));
    const screen = open({ ...link, link_complete: '1', error: 'external_identity_conflict' });
    await expect
      .element(screen.getByText('That provider identity is already linked to a different account.'))
      .toBeVisible();
    await expect.element(screen.getByRole('heading', { name: 'Single Sign-On' })).toBeVisible();
  });

  it('offers to start again when the providers cannot load', async () => {
    mocks.list.mockRejectedValue(new ConnectError('unavailable', Code.Unavailable));
    const screen = open(link);
    await expect.element(screen.getByRole('button', { name: 'Try Again' })).toBeVisible();
    expect(mocks.startLink).not.toHaveBeenCalled();
  });

  describe('fresh sign-in through a linked provider', () => {
    function sso(label: string, id = 'sso') {
      return {
        id,
        type: 'oidc',
        label,
        loginUrl: `/auth/providers/${id}`,
        linkUrl: `/auth/providers/${id}?intent=link`,
        linked: true,
        linkedIdentitySubjectHash: `${id}-subject`,
        reverificationAvailable: true
      };
    }
    const ssoOnly = { hasPassword: false, directLoginEnabled: false };

    beforeEach(() => {
      mocks.list.mockResolvedValue({
        providers: [provider(false), sso('Chatto ID')],
        linkedIdentities: []
      });
      mocks.startReverification.mockResolvedValue('https://idp.example/authorize');
    });

    it('re-verifies with the only linked provider and returns to the window', async () => {
      const screen = open(link, ssoOnly);
      await confirm(screen);
      await vi.waitFor(() =>
        expect(mocks.leavePage).toHaveBeenCalledWith('https://idp.example/authorize')
      );
      expect(mocks.startReverification).toHaveBeenCalledWith({
        providerId: 'sso',
        redirectPath: '/sso/link?link_provider=github-main&link_user=user-alice'
      });
    });

    it('lets the user choose when several providers can re-verify', async () => {
      mocks.list.mockResolvedValue({
        providers: [provider(false), sso('Chatto ID'), sso('Company SSO', 'company')],
        linkedIdentities: []
      });
      const screen = open(link, ssoOnly);
      await confirm(screen);
      await expect.element(screen.getByRole('heading', { name: 'Confirm it’s you' })).toBeVisible();
      await screen.getByRole('button', { name: 'Sign in again with Company SSO' }).click();
      await vi.waitFor(() =>
        expect(mocks.startReverification).toHaveBeenCalledWith(
          expect.objectContaining({ providerId: 'company' })
        )
      );
    });

    it('starts the link after a re-verification and does not loop', async () => {
      const screen = open({ ...link, reverified: '1' }, ssoOnly);
      await vi.waitFor(() => expect(mocks.startLink).toHaveBeenCalledOnce());
      await expect
        .element(screen.getByText(/sign in again before you change your sign-in methods/))
        .toBeVisible();
      expect(mocks.startReverification).not.toHaveBeenCalled();
    });

    it('offers another attempt after an identity of another account', async () => {
      const screen = open({ ...link, error: 'reverification_mismatch' }, ssoOnly);
      await expect
        .element(screen.getByText(/identity that belongs to a different account/))
        .toBeVisible();
      expect(mocks.startLink).not.toHaveBeenCalled();
      await screen.getByRole('button', { name: 'Sign in again with Chatto ID' }).click();
      await vi.waitFor(() => expect(mocks.startReverification).toHaveBeenCalledOnce());
    });
  });
});
