import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import { RoomMessagesStore, ThreadMessagesStore } from './messages.svelte';

/**
 * Minimal GraphQLClient stand-in. `reconnectCount` is a Svelte `$state` so
 * the store's internal `$effect.root` reacts to `bumpReconnect()` just like
 * it would with a real client.
 */
class FakeGqlClient {
	reconnectCount = $state(0);
	client: Client;
	queryMock: ReturnType<typeof vi.fn>;

	constructor(queryData: unknown = null) {
		this.queryMock = vi.fn(() => ({
			toPromise: () => Promise.resolve({ data: queryData, error: null })
		}));
		this.client = {
			query: this.queryMock,
			mutation: vi.fn(),
			subscription: vi.fn()
		} as unknown as Client;
	}

	bumpReconnect() {
		this.reconnectCount++;
		flushSync();
	}
}

async function settle() {
	await Promise.resolve();
	await Promise.resolve();
	flushSync();
}

describe('RoomMessagesStore — lifecycle ownership', () => {
	it('runs an initial fetch on setRoom', async () => {
		const fake = new FakeGqlClient({ room: { events: { events: [], hasOlder: false, hasNewer: false } } });
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();

		expect(fake.queryMock).toHaveBeenCalledTimes(1);
		store.dispose();
	});

	it('triggers a catch-up query when reconnectCount increments', async () => {
		const fake = new FakeGqlClient({ room: { events: { events: [], hasOlder: false, hasNewer: false } } });
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(1);

		fake.bumpReconnect();
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(2);

		store.dispose();
	});

	it('stops reacting to reconnects after dispose()', async () => {
		const fake = new FakeGqlClient({ room: { events: { events: [], hasOlder: false, hasNewer: false } } });
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(1);

		store.dispose();
		fake.bumpReconnect();
		await settle();

		// Still just the initial fetchLatest — the reconnect listener is gone.
		expect(fake.queryMock).toHaveBeenCalledTimes(1);
	});

	it('does not catch up if setRoom has not been called', async () => {
		const fake = new FakeGqlClient();
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);

		fake.bumpReconnect();
		await settle();

		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});

	it('dispose() is idempotent', () => {
		const fake = new FakeGqlClient();
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);
		store.dispose();
		expect(() => store.dispose()).not.toThrow();
	});
});

describe('ThreadMessagesStore — lifecycle ownership', () => {
	it('triggers a catch-up query when reconnectCount increments', async () => {
		const fake = new FakeGqlClient({
			room: { event: { id: 't1', threadReplies: [] } }
		});
		const store = new ThreadMessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setThread('room-1', 't1');
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(1);

		fake.bumpReconnect();
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(2);

		store.dispose();
	});

	it('does not catch up if setThread has not been called', async () => {
		const fake = new FakeGqlClient();
		const store = new ThreadMessagesStore(fake as unknown as GraphQLClient, () => null);

		fake.bumpReconnect();
		await settle();

		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});
});
