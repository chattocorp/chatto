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
  createTestServerScope({
    serverInfo: { name: 'Chatto', version: '', iconUrl: null },
    store: { realtimeSync: { hasDisplayableView: false } }
  });
});

describe('ServerSignedOut', () => {
  it.each([
    [true, true],
    [true, false],
    [false, true],
    [false, false]
  ])(
    'offers separate signup only with desktop=%s and registration enabled=%s',
    async (desktop, enabled) => {
      mocks.desktop = desktop;
      mocks.startRemoteReauthentication.mockResolvedValue(undefined);
      createTestServerScope({
        serverInfo: { name: 'Saved Server', loading: false, directRegistrationEnabled: enabled },
        store: { realtimeSync: { hasDisplayableView: false } }
      });
      render(ServerSignedOut, { props: { registration } });
      const signup = page.getByRole('button', { name: 'Create Account', exact: true });
      if (!desktop || !enabled) {
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
  it('names the server and explains the separate server-hosted sign-in window', async () => {
    createTestServerScope({
      serverInfo: {
        name: 'Saved Server',
        loading: false,
        directLoginEnabled: true,
        authProviders: [{ id: 'chatto-id', type: 'oidc', label: 'Chatto ID' }]
      },
      store: { realtimeSync: { hasDisplayableView: false } }
    });
    render(ServerSignedOut, { props: { registration } });

    await expect.element(page.getByText('Saved Server')).toBeVisible();
    await expect
      .element(
        page.getByText(
          'A new window will open at chat.example.test:8443, where you can sign in or create an account.'
        )
      )
      .toBeVisible();
    await expect.element(page.getByLabelText('Password', { exact: true })).not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Continue with Chatto ID' }))
      .not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Remove server' }))
      .not.toBeInTheDocument();
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
  });

  it('starts sign-in only from the continue button', async () => {
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    render(ServerSignedOut, { props: { registration } });

    await page.getByRole('button', { name: 'Continue on server' }).click();

    expect(mocks.startRemoteReauthentication).toHaveBeenCalledExactlyOnceWith(registration);
    expect(toastError).not.toHaveBeenCalled();
  });

  it('shows where to continue while a window is open and clears the notice afterward', async () => {
    let finish!: () => void;
    mocks.startRemoteReauthentication.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        finish = resolve;
      })
    );
    render(ServerSignedOut, { props: { registration } });
    await page.getByRole('button', { name: 'Continue on server' }).click();
    await expect
      .element(page.getByRole('status'))
      .toHaveTextContent('Continue in the other window.');
    await expect.element(page.getByRole('button', { name: 'Continue on server' })).toBeDisabled();
    finish();
    await expect.element(page.getByRole('status')).not.toBeInTheDocument();
    await expect.element(page.getByRole('button', { name: 'Continue on server' })).toBeEnabled();
  });

  it('shows an error when sign-in cannot start', async () => {
    mocks.startRemoteReauthentication.mockRejectedValue(new Error('blocked'));
    render(ServerSignedOut, { props: { registration } });

    await page.getByRole('button', { name: 'Continue on server' }).click();

    await vi.waitFor(() => expect(toastError).toHaveBeenCalledOnce());
    await expect.element(page.getByRole('button', { name: 'Continue on server' })).toBeEnabled();
  });
});
