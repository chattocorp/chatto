/**
 * Single realtime stream per connected server, covering everything the user
 * can receive (deployment-wide events and room-scoped events over one stream).
 *
 * The manager keeps one bus per registered server. Consumers register
 * handlers either via Svelte context (current active server) or directly
 * against a specific server's bus through the manager (used by the
 * cross-server sidebar wiring).
 */

import { createContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import type { PresenceStatus } from './render/types';
import { eventBusManager } from './state/server/eventBus.svelte';
import type { RealtimeProjectionEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';

type EventEnvelopeEvent =
  | {
      kind: typeof RoomEventKind.MentionNotification;
      roomId: string;
      actorUserId: string;
      actorDisplayName: string;
      roomName: string;
    }
  | {
      kind: typeof RoomEventKind.NewDirectMessageNotification;
      roomId: string;
      senderId: string;
      senderDisplayName: string;
      senderAvatarUrl: string;
      conversationName: string;
    }
  | { kind: typeof RoomEventKind.PresenceChanged; status: PresenceStatus }
  | { kind: typeof RoomEventKind.SessionTerminated; reason: string }
  | {
      kind: typeof RoomEventKind.UserTyping;
      roomId: string;
      typingThreadRootEventId?: string | null;
    };

export type EventEnvelope = {
  id: string;
  createdAt: string;
  actorId?: string | null;
  event: EventEnvelopeEvent;
};

export type EventHandler = (event: EventEnvelope) => void;
export type ProjectionHandler = (event: RealtimeProjectionEvent) => void;

export interface EventBus {
  handlers: SvelteSet<EventHandler>;
  projectionHandlers: SvelteSet<ProjectionHandler>;
}

// The context holds a getter — not a fixed bus — so reads from inside a
// consumer's $effect track whatever reactive state the getter touches
// (typically `page.params.serverId` via `getActiveServer`). When the URL
// `[serverId]` param changes, every typed-event consumer
// re-subscribes against the new server's bus without needing a remount or
// a context refresh.
const [getServerBusGetter, setServerBusGetter] = createContext<() => EventBus | undefined>();

/**
 * Expose the active server's event bus to descendants via Svelte context.
 * Takes a getter so the context follows the active server reactively —
 * pass `() => activeServerId` (e.g. `getActiveServer()`) inside the
 * `[serverId]` tree, or `() => originServerId` at the top of the
 * authenticated app where the bus is fixed to the origin.
 */
export function provideEventBus(getServerId: () => string): void {
  setServerBusGetter(() => {
    const id = getServerId();
    return id ? eventBusManager.getBus(id) : undefined;
  });
}

/** Register a handler for canonical projection operations on the active server. */
export function onProjectionEvent(handler: ProjectionHandler): () => void {
  let getBus: () => EventBus | undefined;
  try {
    getBus = getServerBusGetter();
  } catch {
    return () => {};
  }
  const bus = getBus();
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

function onTypedEvent<TKind extends EventEnvelopeEvent['kind'], T>(
  kind: TKind,
  extract: (envelope: EventEnvelope, event: Extract<EventEnvelopeEvent, { kind: TKind }>) => T,
  handler: (data: T) => void
): () => void {
  let getBus: () => EventBus | undefined;
  try {
    getBus = getServerBusGetter();
  } catch {
    return () => {};
  }
  const bus = getBus();
  if (!bus) return () => {};

  const wrapper: EventHandler = (envelope) => {
    if (roomEventKind(envelope.event) === kind) {
      handler(extract(envelope, envelope.event as Extract<EventEnvelopeEvent, { kind: TKind }>));
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

export function onSessionTerminated(handler: (reason: string) => void): () => void {
  return onTypedEvent(
    RoomEventKind.SessionTerminated,
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

export function onPresenceChange(handler: PresenceHandler): () => void {
  return onTypedEvent(
    RoomEventKind.PresenceChanged,
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

export function onTypingEvent(handler: TypingHandler): () => void {
  let getBus: () => EventBus | undefined;
  try {
    getBus = getServerBusGetter();
  } catch {
    return () => {};
  }
  const bus = getBus();
  if (!bus) return () => {};
  const wrapper: EventHandler = (event) => {
    if (roomEventKind(event.event) !== RoomEventKind.UserTyping) return;
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
