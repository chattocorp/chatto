import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  handleAssetProxyFetch,
  handleAssetProxyMessage,
  parseAssetProxyRequest
} from './assetProxy.worker';

const ORIGIN = 'https://app.example';
const VIRTUAL_URL = `${ORIGIN}/__chatto/assets/remote/assets/files/asset-1`;
const VIRTUAL_PATH = '/__chatto/assets/remote/assets/files/asset-1';
const ORIGIN_VIRTUAL_URL = `${ORIGIN}/__chatto/assets/origin/assets/files/asset-2`;

class MemoryCache {
  readonly entries = new Map<string, Response>();

  async match(request: Request): Promise<Response | undefined> {
    return this.entries.get(request.url)?.clone();
  }

  async put(request: Request, response: Response): Promise<void> {
    this.entries.set(request.url, response.clone());
  }

  async keys(): Promise<Request[]> {
    return Array.from(this.entries.keys(), (url) => new Request(url));
  }

  async delete(request: Request): Promise<boolean> {
    return this.entries.delete(request.url);
  }
}

function stubCacheStorage() {
  const cachesByName = new Map<string, MemoryCache>();

  vi.stubGlobal('caches', {
    open: vi.fn(async (name: string) => {
      let cache = cachesByName.get(name);
      if (!cache) {
        cache = new MemoryCache();
        cachesByName.set(name, cache);
      }
      return cache;
    }),
    delete: vi.fn(async (name: string) => cachesByName.delete(name)),
    keys: vi.fn(async () => Array.from(cachesByName.keys()))
  });
}

async function postWorkerMessage(data: unknown): Promise<void> {
  const pending: Promise<unknown>[] = [];
  handleAssetProxyMessage({
    data,
    waitUntil: (promise: Promise<unknown>) => {
      pending.push(promise);
    }
  } as ExtendableMessageEvent);
  await Promise.all(pending);
}

async function syncServer(): Promise<void> {
  await postWorkerMessage({
    type: 'chatto-asset-proxy-sync-servers',
    servers: [
      {
        id: 'remote',
        url: 'https://remote.example',
        token: 'must-not-enter-worker-state'
      }
    ]
  });
}

async function registerTarget(targetUrl: string): Promise<void> {
  await postWorkerMessage({
    type: 'chatto-asset-proxy-register-url',
    serverId: 'remote',
    virtualPath: VIRTUAL_PATH,
    targetUrl
  });
}

async function fetchVirtualAsset(): Promise<Response> {
  const proxyRequest = parseAssetProxyRequest(VIRTUAL_URL, ORIGIN);
  expect(proxyRequest).not.toBeNull();
  return handleAssetProxyFetch(new Request(VIRTUAL_URL), proxyRequest!);
}

describe('service worker asset proxy fetch', () => {
  beforeEach(() => {
    stubCacheStorage();
    vi.stubGlobal('self', {
      location: { origin: ORIGIN },
      clients: {
        matchAll: vi.fn(async () => [])
      }
    });
  });

  afterEach(async () => {
    await postWorkerMessage({ type: 'chatto-asset-proxy-clear-cache' });
    vi.unstubAllGlobals();
  });

  it('proxies registered asset targets without attaching bearer Authorization', async () => {
    await syncServer();
    await registerTarget('https://remote.example/assets/files/asset-1?access=ticket-a');

    const fetchMock = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      expect(headers.get('Authorization')).toBeNull();
      expect(headers.get('X-Chatto-Asset-Proxy')).toBe('1');
      expect(init?.credentials).toBe('omit');
      return new Response('asset bytes', {
        status: 200,
        headers: { 'Cache-Control': 'private, max-age=3600' }
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchVirtualAsset().then((response) => response.text())).resolves.toBe(
      'asset bytes'
    );
    await expect(fetchVirtualAsset().then((response) => response.text())).resolves.toBe(
      'asset bytes'
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('scopes cache entries to the resolved asset target', async () => {
    await syncServer();
    await registerTarget('https://remote.example/assets/files/asset-1?access=ticket-a');

    const fetchMock = vi.fn(async (url: string | URL | Request) => {
      return new Response(`body:${String(url)}`, {
        status: 200,
        headers: { 'Cache-Control': 'private, max-age=3600' }
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchVirtualAsset().then((response) => response.text())).resolves.toContain(
      'ticket-a'
    );

    await registerTarget('https://remote.example/assets/files/asset-1?access=ticket-b');

    await expect(fetchVirtualAsset().then((response) => response.text())).resolves.toContain(
      'ticket-b'
    );
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('does not cache responses that opt out of caching', async () => {
    await syncServer();
    await registerTarget('https://remote.example/assets/files/asset-1?access=ticket-a');

    const fetchMock = vi.fn(async () => {
      return new Response('uncacheable', {
        status: 200,
        headers: { 'Cache-Control': 'private, no-store' }
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    await fetchVirtualAsset();
    await fetchVirtualAsset();

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('resyncs server and target mappings after worker state is restarted', async () => {
    const client = {
      postMessage: vi.fn((message: unknown, ports: Transferable[]) => {
        expect(message).toEqual({
          type: 'chatto-asset-proxy-resync-request',
          serverId: 'remote',
          virtualPath: VIRTUAL_PATH
        });
        const [port] = ports as MessagePort[];
        port.postMessage({
          type: 'chatto-asset-proxy-resync-response',
          servers: [
            {
              id: 'remote',
              url: 'https://remote.example',
              token: 'must-not-enter-worker-state'
            }
          ],
          targets: [
            {
              serverId: 'remote',
              virtualPath: VIRTUAL_PATH,
              targetUrl: 'https://remote.example/assets/files/asset-1?access=resynced-ticket'
            }
          ]
        });
      })
    };
    vi.stubGlobal('self', {
      location: { origin: ORIGIN },
      clients: {
        matchAll: vi.fn(async () => [client])
      }
    });

    const fetchMock = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      expect(headers.get('Authorization')).toBeNull();
      expect(headers.get('X-Chatto-Asset-Proxy')).toBe('1');
      return new Response('resynced asset', {
        status: 200,
        headers: { 'Cache-Control': 'private, max-age=3600' }
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchVirtualAsset().then((response) => response.text())).resolves.toBe(
      'resynced asset'
    );

    expect(client.postMessage).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      'https://remote.example/assets/files/asset-1?access=resynced-ticket',
      expect.objectContaining({ credentials: 'omit' })
    );
  });

  it('falls back to same-origin cookie asset fetches without Authorization', async () => {
    await postWorkerMessage({
      type: 'chatto-asset-proxy-sync-servers',
      servers: [
        {
          id: 'origin',
          url: ORIGIN,
          token: 'must-not-enter-worker-state'
        }
      ]
    });

    const fetchMock = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      expect(headers.get('Authorization')).toBeNull();
      expect(headers.get('X-Chatto-Asset-Proxy')).toBe('1');
      expect(init?.credentials).toBe('include');
      return new Response('origin asset', {
        status: 200,
        headers: { 'Cache-Control': 'private, max-age=3600' }
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const proxyRequest = parseAssetProxyRequest(ORIGIN_VIRTUAL_URL, ORIGIN);
    expect(proxyRequest).not.toBeNull();

    await expect(
      handleAssetProxyFetch(new Request(ORIGIN_VIRTUAL_URL), proxyRequest!).then((response) =>
        response.text()
      )
    ).resolves.toBe('origin asset');

    expect(fetchMock).toHaveBeenCalledWith(
      `${ORIGIN}/assets/files/asset-2`,
      expect.objectContaining({ credentials: 'include' })
    );
  });
});
