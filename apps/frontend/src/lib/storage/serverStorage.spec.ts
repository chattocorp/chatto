import { describe, it, expect, beforeEach, vi } from 'vitest';
import { serverStorageKey, migrateStorageKey } from './serverStorage';

// Provide a minimal localStorage stub for the Node test environment
const storage = new Map<string, string>();
const localStorageMock: Storage = {
	getItem: (key: string) => storage.get(key) ?? null,
	setItem: (key: string, value: string) => storage.set(key, value),
	removeItem: (key: string) => storage.delete(key),
	clear: () => storage.clear(),
	get length() {
		return storage.size;
	},
	key: (index: number) => [...storage.keys()][index] ?? null
};
vi.stubGlobal('localStorage', localStorageMock);

describe('serverStorageKey', () => {
	it('produces a namespaced key', () => {
		expect(serverStorageKey('chat-example-com', 'lastRooms')).toBe(
			'chatto:i:chat-example-com:lastRooms'
		);
	});

	it('handles different instance IDs', () => {
		expect(serverStorageKey('localhost', 'lastSpace')).toBe('chatto:i:localhost:lastSpace');
	});

	it('handles compound suffixes', () => {
		expect(serverStorageKey('my-instance', 'space:abc:collapsed-sections')).toBe(
			'chatto:i:my-instance:space:abc:collapsed-sections'
		);
	});
});

describe('migrateStorageKey', () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it('moves value from legacy key to namespaced key', () => {
		localStorage.setItem('chatto:lastRooms', '{"space1":"room1"}');

		migrateStorageKey('my-instance', 'chatto:lastRooms', 'lastRooms');

		expect(localStorage.getItem('chatto:i:my-instance:lastRooms')).toBe('{"space1":"room1"}');
		expect(localStorage.getItem('chatto:lastRooms')).toBeNull();
	});

	it('is a no-op if the new key already exists', () => {
		localStorage.setItem('chatto:lastRooms', '{"old":"data"}');
		localStorage.setItem('chatto:i:my-instance:lastRooms', '{"new":"data"}');

		migrateStorageKey('my-instance', 'chatto:lastRooms', 'lastRooms');

		// New key keeps its value, old key is not removed
		expect(localStorage.getItem('chatto:i:my-instance:lastRooms')).toBe('{"new":"data"}');
		expect(localStorage.getItem('chatto:lastRooms')).toBe('{"old":"data"}');
	});

	it('is a no-op if the old key does not exist', () => {
		migrateStorageKey('my-instance', 'chatto:lastRooms', 'lastRooms');

		expect(localStorage.getItem('chatto:i:my-instance:lastRooms')).toBeNull();
	});
});
