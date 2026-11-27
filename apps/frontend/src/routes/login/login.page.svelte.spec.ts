import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { preloadPublicLocaleMessages } from '$lib/i18n/messages';
import { setLocale } from '$lib/i18n/runtime';
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

const serverInfo = {
  name: 'SSO Community',
  version: '0.5.0',
  authorizeUrl: '/oauth/authorize',
  directRegistrationEnabled: false,
  directLoginEnabled: false,
  accountCreationPolicy: 'open' as const,
  welcomeMessage: null,
  description: null,
  iconUrl: null,
  bannerUrl: null,
  authProviders: []
};

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

    await expect.element(getByRole('heading', { name: 'Please sign in.' })).toBeVisible();
    await expect.element(getByRole('link', { name: 'Company SSO', exact: true })).toBeVisible();
    await expect
      .element(
        getByText('The sign-in provider could not complete authentication. Please try again.')
      )
      .toBeVisible();
    await expect.element(getByLabelText('Username or Email')).not.toBeInTheDocument();
    await expect.element(getByLabelText('Password')).not.toBeInTheDocument();
    await expect.element(getByRole('link', { name: 'Forgot password?' })).not.toBeInTheDocument();
    await expect
      .element(getByRole('link', { name: 'Password', exact: true }))
      .not.toBeInTheDocument();
  });

  it.each([
    {
      autoProvision: true,
      accountCreationPolicy: 'open',
      directLoginEnabled: false
    },
    {
      autoProvision: true,
      accountCreationPolicy: 'invite_only',
      directLoginEnabled: false
    },
    {
      autoProvision: false,
      accountCreationPolicy: 'invite_only',
      directLoginEnabled: false
    },
    {
      autoProvision: null,
      accountCreationPolicy: 'invite_only',
      directLoginEnabled: false
    },
    {
      autoProvision: true,
      accountCreationPolicy: 'invite_only',
      directLoginEnabled: true
    }
  ] as const)(
    'uses the provider directly without an email link ($autoProvision, $accountCreationPolicy, password login: $directLoginEnabled)',
    async ({ autoProvision, accountCreationPolicy, directLoginEnabled }) => {
      const { getByRole, getByText } = render(LoginPage, {
        props: {
          data: {
            ...standaloneData,
            serverInfo: {
              ...serverInfo,
              accountCreationPolicy,
              directLoginEnabled,
              authProviders: [
                {
                  id: 'company',
                  type: 'oidc',
                  issuerUrl: 'https://id.example',
                  label: 'Company SSO',
                  loginUrl: '/auth/providers/company',
                  autoProvision
                }
              ]
            }
          }
        }
      });

      await expect.element(getByRole('link', { name: 'Company SSO', exact: true })).toBeVisible();
      const registration = getByRole('link', { name: 'Create an account with email', exact: true });
      await expect.element(registration).not.toBeInTheDocument();
      await expect
        .element(
          getByRole('heading', {
            name: autoProvision === true ? 'Sign in or create an account' : 'Please sign in.'
          })
        )
        .toBeVisible();
      const inviteNotice = getByText(
        'New accounts need a single-use invitation code. Existing users can still sign in.'
      );
      if (autoProvision === true && accountCreationPolicy === 'invite_only')
        await expect.element(inviteNotice).toBeVisible();
      else await expect.element(inviteNotice).not.toBeInTheDocument();
      if (!directLoginEnabled)
        await expect.element(getByText('or', { exact: true })).not.toBeInTheDocument();
    }
  );

  it('describes only the sign-in-only provider when another provider can create accounts', async () => {
    const { getByRole } = render(LoginPage, {
      props: {
        data: {
          ...standaloneData,
          serverInfo: {
            ...serverInfo,
            authProviders: ['Chatto ID', 'Work SSO', 'Legacy'].map((label, index) => ({
              id: String(index),
              type: 'oidc',
              label,
              issuerUrl: 'https://id.example',
              loginUrl: '/auth/providers/' + index,
              autoProvision: [true, false, null][index]
            }))
          }
        }
      }
    });
    await expect
      .element(getByRole('link', { name: 'Work SSO', exact: true }))
      .toHaveAccessibleDescription('Existing accounts only');
    await expect
      .element(getByRole('link', { name: 'Chatto ID', exact: true }))
      .not.toHaveAttribute('aria-describedby');
    await expect
      .element(getByRole('link', { name: 'Legacy', exact: true }))
      .not.toHaveAttribute('aria-describedby');
    await expect
      .element(getByRole('link', { name: 'Create an account with email' }))
      .not.toBeInTheDocument();
  });

  it('offers direct registration and preserves the return URL when enabled', async () => {
    const { getByRole } = render(LoginPage, {
      props: {
        data: {
          ...standaloneData,
          redirectUrl: '/oauth/authorize',
          serverInfo: { ...serverInfo, directRegistrationEnabled: true, directLoginEnabled: true }
        }
      }
    });

    await expect
      .element(getByRole('link', { name: 'Create an account with email', exact: true }))
      .toHaveAttribute('href', '/register?method=email&redirect=%2Foauth%2Fauthorize');
  });

  it('offers provider, password, and registration links when SSO and direct login are enabled', async ({
    onTestFinished
  }) => {
    const { getByRole, getByLabelText } = render(LoginPage, {
      props: {
        data: {
          ...standaloneData,
          redirectUrl: '/chat/abc',
          serverInfo: {
            ...serverInfo,
            directRegistrationEnabled: true,
            directLoginEnabled: true,
            authProviders: [
              {
                id: 'company',
                type: 'oidc',
                issuerUrl: 'https://id.example',
                label: 'Company SSO',
                loginUrl: '/auth/providers/company',
                autoProvision: false
              }
            ]
          }
        }
      }
    });

    await expect.element(getByRole('link', { name: 'Company SSO', exact: true })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Password', exact: true }))
      .toHaveAttribute('href', '/login/password?redirect=%2Fchat%2Fabc');
    await expect.element(getByLabelText('Username or Email')).not.toBeInTheDocument();
    await expect.element(getByRole('link', { name: 'Forgot password?' })).not.toBeInTheDocument();
    await expect
      .element(getByRole('link', { name: 'Create an account with email', exact: true }))
      .toHaveAttribute('href', '/register?method=email&redirect=%2Fchat%2Fabc');

    onTestFinished(() => setLocale('en-GB'));
    await preloadPublicLocaleMessages();
    await setLocale('de-DE');
    await expect
      .element(getByRole('heading', { name: 'Melde dich an oder erstelle ein Konto' }))
      .toBeVisible();
    await expect.element(getByRole('link', { name: 'Company SSO', exact: true })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Passwort', exact: true }))
      .toHaveAccessibleDescription('');
    await expect
      .element(getByRole('link', { name: 'Company SSO', exact: true }))
      .toHaveAccessibleDescription('Nur für bestehende Konten');
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
