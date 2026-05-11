import { SvelteMap } from 'svelte/reactivity';
import { ServerStateStore } from './store.svelte';
import { graphqlClientManager } from './graphqlClient.svelte';
import { eventBusManager } from './eventBus.svelte';

const STORAGE_KEY = 'chatto:instances';

/**
 * A registered Chatto instance in the multi-instance client.
 */
export interface RegisteredInstance {
	/** Local slug (derived from hostname, e.g., "chat-example-com") */
	id: string;
	/** Base URL (e.g., "https://chat.example.com") */
	url: string;
	/** Instance display name (fetched from /api/instance) */
	name: string;
	/** Instance icon URL, or null if none */
	iconUrl: string | null;
	/** Bearer token for cross-origin auth, or null for origin instance (uses cookies) */
	token: string | null;
	/** Authenticated user ID on this instance, or null if not yet authenticated */
	userId: string | null;
	/** Authenticated user's login on this instance */
	userLogin: string | null;
	/** Authenticated user's display name on this instance */
	userDisplayName: string | null;
	/** Authenticated user's avatar URL on this instance */
	userAvatarUrl: string | null;
	/** When this instance was added (epoch ms) */
	addedAt: number;
}

/**
 * Generate a URL-safe instance ID from a base URL.
 * Extracts the hostname and replaces dots/colons with hyphens.
 * If the ID already exists in `existingIds`, appends a numeric suffix.
 */
export function generateInstanceId(url: string, existingIds: string[] = []): string {
	let hostname: string;
	try {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- extracting hostname string, URL not stored
		hostname = new URL(url).hostname;
	} catch {
		hostname = url.replace(/[^a-z0-9-]/gi, '-');
	}

	const base = hostname.replace(/\./g, '-').replace(/^-+|-+$/g, '');

	if (!existingIds.includes(base)) {
		return base;
	}

	let suffix = 2;
	while (existingIds.includes(`${base}-${suffix}`)) {
		suffix++;
	}
	return `${base}-${suffix}`;
}

function loadInstances(): RegisteredInstance[] {
	if (typeof localStorage === 'undefined') {
		return [];
	}

	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored) {
			const parsed = JSON.parse(stored);
			if (Array.isArray(parsed)) {
				return parsed;
			}
		}
	} catch {
		// Ignore parse errors, start fresh
	}
	return [];
}

function saveInstances(instances: RegisteredInstance[]): void {
	if (typeof localStorage === 'undefined') {
		return;
	}

	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(instances));
	} catch {
		// Ignore storage errors (quota exceeded, etc.)
	}
}


/**
 * Client-side registry of connected Chatto instances.
 * Owns both registration data and per-instance state stores.
 *
 * Registration and store creation are atomic — when an instance is added,
 * its store is created immediately. This eliminates race conditions where
 * $derived expressions see a registered instance but no store exists yet.
 *
 * The store map uses SvelteMap so that getStore() lookups are reactive
 * in $derived expressions.
 *
 * The registry does NOT track which instance is "active".
 * The active instance is determined by the URL (via the [[serverId=hostname]] layout)
 * and provided to components through Svelte context.
 */
class ServerRegistry {
	instances = $state<RegisteredInstance[]>(loadInstances());
	#stores = new SvelteMap<string, ServerStateStore>();

	/**
	 * Whether the async origin probe has completed (resolved or rejected).
	 * When `probeOrigin(true)` is called (known instance), this is set immediately.
	 * Use this to distinguish "probe in progress" from "no origin backend."
	 */
	originProbed = $state(false);

	/**
	 * The origin instance — the one serving the SPA.
	 * Derived by matching registered instance URLs against window.location.origin.
	 * Returns undefined if the origin server isn't registered.
	 */
	get originServer(): RegisteredInstance | undefined {
		if (typeof window === 'undefined') return undefined;
		const origin = window.location.origin;
		return this.instances.find((i) => {
			try {
				return new URL(i.url).origin === origin;
			} catch {
				return false;
			}
		});
	}

	/**
	 * Check whether a registered instance is the origin (the server serving the SPA).
	 * Uses URL comparison — no stored flag needed.
	 */
	isOriginInstance(serverId: string): boolean {
		const instance = this.getInstance(serverId);
		if (!instance || typeof window === 'undefined') return false;
		try {
			return new URL(instance.url).origin === window.location.origin;
		} catch {
			return false;
		}
	}

	/**
	 * Auto-register the origin server as a Chatto instance.
	 *
	 * When `knownInstance` is true (e.g., cookie-authenticated user), registers
	 * synchronously with a placeholder name — the store's instance.init() fetches
	 * the real name.
	 *
	 * When `knownInstance` is false, probes /api/instance first. If it responds,
	 * the origin is a Chatto instance — register it. If it fails (static hosting),
	 * nothing happens.
	 *
	 * No-ops if the origin is already registered (e.g., from localStorage).
	 */
	probeOrigin(knownInstance = false): void {
		if (typeof window === 'undefined') return;
		if (this.originServer) {
			this.originProbed = true;
			return; // Already registered
		}

		const origin = window.location.origin;

		if (knownInstance) {
			// Synchronous registration — we already know it's a Chatto instance
			const id = generateInstanceId(origin, this.instances.map((i) => i.id));
			this.#registerOrigin(id, origin, 'Chatto', null);
			this.originProbed = true;
			return;
		}

		// Async probe — detect if the origin is a Chatto instance
		fetch(`${origin}/api/instance`)
			.then((r) => {
				if (!r.ok) return;
				return r.json();
			})
			.then((data) => {
				if (!data) return;
				if (this.originServer) return; // Registered while we were fetching

				const id = generateInstanceId(origin, this.instances.map((i) => i.id));
				this.#registerOrigin(id, origin, data.name || 'Chatto', data.iconUrl ?? null);
			})
			.catch(() => {
				// Not a Chatto instance — ignore
			})
			.finally(() => {
				this.originProbed = true;
			});
	}

