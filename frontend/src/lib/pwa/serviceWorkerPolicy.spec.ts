import { describe, expect, it } from 'vitest';
import {
  classifyServiceWorkerRequest,
  extractSameOriginShellAssetPaths,
  normalizeSameOriginUrl,
  shouldUseOfflineShellFallback
} from './serviceWorkerPolicy';

const ORIGIN = 'https://chatto.example';
const SHELL_ASSETS = new Set([
  '/manifest.webmanifest',
  '/icons/icon-192.png',
  '/_app/immutable/app.js'
]);

function request(method: string, mode: RequestMode = 'same-origin') {
  return {
    method,
    mode,
    destination: ''
  } satisfies Pick<Request, 'method' | 'mode' | 'destination'>;
}

function classify(pathOrUrl: string, method = 'GET', mode: RequestMode = 'same-origin') {
  const url = pathOrUrl.startsWith('http') ? pathOrUrl : `${ORIGIN}${pathOrUrl}`;
  return classifyServiceWorkerRequest(request(method, mode), url, SHELL_ASSETS, ORIGIN);
}

describe('classifyServiceWorkerRequest', () => {
  it('marks same-origin app shell assets as cacheable', () => {
    expect(classify('/manifest.webmanifest')).toEqual({
      cacheableShellAsset: true,
      navigationRequest: false,
      networkOnly: false
    });
    expect(classify('/_app/immutable/app.js')).toMatchObject({
      cacheableShellAsset: true,
      networkOnly: false
    });
  });

  it.each(['/api/graphql', '/auth/login', '/assets/avatar.png', '/webhooks/livekit', '/graphql'])(
    'keeps %s network-only',
    (path) => {
      expect(classify(path)).toMatchObject({
        cacheableShellAsset: false,
        navigationRequest: false,
        networkOnly: true
      });
    }
  );

  it('keeps cross-origin and non-GET requests network-only', () => {
    expect(classify('https://other.example/manifest.webmanifest')).toMatchObject({
      cacheableShellAsset: false,
      networkOnly: true
    });
    expect(classify('/manifest.webmanifest', 'POST')).toMatchObject({
      cacheableShellAsset: false,
      networkOnly: true
    });
  });

  it('classifies same-origin navigations for network-first offline-shell fallback', () => {
    const policy = classify('/chat/server/room', 'GET', 'navigate');

    expect(policy).toEqual({
      cacheableShellAsset: false,
      navigationRequest: true,
      networkOnly: false
    });
    expect(shouldUseOfflineShellFallback(policy, true)).toBe(true);
    expect(shouldUseOfflineShellFallback(policy, false)).toBe(false);
  });
});

describe('normalizeSameOriginUrl', () => {
  it('resolves missing and relative notification URLs to the same origin', () => {
    expect(normalizeSameOriginUrl(undefined, ORIGIN)).toBe(`${ORIGIN}/chat`);
    expect(normalizeSameOriginUrl('/chat/s1/r1?highlight=m1', ORIGIN)).toBe(
      `${ORIGIN}/chat/s1/r1?highlight=m1`
    );
  });

  it('rejects cross-origin and malformed notification URLs', () => {
    expect(normalizeSameOriginUrl('https://other.example/chat', ORIGIN)).toBeNull();
    expect(normalizeSameOriginUrl('http://[', ORIGIN)).toBeNull();
  });
});

describe('extractSameOriginShellAssetPaths', () => {
  it('extracts same-origin shell assets referenced by generated HTML', () => {
    const html = `
      <link href="/_app/immutable/entry/start.abc.js" rel="modulepreload">
      <link href="/_app/immutable/assets/app.def.css" rel="stylesheet">
      <script type="module">
        import("/_app/immutable/entry/app.ghi.js");
      </script>
    `;
    const shellAssets = new Set([
      '/_app/immutable/entry/start.abc.js',
      '/_app/immutable/entry/app.ghi.js',
      '/_app/immutable/assets/app.def.css'
    ]);

    expect(extractSameOriginShellAssetPaths(html, shellAssets, ORIGIN)).toEqual([
      '/_app/immutable/entry/start.abc.js',
      '/_app/immutable/assets/app.def.css',
      '/_app/immutable/entry/app.ghi.js'
    ]);
  });

  it('ignores cross-origin and non-shell asset references', () => {
    const html = `
      <link href="https://cdn.example/app.js" rel="modulepreload">
      <script src="/not-in-shell.js"></script>
      <script type="module">
        import("/_app/immutable/entry/app.ghi.js");
      </script>
    `;
    const shellAssets = new Set(['/_app/immutable/entry/app.ghi.js']);

    expect(extractSameOriginShellAssetPaths(html, shellAssets, ORIGIN)).toEqual([
      '/_app/immutable/entry/app.ghi.js'
    ]);
  });
});
