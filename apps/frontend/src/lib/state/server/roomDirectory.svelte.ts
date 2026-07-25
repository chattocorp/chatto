import { SvelteSet } from 'svelte/reactivity';
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
    return this.navigation.rooms.some((room) => room.id === roomId && room.viewerIsMember);
  }

  async joinRoom(roomId: string): Promise<JoinResult> {
    const generation = this.#generation;
    this.joiningIds.add(roomId);
    try {
      await this.roomAPI.joinRoom(roomId);
      if (generation === this.#generation) {
        this.justJoinedIds.add(roomId);
        this.justLeftIds.delete(roomId);
      }
      return { ok: true, room: this.allRooms.find((room) => room.id === roomId) };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      this.joiningIds.delete(roomId);
    }
  }

  async joinGroup(groupId: string): Promise<JoinGroupResult> {
    const generation = this.#generation;
    this.joiningGroupIds.add(groupId);
    try {
      const joined = await this.roomAPI.joinGroup(groupId);
      if (generation === this.#generation) {
        for (const id of joined) {
          this.justJoinedIds.add(id);
          this.justLeftIds.delete(id);
        }
      }
      return { ok: true, joinedRoomIds: joined };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      this.joiningGroupIds.delete(groupId);
    }
  }

  async leaveRoom(roomId: string): Promise<LeaveResult> {
    const generation = this.#generation;
    this.leavingIds.add(roomId);
    try {
      await this.roomAPI.leaveRoom(roomId);
      if (generation === this.#generation) {
        this.justLeftIds.add(roomId);
        this.justJoinedIds.delete(roomId);
      }
      return { ok: true, room: this.allRooms.find((room) => room.id === roomId) };
    } catch (error) {
      return { ok: false, error: error instanceof Error ? error : new Error(String(error)) };
    } finally {
      this.leavingIds.delete(roomId);
    }
  }

  /** The authoritative membership projection supersedes a local overlay. */
  acknowledgeMembership(roomId: string): void {
    this.justJoinedIds.delete(roomId);
    this.justLeftIds.delete(roomId);
  }

  /** Fence late command responses and clear all optimistic state. */
  resetOptimisticState(): void {
    this.#generation++;
    this.joiningIds.clear();
    this.leavingIds.clear();
    this.justJoinedIds.clear();
    this.justLeftIds.clear();
    this.joiningGroupIds.clear();
  }
}
