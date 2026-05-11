import { Client, fetchExchange, subscriptionExchange, mapExchange } from '@urql/svelte';
import { createClient as createWSClient } from 'graphql-ws';
import { serverRegistry } from './registry.svelte';

const SESSION_VALIDATION_COOLDOWN_MS = 5000;

/**
 * Stop retrying the WebSocket connection after this many consecutive failed
 * attempts. With the 5s retry interval, ~30 attempts ≈ 2.5 minutes of
 * trying — enough to ride out transient outages (server restart, brief
 * network loss) but short enough that a permanently misconfigured instance
 * (e.g. CORS, DNS) doesn't spam the console forever.
 *
 * After giving up, a `retry()` call from the UI (or a tab-visibility /
 * online event) is required to start trying again.
 */
const MAX_WS_RETRY_ATTEMPTS = 30;

export type ConnectionStatus = 'connected' | 'connecting' | 'disconnected';

export interface AuthHandlers {
	/** Called when an auth-failure error is detected in a GraphQL response. */
	onAuthFailure?: () => void;
	/** Called on reconnect or when the tab becomes visible (for session re-validation). */
	onSessionValidation?: () => void;
}

export interface GraphQLClientConfig {
	/** GraphQL HTTP endpoint URL (relative for origin, absolute for remote) */
	url: string;
	/** WebSocket URL (relative for origin, absolute wss:// for remote) */
	wsUrl: string;
	/** Bearer token for cross-origin auth, or null to use cookies */
	token: string | null;
}

/** Construct a WebSocket URL from an HTTP URL (http→ws, https→wss). */
export function httpToWsUrl(httpUrl: string): string {
	return httpUrl.replace(/^http/, 'ws');
}

const HOME_URL = '/api/graphql';

export class GraphQLClient {
	status = $state<ConnectionStatus>('connecting');
	reconnectCount = $state(0);
	#failedAttempts = $state(0);
	/**
	 * True after the WS retry loop has given up (exceeded
	 * MAX_WS_RETRY_ATTEMPTS). `shouldRetry` returns false in this state.
	 * Reset to false by `retry()` or by `forceReconnect()`.
	 */
	gaveUp = $state(false);
	client: Client;
	#wsClient: ReturnType<typeof createWSClient>;
	#activeSocket: WebSocket | null = null;
	#pongTimeoutId: ReturnType<typeof setTimeout> | null = null;
	#immediateReconnect = false;
	#lastVisibleAt = Date.now();
	#visibilityHandler: (() => void) | null = null;
	#onlineHandler: (() => void) | null = null;
	#suspendDetectorInterval: ReturnType<typeof setInterval> | null = null;
	#host: string;
	#handlers: AuthHandlers = {};
	#lastSessionValidation = 0;

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

