import type { ProjectionHandler } from '@chatto/client/realtime/eventBus';
import { eventBusManager } from '$lib/client';
import { useServerScope } from '$lib/state/server/scope.svelte';

type ServerIdSelector = () => string;

export interface TypingEventData {
  userId: string;
  roomId: string;
  threadRootEventId: string | null;
}

type TypingHandler = (data: TypingEventData) => void;

function resolveServerIdSelector(getServerId?: ServerIdSelector): ServerIdSelector {
  if (getServerId) return getServerId;
  const serverScope = useServerScope();
  return () => serverScope.serverId;
}

/**
 * Subscribe to canonical realtime events and snapshot resource updates for one
 * server. The subscription follows the selected server and ends with the owner.
 */
export function useProjectionEvent(
  handler: ProjectionHandler,
  getServerId?: ServerIdSelector
): void {
  const selectServerId = resolveServerIdSelector(getServerId);
  $effect(() => {
    const serverId = selectServerId();
    return serverId ? eventBusManager.getBus(serverId)?.subscribe(handler) : undefined;
  });
}

/** Subscribe to typing signals on the selected server with automatic cleanup. */
export function useTypingEvent(handler: TypingHandler, getServerId?: ServerIdSelector): void {
  useProjectionEvent(({ event }) => {
    if (event?.event.case !== 'userTyping' || !event.actorId) return;
    handler({
      userId: event.actorId,
      roomId: event.event.value.roomId,
      threadRootEventId: event.event.value.threadRootEventId ?? null
    });
  }, getServerId);
}
