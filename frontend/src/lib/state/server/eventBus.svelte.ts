/**
 * Manages per-instance event bus subscriptions.
 *
 * Each connected instance gets its own event bus (a set of handlers) and its own
 * GraphQL subscription. The manager tracks all buses and their subscriptions,
 * allowing the sidebar to register handlers on any instance's bus (not just the
 * active one via Svelte context).
 */

import { type Client } from '@urql/svelte';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { EventHandler, ServerEventBus } from '$lib/serverEventBus.svelte';
import { MyInstanceEventsSubscriptionDoc } from '$lib/serverEventBus.svelte';

class ServerEventBusManager {
	// SvelteMap so getBus() is a reactive read — consumers like NotificationSync
	// re-run their $effect when a bus is started/stopped, which avoids a race
	// where the consumer mounts before startBus and never re-attaches.
	#buses = new SvelteMap<string, ServerEventBus>();
	#subscriptions = new Map<string, { unsubscribe: () => void }>();

	/**
	 * Start an event bus for the given instance. Creates the subscription and
	 * stores the bus. If a bus already exists for this instance, returns a
	 * cleanup function without creating a duplicate.
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
		const bus: ServerEventBus = { handlers };

		const sub = client.subscription(MyInstanceEventsSubscriptionDoc, {}).subscribe((result) => {
			if (result.error) {
				// Surface subscription errors so unreachable instances and other
				// real failures are visible in the dev console. Don't propagate
				// — keep the subscription itself alive so it can recover.
				console.error(
					`[eventBus:${serverId}] subscription error`,
					result.error
				);
			}
			if (!result.data) return;
			const event = result.data.myInstanceEvents;
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

		this.#buses.set(serverId, bus);
		this.#subscriptions.set(serverId, sub);

		return () => this.stopBus(serverId);
	}

	/** Stop and remove the event bus for the given instance. */
	stopBus(serverId: string): void {
		const sub = this.#subscriptions.get(serverId);
		if (sub) {
			sub.unsubscribe();
			this.#subscriptions.delete(serverId);
		}
		this.#buses.delete(serverId);
	}

	/** Get the event bus for an instance, or undefined if not started. */
	getBus(serverId: string): ServerEventBus | undefined {
		return this.#buses.get(serverId);
	}

	/** Stop all buses. Used during teardown (e.g., logout). */
	stopAll(): void {
		for (const serverId of [...this.#buses.keys()]) {
			this.stopBus(serverId);
		}
	}
}

export const serverEventBusManager = new ServerEventBusManager();
