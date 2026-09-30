const desktopPermissions = new Set(['media', 'notifications']);

/** Whether a URL belongs to the privileged Chatto Desktop renderer origin. */
export function hasAppOrigin(value) {
  try {
    const url = new URL(value);
    return url.protocol === 'chatto:' && url.host === 'desktop';
  } catch {
    return false;
  }
}

/**
 * Whether a URL is the frontend page on which an authorization window opens.
 * Keep the path in sync with `AUTHORIZATION_LAUNCH_PATH` in the frontend.
 */
export function isAuthorizationLaunchUrl(value) {
  if (!hasAppOrigin(value)) return false;
  const url = new URL(value);
  return url.pathname === '/servers/authorize' && url.search === '';
}

/** Whether the desktop renderer may use a browser permission. */
export function isDesktopPermissionAllowed(permission, origin) {
  return hasAppOrigin(origin) && desktopPermissions.has(permission);
}
