import { updateMask } from './updateMask';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { messageToTimelineEvent, timelineUsersForMessages } from './roomTimeline.js';
import { createAssetUploadAPI } from './assetUploads.js';

export type CreateMessageInput = {
  roomId: string;
  body: string;
  attachmentAssetIds?: string[];
  attachments?: File[] | null;
  attachmentDescriptions?: AttachmentDescriptionInput[];
  threadRootEventId?: string | null;
  inReplyTo?: string | null;
  alsoSendToChannel?: boolean;
  createThread?: boolean;
  linkPreviewToken?: string | null;
  onAttachmentUploadUpdate?: (update: AttachmentUploadUpdate) => void;
};

export type AttachmentDescriptionInput =
  { file: File; description: string } | { assetId: string; description: string };

export type AttachmentUploadUpdate =
  | {
      file: File;
      phase: 'uploading';
      committedBytes: number;
      totalBytes: number;
    }
  | { file: File; phase: 'uploaded' }
  | { file: File; phase: 'failed' };

export type UpdateMessageInput = {
  roomId: string;
  eventId: string;
  body?: string;
  alsoSendToChannel?: boolean;
};

export type CreateMessageResult = {
  event: TimelineEventView | null;
};

export type UpdateMessageResult = {
  updated: boolean;
  event: TimelineEventView | null;
};

export function createMessageAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MessageService, config);
  return {
    async createMessage(input: CreateMessageInput): Promise<CreateMessageResult> {
      const uploadedAttachments = await uploadMessageAttachments(config, input);
      const uploadedAttachmentAssetIds = uploadedAttachments.map(({ assetId }) => assetId);
      const uploadedAssetIDByFile = new Map(
        uploadedAttachments.map(({ file, assetId }) => [file, assetId] as const)
      );
      const attachmentDescriptions = (input.attachmentDescriptions ?? []).flatMap((entry) => {
        const assetId = 'assetId' in entry ? entry.assetId : uploadedAssetIDByFile.get(entry.file);
        return assetId ? [{ assetId, description: entry.description.trim() }] : [];
      });
      const response = await client.createMessage({
        roomId: input.roomId,
        body: input.body,
        attachmentAssetIds: [...(input.attachmentAssetIds ?? []), ...uploadedAttachmentAssetIds],
        attachmentDescriptions,
        threadRootEventId: input.threadRootEventId ?? '',
        inReplyTo: input.inReplyTo ?? '',
        alsoSendToChannel: input.alsoSendToChannel ?? false,
        createThread: input.createThread ?? false,
        linkPreviewToken: input.linkPreviewToken ?? ''
      });

      const users = await timelineUsersForMessages(
        config,
        response.message ? [response.message] : []
      );
      return {
        event: response.message ? messageToTimelineEvent(response.message, users) : null
      };
    },

    async updateMessage(input: UpdateMessageInput): Promise<UpdateMessageResult> {
      const request: {
        roomId: string;
        eventId: string;
        body?: string;
        alsoSendToChannel?: boolean;
      } = {
        roomId: input.roomId,
        eventId: input.eventId
      };
      if (input.body !== undefined) {
        request.body = input.body;
      }
      if (input.alsoSendToChannel !== undefined) {
        request.alsoSendToChannel = input.alsoSendToChannel;
      }
      const response = await client.updateMessage({
        ...request,
        updateMask: updateMask(request, ['body', 'alsoSendToChannel'])
      });
      const users = await timelineUsersForMessages(
        config,
        response.message ? [response.message] : []
      );
      return {
        updated: true,
        event: response.message ? messageToTimelineEvent(response.message, users) : null
      };
    },

    async deleteMessage(roomId: string, eventId: string): Promise<boolean> {
      await client.deleteMessage({ roomId, eventId });
      return true;
    },

    async deleteAttachment(
      roomId: string,
      eventId: string,
      attachmentId: string
    ): Promise<boolean> {
      await client.deleteAttachment({ roomId, eventId, attachmentId });
      return true;
    },

    async setAttachmentDescription(
      roomId: string,
      eventId: string,
      attachmentId: string,
      description: string
    ): Promise<UpdateMessageResult> {
      const response = await client.setAttachmentDescription({
        roomId,
        eventId,
        attachmentId,
        description: description.trim()
      });
      const users = await timelineUsersForMessages(
        config,
        response.message ? [response.message] : []
      );
      return {
        updated: true,
        event: response.message ? messageToTimelineEvent(response.message, users) : null
      };
    },

    async deleteLinkPreview(roomId: string, eventId: string, url: string): Promise<boolean> {
      await client.deleteLinkPreview({ roomId, eventId, url });
      return true;
    }
  };
}

async function uploadMessageAttachments(config: ConnectAPIConfig, input: CreateMessageInput) {
  const files = input.attachments;
  if (!files?.length) return [];
  const uploads = createAssetUploadAPI(config);
  const results = await Promise.allSettled(
    files.map(async (file) => {
      try {
        const asset = await uploads.uploadAttachment({
          roomId: input.roomId,
          file,
          onProgress: (committedBytes, totalBytes) => {
            input.onAttachmentUploadUpdate?.({
              file,
              phase: 'uploading',
              committedBytes,
              totalBytes
            });
          }
        });
        input.onAttachmentUploadUpdate?.({ file, phase: 'uploaded' });
        return { file, assetId: asset.assetId };
      } catch (error) {
        input.onAttachmentUploadUpdate?.({ file, phase: 'failed' });
        throw error;
      }
    })
  );
  const failed = results.find((result) => result.status === 'rejected');
  if (failed) throw failed.reason;
  return results.map((result) => {
    if (result.status === 'rejected') throw result.reason;
    return result.value;
  });
}
