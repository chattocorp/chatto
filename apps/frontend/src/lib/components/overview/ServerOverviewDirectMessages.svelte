<!--
@component

The viewer's most recent direct messages on the server Overview: who each
conversation is with, and its unread state or notification badges. A row
opens the conversation; a badge opens its newest notification, as in the
sidebar. Renders nothing without conversations. The owner supplies the
actions, so this component does not depend on app-shell state.
-->
<script lang="ts">
  import { buildDirectMessagePresentation } from '@chatto/client/timeline/users';
  import type { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import { m } from '$lib/i18n/messages';
  import { directMessageLabels } from '$lib/render/directMessageLabels';
  import type { RoomsListItem } from '$lib/state/server/navigation';
  import { Panel, UnreadDot } from '$lib/ui';
  import RoomAttentionBadges from './RoomAttentionBadges.svelte';

  let {
    conversations,
    serverId,
    viewerId,
    isUnread,
    getDisplayName,
    onopennotification
  }: {
    /** Direct messages to list, in display order. */
    conversations: readonly RoomsListItem[];
    /** Server of the conversations; links point to its conversation routes. */
    serverId: string;
    viewerId: string | null;
    /** Whether a conversation has unread messages. */
    isUnread: (roomId: string) => boolean;
    /** Live display names, when the owner has them. */
    getDisplayName?: (userId: string, fallback: string) => string;
    /** Open the newest notification of a conversation at an attention level. */
    onopennotification: (roomId: string, attentionLevel: NotificationAttentionLevel) => void;
  } = $props();
</script>

{#if conversations.length > 0}
  <div data-testid="server-overview-direct-messages">
    <Panel
      title={m('room_list.direct_messages')}
      icon="iconify icon-[uil--comment-alt-message] size-5"
      noPadding
    >
      <ul class="selectable-list">
        {#each conversations as conversation (conversation.id)}
          {@const presentation = buildDirectMessagePresentation(
            conversation.members,
            viewerId,
            directMessageLabels(),
            getDisplayName
          )}
          {@const importantCount = conversation.viewerImportantNotificationCount}
          {@const otherCount = Math.max(
            0,
            conversation.viewerNotificationCount - conversation.viewerImportantNotificationCount
          )}
          <!-- The link opens the conversation; each badge opens its newest notification. -->
          <li class="flex min-w-0 items-center gap-1 selectable-list-item pe-2">
            <a
              class="flex min-w-0 flex-1 items-center gap-2 py-1.5 ps-4"
              href={conversationPath(serverId, conversation.id)}
            >
              <span class="flex shrink-0 -space-x-1" aria-hidden="true">
                {#each presentation.visibleParticipants.slice(0, 3) as participant (participant.id)}
                  <UserAvatar user={participant} size="xs" useLiveProfile={!!getDisplayName} />
                {/each}
              </span>
              <span class="min-w-0 flex-1 truncate font-medium">
                <DirectMessageName
                  participants={conversation.members}
                  currentUserId={viewerId}
                  {getDisplayName}
                />
              </span>
              {#if importantCount === 0 && otherCount === 0 && isUnread(conversation.id)}
                <span class="flex size-6 shrink-0 items-center justify-center">
                  <UnreadDot color="neutral" />
                </span>
                <span class="sr-only">{m('chat.overview.catch_up_unread')}</span>
              {/if}
            </a>
            <RoomAttentionBadges
              roomId={conversation.id}
              isDM
              {importantCount}
              {otherCount}
              onopen={onopennotification}
            />
          </li>
        {/each}
      </ul>
    </Panel>
  </div>
{/if}
