<script lang="ts">
  import { DM_SPACE_ID } from '$lib/constants';
  import { graphql } from '$lib/gql';
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
  import MessageEvent from './MessageEvent.svelte';
  import SystemEvent from './SystemEvent.svelte';

  graphql(`
    fragment RoomEventView on SpaceEvent {
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
            users {
              id
              displayName
            }
          }
          updatedAt
          inReplyTo
          inThread
          echoOfEventId
          echoFromThreadRootEventId
          replyCount
          lastReplyAt
          threadParticipants(first: 5) {
            ...UserAvatarUser
          }
          viewerIsFollowingThread
        }
        ... on MessageUpdatedEvent {
          roomId
          messageEventId
        }
        ... on MessageDeletedEvent {
          roomId
          messageEventId
        }
        ... on UserJoinedRoomEvent {
          spaceId
          roomId
        }
        ... on UserLeftRoomEvent {
          spaceId
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
          spaceId
          roomId
          messageEventId
          emoji
        }
        ... on ReactionRemovedEvent {
          spaceId
          roomId
          messageEventId
          emoji
        }
        ... on PresenceChangedEvent {
          status
        }
        ... on UserTypingEvent {
          spaceId
          roomId
          typingThreadRootEventId: threadRootEventId
        }
        ... on VideoProcessingCompletedEvent {
          spaceId
          roomId
          attachmentId
          messageEventId
        }
        ... on SpaceMemberDeletedEvent {
          spaceId
          userId
        }
        ... on CallParticipantJoinedEvent {
          spaceId
          roomId
        }
        ... on CallParticipantLeftEvent {
          spaceId
          roomId
        }
      }
    }
  `);

  let {
    event,
    compact = false,
    spaceId,
    roomId,
    onOpenThread
  }: {
    event: RoomEventViewFragment;
    compact?: boolean;
    spaceId: string;
    roomId: string;
    onOpenThread?: (threadRootEventId: string, highlightEventId?: string) => void;
  } = $props();

  // Join/leave events are suppressed in DM rooms — they're confusing in 1:1 conversations.
  // This also handles historical events from before the backend stopped publishing them.
  // Guard against undefined event during virtualizer data transitions (Svelte 5 reactivity glitch)
  const isDMJoinLeave = $derived(
    spaceId === DM_SPACE_ID &&
      (event?.event?.__typename === 'UserJoinedRoomEvent' ||
        event?.event?.__typename === 'UserLeftRoomEvent')
  );
</script>

{#if !event?.event || isDMJoinLeave}
  <!-- Skip unknown event types, stale virtualizer items, and join/leave events in DM rooms -->
{:else if event.event.__typename === 'MessagePostedEvent'}
  <MessageEvent {event} {compact} {spaceId} {roomId} {onOpenThread} />
{:else}
  <SystemEvent {event} />
{/if}
