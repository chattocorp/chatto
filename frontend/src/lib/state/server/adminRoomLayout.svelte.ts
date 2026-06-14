import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';
import { SvelteMap } from 'svelte/reactivity';

const OWN_MUTATION_ECHO_SUPPRESSION_MS = 2000;

const AdminRoomGroupsQuery = graphql(`
  query AdminRoomGroups {
    server {
      rooms(type: CHANNEL) {
        id
        name
        description
        archived
      }
      roomGroups {
        id
        name
        rooms {
          id
        }
      }
    }
  }
`);

const CreateRoomGroupMutation = graphql(`
  mutation AdminCreateRoomGroup($input: CreateRoomGroupInput!) {
    createRoomGroup(input: $input) {
      id
      name
    }
  }
`);

const UpdateRoomGroupMutation = graphql(`
  mutation AdminUpdateRoomGroup($input: UpdateRoomGroupInput!) {
    updateRoomGroup(input: $input) {
      id
      name
    }
  }
`);

const DeleteRoomGroupMutation = graphql(`
  mutation AdminDeleteRoomGroup($input: DeleteRoomGroupInput!) {
    deleteRoomGroup(input: $input)
  }
`);

const ReorderRoomGroupsMutation = graphql(`
  mutation AdminReorderRoomGroups($input: ReorderRoomGroupsInput!) {
    reorderRoomGroups(input: $input) {
      id
    }
  }
`);

const MoveRoomToGroupMutation = graphql(`
  mutation AdminMoveRoomToGroup($input: MoveRoomToGroupInput!) {
    moveRoomToGroup(input: $input) {
      id
    }
  }
`);

const ReorderRoomsInGroupMutation = graphql(`
  mutation AdminReorderRoomsInGroup($input: ReorderRoomsInGroupInput!) {
    reorderRoomsInGroup(input: $input) {
      id
    }
  }
`);

const UpdateRoomMutation = graphql(`
  mutation AdminUpdateRoom($input: UpdateRoomInput!) {
    updateRoom(input: $input) {
      id
      name
      description
    }
  }
`);

const ArchiveRoomMutation = graphql(`
  mutation ArchiveRoom($input: ArchiveRoomInput!) {
    archiveRoom(input: $input) {
      id
      archived
    }
  }
`);

const UnarchiveRoomMutation = graphql(`
  mutation UnarchiveRoom($input: UnarchiveRoomInput!) {
    unarchiveRoom(input: $input) {
      id
      archived
    }
  }
`);

export type AdminRoomInfo = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
};

export type AdminRoomGroup = {
  id: string;
  name: string;
  rooms: AdminRoomInfo[];
};

export type MoveRoomMutationInput = {
  roomId: string;
  groupId: string;
};

export type ReorderRoomsMutationInput = {
  groupId: string;
  orderedRoomIds: string[];
};

export type RoomMovePlan = {
  moves: MoveRoomMutationInput[];
  reorders: ReorderRoomsMutationInput[];
};

export type StoreResult<T extends object = object> =
  | ({ ok: true } & T)
  | { ok: false; error: string };

export type RoomMoveFlushResult =
  | {
      ok: true;
      movedCount: number;
      reorderedCount: number;
    }
  | {
      ok: false;
      movedCount: number;
      reorderedCount: number;
      errors: string[];
      refreshRequested: true;
    };

export type GroupReorderResult =
  | { ok: true; changed: boolean }
  | { ok: false; changed: true; error: string; refreshRequested: true };

export type GroupRoomOrder = SvelteMap<string, string[]>;

function errorMessage(error: unknown): string {
  if (!error) return 'unknown error';
  if (typeof error === 'string') return error;
  if (error instanceof Error) return error.message;
  if (typeof error === 'object' && 'message' in error && typeof error.message === 'string') {
    return error.message;
  }
  return String(error);
}

