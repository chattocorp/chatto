/** Browser routes that must finish provider authorization before app navigation. */
import { resolve } from '$app/paths';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';

/** Recognize the continuation that links a provider to an existing account. */
export function isProviderContinuation(path: string): boolean {
  if (!isSafeInternalPath(path)) return false;
  const url = new URL(path, 'https://chatto.invalid');
  return (
    url.pathname === resolve('/sso/confirm') &&
    url.searchParams.get('existing') === '1' &&
    !!url.searchParams.get('token')
  );
}
