import { SvelteMap } from 'svelte/reactivity';
import { ServerStateStore } from './store.svelte';
import { graphqlClientManager } from './graphqlClient.svelte';
import { eventBusManager } from './eventBus.svelte';
import { Codecs, globalSlot } from '$lib/storage/slot';
import { clearAssetProxyCache } from '$lib/pwa/assetProxy';

/**
 * A registered Chatto server in the multi-server client.
 */
export interface RegisteredServer {
	/** Local slug (derived from hostname, e.g., "chat-example-com") */
	id: string;
	/** Base URL (e.g., "https://chat.example.com") */
	url: string;
	/** Server display name (fetched from /api/server) */
	name: string;
	/** Server icon URL, or null if none */
	iconUrl: string | null;
	/** Bearer token for cross-origin auth, or null for origin server (uses cookies) */
	token: string | null;
	/** Authenticated user ID on this server, or null if not yet authenticated */
	userId: string | null;
	/** Authenticated user's login on this server */
	userLogin: string | null;
	/** Authenticated user's display name on this server */
	userDisplayName: string | null;
	/** Authenticated user's avatar URL on this server */
	userAvatarUrl: string | null;
	/** When this server was added (epoch ms) */
	addedAt: number;
}

/**
 * Generate a URL-safe server ID from a base URL.
 * Extracts the hostname and replaces dots/colons with hyphens.
 * If the ID already exists in `existingIds`, appends a numeric suffix.
 */
