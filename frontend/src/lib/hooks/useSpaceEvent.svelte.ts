import { onServerEvent, onPresenceChange, type ServerEvent } from '$lib/serverEventBus.svelte';
import type { PresenceStatus } from '$lib/gql/graphql';

type ServerEventHandler = (event: ServerEvent) => void;

/**
 * Hook to subscribe to events on the active server's bus, with automatic
 * cleanup. Receives every event in the unified `myServerEvents` stream —
 * filter by `event.event?.__typename` in the handler.
 *
 * Must be called during component initialization (not inside conditionals).
 *
 * (Despite the historical name, this is no longer "space-scoped" — it's
 * the canonical bus subscription. Kept exported for source-compat with
 * call sites that consume room events.)
 *
 * @example
 * useSpaceEvent((event) => {
 *   if (event.event?.__typename === 'MessagePostedEvent') {
 *     handleNewMessage(event);
 *   }
 * });
 */
export function useSpaceEvent(handler: ServerEventHandler) {
  $effect(() => onServerEvent(handler));
}

/**
 * Hook to subscribe to presence change events with automatic cleanup.
 * Must be called during component initialization.
 */
export function usePresenceChange(handler: (userId: string, status: PresenceStatus) => void) {
  $effect(() => onPresenceChange(handler));
}
