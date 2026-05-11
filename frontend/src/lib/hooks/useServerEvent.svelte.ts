import {
  onInstanceEvent,
  onUserProfileUpdate,
  onUserSettingsUpdate,
  onNotificationLevelChanged,
  onMention,
  onNewDM,
  onNotificationCreated,
  onNotificationDismissed,
  onRoomMarkedAsRead,
  onRoomLayoutUpdated,
  onSessionTerminated,
  type EventHandler,
  type UserProfileUpdate,
  type UserSettingsUpdate,
  type NotificationLevelChanged,
  type MentionNotification,
  type DMNotification,
  type NotificationCreatedInfo,
  type NotificationDismissedInfo,
  type RoomMarkedAsReadInfo,
  type RoomLayoutUpdatedInfo
} from '$lib/serverEventBus.svelte';
import { serverEventBusManager } from '$lib/state/server/eventBus.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';

/**
 * Hook to subscribe to instance events with automatic cleanup.
 * Must be called during component initialization (not inside conditionals).
 *
 * This is a convenience wrapper around onInstanceEvent that handles
 * the $effect cleanup pattern automatically.
 *
 * @example
 * useServerEvent((event) => {
 *   if (event.event?.__typename === 'ServerUpdatedEvent') {
 *     spaceName = event.event.name;
 *   }
 * });
 */
export function useServerEvent(handler: EventHandler) {
  $effect(() => onInstanceEvent(handler));
}

/**
 * Hook to subscribe to user profile updates (avatar, display name changes).
 * Must be called during component initialization.
 */
export function useUserProfileUpdate(handler: (update: UserProfileUpdate) => void) {
  $effect(() => onUserProfileUpdate(handler));
}

/**
 * Hook to subscribe to mention notifications.
 * Must be called during component initialization.
 */
export function useMention(handler: (notification: MentionNotification) => void) {
  $effect(() => onMention(handler));
}

/**
 * Hook to subscribe to new DM message notifications.
 * Must be called during component initialization.
 */
export function useNewDM(handler: (notification: DMNotification) => void) {
  $effect(() => onNewDM(handler));
}

/**
 * Hook to subscribe to notification created events.
 * Must be called during component initialization.
 */
export function useNotificationCreated(handler: (info: NotificationCreatedInfo) => void) {
  $effect(() => onNotificationCreated(handler));
}

/**
 * Hook to subscribe to notification dismissed events.
 * Must be called during component initialization.
 */
export function useNotificationDismissed(handler: (info: NotificationDismissedInfo) => void) {
  $effect(() => onNotificationDismissed(handler));
}

/**
 * Hook to subscribe to room marked as read events (multi-tab/multi-device sync).
 * Must be called during component initialization.
 */
export function useRoomMarkedAsRead(handler: (info: RoomMarkedAsReadInfo) => void) {
  $effect(() => onRoomMarkedAsRead(handler));
}

/**
 * Hook to subscribe to user settings update events (multi-tab sync).
 * Must be called during component initialization.
 */
export function useUserSettingsUpdate(handler: (update: UserSettingsUpdate) => void) {
  $effect(() => onUserSettingsUpdate(handler));
}

/**
 * Hook to subscribe to notification level changed events (multi-tab sync).
 * Must be called during component initialization.
 */
export function useNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void) {
  $effect(() => onNotificationLevelChanged(handler));
}

/**
 * Hook to subscribe to room layout updated events (real-time sidebar updates).
 * Must be called during component initialization.
 */
export function useRoomLayoutUpdated(handler: (info: RoomLayoutUpdatedInfo) => void) {
  $effect(() => onRoomLayoutUpdated(handler));
}

/**
 * Hook to subscribe to session terminated events.
 * Fired when the server terminates the user's session (logout from another tab,
 * admin boot, account deletion). Must be called during component initialization.
 */
export function useSessionTerminated(handler: (reason: string) => void) {
  $effect(() => onSessionTerminated(handler));
}

// --- Active-instance hooks ---
// These subscribe to whichever instance is currently active (from URL context),
// re-subscribing reactively when the instance changes. Use these in components
// that live across instance switches (e.g., the space layout).

/**
 * Subscribe to instance events on the ACTIVE instance's event bus.
 * Re-subscribes automatically when the active instance changes.
 * Reads instance ID from Svelte context (set by [[serverId=hostname]] layout).
 */
export function useActiveInstanceEvent(handler: EventHandler) {
  const getInstanceId = getActiveServer();
  $effect(() => {
    const id = getInstanceId();
    if (!id) return;
    const bus = serverEventBusManager.getBus(id);
    if (!bus) return;
    bus.handlers.add(handler);
    return () => {
      bus.handlers.delete(handler);
    };
  });
}

/**
 * Subscribe to room layout updated events on the ACTIVE instance's event bus.
 * Re-subscribes automatically when the active instance changes.
 */
export function useActiveRoomLayoutUpdated(handler: (info: RoomLayoutUpdatedInfo) => void) {
  const wrapper: EventHandler = (event) => {
    if (!event.event) return;
    if (event.event.__typename === 'RoomLayoutUpdatedEvent') {
      handler({});
    }
  };
  useActiveInstanceEvent(wrapper);
}
