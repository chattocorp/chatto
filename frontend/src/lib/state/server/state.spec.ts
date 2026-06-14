import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Client } from '@urql/svelte';
import { ServerInfoState } from './state.svelte';

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

function makeClientSequence(results: Array<{
  data?: unknown;
  error?: { message: string; networkError?: Error } | null;
}>): Client {
  return {
    query: vi.fn().mockImplementation(() => {
      const result = results.shift() ?? {};
      return {
        toPromise: vi.fn().mockResolvedValue({
          data: result.data ?? null,
          error: result.error ?? null
        })
      };
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

describe('ServerInfoState.init()', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('populates fields and clears loading on success', async () => {
    const client = makeClient({
      data: {
        server: {
          directRegistrationEnabled: false,
          profile: {
            name: 'Acme',
            welcomeMessage: 'welcome',
            description: 'a server for acme',
            logoUrl: 'https://icon',
            bannerUrl: 'https://banner'
          }
        }
      }
    });
    const state = new ServerInfoState(client, 'https://acme.test');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBeNull();
    expect(state.name).toBe('Acme');
    expect(state.welcomeMessage).toBe('welcome');
    expect(state.description).toBe('a server for acme');
    expect(state.directRegistrationEnabled).toBe(false);
    expect(state.videoProcessingEnabled).toBe(false);
    expect(state.messageEditWindowSeconds).toBe(3 * 60 * 60);
    expect(consoleError).not.toHaveBeenCalled();
  });

  it('loads authenticated runtime settings separately', async () => {
    const client = makeClientSequence([
      {
        data: {
          server: {
            directRegistrationEnabled: false,
            profile: {
              name: 'Acme',
              welcomeMessage: 'welcome',
              description: 'a server for acme',
              logoUrl: 'https://icon',
              bannerUrl: 'https://banner'
            }
          }
        }
      },
      {
        data: {
          server: {
            pushNotificationsEnabled: true,
            vapidPublicKey: 'vap',
            livekitUrl: 'wss://lk',
            videoProcessingEnabled: true,
            maxUploadSize: 100,
            maxVideoUploadSize: 200,
            messageEditWindowSeconds: 7200,
            profile: {
              motd: 'hello'
            }
          }
        }
      }
    ]);
    const state = new ServerInfoState(client, 'https://acme.test');

    await state.init();
    await state.refreshAuthenticatedSettings();

    expect(state.motd).toBe('hello');
    expect(state.pushNotificationsEnabled).toBe(true);
    expect(state.vapidPublicKey).toBe('vap');
    expect(state.livekitUrl).toBe('wss://lk');
    expect(state.videoProcessingEnabled).toBe(true);
    expect(state.maxUploadSize).toBe(100);
    expect(state.maxVideoUploadSize).toBe(200);
    expect(state.messageEditWindowSeconds).toBe(7200);
  });

  it('refreshes profile fields without toggling initial loading state', async () => {
    const client = makeClient({
      data: {
        server: {
          directRegistrationEnabled: true,
          profile: {
            name: 'Fresh',
            welcomeMessage: 'fresh welcome',
            description: 'fresh description',
            logoUrl: 'https://fresh-icon',
            bannerUrl: 'https://fresh-banner'
          }
        }
      }
    });
    const state = new ServerInfoState(client, 'https://fresh.test');
    state.loading = false;

    await state.refreshProfile();

    expect(state.loading).toBe(false);
    expect(state.name).toBe('Fresh');
    expect(state.welcomeMessage).toBe('fresh welcome');
    expect(state.description).toBe('fresh description');
    expect(state.iconUrl).toBe('https://fresh-icon');
    expect(state.bannerUrl).toBe('https://fresh-banner');
  });

  it('logs and sets error when urql returns a network error (CORS/unreachable)', async () => {
    const client = makeClient({
      error: {
        message: '[Network] Failed to fetch',
        networkError: new Error('Failed to fetch')
      }
    });
    const state = new ServerInfoState(client, 'https://chatto.run');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('[Network] Failed to fetch');
    expect(state.name).toBe('Chatto'); // default unchanged
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('https://chatto.run');
    expect(consoleError.mock.calls[0][0]).toContain('failed to load server info');
  });

  it('logs and sets error when the query promise rejects', async () => {
    const client = makeRejectingClient(new Error('boom'));
    const state = new ServerInfoState(client, 'https://chatto.run');

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
        c[0].includes('failed to load server info')
    );
    expect(ourCalls.length).toBeGreaterThanOrEqual(1);
  });

  it('does not throw — failure must be isolated to this server', async () => {
    const client = makeRejectingClient(new Error('boom'));
    const state = new ServerInfoState(client);

    // Must resolve, not reject.
    await expect(state.init()).resolves.toBeUndefined();
  });
});
