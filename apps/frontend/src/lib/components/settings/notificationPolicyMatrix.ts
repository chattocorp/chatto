import { RoomKind } from '$lib/api-client/roomDirectory';
import {
  notificationPolicyScopeKey,
  type NotificationPolicyScope
} from '$lib/api-client/notifications';
import type { RoomsListGroup, RoomsListItem } from '$lib/state/server/rooms.svelte';

export type NotificationPolicyColumn = {
  key: string;
  label: string;
  displayLabel: string;
  kind: 'server' | 'roomGroup' | 'room';
  scope: NotificationPolicyScope;
};

/**
 * Build the policy axes from the current member-visible navigation layout.
 * A room match keeps its group for inheritance context, while a group match
 * keeps all of its current-member rooms.
 */
export function notificationPolicyColumns(
  serverLabel: string,
  groups: RoomsListGroup[],
  rooms: RoomsListItem[],
  filter: string
): NotificationPolicyColumn[] {
  const columns: NotificationPolicyColumn[] = [
    column({ kind: 'server' }, serverLabel, serverLabel)
  ];
  const query = filter.trim().toLocaleLowerCase();
  const memberChannels = rooms.filter(
    (room) => room.viewerIsMember && room.type === RoomKind.CHANNEL
  );
  const memberDMs = rooms.filter((room) => room.viewerIsMember && room.type === RoomKind.DM);
  const roomsByID = new Map(memberChannels.map((room) => [room.id, room]));
  const groupedRoomIDs = new Set<string>();

  for (const group of groups) {
    const children = group.roomIds.flatMap((roomID) => {
      const room = roomsByID.get(roomID);
      return room ? [room] : [];
    });
    for (const room of children) groupedRoomIDs.add(room.id);

    const groupMatches = matches(group.name, query);
    const matchedChildren = query ? children.filter((room) => matches(room.name, query)) : children;
    if (query && !groupMatches && matchedChildren.length === 0) continue;

    columns.push(column({ kind: 'roomGroup', id: group.id }, group.name, group.name));
    for (const room of groupMatches ? children : matchedChildren) {
      columns.push(column({ kind: 'room', id: room.id }, room.name, `#${room.name}`));
    }
  }

  for (const room of memberChannels) {
    if (groupedRoomIDs.has(room.id) || !matches(room.name, query)) continue;
    columns.push(column({ kind: 'room', id: room.id }, room.name, `#${room.name}`));
  }
  for (const room of memberDMs) {
    if (!matches(room.name, query)) continue;
    columns.push(column({ kind: 'room', id: room.id }, room.name, room.name));
  }

  return columns;
}

function matches(label: string, query: string): boolean {
  return query.length === 0 || label.toLocaleLowerCase().includes(query);
}

function column(
  scope: NotificationPolicyScope,
  label: string,
  displayLabel: string
): NotificationPolicyColumn {
  return {
    key: notificationPolicyScopeKey(scope),
    label,
    displayLabel,
    kind: scope.kind,
    scope
  };
}
