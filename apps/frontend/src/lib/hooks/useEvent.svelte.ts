import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  onProjectionEvent,
  onPresenceChange,
  onSessionTerminated,
  onTypingEvent,
  type ProjectionHandler,
  type TypingEventData
} from '$lib/eventBus.svelte';
import { useServerScope } from '$lib/state/server/scope.svelte';

type ServerIdSelector = () => string;
type EventSubscription<Handler> = (serverId: string, handler: Handler) => () => void;
type PresenceHandler = (userId: string, status: PresenceStatus) => void;
type SessionTerminatedHandler = (reason: string) => void;
type TypingHandler = (data: TypingEventData) => void;

function resolveServerIdSelector(getServerId?: ServerIdSelector): ServerIdSelector {
  if (getServerId) return getServerId;
  const serverScope = useServerScope();
  return () => serverScope.serverId;
}

function useServerEvent<Handler>(
  handler: Handler,
  subscribe: EventSubscription<Handler>,
  getServerId?: ServerIdSelector
): void {
  const selectServerId = resolveServerIdSelector(getServerId);
  $effect(() => subscribe(selectServerId(), handler));
}

/** Subscribe to canonical projection operations on the route or explicitly selected server. */
export function useProjectionEvent(
  handler: ProjectionHandler,
  getServerId?: ServerIdSelector
): void {
  useServerEvent(handler, onProjectionEvent, getServerId);
}

/** Subscribe to presence changes on the route or explicitly selected server. */
export function usePresenceChange(handler: PresenceHandler, getServerId?: ServerIdSelector): void {
  useServerEvent(handler, onPresenceChange, getServerId);
}

/**
 * Subscribe to session termination from another device, an admin boot, or
 * account deletion on the route or explicitly selected server.
 */
export function useSessionTerminated(
  handler: SessionTerminatedHandler,
  getServerId?: ServerIdSelector
): void {
  useServerEvent(handler, onSessionTerminated, getServerId);
}

/** Subscribe to typing signals on the selected server with automatic cleanup. */
export function useTypingEvent(handler: TypingHandler, getServerId?: ServerIdSelector): void {
  useServerEvent(handler, onTypingEvent, getServerId);
}

export type { TypingEventData };
