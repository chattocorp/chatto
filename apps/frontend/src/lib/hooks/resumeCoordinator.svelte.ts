import { getActiveServer } from '$lib/state/activeServer.svelte';

export type ResumeSignalReason =
  | 'visibility'
  | 'pageshow'
  | 'online'
  | 'reconnect'
  | 'event-bus-subscription-ended'
  | 'event-bus-ws-reconnected'
  | 'event-bus-heartbeat-stalled'
  | 'manual-shortcut';

export type ResumeSignalPhase = 'immediate' | 'projection-grace';

export type ResumeSignalSource = 'browser' | 'event-bus' | 'reconnect' | 'manual';

export type ResumeSignal = {
  serverId: string;
  reason: ResumeSignalReason;
  phase: ResumeSignalPhase;
  source: ResumeSignalSource;
  hiddenDurationMs: number | null;
  epoch: number;
  at: number;
};

type ResumeCallback = (signal: ResumeSignal) => void;

type PendingResume = {
  signal: ResumeSignal;
  timer: ReturnType<typeof setTimeout>;
};

const DEFAULT_RESUME_COALESCE_MS = 1_000;

// Deliberately non-reactive: callbacks register from $effect cleanups. Using
// SvelteMap/SvelteSet here makes registration itself retrigger those effects.
// eslint-disable-next-line svelte/prefer-svelte-reactivity
const callbacksByServer = new Map<string, Set<ResumeCallback>>();
// eslint-disable-next-line svelte/prefer-svelte-reactivity
const pendingByServer = new Map<string, PendingResume>();

let hiddenStartedAt: number | null =
  typeof document !== 'undefined' && document.visibilityState === 'hidden' ? Date.now() : null;
let lastHiddenDurationMs: number | null = null;
let epoch = 0;

function priority(signal: Pick<ResumeSignal, 'source' | 'phase'>): number {
  if (signal.source === 'manual') return 5;
  if (signal.source === 'event-bus' && signal.phase === 'immediate') return 4;
  if (signal.source === 'reconnect') return 3;
  if (signal.source === 'event-bus') return 2;
  return 1;
}

function mergeSignals(current: ResumeSignal, next: ResumeSignal): ResumeSignal {
  const winner = priority(next) >= priority(current) ? next : current;
  return {
    ...winner,
    hiddenDurationMs: Math.max(current.hiddenDurationMs ?? 0, next.hiddenDurationMs ?? 0) || null,
    epoch: Math.max(current.epoch, next.epoch),
    at: Math.max(current.at, next.at)
  };
}

function emitNow(serverId: string, signal: ResumeSignal): void {
  const callbacks = callbacksByServer.get(serverId);
  if (!callbacks) return;
  for (const callback of callbacks) {
    try {
      callback(signal);
    } catch (err) {
      console.error(`[resume:${serverId}] callback failed`, err);
    }
  }
}

function flush(serverId: string): void {
  const pending = pendingByServer.get(serverId);
  if (!pending) return;
  pendingByServer.delete(serverId);
  emitNow(serverId, pending.signal);
}

function signalFor(
  serverId: string,
  input: {
    reason: ResumeSignalReason;
    phase?: ResumeSignalPhase;
    source: ResumeSignalSource;
    hiddenDurationMs?: number | null;
  }
): ResumeSignal {
  return {
    serverId,
    reason: input.reason,
    phase: input.phase ?? 'immediate',
    source: input.source,
    hiddenDurationMs: input.hiddenDurationMs ?? lastHiddenDurationMs,
    epoch: ++epoch,
    at: Date.now()
  };
}

export function emitServerResumeSignal(
  serverId: string | null | undefined,
  input: {
    reason: ResumeSignalReason;
    phase?: ResumeSignalPhase;
    source: ResumeSignalSource;
    hiddenDurationMs?: number | null;
  },
  options: { coalesceMs?: number } = {}
): void {
  if (!serverId) return;
  if (!callbacksByServer.has(serverId)) return;

  const next = signalFor(serverId, input);
  const coalesceMs = options.coalesceMs ?? DEFAULT_RESUME_COALESCE_MS;
  const pending = pendingByServer.get(serverId);
  if (pending) {
    pending.signal = mergeSignals(pending.signal, next);
    return;
  }

  if (coalesceMs <= 0) {
    emitNow(serverId, next);
    return;
  }

  const timer = setTimeout(() => flush(serverId), coalesceMs);
  pendingByServer.set(serverId, { signal: next, timer });
}

export function registerServerResumeCallback(
  serverId: string | null | undefined,
  callback: ResumeCallback
): () => void {
  if (!serverId) return () => {};
  let callbacks = callbacksByServer.get(serverId);
  if (!callbacks) {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    callbacks = new Set();
    callbacksByServer.set(serverId, callbacks);
  }
  callbacks.add(callback);

  return () => {
    const current = callbacksByServer.get(serverId);
    if (!current) return;
    current.delete(callback);
    if (current.size === 0) {
      callbacksByServer.delete(serverId);
      const pending = pendingByServer.get(serverId);
      if (pending) {
        clearTimeout(pending.timer);
        pendingByServer.delete(serverId);
      }
    }
  };
}

export function emitActiveServerResumeSignal(
  input: {
    reason: ResumeSignalReason;
    phase?: ResumeSignalPhase;
    source: ResumeSignalSource;
    hiddenDurationMs?: number | null;
  },
  options?: { coalesceMs?: number }
): void {
  emitServerResumeSignal(getActiveServer(), input, options);
}

function emitToRegisteredServers(
  reason: ResumeSignalReason,
  hiddenDurationMs: number | null
): void {
  for (const serverId of callbacksByServer.keys()) {
    emitServerResumeSignal(serverId, {
      reason,
      source: 'browser',
      hiddenDurationMs
    });
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      hiddenStartedAt = Date.now();
      return;
    }

    const hiddenDurationMs = hiddenStartedAt === null ? null : Date.now() - hiddenStartedAt;
    lastHiddenDurationMs = hiddenDurationMs;
    hiddenStartedAt = null;
    emitToRegisteredServers('visibility', hiddenDurationMs);
  });
}

if (typeof window !== 'undefined') {
  window.addEventListener('pageshow', () => {
    emitToRegisteredServers('pageshow', lastHiddenDurationMs);
  });
  window.addEventListener('online', () => {
    emitToRegisteredServers('online', lastHiddenDurationMs);
  });
}