	#registerOrigin(id: string, url: string, name: string, iconUrl: string | null): void {
		this.addInstance({
			id,
			url,
			name,
			iconUrl,
			token: null,
			userId: null,
			userLogin: null,
			userDisplayName: null,
			userAvatarUrl: null,
			addedAt: Date.now()
		});
	}

	/**
	 * Bootstrap the registry: create stores for all registered instances.
	 * Call once from the root layout's script init (before any $derived reads stores).
	 */
	init(): void {
		for (const instance of this.instances) {
			if (!this.#stores.has(instance.id)) {
				this.#createStore(instance);
			}
		}
	}

	/** Add an instance to the registry, create its state store, and start its event bus. */
	addInstance(instance: RegisteredInstance): void {
		if (this.instances.some((i) => i.id === instance.id)) {
			return; // Already exists
		}
		this.instances.push(instance);
		saveInstances(this.instances);
		const store = this.#createStore(instance);

		// Start the event bus eagerly for already-authenticated instances so
		// child components (ServerSpaceSection) can register handlers during
		// their mount lifecycle. For cookie-auth instances the user is loaded
		// asynchronously by AuthenticatedChatProvider, so the layout's $effect
		// starts the bus once `isAuthenticated` flips true.
		if (store.isAuthenticated) {
			const gqlClient = graphqlClientManager.getClient(instance.id);
			eventBusManager.startBus(instance.id, gqlClient.client);
		}
	}

	/** Remove an instance by ID. Disposes its event bus, store, and GraphQL client. */
	removeInstance(id: string): boolean {
		const instance = this.instances.find((i) => i.id === id);
		if (!instance) {
			return false;
		}

		// Stop event bus subscription
		eventBusManager.stopBus(id);

		// Dispose state store
		this.#stores.get(id)?.dispose();
		this.#stores.delete(id);

		// Dispose GraphQL client
		graphqlClientManager.destroyClient(id);

		this.instances = this.instances.filter((i) => i.id !== id);
		saveInstances(this.instances);
		return true;
	}

	/** Remove all instances. Used by the global sign-out flow.
	 *  Clears dismissals so the origin can be re-discovered on next visit. */
	removeAll(): void {
		for (const instance of [...this.instances]) {
			eventBusManager.stopBus(instance.id);
			this.#stores.get(instance.id)?.dispose();
			this.#stores.delete(instance.id);
			graphqlClientManager.destroyClient(instance.id);
		}
		this.instances = [];
		saveInstances(this.instances);
	}

	/** Update fields on an existing instance. */
	updateServer(id: string, data: Partial<Omit<RegisteredInstance, 'id'>>): boolean {
		const instance = this.instances.find((i) => i.id === id);
		if (!instance) {
			return false;
		}

		Object.assign(instance, data);
		saveInstances(this.instances);
		return true;
	}

	/** Get an instance by ID. */
	getInstance(id: string): RegisteredInstance | undefined {
		return this.instances.find((i) => i.id === id);
	}

	/**
	 * Get the state store for a registered instance.
	 * Safe in $derived — stores are created atomically with registration,
	 * so every registered instance always has a store.
	 */
	getStore(serverId: string): ServerStateStore {
		const store = this.#stores.get(serverId);
		if (!store) {
			throw new Error(
				`No store for instance "${serverId}". Is it registered? ` +
					`Call serverRegistry.init() before accessing stores.`
			);
		}
		return store;
	}

	/**
	 * Get the state store for a registered instance, or undefined if not found.
	 * Use when the instance may not be registered (e.g., unresolved URL segments).
	 */
	tryGetStore(serverId: string): ServerStateStore | undefined {
		return this.#stores.get(serverId);
	}

	/** Create a state store for an instance and wire up remote user sync. */
	#createStore(instance: RegisteredInstance): ServerStateStore {
		const gqlClient = graphqlClientManager.getClient(instance.id);
		const store = new ServerStateStore(instance, gqlClient);
		this.#stores.set(instance.id, store);

		// Eagerly fetch instance info (name, MOTD, upload limits, etc.).
		// This is important for late-registered instances (e.g., origin registered
		// in the chat layout after the root layout script already ran).
		// init() is fail-soft (catches its own errors) but defensively swallow
		// any unexpected rejection so it can never become an unhandled rejection.
		store.instance.init().catch((err) => {
			console.error(
				`[instance:${instance.url}] unexpected init() rejection`,
				err
			);
		});

		if (instance.token === null) {
			// Cookie auth (origin) — the SvelteKit load function already determined
			// auth state. AuthenticatedChatProvider will set the user if authenticated.
			store.currentUser.loading = false;
		} else {
			// Bearer auth (remote) — auto-load the authenticated user via the token.
			// Catch failures (e.g. unreachable host, CORS) so they don't bubble up
			// as an unhandled rejection and crash the entire client.
			store.currentUser
				.load()
				.then(() => {
					const user = store.currentUser.user;
					if (user) {
						this.updateServer(instance.id, {
							userId: user.id,
							userLogin: user.login,
							userDisplayName: user.displayName,
							userAvatarUrl: user.avatarUrl
						});
					}
				})
				.catch((err) => {
					console.error(
						`[instance:${instance.url}] failed to load current user`,
						err
					);
					store.currentUser.loading = false;
				});
		}

		return store;
	}

	/** Whether the instance has an authenticated user. False if not registered. */
	isAuthenticated(serverId: string): boolean {
		return this.tryGetStore(serverId)?.isAuthenticated ?? false;
	}
}

export const serverRegistry = new ServerRegistry();