export function buildGroupRoomOrder(groups: AdminRoomGroup[]): GroupRoomOrder {
  const map = new SvelteMap<string, string[]>();
  for (const group of groups) {
    map.set(
      group.id,
      group.rooms.map((room) => room.id)
    );
  }
  return map;
}

function buildRoomToGroup(snapshot: GroupRoomOrder): SvelteMap<string, string> {
  const map = new SvelteMap<string, string>();
  for (const [groupId, roomIds] of snapshot) {
    for (const roomId of roomIds) {
      map.set(roomId, groupId);
    }
  }
  return map;
}

export function sameOrder(a: readonly string[], b: readonly string[] | undefined): boolean {
  if (!b || a.length !== b.length) return false;
  return a.every((id, index) => id === b[index]);
}

export function planRoomMoveMutations(before: GroupRoomOrder, after: GroupRoomOrder): RoomMovePlan {
  const beforeRoomGroup = buildRoomToGroup(before);
  const afterRoomGroup = buildRoomToGroup(after);
  const moves: MoveRoomMutationInput[] = [];
  const reorders: ReorderRoomsMutationInput[] = [];

  for (const [roomId, groupId] of afterRoomGroup) {
    if (beforeRoomGroup.get(roomId) !== groupId) {
      moves.push({ roomId, groupId });
    }
  }

  for (const [groupId, orderedRoomIds] of after) {
    if (!sameOrder(orderedRoomIds, before.get(groupId))) {
      reorders.push({ groupId, orderedRoomIds });
    }
  }

  return { moves, reorders };
}

export function planGroupReorder(
  beforeIds: readonly string[] | null,
  afterIds: readonly string[]
): string[] | null {
  if (!beforeIds || sameOrder(afterIds, beforeIds)) return null;
  return [...afterIds];
}

function normalizeGroups(groups: AdminRoomGroup[]): AdminRoomGroup[] {
  return groups.map((group) => ({
    ...group,
    rooms: group.rooms ?? []
  }));
}

export class AdminRoomLayoutStore {
  groups = $state<AdminRoomGroup[]>([]);
  initialized = $state(false);
  isRefreshing = $state(false);
  error = $state<string | null>(null);
  isDragging = $state(false);
  draggingGroupId = $state<string | null>(null);
  updatingRoom = $state(false);
  archivingRoomId = $state<string | null>(null);

  #loadId = 0;
  #lastMutationTimestamp = 0;
  #preDragSnapshot: GroupRoomOrder | null = null;
  #pendingMoveDiff = false;
  #preReorderIds: string[] | null = null;

  constructor(
    private readonly client: Client,
    private readonly now: () => number = () => Date.now()
  ) {}

  get loading(): boolean {
    return this.isRefreshing && !this.initialized;
  }

