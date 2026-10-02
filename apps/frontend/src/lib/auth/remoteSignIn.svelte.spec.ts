import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '@chatto/client/server/registry';

const mocks = vi.hoisted(() => ({ startRemoteReauthentication: vi.fn() }));

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: mocks.startRemoteReauthentication
}));

import { toast } from '$lib/ui/toast';
import { RemoteSignIn } from './remoteSignIn.svelte';

const toastError = vi.spyOn(toast, 'error');
const server = { id: 'remote', url: 'https://remote.example' } as RegisteredServer;

describe('RemoteSignIn', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('runs one sign-in at a time and reports the pending server', async () => {
    let finish: () => void = () => {};
    mocks.startRemoteReauthentication.mockReturnValueOnce(
      new Promise<void>((resolve) => (finish = resolve))
    );
    const signIn = new RemoteSignIn();

    const first = signIn.start(server);
    // The window must open from the user's action, before any await.
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledOnce();
    expect(signIn.pendingServerId).toBe('remote');
    await signIn.start(server);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledOnce();

    finish();
    await first;
    expect(signIn.pending).toBe(false);
    expect(toastError).not.toHaveBeenCalled();
  });

  it('shows an error and permits another attempt after a failure', async () => {
    mocks.startRemoteReauthentication.mockRejectedValueOnce(new Error('blocked'));
    const signIn = new RemoteSignIn();

    await signIn.start(server);

    expect(toastError).toHaveBeenCalledOnce();
    expect(signIn.pending).toBe(false);
    mocks.startRemoteReauthentication.mockResolvedValueOnce(undefined);
    await signIn.start(server);
    expect(mocks.startRemoteReauthentication).toHaveBeenCalledTimes(2);
  });
});
