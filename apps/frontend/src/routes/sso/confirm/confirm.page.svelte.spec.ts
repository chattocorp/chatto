import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ConfirmPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  startLink: vi.fn(),
  cancel: vi.fn(),
  confirmLink: vi.fn(),
  createAccount: vi.fn(),
  ensureAtprotoLinkRecord: vi.fn(),
  goto: vi.fn()
}));
vi.mock('$lib/atproto/signIn', () => ({ ensureAtprotoLinkRecord: mocks.ensureAtprotoLinkRecord }));
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

describe('AT Protocol account creation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.ensureAtprotoLinkRecord.mockResolvedValue(undefined);
  });

  it('writes the account link record for the DID that the server linked', async () => {
    mocks.createAccount.mockResolvedValue({
      userId: 'U1',
      login: 'alice.test',
      atprotoDid: 'did:plc:alice'
    });
    const atprotoData = {
      ...data,
      existingAccount: false,
      user: null,
      pending: {
        ...data.pending,
        providerId: 'atproto',
        providerType: 'atproto',
        providerLabel: 'Internet Handle',
        loginHint: 'alice.test',
        displayNameHint: 'Alice',
        redirectPath: '/'
      }
    };
    const { getByRole } = render(ConfirmPage, { props: { data: atprotoData as never } });
    await getByRole('button', { name: 'Create Account', exact: true }).click();
    await vi.waitFor(() =>
      expect(mocks.ensureAtprotoLinkRecord).toHaveBeenCalledWith(
        { serverOrigin: window.location.origin, userId: 'U1' },
        'did:plc:alice'
      )
    );
  });

  it('writes no record without a linked DID', async () => {
    mocks.createAccount.mockResolvedValue({ userId: 'U1', login: 'new-user' });
    const { getByRole } = render(ConfirmPage, {
      props: { data: { ...data, existingAccount: false, user: null } as never }
    });
    await getByRole('button', { name: 'Create Account', exact: true }).click();
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalled());
    expect(mocks.ensureAtprotoLinkRecord).not.toHaveBeenCalled();
  });
});
