import type { MessageAttachmentView } from '$lib/render/messageAttachments';
import type { ExpiringAssetUrl } from '$lib/attachments/attachmentUrls';

/** One image shown by the history-backed attachment viewer. */
export type ImageViewerItem = {
  id?: string;
  src: string;
  originalSrc?: string;
  alt?: string;
  filename?: string;
  description?: string;
};

type RoomModalTarget = {
  serverId: string;
  roomId: string;
};

/** The complete set of shallow-routed global modals and their required payloads. */
export type ChatModal =
  | { type: 'logout' }
  | { type: 'aboutChatto' }
  | { type: 'motd'; motd: string }
  | (RoomModalTarget & { type: 'leaveRoom'; roomName: string })
  | { type: 'removeServer'; serverId: string; spaceName: string }
  | (RoomModalTarget & { type: 'deleteMessage'; eventId: string })
  | (RoomModalTarget & { type: 'deleteAttachment'; eventId: string; attachmentId: string })
  | (RoomModalTarget & {
      type: 'editAttachmentDescription';
      eventId: string;
      attachmentId: string;
      description: string;
    })
  | (RoomModalTarget & { type: 'deleteLinkPreview'; eventId: string; previewUrl: string })
  | (RoomModalTarget & {
      type: 'attachmentViewer';
      eventId: string;
      items: MessageAttachmentView[];
      index: number;
    })
  | (RoomModalTarget & {
      type: 'htmlViewer';
      eventId: string;
      attachmentId: string;
      filename: string;
      contentType: string;
      assetUrl: ExpiringAssetUrl | null;
    })
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
export type EditAttachmentDescriptionModalState = Extract<
  ChatModal,
  { type: 'editAttachmentDescription' }
>;
export type HtmlViewerModalState = Extract<ChatModal, { type: 'htmlViewer' }>;

/** Identifies one modal interaction while allowing its render data to refresh in place. */
export function chatModalKey(modal: ChatModal): ChatModal | string {
  return modal.type === 'imageViewer'
    ? JSON.stringify([modal.type, modal.serverId, modal.roomId, modal.eventId])
    : modal;
}

/** One opening of the shared file viewer, including an optional image gallery. */
export type AttachmentViewerModalState = Extract<ChatModal, { type: 'attachmentViewer' }>;
