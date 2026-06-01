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
	it('applies MessageEditedEvent payloads inline without refetching', async () => {
		const fake = new FakeGqlClient({
			room: {
				events: {
					events: [
						{
							id: 'm1',
							createdAt: '2026-05-27T00:00:00Z',
							actorId: 'u1',
							actor: null,
							event: {
								__typename: 'MessagePostedEvent',
								roomId: 'room-1',
								body: 'before',
								attachments: [],
								linkPreview: null,
								updatedAt: null,
								inReplyTo: null,
								threadRootEventId: null,
								echoOfEventId: null,
								echoFromThreadRootEventId: null,
								replyCount: 0,
								lastReplyAt: null,
								threadParticipants: [],
								viewerIsFollowingThread: null
							}
						}
					],
					hasOlder: false,
					hasNewer: false
				}
			}
		});
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		fake.queryMock.mockClear();

		store.ingestServerEvent({
			id: 'edit-1',
			createdAt: '2026-05-27T00:00:01Z',
			actorId: 'u1',
			actor: null,
			event: {
				__typename: 'MessageEditedEvent',
				roomId: 'room-1',
				messageEventId: 'm1',
				body: 'after',
				attachments: [],
				linkPreview: null,
				updatedAt: '2026-05-27T00:00:01Z'
			}
		} as never);

		expect(store.rootEvents[0].event).toMatchObject({
			__typename: 'MessagePostedEvent',
			body: 'after',
			updatedAt: '2026-05-27T00:00:01Z'
		});
		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});

	it('applies MessageRetractedEvent payloads inline without refetching', async () => {
		const fake = new FakeGqlClient({
			room: {
				events: {
					events: [
						{
							id: 'm1',
							createdAt: '2026-05-27T00:00:00Z',
							actorId: 'u1',
							actor: null,
							event: {
								__typename: 'MessagePostedEvent',
								roomId: 'room-1',
								body: 'before',
								attachments: [{ id: 'a1' }],
								linkPreview: null,
								updatedAt: null,
								inReplyTo: null,
								threadRootEventId: null,
								echoOfEventId: null,
								echoFromThreadRootEventId: null,
								replyCount: 0,
								lastReplyAt: null,
								threadParticipants: [],
								viewerIsFollowingThread: null
							}
						}
					],
					hasOlder: false,
					hasNewer: false
				}
			}
		});
		const store = new RoomMessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		fake.queryMock.mockClear();

		store.ingestServerEvent({
			id: 'retract-1',
			createdAt: '2026-05-27T00:00:01Z',
			actorId: 'u1',
			actor: null,
			event: {
				__typename: 'MessageRetractedEvent',
				roomId: 'room-1',
				messageEventId: 'm1',
				retractedReason: null
			}
		} as never);

		expect(store.rootEvents[0].event).toMatchObject({
			__typename: 'MessagePostedEvent',
			body: null,
			attachments: []
		});
		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});

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
			room: { event: { id: 't1', event: { __typename: 'MessagePostedEvent', threadReplies: [] } } }
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
