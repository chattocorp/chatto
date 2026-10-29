import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ConfirmPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  startLink: vi.fn(),
  cancel: vi.fn(),
  confirmLink: vi.fn(),
  goto: vi.fn()
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: mocks.goto, pushState: vi.fn() }));
vi.mock('$lib/client', async () => (await import('$lib/test-utils/clientMock')).clientMockDefaults);
vi.mock('$lib/auth/originAuthentication', () => ({ completeOriginAuthentication: vi.fn() }));
vi.mock('$lib/api/externalIdentities', () => ({
  ExternalIdentityFlowKind: { CREATE_ACCOUNT: 1, LINK_ACCOUNT: 2 },
  createExternalIdentityFlowAPI: () => ({ cancel: mocks.cancel, confirmLink: mocks.confirmLink }),
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
