import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicServerInfo } from '../api/server.js';
import { ServerRuntimeConfig } from '@chatto/api-types/api/v1/server_state_pb';
import type { ProjectedServerState } from './projection.js';
import { ServerInfoState, serverDisplayName } from './state.js';

function publicServerInfo(overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name: 'Acme',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: false,
    directLoginEnabled: false,
    accountCreationPolicy: 'open',
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
    expect(state.version).toBe('0.5.0');
    expect(state.isSupportedVersion).toBe(true);
    expect(state.lastDiscoveredAt).not.toBeNull();
    expect(state.compatibility.status).toBe('supported');
    expect(state.welcomeMessage).toBe('welcome');
    expect(state.description).toBe('a server for acme');
    expect(state.directRegistrationEnabled).toBe(false);
    expect(state.directLoginEnabled).toBe(false);
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
    expect(state.error).toMatchObject({ message: '[Network] Failed to fetch' });
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
    expect(state.error).toMatchObject({ message: 'boom' });
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
    const loader = vi
      .fn<() => Promise<PublicServerInfo>>()
      .mockResolvedValue(publicServerInfo({ version: '0.4.12' }));
    const state = new ServerInfoState('https://legacy.test', loader);

    await state.init();

    expect(state.compatibility).toMatchObject({
      status: 'unsupported',
      reason: 'server-too-old'
    });
    expect(state.isSupportedVersion).toBe(false);
  });

  it('reads runtime settings from the projection and falls back to defaults', () => {
    let projected: ProjectedServerState | null = null;
    const state = new ServerInfoState('https://acme.test', vi.fn(), () => projected);

    expect(state.motd).toBeNull();
    expect(state.livekitUrl).toBeNull();
    expect(state.maxUploadSize).toBe(25 * 1024 * 1024);
    expect(state.messageEditWindowSeconds).toBe(3 * 60 * 60);

    projected = {
      motd: 'Hello',
      runtime: new ServerRuntimeConfig({
        livekitUrl: 'wss://livekit.acme.test',
        videoProcessingEnabled: true,
        maxUploadSize: 1024n,
        maxVideoUploadSize: 2048n,
        messageEditWindowSeconds: 60
      })
    };

    expect(state.motd).toBe('Hello');
    expect(state.livekitUrl).toBe('wss://livekit.acme.test');
    expect(state.videoProcessingEnabled).toBe(true);
    expect(state.maxUploadSize).toBe(1024);
    expect(state.maxVideoUploadSize).toBe(2048);
    expect(state.messageEditWindowSeconds).toBe(60);

    // A projection reset clears the settings again.
    projected = null;

    expect(state.livekitUrl).toBeNull();
    expect(state.maxUploadSize).toBe(25 * 1024 * 1024);
  });
});

describe('ServerInfoState.compatibilityProblem', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('has no problem while the first discovery is in flight', () => {
    const state = new ServerInfoState('https://acme.test', () => new Promise(() => {}));
    void state.init();

    expect(state.compatibility.status).toBe('unknown');
    expect(state.compatibilityProblem).toBeNull();
  });

  it('reports no problem for a supported server', async () => {
    const state = new ServerInfoState('https://acme.test', async () => publicServerInfo());
    await state.init();

    expect(state.compatibilityProblem).toBeNull();
  });

  it('reports a server that is too old', async () => {
    const state = new ServerInfoState('https://acme.test', async () =>
      publicServerInfo({ version: '0.5.0-beta.7' })
    );
    await state.init();

    expect(state.compatibilityProblem).toBe('server-too-old');
  });

  it('keeps the previous problem while a retry is in flight', async () => {
    let finish!: (info: PublicServerInfo) => void;
    const loader = vi
      .fn<() => Promise<PublicServerInfo>>()
      .mockRejectedValueOnce(new Error('offline'))
      .mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)))
      .mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)));
    const state = new ServerInfoState('https://acme.test', loader);
    await state.init();
    expect(state.compatibilityProblem).toBe('unreachable');

    const unreachableRetry = state.init();
    expect(state.loading).toBe(true);
    expect(state.compatibilityProblem).toBe('unreachable');
    finish(publicServerInfo({ version: '0.5.0-beta.7' }));
    await unreachableRetry;
    expect(state.compatibilityProblem).toBe('server-too-old');

    const upgradeRetry = state.init();
    expect(state.compatibilityProblem).toBe('server-too-old');
    finish(publicServerInfo());
    await upgradeRetry;
    expect(state.compatibilityProblem).toBeNull();
  });
});

describe('serverDisplayName()', () => {
  it('prefers a saved name only while discovery keeps the default name', async () => {
    const loader = vi.fn<() => Promise<PublicServerInfo>>().mockResolvedValue(publicServerInfo());
    const state = new ServerInfoState('https://acme.test', loader);

    expect(serverDisplayName(state, 'Saved')).toBe('Saved');
    expect(serverDisplayName(state, null)).toBe('Chatto');

    await state.init();

    expect(serverDisplayName(state, 'Saved')).toBe('Acme');
  });
});
