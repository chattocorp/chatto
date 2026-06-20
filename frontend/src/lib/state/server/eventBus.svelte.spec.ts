import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { makeSubject, type Source, type Subject } from 'wonka';
import type { Client } from '@urql/svelte';
import { createEventBusHandlerRegistrar } from '$lib/eventBus.svelte';
import { eventBusManager } from './eventBus.svelte';
import type { GraphQLClient } from './graphqlClient.svelte';

/**
 * Returns a fake GraphQLClient-shaped object whose `client.subscription()`
 * yields a fresh Wonka subject each time, plus controls to drive it from the
 * test. `reconnectCount` is a Svelte `$state` so the bus's `$effect` reacts
 * to `bumpReconnect()`.
 *
 * The real `OperationResultSource` is a Wonka `Source` with helper methods
 * tacked on — the bus only uses it through `pipe(source, ...)`, so a bare
 * Source is sufficient. The cast launders TS noise.
 */
class FakeGqlClient {
	reconnectCount = $state(0);
	#subjects: Subject<{ data?: unknown; error?: unknown }>[] = [];
	subscribeCalls = 0;
	client: Client;

	constructor() {
		const subscription = vi.fn().mockImplementation(() => {
			this.subscribeCalls++;
			const subj = makeSubject<{ data?: unknown; error?: unknown }>();
			this.#subjects.push(subj);
			return subj.source as unknown as Source<unknown>;
		});
		this.client = {
			subscription,
			query: vi.fn(),
			mutation: vi.fn()
		} as unknown as Client;
	}

	/** The currently-live subject (the one the bus is subscribed to right now). */
	get current(): Subject<{ data?: unknown; error?: unknown }> {
		if (this.#subjects.length === 0) throw new Error('no subscription started yet');
		return this.#subjects[this.#subjects.length - 1];
	}

	bumpReconnect() {
		this.reconnectCount++;
		flushSync();
	}

	get subscriptionMock() {
		return this.client.subscription as ReturnType<typeof vi.fn>;
	}
}

const TEST_SERVER = 'test-server-bus';

describe('eventBusManager subscription robustness', () => {
	let consoleError: ReturnType<typeof vi.spyOn>;
	let consoleWarn: ReturnType<typeof vi.spyOn>;
	let consoleDebug: ReturnType<typeof vi.spyOn>;

	beforeEach(() => {
		consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
		consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
		consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => {});
	});

	afterEach(() => {
		eventBusManager.stopBus(TEST_SERVER);
		consoleError.mockRestore();
		consoleWarn.mockRestore();
		consoleDebug.mockRestore();
		vi.useRealTimers();
	});

	it('logs an error when the subscription delivers result.error', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);

		fake.current.next({ error: new Error('subscription failed') });

		expect(consoleError).toHaveBeenCalledTimes(1);
		expect(consoleError.mock.calls[0][0]).toContain(TEST_SERVER);
		expect(consoleError.mock.calls[0][0]).toContain('subscription error');
	});

	it('isolates handler errors so one throwing handler does not stop the others', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);

		const bus = eventBusManager.getBus(TEST_SERVER)!;
		const ranBefore = vi.fn();
		const ranAfter = vi.fn();
		bus.handlers.add(ranBefore);
		bus.handlers.add(() => {
			throw new Error('handler boom');
		});
		bus.handlers.add(ranAfter);

		const event = { actorId: 'a', event: { __typename: 'ServerUpdatedEvent' } };
		fake.current.next({ data: { myEvents: event } });

		expect(ranBefore).toHaveBeenCalledTimes(1);
		expect(ranAfter).toHaveBeenCalledTimes(1);
		expect(consoleError).toHaveBeenCalled();
		expect(consoleError.mock.calls[0][0]).toContain('handler threw');
	});

	it('continues delivering events after a handler error on a previous event', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);

		const bus = eventBusManager.getBus(TEST_SERVER)!;
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
		fake.current.next({ data: { myEvents: event } });
		fake.current.next({ data: { myEvents: event } });

		expect(handler).toHaveBeenCalledTimes(2);
	});

	it('re-subscribes when the source ends (onEnd)', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);
		const catchUp = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

		// Server sent Complete (or graphql-ws closed the Sink) → source ends.
		fake.current.complete();

		expect(fake.subscribeCalls).toBe(2);
		expect(catchUp).toHaveBeenCalledWith('subscription-ended');
		expect(consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('source ended'))).toBe(true);

		// And the new subscription is wired through — events flow.
		const handler = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);
		fake.current.next({
			data: { myEvents: { actorId: 'a', event: { __typename: 'ServerUpdatedEvent' } } }
		});
		expect(handler).toHaveBeenCalledTimes(1);
	});

	it('re-notifies catch-up handlers after the projection grace period', async () => {
		vi.useFakeTimers();
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		const catchUp = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

		fake.current.complete();

		expect(catchUp).toHaveBeenCalledTimes(1);
		expect(catchUp).toHaveBeenNthCalledWith(1, 'subscription-ended');

		await vi.advanceTimersByTimeAsync(2_499);
		expect(catchUp).toHaveBeenCalledTimes(1);

		await vi.advanceTimersByTimeAsync(1);
		expect(catchUp).toHaveBeenCalledTimes(2);
		expect(catchUp).toHaveBeenNthCalledWith(2, 'subscription-ended');
	});

	it('re-subscribes when the WebSocket reconnects (reconnectCount increments)', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);
		const catchUp = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

		fake.bumpReconnect();

		expect(fake.subscribeCalls).toBe(2);
		expect(catchUp).toHaveBeenCalledWith('ws-reconnected');
		expect(consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('ws reconnected'))).toBe(true);
	});

	it('re-subscribes and notifies catch-up handlers when heartbeats stall', () => {
		vi.useFakeTimers();
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);
		const catchUp = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

		vi.advanceTimersByTime(90_000);

		expect(fake.subscribeCalls).toBe(2);
		expect(catchUp).toHaveBeenCalledWith('heartbeat-stalled');
		expect(consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('heartbeat stalled'))).toBe(true);
	});

	it('subscribes without variables on initial start and reconnect', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscriptionMock.mock.calls[0][1]).toEqual({});

		fake.current.next({
			data: {
				myEvents: {
					actorId: 'a',
					event: { __typename: 'ServerUpdatedEvent' }
				}
			}
		});
		fake.bumpReconnect();

		expect(fake.subscriptionMock.mock.calls[1][1]).toEqual({});
	});

	it('does not dispatch heartbeat events to handlers', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		const handler = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

		fake.current.next({
			data: { myEvents: { actorId: '', event: { __typename: 'HeartbeatEvent' } } }
		});

		expect(handler).not.toHaveBeenCalled();
	});

	it('treats room universal changes as room layout updates', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		const handler = vi.fn();
		const unsubscribe = createEventBusHandlerRegistrar(TEST_SERVER)!.onRoomLayoutUpdated(handler);

		fake.current.next({
			data: {
				myEvents: {
					actorId: 'a',
					event: {
						__typename: 'RoomUniversalChangedEvent',
						roomId: 'room-1',
						universal: false
					}
				}
			}
		});

		expect(handler).toHaveBeenCalledWith({ roomId: 'room-1', universal: false });

		unsubscribe();
	});

	it('does NOT re-subscribe when stopBus is called (teardown guard)', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);

		eventBusManager.stopBus(TEST_SERVER);

		// Unsubscribing the wonka source completes it, which would trigger
		// onEnd → resubscribe without the guard. With the guard, no new
		// subscription is started.
		expect(fake.subscribeCalls).toBe(1);
	});

});
