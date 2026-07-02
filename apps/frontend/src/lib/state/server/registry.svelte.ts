import { SvelteMap } from 'svelte/reactivity';
import { ServerStateStore } from './store.svelte';
import { serverConnectionManager } from './serverConnection.svelte';
import { eventBusManager } from './eventBus.svelte';
import { Codecs, globalSlot } from '$lib/storage/slot';
import { clearAssetProxyCache } from '$lib/pwa/assetProxy';
import { getPublicServerInfo } from '$lib/api-client/server';

/**
 * A registered Chatto server in the multi-server client.
 */
export interface RegisteredServer {
	/** Local slug (derived from hostname, e.g., "chat-example-com") */
	id: string;
	/** Base URL (e.g., "https://chat.example.com") */
	url: string;
	/** Server display name (fetched from ServerDiscoveryService.GetServer) */
	name: string;
	/** Server icon URL, or null if none */
	iconUrl: string | null;
	/** Bearer token for API auth, or null when unauthenticated/legacy cookie auth */
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

export interface AuthenticatedUserSummary {
	id: string;
	login: string;
	displayName?: string | null;
	avatarUrl?: string | null;
}

/**
 * Generate a URL-safe server ID from a base URL.
 * Extracts the hostname and replaces dots/colons with hyphens.
 * If the ID already exists in `existingIds`, appends a numeric suffix.
 */
export function generateServerId(url: string, existingIds: string[] = []): string {
	let hostname: string;
	try {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- this helper parses an input string once; no reactive URL state is needed
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
	 * When `knownServer` is false, probes ServerDiscoveryService.GetServer first.
	 * If it responds, the origin is a Chatto server — register it. If it fails
	 * (static hosting), nothing happens.
	 *
	 * No-ops if the origin is already registered (e.g., from localStorage).
	 */
	probeOrigin(knownServer = false): void {
		if (typeof window === 'undefined') return;
		if (this.originServer) {
			this.originProbed = true;
			if (!knownServer) {
				this.settleOriginUnauthenticated();
			}
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
		getPublicServerInfo(origin)
			.then((info) => {
				if (this.originServer) return; // Registered while we were fetching

				const id = generateServerId(
					origin,
					this.servers.map((s) => s.id)
				);
				this.#registerOrigin(id, origin, info.name || 'Chatto', info.iconUrl ?? null);
				this.settleOriginUnauthenticated();
			})
			.catch(() => {
				// Not a Chatto server — ignore
			})
			.finally(() => {
				this.originProbed = true;
			});
	}

	#registerOrigin(
		id: string,
		url: string,
		name: string,
		iconUrl: string | null,
		token: string | null = null,
		user: AuthenticatedUserSummary | null = null
	): void {
		this.addServer({
			id,
			url,
			name,
			iconUrl,
			token,
			userId: user?.id ?? null,
			userLogin: user?.login ?? null,
			userDisplayName: user?.displayName ?? user?.login ?? null,
			userAvatarUrl: user?.avatarUrl ?? null,
			addedAt: Date.now()
		});
	}

	authenticateOrigin(token: string, user: AuthenticatedUserSummary | null = null): void {
		if (typeof window === 'undefined') return;
		const origin = this.originServer;
		if (!origin) {
			const originUrl = window.location.origin;
			const id = generateServerId(originUrl, this.servers.map((s) => s.id));
			this.#registerOrigin(id, originUrl, 'Chatto', null, token, user);
			this.originProbed = true;
			return;
		}

		this.#replaceServerAuth(origin.id, {
			token,
			userId: user?.id ?? origin.userId,
			userLogin: user?.login ?? origin.userLogin,
			userDisplayName: user?.displayName ?? user?.login ?? origin.userDisplayName,
			userAvatarUrl: user?.avatarUrl ?? origin.userAvatarUrl
		});
		this.originProbed = true;
	}

	/** Settle the origin cookie-auth store when root load found no user. */
	settleOriginUnauthenticated(): void {
		const origin = this.originServer;
		if (!origin) return;
		if (origin.token !== null) return;
		const store = this.tryGetStore(origin.id);
		if (!store) return;
		store.currentUser.user = undefined;
		store.currentUser.loading = false;
	}

	clearServerAuthentication(id: string): void {
		const server = this.getServer(id);
		if (!server) return;
		this.#replaceServerAuth(id, {
			token: null,
			userId: null,
			userLogin: null,
			userDisplayName: null,
			userAvatarUrl: null
		});
		const store = this.tryGetStore(id);
		if (store) {
			store.currentUser.user = undefined;
			store.currentUser.loading = false;
		}
	}

	clearOriginAuthentication(): void {
		const origin = this.originServer;
		if (!origin) return;
		this.clearServerAuthentication(origin.id);
	}

	handleAuthenticationRequired(id: string): void {
		const server = this.getServer(id);
		if (!server) return;
		const isOrigin = this.isOriginServer(id);
		if (isOrigin) {
			this.clearServerAuthentication(id);
		} else {
			this.removeServer(id);
		}
		if (isOrigin && typeof window !== 'undefined') {
			sessionStorage.setItem('returnUrl', window.location.pathname + window.location.search);
			window.location.href = '/';
		}
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
		// child components (ServerSidebarEntry) can register handlers during
		// their mount lifecycle. For cookie-auth servers the user is loaded
		// asynchronously by AuthenticatedChatProvider, so the root layout's
		// existing bus-start effect
		// starts the bus once `isAuthenticated` flips true.
		if (store.isAuthenticated) {
			const serverConnection = serverConnectionManager.getClient(server.id);
			eventBusManager.startBus(server.id, serverConnection);
		}
	}

	/** Remove a server by ID. Disposes its event bus, store, and connection state. */
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

		// Dispose connection state
		serverConnectionManager.destroyClient(id);
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
			serverConnectionManager.destroyClient(server.id);
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

	#replaceServerAuth(
		id: string,
		data: Pick<
			RegisteredServer,
			'token' | 'userId' | 'userLogin' | 'userDisplayName' | 'userAvatarUrl'
		>
	): boolean {
		const server = this.servers.find((s) => s.id === id);
		if (!server) return false;

		eventBusManager.stopBus(id);
		this.#stores.get(id)?.dispose();
		this.#stores.delete(id);
		serverConnectionManager.destroyClient(id);
		clearAssetProxyCache(id);

		Object.assign(server, data);
		serversSlot.set(this.servers);
		this.#createStore(server);
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
		const serverConnection = serverConnectionManager.getClient(server.id);
		const store = new ServerStateStore(server, serverConnection);
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
			// auth state. AuthenticatedChatProvider sets authenticated state;
			// root load/probe settles unauthenticated state. Leave loading true
			// here so route guards cannot observe a transient "no user" gap.
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
