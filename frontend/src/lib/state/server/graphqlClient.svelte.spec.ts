import { describe, it, expect, beforeEach, vi } from 'vitest';

// Declare mock values via vi.hoisted so they're available in vi.mock factories
const { mockWsDispose, mockWsTerminate, mockWsSubscribe, mockServers, clientConfigs, wsConfigs } =
	vi.hoisted(() => ({
		mockWsDispose: vi.fn(),
		mockWsTerminate: vi.fn(),
		mockWsSubscribe: vi.fn(() => vi.fn()),
		mockServers: new Map<
			string,
			{ id: string; url: string; token: string | null }
		>(),
		clientConfigs: [] as Record<string, unknown>[],
		wsConfigs: [] as Record<string, unknown>[]
	}));


vi.mock('graphql-ws', () => ({
	createClient: vi.fn((config: Record<string, unknown>) => {
		wsConfigs.push(config);
		return {
			subscribe: mockWsSubscribe,
			dispose: mockWsDispose,
			terminate: mockWsTerminate
		};
	})
}));

vi.mock('@urql/svelte', () => ({
	Client: class MockClient {
		constructor(config: Record<string, unknown>) {
			clientConfigs.push(config);
		}
	},
	fetchExchange: { name: 'fetchExchange' },
	subscriptionExchange: vi.fn(() => ({ name: 'subscriptionExchange' })),
	mapExchange: vi.fn((config: { onResult: (result: unknown) => unknown }) => ({
		name: 'mapExchange',
		onResult: config.onResult
	}))
}));

vi.mock('./registry.svelte', () => ({
	serverRegistry: {
		getServer: (id: string) => mockServers.get(id),
		isOriginServer: (id: string) => mockServers.get(id)?.token === null
	}
}));

import { httpToWsUrl, GraphQLClient, type GraphQLClientConfig } from './graphqlClient.svelte';
import { createClient as createWSClient } from 'graphql-ws';
import { mapExchange } from '@urql/svelte';

type MockResult = {
	operation?: { kind?: string };
	error?: { graphQLErrors?: { message?: string }[] };
};

/** Pull the most recent mapExchange `onResult` callback so tests can fire it. */
function lastMapExchangeOnResult(): (result: MockResult) => unknown {
	const calls = vi.mocked(mapExchange).mock.calls;
	const lastConfig = calls[calls.length - 1][0] as {
		onResult: (r: MockResult) => unknown;
	};
	return lastConfig.onResult;
}

/** Build a mock result with the fields the auth-failure path reads. */
function makeAuthErrorResult(message: string): MockResult {
	return {
		operation: { kind: 'query' },
		error: { graphQLErrors: [{ message }] }
	};
}

function makeConfig(overrides: Partial<GraphQLClientConfig> = {}): GraphQLClientConfig {
	return {
		url: '/api/graphql',
		wsUrl: '/api/graphql',
		token: null,
		...overrides
	};
}

/** Get the most recent Client constructor config */
function lastClientConfig(): Record<string, unknown> | undefined {
	return clientConfigs[clientConfigs.length - 1];
}

/** Pull the graphql-ws `on` handler set from the most recent createClient call. */
function lastWsOnHandlers(): Record<string, (...args: unknown[]) => void> {
	const cfg = wsConfigs[wsConfigs.length - 1];
	return cfg.on as Record<string, (...args: unknown[]) => void>;
}

describe('httpToWsUrl', () => {
	it('converts http to ws', () => {
		expect(httpToWsUrl('http://localhost:4000/api/graphql')).toBe(
			'ws://localhost:4000/api/graphql'
		);
	});

	it('converts https to wss', () => {
		expect(httpToWsUrl('https://chat.example.com/api/graphql')).toBe(
			'wss://chat.example.com/api/graphql'
		);
	});

	it('leaves non-http URLs unchanged', () => {
		expect(httpToWsUrl('/api/graphql')).toBe('/api/graphql');
	});
});

