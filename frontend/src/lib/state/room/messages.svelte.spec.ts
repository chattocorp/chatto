import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import { MessagesStore } from './messages.svelte';

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
		const queryDataQueue = Array.isArray(queryData) ? [...queryData] : null;
		this.queryMock = vi.fn(() => ({
			toPromise: () => {
				const data =
					queryDataQueue === null
						? queryData
						: queryDataQueue.length > 1
							? queryDataQueue.shift()
							: queryDataQueue[0];
				return Promise.resolve(data).then((resolvedData) => ({ data: resolvedData, error: null }));
			}
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

function deferred<T>() {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((r) => {
		resolve = r;
	});
	return { promise, resolve };
}

function threadMessageEvent(id: string, threadRootEventId: string | null = null) {
	const offsetSeconds = Number(id.replace(/\D/g, '')) || 0;
	return {
		id,
		createdAt: new Date(Date.UTC(2026, 4, 27, 0, 0, offsetSeconds)).toISOString(),
		actorId: 'u1',
		actor: null,
		event: {
			__typename: 'MessagePostedEvent',
			roomId: 'room-1',
			body: id,
			attachments: [],
			linkPreview: null,
			updatedAt: null,
			inReplyTo: null,
			threadRootEventId,
			echoOfEventId: null,
			echoFromThreadRootEventId: null,
			replyCount: 0,
			lastReplyAt: null,
			threadParticipants: [],
			viewerIsFollowingThread: null
		}
	};
}

function threadQueryResult({
	replies,
	startCursor,
	endCursor,
	hasOlder,
	hasNewer
}: {
	replies: unknown[];
	startCursor: string | null;
	endCursor: string | null;
	hasOlder: boolean;
	hasNewer: boolean;
}) {
	return {
		room: {
			event: {
				...threadMessageEvent('t1'),
				event: {
					...threadMessageEvent('t1').event,
					threadReplies: {
						events: replies,
						startCursor,
						endCursor,
						hasOlder,
						hasNewer
					}
				}
			}
		}
	};
}

function roomEventsResult({
	events,
	startCursor,
	endCursor,
	hasOlder,
	hasNewer
}: {
	events: unknown[];
	startCursor: string | null;
	endCursor: string | null;
	hasOlder: boolean;
	hasNewer: boolean;
}) {
	return {
		room: {
			events: {
				events,
				startCursor,
				endCursor,
				hasOlder,
				hasNewer
			}
		}
	};
}

describe('MessagesStore — room lifecycle ownership', () => {
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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

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

	it('hides only the echo when an echo is retracted', async () => {
		const fake = new FakeGqlClient({
			room: {
				events: {
					events: [
						{
							id: 'root',
							createdAt: '2026-05-27T00:00:00Z',
							actorId: 'u1',
							actor: null,
							event: {
								__typename: 'MessagePostedEvent',
								roomId: 'room-1',
								body: 'root',
								attachments: [],
								linkPreview: null,
								updatedAt: null,
								inReplyTo: null,
								threadRootEventId: null,
								echoOfEventId: null,
								echoFromThreadRootEventId: null,
								replyCount: 1,
								lastReplyAt: null,
								threadParticipants: [],
								viewerIsFollowingThread: null
							}
						},
						{
							id: 'echo',
							createdAt: '2026-05-27T00:00:01Z',
							actorId: 'u1',
							actor: null,
							event: {
								__typename: 'MessagePostedEvent',
								roomId: 'room-1',
								body: 'reply',
								attachments: [],
								linkPreview: null,
								updatedAt: null,
								inReplyTo: null,
								threadRootEventId: null,
								echoOfEventId: 'reply',
								echoFromThreadRootEventId: 'root',
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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		fake.queryMock.mockClear();

		store.ingestServerEvent({
			id: 'retract-echo',
			createdAt: '2026-05-27T00:00:02Z',
			actorId: 'u1',
			actor: null,
			event: {
				__typename: 'MessageRetractedEvent',
				roomId: 'room-1',
				messageEventId: 'echo',
				retractedReason: null
			}
		} as never);

		expect(store.rootEvents.map((event) => event.id)).toEqual(['root']);
		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});

	it('tombstones visible echoes when the original reply is retracted', async () => {
		const fake = new FakeGqlClient({
			room: {
				events: {
					events: [
						{
							id: 'echo',
							createdAt: '2026-05-27T00:00:01Z',
							actorId: 'u1',
							actor: null,
							event: {
								__typename: 'MessagePostedEvent',
								roomId: 'room-1',
								body: 'reply',
								attachments: [{ id: 'a1' }],
								linkPreview: null,
								updatedAt: null,
								inReplyTo: null,
								threadRootEventId: null,
								echoOfEventId: 'reply',
								echoFromThreadRootEventId: 'root',
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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		fake.queryMock.mockClear();

		store.ingestServerEvent({
			id: 'retract-original',
			createdAt: '2026-05-27T00:00:02Z',
			actorId: 'u1',
			actor: null,
			event: {
				__typename: 'MessageRetractedEvent',
				roomId: 'room-1',
				messageEventId: 'reply',
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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();

		expect(fake.queryMock).toHaveBeenCalledTimes(1);
		store.dispose();
	});

	it('triggers a catch-up query when reconnectCount increments', async () => {
		const fake = new FakeGqlClient({ room: { events: { events: [], hasOlder: false, hasNewer: false } } });
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(1);

		fake.bumpReconnect();
		await settle();
		expect(fake.queryMock).toHaveBeenCalledTimes(2);

		store.dispose();
	});

	it('keeps already-loaded room events before reconnect catch-up pages', async () => {
		const fake = new FakeGqlClient([
			roomEventsResult({
				events: [threadMessageEvent('m1'), threadMessageEvent('m2')],
				startCursor: 'seq:1',
				endCursor: 'seq:2',
				hasOlder: false,
				hasNewer: false
			}),
			roomEventsResult({
				events: [threadMessageEvent('m3'), threadMessageEvent('m4')],
				startCursor: 'seq:3',
				endCursor: 'seq:4',
				hasOlder: false,
				hasNewer: true
			})
		]);
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();

		fake.bumpReconnect();
		await settle();

		expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2', 'm3', 'm4']);
		store.dispose();
	});

	it('keeps live room events after in-flight reconnect catch-up pages', async () => {
		const catchUp = deferred<unknown>();
		const fake = new FakeGqlClient([
			roomEventsResult({
				events: [threadMessageEvent('m1'), threadMessageEvent('m2')],
				startCursor: 'seq:1',
				endCursor: 'seq:2',
				hasOlder: false,
				hasNewer: false
			}),
			catchUp.promise
		]);
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setRoom('room-1');
		await settle();

		fake.bumpReconnect();
		store.ingestServerEvent(threadMessageEvent('m5') as never);
		catchUp.resolve(
			roomEventsResult({
				events: [threadMessageEvent('m3'), threadMessageEvent('m4')],
				startCursor: 'seq:3',
				endCursor: 'seq:4',
				hasOlder: false,
				hasNewer: false
			})
		);
		await settle();

		expect(store.rootEvents.map((event) => event.id)).toEqual(['m1', 'm2', 'm3', 'm4', 'm5']);
		store.dispose();
	});

	it('stops reacting to reconnects after dispose()', async () => {
		const fake = new FakeGqlClient({ room: { events: { events: [], hasOlder: false, hasNewer: false } } });
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		fake.bumpReconnect();
		await settle();

		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});

	it('dispose() is idempotent', () => {
		const fake = new FakeGqlClient();
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);
		store.dispose();
		expect(() => store.dispose()).not.toThrow();
	});
});

describe('MessagesStore — thread lifecycle ownership', () => {
	it('loads older reply pages when the first thread page is not complete', async () => {
		const fake = new FakeGqlClient([
			threadQueryResult({
				replies: [threadMessageEvent('r51', 't1'), threadMessageEvent('r52', 't1')],
				startCursor: 'seq:51',
				endCursor: 'seq:52',
				hasOlder: true,
				hasNewer: false
			}),
			threadQueryResult({
				replies: [threadMessageEvent('r49', 't1'), threadMessageEvent('r50', 't1')],
				startCursor: 'seq:49',
				endCursor: 'seq:50',
				hasOlder: false,
				hasNewer: true
			})
		]);
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		store.setThread('room-1', 't1');
		await settle();

		expect(store.threadEvents.map((event) => event.id)).toEqual(['t1', 'r51', 'r52']);
		expect(store.hasReachedStart).toBe(false);

		await store.loadMore();
		await settle();

		expect(fake.queryMock).toHaveBeenCalledTimes(2);
		expect(fake.queryMock.mock.calls[1][1]).toMatchObject({
			roomId: 'room-1',
			threadRootEventId: 't1',
			limit: 50,
			before: 'seq:51'
		});
		expect(store.threadEvents.map((event) => event.id)).toEqual([
			't1',
			'r49',
			'r50',
			'r51',
			'r52'
		]);
		expect(store.hasReachedStart).toBe(true);

		store.dispose();
	});

	it('triggers a catch-up query when reconnectCount increments', async () => {
		const fake = new FakeGqlClient({
			room: {
				event: {
					id: 't1',
					event: {
						__typename: 'MessagePostedEvent',
						threadReplies: {
							events: [],
							startCursor: null,
							endCursor: null,
							hasOlder: false,
							hasNewer: false
						}
					}
				}
			}
		});
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

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
		const store = new MessagesStore(fake as unknown as GraphQLClient, () => null);

		fake.bumpReconnect();
		await settle();

		expect(fake.queryMock).not.toHaveBeenCalled();
		store.dispose();
	});
});
