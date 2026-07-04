import { authHeaders, createChattoClient, handleAuthError } from './connect.js';
import type { LinkPreviewInput, RoomEventView } from './renderTypes.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { messageToRawEvent, timelineUsersForMessages } from './roomTimeline.js';
import { createAssetUploadAPI } from './assetUploads.js';

export type MessageAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type CreateMessageInput = {
  roomId: string;
  body: string;
  attachmentAssetIds?: string[];
  attachments?: File[] | null;
  threadRootEventId?: string | null;
  inReplyTo?: string | null;
  alsoSendToChannel?: boolean;
  linkPreview?: LinkPreviewInput | null;
};

export type UpdateMessageInput = {
  roomId: string;
  eventId: string;
  body?: string;
  alsoSendToChannel?: boolean;
};

export type CreateMessageResult = {
  event: RoomEventView | null;
};

export type UpdateMessageResult = {
  updated: boolean;
  event: RoomEventView | null;
};

export function createMessageAPI(config: MessageAPIConfig) {
  const client = createChattoClient(MessageService, config);
  const headers = () => authHeaders(config);
  return {
    async createMessage(input: CreateMessageInput): Promise<CreateMessageResult> {
      try {
        const uploadedAttachmentAssetIds = await uploadMessageAttachments(config, input);
        const response = await client.createMessage(
          {
            roomId: input.roomId,
            body: input.body,
            attachmentAssetIds: [
              ...(input.attachmentAssetIds ?? []),
              ...uploadedAttachmentAssetIds
            ],
            threadRootEventId: input.threadRootEventId ?? '',
            inReplyTo: input.inReplyTo ?? '',
            alsoSendToChannel: input.alsoSendToChannel ?? false,
            linkPreviewToken: input.linkPreview?.previewToken ?? ''
          },
          { headers: headers() }
        );

        const users = await timelineUsersForMessages(config, response.message ? [response.message] : []);
        return {
          event: response.message
            ? (messageToRawEvent(response.message, users) as RoomEventView | null)
            : null
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async updateMessage(input: UpdateMessageInput): Promise<UpdateMessageResult> {
      try {
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
        const response = await client.updateMessage(request, {
          headers: headers()
        });
        const users = await timelineUsersForMessages(config, response.message ? [response.message] : []);
        return {
          updated: response.updated,
          event: response.message
            ? (messageToRawEvent(response.message, users) as RoomEventView | null)
            : null
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async deleteMessage(roomId: string, eventId: string): Promise<boolean> {
      try {
        const response = await client.deleteMessage({ roomId, eventId }, { headers: headers() });
        return response.deleted;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async deleteAttachment(
      roomId: string,
      eventId: string,
      attachmentId: string
    ): Promise<boolean> {
      try {
        const response = await client.deleteAttachment(
          { roomId, eventId, attachmentId },
          { headers: headers() }
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async deleteLinkPreview(roomId: string, eventId: string, url: string): Promise<boolean> {
      try {
        const response = await client.deleteLinkPreview(
          { roomId, eventId, url },
          { headers: headers() }
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

async function uploadMessageAttachments(config: MessageAPIConfig, input: CreateMessageInput) {
  const files = input.attachments;
  if (!files?.length) return [];
  const uploads = createAssetUploadAPI(config);
  const assets = await Promise.all(
    files.map((file) =>
      uploads.uploadAttachment({
        roomId: input.roomId,
        file
      })
    )
  );
  return assets.map((asset) => asset.assetId);
}
