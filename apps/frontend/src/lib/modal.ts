/** One image shown by the history-backed attachment viewer. */
export type ImageViewerItem = {
  id?: string;
  src: string;
  originalSrc?: string;
  alt?: string;
  filename?: string;
};

type RoomModalTarget = {
  serverId: string;
  roomId: string;
};

/** The complete set of shallow-routed global modals and their required payloads. */
export type ChatModal =
  | { type: 'logout' }
  | { type: 'aboutChatto' }
  | (RoomModalTarget & { type: 'leaveRoom'; roomName: string })
  | { type: 'removeServer'; serverId: string; spaceName: string }
  | (RoomModalTarget & { type: 'deleteMessage'; eventId: string })
  | (RoomModalTarget & { type: 'deleteAttachment'; eventId: string; attachmentId: string })
  | (RoomModalTarget & { type: 'deleteLinkPreview'; eventId: string; previewUrl: string })
  | (RoomModalTarget & {
      type: 'imageViewer';
      eventId: string;
      imageItems: ImageViewerItem[];
      imageIndex: number;
    });

export type LeaveRoomModalState = Extract<ChatModal, { type: 'leaveRoom' }>;
export type RemoveServerModalState = Extract<ChatModal, { type: 'removeServer' }>;
export type DeleteMessageContentModalState = Extract<
  ChatModal,
  { type: 'deleteMessage' | 'deleteAttachment' | 'deleteLinkPreview' }
>;
export type ImageViewerModalState = Extract<ChatModal, { type: 'imageViewer' }>;

/** Identifies one modal interaction while allowing its render data to refresh in place. */
export function chatModalKey(modal: ChatModal): ChatModal | string {
  return modal.type === 'imageViewer'
    ? JSON.stringify([modal.type, modal.serverId, modal.roomId, modal.eventId])
    : modal;
}
