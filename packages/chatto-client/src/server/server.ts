/**
 * One Chatto server of a client: its reactive state, its realtime events, and
 * the requests that integrations use.
 *
 * `ChattoClient.connect` adds a server with a fixed bearer token, such as a
 * bot API key; `ChattoClient.server` returns a server that the client already
 * has. Both are this type. A server that `connect` added receives realtime
 * events from the start, also before `ready()` resolves, and keeps them for
 * the first `consumeEvents` or `run` call.
 *
 * Requests contact only this server, which receives the host's IP address,
 * the credentials, and the request data.
 */

import type { ServiceType } from '@bufbuild/protobuf';
import { Code, ConnectError, type Client, type Interceptor } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import {
  createChattoClient as createServiceClient,
  type ConnectAPIConfig
} from '../api/connect.js';
import type { CurrentUser } from '../api/viewer.js';
import type { PublicServerInfo } from '../api/server.js';
import { effect, effectRoot, signal, untrack } from '../reactivity/index.js';
import {
  conversationKey,
  MessagingRequests,
  replyDestination,
  type AddressedMessage,
  type AddressingOptions,
  type AddressingReason
} from '../messaging/requests.js';
import type {
  ChattoMessage,
  Destination,
  RealtimeStatus,
  RequestOptions,
  ThreadLocation,
  ThreadRead,
  ThreadReadOptions
} from '../messaging/types.js';
import type { ServerRegistration } from './catalog.js';
import type { ServerConnection } from './serverConnection.js';
import type { ServerSession } from './sessions.js';
import { ServerStateStore, type ServerStoreContext } from './store.js';

/** Settings for `ChattoClient.connect`. */
export interface ConnectOptions {
  /** HTTP or HTTPS origin of the Chatto server, without credentials. */
  serverUrl: string;
  /** Bearer token, for example a bot API key. It is kept only in memory. */
  apiKey: string;
}

/** A new projection snapshot; see {@link Server.onSnapshot}. */
export interface SnapshotInfo {
  /** Events since the previous connection can be missing. */
  readonly gap: boolean;
}

/** Realtime status of a server. */
export type ServerStatus = ServerConnection['status'];

/** Options for {@link Server.consumeEvents}. */
export interface ConsumeEventsOptions {
  /** Stops consumption. Without it, consumption stops when the server closes. */
  signal?: AbortSignal;
  /**
   * Handle one event. Events are handled in order; the next event waits until
   * the returned promise resolves. A rejection stops consumption. When more
   * than 1000 received events wait, they are dropped and a gap is reported.
   * `signal` aborts when consumption stops for any reason, for example when
   * the server ends the session.
   */
  onEvent: (event: RealtimeEvent, loop: { signal: AbortSignal }) => void | Promise<void>;
  /**
   * Receive realtime status changes. Repeated statuses are not reported
   * again, but each gap is. A throw stops consumption.
   */
  onStatus?: (status: RealtimeStatus) => void;
}

/** Options for {@link Server.run}. */
export interface RunOptions {
  /** Stops the handler loop. Without it, the loop stops when the server closes. */
  signal?: AbortSignal;
  /** The addressing reasons to handle. Default: direct messages, mentions, and replies. */
  reasons?: readonly AddressingReason[];
  /**
   * Receive realtime status changes; see {@link ConsumeEventsOptions.onStatus}.
   * Before the server accepted the key, `connecting` reports each failed
   * attempt with its `error`, for example an unreachable server.
   */
  onStatus?: (status: RealtimeStatus) => void;
  /**
   * Receive a failure of the handler or of addressing, and keep handling
   * later messages. Without it, the first failure stops the loop and `run`
   * rejects with it.
   */
  onError?: (error: unknown, event: RealtimeEvent) => void;
}

