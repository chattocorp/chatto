import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '@chatto/client/server/registry';

const mocks = vi.hoisted(() => ({
  store: {},
  registered: true,
  replace: vi.fn(),
  resume: vi.fn(),
  login: vi.fn()
}));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    tryGetStore: () => mocks.store,
    getServer: () => (mocks.registered ? {} : undefined),
    replaceServerAuthentication: mocks.replace
  }
}));
vi.mock('$lib/notifications/pushRegistrationCoordinator', () => ({
  resumePushRegistrationAfterAuthentication: mocks.resume
}));

import { hasPasswordLogin, signInWithPassword } from './passwordLogin';

const server = { id: 'remote', url: 'https://remote.example' } as RegisteredServer;
const response = {
  ok: true,
  session: {
    token: 'access',
    refreshToken: 'refresh',
    expiresIn: 900,
    refreshTokenExpiresIn: 86400
  }
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.store = {};
  mocks.registered = true;
  vi.stubGlobal('window', { chattoDesktop: { passwordLogin: mocks.login } });
  mocks.login.mockResolvedValue(response);
});
afterEach(() => vi.unstubAllGlobals());

describe('Desktop password sessions', () => {
  it('keeps web clients on OAuth', async () => {
    vi.stubGlobal('window', {});
    expect(hasPasswordLogin()).toBe(false);
    await expect(
      signInWithPassword(server, 'alice', 'password', new AbortController().signal)
    ).rejects.toThrow();
    expect(mocks.login).not.toHaveBeenCalled();
  });

  it('installs a renewable session only for the selected server', async () => {
    await signInWithPassword(server, 'alice', 'password', new AbortController().signal);
    expect(mocks.login).toHaveBeenCalledWith({
      serverUrl: server.url,
      identifier: 'alice',
      password: 'password'
    });
    expect(mocks.replace).toHaveBeenCalledWith(
      'remote',
      expect.objectContaining({
        token: 'access',
        refreshToken: 'refresh',
        oauthClientId: null,
        reauthRequiredAt: null
      })
    );
    expect(mocks.resume).toHaveBeenCalledWith('remote');
  });

  it.each(['closed', 'removed', 'replaced'])(
    'ignores a late response after the view or session is %s',
    async (change) => {
      let finish!: (value: unknown) => void;
      mocks.login.mockReturnValue(
        new Promise((resolve) => {
          finish = resolve;
        })
      );
      const controller = new AbortController();
      const pending = signInWithPassword(server, 'alice', 'password', controller.signal);
      if (change === 'closed') controller.abort();
      if (change === 'removed') mocks.registered = false;
      if (change === 'replaced') mocks.store = {};
      finish(response);
      await expect(pending).rejects.toThrow();
      expect(mocks.replace).not.toHaveBeenCalled();
      expect(mocks.resume).not.toHaveBeenCalled();
    }
  );

  it.each([{ ok: false }, { ok: true, session: { token: 'legacy' } }])(
    'rejects failed or nonrenewable responses',
    async (result) => {
      mocks.login.mockResolvedValue(result);
      await expect(
        signInWithPassword(server, 'alice', 'password', new AbortController().signal)
      ).rejects.toThrow();
      expect(mocks.replace).not.toHaveBeenCalled();
    }
  );
});
