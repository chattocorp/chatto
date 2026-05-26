/**
 * Manages per-server event bus subscriptions. One `myEvents`
 * subscription per registered server — the bus holds the handler set,
 * the manager stores the subscription handle for teardown.
 *
 * The sidebar wires handlers against any connected server's bus through
 * the manager (not just the one in URL focus), which is how cross-server
 * notification dots work without each server holding its own subscription
 * context.
 */

import { pipe, subscribe as urqlSubscribe, onEnd } from 'wonka';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { EventHandler, EventBus } from '$lib/eventBus.svelte';
import { MyServerEventsSubscriptionDoc } from '$lib/eventBus.svelte';
import type { GraphQLClient } from './graphqlClient.svelte';

// Safety-net watchdog: if no event arrives within this window while the
// tab is visible, force a re-subscribe. The server emits a HeartbeatEvent
// every 25s (see StreamMyEvents in core.go), so 40s tolerates one missed
// heartbeat plus jitter without thrashing.
//
// This is the floor, not the primary detector — `onEnd` and the WS-
// reconnect handler below catch the common failure modes immediately.
const STALE_THRESHOLD_MS = 40_000;
const WATCHDOG_INTERVAL_MS = 15_000;
// On visibilitychange → visible, re-subscribe if last event is older
// than this. Catches laptop-wake-from-sleep cases without thrashing the
// connection on quick tab toggles.
const VISIBILITY_RESUBSCRIBE_AFTER_MS = 30_000;

// Periodic liveness summary cadence. Emitted at log level (not debug) so
// post-incident screenshots show a clean timeline of subscription health
// without needing verbose console filtering enabled.
const LIVENESS_SUMMARY_INTERVAL_MS = 60_000;

class EventBusManager {
	// SvelteMap so getBus() is a reactive read — consumers like NotificationSync
	// re-run their $effect when a bus is started/stopped, which avoids a race
	// where the consumer mounts before startBus and never re-attaches.
	#buses = new SvelteMap<string, EventBus>();
	#subscriptions = new Map<string, { unsubscribe: () => void }>();
	#cleanups = new Map<string, () => void>();

	/**
	 * Start an event bus for the given server. Creates the subscription and
	 * stores the bus. If a bus already exists for this server, returns a
	 * cleanup function without creating a duplicate.
	 *
	 * Three layers of resubscribe coverage, in order of fastest reaction:
	 *
	 *  1. **`onEnd`** — urql/wonka emits an End signal when the source
	 *     terminates (server sent Complete or Error for the subscription
	 *     ID, or graphql-ws closed the Sink). graphql-ws does NOT auto-
	 *     resubscribe a closed Sink on later reconnects, so we explicitly
	 *     re-establish.
	 *  2. **WS reconnect** — when the underlying WebSocket transitions
	 *     disconnected → connected (tracked by `GraphQLClient.reconnectCount`),
	 *     we proactively re-subscribe. Belt-and-suspenders for the case
	 *     where graphql-ws's own auto-resubscribe didn't deliver us a new
	 *     subscription (silently-inert Sink, no `onEnd` to catch).
	 *  3. **Watchdog** — if neither of the above fires but events stop
	 *     flowing for STALE_THRESHOLD_MS (one missed 25s heartbeat + buffer),
	 *     re-subscribe as a last resort.
	 *
	 * @returns Cleanup function that stops the bus.
	 */
	startBus(serverId: string, gqlClient: GraphQLClient): () => void {
		if (this.#buses.has(serverId)) {
			// Already running — return a no-op cleanup (the real cleanup is from
			// the original startBus call)
			return () => {};
		}

		const client = gqlClient.client;
		const handlers = new SvelteSet<EventHandler>();
		const bus: EventBus = { handlers };
		let lastEventAt = Date.now();
		// Running counters so the periodic liveness summary and post-mortem
		// logs can quote concrete numbers ("5 events, 12 heartbeats in the
		// last minute") rather than just "subscription is alive".
		let heartbeatCount = 0;
		let dispatchedEventCount = 0;
		let resubscribeCount = 0;
		// Set while we're tearing down a subscription (either to replace it
		// or because the bus is stopping). Prevents `onEnd` from firing a
		// reentrant resubscribe in response to our own unsubscribe.
		let teardownInProgress = false;
		let stopped = false;

		const subscribeOnce = () =>
			pipe(
				client.subscription(MyServerEventsSubscriptionDoc, {}),
				onEnd(() => {
					if (teardownInProgress || stopped) return;
					console.warn(`[eventBus:${serverId}] subscription source ended`);
					resubscribe('subscription source ended');
				}),
				urqlSubscribe((result) => {
					if (result.error) {
						// Surface subscription errors so unreachable servers and other
						// real failures are visible in the dev console. Don't refresh
						// lastEventAt — an error storm without data should not mask a
						// stalled pipeline from the watchdog.
						console.error(
							`[eventBus:${serverId}] subscription error`,
							result.error
						);
						return;
					}
					lastEventAt = Date.now();
					if (!result.data) return;
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
						`(total: ${dispatchedEventCount})`
					);
					// Run handlers in isolation: a throw from one handler must not
					// stop the others or tear down the subscription itself.
					for (const handler of handlers) {
						try {
							handler(event);
						} catch (err) {
							console.error(
								`[eventBus:${serverId}] handler threw`,
								err
							);
						}
					}
				})
			);

