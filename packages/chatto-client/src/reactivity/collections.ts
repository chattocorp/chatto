/**
 * Reactive `Map` and `Set` replacements with per-key tracking.
 *
 * Reading one key tracks only that key while the key exists. Reading a
 * missing key, the size, or an iterator tracks the collection structure.
 * Structural changes (insertions, deletions, and clears) and value
 * replacements notify structure readers. Key readers are notified when their
 * key changes or disappears.
 *
 * Use them only for state that UI or computed values read. Local scratch
 * collections should stay plain `Map` and `Set` instances.
 */

import { batch, isObservingReads, signal, type Signal } from './core.js';

function versionSignal(): Signal<number> {
  return signal(0, { equals: false });
}

function bump(version: Signal<number>): void {
  version.set(version.peek() + 1);
}

/** Notify several versions as one write, so effects run once. */
function bumpAll(versions: Iterable<Signal<number> | undefined>): void {
  batch(() => {
    for (const version of versions) if (version) bump(version);
  });
}

/** A `Map` whose reads are tracked by computeds, effects, and UI adapters. */
export class ReactiveMap<K, V> extends Map<K, V> {
  /** Per-key versions, created lazily for keys that were read. */
  #keys: Map<K, Signal<number>> | undefined;
  /** Structure version for size, iteration, and missing-key reads. */
  #structure: Signal<number> | undefined;

  constructor(entries?: Iterable<readonly [K, V]> | null) {
    super();
    if (entries) for (const [key, value] of entries) super.set(key, value);
  }

  #structureVersion(): Signal<number> {
    return (this.#structure ??= versionSignal());
  }

  #trackKey(key: K): void {
    if (!super.has(key)) {
      this.#structureVersion().get();
      return;
    }
    let version = this.#keys?.get(key);
    if (!version) {
      // Unobserved reads, such as plain store code in a bot, need no signal.
      if (!isObservingReads()) return;
      version = versionSignal();
      (this.#keys ??= new Map()).set(key, version);
    }
    version.get();
  }

  override get(key: K): V | undefined {
    this.#trackKey(key);
    return super.get(key);
  }

  override has(key: K): boolean {
    this.#trackKey(key);
    return super.has(key);
  }

  override set(key: K, value: V): this {
    const existed = super.has(key);
    const previous = super.get(key);
    super.set(key, value);
    if (!existed) {
      if (this.#structure) bump(this.#structure);
    } else if (!Object.is(previous, value)) {
      bumpAll([this.#keys?.get(key), this.#structure]);
    }
    return this;
  }

  override delete(key: K): boolean {
    if (!super.delete(key)) return false;
    const version = this.#keys?.get(key);
    this.#keys?.delete(key);
    bumpAll([version, this.#structure]);
    return true;
  }

  override clear(): void {
    if (super.size === 0) return;
    super.clear();
    const keys = this.#keys;
    this.#keys = undefined;
    bumpAll([...(keys?.values() ?? []), this.#structure]);
  }

  override get size(): number {
    this.#structureVersion().get();
    return super.size;
  }

  override forEach(callback: (value: V, key: K, map: Map<K, V>) => void, thisArg?: unknown): void {
    this.#structureVersion().get();
    super.forEach(callback, thisArg);
  }

  override keys(): MapIterator<K> {
    this.#structureVersion().get();
    return super.keys();
  }

  override values(): MapIterator<V> {
    this.#structureVersion().get();
    return super.values();
  }

  override entries(): MapIterator<[K, V]> {
    this.#structureVersion().get();
    return super.entries();
  }

  override [Symbol.iterator](): MapIterator<[K, V]> {
    return this.entries();
  }
}

/** A `Set` whose reads are tracked by computeds, effects, and UI adapters. */
export class ReactiveSet<T> extends Set<T> {
  #values: Map<T, Signal<number>> | undefined;
  #structure: Signal<number> | undefined;

  constructor(values?: Iterable<T> | null) {
    super();
    if (values) for (const value of values) super.add(value);
  }

  #structureVersion(): Signal<number> {
    return (this.#structure ??= versionSignal());
  }

  override has(value: T): boolean {
    if (!super.has(value)) {
      this.#structureVersion().get();
      return false;
    }
    let version = this.#values?.get(value);
    if (!version) {
      if (!isObservingReads()) return true;
      version = versionSignal();
      (this.#values ??= new Map()).set(value, version);
    }
    version.get();
    return true;
  }

  override add(value: T): this {
    if (super.has(value)) return this;
    super.add(value);
    if (this.#structure) bump(this.#structure);
    return this;
  }

  override delete(value: T): boolean {
    if (!super.delete(value)) return false;
    const version = this.#values?.get(value);
    this.#values?.delete(value);
    bumpAll([version, this.#structure]);
    return true;
  }

  override clear(): void {
    if (super.size === 0) return;
    super.clear();
    const values = this.#values;
    this.#values = undefined;
    bumpAll([...(values?.values() ?? []), this.#structure]);
  }

  override get size(): number {
    this.#structureVersion().get();
    return super.size;
  }

  override forEach(callback: (value: T, key: T, set: Set<T>) => void, thisArg?: unknown): void {
    this.#structureVersion().get();
    super.forEach(callback, thisArg);
  }

  override keys(): SetIterator<T> {
    this.#structureVersion().get();
    return super.keys();
  }

  override values(): SetIterator<T> {
    this.#structureVersion().get();
    return super.values();
  }

  override entries(): SetIterator<[T, T]> {
    this.#structureVersion().get();
    return super.entries();
  }

  override [Symbol.iterator](): SetIterator<T> {
    return this.values();
  }
}
