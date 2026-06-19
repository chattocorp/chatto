import {
  AddReactionRequest,
  BanRoomMemberRequest,
  CreateRoomRequest,
  DeleteAttachmentRequest,
  DeleteLinkPreviewRequest,
  DeleteMessageRequest,
  FollowThreadRequest,
  JoinGroupRequest,
  JoinRoomRequest,
  LeaveRoomRequest,
  type LinkPreviewInput,
  MarkRoomAsReadRequest,
  MarkThreadAsReadRequest,
  PostMessageRequest,
  RemoveReactionRequest,
  SendTypingIndicatorRequest,
  StartDMRequest,
  UnfollowThreadRequest,
  UpdateMessageRequest
} from '$lib/pb/chatto/api/v1/chat_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
import { WireProtocolError } from './client';

interface WirePostMessageInput {
  roomId: string;
  body: string;
  threadRootEventId?: string | null;
  inReplyToEventId?: string | null;
  alsoSendToChannel?: boolean | null;
  largeMentionConfirmed?: boolean | null;
  attachmentAssetIds?: string[] | null;
  videoProcessingAssetIds?: string[] | null;
  linkPreview?: LinkPreviewInput | null;
  mentionConfirmationToken?: string | null;
}

interface WireUpdateMessageInput {
  roomId: string;
  eventId: string;
  body: string;
  alsoSendToChannel?: boolean | null;
}

interface WireMessageInput {
  roomId: string;
  eventId: string;
}

interface WireDeleteAttachmentInput extends WireMessageInput {
  attachmentId: string;
}

interface WireDeleteLinkPreviewInput extends WireMessageInput {
  url: string;
}

interface WireRoomInput {
  roomId: string;
}

interface WireGroupInput {
  groupId: string;
}

interface WireBanRoomMemberInput {
  roomId: string;
  userId: string;
  reason: string;
  expiresAt?: string | null;
}

interface WireStartDMInput {
  participantIds: string[];
}

interface WireCreateRoomInput {
  name: string;
  description?: string | null;
  groupId: string;
}

interface WireReactionInput {
  roomId: string;
  messageEventId: string;
  emoji: string;
}

interface WireTypingIndicatorInput {
  roomId: string;
  threadRootEventId?: string | null;
}

interface WireThreadInput {
  roomId: string;
  threadRootEventId: string;
}

interface WireMarkThreadAsReadInput extends WireThreadInput {
  upToEventId?: string | null;
}

interface WireMarkRoomAsReadInput {
  roomId: string;
  upToEventId?: string | null;
}

export type WireMentionConfirmation = {
  recipientCount: number;
  token: string;
};

export async function tryWirePostMessage(input: WirePostMessageInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.postMessage(
    new PostMessageRequest({
      roomId: input.roomId,
      body: input.body,
      threadRootEventId: input.threadRootEventId ?? '',
      inReplyToEventId: input.inReplyToEventId ?? '',
      alsoSendToChannel: input.alsoSendToChannel ?? false,
      largeMentionConfirmed: input.largeMentionConfirmed ?? false,
      attachmentAssetIds: input.attachmentAssetIds ?? [],
      videoProcessingAssetIds: input.videoProcessingAssetIds ?? [],
      linkPreview: input.linkPreview ?? undefined,
      mentionConfirmationToken: input.mentionConfirmationToken ?? ''
    })
  );
  return true;
}

export async function tryWireUpdateMessage(input: WireUpdateMessageInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.updateMessage(
    new UpdateMessageRequest({
      roomId: input.roomId,
      eventId: input.eventId,
      body: input.body,
      ...(input.alsoSendToChannel === null || input.alsoSendToChannel === undefined
        ? {}
        : { alsoSendToChannel: input.alsoSendToChannel })
    })
  );
  return true;
}

export async function tryWireDeleteMessage(input: WireMessageInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.deleteMessage(new DeleteMessageRequest(input));
  return true;
}

