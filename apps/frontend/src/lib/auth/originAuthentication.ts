import { invalidateAll } from '$app/navigation';
import type { AuthenticatedUserSummary } from '$lib/state/server/registry.svelte';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';
import { hasPendingReturnNavigation, resumeReturnNavigation } from './returnNavigation';
import type { NewBearerSession } from './bearerSession';

/**
 * Install a newly authenticated origin session and refresh route data.
 *
 * Returns whether route invalidation or a stored authentication return path
 * already took ownership of navigation. Remote-server authentication is
 * deliberately untouched.
 */
export async function completeOriginAuthentication(
  credentials: string | NewBearerSession,
  user: AuthenticatedUserSummary | null
): Promise<boolean> {
  const shouldResumeReturnNavigation = hasPendingReturnNavigation();
  const routeBeforeInvalidation =
    typeof window === 'undefined'
      ? null
      : window.location.pathname + window.location.search + window.location.hash;
  const [{ serverRegistry }, { clearCachedUser }] = await Promise.all([
    import('$lib/state/server/registry.svelte'),
    import('./loadAuth')
  ]);

  const originServerId = serverRegistry.originServer?.id;
  serverRegistry.authenticateOrigin(credentials, user);
  if (originServerId) resumePushRegistrationAfterAuthentication(originServerId);
  clearCachedUser();
  await invalidateAll();

  if (shouldResumeReturnNavigation) {
    await resumeReturnNavigation();
    return true;
  }

  // Auth routes redirect during invalidation once their parent data contains a
  // viewer. Do not let the submitting component's fallback navigation race a
  // redirect that has already moved into the authenticated application.
  return (
    routeBeforeInvalidation !== null &&
    window.location.pathname + window.location.search + window.location.hash !==
      routeBeforeInvalidation
  );
}
