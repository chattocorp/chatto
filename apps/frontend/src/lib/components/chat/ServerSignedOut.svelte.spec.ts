import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  startRemoteReauthentication: vi.fn(),
  pushState: vi.fn(),
  desktop: false,
  passwordLogin: vi.fn()
}));

vi.mock('$lib/desktop/passwordLogin', () => ({
  hasPasswordLogin: () => mocks.desktop,
  signInWithPassword: mocks.passwordLogin
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState, goto: vi.fn() }));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));

import { toast } from '$lib/ui/toast';
import ServerSignedOut from './ServerSignedOut.svelte';

const toastError = vi.spyOn(toast, 'error');

const registration = {
  id: 'remote',
  name: 'Saved Server',
  url: 'https://chat.example.test:8443',
  iconUrl: null,
  token: null,
  reauthRequiredAt: null
} as unknown as RegisteredServer;

beforeEach(() => {
  vi.clearAllMocks();
  mocks.desktop = false;
  // Failed or pending discovery keeps the default name.
  createTestServerScope({ serverInfo: { name: 'Chatto', version: '', iconUrl: null } });
});

describe('ServerSignedOut', () => {
  it.each([true, false])(
    'offers Desktop signup only when registration is enabled=%s',
    async (enabled) => {
      mocks.desktop = true;
      mocks.startRemoteReauthentication.mockResolvedValue(undefined);
      createTestServerScope({
        serverInfo: { name: 'Saved Server', loading: false, directRegistrationEnabled: enabled },
        store: { realtimeSync: { hasDisplayableView: false } }
      });
      render(ServerSignedOut, { props: { registration } });
      const signup = page.getByRole('button', { name: 'Create Account', exact: true });
      if (!enabled) {
        await expect.element(signup).not.toBeInTheDocument();
        return;
      }
      await signup.click();
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(
        registration,
        undefined,
        'signup'
      );
      expect(mocks.passwordLogin).not.toHaveBeenCalled();
    }
  );
  it.each([true, false])(
    'shows provider buttons with password login enabled=%s',
    async (directLoginEnabled) => {
      mocks.desktop = true;
      mocks.startRemoteReauthentication.mockResolvedValue(undefined);
      createTestServerScope({
        serverInfo: {
          name: 'Saved Server',
          loading: false,
          directLoginEnabled,
          authProviders: [{ id: 'chatto-id', type: 'oidc', label: 'Chatto ID' }]
        },
        store: { realtimeSync: { hasDisplayableView: false } }
      });
      render(ServerSignedOut, { props: { registration } });
      if (directLoginEnabled)
        await expect.element(page.getByLabelText('Password', { exact: true })).toBeVisible();
      else
        await expect
          .element(page.getByLabelText('Password', { exact: true }))
          .not.toBeInTheDocument();
      await page.getByRole('button', { name: 'Continue with Chatto ID' }).click();
      expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(registration, 'chatto-id');
      expect(mocks.passwordLogin).not.toHaveBeenCalled();
    }
  );

  it('shows the Desktop password form and submits without opening OAuth', async () => {
    mocks.desktop = true;
    createTestServerScope({
      serverInfo: { name: 'Saved Server', loading: false, directLoginEnabled: true },
      store: { realtimeSync: { hasDisplayableView: false } }
    });
    render(ServerSignedOut, { props: { registration } });
    await page.getByLabelText('Username or Email').fill('alice');
    await page.getByLabelText('Password', { exact: true }).fill('example-password');
    await page.getByRole('button', { name: 'Sign In', exact: true }).click();
    expect(mocks.passwordLogin).toHaveBeenCalledWith(
      registration,
      'alice',
      'example-password',
      expect.any(AbortSignal)
    );
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    await expect.element(page.getByLabelText('Password', { exact: true })).toHaveValue('');
  });

  it('uses OAuth when the Desktop user chooses the server page', async () => {
    mocks.desktop = true;
    createTestServerScope({
      serverInfo: { name: 'Saved Server', loading: false, directLoginEnabled: false },
      store: { realtimeSync: { hasDisplayableView: false } }
    });
    render(ServerSignedOut, { props: { registration } });
    await expect.element(page.getByLabelText('Password', { exact: true })).not.toBeInTheDocument();
    await page.getByRole('button', { name: 'Continue in the browser' }).click();
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(registration);
    expect(mocks.passwordLogin).not.toHaveBeenCalled();
  });
  it('names the server from its registration and explains the signed-out state', async () => {
    render(ServerSignedOut, { props: { registration } });

    await expect
      .element(page.getByRole('heading', { name: 'You are signed out of this server' }))
      .toBeVisible();
    await expect.element(page.getByText('Saved Server')).toBeVisible();
    await expect.element(page.getByText('chat.example.test:8443')).toBeVisible();
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
  });

  it('starts sign-in only from the log-in button', async () => {
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    render(ServerSignedOut, { props: { registration } });

    await page.getByRole('button', { name: 'Log in to server' }).click();

    expect(mocks.startRemoteReauthentication).toHaveBeenCalledExactlyOnceWith(registration);
    expect(toastError).not.toHaveBeenCalled();
  });

  it('shows an error when sign-in cannot start', async () => {
    mocks.startRemoteReauthentication.mockRejectedValue(new Error('blocked'));
    render(ServerSignedOut, { props: { registration } });

    await page.getByRole('button', { name: 'Log in to server' }).click();

    await vi.waitFor(() => expect(toastError).toHaveBeenCalledOnce());
    await expect.element(page.getByRole('button', { name: 'Log in to server' })).toBeEnabled();
  });

  it('opens the remove-server confirmation', async () => {
    render(ServerSignedOut, { props: { registration } });

    await page.getByRole('button', { name: 'Remove server' }).click();

    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: { type: 'removeServer', serverId: 'server-1', spaceName: 'Saved Server' }
    });
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
  });
});
