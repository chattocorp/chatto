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

type MemberSearchCacheEntry = {
  ids: string[];
  complete: boolean;
};

type StandaloneProfile = { member: RoomMember; fromRealtime: boolean };

function memberMatchesSearch(member: RoomMember, search: string): boolean {
  const query = search.trim().toLowerCase();
  if (!query) return true;
  return (
    member.login.toLowerCase().includes(query) || member.displayName.toLowerCase().includes(query)
  );
}

function pageIds(page: MemberDirectoryPage): string[] {
  return page.memberIds ?? page.members.map((member) => member.id);
}

/**
 * Room member store for the current room.
 *
 * The store publishes the first paginated Connect response immediately, then fills `members` with
 * the remaining pages in the background. `hasFirstPage` marks interactive readiness while
 * `hasLoadedAll` marks complete membership IDs; profile resolution can finish later.
 * Searches use a separate cache until their matching directory page enters canonical order.
 */
export class RoomMembersStore {
  #memberIds = $state.raw<string[]>([]);
  readonly #standaloneProfiles = new SvelteMap<string, StandaloneProfile>();
  readonly #users?: UserStore;
  readonly #resolvedMembers = $derived(this.resolveIds(this.#memberIds));
  /** Membership retains IDs. Connected rooms read current profiles from the shared owner. */
  get members(): RoomMember[] {
    return this.#resolvedMembers;
  }
  /** Accept projection or standalone fixture rows while retaining only membership IDs. */
  set members(members: RoomMember[]) {
    this.#memberIds = members.map((member) => member.id);
    if (!this.#users) {
      this.#standaloneProfiles.clear();
      for (const member of members) {
        this.#standaloneProfiles.set(member.id, { member, fromRealtime: false });
      }
    }
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
      if (searched) return this.resolveIds(searched.ids);
    }
    return this.filterLoadedMembers(this.activeSearch);
  }

  /** Resolve current profiles without adding empty rows for pending identities. */
  private resolveIds(ids: string[]): RoomMember[] {
    return ids.flatMap((id) => this.resolveProfile(id) ?? []);
  }

  private resolveProfile(id: string): RoomMember | undefined {
    if (!this.#users) return this.#standaloneProfiles.get(id)?.member;
    const member = this.#users.get(id);
    if (member) return memberFromDirectory(mapDirectoryMember(member));
    if (this.#users.isDeleted(id)) {
      return {
        id,
        login: '',
        displayName: '',
        deleted: true,
        avatarUrl: null,
        presenceStatus: PresenceStatus.OFFLINE
      };
    }
    return undefined;
  }

  /** Standalone fixtures own page profiles locally; realtime updates win over stale pages. */
  private recordPageProfiles(profiles: DirectoryMember[]): void {
    if (this.#users) return;
    for (const profile of profiles) {
      if (this.#standaloneProfiles.get(profile.id)?.fromRealtime) continue;
      this.#standaloneProfiles.set(profile.id, {
        member: memberFromDirectory(profile),
        fromRealtime: false
      });
    }
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

  /** Replace membership IDs from the canonical server projection. Profiles may arrive later. */
  replaceProjection(roomId: string, memberIds: readonly string[]): void {
    if (this.roomId !== roomId) {
      this.roomId = roomId;
      this.reset();
    }
    this.#loadId++;
    this.#memberIds = [...memberIds];
    this.totalCount = memberIds.length;
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
          this.#memberIds = [];
          this.#standaloneProfiles.clear();
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
    if (cached && (cached.complete || cached.ids.length >= limit)) {
      return this.resolveIds(cached.ids).slice(0, limit);
    }
    let page: MemberDirectoryPage;
    try {
      page = await this.fetchPage(0, limit, normalizedSearch);
    } catch (error) {
      console.error('Failed to search room members:', error);
      return this.filteredLoadedMembers(normalizedSearch, limit);
    }
    if (roomId !== this.roomId || loadId !== this.#loadId) return [];
    this.recordPageProfiles(page.members);
    const ids = pageIds(page);
    this.#searchCache.set(normalizedSearch.toLowerCase(), {
      ids,
      complete: !page.hasMore
    });
    return this.resolveIds(ids).slice(0, limit);
  }

  private async searchAllMembers(search: string): Promise<void> {
    const query = search.trim().toLowerCase();
    const loadId = this.#loadId;
    let ids: string[] = [];
    let offset = 0;
    let hasMore = true;

    while (hasMore) {
      let page: MemberDirectoryPage;
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

      this.recordPageProfiles(page.members);
      const pageMemberIds = pageIds(page);
      ids = appendPageIds(ids, pageMemberIds);
      const consumed = page.consumedCount ?? pageMemberIds.length;
      hasMore = page.hasMore && consumed > 0;
      offset += consumed;
      this.#searchCache.set(query, { ids, complete: !hasMore });
    }
  }

  setPresence(userId: string, status: PresenceStatus): void {
    this.livePresence.set(userId, status);
    this.presenceVersion++;
    this.#presenceChanges.set(userId, this.presenceVersion);
  }

  /** Standalone fixtures receive profile changes here; connected rooms use UserStore. */
  updateUsers(users: DirectoryMember[]): void {
    this.#searchCache.clear();
    if (this.#users) return;
    for (const user of users) {
      this.#standaloneProfiles.set(user.id, {
        member: memberFromDirectory(user),
        fromRealtime: true
      });
    }
    if (this.hasLoadedAll) {
      for (const user of users) {
        if (this.#membershipChanges.get(user.id) && !this.#memberIds.includes(user.id)) {
          this.#memberIds = [...this.#memberIds, user.id];
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
    if (!joined && !this.#users) this.#standaloneProfiles.delete(userId);
    this.#searchCache.clear();
    if (this.isInitialLoading || this.isBackgroundLoading) {
      await this.refresh();
      return;
    }
    const exists = this.#memberIds.includes(userId);
    if (!joined) {
      this.#memberIds = this.#memberIds.filter((id) => id !== userId);
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
    if (user && !this.#memberIds.includes(userId)) {
      this.recordPageProfiles([user]);
      this.#memberIds = [...this.#memberIds, userId];
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
        this.recordPageProfiles(page.members);
        const ids = pageIds(page);
        // The filter gives fresh presence even when the profile came from cache.
        // A realtime change received during this request takes precedence.
        for (const id of ids) {
          this.#previewIds.add(id);
          if ((this.#presenceChanges.get(id) ?? 0) <= presenceVersion) {
            this.livePresence.set(id, status);
          }
        }
        this.#memberIds = appendPageIds(this.#memberIds, ids);
        if (ids.length > 0) {
          this.hasFirstPage = true;
          this.isInitialLoading = false;
          this.isBackgroundLoading = true;
          this.totalCount = Math.max(this.totalCount, this.#memberIds.length);
        }
        const consumed = page.consumedCount ?? ids.length;
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
    let fullIds: string[] = [];

    while (hasMore) {
      const page = await this.fetchPage(nextOffset, ROOM_MEMBERS_PAGE_SIZE, '');
      if (loadId !== this.#loadId) return;

      this.recordPageProfiles(page.members);
      const ids = pageIds(page);
      fullIds = appendPageIds(fullIds, ids);
      this.#memberIds = appendPageIds(
        firstPage ? this.#memberIds.filter((id) => this.#previewIds.has(id)) : this.#memberIds,
        ids
      );
      this.totalCount = page.totalCount;
      hasMore = page.hasMore;
      const consumed = page.consumedCount ?? ids.length;
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
      this.#memberIds = fullIds;
      this.hasLoadedAll = true;
      this.isBackgroundLoading = false;
    }
  }

  private async fetchPage(offset: number, limit: number, search: string): Promise<MemberDirectoryPage> {
    if (!this.api) return { members: [], totalCount: 0, hasMore: false };
    const normalizedSearch = search.trim();
    return this.#minimumCursor
      ? this.api.listRoomMembers(this.roomId, normalizedSearch, limit, offset, {
          minimumCursor: this.#minimumCursor
        })
      : this.api.listRoomMembers(this.roomId, normalizedSearch, limit, offset);
  }

  private filterLoadedMembers(search: string): RoomMember[] {
    return this.members.filter((member) => memberMatchesSearch(member, search));
  }

  private filteredLoadedMembers(search: string, limit: number): RoomMember[] {
    return this.filterLoadedMembers(search).slice(0, limit);
  }

  private reset(): void {
    this.#loadId++;
    this.#memberIds = [];
    this.#standaloneProfiles.clear();
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
    this.#minimumCursor = undefined;
    this.livePresence.clear();
    this.#presenceChanges.clear();
    this.#previewIds.clear();
    this.#fullScanFinished = true;
    this.presenceVersion = 0;
  }
}

function appendPageIds(current: string[], incoming: string[]): string[] {
  if (incoming.length === 0) return current;
  const incomingIds = new Set(incoming);
  return [...current.filter((id) => !incomingIds.has(id)), ...incoming];
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
