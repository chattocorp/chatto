import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '$lib/state/server/registry.svelte';

const { servers } = vi.hoisted(() => ({
  servers: new Map<string, RegisteredServer>()
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getServer: (id: string) => servers.get(id)
  }
}));

import { assetUrlForServer } from './assetUrls';

const ORIGIN = 'https://app.example';

function server(overrides: Partial<RegisteredServer> = {}): RegisteredServer {
  return {
    id: 'remote',
    url: 'https://remote.example',
    name: 'Remote',
    iconUrl: null,
    token: 'token',
    userId: 'user',
    userLogin: 'alice',
    userDisplayName: 'Alice',
    userAvatarUrl: null,
    addedAt: 1,
    ...overrides
  };
}

function stubBrowser(postMessage?: (message: unknown) => void) {
  vi.stubGlobal('window', { location: { origin: ORIGIN } });
  vi.stubGlobal('navigator', {
    serviceWorker: {
      controller: postMessage ? { postMessage } : null
    }
  });
}

describe('assetUrlForServer', () => {
  beforeEach(() => {
    servers.clear();
    servers.set('remote', server());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps remote asset URLs direct when the service worker is not controlling the page', () => {
    stubBrowser();

    expect(assetUrlForServer('remote', '/assets/files/att_1?access=ticket')).toBe(
      'https://remote.example/assets/files/att_1?access=ticket'
    );
  });

  it('routes stable asset URLs through the same-origin service worker proxy', () => {
    const postMessage = vi.fn();
    stubBrowser(postMessage);

    expect(assetUrlForServer('remote', '/assets/files/att_1?access=ticket')).toBe(
      '/__chatto/assets/remote/assets/files/att_1'
    );
    expect(postMessage).toHaveBeenCalledWith({
      type: 'chatto-asset-proxy-register-url',
      serverId: 'remote',
      virtualPath: '/__chatto/assets/remote/assets/files/att_1',
      targetUrl: 'https://remote.example/assets/files/att_1?access=ticket'
    });
  });

  it('routes transformed stable asset URLs through a credential-free virtual URL', () => {
    const postMessage = vi.fn();
    stubBrowser(postMessage);

    expect(
      assetUrlForServer('remote', '/assets/files/att_1/image/960x800/contain?access=ticket')
    ).toBe('/__chatto/assets/remote/assets/files/att_1/image/960x800/contain');
  });

  it('leaves non-stable asset URLs unchanged', () => {
    const postMessage = vi.fn();
    stubBrowser(postMessage);

    expect(assetUrlForServer('remote', '/assets/attachments/locator.sig')).toBe(
      '/assets/attachments/locator.sig'
    );
    expect(postMessage).not.toHaveBeenCalled();
  });
});
