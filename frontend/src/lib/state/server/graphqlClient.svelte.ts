import { Client, fetchExchange, subscriptionExchange, mapExchange } from '@urql/svelte';
import { createClient as createWSClient } from 'graphql-ws';
import { serverRegistry } from './registry.svelte';
import { isAuthenticationRequiredError } from '$lib/auth/errors';

const GRAPHQL_REQUEST_HEADERS = { 'X-REQUEST-TYPE': 'GraphQL' };

/**
 * Delay between WS reconnection attempts. The first attempt after a
 * disconnect is always immediate; subsequent attempts wait this long.
 *
 * The retry loop never gives up: graphql-ws's `shouldRetry` returning
 * false would exit the loop permanently for the client instance (per
 * its source), making `terminate()` a no-op and trapping the client
 * in an unrecoverable disconnected state. Retrying forever costs at
 * most one log line per RETRY_WAIT_MS for genuinely-misconfigured
 * instances, which is a cheap price for guaranteed recovery from
 * transient outages and tab suspensions.
 */
const RETRY_WAIT_MS = 5000;

export type ConnectionStatus = 'connected' | 'connecting' | 'disconnected';

export interface GraphQLClientConfig {
	/** GraphQL HTTP endpoint URL (relative for origin, absolute for remote) */
	url: string;
	/** WebSocket URL (relative for origin, absolute wss:// for remote) */
	wsUrl: string;
	/** Bearer token for GraphQL auth, or null for unauthenticated/legacy cookie auth */
	token: string | null;
	/** Registered server ID, used to clear stale credentials after auth failures */
	serverId?: string;
}

/** Construct a WebSocket URL from an HTTP URL (http→ws, https→wss). */
export function httpToWsUrl(httpUrl: string): string {
	return httpUrl.replace(/^http/, 'ws');
}

