import { describe, it, expect } from 'vitest';
import { mergeInstanceConversations } from './mergeConversations';

type Conv = { id: string; instanceId: string; tag?: string };

const c = (id: string, instanceId: string, tag?: string): Conv => ({ id, instanceId, tag });

describe('mergeInstanceConversations', () => {
  it('preserves order of conversations from OTHER instances when one instance refetches', () => {
    const existing = [c('a', 'i1'), c('b', 'i2'), c('c', 'i1'), c('d', 'i2')];
    const loaded = [c('a', 'i1', 'fresh'), c('c', 'i1', 'fresh')];

    const merged = mergeInstanceConversations(existing, 'i1', loaded);

    // i2's entries stay in their original relative slots; i1's entries
    // are replaced in place by their fresh versions.
    expect(merged.map((x) => x.id)).toEqual(['a', 'b', 'c', 'd']);
    expect(merged.find((x) => x.id === 'a')?.tag).toBe('fresh');
    expect(merged.find((x) => x.id === 'c')?.tag).toBe('fresh');
    expect(merged.find((x) => x.id === 'b')?.tag).toBeUndefined();
  });

  it('drops conversations that no longer exist server-side for this instance', () => {
    const existing = [c('a', 'i1'), c('b', 'i1'), c('c', 'i1')];
    const loaded = [c('a', 'i1'), c('c', 'i1')];

    const merged = mergeInstanceConversations(existing, 'i1', loaded);

    expect(merged.map((x) => x.id)).toEqual(['a', 'c']);
  });

  it('inserts genuinely new conversations at the front (server returns most-recent-first)', () => {
    const existing = [c('a', 'i1'), c('b', 'i1')];
    const loaded = [c('new', 'i1'), c('a', 'i1'), c('b', 'i1')];

    const merged = mergeInstanceConversations(existing, 'i1', loaded);

    expect(merged.map((x) => x.id)).toEqual(['new', 'a', 'b']);
  });

  it('does not touch other instances even when adding new conversations', () => {
    const existing = [c('x', 'i2'), c('a', 'i1'), c('y', 'i2')];
    const loaded = [c('new', 'i1'), c('a', 'i1')];

    const merged = mergeInstanceConversations(existing, 'i1', loaded);

    // 'new' goes to the front; 'a' stays where it was; i2 entries are
    // untouched and keep their relative order.
    expect(merged.map((x) => x.id)).toEqual(['new', 'x', 'a', 'y']);
  });

  it('handles initial load (empty existing)', () => {
    const merged = mergeInstanceConversations<Conv>(
      [],
      'i1',
      [c('a', 'i1'), c('b', 'i1')]
    );
    expect(merged.map((x) => x.id)).toEqual(['a', 'b']);
  });

  it('handles refetch returning empty (all conversations for the instance dropped)', () => {
    const existing = [c('a', 'i1'), c('x', 'i2')];
    const merged = mergeInstanceConversations(existing, 'i1', []);

    expect(merged.map((x) => x.id)).toEqual(['x']);
  });

  it('preserves order when a bump-then-refetch happens (regression: refetch must not undo the bump)', () => {
    // After a bump, conversation 'b' moved to the top of i1's slot.
    const afterBump = [c('b', 'i1'), c('a', 'i1'), c('c', 'i1')];
    // The server-driven refetch returns its own most-recent-first order
    // (which happens to also have b at the top, since bumping was triggered
    // by a new message in b).
    const loaded = [c('b', 'i1'), c('a', 'i1'), c('c', 'i1')];

    const merged = mergeInstanceConversations(afterBump, 'i1', loaded);

    // The post-bump order is preserved — refetch replaces in place.
    expect(merged.map((x) => x.id)).toEqual(['b', 'a', 'c']);
  });
});
