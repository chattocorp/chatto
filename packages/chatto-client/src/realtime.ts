import {
  RealtimeSubscribe, RealtimeServerFrame, RealtimeInitialState,
  RealtimeRecovery, RealtimeCloseCode, type RealtimeEvent,
} from "@chatto/api-types/realtime/v1/realtime_pb";

/** Host transport. Implementations must reject redirects before sending credentials. */
export type WebSocketFactory = (url: string) => WebSocket;

/** Process-local checkpoint. Reuse only with the same server and credentials. */
export interface RealtimeCheckpoint { cursor?: string }

/** Safe transport diagnostics, without remote messages or connection details. */
export type RealtimeStatus =
  | { state: "connecting" | "reconnecting" | "stopped" }
  | { state: "ready"; recovery: "resumed" | "live-only"; gap: boolean };

export interface ConsumeRealtimeOptions {
  signal: AbortSignal;
  /** Resolves after accepting the event, not after completing its resulting work. */
  onEvent: (event: RealtimeEvent) => void | Promise<void>;
  onStatus?: (status: RealtimeStatus) => void;
  checkpoint?: RealtimeCheckpoint;
  /** Maximum queued frames. Overflow reconnects from the last accepted cursor. */
  maxPendingFrames?: number;
}

class TerminalError extends Error {}
type Outcome = { delay?: number; error?: Error };

function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise(resolve => {
    const done = () => { clearTimeout(timer); signal.removeEventListener("abort", done); resolve(); };
    const timer = setTimeout(done, ms);
    signal.addEventListener("abort", done, { once: true });
    if (signal.aborted) done();
  });
}

