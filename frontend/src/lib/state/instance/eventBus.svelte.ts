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
import type { EventHandler, InstanceEventBus } from '$lib/instanceEventBus.svelte';
import { MyInstanceEventsSubscriptionDoc } from '$lib/instanceEventBus.svelte';

class InstanceEventBusManager {
	// SvelteMap so getBus() is a reactive read — consumers like NotificationSync
	// re-run their $effect when a bus is started/stopped, which avoids a race
	// where the consumer mounts before startBus and never re-attaches.
	#buses = new SvelteMap<string, InstanceEventBus>();
	#subscriptions = new Map<string, { unsubscribe: () => void }>();

	/**
	 * Start an event bus for the given instance. Creates the subscription and
	 * stores the bus. If a bus already exists for this instance, returns a
	 * cleanup function without creating a duplicate.
	 *
	 * @returns Cleanup function that stops the bus.
	 */
	startBus(instanceId: string, client: Client): () => void {
		if (this.#buses.has(instanceId)) {
			// Already running — return a no-op cleanup (the real cleanup is from
			// the original startBus call)
			return () => {};
		}

		const handlers = new SvelteSet<EventHandler>();
		const bus: InstanceEventBus = { handlers };

		const sub = client.subscription(MyInstanceEventsSubscriptionDoc, {}).subscribe((result) => {
			if (!result.data) return;
			const event = result.data.myInstanceEvents;
			handlers.forEach((handler) => handler(event));
		});

		this.#buses.set(instanceId, bus);
		this.#subscriptions.set(instanceId, sub);

		return () => this.stopBus(instanceId);
	}

	/** Stop and remove the event bus for the given instance. */
	stopBus(instanceId: string): void {
		const sub = this.#subscriptions.get(instanceId);
		if (sub) {
			sub.unsubscribe();
			this.#subscriptions.delete(instanceId);
		}
		this.#buses.delete(instanceId);
	}

	/** Get the event bus for an instance, or undefined if not started. */
	getBus(instanceId: string): InstanceEventBus | undefined {
		return this.#buses.get(instanceId);
	}

	/** Stop all buses. Used during teardown (e.g., logout). */
	stopAll(): void {
		for (const instanceId of [...this.#buses.keys()]) {
			this.stopBus(instanceId);
		}
	}
}

export const instanceEventBusManager = new InstanceEventBusManager();
