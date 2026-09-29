import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  batch,
  computed,
  effect,
  effectRoot,
  setReadHook,
  signal,
  subscribe,
  untrack,
  type ReactiveSource
} from './core.js';

afterEach(() => setReadHook(null));

describe('signal', () => {
  it('reads, writes, and skips equal writes', () => {
    const count = signal(1);
    const runs = vi.fn();
    effect(() => runs(count.get()));
    count.set(1);
    count.set(2);
    expect(runs.mock.calls).toEqual([[1], [2]]);
  });

  it('notifies every write when equality is disabled', () => {
    const items: string[] = [];
    const list = signal(items, { equals: false });
    const runs = vi.fn();
    effect(() => runs(list.get().length));
    items.push('a');
    list.set(items);
    expect(runs.mock.calls).toEqual([[0], [1]]);
  });

  it('notifies after in-place mutation', () => {
    const value = { count: 0 };
    const state = signal(value);
    const runs = vi.fn();
    effect(() => runs(state.get().count));
    value.count = 1;
    state.notify();
    expect(runs.mock.calls).toEqual([[0], [1]]);
  });
});

describe('computed', () => {
  it('evaluates lazily and caches', () => {
    const count = signal(1);
    const fn = vi.fn(() => count.get() * 2);
    const double = computed(fn);
    expect(fn).not.toHaveBeenCalled();
    expect(double.get()).toBe(2);
    expect(double.get()).toBe(2);
    expect(fn).toHaveBeenCalledTimes(1);
    count.set(2);
    expect(double.get()).toBe(4);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('does not rerun effects when the computed value is unchanged', () => {
    const count = signal(1);
    const parity = computed(() => count.get() % 2);
    const runs = vi.fn();
    effect(() => runs(parity.get()));
    count.set(3);
    count.set(4);
    expect(runs.mock.calls).toEqual([[1], [0]]);
  });

  it('updates diamond dependencies once and without glitches', () => {
    const base = signal(1);
    const left = computed(() => base.get() + 1);
    const right = computed(() => base.get() * 10);
    const seen: string[] = [];
    effect(() => {
      seen.push(`${left.get()}:${right.get()}`);
    });
    base.set(2);
    expect(seen).toEqual(['2:10', '3:20']);
  });

  it('switches dynamic dependencies', () => {
    const useLeft = signal(true);
    const left = signal('a');
    const right = signal('b');
    const fn = vi.fn(() => (useLeft.get() ? left.get() : right.get()));
    const value = computed(fn);
    const runs = vi.fn();
    effect(() => runs(value.get()));
    useLeft.set(false);
    left.set('c');
    right.set('d');
    expect(runs.mock.calls).toEqual([['a'], ['b'], ['d']]);
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it('rechecks unobserved computeds against source versions', () => {
    const count = signal(1);
    const fn = vi.fn(() => count.get() + 1);
    const next = computed(fn);
    expect(next.get()).toBe(2);
    count.set(5);
    expect(next.get()).toBe(6);
    const other = signal(0);
    other.set(1);
    expect(next.get()).toBe(6);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('stays correct after losing and regaining observers', () => {
    const count = signal(1);
    const double = computed(() => count.get() * 2);
    const stop = effect(() => {
      double.get();
    });
    stop();
    count.set(2);
    const runs = vi.fn();
    effect(() => runs(double.get()));
    count.set(3);
    expect(runs.mock.calls).toEqual([[4], [6]]);
  });

  it('caches errors and recovers after a source change', () => {
    const fail = signal(true);
    const value = computed(() => {
      if (fail.get()) throw new Error('failed');
      return 'ok';
    });
    expect(() => value.get()).toThrow('failed');
    fail.set(false);
    expect(value.get()).toBe('ok');
  });

  it('detects cycles', () => {
    const cycle: { value?: ReactiveSource<number> } = {};
    cycle.value = computed(() => cycle.value!.get() + 1);
    expect(() => cycle.value!.get()).toThrow('Cycle detected');
  });
});

describe('effect', () => {
  it('runs cleanup before the next run and on disposal', () => {
    const count = signal(0);
    const log: string[] = [];
    const stop = effect(() => {
      const value = count.get();
      log.push(`run ${value}`);
      return () => log.push(`cleanup ${value}`);
    });
    count.set(1);
    stop();
    count.set(2);
    expect(log).toEqual(['run 0', 'cleanup 0', 'run 1', 'cleanup 1']);
  });

  it('disposes nested effects before rerunning the parent', () => {
    const outer = signal(0);
    const inner = signal(0);
    const innerRuns = vi.fn();
    effect(() => {
      outer.get();
      effect(() => innerRuns(inner.get()));
    });
    outer.set(1);
    inner.set(1);
    expect(innerRuns.mock.calls).toEqual([[0], [0], [1]]);
  });

  it('batches writes', () => {
    const a = signal(0);
    const b = signal(0);
    const runs = vi.fn();
    effect(() => runs(a.get() + b.get()));
    batch(() => {
      a.set(1);
      b.set(1);
    });
    expect(runs.mock.calls).toEqual([[0], [2]]);
  });

  it('runs effects that are scheduled by other effects in the same flush', () => {
    const source = signal(0);
    const mirror = signal(0);
    effect(() => mirror.set(source.get()));
    const runs = vi.fn();
    effect(() => runs(mirror.get()));
    source.set(3);
    expect(runs.mock.calls).toEqual([[0], [3]]);
  });

  it('does not track untracked reads', () => {
    const tracked = signal(0);
    const ignored = signal(0);
    const runs = vi.fn();
    effect(() => runs(tracked.get() + untrack(() => ignored.get())));
    ignored.set(1);
    tracked.set(1);
    expect(runs.mock.calls).toEqual([[0], [2]]);
  });

  it('logs effect errors without failing the writer or other effects', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const count = signal(0);
    const other = vi.fn();
    effect(() => {
      if (count.get() === 1) throw new Error('boom');
    });
    effect(() => other(count.get()));
    expect(() => count.set(1)).not.toThrow();
    expect(other).toHaveBeenLastCalledWith(1);
    expect(logged).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ message: 'boom' })
    );
    logged.mockRestore();
  });

  it('disposes effects created in a root', () => {
    const count = signal(0);
    const runs = vi.fn();
    const dispose = effectRoot(() => {
      effect(() => runs(count.get()));
    });
    count.set(1);
    dispose();
    count.set(2);
    expect(runs.mock.calls).toEqual([[0], [1]]);
  });

  it('keeps roots alive when an enclosing effect reruns', () => {
    const outer = signal(0);
    const inner = signal(0);
    const runs = vi.fn();
    effect(() => {
      outer.get();
      if (outer.peek() === 0) effectRoot(() => effect(() => runs(inner.get())));
    });
    outer.set(1);
    inner.set(1);
    expect(runs.mock.calls).toEqual([[0], [1]]);
  });
});

describe('subscribe', () => {
  it('calls the listener after changes only', () => {
    const count = signal(0);
    const listener = vi.fn();
    const stop = subscribe(count, listener);
    expect(listener).not.toHaveBeenCalled();
    count.set(1);
    stop();
    count.set(2);
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it('keeps subscriptions to failing computeds', () => {
    const fail = signal(false);
    const value = computed(() => {
      if (fail.get()) throw new Error('failed');
      return 1;
    });
    const listener = vi.fn();
    subscribe(value, listener);
    fail.set(true);
    fail.set(false);
    expect(listener).toHaveBeenCalledTimes(2);
  });
});

describe('read hook', () => {
  it('receives reads outside computeds and effects only', () => {
    const hooked: ReactiveSource[] = [];
    setReadHook((source) => hooked.push(source));
    const count = signal(0);
    const double = computed(() => count.get() * 2);
    effect(() => {
      count.get();
    });
    expect(hooked).toEqual([]);
    double.get();
    expect(hooked).toEqual([double]);
    untrack(() => count.get());
    expect(hooked).toEqual([double]);
    count.get();
    expect(hooked).toEqual([double, count]);
  });
});

describe('effect loops', () => {
  it('stops an endless loop and keeps queued effects scheduled', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const count = signal(0);
    const observed = signal(0);
    const runs = vi.fn();
    // Queued on every loop iteration, so it is pending when the loop stops.
    effect(() => runs(count.get(), observed.get()));
    const stopLoop = effect(() => {
      count.set(count.get() + 1);
    });
    expect(logged).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ message: expect.stringContaining('maximum number of runs') })
    );
    stopLoop();
    observed.set(1);
    expect(runs).toHaveBeenLastCalledWith(count.peek(), 1);
    logged.mockRestore();
  });

  it('does not restart a stopped loop on an unrelated write', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const count = signal(0);
    let runs = 0;
    effect(() => {
      runs++;
      count.set(count.get() + 1);
    });
    const afterLoop = runs;
    signal(0).set(1);
    expect(runs).toBe(afterLoop);
    expect(logged).toHaveBeenCalledTimes(1);
    logged.mockRestore();
  });

  it('keeps computeds between a stopped loop and its observers live', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const source = signal(0);
    const doubled = computed(() => source.get() * 2);
    const runs = vi.fn();
    effect(() => runs(doubled.get()));
    const loop = signal(0);
    const stopLoop = effect(() => {
      loop.set(loop.get() + 1);
      source.set(loop.peek());
    });
    stopLoop();
    source.set(-1);
    expect(runs).toHaveBeenLastCalledWith(-2);
    logged.mockRestore();
  });
});

