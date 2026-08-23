import { afterEach, describe, it, expect, beforeEach, vi } from 'vitest';
import {
	generateServerId,
	restorePersistedServerState,
	serverRegistry,
	splitPersistedServers,
	type RegisteredServer
} from './registry.svelte';
import { queryClient } from '$lib/query/client';
import { serverStorageKey } from '$lib/storage/serverStorage';

const STORAGE_KEY = 'chatto:instances';

function authenticationStorageKey(serverId: string): string {
	return serverStorageKey(serverId, 'authentication');
}

function updatePersistedAuthentication(
	serverId: string,
	patch: Record<string, string | number | null>
): void {
	const key = authenticationStorageKey(serverId);
	const stored = JSON.parse(localStorage.getItem(key) ?? 'null') as Record<string, unknown> | null;
	if (!stored) throw new Error(`No persisted authentication for ${serverId}`);
	localStorage.setItem(key, JSON.stringify({ ...stored, ...patch }));
}

function makeServer(overrides: Partial<RegisteredServer> = {}): RegisteredServer {
	return {
		id: 'test-instance',
		url: 'https://test.example.com',
		name: 'Test Instance',
		iconUrl: null,
		token: null,
		userId: null,
		userLogin: null,
		userDisplayName: null,
		userAvatarUrl: null,
		reauthRequiredAt: null,
		addedAt: 1000,
		...overrides
	};
}

function createRegistry() {
	return serverRegistry;
}

describe('generateServerId', () => {
	it('extracts hostname and replaces dots with hyphens', () => {
		expect(generateServerId('https://chat.example.com')).toBe('chat-example-com');
	});

	it('handles localhost', () => {
		expect(generateServerId('http://localhost')).toBe('localhost');
	});

	it('handles URLs with ports', () => {
		expect(generateServerId('http://localhost:4000')).toBe('localhost');
	});

	it('deduplicates when ID already exists', () => {
		expect(generateServerId('https://chat.example.com', ['chat-example-com'])).toBe(
			'chat-example-com-2'
		);
	});

	it('increments suffix for multiple collisions', () => {
		expect(
			generateServerId('https://chat.example.com', ['chat-example-com', 'chat-example-com-2'])
		).toBe('chat-example-com-3');
	});

	it('handles invalid URLs gracefully', () => {
		const id = generateServerId('not-a-url');
		expect(id).toBeTruthy();
		expect(id.length).toBeGreaterThan(0);
	});
});

