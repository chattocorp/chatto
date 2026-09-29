import { describe, expect, it, vi } from 'vitest';
import { computed, effect, effectRoot } from '@chatto/client/reactivity';
import type { ServerStateStore } from '@chatto/client/server/store';
import type { ProjectionReset, RoomAccessLoss } from '@chatto/client/server/storeEvents';
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
  const store = {
    isAuthenticated: true,
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
  return { store, emit };
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
});
