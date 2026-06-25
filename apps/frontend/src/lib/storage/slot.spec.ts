import { describe, it, expect, beforeEach, vi } from 'vitest';
import { Codecs, StorageSlot, globalSlot, serverSlot } from './slot';

const storage = new Map<string, string>();
const localStorageMock: Storage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, value),
  removeItem: (key) => storage.delete(key),
  clear: () => storage.clear(),
  get length() {
    return storage.size;
  },
  key: (index) => [...storage.keys()][index] ?? null
};
vi.stubGlobal('localStorage', localStorageMock);

beforeEach(() => {
  storage.clear();
});

describe('Codecs.number', () => {
  it('round-trips finite numbers', () => {
    const c = Codecs.number();
    expect(c.parse(c.serialize(3.14))).toBe(3.14);
  });

  it('rejects NaN and infinity', () => {
    const c = Codecs.number();
    expect(c.parse('not-a-number')).toBeUndefined();
    expect(c.parse('Infinity')).toBeUndefined();
  });

  it('rejects out-of-range values when bounds are set', () => {
    const c = Codecs.number({ min: 10, max: 100 });
    expect(c.parse('5')).toBeUndefined();
    expect(c.parse('150')).toBeUndefined();
    expect(c.parse('50')).toBe(50);
  });
});

describe('Codecs.boolean', () => {
  it('round-trips both states', () => {
    expect(Codecs.boolean.parse(Codecs.boolean.serialize(true))).toBe(true);
    expect(Codecs.boolean.parse(Codecs.boolean.serialize(false))).toBe(false);
  });

  it('accepts legacy "true"/"false" payloads', () => {
    expect(Codecs.boolean.parse('true')).toBe(true);
    expect(Codecs.boolean.parse('false')).toBe(false);
  });

  it('rejects unknown payloads', () => {
    expect(Codecs.boolean.parse('yes')).toBeUndefined();
  });
});

describe('Codecs.json', () => {
  it('round-trips objects', () => {
    const c = Codecs.json<{ a: number }>();
    expect(c.parse(c.serialize({ a: 1 }))).toEqual({ a: 1 });
  });

  it('returns undefined for corrupt JSON', () => {
    const c = Codecs.json();
    expect(c.parse('{not-json')).toBeUndefined();
  });

  it('runs the optional validator', () => {
    const c = Codecs.json<string[]>((v): v is string[] =>
      Array.isArray(v) && v.every((x) => typeof x === 'string')
    );
    expect(c.parse('["a","b"]')).toEqual(['a', 'b']);
    expect(c.parse('[1,2]')).toBeUndefined();
    expect(c.parse('"not-array"')).toBeUndefined();
  });
});

describe('StorageSlot', () => {
  it('returns the default when the key is missing', () => {
    const slot = new StorageSlot('test', 42, Codecs.number());
    expect(slot.get()).toBe(42);
  });

  it('returns the default when the stored value is corrupt', () => {
    storage.set('test', 'not-a-number');
    const slot = new StorageSlot('test', 42, Codecs.number());
    expect(slot.get()).toBe(42);
  });

  it('returns the default when the stored value is out of range', () => {
    storage.set('test', '999');
    const slot = new StorageSlot('test', 42, Codecs.number({ min: 0, max: 100 }));
    expect(slot.get()).toBe(42);
  });

  it('reads and writes round-trip', () => {
    const slot = new StorageSlot('test', 'fallback', Codecs.string);
    slot.set('hello');
    expect(slot.get()).toBe('hello');
    expect(storage.get('test')).toBe('hello');
  });

  it('removes the underlying key', () => {
    const slot = new StorageSlot('test', 'fallback', Codecs.string);
    slot.set('hello');
    slot.remove();
    expect(storage.has('test')).toBe(false);
    expect(slot.get()).toBe('fallback');
  });
});

describe('namespacing factories', () => {
  it('globalSlot prefixes with "chatto:"', () => {
    const slot = globalSlot('foo', 0, Codecs.number());
    expect(slot.key).toBe('chatto:foo');
  });

  it('serverSlot prefixes with "chatto:i:{serverId}:"', () => {
    const slot = serverSlot('chat-example-com', 'lastRoom', '', Codecs.string);
    expect(slot.key).toBe('chatto:i:chat-example-com:lastRoom');
  });
});
