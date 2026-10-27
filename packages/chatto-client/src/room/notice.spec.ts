import { describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomNotice } from '@chatto/api-types/api/v1/rooms_pb';
import type { ServerConnection } from '../server/serverConnection.js';
import type { RoomNoticesAPI } from '../api/roomNotices.js';
import { RoomNoticeStore } from './notice.js';

function fixture() {
  const api = {
    get: vi.fn<RoomNoticesAPI['get']>(),
    publish: vi.fn<RoomNoticesAPI['publish']>(),
    clear: vi.fn<RoomNoticesAPI['clear']>()
  };
  const store = new RoomNoticeStore({ getAPI: () => api } as unknown as ServerConnection, 'R1');
  return { api, store };
}

describe('room notice state', () => {
  it('preserves content and the draft epoch while reconnect refreshes retained state', async () => {
    const { api, store } = fixture();
    const initial = new RoomNotice({ body: 'Initial', version: 'v1' });
    api.get.mockResolvedValue(initial);
    const release = store.retain();
    await vi.waitFor(() => expect(store.notice).toBe(initial));
    const epoch = store.accessEpoch;
    let finish!: (notice: RoomNotice) => void;
    api.get.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    store.reset();
    expect(store.notice).toBe(initial);
    expect(store.accessEpoch).toBe(epoch);
    expect(api.get).toHaveBeenCalledTimes(2);
    const refreshed = new RoomNotice({ body: 'Current', version: 'v2' });
    finish(refreshed);
    await vi.waitFor(() => expect(store.notice).toBe(refreshed));
    expect(store.accessEpoch).toBe(epoch);
    release();
    store.dispose();
  });

  it('does not restore revoked access during a reconnect reset', async () => {
    const { api, store } = fixture();
    api.get.mockResolvedValue(new RoomNotice({ body: 'Private', version: 'v1' }));
    const release = store.retain();
    await vi.waitFor(() => expect(store.notice?.version).toBe('v1'));
    store.reset(true);
    const epoch = store.accessEpoch;
    store.reset();
    await store.refresh();
    expect(store.notice).toBeNull();
    expect(store.accessEpoch).toBe(epoch);
    expect(api.get).toHaveBeenCalledTimes(1);
    await expect(store.publish('Blocked', 'v1', false)).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
    await expect(store.clear('v1')).rejects.toMatchObject({ code: Code.PermissionDenied });
    store.restoreAccess();
    await vi.waitFor(() => expect(store.notice?.version).toBe('v1'));
    expect(api.get).toHaveBeenCalledTimes(2);
    release();
    store.dispose();
  });

  it('keeps a completed live read fenced until a delayed write response arrives', async () => {
    const { api, store } = fixture();
    api.get.mockResolvedValueOnce(new RoomNotice({ body: 'Initial', version: 'v1' }));
    const release = store.retain();
    await vi.waitFor(() => expect(store.notice?.version).toBe('v1'));
    let finishWrite!: (notice: RoomNotice) => void;
    api.publish.mockImplementation(
      () =>
        new Promise((resolve) => {
          finishWrite = resolve;
        })
    );
    const write = store.publish('My update', 'v1', false);
    const newer = new RoomNotice({ body: 'Later update', version: 'v3' });
    api.get.mockResolvedValue(newer);
    store.invalidate('later-event-cursor');
    await vi.waitFor(() => expect(store.notice).toBe(newer));
    finishWrite(new RoomNotice({ body: 'My update', version: 'v2' }));
    await write;
    await vi.waitFor(() => expect(store.notice).toBe(newer));
    expect(api.get).toHaveBeenLastCalledWith(
      'R1',
      expect.objectContaining({ minimumCursor: 'later-event-cursor' })
    );
    release();
    store.dispose();
  });
  it('preserves a pending live cursor when applying a write response', async () => {
    const { api, store } = fixture();
    api.get.mockResolvedValueOnce(new RoomNotice({ body: 'Initial', version: 'v1' }));
    const release = store.retain();
    await vi.waitFor(() => expect(store.notice?.version).toBe('v1'));
    api.get.mockImplementation(() => new Promise(() => {}));
    store.invalidate('later-event-cursor');
    const response = new RoomNotice({ body: 'Updated', version: 'v2' });
    api.publish.mockResolvedValue(response);
    await store.publish('Updated', 'v1', false);
    expect(store.notice).toBe(response);
    expect(api.get).toHaveBeenCalledTimes(3);
    expect(api.get).toHaveBeenLastCalledWith(
      'R1',
      expect.objectContaining({ minimumCursor: 'later-event-cursor' })
    );
    release();
    store.dispose();
  });
  it.each(['publish', 'clear'] as const)(
    'applies the %s response and fences an older in-flight read',
    async (operation) => {
      const { api, store } = fixture();
      let finishRead!: (notice: RoomNotice) => void;
      api.get.mockImplementation(
        () =>
          new Promise((resolve) => {
            finishRead = resolve;
          })
      );
      const read = store.refresh();
      const result = new RoomNotice({
        body: operation === 'clear' ? '' : 'Updated',
        version: 'v2'
      });
      api.publish.mockResolvedValue(result);
      api.clear.mockResolvedValue(result);
      if (operation === 'publish') await store.publish('Updated', 'v1', false);
      else await store.clear('v1');
      expect(store.notice).toBe(result);
      expect(api.get).toHaveBeenCalledTimes(1);
      finishRead(new RoomNotice({ body: 'Old replica', version: 'v1' }));
      await read;
      expect(store.notice).toBe(result);
      store.dispose();
    }
  );

  it.each(['publish', 'clear'] as const)(
    'does not restore a %s response after access loss and restoration',
    async (operation) => {
      const { api, store } = fixture();
      let finish!: (notice: RoomNotice) => void;
      const response = new Promise<RoomNotice>((resolve) => {
        finish = resolve;
      });
      api.publish.mockReturnValue(response);
      api.clear.mockReturnValue(response);
      const write =
        operation === 'publish' ? store.publish('Private', 'v1', false) : store.clear('v1');
      store.reset(true);
      store.restoreAccess();
      finish(new RoomNotice({ body: 'Private', version: 'v2' }));
      await write;
      expect(store.notice).toBeNull();
      store.dispose();
    }
  );
  it('fences reads that finish after membership loss and refreshes on access restoration', async () => {
    const { api, store } = fixture();
    let finish!: (value: RoomNotice) => void;
    api.get.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const release = store.retain();
    store.reset(true);
    finish(new RoomNotice({ body: 'Private', version: 'v1' }));
    await Promise.resolve();
    expect(store.notice).toBeNull();
    api.get.mockResolvedValue(new RoomNotice({ body: 'Restored', version: 'v2' }));
    store.restoreAccess();
    await vi.waitFor(() => expect(store.notice?.body).toBe('Restored'));
    release();
    store.dispose();
  });
  it('uses the event cursor when refreshing and does not restore an older read', async () => {
    const { api, store } = fixture();
    let finish!: (value: RoomNotice) => void;
    api.get.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const release = store.retain();
    const first = finish;
    api.get.mockResolvedValue(new RoomNotice({ body: 'Current', version: 'v2' }));
    store.invalidate('cursor');
    await vi.waitFor(() => expect(store.notice?.version).toBe('v2'));
    first(new RoomNotice({ body: 'Old', version: 'v1' }));
    await Promise.resolve();
    expect(store.notice?.body).toBe('Current');
    expect(api.get).toHaveBeenLastCalledWith(
      'R1',
      expect.objectContaining({ minimumCursor: 'cursor' })
    );
    release();
    store.dispose();
  });
  it('handles older servers and refuses commands after access loss', async () => {
    const { api, store } = fixture();
    api.get.mockRejectedValue(new ConnectError('Unavailable', Code.Unimplemented));
    await store.refresh();
    expect(store.supported).toBe(false);
    store.reset(true);
    await expect(store.publish('body', '', false)).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
    expect(api.publish).not.toHaveBeenCalled();
  });
  it('discards content and invalidates editor drafts when a read denies access', async () => {
    const { api, store } = fixture();
    api.get.mockResolvedValue(new RoomNotice({ body: 'Private', version: 'v1' }));
    await store.refresh();
    const epoch = store.accessEpoch;
    api.get.mockRejectedValue(new ConnectError('Denied', Code.PermissionDenied));
    await store.refresh();
    expect(store.notice).toBeNull();
    expect(store.accessEpoch).toBe(epoch + 1);
    await expect(store.clear('v1')).rejects.toMatchObject({ code: Code.PermissionDenied });
    expect(api.clear).not.toHaveBeenCalled();
  });
});
