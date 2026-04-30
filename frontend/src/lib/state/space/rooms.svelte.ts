import { createContext, untrack } from 'svelte';
import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { NotificationLevelStore } from '$lib/state/instance/notificationLevel.svelte';
import type { RoomUnreadStore } from '$lib/state/instance/roomUnread.svelte';

export type SpaceRoom = {
  id: string;
  name: string;
  hasUnread: boolean;
  hasMention: boolean;
};

export type SpaceLayoutSection = {
  id: string;
  name: string;
  roomIds: string[];
};

const SpaceRoomsQuery = graphql(`
  query GetMyRoomsInSpace($spaceId: ID!) {
    me {
      rooms(spaceId: $spaceId) {
        id
        name
        hasUnread
        hasMention
        archived
        viewerNotificationPreference {
          level
          effectiveLevel
        }
      }
    }
    space(id: $spaceId) {
      roomLayout {
        sections {
          id
          name
          rooms {
            id
          }
        }
        unsectionedRoomIds
      }
    }
  }
`);

/**
 * Reactive store for a space's joined-room list, layout, and per-room
 * unread/mention state. One instance per `<SpaceEventProvider>`; consumers
 * (RoomList sidebar, the `/[spaceId]` redirect page, etc.) read from the same
 * source instead of each running their own `me.rooms(spaceId)` query.
 *
 * Per-room flag mutations (markRead, setMention, ...) are exposed as methods
 * so components can react to local UI events (entering a room) and to other
 * subscriptions (mentions, marked-as-read across tabs).
 *
 * Subscription events are forwarded by the component via {@link ingestSpaceEvent};
 * the store decides whether a refresh is warranted.
 */
export class SpaceRoomsStore {
  rooms = $state<SpaceRoom[]>([]);
  layoutSections = $state<SpaceLayoutSection[] | null>(null);
  unsectionedRoomIds = $state<string[]>([]);
  isInitialLoading = $state(true);

  private loadId = 0;

  constructor(
    private readonly client: Client,
    private readonly spaceId: string,
    private readonly notificationLevels: NotificationLevelStore,
    private readonly roomUnread: RoomUnreadStore
  ) {
    void this.refresh();
  }

  // -------------------------------------------------------------------------
  // Loading
  // -------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const result = await this.client.query(SpaceRoomsQuery, { spaceId: this.spaceId }).toPromise();
    if (this.loadId !== thisLoad) return;

    if (result.data?.me) {
      const allRooms = result.data.me.rooms;

      for (const room of allRooms) {
        const pref = room.viewerNotificationPreference;
        if (pref) {
          this.notificationLevels.setRoomPreference(this.spaceId, room.id, pref.level, pref.effectiveLevel);
        }
      }

      const visible = allRooms.filter((r) => !r.archived);
      this.rooms = visible.map((r) => ({
        id: r.id,
        name: r.name,
        hasUnread: r.hasUnread,
        hasMention: r.hasMention
      }));
      this.roomUnread.initSpaceRooms(this.spaceId, visible);
    }

    if (result.data?.space?.roomLayout) {
      this.layoutSections = result.data.space.roomLayout.sections.map((s) => ({
        id: s.id,
        name: s.name,
        roomIds: s.rooms.map((r) => r.id)
      }));
      this.unsectionedRoomIds = result.data.space.roomLayout.unsectionedRoomIds;
    } else {
      this.layoutSections = null;
      this.unsectionedRoomIds = [];
    }

    this.isInitialLoading = false;
  }

  // -------------------------------------------------------------------------
  // Per-room flag mutations
  // -------------------------------------------------------------------------

  markRead(roomId: string): void {
    this.patchRoom(roomId, { hasUnread: false, hasMention: false });
  }

  setUnread(roomId: string): void {
    this.patchRoom(roomId, { hasUnread: true });
  }

  setMention(roomId: string): void {
    this.patchRoom(roomId, { hasMention: true });
  }

  clearMention(roomId: string): void {
    this.patchRoom(roomId, { hasMention: false });
  }

  private patchRoom(roomId: string, patch: Partial<SpaceRoom>): void {
    // Wrapped in untrack so callers can invoke from within a $effect without
    // creating a read+write loop on `rooms` (e.g. `$effect(() =>
    // store.markRead(activeRoomId))`). Reactivity for other consumers still
    // fires from the assignment.
    untrack(() => {
      const idx = this.rooms.findIndex((r) => r.id === roomId);
      if (idx === -1) return;
      this.rooms[idx] = { ...this.rooms[idx], ...patch };
    });
  }

  // -------------------------------------------------------------------------
  // Subscription event ingestion
  // -------------------------------------------------------------------------

  /**
   * Refresh the room list when membership or room metadata changes. Other
   * event types (messages, reactions, presence) are no-ops at this level.
   */
  ingestSpaceEvent(spaceEvent: RoomEventViewFragment): void {
    const event = spaceEvent.event;
    if (!event) return;
    if (
      event.__typename === 'UserJoinedRoomEvent' ||
      event.__typename === 'UserLeftRoomEvent' ||
      event.__typename === 'RoomUpdatedEvent' ||
      event.__typename === 'RoomArchivedEvent' ||
      event.__typename === 'RoomUnarchivedEvent'
    ) {
      void this.refresh();
    }
  }
}

export const [getSpaceRoomsStore, setSpaceRoomsStore] = createContext<SpaceRoomsStore>();
