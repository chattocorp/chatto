import type { Client } from '@urql/svelte';
import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { graphql, useFragment } from '$lib/gql';
import { isUnsupportedGraphQLArgumentError } from '$lib/gql/compatibility';
import {
  UserAvatarUserFragmentDoc,
  type PresenceStatus,
  type RoomMembersQuery
} from '$lib/gql/graphql';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';

export const ROOM_MEMBERS_PAGE_SIZE = 100;
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
  customStatus?: CustomUserStatus | null;
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
    member.login.toLowerCase().includes(query) || member.displayName.toLowerCase().includes(query)
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
 * The store uses the paginated GraphQL API internally, but room initialization
 * eagerly fills `members` with the complete room member list. Message rendering,
 * mention highlighting, popovers, and moderation checks all treat this as
 * canonical room context, not as a lazy sidebar page. Partial page results are
 * never exposed as canonical state.
 */
export class RoomMembersStore {
  members = $state.raw<RoomMember[]>([]);
  totalCount = $state(0);
  hasLoaded = $state(false);
  isInitialLoading = $state(false);
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

  get filteredMembers(): RoomMember[] {
    return this.filterLoadedMembers(this.activeSearch);
  }

  ensureLoaded(): void {
    if (!this.roomId || this.isInitialLoading || this.hasLoaded) return;
    void this.loadInitial();
  }

  async setSearch(search: string): Promise<void> {
    const nextSearch = search.trim();
    this.searchInput = search;
    if (nextSearch === this.activeSearch) return;
    this.activeSearch = nextSearch;
  }

  async loadInitial(): Promise<void> {
    if (!this.roomId || !this.client) return;
    const loadId = ++this.#loadId;
    this.isInitialLoading = true;
    try {
      const page = await this.fetchAllPages();
      if (loadId !== this.#loadId) return;
      this.members = page.members;
      this.totalCount = page.totalCount;
      this.hasLoaded = true;
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

  async refresh(): Promise<void> {
    if (!this.roomId || !this.client) return;
    const loadId = ++this.#loadId;
    this.isInitialLoading = false;
    try {
      const page = await this.fetchAllPages();
      if (loadId !== this.#loadId) return;
      this.members = page.members;
      this.totalCount = page.totalCount;
      this.hasLoaded = true;
    } catch (error) {
      if (loadId === this.#loadId) {
        console.error('Failed to refresh room members:', error);
      }
    }
  }

  async searchMembers(search: string, limit = MENTION_MEMBER_SEARCH_LIMIT): Promise<RoomMember[]> {
    return this.filteredLoadedMembers(search, limit);
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

  private async fetchAllPages(): Promise<RoomMembersPage> {
    const members: RoomMember[] = [];
    let totalCount = 0;
    let hasMore = true;
    let nextOffset = 0;

    while (hasMore) {
      const page = await this.fetchPage(nextOffset, ROOM_MEMBERS_PAGE_SIZE, '');
      members.push(...page.members);
      totalCount = page.totalCount;
      hasMore = page.hasMore;
      nextOffset += page.members.length;

      if (page.members.length === 0) {
        break;
      }
    }

    return { members, totalCount, hasMore };
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

  private filterLoadedMembers(search: string): RoomMember[] {
    return this.members.filter((member) => memberMatchesSearch(member, search));
  }

  private filteredLoadedMembers(search: string, limit: number): RoomMember[] {
    return this.filterLoadedMembers(search).slice(0, limit);
  }

  private reset(): void {
    this.#loadId++;
    this.members = [];
    this.totalCount = 0;
    this.hasLoaded = false;
    this.isInitialLoading = false;
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