describe('GraphQLClient', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		clientConfigs.length = 0;
		wsConfigs.length = 0;
		document.cookie = 'chatto_csrf=; Max-Age=0; path=/';
	});

	it('creates a client with the provided URL', () => {
		new GraphQLClient(makeConfig({ url: '/api/graphql' }));
		expect(lastClientConfig()?.url).toBe('/api/graphql');
	});

	it('creates a WebSocket client with the provided wsUrl', () => {
		new GraphQLClient(makeConfig({ wsUrl: 'wss://example.com/api/graphql' }));
		expect(createWSClient).toHaveBeenCalledWith(
			expect.objectContaining({ url: 'wss://example.com/api/graphql' })
		);
	});

	it('sets CSRF header for cookie auth when the CSRF cookie exists', () => {
		document.cookie = 'chatto_csrf=csrf-token; path=/';
		new GraphQLClient(makeConfig({ token: null }));
		expect(lastClientConfig()?.fetchOptions).toBeDefined();
		const opts = (lastClientConfig()!.fetchOptions as () => Record<string, unknown>)();
		expect(opts).toEqual({
			headers: { 'X-REQUEST-TYPE': 'GraphQL', 'X-CSRF-Token': 'csrf-token' }
		});
	});

	it('sets GraphQL request type header for cookie auth when the CSRF cookie is missing', () => {
		new GraphQLClient(makeConfig({ token: null }));
		expect(lastClientConfig()?.fetchOptions).toBeDefined();
		const opts = (lastClientConfig()!.fetchOptions as () => Record<string, unknown>)();
		expect(opts).toEqual({ headers: { 'X-REQUEST-TYPE': 'GraphQL' } });
	});

	it('sets fetchOptions with Authorization header when token is provided', () => {
		document.cookie = 'chatto_csrf=csrf-token; path=/';
		new GraphQLClient(
			makeConfig({ url: 'https://remote.example.com/api/graphql', token: 'my-token' })
		);
		expect(lastClientConfig()?.fetchOptions).toBeDefined();
		const opts = (lastClientConfig()!.fetchOptions as () => Record<string, unknown>)();
		expect(opts).toEqual({
			headers: { 'X-REQUEST-TYPE': 'GraphQL', Authorization: 'Bearer my-token' }
		});
	});

	it('sets connectionParams when token is provided', () => {
		new GraphQLClient(
			makeConfig({ url: 'https://remote.example.com/api/graphql', token: 'my-token' })
		);
		expect(createWSClient).toHaveBeenCalledWith(
			expect.objectContaining({
				connectionParams: expect.any(Function)
			})
		);
	});

	it('does not set connectionParams when token is null', () => {
		new GraphQLClient(makeConfig({ token: null }));
		const wsCall = vi.mocked(createWSClient).mock.calls[0][0];
		expect(wsCall.connectionParams).toBeUndefined();
	});

	it('starts with status "connecting"', () => {
		const client = new GraphQLClient(makeConfig());
		expect(client.status).toBe('connecting');
	});

	it('starts with reconnectCount 0', () => {
		const client = new GraphQLClient(makeConfig());
		expect(client.reconnectCount).toBe(0);
	});

	it('dispose cleans up the WebSocket client', () => {
		const client = new GraphQLClient(makeConfig());
		client.dispose();
		expect(mockWsDispose).toHaveBeenCalledOnce();
	});

	it('forceReconnect terminates the WebSocket once connected', () => {
		const client = new GraphQLClient(makeConfig());
		// Simulate a completed connection so status flips from 'connecting' to
		// 'connected'. forceReconnect short-circuits while still connecting.
		lastWsOnHandlers().connected({ readyState: 1 });
		client.forceReconnect('test');
		expect(mockWsTerminate).toHaveBeenCalledOnce();
	});

	it('forceReconnect is a no-op while a connection attempt is already in flight', () => {
		const client = new GraphQLClient(makeConfig());
		// Fresh client starts in 'connecting'; multiple forceReconnect calls
		// during this window (visibility + suspend detector + online races on
		// tab resume) must not kill the in-flight handshake.
		client.forceReconnect('first');
		client.forceReconnect('second');
		expect(mockWsTerminate).not.toHaveBeenCalled();
	});

	describe('setAuthHandlers', () => {
		it('does not call onAuthFailure before handlers are wired', () => {
			const client = new GraphQLClient(makeConfig());
			const onResult = lastMapExchangeOnResult();
			// No setAuthHandlers call — pre-wiring window. This guards the
			// synchronous gap between client construction and ServerStateStore
			// wiring its handlers.
			onResult(makeAuthErrorResult('not authenticated'));
			expect(client).toBeDefined();
		});

		it('fires onAuthFailure when the result contains a "not authenticated" error', () => {
			const client = new GraphQLClient(makeConfig());
			const handler = vi.fn();
			client.setAuthHandlers({ onAuthFailure: handler });

			lastMapExchangeOnResult()(makeAuthErrorResult('user not authenticated'));

			expect(handler).toHaveBeenCalledTimes(1);
		});

		it('does not fire onAuthFailure for unrelated errors', () => {
			const client = new GraphQLClient(makeConfig());
			const handler = vi.fn();
			client.setAuthHandlers({ onAuthFailure: handler });

			lastMapExchangeOnResult()(makeAuthErrorResult('something else broke'));

			expect(handler).not.toHaveBeenCalled();
		});

		it('replacing handlers swaps which callback fires next', () => {
			const client = new GraphQLClient(makeConfig());
			const first = vi.fn();
			const second = vi.fn();

			client.setAuthHandlers({ onAuthFailure: first });
			client.setAuthHandlers({ onAuthFailure: second });

			lastMapExchangeOnResult()(makeAuthErrorResult('not authenticated'));

			expect(first).not.toHaveBeenCalled();
			expect(second).toHaveBeenCalledTimes(1);
		});

		it('clearing handlers (passing {}) makes onAuthFailure a no-op again', () => {
			const client = new GraphQLClient(makeConfig());
			const handler = vi.fn();

			client.setAuthHandlers({ onAuthFailure: handler });
			client.setAuthHandlers({}); // unwired

			lastMapExchangeOnResult()(makeAuthErrorResult('not authenticated'));

			expect(handler).not.toHaveBeenCalled();
		});
	});
});

