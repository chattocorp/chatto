import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { createContext } from 'svelte';
import { Code, isConnectCode } from '$lib/api-client/connect';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';

import {
  createMemberDirectoryAPI,
  type DirectoryMember,
  type MemberDirectoryAPI,
  type MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';
import { getUserStore, type UserStore } from '$lib/state/server/users.svelte';
import { mapDirectoryMember } from '$lib/api-client/memberDirectory';

export const ROOM_MEMBERS_PAGE_SIZE = 250;
const MENTION_MEMBER_SEARCH_LIMIT = 10;

/**
 * Room member data for the current room.
 */
export type RoomMember = {
  id: string;
  login: string;
  displayName: string;
  deleted?: boolean;
  isBot?: boolean;
  avatarUrl?: string | null;
  customStatus?: CustomUserStatus | null;
  presenceStatus: PresenceStatus;
};

export type RoomMembersPage = {
  members: RoomMember[];
  totalCount: number;
  hasMore: boolean;
  consumedCount?: number;
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
    hasMore: page.hasMore,
    consumedCount: page.consumedCount
  };
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
  #memberIds = $state.raw<string[]>([]);
  #standaloneMembers = $state.raw<RoomMember[]>([]);
  readonly #users?: UserStore;
  readonly #resolvedMembers = $derived.by(() => this.#users
    ? this.#memberIds.flatMap((id) => this.resolveProfile(id) ?? [])
    : this.#standaloneMembers);
  /** Membership retains IDs; current public profiles come from the connection owner.
   * Standalone API fixtures can supply render rows without a connection. */
  get members(): RoomMember[] {
    return this.#resolvedMembers;
  }
  set members(members: RoomMember[]) {
    this.#memberIds = members.map((member) => member.id);
    if (!this.#users) this.#standaloneMembers = members;
  }
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
  #membershipChanges = new SvelteMap<string, boolean>();
  #profileUpdates = new SvelteMap<string, RoomMember>();
  #minimumCursor: string | undefined;
  #presenceChanges = new SvelteMap<string, number>();
  #previewIds = new SvelteSet<string>();
  #fullScanFinished = false;

  constructor(source?: ServerConnection | MemberDirectoryAPI | null) {
    if (!source) {
      this.api = null;
    } else if ('listRoomMembers' in source) {
      this.api = source;
    } else {
      this.api = source.getAPI(createMemberDirectoryAPI);
      if (source.serverId) this.#users = getUserStore(source.serverId, source.queryScope);
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
      if (searched) return this.resolveProfiles(searched.members);
    }
    return this.filterLoadedMembers(this.activeSearch);
  }

  /** Cached searches retain membership results, but never override live identities. */
  private resolveProfiles(members: RoomMember[]): RoomMember[] {
    if (!this.#users) return members;
    return members.flatMap((member) => this.resolveProfile(member.id) ?? []);
  }

  private resolveProfile(id: string): RoomMember | undefined {
    const member = this.#users?.get(id);
    // A profile invalidation must not erase membership when another page or
    // profile update arrives before the replacement profile.
    return member ? memberFromDirectory(mapDirectoryMember(member)) : {
      id, login: '', displayName: '', deleted: this.#users?.isDeleted(id), avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE
    };
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

  /** Keep membership explicitly pending until the projection materializes it. */
  awaitProjection(roomId: string): void {
    if (this.roomId === roomId && this.isInitialLoading && !this.hasFirstPage) return;
    if (this.roomId !== roomId) this.roomId = roomId;
    this.reset();
    this.isInitialLoading = true;
  }

  /** Replace membership from the canonical server projection. */
  replaceProjection(roomId: string, members: RoomMember[]): void {
    if (this.roomId !== roomId) {
      this.roomId = roomId;
      this.reset();
    }
    this.#loadId++;
    this.members = members;
    this.totalCount = members.length;
    this.hasFirstPage = true;
    this.hasLoadedAll = true;
    this.isInitialLoading = false;
    this.isBackgroundLoading = false;
    this.loadError = null;
    this.#searchCache.clear();
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
    this.#fullScanFinished = false;
    this.#previewIds.clear();
    this.isBackgroundLoading = false;
    this.loadError = null;
    this.loadOnlinePreview(loadId);
    try {
      await this.loadPages(loadId);
    } catch (error) {
      if (loadId === this.#loadId) {
        this.loadError = error instanceof Error ? error.message : 'Failed to load room members';
        console.error('Failed to load room members:', error);
      }
    } finally {
      if (loadId === this.#loadId) {
        this.#fullScanFinished = true;
        this.isInitialLoading = false;
        this.isBackgroundLoading = false;
      }
    }
  }

  /** Recheck membership after a projection change, at or beyond its cursor. */
  async refresh({ reauthorize = false, minimumCursor }: {
    reauthorize?: boolean;
    minimumCursor?: string;
  } = {}): Promise<void> {
    if (!this.roomId || !this.api) return;
    this.#minimumCursor = minimumCursor ?? this.#minimumCursor;
    const loadId = ++this.#loadId;
    this.isInitialLoading = !this.hasFirstPage;
    this.#fullScanFinished = false;
    this.#previewIds.clear();
    this.isBackgroundLoading = this.hasFirstPage;
    this.hasLoadedAll = false;
    this.loadError = null;
    this.#searchCache.clear();
    this.loadOnlinePreview(loadId);
    try {
      await this.loadPages(loadId);
    } catch (error) {
      if (loadId === this.#loadId) {
        this.loadError = error instanceof Error ? error.message : 'Failed to refresh room members';
        if (reauthorize || isConnectCode(error, Code.PermissionDenied) || isConnectCode(error, Code.NotFound)) {
          this.members = [];
          this.totalCount = 0;
          this.#searchCache.clear();
        }
        console.error('Failed to refresh room members:', error);
      }
    } finally {
      if (loadId === this.#loadId) {
        this.#fullScanFinished = true;
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
      return this.resolveProfiles(cached.members).slice(0, limit);
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
    return this.resolveProfiles(page.members).slice(0, limit);
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
      const consumed = page.consumedCount ?? page.members.length;
      hasMore = page.hasMore && consumed > 0;
      offset += consumed;
      this.#searchCache.set(query, { members, complete: !hasMore });
    }
  }

  setPresence(userId: string, status: PresenceStatus): void {
    this.livePresence.set(userId, status);
    this.presenceVersion++;
    this.#presenceChanges.set(userId, this.presenceVersion);
  }

  /** Standalone stores keep profile rows; connected stores read the shared owner. */
  updateUsers(users: DirectoryMember[]): void {
    this.#searchCache.clear();
    if (this.#users) return;
    const updates = new SvelteMap(users.map((user) => [user.id, memberFromDirectory(user)]));
    for (const [id, user] of updates) this.#profileUpdates.set(id, user);
    this.members = this.members.map((member) => updates.get(member.id) ?? member);
    if (this.hasLoadedAll) {
      for (const [id, user] of updates) {
        if (this.#membershipChanges.get(id) && !this.members.some((member) => member.id === id)) {
          this.members = [...this.members, user];
          this.totalCount++;
        }
      }
    }
  }

  /** Apply membership deltas even while this room is not mounted. A delta
   * during offset pagination restarts that read to avoid skipped rows. */
  async applyMembership(userId: string, joined: boolean, minimumCursor?: string): Promise<void> {
    if (!userId || !this.api) return;
    this.#minimumCursor = minimumCursor ?? this.#minimumCursor;
    this.#membershipChanges.set(userId, joined);
    if (!joined) this.#profileUpdates.delete(userId);
    this.#searchCache.clear();
    if (this.isInitialLoading || this.isBackgroundLoading) {
      await this.refresh();
      return;
    }
    const exists = this.#memberIds.includes(userId);
    if (!joined) {
      this.#memberIds = this.#memberIds.filter((id) => id !== userId);
      if (!this.#users) this.#standaloneMembers = this.#standaloneMembers.filter((member) => member.id !== userId);
      if (exists) this.totalCount = Math.max(0, this.totalCount - 1);
      return;
    }
    if (exists || !this.hasFirstPage) return;
    if (this.#users) {
      // Realtime owns profile hydration. Membership can publish its ID now.
      this.#memberIds = [...this.#memberIds, userId];
      this.totalCount++;
      return;
    }
    const loadId = this.#loadId;
    let users: DirectoryMember[];
    try {
      users = await this.api.batchGetUsers([userId]);
    } catch {
      // An old request must not invalidate a newer snapshot after a reset.
      if (loadId === this.#loadId) this.reset();
      return;
    }
    if (loadId !== this.#loadId || !this.#membershipChanges.get(userId)) return;
    const user = users[0];
    if (user && !this.members.some((member) => member.id === userId)) {
      this.members = [
        ...this.members,
        this.#profileUpdates.get(userId) ?? memberFromDirectory(user)
      ];
      this.totalCount++;
    }
  }

  /** Discard snapshots after a recovery gap or an authorization boundary. */
  resetProjectionState(): void {
    this.reset();
  }

  /** Publish connected members without waiting for unrelated profile batches.
   * This is a best-effort preview; the full scan owns counts and completion. */
  private loadOnlinePreview(loadId: number): void {
    for (const status of [
      PresenceStatus.ONLINE,
      PresenceStatus.AWAY,
      PresenceStatus.DO_NOT_DISTURB
    ]) {
      void this.loadOnlinePages(loadId, status);
    }
  }

  private async loadOnlinePages(loadId: number, status: PresenceStatus): Promise<void> {
    if (!this.api) return;
    let offset = 0;
    try {
      while (loadId === this.#loadId && !this.hasLoadedAll && !this.#fullScanFinished) {
        const presenceVersion = this.presenceVersion;
        const page = await this.api.listOnlineRoomMembers(
          this.roomId,
          status,
          ROOM_MEMBERS_PAGE_SIZE,
          offset,
          this.#minimumCursor ? { minimumCursor: this.#minimumCursor } : {}
        );
        if (loadId !== this.#loadId || this.hasLoadedAll || this.#fullScanFinished) return;
        const members = page.members.map(
          (member) => this.#profileUpdates.get(member.id) ?? memberFromDirectory(member)
        );
        // The filter gives fresh presence even when the profile came from cache.
        // A realtime change received during this request takes precedence.
        for (const member of members) {
          this.#previewIds.add(member.id);
          if ((this.#presenceChanges.get(member.id) ?? 0) <= presenceVersion) {
            this.livePresence.set(member.id, status);
          }
        }
        this.members = appendPageMembers(this.members, members);
        if (members.length > 0) {
          this.hasFirstPage = true;
          this.isInitialLoading = false;
          this.isBackgroundLoading = true;
          this.totalCount = Math.max(this.totalCount, this.members.length);
        }
        const consumed = page.consumedCount ?? page.members.length;
        if (!page.hasMore || consumed === 0) return;
        offset += consumed;
      }
    } catch {
      // The full scan and mention search remain available if the preview fails.
    }
  }

  private async loadPages(loadId: number): Promise<void> {
    let nextOffset = 0;
    let hasMore = true;
    let firstPage = true;
    let fullMembers: RoomMember[] = [];

    while (hasMore) {
      const page = await this.fetchPage(nextOffset, ROOM_MEMBERS_PAGE_SIZE, '');
      if (loadId !== this.#loadId) return;

      const members = page.members.map((member) => this.#profileUpdates.get(member.id) ?? member);
      fullMembers = appendPageMembers(fullMembers, members);
      this.members = appendPageMembers(
        firstPage ? this.members.filter((member) => this.#previewIds.has(member.id)) : this.members,
        members
      );
      this.totalCount = page.totalCount;
      hasMore = page.hasMore;
      const consumed = page.consumedCount ?? page.members.length;
      nextOffset += consumed;

      if (firstPage) {
        firstPage = false;
        this.hasFirstPage = true;
        this.hasLoadedAll = !hasMore;
        this.isInitialLoading = false;
        this.isBackgroundLoading = hasMore;
      }

      if (consumed === 0) {
        hasMore = false;
        break;
      }
    }

    if (loadId === this.#loadId) {
      this.members = fullMembers.map((member) => this.#profileUpdates.get(member.id) ?? member);
      this.hasLoadedAll = true;
      this.isBackgroundLoading = false;
    }
  }

  private async fetchPage(offset: number, limit: number, search: string): Promise<RoomMembersPage> {
    if (!this.api) return { members: [], totalCount: 0, hasMore: false };
    const normalizedSearch = search.trim();
    return mapPage(
      await (this.#minimumCursor
        ? this.api.listRoomMembers(this.roomId, normalizedSearch, limit, offset, {
            minimumCursor: this.#minimumCursor
          })
        : this.api.listRoomMembers(this.roomId, normalizedSearch, limit, offset))
    );
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
    this.#membershipChanges.clear();
    this.#profileUpdates.clear();
    this.#minimumCursor = undefined;
    this.livePresence.clear();
    this.#presenceChanges.clear();
    this.#previewIds.clear();
    this.#fullScanFinished = true;
    this.presenceVersion = 0;
  }
}

function appendPageMembers(current: RoomMember[], incoming: RoomMember[]): RoomMember[] {
  if (incoming.length === 0) return current;
  const incomingIds = new Set(incoming.map((member) => member.id));
  return [...current.filter((member) => !incomingIds.has(member.id)), ...incoming];
}

const [getMembersStoreContext, setMembersStoreContext] = createContext<() => RoomMembersStore>();

export function setRoomMembersStore<T extends RoomMembersStore | (() => RoomMembersStore)>(
  store: T
): T {
  setMembersStoreContext(typeof store === 'function' ? store : () => store);
  return store;
}

export function createRoomMembers(serverConnection?: ServerConnection): RoomMembersStore {
  return setRoomMembersStore(new RoomMembersStore(serverConnection));
}

export function getRoomMembersStore(): RoomMembersStore {
  return getMembersStoreContext()();
}

/** Capture context during initialization, then resolve the selected room later. */
export function useRoomMembersStore(): () => RoomMembersStore {
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
    isBot: member.isBot,
    avatarUrl: member.avatarUrl,
    customStatus: member.customStatus,
    presenceStatus: member.presenceStatus
  };
}
