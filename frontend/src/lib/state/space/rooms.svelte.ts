import { untrack } from 'svelte';
import type { Client } from '@urql/svelte';
import { graphql, useFragment } from '$lib/gql';
import {
  RoomType,
  type RoomEventViewFragment,
  UserAvatarUserFragmentDoc,
  type UserAvatarUserFragment
} from '$lib/gql/graphql';
import type { NotificationLevelStore } from '$lib/state/server/notificationLevel.svelte';
import type { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';

export type RoomsListItem = {
  id: string;
  name: string;
  type: RoomType;
  hasUnread: boolean;
  hasMention: boolean;
  // Populated for DM rooms only — used to derive the display name in the sidebar.
  members: UserAvatarUserFragment[];
};

export type RoomsListGroup = {
  id: string;
  name: string;
  roomIds: string[];
};

const MyRoomsQuery = graphql(`
  query GetMyRoomsInSpace {
    viewer {
      user {
        id
        rooms {
          id
          name
          type
          hasUnread
          hasMention
          archived
          viewerNotificationPreference {
            level
            effectiveLevel
          }
          members {
            ...UserAvatarUser
          }
        }
      }
    }
    server {
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

/**
 * Reactive store for a server's joined-room list, layout, and per-room
 * unread/mention state. One instance per registered server, owned by
 * `ServerStateStore` — consumers (RoomList sidebar, the `/[serverId]` redirect
 * page, etc.) reach the active server's store via
 * `serverRegistry.getStore(activeServerId).rooms`, so the reactivity follows
 * the URL automatically when the user switches servers.
 *
 * Per-room flag mutations (markRead, setMention, ...) are exposed as methods
 * so components can react to local UI events (entering a room) and to other
 * subscriptions (mentions, marked-as-read across tabs).
 *
 * Subscription events are forwarded via {@link ingestServerEvent}; the
 * top-level `RoomsSync` component attaches a handler to every server's bus
 * so every server's store stays current regardless of which one is active.
 */
export class RoomsStore {
  rooms = $state<RoomsListItem[]>([]);
  roomGroups = $state<RoomsListGroup[] | null>(null);
  isInitialLoading = $state(true);
  // The viewer's user ID, captured from the same `viewer { user { id, rooms } }`
  // query that produced `rooms`. Use this in preference to a global auth
  // context when filtering self out of `room.members` — by construction it is
  // set whenever there are rooms (with members) to render, eliminating any
  // race with the auth context being briefly empty during route transitions.
  currentUserId = $state<string | null>(null);

  private loadId = 0;

  constructor(
    private readonly client: Client,
    private readonly notificationLevels: NotificationLevelStore,
    private readonly roomUnread: RoomUnreadStore
  ) {}

  // -------------------------------------------------------------------------
  // Loading
  // -------------------------------------------------------------------------

  async refresh(): Promise<void> {
    const thisLoad = ++this.loadId;
    const result = await this.client.query(MyRoomsQuery, {}).toPromise();
    if (this.loadId !== thisLoad) return;

    if (result.data?.viewer?.user) {
      this.currentUserId = result.data.viewer.user.id;
      const allRooms = result.data.viewer.user.rooms;

      for (const room of allRooms) {
        const pref = room.viewerNotificationPreference;
        if (pref) {
          this.notificationLevels.setRoomPreference(room.id, pref.level, pref.effectiveLevel);
        }
      }

      const visible = allRooms.filter((r: { archived: boolean }) => !r.archived);
      this.rooms = visible.map((r: typeof allRooms[number]) => ({
        id: r.id,
        name: r.name,
        type: r.type,
        hasUnread: r.hasUnread,
        hasMention: r.hasMention,
        members: r.members.map((m: typeof r.members[number]) => useFragment(UserAvatarUserFragmentDoc, m))
      }));
      this.roomUnread.initRooms(visible);
    }

    if (result.data?.server?.roomGroups) {
      type SetT = NonNullable<typeof result.data.server.roomGroups>[number];
      this.roomGroups = result.data.server.roomGroups.map((s: SetT) => ({
        id: s.id,
        name: s.name,
        roomIds: s.rooms.map((r: SetT['rooms'][number]) => r.id)
      }));
    } else {
      this.roomGroups = null;
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

  /**
   * Move a room to the front of the rooms array. RoomList renders DMs in
   * their store-array order, so this is what makes a freshly-active DM jump
   * to the top of the Direct Messages section. Channels render alphabetically
   * regardless of array order, so a bump is a no-op for them visually.
   */
  bumpRoom(roomId: string): void {
    untrack(() => {
      const idx = this.rooms.findIndex((r) => r.id === roomId);
      if (idx <= 0) return;
      const room = this.rooms[idx];
      this.rooms = [room, ...this.rooms.slice(0, idx), ...this.rooms.slice(idx + 1)];
    });
  }

  private patchRoom(roomId: string, patch: Partial<RoomsListItem>): void {
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
   * event types (messages, reactions, presence) are no-ops at this level
   * unless the message arrives for a room we don't yet know about — that's
   * how a freshly-created empty DM (filtered from ListDMConversations until
   * its first message lands) shows up in the sidebar without a manual reload.
   */
  ingestServerEvent(serverEvent: { event?: { __typename?: string; roomId?: string } | null }): void {
    const event = serverEvent.event;
    if (!event) return;
    if (
      event.__typename === 'UserJoinedRoomEvent' ||
      event.__typename === 'UserLeftRoomEvent' ||
      event.__typename === 'RoomUpdatedEvent' ||
      event.__typename === 'RoomArchivedEvent' ||
      event.__typename === 'RoomUnarchivedEvent'
    ) {
      void this.refresh();
      return;
    }
    if (event.__typename === 'MessagePostedEvent') {
      const roomId = event.roomId;
      if (roomId && !this.rooms.some((r) => r.id === roomId)) {
        void this.refresh();
      }
    }
  }
}