export function generateServerId(url: string, existingIds: string[] = []): string {
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

// Storage key intentionally stays as 'instances' — renaming would lose users'
// multi-server registrations (including remote bearer tokens that can't be
// regenerated). The in-code rename is purely cosmetic.
const serversSlot = globalSlot(
	'instances',
	[] as RegisteredServer[],
	Codecs.json<RegisteredServer[]>((v): v is RegisteredServer[] => Array.isArray(v))
);


/**
 * Client-side registry of connected Chatto servers.
 * Owns both registration data and per-server state stores.
 *
 * Registration and store creation are atomic — when a server is added,
 * its store is created immediately. This eliminates race conditions where
 * $derived expressions see a registered server but no store exists yet.
 *
 * The store map uses SvelteMap so that getStore() lookups are reactive
 * in $derived expressions.
 *
 * The registry does NOT track which server is "active".
 * The active server is determined by the URL (via the [[serverId=hostname]] layout)
 * and provided to components through Svelte context.
 */
class ServerRegistry {
	servers = $state<RegisteredServer[]>(serversSlot.get());
	#stores = new SvelteMap<string, ServerStateStore>();

	/**
	 * Whether the async origin probe has completed (resolved or rejected).
	 * When `probeOrigin(true)` is called (known server), this is set immediately.
	 * Use this to distinguish "probe in progress" from "no origin backend."
	 */
	originProbed = $state(false);

	/**
	 * The origin server — the one serving the SPA.
	 * Derived by matching registered server URLs against window.location.origin.
	 * Returns undefined if the origin server isn't registered.
	 */
	get originServer(): RegisteredServer | undefined {
		if (typeof window === 'undefined') return undefined;
		const origin = window.location.origin;
		return this.servers.find((s) => {
			try {
				return new URL(s.url).origin === origin;
			} catch {
				return false;
			}
		});
	}

	/**
	 * Check whether a registered server is the origin (the server serving the SPA).
	 * Uses URL comparison — no stored flag needed.
	 */
	isOriginServer(serverId: string): boolean {
		const server = this.getServer(serverId);
		if (!server || typeof window === 'undefined') return false;
		try {
			return new URL(server.url).origin === window.location.origin;
		} catch {
			return false;
		}
	}

	/**
	 * Auto-register the origin server as a Chatto server.
	 *
	 * When `knownServer` is true (e.g., cookie-authenticated user), registers
	 * synchronously with a placeholder name — the store's serverInfo.init()
	 * fetches the real name.
	 *
	 * When `knownServer` is false, probes /api/server first. If it responds,
	 * the origin is a Chatto server — register it. If it fails (static hosting),
	 * nothing happens.
	 *
	 * No-ops if the origin is already registered (e.g., from localStorage).
	 */
	probeOrigin(knownServer = false): void {
		if (typeof window === 'undefined') return;
		if (this.originServer) {
			this.originProbed = true;
			return; // Already registered
		}

		const origin = window.location.origin;

		if (knownServer) {
			// Synchronous registration — we already know it's a Chatto server
			const id = generateServerId(origin, this.servers.map((s) => s.id));
			this.#registerOrigin(id, origin, 'Chatto', null);
			this.originProbed = true;
			return;
		}

		// Async probe — detect if the origin is a Chatto server
		fetch(`${origin}/api/server`)
			.then((r) => {
				if (!r.ok) return;
				return r.json();
			})
			.then((data) => {
				if (!data) return;
				if (this.originServer) return; // Registered while we were fetching

				const id = generateServerId(origin, this.servers.map((s) => s.id));
				this.#registerOrigin(id, origin, data.name || 'Chatto', data.iconUrl ?? null);
			})
			.catch(() => {
				// Not a Chatto server — ignore
			})
			.finally(() => {
				this.originProbed = true;
			});
	}

	#registerOrigin(id: string, url: string, name: string, iconUrl: string | null): void {
		this.addServer({
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
	 * Bootstrap the registry: create stores for all registered servers.
	 * Call once from the root layout's script init (before any $derived reads stores).
	 */
	init(): void {
		for (const server of this.servers) {
			if (!this.#stores.has(server.id)) {
				this.#createStore(server);
			}
		}
	}

	/** Add a server to the registry, create its state store, and start its event bus. */
	addServer(server: RegisteredServer): void {
		if (this.servers.some((s) => s.id === server.id)) {
			return; // Already exists
		}
		this.servers.push(server);
		serversSlot.set(this.servers);
		const store = this.#createStore(server);

		// Start the event bus eagerly for already-authenticated servers so
		// child components (ServerSpaceSection) can register handlers during
		// their mount lifecycle. For cookie-auth servers the user is loaded
		// asynchronously by AuthenticatedChatProvider, so the layout's $effect
		// starts the bus once `isAuthenticated` flips true.
		if (store.isAuthenticated) {
			const gqlClient = graphqlClientManager.getClient(server.id);
			eventBusManager.startBus(server.id, gqlClient);
		}
	}

	/** Remove a server by ID. Disposes its event bus, store, and GraphQL client. */
	removeServer(id: string): boolean {
		const server = this.servers.find((s) => s.id === id);
		if (!server) {
			return false;
		}

		// Stop event bus subscription
		eventBusManager.stopBus(id);

		// Dispose state store
		this.#stores.get(id)?.dispose();
		this.#stores.delete(id);

		// Dispose GraphQL client
		graphqlClientManager.destroyClient(id);
		clearAssetProxyCache(id);

		this.servers = this.servers.filter((s) => s.id !== id);
		serversSlot.set(this.servers);
		return true;
	}

	/** Remove all servers. Used by the global sign-out flow.
	 *  Clears dismissals so the origin can be re-discovered on next visit. */
	removeAll(): void {
		for (const server of [...this.servers]) {
			eventBusManager.stopBus(server.id);
			this.#stores.get(server.id)?.dispose();
			this.#stores.delete(server.id);
			graphqlClientManager.destroyClient(server.id);
		}
		clearAssetProxyCache();
		this.servers = [];
		serversSlot.set(this.servers);
	}

	/** Update fields on an existing server. */
	updateServer(id: string, data: Partial<Omit<RegisteredServer, 'id'>>): boolean {
		const server = this.servers.find((s) => s.id === id);
		if (!server) {
			return false;
		}

		Object.assign(server, data);
		serversSlot.set(this.servers);
		return true;
	}

	/** Get a server by ID. */
	getServer(id: string): RegisteredServer | undefined {
		return this.servers.find((s) => s.id === id);
	}

	/**
	 * Get the state store for a registered server.
	 * Safe in $derived — stores are created atomically with registration,
	 * so every registered server always has a store.
	 */
	getStore(serverId: string): ServerStateStore {
		const store = this.#stores.get(serverId);
		if (!store) {
			throw new Error(
				`No store for server "${serverId}". Is it registered? ` +
					`Call serverRegistry.init() before accessing stores.`
			);
		}
		return store;
	}

	/**
	 * Get the state store for a registered server, or undefined if not found.
	 * Use when the server may not be registered (e.g., unresolved URL segments).
	 */
	tryGetStore(serverId: string): ServerStateStore | undefined {
		return this.#stores.get(serverId);
	}

	/** Create a state store for a server and wire up remote user sync. */
	#createStore(server: RegisteredServer): ServerStateStore {
		const gqlClient = graphqlClientManager.getClient(server.id);
		const store = new ServerStateStore(server, gqlClient);
		this.#stores.set(server.id, store);

		// Eagerly fetch server info (name, MOTD, upload limits, etc.).
		// This is important for late-registered servers (e.g., origin registered
		// in the chat layout after the root layout script already ran).
		// init() is fail-soft (catches its own errors) but defensively swallow
		// any unexpected rejection so it can never become an unhandled rejection.
		store.serverInfo.init().catch((err) => {
			console.error(
				`[server:${server.url}] unexpected init() rejection`,
				err
			);
		});

		if (server.token === null) {
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
						this.updateServer(server.id, {
							userId: user.id,
							userLogin: user.login,
							userDisplayName: user.displayName,
							userAvatarUrl: user.avatarUrl
						});
					}
				})
				.catch((err) => {
					console.error(
						`[server:${server.url}] failed to load current user`,
						err
					);
					store.currentUser.loading = false;
				});
		}

		return store;
	}

	/** Whether the server has an authenticated user. False if not registered. */
	isAuthenticated(serverId: string): boolean {
		return this.tryGetStore(serverId)?.isAuthenticated ?? false;
	}
}

export const serverRegistry = new ServerRegistry();
