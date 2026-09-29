import { describe, expect, it, vi } from 'vitest';
import { computed, effect, effectRoot } from '@chatto/client/reactivity';
import type { ServerStateStore } from '@chatto/client/server/store';
import type { ProjectionReset, RoomAccessLoss } from '@chatto/client/server/storeEvents';
import { ActiveCall, CallParticipant } from '@chatto/api-types/api/v1/voice_calls_pb';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import type { NotificationAPI } from '@chatto/client/api/notifications';
import { NotificationStore } from '@chatto/client/server/notifications';
import { ServerProjectionStore } from '@chatto/client/server/projection';
import { RoomListView } from '@chatto/client/server/rooms';
import { serverUi } from './serverUi';

type Listener<T extends unknown[] = []> = (...args: T) => unknown;

/** A server store with the boundary events that `ServerUi` subscribes to. */
function fakeStore() {
  const listeners = {
    reset: [] as Listener<[ProjectionReset]>[],
    roomAccessLost: [] as Listener<[RoomAccessLoss]>[],
    userDeleted: [] as Listener<[string]>[],
    permissionsChanged: [] as Listener[],
    update: [] as Listener<[unknown]>[],
    dispose: [] as Listener[]
  };
  const on =
    <T extends unknown[]>(list: Listener<T>[]) =>
    (listener: Listener<T>) => {
      list.push(listener);
      return () => {};
    };
  const projection = new ServerProjectionStore();
  const followed: { roomId: string; threadRootId: string }[] = [];
  const store = {
    serverId: 'server',
    isAuthenticated: true,
    projectionViewerId: 'U1',
    viewerId: 'U1',
    projection,
    notifications: new NotificationStore({} as NotificationAPI),
    roomList: new RoomListView(projection, { hasUsableProjection: true }),
    unreadFollowedThreadsInLoadedRooms: () => followed,
    connection: { getAPI: () => ({}) },
    onReset: on(listeners.reset),
    onRoomAccessLost: on(listeners.roomAccessLost),
    onUserDeleted: on(listeners.userDeleted),
    onPermissionsChanged: on(listeners.permissionsChanged),
    onUpdate: on(listeners.update),
    onDispose: on(listeners.dispose)
  } as unknown as ServerStateStore;
  const emit = {
    reset: (reset: ProjectionReset) => listeners.reset.forEach((listener) => listener(reset)),
    roomAccessLost: (loss: RoomAccessLoss) =>
      listeners.roomAccessLost.forEach((listener) => listener(loss)),
    userDeleted: (userId: string) => listeners.userDeleted.forEach((listener) => listener(userId)),
    permissionsChanged: () => listeners.permissionsChanged.map((listener) => listener()),
    update: (update: unknown) => listeners.update.forEach((listener) => listener(update)),
    dispose: () => listeners.dispose.forEach((listener) => listener())
  };
  return { store, emit, followed };
}

