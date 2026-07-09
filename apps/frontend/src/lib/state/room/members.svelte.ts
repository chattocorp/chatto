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
 * The store uses the paginated Connect member directory API internally, but room initialization
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
  livePresence = new SvelteMap<string, PresenceStatus>();
  presenceVersion = $state(0);

  private readonly api: MemberDirectoryAPI | null;
  private roomId = '';
  #loadId = 0;

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
    if (!this.roomId || !this.api) return;
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
    if (!this.roomId || !this.api) return;
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
    this.hasLoaded = false;
    this.isInitialLoading = false;
    this.searchInput = '';
    this.activeSearch = '';
    this.livePresence.clear();
    this.presenceVersion = 0;
  }
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
