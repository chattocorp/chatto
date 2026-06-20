import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';
import {
  ClientRoomEventsPageSchema,
  ClientRoomEventsRequestSchema,
  ClientRoomEventItemSchema
} from '$lib/pb/chatto/core/v1/client_live_pb';
import {
  LiveMessagePostedEventSchema,
  LiveRoomEventSchema
} from '$lib/pb/chatto/core/v1/live_events_pb';
import { ClientLiveMessageHistoryClient } from './liveHistory';

function messageItem({
  id = 'E1',
  roomId = 'R1',
  body = 'hello from protobuf',
  sequence = 42n
}: {
  id?: string;
  roomId?: string;
  body?: string;
  sequence?: bigint;
} = {}) {
  return create(ClientRoomEventItemSchema, {
    streamSequence: sequence,
    event: create(LiveRoomEventSchema, {
      id,
      actorId: 'U1',
      event: {
        case: 'messagePosted',
        value: create(LiveMessagePostedEventSchema, {
          roomId,
          body,
          replyCount: 3,
          viewerIsFollowingThread: true
        })
      }
    })
  });
}

describe('ClientLiveMessageHistoryClient', () => {
  it('sends room pagination requests with raw protobuf sequence cursors', async () => {
    const request = vi.fn(async (_type: string, _payload: Uint8Array) =>
      Promise.resolve(
        toBinary(
          ClientRoomEventsPageSchema,
          create(ClientRoomEventsPageSchema, {
            events: [messageItem()],
            startCursorSeq: 40n,
            endCursorSeq: 42n,
            hasOlder: true,
            hasNewer: false
          })
        )
      )
    );
    const client = new ClientLiveMessageHistoryClient(request);

    const page = await client.before('R1', 25, 'seq:99');

    expect(request).toHaveBeenCalledTimes(1);
    const firstCall = request.mock.calls[0];
    expect(firstCall).toBeDefined();
    expect(firstCall![0]).toBe('room.events');
    const wireRequest = fromBinary(ClientRoomEventsRequestSchema, firstCall![1]);
    expect(wireRequest.roomId).toBe('R1');
    expect(wireRequest.limit).toBe(25);
    expect(wireRequest.beforeSeq).toBe(99n);
    expect(wireRequest.afterSeq).toBe(0n);
    expect(page.startCursor).toBe('seq:40');
    expect(page.endCursor).toBe('seq:42');
    expect(page.hasOlder).toBe(true);
    expect(page.hasNewer).toBe(false);
  });

  it('decodes hydrated LiveRoomEvent rows into renderable room event envelopes', async () => {
    const request = vi.fn(async (_type: string, _payload: Uint8Array) =>
      Promise.resolve(
        toBinary(
          ClientRoomEventsPageSchema,
          create(ClientRoomEventsPageSchema, {
            events: [messageItem({ body: 'rich body', sequence: 7n })],
            startCursorSeq: 7n,
            endCursorSeq: 7n
          })
        )
      )
    );
    const client = new ClientLiveMessageHistoryClient(request);

    const page = await client.latest('R1', 50);
    const event = page.events[0] as {
      id: string;
      actorId: string | null;
      event?: {
        __typename?: string;
        body?: string | null;
        replyCount?: number;
        viewerIsFollowingThread?: boolean | null;
      };
    };

    expect(event.id).toBe('E1');
    expect(event.actorId).toBe('U1');
    expect(event.event?.__typename).toBe('MessagePostedEvent');
    expect(event.event?.body).toBe('rich body');
    expect(event.event?.replyCount).toBe(3);
    expect(event.event?.viewerIsFollowingThread).toBe(true);
  });
});
