import { SvelteMap, SvelteURL } from 'svelte/reactivity';
import { ServerStateStore } from './store.svelte';
import { serverConnectionManager } from './serverConnection.svelte';
import { eventBusManager } from './eventBus.svelte';
import { Codecs, globalSlot, serverSlot } from '$lib/storage/slot';
import { getPublicServerInfo } from '$lib/api-client/server';
import type { PublicServerInfo } from '$lib/api-client/server';
import { removeRegisteredServerQueries } from '$lib/query/cacheRegistry';
import { isBackendCapableOrigin } from '$lib/runtimeOrigin';
import {
	ServerCatalog,
	type ServerRegistration,
	type ServerRegistrationMetadataPatch
} from './catalog.svelte';
import { emptyServerSession, ServerSessions, type ServerSession } from './sessions.svelte';
import {
	oauthBearerSession,
	persistedBearerSession,
	type NewBearerSession
} from '$lib/auth/bearerSession';

export type { ServerRegistration } from './catalog.svelte';
export type { ServerSession } from './sessions.svelte';

/**
 * A registered Chatto server in the multi-server client.
 */
export interface RegisteredServer extends ServerRegistration, ServerSession {
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
	/** Epoch ms when this server last rejected auth, or null when auth is usable */
	reauthRequiredAt: number | null;
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
		hostname = new SvelteURL(url).hostname;
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
type PersistedRegisteredServer = RegisteredServer & { source?: 'local' | 'synced' };

type PersistedServerAuthentication = {
	version: 1;
	token: string | null;
	refreshToken: string | null;
	accessTokenExpiresAt: number | null;
	refreshTokenExpiresAt: number | null;
	oauthClientId: string | null;
	refreshRequestId: string | null;
	reauthRequiredAt: number | null;
};

type ServerAuthentication = Omit<PersistedServerAuthentication, 'version'>;

function normalizeRegisteredServer(server: PersistedRegisteredServer): RegisteredServer {
	const { source: _retiredSource, ...local } = server;
	return {
		...emptyServerSession(),
		...local,
		iconUrl: server.iconUrl ?? null,
		reauthRequiredAt: server.reauthRequiredAt ?? null
	};
}

function isOptionalNullableString(value: unknown): boolean {
	return value === undefined || value === null || typeof value === 'string';
}

function isOptionalNullableNumber(value: unknown): boolean {
	return (
		value === undefined || value === null || (typeof value === 'number' && Number.isFinite(value))
	);
}

function isNullableString(value: unknown): value is string | null {
	return value === null || typeof value === 'string';
}

function isNullableNumber(value: unknown): value is number | null {
	return value === null || (typeof value === 'number' && Number.isFinite(value));
}

function isPersistedServerAuthentication(value: unknown): value is PersistedServerAuthentication {
	if (typeof value !== 'object' || value === null) return false;
	const authentication = value as Record<string, unknown>;
	return (
		authentication.version === 1 &&
		isNullableString(authentication.token) &&
		isNullableString(authentication.refreshToken) &&
		isNullableNumber(authentication.accessTokenExpiresAt) &&
		isNullableNumber(authentication.refreshTokenExpiresAt) &&
		isNullableString(authentication.oauthClientId) &&
		isNullableString(authentication.refreshRequestId) &&
		isNullableNumber(authentication.reauthRequiredAt)
	);
}

function isPersistedServer(value: unknown): value is PersistedRegisteredServer {
	if (typeof value !== 'object' || value === null) return false;
	const server = value as Record<string, unknown>;
	if (
		typeof server.id !== 'string' ||
		server.id.length === 0 ||
		typeof server.url !== 'string' ||
		typeof server.name !== 'string' ||
		typeof server.addedAt !== 'number' ||
		!Number.isFinite(server.addedAt) ||
		!isOptionalNullableString(server.iconUrl) ||
		!isOptionalNullableString(server.token) ||
		!isOptionalNullableString(server.refreshToken) ||
		!isOptionalNullableNumber(server.accessTokenExpiresAt) ||
		!isOptionalNullableNumber(server.refreshTokenExpiresAt) ||
		!isOptionalNullableString(server.oauthClientId) ||
		!isOptionalNullableString(server.refreshRequestId) ||
		!isOptionalNullableString(server.userId) ||
		!isOptionalNullableString(server.userLogin) ||
		!isOptionalNullableString(server.userDisplayName) ||
		!isOptionalNullableString(server.userAvatarUrl) ||
		(server.reauthRequiredAt !== undefined &&
			server.reauthRequiredAt !== null &&
			(typeof server.reauthRequiredAt !== 'number' || !Number.isFinite(server.reauthRequiredAt))) ||
		(server.source !== undefined && server.source !== 'local' && server.source !== 'synced')
	) {
		return false;
	}

	try {
		const url = new URL(server.url);
		return url.protocol === 'http:' || url.protocol === 'https:';
	} catch {
		return false;
	}
}

function isPersistedServerArray(value: unknown): value is PersistedRegisteredServer[] {
	if (!Array.isArray(value) || !value.every(isPersistedServer)) return false;
	return new Set(value.map((server) => server.id)).size === value.length;
}

function registrationFromServer(server: RegisteredServer): ServerRegistration {
	return {
		id: server.id,
		url: server.url,
		name: server.name,
		iconUrl: server.iconUrl,
		addedAt: server.addedAt
	};
}

function sessionFromServer(server: RegisteredServer): ServerSession {
	return {
		token: server.token,
		refreshToken: server.refreshToken ?? null,
		accessTokenExpiresAt: server.accessTokenExpiresAt ?? null,
		refreshTokenExpiresAt: server.refreshTokenExpiresAt ?? null,
		oauthClientId: server.oauthClientId ?? null,
		refreshRequestId: server.refreshRequestId ?? null,
		userId: server.userId,
		userLogin: server.userLogin,
		userDisplayName: server.userDisplayName,
		userAvatarUrl: server.userAvatarUrl,
		reauthRequiredAt: server.reauthRequiredAt
	};
}

function authenticationFromSession(session: ServerSession): ServerAuthentication {
	return {
		token: session.token,
		refreshToken: session.refreshToken ?? null,
		accessTokenExpiresAt: session.accessTokenExpiresAt ?? null,
		refreshTokenExpiresAt: session.refreshTokenExpiresAt ?? null,
		oauthClientId: session.oauthClientId ?? null,
		refreshRequestId: session.refreshRequestId ?? null,
		reauthRequiredAt: session.reauthRequiredAt
	};
}

function emptyServerAuthentication(): ServerAuthentication {
	return authenticationFromSession(emptyServerSession());
}

/** Split the legacy combined persistence shape into its runtime owners. */
export function splitPersistedServers(servers: PersistedRegisteredServer[]): {
	registrations: ServerRegistration[];
	sessions: Array<readonly [string, ServerSession]>;
} {
	const normalized = servers.map(normalizeRegisteredServer);
	return {
		registrations: normalized.map(registrationFromServer),
		sessions: normalized.map((server) => [server.id, sessionFromServer(server)] as const)
	};
}

const serversSlot = globalSlot(
	'instances',
	[] as PersistedRegisteredServer[],
	Codecs.json<PersistedRegisteredServer[]>(isPersistedServerArray)
);

const serverAuthenticationCodec = Codecs.json<PersistedServerAuthentication | null>(
	(value): value is PersistedServerAuthentication | null =>
		value === null || isPersistedServerAuthentication(value)
);

function authenticationSlot(serverId: string) {
	return serverSlot<PersistedServerAuthentication | null>(
		serverId,
		'authentication',
		null,
		serverAuthenticationCodec
	);
}

/**
 * Read a server's independently keyed authentication state. `undefined` means
 * the legacy combined record has not been migrated; `null` means a present
 * record was corrupt and must not fall back to possibly stale credentials.
 */
function readPersistedAuthentication(serverId: string): ServerAuthentication | null | undefined {
	const slot = authenticationSlot(serverId);
	if (typeof localStorage === 'undefined') return undefined;
	try {
		if (localStorage.getItem(slot.key) === null) return undefined;
	} catch {
		return null;
	}
	const stored = slot.get();
	if (!stored) return null;
	const { version: _version, ...authentication } = stored;
	return authentication;
}

function persistAuthentication(serverId: string, authentication: ServerAuthentication): boolean {
	const slot = authenticationSlot(serverId);
	slot.set({ version: 1, ...authentication });
	const stored = readPersistedAuthentication(serverId);
	return (
		stored !== undefined &&
		stored !== null &&
		stored.token === authentication.token &&
		stored.refreshToken === authentication.refreshToken &&
		stored.accessTokenExpiresAt === authentication.accessTokenExpiresAt &&
		stored.refreshTokenExpiresAt === authentication.refreshTokenExpiresAt &&
		stored.oauthClientId === authentication.oauthClientId &&
		stored.refreshRequestId === authentication.refreshRequestId &&
		stored.reauthRequiredAt === authentication.reauthRequiredAt
	);
}

const RETIRED_ACCOUNT_DATA_KEYS = [
	'chatto:account-data:authorization',
	'chatto:account-data:device-id',
	'chatto:account-data:tinybase'
];

function clearRetiredAccountDataStorage(): void {
	if (typeof localStorage === 'undefined') return;
	try {
		for (const key of RETIRED_ACCOUNT_DATA_KEYS) localStorage.removeItem(key);
	} catch {
		// Browser storage can be unavailable in privacy-restricted contexts.
	}
}

/** Read and split the legacy combined storage shape used at registry construction. */
export function restorePersistedServerState(): ReturnType<typeof splitPersistedServers> {
	const stored = serversSlot.get();
	const normalized = stored.map(normalizeRegisteredServer);
	if (stored.some((server) => server.source !== undefined)) {
		serversSlot.set(normalized);
	}
	const persisted = splitPersistedServers(normalized);
	for (const [serverId, session] of persisted.sessions) {
		const authentication = readPersistedAuthentication(serverId);
		if (authentication === undefined) {
			persistAuthentication(serverId, authenticationFromSession(session));
		} else {
			Object.assign(session, authentication ?? emptyServerAuthentication());
		}
	}
	clearRetiredAccountDataStorage();
	return persisted;
}

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
	readonly catalog: ServerCatalog;
	readonly sessions: ServerSessions;
	#stores = new SvelteMap<string, ServerStateStore>();
	#renewalPromises = new Map<string, Promise<string | null>>();
	#originProbe: Promise<void> | null = null;

