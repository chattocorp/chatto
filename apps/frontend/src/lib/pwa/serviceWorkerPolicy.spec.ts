import { describe, expect, it } from 'vitest';
import {
  classifyServiceWorkerRequest,
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

  it.each([
    '/api/connect',
    '/auth/login',
    '/oauth/authorize',
    '/assets/avatar.png',
    '/webhooks/livekit'
  ])(
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
