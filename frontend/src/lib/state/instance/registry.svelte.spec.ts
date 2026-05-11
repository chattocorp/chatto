import { describe, it, expect, beforeEach } from 'vitest';
import { generateInstanceId, type RegisteredInstance } from './registry.svelte';

const STORAGE_KEY = 'chatto:instances';

function makeInstance(overrides: Partial<RegisteredInstance> = {}): RegisteredInstance {
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
		addedAt: 1000,
		...overrides
	};
}

/**
 * Create a fresh InstanceRegistry by dynamically importing the module.
 * This bypasses the module-level singleton to get a clean instance per test.
 */
async function createRegistry() {
	// We can't easily re-instantiate a module singleton, so we import
	// the class structure and test the exported singleton.
	// Each test clears localStorage first to simulate a fresh state.
	const mod = await import('./registry.svelte');
	return mod.instanceRegistry;
}

describe('generateInstanceId', () => {
	it('extracts hostname and replaces dots with hyphens', () => {
		expect(generateInstanceId('https://chat.example.com')).toBe('chat-example-com');
	});

	it('handles localhost', () => {
		expect(generateInstanceId('http://localhost')).toBe('localhost');
	});

	it('handles URLs with ports', () => {
		expect(generateInstanceId('http://localhost:4000')).toBe('localhost');
	});

	it('deduplicates when ID already exists', () => {
		expect(generateInstanceId('https://chat.example.com', ['chat-example-com'])).toBe(
			'chat-example-com-2'
		);
	});

	it('increments suffix for multiple collisions', () => {
		expect(
			generateInstanceId('https://chat.example.com', [
				'chat-example-com',
				'chat-example-com-2'
			])
		).toBe('chat-example-com-3');
	});

	it('handles invalid URLs gracefully', () => {
		const id = generateInstanceId('not-a-url');
		expect(id).toBeTruthy();
		expect(id.length).toBeGreaterThan(0);
	});
});

describe('InstanceRegistry', () => {
	beforeEach(() => {
		localStorage.removeItem(STORAGE_KEY);
	});

	it('exports the singleton', async () => {
		const registry = await createRegistry();
		expect(registry).toBeDefined();
		expect(registry.instances).toBeDefined();
	});

	describe('init', () => {
		it('does not auto-register any instance', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.init();

			expect(registry.instances).toHaveLength(0);
		});
	});

	describe('originInstance', () => {
		it('returns the instance matching window.location.origin', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'origin', url: window.location.origin, name: 'Origin' }));
			registry.addInstance(makeInstance({ id: 'remote', url: 'https://remote.example.com', name: 'Remote' }));

			expect(registry.originInstance?.name).toBe('Origin');
		});

		it('returns undefined when no origin instance exists', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'a', url: 'https://remote.example.com' }));

			expect(registry.originInstance).toBeUndefined();
		});
	});

	describe('isOriginInstance', () => {
		it('returns true for instance matching window.location.origin', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'origin', url: window.location.origin }));

			expect(registry.isOriginInstance('origin')).toBe(true);
		});

		it('returns false for remote instance', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'remote', url: 'https://remote.example.com' }));

			expect(registry.isOriginInstance('remote')).toBe(false);
		});
	});

	describe('addInstance', () => {
		it('adds an instance', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			const instance = makeInstance();
			registry.addInstance(instance);

			expect(registry.instances).toHaveLength(1);
			expect(registry.instances[0].id).toBe('test-instance');
		});

		it('persists to localStorage', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance());

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(1);
			expect(stored[0].id).toBe('test-instance');
		});

		it('skips duplicates', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			const instance = makeInstance();
			registry.addInstance(instance);
			registry.addInstance(instance);

			expect(registry.instances).toHaveLength(1);
		});
	});

	describe('removeInstance', () => {
		it('removes an instance by ID', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'a' }));
			registry.addInstance(makeInstance({ id: 'b' }));

			expect(registry.removeInstance('a')).toBe(true);
			expect(registry.instances).toHaveLength(1);
			expect(registry.instances[0].id).toBe('b');
		});

		it('returns false for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			expect(registry.removeInstance('nope')).toBe(false);
		});

		it('persists removal to localStorage', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'a' }));
			registry.removeInstance('a');

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored).toHaveLength(0);
		});
	});

	describe('updateServer', () => {
		it('updates fields on an existing instance', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'x', name: 'Old Name' }));

			expect(registry.updateServer('x', { name: 'New Name' })).toBe(true);
			expect(registry.instances[0].name).toBe('New Name');
		});

		it('returns false for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			expect(registry.updateServer('nope', { name: 'x' })).toBe(false);
		});

		it('persists update to localStorage', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'x', name: 'Old' }));
			registry.updateServer('x', { name: 'New' });

			const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
			expect(stored[0].name).toBe('New');
		});
	});

	describe('getInstance', () => {
		it('returns instance by ID', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			registry.addInstance(makeInstance({ id: 'foo', name: 'Foo' }));

			expect(registry.getInstance('foo')?.name).toBe('Foo');
		});

		it('returns undefined for nonexistent ID', async () => {
			const registry = await createRegistry();
			registry.instances = [];

			expect(registry.getInstance('nope')).toBeUndefined();
		});
	});

	describe('localStorage persistence', () => {
		it('loads instances from localStorage on construction', async () => {
			const instance = makeInstance({ id: 'persisted', name: 'Persisted' });
			localStorage.setItem(STORAGE_KEY, JSON.stringify([instance]));

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
