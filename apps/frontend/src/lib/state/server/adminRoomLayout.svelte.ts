import type {
  AdminRoomGroup,
  AdminRoomInfo,
  AdminRoomLayoutAPI,
  AdminRoomLayoutItemMutationInput,
  AdminSidebarItem,
  AdminSidebarLinkInfo
} from '$lib/api/adminRoomLayout';
import type { RoomCommandAPI } from '$lib/api/rooms';
import { RoomEventKind, roomEventKind, type RoomEventKindSource } from '$lib/render/eventKinds';
import { SvelteMap } from 'svelte/reactivity';

const OWN_MUTATION_ECHO_SUPPRESSION_MS = 2000;

export type {
  AdminRoomGroup,
  AdminRoomInfo,
  AdminSidebarItem,
  AdminSidebarLinkInfo
} from '$lib/api/adminRoomLayout';

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
  #lastMutationTimestamp = 0;
  #preDragSnapshot: GroupItemOrder | null = null;
  #pendingMoveDiff = false;
  #preReorderIds: string[] | null = null;

  constructor(
    private readonly layoutAPI: AdminRoomLayoutAPI,
    private readonly roomAPI: Pick<
      RoomCommandAPI,
      'updateRoom' | 'archiveRoom' | 'unarchiveRoom' | 'setRoomUniversal'
    >,
    private readonly now: () => number = () => Date.now()
  ) {}

  get loading(): boolean {
    return this.isRefreshing && !this.initialized;
  }

  async refresh(): Promise<void> {
    const thisLoad = ++this.#loadId;
    this.isRefreshing = true;
    try {
      const groups = await this.layoutAPI.listAdminRoomLayout();
      if (this.#loadId !== thisLoad) return;

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

  async createGroup(name: string): Promise<StoreResult<{ group: AdminRoomGroup }>> {
    let group: AdminRoomGroup | null;
    try {
      group = await this.layoutAPI.createRoomGroup({ name });
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }
    if (!group) return { ok: false, error: 'Room group not found' };
    this.groups = [...this.groups, group];
    this.markMutation();
    return { ok: true, group };
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
    this.markMutation();
    return { ok: true };
  }

  async deleteGroup(groupId: string): Promise<StoreResult> {
    try {
      await this.layoutAPI.deleteRoomGroup(groupId);
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }

    this.groups = this.groups.filter((group) => group.id !== groupId);
    this.markMutation();
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

    this.markMutation();
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

    this.markMutation();
    await this.refresh();
    return { ok: true, link };
  }

  async deleteSidebarLink(linkId: string): Promise<StoreResult> {
    try {
      await this.layoutAPI.deleteSidebarLink(linkId);
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    }

    this.markMutation();
    await this.refresh();
    return { ok: true };
  }

  async updateRoom(roomId: string, name: string, description: string | null): Promise<StoreResult> {
    this.updatingRoom = true;
    try {
      await this.roomAPI.updateRoom({ roomId, name, description });
      this.markMutation();
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

  async setRoomUniversal(roomId: string, isUniversal: boolean): Promise<StoreResult> {
    this.universalRoomId = roomId;
    try {
      await this.roomAPI.setRoomUniversal(roomId, isUniversal);
      this.markMutation();
      await this.refresh();
      return { ok: true };
    } catch (error) {
      return { ok: false, error: errorMessage(error) };
    } finally {
      this.universalRoomId = null;
    }
  }

  handleRoomCreated(): void {
    this.markMutation();
    void this.refresh();
  }

  handleRoomDragConsider(groupId: string, items: Array<AdminSidebarItem | AdminRoomInfo>): void {
    this.isDragging = true;
    this.captureRoomDragSnapshotIfNeeded();
    this.setGroupItems(groupId, toSidebarItems(items));
  }

  async handleRoomDragFinalize(
    groupId: string,
    items: Array<AdminSidebarItem | AdminRoomInfo>
  ): Promise<RoomMoveFlushResult | null> {
    this.setGroupItems(groupId, toSidebarItems(items));
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

    this.markMutation();
    return { ok: true, changed: true };
  }

  async flushRoomMoves(): Promise<RoomMoveFlushResult | null> {
    if (!this.#preDragSnapshot) return null;
    const before = this.#preDragSnapshot;
    this.#preDragSnapshot = null;

    const plan = planSidebarItemMutations(before, buildGroupItemOrder(this.groups));
    if (plan.moves.length === 0 && plan.linkMoves.length === 0 && plan.reorders.length === 0) {
      return null;
    }

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

    this.markMutation();
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

  ingestServerEvent(serverEvent: { event?: RoomEventKindSource }): boolean {
    const kind = roomEventKind(serverEvent.event);
    if (kind === RoomEventKind.RoomGroupsUpdated) {
      return this.ingestRoomLayoutUpdated();
    }
    if (
      kind === RoomEventKind.RoomUpdated ||
      kind === RoomEventKind.RoomArchived ||
      kind === RoomEventKind.RoomUnarchived ||
      kind === RoomEventKind.RoomUniversalChanged
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
      if (archived) {
        await this.roomAPI.archiveRoom(roomId);
      } else {
        await this.roomAPI.unarchiveRoom(roomId);
      }
      this.markMutation();
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

  private markMutation(): void {
    this.#lastMutationTimestamp = this.now();
  }

  private shouldSuppressLiveRefresh(now: number): boolean {
    return this.isDragging || now - this.#lastMutationTimestamp < OWN_MUTATION_ECHO_SUPPRESSION_MS;
  }
}
