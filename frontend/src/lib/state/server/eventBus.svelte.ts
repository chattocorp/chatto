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

import { type Client } from '@urql/svelte';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { EventHandler, EventBus } from '$lib/eventBus.svelte';
import { MyServerEventsSubscriptionDoc } from '$lib/eventBus.svelte';

// Liveness watchdog: re-subscribe if no event arrives within this window
// while the tab is visible. The server emits a HeartbeatEvent every 25s
// on the subscription (see StreamMyEvents in core.go), so 60s of silence
// means the subscription is dead — even in an otherwise-idle room.
//
// The watchdog is intentionally generic: it doesn't track heartbeats
// separately. Any event (real or heartbeat) resets lastEventAt. The
// heartbeat exists so this fallback works even when nothing is happening.
const STALE_THRESHOLD_MS = 60_000;
// How often the watchdog checks for staleness.
const WATCHDOG_INTERVAL_MS = 15_000;
// On visibilitychange → visible, re-subscribe if last event is older
// than this. Catches laptop-wake-from-sleep cases without thrashing the
// connection on quick tab toggles.
const VISIBILITY_RESUBSCRIBE_AFTER_MS = 30_000;

class EventBusManager {
	// SvelteMap so getBus() is a reactive read — consumers like NotificationSync
	// re-run their $effect when a bus is started/stopped, which avoids a race
	// where the consumer mounts before startBus and never re-attaches.
	#buses = new SvelteMap<string, EventBus>();
	#subscriptions = new Map<string, { unsubscribe: () => void }>();
	#watchdogs = new Map<string, () => void>();

	/**
	 * Start an event bus for the given instance. Creates the subscription and
	 * stores the bus. If a bus already exists for this instance, returns a
	 * cleanup function without creating a duplicate.
	 *
	 * A watchdog re-subscribes if no event (real or heartbeat) is received
	 * for STALE_THRESHOLD_MS while the tab is visible. This catches the case
	 * where the WebSocket is healthy but the server-side subscription
	 * goroutine has silently died — without this, mutations succeed but
	 * new events never arrive until the user hard-reloads.
	 *
	 * @returns Cleanup function that stops the bus.
	 */
	startBus(serverId: string, client: Client): () => void {
		if (this.#buses.has(serverId)) {
			// Already running — return a no-op cleanup (the real cleanup is from
			// the original startBus call)
			return () => {};
		}

		const handlers = new SvelteSet<EventHandler>();
		const bus: EventBus = { handlers };
		let lastEventAt = Date.now();

		const subscribe = () => {
			return client.subscription(MyServerEventsSubscriptionDoc, {}).subscribe((result) => {
				lastEventAt = Date.now();

				if (result.error) {
					// Surface subscription errors so unreachable servers and other
					// real failures are visible in the dev console. Don't propagate
					// — keep the subscription itself alive so it can recover.
					console.error(
						`[eventBus:${serverId}] subscription error`,
						result.error
					);
				}
				if (!result.data) return;
				const event = result.data.myEvents;
				// Heartbeats are pure liveness signals — already accounted for
				// via lastEventAt above. Don't dispatch to handlers.
				if (event.event?.__typename === 'HeartbeatEvent') return;
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
			});
		};

		this.#subscriptions.set(serverId, subscribe());

		const resubscribe = (reason: string) => {
			console.warn(`[eventBus:${serverId}] re-subscribing (${reason})`);
			this.#subscriptions.get(serverId)?.unsubscribe();
			lastEventAt = Date.now();
			this.#subscriptions.set(serverId, subscribe());
		};

		const watchdog = setInterval(() => {
			if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
			if (Date.now() - lastEventAt < STALE_THRESHOLD_MS) return;
			resubscribe(`no event for ${STALE_THRESHOLD_MS}ms`);
		}, WATCHDOG_INTERVAL_MS);

		const onVisibility = () => {
			if (document.visibilityState !== 'visible') return;
			if (Date.now() - lastEventAt > VISIBILITY_RESUBSCRIBE_AFTER_MS) {
				resubscribe('tab became visible after gap');
			}
		};
		if (typeof document !== 'undefined') {
			document.addEventListener('visibilitychange', onVisibility);
		}

		this.#watchdogs.set(serverId, () => {
			clearInterval(watchdog);
			if (typeof document !== 'undefined') {
				document.removeEventListener('visibilitychange', onVisibility);
			}
		});

		this.#buses.set(serverId, bus);

		return () => this.stopBus(serverId);
	}

	/** Stop and remove the event bus for the given instance. */
	stopBus(serverId: string): void {
		const stopWatchdog = this.#watchdogs.get(serverId);
		if (stopWatchdog) {
			stopWatchdog();
			this.#watchdogs.delete(serverId);
		}
		const sub = this.#subscriptions.get(serverId);
		if (sub) {
			sub.unsubscribe();
			this.#subscriptions.delete(serverId);
		}
		this.#buses.delete(serverId);
	}

	/** Get the event bus for an instance, or undefined if not started. */
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
