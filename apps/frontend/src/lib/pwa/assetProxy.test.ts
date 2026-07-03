import { afterEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '$lib/state/server/registry.svelte';
import {
  clearAssetProxyCache,
  installAssetProxyResyncHandler,
  registerAssetProxyUrl
} from './assetProxy';

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
    reauthRequiredAt: null,
    addedAt: 1,
    ...overrides
  };
}

function stubServiceWorker() {
  const listeners = new Set<(event: MessageEvent) => void>();
  const controller = { postMessage: vi.fn() };

  vi.stubGlobal('navigator', {
    serviceWorker: {
      controller,
      addEventListener: vi.fn((type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') listeners.add(listener);
      }),
      removeEventListener: vi.fn((type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') listeners.delete(listener);
      })
    }
  });

  return {
    controller,
    dispatchMessage(data: unknown, port: { postMessage: ReturnType<typeof vi.fn> }) {
      for (const listener of listeners) {
        listener({ data, ports: [port] } as unknown as MessageEvent);
      }
    },
    listenerCount() {
      return listeners.size;
    }
  };
}

describe('service worker asset proxy resync', () => {
  afterEach(() => {
    clearAssetProxyCache();
    vi.unstubAllGlobals();
  });

  it('responds to service worker resync requests with servers and the requested target', () => {
    const serviceWorker = stubServiceWorker();
    const stop = installAssetProxyResyncHandler(() => [server()]);
    const virtualPath = '/__chatto/assets/remote/assets/files/att_1';
    const targetUrl = 'https://remote.example/assets/files/att_1?access=ticket';

    registerAssetProxyUrl('remote', virtualPath, targetUrl);

    const port = { postMessage: vi.fn() };
    serviceWorker.dispatchMessage(
      {
        type: 'chatto-asset-proxy-resync-request',
        serverId: 'remote',
        virtualPath
      },
      port
    );

    expect(port.postMessage).toHaveBeenCalledWith({
      type: 'chatto-asset-proxy-resync-response',
      servers: [
        {
          id: 'remote',
          url: 'https://remote.example'
        }
      ],
      targets: [
        {
          serverId: 'remote',
          virtualPath,
          targetUrl
        }
      ]
    });

    stop();
    expect(serviceWorker.listenerCount()).toBe(0);
  });

  it('does not resync cleared server targets', () => {
    const serviceWorker = stubServiceWorker();
    installAssetProxyResyncHandler(() => [server()]);
    const virtualPath = '/__chatto/assets/remote/assets/files/att_2';

    registerAssetProxyUrl('remote', virtualPath, 'https://remote.example/assets/files/att_2');
    clearAssetProxyCache('remote');

    const port = { postMessage: vi.fn() };
    serviceWorker.dispatchMessage(
      {
        type: 'chatto-asset-proxy-resync-request',
        serverId: 'remote',
        virtualPath
      },
      port
    );

    expect(port.postMessage).toHaveBeenCalledWith({
      type: 'chatto-asset-proxy-resync-response',
      servers: [
        {
          id: 'remote',
          url: 'https://remote.example'
        }
      ],
      targets: []
    });
  });
});
