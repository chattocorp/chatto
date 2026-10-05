/**
 * Single realtime stream per connected server, covering everything the user
 * can receive (deployment-wide events and room-scoped events over one stream).
 *
 * The manager keeps one bus per registered server. Route hooks select their
 * bus from `ServerScope`; origin-global and cross-server consumers select a
 * server explicitly. Every consumer subscribes to the same ordered stream of
 * projection updates; a semantic event is `update.event`.
 */

import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { batch } from '../reactivity/index.js';
import type { RealtimeResource, RealtimeResourceUpdate } from '../api/realtimeResources.js';

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
  /** A privacy reset clears viewer authority as well as snapshot resources. */
  readonly privacyReset: boolean;
  /** Keep an existing view readable while a replacement snapshot is checked. */
  readonly retainView: boolean;

  constructor(
    init: {
      event?: RealtimeEvent | null;
      resource?: RealtimeResourceUpdate | null;
      cursor?: string | null;
      reset?: boolean;
      privacyReset?: boolean;
      retainView?: boolean;
      id?: string;
      actorId?: string;
    } = {}
  ) {
    this.resource = init.resource?.resource ?? null;
    this.replaceResource = init.resource?.replace ?? false;
    this.cursor = init.cursor ?? null;
    this.reset = init.reset ?? false;
    this.privacyReset = init.privacyReset ?? false;
    this.retainView = init.retainView ?? false;
    this.event =
      init.event ??
      (init.id || init.actorId ? new RealtimeEvent({ id: init.id, actorId: init.actorId }) : null);
  }
}
export type ProjectionHandler = (update: RealtimeProjectionUpdate) => void;

/**
 * Fan-out for one server's realtime stream.
 *
 * The server store's reducer applies each update first. Listeners then see the
 * same update in subscription order. A listener error is logged and does not
 * stop other listeners or the transport, so the event cursor still advances
 * and the update is not delivered again: a listener must finish its own
 * cleanup work before it can throw. A reducer error propagates, because the
 * projection is then out of date; the transport closes and reconnects. A reset
 * still reaches every listener before the reducer error is thrown.
 */
export class EventBus {
  #reducer: ProjectionHandler | null = null;
  #listeners = new Set<ProjectionHandler>();
  #sessionTerminatedListeners = new Set<(reason: string) => void>();

  constructor(private readonly serverId: string) {}

  /** Install the canonical reducer. A newer store for the same server replaces the previous one. */
  setReducer(reducer: ProjectionHandler): void {
    this.#reducer = reducer;
  }

  /** Remove this reducer if it is still installed, for example when its store is disposed. */
  clearReducer(reducer: ProjectionHandler): void {
    if (this.#reducer === reducer) this.#reducer = null;
  }

  /** Number of listeners, for transport diagnostics. */
  get listenerCount(): number {
    return this.#listeners.size;
  }

  /** Receive every semantic event and resource update after the reducer applied it. */
  subscribe(listener: ProjectionHandler): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  /** Receive the reason when the server terminates this session. */
  onSessionTerminated(listener: (reason: string) => void): () => void {
    this.#sessionTerminatedListeners.add(listener);
    return () => this.#sessionTerminatedListeners.delete(listener);
  }

  /** Apply a transport update to the reducer, then notify listeners. */
  publish(update: RealtimeProjectionUpdate): void {
    // Apply the reducer and notify listeners as one change: effects run once,
    // after the complete update, instead of after each written signal.
    batch(() => this.#publish(update));
  }

  #publish(update: RealtimeProjectionUpdate): void {
    const reducer = this.#reducer;
    if (!reducer) throw new Error('projection update received before reducer registration');
    if (!update.reset) {
      reducer(update);
      this.notify(update);
      return;
    }
    let reducerFailed = false;
    let reducerFailure: unknown;
    try {
      reducer(update);
    } catch (error) {
      reducerFailed = true;
      reducerFailure = error;
      console.error(`[eventBus:${this.serverId}] reset handler failed`);
    }
    this.notify(update);
    if (reducerFailed) throw reducerFailure;
  }

  /** Notify listeners of an update that the reducer already applied. */
  notify(update: RealtimeProjectionUpdate): void {
    for (const listener of [...this.#listeners]) {
      try {
        listener(update);
      } catch (error) {
        console.error(`[eventBus:${this.serverId}] handler threw`, error);
      }
    }
  }

  /** Tell session-termination listeners that the server ended this session. */
  terminateSession(reason: string): void {
    for (const listener of [...this.#sessionTerminatedListeners]) {
      try {
        listener(reason);
      } catch (error) {
        console.error(`[eventBus:${this.serverId}] session termination handler threw`, error);
      }
    }
  }
}
