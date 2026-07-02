import { authHeaders, createChattoClient, handleAuthError } from "./connect.js";
import type { LinkPreviewInput, RoomEventView } from "./renderTypes.js";
import { MessageService } from "@chatto/api-types/api/v1/messages_connect";
import {
  MessageAttachmentUpload,
  MessageLinkPreviewInput,
} from "@chatto/api-types/api/v1/messages_pb";
import { roomTimelineEventToRawEvent } from "./roomTimeline.js";

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
  mentionConfirmationToken?: string | null;
  linkPreview?: LinkPreviewInput | null;
};

export type UpdateMessageInput = {
  roomId: string;
  eventId: string;
  body?: string;
  alsoSendToChannel?: boolean;
};

export type CreateMessageResult =
  | {
      kind: "event";
      event: RoomEventView | null;
    }
  | {
      kind: "mentionConfirmation";
      recipientCount: number;
      token: string;
    };

export type UpdateMessageResult = {
  updated: boolean;
  event: RoomEventView | null;
};

export function createMessageAPI(config: MessageAPIConfig) {
  const client = createChattoClient(MessageService, config);
  const headers = () => authHeaders(config);
  return {
    async createMessage(
      input: CreateMessageInput,
    ): Promise<CreateMessageResult> {
      try {
        const response = await client.createMessage(
          {
            roomId: input.roomId,
            body: input.body,
            attachmentAssetIds: input.attachmentAssetIds ?? [],
            attachments: await messageAttachmentUploads(input.attachments),
            threadRootEventId: input.threadRootEventId ?? "",
            inReplyTo: input.inReplyTo ?? "",
            alsoSendToChannel: input.alsoSendToChannel ?? false,
            mentionConfirmationToken: input.mentionConfirmationToken ?? "",
            linkPreview: messageLinkPreviewInput(input.linkPreview),
          },
          { headers: headers() },
        );

        if (response.result.case === "mentionConfirmation") {
          return {
            kind: "mentionConfirmation",
            recipientCount: response.result.value.recipientCount,
            token: response.result.value.token,
          };
        }

        if (response.result.case === "event") {
          return {
            kind: "event",
            event: roomTimelineEventToRawEvent(
              response.result.value,
              response.includes?.users ?? {},
            ) as RoomEventView | null,
          };
        }

        return { kind: "event", event: null };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async updateMessage(
      input: UpdateMessageInput,
    ): Promise<UpdateMessageResult> {
      try {
        const request: {
          roomId: string;
          eventId: string;
          body?: string;
          alsoSendToChannel?: boolean;
        } = {
          roomId: input.roomId,
          eventId: input.eventId,
        };
        if (input.body !== undefined) {
          request.body = input.body;
        }
        if (input.alsoSendToChannel !== undefined) {
          request.alsoSendToChannel = input.alsoSendToChannel;
        }
        const response = await client.updateMessage(request, {
          headers: headers(),
        });
        return {
          updated: response.updated,
          event: response.event
            ? (roomTimelineEventToRawEvent(
                response.event,
                response.includes?.users ?? {},
              ) as RoomEventView | null)
            : null,
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async deleteMessage(roomId: string, eventId: string): Promise<boolean> {
      try {
        const response = await client.deleteMessage(
          { roomId, eventId },
          { headers: headers() },
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async deleteAttachment(
      roomId: string,
      eventId: string,
      attachmentId: string,
    ): Promise<boolean> {
      try {
        const response = await client.deleteAttachment(
          { roomId, eventId, attachmentId },
          { headers: headers() },
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async deleteLinkPreview(
      roomId: string,
      eventId: string,
      url: string,
    ): Promise<boolean> {
      try {
        const response = await client.deleteLinkPreview(
          { roomId, eventId, url },
          { headers: headers() },
        );
        return response.deleted;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
  };
}

async function messageAttachmentUploads(files: File[] | null | undefined) {
  if (!files?.length) return [];
  return Promise.all(
    files.map(async (file) => {
      const buffer = await file.arrayBuffer();
      return new MessageAttachmentUpload({
        content: new Uint8Array(buffer),
        filename: file.name,
        contentType: file.type || "application/octet-stream",
      });
    }),
  );
}

function messageLinkPreviewInput(input: LinkPreviewInput | null | undefined) {
  if (!input) return undefined;
  return new MessageLinkPreviewInput({
    url: input.url,
    title: input.title ?? undefined,
    description: input.description ?? undefined,
    siteName: input.siteName ?? undefined,
    imageAssetId: input.imageAssetId ?? undefined,
    embedType: input.embedType ?? undefined,
    embedId: input.embedId ?? undefined,
  });
}
