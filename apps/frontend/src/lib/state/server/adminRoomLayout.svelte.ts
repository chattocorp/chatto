import type {
  AdminRoomGroup,
  AdminRoomInfo,
  AdminRoomLayoutAPI,
  AdminRoomLayoutItemMutationInput,
  AdminSidebarItem,
  AdminSidebarLinkInfo
} from '$lib/api-client/adminRoomLayout';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';

export type {
  AdminRoomGroup,
  AdminRoomInfo,
  AdminSidebarItem,
  AdminSidebarLinkInfo
} from '$lib/api-client/adminRoomLayout';

export type MoveRoomMutationInput = {
  roomId: string;
  groupId: string;
};

export type ReorderRoomsMutationInput = {
  groupId: string;
  items: AdminRoomLayoutItemMutationInput[];
};

export type RoomMovePlan = {
  moves: MoveRoomMutationInput[];
  linkMoves: MoveRoomMutationInput[];
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
export type GroupItemOrder = SvelteMap<string, AdminSidebarItem[]>;

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

export function buildGroupItemOrder(groups: AdminRoomGroup[]): GroupItemOrder {
  const map = new SvelteMap<string, AdminSidebarItem[]>();
  for (const group of groups) {
    map.set(
      group.id,
      group.items ??
        group.rooms.map((room) => ({ id: `room:${room.id}`, kind: 'room' as const, room }))
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

function buildItemToGroup(
  snapshot: GroupItemOrder,
  kind: AdminSidebarItem['kind']
): SvelteMap<string, string> {
  const map = new SvelteMap<string, string>();
  for (const [groupId, items] of snapshot) {
    for (const item of items) {
      if (item.kind === kind) map.set(itemId(item), groupId);
    }
  }
  return map;
}

function itemId(item: AdminSidebarItem): string {
  return item.kind === 'room' ? item.room.id : item.link.id;
}

function itemToMutationInput(item: AdminSidebarItem): AdminRoomLayoutItemMutationInput {
  return {
    kind: item.kind,
    id: itemId(item)
  };
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
      reorders.push({
        groupId,
        items: orderedRoomIds.map((id) => ({ kind: 'room', id }))
      });
    }
  }

  return { moves, linkMoves: [], reorders };
}

export function planSidebarItemMutations(
  before: GroupItemOrder,
  after: GroupItemOrder
): RoomMovePlan {
  const beforeRooms = buildItemToGroup(before, 'room');
  const afterRooms = buildItemToGroup(after, 'room');
  const beforeLinks = buildItemToGroup(before, 'link');
  const afterLinks = buildItemToGroup(after, 'link');
  const moves: MoveRoomMutationInput[] = [];
  const linkMoves: MoveRoomMutationInput[] = [];
  const reorders: ReorderRoomsMutationInput[] = [];

  for (const [roomId, groupId] of afterRooms) {
    if (beforeRooms.get(roomId) !== groupId) {
      moves.push({ roomId, groupId });
    }
  }
  for (const [linkId, groupId] of afterLinks) {
    if (beforeLinks.get(linkId) !== groupId) {
      linkMoves.push({ roomId: linkId, groupId });
    }
  }
  for (const [groupId, items] of after) {
    const beforeItems = before.get(groupId);
    if (
      !beforeItems ||
      !sameOrder(
        items.map((item) => item.id),
        beforeItems.map((item) => item.id)
      )
    ) {
      reorders.push({ groupId, items: items.map(itemToMutationInput) });
    }
  }
  return { moves, linkMoves, reorders };
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
    rooms:
      group.items?.filter((item) => item.kind === 'room').map((item) => item.room) ??
      group.rooms ??
      [],
    items:
      group.items ??
      (group.rooms ?? []).map((room) => ({ id: `room:${room.id}`, kind: 'room' as const, room }))
  }));
}

