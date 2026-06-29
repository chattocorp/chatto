import {
  createRoomTimelineAPI as createBaseRoomTimelineAPI,
  roomTimelineEventToRawEvent as baseRoomTimelineEventToRawEvent,
  roomTimelinePageToEventConnectionPage as baseRoomTimelinePageToEventConnectionPage
} from '@chatto/api-client/roomTimeline';
import { withRoomTimelineHooks } from './clientHooks';
import type { RoomTimelineAPIConfig } from '@chatto/api-client/roomTimeline';
import type { RoomTimelineEvent, RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import type { User } from '@chatto/api-types/api/v1/users_pb';
import type { EventConnectionPage, RawEvent } from '$lib/state/room/messages/helpers';

export type { RoomTimelineAPIConfig };

export type RoomTimelineAPI = {
  getRoomEvents(input: {
    roomId: string;
    limit: number;
    before?: string;
    after?: string;
  }): Promise<EventConnectionPage>;
  getRoomEventsAround(input: {
    roomId: string;
    eventId: string;
    limit: number;
  }): Promise<EventConnectionPage>;
  resolveMessageLinkTarget(input: {
    roomId: string;
    eventId: string;
  }): Promise<{ event: RawEvent | null; threadRootEventId: string | null }>;
  getThreadEvents(input: {
    roomId: string;
    threadRootEventId: string;
    limit: number;
    before?: string;
    after?: string;
  }): Promise<EventConnectionPage>;
  getThreadEventsAround(input: {
    roomId: string;
    threadRootEventId: string;
    eventId: string;
    limit: number;
  }): Promise<EventConnectionPage>;
};

export function createRoomTimelineAPI(config: RoomTimelineAPIConfig): RoomTimelineAPI {
  return createBaseRoomTimelineAPI(withRoomTimelineHooks(config)) as unknown as RoomTimelineAPI;
}

export function roomTimelinePageToEventConnectionPage(
  page: RoomTimelinePage
): EventConnectionPage {
  return baseRoomTimelinePageToEventConnectionPage(page) as unknown as EventConnectionPage;
}

export function roomTimelineEventToRawEvent(
  event: RoomTimelineEvent,
  users: Record<string, User>
): RawEvent | null {
  return baseRoomTimelineEventToRawEvent(event, users) as unknown as RawEvent | null;
}