/** One addressed message and the operations to answer it; see {@link Server.run}. */
export interface MessageContext {
  /** The message addressed to the viewer. */
  readonly message: AddressedMessage;
  /** Aborts when the loop stops. Pass it to your own cancellable work. */
  readonly signal: AbortSignal;
  /** The account of the server's credentials. */
  readonly viewerId: string;
  /** The default conversation scope of the message; see `conversationKey`. */
  readonly conversationKey: string;
  /** The server that received the message. */
  readonly server: Server;
  /**
   * Reply in the message's thread, with a reference to the message. Returns
   * the IDs of the new messages; see `postMessage`.
   */
  reply(body: string): Promise<{ ids: string[] }>;
  /** Read the message's thread; see `readThread`. */
  readThread(options?: Omit<ThreadReadOptions, 'signal'>): Promise<ThreadRead>;
  /** Refresh the typing indicator in the message's thread once. */
  refreshTyping(): Promise<void>;
  /** Show the typing indicator in the message's thread while `work` runs. */
  withTyping<Result>(work: () => Promise<Result>): Promise<Result>;
  /** React to the message. */
  addReaction(emoji: string): Promise<void>;
}

/** What a server needs from its client, in addition to the store's context. */
export interface ServerContext extends ServerStoreContext {
  /** Remove this server from its client; see {@link Server.close}. */
  readonly remove: () => void;
  /**
   * The server's token is fixed, as `ChattoClient.connect` adds it. Its
   * account never changes, so a privacy reset does not fail its requests.
   */
  readonly fixedToken: boolean;
}

/** Received events that wait for the handler, dropped beyond this number. */
const MAX_QUEUED_EVENTS = 1000;

/** Realtime events and gaps that wait for a consumer. */
interface Inbox {
  queue: RealtimeEvent[];
  /** Events can be missing; reported with the next `ready`. */
  pendingGap: boolean;
  /** Called after the queue dropped its backlog. */
  onOverflow?: () => void;
  wake?: () => void;
  close(): void;
}

/** One server of a client; see the module documentation. */
export class Server extends ServerStateStore {
  /** Origin of the server. */
  readonly serverUrl: string;
  readonly #serverContext: ServerContext;
  readonly #closed = signal(false);
  /** Aborts when the server closes; every request of {@link service} includes it. */
  readonly #closeController = new AbortController();
  readonly #eventListeners = new Set<(event: RealtimeEvent) => void>();
  readonly #snapshotListeners = new Set<(snapshot: SnapshotInfo) => void>();
  readonly #requests: MessagingRequests;
  #firstInbox: Inbox | undefined;
  #disposeBusSubscription: (() => void) | undefined;

