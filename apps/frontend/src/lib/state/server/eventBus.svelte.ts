/**
 * Manages per-server event bus subscriptions. One `myEvents`
 * subscription per registered server — the bus holds the handler set,
 * the manager stores the subscription handle for teardown.
 *
 * The sidebar wires handlers against any connected server's bus through
 * the manager (not just the one in URL focus), which is how cross-server
 * notification indicators work without each server holding its own subscription
 * context.
 */

import { pipe, subscribe as urqlSubscribe, onEnd } from 'wonka';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { EventBusCatchUpReason, EventHandler, EventBus } from '$lib/eventBus.svelte';
import {
	MyServerEventsLegacySubscriptionDoc,
	MyServerEventsSubscriptionDoc
} from '$lib/eventBus.svelte';
import {
	isUnsupportedGraphQLFieldError,
	isUnsupportedGraphQLTypeError
} from '$lib/gql/compatibility';
import type { GraphQLClient } from './graphqlClient.svelte';

const HEARTBEAT_STALL_MS = 75_000;
const HEARTBEAT_WATCHDOG_MS = 15_000;
const CATCH_UP_RETRY_MS = 2_500;

function errorDebug(error: unknown) {
	const graphQLErrors = (error as { graphQLErrors?: Array<{ message?: string; extensions?: { code?: unknown } }> })
		?.graphQLErrors;
	const networkError = (error as { networkError?: unknown })?.networkError;
	return {
		message: error instanceof Error ? error.message : undefined,
		graphQLErrors: graphQLErrors?.map((e) => ({
			message: e.message,
			code: e.extensions?.code
		})),
		networkError: networkError instanceof Error ? networkError.message : networkError
	};
}

function isUnsupportedLiveEventSubscriptionError(error: unknown): boolean {
	return (
		isUnsupportedGraphQLFieldError(error, 'callId') ||
		isUnsupportedGraphQLFieldError(error, 'customStatus') ||
		[
			'CallStartedEvent',
			'CallParticipantJoinedEvent',
			'CallParticipantLeftEvent',
			'CallEndedEvent',
			'UserCustomStatusSetEvent',
			'UserCustomStatusClearedEvent'
		].some((typeName) => isUnsupportedGraphQLTypeError(error, typeName))
	);
}

class EventBusManager {
	// SvelteMap so getBus() is a reactive read — consumers like NotificationSync
	// re-run their $effect when a bus is started/stopped, which avoids a race
	// where the consumer mounts before startBus and never re-attaches.
	#buses = new SvelteMap<string, EventBus>();
	#subscriptions = new Map<string, { unsubscribe: () => void }>();
	#cleanups = new Map<string, () => void>();
	#paused = false;

