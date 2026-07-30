/** One image shown by the history-backed attachment viewer. */
export type ImageViewerItem = {
  id?: string;
  src: string;
  originalSrc?: string;
  alt?: string;
  filename?: string;
};

/** The complete set of shallow-routed global modals and their required payloads. */
export type ChatModal =
  | { type: 'logout' }
  | { type: 'aboutChatto' }
  | { type: 'leaveRoom'; roomId: string; roomName: string }
  | { type: 'removeServer'; serverId: string; spaceName: string }
  | { type: 'deleteMessage'; roomId: string; eventId: string }
  | { type: 'deleteAttachment'; roomId: string; eventId: string; attachmentId: string }
  | { type: 'deleteLinkPreview'; roomId: string; eventId: string; previewUrl: string }
  | {
      type: 'imageViewer';
      roomId: string;
      eventId: string;
      imageItems: ImageViewerItem[];
      imageIndex: number;
    };

export type LeaveRoomModalState = Extract<ChatModal, { type: 'leaveRoom' }>;
export type RemoveServerModalState = Extract<ChatModal, { type: 'removeServer' }>;
export type DeleteMessageModalState = Extract<ChatModal, { type: 'deleteMessage' }>;
export type DeleteAttachmentModalState = Extract<ChatModal, { type: 'deleteAttachment' }>;
export type DeleteLinkPreviewModalState = Extract<ChatModal, { type: 'deleteLinkPreview' }>;
export type ImageViewerModalState = Extract<ChatModal, { type: 'imageViewer' }>;

/** Identifies one modal interaction while allowing its render data to refresh in place. */
export function chatModalKey(modal: ChatModal): ChatModal | string {
  return modal.type === 'imageViewer'
    ? JSON.stringify([modal.type, modal.roomId, modal.eventId])
    : modal;
}
