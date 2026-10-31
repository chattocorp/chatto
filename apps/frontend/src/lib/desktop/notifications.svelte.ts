/** Device-local native notification consent and renderer-owned click targets. */
import { Codecs, globalSlot } from '@chatto/client/storage/slot';
import type { NotificationStore } from '@chatto/client/server/notifications';

/** Account-scoped ownership lets one registry clear both native alerts and click targets. */
export type DesktopNotificationContext = {
  owner: object;
  occurrenceId: string;
  expiresAt?: string;
};

/** Browser deployments retain Web Push and their selected-server connection policy. */
export function hasDesktopNotifications(): boolean {
  return (
    typeof window !== 'undefined' && typeof window.chattoDesktop?.notifications?.show === 'function'
  );
}

const slot = globalSlot('desktopNotifications', false, Codecs.boolean);
// Bounded imperative handles, not UI state: changes must not trigger render effects.
const callbacks: { id: string; callback: () => void; context?: DesktopNotificationContext }[] = [];
let enabled = $state(slot.get());
// Invalidates pending preview reads even when consent is disabled and enabled again.
let consentRevision = 0;

function removeCallback(id: string): (() => void) | undefined {
  const index = callbacks.findIndex((entry) => entry.id === id);
  return index < 0 ? undefined : callbacks.splice(index, 1)[0].callback;
}

export const desktopNotifications = {
  get consentRevision() {
    return consentRevision;
  },
  get enabled() {
    return enabled;
  },
  set enabled(value: boolean) {
    if (value !== enabled) consentRevision++;
    enabled = value;
    slot.set(value);
    if (!value) clearDesktopNotifications(callbacks.map((entry) => entry.id));
  }
};

/** Show plain text. Routing stays in this renderer's authenticated session. */
export async function showDesktopNotification(
  id: string,
  content: { title: string; body: string },
  onClick: () => void,
  context?: DesktopNotificationContext
): Promise<void> {
  const bridge = typeof window !== 'undefined' ? window.chattoDesktop?.notifications : undefined;
  if (!bridge) return;
  removeCallback(id);
  if (callbacks.length >= 64) clearDesktopNotifications([callbacks[0].id]);
  callbacks.push({ id, callback: onClick, context });
  try {
    await bridge.show({
      id,
      title: truncateNotificationText(content.title, 256),
      body: truncateNotificationText(content.body, 1024)
    });
  } catch (error) {
    removeCallback(id);
    throw error;
  }
}

/** Match the native bridge's UTF-16 limits without splitting a surrogate pair. */
function truncateNotificationText(text: string, limit: number): string {
  if (text.length <= limit) return text;
  return text.slice(0, limit - 1).replace(/[\uD800-\uDBFF]$/, '') + '…';
}

/** Clear all handles owned by one account lifecycle, including alerts no longer on the loaded page. */
export function clearDesktopNotificationOwner(owner: object): void {
  clearDesktopNotifications(
    callbacks.filter((entry) => entry.context?.owner === owner).map((entry) => entry.id)
  );
}

/** Keep unknown IDs on partial pages; confirmed handling and expiry release the same registry entry. */
export function reconcileDesktopNotifications(
  owner: object,
  notifications: NotificationStore
): void {
  const owned = callbacks.filter((entry) => entry.context?.owner === owner);
  const handled = notifications.knownHandledNotificationIds(
    owned.flatMap((entry) => (entry.context ? [entry.context.occurrenceId] : []))
  );
  clearDesktopNotifications(
    owned
      .filter(
        (entry) =>
          entry.context &&
          (handled.has(entry.context.occurrenceId) ||
            (entry.context.expiresAt && Date.parse(entry.context.expiresAt) <= Date.now()))
      )
      .map((entry) => entry.id)
  );
}

/** Remove alerts and their targets on sign-out, permission loss, or document cleanup. */
export function clearDesktopNotifications(ids: string[]): void {
  if (ids.length === 0) return;
  for (const id of ids) removeCallback(id);
  if (typeof window !== 'undefined') window.chattoDesktop?.notifications?.clear(ids);
}

/** One app-root listener; disposal invalidates all callbacks from the old document. */
export function listenForDesktopNotificationClicks(): () => void {
  const stop =
    typeof window !== 'undefined'
      ? window.chattoDesktop?.notifications?.onClick((id) => {
          const callback = removeCallback(id);
          callback?.();
        })
      : undefined;
  return () => {
    stop?.();
    clearDesktopNotifications(callbacks.map((entry) => entry.id));
  };
}
