import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { RoomKind } from '$lib/api-client/roomDirectory';
import type { MemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import type { UserAvatarUserView } from '$lib/render/users';
import {
  avatarUserFromDirectoryMember,
  type RoomsListGroup,
  type RoomsListItem
} from './rooms.svelte';

export type DirectoryRoom = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  isUniversal: boolean;
  viewerCanJoinRoom: boolean;
};

export type DirectoryRoomJoinPreview = {
  memberCount: number;
  sampleMembers: UserAvatarUserView[];
};

export type JoinResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type LeaveResult = { ok: true; room?: DirectoryRoom } | { ok: false; error: Error };
export type JoinGroupResult = { ok: true; joinedRoomIds: string[] } | { ok: false; error: Error };

export type RoomDirectoryNavigation = {
  rooms: RoomsListItem[];
  roomGroups: RoomsListGroup[];
  isInitialLoading: boolean;
  isRoomMember(roomId: string): boolean;
};

/**
 * Command state for room membership changes.
 *
 * Directory rows and authoritative membership come directly from the
 * projection-backed navigation view. This store owns only in-flight and
 * just-completed optimistic state plus the explicit join-preview query.
 */
export class RoomDirectoryStore {
  joiningIds = new SvelteSet<string>();
  leavingIds = new SvelteSet<string>();
  justJoinedIds = new SvelteSet<string>();
  justLeftIds = new SvelteSet<string>();
  joiningGroupIds = new SvelteSet<string>();

  #generation = 0;
  #commandToken = 0;
  #membershipRevisions = new SvelteMap<string, number>();
  #joiningTokens = new SvelteMap<string, number>();
  #leavingTokens = new SvelteMap<string, number>();
  #joiningGroupTokens = new SvelteMap<string, number>();

  constructor(
    private readonly navigation: RoomDirectoryNavigation,
    private readonly memberDirectoryAPI: Pick<MemberDirectoryAPI, 'listRoomMembers'>,
    private readonly roomAPI: Pick<RoomCommandAPI, 'joinRoom' | 'leaveRoom' | 'joinGroup'>
  ) {}

  get allRooms(): DirectoryRoom[] {
    return this.navigation.rooms
      .filter((room) => room.type === RoomKind.CHANNEL)
      .map((room) => ({
        id: room.id,
        name: room.name,
        description: room.description,
        archived: false,
        isUniversal: room.isUniversal,
        viewerCanJoinRoom: room.viewerCanJoinRoom
      }));
  }

  get isLoading(): boolean {
    return this.navigation.isInitialLoading;
  }

  get roomGroups() {
    return this.navigation.roomGroups;
  }

  async loadJoinPreview(roomId: string): Promise<DirectoryRoomJoinPreview | null> {
    try {
      const page = await this.memberDirectoryAPI.listRoomMembers(roomId, '', 5, 0);
      return {
        memberCount: page.totalCount,
        sampleMembers: page.members.map(avatarUserFromDirectoryMember)
      };
    } catch {
      return null;
    }
  }

  isJoined(roomId: string): boolean {
    if (this.justLeftIds.has(roomId)) return false;
    if (this.justJoinedIds.has(roomId)) return true;
    return this.navigation.isRoomMember(roomId);
  }

  async joinRoom(roomId: string): Promise<JoinResult> {
    const generation = this.#generation;
    const membershipRevision = this.membershipRevision(roomId);
    const commandToken = ++this.#commandToken;
    this.#joiningTokens.set(roomId, commandToken);
    this.joiningIds.add(roomId);
    try {
      await this.roomAPI.joinRoom(roomId);
      if (
        generation === this.#generation &&
        membershipRevision === this.membershipRevision(roomId)
      ) {
        this.justJoinedIds.add(roomId);
        this.justLeftIds.delete(roomId);
      }
      return { ok: true, room: this.allRooms.find((room) => room.id === roomId) };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      if (this.#joiningTokens.get(roomId) === commandToken) {
        this.#joiningTokens.delete(roomId);
        this.joiningIds.delete(roomId);
      }
    }
  }

  async joinGroup(groupId: string): Promise<JoinGroupResult> {
    const generation = this.#generation;
    const membershipRevisions = new SvelteMap(
      this.navigation.rooms.map((room) => [room.id, this.membershipRevision(room.id)])
    );
    const commandToken = ++this.#commandToken;
    this.#joiningGroupTokens.set(groupId, commandToken);
    this.joiningGroupIds.add(groupId);
    try {
      const joined = await this.roomAPI.joinGroup(groupId);
      if (generation === this.#generation) {
        for (const id of joined) {
          if ((membershipRevisions.get(id) ?? 0) === this.membershipRevision(id)) {
            this.justJoinedIds.add(id);
            this.justLeftIds.delete(id);
          }
        }
      }
      return { ok: true, joinedRoomIds: joined };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      if (this.#joiningGroupTokens.get(groupId) === commandToken) {
        this.#joiningGroupTokens.delete(groupId);
        this.joiningGroupIds.delete(groupId);
      }
    }
  }

  async leaveRoom(roomId: string): Promise<LeaveResult> {
    const generation = this.#generation;
    const membershipRevision = this.membershipRevision(roomId);
    const commandToken = ++this.#commandToken;
    this.#leavingTokens.set(roomId, commandToken);
    this.leavingIds.add(roomId);
    try {
      await this.roomAPI.leaveRoom(roomId);
      if (
        generation === this.#generation &&
        membershipRevision === this.membershipRevision(roomId)
      ) {
        this.justLeftIds.add(roomId);
        this.justJoinedIds.delete(roomId);
      }
      return { ok: true, room: this.allRooms.find((room) => room.id === roomId) };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      if (this.#leavingTokens.get(roomId) === commandToken) {
        this.#leavingTokens.delete(roomId);
        this.leavingIds.delete(roomId);
      }
    }
  }

  /** Clear only the local overlay confirmed by this projected membership. */
  acknowledgeMembership(roomId: string, isMember: boolean | undefined): void {
    this.#membershipRevisions.set(roomId, this.membershipRevision(roomId) + 1);
    if (isMember === true) this.justJoinedIds.delete(roomId);
    if (isMember === false) this.justLeftIds.delete(roomId);
  }

  /** A removed room cannot retain either optimistic membership answer. */
  removeMembershipProjection(roomId: string): void {
    this.#membershipRevisions.set(roomId, this.membershipRevision(roomId) + 1);
    this.justJoinedIds.delete(roomId);
    this.justLeftIds.delete(roomId);
  }

  /** Fence late command responses and clear all optimistic state. */
  resetOptimisticState(): void {
    this.#generation++;
    this.#membershipRevisions.clear();
    this.#joiningTokens.clear();
    this.#leavingTokens.clear();
    this.#joiningGroupTokens.clear();
    this.joiningIds.clear();
    this.leavingIds.clear();
    this.justJoinedIds.clear();
    this.justLeftIds.clear();
    this.joiningGroupIds.clear();
  }

  private membershipRevision(roomId: string): number {
    return this.#membershipRevisions.get(roomId) ?? 0;
  }
}
