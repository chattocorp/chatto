import { afterEach, describe, it, expect, beforeEach } from 'vitest';
import {
	generateServerId,
	restorePersistedServerState,
	serverRegistry,
	splitPersistedServers,
	type RegisteredServer
} from './registry.svelte';
import { queryClient } from '$lib/query/client';

const STORAGE_KEY = 'chatto:instances';

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
		source: 'local',
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
		localStorage.removeItem(STORAGE_KEY);
	});

	afterEach(() => {
		serverRegistry.removeAll();
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

	it('distinguishes public registry changes from a local sign-out reset', async () => {
		const registry = await createRegistry();
		registry.removeAll();
		const changes: Array<'public' | 'local-reset'> = [];
		const unsubscribe = registry.subscribeCatalog((change) => changes.push(change));

		registry.addServer(makeServer());
		registry.updateRegistration('test-instance', { name: 'Updated' });
		registry.removeAll();
		unsubscribe();

		expect(changes).toEqual(['public', 'public', 'local-reset']);
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
		it('updates public metadata without changing or publishing the local session', async () => {
			const registry = await createRegistry();
			registry.removeAll();
			registry.addServer(makeServer({ token: 'secret-token', userId: 'user-1' }));
			const changes: Array<'public' | 'local-reset'> = [];
			const unsubscribe = registry.subscribeCatalog((change) => changes.push(change));

			registry.updateRegistration('test-instance', { name: 'Renamed' });
			registry.replaceServerAuthentication('test-instance', {
				token: 'replacement-token',
				userId: 'user-2',
				userLogin: 'bob',
				userDisplayName: 'Bob',
				userAvatarUrl: null,
				reauthRequiredAt: null
			});
			unsubscribe();

			expect(registry.registrations[0]).toEqual({
				id: 'test-instance',
				url: 'https://test.example.com',
				name: 'Renamed',
				iconUrl: null,
				addedAt: 1000,
				source: 'local'
			});
			expect(registry.getServer('test-instance')).toMatchObject({
				token: 'replacement-token',
				userId: 'user-2'
			});
			expect(changes).toEqual(['public']);
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

		it('drops signed-out synced entries and promotes authenticated ones on disconnect', async () => {
			const registry = await createRegistry();
			registry.removeAll();
			registry.addServer(
				makeServer({ id: 'local', url: 'https://local.example.com', source: 'local' })
			);
			registry.addServer(
				makeServer({ id: 'signed-out', url: 'https://signed-out.example.com', source: 'synced' })
			);
			registry.addServer(
				makeServer({
					id: 'signed-in',
					url: 'https://signed-in.example.com',
					source: 'synced',
					token: 'remote-token'
				})
			);

			registry.detachSyncedRegistrations();

			expect(registry.getServer('local')?.source).toBe('local');
			expect(registry.getServer('signed-out')).toBeUndefined();
			expect(registry.getServer('signed-in')).toMatchObject({
				source: 'local',
				token: 'remote-token'
			});
		});

		it('loads the existing combined storage shape as separate runtime state', () => {
			const persisted = makeServer({ token: 'persisted-token', userId: 'persisted-user' });
			delete (persisted as Partial<RegisteredServer>).source;
			const restored = splitPersistedServers([persisted]);

			expect(restored.registrations[0]).toEqual(
				expect.objectContaining({ id: 'test-instance', source: 'local' })
			);
			expect(restored.sessions).toEqual([
				[
					'test-instance',
					expect.objectContaining({ token: 'persisted-token', userId: 'persisted-user' })
				]
			]);
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
				[
					'persisted',
					expect.objectContaining({ token: server.token, userId: server.userId })
				]
			]);
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