  async refresh(): Promise<void> {
    const thisLoad = ++this.#loadId;
    this.isRefreshing = true;
    try {
      const result = await this.client
        .query(AdminRoomGroupsQuery, {}, { requestPolicy: 'network-only' })
        .toPromise();
      if (this.#loadId !== thisLoad) return;

      if (result.error) {
        this.error = errorMessage(result.error);
        return;
      }

      const server = result.data?.server;
      if (!server) {
        this.error = 'Server not found';
        return;
      }

      const roomsMap = new SvelteMap<string, AdminRoomInfo>(
        (server.rooms ?? []).map((room) => [
          room.id,
          {
            id: room.id,
            name: room.name,
            description: room.description,
            archived: room.archived
          }
        ])
      );

      this.groups = normalizeGroups(
        server.roomGroups.map((group) => ({
          id: group.id,
          name: group.name,
          rooms: (group.rooms ?? [])
            .map((room) => roomsMap.get(room.id))
            .filter((room): room is AdminRoomInfo => room != null)
        }))
      );
      this.error = null;
      this.initialized = true;
    } catch (err) {
      if (this.#loadId === thisLoad) {
        this.error = errorMessage(err);
      }
    } finally {
      if (this.#loadId === thisLoad) {
        this.isRefreshing = false;
      }
    }
  }

  async createGroup(name: string): Promise<StoreResult<{ group: AdminRoomGroup }>> {
    const result = await this.client
      .mutation(CreateRoomGroupMutation, { input: { name } })
      .toPromise();

    if (result.error || !result.data?.createRoomGroup) {
      return { ok: false, error: errorMessage(result.error) };
    }

    const created = result.data.createRoomGroup;
    const group = { id: created.id, name: created.name, rooms: [] };
    this.groups = [...this.groups, group];
    this.markMutation();
    return { ok: true, group };
  }

  async renameGroup(groupId: string, newName: string): Promise<StoreResult> {
    const idx = this.groups.findIndex((group) => group.id === groupId);
    if (idx === -1) return { ok: true };

    const result = await this.client
      .mutation(UpdateRoomGroupMutation, { input: { id: groupId, name: newName } })
      .toPromise();

    if (result.error) {
      return { ok: false, error: errorMessage(result.error) };
    }

    this.groups[idx] = { ...this.groups[idx], name: newName };
    this.markMutation();
    return { ok: true };
  }

  async deleteGroup(groupId: string): Promise<StoreResult> {
    const result = await this.client
      .mutation(DeleteRoomGroupMutation, { input: { id: groupId } })
      .toPromise();

    if (result.error) {
      return { ok: false, error: errorMessage(result.error) };
    }

    this.groups = this.groups.filter((group) => group.id !== groupId);
    this.markMutation();
    return { ok: true };
  }

  async updateRoom(roomId: string, name: string, description: string | null): Promise<StoreResult> {
    this.updatingRoom = true;
    try {
      const result = await this.client
        .mutation(UpdateRoomMutation, { input: { roomId, name, description } })
        .toPromise();

      if (result.error) {
        return { ok: false, error: errorMessage(result.error) };
      }

      this.markMutation();
      await this.refresh();
      return { ok: true };
    } finally {
      this.updatingRoom = false;
    }
  }

  async archiveRoom(roomId: string): Promise<StoreResult> {
    return this.setRoomArchived(roomId, true);
  }

  async unarchiveRoom(roomId: string): Promise<StoreResult> {
    return this.setRoomArchived(roomId, false);
  }

  handleRoomCreated(): void {
    this.markMutation();
    void this.refresh();
  }

  handleRoomDragConsider(groupId: string, items: AdminRoomInfo[]): void {
    this.isDragging = true;
    this.captureRoomDragSnapshotIfNeeded();
    this.setGroupRooms(groupId, items);
  }

  async handleRoomDragFinalize(
    groupId: string,
    items: AdminRoomInfo[]
  ): Promise<RoomMoveFlushResult | null> {
    this.setGroupRooms(groupId, items);
    this.isDragging = false;

    if (this.#pendingMoveDiff) return null;
    this.#pendingMoveDiff = true;
    await Promise.resolve();
    this.#pendingMoveDiff = false;
    return this.flushRoomMoves();
  }

  handleGroupsConsider(items: AdminRoomGroup[], draggingGroupId?: string | null): void {
    this.isDragging = true;
    this.draggingGroupId = draggingGroupId ?? null;
    if (!this.#preReorderIds) {
      this.#preReorderIds = this.groups.map((group) => group.id);
    }
    this.groups = normalizeGroups(items);
  }

  async handleGroupsFinalize(items: AdminRoomGroup[]): Promise<GroupReorderResult> {
    this.draggingGroupId = null;
    this.groups = normalizeGroups(items);
    this.isDragging = false;

    const orderedIds = planGroupReorder(
      this.#preReorderIds,
      this.groups.map((group) => group.id)
    );
    this.#preReorderIds = null;
    if (!orderedIds) return { ok: true, changed: false };

    const result = await this.client
      .mutation(ReorderRoomGroupsMutation, { input: { orderedIds } })
      .toPromise();

    if (result.error) {
      void this.refresh();
      return {
        ok: false,
        changed: true,
        error: errorMessage(result.error),
        refreshRequested: true
      };
    }

    this.markMutation();
    return { ok: true, changed: true };
  }

  async flushRoomMoves(): Promise<RoomMoveFlushResult | null> {
    if (!this.#preDragSnapshot) return null;
    const before = this.#preDragSnapshot;
    this.#preDragSnapshot = null;

    const plan = planRoomMoveMutations(before, buildGroupRoomOrder(this.groups));
    if (plan.moves.length === 0 && plan.reorders.length === 0) return null;

    const errors: string[] = [];
    for (const move of plan.moves) {
      const result = await this.client
        .mutation(MoveRoomToGroupMutation, { input: move })
        .toPromise();
      if (result.error) {
        errors.push(`Failed to move room: ${errorMessage(result.error)}`);
      }
    }

    for (const reorder of plan.reorders) {
      const result = await this.client
        .mutation(ReorderRoomsInGroupMutation, { input: reorder })
        .toPromise();
      if (result.error) {
        errors.push(`Failed to reorder rooms: ${errorMessage(result.error)}`);
      }
    }

    this.markMutation();
    if (errors.length > 0) {
      void this.refresh();
      return {
        ok: false,
        movedCount: plan.moves.length,
        reorderedCount: plan.reorders.length,
        errors,
        refreshRequested: true
      };
    }

    return {
      ok: true,
      movedCount: plan.moves.length,
      reorderedCount: plan.reorders.length
    };
  }

  ingestServerEvent(serverEvent: { event?: { __typename?: string } | null }): boolean {
    const event = serverEvent.event;
    if (!event) return false;
    if (event.__typename === 'RoomGroupsUpdatedEvent') {
      return this.ingestRoomLayoutUpdated();
    }
    if (
      event.__typename === 'RoomUpdatedEvent' ||
      event.__typename === 'RoomArchivedEvent' ||
      event.__typename === 'RoomUnarchivedEvent'
    ) {
      return this.ingestRoomMetadataUpdated();
    }
    return false;
  }

  ingestRoomLayoutUpdated(now = this.now()): boolean {
    if (this.shouldSuppressLiveRefresh(now)) return false;
    void this.refresh();
    return true;
  }

  private ingestRoomMetadataUpdated(now = this.now()): boolean {
    if (this.shouldSuppressLiveRefresh(now)) return false;
    void this.refresh();
    return true;
  }

  private async setRoomArchived(roomId: string, archived: boolean): Promise<StoreResult> {
    this.archivingRoomId = roomId;
    try {
      const result = await this.client
        .mutation(archived ? ArchiveRoomMutation : UnarchiveRoomMutation, { input: { roomId } })
        .toPromise();

      if (result.error) {
        return { ok: false, error: errorMessage(result.error) };
      }

      this.markMutation();
      await this.refresh();
      return { ok: true };
    } finally {
      this.archivingRoomId = null;
    }
  }

  private captureRoomDragSnapshotIfNeeded(): void {
    if (!this.#preDragSnapshot) {
      this.#preDragSnapshot = buildGroupRoomOrder(this.groups);
    }
  }

  private setGroupRooms(groupId: string, items: AdminRoomInfo[]): void {
    const idx = this.groups.findIndex((group) => group.id === groupId);
    if (idx !== -1) {
      this.groups[idx] = { ...this.groups[idx], rooms: items };
    }
  }

  private markMutation(): void {
    this.#lastMutationTimestamp = this.now();
  }

  private shouldSuppressLiveRefresh(now: number): boolean {
    return this.isDragging || now - this.#lastMutationTimestamp < OWN_MUTATION_ECHO_SUPPRESSION_MS;
  }
}
