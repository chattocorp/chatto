import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicServerInfo } from '$lib/api/server';
import { ServerInfoState } from './state.svelte';

function publicServerInfo(overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name: 'Acme',
    version: 'test',
    authMethods: ['password'],
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: false,
    welcomeMessage: 'welcome',
    description: 'a server for acme',
    iconUrl: 'https://icon',
    bannerUrl: 'https://banner',
    authProviders: [],
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
    expect(state.welcomeMessage).toBe('welcome');
    expect(state.description).toBe('a server for acme');
    expect(state.directRegistrationEnabled).toBe(false);
    expect(state.videoProcessingEnabled).toBe(false);
    expect(state.messageEditWindowSeconds).toBe(3 * 60 * 60);
    expect(consoleError).not.toHaveBeenCalled();
  });

  it('loads authenticated runtime settings separately', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockResolvedValue(publicServerInfo());
    const authenticatedLoader = vi.fn().mockResolvedValue({
      motd: 'hello',
      pushNotificationsEnabled: true,
      vapidPublicKey: 'vap',
      livekitUrl: 'wss://lk',
      videoProcessingEnabled: true,
      maxUploadSize: 100,
      maxVideoUploadSize: 200,
      messageEditWindowSeconds: 7200
    });
    const state = new ServerInfoState(
      'https://acme.test',
      loader,
      { baseUrl: 'https://acme.test/api/connect', bearerToken: 'token' },
      authenticatedLoader
    );

    await state.init();
    await state.refreshAuthenticatedSettings();

    expect(authenticatedLoader).toHaveBeenCalledWith({
      baseUrl: 'https://acme.test/api/connect',
      bearerToken: 'token'
    });
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
});