describe('ServerRegistry', () => {
	beforeEach(() => {
		localStorage.clear();
	});

	afterEach(() => {
		serverRegistry.removeAll();
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	it('exports the singleton', async () => {
		const registry = await createRegistry();
		expect(registry).toBeDefined();
		expect(registry.servers).toBeDefined();
	});

	describe('init', () => {
		it('does not auto-register any instance', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.init();

			expect(registry.servers).toHaveLength(0);
		});
	});

	describe('probeOrigin', () => {
		it('settles a custom application origin without registering it', async () => {
			const registry = await createRegistry();
			registry.removeAll();
			registry.originProbed = false;

			registry.probeOrigin(false, new URL('chatto://desktop'));

			expect(registry.originProbed).toBe(true);
			expect(registry.servers).toHaveLength(0);
		});
	});

	describe('originServer', () => {
		it('returns the instance matching window.location.origin', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'origin', url: window.location.origin, name: 'Origin' }));
			registry.addServer(
				makeServer({ id: 'remote', url: 'https://remote.example.com', name: 'Remote' })
			);

			expect(registry.originServer?.name).toBe('Origin');
		});

		it('returns undefined when no origin instance exists', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'a', url: 'https://remote.example.com' }));

			expect(registry.originServer).toBeUndefined();
		});
	});

	describe('isOriginServer', () => {
		it('returns true for instance matching window.location.origin', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'origin', url: window.location.origin }));

			expect(registry.isOriginServer('origin')).toBe(true);
		});

		it('returns false for remote instance', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(
				makeServer({ id: 'remote', url: 'https://remote.example.com', token: 'remote-token' })
			);

			expect(registry.isOriginServer('remote')).toBe(false);
		});
	});

	describe('firstAuthenticatedServerId', () => {
		it('prefers the origin and can exclude the session being cleared', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(
				makeServer({ id: 'remote', url: 'https://remote.example.com', token: 'remote-token' })
			);
			registry.addServer(makeServer({ id: 'origin', url: window.location.origin }));
			registry.getStore('remote').currentUser.user = { id: 'remote-user' } as never;
			registry.getStore('origin').currentUser.user = { id: 'origin-user' } as never;

			expect(registry.firstAuthenticatedServerId()).toBe('origin');
			expect(registry.firstAuthenticatedServerId('origin')).toBe('remote');
		});
	});

	describe('addServer', () => {
		it('adds an instance', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			const server = makeServer();
			registry.addServer(server);

			expect(registry.servers).toHaveLength(1);
			expect(registry.servers[0].id).toBe('test-instance');
		});

		it('persists to localStorage', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer());

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(1);
			expect(stored[0].id).toBe('test-instance');
		});

		it('skips duplicates', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			const server = makeServer();
			registry.addServer(server);
			registry.addServer(server);

			expect(registry.servers).toHaveLength(1);
		});
	});

	describe('removeServer', () => {
		it('removes an instance by ID', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'a' }));
			registry.addServer(makeServer({ id: 'b' }));

			expect(registry.removeServer('a')).toBe(true);
			expect(registry.servers).toHaveLength(1);
			expect(registry.servers[0].id).toBe('b');
		});

		it('returns false for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			expect(registry.removeServer('nope')).toBe(false);
		});

		it('persists removal to localStorage', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'a' }));
			registry.removeServer('a');

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(0);
		});
	});

	describe('handleAuthenticationRequired', () => {
		it('marks remote instances as needing reauth without removing them', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(
				makeServer({
					id: 'remote',
					url: 'https://remote.example.com',
					token: 'remote-token',
					userId: 'U1',
					userLogin: 'alice',
					userDisplayName: 'Alice'
				})
			);
			queryClient.setQueryData(['server', 'remote', 'private'], 'cached admin data');

			registry.handleAuthenticationRequired('remote');

			expect(registry.getServer('remote')?.token).toBe('remote-token');
			expect(registry.getServer('remote')?.reauthRequiredAt).toEqual(expect.any(Number));
			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(1);
			expect(stored[0].reauthRequiredAt).toEqual(expect.any(Number));
			expect(queryClient.getQueryData(['server', 'remote', 'private'])).toBeUndefined();
		});

		it('clears reauth-required state explicitly', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'remote', token: 'remote-token' }));
			registry.handleAuthenticationRequired('remote');
			registry.clearAuthenticationRequired('remote');

			expect(registry.getServer('remote')?.reauthRequiredAt).toBeNull();
			expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)[0].reauthRequiredAt).toBeNull();
		});

		it('keeps origin instances registered when clearing origin auth', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(
				makeServer({
					id: 'origin',
					url: window.location.origin,
					token: 'origin-token',
					userId: 'U1',
					userLogin: 'alice'
				})
			);

			registry.clearOriginAuthentication();

			expect(registry.getServer('origin')?.token).toBeNull();
			expect(registry.getServer('origin')?.userId).toBeNull();
		});
	});

	describe('authenticateOrigin', () => {
		it('replaces only origin authentication and retains remote server state', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(
				makeServer({
					id: 'origin',
					url: window.location.origin,
					token: 'old-origin-token',
					userId: 'origin-user'
				})
			);
			registry.addServer(
				makeServer({
					id: 'remote',
					url: 'https://remote.example.com',
					token: 'remote-token',
					userId: 'remote-user',
					userLogin: 'remote-login',
					reauthRequiredAt: 1234
				})
			);
			const remoteStore = registry.getStore('remote');

			registry.authenticateOrigin('new-origin-token', {
				id: 'new-origin-user',
				login: 'new-origin-login'
			});

			expect(registry.getServer('origin')).toMatchObject({
				token: 'new-origin-token',
				userId: 'new-origin-user',
				userLogin: 'new-origin-login',
				reauthRequiredAt: null
			});
			expect(registry.getServer('remote')).toMatchObject({
				token: 'remote-token',
				userId: 'remote-user',
				userLogin: 'remote-login',
				reauthRequiredAt: 1234
			});
			expect(registry.getStore('remote')).toBe(remoteStore);
		});
	});

	describe('updateServer', () => {
		it('updates fields on an existing instance', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'x', name: 'Old Name' }));

			expect(registry.updateRegistration('x', { name: 'New Name' })).toBe(true);
			expect(registry.servers[0].name).toBe('New Name');
		});

		it('returns false for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			expect(registry.updateRegistration('nope', { name: 'x' })).toBe(false);
		});

		it('persists update to localStorage', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'x', name: 'Old' }));
			registry.updateRegistration('x', { name: 'New' });

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored[0].name).toBe('New');
		});
	});

	describe('catalogue and session ownership', () => {
		it('updates public metadata without changing the local session', async () => {
			const registry = await createRegistry();
			registry.removeAll();
			registry.addServer(makeServer({ token: 'secret-token', userId: 'user-1' }));

			registry.updateRegistration('test-instance', { name: 'Renamed' });
			registry.replaceServerAuthentication('test-instance', {
				token: 'replacement-token',
				userId: 'user-2',
				userLogin: 'bob',
				userDisplayName: 'Bob',
				userAvatarUrl: null,
				reauthRequiredAt: null
			});

			expect(registry.registrations[0]).toEqual({
				id: 'test-instance',
				url: 'https://test.example.com',
				name: 'Renamed',
				iconUrl: null,
				addedAt: 1000
			});
			expect(registry.getServer('test-instance')).toMatchObject({
				token: 'replacement-token',
				userId: 'user-2'
			});
		});

		it('retains only an unauthenticated origin during a local all-server reset', async () => {
			const registry = await createRegistry();
			registry.removeAll();
			registry.addServer(
				makeServer({
					id: 'origin',
					url: window.location.origin,
					token: 'origin-token',
					userId: 'origin-user'
				})
			);
			registry.addServer(
				makeServer({ id: 'remote', url: 'https://remote.example.com', token: 'remote-token' })
			);

			registry.resetToOrigin();

			expect(registry.servers).toHaveLength(1);
			expect(registry.originServer).toMatchObject({ id: 'origin', token: null, userId: null });
			expect(registry.getServer('remote')).toBeUndefined();
			expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)).toEqual([
				expect.objectContaining({ id: 'origin', token: null })
			]);
		});

		it('loads the existing combined storage shape and retires sync provenance', () => {
			const persisted = {
				...makeServer({ token: 'persisted-token', userId: 'persisted-user' }),
				source: 'synced' as const
			};
			const restored = splitPersistedServers([persisted]);

			expect(restored.registrations[0]).toEqual(expect.objectContaining({ id: 'test-instance' }));
			expect(restored.registrations[0]).not.toHaveProperty('source');
			expect(restored.sessions).toEqual([
				[
					'test-instance',
					expect.objectContaining({ token: 'persisted-token', userId: 'persisted-user' })
				]
			]);
		});
	});

	describe('bearer renewal', () => {
		function renewableServer() {
			return makeServer({
				id: 'renewable',
				url: 'https://renewable.example',
				token: 'access-1',
				refreshToken: 'refresh-1',
				accessTokenExpiresAt: Date.now() + 10 * 60_000,
				refreshTokenExpiresAt: Date.now() + 24 * 60 * 60_000,
				oauthClientId: 'https://client.example/oauth/client-metadata.json'
			});
		}

		function refreshedResponse() {
			return new Response(
				JSON.stringify({
					access_token: 'access-2',
					refresh_token: 'refresh-2',
					expires_in: 900,
					refresh_token_expires_in: 86_400
				}),
				{ headers: { 'Content-Type': 'application/json' } }
			);
		}

		it('coalesces concurrent rotations and installs the pair in place', async () => {
			const fetchMock = vi.fn(async (input: string | URL | Request) => {
				if (String(input).endsWith('/oauth/token')) return refreshedResponse();
				return new Response('', { status: 503 });
			});
			vi.stubGlobal('fetch', fetchMock);
			serverRegistry.addServer(renewableServer());

			const [first, second] = await Promise.all([
				serverRegistry.renewServerAuthentication('renewable', true),
				serverRegistry.renewServerAuthentication('renewable', true)
			]);

			expect(first).toBe('access-2');
			expect(second).toBe('access-2');
			const tokenCalls = fetchMock.mock.calls.filter(([input]) =>
				String(input).endsWith('/oauth/token')
			);
			expect(tokenCalls).toHaveLength(1);
			expect(serverRegistry.getServer('renewable')).toMatchObject({
				token: 'access-2',
				refreshToken: 'refresh-2',
				refreshRequestId: null,
				reauthRequiredAt: null
			});
		});

		it('reuses its persisted request ID after a lost refresh response', async () => {
			let refreshAttempts = 0;
			const requestBodies: Array<Record<string, string>> = [];
			vi.stubGlobal(
				'fetch',
				vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
					if (!String(input).endsWith('/oauth/token')) return new Response('', { status: 503 });
					requestBodies.push(JSON.parse(String(init?.body)) as Record<string, string>);
					refreshAttempts++;
					if (refreshAttempts === 1) throw new TypeError('response lost');
					return refreshedResponse();
				})
			);
			serverRegistry.addServer(renewableServer());

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).rejects.toThrow(
				'response lost'
			);
			const persistedAfterFailure = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]')[0];
			expect(persistedAfterFailure.refreshRequestId).toBeTruthy();

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).resolves.toBe(
				'access-2'
			);
			expect(requestBodies[1].refresh_request_id).toBe(requestBodies[0].refresh_request_id);
		});

		it('does not rotate when the recovery request ID cannot be persisted', async () => {
			const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => refreshedResponse());
			vi.stubGlobal('fetch', fetchMock);
			serverRegistry.addServer(renewableServer());
			vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
				throw new DOMException('Storage quota exceeded', 'QuotaExceededError');
			});

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).rejects.toThrow(
				'Unable to persist bearer renewal state.'
			);
			expect(
				fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/oauth/token'))
			).toHaveLength(0);
		});

		it("adopts another tab's persisted rotation after acquiring the refresh lock", async () => {
			const fetchMock = vi.fn();
			vi.stubGlobal('fetch', fetchMock);
			serverRegistry.addServer(renewableServer());
			updatePersistedAuthentication('renewable', {
				token: 'access-from-other-tab',
				refreshToken: 'refresh-from-other-tab',
				accessTokenExpiresAt: Date.now() + 15 * 60_000,
				refreshRequestId: null
			});

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).resolves.toBe(
				'access-from-other-tab'
			);
			expect(
				fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/oauth/token'))
			).toHaveLength(0);
			expect(serverRegistry.getServer('renewable')).toMatchObject({
				token: 'access-from-other-tab',
				refreshToken: 'refresh-from-other-tab',
				reauthRequiredAt: null
			});
		});

		it("adopts another tab's request ID after a lost refresh response", async () => {
			const requestBodies: Array<Record<string, string>> = [];
			vi.stubGlobal(
				'fetch',
				vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
					if (!String(input).endsWith('/oauth/token')) return new Response('', { status: 503 });
					requestBodies.push(JSON.parse(String(init?.body)) as Record<string, string>);
					return refreshedResponse();
				})
			);
			serverRegistry.addServer(renewableServer());
			updatePersistedAuthentication('renewable', {
				refreshRequestId: 'request-from-other-tab'
			});

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).resolves.toBe(
				'access-2'
			);
			expect(requestBodies).toHaveLength(1);
			expect(requestBodies[0].refresh_request_id).toBe('request-from-other-tab');
		});

		it("does not let a stale tab's metadata write restore rotated credentials", async () => {
			const fetchMock = vi.fn();
			vi.stubGlobal('fetch', fetchMock);
			serverRegistry.addServer(renewableServer());
			updatePersistedAuthentication('renewable', {
				token: 'access-from-other-tab',
				refreshToken: 'refresh-from-other-tab',
				accessTokenExpiresAt: Date.now() + 15 * 60_000,
				refreshRequestId: 'request-from-other-tab'
			});

			serverRegistry.updateRegistration('renewable', { name: 'Renamed by stale tab' });

			const combined = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]')[0];
			expect(combined).toMatchObject({
				name: 'Renamed by stale tab',
				token: 'access-from-other-tab',
				refreshToken: 'refresh-from-other-tab',
				refreshRequestId: 'request-from-other-tab'
			});
			await expect(serverRegistry.renewServerAuthentication('renewable', true)).resolves.toBe(
				'access-from-other-tab'
			);
			expect(
				fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/oauth/token'))
			).toHaveLength(0);
		});

		it('preserves the session and marks explicit reconnect after invalid_grant', async () => {
			const fetchMock = vi.fn(async (input: string | URL | Request) => {
				if (!String(input).endsWith('/oauth/token')) return new Response('', { status: 503 });
				return new Response(JSON.stringify({ error: 'invalid_grant' }), {
					status: 400,
					headers: { 'Content-Type': 'application/json' }
				});
			});
			vi.stubGlobal('fetch', fetchMock);
			serverRegistry.addServer(renewableServer());

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).resolves.toBeNull();
			expect(serverRegistry.getServer('renewable')).toMatchObject({
				token: 'access-1',
				refreshToken: 'refresh-1',
				reauthRequiredAt: expect.any(Number)
			});

			await expect(serverRegistry.renewServerAuthentication('renewable', true)).resolves.toBeNull();
			expect(
				fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/oauth/token'))
			).toHaveLength(1);
		});
	});

	describe('getServer', () => {
		it('returns instance by ID', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			registry.addServer(makeServer({ id: 'foo', name: 'Foo' }));

			expect(registry.getServer('foo')?.name).toBe('Foo');
		});

		it('returns undefined for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.removeAll();

			expect(registry.getServer('nope')).toBeUndefined();
		});
	});

	describe('localStorage persistence', () => {
		it('loads instances from localStorage on construction', () => {
			const server = makeServer({ id: 'persisted', name: 'Persisted' });
			localStorage.setItem(STORAGE_KEY, JSON.stringify([server]));

			const restored = restorePersistedServerState();

			expect(restored.registrations).toEqual([
				expect.objectContaining({ id: 'persisted', name: 'Persisted' })
			]);
			expect(restored.sessions).toEqual([
				['persisted', expect.objectContaining({ token: server.token, userId: server.userId })]
			]);
		});

		it('restores per-server authentication instead of stale combined credentials', () => {
			const server = makeServer({
				id: 'persisted',
				token: 'stale-access',
				refreshToken: 'stale-refresh',
				refreshRequestId: 'stale-request-id'
			});
			localStorage.setItem(STORAGE_KEY, JSON.stringify([server]));
			localStorage.setItem(
				authenticationStorageKey('persisted'),
				JSON.stringify({
					version: 1,
					token: 'rotated-access',
					refreshToken: 'rotated-refresh',
					accessTokenExpiresAt: 20_000,
					refreshTokenExpiresAt: 30_000,
					oauthClientId: null,
					refreshRequestId: 'rotated-request-id',
					reauthRequiredAt: null
				})
			);

			const restored = restorePersistedServerState();

			expect(restored.sessions[0][1]).toMatchObject({
				token: 'rotated-access',
				refreshToken: 'rotated-refresh',
				refreshRequestId: 'rotated-request-id'
			});
		});

		it('clears retired account-data browser storage', () => {
			localStorage.setItem('chatto:account-data:authorization', 'grant');
			localStorage.setItem('chatto:account-data:device-id', 'device');
			localStorage.setItem('chatto:account-data:tinybase', 'cache');

			restorePersistedServerState();

			expect(localStorage.getItem('chatto:account-data:authorization')).toBeNull();
			expect(localStorage.getItem('chatto:account-data:device-id')).toBeNull();
			expect(localStorage.getItem('chatto:account-data:tinybase')).toBeNull();
		});

		it('rewrites retired sync provenance as device-local storage', () => {
			localStorage.setItem(
				STORAGE_KEY,
				JSON.stringify([{ ...makeServer({ id: 'migrated' }), source: 'synced' }])
			);

			restorePersistedServerState();

			expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]')[0]).not.toHaveProperty('source');
		});

		it('handles corrupted localStorage gracefully', () => {
			localStorage.setItem(STORAGE_KEY, 'not valid json!!!');

			expect(restorePersistedServerState()).toEqual({ registrations: [], sessions: [] });
		});

		it('handles non-array localStorage gracefully', () => {
			localStorage.setItem(STORAGE_KEY, JSON.stringify({ not: 'an array' }));

			expect(restorePersistedServerState()).toEqual({ registrations: [], sessions: [] });
		});

		it.each([[null], [1], [{ id: 'partial' }]])(
			'handles malformed entries in the persisted array: %j',
			(value) => {
				localStorage.setItem(STORAGE_KEY, JSON.stringify(value));

				expect(restorePersistedServerState()).toEqual({ registrations: [], sessions: [] });
			}
		);
	});
});