function hostFromGraphQLEndpoint(url: string): string {
	if (url.startsWith('/')) {
		return typeof window !== 'undefined' ? window.location.host : 'localhost';
	}
	return url.match(/^[a-z][a-z0-9+.-]*:\/\/([^/?#]+)/i)?.[1] ?? url;
}

function connectBaseUrlFromGraphQLEndpoint(url: string): string {
	if (url.startsWith('/')) {
		const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost';
		return new URL('/api/connect', origin).toString();
	}
	const parsed = new URL(url);
	return new URL('/api/connect', parsed.origin).toString();
}

const HOME_URL = '/api/graphql';

export class GraphQLClient {
	status = $state<ConnectionStatus>('connecting');
	reconnectCount = $state(0);
	#failedAttempts = $state(0);
	client: Client;
	#wsClient: ReturnType<typeof createWSClient>;
	#activeSocket: WebSocket | null = null;
	#pongTimeoutId: ReturnType<typeof setTimeout> | null = null;
	#immediateReconnect = false;
	/**
	 * Resolver for the current `retryWait` promise, set while a retry is
	 * waiting out the inter-attempt delay. `forceReconnect()` calls this
	 * (if set) so the next attempt happens immediately instead of having
	 * to wait out a potentially-stale setTimeout — important after a tab
	 * resume, where a frozen 5s timer can fire many minutes late.
	 */
	#pendingRetryResolve: (() => void) | null = null;
	/**
	 * Set when status flips to 'disconnected' (via `closed:`), cleared when
	 * `connected:` consumes it. Lets us detect "we just reconnected" even
	 * though `connecting:` has since overwritten status away from
	 * 'disconnected'.
	 */
	#wasDisconnected = false;
	#lastVisibleAt = Date.now();
	#visibilityHandler: (() => void) | null = null;
	#onlineHandler: (() => void) | null = null;
	#suspendDetectorInterval: ReturnType<typeof setInterval> | null = null;
	#host: string;
	#connectBaseUrl: string;
	#token: string | null;
	#serverId: string | undefined;

	get isConnected() {
		return this.status === 'connected';
	}

	/** Show disconnection icon immediately when WebSocket is not connected */
	get showConnectionLostIcon() {
		return this.status === 'disconnected';
	}

	/** Show urgent (orange) disconnection indicator after 6 failed reconnection attempts (~30+ seconds) */
	get showConnectionLostBanner() {
		return this.#failedAttempts >= 6;
	}

	get connectBaseUrl(): string {
		return this.#connectBaseUrl;
	}

	get bearerToken(): string | null {
		return this.#token;
	}

	get serverId(): string | undefined {
		return this.#serverId;
	}

	/** Force-terminate and immediately reconnect the WebSocket. */
	forceReconnect(reason: string) {
		// If we're already mid-handshake, killing the in-flight socket just
		// restarts the same work we were about to finish — and on tab resume
		// the visibility handler, suspend detector, and online handler all
		// fire in quick succession, so several forceReconnect calls land back
		// to back. Let the first one win.
		if (this.status === 'connecting') {
			console.log('[ws:%s] Force reconnect skipped — already connecting: %s', this.#host, reason);
			return;
		}
		console.log('[ws:%s] Force reconnect: %s (status: %s)', this.#host, reason, this.status);
		this.#immediateReconnect = true;
		this.#failedAttempts = 0;
		// If a retryWait is currently sleeping, resolve it so the next
		// attempt happens now instead of after the (possibly stale) timer.
		if (this.#pendingRetryResolve) {
			const resolve = this.#pendingRetryResolve;
			this.#pendingRetryResolve = null;
			resolve();
		}
		this.#wsClient.terminate();
	}

	/** Explicit user-initiated retry; equivalent to forceReconnect. */
	retry() {
		this.forceReconnect('user-initiated retry');
	}

	constructor(config: GraphQLClientConfig) {
		const { url, wsUrl, token, serverId } = config;
		this.#host = hostFromGraphQLEndpoint(url);
		this.#connectBaseUrl = connectBaseUrlFromGraphQLEndpoint(url);
		this.#token = token;
		this.#serverId = serverId;

		// Client pings the server every 15s. The `ping` handler starts a 5s
		// pong timeout; if the server doesn't respond, we close the socket.
		// Combined with the server's own 10s ping interval, this gives two
		// independent liveness checks.
		this.#wsClient = createWSClient({
			url: wsUrl,
			keepAlive: 15_000,
			retryAttempts: Infinity,
			// Never give up. Returning false here would exit graphql-ws's
			// retry loop permanently for this client instance, after which
			// `terminate()` is a no-op and the only recovery is to recreate
			// the client. See the RETRY_WAIT_MS comment.
			shouldRetry: () => true,
			...(token ? { connectionParams: () => ({ token }) } : {}),
			retryWait: async (retries) => {
				// Track failed attempts for UI display (banner shows after 6 failures)
				this.#failedAttempts = retries;

				// Skip delay if this is a manual reconnect (e.g., tab became visible)
				if (this.#immediateReconnect) {
					this.#immediateReconnect = false;
					console.log('[ws:%s] Retry attempt %d (immediate)', this.#host, retries);
					return;
				}
				// First attempt: immediate (catches quick server restarts)
				if (retries === 0) {
					console.log('[ws:%s] Retry attempt %d (immediate)', this.#host, retries);
					return;
				}
				// Subsequent attempts wait RETRY_WAIT_MS, but the wait is
				// interruptible by forceReconnect() — that avoids stalling
				// the next attempt behind a frozen-during-sleep setTimeout
				// after a tab resume.
				console.log('[ws:%s] Retry attempt %d (waiting %dms)', this.#host, retries, RETRY_WAIT_MS);
				await new Promise<void>((resolve) => {
					this.#pendingRetryResolve = resolve;
					setTimeout(() => {
						if (this.#pendingRetryResolve === resolve) {
							this.#pendingRetryResolve = null;
						}
						resolve();
					}, RETRY_WAIT_MS);
				});
			},
			on: {
				connecting: () => {
					// Fires after retryWait, when a fresh socket is about to be
					// opened. Move out of 'disconnected' so the mapExchange
					// HTTP-result path doesn't kick another forceReconnect mid-
					// handshake and kill the attempt we just started.
					//
					// Side effect: overwrites 'disconnected' before `connected:`
					// can see it. The standalone `#wasDisconnected` flag below
					// preserves the "did we just reconnect?" signal so the
					// `connected:` handler can still bump `reconnectCount`.
					if (this.status === 'disconnected') {
						this.#wasDisconnected = true;
					}
					console.debug('[ws:%s] Connecting (prev status: %s)', this.#host, this.status);
					this.status = 'connecting';
				},
				ping: (received) => {
					if (received) {
						console.debug('[ws:%s] Server ping received', this.#host);
						return;
					}
					console.debug('[ws:%s] Client ping sent, awaiting pong', this.#host);
					this.#pongTimeoutId = setTimeout(() => {
						if (this.#activeSocket?.readyState === WebSocket.OPEN) {
							console.log(
								'[ws:%s] Pong timeout (no response in 5s), closing socket',
								this.#host
							);
							this.#activeSocket.close(4408, 'Pong Timeout');
						}
					}, 5_000);
				},
				pong: (received) => {
					if (received && this.#pongTimeoutId !== null) {
						console.debug('[ws:%s] Pong received', this.#host);
						clearTimeout(this.#pongTimeoutId);
						this.#pongTimeoutId = null;
					}
				},
				connected: (socket) => {
					this.#activeSocket = socket as WebSocket;
					console.log('[ws:%s] Connected (prev status: %s, wasDisconnected: %s)', this.#host, this.status, this.#wasDisconnected);

					if (this.#wasDisconnected) {
						this.#wasDisconnected = false;
						this.reconnectCount++;
						console.log(
							'[ws:%s] Reconnected (count: %d)',
							this.#host,
							this.reconnectCount
						);
					}
					this.status = 'connected';
					this.#failedAttempts = 0;
				},
				closed: (event) => {
					this.#activeSocket = null;
					if (this.#pongTimeoutId !== null) {
						clearTimeout(this.#pongTimeoutId);
						this.#pongTimeoutId = null;
					}
					const closeEvent = event as CloseEvent | undefined;
					console.log(
						'[ws:%s] Closed (code: %s, reason: %s, prev status: %s)',
						this.#host,
						closeEvent?.code ?? 'unknown',
						closeEvent?.reason || 'none',
						this.status
					);
					if (this.status === 'connected') {
						this.#wasDisconnected = true;
					}
					this.status = 'disconnected';
				},
				error: (err) => console.error('[ws:%s] Error:', this.#host, err)
			}
		});

		this.client = new Client({
			url,
			preferGetMethod: false,
			fetchOptions: () => ({
				// GraphQL does not participate in the double-submit CSRF token
				// flow. Authenticated clients use bearer tokens; the marker keeps
				// legacy cookie-auth fallback requests non-simple.
				headers: token
					? { ...GRAPHQL_REQUEST_HEADERS, Authorization: `Bearer ${token}` }
					: { ...GRAPHQL_REQUEST_HEADERS }
			}),
			exchanges: [
				mapExchange({
					onResult: (result) => {
						// If an HTTP request succeeded but WebSocket is disconnected,
						// the server is reachable — force reconnect the WebSocket
						if (!result.error && this.status === 'disconnected') {
							this.forceReconnect('HTTP request succeeded while WS disconnected');
						}
						if (serverId && result.error && isAuthenticationRequiredError(result.error)) {
							serverRegistry.handleAuthenticationRequired(serverId);
						}

						return result;
					}
				}),
				subscriptionExchange({
					forwardSubscription: (request) => {
						const input = { ...request, query: request.query || '' };
						return {
							subscribe: (sink) => {
								const unsubscribe = this.#wsClient.subscribe(input, sink);
								return { unsubscribe };
							}
						};
					}
				}),
				fetchExchange
			]
		});

		// Reconnect when tab becomes visible after being backgrounded.
		// If the tab was hidden for >30s, force-terminate the WebSocket regardless of
		// reported status. This catches silently-dead connections where the OS killed
		// the socket during sleep but the client never received a close event.
		if (typeof document !== 'undefined') {
			this.#visibilityHandler = () => {
				if (document.visibilityState === 'visible') {
					const hiddenDuration = Date.now() - this.#lastVisibleAt;

					if (this.status === 'disconnected' || hiddenDuration > 30_000) {
						console.debug(
							'[ws:%s] visibility=visible after %ds hidden, status=%s → forceReconnect',
							this.#host,
							Math.round(hiddenDuration / 1000),
							this.status
						);
						this.forceReconnect(
							`tab visible after ${Math.round(hiddenDuration / 1000)}s hidden`
						);
					} else {
						console.debug(
							'[ws:%s] visibility=visible after %ds hidden, status=%s → no reconnect',
							this.#host,
							Math.round(hiddenDuration / 1000),
							this.status
						);
					}

					this.#lastVisibleAt = Date.now();
				} else {
					this.#lastVisibleAt = Date.now();
				}
			};
			document.addEventListener('visibilitychange', this.#visibilityHandler);
		}

		// Detect wake from OS-level sleep/suspend via timer gap. When the JS
		// event loop is frozen (lid close, phone lock), setInterval callbacks
		// don't fire. On wake the first callback fires with a large actual gap.
		//
		// Background-tab throttling produces the same signal (Chrome/Firefox
		// throttle setInterval to ~1/min in hidden tabs), so the gap is only
		// meaningful while the tab is visible. The visibility handler covers
		// the hidden case on resume.
		if (typeof window !== 'undefined') {
			let lastTick = Date.now();
			this.#suspendDetectorInterval = setInterval(() => {
				const now = Date.now();
				const gap = now - lastTick;
				lastTick = now;
				if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
				if (gap > 30_000) {
					console.debug(
						'[ws:%s] Suspend detector fired (timer gap %ds)',
						this.#host,
						Math.round(gap / 1000)
					);
					this.forceReconnect(
						`suspend detected (timer gap: ${Math.round(gap / 1000)}s)`
					);
				}
			}, 10_000);

			// Reconnect when network comes back online (e.g., after airplane mode
			// or Wi-Fi re-association following sleep).
			this.#onlineHandler = () => {
				console.debug('[ws:%s] online event fired', this.#host);
				this.forceReconnect('network came back online');
			};
			window.addEventListener('online', this.#onlineHandler);
		}
	}

	/** Clean up WebSocket connection and event listeners. */
	dispose() {
		if (this.#visibilityHandler && typeof document !== 'undefined') {
			document.removeEventListener('visibilitychange', this.#visibilityHandler);
			this.#visibilityHandler = null;
		}
		if (this.#onlineHandler && typeof window !== 'undefined') {
			window.removeEventListener('online', this.#onlineHandler);
			this.#onlineHandler = null;
		}
		if (this.#suspendDetectorInterval !== null) {
			clearInterval(this.#suspendDetectorInterval);
			this.#suspendDetectorInterval = null;
		}
		this.#wsClient.dispose();
	}
}

/**
 * Manages GraphQL clients for multiple Chatto instances.
 * The origin client is created eagerly; remote clients are created lazily on first access.
 */
class GraphQLClientManager {
	#clients = new Map<string, GraphQLClient>();
	#originClient: GraphQLClient | null = null;
	#originClientToken: string | null = null;
	#originClientServerId: string | undefined;

	/** The origin instance client (serves the SPA, prefers bearer auth when available). */
	get originClient(): GraphQLClient {
		const origin = serverRegistry.originServer;
		const token = origin?.token ?? null;
		const serverId = origin?.id;
		if (
			this.#originClient &&
			this.#originClientToken === token &&
			this.#originClientServerId === serverId
		) {
			return this.#originClient;
		}

		this.#originClient?.dispose();
		this.#originClient = new GraphQLClient({
			url: HOME_URL,
			wsUrl: HOME_URL,
			token,
			serverId
		});
		this.#originClientToken = token;
		this.#originClientServerId = serverId;
		return this.#originClient;
	}

	/** Get or create a client for a registered instance. */
	getClient(serverId: string): GraphQLClient {
		if (serverRegistry.isOriginServer(serverId)) {
			return this.originClient;
		}

		const existing = this.#clients.get(serverId);
		if (existing) return existing;

		const server = serverRegistry.getServer(serverId);
		if (!server) {
			throw new Error(`Server "${serverId}" not found in registry`);
		}

		const url = `${server.url}/api/graphql`;
		const client = new GraphQLClient({
			url,
			wsUrl: httpToWsUrl(url),
			token: server.token,
			serverId
		});

		this.#clients.set(serverId, client);
		return client;
	}

	/** Destroy and remove a client. */
	destroyClient(serverId: string): boolean {
		if (serverRegistry.isOriginServer(serverId)) {
			if (!this.#originClient) return false;
			this.#originClient.dispose();
			this.#originClient = null;
			this.#originClientToken = null;
			this.#originClientServerId = undefined;
			return true;
		}

		const client = this.#clients.get(serverId);
		if (!client) return false;

		client.dispose();
		this.#clients.delete(serverId);
		return true;
	}
}

export const graphqlClientManager = new GraphQLClientManager();
