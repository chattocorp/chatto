import {
  onProjectionEvent,
  onPresenceChange,
  onSessionTerminated,
  type ProjectionHandler
} from '$lib/eventBus.svelte';
import type { PresenceStatus } from '$lib/render/types';

/** Subscribe to canonical projection operations on the active server. */
export function useProjectionEvent(handler: ProjectionHandler) {
  $effect(() => onProjectionEvent(handler));
}

/**
 * Hook to subscribe to presence-change events with automatic cleanup.
 * Must be called during component initialization.
 */
export function usePresenceChange(handler: (userId: string, status: PresenceStatus) => void) {
  $effect(() => onPresenceChange(handler));
}

/**
 * Hook to subscribe to session terminated events.
 * Fired when the server terminates the user's session (logout from another tab,
 * admin boot, account deletion). Must be called during component initialization.
 */
export function useSessionTerminated(handler: (reason: string) => void) {
  $effect(() => onSessionTerminated(handler));
}
