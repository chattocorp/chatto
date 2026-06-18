import type { Client } from '@urql/svelte';
import { createContext } from 'svelte';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { graphql, useFragment } from '$lib/gql';
import { isUnsupportedGraphQLArgumentError } from '$lib/gql/compatibility';
import {
	UserAvatarUserFragmentDoc,
	type PresenceStatus,
	type RoomMembersQuery
} from '$lib/gql/graphql';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';

export const ROOM_MEMBERS_PAGE_SIZE = 50;
const MENTION_MEMBER_SEARCH_LIMIT = 10;

/**
 * Room member data for the current room.
 */
export type RoomMember = {
	id: string;
	login: string;
	displayName: string;
	deleted?: boolean;
	avatarUrl?: string | null;
	presenceStatus: PresenceStatus;
};

export type RoomMembersPage = {
	members: RoomMember[];
	totalCount: number;
	hasMore: boolean;
};

const RoomMembersQuery = graphql(`
	query RoomMembers($roomId: ID!, $search: String, $limit: Int!, $offset: Int!) {
		room(roomId: $roomId) {
			members(search: $search, limit: $limit, offset: $offset) {
				users {
					...UserAvatarUser
				}
				totalCount
				hasMore
			}
		}
	}
`);

const RoomMembersWithoutSearchQuery = graphql(`
	query RoomMembersWithoutSearch($roomId: ID!, $limit: Int!, $offset: Int!) {
		room(roomId: $roomId) {
			members(limit: $limit, offset: $offset) {
				users {
					...UserAvatarUser
				}
				totalCount
				hasMore
			}
		}
	}
`);

function memberMatchesSearch(member: RoomMember, search: string): boolean {
	const query = search.trim().toLowerCase();
	if (!query) return true;
	return (
		member.login.toLowerCase().includes(query) ||
		member.displayName.toLowerCase().includes(query)
	);
}

function mapPage(connectionData: {
	users: NonNullable<RoomMembersQuery['room']>['members']['users'];
	totalCount: number;
	hasMore: boolean;
}): RoomMembersPage {
	return {
		members: connectionData.users.map((m) => useFragment(UserAvatarUserFragmentDoc, m)),
		totalCount: connectionData.totalCount,
		hasMore: connectionData.hasMore
	};
}

/**
 * Room member store for the current room.
 *
 * The store owns pagination/search state. Components decide when to call
 * `ensureLoaded`, so opening a room no longer fetches member pages eagerly.
 */
export class RoomMembersStore {
	members = $state.raw<RoomMember[]>([]);
	totalCount = $state(0);
	hasMore = $state(false);
	hasLoaded = $state(false);
	isInitialLoading = $state(false);
	isLoadingMore = $state(false);
	searchInput = $state('');
	activeSearch = $state('');
	searchUnsupported = $state(false);
	livePresence = new SvelteMap<string, PresenceStatus>();
	presenceVersion = $state(0);

	private readonly client: Client | null;
	private roomId = '';
	#loadId = 0;

	constructor(gqlClient?: GraphQLClient) {
		this.client = gqlClient?.client ?? null;
	}

	setRoom(roomId: string): void {
		if (this.roomId === roomId) return;
		this.roomId = roomId;
		this.reset();
	}

	ensureLoaded(): void {
		if (!this.roomId || this.isInitialLoading || this.hasLoaded) return;
		void this.loadInitial();
	}

	async setSearch(search: string): Promise<void> {
		const nextSearch = search.trim();
		this.searchInput = search;
		if (nextSearch === this.activeSearch) return;
		await this.loadInitial(nextSearch);
	}

