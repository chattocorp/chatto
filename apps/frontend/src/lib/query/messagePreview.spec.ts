import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { afterEach, describe, expect, it } from 'vitest';
import {
  purgeRegisteredAuthorMessagePreviews,
  purgeRegisteredMessagePreview,
  purgeRegisteredRoomMessagePreviews,
  refreshRegisteredMessagePreview
} from './cacheRegistry';
import { queryClient } from './client';
import { messagePreviewQueryKey, type MessagePreview } from './messagePreview';

const serverId = 'server-1';
const connection = { queryScope: 'message-preview-test' };

function preview(body: string, actorId = 'author-1'): MessagePreview {
  return {
    body,
    attachments: [],
    actor: {
      id: actorId,
      login: actorId,
      displayName: actorId,
      deleted: false,
      presenceStatus: PresenceStatus.OFFLINE
    }
  };
}

function key(roomId: string, messageId: string) {
  return messagePreviewQueryKey(serverId, connection, roomId, messageId);
}

afterEach(() => {
  queryClient.clear();
});

describe('message preview query cache', () => {
  it('hides a retracted message preview and leaves other previews', () => {
    queryClient.setQueryData(key('room-1', 'event-1'), preview('retracted'));
    queryClient.setQueryData(key('room-1', 'event-2'), preview('kept'));

    purgeRegisteredMessagePreview(serverId, 'room-1', 'event-1');

    expect(queryClient.getQueryData(key('room-1', 'event-1'))).toBeNull();
    expect(queryClient.getQueryData(key('room-1', 'event-2'))).toEqual(preview('kept'));
  });

  it('does not let an in-flight read restore a purged preview', async () => {
    const queryKey = key('room-1', 'event-1');
    queryClient.setQueryData(queryKey, preview('before'));
    let resolveRead!: (value: MessagePreview) => void;
    const read = queryClient
      .fetchQuery({
        queryKey,
        queryFn: () => new Promise<MessagePreview>((resolve) => (resolveRead = resolve)),
        staleTime: 0
      })
      .catch(() => undefined);

    purgeRegisteredMessagePreview(serverId, 'room-1', 'event-1');
    resolveRead(preview('late response'));
    await read;
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(queryClient.getQueryData(queryKey)).toBeNull();
  });

  it('hides every preview from a room that lost message access', () => {
    queryClient.setQueryData(key('room-1', 'event-1'), preview('one'));
    queryClient.setQueryData(key('room-1', 'event-2'), preview('two'));
    queryClient.setQueryData(key('room-2', 'event-3'), preview('other room'));

    purgeRegisteredRoomMessagePreviews(serverId, 'room-1');

    expect(queryClient.getQueryData(key('room-1', 'event-1'))).toBeNull();
    expect(queryClient.getQueryData(key('room-1', 'event-2'))).toBeNull();
    expect(queryClient.getQueryData(key('room-2', 'event-3'))).toEqual(preview('other room'));
  });

  it('hides previews written by a deleted account', () => {
    queryClient.setQueryData(key('room-1', 'event-1'), preview('deleted author', 'author-1'));
    queryClient.setQueryData(key('room-1', 'event-2'), preview('other author', 'author-2'));

    purgeRegisteredAuthorMessagePreviews(serverId, 'author-1');

    expect(queryClient.getQueryData(key('room-1', 'event-1'))).toBeNull();
    expect(queryClient.getQueryData(key('room-1', 'event-2'))).toEqual(
      preview('other author', 'author-2')
    );
  });

  it('keeps an edited message preview visible while it reloads', () => {
    queryClient.setQueryData(key('room-1', 'event-1'), preview('before edit'));

    refreshRegisteredMessagePreview(serverId, 'room-1', 'event-1');

    const state = queryClient.getQueryState(key('room-1', 'event-1'));
    expect(state?.data).toEqual(preview('before edit'));
    expect(state?.isInvalidated).toBe(true);
  });
});