export async function tryWireDeleteAttachment(input: WireDeleteAttachmentInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.deleteAttachment(new DeleteAttachmentRequest(input));
  return true;
}

export async function tryWireDeleteLinkPreview(input: WireDeleteLinkPreviewInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.deleteLinkPreview(new DeleteLinkPreviewRequest(input));
  return true;
}

export async function tryWireJoinRoom(input: WireRoomInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.joinRoom(new JoinRoomRequest(input));
  return true;
}

export async function tryWireLeaveRoom(input: WireRoomInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.leaveRoom(new LeaveRoomRequest(input));
  return true;
}

export async function tryWireJoinGroup(input: WireGroupInput): Promise<string[] | null> {
  const client = activeWireClient();
  if (!client) return null;

  const response = await client.joinGroup(new JoinGroupRequest(input));
  return response.joinedRoomIds;
}

export async function tryWireBanRoomMember(input: WireBanRoomMemberInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.banRoomMember(
    new BanRoomMemberRequest({
      roomId: input.roomId,
      userId: input.userId,
      reason: input.reason,
      expiresAt: input.expiresAt
        ? Timestamp.fromDate(new Date(input.expiresAt))
        : undefined
    })
  );
  return true;
}

export async function tryWireStartDM(
  serverId: string,
  input: WireStartDMInput
): Promise<string | null> {
  const client = wireEventBusManager.getClient(serverId);
  if (!client) return null;

  const response = await client.startDM(
    new StartDMRequest({
      participantIds: input.participantIds
    })
  );
  return response.room?.id ?? null;
}

export async function tryWireCreateRoom(input: WireCreateRoomInput): Promise<string | null> {
  const client = activeWireClient();
  if (!client) return null;

  const response = await client.createRoom(
    new CreateRoomRequest({
      name: input.name,
      description: input.description ?? '',
      groupId: input.groupId
    })
  );
  return response.room?.id ?? null;
}

export async function tryWireAddReaction(input: WireReactionInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.addReaction(new AddReactionRequest(input));
  return true;
}

export async function tryWireRemoveReaction(input: WireReactionInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.removeReaction(new RemoveReactionRequest(input));
  return true;
}

export async function tryWireSendTypingIndicator(input: WireTypingIndicatorInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.sendTypingIndicator(
    new SendTypingIndicatorRequest({
      roomId: input.roomId,
      threadRootEventId: input.threadRootEventId ?? ''
    })
  );
  return true;
}

export async function tryWireFollowThread(input: WireThreadInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.followThread(new FollowThreadRequest(input));
  return true;
}

export async function tryWireUnfollowThread(input: WireThreadInput): Promise<boolean> {
  const client = activeWireClient();
  if (!client) return false;

  await client.unfollowThread(new UnfollowThreadRequest(input));
  return true;
}

export async function tryWireMarkThreadAsRead(input: WireMarkThreadAsReadInput) {
  const client = activeWireClient();
  if (!client) return null;

  return client.markThreadAsRead(
    new MarkThreadAsReadRequest({
      roomId: input.roomId,
      threadRootEventId: input.threadRootEventId,
      upToEventId: input.upToEventId ?? ''
    })
  );
}

export async function tryWireMarkRoomAsRead(input: WireMarkRoomAsReadInput) {
  const client = activeWireClient();
  if (!client) return null;

  return client.markRoomAsRead(
    new MarkRoomAsReadRequest({
      roomId: input.roomId,
      upToEventId: input.upToEventId ?? ''
    })
  );
}

export function wireMentionConfirmation(error: unknown): WireMentionConfirmation | null {
  if (!(error instanceof WireProtocolError)) return null;
  const confirmation = error.wireError?.mentionConfirmationRequired;
  if (!confirmation?.token || confirmation.recipientCount <= 0) return null;
  return {
    recipientCount: confirmation.recipientCount,
    token: confirmation.token
  };
}

export function activeWireClient() {
  const serverId = getActiveServer();
  if (!serverId) return undefined;
  return wireEventBusManager.getClient(serverId);
}
