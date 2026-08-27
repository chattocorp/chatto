import { invalidateAll } from '$app/navigation';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';
import { hasPendingReturnNavigation, resumeReturnNavigation } from './returnNavigation';

/**
 * Complete a new cookie-backed origin session and refresh route data.
 *
 * Returns whether route invalidation or a stored authentication return path
 * already took ownership of navigation. Remote-server authentication is
 * deliberately untouched.
 */
export async function completeOriginAuthentication(): Promise<boolean> {
  const shouldResumeReturnNavigation = hasPendingReturnNavigation();
  const routeBeforeInvalidation =
    typeof window === 'undefined'
      ? null
      : window.location.pathname + window.location.search + window.location.hash;
  const [{ serverRegistry }, { clearCachedUser }] = await Promise.all([
    import('$lib/state/server/registry.svelte'),
    import('./loadAuth')
  ]);

  clearCachedUser();
  await invalidateAll();
  const originServerId = serverRegistry.originServer?.id;
  if (originServerId) resumePushRegistrationAfterAuthentication(originServerId);

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