		const resubscribe = (reason: string) => {
			if (stopped) return;
			resubscribeCount++;
			console.warn(
				`[eventBus:${serverId}] re-subscribing (${reason}; total resubscribes: ${resubscribeCount}; lastEvent: ${Math.round((Date.now() - lastEventAt) / 1000)}s ago)`
			);
			teardownInProgress = true;
			this.#subscriptions.get(serverId)?.unsubscribe();
			teardownInProgress = false;
			lastEventAt = Date.now();
			this.#subscriptions.set(serverId, subscribeOnce());
		};

		console.debug(`[eventBus:${serverId}] bus started`);
		this.#subscriptions.set(serverId, subscribeOnce());

		const watchdog = setInterval(() => {
			if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
				console.debug(`[eventBus:${serverId}] watchdog skipped (tab hidden)`);
				return;
			}
			const gap = Date.now() - lastEventAt;
			if (gap < STALE_THRESHOLD_MS) return;
			console.warn(
				`[eventBus:${serverId}] watchdog: stale (${Math.round(gap / 1000)}s since last event, threshold ${STALE_THRESHOLD_MS / 1000}s)`
			);
			resubscribe(`no event for ${STALE_THRESHOLD_MS}ms`);
		}, WATCHDOG_INTERVAL_MS);

		// Periodic at-a-glance health snapshot so a multi-hour log shows
		// whether the subscription was alive throughout — invaluable when
		// debugging post-sleep "no events received" reports where the
		// per-event debug lines may be too noisy to scroll through.
		const livenessSummary = setInterval(() => {
			const gapSec = Math.round((Date.now() - lastEventAt) / 1000);
			console.debug(
				`[eventBus:${serverId}] alive (handlers=${handlers.size}, events=${dispatchedEventCount}, heartbeats=${heartbeatCount}, resubscribes=${resubscribeCount}, lastEvent=${gapSec}s ago, visible=${typeof document === 'undefined' ? 'n/a' : document.visibilityState === 'visible'})`
			);
		}, LIVENESS_SUMMARY_INTERVAL_MS);

		const onVisibility = () => {
			if (document.visibilityState !== 'visible') return;
			const gap = Date.now() - lastEventAt;
			if (gap > VISIBILITY_RESUBSCRIBE_AFTER_MS) {
				console.debug(
					`[eventBus:${serverId}] visibility=visible, gap=${Math.round(gap / 1000)}s → resubscribing`
				);
				resubscribe('tab became visible after gap');
			} else {
				console.debug(
					`[eventBus:${serverId}] visibility=visible, gap=${Math.round(gap / 1000)}s → no resubscribe (under threshold)`
				);
			}
		};
		if (typeof document !== 'undefined') {
			document.addEventListener('visibilitychange', onVisibility);
		}

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
					resubscribe('ws reconnected');
				}
			});
		});

		this.#cleanups.set(serverId, () => {
			// Flag the closure so the upcoming sub.unsubscribe() in stopBus
			// doesn't fire a reentrant resubscribe through onEnd.
			stopped = true;
			clearInterval(watchdog);
			clearInterval(livenessSummary);
			if (typeof document !== 'undefined') {
				document.removeEventListener('visibilitychange', onVisibility);
			}
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
}

export const eventBusManager = new EventBusManager();
