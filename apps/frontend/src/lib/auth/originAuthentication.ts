import { invalidateAll } from '$app/navigation';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';
import { hasPendingReturnNavigation, resumeReturnNavigation } from './returnNavigation';

/**
 * Complete a new cookie-backed origin session and refresh route data.
 *
 * Returns whether route invalidation or a stored authentication return path
 * already took ownership of navigation. Remote-server authentication is
 * deliberately untouched.
 * Set resumeReturnNavigation to false when an explicit provider continuation
 * must finish before the opening tab can resume its saved app destination.
 */
export async function completeOriginAuthentication(
  options: { resumeReturnNavigation?: boolean } = {}
): Promise<boolean> {
  // A provider window can inherit the opening tab's sessionStorage. Its explicit
  // continuation owns navigation until it reports completion to that tab.
  const shouldResumeReturnNavigation =
    options.resumeReturnNavigation !== false && hasPendingReturnNavigation();
  const routeBeforeInvalidation =
    typeof window === 'undefined'
      ? null
      : window.location.pathname + window.location.search + window.location.hash;
  const { serverRegistry } = await import('$lib/client');
  const origin = serverRegistry.originServer;
  if (origin) serverRegistry.getStore(origin.id).currentUser.reset();
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
