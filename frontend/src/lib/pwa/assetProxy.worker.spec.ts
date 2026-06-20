import { afterEach, describe, expect, it, vi } from 'vitest';

class MemoryCache {
  private readonly entries = new Map<string, Response>();

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

async function loadWorker(cache: MemoryCache) {
  vi.resetModules();
  vi.stubGlobal('caches', {
    open: vi.fn().mockResolvedValue(cache),
    delete: vi.fn().mockResolvedValue(true)
  });
  vi.stubGlobal('fetch', vi.fn());
  vi.stubGlobal('self', {
    clients: {
      matchAll: vi.fn().mockResolvedValue([])
    }
  });

  return import('./assetProxy.worker');
}

function message(data: unknown) {
  return {
    data,
    waitUntil: vi.fn()
  } as unknown as ExtendableMessageEvent;
}

describe('service worker asset proxy fetch', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it('does not serve cached virtual assets without a current server registration', async () => {
    const cache = new MemoryCache();
    const worker = await loadWorker(cache);
    const requestUrl = 'https://app.example/__chatto/assets/remote/assets/files/att_1';
    await cache.put(new Request(requestUrl), new Response('cached asset'));

    const proxyRequest = worker.parseAssetProxyRequest(requestUrl, 'https://app.example');
    expect(proxyRequest).not.toBeNull();

    const response = await worker.handleAssetProxyFetch(new Request(requestUrl), proxyRequest!);

    expect(response.status).toBe(404);
    await expect(response.text()).resolves.toBe('Asset target is not registered');
    expect(fetch).not.toHaveBeenCalled();
  });

  it('serves cached virtual assets after the matching server and target are registered', async () => {
    const cache = new MemoryCache();
    const worker = await loadWorker(cache);
    const requestUrl = 'https://app.example/__chatto/assets/remote/assets/files/att_1';
    await cache.put(new Request(requestUrl), new Response('cached asset'));

    worker.handleAssetProxyMessage(
      message({
        type: 'chatto-asset-proxy-sync-servers',
        servers: [{ id: 'remote', url: 'https://remote.example', token: 'token' }]
      })
    );
    worker.handleAssetProxyMessage(
      message({
        type: 'chatto-asset-proxy-register-url',
        serverId: 'remote',
        virtualPath: '/__chatto/assets/remote/assets/files/att_1',
        targetUrl: 'https://remote.example/assets/files/att_1'
      })
    );

    const proxyRequest = worker.parseAssetProxyRequest(requestUrl, 'https://app.example');
    expect(proxyRequest).not.toBeNull();

    const response = await worker.handleAssetProxyFetch(new Request(requestUrl), proxyRequest!);

    expect(response.status).toBe(200);
    await expect(response.text()).resolves.toBe('cached asset');
    expect(fetch).not.toHaveBeenCalled();
  });
});
