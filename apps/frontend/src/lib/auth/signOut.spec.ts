import { afterEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '$lib/state/server/registry.svelte';
import { SIGN_OUT_TIMEOUT_MS, signOutServer } from './signOut';

const remoteServer: RegisteredServer = {
	id: 'remote',
	url: 'https://remote.example.test',
	name: 'Remote',
	iconUrl: null,
	token: 'remote-token',
	userId: 'user-1',
	userLogin: 'alice',
	userDisplayName: 'Alice',
	userAvatarUrl: null,
	reauthRequiredAt: null,
	addedAt: 1
};

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe('signOutServer', () => {
	it('aborts stale remote logout requests', async () => {
		vi.useFakeTimers();

		const fetchMock = vi.fn(
			(_url: string | URL | Request, init?: RequestInit) =>
				new Promise<Response>((_resolve, reject) => {
					init?.signal?.addEventListener('abort', () => {
						reject(new DOMException('The operation was aborted.', 'AbortError'));
					});
				})
		);
		vi.stubGlobal('fetch', fetchMock);

		const logoutError = signOutServer(remoteServer, false).catch((error) => error);
		await vi.advanceTimersByTimeAsync(SIGN_OUT_TIMEOUT_MS);

		await expect(logoutError).resolves.toMatchObject({ name: 'AbortError' });
		expect(fetchMock).toHaveBeenCalledWith(
			'https://remote.example.test/auth/logout',
			expect.objectContaining({
				method: 'POST',
				headers: { Authorization: 'Bearer remote-token' },
				signal: expect.any(AbortSignal)
			})
		);
	});
});
