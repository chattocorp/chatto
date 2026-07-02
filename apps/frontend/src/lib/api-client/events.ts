import type { RoomEventView } from "./renderTypes.js";

export type RawEvent = RoomEventView;

export type EventConnectionPage = {
  events: readonly RawEvent[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export type UserSummaryForCache = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl: string | null;
};
