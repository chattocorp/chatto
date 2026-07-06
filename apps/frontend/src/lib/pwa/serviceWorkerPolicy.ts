export const OFFLINE_SHELL_PATH = '/200.html';

const NETWORK_ONLY_PREFIXES = ['/api', '/auth', '/oauth', '/assets', '/webhooks'];

export interface ServiceWorkerPolicy {
  cacheableShellAsset: boolean;
  navigationRequest: boolean;
  networkOnly: boolean;
}

export function normalizeSameOriginUrl(value: string | undefined, origin: string): string | null {
  try {
    const url = new URL(value ?? '/chat', origin);
    return url.origin === origin ? url.href : null;
  } catch {
    return null;
  }
}

export function classifyServiceWorkerRequest(
  request: Pick<Request, 'method' | 'mode' | 'destination'>,
  requestUrl: string,
  shellAssetPaths: ReadonlySet<string>,
  origin: string
): ServiceWorkerPolicy {
  const url = new URL(requestUrl);
  const sameOrigin = url.origin === origin;
  const networkOnly =
    request.method !== 'GET' ||
    !sameOrigin ||
    NETWORK_ONLY_PREFIXES.some(
      (prefix) => url.pathname === prefix || url.pathname.startsWith(`${prefix}/`)
    );

  return {
    cacheableShellAsset: !networkOnly && shellAssetPaths.has(url.pathname),
    navigationRequest: !networkOnly && request.mode === 'navigate',
    networkOnly
  };
}

export function shouldUseOfflineShellFallback(
  policy: Pick<ServiceWorkerPolicy, 'navigationRequest'>,
  networkFailed: boolean
): boolean {
  return policy.navigationRequest && networkFailed;
}
