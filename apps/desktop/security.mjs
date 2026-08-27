const desktopPermissions = new Set(["media", "notifications"]);

/** Whether a URL belongs to the privileged Chatto Desktop renderer origin. */
export function hasAppOrigin(value) {
  try {
    const url = new URL(value);
    return url.protocol === "chatto:" && url.host === "desktop";
  } catch {
    return false;
  }
}

/** Whether the desktop renderer may use a browser permission. */
export function isDesktopPermissionAllowed(permission, origin) {
  return hasAppOrigin(origin) && desktopPermissions.has(permission);
}
