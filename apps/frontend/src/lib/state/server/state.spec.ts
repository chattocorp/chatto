import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicServerInfo } from '$lib/api-client/server';
import { ServerInfoState } from './state.svelte';

function publicServerInfo(overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name: 'Acme',
    version: 'test',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: false,
    welcomeMessage: 'welcome',
    description: 'a server for acme',
    iconUrl: 'https://icon',
    bannerUrl: 'https://banner',
    authProviders: [],
    compatibility: {
      protocolCapabilities: [
        'chatto.api.v1',
        'chatto.realtime.v1',
        'chatto.realtime.projection.v1'
      ],
      minimumWebClientVersion: null
    },
    ...overrides
  };
}

describe('ServerInfoState.init()', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleError.mockRestore();
  });

  it('populates fields and clears loading on success', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockResolvedValue(publicServerInfo());
    const state = new ServerInfoState('https://acme.test', loader);

    await state.init();

    expect(loader).toHaveBeenCalledWith('https://acme.test');
    expect(state.loading).toBe(false);
    expect(state.error).toBeNull();
    expect(state.name).toBe('Acme');
    expect(state.version).toBe('test');
    expect(state.protocolCapabilities).toEqual([
      'chatto.api.v1',
      'chatto.realtime.v1',
      'chatto.realtime.projection.v1'
    ]);
    expect(state.supportsRealtimeProjection).toBe(true);
    expect(state.lastDiscoveredAt).not.toBeNull();
    expect(state.compatibility.status).toBe('supported');
    expect(state.welcomeMessage).toBe('welcome');
    expect(state.description).toBe('a server for acme');
    expect(state.directRegistrationEnabled).toBe(false);
    expect(state.videoProcessingEnabled).toBe(false);
    expect(state.messageEditWindowSeconds).toBe(3 * 60 * 60);
    expect(consoleError).not.toHaveBeenCalled();
  });

  it('coalesces concurrent discovery requests', async () => {
    let resolve!: (info: PublicServerInfo) => void;
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const state = new ServerInfoState('https://acme.test', loader);

    const first = state.init();
    const second = state.init();
    expect(loader).toHaveBeenCalledTimes(1);

    resolve(publicServerInfo());
    await Promise.all([first, second]);

    expect(state.name).toBe('Acme');
    expect(state.loading).toBe(false);
  });

  it('refreshes profile fields without toggling initial loading state', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockResolvedValue(
      publicServerInfo({
        name: 'Fresh',
        directRegistrationEnabled: true,
        welcomeMessage: 'fresh welcome',
        description: 'fresh description',
        iconUrl: 'https://fresh-icon',
        bannerUrl: 'https://fresh-banner'
      })
    );
    const state = new ServerInfoState('https://fresh.test', loader);
    state.loading = false;

    await state.refreshProfile();

    expect(state.loading).toBe(false);
    expect(state.name).toBe('Fresh');
    expect(state.welcomeMessage).toBe('fresh welcome');
    expect(state.description).toBe('fresh description');
    expect(state.iconUrl).toBe('https://fresh-icon');
    expect(state.bannerUrl).toBe('https://fresh-banner');
  });

  it('logs and sets error when Connect server metadata fails', async () => {
    const loader = vi
      .fn<() => Promise<PublicServerInfo>>()
      .mockRejectedValue(new Error('[Network] Failed to fetch'));
    const state = new ServerInfoState('https://chatto.run', loader);

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('[Network] Failed to fetch');
    expect(state.name).toBe('Chatto'); // default unchanged
    expect(state.compatibility.status).toBe('unreachable');
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('https://chatto.run');
    expect(consoleError.mock.calls[0][0]).toContain('failed to load server info');
  });

  it('logs and sets error when the Connect loader rejects', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockRejectedValue(new Error('boom'));
    const state = new ServerInfoState('https://chatto.run', loader);

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('boom');
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('https://chatto.run');
    expect(consoleError.mock.calls[0][0]).toContain('failed to load server info');
  });

  it('does not throw — failure must be isolated to this server', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockRejectedValue(new Error('boom'));
    const state = new ServerInfoState('unknown', loader);

    // Must resolve, not reject.
    await expect(state.init()).resolves.toBeUndefined();
  });

  it('loads public profile fields through ConnectRPC', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockResolvedValue(
      publicServerInfo({
        name: 'Connect Server',
        directRegistrationEnabled: false,
        welcomeMessage: 'hello from connect',
        description: 'protobuf path',
        iconUrl: 'https://cdn/icon.webp',
        bannerUrl: 'https://cdn/banner.webp'
      })
    );
    const state = new ServerInfoState('https://connect.test', loader);

    await state.init();

    expect(loader).toHaveBeenCalledWith('https://connect.test');
    expect(state.error).toBeNull();
    expect(state.name).toBe('Connect Server');
    expect(state.directRegistrationEnabled).toBe(false);
    expect(state.welcomeMessage).toBe('hello from connect');
    expect(state.description).toBe('protobuf path');
    expect(state.iconUrl).toBe('https://cdn/icon.webp');
    expect(state.bannerUrl).toBe('https://cdn/banner.webp');
  });

  it('rejects a legacy pre-0.5 server without the projection stream', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockResolvedValue(
      publicServerInfo({ version: '0.4.12', compatibility: null })
    );
    const state = new ServerInfoState('https://legacy.test', loader);

    await state.init();

    expect(state.protocolCapabilities).toBeNull();
    expect(state.compatibility).toMatchObject({
      status: 'unsupported',
      reason: 'server-too-old'
    });
    expect(state.supportsRealtimeProjection).toBe(false);
  });
});
