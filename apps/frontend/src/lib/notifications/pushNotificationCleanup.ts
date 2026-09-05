/** Identity of the account whose displayed pushes can be reconciled. */
export type PushNotificationOwner = { serverOrigin: string; recipientId: string };

/**
 * Close only this account's handled notifications, including remote-server
 * registrations. Enumerate before querying the server so a late push cannot
 * be compared with an older snapshot. Unsupported APIs and failures are benign.
 */
export async function cleanupPushNotifications(
  owner: PushNotificationOwner,
  handledIds: (ids: readonly string[]) => Promise<ReadonlySet<string>>,
  isCurrent: () => boolean
): Promise<void> {
  if (typeof navigator === 'undefined' || !navigator.serviceWorker?.getRegistrations) return;
  try {
    const registrations = await navigator.serviceWorker.getRegistrations();
    const notifications: Notification[] = [];
    for (const registration of registrations) {
      if (!isCurrent()) return;
      if (!registration.getNotifications) continue;
      notifications.push(...(await registration.getNotifications()));
    }
    const matching = notifications.filter((notification) => {
      const data: unknown = notification.data;
      return (
        typeof data === 'object' &&
        data !== null &&
        'serverOrigin' in data &&
        data.serverOrigin === owner.serverOrigin &&
        'recipientId' in data &&
        data.recipientId === owner.recipientId &&
        'notificationId' in data &&
        typeof data.notificationId === 'string' &&
        data.notificationId.length > 0
      );
    });
    if (!isCurrent() || matching.length === 0) return;
    const ids = await handledIds([
      ...new Set(matching.map((item) => item.data.notificationId as string))
    ]);
    if (!isCurrent()) return;
    for (const notification of matching) {
      if (ids.has(notification.data.notificationId)) notification.close();
    }
  } catch {
    // Notification centre support, permissions, and server availability vary.
  }
}
