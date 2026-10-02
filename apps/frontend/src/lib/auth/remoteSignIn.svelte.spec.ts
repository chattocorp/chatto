import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '@chatto/client/server/registry';

const mocks = vi.hoisted(() => ({ startRemoteReauthentication: vi.fn() }));

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));

import { toast } from '$lib/ui/toast';
import { isRemoteSignInPending, startRemoteSignIn } from './remoteSignIn.svelte';

const toastError = vi.spyOn(toast, 'error');
const server = { id: 'remote', url: 'https://remote.example' } as RegisteredServer;
const other = { id: 'other', url: 'https://other.example' } as RegisteredServer;

describe('startRemoteSignIn', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

  it('shows an error and permits another attempt after a failure', async () => {
    mocks.startRemoteReauthentication.mockRejectedValueOnce(new Error('blocked'));

    await startRemoteSignIn(server);

    expect(toastError).toHaveBeenCalledOnce();
    expect(isRemoteSignInPending('remote')).toBe(false);
    mocks.startRemoteReauthentication.mockResolvedValueOnce(undefined);
    await startRemoteSignIn(server);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledTimes(2);
  });
});
