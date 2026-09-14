import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
	clearPendingEmailVerification,
	readPendingEmailVerification,
	storePendingEmailVerification
} from './verifiedEmailChallenge';

function memoryStorage(): Storage {
	const values = new Map<string, string>();
	return {
		get length() {
			return values.size;
		},
		clear: () => values.clear(),
		getItem: (key) => values.get(key) ?? null,
		key: (index) => [...values.keys()][index] ?? null,
		removeItem: (key) => values.delete(key),
		setItem: (key, value) => values.set(key, value)
	};
}

describe('pending email verification', () => {
	beforeEach(() => vi.stubGlobal('sessionStorage', memoryStorage()));

	it('keeps pending addresses isolated by server', () => {
		expect(storePendingEmailVerification('one', 'user-one', 'first@example.com')).toBe(true);
		expect(readPendingEmailVerification('one', 'user-one')).toBe('first@example.com');
		expect(readPendingEmailVerification('two', 'user-one')).toBe('');
		expect(readPendingEmailVerification('one', 'user-two')).toBe('');
	});

	it('clears only the expected challenge', () => {
		storePendingEmailVerification('one', 'user-one', 'first@example.com');
		clearPendingEmailVerification('one', 'user-one', 'other@example.com');
		expect(readPendingEmailVerification('one', 'user-one')).toBe('first@example.com');
		clearPendingEmailVerification('one', 'user-one', 'first@example.com');
		expect(readPendingEmailVerification('one', 'user-one')).toBe('');
	});

	it('rejects storage that cannot be read back', () => {
		vi.stubGlobal('sessionStorage', {
			setItem: () => undefined,
			getItem: () => null
		});
		expect(storePendingEmailVerification('one', 'user-one', 'first@example.com')).toBe(false);
	});
});
