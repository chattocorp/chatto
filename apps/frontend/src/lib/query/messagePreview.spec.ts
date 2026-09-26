import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { QueryObserver } from '@tanstack/svelte-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { refreshRegisteredMessagePreviews } from './cacheRegistry';
import { queryClient } from './client';
import {
  messagePreviewQueryKey,
  messagePreviewReadCursor,
  type MessagePreview
} from './messagePreview';

const connection = { queryScope: 'message-preview-test' };

function preview(body: string): MessagePreview {
  return {
    body,
    attachments: [],
    actor: {
      id: 'author-1',
      login: 'author-1',
      displayName: 'Author',
      deleted: false,
      presenceStatus: PresenceStatus.OFFLINE
    }
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => (resolve = resolvePromise));
  return { promise, resolve };
}

afterEach(() => {
  queryClient.clear();
});

describe('message preview snapshot refresh', () => {
  it('reloads mounted previews of the server and keeps them visible meanwhile', async () => {
    const key = messagePreviewQueryKey('server-1', connection, 'room-1', 'event-1');
    const otherServerKey = messagePreviewQueryKey('server-2', connection, 'room-1', 'event-1');
    const reload = deferred<MessagePreview>();
    const responses = [Promise.resolve(preview('before gap')), reload.promise];
    const observer = new QueryObserver(queryClient, {
      queryKey: key,
      queryFn: () => responses.shift()!
    });
    const unsubscribe = observer.subscribe(() => {});
    queryClient.setQueryData(otherServerKey, preview('other server'));
    await vi.waitFor(() => expect(observer.getCurrentResult().data).toEqual(preview('before gap')));

    refreshRegisteredMessagePreviews('server-1', 'snapshot-cursor');
    await vi.waitFor(() => expect(observer.getCurrentResult().isFetching).toBe(true));

    expect(observer.getCurrentResult().data).toEqual(preview('before gap'));
    expect(queryClient.getQueryState(otherServerKey)?.isInvalidated).toBe(false);
    reload.resolve(preview('after gap'));
    await vi.waitFor(() => expect(observer.getCurrentResult().data).toEqual(preview('after gap')));
    unsubscribe();
  });

  it('does not let a read that started before the snapshot answer the refresh', async () => {
    const key = messagePreviewQueryKey('server-1', connection, 'room-1', 'event-1');
    const early = deferred<MessagePreview>();
    const responses = [early.promise, Promise.resolve(preview('after gap'))];
    const observer = new QueryObserver(queryClient, {
      queryKey: key,
      queryFn: () => responses.shift()!
    });
    const unsubscribe = observer.subscribe(() => {});
    await vi.waitFor(() => expect(observer.getCurrentResult().isFetching).toBe(true));

    refreshRegisteredMessagePreviews('server-1', 'snapshot-cursor');
    early.resolve(preview('before gap'));

    await vi.waitFor(() => expect(observer.getCurrentResult().data).toEqual(preview('after gap')));
    unsubscribe();
  });

  it('reads at the snapshot cursor until the stream accepts a newer cursor', () => {
    refreshRegisteredMessagePreviews('server-3', 'snapshot-cursor');

    expect(messagePreviewReadCursor('server-3', null)).toBe('snapshot-cursor');
    expect(messagePreviewReadCursor('server-3', 'accepted-cursor')).toBe('accepted-cursor');
    expect(messagePreviewReadCursor('server-4', null)).toBeUndefined();
  });
});