	async loadInitial(search = this.activeSearch): Promise<void> {
		if (!this.roomId || !this.client) return;
		const loadId = ++this.#loadId;
		this.isInitialLoading = true;
		this.activeSearch = search;
		try {
			const page = await this.fetchPage(0, ROOM_MEMBERS_PAGE_SIZE, search);
			if (loadId !== this.#loadId) return;
			this.members = page.members;
			this.totalCount = page.totalCount;
			this.hasMore = page.hasMore;
		} catch (error) {
			if (loadId === this.#loadId) {
				console.error('Failed to load room members:', error);
			}
		} finally {
			if (loadId === this.#loadId) {
				this.hasLoaded = true;
				this.isInitialLoading = false;
			}
		}
	}

	async loadMore(): Promise<void> {
		if (this.isLoadingMore || !this.hasMore || !this.roomId || !this.client) return;
		const loadId = this.#loadId;
		this.isLoadingMore = true;
		try {
			const page = await this.fetchPage(
				this.members.length,
				ROOM_MEMBERS_PAGE_SIZE,
				this.activeSearch
			);
			if (loadId !== this.#loadId) return;
			const seen = new SvelteSet(this.members.map((member) => member.id));
			const nextMembers = page.members.filter((member) => !seen.has(member.id));
			this.members = [...this.members, ...nextMembers];
			this.totalCount = page.totalCount;
			this.hasMore = page.hasMore;
		} catch (error) {
			if (loadId === this.#loadId) {
				console.error('Failed to load more room members:', error);
			}
		} finally {
			if (loadId === this.#loadId) {
				this.isLoadingMore = false;
			}
		}
	}

	async refresh(): Promise<void> {
		if (!this.roomId || !this.client) return;
		const limit = Math.max(ROOM_MEMBERS_PAGE_SIZE, this.members.length);
		const loadId = ++this.#loadId;
		this.isInitialLoading = false;
		this.isLoadingMore = false;
		try {
			const page = await this.fetchPage(0, limit, this.activeSearch);
			if (loadId !== this.#loadId) return;
			this.members = page.members;
			this.totalCount = page.totalCount;
			this.hasMore = page.hasMore;
			this.hasLoaded = true;
		} catch (error) {
			if (loadId === this.#loadId) {
				console.error('Failed to refresh room members:', error);
			}
		}
	}

	async searchMembers(search: string, limit = MENTION_MEMBER_SEARCH_LIMIT): Promise<RoomMember[]> {
		if (!this.roomId || !this.client) return this.filteredLoadedMembers(search, limit);
		try {
			const page = await this.fetchPage(0, limit, search);
			return page.members;
		} catch (error) {
			console.error('Failed to search room members:', error);
			return this.filteredLoadedMembers(search, limit);
		}
	}

	ingestServerEvent(serverEvent: EventEnvelope): void {
		const eventData = serverEvent.event;
		if (!eventData) return;
		if (
			(eventData.__typename === 'UserJoinedRoomEvent' ||
				eventData.__typename === 'UserLeftRoomEvent') &&
			eventData.roomId === this.roomId
		) {
			void this.refresh();
		}
	}

	updatePresence(userId: string, status: PresenceStatus): void {
		this.livePresence.set(userId, status);
		this.presenceVersion++;
	}

	private async fetchPage(offset: number, limit: number, search: string): Promise<RoomMembersPage> {
		if (!this.client) return { members: [], totalCount: 0, hasMore: false };
		const normalizedSearch = search.trim();
		if (!this.searchUnsupported) {
			const response = await this.client
				.query(RoomMembersQuery, {
					roomId: this.roomId,
					search: normalizedSearch || null,
					limit,
					offset
				})
				.toPromise();

			if (!response.error) {
				const connectionData = response.data?.room?.members;
				if (!connectionData) throw new Error('Room members response missing connection');
				return mapPage(connectionData);
			}

			if (!isUnsupportedGraphQLArgumentError(response.error, 'search')) {
				throw response.error;
			}
			this.searchUnsupported = true;
		}

		const response = await this.client
			.query(RoomMembersWithoutSearchQuery, { roomId: this.roomId, limit, offset })
			.toPromise();
		if (response.error) throw response.error;
		const connectionData = response.data?.room?.members;
		if (!connectionData) throw new Error('Room members response missing connection');
		const page = mapPage(connectionData);
		if (!normalizedSearch) return page;

		const members = page.members.filter((member) => memberMatchesSearch(member, normalizedSearch));
		return {
			members,
			totalCount: members.length,
			hasMore: page.hasMore
		};
	}

	private filteredLoadedMembers(search: string, limit: number): RoomMember[] {
		return this.members.filter((member) => memberMatchesSearch(member, search)).slice(0, limit);
	}

	private reset(): void {
		this.#loadId++;
		this.members = [];
		this.totalCount = 0;
		this.hasMore = false;
		this.hasLoaded = false;
		this.isInitialLoading = false;
		this.isLoadingMore = false;
		this.searchInput = '';
		this.activeSearch = '';
		this.searchUnsupported = false;
		this.livePresence.clear();
		this.presenceVersion = 0;
	}
}

const [getMembersStoreContext, setMembersStoreContext] = createContext<RoomMembersStore>();

export function setRoomMembersStore(store: RoomMembersStore): RoomMembersStore {
	setMembersStoreContext(store);
	return store;
}

export function createRoomMembers(gqlClient?: GraphQLClient): RoomMembersStore {
	return setRoomMembersStore(new RoomMembersStore(gqlClient));
}

export function getRoomMembersStore(): RoomMembersStore {
	return getMembersStoreContext();
}

export function getRoomMembers(): RoomMember[] {
	return getRoomMembersStore().members;
}

export function getMemberPresence(member: RoomMember): PresenceStatus {
	const state = getRoomMembersStore();
	return state.livePresence.get(member.id) ?? member.presenceStatus;
}
