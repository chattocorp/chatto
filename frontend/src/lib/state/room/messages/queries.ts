import { graphql } from '$lib/gql';

export const RoomLatestQuery = graphql(`
  query RoomMessagesLatest($roomId: ID!, $limit: Int) {
    room(roomId: $roomId) {
      events(limit: $limit) {
        events { ...RoomEventView }
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

export const RoomBeforeQuery = graphql(`
  query RoomMessagesBefore($roomId: ID!, $limit: Int, $before: String) {
    room(roomId: $roomId) {
      events(limit: $limit, before: $before) {
        events { ...RoomEventView }
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

export const RoomAfterQuery = graphql(`
  query RoomMessagesAfter($roomId: ID!, $limit: Int, $after: String) {
    room(roomId: $roomId) {
      events(limit: $limit, after: $after) {
        events { ...RoomEventView }
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

export const RoomAroundQuery = graphql(`
  query RoomMessagesAround($roomId: ID!, $eventId: ID!, $limit: Int) {
    room(roomId: $roomId) {
      eventsAround(eventId: $eventId, limit: $limit) {
        events { ...RoomEventView }
        targetIndex
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

export const RefetchOneQuery = graphql(`
  query RoomMessagesRefetchOne($roomId: ID!, $eventId: ID!) {
    room(roomId: $roomId) {
      event(eventId: $eventId) {
        ...RoomEventView
      }
    }
  }
`);

export const PAGE_SIZE = 50;