	constructor() {
		const persisted = restorePersistedServerState();
		this.catalog = new ServerCatalog(persisted.registrations);
		this.sessions = new ServerSessions(persisted.sessions);
	}

	/** Composed compatibility view for cross-server rendering and commands. */
	get servers(): RegisteredServer[] {
		return this.catalog.registrations.map((registration) => ({
			...registration,
			...(this.sessions.get(registration.id) ?? emptyServerSession())
		}));
	}

	/** Device-local public metadata for the servers known to this client. */
	get registrations(): ServerRegistration[] {
		return this.catalog.registrations;
	}

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
	async probeOrigin(
		knownServer = false,
		location?: Pick<Location, 'origin' | 'protocol'> | URL,
		discoveredServerInfo?: PublicServerInfo
	): Promise<void> {
		if (typeof window === 'undefined') return;
		const currentLocation = location ?? window.location;
		if (!isBackendCapableOrigin(currentLocation)) {
			this.originProbed = true;
			return;
		}
		if (this.originServer) {
			this.originProbed = true;
			if (!knownServer) {
				this.settleOriginUnauthenticated();
			}
			return; // Already registered
		}

		const origin = currentLocation.origin;

		if (knownServer) {
			// Synchronous registration — we already know it's a Chatto server
			const id = generateServerId(
				origin,
				this.servers.map((s) => s.id)
			);
			this.#registerOrigin(id, origin, 'Chatto', null);
			this.originProbed = true;
			return;
		}

		// Root layout load already retrieves this data for the public shell. Reuse
		// that result so route bootstrap does not issue a second discovery request.
		if (discoveredServerInfo !== undefined) {
			const id = generateServerId(
				origin,
				this.servers.map((s) => s.id)
			);
			this.#registerOrigin(
				id,
				origin,
				discoveredServerInfo.name || 'Chatto',
				discoveredServerInfo.iconUrl ?? null
			);
			this.settleOriginUnauthenticated();
			this.originProbed = true;
			return;
		}