describe('writes during a run', () => {
  it('reruns an effect when its run changed a computed it read', () => {
    const source = signal(1);
    const doubled = computed(() => source.get() * 2);
    const runs = vi.fn();
    let first = true;
    effect(() => {
      runs(doubled.get());
      if (first) {
        first = false;
        source.set(2);
      }
    });
    expect(runs.mock.calls).toEqual([[2], [4]]);
  });
});

describe('effect disposal', () => {
  it('does not re-register an effect that disposed itself while running', () => {
    const source = signal(0);
    const stop: { current?: () => void } = {};
    stop.current = subscribe(source, () => stop.current?.());
    source.set(1);
    expect(source.observers.size).toBe(0);
  });

  it('disposes an effect whose first run throws', () => {
    const source = signal(0);
    expect(() =>
      effect(() => {
        source.get();
        throw new Error('boom');
      })
    ).toThrow('boom');
    expect(source.observers.size).toBe(0);
    expect(() => source.set(1)).not.toThrow();
  });
});

describe('batch errors', () => {
  it('keeps the original error when effects run after a failed batch', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const count = signal(0);
    effect(() => {
      if (count.get() === 1) throw new Error('effect failed');
    });
    expect(() =>
      batch(() => {
        count.set(1);
        throw new Error('batch failed');
      })
    ).toThrow('batch failed');
    logged.mockRestore();
  });
});

describe('disposal', () => {
  it('disposes every effect of a root when one cleanup throws', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const source = signal(0);
    const runs = vi.fn();
    const dispose = effectRoot(() => {
      effect(() => {
        source.get();
        return () => {
          throw new Error('cleanup failed');
        };
      });
      effect(() => runs(source.get()));
    });
    dispose();
    source.set(1);
    expect(runs).toHaveBeenCalledTimes(1);
    expect(logged).toHaveBeenCalled();
    logged.mockRestore();
  });
});

describe('self-disposal', () => {
  it('runs the cleanup of a run that disposed its own effect', () => {
    const cleanup = vi.fn();
    const source = signal(0);
    // The first run does not read `dispose`; later runs do.
    const dispose: () => void = effect(() => {
      if (source.get() === 1) dispose();
      return cleanup;
    });
    source.set(1);
    expect(cleanup).toHaveBeenCalledTimes(2);
    source.set(2);
    expect(cleanup).toHaveBeenCalledTimes(2);
  });
});
