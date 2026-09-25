// SPDX-License-Identifier: Apache-2.0

import { RoomWithViewerState, RoomGroup } from '@chatto/api-types/api/v1/room_directory_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { ServerRuntimeConfig } from '@chatto/api-types/api/v1/server_state_pb';
import { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { SavedView } from './savedViews';
import { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';

/** Decode atomically at the storage boundary. A damaged cache must not start a partial view. */
export function decodePresentation(view: SavedView) {
  const presentation = view.presentation;
  const viewer = presentation.viewer ? GetViewerResponse.fromJsonString(presentation.viewer) : null;
  if (viewer && viewer.user?.profile?.id !== view.userId)
    throw new Error('Snapshot viewer mismatch');
  return {
    server: ServerPublicProfile.fromJsonString(presentation.server),
    groups: presentation.roomGroups.map((value) => RoomGroup.fromJsonString(value)),
    users: presentation.users.map((value) => DirectoryMember.fromJsonString(value)),
    viewer,
    activeCalls: (presentation.activeCalls ?? []).map((value) => ActiveCall.fromJsonString(value)),
    notifications: presentation.notifications,
    runtime: presentation.runtime
      ? ServerRuntimeConfig.fromJsonString(presentation.runtime)
      : undefined,
    rooms: view.rooms.map((saved) => {
      const resource = RoomWithViewerState.fromJsonString(saved.resource);
      if (resource.room?.id !== saved.id) throw new Error('Snapshot room mismatch');
      return { saved, resource, events: saved.events };
    })
  };
}
