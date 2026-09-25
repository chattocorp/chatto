// SPDX-License-Identifier: Apache-2.0

import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import type { SavedView } from '$lib/storage/savedViews';

/** Build complete disk fixtures from readable room and message examples. */
export function savedViewFixture(input: {
  serverId: string;
  userId: string;
  serverName: string;
  viewerName?: string;
  savedAt: number;
  rooms: {
    id: string;
    name: string;
    kind?: number;
    messages: {
      id: string;
      createdAt: string;
      author: string;
      authorId?: string;
      body: string;
    }[];
  }[];
}): SavedView {
  return {
    ...input,
    version: 2,
    presentation: { server: JSON.stringify({ name: input.serverName }), roomGroups: [], users: [] },
    rooms: input.rooms.map(({ messages, ...room }) => ({
      ...room,
      resource: new RoomWithViewerState({
        room,
        viewerState: {
          isMember: true,
          permissions: [
            { permission: 'message.read', granted: true },
            { permission: 'message.read-interactions', granted: true },
            { permission: 'message.post', granted: true }
          ]
        }
      }).toJsonString(),
      events: messages.map((message) => ({
        id: message.id,
        createdAt: message.createdAt,
        actorId: message.authorId,
        event: {
          kind: 'messagePosted',
          roomId: room.id,
          body: message.body,
          attachments: [],
          reactions: [],
          replyCount: 0,
          threadParticipants: []
        }
      }))
    }))
  };
}
