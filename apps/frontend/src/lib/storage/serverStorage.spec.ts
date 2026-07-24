import { describe, it, expect } from 'vitest';
import { serverStorageKey } from './serverStorage';

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
