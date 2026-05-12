import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Client } from '@urql/svelte';
import { InstanceState } from './state.svelte';

/** Build a minimal urql Client mock with controllable query result. */
function makeClient(result: {
  data?: unknown;
  error?: { message: string; networkError?: Error } | null;
}): Client {
  return {
    query: vi.fn().mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: result.data ?? null,
        error: result.error ?? null
      })
    }),
    mutation: vi.fn(),
    subscription: vi.fn()
  } as unknown as Client;
}

/** Build a urql Client mock whose query rejects (synchronous throw inside the chain). */
function makeRejectingClient(err: Error): Client {
  return {
    query: vi.fn().mockReturnValue({
      toPromise: vi.fn().mockRejectedValue(err)
    }),
    mutation: vi.fn(),
    subscription: vi.fn()
  } as unknown as Client;
}

describe('InstanceState.init()', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('populates fields and clears loading on success', async () => {
    const client = makeClient({
      data: {
        server: {
          directRegistrationEnabled: false,
          pushNotificationsEnabled: true,
          vapidPublicKey: 'vap',
          livekitUrl: 'wss://lk',
          maxUploadSize: 100,
          maxVideoUploadSize: 200,
          messageEditWindowSeconds: 7200,
          config: {
            serverName: 'Acme',
            motd: 'hello',
            welcomeMessage: 'welcome',
            description: 'a server for acme',
            logoUrl: 'https://icon',
            bannerUrl: 'https://banner'
          }
        }
      }
    });
    const state = new InstanceState(client, 'https://acme.test');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBeNull();
    expect(state.name).toBe('Acme');
    expect(state.messageEditWindowSeconds).toBe(7200);
    expect(consoleError).not.toHaveBeenCalled();
  });

  it('logs and sets error when urql returns a network error (CORS/unreachable)', async () => {
    const client = makeClient({
      error: {
        message: '[Network] Failed to fetch',
        networkError: new Error('Failed to fetch')
      }
    });
    const state = new InstanceState(client, 'https://chatto.run');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('[Network] Failed to fetch');
    expect(state.name).toBe('Chatto'); // default unchanged
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('https://chatto.run');
    expect(consoleError.mock.calls[0][0]).toContain('failed to load instance info');
  });

  it('logs and sets error when the query promise rejects', async () => {
    const client = makeRejectingClient(new Error('boom'));
    const state = new InstanceState(client, 'https://chatto.run');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('boom');
    // The .catch path logs at least once with our scoped message
    // (there may be additional unhandled-rejection-style logs depending
    // on the runtime, but our explicit log must be present).
    const ourCalls = consoleError.mock.calls.filter(
      (c: unknown[]) =>
        typeof c[0] === 'string' &&
        c[0].includes('https://chatto.run') &&
        c[0].includes('failed to load instance info')
    );
    expect(ourCalls.length).toBeGreaterThanOrEqual(1);
  });

  it('does not throw — failure must be isolated to this instance', async () => {
    const client = makeRejectingClient(new Error('boom'));
    const state = new InstanceState(client);

    // Must resolve, not reject.
    await expect(state.init()).resolves.toBeUndefined();
  });
});
