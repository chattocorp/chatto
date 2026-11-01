import type { MessageAttachmentView } from '@chatto/client/timeline/messageAttachments';
import type { ExpiringAssetUrl } from '@chatto/client/attachments/attachmentUrls';

type RoomModalTarget = {
  serverId: string;
  roomId: string;
};

/** The complete set of shallow-routed global modals and their required payloads. */
export type ChatModal =
  | { type: 'aboutChatto' }
  /** Address lookup owned by the Server Directory route. */
  | { type: 'serverAddress' }
  /** Cached recommendation provenance; opening this dialog makes no requests. */
  | {
      type: 'serverRecommendation';
      name: string;
      origin: string;
      sources: Array<{ name: string; origin: string }>;
    }
  | { type: 'motd'; motd: string }
  /** Local SVG image snapshot; history can reopen it without the original preview. */
  | { type: 'renderedImageViewer'; src: string; alt: string }
  /** Rendered by the active invitation-code table that opened it. */
  | { type: 'createInvitationCode'; serverId: string; creatorId: string }
  /** Rendered by the active room's notice panel so access loss also removes its draft. */
  | (RoomModalTarget & { type: 'roomNoticeEditor'; editorId: string })
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
    });

export type LeaveRoomModalState = Extract<ChatModal, { type: 'leaveRoom' }>;
export type RemoveServerModalState = Extract<ChatModal, { type: 'removeServer' }>;
export type DeleteMessageContentModalState = Extract<
  ChatModal,
  { type: 'deleteMessage' | 'deleteAttachment' | 'deleteLinkPreview' }
>;
export type EditAttachmentDescriptionModalState = Extract<
  ChatModal,
  { type: 'editAttachmentDescription' }
>;
export type HtmlViewerModalState = Extract<ChatModal, { type: 'htmlViewer' }>;
export type RenderedImageViewerModalState = Extract<ChatModal, { type: 'renderedImageViewer' }>;

/** One opening of the shared file viewer, including an optional image gallery. */
export type AttachmentViewerModalState = Extract<ChatModal, { type: 'attachmentViewer' }>;
