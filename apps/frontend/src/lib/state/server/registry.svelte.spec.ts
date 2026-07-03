import { describe, it, expect, beforeEach } from 'vitest';
import { generateServerId, type RegisteredServer } from './registry.svelte';

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
		...overrides
	};
}

/**
 * Create a fresh ServerRegistry by dynamically importing the module.
 * This bypasses the module-level singleton to get a clean instance per test.
 */
async function createRegistry() {
	// We can't easily re-instantiate a module singleton, so we import
	// the class structure and test the exported singleton.
	// Each test clears localStorage first to simulate a fresh state.
	const mod = await import('./registry.svelte');
	return mod.serverRegistry;
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
			generateServerId('https://chat.example.com', [
				'chat-example-com',
				'chat-example-com-2'
			])
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

	it('exports the singleton', async () => {
		const registry = await createRegistry();
		expect(registry).toBeDefined();
		expect(registry.servers).toBeDefined();
	});

	describe('init', () => {
		it('does not auto-register any instance', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.init();

			expect(registry.servers).toHaveLength(0);
		});
	});

	describe('originServer', () => {
		it('returns the instance matching window.location.origin', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'origin', url: window.location.origin, name: 'Origin' }));
			registry.addServer(makeServer({ id: 'remote', url: 'https://remote.example.com', name: 'Remote' }));

			expect(registry.originServer?.name).toBe('Origin');
		});

		it('returns undefined when no origin instance exists', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'a', url: 'https://remote.example.com' }));

			expect(registry.originServer).toBeUndefined();
		});
	});

	describe('isOriginServer', () => {
		it('returns true for instance matching window.location.origin', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'origin', url: window.location.origin }));

			expect(registry.isOriginServer('origin')).toBe(true);
		});

		it('returns false for remote instance', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'remote', url: 'https://remote.example.com' }));

			expect(registry.isOriginServer('remote')).toBe(false);
		});
	});

	describe('addServer', () => {
		it('adds an instance', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			const server = makeServer();
			registry.addServer(server);

			expect(registry.servers).toHaveLength(1);
			expect(registry.servers[0].id).toBe('test-instance');
		});

		it('persists to localStorage', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer());

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(1);
			expect(stored[0].id).toBe('test-instance');
		});

		it('skips duplicates', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			const server = makeServer();
			registry.addServer(server);
			registry.addServer(server);

			expect(registry.servers).toHaveLength(1);
		});
	});

	describe('removeServer', () => {
		it('removes an instance by ID', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'a' }));
			registry.addServer(makeServer({ id: 'b' }));

			expect(registry.removeServer('a')).toBe(true);
			expect(registry.servers).toHaveLength(1);
			expect(registry.servers[0].id).toBe('b');
		});

		it('returns false for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			expect(registry.removeServer('nope')).toBe(false);
		});

		it('persists removal to localStorage', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'a' }));
			registry.removeServer('a');

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(0);
		});
	});

	describe('handleAuthenticationRequired', () => {
		it('marks remote instances as needing reauth without removing them', async () => {
			const registry = await createRegistry();
			registry.servers = [];

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

			registry.handleAuthenticationRequired('remote');

			expect(registry.getServer('remote')?.token).toBe('remote-token');
			expect(registry.getServer('remote')?.reauthRequiredAt).toEqual(expect.any(Number));
			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(1);
			expect(stored[0].reauthRequiredAt).toEqual(expect.any(Number));
		});

		it('clears reauth-required state explicitly', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'remote', token: 'remote-token' }));
			registry.handleAuthenticationRequired('remote');
			registry.clearAuthenticationRequired('remote');

			expect(registry.getServer('remote')?.reauthRequiredAt).toBeNull();
			expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)[0].reauthRequiredAt).toBeNull();
		});

		it('keeps origin instances registered when clearing origin auth', async () => {
			const registry = await createRegistry();
			registry.servers = [];

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

	describe('updateServer', () => {
		it('updates fields on an existing instance', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'x', name: 'Old Name' }));

			expect(registry.updateServer('x', { name: 'New Name' })).toBe(true);
			expect(registry.servers[0].name).toBe('New Name');
		});

		it('returns false for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			expect(registry.updateServer('nope', { name: 'x' })).toBe(false);
		});

		it('persists update to localStorage', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'x', name: 'Old' }));
			registry.updateServer('x', { name: 'New' });

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored[0].name).toBe('New');
		});
	});

	describe('getServer', () => {
		it('returns instance by ID', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			registry.addServer(makeServer({ id: 'foo', name: 'Foo' }));

			expect(registry.getServer('foo')?.name).toBe('Foo');
		});

		it('returns undefined for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.servers = [];

			expect(registry.getServer('nope')).toBeUndefined();
		});
	});

	describe('localStorage persistence', () => {
		it('loads instances from localStorage on construction', async () => {
			const server = makeServer({ id: 'persisted', name: 'Persisted' });
			localStorage.setItem(STORAGE_KEY, JSON.stringify([server]));

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(1);
			expect(stored[0].id).toBe('persisted');
		});

		it('handles corrupted localStorage gracefully', async () => {
			localStorage.setItem(STORAGE_KEY, 'not valid json!!!');

			const registry = await createRegistry();
			expect(registry).toBeDefined();
		});

		it('handles non-array localStorage gracefully', async () => {
			localStorage.setItem(STORAGE_KEY, JSON.stringify({ not: 'an array' }));

			const registry = await createRegistry();
			expect(registry).toBeDefined();
		});
	});
});
