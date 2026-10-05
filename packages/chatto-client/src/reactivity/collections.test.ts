import { describe, expect, it, vi } from 'vitest';
import { effect } from './core.js';
import { ReactiveMap, ReactiveSet } from './collections.js';

describe('ReactiveMap', () => {
  it('tracks existing keys individually', () => {
    const map = new ReactiveMap([
      ['a', 1],
      ['b', 2]
    ]);
    const runs = vi.fn();
    effect(() => runs(map.get('a')));
    map.set('b', 3);
    map.set('a', 1);
    map.set('a', 4);
    expect(runs.mock.calls).toEqual([[1], [4]]);
  });

  it('tracks missing keys through the structure', () => {
    const map = new ReactiveMap<string, number>();
    const runs = vi.fn();
    effect(() => runs(map.get('a')));
    map.set('a', 1);
    map.delete('a');
    expect(runs.mock.calls).toEqual([[undefined], [1], [undefined]]);
  });

  it('notifies iteration and size readers about structure and values', () => {
    const map = new ReactiveMap([['a', 1]]);
    const sizes = vi.fn();
    const sums = vi.fn();
    effect(() => sizes(map.size));
    effect(() => sums([...map.values()].reduce((total, value) => total + value, 0)));
    map.set('b', 2);
    map.set('a', 5);
    map.clear();
    expect(sizes.mock.calls).toEqual([[1], [2], [2], [0]]);
    expect(sums.mock.calls).toEqual([[1], [3], [7], [0]]);
  });

  it('behaves like a Map', () => {
    const map = new ReactiveMap([['a', 1]]);
    expect(map instanceof Map).toBe(true);
    expect([...map]).toEqual([['a', 1]]);
    expect(new Map(map).get('a')).toBe(1);
  });
});

describe('ReactiveSet', () => {
  it('tracks membership and structure', () => {
    const set = new ReactiveSet(['a']);
    const hasA = vi.fn();
    const hasB = vi.fn();
    const sizes = vi.fn();
    effect(() => hasA(set.has('a')));
    effect(() => hasB(set.has('b')));
    effect(() => sizes(set.size));
    set.add('b');
    set.add('b');
    set.delete('a');
    expect(hasA.mock.calls).toEqual([[true], [false]]);
    expect(hasB.mock.calls).toEqual([[false], [true]]);
    expect(sizes.mock.calls).toEqual([[1], [2], [1]]);
  });
});

describe('unobserved reads', () => {
  it('create no per-key signals outside computeds, effects, and adapters', () => {
    const map = new ReactiveMap([['a', 1]]);
    for (let index = 0; index < 3; index++) map.get('a');
    const runs = vi.fn();
    effect(() => runs(map.get('a')));
    map.set('a', 2);
    expect(runs.mock.calls).toEqual([[1], [2]]);
  });
});
