import type {
  DirectoryMember,
  MemberDirectoryAPI,
  MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import { SvelteSet } from 'svelte/reactivity';

export const ROOM_MEMBER_MANAGEMENT_PAGE_SIZE = 20;
const USER_PICKER_PAGE_SIZE = 20;

export type RoomMemberManagementAPIs = {
  directory: MemberDirectoryAPI;
  commands: RoomCommandAPI;
};

type APIProvider = (serverId: string) => RoomMemberManagementAPIs;

function appendMembers(current: DirectoryMember[], incoming: DirectoryMember[]): DirectoryMember[] {
  const incomingIds = new Set(incoming.map((member) => member.id));
  return [...current.filter((member) => !incomingIds.has(member.id)), ...incoming];
}

/**
 * Owns the room-management page's paged membership reads and mutation lifecycle.
 *
 * Mutations never optimistically rewrite the list. The store re-reads the canonical
 * membership projection after each command. The server-scoped realtime projection
 * independently reconciles shared room mirrors.
 */
export class RoomMemberManagementStore {
  members = $state.raw<DirectoryMember[]>([]);
  totalCount = $state(0);
  hasMore = $state(false);
  loading = $state(false);
  loadingMore = $state(false);
  loadError = $state<string | null>(null);

  directoryResults = $state.raw<DirectoryMember[]>([]);
  directoryLoading = $state(false);
  directoryError = $state<string | null>(null);
  activeDirectorySearch = $state('');

  addingUserId = $state<string | null>(null);
  removingUserId = $state<string | null>(null);

  readonly #getAPIs: APIProvider;
  #serverId = '';
  #roomId = '';
  #roomGeneration = 0;
  #membersRequestId = 0;
  #directoryRequestId = 0;

  constructor(getAPIs: APIProvider) {
    this.#getAPIs = getAPIs;
  }

  setRoom(serverId: string, roomId: string): void {
    if (serverId === this.#serverId && roomId === this.#roomId) return;
    this.#serverId = serverId;
    this.#roomId = roomId;
    this.#fenceAndClear();
  }

  /**
   * Immediately purge state for a projected room removal before attempting any
   * authorised reload. Incrementing both request generations prevents older
   * responses from reopening the privacy boundary.
   */
  clearRoom(serverId: string, roomId: string): boolean {
    if (!this.#isCurrentRoom(serverId, roomId)) return false;
    this.#fenceAndClear();
    return true;
  }

  #fenceAndClear(): void {
    this.#roomGeneration++;
    this.#membersRequestId++;
    this.#directoryRequestId++;
    this.members = [];
    this.totalCount = 0;
    this.hasMore = false;
    this.loading = false;
    this.loadingMore = false;
    this.loadError = null;
    this.activeDirectorySearch = '';
    this.directoryResults = [];
    this.directoryLoading = false;
    this.directoryError = null;
    this.addingUserId = null;
    this.removingUserId = null;
  }

  async loadFirstPage(): Promise<void> {
    if (!this.#serverId || !this.#roomId || this.loading) return;
    const requestId = ++this.#membersRequestId;
    const serverId = this.#serverId;
    const roomId = this.#roomId;
    this.loading = true;
    this.loadingMore = false;
    this.loadError = null;

    try {
      const page = await this.#getAPIs(serverId).directory.listRoomMembers(
        roomId,
        '',
        ROOM_MEMBER_MANAGEMENT_PAGE_SIZE,
        0
      );
      if (!this.#isCurrentMembersRequest(requestId, serverId, roomId)) return;
      this.applyFirstPage(page);
    } catch (error) {
      if (!this.#isCurrentMembersRequest(requestId, serverId, roomId)) return;
      this.loadError = error instanceof Error ? error.message : String(error);
    } finally {
      if (this.#isCurrentMembersRequest(requestId, serverId, roomId)) this.loading = false;
    }
  }

  async refresh(): Promise<void> {
    if (!this.#serverId || !this.#roomId) return;
    this.#membersRequestId++;
    this.loading = false;
    await this.loadFirstPage();
  }

  async loadMore(): Promise<void> {
    if (!this.#serverId || !this.#roomId || this.loading || this.loadingMore || !this.hasMore) {
      return;
    }
    const requestId = this.#membersRequestId;
    const serverId = this.#serverId;
    const roomId = this.#roomId;
    const offset = this.members.length;
    this.loadingMore = true;
    this.loadError = null;

    try {
      const page = await this.#getAPIs(serverId).directory.listRoomMembers(
        roomId,
        '',
        ROOM_MEMBER_MANAGEMENT_PAGE_SIZE,
        offset
      );
      if (!this.#isCurrentMembersRequest(requestId, serverId, roomId)) return;
      this.members = appendMembers(this.members, page.members);
      this.totalCount = page.totalCount;
      this.hasMore = page.hasMore && page.members.length > 0;
    } catch (error) {
      if (!this.#isCurrentMembersRequest(requestId, serverId, roomId)) return;
      this.loadError = error instanceof Error ? error.message : String(error);
    } finally {
      if (this.#isCurrentMembersRequest(requestId, serverId, roomId)) this.loadingMore = false;
    }
  }

  async searchDirectory(search: string): Promise<void> {
    const query = search.trim();
    const requestId = ++this.#directoryRequestId;
    const serverId = this.#serverId;
    const roomId = this.#roomId;
    this.activeDirectorySearch = query;
    this.directoryResults = [];
    this.directoryError = null;

    if (!query || !serverId || !roomId) {
      this.directoryLoading = false;
      return;
    }

    this.directoryLoading = true;
    try {
      const api = this.#getAPIs(serverId).directory;
      const eligible: DirectoryMember[] = [];
      const seenIds = new SvelteSet<string>();
      let offset = 0;
      let hasMore = true;

      while (hasMore && eligible.length < USER_PICKER_PAGE_SIZE) {
        const page = await api.listUsers(query, USER_PICKER_PAGE_SIZE, offset);
        if (!this.#isCurrentDirectoryRequest(requestId, serverId, roomId, query)) return;

        const candidates = page.members.filter(
          (member) => !member.deleted && !seenIds.has(member.id)
        );
        for (const candidate of candidates) seenIds.add(candidate.id);

        const existing = candidates.length
          ? await api.batchGetRoomMembers(
              roomId,
              candidates.map((member) => member.id)
            )
          : [];
        if (!this.#isCurrentDirectoryRequest(requestId, serverId, roomId, query)) return;

        const existingIds = new SvelteSet(existing.map((member) => member.id));
        eligible.push(...candidates.filter((member) => !existingIds.has(member.id)));

        hasMore = page.hasMore && page.members.length > 0;
        offset += page.members.length;
      }

      this.directoryResults = eligible.slice(0, USER_PICKER_PAGE_SIZE);
    } catch (error) {
      if (!this.#isCurrentDirectoryRequest(requestId, serverId, roomId, query)) return;
      this.directoryError = error instanceof Error ? error.message : String(error);
    } finally {
      if (this.#isCurrentDirectoryRequest(requestId, serverId, roomId, query)) {
        this.directoryLoading = false;
      }
    }
  }

  clearDirectorySearch(): void {
    this.#directoryRequestId++;
    this.activeDirectorySearch = '';
    this.directoryResults = [];
    this.directoryLoading = false;
    this.directoryError = null;
  }

  async addMember(user: DirectoryMember): Promise<boolean> {
    if (!this.#serverId || !this.#roomId || this.addingUserId || this.removingUserId) return false;
    const serverId = this.#serverId;
    const roomId = this.#roomId;
    const roomGeneration = this.#roomGeneration;
    const api = this.#getAPIs(serverId);
    this.addingUserId = user.id;
    try {
      await api.commands.addMember({ roomId, userId: user.id });
      if (!this.#isCurrentRoom(serverId, roomId, roomGeneration)) return false;
      this.directoryResults = this.directoryResults.filter((candidate) => candidate.id !== user.id);
      await this.refresh();
      return this.#isCurrentRoom(serverId, roomId, roomGeneration);
    } finally {
      if (this.#isCurrentRoom(serverId, roomId, roomGeneration)) this.addingUserId = null;
    }
  }

  async removeMember(user: DirectoryMember): Promise<boolean> {
    if (!this.#serverId || !this.#roomId || this.addingUserId || this.removingUserId) return false;
    const serverId = this.#serverId;
    const roomId = this.#roomId;
    const roomGeneration = this.#roomGeneration;
    const api = this.#getAPIs(serverId);
    this.removingUserId = user.id;
    try {
      await api.commands.removeMember({ roomId, userId: user.id });
      if (!this.#isCurrentRoom(serverId, roomId, roomGeneration)) return false;
      await this.refresh();
      return this.#isCurrentRoom(serverId, roomId, roomGeneration);
    } finally {
      if (this.#isCurrentRoom(serverId, roomId, roomGeneration)) this.removingUserId = null;
    }
  }

  private applyFirstPage(page: MemberDirectoryPage): void {
    this.members = page.members;
    this.totalCount = page.totalCount;
    this.hasMore = page.hasMore;
  }

  #isCurrentRoom(serverId: string, roomId: string, roomGeneration?: number): boolean {
    return (
      serverId === this.#serverId &&
      roomId === this.#roomId &&
      (roomGeneration === undefined || roomGeneration === this.#roomGeneration)
    );
  }

  #isCurrentMembersRequest(requestId: number, serverId: string, roomId: string): boolean {
    return requestId === this.#membersRequestId && this.#isCurrentRoom(serverId, roomId);
  }

  #isCurrentDirectoryRequest(
    requestId: number,
    serverId: string,
    roomId: string,
    search: string
  ): boolean {
    return (
      requestId === this.#directoryRequestId &&
      this.#isCurrentRoom(serverId, roomId) &&
      search === this.activeDirectorySearch
    );
  }
}