	/**
	 * Start an event bus for the given server. Creates the subscription and
	 * stores the bus. If a bus already exists for this server, returns a
	 * cleanup function without creating a duplicate.
	 *
	 * The bus stays intentionally small: it re-subscribes when the current
	 * source ends, when the underlying WebSocket reconnects, or when the
	 * server heartbeat goes silent while the tab is visible. Consumers that
	 * own projected state can register `catchUpHandlers` to refetch after
	 * those gaps instead of relying on subscription replay.
	 *
	 * @returns Cleanup function that stops the bus.
	 */
	startBus(serverId: string, gqlClient: GraphQLClient): () => void {
		if (this.#paused) {
			return () => {};
		}
		if (this.#buses.has(serverId)) {
			// Already running — return a no-op cleanup (the real cleanup is from
			// the original startBus call)
			return () => {};
		}

		const client = gqlClient.client;
		const handlers = new SvelteSet<EventHandler>();
		const catchUpHandlers = new SvelteSet<(reason: EventBusCatchUpReason) => void>();
		const bus: EventBus = { handlers, catchUpHandlers };
		let lastEventAt = Date.now();
		let heartbeatCount = 0;
		let dispatchedEventCount = 0;
		let resubscribeCount = 0;
		let subscriptionGeneration = 0;
		let usingLegacySubscription = false;
		let catchUpRetryTimer: ReturnType<typeof setTimeout> | null = null;
		// Set while we're tearing down a subscription (either to replace it
		// or because the bus is stopping). Prevents `onEnd` from firing a
		// reentrant resubscribe in response to our own unsubscribe.
		let teardownInProgress = false;
		let stopped = false;

		const debugState = () => ({
			generation: subscriptionGeneration,
			handlers: handlers.size,
			events: dispatchedEventCount,
			heartbeats: heartbeatCount,
			resubscribes: resubscribeCount,
			lastEventAgeMs: Date.now() - lastEventAt
		});

		const subscribeOnce = (reason: string) => {
			subscriptionGeneration++;
			const generation = subscriptionGeneration;
			console.debug(`[eventBus:${serverId}] subscribing`, {
				reason,
				...debugState()
			});
			const subscriptionDoc = usingLegacySubscription
				? MyServerEventsLegacySubscriptionDoc
				: MyServerEventsSubscriptionDoc;
			return pipe(
				client.subscription(subscriptionDoc, {}),
				onEnd(() => {
					if (teardownInProgress || stopped) return;
					console.debug(`[eventBus:${serverId}] subscription source ended`, {
						sourceGeneration: generation,
						...debugState()
					});
					console.warn(`[eventBus:${serverId}] subscription source ended`);
					resubscribe('subscription source ended', 'subscription-ended');
				}),
				urqlSubscribe((result) => {
					if (result.error) {
						if (
							!usingLegacySubscription &&
							isUnsupportedLiveEventSubscriptionError(result.error)
						) {
							usingLegacySubscription = true;
							console.warn(
								`[eventBus:${serverId}] live event subscription fields unsupported; falling back to legacy myEvents subscription`
							);
							resubscribe('live event fragments unsupported', 'subscription-ended');
							return;
						}
						console.debug(`[eventBus:${serverId}] subscription error state`, {
							generation,
							state: debugState(),
							error: errorDebug(result.error)
						});
						console.error(`[eventBus:${serverId}] subscription error`, result.error);
						return;
					}
					lastEventAt = Date.now();
					if (!result.data) {
						console.debug(`[eventBus:${serverId}] subscription result without data`, {
							sourceGeneration: generation,
							...debugState()
						});
						return;
					}
					const event = result.data.myEvents;
					// Heartbeats are pure liveness signals — already accounted for
					// via lastEventAt above. Don't dispatch to handlers.
					if (event.event?.__typename === 'HeartbeatEvent') {
						heartbeatCount++;
						console.debug(`[eventBus:${serverId}] heartbeat received (total: ${heartbeatCount})`);
						return;
					}
					dispatchedEventCount++;
					console.debug(
						`[eventBus:${serverId}] event dispatched`,
						event.event?.__typename ?? '<unknown>',
						{
							generation,
							eventId: event.id,
							total: dispatchedEventCount
						}
					);
					// Run handlers in isolation: a throw from one handler must not
					// stop the others or tear down the subscription itself.
					for (const handler of handlers) {
						try {
							handler(event);
						} catch (err) {
							console.error(`[eventBus:${serverId}] handler threw`, err);
						}
					}
				})
			);
		};

		const notifyCatchUpHandlers = (
			reason: EventBusCatchUpReason,
			phase: 'immediate' | 'projection-grace' = 'immediate'
		) => {
			console.debug(`[eventBus:${serverId}] notifying catch-up handlers`, {
				reason,
				phase,
				catchUpHandlers: catchUpHandlers.size,
				...debugState()
			});
			for (const handler of catchUpHandlers) {
				try {
					handler(reason);
				} catch (err) {
					console.error(`[eventBus:${serverId}] catch-up handler threw`, err);
				}
			}
		};

		const scheduleCatchUpRetry = (reason: EventBusCatchUpReason) => {
			if (catchUpRetryTimer) clearTimeout(catchUpRetryTimer);
			catchUpRetryTimer = setTimeout(() => {
				catchUpRetryTimer = null;
				if (stopped) return;
				console.debug(`[eventBus:${serverId}] retrying catch-up after projection grace period`, {
					reason,
					...debugState()
				});
				notifyCatchUpHandlers(reason, 'projection-grace');
			}, CATCH_UP_RETRY_MS);
		};

		const resubscribe = (reason: string, catchUpReason: EventBusCatchUpReason) => {
			if (stopped) return;
			resubscribeCount++;
			console.debug(`[eventBus:${serverId}] resubscribe requested`, {
				reason,
				...debugState()
			});
			console.warn(
				`[eventBus:${serverId}] re-subscribing (${reason}; total resubscribes: ${resubscribeCount}; lastEvent: ${Math.round((Date.now() - lastEventAt) / 1000)}s ago)`
			);
			teardownInProgress = true;
			this.#subscriptions.get(serverId)?.unsubscribe();
			teardownInProgress = false;
			lastEventAt = Date.now();
			this.#subscriptions.set(serverId, subscribeOnce(reason));
			notifyCatchUpHandlers(catchUpReason);
			scheduleCatchUpRetry(catchUpReason);
		};

		console.debug(`[eventBus:${serverId}] bus started`, debugState());
		this.#subscriptions.set(serverId, subscribeOnce('initial start'));

		// Force resubscribe on every WebSocket reconnect. graphql-ws's
		// internal auto-resubscribe should handle the live Sinks, but we
		// can't observe whether it actually delivered the new subscription
		// to the server — `onEnd` only catches Sinks that were closed,
		// not Sinks that are silently inert. An explicit re-subscribe on
		// reconnect closes that gap.
		let lastSeenReconnects = gqlClient.reconnectCount;
		const stopReconnectEffect = $effect.root(() => {
			$effect(() => {
				const n = gqlClient.reconnectCount;
				if (n > lastSeenReconnects) {
					console.debug(
						`[eventBus:${serverId}] ws reconnectCount ${lastSeenReconnects} → ${n}, resubscribing`
					);
					lastSeenReconnects = n;
					resubscribe('ws reconnected', 'ws-reconnected');
				}
			});
		});

		const heartbeatWatchdog = setInterval(() => {
			if (stopped) return;
			if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
			const ageMs = Date.now() - lastEventAt;
			if (ageMs < HEARTBEAT_STALL_MS) return;
			console.debug(`[eventBus:${serverId}] heartbeat watchdog detected stale stream`, {
				ageMs,
				...debugState()
			});
			console.warn(
				`[eventBus:${serverId}] heartbeat stalled; re-subscribing (${Math.round(ageMs / 1000)}s since last event)`
			);
			resubscribe('heartbeat stalled', 'heartbeat-stalled');
		}, HEARTBEAT_WATCHDOG_MS);

		this.#cleanups.set(serverId, () => {
			// Flag the closure so the upcoming sub.unsubscribe() in stopBus
			// doesn't fire a reentrant resubscribe through onEnd.
			stopped = true;
			console.debug(`[eventBus:${serverId}] bus stopping`, debugState());
			if (catchUpRetryTimer) clearTimeout(catchUpRetryTimer);
			clearInterval(heartbeatWatchdog);
			stopReconnectEffect();
		});

		this.#buses.set(serverId, bus);

		return () => this.stopBus(serverId);
	}

	/** Stop and remove the event bus for the given server. */
	stopBus(serverId: string): void {
		const cleanup = this.#cleanups.get(serverId);
		if (cleanup) {
			cleanup();
			this.#cleanups.delete(serverId);
		}
		const sub = this.#subscriptions.get(serverId);
		if (sub) {
			// Mark teardown so the `onEnd` callback inside the pipe doesn't
			// try to resubscribe a bus that's going away.
			sub.unsubscribe();
			this.#subscriptions.delete(serverId);
		}
		this.#buses.delete(serverId);
	}

	/** Get the event bus for a server, or undefined if not started. */
	getBus(serverId: string): EventBus | undefined {
		return this.#buses.get(serverId);
	}

	/** Stop all buses. Used during teardown (e.g., logout). */
	stopAll(): void {
		for (const serverId of [...this.#buses.keys()]) {
			this.stopBus(serverId);
		}
	}

	/** Stop all event streams and block new starts until resumeAll() is called. */
	pauseAll(): void {
		this.#paused = true;
		this.stopAll();
	}

	/** Allow event streams to be started again. Callers decide which buses to restart. */
	resumeAll(): void {
		this.#paused = false;
	}
}

export const eventBusManager = new EventBusManager();