	/** Force-terminate and immediately reconnect the WebSocket. */
	forceReconnect(reason: string) {
		console.log('[ws:%s] Force reconnect: %s (status: %s)', this.#host, reason, this.status);
		this.#immediateReconnect = true;
		this.gaveUp = false;
		this.#failedAttempts = 0;
		this.#wsClient.terminate();
	}

	/**
	 * Explicit user-initiated retry after the WS retry loop gave up. Resets
	 * the give-up flag and triggers an immediate reconnect attempt.
	 */
	retry() {
		this.forceReconnect('user-initiated retry');
	}

	/**
	 * Wire per-instance auth handlers. Called by ServerStateStore once both the
	 * client and the store's CurrentUserState exist. May be called more than once;
	 * the latest handlers win.
	 */
	setAuthHandlers(handlers: AuthHandlers) {
		this.#handlers = handlers;
	}

	#triggerSessionValidation() {
		if (!this.#handlers.onSessionValidation) return;
		const now = Date.now();
		if (now - this.#lastSessionValidation < SESSION_VALIDATION_COOLDOWN_MS) return;
		this.#lastSessionValidation = now;
		this.#handlers.onSessionValidation();
	}

	constructor(config: GraphQLClientConfig) {
		const { url, wsUrl, token } = config;
		this.#host = url.startsWith('/')
			? (typeof window !== 'undefined' ? window.location.host : 'localhost')
			: // eslint-disable-next-line svelte/prefer-svelte-reactivity -- extracting host string, URL not stored
				new URL(url).host;

		// Client pings the server every 15s. The `ping` handler starts a 5s
		// pong timeout; if the server doesn't respond, we close the socket.
		// Combined with the server's own 10s ping interval, this gives two
		// independent liveness checks.
		this.#wsClient = createWSClient({
			url: wsUrl,
			keepAlive: 15_000,
			retryAttempts: Infinity,
			shouldRetry: () => {
				// Stop retrying once we've crossed the threshold. Logs once
				// when transitioning to the give-up state so the failure is
				// visible in the console without spamming.
				if (this.#failedAttempts >= MAX_WS_RETRY_ATTEMPTS) {
					if (!this.gaveUp) {
						this.gaveUp = true;
						console.error(
							`[ws:${this.#host}] giving up after ${this.#failedAttempts} failed attempts; instance unreachable`
						);
					}
					return false;
				}
				return true;
			},
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
				// All subsequent attempts: every 5s
				console.log('[ws:%s] Retry attempt %d (waiting 5s)', this.#host, retries);
				await new Promise((resolve) => setTimeout(resolve, 5000));
			},
			on: {
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
					console.log('[ws:%s] Connected', this.#host);

					if (this.status === 'disconnected') {
						this.reconnectCount++;
						console.log(
							'[ws:%s] Reconnected (count: %d)',
							this.#host,
							this.reconnectCount
						);
						this.#triggerSessionValidation();
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
						'[ws:%s] Closed (code: %s, reason: %s)',
						this.#host,
						closeEvent?.code ?? 'unknown',
						closeEvent?.reason || 'none'
					);
					this.status = 'disconnected';
				},
				error: (err) => console.error('[ws:%s] Error:', this.#host, err)
			}
		});

		this.client = new Client({
			url,
			preferGetMethod: false,
			...(token ? { fetchOptions: () => ({ headers: { Authorization: `Bearer ${token}` } }) } : {}),
			exchanges: [
				mapExchange({
					onResult: (result) => {
						if (
							this.#handlers.onAuthFailure &&
							result.error?.graphQLErrors?.some((e) => e.message?.includes('not authenticated'))
						) {
							console.warn(
								'[auth] GraphQL "not authenticated" error → triggering auth failure',
								{ operation: result.operation.kind, errors: result.error.graphQLErrors }
							);
							this.#handlers.onAuthFailure();
						}

						// If an HTTP request succeeded but WebSocket is disconnected,
						// the server is reachable — force reconnect the WebSocket
						if (!result.error && this.status === 'disconnected') {
							this.forceReconnect('HTTP request succeeded while WS disconnected');
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

					this.#triggerSessionValidation();

					if (this.status === 'disconnected' || hiddenDuration > 30_000) {
						this.forceReconnect(
							`tab visible after ${Math.round(hiddenDuration / 1000)}s hidden`
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
		if (typeof window !== 'undefined') {
			let lastTick = Date.now();
			this.#suspendDetectorInterval = setInterval(() => {
				const now = Date.now();
				if (now - lastTick > 30_000) {
					this.forceReconnect(
						`suspend detected (timer gap: ${Math.round((now - lastTick) / 1000)}s)`
					);
				}
				lastTick = now;
			}, 10_000);

			// Reconnect when network comes back online (e.g., after airplane mode
			// or Wi-Fi re-association following sleep).
			this.#onlineHandler = () => {
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
	#originClient: GraphQLClient;

	constructor() {
		this.#originClient = new GraphQLClient({
			url: HOME_URL,
			wsUrl: HOME_URL,
			token: null
		});
	}

	/** The origin instance client (serves the SPA, uses cookies). */
	get originClient(): GraphQLClient {
		return this.#originClient;
	}

	/** Get or create a client for a registered instance. */
	getClient(serverId: string): GraphQLClient {
		if (serverRegistry.isOriginInstance(serverId)) {
			return this.#originClient;
		}

		const existing = this.#clients.get(serverId);
		if (existing) return existing;

		const instance = serverRegistry.getInstance(serverId);
		if (!instance) {
			throw new Error(`Instance "${serverId}" not found in registry`);
		}

		const url = `${instance.url}/api/graphql`;
		const client = new GraphQLClient({
			url,
			wsUrl: httpToWsUrl(url),
			token: instance.token
		});

		this.#clients.set(serverId, client);
		return client;
	}

	/** Destroy and remove a client. Cannot destroy the origin client. */
	destroyClient(serverId: string): boolean {
		const client = this.#clients.get(serverId);
		if (!client) return false;

		client.dispose();
		this.#clients.delete(serverId);
		return true;
	}
}

export const graphqlClientManager = new GraphQLClientManager();
