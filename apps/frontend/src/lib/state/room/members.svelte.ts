import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { type PresenceStatus } from '$lib/render/types';
import {
  createMemberDirectoryAPI,
  type DirectoryMember,
  type MemberDirectoryAPI,
  type MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';

export const ROOM_MEMBERS_PAGE_SIZE = 250;
const MENTION_MEMBER_SEARCH_LIMIT = 10;
const roomMemberInvalidatingEventKinds = new Set<RoomEventKind>([
  RoomEventKind.UserJoinedRoom,
  RoomEventKind.UserLeftRoom
]);

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

type MemberSearchCacheEntry = {
  members: RoomMember[];
  complete: boolean;
};

function memberMatchesSearch(member: RoomMember, search: string): boolean {
  const query = search.trim().toLowerCase();
  if (!query) return true;
  return (
    member.login.toLowerCase().includes(query) || member.displayName.toLowerCase().includes(query)
  );
}

function mapPage(page: MemberDirectoryPage): RoomMembersPage {
  return {
    members: page.members.map(memberFromDirectory),
    totalCount: page.totalCount,
    hasMore: page.hasMore
  };
}

function eventRoomId(eventData: EventEnvelope['event']): string | null {
  if (!eventData || !('roomId' in eventData) || typeof eventData.roomId !== 'string') return null;
  return eventData.roomId;
}

/**
 * Room member store for the current room.
 *
 * The store publishes the first paginated Connect response immediately, then fills `members` with
 * the remaining pages in the background. `hasFirstPage` marks interactive readiness while
 * `hasLoadedAll` marks complete membership for mention rendering and other exhaustive consumers.
 * Searches use a separate cache until their matching directory page enters canonical order.
 */
export class RoomMembersStore {
  members = $state.raw<RoomMember[]>([]);
  totalCount = $state(0);
  hasFirstPage = $state(false);
  hasLoadedAll = $state(false);
  isInitialLoading = $state(false);
  isBackgroundLoading = $state(false);
  loadError = $state<string | null>(null);
  searchInput = $state('');
  activeSearch = $state('');
  livePresence = new SvelteMap<string, PresenceStatus>();
  presenceVersion = $state(0);

  private readonly api: MemberDirectoryAPI | null;
  private roomId = '';
  #loadId = 0;
  #searchCache = new SvelteMap<string, MemberSearchCacheEntry>();

  constructor(source?: ServerConnection | MemberDirectoryAPI | null) {
    if (!source) {
      this.api = null;
    } else if ('listRoomMembers' in source) {
      this.api = source;
    } else {
      this.api = createMemberDirectoryAPI({
        baseUrl: source.connectBaseUrl,
        bearerToken: source.bearerToken
      });
    }
  }

  setRoom(roomId: string): void {
    if (this.roomId === roomId) return;
    this.roomId = roomId;
    this.reset();
  }

  get filteredMembers(): RoomMember[] {
    const query = this.activeSearch.trim().toLowerCase();
    if (query && !this.hasLoadedAll) {
      const searched = this.#searchCache.get(query);
      if (searched) return searched.members;
    }
    return this.filterLoadedMembers(this.activeSearch);
  }

  /** Compatibility alias for consumers that only care whether hydration is complete. */
  get hasLoaded(): boolean {
    return this.hasLoadedAll;
  }

  ensureLoaded(): void {
    if (
      !this.roomId ||
      this.isInitialLoading ||
      this.isBackgroundLoading ||
      this.hasLoadedAll ||
      this.loadError
    )
      return;
    void this.loadInitial();
  }

  async setSearch(search: string): Promise<void> {
    const nextSearch = search.trim();
    this.searchInput = search;
    if (nextSearch === this.activeSearch) return;
    this.activeSearch = nextSearch;
    if (nextSearch && !this.hasLoadedAll) {
      await this.searchAllMembers(nextSearch);
    }
  }

  async loadInitial(): Promise<void> {
    if (!this.roomId || !this.api) return;
    const loadId = ++this.#loadId;
    this.isInitialLoading = true;
    this.isBackgroundLoading = false;
    this.loadError = null;
    try {
      await this.loadPages(loadId);
    } catch (error) {
      if (loadId === this.#loadId) {
        this.loadError = error instanceof Error ? error.message : 'Failed to load room members';
        console.error('Failed to load room members:', error);
      }
    } finally {
      if (loadId === this.#loadId) {
        this.isInitialLoading = false;
        this.isBackgroundLoading = false;
      }
    }
  }

  async refresh(): Promise<void> {
    if (!this.roomId || !this.api) return;
    const loadId = ++this.#loadId;
    this.isInitialLoading = false;
    this.isBackgroundLoading = false;
    this.hasLoadedAll = false;
    this.loadError = null;
    this.#searchCache.clear();
    try {
      await this.loadPages(loadId);
    } catch (error) {
      if (loadId === this.#loadId) {
        this.loadError = error instanceof Error ? error.message : 'Failed to refresh room members';
        console.error('Failed to refresh room members:', error);
      }
    } finally {
      if (loadId === this.#loadId) {
        this.isInitialLoading = false;
        this.isBackgroundLoading = false;
      }
    }
  }

  async searchMembers(search: string, limit = MENTION_MEMBER_SEARCH_LIMIT): Promise<RoomMember[]> {
    const normalizedSearch = search.trim();
    if (!normalizedSearch || this.hasLoadedAll || !this.roomId || !this.api) {
      return this.filteredLoadedMembers(normalizedSearch, limit);
    }

    const roomId = this.roomId;
    const loadId = this.#loadId;
    const cached = this.#searchCache.get(normalizedSearch.toLowerCase());
    if (cached && (cached.complete || cached.members.length >= limit)) {
      return cached.members.slice(0, limit);
    }
    let page: RoomMembersPage;
    try {
      page = await this.fetchPage(0, limit, normalizedSearch);
    } catch (error) {
      console.error('Failed to search room members:', error);
      return this.filteredLoadedMembers(normalizedSearch, limit);
    }
    if (roomId !== this.roomId || loadId !== this.#loadId) return [];
    this.#searchCache.set(normalizedSearch.toLowerCase(), {
      members: page.members,
      complete: !page.hasMore
    });
    return page.members.slice(0, limit);
  }

  private async searchAllMembers(search: string): Promise<void> {
    const query = search.trim().toLowerCase();
    const loadId = this.#loadId;
    let members: RoomMember[] = [];
    let offset = 0;
    let hasMore = true;

    while (hasMore) {
      let page: RoomMembersPage;
      try {
        page = await this.fetchPage(offset, ROOM_MEMBERS_PAGE_SIZE, search);
      } catch (error) {
        console.error('Failed to search room members:', error);
        return;
      }
      if (
        loadId !== this.#loadId ||
        query !== this.activeSearch.trim().toLowerCase() ||
        !this.roomId
      )
        return;

      members = appendPageMembers(members, page.members);
      hasMore = page.hasMore && page.members.length > 0;
      offset += page.members.length;
      this.#searchCache.set(query, { members, complete: !hasMore });
    }
  }

  ingestServerEvent(serverEvent: EventEnvelope): void {
    const eventData = serverEvent.event;
    if (!eventData) return;
    const kind = roomEventKind(eventData);
    if (
      kind &&
      roomMemberInvalidatingEventKinds.has(kind) &&
      eventRoomId(eventData) === this.roomId
    ) {
      void this.refresh();
    }
  }

  updatePresence(userId: string, status: PresenceStatus): void {
    this.livePresence.set(userId, status);
    this.presenceVersion++;
  }

  private async loadPages(loadId: number): Promise<void> {
    let nextOffset = 0;
    let hasMore = true;
    let firstPage = true;

    while (hasMore) {
      const page = await this.fetchPage(nextOffset, ROOM_MEMBERS_PAGE_SIZE, '');
      if (loadId !== this.#loadId) return;

      this.members = firstPage ? page.members : appendPageMembers(this.members, page.members);
      this.totalCount = page.totalCount;
      hasMore = page.hasMore;
      nextOffset += page.members.length;

      if (firstPage) {
        firstPage = false;
        this.hasFirstPage = true;
        this.hasLoadedAll = !hasMore;
        this.isInitialLoading = false;
        this.isBackgroundLoading = hasMore;
      }

      if (page.members.length === 0) {
        hasMore = false;
        break;
      }
    }

    if (loadId === this.#loadId) {
      this.hasLoadedAll = true;
      this.isBackgroundLoading = false;
    }
  }

  private async fetchPage(offset: number, limit: number, search: string): Promise<RoomMembersPage> {
    if (!this.api) return { members: [], totalCount: 0, hasMore: false };
    const normalizedSearch = search.trim();
    return mapPage(await this.api.listRoomMembers(this.roomId, normalizedSearch, limit, offset));
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
    this.hasFirstPage = false;
    this.hasLoadedAll = false;
    this.isInitialLoading = false;
    this.isBackgroundLoading = false;
    this.loadError = null;
    this.searchInput = '';
    this.activeSearch = '';
    this.#searchCache.clear();
    this.livePresence.clear();
    this.presenceVersion = 0;
  }
}

function appendPageMembers(current: RoomMember[], incoming: RoomMember[]): RoomMember[] {
  if (incoming.length === 0) return current;
  const incomingIds = new Set(incoming.map((member) => member.id));
  return [...current.filter((member) => !incomingIds.has(member.id)), ...incoming];
}

const [getMembersStoreContext, setMembersStoreContext] = createContext<RoomMembersStore>();

export function setRoomMembersStore(store: RoomMembersStore): RoomMembersStore {
  setMembersStoreContext(store);
  return store;
}

export function createRoomMembers(serverConnection?: ServerConnection): RoomMembersStore {
  return setRoomMembersStore(new RoomMembersStore(serverConnection));
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

function memberFromDirectory(member: DirectoryMember): RoomMember {
  return {
    id: member.id,
    login: member.login,
    displayName: member.displayName,
    deleted: member.deleted,
    avatarUrl: member.avatarUrl,
    customStatus: member.customStatus,
    presenceStatus: member.presenceStatus
  };
}
