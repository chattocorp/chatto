import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const workers = vi.hoisted(
  () =>
    [] as {
      onmessage?: (event: { data: unknown }) => void;
      onerror?: () => void;
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
let requestMermaid: typeof import('./queue').requestMermaid;

beforeEach(async () => {
  vi.resetModules();
  ({ requestMermaid } = await import('./queue'));
});

afterEach(() => {
  workers.length = 0;
  vi.useRealTimers();
});

describe('Mermaid worker queue', () => {
  it('serialises jobs and reuses the ready worker even after a parser failure', async () => {
    const controller = new AbortController();
    const first = requestMermaid('first', controller.signal);
    const second = requestMermaid('second', controller.signal);
    expect(workers).toHaveLength(1);
    expect(workers[0].postMessage).not.toHaveBeenCalled();
    workers[0].onmessage?.({ data: { type: 'ready' } });
    expect(workers[0].postMessage).toHaveBeenLastCalledWith('first');
    workers[0].onmessage?.({ data: '<svg/>' });
    expect(workers[0].terminate).not.toHaveBeenCalled();
    expect(workers).toHaveLength(1);
    expect(workers[0].postMessage).toHaveBeenLastCalledWith('second');
    workers[0].onmessage?.({ data: null });
    expect(await first).toBe('<svg/>');
    expect(await second).toBeNull();
    expect(workers[0].onmessage).toBeNull();
    const third = requestMermaid('third', controller.signal);
    expect(workers[0].postMessage).toHaveBeenLastCalledWith('third');
    workers[0].onmessage?.({ data: '<svg/>' });
    expect(await third).toBe('<svg/>');
    expect(workers).toHaveLength(1);
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
    workers[0].onmessage?.({ data: { type: 'ready' } });
    vi.advanceTimersByTime(3_000);
    expect(await stalled).toBeNull();
    expect(workers[0].terminate).toHaveBeenCalledOnce();
    expect(workers).toHaveLength(2);
    workers[1].onmessage?.({ data: { type: 'ready' } });
    workers[1].onmessage?.({ data: '<svg/>' });
    expect(await next).toBe('<svg/>');
  });

  it('cancels queued work without interrupting the active renderer', async () => {
    const first = requestMermaid('first', new AbortController().signal);
    workers[0].onmessage?.({ data: { type: 'ready' } });
    const controller = new AbortController();
    const queued = requestMermaid('queued', controller.signal);
    controller.abort();
    expect(await queued).toBeNull();
    expect(workers[0].terminate).not.toHaveBeenCalled();
    expect(workers[0].postMessage).toHaveBeenCalledTimes(1);
    workers[0].onmessage?.({ data: '<svg/>' });
    expect(await first).toBe('<svg/>');
  });

  it('does not spend the render budget downloading and starting the worker', async () => {
    vi.useFakeTimers();
    const job = requestMermaid('slow download', new AbortController().signal);
    vi.advanceTimersByTime(10_000);
    expect(workers[0].terminate).not.toHaveBeenCalled();
    expect(workers[0].postMessage).not.toHaveBeenCalled();
    workers[0].onmessage?.({ data: { type: 'ready' } });
    vi.advanceTimersByTime(2_999);
    workers[0].onmessage?.({ data: '<svg/>' });
    expect(await job).toBe('<svg/>');
  });

  it('bounds failed startup and retries queued work with a fresh worker', async () => {
    vi.useFakeTimers();
    const first = requestMermaid('first', new AbortController().signal);
    const second = requestMermaid('second', new AbortController().signal);
    vi.advanceTimersByTime(30_000);
    expect(await first).toBeNull();
    expect(workers[0].terminate).toHaveBeenCalledOnce();
    workers[1].onmessage?.({ data: { type: 'ready' } });
    workers[1].onmessage?.({ data: '<svg/>' });
    expect(await second).toBe('<svg/>');
  });

  it('replaces a cancelled active worker and ignores its late result', async () => {
    const controller = new AbortController();
    const first = requestMermaid('first', controller.signal);
    workers[0].onmessage?.({ data: { type: 'ready' } });
    const stale = workers[0].onmessage;
    const second = requestMermaid('second', new AbortController().signal);
    controller.abort();
    stale?.({ data: '<stale/>' });
    expect(await first).toBeNull();
    expect(workers[0].terminate).toHaveBeenCalledOnce();
    workers[1].onmessage?.({ data: { type: 'ready' } });
    workers[1].onmessage?.({ data: '<fresh/>' });
    expect(await second).toBe('<fresh/>');
  });

  it('replaces a worker that fails while loading', async () => {
    const first = requestMermaid('first', new AbortController().signal);
    const second = requestMermaid('second', new AbortController().signal);
    workers[0].onerror?.();
    expect(await first).toBeNull();
    expect(workers[0].terminate).toHaveBeenCalledOnce();
    workers[1].onmessage?.({ data: { type: 'ready' } });
    workers[1].onmessage?.({ data: '<svg/>' });
    expect(await second).toBe('<svg/>');
  });
});
