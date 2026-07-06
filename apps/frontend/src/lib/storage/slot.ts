/**
 * Typed `localStorage` accessors.
 *
 * `StorageSlot<T>` wraps a single localStorage entry: serialize/parse via a
 * `Codec<T>`, fall back to a typed default on missing/corrupt data, and
 * swallow SSR (`localStorage === undefined`) and quota errors at the edge.
 *
 * Build items through the factories, which encode Chatto's namespacing:
 *
 * - `globalSlot('foo', …)` → `chatto:foo`
 * - `serverSlot(serverId, 'foo', …)` → `chatto:i:{serverId}:foo`
 *
 * Use `globalSlot` for things that are meaningful regardless of which server
 * the user is looking at (sidebar widths, UI preferences, the server registry
 * itself). Use `serverSlot` for things scoped to a specific connected server
 * (last visited room, recent emojis on that server, member-list collapse state).
 */

import { serverStorageKey } from './serverStorage';

/** Serialize/parse pair for a typed localStorage value. */
export interface Codec<T> {
  serialize(value: T): string;
  /**
   * Return `undefined` when the stored payload is missing or invalid; the
   * caller falls back to the slot's default.
   */
  parse(raw: string): T | undefined;
}

/** Built-in codecs for the common shapes. */
export const Codecs = {
  string: {
    serialize: (v: string) => v,
    parse: (raw: string) => raw
  } satisfies Codec<string>,

  /**
   * Number codec with optional clamping. Out-of-range or non-finite stored
   * values parse as `undefined` so the consumer falls back to the default.
   */
  number(opts: { min?: number; max?: number } = {}): Codec<number> {
    return {
      serialize: (v) => String(v),
      parse: (raw) => {
        const n = parseFloat(raw);
        if (!Number.isFinite(n)) return undefined;
        if (opts.min !== undefined && n < opts.min) return undefined;
        if (opts.max !== undefined && n > opts.max) return undefined;
        return n;
      }
    };
  },

  boolean: {
    serialize: (v: boolean) => (v ? '1' : '0'),
    parse: (raw: string) =>
      raw === '1' || raw === 'true' ? true : raw === '0' || raw === 'false' ? false : undefined
  } satisfies Codec<boolean>,

  /**
   * JSON codec with an optional runtime validator. Parse failures and
   * validator rejections both return `undefined` so corrupt data falls back
   * to the default cleanly.
   */
  json<T>(validate?: (value: unknown) => value is T): Codec<T> {
    return {
      serialize: (v) => JSON.stringify(v),
      parse: (raw) => {
        try {
          const parsed: unknown = JSON.parse(raw);
          if (validate && !validate(parsed)) return undefined;
          return parsed as T;
        } catch {
          return undefined;
        }
      }
    };
  }
};

/** A single localStorage entry with a typed default and corruption-safe IO. */
export class StorageSlot<T> {
  constructor(
    public readonly key: string,
    public readonly defaultValue: T,
    private readonly codec: Codec<T>
  ) {}

  /** Read the stored value, or the default if missing / invalid / unavailable. */
  get(): T {
    if (typeof localStorage === 'undefined') return this.defaultValue;
    try {
      const raw = localStorage.getItem(this.key);
      if (raw === null) return this.defaultValue;
      const parsed = this.codec.parse(raw);
      return parsed !== undefined ? parsed : this.defaultValue;
    } catch {
      return this.defaultValue;
    }
  }

  /** Persist a value. Silently no-ops if storage is unavailable / full. */
  set(value: T): void {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(this.key, this.codec.serialize(value));
    } catch {
      // Ignore quota / privacy-mode failures.
    }
  }

  remove(): void {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.removeItem(this.key);
    } catch {
      // Ignore.
    }
  }
}

/** Build a globally-scoped `StorageSlot<T>` at `chatto:{suffix}`. */
export function globalSlot<T>(
  suffix: string,
  defaultValue: T,
  codec: Codec<T>
): StorageSlot<T> {
  return new StorageSlot(`chatto:${suffix}`, defaultValue, codec);
}

/** Build a per-server `StorageSlot<T>` at `chatto:i:{serverId}:{suffix}`. */
export function serverSlot<T>(
  serverId: string,
  suffix: string,
  defaultValue: T,
  codec: Codec<T>
): StorageSlot<T> {
  return new StorageSlot(serverStorageKey(serverId, suffix), defaultValue, codec);
}
