import { ReactiveMap, signal } from '../reactivity/index.js';
import { UserStore } from './users.js';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import type { RoomGroup, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { ServerRuntimeConfig } from '@chatto/api-types/api/v1/server_state_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import type { RealtimeProjectionUpdate } from '../realtime/eventBus.js';

/** Authenticated server resources assembled from two canonical responses. */
export type ProjectedServerState = {
  motd?: string;
  runtime?: ServerRuntimeConfig;
};

/** Canonical protobuf-native resources for one connected Chatto server. */
export class ServerProjectionStore {
  readonly #serverSignal = signal<ServerPublicProfile | null>(null);
  get server(): ServerPublicProfile | null {
    return this.#serverSignal.get();
  }
  set server(value: ServerPublicProfile | null) {
    this.#serverSignal.set(value);
  }
  readonly #serverStateSignal = signal<ProjectedServerState | null>(null);
  get serverState(): ProjectedServerState | null {
    return this.#serverStateSignal.get();
  }
  set serverState(value: ProjectedServerState | null) {
    this.#serverStateSignal.set(value);
  }
  readonly #viewerSignal = signal<GetViewerResponse | null>(null);
  get viewer(): GetViewerResponse | null {
    return this.#viewerSignal.get();
  }
  set viewer(value: GetViewerResponse | null) {
    this.#viewerSignal.set(value);
  }
  /** Shared profile owner, also hydrated by room and timeline reads. */
  constructor(readonly users = new UserStore()) {}
  rooms = new ReactiveMap<string, RoomWithViewerState>();
  readonly #roomGroupsSignal = signal<RoomGroup[]>([]);
  get roomGroups(): RoomGroup[] {
    return this.#roomGroupsSignal.get();
  }
  set roomGroups(value: RoomGroup[]) {
    this.#roomGroupsSignal.set(value);
  }
  readonly #activeCallsSignal = signal<ActiveCall[]>([]);
  get activeCalls(): ActiveCall[] {
    return this.#activeCallsSignal.get();
  }
  set activeCalls(value: ActiveCall[]) {
    this.#activeCallsSignal.set(value);
  }

  apply(update: RealtimeProjectionUpdate): void {
    if (update.reset && !update.retainView) this.reset({ preserveViewer: !update.privacyReset });
    const chunk = update.resource;
    if (chunk) {
      switch (chunk.case) {
        case 'server':
          this.server = chunk.value;
          break;
        case 'motd':
          this.serverState = { ...this.serverState, motd: chunk.value.motd };
          break;
        case 'runtimeConfig':
          this.serverState = {
            ...this.serverState,
            runtime: chunk.value.runtime
          };
          break;
        case 'viewer':
          this.viewer = chunk.value;
          if (chunk.value.user?.profile?.id) {
            const profile = chunk.value.user.profile;
            const member = this.users.get(profile.id)?.clone() ?? new DirectoryMember();
            member.user = profile;
            this.users.set(profile.id, member);
          }
          break;
        case 'users': {
          for (const member of chunk.value.users) {
            const userId = member.user?.id;
            if (!userId) continue;
            this.users.set(userId, member);
          }
          break;
        }
        case 'rooms': {
          const nextIds = new Set<string>();
          for (const room of chunk.value.rooms) {
            const roomId = room.room?.id;
            if (!roomId) continue;
            nextIds.add(roomId);
            this.rooms.set(roomId, room);
          }
          for (const roomId of this.rooms.keys()) if (!nextIds.has(roomId)) this.removeRoom(roomId);
          break;
        }
        case 'roomGroups':
          this.roomGroups = [...chunk.value.groups];
          break;
        case 'notifications':
          break;
        case 'activeCalls':
          this.activeCalls = [...chunk.value.calls];
          break;
        case undefined:
          break;
      }
    }

    const semantic = update.event?.event;
    if (semantic?.case === 'messagePosted' && !semantic.value.threadRootEventId) {
      this.activateRoom(semantic.value.roomId);
    }
  }

  reset({ preserveViewer = false }: { preserveViewer?: boolean } = {}): void {
    const viewer = preserveViewer ? this.viewer : null;
    this.server = null;
    this.serverState = null;
    this.viewer = viewer;
    this.users.clear();
    this.rooms.clear();
    this.roomGroups = [];
    this.activeCalls = [];
  }

  /**
   * Removes an account's profile and call participation. The profile becomes
   * a deleted-user tombstone. DM `memberUserIds` keep the ID, because DM
   * membership is fixed at creation and clients show deleted participants.
   */
  removeUser(userId: string): void {
    this.users.delete(userId);
    const inCall = (call: (typeof this.activeCalls)[number]) =>
      call.participants.some((participant) => participant.user?.id === userId);
    // Publish only when a call changed, so other call observers stay quiet.
    if (!this.activeCalls.some(inCall)) return;
    this.activeCalls = this.activeCalls.map((call) => {
      if (!inCall(call)) return call;
      const next = call.clone();
      next.participants = next.participants.filter(
        (participant) => participant.user?.id !== userId
      );
      return next;
    });
  }

  removeRoom(roomId: string): void {
    this.rooms.delete(roomId);
    this.removeRoomCalls(roomId);
  }

  /** Drop a room's active call, for example at an authorization boundary. */
  removeRoomCalls(roomId: string): void {
    this.activeCalls = this.activeCalls.filter((call) => call.room?.id !== roomId);
  }

  /**
   * Optimistically remove one participant from a room's call, for example after
   * the viewer's join fails. The next active-calls replacement is authoritative.
   */
  removeCallParticipant(roomId: string, userId: string): void {
    this.activeCalls = this.activeCalls.flatMap((call) => {
      if (call.room?.id !== roomId) return [call];
      if (!call.participants.some((participant) => participant.user?.id === userId)) return [call];
      const next = call.clone();
      next.participants = next.participants.filter(
        (participant) => participant.user?.id !== userId
      );
      // As before, a call ends locally when no identified participant remains.
      return next.participants.some((participant) => participant.user?.id) ? [next] : [];
    });
  }

  private activateRoom(roomId: string): void {
    const current = this.rooms.get(roomId);
    if (!current) return;
    const room = current.clone();
    room.hasMessageHistory = true;
    const remaining = [...this.rooms.entries()].filter(([id]) => id !== roomId);
    this.rooms.clear();
    this.rooms.set(roomId, room);
    for (const [id, entry] of remaining) this.rooms.set(id, entry);
  }
}
