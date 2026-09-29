/**
 * Framework-neutral fine-grained reactivity for the Chatto client.
 *
 * The graph has three node kinds. A {@link Signal} holds a value. A
 * {@link Computed} derives a cached value from other nodes. An effect runs a
 * function again after a node that it read changes. Reads inside a computed or
 * an effect are tracked automatically.
 *
 * Writes mark dependent nodes as possibly stale. Computed values are pulled
 * lazily: a computed recomputes only when it is read and one of its sources
 * has a new version. Effects run synchronously when the outermost write or
 * {@link batch} completes. An error in a later run of an effect is logged; it
 * does not reach the code that wrote the signal.
 *
 * A computed that no effect observes does not register with its sources. It
 * checks source versions when it is read instead, so an unobserved computed
 * can be garbage-collected together with its owner even when its sources live
 * longer.
 *
 * UI adapters observe reads outside the graph through {@link setReadHook}; see
 * `@chatto/client/svelte`.
 */

/** A node whose value can be read and observed. */
export interface ReactiveSource<T = unknown> {
  /** Read the value and track the read in the active computed, effect, or UI adapter. */
  get(): T;
  /** Read the value without tracking it. */
  peek(): T;
}

/** Equality used to decide whether a write or recomputation changed a value. */
export type Equality<T> = (previous: T, next: T) => boolean;

/** Options for a signal or computed value. */
export interface ReactiveOptions<T> {
  /**
   * Returns true when two values are equal, so observers are not notified.
   * Defaults to `Object.is`. Pass `false` to notify on every write.
   */
  equals?: Equality<T> | false;
}

type Observer = ComputedNode<unknown> | EffectNode;
type Source = SignalNode<unknown> | ComputedNode<unknown>;

/** Hook that receives untracked reads. */
export type ReadHook = (source: ReactiveSource) => void;

/** The computed or effect that is currently running. */
let activeObserver: Observer | null = null;
/** Sources read by the active observer during its current run. */
let activeSources: Map<Source, number> | null = null;
/** Effects created by the active effect or root belong to this owner. */
let activeOwner: Owner | null = null;
/** Nested untrack depth. Reads inside untrack are not tracked or hooked. */
let untrackDepth = 0;
/** Incremented by every signal write. Unobserved computeds compare it on read. */
let globalVersion = 0;
let batchDepth = 0;
let pendingEffects: EffectNode[] = [];
let flushing = false;
let readHook: ReadHook | null = null;

/** Upper bound for effect runs in one flush, to stop write loops between effects. */
const MAX_EFFECT_RUNS_PER_FLUSH = 100_000;

/**
 * Install the adapter hook for reads made outside computeds and effects.
 * Only one hook can be installed; installing `null` removes it. The hook
 * must not write signals.
 */
export function setReadHook(hook: ReadHook | null): void {
  readHook = hook;
}

/**
 * Whether a read now is observed: by the running computed or effect, or by an
 * installed UI adapter. Collections use it to create per-key signals only
 * when a read can subscribe to them.
 */
export function isObservingReads(): boolean {
  return untrackDepth === 0 && (activeSources !== null || readHook !== null);
}

function track(source: Source): void {
  if (untrackDepth > 0) return;
  if (activeSources) {
    if (!activeSources.has(source)) activeSources.set(source, source.version);
    return;
  }
  readHook?.(source);
}

function defaultEquals(previous: unknown, next: unknown): boolean {
  return Object.is(previous, next);
}

function equalityFrom<T>(options: ReactiveOptions<T> | undefined): Equality<T> | null {
  if (options?.equals === false) return null;
  return (options?.equals as Equality<T> | undefined) ?? defaultEquals;
}

class SignalNode<T> implements ReactiveSource<T> {
  /** Incremented on every accepted write. */
  version = 0;
  /** Live observers that must be marked when this value changes. */
  readonly observers = new Set<Observer>();
  #value: T;
  readonly #equals: Equality<T> | null;

  constructor(value: T, options?: ReactiveOptions<T>) {
    this.#value = value;
    this.#equals = equalityFrom(options);
  }

  get(): T {
    track(this as SignalNode<unknown>);
    return this.#value;
  }

  peek(): T {
    return this.#value;
  }

