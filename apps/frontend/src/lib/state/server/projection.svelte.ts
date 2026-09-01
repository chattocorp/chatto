import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import type { ThreadViewerState } from '@chatto/api-types/api/v1/message_types_pb';
import type {
  RoomGroup,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import type { RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { ServerRuntimeConfig } from '@chatto/api-types/api/v1/server_state_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import type { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';

/** Authenticated server resources assembled from two canonical responses. */
export type ProjectedServerState = {
  motd?: string;
  runtime?: ServerRuntimeConfig;
};

/** Canonical protobuf-native resources for one connected Chatto server. */
export class ServerProjectionStore {
  server = $state.raw<ServerPublicProfile | null>(null);
  serverState = $state.raw<ProjectedServerState | null>(null);
  viewer = $state.raw<GetViewerResponse | null>(null);
  users = new SvelteMap<string, DirectoryMember>();
  rooms = new SvelteMap<string, RoomWithViewerState>();
  roomGroups = $state.raw<RoomGroup[]>([]);
  activeCalls = $state.raw<ActiveCall[]>([]);

  // Timelines and followed-thread state come from explicit ConnectRPC reads.
  // These maps remain as temporary selectors while their consumers migrate.
  threadViewerStates = new SvelteMap<string, ThreadViewerState>();
  timelines = new SvelteMap<string, RoomTimelinePage>();

  apply(update: RealtimeProjectionUpdate): void {
    if (update.reset) this.reset({ preserveViewer: true });
    const chunk = update.snapshot;
    if (chunk) {
      switch (chunk.resource.case) {
        case 'server':
          this.server = chunk.resource.value;
          break;
        case 'motd':
          this.serverState = { ...this.serverState, motd: chunk.resource.value.motd };
          break;
        case 'runtimeConfig':
          this.serverState = {
            ...this.serverState,
            runtime: chunk.resource.value.runtime
          };
          break;
        case 'viewer':
          this.viewer = chunk.resource.value;
          break;
        case 'users': {
          const nextIds = new SvelteSet<string>();
          for (const member of chunk.resource.value.users) {
            const userId = member.user?.id;
            if (!userId) continue;
            nextIds.add(userId);
            this.users.set(userId, member);
          }
          for (const userId of this.users.keys()) if (!nextIds.has(userId)) this.removeUser(userId);
          break;
        }
        case 'rooms': {
          const nextIds = new SvelteSet<string>();
          for (const room of chunk.resource.value.rooms) {
            const roomId = room.room?.id;
            if (!roomId) continue;
            nextIds.add(roomId);
            this.rooms.set(roomId, room);
          }
          for (const roomId of this.rooms.keys()) if (!nextIds.has(roomId)) this.removeRoom(roomId);
          break;
        }
        case 'roomGroups':
          this.roomGroups = [...chunk.resource.value.groups];
          break;
        case 'notifications':
          break;
        case 'activeCalls':
          this.activeCalls = [...chunk.resource.value.calls];
          break;
        case undefined:
          break;
      }
    }

    const semantic = update.event?.event;
    if (semantic?.case === 'messagePosted' && !semantic.value.inThread) {
      this.activateRoom(semantic.value.roomId);
    }
  }

  /** Drop one legacy in-memory timeline selector. */
  evictRoomTimeline(roomId: string, _clearMembership: boolean): void {
    this.timelines.delete(roomId);
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
    this.threadViewerStates.clear();
    this.timelines.clear();
  }

  removeUser(userId: string): void {
    this.users.delete(userId);
    for (const [roomId, room] of this.rooms) {
      if (!room.memberUserIds.includes(userId)) continue;
      const next = room.clone();
      next.memberUserIds = next.memberUserIds.filter((candidate) => candidate !== userId);
      this.rooms.set(roomId, next);
    }
    this.activeCalls = this.activeCalls.map((call) => {
      if (!call.participants.some((participant) => participant.user?.id === userId)) return call;
      const next = call.clone();
      next.participants = next.participants.filter((participant) => participant.user?.id !== userId);
      return next;
    });
  }

  removeRoom(roomId: string): void {
    this.rooms.delete(roomId);
    this.timelines.delete(roomId);
    this.activeCalls = this.activeCalls.filter((call) => call.room?.id !== roomId);
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