  /** The registry creates servers; use `ChattoClient.connect` or `ChattoClient.server`. */
  constructor(
    registration: ServerRegistration,
    getSession: () => ServerSession,
    originServer: boolean,
    serverConnection: ServerConnection,
    context: ServerContext,
    publicServerInfoLoader?: (baseUrl: string) => Promise<PublicServerInfo>,
    onAuthenticationRequired?: () => void,
    onViewerLoaded?: (user: CurrentUser) => void
  ) {
    super(
      registration,
      getSession,
      originServer,
      serverConnection,
      context,
      publicServerInfoLoader,
      onAuthenticationRequired,
      onViewerLoaded
    );
    this.serverUrl = registration.url;
    this.#serverContext = context;
    this.#requests = new MessagingRequests(this, async (options) => {
      const current = this.accountId;
      if (current) return current;
      return (await this.ready(options)).viewerId;
    });
    this.#disposeBusSubscription = effectRoot(() => this.#subscribeToBus());
    this.onDispose(() => this.#release());
  }

  /**
   * Keep the events that arrive from now on for the first `consumeEvents` or
   * `run` call. `ChattoClient.connect` calls it when it adds the server.
   */
  retainEvents(): void {
    this.#firstInbox ??= this.#openInbox();
  }

  /** Realtime status, or `disconnected` after close. Reactive. */
  get status(): ServerStatus {
    if (this.#closed.get()) return 'disconnected';
    return this.connection.status;
  }

  /**
   * Whether the server does not support this client's realtime protocol:
   * discovery reported an unsupported release, or the server closed the
   * stream for that reason. No events arrive. Reactive.
   */
  get realtimeUnsupported(): boolean {
    if (this.#closed.get()) return false;
    return this.connection.realtimeUnsupported || this.#releaseUnsupported();
  }

  /** Whether the server was closed or removed from its client. Reactive. */
  get closed(): boolean {
    return this.#closed.get();
  }

  /**
   * Wait until the server accepted the credentials and the viewer loaded.
   * Rejects when the server rejects them, when a viewer read fails (for
   * example because the server is unreachable or returned an error), when
   * discovery reports a server release that this client does not support,
   * when the server closes, or when `signal` aborts.
   *
   * The server retries a failed viewer read in the background, with a
   * backoff. A `ready()` call made between two attempts waits for the next
   * attempt and reports its result.
   */
  ready({ signal: abortSignal }: RequestOptions = {}): Promise<{ viewerId: string }> {
    return new Promise((resolve, reject) => {
      abortSignal?.throwIfAborted();
      // `stop` is read in a microtask, after effectRoot returned it.
      const finish = (settle: () => void) => {
        queueMicrotask(() => stop());
        abortSignal?.removeEventListener('abort', abort);
        settle();
      };
      const abort = () => finish(() => reject(abortSignal!.reason));
      abortSignal?.addEventListener('abort', abort, { once: true });
      // Report only failures of attempts that start after this call. A viewer
      // attempt clears its error first; a discovery attempt sets `loading` and
      // keeps its previous error until it settles. Between two recovery
      // attempts, wait for the next one.
      let discoveryAttempted = false;
      let viewerAttempted = false;
      const stop = effectRoot(() => {
        effect(() => {
          if (this.#closed.get()) {
            finish(() => reject(new Error('The Chatto server is closed')));
            return;
          }
          if (this.sessionEnded) {
            finish(() => reject(new Error('Chatto rejected the API key')));
            return;
          }
          const { serverInfo, currentUser } = this;
          const viewerId = this.accountId;
          if (viewerId) {
            // Discovery that fails does not block requests, but a server
            // release without a supported realtime projection sends no events.
            if (serverInfo.loading) return;
            if (this.#releaseUnsupported()) {
              finish(() => reject(new Error('The Chatto server version is not supported')));
            } else {
              finish(() => resolve({ viewerId }));
            }
            return;
          }
          // The server is unreachable or failed. The store logs the error.
          const discoveryFailed = !serverInfo.loading && serverInfo.error !== null;
          const viewerFailed = currentUser.loadError !== null;
          discoveryAttempted ||= serverInfo.loading || serverInfo.error === null;
          viewerAttempted ||= !viewerFailed;
          if (viewerFailed && viewerAttempted) {
            finish(() => reject(new Error('Could not load the viewer from the Chatto server')));
          } else if (discoveryFailed && discoveryAttempted && !currentUser.loading) {
            // A viewer read in progress can still succeed without discovery.
            finish(() => reject(new Error('Could not reach the Chatto server')));
          }
        });
      });
    });
  }

  /**
   * Create a typed Connect client for a public service, with this server's
   * authentication. After `close()`, clients send nothing, responses in
   * flight fail, and this method throws a `Canceled` `ConnectError`. For a
   * server without a fixed token, a response that arrives after a privacy
   * reset fails too, because the account can have changed.
   */
  service<T extends ServiceType>(service: T): Client<T> {
    const closed = this.#closed;
    if (closed.peek()) throw new ConnectError('The Chatto server is closed', Code.Canceled);
    const base = this.connection.apiConfig;
    // After close(), send nothing: the credentials must not outlive the server.
    const closing = this.#closeController.signal;
    const refuseAfterClose: Interceptor = (next) => (request) => {
      if (closed.peek()) throw new ConnectError('The Chatto server is closed', Code.Canceled);
      // close() cancels requests in flight, so no open request outlives it.
      return next({ ...request, signal: AbortSignal.any([request.signal, closing]) });
    };
    const send =
      base.transport ??
      ((interceptors: Interceptor[]) =>
        createConnectTransport({ baseUrl: base.baseUrl, useBinaryFormat: true, interceptors }));
    const fixedToken = this.#serverContext.fixedToken;
    const config: ConnectAPIConfig = {
      ...base,
      // Fail responses that arrive after close(). With a fixed token, the
      // account cannot change, so a privacy reset does not fail them: the
      // host, not a shared cache, receives these responses.
      dataGeneration: () =>
        closed.peek() ? -1 : fixedToken ? 0 : (base.dataGeneration?.() ?? 0),
      transport: (interceptors) => send([refuseAfterClose, ...interceptors])
    };
    return createServiceClient(service, config);
  }

  /**
   * Receive semantic realtime events after the store applied them. Listeners
   * run in order. A listener error is logged and does not stop other
   * listeners. Returns a function that removes the listener.
   */
  onEvent(listener: (event: RealtimeEvent) => void): () => void {
    this.#eventListeners.add(listener);
    return () => this.#eventListeners.delete(listener);
  }

  /**
   * Receive each new projection snapshot, after the store applied it. The
   * first snapshot is the initial one. `gap` is true when a later snapshot
   * replaced a stream that the server could not resume after it connected or
   * delivered events. Returns a function that removes the listener. To clear
   * copies of server data at a reset, use `onReset` instead.
   */
  onSnapshot(listener: (snapshot: SnapshotInfo) => void): () => void {
    this.#snapshotListeners.add(listener);
    return () => this.#snapshotListeners.delete(listener);
  }

  /**
   * Handle realtime events in order. The server keeps receiving events while
   * a handler runs; up to 1000 wait in memory. After {@link retainEvents},
   * the first call also receives the events that arrived since then; a later
   * call reports a gap first.
   *
   * Resolves when `signal` aborts or the server closes. Rejects when
   * `onEvent` or `onStatus` throws, when the server ends the session, or when
   * the server does not support the realtime protocol.
   */
  async consumeEvents({ signal: stopSignal, onEvent, onStatus }: ConsumeEventsOptions) {
    if (stopSignal?.aborted) return;
    // Events between two calls were not received: a later call reports a gap.
    const inbox = this.#firstInbox ?? this.#openInbox(true);
    this.#firstInbox = undefined;
    let failure: { error: unknown } | undefined;
    // Aborts when the loop stops, so that a running handler can stop too.
    const loop = new AbortController();
    const stop = (reason: unknown) => {
      if (!loop.signal.aborted) loop.abort(reason);
    };
    const fail = (error: unknown) => {
      failure ??= { error };
      stop(error);
      inbox.wake?.();
    };
    let lastStatus: string | undefined;
    // Report outside reactive tracking, and only changes. A failing status
    // callback stops consumption; it must not throw into the code that
    // changed the status.
    const report = (status: RealtimeStatus) => {
      const key = JSON.stringify(status);
      if (key === lastStatus && !('gap' in status && status.gap)) return;
      lastStatus = key;
      try {
        untrack(() => onStatus?.(status));
      } catch (error) {
        fail(error);
      }
    };
    // A reset gap is reported with the next `ready`, so hosts are not told
    // `ready` while the stream reconnects or a replacement snapshot loads.
    // A dropped backlog is reported at once when the stream is connected.
    inbox.onOverflow = () => {
      if (untrack(() => this.status) !== 'connected') return;
      inbox.pendingGap = false;
      report({ state: 'ready', gap: true });
    };
    let connectedBefore = false;
    const stopEffects = effectRoot(() => {
      effect(() => {
        if (this.#closed.get()) {
          stop(new Error('The Chatto server is closed'));
          inbox.wake?.();
        } else if (this.sessionEnded)
          fail(new Error('Chatto ended the session; the API key can be revoked'));
        else if (this.realtimeUnsupported)
          fail(new Error("The Chatto server does not support this client's realtime protocol"));
      });
      effect(() => {
        const status = this.status;
        const ended = () =>
          untrack(() => this.#closed.get() || this.sessionEnded || this.realtimeUnsupported);
        // A closed server or ended session does not reconnect; the loop
        // stops instead of reporting a reconnect.
        if (failure || ended()) return;
        if (status === 'connected') {
          connectedBefore = true;
          report({ state: 'ready', gap: inbox.pendingGap });
          inbox.pendingGap = false;
        } else if (status === 'connecting') {
          report({ state: connectedBefore ? 'reconnecting' : 'connecting' });
        } else if (status === 'disconnected') {
          // The runtime ends a terminated session one microtask later. Report
          // a reconnect only for a server that can still reconnect.
          queueMicrotask(() => {
            if (failure || ended() || untrack(() => this.status) !== 'disconnected') return;
            report({ state: 'reconnecting' });
          });
        }
      });
    });
    // Abort wakes the loop directly; no promise outlives one wait.
    const onAbort = () => {
      stop(stopSignal?.reason);
      inbox.wake?.();
    };
    stopSignal?.addEventListener('abort', onAbort, { once: true });
    try {
      while (!stopSignal?.aborted && !this.#closed.peek()) {
        if (failure) throw failure.error;
        const event = inbox.queue.shift();
        if (!event) {
          await new Promise<void>((resolve) => (inbox.wake = resolve));
          inbox.wake = undefined;
          continue;
        }
        await onEvent(event, { signal: loop.signal });
      }
    } finally {
      stop(new Error('The event loop stopped'));
      inbox.close();
      stopEffects();
      stopSignal?.removeEventListener('abort', onAbort);
    }
  }

  /**
   * Handle the messages addressed to the viewer, in order: direct messages,
   * mentions, and verified replies (see `addressedMessage`). Each message
   * gets a {@link MessageContext} with the operations to answer it.
   *
   * Waits for {@link ready} first, also through failed attempts while the
   * server is unreachable, and then behaves like {@link consumeEvents}.
   * Resolves when `signal` aborts or the server closes. Rejects when the
   * server rejects the key or does not support this client, and, without
   * `onError`, with the first failure of the handler.
   *
   * ```ts
   * await server.run(async (ctx) => {
   *   await ctx.withTyping(() => ctx.reply(`You said: ${ctx.message.body}`));
   * });
   * ```
   */
  async run(
    handler: (context: MessageContext) => void | Promise<void>,
    { signal: stopSignal, reasons, onStatus: reportStatus, onError }: RunOptions = {}
  ): Promise<void> {
    // Startup and the event loop report status; report a repeated plain
    // status once.
    let lastPlain: string | undefined;
    const onStatus =
      reportStatus &&
      ((status: RealtimeStatus) => {
        const plain = status.state !== 'ready' && !status.error ? status.state : undefined;
        if (plain && plain === lastPlain) return;
        lastPlain = plain;
        reportStatus(status);
      });
    const viewerId = await this.#readyForLoop(stopSignal, onStatus);
    if (viewerId === undefined) return;
    await this.consumeEvents({
      signal: stopSignal,
      onStatus,
      onEvent: async (event, { signal: loopSignal }) => {
        try {
          const message = await this.#requests.addressedMessage(event, {
            signal: loopSignal,
            reasons
          });
          if (!message) return;
          await handler(this.#messageContext(message, viewerId, loopSignal));
        } catch (error) {
          // A stopped loop ends quietly, also when the handler failed because of it.
          if (loopSignal.aborted) return;
          if (!onError) throw error;
          onError(error, event);
        }
      }
    });
  }

  /**
   * Read one visible message. Returns undefined when it is missing or does
   * not match the room. Request failures reject.
   */
  getMessage(message: { roomId: string; messageId: string }, options?: RequestOptions) {
    return this.#requests.getMessage(message, options);
  }

  /** Send one message; resolves to `{ id }` of the new message. */
  createMessage(
    destination: Destination,
    body: string,
    options?: RequestOptions & { inReplyTo?: string }
  ) {
    return this.#requests.createMessage(destination, body, options);
  }

  /** Send text of any length, split at 8000 Unicode code points and sent in order. */
  postMessage(destination: Destination, body: string, options?: RequestOptions) {
    return this.#requests.postMessage(destination, body, options);
  }

  /** Post in the message's thread with a reference to the message. */
  reply(message: ChattoMessage, body: string, options?: RequestOptions) {
    return this.#requests.reply(message, body, options);
  }

  /** Refresh the thread's typing indicator once. */
  refreshTyping(destination: Destination, options?: RequestOptions) {
    return this.#requests.refreshTyping(destination, options);
  }

  /** Show the typing indicator in the destination's thread while `work` runs. */
  withTyping<Result>(
    destination: Destination,
    work: () => Promise<Result>,
    options?: RequestOptions
  ) {
    return this.#requests.withTyping(destination, work, options);
  }

  /** Add a reaction to a message. */
  addReaction(
    message: { roomId: string; messageId: string },
    emoji: string,
    options?: RequestOptions
  ) {
    return this.#requests.addReaction(message, emoji, options);
  }

  /** Read a thread; see `MessagingRequests.readThread`. */
  readThread(location: ThreadLocation, options?: ThreadReadOptions) {
    return this.#requests.readThread(location, options);
  }

  /** Recognize an event as a message addressed to the viewer; see `run`. */
  addressedMessage(event: RealtimeEvent, options?: AddressingOptions) {
    return this.#requests.addressedMessage(event, options);
  }

  /**
   * Remove this server from its client: stop its realtime delivery, cancel
   * its requests in flight, and release its state. Requests in flight fail,
   * even when the server applied them. Use `createApi` for work that can
   * outlive the server. A server in device storage is removed from it too.
   */
  close(): void {
    if (this.#closed.peek()) return;
    this.#serverContext.remove();
    // A server that the registry did not know is released here.
    this.#release();
  }

  /** End every use of this server; its client removed or replaced it. */
  #release(): void {
    if (this.#closed.peek()) return;
    this.#closed.set(true);
    this.#closeController.abort(new Error('The Chatto server is closed'));
    this.#firstInbox?.close();
    this.#firstInbox = undefined;
    this.#disposeBusSubscription?.();
  }

  /**
   * Whether discovery reported a server release without a supported realtime
   * projection. The runtime then never opens the realtime stream.
   */
  #releaseUnsupported(): boolean {
    const { serverInfo } = this;
    return !serverInfo.loading && serverInfo.error === null && !serverInfo.isSupportedVersion;
  }

  /**
   * Wait for {@link ready} through transient failures: each call waits for the
   * next recovery attempt, and `onStatus` receives each failure. Returns
   * undefined when the loop stops first, and rejects on a failure that
   * recovery cannot fix.
   */
  async #readyForLoop(
    stopSignal: AbortSignal | undefined,
    onStatus: ((status: RealtimeStatus) => void) | undefined
  ): Promise<string | undefined> {
    onStatus?.({ state: 'connecting' });
    while (!stopSignal?.aborted && !this.#closed.peek()) {
      try {
        return (await this.ready({ signal: stopSignal })).viewerId;
      } catch (error) {
        if (stopSignal?.aborted || this.#closed.peek()) return undefined;
        // Report the condition that recovery cannot fix, not the attempt that
        // happened to fail last.
        if (this.sessionEnded) throw new Error('Chatto rejected the API key', { cause: error });
        if (this.realtimeUnsupported) {
          throw new Error('The Chatto server version is not supported', { cause: error });
        }
        onStatus?.({ state: 'connecting', error: error as Error });
      }
    }
    return undefined;
  }

  #messageContext(
    message: AddressedMessage,
    viewerId: string,
    signal: AbortSignal
  ): MessageContext {
    const destination = replyDestination(message);
    const thread = { roomId: destination.roomId, threadRootId: destination.threadRootId };
    return {
      message,
      signal,
      viewerId,
      conversationKey: conversationKey(viewerId, message),
      server: this,
      reply: (body) => this.#requests.reply(message, body, { signal }),
      readThread: (options) => this.#requests.readThread(thread, { ...options, signal }),
      refreshTyping: () => this.#requests.refreshTyping(destination, { signal }),
      withTyping: (work) => this.#requests.withTyping(destination, work, { signal }),
      addReaction: (emoji) =>
        this.#requests.addReaction({ roomId: message.roomId, messageId: message.id }, emoji, {
          signal
        })
    };
  }

  /** Subscribe to the server's events and resets. */
  #openInbox(pendingGap = false): Inbox {
    const inbox: Inbox = { queue: [], pendingGap, close: () => {} };
    const stopEvents = this.onEvent((event) => {
      if (inbox.queue.length >= MAX_QUEUED_EVENTS) {
        // Keep the host responsive: drop the backlog and report the loss.
        inbox.queue = [];
        inbox.pendingGap = true;
        inbox.onOverflow?.();
      }
      inbox.queue.push(event);
      inbox.wake?.();
    });
    const stopResets = this.onSnapshot(({ gap }) => {
      if (gap) inbox.pendingGap = true;
    });
    inbox.close = () => {
      stopEvents();
      stopResets();
    };
    return inbox;
  }

  /**
   * Subscribe whenever the runtime creates (or replaces) this server's bus.
   * The bus starts when the server is authenticated and discovery finished,
   * which can be before the viewer loads.
   */
  #subscribeToBus(): void {
    let resets = 0;
    // Whether the stream was connected after the previous reset. A resync
    // publishes a reset and then a snapshot before the next connection.
    let connectedSinceReset = false;
    // Whether events were delivered after the previous reset, for example
    // catch-up events before the stream failed. A later snapshot can then
    // skip the events that followed them.
    let eventsSinceReset = false;
    /** The reset that listeners have not received yet. */
    let pendingReset: SnapshotInfo | null = null;
    const resetPublished = signal(0);

    effect(() => {
      if (this.status === 'connected') connectedSinceReset = true;
    });
    // A snapshot publishes its reset and then its resources in one batch.
    // Report the reset when the batch ends, so listeners read the new state.
    effect(() => {
      resetPublished.get();
      const reset = pendingReset;
      pendingReset = null;
      if (reset) untrack(() => notify(this.#snapshotListeners, reset));
    });
    effect(() => {
      const bus = this.#serverContext.realtime.getBus(this.serverId);
      if (!bus) return;
      return untrack(() =>
        bus.subscribe((update) => {
          if (update.reset) {
            resets++;
            const gap = resets > 1 && (connectedSinceReset || eventsSinceReset);
            connectedSinceReset = false;
            eventsSinceReset = false;
            pendingReset = { gap: (pendingReset?.gap ?? false) || gap };
            resetPublished.update((count) => count + 1);
          }
          const event = update.event;
          if (event) {
            eventsSinceReset = true;
            notify(this.#eventListeners, event);
          }
        })
      );
    });
  }
}

/** Call each listener; one failing listener does not stop the others. */
function notify<T>(listeners: Set<(value: T) => void>, value: T): void {
  for (const listener of [...listeners]) {
    try {
      listener(value);
    } catch (error) {
      console.error('[chatto-client] a realtime listener failed', error);
    }
  }
}
