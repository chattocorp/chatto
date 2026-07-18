import type { RoomEventPayload } from './types';

export const RoomEventKind = {
  AssetDeleted: 'assetDeleted',
  AssetProcessingFailed: 'assetProcessingFailed',
  AssetProcessingStarted: 'assetProcessingStarted',
  AssetProcessingSucceeded: 'assetProcessingSucceeded',
  CallEnded: 'callEnded',
  CallParticipantJoined: 'callParticipantJoined',
  CallParticipantLeft: 'callParticipantLeft',
  CallStarted: 'callStarted',
  MentionNotification: 'mentionNotification',
  MessageEdited: 'messageEdited',
  MessagePosted: 'messagePosted',
  MessageRetracted: 'messageRetracted',
  NewDirectMessageNotification: 'newDirectMessageNotification',
  PresenceChanged: 'presenceChanged',
  ReactionAdded: 'reactionAdded',
  ReactionRemoved: 'reactionRemoved',
  RoomArchived: 'roomArchived',
  RoomCreated: 'roomCreated',
  RoomDeleted: 'roomDeleted',
  RoomMemberBanned: 'roomMemberBanned',
  RoomMemberUnbanned: 'roomMemberUnbanned',
  RoomUniversalChanged: 'roomUniversalChanged',
  RoomUnarchived: 'roomUnarchived',
  RoomUpdated: 'roomUpdated',
  SessionTerminated: 'sessionTerminated',
  ThreadCreated: 'threadCreated',
  UserCreated: 'userCreated',
  UserCustomStatusCleared: 'userCustomStatusCleared',
  UserCustomStatusSet: 'userCustomStatusSet',
  UserDeleted: 'userDeleted',
  UserJoinedRoom: 'userJoinedRoom',
  UserLeftRoom: 'userLeftRoom',
  UserTyping: 'userTyping'
} as const;

export type RoomEventKind = (typeof RoomEventKind)[keyof typeof RoomEventKind];

export type RoomEventKindSource = object | null | undefined;

export type MessagePostedPayload = Extract<
  RoomEventPayload,
  { kind: typeof RoomEventKind.MessagePosted }
>;

const roomEventKinds = new Set<string>(Object.values(RoomEventKind));

export function roomEventKind(event: RoomEventKindSource): RoomEventKind | null {
  if (!event) return null;
  const localKind = (event as { kind?: unknown }).kind;
  if (typeof localKind === 'string' && roomEventKinds.has(localKind)) {
    return localKind as RoomEventKind;
  }
  return null;
}

export function isMessagePostedEvent(event: RoomEventKindSource): event is MessagePostedPayload {
  return roomEventKind(event) === RoomEventKind.MessagePosted;
}
