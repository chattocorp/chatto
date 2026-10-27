import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '@chatto/client/server/registry';

const mocks = vi.hoisted(() => ({
  startRemoteReauthentication: vi.fn(),
  desktop: false,
  goto: vi.fn(),
  active: ''
}));
vi.mock('$app/navigation', () => ({ goto: mocks.goto, pushState: vi.fn(), replaceState: vi.fn() }));
vi.mock('$app/paths', () => ({
  resolve: (_: string, params: { serverId: string }) => `/chat/${params.serverId}`
}));
vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (id: string) => id,
  segmentToServerId: (id: string) => id
}));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => mocks.active }));
vi.mock('$lib/desktop/passwordLogin', () => ({ hasPasswordLogin: () => mocks.desktop }));
vi.mock('$lib/ui/toast', () => ({ toast: { error: vi.fn() } }));

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));

import { toast } from '$lib/ui/toast';
import { OAuthPopupError } from '$lib/oauth/popupError';
import {
  closeInlineSignIn,
  isInlineSignInRequested,
  isRemoteSignInPending,
  startRemoteSignIn
} from './remoteSignIn.svelte';

const toastError = vi.spyOn(toast, 'error');
const server = { id: 'remote', url: 'https://remote.example' } as RegisteredServer;
const other = { id: 'other', url: 'https://other.example' } as RegisteredServer;

describe('startRemoteSignIn', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.desktop = false;
    mocks.active = '';
    closeInlineSignIn(server.id);
  });

  it('opens the Desktop form without OAuth and retains the active deep link', async () => {
    mocks.desktop = true;
    await startRemoteSignIn(server);
    expect(isInlineSignInRequested(server.id)).toBe(true);
    expect(mocks.goto).toHaveBeenCalledWith('/chat/remote');
    expect(mocks.startRemoteReauthentication).not.toHaveBeenCalled();
    mocks.goto.mockClear();
    mocks.active = server.id;
    await startRemoteSignIn(server);
    expect(mocks.goto).not.toHaveBeenCalled();
    closeInlineSignIn(server.id);
    expect(isInlineSignInRequested(server.id)).toBe(false);
  });

  it('runs one sign-in per server at a time', async () => {
    let finish: () => void = () => {};
    mocks.startRemoteReauthentication.mockReturnValueOnce(
      new Promise<void>((resolve) => (finish = resolve))
    );

    const first = startRemoteSignIn(server);
    // The window must open from the user's action, before any await.
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledOnce();
    expect(isRemoteSignInPending('remote')).toBe(true);
    expect(isRemoteSignInPending('other')).toBe(false);
    await startRemoteSignIn(server);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledOnce();
    mocks.startRemoteReauthentication.mockResolvedValueOnce(undefined);
    await startRemoteSignIn(other);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledTimes(2);

    finish();
    await first;
    expect(isRemoteSignInPending('remote')).toBe(false);
    expect(toastError).not.toHaveBeenCalled();
  });

  it('starts provider OAuth immediately instead of reopening the Desktop form', async () => {
    mocks.desktop = true;
    mocks.startRemoteReauthentication.mockResolvedValue(undefined);
    const pending = startRemoteSignIn(server, { providerId: 'chatto-id' });
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledWith(server, 'chatto-id');
    expect(mocks.goto).not.toHaveBeenCalled();
    await pending;
    expect(isRemoteSignInPending(server.id)).toBe(false);
  });

  it('shows an error and permits another attempt after a failure', async () => {
    mocks.startRemoteReauthentication.mockRejectedValueOnce(new Error('blocked'));

    await startRemoteSignIn(server);

    expect(toastError).toHaveBeenCalledOnce();
    expect(isRemoteSignInPending('remote')).toBe(false);
    mocks.startRemoteReauthentication.mockResolvedValueOnce(undefined);
    await startRemoteSignIn(server);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledTimes(2);
  });

  it('quietly cancels a closed window and permits another attempt', async () => {
    mocks.startRemoteReauthentication.mockRejectedValueOnce(
      new OAuthPopupError('closed', 'cancelled')
    );
    await startRemoteSignIn(server);
    expect(toastError).not.toHaveBeenCalled();
    expect(isRemoteSignInPending(server.id)).toBe(false);
    mocks.startRemoteReauthentication.mockResolvedValueOnce(undefined);
    await startRemoteSignIn(server);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledTimes(2);
  });

  it('explains how to retry a blocked window', async () => {
    mocks.startRemoteReauthentication.mockRejectedValueOnce(
      new OAuthPopupError('blocked', 'blocked')
    );
    await startRemoteSignIn(server);
    expect(toastError).toHaveBeenCalledWith(
      'The sign-in window was blocked. Allow popups for this site and try again.'
    );
    expect(isRemoteSignInPending(server.id)).toBe(false);
  });
});
