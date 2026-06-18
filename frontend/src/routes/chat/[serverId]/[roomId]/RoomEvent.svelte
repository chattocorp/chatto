<script lang="ts">
  import { graphql } from '$lib/gql';
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
  import type { MessagesStore } from '$lib/state/room';
  import MessageEvent from './MessageEvent.svelte';
  import SystemEvent from './SystemEvent.svelte';

  graphql(`
    fragment RoomEventView on Event {
      id
      createdAt
      actorId
      actor {
        ...UserAvatarUser
      }
      event {
        __typename
        ... on MessagePostedEvent {
          roomId
          body
          attachments {
            ...MessageAttachmentView
          }
          linkPreview {
            ...LinkPreviewView
          }
          reactions {
            emoji
            count
            hasReacted
            users(first: 5) {
              id
              displayName
            }
          }
          updatedAt
          inReplyTo
          threadRootEventId
          echoOfEventId
          echoFromThreadRootEventId
          channelEchoEventId
          replyCount
          lastReplyAt
          threadParticipants(first: 5) {
            ...UserAvatarUser
          }
          viewerIsFollowingThread
        }
        ... on MessageEditedEvent {
          roomId
          messageEventId
          body
          attachments {
            ...MessageAttachmentView
          }
          linkPreview {
            ...LinkPreviewView
          }
          updatedAt
        }
        ... on MessageRetractedEvent {
          roomId
          messageEventId
          retractedReason: reason
        }
        ... on UserJoinedRoomEvent {
          roomId
        }
        ... on UserLeftRoomEvent {
          roomId
        }
        ... on RoomUpdatedEvent {
          roomId
        }
        ... on RoomDeletedEvent {
          roomId
        }
        ... on RoomArchivedEvent {
          roomId
        }
        ... on RoomUnarchivedEvent {
          roomId
        }
        ... on ReactionAddedEvent {
          roomId
          messageEventId
          emoji
        }
        ... on ReactionRemovedEvent {
          roomId
          messageEventId
          emoji
        }
        ... on PresenceChangedEvent {
          status
        }
        ... on UserTypingEvent {
          roomId
          typingThreadRootEventId: threadRootEventId
        }
        ... on AssetProcessingStartedEvent {
          processingRoomId: roomId
          assetId
          processingMessageEventId: messageEventId
        }
        ... on AssetProcessingSucceededEvent {
          processingRoomId: roomId
          assetId
          processingMessageEventId: messageEventId
        }
        ... on AssetProcessingFailedEvent {
          processingRoomId: roomId
          assetId
          processingMessageEventId: messageEventId
        }
        ... on AssetDeletedEvent {
          deletedRoomId: roomId
          assetId
        }
        ... on ServerMemberDeletedEvent {
          userId
        }
        ... on CallStartedEvent {
          roomId
          callId
        }
        ... on CallParticipantJoinedEvent {
          roomId
          callId
        }
        ... on CallParticipantLeftEvent {
          roomId
          callId
        }
        ... on CallEndedEvent {
          roomId
          callId
        }
        ... on CallEndedEvent {
          roomId
          callId
        }
      }
    }
  `);

  let {
    event,
    compact = false,
    roomId,
    messageStore = null,
    onOpenThread
  }: {
    event: RoomEventViewFragment;
    compact?: boolean;
    roomId: string;
    messageStore?: MessagesStore | null;
    onOpenThread?: (
      threadRootEventId: string,
      highlightEventId?: string,
      quoteText?: string
    ) => void;
  } = $props();

  // Join/leave events are confusing in DM 1:1 conversations. Post-PR(b) we
  // can no longer derive "is this a DM room" from a spaceId — the backend
  // routes both kinds through the same surface. We always render join/leave
  // for now; a future iteration can teach Room.svelte to pass `isDM` down
  // and we can revive the suppression here.
  const isDMJoinLeave = $derived(false);
</script>

{#if !event?.event || isDMJoinLeave}
  <!-- Skip unknown event types, stale virtualizer items, and join/leave events in DM rooms -->
{:else if event.event.__typename === 'MessagePostedEvent'}
  <MessageEvent {event} {compact} {roomId} {messageStore} {onOpenThread} />
{:else}
  <SystemEvent {event} />
{/if}
