import { afterEach, describe, expect, it, vi } from 'vitest';

const workers = vi.hoisted(
  () =>
    [] as {
      onmessage?: (event: { data: unknown }) => void;
      terminate: ReturnType<typeof vi.fn>;
      postMessage: ReturnType<typeof vi.fn>;
    }[]
);
vi.mock('./renderer.worker?worker', () => ({
  default: class {
    terminate = vi.fn();
    postMessage = vi.fn();
    constructor() {
      workers.push(this);
    }
  }
}));
import { requestMermaid } from './queue';

afterEach(() => {
  workers.length = 0;
  vi.useRealTimers();
});

describe('Mermaid worker queue', () => {
  it('serialises jobs and terminates each completed worker', async () => {
    const controller = new AbortController();
    const first = requestMermaid('first', controller.signal);
    const second = requestMermaid('second', controller.signal);
    expect(workers).toHaveLength(1);
    workers[0].onmessage?.({ data: '<svg/>' });
    expect(workers[0].terminate).toHaveBeenCalledOnce();
    expect(workers).toHaveLength(2);
    workers[1].onmessage?.({ data: null });
    expect(await first).toBe('<svg/>');
    expect(await second).toBeNull();
  });

  it('removes cancelled queued work and terminates active work', async () => {
    const active = new AbortController();
    const queued = new AbortController();
    const first = requestMermaid('first', active.signal);
    const second = requestMermaid('second', queued.signal);
    queued.abort();
    active.abort();
    expect(await first).toBeNull();
    expect(await second).toBeNull();
    expect(workers).toHaveLength(1);
    expect(workers[0].terminate).toHaveBeenCalledOnce();
  });

  it('times out stalled work and continues with the next diagram', async () => {
    vi.useFakeTimers();
    const controller = new AbortController();
    const stalled = requestMermaid('stalled', controller.signal);
    const next = requestMermaid('next', controller.signal);
    vi.advanceTimersByTime(3_000);
    expect(await stalled).toBeNull();
    expect(workers[0].terminate).toHaveBeenCalledOnce();
    expect(workers).toHaveLength(2);
    workers[1].onmessage?.({ data: '<svg/>' });
    expect(await next).toBe('<svg/>');
  });
});