describe('GraphQLClientManager', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockServers.clear();
	});

	// We can't easily test the manager singleton due to module-level instantiation
	// with mocked dependencies, but we test the key behaviors through the
	// exported functions and the GraphQLClient class configuration above.

	it('exports graphqlClientManager', async () => {
		const mod = await import('./graphqlClient.svelte');
		expect(mod.graphqlClientManager).toBeDefined();
	});

	it('originClient uses relative URL', async () => {
		const mod = await import('./graphqlClient.svelte');
		// The home client should have been created - verify it exists
		expect(mod.graphqlClientManager.originClient).toBeDefined();
		expect(mod.graphqlClientManager.originClient.status).toBe('connecting');
	});

	it('getClient returns originClient for home instances', async () => {
		const mod = await import('./graphqlClient.svelte');
		mockServers.set('my-home', {
			id: 'my-home',
			url: 'http://localhost:4000',
			token: null
		});

		const client = mod.graphqlClientManager.getClient('my-home');
		expect(client).toBe(mod.graphqlClientManager.originClient);
	});

	it('getClient throws for unknown instance IDs', async () => {
		const mod = await import('./graphqlClient.svelte');
		expect(() => mod.graphqlClientManager.getClient('nonexistent')).toThrow(
			'Server "nonexistent" not found in registry'
		);
	});

	it('getClient creates and caches remote clients', async () => {
		const mod = await import('./graphqlClient.svelte');
		mockServers.set('remote-1', {
			id: 'remote-1',
			url: 'https://remote.example.com',
			token: 'remote-token'
		});

		const client1 = mod.graphqlClientManager.getClient('remote-1');
		const client2 = mod.graphqlClientManager.getClient('remote-1');
		expect(client1).toBe(client2); // Same cached instance
		expect(client1).not.toBe(mod.graphqlClientManager.originClient);
	});

	it('destroyClient disposes and removes remote clients', async () => {
		const mod = await import('./graphqlClient.svelte');
		mockServers.set('remote-2', {
			id: 'remote-2',
			url: 'https://other.example.com',
			token: 'token-2'
		});

		// Create the client first
		mod.graphqlClientManager.getClient('remote-2');

		// Destroy it
		expect(mod.graphqlClientManager.destroyClient('remote-2')).toBe(true);
		expect(mockWsDispose).toHaveBeenCalled();

		// Getting it again should create a new one
		const newClient = mod.graphqlClientManager.getClient('remote-2');
		expect(newClient).toBeDefined();
	});

	it('destroyClient returns false for nonexistent clients', async () => {
		const mod = await import('./graphqlClient.svelte');
		expect(mod.graphqlClientManager.destroyClient('nope')).toBe(false);
	});

});
