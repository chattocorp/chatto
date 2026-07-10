export type OptimisticMutationToken = number;

/**
 * Tracks pending optimistic mutations by a caller-defined key.
 * Callers keep the domain-specific patch and rollback logic; this registry
 * only answers whether an async completion still belongs to the latest
 * optimistic mutation for that key.
 */
export class OptimisticMutationRegistry {
  private nextToken = 0;
  private tokens = new Map<string, OptimisticMutationToken>();

  createToken(): OptimisticMutationToken {
    this.nextToken += 1;
    return this.nextToken;
  }

  mark(key: string, token: OptimisticMutationToken): void {
    this.tokens.set(key, token);
  }

  isCurrent(key: string, token: OptimisticMutationToken): boolean {
    return this.tokens.get(key) === token;
  }

  clear(key: string): void {
    this.tokens.delete(key);
  }

  clearPrefixes(prefixes: readonly string[]): void {
    for (const key of this.tokens.keys()) {
      if (prefixes.some((prefix) => key.startsWith(prefix))) {
        this.tokens.delete(key);
      }
    }
  }

  clearAll(): void {
    this.tokens.clear();
  }
}
