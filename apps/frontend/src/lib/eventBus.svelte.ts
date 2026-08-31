/**
 * Single realtime stream per connected server, covering everything the user
 * can receive (deployment-wide events and room-scoped events over one stream).
 *
 * The manager keeps one bus per registered server. Route hooks select their
 * bus from `ServerScope`; origin-global and cross-server consumers select a
 * server explicitly.
 */

import { SvelteSet } from 'svelte/reactivity';
import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { eventBusManager } from './state/server/eventBus.svelte';
import { RealtimeEvent, type RealtimeStateItem } from '@chatto/api-types/realtime/v1/realtime_pb';
import {
  TransientEventKind,
  transientEventKind,
  type TransientEventEnvelope,
  type TransientEventPayload
} from '$lib/realtimeEvents';

export type EventHandler = (event: TransientEventEnvelope) => void;
/** One ordered update consumed by the bundled frontend projection. */
export class RealtimeProjectionUpdate {
  /** Semantic source event. Snapshot state items do not have one. */
  readonly event: RealtimeEvent | null;
  /** Authorized current state to apply in order. */
  readonly state: readonly RealtimeStateItem[];
  /** Clear the retained projection before applying this update. */
  readonly reset: boolean;

  constructor(
    init: {
      event?: RealtimeEvent | null;
      state?: readonly RealtimeStateItem[];
      /** Test and migration convenience for the former projection vocabulary. */
      operations?: readonly RealtimeStateItem[];
      reset?: boolean;
      id?: string;
      actorId?: string;
    } = {}
  ) {
    this.state = init.state ?? init.operations ?? [];
    this.reset = init.reset ?? false;
    this.event =
      init.event ??
      (init.id || init.actorId
        ? new RealtimeEvent({ id: init.id, actorId: init.actorId, state: [...this.state] })
        : null);
  }
}
export type ProjectionHandler = (update: RealtimeProjectionUpdate) => void;

export interface EventBus {
  handlers: SvelteSet<EventHandler>;
  projectionHandlers: SvelteSet<ProjectionHandler>;
}

function selectedBus(serverId: string): EventBus | undefined {
  return serverId ? eventBusManager.getBus(serverId) : undefined;
}

/** Register a handler for semantic events and canonical current-state items. */
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

// The extractor receives the inner event payload; helpers needing envelope
// fields (actorId, etc.) read them from the closure instead.

function onTypedEvent<TKind extends TransientEventPayload['kind'], T>(
  serverId: string,
  kind: TKind,
  extract: (
    envelope: TransientEventEnvelope,
    event: Extract<TransientEventPayload, { kind: TKind }>
  ) => T,
  handler: (data: T) => void
): () => void {
  const bus = selectedBus(serverId);
  if (!bus) return () => {};

  const wrapper: EventHandler = (envelope) => {
    if (transientEventKind(envelope.event) === kind) {
      handler(extract(envelope, envelope.event as Extract<TransientEventPayload, { kind: TKind }>));
    }
  };

  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

// ---------------------------------------------------------------------------
// Typed event handler exports
// ---------------------------------------------------------------------------

export function onSessionTerminated(
  serverId: string,
  handler: (reason: string) => void
): () => void {
  return onTypedEvent(
    serverId,
    TransientEventKind.SessionTerminated,
    (_env, e) => {
      return e.reason;
    },
    handler
  );
}

// ---------------------------------------------------------------------------
// Room-scoped helpers
// ---------------------------------------------------------------------------

type PresenceHandler = (userId: string, status: PresenceStatus) => void;

export function onPresenceChange(serverId: string, handler: PresenceHandler): () => void {
  return onTypedEvent(
    serverId,
    TransientEventKind.PresenceChanged,
    (envelope, e) => {
      return { userId: envelope.actorId, status: e.status as PresenceStatus };
    },
    ({ userId, status }) => {
      if (!userId) return;
      handler(userId, status);
    }
  );
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
    if (transientEventKind(event.event) !== TransientEventKind.UserTyping) return;
    if (!event.actorId) return;
    const ev = event.event as { roomId: string; typingThreadRootEventId?: string | null };
    handler({
      userId: event.actorId,
      roomId: ev.roomId,
      threadRootEventId: ev.typingThreadRootEventId ?? null
    });
  };
  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}
