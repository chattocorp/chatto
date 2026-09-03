/**
 * Single realtime stream per connected server, covering everything the user
 * can receive (deployment-wide events and room-scoped events over one stream).
 *
 * The manager keeps one bus per registered server. Route hooks select their
 * bus from `ServerScope`; origin-global and cross-server consumers select a
 * server explicitly.
 */

import { SvelteSet } from 'svelte/reactivity';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { eventBusManager } from './state/server/eventBus.svelte';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { RealtimeResource, RealtimeResourceUpdate } from '$lib/api-client/realtimeResources';

export type EventHandler = (event: RealtimeEvent) => void;
/** One ordered public event or canonical resource response consumed by the frontend. */
export class RealtimeProjectionUpdate {
  /** Semantic source event. Resource responses do not have one. */
  readonly event: RealtimeEvent | null;
  /** Authorized canonical resource response, when this is a resource update. */
  readonly resource: RealtimeResource | null;
  /** Whether this response replaces the complete resource family. */
  readonly replaceResource: boolean;
  /** Opaque minimum cursor for reads caused by this event. */
  readonly cursor: string | null;
  /** Clear the retained projection before applying this update. */
  readonly reset: boolean;

  constructor(
    init: {
      event?: RealtimeEvent | null;
      resource?: RealtimeResourceUpdate | null;
      cursor?: string | null;
      reset?: boolean;
      id?: string;
      actorId?: string;
    } = {}
  ) {
    this.resource = init.resource?.resource ?? null;
    this.replaceResource = init.resource?.replace ?? false;
    this.cursor = init.cursor ?? null;
    this.reset = init.reset ?? false;
    this.event =
      init.event ??
      (init.id || init.actorId ? new RealtimeEvent({ id: init.id, actorId: init.actorId }) : null);
  }
}
export type ProjectionHandler = (update: RealtimeProjectionUpdate) => void;

export interface EventBus {
  handlers: SvelteSet<EventHandler>;
  projectionHandlers: SvelteSet<ProjectionHandler>;
  sessionTerminatedHandlers: SvelteSet<(reason: string) => void>;
}

function selectedBus(serverId: string): EventBus | undefined {
  return serverId ? eventBusManager.getBus(serverId) : undefined;
}

/** Register a handler for semantic events and canonical resource responses. */
export function onProjectionEvent(serverId: string, handler: ProjectionHandler): () => void {
  const bus = selectedBus(serverId);
  if (!bus) return () => {};
  bus.projectionHandlers.add(handler);
  return () => {
    bus.projectionHandlers.delete(handler);
  };
}

// ---------------------------------------------------------------------------
// Typed event handler helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Typed event handler exports
// ---------------------------------------------------------------------------

export function onSessionTerminated(
  serverId: string,
  handler: (reason: string) => void
): () => void {
  const bus = selectedBus(serverId);
  if (!bus) return () => {};
  bus.sessionTerminatedHandlers.add(handler);
  return () => bus.sessionTerminatedHandlers.delete(handler);
}

// ---------------------------------------------------------------------------
// Room-scoped helpers
// ---------------------------------------------------------------------------

type PresenceHandler = (userId: string, status: PresenceStatus) => void;

export function onPresenceChange(serverId: string, handler: PresenceHandler): () => void {
  const bus = selectedBus(serverId);
  if (!bus) return () => {};
  const wrapper: EventHandler = (event) => {
    if (event.event.case !== 'presenceChanged' || !event.actorId) return;
    handler(event.actorId, presenceStatus(event.event.value.status));
  };
  bus.handlers.add(wrapper);
  return () => bus.handlers.delete(wrapper);
}

export interface TypingEventData {
  userId: string;
  roomId: string;
  threadRootEventId: string | null;
}

type TypingHandler = (data: TypingEventData) => void;

export function onTypingEvent(serverId: string, handler: TypingHandler): () => void {
  const bus = selectedBus(serverId);
  if (!bus) return () => {};
  const wrapper: EventHandler = (event) => {
    if (event.event.case !== 'userTyping') return;
    if (!event.actorId) return;
    const ev = event.event.value;
    handler({
      userId: event.actorId,
      roomId: ev.roomId,
      threadRootEventId: ev.threadRootEventId ?? null
    });
  };
  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

function presenceStatus(status: string): PresenceStatus {
  switch (status) {
    case 'ONLINE':
      return PresenceStatus.ONLINE;
    case 'AWAY':
      return PresenceStatus.AWAY;
    case 'DO_NOT_DISTURB':
      return PresenceStatus.DO_NOT_DISTURB;
    default:
      return PresenceStatus.OFFLINE;
  }
}