  set(value: T): void {
    if (this.#equals?.(this.#value, value)) return;
    this.#value = value;
    this.notify();
  }

  update(updater: (value: T) => T): void {
    this.set(updater(this.#value));
  }

  /** Notify observers after an in-place mutation of the current value. */
  notify(): void {
    this.version++;
    globalVersion++;
    batchDepth++;
    try {
      for (const observer of this.observers) observer.markStale();
    } finally {
      endBatch();
    }
  }

  addObserver(observer: Observer): void {
    this.observers.add(observer);
  }

  removeObserver(observer: Observer): void {
    this.observers.delete(observer);
  }
}

/**
 * Run an observer function while collecting the sources it reads into
 * `sources`. The map keeps the reads made before a failure.
 */
function collectSources<T>(observer: Observer, sources: Map<Source, number>, run: () => T): T {
  const previousObserver = activeObserver;
  const previousSources = activeSources;
  const previousUntrack = untrackDepth;
  activeObserver = observer;
  activeSources = sources;
  untrackDepth = 0;
  try {
    return run();
  } finally {
    activeObserver = previousObserver;
    activeSources = previousSources;
    untrackDepth = previousUntrack;
  }
}

/**
 * Replace an observer's source links. Live observers register with each
 * source; unobserved computeds only remember versions.
 */
function relink(
  observer: Observer,
  previous: Map<Source, number>,
  next: Map<Source, number>,
  live: boolean
): void {
  if (!live) return;
  for (const source of previous.keys()) {
    if (!next.has(source)) source.removeObserver(observer);
  }
  for (const source of next.keys()) {
    if (!previous.has(source)) source.addObserver(observer);
  }
}

/** Whether any source has a newer version than the observer last saw. */
function sourcesChanged(sources: Map<Source, number>): boolean {
  for (const [source, seenVersion] of sources) {
    if (source instanceof ComputedNode) source.refresh();
    if (source.version !== seenVersion) return true;
  }
  return false;
}

class ComputedNode<T> implements ReactiveSource<T> {
  /** Incremented when a recomputation produces a different value. */
  version = 0;
  readonly observers = new Set<Observer>();
  #sources = new Map<Source, number>();
  #fn: () => T;
  #equals: Equality<T> | null;
  #value: T | undefined = undefined;
  #error: unknown = undefined;
  #hasError = false;
  #initialized = false;
  /** Set by propagation while this computed is live. */
  #stale = true;
  #seenGlobalVersion = -1;
  #computing = false;

  constructor(fn: () => T, options?: ReactiveOptions<T>) {
    this.#fn = fn;
    this.#equals = equalityFrom(options);
  }

  /** Live computeds register with their sources; unobserved ones do not. */
  get #live(): boolean {
    return this.observers.size > 0;
  }

  get(): T {
    // Refresh before tracking so the observer records the current version.
    this.refresh();
    track(this as ComputedNode<unknown>);
    if (this.#hasError) throw this.#error;
    return this.#value as T;
  }

  peek(): T {
    this.refresh();
    if (this.#hasError) throw this.#error;
    return this.#value as T;
  }

  /** Bring the cached value up to date without tracking the read. */
  refresh(): void {
    if (this.#computing) throw new Error('Cycle detected in computed value');
    if (this.#initialized) {
      if (this.#live ? !this.#stale : this.#seenGlobalVersion === globalVersion) return;
      if (!sourcesChanged(this.#sources)) {
        this.#stale = false;
        this.#seenGlobalVersion = globalVersion;
        return;
      }
    }
    this.#recompute();
  }

  #recompute(): void {
    this.#computing = true;
    const previousSources = this.#sources;
    // A failed run keeps the sources it read before failing, so a later
    // change to one of them retries the computation.
    const sources = new Map<Source, number>();
    let value: T | undefined;
    let error: unknown;
    let failed = false;
    try {
      value = collectSources(this as ComputedNode<unknown>, sources, this.#fn);
    } catch (caught) {
      failed = true;
      error = caught;
    } finally {
      this.#computing = false;
    }
    this.#sources = sources;
    relink(this as ComputedNode<unknown>, previousSources, sources, this.#live);
    this.#stale = false;
    this.#seenGlobalVersion = globalVersion;

    if (failed) {
      this.#hasError = true;
      this.#error = error;
      this.#initialized = true;
      this.version++;
      return;
    }
    const changed =
      !this.#initialized || this.#hasError || !this.#equals?.(this.#value as T, value as T);
    this.#hasError = false;
    this.#error = undefined;
    this.#initialized = true;
    if (changed) {
      this.#value = value as T;
      this.version++;
    }
  }

  markStale(): void {
    if (this.#stale) return;
    this.#stale = true;
    for (const observer of this.observers) observer.markStale();
  }

  addObserver(observer: Observer): void {
    if (!this.#live) {
      // Becoming live: validate the cached value while still unobserved, then
      // register with its sources. From now on, propagation marks it stale.
      this.refresh();
      for (const source of this.#sources.keys()) source.addObserver(this as ComputedNode<unknown>);
      this.#stale = false;
    }
    this.observers.add(observer);
  }

  removeObserver(observer: Observer): void {
    if (!this.observers.delete(observer) || this.#live) return;
    for (const source of this.#sources.keys()) source.removeObserver(this as ComputedNode<unknown>);
    this.#seenGlobalVersion = -1;
  }
}

/** Ownership scope for effects. Disposing an owner disposes its effects. */
class Owner {
  readonly #children = new Set<{ dispose(): void }>();
  #disposed = false;

  get disposed(): boolean {
    return this.#disposed;
  }

  adopt(child: { dispose(): void }): void {
    if (this.#disposed) {
      child.dispose();
      return;
    }
    this.#children.add(child);
  }

  release(child: { dispose(): void }): void {
    this.#children.delete(child);
  }

  /** Dispose every child. A failing cleanup is logged and does not stop the others. */
  disposeChildren(): void {
    const children = [...this.#children];
    this.#children.clear();
    for (const child of children) {
      try {
        child.dispose();
      } catch (error) {
        reportEffectError(error);
      }
    }
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.disposeChildren();
  }
}

type EffectCleanup = void | (() => void);

class EffectNode {
  #sources = new Map<Source, number>();
  readonly #fn: () => EffectCleanup;
  readonly #owner: Owner;
  readonly #parent: Owner | null;
  #cleanup: (() => void) | null = null;
  #stale = false;
  #disposed = false;

  constructor(fn: () => EffectCleanup, parent: Owner | null) {
    this.#fn = fn;
    this.#owner = new Owner();
    this.#parent = parent;
    parent?.adopt(this);
  }

  markStale(): void {
    if (this.#stale || this.#disposed) return;
    this.#stale = true;
    pendingEffects.push(this);
  }

  /**
   * Forget a pending run without running. Stale computed sources are brought
   * up to date, so they propagate the next change again. A later change to a
   * source schedules the effect again.
   */
  cancelPending(): void {
    this.#stale = false;
    for (const source of this.#sources.keys()) {
      if (!(source instanceof ComputedNode)) continue;
      try {
        source.refresh();
      } catch (error) {
        reportEffectError(error);
      }
    }
  }

  /** Run if a source changed since the previous run. */
  runIfStale(): void {
    if (!this.#stale || this.#disposed) return;
    this.#stale = false;
    if (sourcesChanged(this.#sources)) this.run();
  }

  run(): void {
    if (this.#disposed) return;
    this.#stale = false;
    this.#teardown();
    const previousSources = this.#sources;
    // A failed run keeps the sources it read before failing, so a later
    // change to one of them retries the effect.
    const sources = new Map<Source, number>();
    const previousOwner = activeOwner;
    const versionBeforeRun = globalVersion;
    activeOwner = this.#owner;
    try {
      const cleanup = collectSources(this, sources, this.#fn);
      if (typeof cleanup === 'function') {
        // A run that disposed its own effect already missed the teardown.
        if (this.#disposed) untrack(cleanup);
        else this.#cleanup = cleanup;
      }
    } finally {
      activeOwner = previousOwner;
      // The run can dispose its own effect, for example a subscriber that
      // unsubscribes. A disposed effect must not register with its sources.
      if (!this.#disposed) {
        this.#sources = sources;
        relink(this, previousSources, sources, true);
        // A source that was not linked yet can have changed during this run,
        // for example a signal that the run itself wrote. Run again then.
        if (globalVersion !== versionBeforeRun && sourcesChanged(sources)) this.markStale();
      }
    }
  }

  #teardown(): void {
    this.#owner.disposeChildren();
    const cleanup = this.#cleanup;
    this.#cleanup = null;
    if (cleanup) untrack(cleanup);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#parent?.release(this);
    try {
      this.#teardown();
    } finally {
      // A failing cleanup must not leave the effect linked or its children live.
      this.#owner.dispose();
      for (const source of this.#sources.keys()) source.removeObserver(this);
      this.#sources.clear();
    }
  }
}

function endBatch(): void {
  batchDepth--;
  if (batchDepth > 0 || flushing) return;
  flushEffects();
}

/**
 * Report an effect failure. An effect error is a defect of the effect's
 * owner, not of the code that wrote a signal, so it does not reach the writer:
 * a realtime reducer must not fail because an unrelated effect failed.
 */
function reportEffectError(error: unknown): void {
  console.error('[chatto-client] an effect failed', error);
}

function flushEffects(): void {
  flushing = true;
  let runs = 0;
  try {
    while (pendingEffects.length > 0) {
      const effects = pendingEffects;
      pendingEffects = [];
      for (const [index, effect] of effects.entries()) {
        if (++runs > MAX_EFFECT_RUNS_PER_FLUSH) {
          // Drop the loop. Later changes to their sources schedule the
          // dropped effects again; unrelated writes do not restart the loop.
          const dropped = [...effects.slice(index), ...pendingEffects];
          pendingEffects = [];
          for (const effect of dropped) effect.cancelPending();
          reportEffectError(new Error('Effect update loop exceeded the maximum number of runs'));
          return;
        }
        try {
          effect.runIfStale();
        } catch (error) {
          reportEffectError(error);
        }
      }
    }
  } finally {
    flushing = false;
  }
}

/** A writable reactive value. */
export type Signal<T> = SignalNode<T>;

/** A cached value derived from other reactive values. */
export type Computed<T> = ComputedNode<T>;

/** Create a writable reactive value. */
export function signal<T>(value: T, options?: ReactiveOptions<T>): Signal<T> {
  return new SignalNode(value, options);
}

/**
 * Create a lazily evaluated, cached value. The function must not write
 * signals. A thrown error is cached and rethrown on read until a source changes.
 */
export function computed<T>(fn: () => T, options?: ReactiveOptions<T>): Computed<T> {
  return new ComputedNode(fn, options);
}

/**
 * Run `fn` now and again after any value it read changes. `fn` may return a
 * cleanup function, which runs before the next run and on disposal. Effects
 * created inside `fn` are disposed before the next run. An effect created in
 * another effect or {@link effectRoot} is disposed with its owner.
 *
 * Returns a function that disposes the effect.
 */
export function effect(fn: () => EffectCleanup): () => void {
  const node = new EffectNode(fn, activeOwner);
  try {
    node.run();
    // The first run can have changed its own sources; run it again now.
    if (batchDepth === 0 && !flushing && pendingEffects.length > 0) flushEffects();
  } catch (error) {
    // The caller receives no dispose function, so do not leave a live effect.
    node.dispose();
    throw error;
  }
  return () => node.dispose();
}

/**
 * Create an ownership scope that is not disposed with the current effect.
 * Effects created while `fn` runs belong to the scope. Returns a function that
 * disposes every effect in the scope.
 */
export function effectRoot(fn: () => void): () => void {
  const owner = new Owner();
  const previousOwner = activeOwner;
  activeOwner = owner;
  try {
    untrack(fn);
  } catch (error) {
    owner.dispose();
    throw error;
  } finally {
    activeOwner = previousOwner;
  }
  return () => owner.dispose();
}

/**
 * Apply several writes before effects run. Nested batches flush when the
 * outermost batch ends. Returns the function's result.
 */
export function batch<T>(fn: () => T): T {
  batchDepth++;
  try {
    return fn();
  } finally {
    endBatch();
  }
}

/** Read reactive values without tracking them in the active observer or adapter. */
export function untrack<T>(fn: () => T): T {
  untrackDepth++;
  try {
    return fn();
  } finally {
    untrackDepth--;
  }
}

/**
 * Call `listener` after the value of `source` changes. The listener is not
 * called for the current value. A computed that fails counts as changed, so
 * the listener can read the error. Returns a function that stops the subscription.
 */
export function subscribe(source: ReactiveSource, listener: () => void): () => void {
  let initialized = false;
  const previousOwner = activeOwner;
  activeOwner = null;
  try {
    return effect(() => {
      try {
        source.get();
      } catch {
        // The subscriber reads the error itself; the subscription stays active.
      }
      if (!initialized) {
        initialized = true;
        return;
      }
      untrack(listener);
    });
  } finally {
    activeOwner = previousOwner;
  }
}
