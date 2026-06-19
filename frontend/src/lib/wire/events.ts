import type { Timestamp } from '@bufbuild/protobuf';
import type { Event as DurableEvent } from '$lib/pb/chatto/core/v1/event_pb';
import type { LiveEvent } from '$lib/pb/chatto/core/v1/live_events_pb';
import { InvalidationKind, type StreamEvent } from '$lib/pb/chatto/wire/v1/protocol_pb';

export interface WireMessagePosted {
  eventId: string;
  actorId: string;
  createdAt: string;
  roomId: string;
  threadRootEventId: string | null;
  echoOfEventId: string | null;
  echoFromThreadRootEventId: string | null;
}

export interface WireMessageRetracted {
  eventId: string;
  actorId: string;
  roomId: string;
  messageEventId: string;
}

export interface WireUserTyping {
  userId: string;
  roomId: string;
  threadRootEventId: string | null;
}

export interface WireThreadFollowChanged {
  roomId: string;
  threadRootEventId: string;
  isFollowing: boolean;
}

export function wireDurableEvent(event: StreamEvent): DurableEvent | null {
  return event.payload.case === 'durableEvent' ? event.payload.value : null;
}

export function wireLiveEvent(event: StreamEvent): LiveEvent | null {
  return event.payload.case === 'liveEvent' ? event.payload.value : null;
}

export function wireRoomTimelineId(event: StreamEvent): string | null {
  return event.invalidates.find((hint) => hint.kind === InvalidationKind.ROOM_TIMELINE)?.id ?? null;
}

export function wireMessagePosted(event: StreamEvent): WireMessagePosted | null {
  const durable = wireDurableEvent(event);
  if (!durable || durable.event.case !== 'messagePosted') return null;
  const payload = durable.event.value;
  return {
    eventId: durable.id,
    actorId: durable.actorId,
    createdAt: timestampToIso(durable.createdAt),
    roomId: payload.roomId,
    threadRootEventId: payload.inThread || null,
    echoOfEventId: payload.echoOfEventId || null,
    echoFromThreadRootEventId: payload.echoFromThreadRootEventId || null
  };
}

export function wireMessageRetracted(event: StreamEvent): WireMessageRetracted | null {
  const durable = wireDurableEvent(event);
  if (!durable || durable.event.case !== 'messageRetracted') return null;
  const payload = durable.event.value;
  return {
    eventId: durable.id,
    actorId: durable.actorId,
    roomId: payload.roomId,
    messageEventId: payload.eventId
  };
}

export function wireUserTyping(event: StreamEvent): WireUserTyping | null {
  const live = wireLiveEvent(event);
  if (!live || live.event.case !== 'userTyping') return null;
  const payload = live.event.value;
  return {
    userId: live.actorId,
    roomId: payload.roomId,
    threadRootEventId: payload.threadRootEventId || null
  };
}

export function wireThreadFollowChanged(event: StreamEvent): WireThreadFollowChanged | null {
  const live = wireLiveEvent(event);
  if (!live || live.event.case !== 'threadFollowChanged') return null;
  const payload = live.event.value;
  return {
    roomId: payload.roomId,
    threadRootEventId: payload.threadRootEventId,
    isFollowing: payload.isFollowing
  };
}

export function wireDurableRoomId(event: StreamEvent): string | null {
  const durable = wireDurableEvent(event);
  if (!durable) return null;

  switch (durable.event.case) {
    case 'messagePosted':
    case 'messageEdited':
    case 'messageRetracted':
    case 'reactionAdded':
    case 'reactionRemoved':
    case 'roomCreated':
    case 'roomUpdated':
    case 'roomDeleted':
    case 'roomArchived':
    case 'roomUnarchived':
    case 'userJoinedRoom':
    case 'userLeftRoom':
    case 'voiceCallStarted':
    case 'voiceCallEnded':
      return durable.event.value.roomId || null;
    case 'assetProcessingStarted':
    case 'assetProcessingSucceeded':
    case 'assetProcessingFailed':
      return wireRoomTimelineId(event);
    default:
      return null;
  }
}

function timestampToIso(timestamp: Timestamp | undefined): string {
  return timestamp?.toDate().toISOString() ?? new Date().toISOString();
}