		if (this.#originProbe) {
			await this.#originProbe;
			return;
		}

		// Async probe — detect if the origin is a Chatto server
		const probe = getPublicServerInfo(origin)
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
				if (this.#originProbe === probe) this.#originProbe = null;
			});
		this.#originProbe = probe;
		await probe;
	}

	#registerOrigin(
		id: string,
		url: string,
		name: string,
		iconUrl: string | null,
		credentials: string | NewBearerSession | null = null,
		user: AuthenticatedUserSummary | null = null
	): void {
		this.addServer(
			{
				id,
				url,
				name,
				iconUrl,
				addedAt: Date.now()
			},
			{
				...(typeof credentials === 'string'
					? { token: credentials }
					: credentials
						? persistedBearerSession(credentials)
						: { token: null }),
				userId: user?.id ?? null,
				userLogin: user?.login ?? null,
				userDisplayName: user?.displayName ?? user?.login ?? null,
				userAvatarUrl: user?.avatarUrl ?? null,
				reauthRequiredAt: null
			}
		);
	}

	/** Install origin cookie authentication and discard any legacy origin bearer session. */
	authenticateOriginCookie(user: AuthenticatedUserSummary | null = null): void {
		if (typeof window === 'undefined') return;
		const origin = this.originServer;
		if (!origin) {
			const originUrl = window.location.origin;
			const id = generateServerId(
				originUrl,
				this.servers.map((s) => s.id)
			);
			this.#registerOrigin(id, originUrl, 'Chatto', null, null, user);
			this.originProbed = true;
			return;
		}

		const cookieSession: ServerSession = {
			token: null,
			refreshToken: null,
			accessTokenExpiresAt: null,
			refreshTokenExpiresAt: null,
			oauthClientId: null,
			refreshRequestId: null,
			userId: user?.id ?? origin.userId,
			userLogin: user?.login ?? origin.userLogin,
			userDisplayName: user?.displayName ?? user?.login ?? origin.userDisplayName,
			userAvatarUrl: user?.avatarUrl ?? origin.userAvatarUrl,
			reauthRequiredAt: null
		};
		if (
			origin.token === null &&
			origin.refreshToken === null &&
			origin.accessTokenExpiresAt === null &&
			origin.refreshTokenExpiresAt === null &&
			origin.oauthClientId === null &&
			origin.refreshRequestId === null
		) {
			this.sessions.replace(origin.id, cookieSession);
			this.#persistAuthentication(origin.id);
			this.#persist();
		} else {
			this.#replaceServerAuth(origin.id, cookieSession);
		}
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
			refreshToken: null,
			accessTokenExpiresAt: null,
			refreshTokenExpiresAt: null,
			oauthClientId: null,
			refreshRequestId: null,
			userId: null,
			userLogin: null,
			userDisplayName: null,
			userAvatarUrl: null,
			reauthRequiredAt: null
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
		const session = this.sessions.get(id);
		if (!session || session.reauthRequiredAt !== null) return;

		eventBusManager.stopBus(id);
		removeRegisteredServerQueries(id);
		this.sessions.update(id, { reauthRequiredAt: Date.now() });
		this.#persistAuthenticationPatch(id, {
			reauthRequiredAt: this.sessions.get(id)?.reauthRequiredAt ?? null
		});
		this.#persist();
		const store = this.tryGetStore(id);
		if (store) {
			store.currentUser.loading = false;
		}
	}

	clearAuthenticationRequired(id: string): void {
		const session = this.sessions.get(id);
		if (!session || session.reauthRequiredAt === null) return;
		this.sessions.update(id, { reauthRequiredAt: null });
		this.#persistAuthenticationPatch(id, { reauthRequiredAt: null });
		this.#persist();
	}

	/** Return a usable access token, rotating the persisted pair when needed. */
	renewServerAuthentication(id: string, force = false): Promise<string | null> {
		const existing = this.#renewalPromises.get(id);
		if (existing) {
			if (!force) return existing;
			const tokenBeforeWait = this.sessions.get(id)?.token ?? null;
			return existing.then((token) => {
				if (!token || token !== tokenBeforeWait) return token;
				return this.renewServerAuthentication(id, true);
			});
		}
		const renewal = this.#renewServerAuthentication(id, force).finally(() => {
			if (this.#renewalPromises.get(id) === renewal) this.#renewalPromises.delete(id);
		});
		this.#renewalPromises.set(id, renewal);
		return renewal;
	}

	async #renewServerAuthentication(id: string, force: boolean): Promise<string | null> {
		const originalToken = this.sessions.get(id)?.token ?? null;
		return this.#withRefreshLock(id, async () => {
			this.#adoptPersistedBearerSession(id);
			let session = this.sessions.get(id);
			const registration = this.catalog.get(id);
			if (!session || !registration || !session.token || !session.refreshToken) {
				this.handleAuthenticationRequired(id);
				return null;
			}
			if (session.reauthRequiredAt !== null) return null;

			if (session.token !== originalToken) return session.token;
			if (!force && (session.accessTokenExpiresAt ?? 0) > Date.now()) {
				return session.token;
			}

			const requestId = session.refreshRequestId || crypto.randomUUID();
			if (session.refreshRequestId !== requestId) {
				this.sessions.update(id, { refreshRequestId: requestId });
				session = this.sessions.get(id);
			}
			if (!session?.refreshToken) {
				this.handleAuthenticationRequired(id);
				return null;
			}
			// Rotation is unsafe unless a lost response can be retried with the
			// exact same ID after a reload or in another tab. StorageSlot writes
			// are intentionally best-effort elsewhere, so verify this security-
			// sensitive write before sending the refresh credential.
			this.#persistAuthentication(id);
			this.#persist();
			const persistedRequestId = readPersistedAuthentication(id)?.refreshRequestId;
			if (persistedRequestId !== requestId) {
				throw new Error('Unable to persist bearer renewal state.');
			}

			const response = await fetch(new URL('/oauth/token', registration.url), {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					grant_type: 'refresh_token',
					refresh_token: session.refreshToken,
					refresh_request_id: requestId,
					client_id: session.oauthClientId ?? ''
				}),
				signal: AbortSignal.timeout(10_000)
			});
			const body: Record<string, unknown> = await response.json().catch(() => ({}));
			if (!response.ok) {
				if (response.status === 400 && body.error === 'invalid_grant') {
					this.handleAuthenticationRequired(id);
					return null;
				}
				throw new Error(
					typeof body.error_description === 'string'
						? body.error_description
						: `Bearer session renewal failed (${response.status})`
				);
			}

			const credentials = oauthBearerSession(body, session.oauthClientId ?? null);
			if (!credentials) throw new Error('The server returned an invalid bearer session.');
			this.#updateBearerSessionInPlace(id, {
				...persistedBearerSession(credentials),
				reauthRequiredAt: null
			});
			return credentials.token;
		});
	}

	async #withRefreshLock<T>(id: string, operation: () => Promise<T>): Promise<T> {
		if (typeof navigator !== 'undefined' && navigator.locks) {
			return navigator.locks.request(`chatto:bearer-refresh:${id}`, operation);
		}
		return operation();
	}

	#adoptPersistedBearerSession(id: string): void {
		const persisted = readPersistedAuthentication(id);
		if (!persisted) return;
		const current = this.sessions.get(id);
		if (!current || !persisted.token) return;
		if (
			persisted.token === current.token &&
			persisted.refreshToken === current.refreshToken &&
			persisted.accessTokenExpiresAt === current.accessTokenExpiresAt &&
			persisted.refreshTokenExpiresAt === current.refreshTokenExpiresAt &&
			persisted.oauthClientId === current.oauthClientId &&
			persisted.refreshRequestId === current.refreshRequestId &&
			persisted.reauthRequiredAt === current.reauthRequiredAt
		) {
			return;
		}
		this.#updateBearerSessionInPlace(
			id,
			{
				token: persisted.token,
				refreshToken: persisted.refreshToken,
				accessTokenExpiresAt: persisted.accessTokenExpiresAt,
				refreshTokenExpiresAt: persisted.refreshTokenExpiresAt,
				oauthClientId: persisted.oauthClientId,
				refreshRequestId: persisted.refreshRequestId,
				reauthRequiredAt: persisted.reauthRequiredAt
			},
			false
		);
	}

	#updateBearerSessionInPlace(id: string, data: Partial<ServerSession>, persist = true): void {
		if (!this.sessions.update(id, data)) return;
		if (persist) this.#persistAuthentication(id);
		this.#persist();
		serverConnectionManager.updateBearerSession(id);
	}

	/**
	 * Bootstrap the registry: create stores for all registered servers.
	 * Call once from the root layout's script init (before any $derived reads stores).
	 */
	init(): void {
		for (const registration of this.registrations) {
			if (!this.#stores.has(registration.id)) {
				this.#createStore(registration.id);
			}
		}
	}

	/** Add a server and create its retained state store. Transport ownership is centralized. */
	addServer(registration: ServerRegistration | RegisteredServer, session?: ServerSession): void {
		const publicRegistration: ServerRegistration = {
			id: registration.id,
			url: registration.url,
			name: registration.name,
			iconUrl: registration.iconUrl,
			addedAt: registration.addedAt
		};
		const localSession =
			session ?? ('token' in registration ? sessionFromServer(registration) : emptyServerSession());
		if (!this.catalog.add(publicRegistration)) return;
		this.sessions.replace(registration.id, localSession);
		this.#persistAuthentication(registration.id);
		this.#persist();
		this.#createStore(registration.id);
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

		this.sessions.remove(id);
		this.catalog.remove(id);
		persistAuthentication(id, emptyServerAuthentication());
		this.#persist();
		return true;
	}

	/** Remove all local registrations and sessions without synchronizing deletions. */
	removeAll(): void {
		const ids = this.servers.map((server) => server.id);
		this.#disposeServers(ids);
		for (const id of ids) persistAuthentication(id, emptyServerAuthentication());
		this.sessions.clear();
		this.catalog.reset();
		this.#persist();
	}

	/** Clear every session and remote registration while retaining the configured origin. */
	resetToOrigin(): void {
		const origin = this.originServer;
		const ids = this.servers.map((server) => server.id);
		this.#disposeServers(ids);
		for (const id of ids) persistAuthentication(id, emptyServerAuthentication());
		this.sessions.clear();
		this.catalog.reset(origin ? [registrationFromServer(origin)] : []);
		if (origin) {
			this.sessions.ensure(origin.id);
			this.#persistAuthentication(origin.id);
			this.#createStore(origin.id);
			this.settleOriginUnauthenticated();
		}
		this.#persist();
	}

	#disposeServers(ids: string[]): void {
		for (const id of ids) {
			eventBusManager.stopBus(id);
			this.#stores.get(id)?.dispose();
			this.#stores.delete(id);
			serverConnectionManager.destroyClient(id);
		}
	}

	/** Update device-local public metadata without touching the local session. */
	updateRegistration(id: string, data: ServerRegistrationMetadataPatch): boolean {
		if (!this.catalog.update(id, data)) return false;
		this.#persist();
		return true;
	}

	replaceServerAuthentication(
		id: string,
		data: Pick<
			RegisteredServer,
			| 'token'
			| 'refreshToken'
			| 'accessTokenExpiresAt'
			| 'refreshTokenExpiresAt'
			| 'oauthClientId'
			| 'refreshRequestId'
			| 'userId'
			| 'userLogin'
			| 'userDisplayName'
			| 'userAvatarUrl'
			| 'reauthRequiredAt'
		>
	): boolean {
		return this.#replaceServerAuth(id, data);
	}

	#replaceServerAuth(
		id: string,
		data: Pick<
			RegisteredServer,
			| 'token'
			| 'refreshToken'
			| 'accessTokenExpiresAt'
			| 'refreshTokenExpiresAt'
			| 'oauthClientId'
			| 'refreshRequestId'
			| 'userId'
			| 'userLogin'
			| 'userDisplayName'
			| 'userAvatarUrl'
			| 'reauthRequiredAt'
		>
	): boolean {
		if (!this.catalog.get(id) || !this.sessions.get(id)) return false;

		eventBusManager.stopBus(id);
		this.#stores.get(id)?.dispose();
		this.#stores.delete(id);
		serverConnectionManager.destroyClient(id);

		this.sessions.replace(id, data);
		this.#persistAuthentication(id);
		this.#persist();
		this.#createStore(id);
		return true;
	}

	#persist(): void {
		// The combined record remains a migration/compatibility adapter. Merge
		// independently persisted authentication at write time so a stale tab's
		// metadata snapshot can never put old rotated credentials back into it.
		serversSlot.set(
			this.servers.map((server) => {
				const persisted = readPersistedAuthentication(server.id);
				return {
					...server,
					...(persisted === undefined
						? authenticationFromSession(server)
						: (persisted ?? emptyServerAuthentication()))
				};
			})
		);
	}

	#persistAuthentication(id: string): boolean {
		const session = this.sessions.get(id);
		if (!session) return false;
		return persistAuthentication(id, authenticationFromSession(session));
	}

	#persistAuthenticationPatch(id: string, patch: Partial<ServerAuthentication>): boolean {
		const session = this.sessions.get(id);
		if (!session) return false;
		const stored = readPersistedAuthentication(id);
		const current = stored ?? authenticationFromSession(session);
		return persistAuthentication(id, { ...current, ...patch });
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
	#createStore(serverId: string): ServerStateStore {
		const registration = this.catalog.get(serverId);
		if (!registration) throw new Error(`Server "${serverId}" not found in catalogue`);
		const session = this.sessions.ensure(serverId);
		const serverConnection = serverConnectionManager.getClient(serverId);
		const store = new ServerStateStore(
			registration,
			() => this.sessions.ensure(serverId),
			this.isOriginServer(serverId),
			serverConnection,
			undefined,
			() => {
				this.handleAuthenticationRequired(serverId);
			}
		);
		this.#stores.set(serverId, store);

		const serverUrl = registration.url;
		store.serverInfo.init().catch((err) => {
			console.error(`[server:${serverUrl}] unexpected init() rejection`, err);
		});

		if (session.token === null) {
			if (!this.isOriginServer(serverId)) {
				// A remotely synchronized registration carries no credential. It is
				// ready for the normal remote sign-in flow, not cookie discovery.
				store.currentUser.user = undefined;
				store.currentUser.loading = false;
			}
			// Cookie auth on the origin is settled by the root load/probe. Leave it
			// loading here so route guards cannot observe a transient "no user" gap.
		} else {
			// Bearer auth (remote) — auto-load the authenticated user via the token.
			// Catch failures (e.g. unreachable host, CORS) so they don't bubble up
			// as an unhandled rejection and crash the entire client.
			store.currentUser
				.load()
				.then(() => {
					const user = store.currentUser.user;
					if (user) {
						this.sessions.update(serverId, {
							userId: user.id,
							userLogin: user.login,
							userDisplayName: user.displayName,
							userAvatarUrl: user.avatarUrl
						});
						this.#persist();
					}
				})
				.catch((err) => {
					console.error(`[server:${serverUrl}] failed to load current user`, err);
					store.currentUser.loading = false;
				});
		}

		return store;
	}

	/** Whether the server has an authenticated user. False if not registered. */
	isAuthenticated(serverId: string): boolean {
		return this.tryGetStore(serverId)?.isAuthenticated ?? false;
	}

	/** Prefer the origin, then registration order, when choosing a retained session. */
	firstAuthenticatedServerId(excludedId?: string): string | undefined {
		const originId = this.originServer?.id;
		if (originId && originId !== excludedId && this.isAuthenticated(originId)) {
			return originId;
		}

		return this.servers.find(
			(server) => server.id !== excludedId && this.isAuthenticated(server.id)
		)?.id;
	}
}

export const serverRegistry = new ServerRegistry();
