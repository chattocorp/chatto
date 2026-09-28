/** What the viewer may do in one room. */
export type RoomPermissions = {
  canPostMessage: boolean;
  canPostInThread: boolean;
  canAttach: boolean;
  canReact: boolean;
  canManageOthersMessage: boolean;
  canEchoMessage: boolean;
  canManageRoom: boolean;
  canViewPinnedMessages: boolean;
  canPinMessages: boolean;
  canBanRoomMembers: boolean;
};

export const DEFAULT_ROOM_PERMISSIONS: RoomPermissions = {
  canPostMessage: false,
  canPostInThread: false,
  canAttach: false,
  canReact: false,
  canManageOthersMessage: false,
  canEchoMessage: false,
  canManageRoom: false,
  canViewPinnedMessages: false,
  canPinMessages: false,
  canBanRoomMembers: false
};
