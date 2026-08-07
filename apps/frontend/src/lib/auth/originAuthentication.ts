import { invalidateAll } from '$app/navigation';
import type { AuthenticatedUserSummary } from '$lib/state/server/registry.svelte';
import { hasPendingReturnNavigation, resumeReturnNavigation } from './returnNavigation';

/**
 * Install a newly authenticated origin session and refresh route data.
 *
 * Returns whether a stored authentication return path took ownership of the
 * next navigation. Remote-server authentication is deliberately untouched.
 */
export async function completeOriginAuthentication(
  token: string,
  user: AuthenticatedUserSummary | null
): Promise<boolean> {
  const shouldResumeReturnNavigation = hasPendingReturnNavigation();
  const [{ serverRegistry }, { clearCachedUser }] = await Promise.all([
    import('$lib/state/server/registry.svelte'),
    import('./loadAuth')
  ]);

  serverRegistry.authenticateOrigin(token, user);
  clearCachedUser();
  await invalidateAll();

  if (!shouldResumeReturnNavigation) return false;
  await resumeReturnNavigation();
  return true;
}
