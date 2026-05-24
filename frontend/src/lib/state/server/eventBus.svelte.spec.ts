import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { makeSubject, type Source, type Subject } from 'wonka';
import type { Client } from '@urql/svelte';
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
}

const TEST_SERVER = 'test-server-bus';

describe('eventBusManager subscription robustness', () => {
	let consoleError: ReturnType<typeof vi.spyOn>;
	let consoleWarn: ReturnType<typeof vi.spyOn>;

	beforeEach(() => {
		consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
		consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
	});

	afterEach(() => {
		eventBusManager.stopBus(TEST_SERVER);
		consoleError.mockRestore();
		consoleWarn.mockRestore();
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

		// Server sent Complete (or graphql-ws closed the Sink) → source ends.
		fake.current.complete();

		expect(fake.subscribeCalls).toBe(2);
		expect(consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('source ended'))).toBe(true);

		// And the new subscription is wired through — events flow.
		const handler = vi.fn();
		eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);
		fake.current.next({
			data: { myEvents: { actorId: 'a', event: { __typename: 'ServerUpdatedEvent' } } }
		});
		expect(handler).toHaveBeenCalledTimes(1);
	});

	it('re-subscribes when the WebSocket reconnects (reconnectCount increments)', () => {
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);

		fake.bumpReconnect();

		expect(fake.subscribeCalls).toBe(2);
		expect(consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('ws reconnected'))).toBe(true);
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

	it('re-subscribes after STALE_THRESHOLD_MS of only heartbeats (watchdog)', () => {
		vi.useFakeTimers();
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);

		// Heartbeat counts as activity — refreshes lastEventAt but is not
		// dispatched. Send one near the start.
		fake.current.next({
			data: { myEvents: { actorId: '', event: { __typename: 'HeartbeatEvent' } } }
		});

		// Advance past the 40s watchdog threshold without sending anything else.
		vi.advanceTimersByTime(50_000);

		expect(fake.subscribeCalls).toBeGreaterThanOrEqual(2);
	});

	it('does NOT refresh the watchdog on error-only results', () => {
		vi.useFakeTimers();
		const fake = new FakeGqlClient();
		eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
		expect(fake.subscribeCalls).toBe(1);

		// Stream of errors with no data — must NOT keep the watchdog alive.
		for (let i = 0; i < 5; i++) {
			vi.advanceTimersByTime(10_000);
			fake.current.next({ error: new Error('flapping') });
		}

		// Total elapsed: 50s — well past the 40s threshold. If the error-only
		// path were refreshing lastEventAt, the watchdog would never fire.
		expect(fake.subscribeCalls).toBeGreaterThanOrEqual(2);
	});
});
