import { describe, it, expect, vi, beforeEach } from 'vitest';
import { AuthenticatedServerSettingsView } from '$lib/pb/chatto/api/v1/chat_pb';
import { ServerInfoState } from './state.svelte';

function okServerInfo(data: unknown): Response {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { 'Content-Type': 'application/json' }
  });
}

function makeWireSettings(settings: Partial<AuthenticatedServerSettingsView>) {
  return {
    getAuthenticatedServerSettings: vi.fn().mockResolvedValue({
      settings: new AuthenticatedServerSettingsView(settings)
    })
  };
}

describe('ServerInfoState.init()', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.stubGlobal('fetch', vi.fn());
  });

  it('populates fields and clears loading on success', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      okServerInfo({
        name: 'Acme',
        registrationOpen: false,
        welcomeMessage: 'welcome',
        description: 'a server for acme',
        iconUrl: 'https://icon',
        bannerUrl: 'https://banner'
      })
    );
    const state = new ServerInfoState('https://acme.test');

    await state.init();

    expect(fetch).toHaveBeenCalledWith('https://acme.test/api/server');
    expect(state.loading).toBe(false);
    expect(state.error).toBeNull();
    expect(state.name).toBe('Acme');
    expect(state.welcomeMessage).toBe('welcome');
    expect(state.description).toBe('a server for acme');
    expect(state.iconUrl).toBe('https://icon');
    expect(state.bannerUrl).toBe('https://banner');
    expect(state.directRegistrationEnabled).toBe(false);
    expect(state.videoProcessingEnabled).toBe(false);
    expect(state.messageEditWindowSeconds).toBe(3 * 60 * 60);
    expect(consoleError).not.toHaveBeenCalled();
  });

  it('loads authenticated runtime settings separately', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      okServerInfo({
        name: 'Acme',
        registrationOpen: false
      })
    );
    const wire = makeWireSettings({
      pushNotificationsEnabled: true,
      vapidPublicKey: 'vap',
      livekitUrl: 'wss://lk',
      videoProcessingEnabled: true,
      maxUploadSize: BigInt(100),
      maxVideoUploadSize: BigInt(200),
      messageEditWindowSeconds: 7200,
      motd: 'hello'
    });
    const state = new ServerInfoState('https://acme.test', {}, () => wire);

    await state.init();
    await state.refreshAuthenticatedSettings();

    expect(wire.getAuthenticatedServerSettings).toHaveBeenCalledOnce();
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
    vi.mocked(fetch).mockResolvedValueOnce(
      okServerInfo({
        name: 'Fresh',
        registrationOpen: true,
        welcomeMessage: 'fresh welcome',
        description: 'fresh description',
        iconUrl: 'https://fresh-icon',
        bannerUrl: 'https://fresh-banner'
      })
    );
    const state = new ServerInfoState('https://fresh.test');
    state.loading = false;

    await state.refreshProfile();

    expect(state.loading).toBe(false);
    expect(state.name).toBe('Fresh');
    expect(state.welcomeMessage).toBe('fresh welcome');
    expect(state.description).toBe('fresh description');
    expect(state.iconUrl).toBe('https://fresh-icon');
    expect(state.bannerUrl).toBe('https://fresh-banner');
  });

  it('logs and sets error when /api/server returns a non-OK response', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('{}', { status: 500 }));
    const state = new ServerInfoState('https://chatto.run');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('GET /api/server failed with 500');
    expect(state.name).toBe('Chatto');
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('https://chatto.run');
    expect(consoleError.mock.calls[0][0]).toContain('failed to load server info');
  });

  it('logs and sets error when the fetch promise rejects', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('boom'));
    const state = new ServerInfoState('https://chatto.run');

    await state.init();

    expect(state.loading).toBe(false);
    expect(state.error).toBe('boom');
    const ourCalls = consoleError.mock.calls.filter(
      (c: unknown[]) =>
        typeof c[0] === 'string' &&
        c[0].includes('https://chatto.run') &&
        c[0].includes('failed to load server info')
    );
    expect(ourCalls.length).toBeGreaterThanOrEqual(1);
  });

  it('does not throw — failure must be isolated to this server', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('boom'));
    const state = new ServerInfoState();

    await expect(state.init()).resolves.toBeUndefined();
  });
});
