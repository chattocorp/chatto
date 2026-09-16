import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import LoginPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  getPublicServerInfo: vi.fn(),
  authenticatedIds: new Set<string>(),
  startRemoteReauthentication: vi.fn(async () => undefined),
  servers: [] as Array<Record<string, unknown>>
}));

vi.mock('$lib/api-client/server', () => ({ getPublicServerInfo: mocks.getPublicServerInfo }));

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isAuthenticated: (id: string) => mocks.authenticatedIds.has(id),
    get servers() {
      return mocks.servers;
    }
  }
}));

const standaloneData = {
  user: null,
  serverInfo: null,
  serverInfoLoaded: true,
  redirectUrl: '/',
  loginErrorCode: '',
  passwordResetSuccess: false
};

describe('standalone server selection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.servers = [];
    mocks.authenticatedIds.clear();
    mocks.getPublicServerInfo.mockReset();
    mocks.getPublicServerInfo.mockResolvedValue({ name: 'Available', authorizeUrl: '/oauth/authorize' });
  });

  it('keeps saved servers out of the welcome page without checking them', async () => {
    mocks.servers = [
      { id: 'remote', url: 'https://remote.example', name: 'Remote Community', token: null }
    ];
    const { getByRole, getByText } = render(LoginPage, { props: { data: standaloneData } });

    await expect.element(getByRole('heading', { name: 'Choose a server to get started' })).toBeVisible();
    await expect.element(getByRole('link', { name: 'Connect to a server' })).toBeVisible();
    await expect.element(getByText('Remote Community')).not.toBeInTheDocument();
    await expect.element(getByText(/This page checks saved servers directly/)).not.toBeInTheDocument();
    expect(mocks.getPublicServerInfo).not.toHaveBeenCalled();
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    expect(mocks.servers).toHaveLength(1);
  });

  it('opens the full Server Directory instead of a modal', async () => {
    const { getByRole } = render(LoginPage, { props: { data: standaloneData } });

    const link = getByRole('link', { name: 'Connect to a server' });
    await expect.element(link).toHaveAttribute('href', '/chat/servers');
  });

  it('shows provider errors without password controls when password login is disabled', async () => {
    const { getByRole, getByLabelText, getByText } = render(LoginPage, {
      props: {
        data: {
          ...standaloneData,
          loginErrorCode: 'provider_failed',
          serverInfo: {
            name: 'SSO Community',
            version: '0.5.0',
            authorizeUrl: '/oauth/authorize',
            directRegistrationEnabled: false,
            directLoginEnabled: false,
            accountCreationPolicy: 'open',
            welcomeMessage: null,
            description: null,
            iconUrl: null,
            bannerUrl: null,
            authProviders: [
              {
                id: 'company',
                type: 'oidc',
                label: 'Company SSO',
                loginUrl: '/auth/providers/company',
                issuerUrl: 'https://id.example',
                autoProvision: false
              }
            ]
          },
          serverInfoLoaded: true
        }
      }
    });

    await expect.element(getByRole('link', { name: 'Continue with Company SSO' })).toBeVisible();
    await expect
      .element(getByText('The sign-in provider could not complete authentication. Please try again.'))
      .toBeVisible();
    await expect.element(getByLabelText('Username or Email')).not.toBeInTheDocument();
    await expect.element(getByLabelText('Password')).not.toBeInTheDocument();
    await expect.element(getByRole('link', { name: 'Forgot password?' })).not.toBeInTheDocument();
  });
});
