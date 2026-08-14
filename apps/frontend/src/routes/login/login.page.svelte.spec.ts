import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import LoginPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  startRemoteReauthentication: vi.fn(async () => undefined),
  servers: [] as Array<Record<string, unknown>>
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

describe('standalone server selection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.servers = [];
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
    await getByRole('button', { name: 'Sign in' }).click();
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'remote' })
    );
  });
});
