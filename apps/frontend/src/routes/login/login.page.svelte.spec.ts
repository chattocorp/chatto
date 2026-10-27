import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import LoginPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  getPublicServerInfo: vi.fn(),
  authenticatedIds: new Set<string>(),
  startRemoteReauthentication: vi.fn(async () => undefined),
  servers: [] as Array<Record<string, unknown>>
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isAuthenticated: (id: string) => mocks.authenticatedIds.has(id),
    get servers() {
      return mocks.servers;
    }
  }
}));

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('@chatto/client/api/server', () => ({ getPublicServerInfo: mocks.getPublicServerInfo }));

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));

const standaloneData = {
  user: null,
  serverInfo: null,
  serverInfoLoaded: true,
  redirectUrl: '/',
  loginErrorCode: '',
  passwordResetSuccess: false
};

describe('origin sign-in page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.servers = [];
    mocks.authenticatedIds.clear();
    mocks.getPublicServerInfo.mockReset();
    mocks.getPublicServerInfo.mockResolvedValue({
      name: 'Available',
      authorizeUrl: '/oauth/authorize'
    });
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
      .element(
        getByText('The sign-in provider could not complete authentication. Please try again.')
      )
      .toBeVisible();
    await expect.element(getByLabelText('Username or Email')).not.toBeInTheDocument();
    await expect.element(getByLabelText('Password')).not.toBeInTheDocument();
    await expect.element(getByRole('link', { name: 'Forgot password?' })).not.toBeInTheDocument();
  });

  it('explains provider restriction denials without naming the rule', async () => {
    const { getByText } = render(LoginPage, {
      props: { data: { ...standaloneData, loginErrorCode: 'provider_access_denied' } }
    });

    await expect
      .element(
        getByText('Your account at that sign-in provider is not allowed to sign in to this server.')
      )
      .toBeVisible();
  });
});