function toSidebarItems(items: Array<AdminSidebarItem | AdminRoomInfo>): AdminSidebarItem[] {
  return items.map((item) => {
    if ('kind' in item) return item;
    return { id: `room:${item.id}`, kind: 'room', room: item };
  });
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
  universalRoomId = $state<string | null>(null);

  #loadId = 0;
  #interactionGeneration = 0;
  #activeRoomDragGeneration: number | null = null;
  #activeGroupDragGeneration: number | null = null;
  #preDragSnapshot: GroupItemOrder | null = null;
  #roomPersistenceGenerations = new SvelteSet<number>();
  #groupPersistenceGenerations = new SvelteSet<number>();
  #roomPersistenceTail: Promise<void> = Promise.resolve();
  #groupPersistenceTail: Promise<void> = Promise.resolve();
  #preReorderIds: string[] | null = null;
  #projectionRefreshPending = false;
  #projectionRefreshTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private readonly layoutAPI: AdminRoomLayoutAPI,
    private readonly roomAPI: Pick<RoomCommandAPI, 'updateRoom' | 'archiveRoom' | 'unarchiveRoom'>
  ) {}

  get loading(): boolean {
    return this.isRefreshing && !this.initialized;
  }

  async refresh(): Promise<void> {
    const thisLoad = ++this.#loadId;
    const interactionGeneration = this.#interactionGeneration;
    this.isRefreshing = true;
    try {
      const groups = await this.layoutAPI.listRoomGroups();
      if (this.#loadId !== thisLoad) return;
      if (
        interactionGeneration !== this.#interactionGeneration ||
        this.isDragging ||
        this.#roomPersistenceGenerations.size > 0 ||
        this.#groupPersistenceGenerations.size > 0
      ) {
        // This read began against pre-interaction state. Never let its result
        // replace an active drag or the optimistic order being persisted.
        this.#projectionRefreshPending = true;
        this.scheduleProjectionRefresh();
        return;
      }

      this.groups = normalizeGroups(groups);
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

  /**
   * Reconcile an open editor after the realtime projection changes. Refreshes
   * are coalesced and deferred until drag persistence has completed so an
   * authoritative read cannot replace the drag snapshot halfway through a
   * room move or group reorder.
   */
  requestProjectionRefresh(): void {
    this.#projectionRefreshPending = true;
    this.scheduleProjectionRefresh();
  }

  /** Cancel projection and transient drag work when the admin route unmounts. */
  deactivateProjectionRefresh(): void {
    this.#projectionRefreshPending = false;
    if (this.#projectionRefreshTimer) clearTimeout(this.#projectionRefreshTimer);
    this.#projectionRefreshTimer = null;
    // Drag-and-drop libraries do not guarantee a finalize callback after the
    // component is destroyed. Treat route teardown as cancellation so the
    // cached per-server store can be activated again with a clean lifecycle.
    this.#loadId += 1;
    this.#interactionGeneration += 1;
    this.#activeRoomDragGeneration = null;
    this.#activeGroupDragGeneration = null;
    this.isRefreshing = false;
    this.isDragging = false;
    this.draggingGroupId = null;
    this.#preDragSnapshot = null;
    this.#preReorderIds = null;
  }

  async createGroup(name: string): Promise<StoreResult<{ group: AdminRoomGroup }>> {
    let group: AdminRoomGroup | null;
    try {
      group = await this.layoutAPI.createRoomGroup({ name });
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }
    if (!group) return { ok: false, error: 'Room group not found' };
    this.groups = [...this.groups, group];
    await this.refresh();
    return { ok: true, group: this.groups.find((candidate) => candidate.id === group.id) ?? group };
  }

  async renameGroup(groupId: string, newName: string): Promise<StoreResult> {
    const idx = this.groups.findIndex((group) => group.id === groupId);
    if (idx === -1) return { ok: true };

    try {
      await this.layoutAPI.updateRoomGroup({ groupId, name: newName });
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }

    this.groups[idx] = { ...this.groups[idx], name: newName };
    return { ok: true };
  }

  async deleteGroup(groupId: string): Promise<StoreResult> {
    try {
      await this.layoutAPI.deleteRoomGroup(groupId);
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }

    this.groups = this.groups.filter((group) => group.id !== groupId);
    return { ok: true };
  }

  async createSidebarLink(
    groupId: string,
    label: string,
    url: string
  ): Promise<StoreResult<{ link: AdminSidebarLinkInfo }>> {
    let link: AdminSidebarLinkInfo | null;
    try {
      link = await this.layoutAPI.createSidebarLink({ groupId, label, url });
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }
    if (!link) return { ok: false, error: 'Sidebar link not found' };

    await this.refresh();
    return { ok: true, link };
  }

  async updateSidebarLink(
    linkId: string,
    label: string,
    url: string
  ): Promise<StoreResult<{ link: AdminSidebarLinkInfo }>> {
    let link: AdminSidebarLinkInfo | null;
    try {
      link = await this.layoutAPI.updateSidebarLink({ linkId, label, url });
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }
    if (!link) return { ok: false, error: 'Sidebar link not found' };

    await this.refresh();
    return { ok: true, link };
  }

  async deleteSidebarLink(linkId: string): Promise<StoreResult> {
    try {
      await this.layoutAPI.deleteSidebarLink(linkId);
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }

    await this.refresh();
    return { ok: true };
  }

  async updateRoom(roomId: string, name: string, description: string | null): Promise<StoreResult> {
    this.updatingRoom = true;
    try {
      await this.roomAPI.updateRoom({ roomId, name, description });
      await this.refresh();
      return { ok: true };
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
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

  async updateRoomUniversal(roomId: string, isUniversal: boolean): Promise<StoreResult> {
    this.universalRoomId = roomId;
    try {
      await this.roomAPI.updateRoom({ roomId, universal: isUniversal });
      await this.refresh();
      return { ok: true };
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    } finally {
      this.universalRoomId = null;
    }
  }

  handleRoomCreated(): void {
    void this.refresh();
  }

  handleRoomDragConsider(
    groupId: string,
    items: Array<AdminSidebarItem | AdminRoomInfo>,
    dragGeneration: number | null = null
  ): number {
    if (dragGeneration !== null && dragGeneration !== this.#activeRoomDragGeneration) {
      return dragGeneration;
    }
    if (dragGeneration === null) {
      this.#interactionGeneration += 1;
      dragGeneration = this.#interactionGeneration;
      this.#activeRoomDragGeneration = dragGeneration;
    }
    this.isDragging = true;
    this.captureRoomDragSnapshotIfNeeded();
    this.setGroupItems(groupId, toSidebarItems(items));
    return dragGeneration;
  }

  async handleRoomDragFinalize(
    groupId: string,
    items: Array<AdminSidebarItem | AdminRoomInfo>,
    dragGeneration: number | null = this.#activeRoomDragGeneration
  ): Promise<RoomMoveFlushResult | null> {
    if (dragGeneration === null || dragGeneration !== this.#activeRoomDragGeneration) return null;
    this.setGroupItems(groupId, toSidebarItems(items));
    this.isDragging = false;

    if (this.#roomPersistenceGenerations.has(dragGeneration)) return null;
    const interactionGeneration = this.#interactionGeneration;
    this.#roomPersistenceGenerations.add(dragGeneration);
    try {
      await Promise.resolve();
      const plan = this.takeRoomMovePlan();
      if (!plan) return null;
      const persistence = this.#roomPersistenceTail.then(() => this.persistRoomMoves(plan));
      this.#roomPersistenceTail = persistence.then(
        () => undefined,
        () => undefined
      );
      return await persistence;
    } finally {
      this.#roomPersistenceGenerations.delete(dragGeneration);
      if (
        interactionGeneration === this.#interactionGeneration &&
        this.#activeRoomDragGeneration === dragGeneration
      ) {
        this.#activeRoomDragGeneration = null;
      }
      this.scheduleProjectionRefresh();
    }
  }

  handleGroupsConsider(
    items: AdminRoomGroup[],
    draggingGroupId?: string | null,
    dragGeneration: number | null = null
  ): number {
    if (dragGeneration !== null && dragGeneration !== this.#activeGroupDragGeneration) {
      return dragGeneration;
    }
    if (dragGeneration === null) {
      this.#interactionGeneration += 1;
      dragGeneration = this.#interactionGeneration;
      this.#activeGroupDragGeneration = dragGeneration;
    }
    this.isDragging = true;
    this.draggingGroupId = draggingGroupId ?? null;
    if (!this.#preReorderIds) {
      this.#preReorderIds = this.groups.map((group) => group.id);
    }
    this.groups = normalizeGroups(items);
    return dragGeneration;
  }

  async handleGroupsFinalize(
    items: AdminRoomGroup[],
    dragGeneration: number | null = this.#activeGroupDragGeneration
  ): Promise<GroupReorderResult> {
    if (dragGeneration === null || dragGeneration !== this.#activeGroupDragGeneration) {
      return { ok: true, changed: false };
    }
    this.#activeGroupDragGeneration = null;
    this.draggingGroupId = null;
    this.groups = normalizeGroups(items);
    this.isDragging = false;
    this.#groupPersistenceGenerations.add(dragGeneration);

    try {
      const orderedIds = planGroupReorder(
        this.#preReorderIds,
        this.groups.map((group) => group.id)
      );
      this.#preReorderIds = null;
      if (!orderedIds) return { ok: true, changed: false };

      const persistence = this.#groupPersistenceTail.then(async (): Promise<GroupReorderResult> => {
        try {
          await this.layoutAPI.reorderRoomGroups(orderedIds);
        } catch (error) {
          void this.refresh();
          return {
            ok: false,
            changed: true,
            error: errorMessage(error),
            refreshRequested: true
          };
        }
        return { ok: true, changed: true };
      });
      this.#groupPersistenceTail = persistence.then(
        () => undefined,
        () => undefined
      );
      return await persistence;
    } finally {
      this.#groupPersistenceGenerations.delete(dragGeneration);
      this.scheduleProjectionRefresh();
    }
  }

  private takeRoomMovePlan(): ReturnType<typeof planSidebarItemMutations> | null {
    if (!this.#preDragSnapshot) return null;
    const before = this.#preDragSnapshot;
    this.#preDragSnapshot = null;

    const plan = planSidebarItemMutations(before, buildGroupItemOrder(this.groups));
    if (plan.moves.length === 0 && plan.linkMoves.length === 0 && plan.reorders.length === 0) {
      return null;
    }
    return plan;
  }

  private async persistRoomMoves(
    plan: ReturnType<typeof planSidebarItemMutations>
  ): Promise<RoomMoveFlushResult> {
    const errors: string[] = [];
    for (const move of plan.moves) {
      try {
        await this.layoutAPI.moveRoomToGroup(move);
      } catch (error) {
        errors.push(`Failed to move room: ${errorMessage(error)}`);
      }
    }

    for (const move of plan.linkMoves) {
      try {
        await this.layoutAPI.moveSidebarLinkToGroup({ linkId: move.roomId, groupId: move.groupId });
      } catch (error) {
        errors.push(`Failed to move link: ${errorMessage(error)}`);
      }
    }

    for (const reorder of plan.reorders) {
      try {
        await this.layoutAPI.reorderSidebarItemsInGroup(reorder);
      } catch (error) {
        errors.push(`Failed to reorder rooms: ${errorMessage(error)}`);
      }
    }

    if (errors.length > 0) {
      void this.refresh();
      return {
        ok: false,
        movedCount: plan.moves.length + plan.linkMoves.length,
        reorderedCount: plan.reorders.length,
        errors,
        refreshRequested: true
      };
    }

    return {
      ok: true,
      movedCount: plan.moves.length + plan.linkMoves.length,
      reorderedCount: plan.reorders.length
    };
  }

  private async setRoomArchived(roomId: string, archived: boolean): Promise<StoreResult> {
    this.archivingRoomId = roomId;
    try {
      if (archived) {
        await this.roomAPI.archiveRoom(roomId);
      } else {
        await this.roomAPI.unarchiveRoom(roomId);
      }
      await this.refresh();
      return { ok: true };
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    } finally {
      this.archivingRoomId = null;
    }
  }

  private captureRoomDragSnapshotIfNeeded(): void {
    if (!this.#preDragSnapshot) {
      this.#preDragSnapshot = buildGroupItemOrder(this.groups);
    }
  }

  private setGroupItems(groupId: string, items: AdminSidebarItem[]): void {
    const idx = this.groups.findIndex((group) => group.id === groupId);
    if (idx !== -1) {
      this.groups[idx] = normalizeGroups([{ ...this.groups[idx], items }])[0];
    }
  }

  private scheduleProjectionRefresh(): void {
    if (
      !this.#projectionRefreshPending ||
      this.isDragging ||
      this.#roomPersistenceGenerations.size > 0 ||
      this.#groupPersistenceGenerations.size > 0
    ) {
      return;
    }
    if (this.#projectionRefreshTimer) clearTimeout(this.#projectionRefreshTimer);
    this.#projectionRefreshTimer = setTimeout(() => {
      this.#projectionRefreshTimer = null;
      if (
        this.isDragging ||
        this.#roomPersistenceGenerations.size > 0 ||
        this.#groupPersistenceGenerations.size > 0
      ) {
        return;
      }
      this.#projectionRefreshPending = false;
      void this.refresh();
    }, 50);
  }
}
