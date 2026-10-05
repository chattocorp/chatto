import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { createPinnedMessagesAPI } from '../pinnedMessages.js';
import { REALTIME_MINIMUM_CURSOR_HEADER } from '../connect.js';
import {
  fakeServer,
  mockService,
  receivedContext,
  receivedRequest
} from '../../testing/fakeServer.js';

const rooms = mockService(RoomService);

function pinnedAPI() {
  return createPinnedMessagesAPI(fakeServer((router) => router.service(RoomService, rooms)));
}

describe('createPinnedMessagesAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('lists pinned messages with their page and pin marker', async () => {
    rooms.listPinnedMessages.mockReturnValue({
      pinnedMessages: [{ message: { id: 'M1' } }],
      page: { totalCount: 3n, hasMore: true },
      latestPinMarker: 'marker-1'
    });

    await expect(pinnedAPI().list('R1', 1, 2)).resolves.toMatchObject({
      items: [{ message: { id: 'M1' } }],
      totalCount: 3,
      hasMore: true,
      latestPinMarker: 'marker-1'
    });
    expect(receivedRequest(rooms.listPinnedMessages)).toMatchObject({
      roomId: 'R1',
      page: { limit: 1, offset: 2 }
    });
    expect(
      receivedContext(rooms.listPinnedMessages)?.requestHeader.has(REALTIME_MINIMUM_CURSOR_HEADER)
    ).toBe(false);
  });

  it('waits for the realtime cursor and defaults a missing page', async () => {
    rooms.listPinnedMessages.mockReturnValue({ pinnedMessages: [] });

    await expect(pinnedAPI().list('R1', 10, 0, 'cursor-7')).resolves.toEqual({
      items: [],
      totalCount: 0,
      hasMore: false,
      latestPinMarker: ''
    });
    expect(
      receivedContext(rooms.listPinnedMessages)?.requestHeader.get(REALTIME_MINIMUM_CURSOR_HEADER)
    ).toBe('cursor-7');
  });

  it('pins and unpins a message', async () => {
    rooms.createPinnedMessage.mockReturnValueOnce({ pinnedMessage: { message: { id: 'M1' } } });
    rooms.createPinnedMessage.mockReturnValueOnce({});
    rooms.deletePinnedMessage.mockReturnValue({});
    const api = pinnedAPI();

    await expect(api.create('R1', 'E1')).resolves.toMatchObject({ message: { id: 'M1' } });
    await expect(api.create('R1', 'E1')).resolves.toBeNull();
    await api.remove('R1', 'E1');
    expect(receivedRequest(rooms.deletePinnedMessage)).toMatchObject({
      roomId: 'R1',
      messageEventId: 'E1'
    });
  });
});
