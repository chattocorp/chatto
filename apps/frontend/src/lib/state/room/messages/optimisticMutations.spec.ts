import { describe, expect, it } from 'vitest';
import { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';

describe('OptimisticMutationRegistry', () => {
  it('tracks the latest token per key', () => {
    const registry = new OptimisticMutationRegistry();
    const first = registry.createToken();
    const second = registry.createToken();

    registry.mark('event:m1', first);
    expect(registry.isCurrent('event:m1', first)).toBe(true);

    registry.mark('event:m1', second);
    expect(registry.isCurrent('event:m1', first)).toBe(false);
    expect(registry.isCurrent('event:m1', second)).toBe(true);
  });

  it('clears groups of keys by prefix', () => {
    const registry = new OptimisticMutationRegistry();
    const token = registry.createToken();

    registry.mark('events:m1\u0000heart', token);
    registry.mark('events:m1\u0000thumbsup', token);
    registry.mark('events:m2\u0000heart', token);

    registry.clearPrefixes(['events:m1\u0000']);

    expect(registry.isCurrent('events:m1\u0000heart', token)).toBe(false);
    expect(registry.isCurrent('events:m1\u0000thumbsup', token)).toBe(false);
    expect(registry.isCurrent('events:m2\u0000heart', token)).toBe(true);
  });
});
