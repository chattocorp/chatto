import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  startRemoteReauthentication: vi.fn(),
  pushState: vi.fn()
}));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState }));

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
  // Failed or pending discovery keeps the default name.
  createTestServerScope({ serverInfo: { name: 'Chatto', version: '', iconUrl: null } });
});

describe('ServerSignedOut', () => {
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

    await page.getByRole('button', { name: 'Log in to this server' }).click();

    expect(mocks.startRemoteReauthentication).toHaveBeenCalledExactlyOnceWith(registration);
    expect(toastError).not.toHaveBeenCalled();
  });

  it('shows an error when sign-in cannot start', async () => {
    mocks.startRemoteReauthentication.mockRejectedValue(new Error('blocked'));
    render(ServerSignedOut, { props: { registration } });

    await page.getByRole('button', { name: 'Log in to this server' }).click();

    await vi.waitFor(() => expect(toastError).toHaveBeenCalledOnce());
    await expect.element(page.getByRole('button', { name: 'Log in to this server' })).toBeEnabled();
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
