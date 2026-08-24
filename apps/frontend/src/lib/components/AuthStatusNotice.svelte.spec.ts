import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AuthStatusNotice from './AuthStatusNotice.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    activeServerId: 'origin',
    servers: [] as Array<{ id: string; name: string; reauthRequiredAt: number | null }>,
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
});
