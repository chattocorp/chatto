import { describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { RoomType } from '$lib/chatTypes';
import {
  ListMyRoomsResponse,
  RoomListItemView,
  ViewerNotificationPreferenceView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { Room, RoomGroup, RoomKind, User } from '$lib/pb/chatto/core/v1/models_pb';
import { NotificationLevel as WireNotificationLevel } from '$lib/pb/chatto/core/v1/user_preferences_pb';
import { NotificationLevel } from '$lib/preferences/notificationLevel';
import type { WireClient } from '$lib/wire/client';
import { NotificationLevelStore } from './notificationLevel.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { isRoomStateRefreshEvent, RoomsStore } from './rooms.svelte';

type WireClientStub = Pick<WireClient, 'listMyRooms'>;

type RoomViewOverrides = {
  name?: string;
  kind?: RoomKind;
  archived?: boolean;
  hasUnread?: boolean;
  level?: WireNotificationLevel;
  effectiveLevel?: WireNotificationLevel;
  members?: User[];
};

function makeRoomView(id: string, overrides: RoomViewOverrides = {}): RoomListItemView {
  return new RoomListItemView({
    room: new Room({
      id,
      name: overrides.name ?? id,
      kind: overrides.kind ?? RoomKind.CHANNEL,
      archived: overrides.archived ?? false
    }),
    hasUnread: overrides.hasUnread ?? false,
    viewerNotificationPreference: new ViewerNotificationPreferenceView({
      level: overrides.level ?? WireNotificationLevel.UNSPECIFIED,
      effectiveLevel: overrides.effectiveLevel ?? WireNotificationLevel.NORMAL
    }),
    members: overrides.members ?? [
      new User({
        id: 'U1',
        login: 'alice',
        displayName: 'Alice'
      })
    ]
  });
}

function makeResponse(
  roomViews: RoomListItemView[],
  roomGroups: RoomGroup[] = []
): ListMyRoomsResponse {
  return new ListMyRoomsResponse({
    viewerUserId: 'U1',
    roomViews,
    roomGroups
  });
}

function makeStore(
  client: WireClientStub,
  notificationLevels = new NotificationLevelStore(),
  roomUnread = new RoomUnreadStore()
) {
  return {
    store: new RoomsStore('server_1', notificationLevels, roomUnread, () => client as WireClient),
    notificationLevels,
    roomUnread
  };
}

function makeClient(responses: ListMyRoomsResponse[]) {
  const queue = [...responses];
  const listMyRooms = vi.fn<WireClientStub['listMyRooms']>(() =>
    Promise.resolve(queue.shift() ?? new ListMyRoomsResponse())
  );
  return { client: { listMyRooms } as WireClientStub, listMyRooms };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('RoomsStore - refresh', () => {
  it('loads room rows, groups, unread state, and notification preferences from wire responses', async () => {
    const roomUnread = new RoomUnreadStore();
    const notificationLevels = new NotificationLevelStore();
    const { client, listMyRooms } = makeClient([
      makeResponse(
        [
          makeRoomView('channel_1', {
            name: 'General',
            hasUnread: true,
            level: WireNotificationLevel.MUTED,
            effectiveLevel: WireNotificationLevel.MUTED
          }),
          makeRoomView('archived_channel', { archived: true })
        ],
        [new RoomGroup({ id: 'g1', name: 'Lobby', roomIds: ['channel_1', 'channel_1'] })]
      ),
      makeResponse([
        makeRoomView('dm_1', {
          name: '',
          kind: RoomKind.DM,
          members: [
            new User({ id: 'U1', login: 'alice', displayName: 'Alice' }),
            new User({ id: 'U2', login: 'bob', displayName: 'Bob' })
          ]
        })
      ])
    ]);
    const { store } = makeStore(client, notificationLevels, roomUnread);

    await store.refresh();

    expect(listMyRooms).toHaveBeenCalledTimes(2);
    expect(listMyRooms.mock.calls.map(([request]) => request?.kind)).toEqual([
      RoomKind.CHANNEL,
      RoomKind.DM
    ]);
    expect(store.currentUserId).toBe('U1');
    expect(
      store.rooms.map((room) => ({ id: room.id, type: room.type, hasUnread: room.hasUnread }))
    ).toEqual([
      { id: 'channel_1', type: RoomType.Channel, hasUnread: true },
      { id: 'dm_1', type: RoomType.Dm, hasUnread: false }
    ]);
    expect(
      store.rooms.find((room) => room.id === 'dm_1')?.members.map((member) => member.id)
    ).toEqual(['U1', 'U2']);
    expect(store.roomGroups).toEqual([{ id: 'g1', name: 'Lobby', roomIds: ['channel_1'] }]);
    expect(roomUnread.roomIsUnread('channel_1')).toBe(true);
    expect(notificationLevels.getRoomPreference('channel_1')).toEqual({
      level: NotificationLevel.Muted,
      effectiveLevel: NotificationLevel.Muted
    });
    expect(store.isInitialLoading).toBe(false);
  });

  it('discards out-of-order responses', async () => {
    const deferredCalls = [
      deferred<ListMyRoomsResponse>(),
      deferred<ListMyRoomsResponse>(),
      deferred<ListMyRoomsResponse>(),
      deferred<ListMyRoomsResponse>()
    ];
    const queue = [...deferredCalls];
    const listMyRooms = vi.fn<WireClientStub['listMyRooms']>(() => {
      const next = queue.shift();
      if (!next) return Promise.resolve(new ListMyRoomsResponse());
      return next.promise;
    });
    const { store } = makeStore({ listMyRooms } as WireClientStub);

    const firstRefresh = store.refresh();
    const secondRefresh = store.refresh();

    deferredCalls[2].resolve(
      makeResponse(
        [makeRoomView('newer')],
        [new RoomGroup({ id: 'g1', name: 'Lobby', roomIds: ['newer'] })]
      )
    );
    deferredCalls[3].resolve(makeResponse([]));
    await secondRefresh;
    await settle();

    expect(store.rooms.map((room) => room.id)).toEqual(['newer']);
    expect(store.roomGroups).toEqual([{ id: 'g1', name: 'Lobby', roomIds: ['newer'] }]);

    deferredCalls[0].resolve(
      makeResponse(
        [makeRoomView('older')],
        [new RoomGroup({ id: 'g1', name: 'Lobby', roomIds: ['older'] })]
      )
    );
    deferredCalls[1].resolve(makeResponse([]));
    await firstRefresh;
    await settle();

    expect(store.rooms.map((room) => room.id)).toEqual(['newer']);
    expect(store.roomGroups).toEqual([{ id: 'g1', name: 'Lobby', roomIds: ['newer'] }]);
  });
});

describe('RoomsStore - ingestServerEvent', () => {
  function makeEvent(typename: string) {
    return { event: { __typename: typename } };
  }

  it('uses one shared predicate for room state refresh events', () => {
    expect(isRoomStateRefreshEvent('RoomCreatedEvent')).toBe(true);
    expect(isRoomStateRefreshEvent('RoomGroupsUpdatedEvent')).toBe(true);
    expect(isRoomStateRefreshEvent('ReactionAddedEvent')).toBe(false);
  });

  it('refreshes on RoomCreatedEvent', () => {
    const { client } = makeClient([]);
    const { store } = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('RoomCreatedEvent'));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes on RoomGroupsUpdatedEvent', () => {
    const { client } = makeClient([]);
    const { store } = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('RoomGroupsUpdatedEvent'));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes on UserJoinedRoomEvent', () => {
    const { client } = makeClient([]);
    const { store } = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('UserJoinedRoomEvent'));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('does not refresh on irrelevant event types', () => {
    const { client } = makeClient([]);
    const { store } = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('ReactionAddedEvent'));
    store.ingestServerEvent(makeEvent('HeartbeatEvent'));

    expect(store.refresh).not.toHaveBeenCalled();
  });
});
