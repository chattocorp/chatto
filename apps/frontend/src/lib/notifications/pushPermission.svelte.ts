/**
 * Reactive browser notification permission.
 *
 * Notification permission belongs to the browser origin, not to a Chatto
 * server. It can change outside this page: in another tab, in browser or
 * operating-system settings, or through the permission prompt. This state
 * follows those changes so the automatic permission request, push
 * registration, and the push settings always use the current value.
 */

function readNotificationPermission(): NotificationPermission | null {
  if (typeof Notification === 'undefined') return null;
  return Notification.permission;
}

class NotificationPermissionState {
  /** Changes whenever the browser reports a permission change. */
  #current = $state<NotificationPermission | null>(readNotificationPermission());
  #watching = false;

  /**
   * The current permission, or null when the browser has no Notification API.
   * Reading it starts to follow permission changes. The value always comes
   * from the browser; the tracked copy makes reactive readers update when a
   * change event arrives.
   */
  get current(): NotificationPermission | null {
    this.#watch();
    void this.#current;
    return readNotificationPermission();
  }

  /** Reads the permission from the browser again and returns it. */
  refresh(): NotificationPermission | null {
    const permission = readNotificationPermission();
    this.#current = permission;
    return permission;
  }

  #watch(): void {
    if (this.#watching || typeof window === 'undefined' || typeof document === 'undefined') return;
    this.#watching = true;

    // Not every browser sends permission change events. Focus and visibility
    // changes cover a decision made in another tab or in browser settings.
    window.addEventListener('focus', () => this.refresh());
    document.addEventListener('visibilitychange', () => this.refresh());
    void navigator.permissions
      ?.query({ name: 'notifications' })
      .then((status) => status.addEventListener('change', () => this.refresh()))
      .catch(() => undefined);
  }
}

/** The single owner of this page's notification permission state. */
export const notificationPermission = new NotificationPermissionState();