/** Ordered protobuf subscription with session-local recovery and bounded buffering. */
export async function consumeRealtime(
  base: URL, apiKey: string, factory: WebSocketFactory | undefined, options: ConsumeRealtimeOptions,
): Promise<void> {
  const { signal, onEvent } = options;
  const checkpoint = options.checkpoint ?? {};
  const limit = options.maxPendingFrames ?? 256;
  if (!Number.isSafeInteger(limit) || limit < 1) throw new Error("Invalid realtime queue limit");
  const url = new URL("/api/realtime", base);
  url.protocol = base.protocol === "https:" ? "wss:" : "ws:";
  const status = (value: RealtimeStatus) => options.onStatus?.(value);
  let attempt = 0;
  let connectedBefore = Boolean(checkpoint.cursor);

  async function session(): Promise<Outcome> {
    let socket: WebSocket;
    try { socket = (factory ?? (url => new WebSocket(url)))(url.href); }
    catch { return { error: new TerminalError("Cannot create the realtime transport") }; }
    socket.binaryType = "arraybuffer";
    let ended = false;
    let outcome: Outcome = {};
    let wake: (() => void) | undefined;
    const queue: ArrayBuffer[] = [];
    let queuedBytes = 0;
    const requestedResume = Boolean(checkpoint.cursor);
    const finish = (result: Outcome = {}) => {
      if (ended) return;
      ended = true;
      outcome = result;
      queue.length = 0;
      clearTimeout(timer);
      try { socket.close(); } catch { /* A connecting socket can reject close. */ }
      wake?.();
    };
    // Bound handshake and dead connections. The server sends periodic heartbeats.
    let timer = setTimeout(() => finish(), 10_000);
    const onAbort = () => finish();
    const onOpen = () => {
      if (ended) return;
      try {
        socket.send(new Uint8Array(new RealtimeSubscribe({
          protocolVersion: 4, bearerToken: apiKey,
          resumeCursor: checkpoint.cursor, initialState: RealtimeInitialState.LIVE_ONLY,
        }).toBinary()));
      } catch { finish(); }
    };
    const onMessage = (event: MessageEvent) => {
      if (ended) return;
      clearTimeout(timer);
      timer = setTimeout(() => finish(), 60_000);
      if (!(event.data instanceof ArrayBuffer)) {
        finish({ error: new TerminalError("Invalid realtime frame") });
        return;
      }
      if (queue.length >= limit || queuedBytes + event.data.byteLength > 8 * 1024 * 1024) {
        finish();
        return;
      }
      queue.push(event.data);
      queuedBytes += event.data.byteLength;
      wake?.();
    };
    // A normal socket close must drain preceding frames, notably server close guidance.
    let closed = false;
    const onClose = () => { closed = true; wake?.(); };
    const onError = () => { closed = true; wake?.(); };
    socket.addEventListener("open", onOpen);
    socket.addEventListener("message", onMessage);
    socket.addEventListener("close", onClose);
    socket.addEventListener("error", onError);
    signal.addEventListener("abort", onAbort, { once: true });
    if (signal.aborted) finish();
    try {
      while (!ended) {
        const data = queue.shift();
        if (!data) {
          if (closed) break;
          await new Promise<void>(resolve => { wake = resolve; });
          wake = undefined;
          continue;
        }
        queuedBytes -= data.byteLength;
        let frame: RealtimeServerFrame;
        try { frame = RealtimeServerFrame.fromBinary(new Uint8Array(data)); }
        catch { throw new TerminalError("Cannot decode realtime frame"); }
        switch (frame.frame.case) {
          case "event": {
            const event = frame.frame.value;
            // Unknown semantic variants retain decodable envelope metadata.
            if (event.event.case !== undefined) {
              try { await onEvent(event); }
              catch { throw new TerminalError("Realtime event handler failed"); }
            }
            if (event.cursor) checkpoint.cursor = event.cursor;
            break;
          }
          case "heartbeat":
            if (frame.frame.value.cursor) checkpoint.cursor = frame.frame.value.cursor;
            break;
          case "caughtUp": {
            const { recovery, cursor } = frame.frame.value;
            if (!cursor || (recovery !== RealtimeRecovery.RESUMED && recovery !== RealtimeRecovery.LIVE_ONLY)) {
              throw new TerminalError("Invalid realtime recovery result");
            }
            checkpoint.cursor = cursor;
            status({ state: "ready", recovery: recovery === RealtimeRecovery.RESUMED ? "resumed" : "live-only",
              gap: recovery === RealtimeRecovery.LIVE_ONLY && (requestedResume || connectedBefore) });
            connectedBefore = true;
            attempt = 0;
            break;
          }
          case "close": {
            const close = frame.frame.value;
            if (!close.reconnect || ![
              RealtimeCloseCode.TEMPORARILY_UNAVAILABLE, RealtimeCloseCode.RESYNC_REQUIRED,
              RealtimeCloseCode.PRIVILEGED_MODE_EXPIRED,
            ].includes(close.code)) throw new TerminalError("Realtime subscription was rejected or ended");
            if (close.code === RealtimeCloseCode.RESYNC_REQUIRED) delete checkpoint.cursor;
            const seconds = Number(close.retryAfter?.seconds ?? 0);
            finish({ delay: Math.max(0, seconds * 1000 + (close.retryAfter?.nanos ?? 0) / 1e6) });
            break;
          }
          default: throw new TerminalError("Unsupported realtime frame");
        }
      }
    } catch (error) {
      outcome = { error: error instanceof TerminalError ? error : new TerminalError("Realtime processing failed") };
    } finally {
      ended = true;
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      socket.removeEventListener("open", onOpen);
      socket.removeEventListener("message", onMessage);
      socket.removeEventListener("close", onClose);
      socket.removeEventListener("error", onError);
      try { socket.close(); } catch { /* Transport already closed. */ }
    }
    return outcome;
  }

  try {
    while (!signal.aborted) {
      status({ state: "connecting" });
      const result = await session();
      if (signal.aborted) break;
      if (result.error) throw result.error;
      status({ state: "reconnecting" });
      const backoff = Math.min(30_000, 500 * 2 ** Math.min(attempt++, 6));
      await pause(Math.max(result.delay ?? 0, backoff * (0.5 + Math.random() * 0.5)), signal);
    }
  } finally { status({ state: "stopped" }); }
}
