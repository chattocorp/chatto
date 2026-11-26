/**
 * View logic for the server Overview page: the setup reminder for
 * administrators and the rooms that the "Catch up" card lists.
 */

import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { RoomsListItem } from '$lib/state/server/navigation';

/** A public profile field that a server without it shows as a setup step. */
export type ProfileSetupItem = 'logo' | 'banner' | 'description' | 'welcome_message';

/** Return the public profile fields that are empty, in the order of the settings form. */
export function missingProfileItems(
  profile: Pick<ServerPublicProfile, 'logoUrl' | 'bannerUrl' | 'description' | 'welcomeMessage'>
): ProfileSetupItem[] {
  const missing: ProfileSetupItem[] = [];
  if (!profile.logoUrl) missing.push('logo');
  if (!profile.bannerUrl) missing.push('banner');
  if (!profile.description?.trim()) missing.push('description');
  if (!profile.welcomeMessage?.trim()) missing.push('welcome_message');
  return missing;
}

/** A joined room that the "Catch up" card lists. */
export type CatchUpRoom = {
  id: string;
  name: string;
  /** Important notification occurrences that need attention. */
  importantCount: number;
  /** All other notification occurrences that need attention. */
  otherCount: number;
};

/** The maximum number of rooms in the "Catch up" card. */
export const CATCH_UP_LIMIT = 5;

/**
 * Select the joined channel rooms with unread messages or notifications.
 * Rooms with important notifications come first, then rooms with other
 * notifications, then unread rooms. Direct messages stay in the sidebar.
 */
export function catchUpRooms(
  rooms: readonly RoomsListItem[],
  isUnread: (roomId: string) => boolean,
  limit = CATCH_UP_LIMIT
): CatchUpRoom[] {
  return rooms
    .filter(
      (room) =>
        room.type !== RoomKind.DM &&
        room.viewerIsMember === true &&
        (room.viewerNotificationCount > 0 || isUnread(room.id))
    )
    .map((room) => ({
      id: room.id,
      name: room.name,
      importantCount: room.viewerImportantNotificationCount,
      otherCount: Math.max(0, room.viewerNotificationCount - room.viewerImportantNotificationCount)
    }))
    .sort(
      (a, b) =>
        b.importantCount - a.importantCount ||
        b.otherCount - a.otherCount ||
        a.name.localeCompare(b.name)
    )
    .slice(0, limit);
}

/** How long a room counts as new after it was created. */
export const NEW_ROOM_DAYS = 7;

/** Whether a room was created within the last {@link NEW_ROOM_DAYS} days. */
export function isNewRoom(createdAt: string | null | undefined, now = Date.now()): boolean {
  if (!createdAt) return false;
  const created = Date.parse(createdAt);
  if (Number.isNaN(created)) return false;
  return now - created < NEW_ROOM_DAYS * 24 * 60 * 60 * 1000;
}

/** The maximum number of conversations in the "Direct messages" card. */
export const RECENT_DIRECT_MESSAGES_LIMIT = 5;

/**
 * Select the viewer's direct messages with messages, newest activity first.
 * Conversations without a known last message keep their list order at the end.
 */
export function recentDirectMessages(
  rooms: readonly RoomsListItem[],
  limit = RECENT_DIRECT_MESSAGES_LIMIT
): RoomsListItem[] {
  const activity = (room: RoomsListItem) =>
    room.lastMessageAt ? Date.parse(room.lastMessageAt) : Number.NEGATIVE_INFINITY;
  return rooms
    .filter(
      (room) =>
        room.type === RoomKind.DM &&
        room.viewerIsMember !== false &&
        room.hasMessageHistory !== false
    )
    .sort((a, b) => activity(b) - activity(a))
    .slice(0, limit);
}
