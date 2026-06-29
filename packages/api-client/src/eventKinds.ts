export const RoomEventKind = {
  MessagePosted: "messagePosted",
  RoomArchived: "roomArchived",
  RoomCreated: "roomCreated",
  RoomDeleted: "roomDeleted",
  RoomUnarchived: "roomUnarchived",
  RoomUpdated: "roomUpdated",
  UserJoinedRoom: "userJoinedRoom",
  UserLeftRoom: "userLeftRoom",
} as const;

export type RoomEventKind = (typeof RoomEventKind)[keyof typeof RoomEventKind];
