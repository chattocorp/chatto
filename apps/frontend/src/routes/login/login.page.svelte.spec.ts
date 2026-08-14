import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import LoginPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  accountDataSync: {
    status: 'disconnected',
    accountId: null as string | null,
    error: null as string | null,
    initialize: vi.fn(async () => undefined),
    connect: vi.fn(async () => undefined)
  },
  findAuthlingServerProvider: vi.fn(async () => null),
  getClientConfiguration: vi.fn(),
  startRemoteReauthentication: vi.fn(async () => undefined),
  servers: [] as Array<Record<string, unknown>>
}));

vi.mock('$lib/accountData/sync.svelte', () => ({ accountDataSync: mocks.accountDataSync }));
vi.mock('$lib/authling/serverProvider', () => ({
  findAuthlingServerProvider: mocks.findAuthlingServerProvider
}));
vi.mock('$lib/clientConfig', () => ({
  getClientConfiguration: mocks.getClientConfiguration
}));
vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
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

describe('frontend Authling sign-in', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.accountDataSync.status = 'disconnected';
    mocks.accountDataSync.accountId = null;
    mocks.accountDataSync.error = null;
    mocks.servers = [];
    mocks.getClientConfiguration.mockResolvedValue({
      version: 1,
      authling: {
        issuer: 'https://id.example',
        clientId: 'https://app.example/oauth/frontend-client-metadata.json'
      }
    });
  });

  it('shows a trusted Authling action and starts the frontend session', async () => {
    const { getByRole } = render(LoginPage, { props: { data: standaloneData } });

    const button = getByRole('button', { name: 'Continue with Authling' });
    await expect.element(button).toBeVisible();
    await button.click();

    expect(mocks.accountDataSync.connect).toHaveBeenCalledOnce();
  });

  it('shows the Authling identity and synced servers for a connected frontend', async () => {
    mocks.accountDataSync.status = 'connected';
    mocks.accountDataSync.accountId = 'account-123';
    mocks.servers = [
      {
        id: 'remote',
        url: 'https://remote.example',
        name: 'Remote Community',
        iconUrl: null,
        token: null,
        userId: null,
        userLogin: null,
        userDisplayName: null,
        userAvatarUrl: null,
        reauthRequiredAt: null,
        addedAt: 1
      }
    ];

    const { getByText, getByRole } = render(LoginPage, { props: { data: standaloneData } });

    await expect.element(getByText('Authling · account-123')).toBeVisible();
    await expect.element(getByText('Remote Community')).toBeVisible();
    await getByRole('button', { name: 'Sign in' }).click();
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'remote' })
    );
  });

  it('shows provider errors without password controls when password login is disabled', async () => {
    mocks.getClientConfiguration.mockResolvedValue({ version: 1, authling: null });
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
