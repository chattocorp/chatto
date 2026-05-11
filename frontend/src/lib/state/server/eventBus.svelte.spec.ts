import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Client } from '@urql/svelte';
import { eventBusManager } from './eventBus.svelte';

type SubscriptionCallback = (result: { data?: unknown; error?: unknown }) => void;

/** Captures the subscription callback so the test can drive results manually. */
function makeClient(): { client: Client; deliver: (result: { data?: unknown; error?: unknown }) => void } {
  let cb: SubscriptionCallback | null = null;
  const client = {
    subscription: vi.fn().mockReturnValue({
      subscribe: (callback: SubscriptionCallback) => {
        cb = callback;
        return { unsubscribe: vi.fn() };
      }
    }),
    query: vi.fn(),
    mutation: vi.fn()
  } as unknown as Client;

  return {
    client,
    deliver: (result) => {
      if (!cb) throw new Error('No subscriber attached yet');
      cb(result);
    }
  };
}

describe('eventBusManager subscription robustness', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;
  const TEST_INSTANCE = 'test-instance-bus';

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    eventBusManager.stopBus(TEST_INSTANCE);
    consoleError.mockRestore();
  });

  it('logs an error when the subscription delivers result.error', () => {
    const { client, deliver } = makeClient();
    eventBusManager.startBus(TEST_INSTANCE, client);

    deliver({ error: new Error('subscription failed') });

    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain(TEST_INSTANCE);
    expect(consoleError.mock.calls[0][0]).toContain('subscription error');
  });

  it('isolates handler errors so one throwing handler does not stop the others', () => {
    const { client, deliver } = makeClient();
    eventBusManager.startBus(TEST_INSTANCE, client);

    const bus = eventBusManager.getBus(TEST_INSTANCE);
    expect(bus).toBeDefined();

    const ranBefore = vi.fn();
    const ranAfter = vi.fn();
    bus!.handlers.add(ranBefore);
    bus!.handlers.add(() => {
      throw new Error('handler boom');
    });
    bus!.handlers.add(ranAfter);

    const event = { actorId: 'a', event: { __typename: 'ServerUpdatedEvent' } };
    deliver({ data: { myInstanceEvents: event } });

    expect(ranBefore).toHaveBeenCalledTimes(1);
    expect(ranAfter).toHaveBeenCalledTimes(1);
    expect(consoleError).toHaveBeenCalled();
    expect(consoleError.mock.calls[0][0]).toContain('handler threw');
  });

  it('continues delivering events after a handler error on a previous event', () => {
    const { client, deliver } = makeClient();
    eventBusManager.startBus(TEST_INSTANCE, client);

    const bus = eventBusManager.getBus(TEST_INSTANCE)!;
    const handler = vi.fn();
    let throwOnce = true;
    bus.handlers.add(() => {
      if (throwOnce) {
        throwOnce = false;
        throw new Error('handler boom');
      }
    });
    bus.handlers.add(handler);

    const event = { actorId: 'a', event: { __typename: 'ServerUpdatedEvent' } };
    deliver({ data: { myInstanceEvents: event } });
    deliver({ data: { myInstanceEvents: event } });

    expect(handler).toHaveBeenCalledTimes(2);
  });
});
