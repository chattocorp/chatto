import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';
import { isUnsupportedGraphQLFieldError } from '$lib/gql/compatibility';
import { RoomGroupItemType } from '$lib/gql/graphql';
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
        isUniversal
      }
      roomGroups {
        id
        name
        rooms {
          id
        }
        items {
          type
          id
          room {
            id
          }
          link {
            id
            label
            url
          }
        }
      }
    }
  }
`);

const AdminRoomGroupsCompatibilityQuery = graphql(`
  query AdminRoomGroupsCompatibility {
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
        items {
          type
          id
          room {
            id
          }
          link {
            id
            label
            url
          }
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
  mutation AdminReorderSidebarItemsInGroup($input: ReorderSidebarItemsInGroupInput!) {
    reorderSidebarItemsInGroup(input: $input) {
      id
    }
  }
`);

const CreateSidebarLinkMutation = graphql(`
  mutation AdminCreateSidebarLink($input: CreateSidebarLinkInput!) {
    createSidebarLink(input: $input) {
      id
      label
      url
    }
  }
`);

const UpdateSidebarLinkMutation = graphql(`
  mutation AdminUpdateSidebarLink($input: UpdateSidebarLinkInput!) {
    updateSidebarLink(input: $input) {
      id
      label
      url
    }
  }
`);

const DeleteSidebarLinkMutation = graphql(`
  mutation AdminDeleteSidebarLink($input: DeleteSidebarLinkInput!) {
    deleteSidebarLink(input: $input)
  }
`);

const MoveSidebarLinkToGroupMutation = graphql(`
  mutation AdminMoveSidebarLinkToGroup($input: MoveSidebarLinkToGroupInput!) {
    moveSidebarLinkToGroup(input: $input) {
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

const SetRoomUniversalMutation = graphql(`
  mutation AdminSetRoomUniversal($input: SetRoomUniversalInput!) {
    setRoomUniversal(input: $input) {
      id
      isUniversal
    }
  }
`);

export type AdminRoomInfo = {
  id: string;
  name: string;
  description?: string | null;
  archived: boolean;
  isUniversal: boolean;
};

export type AdminSidebarLinkInfo = {
  id: string;
  label: string;
  url: string;
};

export type AdminSidebarItem =
  | {
      id: string;
      kind: 'room';
      room: AdminRoomInfo;
    }
  | {
      id: string;
      kind: 'link';
      link: AdminSidebarLinkInfo;
    };

export type AdminRoomGroup = {
  id: string;
  name: string;
  rooms: AdminRoomInfo[];
  items?: AdminSidebarItem[];
};

export type MoveRoomMutationInput = {
  roomId: string;
  groupId: string;
};

export type ReorderRoomsMutationInput = {
  groupId: string;
  items: Array<{ type: RoomGroupItemType; id: string }>;
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

function isUniversalRoom(room: object): boolean {
  return 'isUniversal' in room && room.isUniversal === true;
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

function itemToMutationInput(item: AdminSidebarItem): { type: RoomGroupItemType; id: string } {
  return {
    type: item.kind === 'room' ? RoomGroupItemType.Room : RoomGroupItemType.SidebarLink,
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
        items: orderedRoomIds.map((id) => ({ type: RoomGroupItemType.Room, id }))
      });
    }
  }

  return { moves, linkMoves: [], reorders };
}

export function planSidebarItemMutations(before: GroupItemOrder, after: GroupItemOrder): RoomMovePlan {
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

function adminItemsFromQuery(
  group: {
    rooms?: Array<{ id: string }> | null;
    items?: Array<{
      type: RoomGroupItemType;
      id: string;
      room?: { id: string } | null;
      link?: { id: string; label: string; url: string } | null;
    }> | null;
  },
  roomsMap: SvelteMap<string, AdminRoomInfo>
): AdminSidebarItem[] {
  if (!group.items || group.items.length === 0) {
    return (group.rooms ?? [])
      .map((room) => roomsMap.get(room.id))
      .filter((room): room is AdminRoomInfo => room != null)
      .map((room) => ({ id: `room:${room.id}`, kind: 'room', room }));
  }
  return group.items
    .map((item): AdminSidebarItem | null => {
      if (item.type === RoomGroupItemType.Room && item.room) {
        const room = roomsMap.get(item.room.id);
        return room ? { id: `room:${room.id}`, kind: 'room', room } : null;
      }
      if (item.type === RoomGroupItemType.SidebarLink && item.link) {
        return {
          id: `link:${item.link.id}`,
          kind: 'link',
          link: {
            id: item.link.id,
            label: item.link.label,
            url: item.link.url
          }
        };
      }
      return null;
    })
    .filter((item): item is AdminSidebarItem => item != null);
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
      const initialResult = await this.client
        .query(AdminRoomGroupsQuery, {}, { requestPolicy: 'network-only' })
        .toPromise();
      const result =
        initialResult.error && isUnsupportedGraphQLFieldError(initialResult.error, 'isUniversal')
          ? await this.client
              .query(AdminRoomGroupsCompatibilityQuery, {}, { requestPolicy: 'network-only' })
              .toPromise()
          : initialResult;
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
            archived: room.archived,
            isUniversal: isUniversalRoom(room)
          }
        ])
      );

      this.groups = normalizeGroups(
        server.roomGroups.map((group) => {
          const items = adminItemsFromQuery(group, roomsMap);
          return {
            id: group.id,
            name: group.name,
            rooms: items.filter((item) => item.kind === 'room').map((item) => item.room),
            items
          };
        })
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
    const group = { id: created.id, name: created.name, rooms: [], items: [] };
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

  async createSidebarLink(
    groupId: string,
    label: string,
    url: string
  ): Promise<StoreResult<{ link: AdminSidebarLinkInfo }>> {
    const result = await this.client
      .mutation(CreateSidebarLinkMutation, { input: { groupId, label, url } })
      .toPromise();

    if (result.error || !result.data?.createSidebarLink) {
      return { ok: false, error: errorMessage(result.error) };
    }

    const link = result.data.createSidebarLink;
    this.markMutation();
    await this.refresh();
    return { ok: true, link };
  }

  async updateSidebarLink(
    linkId: string,
    label: string,
    url: string
  ): Promise<StoreResult<{ link: AdminSidebarLinkInfo }>> {
    const result = await this.client
      .mutation(UpdateSidebarLinkMutation, { input: { linkId, label, url } })
      .toPromise();

    if (result.error || !result.data?.updateSidebarLink) {
      return { ok: false, error: errorMessage(result.error) };
    }

    const link = result.data.updateSidebarLink;
    this.markMutation();
    await this.refresh();
    return { ok: true, link };
  }

  async deleteSidebarLink(linkId: string): Promise<StoreResult> {
    const result = await this.client
      .mutation(DeleteSidebarLinkMutation, { input: { linkId } })
      .toPromise();

    if (result.error) {
      return { ok: false, error: errorMessage(result.error) };
    }

    this.markMutation();
    await this.refresh();
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

  async setRoomUniversal(roomId: string, isUniversal: boolean): Promise<StoreResult> {
    this.universalRoomId = roomId;
    try {
      const result = await this.client
        .mutation(SetRoomUniversalMutation, { input: { roomId, isUniversal } })
        .toPromise();

      if (result.error) {
        return { ok: false, error: errorMessage(result.error) };
      }

      this.markMutation();
      await this.refresh();
      return { ok: true };
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

    const plan = planSidebarItemMutations(before, buildGroupItemOrder(this.groups));
    if (plan.moves.length === 0 && plan.linkMoves.length === 0 && plan.reorders.length === 0) {
      return null;
    }

    const errors: string[] = [];
    for (const move of plan.moves) {
      const result = await this.client
        .mutation(MoveRoomToGroupMutation, { input: move })
        .toPromise();
      if (result.error) {
        errors.push(`Failed to move room: ${errorMessage(result.error)}`);
      }
    }

    for (const move of plan.linkMoves) {
      const result = await this.client
        .mutation(MoveSidebarLinkToGroupMutation, {
          input: { linkId: move.roomId, groupId: move.groupId }
        })
        .toPromise();
      if (result.error) {
        errors.push(`Failed to move link: ${errorMessage(result.error)}`);
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

  ingestServerEvent(serverEvent: { event?: { __typename?: string } | null }): boolean {
    const event = serverEvent.event;
    if (!event) return false;
    if (event.__typename === 'RoomGroupsUpdatedEvent') {
      return this.ingestRoomLayoutUpdated();
    }
    if (
      event.__typename === 'RoomUpdatedEvent' ||
      event.__typename === 'RoomArchivedEvent' ||
      event.__typename === 'RoomUnarchivedEvent' ||
      event.__typename === 'RoomUniversalChangedEvent'
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
