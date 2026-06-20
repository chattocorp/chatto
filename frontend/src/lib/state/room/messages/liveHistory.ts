import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import {
  ClientRoomEventRequestSchema,
  ClientRoomEventResponseSchema,
  ClientRoomEventsAroundPageSchema,
  ClientRoomEventsAroundRequestSchema,
  ClientRoomEventsPageSchema,
  ClientRoomEventsRequestSchema,
  ClientThreadEventsAroundRequestSchema,
  ClientThreadEventsRequestSchema
} from '$lib/pb/chatto/core/v1/client_live_pb';
import type { ClientRoomEventItem } from '$lib/pb/chatto/core/v1/client_live_pb';
import {
  liveRoomEventToEnvelope,
  type ClientLiveRequestFunction
} from '$lib/state/server/clientLive';
import type { EventConnectionPage, RawEvent } from './helpers';

export interface MessageHistoryClient {
  latest(roomId: string, limit: number): Promise<EventConnectionPage>;
  before(roomId: string, limit: number, before: string): Promise<EventConnectionPage>;
  after(roomId: string, limit: number, after: string): Promise<EventConnectionPage>;
  around(roomId: string, eventId: string, limit: number): Promise<EventConnectionPage | null>;
  single(roomId: string, eventId: string): Promise<RawEvent | null>;
  thread(
    roomId: string,
    threadRootEventId: string,
    limit: number,
    cursor?: { before?: string; after?: string }
  ): Promise<EventConnectionPage>;
  threadAround(
    roomId: string,
    threadRootEventId: string,
    anchorEventId: string,
    limit: number
  ): Promise<EventConnectionPage | null>;
}

export class ClientLiveMessageHistoryClient implements MessageHistoryClient {
  constructor(private readonly request: ClientLiveRequestFunction) {}

  async latest(roomId: string, limit: number): Promise<EventConnectionPage> {
    return this.roomEvents({ roomId, limit });
  }

  async before(roomId: string, limit: number, before: string): Promise<EventConnectionPage> {
    return this.roomEvents({ roomId, limit, beforeSeq: cursorToSeq(before) });
  }

  async after(roomId: string, limit: number, after: string): Promise<EventConnectionPage> {
    return this.roomEvents({ roomId, limit, afterSeq: cursorToSeq(after) });
  }

  async around(
    roomId: string,
    eventId: string,
    limit: number
  ): Promise<EventConnectionPage | null> {
    const response = await this.request(
      'room.eventsAround',
      toBinary(
        ClientRoomEventsAroundRequestSchema,
        create(ClientRoomEventsAroundRequestSchema, { roomId, eventId, limit })
      )
    );
    return pageFromWire(fromBinary(ClientRoomEventsAroundPageSchema, response));
  }

  async single(roomId: string, eventId: string): Promise<RawEvent | null> {
    const response = await this.request(
      'room.event',
      toBinary(
        ClientRoomEventRequestSchema,
        create(ClientRoomEventRequestSchema, { roomId, eventId })
      )
    );
    const decoded = fromBinary(ClientRoomEventResponseSchema, response);
    return decoded.event ? eventFromItem(decoded.event) : null;
  }

  async thread(
    roomId: string,
    threadRootEventId: string,
    limit: number,
    cursor: { before?: string; after?: string } = {}
  ): Promise<EventConnectionPage> {
    const response = await this.request(
      'thread.events',
      toBinary(
        ClientThreadEventsRequestSchema,
        create(ClientThreadEventsRequestSchema, {
          roomId,
          threadRootEventId,
          limit,
          beforeSeq: cursor.before ? cursorToSeq(cursor.before) : 0n,
          afterSeq: cursor.after ? cursorToSeq(cursor.after) : 0n
        })
      )
    );
    return pageFromWire(fromBinary(ClientRoomEventsPageSchema, response));
  }

  async threadAround(
    roomId: string,
    threadRootEventId: string,
    anchorEventId: string,
    limit: number
  ): Promise<EventConnectionPage | null> {
    const response = await this.request(
      'thread.eventsAround',
      toBinary(
        ClientThreadEventsAroundRequestSchema,
        create(ClientThreadEventsAroundRequestSchema, {
          roomId,
          threadRootEventId,
          anchorEventId,
          limit
        })
      )
    );
    return pageFromWire(fromBinary(ClientRoomEventsPageSchema, response));
  }

  private async roomEvents({
    roomId,
    limit,
    beforeSeq = 0n,
    afterSeq = 0n
  }: {
    roomId: string;
    limit: number;
    beforeSeq?: bigint;
    afterSeq?: bigint;
  }): Promise<EventConnectionPage> {
    const response = await this.request(
      'room.events',
      toBinary(
        ClientRoomEventsRequestSchema,
        create(ClientRoomEventsRequestSchema, { roomId, limit, beforeSeq, afterSeq })
      )
    );
    return pageFromWire(fromBinary(ClientRoomEventsPageSchema, response));
  }
}

function pageFromWire(page: {
  events: ClientRoomEventItem[];
  startCursorSeq: bigint;
  endCursorSeq: bigint;
  hasOlder: boolean;
  hasNewer: boolean;
}): EventConnectionPage {
  return {
    events: page.events.map(eventFromItem).filter((event): event is RawEvent => event !== null),
    startCursor: seqToCursor(page.startCursorSeq),
    endCursor: seqToCursor(page.endCursorSeq),
    hasOlder: page.hasOlder,
    hasNewer: page.hasNewer
  };
}

function eventFromItem(item: ClientRoomEventItem): RawEvent | null {
  if (!item.event) return null;
  return liveRoomEventToEnvelope(item.event) as RawEvent | null;
}

function cursorToSeq(cursor: string): bigint {
  if (!cursor.startsWith('seq:')) return 0n;
  try {
    return BigInt(cursor.slice(4));
  } catch {
    return 0n;
  }
}

function seqToCursor(seq: bigint): string | null {
  return seq > 0n ? `seq:${seq.toString()}` : null;
}
