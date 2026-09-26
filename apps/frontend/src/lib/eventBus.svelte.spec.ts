import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { EventBus, RealtimeProjectionUpdate } from './eventBus.svelte';

describe('EventBus', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleError.mockRestore();
  });

  it('throws when an update arrives before a reducer is registered', () => {
    const bus = new EventBus('server-1');
    const listener = vi.fn();
    bus.subscribe(listener);

    expect(() => bus.publish(new RealtimeProjectionUpdate({ id: 'evt-1' }))).toThrow(
      'projection update received before reducer registration'
    );
    expect(listener).not.toHaveBeenCalled();
  });

  it('logs a listener error and still runs the reducer and later listeners', () => {
    const bus = new EventBus('server-1');
    const calls: string[] = [];
    bus.setReducer(() => calls.push('reducer'));
    bus.subscribe(() => calls.push('first'));
    bus.subscribe(() => {
      calls.push('failing');
      throw new Error('listener boom');
    });
    bus.subscribe(() => calls.push('last'));

    expect(() => bus.publish(new RealtimeProjectionUpdate({ id: 'evt-1' }))).not.toThrow();

    expect(calls).toEqual(['reducer', 'first', 'failing', 'last']);
    expect(consoleError).toHaveBeenCalledWith(
      '[eventBus:server-1] handler threw',
      expect.objectContaining({ message: 'listener boom' })
    );
  });

  it('propagates a reducer error without notifying listeners', () => {
    const bus = new EventBus('server-1');
    const listener = vi.fn();
    bus.setReducer(() => {
      throw new Error('reducer boom');
    });
    bus.subscribe(listener);

    expect(() => bus.publish(new RealtimeProjectionUpdate({ id: 'evt-1' }))).toThrow(
      'reducer boom'
    );
    expect(listener).not.toHaveBeenCalled();
  });

  it('throws a reset reducer error after every listener saw the reset', () => {
    const bus = new EventBus('server-1');
    const listener = vi.fn();
    bus.setReducer(() => {
      throw new Error('reset boom');
    });
    bus.subscribe(listener);
    const reset = new RealtimeProjectionUpdate({ reset: true, privacyReset: true });

    expect(() => bus.publish(reset)).toThrow('reset boom');
    expect(listener).toHaveBeenCalledExactlyOnceWith(reset);
    expect(consoleError).toHaveBeenCalledWith('[eventBus:server-1] reset handler failed');
  });

  it('replaces the previous reducer when a new one is set', () => {
    const bus = new EventBus('server-1');
    const first = vi.fn();
    const second = vi.fn();
    bus.setReducer(first);
    bus.setReducer(second);

    bus.publish(new RealtimeProjectionUpdate({ id: 'evt-1' }));

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledOnce();
  });

  it('clears the reducer only when the given reducer is still installed', () => {
    const bus = new EventBus('server-1');
    const stale = vi.fn();
    const current = vi.fn();
    bus.setReducer(stale);
    bus.setReducer(current);

    bus.clearReducer(stale);
    bus.publish(new RealtimeProjectionUpdate({ id: 'evt-1' }));
    expect(current).toHaveBeenCalledOnce();

    bus.clearReducer(current);
    expect(() => bus.publish(new RealtimeProjectionUpdate({ id: 'evt-2' }))).toThrow(
      'projection update received before reducer registration'
    );
    expect(stale).not.toHaveBeenCalled();
  });

  it('notifies listeners without running the reducer', () => {
    const bus = new EventBus('server-1');
    const reducer = vi.fn();
    const listener = vi.fn();
    bus.setReducer(reducer);
    bus.subscribe(listener);
    const update = new RealtimeProjectionUpdate({ id: 'evt-1' });

    bus.notify(update);

    expect(reducer).not.toHaveBeenCalled();
    expect(listener).toHaveBeenCalledExactlyOnceWith(update);
  });

  it('stops delivering to a listener after it unsubscribes', () => {
    const bus = new EventBus('server-1');
    bus.setReducer(vi.fn());
    const listener = vi.fn();
    const unsubscribe = bus.subscribe(listener);
    expect(bus.listenerCount).toBe(1);

    unsubscribe();
    bus.publish(new RealtimeProjectionUpdate({ id: 'evt-1' }));

    expect(bus.listenerCount).toBe(0);
    expect(listener).not.toHaveBeenCalled();
  });

  it('isolates session-termination listener errors', () => {
    const bus = new EventBus('server-1');
    const later = vi.fn();
    const unsubscribed = vi.fn();
    bus.onSessionTerminated(() => {
      throw new Error('termination boom');
    });
    bus.onSessionTerminated(later);
    bus.onSessionTerminated(unsubscribed)();

    bus.terminateSession('admin_boot');

    expect(later).toHaveBeenCalledExactlyOnceWith('admin_boot');
    expect(unsubscribed).not.toHaveBeenCalled();
    expect(consoleError).toHaveBeenCalledWith(
      '[eventBus:server-1] session termination handler threw',
      expect.any(Error)
    );
  });
});
