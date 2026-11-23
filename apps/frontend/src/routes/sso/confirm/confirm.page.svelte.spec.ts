import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ConfirmPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  startLink: vi.fn(),
  cancel: vi.fn(),
  confirmLink: vi.fn(),
  createAccount: vi.fn(),
  goto: vi.fn()
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: mocks.goto, pushState: vi.fn() }));
vi.mock('$lib/client', async () => (await import('$lib/test-utils/clientMock')).clientMockDefaults);
vi.mock('$lib/auth/originAuthentication', () => ({ completeOriginAuthentication: vi.fn() }));
vi.mock('$lib/api/externalIdentities', () => ({
  ExternalIdentityFlowKind: { CREATE_ACCOUNT: 1, LINK_ACCOUNT: 2 },
  createExternalIdentityFlowAPI: () => ({
    cancel: mocks.cancel,
    confirmLink: mocks.confirmLink,
    createAccount: mocks.createAccount
  }),
  createExternalIdentityAPI: () => ({ startLink: mocks.startLink })
}));

const data = {
  token: 'create-token',
  existingAccount: true,
  loadError: null,
  user: { id: 'account', login: 'existing', displayName: 'Existing User' },
  pending: {
    kind: 1,
    providerId: 'discord-main',
    providerType: 'discord',
    providerLabel: 'Discord',
    loginHint: 'new-user',
    displayNameHint: '',
    verifiedEmail: null,
    boundUserId: null,
    redirectPath: '/servers/callback?mode=provider&state=original-state'
  }
};

describe('existing-account provider confirmation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.cancel.mockResolvedValue(undefined);
    mocks.startLink.mockRejectedValue(new Error('Link start failed'));
  });

  it('preserves the create flow when starting a bound link fails, and allows retry', async () => {
    const { getByRole, getByText } = render(ConfirmPage, { props: { data: data as never } });
    await expect.element(getByText('@existing', { exact: true })).toBeVisible();
    const button = getByRole('button', { name: 'Link Account', exact: true });
    await button.click();
    await expect.element(getByText('Link start failed')).toBeVisible();
    expect(mocks.startLink).toHaveBeenCalledWith({
      providerId: 'discord-main',
      redirectPath: data.pending.redirectPath
    });
    expect(mocks.cancel).not.toHaveBeenCalled();
    expect(mocks.confirmLink).not.toHaveBeenCalled();
    await button.click();
    expect(mocks.startLink).toHaveBeenCalledTimes(2);
  });

  it('returns cancellation to the opening window even when cleanup fails', async () => {
    mocks.cancel.mockRejectedValue(new Error('Network unavailable'));
    const { getByRole } = render(ConfirmPage, { props: { data: data as never } });
    await getByRole('button', { name: 'Cancel', exact: true }).click();
    expect(mocks.goto).toHaveBeenCalledWith(
      '/servers/callback?mode=provider&state=original-state&error=access_denied',
      { replaceState: true }
    );
    expect(mocks.startLink).not.toHaveBeenCalled();
  });
});

describe('new-account provider confirmation', () => {
  const createData = (chooseUsername: number) => ({
    ...data,
    existingAccount: false,
    user: null,
    pending: {
      ...data.pending,
      providerType: 'atproto',
      providerLabel: 'Internet Handle',
      loginHint: 'alice.example.social',
      avatarUrl: '/auth/external-identity-avatar?token=create-token',
      chooseUsername
    }
  });

  it('shows a provider-managed username as a disabled field and never sends it', async () => {
    mocks.createAccount.mockResolvedValue({ userId: 'new', login: 'alice.example.social' });
    const { UsernameChoiceAvailability } = await import('@chatto/client/api/actionAvailability');
    const screen = render(ConfirmPage, {
      props: { data: createData(UsernameChoiceAvailability.PROVIDER_MANAGED) as never }
    });

    await expect.element(screen.getByRole('heading', { name: /^Join / })).toBeVisible();
    await expect
      .element(screen.getByText('Signed in with Internet Handle', { exact: true }))
      .toBeVisible();
    const preview = screen.getByRole('region', { name: 'Profile preview' });
    await expect
      .element(preview.getByRole('img', { name: 'alice.example.social' }))
      .toHaveAttribute('src', '/auth/external-identity-avatar?token=create-token');
    await expect.element(preview.getByText('@alice.example.social', { exact: true })).toBeVisible();
    const username = screen.getByRole('textbox', { name: 'Username' });
    await expect.element(username).toBeDisabled();
    await expect.element(username).toHaveValue('alice.example.social');
    await expect
      .element(screen.getByText('Your sign-in provider sets this username.', { exact: true }))
      .toBeVisible();
    await expect
      .element(screen.getByLabelText('Display Name'))
      .toHaveAttribute('placeholder', 'alice.example.social');
    await expect
      .element(screen.getByRole('link', { name: 'Sign in to existing account' }))
      .toHaveAttribute('href', '/sso/confirm?token=create-token&existing=1');

    await screen.getByRole('button', { name: 'Create account' }).click();
    expect(mocks.createAccount).toHaveBeenCalledWith({
      token: 'create-token',
      login: '',
      displayName: ''
    });
  });

  it('asks for a username when the user chooses it', async () => {
    const { UsernameChoiceAvailability } = await import('@chatto/client/api/actionAvailability');
    const screen = render(ConfirmPage, {
      props: { data: createData(UsernameChoiceAvailability.ALLOWED) as never }
    });

    const username = screen.getByRole('textbox', { name: 'Username' });
    await expect.element(username).toBeEnabled();
    await expect.element(username).toHaveValue('alice.example.social');

    // The preview follows the form, so the user sees the profile they create.
    const preview = screen.getByRole('region', { name: 'Profile preview' });
    await username.fill('alice');
    await screen.getByLabelText('Display Name').fill('Alice Liddell');
    await expect.element(preview.getByText('Alice Liddell', { exact: true })).toBeVisible();
    await expect.element(preview.getByText('@alice', { exact: true })).toBeVisible();
  });
});