describe('serverUi', () => {
  it('keeps one state for each store', () => {
    const { store } = fakeStore();
    expect(serverUi(store)).toBe(serverUi(store));
    expect(serverUi(fakeStore().store)).not.toBe(serverUi(store));
  });

  it('retains separate transient search state for each room', () => {
    const ui = serverUi(fakeStore().store);
    const first = ui.roomSearch('R1');
    const second = ui.roomSearch('R2');
    first.query = 'first room only';

    expect(ui.roomSearch('R1')).toBe(first);
    expect(second).not.toBe(first);
    expect(second.query).toBe('');
    expect(ui.messageSearch.query).toBe('');
  });

  it('bounds retained room search plaintext', async () => {
    const ui = serverUi(fakeStore().store);
    const oldest = ui.roomSearch('R1');
    oldest.query = 'sensitive result scope';
    for (let index = 2; index <= 11; index++) ui.roomSearch(`R${index}`);

    await Promise.resolve();
    expect(oldest.query).toBe('');
    expect(ui.roomSearch('R1')).not.toBe(oldest);
  });

  it('can select the eleventh room search from a render derivation', async () => {
    const ui = serverUi(fakeStore().store);
    const oldest = ui.roomSearch('R1');
    oldest.query = 'old room query';
    for (let index = 2; index <= 10; index++) ui.roomSearch(`R${index}`);

    const closeRoute = effectRoot(() => {
      const selected = computed(() => ui.roomSearch('R11'));
      effect(() => {
        expect(selected.get().query).toBe('');
      });
    });
    await Promise.resolve();
    expect(oldest.query).toBe('');
    closeRoute();
  });

  it('does not clear a replacement search while cleaning up its evicted owner', async () => {
    const ui = serverUi(fakeStore().store);
    const oldest = ui.roomSearch('R1');
    oldest.query = 'old query';
    for (let index = 2; index <= 11; index++) ui.roomSearch(`R${index}`);
    const replacement = ui.roomSearch('R1');
    replacement.query = 'new query';

    await Promise.resolve();
    expect(oldest.query).toBe('');
    expect(replacement.query).toBe('new query');
  });

  it('clears copies at a reset that does not keep the view', () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    ui.pendingHighlights.set('R1', null, 'M1');
    const search = vi.spyOn(ui.messageSearch, 'clearResults');
    const layout = vi.spyOn(ui.adminRoomLayout, 'resetProjectionState');

    emit.reset({ privacy: false, retainView: true });
    expect(ui.pendingHighlights.has('R1', null)).toBe(true);
    expect(search).not.toHaveBeenCalled();

    emit.reset({ privacy: true, retainView: false });
    expect(ui.pendingHighlights.has('R1', null)).toBe(false);
    expect(search).toHaveBeenCalledOnce();
    expect(layout).toHaveBeenCalledOnce();
  });

  it('removes search plaintext of a room, a deleted author, and changed messages', () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    const room = ui.roomSearch('R1');
    const other = ui.roomSearch('R2');
    const revoke = [vi.spyOn(ui.messageSearch, 'revokeRoom'), vi.spyOn(room, 'revokeRoom')];
    const otherRevoke = vi.spyOn(other, 'revokeRoom');
    const author = vi.spyOn(other, 'invalidateAuthor');
    const invalidate = vi.spyOn(room, 'invalidateRoom');
    const clear = vi.spyOn(other, 'clearResults');

    emit.roomAccessLost({ roomId: 'R1', messagesOnly: true, removed: false });
    for (const spy of revoke) expect(spy).toHaveBeenCalledWith('R1');
    expect(otherRevoke).not.toHaveBeenCalled();

    emit.userDeleted('U2');
    expect(author).toHaveBeenCalledWith('U2');

    emit.update({ event: { event: { case: 'messageEdited', value: { roomId: 'R1' } } } });
    expect(invalidate).toHaveBeenCalledWith('R1');
    emit.update({ event: { event: { case: 'userJoinedRoom', value: { roomId: 'R2' } } } });
    expect(clear).not.toHaveBeenCalled();
    emit.update({ event: { event: { case: 'assetDeleted', value: {} } } });
    expect(clear).toHaveBeenCalledOnce();
  });

  it('rechecks search and an active layout editor when permissions change', async () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    const search = vi.spyOn(ui.messageSearch, 'refreshPermissions');
    const reset = vi.spyOn(ui.adminRoomLayout, 'resetProjectionState');
    const refresh = vi.spyOn(ui.adminRoomLayout, 'refreshPermissions').mockResolvedValue();
    vi.spyOn(ui.adminRoomLayout, 'refresh').mockResolvedValue();

    expect(emit.permissionsChanged()).toEqual([undefined]);
    expect(search).toHaveBeenCalledOnce();
    expect(reset).toHaveBeenCalledOnce();

    const release = ui.activateAdminRoomLayout();
    const [waiting] = emit.permissionsChanged();
    await expect(waiting).resolves.toBeUndefined();
    expect(refresh).toHaveBeenCalledOnce();
    release();
  });

  it('refreshes an active layout editor after room and group changes', () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    vi.spyOn(ui.adminRoomLayout, 'refresh').mockResolvedValue();
    const request = vi.spyOn(ui.adminRoomLayout, 'requestProjectionRefresh');
    const deactivate = vi.spyOn(ui.adminRoomLayout, 'deactivateProjectionRefresh');

    emit.update({ resource: { case: 'rooms' } });
    expect(request).not.toHaveBeenCalled();

    const release = ui.activateAdminRoomLayout();
    emit.update({ resource: { case: 'roomGroups' } });
    emit.update({ reset: true });
    emit.update({ resource: { case: 'viewer' } });
    expect(request).toHaveBeenCalledTimes(2);
    release();
    expect(deactivate).toHaveBeenCalledOnce();
  });

  it('releases every copy when its store is disposed', () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    const room = ui.roomSearch('R1');
    room.query = 'secret';
    ui.messageSearch.query = 'secret';
    ui.pendingHighlights.set('R1', null, 'M1');

    emit.dispose();
    expect(room.query).toBe('');
    expect(ui.messageSearch.query).toBe('');
    expect(ui.pendingHighlights.has('R1', null)).toBe(false);
  });

  it('keeps a call across a reset, rechecks it after room updates, and reads room permissions', () => {
    const { store, emit } = fakeStore();
    const call = serverUi(store).voiceCall;
    const forget = vi.spyOn(call, 'handleProjectionReset');
    const revoke = vi.spyOn(call, 'handleRoomAccessRevoked');
    const reconcile = vi.spyOn(call, 'reconcilePermissions').mockResolvedValue();

    emit.reset({ privacy: true, retainView: false });
    expect(forget).toHaveBeenCalledOnce();
    expect(revoke).not.toHaveBeenCalled();
    expect(reconcile).not.toHaveBeenCalled();
    expect(call.permissionsFor('R1').join).toBe(false);

    for (const join of [true, false]) {
      store.projection.rooms.set(
        'R1',
        new RoomWithViewerState({
          room: { id: 'R1' },
          viewerState: {
            isMember: true,
            permissions: [{ permission: 'call.join', granted: join }]
          }
        })
      );
      emit.update({ resource: { case: 'rooms' } });
      expect(call.permissionsFor('R1').join).toBe(join);
    }
    expect(reconcile).toHaveBeenCalledTimes(2);
  });

  it('leaves the call only when room access, not only message reading, is lost', () => {
    const { store, emit } = fakeStore();
    const revoke = vi.spyOn(serverUi(store).voiceCall, 'handleRoomAccessRevoked');
    emit.roomAccessLost({ roomId: 'R1', messagesOnly: true, removed: false });
    expect(revoke).not.toHaveBeenCalled();
    emit.roomAccessLost({ roomId: 'R1', messagesOnly: false, removed: false });
    expect(revoke).toHaveBeenCalledExactlyOnceWith('R1');
  });

  it('forwards participant and end events to the call', () => {
    const { store, emit } = fakeStore();
    const call = serverUi(store).voiceCall;
    const transition = vi.spyOn(call, 'handleParticipantTransition');
    const ended = vi.spyOn(call, 'handleCallEndedEvent');

    emit.update({
      event: {
        id: 'E-CALL-JOIN',
        actorId: 'U2',
        event: { case: 'voiceCallParticipantJoined', value: { roomId: 'R1', callId: 'CALL-1' } }
      }
    });
    expect(transition).toHaveBeenCalledExactlyOnceWith({
      eventId: 'E-CALL-JOIN',
      kind: 'join',
      roomId: 'R1',
      callId: 'CALL-1',
      actorId: 'U2',
      viewerId: 'U1'
    });

    emit.update({
      event: { id: 'E-END', event: { case: 'voiceCallEnded', value: { roomId: 'R1', callId: '' } } }
    });
    expect(ended).toHaveBeenCalledExactlyOnceWith('R1', null);
  });

  it('overlays the projected calls with its own call', () => {
    const { store } = fakeStore();
    const ui = serverUi(store);
    store.projection.activeCalls = [
      new ActiveCall({
        room: { id: 'R1' },
        callId: 'CALL-1',
        participants: [new CallParticipant({ user: { id: 'U2', login: 'bob' } })]
      })
    ];
    expect(ui.activeCallRooms.has('R1')).toBe(true);
    expect(ui.activeCallRooms.getParticipants('R1').map(({ userId }) => userId)).toEqual(['U2']);
    ui.voiceCall.connected = true;
    ui.voiceCall.roomId = 'R2';
    expect(ui.activeCallRooms.has('R2')).toBe(true);
  });

  it('releases call media when its store is disposed', () => {
    const { store, emit } = fakeStore();
    const dispose = vi.spyOn(serverUi(store).voiceCall, 'dispose');
    emit.dispose();
    expect(dispose).toHaveBeenCalledOnce();
  });

  it('confirms optimistic unread and membership state from projected rooms', () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    ui.roomUnread.setRoomUnread('R1', true);
    const acknowledge = vi.spyOn(ui.roomDirectory, 'acknowledgeMembership');
    const viewer = vi.spyOn(ui.roomUnread, 'acknowledgeViewerProjection');
    store.projection.rooms.set(
      'R1',
      new RoomWithViewerState({
        room: { id: 'R1' },
        viewerState: { isMember: true, hasUnread: false }
      })
    );

    emit.update({ resource: { case: 'rooms' } });
    expect(ui.roomUnread.roomIsUnread('R1')).toBe(false);
    expect(acknowledge).toHaveBeenCalledWith('R1', true);
    emit.update({ resource: { case: 'viewer' } });
    expect(viewer).toHaveBeenCalledOnce();
  });

  it('forgets optimistic state of a removed room and at a reset', () => {
    const { store, emit } = fakeStore();
    const ui = serverUi(store);
    ui.roomUnread.setRoomUnread('R1', true);
    const removeMembership = vi.spyOn(ui.roomDirectory, 'removeMembershipProjection');

    emit.roomAccessLost({ roomId: 'R1', messagesOnly: false, removed: false });
    expect(ui.roomUnread.roomIsUnread('R1')).toBe(true);
    emit.roomAccessLost({ roomId: 'R1', messagesOnly: false, removed: true });
    expect(ui.roomUnread.roomIsUnread('R1')).toBe(false);
    expect(removeMembership).toHaveBeenCalledWith('R1');

    ui.roomUnread.setRoomUnread('R2', true);
    emit.reset({ privacy: false, retainView: false });
    expect(ui.roomUnread.roomIsUnread('R2')).toBe(false);
  });

  it('shows notifications before unread rooms on the server icon', () => {
    const { store } = fakeStore();
    const ui = serverUi(store);
    expect(ui.serverIndicator()).toBeNull();
    ui.roomUnread.setRoomUnread('R1', true);
    expect(ui.serverIndicator()).toBe('unread');
    store.notifications.setUnreadNotificationCount(1);
    expect(ui.serverIndicator()).toBe('notification');
  });

  it('ignores an unread followed thread that the user reads now', () => {
    const { store, followed } = fakeStore();
    const ui = serverUi(store);
    followed.push({ roomId: 'R1', threadRootId: 'T1' });
    expect(ui.hasUnreadFollowedThreadInLoadedRooms()).toBe(true);
    const close = ui.readViews.register({ roomId: 'R1', threadRootId: 'T1' });
    expect(ui.hasUnreadFollowedThreadInLoadedRooms()).toBe(false);
    close();
  });
});
