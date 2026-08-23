import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AuthStatusNotice from './AuthStatusNotice.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    activeServerId: 'origin',
    servers: [] as Array<{
      id: string;
      name: string;
      reauthRequiredAt: number | null;
      refreshToken?: string | null;
      refreshTokenExpiresAt?: number | null;
    }>,
    beginOriginReauthentication: vi.fn(),
    startRemoteReauthentication: vi.fn(() => Promise.resolve()),
    toastError: vi.fn()
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServerId
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get originServer() {
      return mocks.servers.find((server) => server.id === 'origin');
    },
    getServer(id: string) {
      return mocks.servers.find((server) => server.id === id);
    }
  }
}));

vi.mock('$lib/auth/reauth', () => ({
  beginOriginReauthentication: mocks.beginOriginReauthentication,
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    error: mocks.toastError
  }
}));

describe('AuthStatusNotice', () => {
  beforeEach(() => {
    mocks.activeServerId = 'origin';
    mocks.servers = [];
    mocks.beginOriginReauthentication.mockReset();
    mocks.startRemoteReauthentication.mockClear();
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    mocks.toastError.mockReset();
  });

  it('shows an origin reauth notice with a sign-in action', async () => {
    mocks.servers = [{ id: 'origin', name: 'Home', reauthRequiredAt: 123 }];

    const { container } = render(AuthStatusNotice);

    expect(container.textContent).toContain('Session expired');
    const button = container.querySelector<HTMLButtonElement>('button');
    expect(button?.textContent).toContain('Sign in again');

    button?.click();

    expect(mocks.beginOriginReauthentication).toHaveBeenCalledOnce();
  });

  it('shows an active remote reauth notice with a reconnect action', async () => {
    mocks.activeServerId = 'remote';
    const remote = { id: 'remote', name: 'Remote', reauthRequiredAt: 456 };
    mocks.servers = [{ id: 'origin', name: 'Home', reauthRequiredAt: null }, remote];

    const { container } = render(AuthStatusNotice);

    expect(container.textContent).toContain('Remote needs sign-in');
    const button = container.querySelector<HTMLButtonElement>('button');
    expect(button?.textContent).toContain('Reconnect');

    button?.click();

    await vi.waitFor(() => {
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(remote);
    });
  });

  it('warns before the active renewable session expires', async () => {
    mocks.activeServerId = 'remote';
    const remote = {
      id: 'remote',
      name: 'Remote',
      reauthRequiredAt: null,
      refreshToken: 'refresh-token',
      refreshTokenExpiresAt: Date.now() + 6 * 24 * 60 * 60 * 1000
    };
    mocks.servers = [{ id: 'origin', name: 'Home', reauthRequiredAt: null }, remote];

    const { container } = render(AuthStatusNotice);

    expect(container.textContent).toContain('Remote sign-in expires soon');
    const button = container.querySelector<HTMLButtonElement>('button');
    expect(button?.textContent).toContain('Reconnect now');

    button?.click();
    await vi.waitFor(() => {
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(remote);
    });
  });

  it('does not warn before the seven-day renewal window', () => {
    mocks.activeServerId = 'remote';
    mocks.servers = [
      { id: 'origin', name: 'Home', reauthRequiredAt: null },
      {
        id: 'remote',
        name: 'Remote',
        reauthRequiredAt: null,
        refreshToken: 'refresh-token',
        refreshTokenExpiresAt: Date.now() + 8 * 24 * 60 * 60 * 1000
      }
    ];

    const { container } = render(AuthStatusNotice);

    expect(container.textContent.trim()).toBe('');
  });
});
