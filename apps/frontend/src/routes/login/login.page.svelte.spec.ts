import { queryClient } from '$lib/query/client';
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
    queryClient.clear();
    mocks.getPublicServerInfo.mockReset();
    mocks.getPublicServerInfo.mockResolvedValue({ name: 'Available', authorizeUrl: '/oauth/authorize' });
  });

  it('shows signed-out locally known servers', async () => {
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

    await expect.element(getByText('Remote Community')).toBeVisible();
    await expect.element(getByRole('button', { name: 'Sign in' })).toBeEnabled();
    await getByRole('button', { name: 'Sign in' }).click();
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'remote' })
    );
  });

  it('keeps an unavailable remembered origin and retries before enabling sign-in', async () => {
    mocks.servers = [{ id: 'origin', url: window.location.origin, name: 'Remembered origin', token: null }];
    mocks.getPublicServerInfo.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    const { getByText, getByRole } = render(LoginPage, { props: { data: standaloneData } });

    await expect.element(getByText('Server unavailable')).toBeVisible();
    await expect.element(getByText('Remembered origin')).toBeVisible();
    await expect.element(getByRole('button', { name: 'Sign in' })).not.toBeInTheDocument();
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    expect(mocks.servers).toHaveLength(1);

    await getByRole('button', { name: 'Try Again' }).click();
    await expect.element(getByRole('button', { name: 'Sign in' })).toBeEnabled();
    await getByRole('button', { name: 'Sign in' }).click();
    expect(mocks.getPublicServerInfo).toHaveBeenCalledTimes(2);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledOnce();
  });

  it('does not offer sign-in while discovery is pending', async () => {
    mocks.servers = [{ id: 'remote', url: 'https://remote.example', name: 'Remote', token: null }];
    mocks.getPublicServerInfo.mockImplementationOnce((_url, { signal }) =>
      new Promise((_resolve, reject) => signal.addEventListener('abort', () => reject(signal.reason)))
    );
    const { getByText, getByRole, unmount } = render(LoginPage, { props: { data: standaloneData } });
    await expect.element(getByText('Checking server…')).toBeVisible();
    await expect.element(getByRole('button', { name: 'Sign in' })).toBeDisabled();
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    await unmount();
    expect(mocks.getPublicServerInfo.mock.calls[0][1].signal.aborted).toBe(true);
  });

  it('excludes cookie-authenticated servers without bearer tokens', async () => {
    mocks.servers = [
      { id: 'origin', url: window.location.origin, name: 'Signed-in origin', token: null },
      { id: 'remote', url: 'https://remote.example', name: 'Signed-out remote', token: null }
    ];
    mocks.authenticatedIds.add('origin');
    const { getByText, getByRole } = render(LoginPage, { props: { data: standaloneData } });
    await expect.element(getByText('Signed-in origin')).not.toBeInTheDocument();
    await expect.element(getByText('Signed-out remote')).toBeVisible();
    await expect.element(getByRole('button', { name: 'Sign in' })).toBeEnabled();
    expect(mocks.getPublicServerInfo).toHaveBeenCalledTimes(1);
    expect(mocks.getPublicServerInfo.mock.calls[0][0]).toBe('https://remote.example');
  });

  it('keeps sign-in disabled when discovery has no authorization endpoint', async () => {
    mocks.servers = [{ id: 'remote', url: 'https://remote.example', name: 'Remote', token: null }];
    mocks.getPublicServerInfo.mockResolvedValueOnce({ name: 'Remote', authorizeUrl: '' });
    const { getByRole } = render(LoginPage, { props: { data: standaloneData } });
    await expect.element(getByRole('button', { name: 'Sign in' })).toBeDisabled();
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
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
