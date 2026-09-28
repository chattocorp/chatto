/**
 * Vitest setup for the package tests.
 *
 * happy-dom does not implement the Web Locks API. The client uses it to
 * serialize session renewal across browser tabs, so tests get a small
 * in-process implementation that serializes callbacks per lock name.
 */

if (typeof navigator !== 'undefined' && !navigator.locks) {
  const tails = new Map<string, Promise<unknown>>();
  const locks = {
    request<T>(name: string, callback: () => Promise<T> | T): Promise<T> {
      const previous = tails.get(name) ?? Promise.resolve();
      const result = previous.then(
        () => callback(),
        () => callback()
      );
      const tail = result.catch(() => undefined);
      tails.set(name, tail);
      void tail.then(() => {
        if (tails.get(name) === tail) tails.delete(name);
      });
      return result;
    }
  };
  Object.defineProperty(navigator, 'locks', { value: locks, configurable: true });
}

/**
 * happy-dom stores properties that tests set on its `Storage` proxy as items,
 * and its methods bypass `Storage.prototype`. Tests spy on the prototype to
 * simulate quota errors, so install a plain in-memory implementation.
 */
class MemoryStorage implements Storage {
  #items = new Map<string, string>();

  get length(): number {
    return this.#items.size;
  }

  clear(): void {
    this.#items.clear();
  }

  getItem(key: string): string | null {
    return this.#items.get(String(key)) ?? null;
  }

  key(index: number): string | null {
    return [...this.#items.keys()][index] ?? null;
  }

  removeItem(key: string): void {
    this.#items.delete(String(key));
  }

  setItem(key: string, value: string): void {
    this.#items.set(String(key), String(value));
  }
}

if (typeof window !== 'undefined') {
  for (const name of ['localStorage', 'sessionStorage'] as const) {
    Object.defineProperty(window, name, { value: new MemoryStorage(), configurable: true });
  }
  Object.defineProperty(window, 'Storage', { value: MemoryStorage, configurable: true });
  Object.defineProperty(globalThis, 'Storage', { value: MemoryStorage, configurable: true });
}
